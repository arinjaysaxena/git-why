// Package target parses what the user asked about: a single line, a line
// range, or a named symbol inside a file.
package target

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Target identifies code to explain. Exactly one of the line range or Symbol
// is set, except for a bare file which selects the whole file.
type Target struct {
	// File is a slash-separated path as given by the user.
	File string
	// Start and End are 1-based inclusive line numbers; zero when unset.
	Start, End int
	// Symbol is a function, method or type name such as "Run" or "Server.Start".
	Symbol string
}

// HasRange reports whether the target is an explicit line range.
func (t Target) HasRange() bool { return t.Start > 0 }

// HasSymbol reports whether the target must be resolved through the symbol locator.
func (t Target) HasSymbol() bool { return t.Symbol != "" }

// String renders the canonical form accepted by Parse.
func (t Target) String() string {
	switch {
	case t.HasSymbol():
		return t.File + "#" + t.Symbol
	case t.HasRange() && t.End != t.Start:
		return fmt.Sprintf("%s:%d-%d", t.File, t.Start, t.End)
	case t.HasRange():
		return fmt.Sprintf("%s:%d", t.File, t.Start)
	default:
		return t.File
	}
}

var (
	// ErrEmpty is returned for an empty target string.
	ErrEmpty = errors.New("empty target")

	rangeSuffix = regexp.MustCompile(`^(\d+)(?:-(\d+))?$`)
)

// Parse interprets arg, optionally combined with the --fn flag value fn.
//
// Accepted forms:
//
//	file:12        a single line
//	file:12-20     an inclusive line range
//	file#Symbol    a function, method (Type.Method) or type
//	file --fn Sym  same as file#Sym
func Parse(arg, fn string) (Target, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return Target{}, ErrEmpty
	}
	if fn != "" && !validSymbol(fn) {
		return Target{}, fmt.Errorf("invalid symbol name %q", fn)
	}

	file, spec, sep := split(arg)
	if file == "" {
		return Target{}, fmt.Errorf("target %q has no file path", arg)
	}
	t := Target{File: normalizePath(file)}

	switch sep {
	case '#':
		if !validSymbol(spec) {
			return Target{}, fmt.Errorf("invalid symbol %q in %q", spec, arg)
		}
		if fn != "" {
			return Target{}, errors.New("use either <file>#<Symbol> or --fn, not both")
		}
		t.Symbol = spec
	case ':':
		if fn != "" {
			return Target{}, errors.New("--fn cannot be combined with a line number or range")
		}
		start, end, err := parseRange(spec)
		if err != nil {
			return Target{}, fmt.Errorf("%w in %q", err, arg)
		}
		t.Start, t.End = start, end
	default:
		t.Symbol = fn
	}
	return t, nil
}

// split separates the file from the selector. A trailing ":<digits>[-<digits>]"
// is a line selector (so Windows drive letters and colons in names survive);
// otherwise the last '#' starts a symbol.
func split(arg string) (file, spec string, sep byte) {
	if i := strings.LastIndexByte(arg, ':'); i >= 0 {
		rest := arg[i+1:]
		if rangeSuffix.MatchString(rest) || looksLikeBadRange(rest) {
			return arg[:i], rest, ':'
		}
	}
	if i := strings.LastIndexByte(arg, '#'); i >= 0 {
		return arg[:i], arg[i+1:], '#'
	}
	return arg, "", 0
}

// looksLikeBadRange catches inputs such as "a.go:10-" or "a.go:-3" so they
// produce a range error instead of being treated as part of the file name.
func looksLikeBadRange(s string) bool {
	return s != "" && strings.Trim(s, "0123456789-") == ""
}

func parseRange(spec string) (start, end int, err error) {
	m := rangeSuffix.FindStringSubmatch(spec)
	if m == nil {
		return 0, 0, fmt.Errorf("invalid line range %q (want N or N-M)", spec)
	}
	start, err = strconv.Atoi(m[1])
	if err != nil || start < 1 {
		return 0, 0, fmt.Errorf("line numbers start at 1, got %q", m[1])
	}
	end = start
	if m[2] != "" {
		end, err = strconv.Atoi(m[2])
		if err != nil || end < 1 {
			return 0, 0, fmt.Errorf("line numbers start at 1, got %q", m[2])
		}
	}
	if end < start {
		return 0, 0, fmt.Errorf("range end %d is before start %d", end, start)
	}
	return start, end, nil
}

func normalizePath(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(p, "./") {
		p = path.Clean(p)
	}
	return p
}

// validSymbol accepts dotted identifiers such as Run, Server.Start or
// $scope.apply: letters, digits, '_' and '$', joined by single dots.
func validSymbol(s string) bool {
	if s == "" {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if part == "" {
			return false
		}
		for i, r := range part {
			switch {
			case r == '_' || r == '$' || unicode.IsLetter(r):
			case i > 0 && unicode.IsDigit(r):
			default:
				return false
			}
		}
	}
	return true
}
