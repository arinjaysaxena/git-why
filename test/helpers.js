import { execFileSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';

/** Create a temp git repo; `commits` is [{ message, files: {path: content}, author? }]. */
export function makeRepo(commits) {
  const dir = mkdtempSync(join(tmpdir(), 'gw-'));
  const run = (...a) => execFileSync('git', a, { cwd: dir, stdio: 'pipe' });
  run('init', '-q', '-b', 'main');
  run('config', 'user.name', 'Test Dev');
  run('config', 'user.email', 'dev@example.com');
  run('config', 'commit.gpgsign', 'false');
  for (const c of commits) {
    for (const [p, content] of Object.entries(c.files)) {
      mkdirSync(dirname(join(dir, p)), { recursive: true });
      writeFileSync(join(dir, p), content);
    }
    run('add', '-A');
    run('commit', '-q', '-m', c.message);
  }
  return dir;
}
