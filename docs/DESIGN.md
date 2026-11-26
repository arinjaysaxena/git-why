# git-why v1 design

> `git blame` tells you who. `git why` tells you why.

## The problem

You're staring at a weird line: a retry loop capped at 3, a hard-coded timezone, a check that looks redundant. `git blame` points at a commit called "fix". The real reasons are scattered across a chain of commits, a pull request description, a review thread where someone said "don't remove this, it breaks Safari", and a linked issue. git-why reconstructs that chain and writes the decision history.

## What v1 does

Illustrative output:

```
$ git why internal/retry/backoff.go:42

internal/retry/backoff.go:42   maxAttempts := 3
Decision history (4 commits · 2 PRs · 1 issue · 6 review comments)

2025-03-11  #812  "Cap retries to avoid thundering herd"   (a1b2c3d, @maya)
            Issue #799: retry storms during the March 9 outage took down the auth service.
            Review: "3 is arbitrary but matches the LB's 10s timeout window" (@dev-lee)
2024-11-02  #640  "Add exponential backoff"                  (9f8e7d6, @sam)
            Originally unbounded; jitter added after review.

Why it looks like this
  The cap of 3 exists because of the outage in #799: unbounded retries amplified load.
  3 attempts fit inside the load balancer's 10s timeout (review on #812).
Constraints still in force
  · Changing the cap affects the LB timeout budget (see #812 review).
Tried before
  · Unbounded retries with jitter (#640), replaced after #799.

Sources: a1b2c3d 9f8e7d6 · PR #812 #640 · issue #799
```

Every claim in the write-up cites its source. `--raw` prints the evidence timeline with no model at all, which is fully offline when the cache is warm.

## Language and tooling

- **Go 1.23**, one static binary named `git-why`, so `git why` works as a git subcommand automatically.
- Minimal dependencies: standard library plus `spf13/cobra` for the CLI. Git itself is driven through the `git` binary (`exec.Command`), which is the most faithful implementation of `blame`, `log -L` and rename following.
- Native Go parsing via `go/parser` for Go files; a portable symbol locator for other languages (see below). No cgo, so cross-compiling stays trivial.
- `golangci-lint`, `go test -race`, and `goreleaser` for releases (linux/darwin/windows, amd64/arm64, Homebrew tap formula and checksums).
- **Migration:** the current Node prototype stays only until the Go binary reaches feature parity, then it is deleted. The repo must end up Go only.

## Architecture

```
cmd/git-why/            main, cobra commands
internal/target/        parse "file:line", "file:10-20", "file#Symbol", "--fn name"
internal/locate/        symbol → line range (go/parser for Go; brace/indent scanner for TS/JS/Java/Python/Rust)
internal/gitx/          thin wrapper over the git binary: blame --porcelain, log -L, show, rev-parse, remote
internal/history/       line-range history walker: follows the range back through commits, moves and renames
internal/forge/         Forge interface + GitHub implementation (REST + GraphQL); GitLab stub behind the interface
internal/evidence/      Evidence graph: commits, PRs, issues, review comments, edges with relevance scores
internal/rank/          relevance scoring and pruning
internal/narrate/       prompt building, OpenAI-compatible client, citation checking
internal/render/        terminal (colours, wrapping), markdown, json
internal/cache/         on-disk cache with ETag revalidation
```

### history/

- `git log -L <start>,<end>:<file> --format=<sentinel format> --no-patch` returns every commit that touched the range, following the range as lines move. Add `--follow`-style rename handling by re-anchoring on the pre-rename path when a commit renames the file.
- For each commit, fetch the hunk restricted to the range (`git log -L ... -n1 <sha>` with patch) so the evidence shows what actually changed on those lines.
- Stop conditions: the commit that introduced the lines, `--depth N` (default 8), or `--since`.
- Merge commits: attribute to the PR via the merge, but walk the first-parent and the PR's own commits.

### forge/ (GitHub)

- `GET /repos/{o}/{r}/commits/{sha}/pulls` maps commits to PRs (covers squash and rebase merges).
- GraphQL for the PR in one round trip: title, body, author, mergedAt, `closingIssuesReferences`, timeline cross-references, and review threads with `path`, `line`, `originalLine`, `diffHunk` and comments.
- Review comments are filtered to the ones anchored on the target file and overlapping the line range (accounting for `originalLine` drift), plus PR-level comments that mention the symbol.
- Issues: title, body, labels and the first N comments. Parse `#123`, `GH-123` and full URLs in commit messages too.
- Token from `GITHUB_TOKEN`, `GH_TOKEN` or `gh auth token`. Without a token, use unauthenticated mode with a clear notice and lower depth.
- **Rate limits:** a token-bucket limiter reads `X-RateLimit-Remaining`/`Reset`; requests fan out concurrently (bounded worker pool) and degrade gracefully to local-only evidence on 403/429, saying so in the output.

### evidence/ and rank/

- The evidence graph is a set of typed nodes (Commit, PR, Issue, ReviewComment) with edges (introduced-by, discussed-in, closes, mentions).
- Relevance score per node: line overlap of the hunk with the target range, recency, review comments anchored on the exact lines (high weight), keywords shared with the code (identifiers on the line), and maintainer authorship. Low scorers are pruned so the narrative stays focused.
- The graph serialises to the `--json` output (`schemaVersion: 1`, documented in `docs/json.md`), which editor integrations can consume.

### narrate/

- OpenAI-compatible `/chat/completions` client (`GIT_WHY_BASE_URL`, `GIT_WHY_MODEL`, `GIT_WHY_API_KEY`), defaulting to a local Ollama endpoint. Streaming output in the terminal.
- The prompt gives the model only the ranked evidence, with stable IDs (`[C1]`, `[PR2]`, `[I1]`, `[R3]`), and asks for the fixed sections: *Why it looks like this*, *Constraints still in force*, *Tried before*.
- **Citation check:** after generation, every sentence must cite at least one evidence ID that exists. Uncited sentences are dropped or flagged. This keeps the write-up grounded.
- No model reachable → automatic fallback to `--raw` with a notice.

### cache/

- Under `$XDG_CACHE_HOME/git-why` (or the OS equivalent), keyed by forge, repo and object (commit sha, PR number, issue number). Stores the ETag and revalidates with `If-None-Match`, so revalidation doesn't spend rate limit.
- `git why cache clear|stats`.

## CLI

```
git why <file>:<line>[-<end>]      decision history for a line or range
git why <file> --fn <name>         for a function or method
git why <file>#<Symbol>            same, symbol syntax
  --raw          evidence timeline only, no model
  --json         machine-readable evidence graph
  --depth N      max commits to walk (default 8)
  --since DATE   ignore older history
  --no-network   local git only
  --explain      show the ranking scores (debug)
```

## Quality bar

- Table-driven unit tests for every package; `go test -race ./...` in CI on Linux, macOS and Windows.
- **Fixture repositories are built in tests:** `internal/testrepo` creates temp git repos with scripted histories (renames, moves, merges, squash merges) using the real git binary.
- **Forge tests use `httptest.Server`** with recorded JSON fixtures in `testdata/github/`. No network in default CI.
- **Golden tests** for `--raw`, `--json` and markdown output.
- Opt-in e2e (`make e2e`, needs a token) against a few well-known public repositories, asserting that the expected PR and issue are found for chosen lines.
- `goreleaser release --snapshot` runs in CI so packaging never rots.

## Out of scope for v1

Hosted service, IDE extensions (they consume `--json` later), and forges other than GitHub (the interface is there).
