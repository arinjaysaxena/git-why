// Package testrepo builds throwaway git repositories with scripted histories
// for tests. It drives the real git binary, so fixtures behave exactly like
// repositories created by hand. Commit dates advance one day per commit, which
// keeps histories reproducible without touching the process environment.
package testrepo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Repo is a temporary git repository with a main branch.
type Repo struct {
	t    testing.TB
	dir  string
	tick int
}

var epoch = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

// New creates an empty repository on branch "main" inside t.TempDir().
func New(t testing.TB) *Repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	r := &Repo{t: t, dir: t.TempDir()}
	r.Git("init", "--quiet", "--initial-branch=main")
	r.Git("config", "user.name", "Test Author")
	r.Git("config", "user.email", "author@example.com")
	r.Git("config", "commit.gpgsign", "false")
	r.Git("config", "core.autocrlf", "false")
	return r
}

// Dir is the working tree root.
func (r *Repo) Dir() string { return r.dir }

// Path returns the absolute path of a slash-separated path inside the repo.
func (r *Repo) Path(rel string) string { return filepath.Join(r.dir, filepath.FromSlash(rel)) }

// Git runs git in the repository and returns trimmed stdout. A failing
// command fails the test with git's stderr.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	out, err := r.run(nil, args...)
	if err != nil {
		r.t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func (r *Repo) run(extraEnv []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_COUNT=0", // ignore config injected through the environment
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", &gitError{err: err, stderr: strings.TrimSpace(stderr.String())}
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}

type gitError struct {
	err    error
	stderr string
}

func (e *gitError) Error() string { return e.err.Error() + ": " + e.stderr }
func (e *gitError) Unwrap() error { return e.err }

// Write creates or replaces a file, creating parent directories.
func (r *Repo) Write(rel, content string) *Repo {
	r.t.Helper()
	p := r.Path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
	return r
}

// Read returns the content of a working-tree file.
func (r *Repo) Read(rel string) string {
	r.t.Helper()
	b, err := os.ReadFile(r.Path(rel))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

// Remove deletes a file from the working tree.
func (r *Repo) Remove(rel string) *Repo {
	r.t.Helper()
	if err := os.Remove(r.Path(rel)); err != nil {
		r.t.Fatal(err)
	}
	return r
}

// Move renames a tracked file with git mv, creating the destination directory.
func (r *Repo) Move(from, to string) *Repo {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(r.Path(to)), 0o755); err != nil {
		r.t.Fatal(err)
	}
	r.Git("mv", from, to)
	return r
}

// Commit stages everything and commits as the default author.
func (r *Repo) Commit(msg string) string {
	r.t.Helper()
	return r.CommitAs("Test Author", "author@example.com", msg)
}

// CommitAs stages everything and commits as the given author. It returns the
// new commit's full SHA.
func (r *Repo) CommitAs(name, email, msg string) string {
	r.t.Helper()
	r.Git("add", "-A")
	r.commit(name, email, "commit", "--quiet", "--allow-empty", "-m", msg)
	return r.Head()
}

// Merge merges a branch with a merge commit (never fast-forward).
func (r *Repo) Merge(branch, msg string) string {
	r.t.Helper()
	r.commit("Test Author", "author@example.com", "merge", "--no-ff", "--quiet", "-m", msg, branch)
	return r.Head()
}

// SquashMerge squashes a branch into the current branch as one ordinary
// commit, the way GitHub's "squash and merge" does.
func (r *Repo) SquashMerge(branch, msg string) string {
	r.t.Helper()
	r.Git("merge", "--squash", branch)
	r.commit("Test Author", "author@example.com", "commit", "--quiet", "--allow-empty", "-m", msg)
	return r.Head()
}

func (r *Repo) commit(name, email string, args ...string) {
	r.t.Helper()
	r.tick++
	date := epoch.AddDate(0, 0, r.tick).Format(time.RFC3339)
	env := []string{
		"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email, "GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email, "GIT_COMMITTER_DATE=" + date,
	}
	if _, err := r.run(env, args...); err != nil {
		r.t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

// Branch creates a branch at HEAD and switches to it.
func (r *Repo) Branch(name string) *Repo {
	r.t.Helper()
	r.Git("checkout", "--quiet", "-b", name)
	return r
}

// Checkout switches to an existing branch.
func (r *Repo) Checkout(name string) *Repo {
	r.t.Helper()
	r.Git("checkout", "--quiet", name)
	return r
}

// Head returns the full SHA of HEAD.
func (r *Repo) Head() string {
	r.t.Helper()
	return r.Git("rev-parse", "HEAD")
}

// Parents returns the parent SHAs of a revision.
func (r *Repo) Parents(rev string) []string {
	r.t.Helper()
	out := r.Git("rev-list", "--parents", "-n", "1", rev)
	fields := strings.Fields(out)
	if len(fields) <= 1 {
		return nil
	}
	return fields[1:]
}

// AddRemote registers a remote, for tests that parse remote URLs.
func (r *Repo) AddRemote(name, url string) *Repo {
	r.t.Helper()
	r.Git("remote", "add", name, url)
	return r
}
