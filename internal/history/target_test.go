package history_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arinjaysaxena/git-why/internal/history"
	"github.com/arinjaysaxena/git-why/internal/locate"
	"github.com/arinjaysaxena/git-why/internal/target"
	"github.com/arinjaysaxena/git-why/internal/testrepo"
)

func goSource(retry, do string) string {
	return "package svc\n\nfunc Other() int { return 0 }\n\nfunc Retry(n int) int {\n\treturn " + retry + "\n}\n\ntype Client struct{}\n\nfunc (c *Client) Do() string {\n\treturn \"" + do + "\"\n}\n"
}

func mustTarget(t *testing.T, arg, fn string) target.Target {
	t.Helper()
	tg, err := target.Parse(arg, fn)
	if err != nil {
		t.Fatal(err)
	}
	return tg
}

func TestWalkTargetSymbolFollowsOnlyThatFunction(t *testing.T) {
	r := testrepo.New(t)
	r.Write("svc.go", goSource("n", "a"))
	r.Commit("add svc")
	r.Write("svc.go", goSource("n+1", "a"))
	r.Commit("retry plus one")
	r.Write("svc.go", goSource("n+1", "b")) // only Client.Do changes
	r.Commit("change do")

	w := walker(r)
	res, got, err := w.WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "svc.go#Retry", "")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Symbol != "Retry" || got.Start != 5 || got.End != 7 {
		t.Fatalf("resolved = %+v", got)
	}
	if want := []string{"retry plus one", "add svc"}; !equalStrings(subjects(res), want) {
		t.Fatalf("subjects = %v, want %v", subjects(res), want)
	}

	res, got, err = w.WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "svc.go", "Client.Do")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Symbol != "Client.Do" || got.Start != 11 {
		t.Fatalf("resolved = %+v", got)
	}
	if want := []string{"change do", "add svc"}; !equalStrings(subjects(res), want) {
		t.Fatalf("subjects = %v, want %v", subjects(res), want)
	}
}

func TestWalkTargetSymbolAtAnEarlierRevision(t *testing.T) {
	r := testrepo.New(t)
	r.Write("svc.go", goSource("n", "a"))
	first := r.Commit("add svc")
	r.Write("svc.go", "package svc\n\n// shifted\n\n"+goSource("n", "a")[len("package svc\n\n"):])
	r.Commit("shift everything down")

	_, got, err := walker(r).WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "svc.go#Retry", ""), Rev: first})
	if err != nil {
		t.Fatal(err)
	}
	if got.Start != 5 {
		t.Fatalf("at the first revision Retry starts on line 5, got %d", got.Start)
	}
}

func TestWalkTargetLinesAndWholeFile(t *testing.T) {
	r := testrepo.New(t)
	r.Write("svc.go", goSource("n", "a"))
	r.Commit("add svc")
	w := walker(r)

	_, got, err := w.WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "svc.go:6", "")})
	if err != nil || got.Start != 6 || got.End != 6 {
		t.Fatalf("line target = %+v, %v", got, err)
	}
	_, got, err = w.WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "svc.go", "")})
	if err != nil || got.Start != 1 || got.End != 13 {
		t.Fatalf("whole file = %+v, %v", got, err)
	}
	if _, _, err = w.WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "svc.go:10-99", "")}); err == nil {
		t.Fatal("a range past the end of the file must fail")
	}
}

func TestWalkTargetErrors(t *testing.T) {
	r := testrepo.New(t)
	r.Write("svc.go", goSource("n", "a"))
	r.Write("notes.txt", "hello\n")
	r.Commit("add files")
	w := walker(r)

	_, _, err := w.WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "svc.go#Nope", "")})
	if !errors.Is(err, locate.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	_, _, err = w.WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "notes.txt#Thing", "")})
	if !errors.Is(err, locate.ErrUnsupported) {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
	if _, _, err = w.WalkTarget(context.Background(), history.TargetQuery{Target: mustTarget(t, "missing.go#X", "")}); err == nil {
		t.Fatal("missing file must fail")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
