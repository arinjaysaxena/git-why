package locate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func load(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type want struct {
	sym        string
	start, end int
	kind       string
}

func check(t *testing.T, file string, cases []want) {
	t.Helper()
	src := load(t, file)
	for _, c := range cases {
		t.Run(file+"/"+c.sym, func(t *testing.T) {
			got, err := Locate(file, src, c.sym)
			if err != nil {
				t.Fatal(err)
			}
			if got.Start != c.start || got.End != c.end || (c.kind != "" && got.Kind != c.kind) {
				t.Fatalf("got %d-%d (%s %s), want %d-%d %s", got.Start, got.End, got.Kind, got.Name, c.start, c.end, c.kind)
			}
		})
	}
}

func TestTypeScript(t *testing.T) {
	check(t, "sample.ts", []want{
		{"Greeter", 4, 24, "class"},
		{"Greeter.constructor", 7, 9, "method"},
		{"Greeter.greet", 11, 17, "method"}, // includes the @memo() decorator
		{"greet", 11, 17, "method"},
		{"Greeter.create", 19, 19, "method"},
		{"Greeter.handler", 21, 23, "method"},
		{"Options", 26, 29, "interface"},
		{"Id", 31, 31, "type"},
		{"parse", 33, 37, "func"}, // overloads and implementation as one span
		{"double", 39, 39, "func"},
		{"triple", 41, 43, "func"},
		{"add", 45, 47, "func"},
		{"gen", 49, 51, "func"},
	})
}

func TestJava(t *testing.T) {
	check(t, "Sample.java", []want{
		{"Service", 6, 34, "class"},
		{"Service.Service", 9, 11, "method"}, // constructor
		{"Service.toString", 13, 16, "method"},
		{"Service.find", 18, 25, "method"}, // both overloads
		{"Service.run", 27, 27, "method"},
		{"Service.Inner", 29, 33, "class"},
		{"Inner.go", 30, 32, "method"},
		{"Shape.area", 37, 37, "method"},
	})
}

func TestRust(t *testing.T) {
	check(t, "sample.rs", []want{
		{"Point", 4, 7, "type"}, // includes the attribute
		{"Unit", 9, 9, "type"},
		{"Point.new", 12, 14, "func"},
		{"Point.describe", 16, 19, "func"},
		{"Point.fmt", 23, 25, "func"},
		{"Shape.area", 29, 29, "func"},
		{"run", 32, 37, "func"},
		{"util.helper", 40, 40, "func"},
	})
}

func TestPython(t *testing.T) {
	check(t, "sample.py", []want{
		{"top", 4, 9, "func"}, // includes the decorator
		{"Repo", 12, 27, "class"},
		{"Repo.__init__", 15, 16, "method"},
		{"size", 18, 21, "method"},
		{"Repo.fetch", 23, 27, "method"},
		{"last", 30, 31, "func"},
	})
}

func TestPythonSkipsNestedFunctions(t *testing.T) {
	src := load(t, "sample.py")
	if _, err := Locate("sample.py", src, "inner"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nested def must not be addressable, got %v", err)
	}
	if _, err := Locate("sample.py", src, "fake"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("text inside a docstring must not match, got %v", err)
	}
}

func TestRustAmbiguousAndMissing(t *testing.T) {
	src := "struct A;\nstruct B;\nimpl A {\n    fn new() -> A { A }\n}\nimpl B {\n    fn new() -> B { B }\n}\n"
	var amb *AmbiguousError
	if _, err := Locate("x.rs", src, "new"); !errors.As(err, &amb) || len(amb.Candidates) != 2 {
		t.Fatalf("want ambiguity, got %v", err)
	}
	if r, err := Locate("x.rs", src, "B.new"); err != nil || r.Start != 7 {
		t.Fatalf("B.new = %+v, %v", r, err)
	}
	if _, err := Locate("x.rs", src, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestMaskKeepsLineNumbers(t *testing.T) {
	src := "a /* x\ny */ b\n\"q\nz\" `t\nu` // c\n"
	got := mask(src, jsStyle)
	if len(got) != len(src) {
		t.Fatalf("length changed: %d vs %d", len(got), len(src))
	}
	for i := range src {
		if (src[i] == '\n') != (got[i] == '\n') {
			t.Fatalf("newline moved at %d", i)
		}
	}
}
