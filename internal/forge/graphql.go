package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Comment is one message inside a review thread or on a pull request.
type Comment struct {
	Author    string
	Body      string
	URL       string
	CreatedAt time.Time
}

// ReviewThread is a code-review conversation anchored to a file.
//
// Line is the line in the current diff (0 when the thread is outdated, i.e.
// the code it pointed at has since changed); OriginalLine is where the thread
// was anchored in the diff version it was written against, so it keeps
// meaning after later pushes drift the code.
type ReviewThread struct {
	Path              string
	Line              int
	StartLine         int
	OriginalLine      int
	OriginalStartLine int
	DiffHunk          string
	Resolved          bool
	Outdated          bool
	Comments          []Comment
}

// PullRequestDetails is a pull request with its linked issues and discussion.
type PullRequestDetails struct {
	PullRequest
	ClosingIssues []int
	Comments      []Comment // PR-level conversation comments
	Threads       []ReviewThread
	// Truncated is true when pagination limits cut off threads or comments.
	Truncated bool
}

const (
	maxThreadPages = 5
	pageSize       = 50
)

const prQuery = `query($owner:String!,$repo:String!,$number:Int!,$cursor:String){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$number){
      number title body url merged mergedAt
      author{login}
      closingIssuesReferences(first:20){nodes{number}}
      comments(first:30){nodes{body url createdAt author{login}}}
      reviewThreads(first:50,after:$cursor){
        pageInfo{hasNextPage endCursor}
        nodes{
          path line startLine originalLine originalStartLine isResolved isOutdated diffHunk
          comments(first:20){nodes{body url createdAt author{login}}}
        }
      }
    }
  }
}`

type gqlComment struct {
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
	Author    *struct {
		Login string `json:"login"`
	} `json:"author"`
}

func (c gqlComment) convert() Comment {
	out := Comment{Body: c.Body, URL: c.URL, CreatedAt: c.CreatedAt}
	if c.Author != nil {
		out.Author = c.Author.Login
	}
	return out
}

type gqlPR struct {
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	URL      string     `json:"url"`
	Merged   bool       `json:"merged"`
	MergedAt *time.Time `json:"mergedAt"`
	Author   *struct {
		Login string `json:"login"`
	} `json:"author"`
	Closing struct {
		Nodes []struct {
			Number int `json:"number"`
		} `json:"nodes"`
	} `json:"closingIssuesReferences"`
	Comments struct {
		Nodes []gqlComment `json:"nodes"`
	} `json:"comments"`
	ReviewThreads struct {
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
		Nodes []struct {
			Path              string `json:"path"`
			Line              *int   `json:"line"`
			StartLine         *int   `json:"startLine"`
			OriginalLine      *int   `json:"originalLine"`
			OriginalStartLine *int   `json:"originalStartLine"`
			IsResolved        bool   `json:"isResolved"`
			IsOutdated        bool   `json:"isOutdated"`
			DiffHunk          string `json:"diffHunk"`
			Comments          struct {
				Nodes []gqlComment `json:"nodes"`
			} `json:"comments"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
}

type gqlResponse struct {
	Data struct {
		Repository *struct {
			PullRequest *gqlPR `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"errors"`
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// PullRequestDetails fetches a PR, its closing issues, conversation comments
// and all review threads (paginated) in as few round trips as possible.
func (g *GitHub) PullRequestDetails(ctx context.Context, number int) (PullRequestDetails, error) {
	var d PullRequestDetails
	cursor := ""
	for page := 0; ; page++ {
		pr, err := g.queryPR(ctx, number, cursor)
		if err != nil {
			return PullRequestDetails{}, err
		}
		if page == 0 {
			d.PullRequest = PullRequest{Number: pr.Number, Title: pr.Title, Body: pr.Body, URL: pr.URL, Merged: pr.Merged}
			if pr.Author != nil {
				d.Author = pr.Author.Login
			}
			if pr.MergedAt != nil {
				d.MergedAt = *pr.MergedAt
			}
			for _, n := range pr.Closing.Nodes {
				d.ClosingIssues = append(d.ClosingIssues, n.Number)
			}
			for _, c := range pr.Comments.Nodes {
				d.Comments = append(d.Comments, c.convert())
			}
		}
		for _, t := range pr.ReviewThreads.Nodes {
			rt := ReviewThread{
				Path: t.Path, Line: deref(t.Line), StartLine: deref(t.StartLine),
				OriginalLine: deref(t.OriginalLine), OriginalStartLine: deref(t.OriginalStartLine),
				DiffHunk: t.DiffHunk, Resolved: t.IsResolved, Outdated: t.IsOutdated,
			}
			for _, c := range t.Comments.Nodes {
				rt.Comments = append(rt.Comments, c.convert())
			}
			d.Threads = append(d.Threads, rt)
		}
		if !pr.ReviewThreads.PageInfo.HasNextPage {
			return d, nil
		}
		if page+1 >= maxThreadPages {
			d.Truncated = true
			return d, nil
		}
		cursor = pr.ReviewThreads.PageInfo.EndCursor
	}
}

func (g *GitHub) queryPR(ctx context.Context, number int, cursor string) (*gqlPR, error) {
	vars := map[string]any{"owner": g.owner, "repo": g.repo, "number": number}
	if cursor != "" {
		vars["cursor"] = cursor
	}
	body, err := json.Marshal(map[string]any{"query": prQuery, "variables": vars})
	if err != nil {
		return nil, err
	}
	if g.token == "" {
		// GraphQL requires authentication; fail fast so callers degrade.
		return nil, fmt.Errorf("github: graphql needs a token: %w", ErrUnavailable)
	}
	req, err := g.newRequest(ctx, http.MethodPost, g.graphqlURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var resp gqlResponse
	if err := g.do(ctx, req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, 0, len(resp.Errors))
		notFound := false
		for _, e := range resp.Errors {
			msgs = append(msgs, e.Message)
			if e.Type == "NOT_FOUND" {
				notFound = true
			}
		}
		err := &APIError{Status: http.StatusOK, Message: strings.Join(msgs, "; ")}
		if notFound {
			err.Status = http.StatusNotFound
		}
		return nil, err
	}
	if resp.Data.Repository == nil || resp.Data.Repository.PullRequest == nil {
		return nil, &APIError{Status: http.StatusNotFound, Message: fmt.Sprintf("pull request #%d not found", number)}
	}
	return resp.Data.Repository.PullRequest, nil
}

// Range is an inclusive 1-based line range.
type Range struct{ Start, End int }

func (r Range) overlaps(start, end int) bool {
	if r.Start <= 0 || r.End < r.Start || end <= 0 {
		return false
	}
	if start <= 0 || start > end {
		start = end
	}
	return start <= r.End && end >= r.Start
}

// FilterThreads keeps review threads anchored on path and overlapping the
// target lines. current is the target range in the coordinates of the current
// diff (matches Line/StartLine); original is the same code's range in the
// coordinates the thread was written against (matches OriginalLine and
// OriginalStartLine), which survives later pushes that shifted the code.
// Pass a zero Range to skip one of the two comparisons. paths lists every
// name the file has had, so threads written before a rename still match.
func FilterThreads(threads []ReviewThread, paths []string, current, original Range) []ReviewThread {
	want := make(map[string]bool, len(paths))
	for _, p := range paths {
		want[p] = true
	}
	var out []ReviewThread
	for _, t := range threads {
		if !want[t.Path] {
			continue
		}
		if (t.Line > 0 && current.overlaps(t.StartLine, t.Line)) ||
			(t.OriginalLine > 0 && original.overlaps(t.OriginalStartLine, t.OriginalLine)) {
			out = append(out, t)
		}
	}
	return out
}

// MentionsSymbol returns the PR-level comments that mention symbol, so general
// discussion about a function is kept even when it is not anchored to a line.
func MentionsSymbol(comments []Comment, symbol string) []Comment {
	if symbol == "" {
		return nil
	}
	var out []Comment
	for _, c := range comments {
		if strings.Contains(c.Body, symbol) {
			out = append(out, c)
		}
	}
	return out
}
