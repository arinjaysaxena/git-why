package locate

import (
	"errors"
	"strings"
	"testing"
)

const goSrc = `package demo

import "fmt"

// Server serves.
type Server struct {
	addr string
}

type (
	ID   int
	Pair struct{ A, B int }
)

func New() *Server {
	return &Server{}
}

func (s *Server) Start() error {
	fmt.Println("start")
	return nil
}

func (s Server) Stop() {}

func (c *Cache[K, V]) Get(k K) V {
	var zero V
	return zero
}

func (p Pair) Stop() {}

func Run() {
}
`

func TestLocateGo(t *testing.T) {
	cases := []struct {
		sym        string
		start, end int
		name, kind string
	}{
		{"New", 15, 17, "New", "func"},
		{"Run", 33, 34, "Run", "func"},
		{"Server.Start", 19, 22, "Server.Start", "method"},
		{"Start", 19, 22, "Server.Start", "method"}, // unique bare method name
		{"Server", 6, 8, "Server", "type"},
		{"ID", 11, 11, "ID", "type"},
		{"Pair", 12, 12, "Pair", "type"},
		{"Cache.Get", 26, 29, "Cache.Get", "method"}, // generic receiver
	}
	for _, c := range cases {
		t.Run(c.sym, func(t *testing.T) {
			got, err := Locate("demo.go", goSrc, c.sym)
			if err != nil {
				t.Fatal(err)
			}
			want := Range{Start: c.start, End: c.end, Name: c.name, Kind: c.kind}
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestLocateGoErrors(t *testing.T) {
	if _, err := Locate("demo.go", goSrc, "Missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	_, err := Locate("demo.go", goSrc, "Stop")
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("want AmbiguousError, got %v", err)
	}
	if strings.Join(amb.Candidates, ",") != "Server.Stop,Pair.Stop" {
		t.Fatalf("candidates = %v", amb.Candidates)
	}
	if _, err := Locate("notes.txt", "x", "X"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
	if _, err := Locate("demo.go", goSrc, ""); err == nil {
		t.Fatal("empty symbol must fail")
	}
}

func TestLocateGoToleratesSyntaxErrorsElsewhere(t *testing.T) {
	src := "package p\n\nfunc Good() {\n}\n\nfunc Bad( {\n"
	got, err := Locate("p.go", src, "Good")
	if err != nil {
		t.Fatal(err)
	}
	if got.Start != 3 || got.End != 4 {
		t.Fatalf("got %+v", got)
	}
}
