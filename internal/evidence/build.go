package evidence

import (
	"sort"

	"github.com/arinjaysaxena/git-why/internal/forge"
	"github.com/arinjaysaxena/git-why/internal/history"
)

// Input is everything gathered for one target. Builder performs no I/O: the
// caller fetches, then hands the results over, which keeps the graph logic
// deterministic and testable.
type Input struct {
	// Owner and Repo identify the repository; they decide which references are local.
	Owner, Repo string
	// Commits are the walked commits, newest first.
	Commits []history.Commit
	// Paths lists every name the target file has had (see history.Result.Paths).
	Paths []string
	// Range is the target line range at the walked revision.
	Range forge.Range
	// PRs maps a commit SHA to the pull requests that contain it.
	PRs map[string][]forge.PullRequest
	// Details maps a PR number to its fetched details (closing issues, review threads).
	Details map[int]forge.PullRequestDetails
	// Issues maps an issue number to its fetched content.
	Issues map[int]forge.Issue
	// Maintainers are logins known to have write access; used as a ranking signal.
	Maintainers map[string]bool
}

// IssueCandidates lists the local issue-or-PR numbers referenced anywhere in
// the evidence that are not yet in in.Issues, in ascending order. Callers
// fetch these and rerun Build. Numbers that are known pull requests are skipped.
func IssueCandidates(in Input) []int {
	prs := knownPRs(in)
	want := map[int]bool{}
	add := func(refs []Ref) {
		for _, r := range refs {
			if !r.Local(in.Owner, in.Repo) || r.Kind == "pr" || prs[r.Number] {
				continue
			}
			if _, have := in.Issues[r.Number]; !have {
				want[r.Number] = true
			}
		}
	}
	for _, c := range in.Commits {
		add(ParseRefs(c.Subject + "\n" + c.Body))
	}
	for n, d := range in.Details {
		add(ParseRefs(d.Body))
		for _, i := range d.ClosingIssues {
			if _, have := in.Issues[i]; !have && i != n {
				want[i] = true
			}
		}
		for _, c := range d.Comments {
			add(ParseRefs(c.Body))
		}
	}
	for _, list := range in.PRs {
		for _, p := range list {
			add(ParseRefs(p.Body))
		}
	}
	out := make([]int, 0, len(want))
	for n := range want {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func knownPRs(in Input) map[int]bool {
	m := map[int]bool{}
	for _, list := range in.PRs {
		for _, p := range list {
			m[p.Number] = true
		}
	}
	for n := range in.Details {
		m[n] = true
	}
	return m
}

// Build assembles the evidence graph. Commits keep their newest-first order;
// pull requests, issues and review comments are numbered in order of discovery.
func Build(in Input) (*Graph, error) {
	b := &builder{in: in, g: NewGraph(), prs: map[int]*Node{}, issues: map[int]*Node{}, unresolved: map[string]Ref{}}
	return b.build()
}

type builder struct {
	in         Input
	g          *Graph
	prs        map[int]*Node
	issues     map[int]*Node
	unresolved map[string]Ref
	pending    []pendingMention
}

// pendingMention is a text reference resolved once every node exists, so a
// reference can point at a PR or issue that is added later in the walk.
type pendingMention struct {
	from string
	text string
}

func (b *builder) build() (*Graph, error) {
	for _, c := range b.in.Commits {
		cn := b.addCommit(c)
		for _, p := range b.in.PRs[c.SHA] {
			pn := b.pr(p)
			if err := b.g.Link(cn.ID, pn.ID, EdgeIntroducedBy); err != nil {
				return nil, err
			}
			if err := b.addDiscussion(pn, c); err != nil {
				return nil, err
			}
		}
	}
	// Closing issues come after PRs so issue numbering follows PR order.
	for _, pn := range append([]*Node(nil), b.g.OfKind(KindPullRequest)...) {
		d, ok := b.in.Details[pn.Number]
		if !ok {
			continue
		}
		for _, num := range d.ClosingIssues {
			in := b.issue(num)
			if in == nil {
				continue
			}
			if err := b.g.Link(pn.ID, in.ID, EdgeCloses); err != nil {
				return nil, err
			}
		}
	}
	if err := b.resolveMentions(); err != nil {
		return nil, err
	}
	b.g.Unresolved = b.unresolvedRefs()
	return b.g, nil
}

func (b *builder) maintainer(login string) bool { return login != "" && b.in.Maintainers[login] }

func (b *builder) addCommit(c history.Commit) *Node {
	added, removed := touched(c.Hunks)
	n := b.g.Add(Node{
		Kind: KindCommit, SHA: c.SHA, Title: c.Subject, Body: c.Body, Author: c.Author, Time: c.Time,
		Introduced: c.Introduced, Renamed: c.Renamed, Added: added, Removed: removed,
		LineOverlap: overlap(c.Hunks, b.in.Range), Maintainer: b.maintainer(c.Author),
	})
	b.pending = append(b.pending, pendingMention{from: n.ID, text: c.Subject + "\n" + c.Body})
	return n
}

// pr returns the node for p, creating it on first sight. Details, when
// fetched, win over the thinner commit-lookup view.
func (b *builder) pr(p forge.PullRequest) *Node {
	if n, ok := b.prs[p.Number]; ok {
		return n
	}
	if d, ok := b.in.Details[p.Number]; ok {
		p = mergePR(p, d.PullRequest)
	}
	n := b.g.Add(Node{
		Kind: KindPullRequest, Number: p.Number, Title: p.Title, Body: p.Body, Author: p.Author, URL: p.URL,
		Time: p.MergedAt, Merged: p.Merged, Maintainer: b.maintainer(p.Author),
	})
	b.prs[p.Number] = n
	b.pending = append(b.pending, pendingMention{from: n.ID, text: p.Body})
	return n
}

// mergePR fills gaps in base from detail without discarding what base knows.
func mergePR(base, detail forge.PullRequest) forge.PullRequest {
	if base.Title == "" {
		base.Title = detail.Title
	}
	if base.Body == "" {
		base.Body = detail.Body
	}
	if base.Author == "" {
		base.Author = detail.Author
	}
	if base.URL == "" {
		base.URL = detail.URL
	}
	if base.MergedAt.IsZero() {
		base.MergedAt = detail.MergedAt
	}
	base.Merged = base.Merged || detail.Merged
	return base
}

// addDiscussion adds the review comments of pn that sit on the lines commit c changed.
func (b *builder) addDiscussion(pn *Node, c history.Commit) error {
	d, ok := b.in.Details[pn.Number]
	if !ok {
		return nil
	}
	r := hunkRange(c.Hunks)
	if r == (forge.Range{}) {
		r = b.in.Range
	}
	for _, t := range forge.FilterThreads(d.Threads, b.in.Paths, r, r) {
		line := t.Line
		if line == 0 {
			line = t.OriginalLine
		}
		for _, cm := range t.Comments {
			if b.hasComment(pn.ID, cm.URL, cm.Body) {
				continue
			}
			rn := b.g.Add(Node{
				Kind: KindReviewComment, Body: cm.Body, Author: cm.Author, Time: cm.CreatedAt, URL: cm.URL,
				Path: t.Path, Line: line, Resolved: t.Resolved, Outdated: t.Outdated, Maintainer: b.maintainer(cm.Author),
			})
			if err := b.g.Link(rn.ID, pn.ID, EdgeDiscussedIn); err != nil {
				return err
			}
			b.pending = append(b.pending, pendingMention{from: rn.ID, text: cm.Body})
		}
	}
	return nil
}

func (b *builder) hasComment(prID, url, body string) bool {
	for _, n := range b.g.In(prID, EdgeDiscussedIn) {
		if (url != "" && n.URL == url) || (url == "" && n.Body == body) {
			return true
		}
	}
	return false
}

// issue returns the node for issue number num, or nil when it was not fetched
// (the number is then recorded as unresolved).
func (b *builder) issue(num int) *Node {
	if n, ok := b.issues[num]; ok {
		return n
	}
	is, ok := b.in.Issues[num]
	if !ok {
		b.noteUnresolved(Ref{Number: num})
		return nil
	}
	n := b.g.Add(Node{
		Kind: KindIssue, Number: num, Title: is.Title, Body: is.Body, Author: is.Author, URL: is.URL,
		State: is.State, Labels: is.Labels, Maintainer: b.maintainer(is.Author),
	})
	b.issues[num] = n
	return n
}

func (b *builder) resolveMentions() error {
	for i := 0; i < len(b.pending); i++ { // issue() may not append, but index keeps this safe if it ever does
		pm := b.pending[i]
		for _, ref := range ParseRefs(pm.text) {
			if !ref.Local(b.in.Owner, b.in.Repo) {
				b.noteUnresolved(ref)
				continue
			}
			target := b.target(ref)
			if target == nil || target.ID == pm.from {
				continue
			}
			typ := EdgeMentions
			if ref.Closes && target.Kind == KindIssue && b.g.Node(pm.from).Kind != KindReviewComment {
				typ = EdgeCloses
			}
			if err := b.g.Link(pm.from, target.ID, typ); err != nil {
				return err
			}
		}
	}
	return nil
}

// target resolves a local reference to an existing PR or issue node.
func (b *builder) target(ref Ref) *Node {
	if ref.Kind != "issue" {
		if n, ok := b.prs[ref.Number]; ok {
			return n
		}
	}
	if ref.Kind != "pr" {
		if n := b.issue(ref.Number); n != nil {
			return n
		}
		return nil
	}
	b.noteUnresolved(ref)
	return nil
}

func (b *builder) noteUnresolved(r Ref) {
	// Keep the identity of the reference only; how it was written is irrelevant once deduplicated.
	r = Ref{Number: r.Number, Owner: r.Owner, Repo: r.Repo, Kind: r.Kind, Raw: refText(r)}
	b.unresolved[r.Raw] = r
}

func refText(r Ref) string {
	if r.Owner != "" || r.Repo != "" {
		return r.Owner + "/" + r.Repo + "#" + itoa(r.Number)
	}
	return "#" + itoa(r.Number)
}

// unresolvedRefs lists unresolved references: this repository first, then others, each by number.
func (b *builder) unresolvedRefs() []Ref {
	out := make([]Ref, 0, len(b.unresolved))
	for _, r := range b.unresolved {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, c := out[i], out[j]
		if a.Owner != c.Owner || a.Repo != c.Repo {
			return a.Owner+"/"+a.Repo < c.Owner+"/"+c.Repo
		}
		return a.Number < c.Number
	})
	return out
}

// touched returns the added and removed lines of all hunks.
func touched(hunks []history.Hunk) (added, removed []string) {
	for _, h := range hunks {
		added = append(added, h.Added...)
		removed = append(removed, h.Removed...)
	}
	return added, removed
}

// hunkRange is the union of the new-side line spans of the hunks.
func hunkRange(hunks []history.Hunk) forge.Range {
	var r forge.Range
	for _, h := range hunks {
		if h.NewLines == 0 {
			continue
		}
		start, end := h.NewStart, h.NewStart+h.NewLines-1
		if r == (forge.Range{}) {
			r = forge.Range{Start: start, End: end}
			continue
		}
		r.Start, r.End = min(r.Start, start), max(r.End, end)
	}
	return r
}

// overlap is the fraction of the target range covered by the hunks' new-side spans.
func overlap(hunks []history.Hunk, target forge.Range) float64 {
	size := target.End - target.Start + 1
	if size <= 0 {
		return 0
	}
	covered := map[int]bool{}
	for _, h := range hunks {
		for l := max(h.NewStart, target.Start); l <= min(h.NewStart+h.NewLines-1, target.End); l++ {
			covered[l] = true
		}
	}
	return float64(len(covered)) / float64(size)
}
