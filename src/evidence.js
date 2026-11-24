// Evidence model: everything collected about a target, plus the offline `--raw` text renderer.
import { blameRange, commitInfo, historyChain } from './git.js';

/**
 * Build evidence for lines [start, end] of `file`.
 * `context` is optional provider output ({ pulls }) for the blamed commit(s), keyed by sha.
 * Returns { target, lines, commits: [{ sha, subject, body, author, date, lines, pulls }], history }.
 */
export function buildEvidence(cwd, { file, start, end = start, fn = null }, { rev = 'HEAD', context = {}, maxHistory = 10 } = {}) {
  const blame = blameRange(cwd, file, start, end, rev);
  if (!blame.length) throw new Error(`No blame data for ${file}:${start}`);
  const bySha = new Map();
  for (const b of blame) {
    if (!bySha.has(b.sha)) bySha.set(b.sha, []);
    bySha.get(b.sha).push(b.line);
  }
  const commits = [...bySha].map(([sha, lines]) => {
    const info = commitInfo(cwd, sha);
    return { sha, subject: info.subject, body: info.body, author: info.author, date: info.date, lines, pulls: context[sha]?.pulls ?? [] };
  });
  const history = historyChain(cwd, file, start, end, { rev, max: maxHistory });
  return {
    target: { file, start, end, fn },
    lines: blame.map((b) => ({ line: b.line, sha: b.sha, content: b.content })),
    commits,
    history,
  };
}

const short = (sha) => sha.slice(0, 7);
const day = (iso) => String(iso).slice(0, 10);
const indent = (text, pad = '    ') => text.split('\n').map((l) => (l ? pad + l : l)).join('\n');

/** Render evidence as plain text, no model involved. */
export function renderRaw(ev) {
  const { file, start, end, fn } = ev.target;
  const out = [];
  out.push(`# ${file}:${start === end ? start : `${start}-${end}`}${fn ? ` (${fn})` : ''}`, '');
  out.push('## Code');
  for (const l of ev.lines) out.push(`${String(l.line).padStart(5)} ${short(l.sha)}  ${l.content}`);
  out.push('', '## Commits that last changed these lines');
  for (const c of ev.commits) {
    out.push(`- ${short(c.sha)} ${day(c.date)} ${c.author}: ${c.subject} (lines ${c.lines.join(', ')})`);
    if (c.body) out.push(indent(c.body));
    for (const p of c.pulls) {
      out.push(`  PR #${p.number}: ${p.title}${p.mergedAt ? ` (merged ${day(p.mergedAt)})` : ''} ${p.url ?? ''}`.trimEnd());
      if (p.body) out.push(indent(p.body, '    | '));
      for (const i of p.issues ?? []) out.push(`  ${i.closes ? 'Closes' : 'Mentions'} issue #${i.number}: ${i.title}${i.state ? ` [${i.state}]` : ''}`);
      for (const d of p.discussion ?? []) {
        const where = d.path ? ` on ${d.path}${d.line ? `:${d.line}` : ''}` : '';
        out.push(`  ${d.kind} by ${d.author}${where}:`, indent(d.body, '    > '));
      }
    }
  }
  if (ev.history.length > 1) {
    out.push('', '## History of these lines (newest first)');
    for (const h of ev.history) out.push(`- ${short(h.sha)} ${day(h.date)} ${h.subject}`);
  }
  return `${out.join('\n')}\n`;
}
