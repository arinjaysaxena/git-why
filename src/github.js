// GitHub provider: commit -> pull requests -> PR body -> linked issues. `fetchImpl` is injectable.
const API = 'https://api.github.com';

export class GitHubError extends Error {
  constructor(message, status) {
    super(message);
    this.name = 'GitHubError';
    this.status = status;
  }
}

export function createGitHub({ token, fetchImpl = globalThis.fetch, baseUrl = API } = {}) {
  async function get(path) {
    const headers = { accept: 'application/vnd.github+json', 'x-github-api-version': '2022-11-28', 'user-agent': 'git-why' };
    if (token) headers.authorization = `Bearer ${token}`;
    let res;
    try {
      res = await fetchImpl(`${baseUrl}${path}`, { headers });
    } catch (e) {
      throw new GitHubError(`Cannot reach GitHub (${e.message}).`, 0);
    }
    if (res.status === 404) return null;
    if (!res.ok) {
      const remaining = res.headers?.get?.('x-ratelimit-remaining');
      if ((res.status === 403 && remaining === '0') || res.status === 429) {
        const reset = Number(res.headers?.get?.('x-ratelimit-reset'));
        const when = reset ? ` Resets at ${new Date(reset * 1000).toISOString()}.` : '';
        const hint = token ? 'wait for the limit to reset' : 'set GITHUB_TOKEN to raise it';
        throw new GitHubError(`GitHub rate limit reached; ${hint}.${when}`, res.status);
      }
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

const MAX_DISCUSSION = 40;

/**
 * Review comments, review summaries and conversation comments of a PR, oldest first.
 * Returns [{ kind: 'review-comment'|'review'|'comment', author, body, path?, line?, createdAt, url }].
 * Empty reviews (approvals with no text) are dropped; bodies are truncated.
 */
export async function collectDiscussion(gh, { owner, repo }, number) {
  const [inline, reviews, convo] = await Promise.all([
    gh.get(`/repos/${owner}/${repo}/pulls/${number}/comments`),
    gh.get(`/repos/${owner}/${repo}/pulls/${number}/reviews`),
    gh.get(`/repos/${owner}/${repo}/issues/${number}/comments`),
  ]);
  const items = [
    ...(inline ?? []).map((c) => ({ kind: 'review-comment', author: c.user?.login, body: c.body, path: c.path, line: c.line ?? c.original_line ?? null, createdAt: c.created_at, url: c.html_url })),
    ...(reviews ?? []).filter((r) => r.body?.trim()).map((r) => ({ kind: 'review', author: r.user?.login, body: r.body, state: r.state, createdAt: r.submitted_at, url: r.html_url })),
    ...(convo ?? []).map((c) => ({ kind: 'comment', author: c.user?.login, body: c.body, createdAt: c.created_at, url: c.html_url })),
  ];
  return items
    .filter((i) => i.body?.trim() && !/\[bot\]$/.test(i.author ?? ''))
    .sort((a, b) => String(a.createdAt).localeCompare(String(b.createdAt)))
    .slice(0, MAX_DISCUSSION)
    .map((i) => ({ ...i, body: trim(i.body, 1500) }));
}

/**
 * Collect PR and issue context for a commit sha.
 * Returns { pulls: [{ number, title, body, url, author, mergedAt, issues: [...], discussion: [...] }] }.
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
    const discussion = await collectDiscussion(gh, { owner, repo }, full.number);
    pulls.push({ number: full.number, title: full.title, body: trim(full.body), url: full.html_url, author: full.user?.login, mergedAt: full.merged_at ?? null, issues, discussion });
  }
  return { pulls };
}
