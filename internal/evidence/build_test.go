package evidence

import (
	"reflect"
	"testing"
	"time"

	"github.com/arinjaysaxena/git-why/internal/forge"
	"github.com/arinjaysaxena/git-why/internal/gitx"
	"github.com/arinjaysaxena/git-why/internal/history"
)

func commit(sha, subject, body string, when time.Time, hunks ...history.Hunk) history.Commit {
	return history.Commit{
		RangeCommit: gitx.RangeCommit{SHA: sha, Subject: subject, Body: body, Author: "dev-" + sha[:1], Time: when},
		Hunks:       hunks,
	}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }

// retryScenario mirrors the example in docs/DESIGN.md.
func retryScenario() Input {
	const path = "internal/retry/backoff.go"
	capCommit := commit("a1b2c3d4", "Cap retries to avoid thundering herd (#812)", "Fixes #799", day(2025, 3, 11),
		history.Hunk{NewStart: 42, NewLines: 1, Added: []string{"maxAttempts := 3"}, Removed: []string{"maxAttempts := -1"}})
	backoff := commit("9f8e7d6c", "Add exponential backoff", "See #640 and acme/other#5", day(2024, 11, 2),
		history.Hunk{NewStart: 40, NewLines: 4, Added: []string{"for {"}})
	backoff.Introduced = true
	return Input{
		Owner: "acme", Repo: "app",
		Commits: []history.Commit{capCommit, backoff},
		Paths:   []string{path},
		Range:   forge.Range{Start: 42, End: 42},
		PRs: map[string][]forge.PullRequest{
			"a1b2c3d4": {{Number: 812, Title: "Cap retries", Author: "maya", Merged: true, MergedAt: day(2025, 3, 11)}},
			"9f8e7d6c": {{Number: 640, Title: "Add exponential backoff", Author: "sam", Merged: true}},
		},
		Details: map[int]forge.PullRequestDetails{
			812: {
				PullRequest:   forge.PullRequest{Number: 812, Body: "Retry storms, see #799. Follow-up in #900.", URL: "https://x/812"},
				ClosingIssues: []int{799},
				Threads: []forge.ReviewThread{
					{Path: path, Line: 42, OriginalLine: 42, Comments: []forge.Comment{
						{Author: "dev-lee", Body: "3 is arbitrary but matches the LB's 10s window", URL: "https://x/812#r1", CreatedAt: day(2025, 3, 10)},
						{Author: "maya", Body: "agreed, see #799", URL: "https://x/812#r2"},
					}},
					{Path: "other.go", Line: 42, Comments: []forge.Comment{{Author: "x", Body: "unrelated", URL: "https://x/812#r3"}}},
					{Path: path, Line: 90, OriginalLine: 90, Comments: []forge.Comment{{Author: "x", Body: "elsewhere", URL: "https://x/812#r4"}}},
				},
			},
		},
		Issues:      map[int]forge.Issue{799: {Number: 799, Title: "Retry storms during outage", State: "closed", Labels: []string{"incident"}}},
		Maintainers: map[string]bool{"maya": true},
	}
}

func ids(nodes []*Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}

func TestBuildRetryScenario(t *testing.T) {
	g, err := Build(retryScenario())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := g.Counts(), map[Kind]int{KindCommit: 2, KindPullRequest: 2, KindIssue: 1, KindReviewComment: 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("counts = %v, want %v", got, want)
	}

	if got := ids(g.OfKind(KindCommit)); !reflect.DeepEqual(got, []string{"C1", "C2"}) {
		t.Errorf("commit ids = %v", got)
	}
	c1 := g.Node("C1")
	if c1.SHA != "a1b2c3d4" || c1.LineOverlap != 1 || len(c1.Added) != 1 || c1.Added[0] != "maxAttempts := 3" || c1.Removed[0] != "maxAttempts := -1" {
		t.Errorf("C1 = %+v", c1)
	}
	if c2 := g.Node("C2"); !c2.Introduced || c2.LineOverlap != 1 {
		t.Errorf("C2 = %+v", c2)
	}

	pr := g.ByNumber(KindPullRequest, 812)
	if pr == nil || pr.ID != "PR1" || pr.URL != "https://x/812" || !pr.Maintainer || pr.Title != "Cap retries" {
		t.Fatalf("PR 812 = %+v (details must fill the URL without replacing the title)", pr)
	}
	if got := ids(g.Out("C1", EdgeIntroducedBy)); !reflect.DeepEqual(got, []string{"PR1"}) {
		t.Errorf("C1 introduced-by = %v", got)
	}

	// Only the two comments on line 42 of the target file are evidence.
	comments := g.In("PR1", EdgeDiscussedIn)
	if got := ids(comments); !reflect.DeepEqual(got, []string{"R1", "R2"}) {
		t.Fatalf("discussion = %v", got)
	}
	if r := comments[0]; r.Author != "dev-lee" || r.Line != 42 || r.Path != "internal/retry/backoff.go" {
		t.Errorf("R1 = %+v", r)
	}

	// Issue 799 is closed by the PR and by the commit body; closing subsumes mentioning, so only the
	// review comment remains a plain mention.
	issue := g.ByNumber(KindIssue, 799)
	if issue == nil || issue.State != "closed" || issue.Labels[0] != "incident" {
		t.Fatalf("issue = %+v", issue)
	}
	if got := ids(g.In(issue.ID, EdgeCloses)); !reflect.DeepEqual(got, []string{"PR1", "C1"}) {
		t.Errorf("closers of I1 = %v", got)
	}
	if got := ids(g.In(issue.ID, EdgeMentions)); !reflect.DeepEqual(got, []string{"R2"}) {
		t.Errorf("mentions of I1 = %v", got)
	}
	if got := ids(g.Out("C2", EdgeMentions)); !reflect.DeepEqual(got, []string{"PR2"}) {
		t.Errorf("C2 mentions = %v (a #640 reference must resolve to the PR)", got)
	}

	want := []Ref{{Number: 900, Raw: "#900"}, {Number: 5, Owner: "acme", Repo: "other", Raw: "acme/other#5"}}
	if !reflect.DeepEqual(g.Unresolved, want) {
		t.Errorf("unresolved = %+v, want %+v", g.Unresolved, want)
	}
}

func TestIssueCandidatesListsOnlyMissingLocalIssues(t *testing.T) {
	in := retryScenario()
	if got := IssueCandidates(in); !reflect.DeepEqual(got, []int{900}) {
		t.Errorf("candidates = %v, want [900]", got)
	}
	in.Issues[900] = forge.Issue{Number: 900, Title: "follow-up"}
	if got := IssueCandidates(in); len(got) != 0 {
		t.Errorf("after fetching: %v", got)
	}
	g, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if g.ByNumber(KindIssue, 900) == nil {
		t.Error("fetched issue 900 should now be a node (mentioned by PR 812's body)")
	}
}

func TestBuildWithoutForgeKeepsCommitsAndReportsReferences(t *testing.T) {
	in := Input{
		Owner: "acme", Repo: "app", Range: forge.Range{Start: 1, End: 10},
		Commits: []history.Commit{commit("deadbeef", "Work around Safari bug", "Closes #77", day(2024, 5, 1),
			history.Hunk{NewStart: 3, NewLines: 2, Added: []string{"a", "b"}})},
	}
	g, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].Kind != KindCommit || len(g.Edges) != 0 {
		t.Fatalf("graph = %+v", g)
	}
	if g.Nodes[0].LineOverlap != 0.2 {
		t.Errorf("overlap = %v, want 0.2", g.Nodes[0].LineOverlap)
	}
	if len(g.Unresolved) != 1 || g.Unresolved[0].Number != 77 {
		t.Errorf("unresolved = %+v", g.Unresolved)
	}
}

func TestSharedPRAcrossCommitsIsOneNodeWithDeduplicatedComments(t *testing.T) {
	in := Input{
		Owner: "o", Repo: "r", Paths: []string{"f.go"}, Range: forge.Range{Start: 5, End: 6},
		Commits: []history.Commit{
			commit("bbbb1111", "second", "", day(2025, 1, 2), history.Hunk{NewStart: 5, NewLines: 2}),
			commit("aaaa2222", "first", "", day(2025, 1, 1), history.Hunk{NewStart: 5, NewLines: 1}),
		},
		PRs: map[string][]forge.PullRequest{
			"bbbb1111": {{Number: 3, Title: "PR"}},
			"aaaa2222": {{Number: 3, Title: "PR"}},
		},
		Details: map[int]forge.PullRequestDetails{3: {Threads: []forge.ReviewThread{
			{Path: "f.go", Line: 5, OriginalLine: 5, Comments: []forge.Comment{{Author: "a", Body: "once", URL: "u1"}}},
		}}},
	}
	g, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if c := g.Counts(); c[KindPullRequest] != 1 || c[KindReviewComment] != 1 {
		t.Errorf("counts = %v", c)
	}
	if got := ids(g.In("PR1", EdgeIntroducedBy)); !reflect.DeepEqual(got, []string{"C1", "C2"}) {
		t.Errorf("PR1 commits = %v", got)
	}
}

func TestThreadsWrittenBeforeARenameStillMatch(t *testing.T) {
	in := Input{
		Owner: "o", Repo: "r", Paths: []string{"new/name.go", "old/name.go"}, Range: forge.Range{Start: 10, End: 10},
		Commits: []history.Commit{commit("cccc3333", "Do thing", "", day(2025, 2, 1), history.Hunk{NewStart: 10, NewLines: 1})},
		PRs:     map[string][]forge.PullRequest{"cccc3333": {{Number: 1}}},
		Details: map[int]forge.PullRequestDetails{1: {Threads: []forge.ReviewThread{
			{Path: "old/name.go", Line: 0, OriginalLine: 10, Outdated: true, Comments: []forge.Comment{{Body: "legacy path", URL: "u"}}},
		}}},
	}
	g, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	rc := g.OfKind(KindReviewComment)
	if len(rc) != 1 || rc[0].Line != 10 || !rc[0].Outdated {
		t.Fatalf("review comments = %+v", rc)
	}
}

func TestLinkRejectsUnknownNodes(t *testing.T) {
	g := NewGraph()
	a := g.Add(Node{Kind: KindCommit})
	if err := g.Link(a.ID, "PR9", EdgeMentions); err == nil {
		t.Error("link to unknown node accepted")
	}
	b := g.Add(Node{Kind: KindIssue})
	for i := 0; i < 2; i++ {
		if err := g.Link(a.ID, b.ID, EdgeMentions); err != nil {
			t.Fatal(err)
		}
	}
	if len(g.Edges) != 1 {
		t.Errorf("duplicate edge stored: %v", g.Edges)
	}
}

func TestSelfReferenceAndPRKindMismatchAreIgnored(t *testing.T) {
	in := Input{
		Owner: "o", Repo: "r", Range: forge.Range{Start: 1, End: 1},
		Commits: []history.Commit{commit("dddd4444", "x", "refs https://github.com/o/r/pull/50 and #51", day(2025, 1, 1))},
		PRs:     map[string][]forge.PullRequest{"dddd4444": {{Number: 51, Body: "this is #51"}}},
	}
	g, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range g.Edges {
		if e.From == e.To {
			t.Errorf("self edge %+v", e)
		}
	}
	if len(g.Unresolved) != 1 || g.Unresolved[0].Number != 50 {
		t.Errorf("unresolved = %+v (PR #50 is not in the evidence)", g.Unresolved)
	}
}
