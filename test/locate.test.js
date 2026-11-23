import test from 'node:test';
import assert from 'node:assert/strict';
import { locateFunction, languageOf } from '../src/locate.js';

test('languageOf maps extensions', () => {
  assert.equal(languageOf('a.tsx'), 'js');
  assert.equal(languageOf('a.py'), 'python');
  assert.equal(languageOf('A.java'), 'java');
  assert.equal(languageOf('a.rb'), null);
});

const js = `// helper
export async function fetchUser(id) {
  if (!id) {
    return null;
  }
  return get('/u/' + id);
}

const parse = (s) => {
  return JSON.parse(s);
};

class Svc {
  async load(x) {
    return x;
  }
}
`;

test('locates JS function declarations, arrows and methods by brace matching', () => {
  assert.deepEqual(locateFunction('a.js', js, 'fetchUser'), [{ name: 'fetchUser', start: 2, end: 7 }]);
  assert.deepEqual(locateFunction('a.js', js, 'parse'), [{ name: 'parse', start: 9, end: 11 }]);
  assert.deepEqual(locateFunction('a.ts', js, 'load'), [{ name: 'load', start: 14, end: 16 }]);
  assert.deepEqual(locateFunction('a.js', js, 'nope'), []);
});

test('does not treat calls or control flow as definitions', () => {
  const src = 'function run() {\n  if (x) {\n    run();\n  }\n}\n';
  assert.equal(locateFunction('a.js', src, 'run').length, 1);
});

test('locates Python functions by indentation, including methods', () => {
  const py = 'def a():\n    return 1\n\nclass K:\n    def go(self, x):\n        y = x\n\n        return y\n\n    def other(self):\n        pass\n';
  assert.deepEqual(locateFunction('m.py', py, 'a'), [{ name: 'a', start: 1, end: 2 }]);
  assert.deepEqual(locateFunction('m.py', py, 'go'), [{ name: 'go', start: 5, end: 8 }]);
});

test('locates Java methods and reports overloads separately', () => {
  const java = `public class A {
  public static int add(int a, int b) {
    return a + b;
  }
  private String add(String a) {
    return a;
  }
}
`;
  assert.deepEqual(locateFunction('A.java', java, 'add'), [
    { name: 'add', start: 2, end: 4 },
    { name: 'add', start: 5, end: 7 },
  ]);
});

test('unsupported languages give a helpful error', () => {
  assert.throws(() => locateFunction('a.rb', '', 'x'), /not supported/);
});
