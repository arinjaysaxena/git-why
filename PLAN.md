# git-why plan

Status: v1 rebuild in progress (Go). Spec: [docs/DESIGN.md](docs/DESIGN.md).

v0.1 was a Node prototype. v1 is a rewrite in Go as a single static binary, with a line-range history walker, a GitHub forge using GraphQL review threads, an evidence graph with relevance ranking, citation-checked narration and goreleaser packaging.

Rules for v1:
- Work through the milestones in order; one or two tasks per working session, each a complete, tested change.
- v1 ships only when **every acceptance criterion** below passes and not before 2026-12-04.
- After v1 ships, keep going down the Roadmap section. There is always a next task.

## Milestone 1: Go foundation (2026-11-27)
- [x] Go module, `cmd/git-why` with cobra, Makefile, golangci-lint, CI (`go test -race`) on Linux, macOS, Windows
- [x] `internal/testrepo`: build temp git repos with scripted histories (commits, renames, moves, merges, squash merges)
- [x] `internal/target`: parse `file:line`, `file:a-b`, `file#Symbol`, `--fn`

## Milestone 2: git layer + history (2026-11-28)
- [x] `internal/gitx`: blame porcelain, `log -L` with sentinel format, show, remote parsing (https/ssh, GitHub Enterprise hosts)
- [x] `internal/history`: walk a line range back through commits, follow renames and moves, depth and since limits
- [x] Per-commit hunk restricted to the target range

## Milestone 3: symbol locator (2026-11-29)
- [x] `go/parser`-based locator for Go functions, methods and types
- [x] Brace/indent scanner for TS/JS, Java, Python and Rust with fixture files
- [x] Wire `--fn` and `#Symbol` into history

## Milestone 4: GitHub forge (2026-11-30)
- [x] REST: commit → PRs; issues; token discovery (`GITHUB_TOKEN`, `GH_TOKEN`, `gh auth token`)
- [x] GraphQL: PR body, closing issues, review threads with path/line/originalLine; filter to the target range
- [x] Rate-limit-aware limiter, bounded worker pool, graceful degradation; httptest fixtures

## Milestone 5: cache + evidence (2026-12-01)
- [x] On-disk cache with ETag revalidation; `git why cache clear|stats`
- [ ] Evidence graph types and builder; reference parsing (`#123`, `GH-123`, URLs)
- [ ] Relevance ranking and pruning; `--explain`

## Milestone 6: output + narration (2026-12-02)
- [ ] `--raw` terminal timeline and `--json` (schemaVersion 1, `docs/json.md`); golden tests
- [ ] OpenAI-compatible streaming client (Ollama default), prompt with evidence IDs
- [ ] Citation checker that drops or flags uncited sentences; no-model fallback to `--raw`

## Milestone 7: parity + packaging (2026-12-03)
- [ ] Feature parity with the Node prototype; delete the Node code (repo is Go only)
- [ ] goreleaser config (linux/darwin/windows, amd64/arm64, checksums, Homebrew formula); snapshot build in CI
- [ ] Opt-in e2e (`make e2e`) against public repositories

## Milestone 8: v1 release (2026-12-04)
- [ ] README rewrite: pitch, real example, install (`go install`, Homebrew, binaries), CLI reference, configuration, how it works
- [ ] CHANGELOG, tag v1.0.0, release notes

### v1 acceptance criteria
- [ ] No JavaScript left in the repo; `go vet` and golangci-lint clean
- [ ] History walker passes fixture tests for renames, moves, merges and squash merges
- [ ] Review threads are correctly filtered to the target lines (fixture with drifted `originalLine`)
- [ ] Narration never outputs an uncited sentence (tested with a fake model)
- [ ] Works with no token and with `--no-network`, with clear notices
- [ ] goreleaser snapshot builds all targets in CI
- [ ] CI green on Linux, macOS and Windows

## Roadmap (after v1, keep working down this list)
- [ ] GitLab forge (merge requests, discussions) behind the Forge interface
- [ ] `git why --pr <n>`: explain every non-trivial change in a PR from its history
- [ ] VS Code extension that renders `--json` as a hover card
- [ ] Interactive TUI (bubbletea) to browse the evidence graph
- [ ] Batch mode: `git why --hotspots` lists the most-discussed lines in a repo
- [ ] Offline issue mirror: sync PRs and issues once, then fully offline
- [ ] Benchmarks on large repositories and history-walk caching
