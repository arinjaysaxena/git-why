import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createCache, withCache, defaultCacheDir } from '../src/cache.js';
import { collectDiscussion } from '../src/github.js';

const repo = { owner: 'o', repo: 'r' };

test('cache round-trips and withCache only loads once', async () => {
  const cache = createCache(mkdtempSync(join(tmpdir(), 'gwc-')));
  let n = 0;
  const load = async () => ({ n: ++n });
  assert.deepEqual(await withCache(cache, repo, 'abc', load), { value: { n: 1 }, cached: false });
  assert.deepEqual(await withCache(cache, repo, 'abc', load), { value: { n: 1 }, cached: true });
  assert.equal(n, 1);
});

test('disabled cache never reads or writes (--no-cache)', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'gwc-'));
  const cache = createCache(dir, { enabled: false });
  let n = 0;
  await withCache(cache, repo, 'abc', async () => ++n);
  const again = await withCache(cache, repo, 'abc', async () => ++n);
  assert.equal(again.cached, false);
  assert.equal(n, 2);
  assert.equal(createCache(dir).get(repo, 'abc'), null);
});

test('corrupt or wrong-version entries are misses; keys are sanitised', () => {
  const dir = mkdtempSync(join(tmpdir(), 'gwc-'));
  const cache = createCache(dir);
  cache.set({ owner: '../x', repo: 'r' }, 'abc', { ok: 1 });
  assert.deepEqual(cache.get({ owner: '../x', repo: 'r' }, 'abc'), { ok: 1 });
  cache.set(repo, 'bad', 1);
  writeFileSync(join(dir, 'o', 'r', 'bad.json'), '{nope');
  assert.equal(cache.get(repo, 'bad'), null);
  writeFileSync(join(dir, 'o', 'r', 'old.json'), JSON.stringify({ version: 0, value: 1 }));
  assert.equal(cache.get(repo, 'old'), null);
});

test('defaultCacheDir honours env overrides', () => {
  assert.equal(defaultCacheDir({ GIT_WHY_CACHE_DIR: '/c' }), '/c');
  assert.equal(defaultCacheDir({ XDG_CACHE_HOME: '/x' }), '/x/git-why');
});

test('collectDiscussion merges, orders, and drops empty and bot items', async () => {
  const routes = {
    '/repos/o/r/pulls/9/comments': [{ user: { login: 'rev' }, body: 'Why not a map?', path: 'a.js', line: 3, created_at: '2024-01-02', html_url: 'c1' }],
    '/repos/o/r/pulls/9/reviews': [
      { user: { login: 'rev' }, body: 'Looks good with retries', state: 'APPROVED', submitted_at: '2024-01-03', html_url: 'r1' },
      { user: { login: 'rev2' }, body: '', state: 'APPROVED', submitted_at: '2024-01-03' },
    ],
    '/repos/o/r/issues/9/comments': [
      { user: { login: 'ci[bot]' }, body: 'coverage 90%', created_at: '2024-01-01' },
      { user: { login: 'dev' }, body: 'Because of the 429s', created_at: '2024-01-04', html_url: 'k1' },
    ],
  };
  const gh = { get: async (p) => routes[p] ?? null };
  const d = await collectDiscussion(gh, repo, 9);
  assert.deepEqual(d.map((x) => x.kind), ['review-comment', 'review', 'comment']);
  assert.equal(d[0].path, 'a.js');
  assert.equal(d[2].author, 'dev');
});
