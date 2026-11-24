# git-why

Ask "why is this code like this?" and get a short decision history: what the code was meant to do, what was tried, and which constraints still apply.

Give it `file:line` or a function name. It walks `git blame` to the commit, then (for GitHub remotes) the pull request, linked issues and review comments, and asks a model to write the history. Any OpenAI-compatible endpoint works; local Ollama is the default. An offline `--raw` mode prints the collected evidence with no network and no model.

_Demo GIF: coming soon (recording steps below)._

<!-- To record docs/demo.gif:
  1. In a repo with a GitHub remote and some history, run: git-why src/app.js:42
  2. Record with: asciinema rec demo.cast && agg demo.cast docs/demo.gif
  3. Keep it under 20 seconds, add docs/demo.gif and link it here. -->

## Install

Requires Node 20 or newer and git.

```sh
git clone https://github.com/arinjaysaxena/git-why
cd git-why && npm install && npm link
```

## Usage

```sh
git-why src/queue.js:42              # why is this line like this?
git-why src/queue.js --fn retry      # why is this function like this?
git-why src/queue.js:42 --raw        # evidence only: offline, no model
git-why src/queue.js:42 --no-cache   # ignore the on-disk cache
```

Functions are located by pattern in JS/TS, Python and Java files. If a name is defined more than once, use `file:line`.

### Configuration

| Variable | Purpose | Default |
| --- | --- | --- |
| `GITWHY_BASE_URL` | OpenAI-compatible base URL (or `--base-url`) | `http://localhost:11434/v1` |
| `GITWHY_MODEL` | Model name (or `--model`) | `llama3.1` |
| `GITWHY_API_KEY` | Key sent as a bearer token | `ollama` |
| `GITHUB_TOKEN` | Raises the GitHub rate limit; needed for private repos | unset |
| `GIT_WHY_CACHE_DIR` | Where GitHub lookups are cached | user cache directory |

If GitHub is unreachable, rate limited or rejects the token, git-why prints a warning and continues with local git history only. Failed lookups are never cached.

## How it works

1. **Blame.** `git blame --porcelain` maps the line (or function range) to commits; `git log -L` follows the lines back through earlier commits.
2. **Provider.** For each commit, the GitHub REST API gives the pull requests that introduced it, their descriptions, issues they close or mention, and review comments and discussion (bots and empty approvals dropped, long bodies truncated).
3. **Cache.** Results are stored on disk keyed by repository and commit sha, so repeat runs are free.
4. **Evidence.** Everything is assembled into one evidence document. `--raw` prints it as is.
5. **Write-up.** Otherwise the evidence goes to the model with a prompt that asks for why the code looks like this, what was tried, and which constraints still apply, citing PR and issue numbers.

## Roadmap

- GitLab and Bitbucket providers
- `--json` output
- Following renames across files
- Editor integration (hover for the decision history)

## Development

```sh
npm install
npm test     # offline; temp git repos and mocked fetch
npm run lint
npm run build
```

## License

MIT
