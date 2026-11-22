// Parse `git blame --porcelain` output and `file:line` targets.

/** Parse "src/a.js:42" (also accepts a Windows-free path containing colons only before the last one). */
export function parseTarget(spec) {
  const m = /^(.+):(\d+)$/.exec(spec);
  if (!m) throw new Error(`Expected <file>:<line>, got "${spec}"`);
  const line = Number(m[2]);
  if (line < 1) throw new Error('Line numbers start at 1.');
  return { file: m[1], line };
}

/**
 * Parse porcelain blame into entries: { line, sha, author, authorTime, summary, previous, boundary }.
 * Header lines repeat the sha for each line; metadata appears only on the first occurrence.
 */
export function parseBlamePorcelain(text) {
  const meta = new Map();
  const lines = [];
  let current = null;
  for (const raw of text.split('\n')) {
    if (raw.startsWith('\t')) {
      if (current) lines.push({ ...current, content: raw.slice(1) });
      current = null;
      continue;
    }
    const head = /^([0-9a-f]{40}) \d+ (\d+)(?: \d+)?$/.exec(raw);
    if (head) {
      current = { sha: head[1], line: Number(head[2]) };
      if (!meta.has(head[1])) meta.set(head[1], { sha: head[1] });
      continue;
    }
    if (!current) continue;
    const m = meta.get(current.sha);
    const sp = raw.indexOf(' ');
    const key = sp === -1 ? raw : raw.slice(0, sp);
    const val = sp === -1 ? '' : raw.slice(sp + 1);
    if (key === 'author') m.author = val;
    else if (key === 'author-mail') m.email = val.replace(/^<|>$/g, '');
    else if (key === 'author-time') m.authorTime = Number(val);
    else if (key === 'summary') m.summary = val;
    else if (key === 'previous') m.previous = val.split(' ')[0];
    else if (key === 'boundary') m.boundary = true;
  }
  return lines.map((l) => ({ ...meta.get(l.sha), line: l.line, content: l.content }));
}
