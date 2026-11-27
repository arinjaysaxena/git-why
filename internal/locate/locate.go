// Package locate turns a symbol name into the line range that declares it.
// Go sources are parsed with go/parser; other languages go through a small
// declaration scanner (see scan.go).
package locate

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Range is a 1-based inclusive line range.
type Range struct {
	Start, End int
	// Name is the fully qualified symbol that matched, e.g. "Server.Start".
	Name string
	// Kind is "func", "method", "type", "class", ... as the language names it.
	Kind string
}

var (
	// ErrNotFound means the file has no declaration with that name.
	ErrNotFound = errors.New("symbol not found")
	// ErrUnsupported means there is no locator for the file's language.
	ErrUnsupported = errors.New("symbol lookup is not supported for this file type")
)

// AmbiguousError lists the qualified names a bare symbol could refer to.
type AmbiguousError struct {
	Symbol     string
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("symbol %q is ambiguous; use one of: %s", e.Symbol, strings.Join(e.Candidates, ", "))
}

// Locate finds symbol in src, the contents of the file at filePath. The
// language is chosen from the file extension.
func Locate(filePath, src, symbol string) (Range, error) {
	if symbol == "" {
		return Range{}, errors.New("locate: empty symbol")
	}
	switch strings.ToLower(path.Ext(filePath)) {
	case ".go":
		return locateGo(filePath, src, symbol)
	default:
		return Range{}, fmt.Errorf("%w: %s", ErrUnsupported, path.Ext(filePath))
	}
}

// pick resolves a symbol against declared candidates. An exact qualified match
// wins; a bare name may match a single "Type.name" member.
func pick(symbol string, decls []Range) (Range, error) {
	for _, d := range decls {
		if d.Name == symbol {
			return d, nil
		}
	}
	var members []Range
	for _, d := range decls {
		if i := strings.LastIndex(d.Name, "."); i >= 0 && d.Name[i+1:] == symbol {
			members = append(members, d)
		}
	}
	switch len(members) {
	case 0:
		return Range{}, fmt.Errorf("%w: %q", ErrNotFound, symbol)
	case 1:
		return members[0], nil
	}
	e := &AmbiguousError{Symbol: symbol}
	for _, m := range members {
		e.Candidates = append(e.Candidates, m.Name)
	}
	return Range{}, e
}
