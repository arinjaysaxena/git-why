// Function locator for `--fn`: finds a function's line range by regex for JS/TS, Python and Java.
const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

export function languageOf(file) {
  if (/\.(?:[mc]?[jt]sx?)$/.test(file)) return 'js';
  if (/\.py$/.test(file)) return 'python';
  if (/\.java$/.test(file)) return 'java';
  return null;
}

const jsDefs = (n) => [
  new RegExp(`^\\s*(?:export\\s+)?(?:default\\s+)?(?:async\\s+)?function\\s*\\*?\\s*${n}\\s*[<(]`),
  new RegExp(`^\\s*(?:export\\s+)?(?:const|let|var)\\s+${n}\\s*(?::[^=]+)?=\\s*(?:async\\s*)?(?:\\([^)]*\\)|\\w+)\\s*(?::[^=]+)?=>`),
  new RegExp(`^\\s*(?:export\\s+)?(?:const|let|var)\\s+${n}\\s*=\\s*(?:async\\s+)?function`),
  new RegExp(`^\\s*(?:(?:public|private|protected|static|async|readonly|get|set)\\s+)*\\*?${n}\\s*\\([^)]*\\)\\s*(?::\\s*[^{]+)?\\{`),
];
const pyDef = (n) => new RegExp(`^(\\s*)(?:async\\s+)?def\\s+${n}\\s*\\(`);
const javaDef = (n) =>
  new RegExp(`^\\s*(?:(?:public|private|protected|static|final|abstract|synchronized|native)\\s+)*[\\w<>\\[\\],.? ]+\\s+${n}\\s*\\([^;]*$`);

/** End line of a brace-delimited block starting on `start` (0-based index into lines). */
function braceEnd(lines, start) {
  let depth = 0;
  let seen = false;
  for (let i = start; i < lines.length; i++) {
    const code = lines[i].replace(/(["'`])(?:\\.|(?!\1).)*\1/g, '').replace(/\/\/.*$/, '');
    for (const ch of code) {
      if (ch === '{') {
        depth++;
        seen = true;
      } else if (ch === '}') depth--;
    }
    if (seen && depth <= 0) return i;
  }
  return lines.length - 1;
}

function pythonEnd(lines, start, indent) {
  let last = start;
  for (let i = start + 1; i < lines.length; i++) {
    if (!lines[i].trim()) continue;
    const ind = /^\s*/.exec(lines[i])[0].length;
    if (ind <= indent) break;
    last = i;
  }
  return last;
}

/**
 * Find function `name` in `source`. Returns [{ name, start, end }] (1-based, inclusive),
 * one per definition found, so callers can detect ambiguity.
 */
export function locateFunction(file, source, name) {
  const lang = languageOf(file);
  if (!lang) throw new Error(`--fn is not supported for ${file}; use <file>:<line> instead.`);
  const n = esc(name);
  const lines = source.split('\n');
  const found = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (/^\s*(?:\/\/|#|\*|\/\*)/.test(line)) continue;
    if (lang === 'python') {
      const m = pyDef(n).exec(line);
      if (m) found.push({ name, start: i + 1, end: pythonEnd(lines, i, m[1].length) + 1 });
    } else if (lang === 'java') {
      if (javaDef(n).test(line) && !/^\s*(?:return|new|else|throw)\b/.test(line)) found.push({ name, start: i + 1, end: braceEnd(lines, i) + 1 });
    } else if (jsDefs(n).some((re) => re.test(line)) && !/^\s*(?:if|for|while|switch|catch|return)\b/.test(line)) {
      found.push({ name, start: i + 1, end: braceEnd(lines, i) + 1 });
    }
  }
  return found;
}
