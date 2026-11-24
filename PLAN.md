# git-why plan

**Pitch.** git-why answers "why is this code like this?". Give it `file:line` or a function name and it walks blame, then the commit, the pull request that introduced it, the linked issue and the review comments, and writes a short decision history: why the code looks like this, what was tried, which constraints still apply. It uses any OpenAI-compatible endpoint for the write-up (local Ollama by default) and has an offline `--raw` mode that prints the collected evidence with no model at all.

## v0.1 scope
- CLI: `git-why <file>:<line> | <file> --fn <name> [--raw] [--no-cache]`
- Blame via the local git binary (porcelain parser), follows line history
- GitHub provider: commit -> PR -> linked issues -> review comments (REST, token via env)
- On-disk cache so repeated runs are free
- `--raw` offline mode: evidence only, no network, no model
- Tests never need network or API keys (mock fetch, temp git repos)

## Schedule
### 2026-11-23 - foundations
- [x] Skeleton: ESM, node:test, eslint, CI
- [x] Blame porcelain parser + target parsing (`file:line`)

### 2026-11-24 - git layer
- [x] Run git blame on a real temp repo; resolve line to commit
- [x] Function locator (`--fn`) for JS/TS/Python/Java by regex

### 2026-11-25 - provider
- [x] Remote detection (origin -> owner/repo)
- [x] GitHub provider: commit -> PRs, PR body, linked issues (injectable fetch)

### 2026-11-26 - reviews + cache
- [x] Review comments and discussion collection
- [x] On-disk cache keyed by repo+sha, `--no-cache`

### 2026-11-27 - evidence
- [x] Evidence model + `--raw` renderer (offline)
- [x] Walk back through earlier commits for the same lines (history chain)

### 2026-11-29 - narration
- [x] Provider-neutral LLM client + decision-history prompt
- [x] CLI wiring end to end

### 2026-12-01 - quality
- [x] Rate-limit and missing-token handling, error messages
- [x] Integration tests on temp repo with mocked provider

### 2026-12-02 - ship
- [x] README (pitch, demo GIF steps, install, usage, how it works, roadmap)
- [x] LICENSE (MIT), polish, CI green

Status: v0.1 shipped
