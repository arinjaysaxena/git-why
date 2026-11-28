package forge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/arinjaysaxena/git-why/internal/cache"
)

func newCachedClient(t *testing.T, h http.Handler, c *cache.Cache) *GitHub {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	g, err := NewGitHub(Options{Owner: "o", Repo: "r", Token: "tok", BaseURL: srv.URL, Cache: c})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

const issueJSON = `{"number":799,"title":"Retry storm","body":"outage","state":"closed","html_url":"https://x/799","user":{"login":"maya"},"labels":[{"name":"incident"}]}`

func TestRESTRevalidatesWithETag(t *testing.T) {
	var full, conditional atomic.Int32
	c := cache.Open(t.TempDir())
	g := newCachedClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		full.Add(1)
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(issueJSON))
	}), c)

	for i := 0; i < 3; i++ {
		is, err := g.Issue(context.Background(), 799)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if is.Title != "Retry storm" || is.Author != "maya" || len(is.Labels) != 1 || is.Labels[0] != "incident" {
			t.Fatalf("call %d: issue = %+v", i, is)
		}
	}
	if full.Load() != 1 || conditional.Load() != 2 {
		t.Errorf("full=%d conditional=%d, want 1 and 2", full.Load(), conditional.Load())
	}
}

func TestRESTCacheSharedAcrossClientsAndReplacedOnChange(t *testing.T) {
	dir := t.TempDir()
	version := "v1"
	title := "old"
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"`+version+`"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"`+version+`"`)
		_, _ = w.Write([]byte(`{"number":1,"title":"` + title + `"}`))
	})
	first, err := newCachedClient(t, h, cache.Open(dir)).Issue(context.Background(), 1)
	if err != nil || first.Title != "old" {
		t.Fatalf("first: %+v, %v", first, err)
	}
	version, title = "v2", "new"
	second, err := newCachedClient(t, h, cache.Open(dir)).Issue(context.Background(), 1)
	if err != nil || second.Title != "new" {
		t.Fatalf("after change: %+v, %v", second, err)
	}
}

func TestRESTWithoutETagIsNotCached(t *testing.T) {
	var calls atomic.Int32
	c := cache.Open(t.TempDir())
	g := newCachedClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("If-None-Match") != "" {
			t.Error("sent If-None-Match without a stored ETag")
		}
		_, _ = w.Write([]byte(`{"number":1,"title":"t"}`))
	}), c)
	for i := 0; i < 2; i++ {
		if _, err := g.Issue(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
	if s, _ := c.Stats(); s.Entries != 0 {
		t.Errorf("cached %d entries without ETags", s.Entries)
	}
}

func TestRESTErrorsAreNotCachedAndKeepWorking(t *testing.T) {
	c := cache.Open(t.TempDir())
	g := newCachedClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"nope"`)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}), c)
	if _, err := g.Issue(context.Background(), 5); !NotFound(err) {
		t.Fatalf("err = %v, want not found", err)
	}
	if s, _ := c.Stats(); s.Entries != 0 {
		t.Errorf("error response was cached")
	}
}

func TestRESTCacheKeysSeparateRepositories(t *testing.T) {
	c := cache.Open(t.TempDir())
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+r.URL.Path+`"`)
		_, _ = w.Write([]byte(`{"number":1,"title":"` + r.URL.Path + `"}`))
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	for _, repo := range []string{"a", "b"} {
		g, err := NewGitHub(Options{Owner: "o", Repo: repo, BaseURL: srv.URL, Cache: c})
		if err != nil {
			t.Fatal(err)
		}
		is, err := g.Issue(context.Background(), 1)
		if err != nil || is.Title != "/repos/o/"+repo+"/issues/1" {
			t.Fatalf("repo %s: %+v, %v", repo, is, err)
		}
	}
	if s, _ := c.Stats(); s.Entries != 2 || s.Scopes["github.com/o/a"] != 1 || s.Scopes["github.com/o/b"] != 1 {
		t.Errorf("stats = %+v", s)
	}
}
