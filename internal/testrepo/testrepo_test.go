package testrepo_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/arinjaysaxena/git-why/internal/testrepo"
)

func TestLinearHistoryIsDeterministic(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "one\n")
	first := r.Commit("first")
	r.Write("a.txt", "one\ntwo\n")
	second := r.CommitAs("Maya", "maya@example.com", "second")

	// Compare Unix timestamps: git versions differ in how %aI spells UTC
	// ("+00:00" vs "Z"), but the instant must be identical everywhere.
	day := func(d int) int64 { return time.Date(2024, 1, d, 12, 0, 0, 0, time.UTC).Unix() }
	want := strings.Join([]string{
		fmt.Sprintf("Test Author|first|%d", day(2)),
		fmt.Sprintf("Maya|second|%d", day(3)),
	}, "\n")
	if got := r.Git("log", "--format=%an|%s|%at", "--reverse"); got != want {
		t.Errorf("unexpected log:\n%s\nwant:\n%s", got, want)
	}
	if p := r.Parents(second); len(p) != 1 || p[0] != first {
		t.Errorf("parents of second = %v, want [%s]", p, first)
	}
	if p := r.Parents(first); p != nil {
		t.Errorf("root commit has parents %v", p)
	}
}

func TestMoveIsTrackedAsRename(t *testing.T) {
	r := testrepo.New(t)
	r.Write("old/name.go", "package old\n\nfunc F() int { return 1 }\n")
	r.Commit("add")
	r.Move("old/name.go", "pkg/renamed.go")
	r.Commit("move")

	got := r.Git("log", "--follow", "--format=%s", "--", "pkg/renamed.go")
	if got != "move\nadd" {
		t.Errorf("--follow history = %q", got)
	}
	if r.Read("pkg/renamed.go") == "" {
		t.Error("moved file is empty")
	}
}

func TestMergeCommitHasTwoParents(t *testing.T) {
	r := testrepo.New(t)
	r.Write("f.txt", "base\n")
	r.Commit("base")
	r.Branch("feature")
	r.Write("g.txt", "feature\n")
	feature := r.Commit("feature work")
	r.Checkout("main")
	r.Write("h.txt", "main\n")
	r.Commit("main work")

	merge := r.Merge("feature", "Merge pull request #1")
	parents := r.Parents(merge)
	if len(parents) != 2 || parents[1] != feature {
		t.Errorf("merge parents = %v, want second parent %s", parents, feature)
	}
}

func TestSquashMergeIsSingleParentCommitWithContent(t *testing.T) {
	r := testrepo.New(t)
	r.Write("f.txt", "base\n")
	r.Commit("base")
	r.Branch("feature")
	r.Write("g.txt", "one\n")
	r.Commit("wip 1")
	r.Write("g.txt", "one\ntwo\n")
	r.Commit("wip 2")
	r.Checkout("main")

	squash := r.SquashMerge("feature", "Add g (#2)")
	if n := len(r.Parents(squash)); n != 1 {
		t.Errorf("squash commit has %d parents, want 1", n)
	}
	if got := r.Read("g.txt"); got != "one\ntwo\n" {
		t.Errorf("g.txt = %q", got)
	}
	if subjects := r.Git("log", "--format=%s", "--", "g.txt"); subjects != "Add g (#2)" {
		t.Errorf("history of g.txt on main = %q", subjects)
	}
}

func TestRemoveAndRemote(t *testing.T) {
	r := testrepo.New(t)
	r.Write("x", "1").Commit("add x")
	r.Remove("x").Commit("drop x")
	r.AddRemote("origin", "git@github.com:acme/widget.git")
	if got := r.Git("remote", "get-url", "origin"); got != "git@github.com:acme/widget.git" {
		t.Errorf("remote = %q", got)
	}
	if got := r.Git("ls-files"); got != "" {
		t.Errorf("tracked files after remove: %q", got)
	}
}
