import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { makeRepo } from './helpers.js';
import { parseArgs, main, run } from '../src/cli.js';

const SRC = 'export function retry(fn) {\n  return fn();\n}\n\nexport function other() {\n  return 1;\n}\n';
const repo = (remote) => {
  const dir = makeRepo([{ message: 'Add retry helper to survive flaky API', files: { 'src/a.js': SRC } }]);
  if (remote) execFileSync('git', ['remote', 'add', 'origin', 'https://github.com/acme/widgets.git'], { cwd: dir });
  return dir;
};
const out = () => { const lines = []; return { lines, log: (m) => lines.push(m) }; };

test('parseArgs handles target, flags and errors', () => {
  assert.deepEqual(parseArgs(['a.js:3', '--raw', '--no-cache']), { target: 'a.js:3', fn: null, raw: true, cache: false, model: undefined, baseUrl: undefined });
  assert.equal(parseArgs(['a.js', '--fn', 'x']).fn, 'x');
  assert.throws(() => parseArgs([]), /missing/);
  assert.throws(() => parseArgs(['a', '--bogus']), /unknown option/);
  assert.throws(() => parseArgs(['a', '--fn']), /needs a value/);
});

test('--raw prints evidence offline and never calls fetch', async () => {
  const dir = repo(true);
  const fetchImpl = async () => { throw new Error('network used'); };
  const text = await run({ target: 'src/a.js:2', raw: true, cache: true }, { cwd: dir, fetchImpl });
  assert.match(text, /Add retry helper/);
  assert.match(text, /src\/a\.js:2/);
});

test('--fn resolves the function range', async () => {
  const dir = repo(false);
  const text = await run({ target: 'src/a.js', fn: 'retry', raw: true, cache: true }, { cwd: dir });
  assert.match(text, /src\/a\.js:1-3 \(retry\)/);
});

test('--fn reports missing functions and bad targets via exit code 1', async () => {
  const dir = repo(false);
  const o = out();
  const code = await main(['src/a.js', '--fn', 'nope', '--raw'], { cwd: dir, log: o.log });
  assert.equal(code, 1);
  assert.match(o.lines.join('\n'), /not found/);
  const o2 = out();
  assert.equal(await main([], { log: o2.log }), 2);
});

test('full run: GitHub context from mocked fetch, cached, then model answer', async () => {
  const dir = repo(true);
  const cacheDir = mkdtempSync(join(tmpdir(), 'gwc-'));
  const env = { GIT_WHY_CACHE_DIR: cacheDir, GITHUB_TOKEN: 't' };
  let calls = 0;
  const fetchImpl = async (url) => {
    calls++;
    const body = url.endsWith('/pulls') ? [{ number: 5 }]
      : url.endsWith('/pulls/5') ? { number: 5, title: 'Retry flaky API', body: 'Fixes #2', user: { login: 'dev' }, merged_at: '2026-01-02T00:00:00Z', html_url: 'u' }
      : url.endsWith('/issues/2') ? { title: 'API times out', state: 'closed', body: 'b', html_url: 'i' }
      : [];
    return { ok: true, status: 200, json: async () => body, headers: { get: () => null } };
  };
  let prompt;
  const llm = { complete: async (m) => { prompt = m[1].content; return '## Why it looks like this\nBecause of #2.'; } };
  const text = await run({ target: 'src/a.js:1', raw: false, cache: true }, { cwd: dir, env, fetchImpl, llm });
  assert.match(prompt, /PR #5: Retry flaky API/);
  assert.match(prompt, /issue #2: API times out/);
  assert.match(text, /Because of #2/);
  const first = calls;
  assert.ok(first > 0);
  await run({ target: 'src/a.js:1', raw: false, cache: true }, { cwd: dir, env, fetchImpl, llm });
  assert.equal(calls, first, 'second run served from cache');
  await run({ target: 'src/a.js:1', raw: false, cache: false }, { cwd: dir, env, fetchImpl, llm });
  assert.ok(calls > first, '--no-cache refetches');
});

test('model failure surfaces a helpful error', async () => {
  const dir = repo(false);
  const o = out();
  const fetchImpl = async () => { throw new Error('ECONNREFUSED'); };
  const code = await main(['src/a.js:1'], { cwd: dir, env: {}, fetchImpl, log: o.log });
  assert.equal(code, 1);
  assert.match(o.lines.join('\n'), /Cannot reach the model/);
});
