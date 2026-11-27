package history_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/arinjaysaxena/git-why/internal/gitx"
	"github.com/arinjaysaxena/git-why/internal/history"
	"github.com/arinjaysaxena/git-why/internal/testrepo"
)

func walker(r *testrepo.Repo) *history.Walker { return &history.Walker{Git: gitx.New(r.Dir())} }

func subjects(res history.Result) []string {
	var s []string
	for _, c := range res.Commits {
		s = append(s, c.Subject)
	}
	return s
}

func src(v string) string { return "package a\n\nfunc F() int {\n\treturn " + v + "\n}\n" }

func TestWalkFollowsRenameAndStopsAtIntroduction(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.go", src("1"))
	first := r.Commit("add F")
	r.Move("a.go", "pkg/b.go")
	r.Commit("move file")
	r.Write("pkg/b.go", src("2"))
	r.Commit("return two")
	r.Write("pkg/b.go", src("3"))
	r.Commit("return three")

	res, err := walker(r).Walk(context.Background(), history.Query{Path: "pkg/b.go", Start: 4, End: 4})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"return three", "return two", "add F"}; !reflect.DeepEqual(subjects(res), want) {
		t.Fatalf("subjects = %v, want %v", subjects(res), want)
	}
	if res.Stop != history.StopIntroduced {
		t.Errorf("stop = %q", res.Stop)
	}
	last := res.Commits[2]
	if !last.Introduced || last.SHA != first || last.Path != "a.go" {
		t.Errorf("last = %+v", last)
	}
	if res.Commits[0].Introduced {
		t.Error("newest commit must not be marked introduced")
	}
	if got, want := res.Paths(), []string{"pkg/b.go", "a.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v", got, want)
	}
	h := res.Commits[0].Hunks
	if len(h) != 1 || !reflect.DeepEqual(h[0].Removed, []string{"\treturn 2"}) || !reflect.DeepEqual(h[0].Added, []string{"\treturn 3"}) {
		t.Errorf("hunks = %+v", h)
	}
}

func TestWalkRenameWithEditIsFlagged(t *testing.T) {
	r := testrepo.New(t)
	r.Write("old.go", src("1"))
	r.Commit("add")
	r.Move("old.go", "new.go")
	r.Write("new.go", src("2"))
	r.Commit("rename and edit")

	res, err := walker(r).Walk(context.Background(), history.Query{Path: "new.go", Start: 4, End: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Commits) != 2 {
		t.Fatalf("commits = %v", subjects(res))
	}
	if c := res.Commits[0]; !c.Renamed || c.OldPath != "old.go" || c.Path != "new.go" {
		t.Errorf("rename commit = %+v", c.RangeCommit)
	}
	if res.Commits[1].Renamed {
		t.Error("creating commit is not a rename")
	}
}

func TestWalkIgnoresChangesOutsideRange(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.go", src("1"))
	r.Commit("add F")
	r.Write("a.go", src("1")+"\nfunc G() {}\n")
	r.Commit("add G")
	r.Write("a.go", "// header\n"+src("1")+"\nfunc G() {}\n")
	r.Commit("add header") // shifts lines, does not change them

	res, err := walker(r).Walk(context.Background(), history.Query{Path: "a.go", Start: 5, End: 5})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"add F"}; !reflect.DeepEqual(subjects(res), want) {
		t.Errorf("subjects = %v, want %v", subjects(res), want)
	}
}

func TestWalkDepthTruncates(t *testing.T) {
	r := testrepo.New(t)
	for _, v := range []string{"1", "2", "3", "4"} {
		r.Write("a.go", src(v))
		r.Commit("v" + v)
	}
	res, err := walker(r).Walk(context.Background(), history.Query{Path: "a.go", Start: 4, End: 4, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"v4", "v3"}; !reflect.DeepEqual(subjects(res), want) {
		t.Errorf("subjects = %v", subjects(res))
	}
	if res.Stop != history.StopDepth {
		t.Errorf("stop = %q, want depth", res.Stop)
	}

	// Exactly as many commits as the depth is not a truncation.
	res, err = walker(r).Walk(context.Background(), history.Query{Path: "a.go", Start: 4, End: 4, Depth: 4})
	if err != nil || res.Stop != history.StopIntroduced || len(res.Commits) != 4 {
		t.Errorf("depth==len: %v %q %d", err, res.Stop, len(res.Commits))
	}
}

func TestWalkSince(t *testing.T) {
	r := testrepo.New(t)
	for _, v := range []string{"1", "2", "3"} {
		r.Write("a.go", src(v))
		r.Commit("v" + v) // Jan 2, 3, 4 2024
	}
	since := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	res, err := walker(r).Walk(context.Background(), history.Query{Path: "a.go", Start: 4, End: 4, Since: &since})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"v3", "v2"}; !reflect.DeepEqual(subjects(res), want) {
		t.Errorf("subjects = %v", subjects(res))
	}
	if res.Stop != history.StopSince || res.Commits[1].Introduced {
		t.Errorf("stop = %q introduced=%v", res.Stop, res.Commits[1].Introduced)
	}

	// Window that still includes the root commit: it is the origin.
	early := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err = walker(r).Walk(context.Background(), history.Query{Path: "a.go", Start: 4, End: 4, Since: &early})
	if err != nil || res.Stop != history.StopIntroduced || !res.Commits[2].Introduced {
		t.Errorf("early since: %v %q", err, res.Stop)
	}
}

func TestWalkAttributesBranchCommitsToMerge(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.go", src("1"))
	r.Write("other.txt", "x\n")
	r.Commit("add F")
	r.Branch("feature")
	r.Write("a.go", src("2"))
	feature := r.Commit("feature: return two")
	r.Checkout("main")
	r.Write("other.txt", "y\n")
	r.Commit("main work")
	merge := r.Merge("feature", "Merge pull request #7 from o/feature")

	res, err := walker(r).Walk(context.Background(), history.Query{Path: "a.go", Start: 4, End: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Commits) != 2 || res.Commits[0].SHA != feature {
		t.Fatalf("commits = %v", subjects(res))
	}
	if res.Commits[0].Merge != merge {
		t.Errorf("merge = %q, want %q", res.Commits[0].Merge, merge)
	}
	if res.Commits[1].Merge != "" {
		t.Errorf("mainline commit has merge %q", res.Commits[1].Merge)
	}
}

func TestWalkSquashMergeIsMainlineCommit(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.go", src("1"))
	r.Commit("add F")
	r.Branch("feature")
	r.Write("a.go", src("2"))
	r.Commit("wip")
	r.Write("a.go", src("3"))
	r.Commit("wip 2")
	r.Checkout("main")
	squash := r.SquashMerge("feature", "Return three (#9)")

	res, err := walker(r).Walk(context.Background(), history.Query{Path: "a.go", Start: 4, End: 4})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Return three (#9)", "add F"}; !reflect.DeepEqual(subjects(res), want) {
		t.Fatalf("subjects = %v", subjects(res))
	}
	if res.Commits[0].SHA != squash || res.Commits[0].Merge != "" {
		t.Errorf("squash commit = %+v", res.Commits[0])
	}
}

func TestWalkFromOlderRev(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.go", src("1"))
	r.Commit("v1")
	r.Write("a.go", src("2"))
	r.Commit("v2")
	old := r.Head()
	r.Write("a.go", src("3"))
	r.Commit("v3")

	res, err := walker(r).Walk(context.Background(), history.Query{Path: "a.go", Start: 4, End: 4, Rev: old})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"v2", "v1"}; !reflect.DeepEqual(subjects(res), want) {
		t.Errorf("subjects = %v", subjects(res))
	}
}

func TestWalkValidation(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.go", src("1"))
	r.Commit("v1")
	w := walker(r)
	ctx := context.Background()
	for name, q := range map[string]history.Query{
		"no path":        {Start: 1, End: 1},
		"zero start":     {Path: "a.go", Start: 0, End: 1},
		"inverted":       {Path: "a.go", Start: 3, End: 2},
		"missing file":   {Path: "nope.go", Start: 1, End: 1},
		"range past EOF": {Path: "a.go", Start: 50, End: 60},
	} {
		if _, err := w.Walk(ctx, q); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseHunks(t *testing.T) {
	patch := "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -4 +4,2 @@ func F() {\n-\told\n+\tnew\n+\textra\n@@ -10,0 +12,1 @@\n+z"
	h, err := history.ParseHunks(patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 2 {
		t.Fatalf("hunks = %+v", h)
	}
	if h[0].OldStart != 4 || h[0].OldLines != 1 || h[0].NewStart != 4 || h[0].NewLines != 2 {
		t.Errorf("header 0 = %+v", h[0])
	}
	if !reflect.DeepEqual(h[0].Added, []string{"\tnew", "\textra"}) || !reflect.DeepEqual(h[0].Removed, []string{"\told"}) {
		t.Errorf("body 0 = %+v", h[0])
	}
	if h[1].OldLines != 0 || h[1].NewStart != 12 {
		t.Errorf("header 1 = %+v", h[1])
	}
	// File headers must not be mistaken for removed/added lines.
	if len(h[0].Removed) != 1 {
		t.Errorf("file headers leaked into hunk: %+v", h[0].Removed)
	}
	for _, bad := range []string{"@@ nonsense @@", "@@ -x +1 @@", "@@ -1,y +1 @@"} {
		if _, err := history.ParseHunks(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
	if h, err := history.ParseHunks(""); err != nil || len(h) != 0 {
		t.Errorf("empty patch: %v %v", h, err)
	}
}

func TestEmptyResultWhenRangeNeverChanged(t *testing.T) {
	// A file that is untracked has no history; git rejects it, which we surface.
	r := testrepo.New(t)
	r.Write("a.go", src("1"))
	r.Commit("v1")
	r.Write("untracked.go", src("1"))
	if _, err := walker(r).Walk(context.Background(), history.Query{Path: "untracked.go", Start: 1, End: 1}); err == nil {
		t.Error("expected error for untracked file")
	}
}
