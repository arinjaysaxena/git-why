package gitx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RangeCommit is a commit that touched a line range, with the diff of the
// range as git followed it.
type RangeCommit struct {
	SHA     string
	Parents []string
	Author  string
	Email   string
	Time    time.Time
	Subject string
	Body    string
	Path    string // path of the file after this commit
	OldPath string // path before this commit; differs from Path on a rename
	Patch   string // unified diff restricted to the tracked range
}

// LogRangeOptions bound a range-history query.
type LogRangeOptions struct {
	Rev   string     // start revision; default HEAD
	Max   int        // maximum commits; 0 means unlimited
	Since *time.Time // ignore commits authored before this time
}

const (
	recordSep = "\x1e"
	fieldSep  = "\x1f"
	logFormat = recordSep + "%H" + fieldSep + "%P" + fieldSep + "%an" + fieldSep + "%ae" + fieldSep + "%at" + fieldSep + "%s" + fieldSep + "%b" + fieldSep
)

// LogRange runs `git log -L start,end:path` and returns commits newest first.
// git follows the range as lines move and as the file is renamed.
func (c *Client) LogRange(ctx context.Context, path string, start, end int, opt LogRangeOptions) ([]RangeCommit, error) {
	if start < 1 || end < start {
		return nil, fmt.Errorf("invalid line range %d-%d", start, end)
	}
	args := []string{"log", "--no-color", "--no-ext-diff", "--format=" + logFormat,
		fmt.Sprintf("-L%d,%d:%s", start, end, path)}
	if opt.Max > 0 {
		args = append(args, "-n", strconv.Itoa(opt.Max))
	}
	if opt.Since != nil {
		args = append(args, "--since="+opt.Since.Format(time.RFC3339))
	}
	rev := opt.Rev
	if rev == "" {
		rev = "HEAD"
	}
	args = append(args, rev, "--")
	out, err := c.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return ParseLogRange(out)
}

// ParseLogRange parses the output of LogRange's git invocation.
func ParseLogRange(out string) ([]RangeCommit, error) {
	var commits []RangeCommit
	for _, rec := range strings.Split(out, recordSep) {
		if strings.TrimSpace(rec) == "" {
			continue
		}
		f := strings.SplitN(rec, fieldSep, 8)
		if len(f) < 8 {
			return nil, fmt.Errorf("log: malformed record %q", truncate(rec, 60))
		}
		sec, err := strconv.ParseInt(f[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("log: bad timestamp %q", f[4])
		}
		rc := RangeCommit{
			SHA: f[0], Parents: strings.Fields(f[1]), Author: f[2], Email: f[3],
			Time: time.Unix(sec, 0).UTC(), Subject: f[5], Body: strings.TrimSpace(f[6]),
		}
		rc.Path, rc.OldPath, rc.Patch = splitPatch(f[7])
		commits = append(commits, rc)
	}
	return commits, nil
}

// splitPatch extracts old/new paths and the trimmed patch from a log body.
func splitPatch(s string) (path, oldPath, patch string) {
	s = strings.Trim(s, "\n")
	patch = s
headers:
	for _, ln := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(ln, "--- "):
			oldPath = stripPrefix(ln[4:])
		case strings.HasPrefix(ln, "+++ "):
			path = stripPrefix(ln[4:])
		case strings.HasPrefix(ln, "@@"):
			break headers
		}
	}
	// A created file has /dev/null on the old side and a deleted one on the
	// new side; report the surviving path for both so callers need no special case.
	if oldPath == "" {
		oldPath = path
	}
	if path == "" {
		path = oldPath
	}
	return path, oldPath, patch
}

func stripPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(p, "a/"), "b/")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
