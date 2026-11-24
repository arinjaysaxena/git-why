// Thin wrapper over the local git binary.
import { execFileSync } from 'node:child_process';
import { parseBlamePorcelain } from './blame.js';

export function git(cwd, args) {
  try {
    return execFileSync('git', args, { cwd, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'] });
  } catch (e) {
    const msg = (e.stderr ?? '').toString().trim() || e.message;
    throw new Error(`git ${args[0]} failed: ${msg}`);
  }
}

/** Absolute path of the repository root containing `cwd`. */
export const repoRoot = (cwd) => git(cwd, ['rev-parse', '--show-toplevel']).trim();

/** Blame a line range [start, end] of `file` at `rev` (default HEAD). Returns blame entries. */
export function blameRange(cwd, file, start, end = start, rev = 'HEAD') {
  const out = git(cwd, ['blame', '--porcelain', '-w', '-M', '-C', `-L${start},${end}`, rev, '--', file]);
  return parseBlamePorcelain(out);
}

/** Resolve the commit that last touched one line. */
export function blameLine(cwd, file, line, rev = 'HEAD') {
  const [entry] = blameRange(cwd, file, line, line, rev);
  if (!entry) throw new Error(`No blame data for ${file}:${line}`);
  return entry;
}

/** Full commit details: { sha, author, email, date (ISO), subject, body, parents }. */
export function commitInfo(cwd, sha) {
  const out = git(cwd, ['show', '-s', '--format=%H%x00%an%x00%ae%x00%aI%x00%s%x00%P%x00%b', sha]);
  const [full, author, email, date, subject, parents, ...body] = out.replace(/\n$/, '').split('\0');
  return { sha: full, author, email, date, subject, parents: parents ? parents.split(' ') : [], body: body.join('\0').trim() };
}

/** Number of lines in `file` at `rev`. */
export function lineCount(cwd, file, rev = 'HEAD') {
  const text = git(cwd, ['show', `${rev}:${file}`]);
  return text === '' ? 0 : text.replace(/\n$/, '').split('\n').length;
}

/**
 * History chain for a line range: every commit that touched lines [start, end] of `file`,
 * newest first, following renames and moved code (`git log -L`). Returns [{ sha, subject, date }].
 */
export function historyChain(cwd, file, start, end = start, { rev = 'HEAD', max = 10 } = {}) {
  const out = git(cwd, ['log', `-L${start},${end}:${file}`, '-s', `--max-count=${max}`, '--format=%H%x00%aI%x00%s', rev]);
  return out
    .split('\n')
    .filter((l) => l.includes('\0'))
    .map((l) => {
      const [sha, date, ...subject] = l.split('\0');
      return { sha, date, subject: subject.join('\0') };
    });
}
