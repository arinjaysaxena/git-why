package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPutGetRoundTrip(t *testing.T) {
	c := Open(filepath.Join(t.TempDir(), "nested", "cache"))
	if _, ok := c.Get("k"); ok {
		t.Fatal("empty cache returned a hit")
	}
	at := time.Date(2025, 3, 11, 9, 0, 0, 0, time.UTC)
	if err := c.Put("github.com/o/r/pulls/1", Entry{ETag: `W/"abc"`, Body: []byte(`{"n":1}`), Stored: at}); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get("github.com/o/r/pulls/1")
	if !ok || got.ETag != `W/"abc"` || string(got.Body) != `{"n":1}` || !got.Stored.Equal(at) {
		t.Fatalf("round trip mismatch: %+v ok=%v", got, ok)
	}
}

func TestPutOverwritesAndStampsTime(t *testing.T) {
	c := Open(t.TempDir())
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c.now = func() time.Time { return fixed }
	for _, body := range []string{"one", "two"} {
		if err := c.Put("k", Entry{Body: []byte(body)}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := c.Get("k")
	if string(got.Body) != "two" || !got.Stored.Equal(fixed) {
		t.Fatalf("got %+v", got)
	}
}

func TestKeysDoNotCollideAndBinaryBodiesSurvive(t *testing.T) {
	c := Open(t.TempDir())
	bin := []byte{0, 1, 2, 255, '\n', '"'}
	_ = c.Put("a", Entry{Body: bin})
	_ = c.Put("b", Entry{Body: []byte("other")})
	if got, _ := c.Get("a"); !bytes.Equal(got.Body, bin) {
		t.Errorf("binary body mangled: %v", got.Body)
	}
	if got, _ := c.Get("b"); string(got.Body) != "other" {
		t.Errorf("b = %q", got.Body)
	}
}

func TestCorruptEntryIsAMissAndIsRemoved(t *testing.T) {
	c := Open(t.TempDir())
	_ = c.Put("k", Entry{Body: []byte("x")})
	p := c.path("k")
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("k"); ok {
		t.Fatal("corrupt entry returned a hit")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("corrupt file not removed: %v", err)
	}
}

func TestKeyMismatchInFileIsAMiss(t *testing.T) {
	c := Open(t.TempDir())
	_ = c.Put("a", Entry{Body: []byte("x")})
	// Simulate a hash collision: a's file served under b's path.
	data, _ := os.ReadFile(c.path("a"))
	if err := os.MkdirAll(filepath.Dir(c.path("b")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.path("b"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("entry for a different key must not be returned")
	}
}

func TestNilCacheIsInert(t *testing.T) {
	var c *Cache
	if _, ok := c.Get("k"); ok {
		t.Error("nil cache hit")
	}
	if err := c.Put("k", Entry{Body: []byte("x")}); err != nil {
		t.Error(err)
	}
	if n, err := c.Clear(); n != 0 || err != nil {
		t.Errorf("Clear = %d, %v", n, err)
	}
	if s, err := c.Stats(); s.Entries != 0 || err != nil {
		t.Errorf("Stats = %+v, %v", s, err)
	}
	if c.Dir() != "" {
		t.Error("nil Dir not empty")
	}
}

func TestStats(t *testing.T) {
	c := Open(t.TempDir())
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(48 * time.Hour)
	_ = c.Put("github.com/o/r/pulls/1", Entry{Body: []byte("aaaa"), Stored: t1})
	_ = c.Put("github.com/o/r/issues/2", Entry{Body: []byte("bb"), Stored: t0})
	_ = c.Put("github.com/x/y/commits/abc/pulls", Entry{Body: []byte("c"), Stored: t0.Add(time.Hour)})
	s, err := c.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if s.Entries != 3 || s.Bytes <= 0 {
		t.Errorf("entries=%d bytes=%d", s.Entries, s.Bytes)
	}
	if s.Scopes["github.com/o/r"] != 2 || s.Scopes["github.com/x/y"] != 1 {
		t.Errorf("scopes = %v", s.Scopes)
	}
	if !s.Oldest.Equal(t0) || !s.Newest.Equal(t1) {
		t.Errorf("oldest=%v newest=%v", s.Oldest, s.Newest)
	}
}

func TestStatsOnMissingDirIsEmpty(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "never-created")).Stats()
	if err != nil || s.Entries != 0 {
		t.Fatalf("Stats = %+v, %v", s, err)
	}
}

func TestClearRemovesOnlyEntries(t *testing.T) {
	dir := t.TempDir()
	c := Open(dir)
	_ = c.Put("a", Entry{Body: []byte("1")})
	_ = c.Put("b", Entry{Body: []byte("2")})
	keep := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := c.Clear()
	if err != nil || n != 2 {
		t.Fatalf("Clear = %d, %v", n, err)
	}
	if _, ok := c.Get("a"); ok {
		t.Error("entry survived Clear")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("unrelated file removed: %v", err)
	}
	if n, _ := c.Clear(); n != 0 {
		t.Errorf("second Clear removed %d", n)
	}
}

func TestConcurrentWritersNeverExposePartialFiles(t *testing.T) {
	c := Open(t.TempDir())
	var wg sync.WaitGroup
	body := []byte(strings.Repeat("x", 64<<10))
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if err := c.Put("shared", Entry{Body: body}); err != nil {
					t.Error(err)
					return
				}
				if e, ok := c.Get("shared"); ok && len(e.Body) != len(body) {
					t.Errorf("partial read: %d bytes", len(e.Body))
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestDefaultDirPrecedence(t *testing.T) {
	t.Setenv("GIT_WHY_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", filepath.Join("x", "cache"))
	if d, err := DefaultDir(); err != nil || d != filepath.Join("x", "cache", "git-why") {
		t.Errorf("xdg: %q, %v", d, err)
	}
	t.Setenv("GIT_WHY_CACHE", filepath.Join("y", "explicit"))
	if d, _ := DefaultDir(); d != filepath.Join("y", "explicit") {
		t.Errorf("explicit: %q", d)
	}
}
