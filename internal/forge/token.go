package forge

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TokenSource names where a token was found, for diagnostics.
type TokenSource string

const (
	SourceNone   TokenSource = "none"
	SourceGitHub TokenSource = "GITHUB_TOKEN"
	SourceGH     TokenSource = "GH_TOKEN"
	SourceGHCLI  TokenSource = "gh auth token"
)

// DiscoverToken looks for a GitHub token in GITHUB_TOKEN, GH_TOKEN and finally
// `gh auth token` for the given host. getenv and run are injectable for tests;
// nil values use the process environment and the real gh binary.
func DiscoverToken(ctx context.Context, host string, getenv func(string) string, run func(ctx context.Context, name string, args ...string) (string, error)) (string, TokenSource) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if run == nil {
		run = runCommand
	}
	if v := strings.TrimSpace(getenv("GITHUB_TOKEN")); v != "" {
		return v, SourceGitHub
	}
	if v := strings.TrimSpace(getenv("GH_TOKEN")); v != "" {
		return v, SourceGH
	}
	args := []string{"auth", "token"}
	if host != "" && host != "github.com" {
		args = append(args, "--hostname", host)
	}
	out, err := run(ctx, "gh", args...)
	if err != nil {
		return "", SourceNone
	}
	if v := strings.TrimSpace(out); v != "" {
		return v, SourceGHCLI
	}
	return "", SourceNone
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
