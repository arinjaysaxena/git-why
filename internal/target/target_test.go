package target

import (
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		fn   string
		want Target
	}{
		{"single line", "internal/retry/backoff.go:42", "", Target{File: "internal/retry/backoff.go", Start: 42, End: 42}},
		{"range", "main.go:10-20", "", Target{File: "main.go", Start: 10, End: 20}},
		{"one-line range", "main.go:7-7", "", Target{File: "main.go", Start: 7, End: 7}},
		{"symbol hash", "pkg/server.go#Server.Start", "", Target{File: "pkg/server.go", Symbol: "Server.Start"}},
		{"fn flag", "pkg/server.go", "Start", Target{File: "pkg/server.go", Symbol: "Start"}},
		{"bare file", "README.md", "", Target{File: "README.md"}},
		{"leading dot slash", "./src/a.ts:3", "", Target{File: "src/a.ts", Start: 3, End: 3}},
		{"windows path", `src\win\a.go:5-6`, "", Target{File: "src/win/a.go", Start: 5, End: 6}},
		{"drive letter", `C:\repo\a.go:9`, "", Target{File: "C:/repo/a.go", Start: 9, End: 9}},
		{"drive letter no line", `C:\repo\a.go`, "", Target{File: "C:/repo/a.go"}},
		{"unicode and dollar symbols", "a.js#$apply", "", Target{File: "a.js", Symbol: "$apply"}},
		{"surrounding space", "  a.go:1  ", "", Target{File: "a.go", Start: 1, End: 1}},
		{"hash in dir name", "dir#1/a.go:4", "", Target{File: "dir#1/a.go", Start: 4, End: 4}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.arg, tc.fn)
			if err != nil {
				t.Fatalf("Parse(%q, %q): %v", tc.arg, tc.fn, err)
			}
			if got != tc.want {
				t.Errorf("Parse(%q, %q) = %+v, want %+v", tc.arg, tc.fn, got, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		fn   string
		want string
	}{
		{"empty", "", "", "empty target"},
		{"blank", "   ", "", "empty target"},
		{"line zero", "a.go:0", "", "start at 1"},
		{"reversed range", "a.go:20-10", "", "before start"},
		{"open range", "a.go:10-", "", "invalid line range"},
		{"negative start", "a.go:-3", "", "invalid line range"},
		{"no file", ":12", "", "no file path"},
		{"empty symbol", "a.go#", "", "invalid symbol"},
		{"bad symbol", "a.go#foo bar", "", "invalid symbol"},
		{"double dot symbol", "a.go#A..B", "", "invalid symbol"},
		{"digit-leading symbol", "a.go#1abc", "", "invalid symbol"},
		{"hash and fn", "a.go#Foo", "Bar", "not both"},
		{"line and fn", "a.go:5", "Bar", "cannot be combined"},
		{"bad fn", "a.go", "9lives", "invalid symbol name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.arg, tc.fn)
			if err == nil {
				t.Fatalf("Parse(%q, %q) succeeded, want error containing %q", tc.arg, tc.fn, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
	if _, err := Parse("", ""); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty input error = %v, want ErrEmpty", err)
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, in := range []string{"a/b.go:12", "a/b.go:12-30", "a/b.go#T.M", "a/b.go"} {
		got, err := Parse(in, "")
		if err != nil {
			t.Fatal(err)
		}
		if got.String() != in {
			t.Errorf("String() = %q, want %q", got.String(), in)
		}
		again, err := Parse(got.String(), "")
		if err != nil || again != got {
			t.Errorf("round trip of %q: %+v, %v", in, again, err)
		}
	}
}

func TestPredicates(t *testing.T) {
	r := Target{File: "a", Start: 1, End: 2}
	s := Target{File: "a", Symbol: "X"}
	if !r.HasRange() || r.HasSymbol() || s.HasRange() || !s.HasSymbol() {
		t.Error("predicates disagree with fields")
	}
}
