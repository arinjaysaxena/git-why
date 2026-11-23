// Remote detection: turn a git remote URL into { host, owner, repo }.
import { git } from './git.js';

/** Parse https, ssh:// and scp-style (git@host:owner/repo) remote URLs. Returns null if unrecognised. */
export function parseRemoteUrl(url) {
  const u = url.trim();
  const m =
    /^(?:https?|ssh|git):\/\/(?:[^@/]+@)?([^/:]+)(?::\d+)?\/([^/]+)\/([^/]+?)(?:\.git)?\/?$/.exec(u) ||
    /^(?:[^@/]+@)?([^:/]+):([^/]+)\/([^/]+?)(?:\.git)?\/?$/.exec(u);
  if (!m) return null;
  return { host: m[1].toLowerCase(), owner: m[2], repo: m[3] };
}

/** Detect the GitHub repo from `origin` (or the first remote). Throws a helpful error otherwise. */
export function detectRemote(cwd, name = 'origin') {
  let url;
  try {
    url = git(cwd, ['remote', 'get-url', name]).trim();
  } catch {
    const first = git(cwd, ['remote']).split('\n').filter(Boolean)[0];
    if (!first) throw new Error('No git remote configured; use --raw for local-only evidence.');
    url = git(cwd, ['remote', 'get-url', first]).trim();
  }
  const parsed = parseRemoteUrl(url);
  if (!parsed) throw new Error(`Could not parse remote URL "${url}".`);
  if (parsed.host !== 'github.com') throw new Error(`Unsupported host "${parsed.host}": only GitHub is supported in v0.1.`);
  return parsed;
}
