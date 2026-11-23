// On-disk cache keyed by repo + sha so repeated runs make no network calls.
import { mkdirSync, readFileSync, writeFileSync, existsSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

export const CACHE_VERSION = 1;

export function defaultCacheDir(env = process.env) {
  return env.GIT_WHY_CACHE_DIR || join(env.XDG_CACHE_HOME || join(homedir(), '.cache'), 'git-why');
}

const safe = (s) => String(s).replace(/[^\w.-]/g, '_');

export function createCache(dir, { enabled = true } = {}) {
  const file = (repo, sha) => join(dir, safe(repo.owner), safe(repo.repo), `${safe(sha)}.json`);
  return {
    get(repo, sha) {
      if (!enabled) return null;
      const f = file(repo, sha);
      if (!existsSync(f)) return null;
      try {
        const data = JSON.parse(readFileSync(f, 'utf8'));
        return data.version === CACHE_VERSION ? data.value : null;
      } catch {
        return null; // corrupt entry: treat as a miss
      }
    },
    set(repo, sha, value) {
      if (!enabled) return;
      const f = file(repo, sha);
      mkdirSync(join(f, '..'), { recursive: true });
      writeFileSync(f, JSON.stringify({ version: CACHE_VERSION, value }));
    },
  };
}

/** Return cached context for sha or compute it with `load()` and store it. */
export async function withCache(cache, repo, sha, load) {
  const hit = cache.get(repo, sha);
  if (hit) return { value: hit, cached: true };
  const value = await load();
  cache.set(repo, sha, value);
  return { value, cached: false };
}
