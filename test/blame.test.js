import test from 'node:test';
import assert from 'node:assert/strict';
import { parseBlamePorcelain, parseTarget } from '../src/blame.js';

const A = 'a'.repeat(40);
const B = 'b'.repeat(40);
const sample = [
  `${A} 1 1 2`, 'author Ann', 'author-mail <ann@example.com>', 'author-time 1700000000', 'author-tz +0000',
  'committer Ann', 'summary Add retry loop', `previous ${B} src/a.js`, 'filename src/a.js', '\tconst x = 1;',
  `${A} 2 2`, '\tconst y = 2;',
  `${B} 3 3 1`, 'author Bob', 'author-mail <bob@example.com>', 'author-time 1600000000', 'summary Initial import', 'boundary', 'filename src/a.js', '\treturn x + y;',
  '',
].join('\n');

test('parses target specs', () => {
  assert.deepEqual(parseTarget('src/a.js:42'), { file: 'src/a.js', line: 42 });
  assert.deepEqual(parseTarget('a:b/c.py:7'), { file: 'a:b/c.py', line: 7 });
  assert.throws(() => parseTarget('src/a.js'), /Expected/);
  assert.throws(() => parseTarget('src/a.js:0'), /start at 1/);
});

test('parses porcelain blame, sharing metadata across lines of one commit', () => {
  const rows = parseBlamePorcelain(sample);
  assert.equal(rows.length, 3);
  assert.equal(rows[0].sha, A);
  assert.equal(rows[0].author, 'Ann');
  assert.equal(rows[0].summary, 'Add retry loop');
  assert.equal(rows[0].previous, B);
  assert.equal(rows[1].author, 'Ann');
  assert.equal(rows[1].content, 'const y = 2;');
  assert.equal(rows[2].boundary, true);
  assert.equal(rows[2].email, 'bob@example.com');
  assert.equal(rows[2].line, 3);
});

test('empty input yields no rows', () => {
  assert.deepEqual(parseBlamePorcelain(''), []);
});
