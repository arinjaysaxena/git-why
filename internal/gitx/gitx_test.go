package gitx_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arinjaysaxena/git-why/internal/gitx"
	"github.com/arinjaysaxena/git-why/internal/testrepo"
)

func TestParseRemote(t *testing.T) {
	tests := []struct {
		in      string
		want    gitx.Remote
		wantErr bool
	}{
		{in: "https://github.com/arinjaysaxena/git-why.git", want: gitx.Remote{Host: "github.com", Owner: "arinjaysaxena", Repo: "git-why"}},
		{in: "https://github.com/o/r", want: gitx.Remote{Host: "github.com", Owner: "o", Repo: "r"}},
		{in: "https://user:tok@github.com/o/r.git/", want: gitx.Remote{Host: "github.com", Owner: "o", Repo: "r"}},
		{in: "git@github.com:o/r.git", want: gitx.Remote{Host: "github.com", Owner: "o", Repo: "r"}},
		{in: "ssh://git@ghe.example.com:2222/team/svc.git", want: gitx.Remote{Host: "ghe.example.com", Owner: "team", Repo: "svc"}},
		{in: "git://GitHub.com/o/r.git", want: gitx.Remote{Host: "github.com", Owner: "o", Repo: "r"}},
		{in: "git@gitlab.com:group/sub/proj.git", want: gitx.Remote{Host: "gitlab.com", Owner: "group/sub", Repo: "proj"}},
		{in: "", wantErr: true},
		{in: "/local/path/repo", wantErr: true},
		{in: "https://github.com/onlyowner", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := gitx.ParseRemote(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	if s := (gitx.Remote{Owner: "o", Repo: "r"}).Slug(); s != "o/r" {
		t.Errorf("Slug = %q", s)
	}
}

func TestBlameFollowsMoves(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.go", "package a\n\nfunc F() int {\n\treturn 1\n}\n")
	first := r.CommitAs("Maya", "maya@example.com", "add F")
	r.Move("a.go", "pkg/b.go")
	r.Commit("move")
	r.Write("pkg/b.go", "package a\n\nfunc F() int {\n\treturn 2\n}\n")
	second := r.CommitAs("Sam", "sam@example.com", "change F")

	c := gitx.New(r.Dir())
	lines, err := c.Blame(context.Background(), "", "pkg/b.go", 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines", len(lines))
	}
	if lines[0].SHA != first || lines[0].Author != "Maya" || lines[0].Path != "a.go" || lines[0].Summary != "add F" {
		t.Errorf("line 3 = %+v", lines[0])
	}
	if lines[1].SHA != second || lines[1].Email != "sam@example.com" || lines[1].Content != "\treturn 2" || lines[1].Line != 4 {
		t.Errorf("line 4 = %+v", lines[1])
	}
	if !lines[0].Boundary {
		t.Errorf("root commit line should be a boundary: %+v", lines[0])
	}
	if lines[0].Time.IsZero() {
		t.Error("blame time not parsed")
	}
}

func TestBlameRejectsBadRange(t *testing.T) {
	c := gitx.New(t.TempDir())
	if _, err := c.Blame(context.Background(), "", "x", 5, 2); err == nil {
		t.Error("expected error for inverted range")
	}
}

func TestParseBlameErrors(t *testing.T) {
	if _, err := gitx.ParseBlame("garbage line\n"); err == nil {
		t.Error("expected header error")
	}
	hdr := strings.Repeat("a", 40) + " 1 1 1\nauthor x\n"
	if _, err := gitx.ParseBlame(hdr); err == nil {
		t.Error("expected truncation error")
	}
}

func TestLogRangeFollowsRenameAndRange(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.go", "package a\n\nfunc F() int {\n\treturn 1\n}\n")
	first := r.Commit("add F")
	r.Move("a.go", "pkg/b.go")
	r.Commit("move file")
	r.Write("pkg/b.go", "package a\n\n// F returns two.\nfunc F() int {\n\treturn 2\n}\n")
	second := r.CommitAs("Sam", "sam@example.com", "return two\n\nBecause of #12.")
	r.Write("pkg/b.go", "package a\n\n// F returns two.\nfunc F() int {\n\treturn 2\n}\n\nfunc G() {}\n")
	r.Commit("unrelated G") // outside the tracked range

	c := gitx.New(r.Dir())
	commits, err := c.LogRange(context.Background(), "pkg/b.go", 5, 5, gitx.LogRangeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2: %+v", len(commits), commits)
	}
	if commits[0].SHA != second || commits[1].SHA != first {
		t.Errorf("order = %s, %s", commits[0].SHA, commits[1].SHA)
	}
	if commits[0].Subject != "return two" || commits[0].Body != "Because of #12." || commits[0].Author != "Sam" {
		t.Errorf("metadata = %+v", commits[0])
	}
	if commits[0].Path != "pkg/b.go" || commits[0].OldPath != "pkg/b.go" {
		t.Errorf("paths = %q <- %q", commits[0].Path, commits[0].OldPath)
	}
	if commits[1].Path != "a.go" {
		t.Errorf("introducing commit path = %q, want a.go", commits[1].Path)
	}
	if !strings.Contains(commits[0].Patch, "-\treturn 1") || !strings.Contains(commits[0].Patch, "+\treturn 2") {
		t.Errorf("patch missing change:\n%s", commits[0].Patch)
	}
	if len(commits[1].Parents) != 0 || len(commits[0].Parents) != 1 {
		t.Errorf("parents = %v / %v", commits[0].Parents, commits[1].Parents)
	}
}

func TestLogRangeMaxAndSince(t *testing.T) {
	r := testrepo.New(t)
	r.Write("f.txt", "v1\n")
	r.Commit("one")
	r.Write("f.txt", "v2\n")
	r.Commit("two")
	r.Write("f.txt", "v3\n")
	r.Commit("three")

	c := gitx.New(r.Dir())
	got, err := c.LogRange(context.Background(), "f.txt", 1, 1, gitx.LogRangeOptions{Max: 2})
	if err != nil || len(got) != 2 || got[0].Subject != "three" {
		t.Fatalf("max: %v %+v", err, got)
	}
	since := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC) // commit 2 is Jan 3 12:00
	got, err = c.LogRange(context.Background(), "f.txt", 1, 1, gitx.LogRangeOptions{Since: &since})
	if err != nil || len(got) != 2 || got[1].Subject != "two" {
		t.Fatalf("since: %v %+v", err, got)
	}
	got, err = c.LogRange(context.Background(), "f.txt", 1, 1, gitx.LogRangeOptions{Rev: r.Parents(r.Head())[0]})
	if err != nil || len(got) != 2 || got[0].Subject != "two" {
		t.Fatalf("rev: %v %+v", err, got)
	}
}

func TestParseLogRangeMalformed(t *testing.T) {
	if _, err := gitx.ParseLogRange("\x1eabc\x1fdef"); err == nil {
		t.Error("expected malformed record error")
	}
	if _, err := gitx.ParseLogRange("\x1eh\x1fp\x1fa\x1fe\x1fnotanumber\x1fs\x1fb\x1f"); err == nil {
		t.Error("expected bad timestamp error")
	}
	if got, err := gitx.ParseLogRange(""); err != nil || got != nil {
		t.Errorf("empty = %v, %v", got, err)
	}
}

func TestClientBasics(t *testing.T) {
	r := testrepo.New(t)
	r.Write("x.txt", "hello\n")
	sha := r.Commit("init")
	r.AddRemote("origin", "git@github.com:o/r.git")
	c := gitx.New(r.Dir())
	ctx := context.Background()

	if got, err := c.RevParse(ctx, "HEAD"); err != nil || got != sha {
		t.Errorf("RevParse = %q, %v", got, err)
	}
	if _, err := c.RevParse(ctx, "nope"); err == nil {
		t.Error("expected error for unknown rev")
	}
	if got, err := c.ShowFile(ctx, "HEAD", "x.txt"); err != nil || got != "hello\n" {
		t.Errorf("ShowFile = %q, %v", got, err)
	}
	u, err := c.RemoteURL(ctx, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if rem, err := gitx.ParseRemote(u); err != nil || rem.Slug() != "o/r" {
		t.Errorf("remote = %+v, %v", rem, err)
	}
	var ge *gitx.Error
	if _, err := c.Run(ctx, "bogus-subcommand"); !errors.As(err, &ge) {
		t.Errorf("want *gitx.Error, got %v", err)
	}
	if top, err := c.TopLevel(ctx); err != nil || top == "" {
		t.Errorf("TopLevel = %q, %v", top, err)
	}
}

func TestNotARepo(t *testing.T) {
	c := gitx.New(t.TempDir())
	if _, err := c.TopLevel(context.Background()); !errors.Is(err, gitx.ErrNotARepo) {
		t.Errorf("err = %v, want ErrNotARepo", err)
	}
}
