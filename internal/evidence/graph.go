package evidence

import (
	"fmt"
	"time"
)

// Kind is the type of an evidence node.
type Kind string

// Node kinds. Their IDs are prefixed C, PR, I and R respectively.
const (
	KindCommit        Kind = "commit"
	KindPullRequest   Kind = "pr"
	KindIssue         Kind = "issue"
	KindReviewComment Kind = "review_comment"
)

var idPrefix = map[Kind]string{KindCommit: "C", KindPullRequest: "PR", KindIssue: "I", KindReviewComment: "R"}

// EdgeType says how two nodes relate.
type EdgeType string

// Edge types. Direction is documented per type.
const (
	// EdgeIntroducedBy goes from a commit to the pull request that brought it in.
	EdgeIntroducedBy EdgeType = "introduced-by"
	// EdgeDiscussedIn goes from a review comment to the pull request it belongs to.
	EdgeDiscussedIn EdgeType = "discussed-in"
	// EdgeCloses goes from a pull request (or commit) to an issue it closes.
	EdgeCloses EdgeType = "closes"
	// EdgeMentions goes from any node to an issue or pull request its text refers to.
	EdgeMentions EdgeType = "mentions"
)

// Node is one piece of evidence. Fields that do not apply to a kind stay zero.
type Node struct {
	// ID is stable within one graph and is what narration cites: C1, PR2, I1, R3.
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`

	Title  string    `json:"title,omitempty"`
	Body   string    `json:"body,omitempty"`
	Author string    `json:"author,omitempty"`
	Time   time.Time `json:"time,omitzero"`
	URL    string    `json:"url,omitempty"`

	// SHA is the full commit hash (commits).
	SHA string `json:"sha,omitempty"`
	// Number is the PR or issue number.
	Number int `json:"number,omitempty"`
	// Merged reports whether a pull request was merged.
	Merged bool `json:"merged,omitempty"`
	// State is "open" or "closed" for issues.
	State string `json:"state,omitempty"`
	// Labels are issue labels.
	Labels []string `json:"labels,omitempty"`

	// Path and Line locate a review comment (Line is the current line when
	// known, otherwise the original line).
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Resolved bool   `json:"resolved,omitempty"`
	Outdated bool   `json:"outdated,omitempty"`

	// Introduced marks the commit that created the traced lines.
	Introduced bool `json:"introduced,omitempty"`
	// Renamed marks a commit that moved the file.
	Renamed bool `json:"renamed,omitempty"`
	// Added and Removed are the target-range lines a commit touched.
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
	// LineOverlap is, for commits, the fraction (0-1) of the target range the
	// commit's hunks cover; ranking uses it.
	LineOverlap float64 `json:"lineOverlap,omitempty"`
	// Maintainer is true when the author is known to be a repository maintainer.
	Maintainer bool `json:"maintainer,omitempty"`
}

// Edge is a directed, typed link between two node IDs.
type Edge struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Type EdgeType `json:"type"`
}

// Graph is the evidence for one target.
type Graph struct {
	Nodes []*Node `json:"nodes"`
	Edges []Edge  `json:"edges"`
	// Unresolved are references found in the text that were not fetched
	// (other repositories, or lookups that failed or were skipped).
	Unresolved []Ref `json:"unresolved,omitempty"`

	byID map[string]*Node
	seq  map[Kind]int
}

// NewGraph returns an empty graph.
func NewGraph() *Graph {
	return &Graph{byID: map[string]*Node{}, seq: map[Kind]int{}}
}

// Add appends n, assigning its ID, and returns it.
func (g *Graph) Add(n Node) *Node {
	g.seq[n.Kind]++
	n.ID = fmt.Sprintf("%s%d", idPrefix[n.Kind], g.seq[n.Kind])
	p := &n
	g.Nodes = append(g.Nodes, p)
	g.byID[n.ID] = p
	return p
}

// Link adds an edge unless an identical one exists. Unknown IDs are rejected
// so a typo cannot silently produce a dangling edge.
func (g *Graph) Link(from, to string, t EdgeType) error {
	if g.byID[from] == nil || g.byID[to] == nil {
		return fmt.Errorf("evidence: link %s -[%s]-> %s: unknown node", from, t, to)
	}
	// "closes" subsumes "mentions" between the same pair of nodes.
	kept := make([]Edge, 0, len(g.Edges)+1)
	for _, e := range g.Edges {
		if e.From == from && e.To == to {
			if e.Type == t || (t == EdgeMentions && e.Type == EdgeCloses) {
				return nil
			}
			if t == EdgeCloses && e.Type == EdgeMentions {
				continue
			}
		}
		kept = append(kept, e)
	}
	kept = append(kept, Edge{From: from, To: to, Type: t})
	g.Edges = kept
	return nil
}

// Node returns the node with the given ID, or nil.
func (g *Graph) Node(id string) *Node { return g.byID[id] }

// OfKind returns the nodes of kind k in insertion order.
func (g *Graph) OfKind(k Kind) []*Node {
	var out []*Node
	for _, n := range g.Nodes {
		if n.Kind == k {
			out = append(out, n)
		}
	}
	return out
}

// Out returns the targets of edges of type t leaving id.
func (g *Graph) Out(id string, t EdgeType) []*Node {
	var out []*Node
	for _, e := range g.Edges {
		if e.From == id && e.Type == t {
			out = append(out, g.byID[e.To])
		}
	}
	return out
}

// In returns the sources of edges of type t arriving at id.
func (g *Graph) In(id string, t EdgeType) []*Node {
	var out []*Node
	for _, e := range g.Edges {
		if e.To == id && e.Type == t {
			out = append(out, g.byID[e.From])
		}
	}
	return out
}

// ByNumber finds the pull request or issue with the given number.
func (g *Graph) ByNumber(k Kind, number int) *Node {
	for _, n := range g.Nodes {
		if n.Kind == k && n.Number == number {
			return n
		}
	}
	return nil
}

// Counts reports how many nodes of each kind the graph holds.
func (g *Graph) Counts() map[Kind]int {
	c := map[Kind]int{}
	for _, n := range g.Nodes {
		c[n.Kind]++
	}
	return c
}
