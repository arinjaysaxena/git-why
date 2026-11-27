package gitx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BlameLine is one line of `git blame --porcelain` output.
type BlameLine struct {
	Line     int    // line number in the blamed revision (1-based)
	OrigLine int    // line number in the commit that introduced it
	SHA      string // introducing commit
	Author   string
	Email    string
	Time     time.Time
	Summary  string
	Path     string // file path in the introducing commit
	Boundary bool   // the line predates the walked history
	Content  string
}

// Blame blames lines start..end (1-based, inclusive) of path at rev. An empty
// rev means the working tree. Rename and move detection is enabled.
func (c *Client) Blame(ctx context.Context, rev, path string, start, end int) ([]BlameLine, error) {
	if start < 1 || end < start {
		return nil, fmt.Errorf("invalid line range %d-%d", start, end)
	}
	args := []string{"blame", "--porcelain", "-M", "-C", "-L", fmt.Sprintf("%d,%d", start, end)}
	if rev != "" {
		args = append(args, rev)
	}
	args = append(args, "--", path)
	out, err := c.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return ParseBlame(out)
}

// ParseBlame parses `git blame --porcelain` output. Metadata appears only on
// the first occurrence of each commit, so it is cached by SHA.
func ParseBlame(out string) ([]BlameLine, error) {
	type meta struct {
		author, email, summary, path string
		when                         time.Time
		boundary                     bool
	}
	metas := map[string]*meta{}
	var lines []BlameLine
	var cur *BlameLine
	var curMeta *meta

	for _, raw := range strings.Split(out, "\n") {
		if cur == nil {
			if raw == "" {
				continue
			}
			f := strings.Fields(raw)
			if len(f) < 3 || len(f[0]) < 40 {
				return nil, fmt.Errorf("blame: unexpected header %q", raw)
			}
			orig, err1 := strconv.Atoi(f[1])
			final, err2 := strconv.Atoi(f[2])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("blame: bad line numbers in %q", raw)
			}
			if metas[f[0]] == nil {
				metas[f[0]] = &meta{}
			}
			curMeta = metas[f[0]]
			cur = &BlameLine{SHA: f[0], OrigLine: orig, Line: final}
			continue
		}
		if strings.HasPrefix(raw, "\t") {
			cur.Content = raw[1:]
			cur.Author, cur.Email, cur.Time = curMeta.author, curMeta.email, curMeta.when
			cur.Summary, cur.Path, cur.Boundary = curMeta.summary, curMeta.path, curMeta.boundary
			lines = append(lines, *cur)
			cur, curMeta = nil, nil
			continue
		}
		key, val, _ := strings.Cut(raw, " ")
		switch key {
		case "author":
			curMeta.author = val
		case "author-mail":
			curMeta.email = strings.Trim(val, "<>")
		case "author-time":
			if sec, err := strconv.ParseInt(val, 10, 64); err == nil {
				curMeta.when = time.Unix(sec, 0).UTC()
			}
		case "summary":
			curMeta.summary = val
		case "filename":
			curMeta.path = val
		case "boundary":
			curMeta.boundary = true
		}
	}
	if cur != nil {
		return nil, fmt.Errorf("blame: truncated output")
	}
	return lines, nil
}
