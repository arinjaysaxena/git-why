package main

import (
	"strings"
	"testing"

	"github.com/arinjaysaxena/git-why/internal/cache"
)

func TestCacheStatsAndClear(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_WHY_CACHE", dir)
	out, err := execute(t, nil, "cache", "stats")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "entries: 0") || !strings.Contains(out, dir) || strings.Contains(out, "oldest") {
		t.Errorf("empty stats:\n%s", out)
	}

	c := cache.Open(dir)
	_ = c.Put("github.com/o/r/issues/1", cache.Entry{ETag: `"a"`, Body: []byte("x")})
	_ = c.Put("github.com/o/r/pulls/2", cache.Entry{ETag: `"b"`, Body: []byte("y")})
	out, err = execute(t, nil, "cache", "stats")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"entries: 2", "oldest:", "newest:", "github.com/o/r"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats missing %q:\n%s", want, out)
		}
	}

	out, err = execute(t, nil, "cache", "clear")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "removed 2 entries") {
		t.Errorf("clear output: %q", out)
	}
	out, _ = execute(t, nil, "cache", "clear")
	if !strings.Contains(out, "removed 0 entries") {
		t.Errorf("second clear: %q", out)
	}
	_ = c.Put("k", cache.Entry{Body: []byte("z")})
	out, _ = execute(t, nil, "cache", "clear")
	if !strings.Contains(out, "removed 1 entry ") {
		t.Errorf("singular: %q", out)
	}
}

func TestCacheRejectsExtraArgsAndUnknownSubcommands(t *testing.T) {
	t.Setenv("GIT_WHY_CACHE", t.TempDir())
	if _, err := execute(t, nil, "cache", "stats", "extra"); err == nil {
		t.Error("extra argument accepted")
	}
	if _, err := execute(t, nil, "cache", "wipe"); err == nil {
		t.Error("unknown subcommand accepted")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 1536: "1.5 KiB", 5 << 20: "5.0 MiB"}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
