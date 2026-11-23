import test from 'node:test';
import assert from 'node:assert/strict';
import { makeRepo } from './helpers.js';
import { execFileSync } from 'node:child_process';
import { parseRemoteUrl, detectRemote } from '../src/remote.js';
import { createGitHub, parseLinkedIssues, collectCommitContext } from '../src/github.js';

const res = (body, status = 200, headers = {}) => ({
  ok: status < 400, status, json: async () => body, headers: { get: (k) => headers[k.toLowerCase()] ?? null },
});

test('parseRemoteUrl handles https, ssh and scp styles', () => {
  const want = { host: 'github.com', owner: 'o', repo: 'r' };
  for (const u of ['https://github.com/o/r.git', 'https://github.com/o/r', 'git@github.com:o/r.git', 'ssh://git@github.com/o/r.git', 'https://tok@github.com/o/r/']) {
    assert.deepEqual(parseRemoteUrl(u), want, u);
  }
  assert.equal(parseRemoteUrl('not a url'), null);
});

test('detectRemote reads origin from a real repo and rejects other hosts', () => {
  const dir = makeRepo([{ message: 'init', files: { 'a.txt': 'x' } }]);
  assert.throws(() => detectRemote(dir), /No git remote/);
  execFileSync('git', ['remote', 'add', 'origin', 'git@github.com:acme/widgets.git'], { cwd: dir });
  assert.deepEqual(detectRemote(dir), { host: 'github.com', owner: 'acme', repo: 'widgets' });
  execFileSync('git', ['remote', 'set-url', 'origin', 'https://gitlab.com/acme/widgets.git'], { cwd: dir });
  assert.throws(() => detectRemote(dir), /Unsupported host/);
});

test('parseLinkedIssues finds closing keywords and cross-repo refs', () => {
  const refs = parseLinkedIssues('Fixes #12, see also #7 and closes other/lib#3. Fixes #7', { owner: 'o', repo: 'r' });
  const by = Object.fromEntries(refs.map((r) => [`${r.owner}/${r.repo}#${r.number}`, r.closes]));
  assert.deepEqual(by, { 'o/r#12': true, 'o/r#7': true, 'other/lib#3': true });
  assert.deepEqual(parseLinkedIssues('no refs here', { owner: 'o', repo: 'r' }), []);
});

test('collectCommitContext walks commit -> PR -> issues, skipping PR refs', async () => {
  const routes = {
    '/repos/o/r/commits/abc/pulls': [{ number: 5 }],
    '/repos/o/r/pulls/5': { number: 5, title: 'Retry on 429', body: 'Fixes #2. Related to #4', user: { login: 'dev' }, html_url: 'u5', merged_at: '2024-01-01T00:00:00Z' },
    '/repos/o/r/issues/2': { title: 'Rate limit crash', body: 'details', html_url: 'u2', state: 'closed' },
    '/repos/o/r/issues/4': { title: 'Other PR', pull_request: {}, html_url: 'u4' },
  };
  const calls = [];
  const gh = createGitHub({ token: 't', fetchImpl: async (url, init) => {
    const path = url.replace('https://api.github.com', '');
    calls.push([path, init.headers.authorization]);
    return path in routes ? res(routes[path]) : res(null, 404);
  } });
  const ctx = await collectCommitContext(gh, { owner: 'o', repo: 'r' }, 'abc');
  assert.equal(ctx.pulls.length, 1);
  assert.equal(ctx.pulls[0].author, 'dev');
  assert.deepEqual(ctx.pulls[0].issues.map((i) => [i.number, i.closes, i.title]), [[2, true, 'Rate limit crash']]);
  assert.equal(calls[0][1], 'Bearer t');
});

test('commit without PR yields empty context; errors are explained', async () => {
  const none = createGitHub({ fetchImpl: async () => res(null, 404) });
  assert.deepEqual(await collectCommitContext(none, { owner: 'o', repo: 'r' }, 'x'), { pulls: [] });
  const limited = createGitHub({ fetchImpl: async () => res({}, 403, { 'x-ratelimit-remaining': '0' }) });
  await assert.rejects(limited.get('/x'), /rate limit/);
  const unauth = createGitHub({ fetchImpl: async () => res({}, 401) });
  await assert.rejects(unauth.get('/x'), /GITHUB_TOKEN/);
  const boom = createGitHub({ fetchImpl: async () => res({}, 500) });
  await assert.rejects(boom.get('/x'), /500/);
});
