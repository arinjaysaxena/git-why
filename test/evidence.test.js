import test from 'node:test';
import assert from 'node:assert/strict';
import { makeRepo } from './helpers.js';
import { buildEvidence, renderRaw } from '../src/evidence.js';

const repo = () =>
  makeRepo([
    { message: 'Initial import', files: { 'src/a.js': 'const a = 1;\nconst b = 2;\nreturn a + b;\n' } },
    { message: 'Retry on timeout\n\nUpstream drops connections after 30s.', files: { 'src/a.js': 'const a = 1;\nconst b = retry(2);\nreturn a + b;\n' } },
  ]);

test('buildEvidence groups lines by commit and includes history', () => {
  const dir = repo();
  const ev = buildEvidence(dir, { file: 'src/a.js', start: 1, end: 2 });
  assert.equal(ev.commits.length, 2);
  assert.deepEqual(ev.commits.map((c) => c.subject).sort(), ['Initial import', 'Retry on timeout']);
  assert.equal(ev.commits.find((c) => c.subject === 'Retry on timeout').body, 'Upstream drops connections after 30s.');
  assert.equal(ev.lines.length, 2);
  assert.equal(ev.history.length, 2);
});

test('buildEvidence attaches provider context by sha', () => {
  const dir = repo();
  const first = buildEvidence(dir, { file: 'src/a.js', start: 2 });
  const sha = first.commits[0].sha;
  const pulls = [{ number: 9, title: 'Retry', body: 'Because of 30s drops', url: 'u', mergedAt: '2024-02-03T00:00:00Z',
    issues: [{ number: 3, title: 'Flaky uploads', state: 'closed', closes: true }],
    discussion: [{ kind: 'review-comment', author: 'bob', body: 'Keep it to one retry', path: 'src/a.js', line: 2 }] }];
  const ev = buildEvidence(dir, { file: 'src/a.js', start: 2 }, { context: { [sha]: { pulls } } });
  const text = renderRaw(ev);
  assert.match(text, /# src\/a\.js:2\n/);
  assert.match(text, /PR #9: Retry \(merged 2024-02-03\)/);
  assert.match(text, /Closes issue #3: Flaky uploads \[closed\]/);
  assert.match(text, /review-comment by bob on src\/a\.js:2/);
  assert.match(text, /> Keep it to one retry/);
  assert.match(text, /## History of these lines/);
});

test('renderRaw works with no provider data', () => {
  const dir = repo();
  const text = renderRaw(buildEvidence(dir, { file: 'src/a.js', start: 1, end: 3, fn: 'sum' }));
  assert.match(text, /# src\/a\.js:1-3 \(sum\)/);
  assert.match(text, /Retry on timeout/);
  assert.doesNotMatch(text, /PR #/);
});
