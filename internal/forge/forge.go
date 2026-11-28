// Package forge defines the interface git-why uses to look up pull requests,
// issues and review discussion on a code host, plus a GitHub implementation.
package forge

import (
	"context"
	"errors"
	"time"
)

// ErrUnavailable marks failures after which the caller should degrade to
// local-only evidence (rate limited, unauthorised, network down).
var ErrUnavailable = errors.New("forge unavailable")

// PullRequest is the subset of a pull request relevant to a decision history.
type PullRequest struct {
	Number   int
	Title    string
	Body     string
	Author   string
	URL      string
	MergedAt time.Time
	Merged   bool
}

// Issue is a GitHub issue (or the issue-shaped view of a pull request).
type Issue struct {
	Number int
	Title  string
	Body   string
	Author string
	URL    string
	State  string
	Labels []string
}

// Forge looks up code-host metadata for a single repository.
type Forge interface {
	// PullRequestsForCommit returns the pull requests that contain a commit.
	PullRequestsForCommit(ctx context.Context, sha string) ([]PullRequest, error)
	// PullRequestDetails fetches a PR with closing issues, comments and review threads.
	PullRequestDetails(ctx context.Context, number int) (PullRequestDetails, error)
	// Issue fetches one issue by number.
	Issue(ctx context.Context, number int) (Issue, error)
}
