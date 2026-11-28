package forge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "github", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func graphqlServer(t *testing.T, h func(vars map[string]any) (int, []byte)) *GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(raw, &req); err != nil || req.Query == "" {
			t.Errorf("bad request body: %v", err)
		}
		code, body := h(req.Variables)
		w.WriteHeader(code)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	g, err := NewGitHub(Options{Owner: "o", Repo: "r", Token: "tok", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestPullRequestDetailsPaginates(t *testing.T) {
	var cursors []any
	g := graphqlServer(t, func(vars map[string]any) (int, []byte) {
		cursors = append(cursors, vars["cursor"])
		if vars["cursor"] == "CUR1" {
			return 200, fixture(t, "pr812_page2.json")
		}
		return 200, fixture(t, "pr812_page1.json")
	})
	d, err := g.PullRequestDetails(context.Background(), 812)
	if err != nil {
		t.Fatal(err)
	}
	if len(cursors) != 2 || cursors[0] != nil || cursors[1] != "CUR1" {
		t.Fatalf("cursors = %v", cursors)
	}
	if d.Number != 812 || !d.Merged || d.Author != "maya" || d.MergedAt.IsZero() {
		t.Fatalf("pr = %+v", d.PullRequest)
	}
	if len(d.ClosingIssues) != 1 || d.ClosingIssues[0] != 799 {
		t.Fatalf("closing = %v", d.ClosingIssues)
	}
	if len(d.Threads) != 3 || d.Truncated {
		t.Fatalf("threads = %d truncated=%v", len(d.Threads), d.Truncated)
	}
	if d.Comments[1].Author != "" {
		t.Errorf("ghost author should be empty, got %q", d.Comments[1].Author)
	}
	th := d.Threads[0]
	if th.Line != 0 || th.OriginalLine != 10 || !th.Outdated || !th.Resolved || len(th.Comments) != 1 {
		t.Fatalf("thread = %+v", th)
	}
}

func TestPullRequestDetailsTruncates(t *testing.T) {
	calls := 0
	g := graphqlServer(t, func(map[string]any) (int, []byte) {
		calls++
		return 200, fixture(t, "pr812_page1.json") // always claims a next page
	})
	d, err := g.PullRequestDetails(context.Background(), 812)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Truncated || calls != maxThreadPages {
		t.Fatalf("truncated=%v calls=%d", d.Truncated, calls)
	}
}

func TestPullRequestDetailsErrors(t *testing.T) {
	g := graphqlServer(t, func(map[string]any) (int, []byte) {
		return 200, []byte(`{"data":{"repository":{"pullRequest":null}},"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a PullRequest"}]}`)
	})
	if _, err := g.PullRequestDetails(context.Background(), 1); !NotFound(err) {
		t.Fatalf("err = %v", err)
	}
	g = graphqlServer(t, func(map[string]any) (int, []byte) {
		return 403, []byte(`{"message":"API rate limit exceeded"}`)
	})
	if _, err := g.PullRequestDetails(context.Background(), 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	g = graphqlServer(t, func(map[string]any) (int, []byte) {
		return 200, []byte(`{"data":{"repository":{"pullRequest":null}}}`)
	})
	if _, err := g.PullRequestDetails(context.Background(), 1); !NotFound(err) {
		t.Fatalf("null PR err = %v", err)
	}
}

func TestPullRequestDetailsRequiresToken(t *testing.T) {
	g, _ := NewGitHub(Options{Owner: "o", Repo: "r", BaseURL: "http://127.0.0.1:1"})
	if _, err := g.PullRequestDetails(context.Background(), 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestFilterThreadsDriftedOriginalLine(t *testing.T) {
	threads := []ReviewThread{
		{Path: "internal/retry/backoff.go", Line: 0, OriginalLine: 10, Outdated: true},                        // drifted: only original matches
		{Path: "internal/retry/backoff.go", Line: 42, StartLine: 40, OriginalLine: 42, OriginalStartLine: 40}, // current matches
		{Path: "internal/retry/backoff.go", Line: 90, OriginalLine: 90},                                       // elsewhere in file
		{Path: "README.md", Line: 42, OriginalLine: 10},                                                       // other file
		{Path: "old/backoff.go", OriginalLine: 11, Outdated: true},                                            // pre-rename path
	}
	paths := []string{"internal/retry/backoff.go", "old/backoff.go"}
	got := FilterThreads(threads, paths, Range{41, 43}, Range{9, 12})
	if len(got) != 3 {
		t.Fatalf("got %d threads: %+v", len(got), got)
	}
	if got[0].OriginalLine != 10 || got[1].Line != 42 || got[2].Path != "old/backoff.go" {
		t.Fatalf("unexpected selection %+v", got)
	}
	// Without an original-coordinate range only live anchors can match.
	got = FilterThreads(threads, paths, Range{41, 43}, Range{})
	if len(got) != 1 || got[0].Line != 42 {
		t.Fatalf("current-only = %+v", got)
	}
	if got := FilterThreads(threads, nil, Range{1, 100}, Range{1, 100}); len(got) != 0 {
		t.Fatalf("no paths should match nothing, got %d", len(got))
	}
}

func TestRangeOverlapMultiLineThread(t *testing.T) {
	// A thread spanning 30-35 overlaps a target 35-40 at its boundary only.
	th := []ReviewThread{{Path: "f", Line: 35, StartLine: 30}}
	if len(FilterThreads(th, []string{"f"}, Range{35, 40}, Range{})) != 1 {
		t.Error("boundary overlap should match")
	}
	if len(FilterThreads(th, []string{"f"}, Range{36, 40}, Range{})) != 0 {
		t.Error("adjacent range must not match")
	}
}

func TestMentionsSymbol(t *testing.T) {
	cs := []Comment{{Body: "keep maxAttempts small"}, {Body: "lgtm"}}
	if got := MentionsSymbol(cs, "maxAttempts"); len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if MentionsSymbol(cs, "") != nil {
		t.Error("empty symbol should match nothing")
	}
}
