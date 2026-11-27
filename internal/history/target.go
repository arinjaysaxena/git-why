package history

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arinjaysaxena/git-why/internal/locate"
	"github.com/arinjaysaxena/git-why/internal/target"
)

// TargetQuery is a user-facing target plus the walk limits.
type TargetQuery struct {
	Target target.Target
	Rev    string
	Depth  int
	Since  *time.Time
}

// Resolved says which lines of which file a target turned out to mean.
type Resolved struct {
	Path       string
	Start, End int
	// Symbol is the qualified name the locator matched; empty for line targets.
	Symbol string
}

// WalkTarget resolves t to a line range at the queried revision (running the
// symbol locator for symbol targets) and walks that range's history.
func (w *Walker) WalkTarget(ctx context.Context, q TargetQuery) (Result, Resolved, error) {
	rev := q.Rev
	if rev == "" {
		rev = "HEAD"
	}
	res, err := w.resolve(ctx, q.Target, rev)
	if err != nil {
		return Result{}, Resolved{}, err
	}
	out, err := w.Walk(ctx, Query{Path: res.Path, Start: res.Start, End: res.End, Rev: rev, Depth: q.Depth, Since: q.Since})
	if err != nil {
		return Result{}, res, err
	}
	return out, res, nil
}

func (w *Walker) resolve(ctx context.Context, t target.Target, rev string) (Resolved, error) {
	content, err := w.Git.ShowFile(ctx, rev, t.File)
	if err != nil {
		return Resolved{}, fmt.Errorf("read %s at %s: %w", t.File, rev, err)
	}
	lines := countLines(content)
	switch {
	case t.HasSymbol():
		r, err := locate.Locate(t.File, content, t.Symbol)
		if err != nil {
			if errors.Is(err, locate.ErrNotFound) {
				return Resolved{}, fmt.Errorf("%s: %w", t.File, err)
			}
			return Resolved{}, err
		}
		return Resolved{Path: t.File, Start: r.Start, End: r.End, Symbol: r.Name}, nil
	case t.HasRange():
		if t.End > lines || t.Start > lines {
			return Resolved{}, fmt.Errorf("%s has %d lines, cannot trace %d-%d", t.File, lines, t.Start, max(t.Start, t.End))
		}
		end := t.End
		if end == 0 {
			end = t.Start
		}
		return Resolved{Path: t.File, Start: t.Start, End: end}, nil
	default:
		if lines == 0 {
			return Resolved{}, fmt.Errorf("%s is empty", t.File)
		}
		return Resolved{Path: t.File, Start: 1, End: lines}, nil
	}
}

// countLines counts lines the way editors number them: a trailing newline does not add a line.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
