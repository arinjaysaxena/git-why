// Package history walks a line range back through the commits that changed it.
// It relies on git's own range tracking (log -L), which follows the range as
// surrounding code shifts and as the file is moved, and layers on the parts
// git does not give directly: stop conditions, per-commit hunks and the merge
// each commit reached the mainline through.
package history

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/arinjaysaxena/git-why/internal/gitx"
)

// DefaultDepth is the default maximum number of commits to walk.
const DefaultDepth = 8

// Query describes the range to trace. Line numbers refer to Path at Rev.
type Query struct {
	Path  string
	Start int
	End   int
	Rev   string     // default HEAD
	Depth int        // max commits; <=0 means DefaultDepth
	Since *time.Time // ignore commits authored before this
}

// StopReason says why the walk ended.
type StopReason string

const (
	StopIntroduced StopReason = "introduced" // reached the commit that created the lines
	StopDepth      StopReason = "depth"      // more history exists beyond Depth
	StopSince      StopReason = "since"      // older history was excluded by Since
	StopEmpty      StopReason = "empty"      // no commit touched the range
)

// Commit is one step in the range's history.
type Commit struct {
	gitx.RangeCommit
	Hunks      []Hunk
	Renamed    bool   // the file was moved by this commit; OldPath is the earlier name
	Merge      string // merge commit that brought this commit onto the mainline, if any
	Introduced bool   // this commit created the tracked lines
}

// Result is the walk outcome, newest commit first.
type Result struct {
	Commits []Commit
	Stop    StopReason
}

// Paths returns every distinct path the range lived at, newest first.
func (r Result) Paths() []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range r.Commits {
		for _, p := range []string{c.Path, c.OldPath} {
			if p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// Walker traces ranges in one repository.
type Walker struct {
	Git *gitx.Client
}

// Walk traces q and returns its history.
func (w *Walker) Walk(ctx context.Context, q Query) (Result, error) {
	if q.Path == "" {
		return Result{}, errors.New("history: empty path")
	}
	if q.Start < 1 || q.End < q.Start {
		return Result{}, fmt.Errorf("history: invalid line range %d-%d", q.Start, q.End)
	}
	depth := q.Depth
	if depth <= 0 {
		depth = DefaultDepth
	}
	rev := q.Rev
	if rev == "" {
		rev = "HEAD"
	}
	// Ask for one extra commit so truncation is detectable without a second query.
	raw, err := w.Git.LogRange(ctx, q.Path, q.Start, q.End, gitx.LogRangeOptions{Rev: rev, Max: depth + 1, Since: q.Since})
	if err != nil {
		return Result{}, fmt.Errorf("history: %w", err)
	}
	res := Result{Stop: StopEmpty}
	if len(raw) == 0 {
		return res, nil
	}
	truncated := len(raw) > depth
	if truncated {
		raw = raw[:depth]
	}

	chain, err := w.Git.FirstParentChain(ctx, rev)
	if err != nil {
		return Result{}, fmt.Errorf("history: %w", err)
	}
	mainline := make(map[string]bool, len(chain))
	for _, s := range chain {
		mainline[s] = true
	}

	for _, rc := range raw {
		hunks, err := ParseHunks(rc.Patch)
		if err != nil {
			return Result{}, fmt.Errorf("commit %.8s: %w", rc.SHA, err)
		}
		c := Commit{RangeCommit: rc, Hunks: hunks, Renamed: rc.OldPath != rc.Path}
		if !mainline[rc.SHA] {
			if c.Merge, err = w.Git.MergeInto(ctx, rc.SHA, rev); err != nil {
				return Result{}, fmt.Errorf("history: %w", err)
			}
		}
		res.Commits = append(res.Commits, c)
	}

	last := &res.Commits[len(res.Commits)-1]
	switch {
	case truncated:
		res.Stop = StopDepth
	case q.Since != nil && len(last.Parents) > 0:
		// With a lower bound the oldest listed commit may not be the origin.
		res.Stop = StopSince
	default:
		// git ran out of history: the oldest commit created these lines.
		last.Introduced = true
		res.Stop = StopIntroduced
	}
	return res, nil
}
