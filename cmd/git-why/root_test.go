package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func execute(t *testing.T, analyze analyzeFunc, args ...string) (stdout string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := newRootCmd(&out, &errOut, analyze)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), err
}

func TestFlagsReachAnalyze(t *testing.T) {
	var got *options
	capture := func(_ io.Writer, o *options) error { got = o; return nil }
	_, err := execute(t, capture, "pkg/a.go:12-20", "--raw", "--depth", "3", "--since", "2024-05-01", "--no-network", "--explain")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("analyze was not called")
	}
	if !got.Raw || !got.NoNetwork || !got.Explain || got.JSON {
		t.Errorf("unexpected flags: %+v", got)
	}
	if got.Depth != 3 {
		t.Errorf("depth = %d, want 3", got.Depth)
	}
	if want := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC); !got.SinceTime.Equal(want) {
		t.Errorf("since = %v, want %v", got.SinceTime, want)
	}
	if len(got.Targets) != 1 || got.Targets[0] != "pkg/a.go:12-20" {
		t.Errorf("targets = %v", got.Targets)
	}
}

func TestDefaults(t *testing.T) {
	var got *options
	capture := func(_ io.Writer, o *options) error { got = o; return nil }
	if _, err := execute(t, capture, "main.go", "--fn", "Run"); err != nil {
		t.Fatal(err)
	}
	if got.Depth != defaultDepth || got.Fn != "Run" || !got.SinceTime.IsZero() {
		t.Errorf("unexpected defaults: %+v", got)
	}
}

func TestUsageErrors(t *testing.T) {
	never := func(io.Writer, *options) error { t.Error("analyze must not run"); return errors.New("unexpected call") }
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no target", nil, "missing target"},
		{"two targets", []string{"a.go:1", "b.go:2"}, "expected one target"},
		{"raw and json", []string{"a.go:1", "--raw", "--json"}, "cannot be combined"},
		{"zero depth", []string{"a.go:1", "--depth", "0"}, "--depth must be at least 1"},
		{"bad since", []string{"a.go:1", "--since", "last tuesday"}, "--since"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := execute(t, never, tc.args...)
			var ue *usageError
			if !errors.As(err, &ue) {
				t.Fatalf("want usageError, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestAnalyzeErrorIsNotUsageError(t *testing.T) {
	boom := errors.New("boom")
	_, err := execute(t, func(io.Writer, *options) error { return boom }, "a.go:1")
	var ue *usageError
	if errors.As(err, &ue) || !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestVersionFlag(t *testing.T) {
	out, err := execute(t, analyze, "--version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "git-why ") {
		t.Errorf("version output = %q", out)
	}
}
