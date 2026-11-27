// Package gitx is a thin wrapper over the git binary. Driving git directly is
// the most faithful way to get blame, range history and rename handling.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Client runs git commands inside a working directory.
type Client struct {
	Dir string
}

// New returns a Client rooted at dir.
func New(dir string) *Client { return &Client{Dir: dir} }

// Error is a failed git invocation with its stderr.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// ErrNotARepo is returned when Dir is not inside a git work tree.
var ErrNotARepo = errors.New("not a git repository")

// Run executes git and returns stdout verbatim.
func (c *Client) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		se := strings.TrimSpace(stderr.String())
		if strings.Contains(se, "not a git repository") {
			return "", fmt.Errorf("%w: %s", ErrNotARepo, c.Dir)
		}
		return "", &Error{Args: args, Stderr: se, Err: err}
	}
	return stdout.String(), nil
}

// TopLevel returns the absolute path of the work tree root.
func (c *Client) TopLevel(ctx context.Context) (string, error) {
	out, err := c.Run(ctx, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

// RevParse resolves a revision to a full SHA.
func (c *Client) RevParse(ctx context.Context, rev string) (string, error) {
	out, err := c.Run(ctx, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	return strings.TrimSpace(out), err
}

// ShowFile returns a file's content at a revision.
func (c *Client) ShowFile(ctx context.Context, rev, path string) (string, error) {
	return c.Run(ctx, "show", rev+":"+path)
}

// RemoteURL returns the URL configured for a remote.
func (c *Client) RemoteURL(ctx context.Context, name string) (string, error) {
	out, err := c.Run(ctx, "remote", "get-url", name)
	return strings.TrimSpace(out), err
}

// FirstParentChain returns the SHAs reachable from rev by following only first
// parents, newest first. These are the commits that sit directly on the
// mainline: ordinary commits, squash merges and merge commits themselves.
func (c *Client) FirstParentChain(ctx context.Context, rev string) ([]string, error) {
	out, err := c.Run(ctx, "rev-list", "--first-parent", rev, "--")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// MergeInto finds the merge commit on rev's first-parent line that first
// brought sha into rev. It returns "" when sha is not reachable through a merge
// (it is on the mainline itself, or is not an ancestor of rev).
func (c *Client) MergeInto(ctx context.Context, sha, rev string) (string, error) {
	out, err := c.Run(ctx, "rev-list", "--first-parent", "--ancestry-path", "--merges", "--reverse", sha+".."+rev, "--")
	if err != nil {
		return "", err
	}
	if f := strings.Fields(out); len(f) > 0 {
		return f[0], nil
	}
	return "", nil
}
