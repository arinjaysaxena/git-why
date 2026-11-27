package gitx

import (
	"fmt"
	"net/url"
	"strings"
)

// Remote is a parsed repository location on a git host.
type Remote struct {
	Host  string // e.g. github.com or ghe.example.com
	Owner string // user, organisation or group path
	Repo  string // repository name without .git
}

// Slug returns "owner/repo".
func (r Remote) Slug() string { return r.Owner + "/" + r.Repo }

// ParseRemote understands https, http, ssh://, git:// and scp-style
// (git@host:owner/repo.git) URLs, including GitHub Enterprise hosts and ports.
func ParseRemote(raw string) (Remote, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Remote{}, fmt.Errorf("empty remote url")
	}
	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Remote{}, fmt.Errorf("parse remote %q: %w", raw, err)
		}
		host, path = u.Hostname(), u.Path
	} else if i := strings.Index(raw, ":"); i > 0 && !strings.ContainsAny(raw[:i], "/\\") {
		// scp-like syntax: [user@]host:path
		host, path = raw[:i], raw[i+1:]
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
	} else {
		return Remote{}, fmt.Errorf("unrecognised remote url %q", raw)
	}
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	idx := strings.LastIndex(path, "/")
	if host == "" || idx <= 0 || idx == len(path)-1 {
		return Remote{}, fmt.Errorf("remote %q has no owner/repo path", raw)
	}
	return Remote{Host: strings.ToLower(host), Owner: path[:idx], Repo: path[idx+1:]}, nil
}
