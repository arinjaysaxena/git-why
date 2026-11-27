package history

import (
	"fmt"
	"strconv"
	"strings"
)

// Hunk is one diff hunk restricted to the tracked line range.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Added, Removed     []string // line contents without the +/- marker
}

// ParseHunks extracts hunks from a unified diff as produced by git log -L.
func ParseHunks(patch string) ([]Hunk, error) {
	var hunks []Hunk
	var cur *Hunk
	for _, ln := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(ln, "@@"):
			h, err := parseHunkHeader(ln)
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, h)
			cur = &hunks[len(hunks)-1]
		case cur == nil:
			// file headers before the first hunk
		case strings.HasPrefix(ln, "+"):
			cur.Added = append(cur.Added, ln[1:])
		case strings.HasPrefix(ln, "-"):
			cur.Removed = append(cur.Removed, ln[1:])
		}
	}
	return hunks, nil
}

// parseHunkHeader parses "@@ -4,1 +4,2 @@ optional context". A missing length
// means 1, as in `git diff`.
func parseHunkHeader(ln string) (Hunk, error) {
	f := strings.Fields(ln)
	if len(f) < 3 || f[0] != "@@" || !strings.HasPrefix(f[1], "-") || !strings.HasPrefix(f[2], "+") {
		return Hunk{}, fmt.Errorf("history: malformed hunk header %q", ln)
	}
	os, ol, err := parseSpan(f[1][1:])
	if err != nil {
		return Hunk{}, fmt.Errorf("history: hunk header %q: %w", ln, err)
	}
	ns, nl, err := parseSpan(f[2][1:])
	if err != nil {
		return Hunk{}, fmt.Errorf("history: hunk header %q: %w", ln, err)
	}
	return Hunk{OldStart: os, OldLines: ol, NewStart: ns, NewLines: nl}, nil
}

func parseSpan(s string) (start, n int, err error) {
	a, b, hasLen := strings.Cut(s, ",")
	if start, err = strconv.Atoi(a); err != nil {
		return 0, 0, err
	}
	n = 1
	if hasLen {
		if n, err = strconv.Atoi(b); err != nil {
			return 0, 0, err
		}
	}
	return start, n, nil
}
