import test from 'node:test';
import assert from 'node:assert/strict';
import { blameLine, blameRange, commitInfo, lineCount, repoRoot, git } from '../src/git.js';
import { makeRepo } from './helpers.js';

const repo = () =>
  makeRepo([
    { message: 'Initial import', files: { 'src/a.js': 'const a = 1;\nconst b = 2;\nreturn a + b;\n' } },
    { message: 'Retry on timeout\n\nThe upstream drops connections after 30s, so retry once.', files: { 'src/a.js': 'const a = 1;\nconst b = retry(2);\nreturn a + b;\n' } },
  ]);

test('blameLine resolves a line to the commit that last changed it', () => {
  const dir = repo();
  const changed = blameLine(dir, 'src/a.js', 2);
  assert.equal(changed.summary, 'Retry on timeout');
  assert.equal(changed.content, 'const b = retry(2);');
  const original = blameLine(dir, 'src/a.js', 1);
  assert.equal(original.summary, 'Initial import');
  assert.notEqual(original.sha, changed.sha);
});

test('blameRange returns one entry per line', () => {
  const dir = repo();
  const r = blameRange(dir, 'src/a.js', 1, 3);
  assert.deepEqual(r.map((e) => e.line), [1, 2, 3]);
});

test('commitInfo returns subject, body and parents', () => {
  const dir = repo();
  const { sha } = blameLine(dir, 'src/a.js', 2);
  const info = commitInfo(dir, sha);
  assert.equal(info.subject, 'Retry on timeout');
  assert.match(info.body, /drops connections/);
  assert.equal(info.parents.length, 1);
  assert.equal(info.author, 'Test Dev');
  assert.match(info.date, /^\d{4}-\d\d-\d\dT/);
});

test('lineCount and repoRoot work, and errors are readable', () => {
  const dir = repo();
  assert.equal(lineCount(dir, 'src/a.js'), 3);
  assert.ok(repoRoot(dir + '/src').endsWith(dir.split('/').pop()));
  assert.throws(() => blameLine(dir, 'missing.js', 1), /git blame failed/);
  assert.throws(() => git(dir, ['bogus-cmd']), /git bogus-cmd failed/);
});

test('historyChain lists every commit that touched a line, newest first', async () => {
  const { historyChain } = await import('../src/git.js');
  const dir = makeRepo([
    { message: 'Initial import', files: { 'src/a.js': 'const a = 1;\nconst b = 2;\nreturn a + b;\n' } },
    { message: 'Unrelated change', files: { 'src/b.js': 'x\n' } },
    { message: 'Retry on timeout', files: { 'src/a.js': 'const a = 1;\nconst b = retry(2);\nreturn a + b;\n' } },
    { message: 'Add backoff', files: { 'src/a.js': 'const a = 1;\nconst b = retry(2, backoff);\nreturn a + b;\n' } },
  ]);
  const chain = historyChain(dir, 'src/a.js', 2);
  assert.deepEqual(chain.map((c) => c.subject), ['Add backoff', 'Retry on timeout', 'Initial import']);
  assert.match(chain[0].date, /^\d{4}-\d\d-\d\dT/);
  assert.equal(historyChain(dir, 'src/a.js', 2, 2, { max: 2 }).length, 2);
  assert.throws(() => historyChain(dir, 'src/missing.js', 1), /git log failed/);
});
