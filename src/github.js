// GitHub provider: commit -> pull requests -> PR body -> linked issues. `fetchImpl` is injectable.
const API = 'https://api.github.com';

export class GitHubError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

export function createGitHub({ token, fetchImpl = globalThis.fetch, baseUrl = API } = {}) {
  async function get(path) {
    const headers = { accept: 'application/vnd.github+json', 'x-github-api-version': '2022-11-28', 'user-agent': 'git-why' };
    if (token) headers.authorization = `Bearer ${token}`;
    const res = await fetchImpl(`${baseUrl}${path}`, { headers });
    if (res.status === 404) return null;
    if (!res.ok) {
      const remaining = res.headers?.get?.('x-ratelimit-remaining');
      if (res.status === 403 && remaining === '0') throw new GitHubError('GitHub rate limit reached; set GITHUB_TOKEN to raise it.', 403);
      if (res.status === 401) throw new GitHubError('GitHub rejected the token (401); check GITHUB_TOKEN.', 401);
      throw new GitHubError(`GitHub request failed (${res.status}) for ${path}`, res.status);
    }
    return res.json();
  }
  return { get };
}

/**
 * Find issue references that a PR/commit text closes or mentions.
 * Returns unique [{ owner, repo, number, closes }] where owner/repo default to `ctx`.
 */
export function parseLinkedIssues(text, ctx) {
  const out = new Map();
  const re = /(?:\b(close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+)?(?:([\w.-]+)\/([\w.-]+))?#(\d+)\b/gi;
  for (const m of (text ?? '').matchAll(re)) {
    const owner = m[2] ?? ctx.owner;
    const repo = m[3] ?? ctx.repo;
    const key = `${owner}/${repo}#${m[4]}`;
    const prev = out.get(key);
    out.set(key, { owner, repo, number: Number(m[4]), closes: Boolean(m[1]) || Boolean(prev?.closes) });
  }
  return [...out.values()];
}

const trim = (s, n = 4000) => (s && s.length > n ? `${s.slice(0, n)}\n[truncated]` : (s ?? ''));

/**
 * Collect PR and issue context for a commit sha.
 * Returns { pulls: [{ number, title, body, url, author, mergedAt, issues: [...] }] }.
 */
export async function collectCommitContext(gh, { owner, repo }, sha) {
  const found = (await gh.get(`/repos/${owner}/${repo}/commits/${sha}/pulls`)) ?? [];
  const pulls = [];
  for (const p of found) {
    const full = (await gh.get(`/repos/${owner}/${repo}/pulls/${p.number}`)) ?? p;
    const refs = parseLinkedIssues(`${full.title ?? ''}\n${full.body ?? ''}`, { owner, repo }).filter((r) => r.number !== full.number);
    const issues = [];
    for (const r of refs) {
      const issue = await gh.get(`/repos/${r.owner}/${r.repo}/issues/${r.number}`);
      if (!issue || issue.pull_request) continue; // references to other PRs are not issues
      issues.push({ number: r.number, repo: `${r.owner}/${r.repo}`, title: issue.title, body: trim(issue.body, 2000), url: issue.html_url, state: issue.state, closes: r.closes });
    }
    pulls.push({ number: full.number, title: full.title, body: trim(full.body), url: full.html_url, author: full.user?.login, mergedAt: full.merged_at ?? null, issues });
  }
  return { pulls };
}
