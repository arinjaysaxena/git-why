package forge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHub implements Forge against the GitHub REST (and GraphQL) APIs.
type GitHub struct {
	owner, repo string
	token       string
	baseURL     string // REST root, no trailing slash
	graphqlURL  string
	http        *http.Client
	lim         *limiter
}

// Options configures a GitHub client.
type Options struct {
	Host       string // "github.com" (default) or a GitHub Enterprise host
	Owner      string
	Repo       string
	Token      string       // empty means unauthenticated
	HTTPClient *http.Client // optional
	BaseURL    string       // optional REST root override (tests)
	GraphQLURL string       // optional GraphQL endpoint override (tests)
}

// NewGitHub builds a client. Enterprise hosts use /api/v3 and /api/graphql.
func NewGitHub(o Options) (*GitHub, error) {
	if o.Owner == "" || o.Repo == "" {
		return nil, errors.New("github: owner and repo are required")
	}
	g := &GitHub{owner: o.Owner, repo: o.Repo, token: o.Token, http: o.HTTPClient, lim: newLimiter()}
	if g.http == nil {
		g.http = &http.Client{Timeout: 20 * time.Second}
	}
	host := strings.ToLower(o.Host)
	switch {
	case o.BaseURL != "":
		g.baseURL = strings.TrimRight(o.BaseURL, "/")
		g.graphqlURL = o.GraphQLURL
	case host == "" || host == "github.com":
		g.baseURL = "https://api.github.com"
		g.graphqlURL = "https://api.github.com/graphql"
	default:
		g.baseURL = "https://" + host + "/api/v3"
		g.graphqlURL = "https://" + host + "/api/graphql"
	}
	if g.graphqlURL == "" {
		g.graphqlURL = g.baseURL + "/graphql"
	}
	return g, nil
}

// Authenticated reports whether a token is configured.
func (g *GitHub) Authenticated() bool { return g.token != "" }

// APIError is a non-2xx response from the API.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("github: HTTP %d: %s", e.Status, e.Message) }

// NotFound reports whether err is a 404 from the API.
func NotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

func (g *GitHub) newRequest(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "git-why")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	return req, nil
}

// do executes a request, honouring the limiter. Rate-limit and auth failures
// are wrapped so errors.Is(err, ErrUnavailable) holds.
func (g *GitHub) do(ctx context.Context, req *http.Request, out any) error {
	if err := g.lim.wait(ctx); err != nil {
		return fmt.Errorf("github: rate limit exhausted: %w", err)
	}
	resp, err := g.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("github: %w: %w", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	g.lim.observe(resp.Header)
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("github: read body: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("github: decode response: %w", err)
		}
		return nil
	}
	ae := &APIError{Status: resp.StatusCode, Message: apiMessage(data)}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusUnauthorized,
		resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %w", ErrUnavailable, ae)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: %w", ErrUnavailable, ae)
	}
	return ae
}

func apiMessage(data []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &m) == nil && m.Message != "" {
		return m.Message
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func (g *GitHub) get(ctx context.Context, path string, out any) error {
	req, err := g.newRequest(ctx, http.MethodGet, g.baseURL+path, nil)
	if err != nil {
		return err
	}
	return g.do(ctx, req, out)
}

func (g *GitHub) repoPath(parts ...string) string {
	p := "/repos/" + url.PathEscape(g.owner) + "/" + url.PathEscape(g.repo)
	for _, s := range parts {
		p += "/" + url.PathEscape(s)
	}
	return p
}

type restUser struct {
	Login string `json:"login"`
}

// PullRequestsForCommit lists PRs containing sha. Squash and rebase merges are
// covered because GitHub associates the resulting commit with the PR.
func (g *GitHub) PullRequestsForCommit(ctx context.Context, sha string) ([]PullRequest, error) {
	var raw []struct {
		Number   int        `json:"number"`
		Title    string     `json:"title"`
		Body     string     `json:"body"`
		HTMLURL  string     `json:"html_url"`
		MergedAt *time.Time `json:"merged_at"`
		User     restUser   `json:"user"`
	}
	if err := g.get(ctx, g.repoPath("commits", sha, "pulls")+"?per_page=30", &raw); err != nil {
		if NotFound(err) {
			return nil, nil // unknown commit (unpushed): no PRs
		}
		return nil, err
	}
	prs := make([]PullRequest, 0, len(raw))
	for _, r := range raw {
		pr := PullRequest{Number: r.Number, Title: r.Title, Body: r.Body, URL: r.HTMLURL, Author: r.User.Login}
		if r.MergedAt != nil {
			pr.Merged, pr.MergedAt = true, *r.MergedAt
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

// Issue fetches one issue.
func (g *GitHub) Issue(ctx context.Context, number int) (Issue, error) {
	var raw struct {
		Number  int      `json:"number"`
		Title   string   `json:"title"`
		Body    string   `json:"body"`
		State   string   `json:"state"`
		HTMLURL string   `json:"html_url"`
		User    restUser `json:"user"`
		Labels  []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := g.get(ctx, g.repoPath("issues", fmt.Sprint(number)), &raw); err != nil {
		return Issue{}, err
	}
	is := Issue{Number: raw.Number, Title: raw.Title, Body: raw.Body, State: raw.State, URL: raw.HTMLURL, Author: raw.User.Login}
	for _, l := range raw.Labels {
		is.Labels = append(is.Labels, l.Name)
	}
	return is, nil
}

var _ Forge = (*GitHub)(nil)
