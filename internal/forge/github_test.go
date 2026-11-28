package forge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.Handler, token string) *GitHub {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	g, err := NewGitHub(Options{Owner: "o", Repo: "r", Token: token, BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestPullRequestsForCommit(t *testing.T) {
	g := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/commits/abc123/pulls" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth = %q", got)
		}
		_, _ = w.Write([]byte(`[
		 {"number":812,"title":"Cap retries","body":"Fixes #799","html_url":"https://x/812","merged_at":"2025-03-11T10:00:00Z","user":{"login":"maya"}},
		 {"number":813,"title":"Draft","body":"","html_url":"https://x/813","merged_at":null,"user":{"login":"sam"}}]`))
	}), "tok")
	prs, err := g.PullRequestsForCommit(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 || prs[0].Number != 812 || !prs[0].Merged || prs[0].Author != "maya" || prs[0].MergedAt.Year() != 2025 {
		t.Fatalf("prs = %+v", prs)
	}
	if prs[1].Merged {
		t.Errorf("unmerged PR reported merged")
	}
}

func TestPullRequestsForUnknownCommit(t *testing.T) {
	g := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"No commit found"}`))
	}), "")
	prs, err := g.PullRequestsForCommit(context.Background(), "deadbeef")
	if err != nil || len(prs) != 0 {
		t.Fatalf("prs=%v err=%v", prs, err)
	}
}

func TestIssue(t *testing.T) {
	g := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected auth header")
		}
		_, _ = w.Write([]byte(`{"number":799,"title":"Retry storms","body":"outage","state":"closed","html_url":"https://x/799","user":{"login":"ops"},"labels":[{"name":"incident"},{"name":"p1"}]}`))
	}), "")
	is, err := g.Issue(context.Background(), 799)
	if err != nil {
		t.Fatal(err)
	}
	if is.Title != "Retry storms" || is.State != "closed" || len(is.Labels) != 2 || is.Labels[1] != "p1" {
		t.Fatalf("issue = %+v", is)
	}
	if g.Authenticated() {
		t.Error("should be unauthenticated")
	}
}

func TestIssueNotFoundIsNotUnavailable(t *testing.T) {
	g := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}), "")
	_, err := g.Issue(context.Background(), 1)
	if !NotFound(err) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestDegradationErrors(t *testing.T) {
	for _, code := range []int{401, 403, 429, 502} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			g := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
			}), "")
			_, err := g.Issue(context.Background(), 1)
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestNetworkFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	g, _ := NewGitHub(Options{Owner: "o", Repo: "r", BaseURL: url})
	if _, err := g.Issue(context.Background(), 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestEnterpriseURLs(t *testing.T) {
	g, err := NewGitHub(Options{Host: "ghe.example.com", Owner: "o", Repo: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if g.baseURL != "https://ghe.example.com/api/v3" || g.graphqlURL != "https://ghe.example.com/api/graphql" {
		t.Fatalf("urls = %s %s", g.baseURL, g.graphqlURL)
	}
	if _, err := NewGitHub(Options{}); err == nil {
		t.Error("expected error for missing owner/repo")
	}
}

func TestLimiterWaitsUntilReset(t *testing.T) {
	now := time.Unix(1000, 0)
	var slept []time.Duration
	l := newLimiter()
	l.now = func() time.Time { return now }
	l.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "0")
	h.Set("X-RateLimit-Reset", "1005")
	l.observe(h)
	if err := l.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 || slept[0] != 5*time.Second {
		t.Fatalf("slept = %v", slept)
	}
}

func TestLimiterGivesUpOnLongReset(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter()
	l.now = func() time.Time { return now }
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "0")
	h.Set("X-RateLimit-Reset", "5000")
	l.observe(h)
	if err := l.wait(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestLimiterRetryAfter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter()
	l.now = func() time.Time { return now }
	var slept time.Duration
	l.sleep = func(_ context.Context, d time.Duration) error { slept = d; return nil }
	h := http.Header{}
	h.Set("Retry-After", "3")
	l.observe(h)
	_ = l.wait(context.Background())
	if slept != 3*time.Second {
		t.Fatalf("slept = %v", slept)
	}
}

func TestLimiterDecrementsRemaining(t *testing.T) {
	l := newLimiter()
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "2")
	l.observe(h)
	l.now = func() time.Time { return time.Unix(0, 0) }
	l.sleep = func(context.Context, time.Duration) error { t.Error("should not sleep"); return nil }
	_ = l.wait(context.Background())
	if l.remaining != 1 {
		t.Fatalf("remaining = %d", l.remaining)
	}
}

func TestMapBoundedAndOrdered(t *testing.T) {
	var cur, peak int32
	items := make([]int, 20)
	for i := range items {
		items[i] = i
	}
	out, err := Map(context.Background(), 3, items, func(_ context.Context, n int) (int, error) {
		c := atomic.AddInt32(&cur, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if c <= p || atomic.CompareAndSwapInt32(&peak, p, c) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		return n * n, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range out {
		if v != i*i {
			t.Fatalf("out[%d] = %d", i, v)
		}
	}
	if peak > 3 {
		t.Fatalf("peak concurrency %d > 3", peak)
	}
}

func TestMapFirstErrorStops(t *testing.T) {
	boom := errors.New("boom")
	_, err := Map(context.Background(), 2, []int{1, 2, 3, 4}, func(_ context.Context, n int) (int, error) {
		if n == 2 {
			return 0, boom
		}
		return n, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverToken(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	noGH := func(context.Context, string, ...string) (string, error) { return "", errors.New("not installed") }
	ctx := context.Background()

	if tok, src := DiscoverToken(ctx, "github.com", env(map[string]string{"GITHUB_TOKEN": " a ", "GH_TOKEN": "b"}), noGH); tok != "a" || src != SourceGitHub {
		t.Errorf("got %q %s", tok, src)
	}
	if tok, src := DiscoverToken(ctx, "github.com", env(map[string]string{"GH_TOKEN": "b"}), noGH); tok != "b" || src != SourceGH {
		t.Errorf("got %q %s", tok, src)
	}
	var gotArgs []string
	gh := func(_ context.Context, name string, args ...string) (string, error) {
		gotArgs = append([]string{name}, args...)
		return "ghtok\n", nil
	}
	if tok, src := DiscoverToken(ctx, "ghe.example.com", env(nil), gh); tok != "ghtok" || src != SourceGHCLI {
		t.Errorf("got %q %s", tok, src)
	}
	if len(gotArgs) != 5 || gotArgs[4] != "ghe.example.com" {
		t.Errorf("args = %v", gotArgs)
	}
	if tok, src := DiscoverToken(ctx, "github.com", env(nil), noGH); tok != "" || src != SourceNone {
		t.Errorf("got %q %s", tok, src)
	}
}
