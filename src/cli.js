// CLI: git-why <file>:<line> | <file> --fn <name> [--raw] [--no-cache] [--model m] [--base-url url]
import { readFileSync } from 'node:fs';
import { resolve, relative } from 'node:path';
import { parseTarget } from './blame.js';
import { repoRoot } from './git.js';
import { locateFunction } from './locate.js';
import { buildEvidence, renderRaw } from './evidence.js';
import { detectRemote } from './remote.js';
import { createGitHub, collectCommitContext, GitHubError } from './github.js';
import { createCache, defaultCacheDir, withCache } from './cache.js';
import { createLlm, loadLlmConfig } from './llm.js';
import { explain } from './prompt.js';

export const USAGE = 'usage: git-why <file>:<line> | <file> --fn <name> [--raw] [--no-cache] [--model m] [--base-url url]';

export function parseArgs(argv) {
  const o = { target: null, fn: null, raw: false, cache: true, model: undefined, baseUrl: undefined };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const val = () => {
      if (i + 1 >= argv.length) throw new Error(`${a} needs a value`);
      return argv[++i];
    };
    if (a === '--fn') o.fn = val();
    else if (a === '--raw') o.raw = true;
    else if (a === '--no-cache') o.cache = false;
    else if (a === '--model') o.model = val();
    else if (a === '--base-url') o.baseUrl = val();
    else if (a.startsWith('-')) throw new Error(`unknown option ${a}`);
    else if (!o.target) o.target = a;
    else throw new Error(`unexpected argument ${a}`);
  }
  if (!o.target) throw new Error('missing <file>:<line> or <file> --fn <name>');
  return o;
}

/** Resolve the target to { file (repo-relative), start, end, fn }. */
export function resolveTarget(opts, cwd, root) {
  let file;
  let start;
  let end;
  if (opts.fn) {
    file = opts.target;
  } else {
    ({ file, line: start } = parseTarget(opts.target));
    end = start;
  }
  file = relative(root, resolve(cwd, file));
  if (opts.fn) {
    const found = locateFunction(file, readFileSync(resolve(root, file), 'utf8'), opts.fn);
    if (!found.length) throw new Error(`Function "${opts.fn}" not found in ${file}.`);
    if (found.length > 1) throw new Error(`"${opts.fn}" is defined ${found.length} times in ${file} (lines ${found.map((f) => f.start).join(', ')}); use <file>:<line>.`);
    ({ start, end } = found[0]);
  }
  return { file, start, end, fn: opts.fn };
}

/**
 * Run the pipeline and return the text to print.
 * Injectables: `fetchImpl` (GitHub and LLM), `env`, `llm`.
 */
export async function run(opts, { cwd = process.cwd(), env = process.env, fetchImpl, llm, log = () => {} } = {}) {
  const root = repoRoot(cwd);
  const target = resolveTarget(opts, cwd, root);
  let context = {};
  if (!opts.raw) {
    context = await collectContext(root, target, { env, fetchImpl, cache: opts.cache, log });
  }
  const evidence = buildEvidence(root, target, { context });
  if (opts.raw) return renderRaw(evidence);
  const client = llm ?? createLlm(loadLlmConfig(env, { model: opts.model, baseUrl: opts.baseUrl }), { fetchImpl });
  const answer = await explain(client, evidence);
  return `# ${target.file}:${target.start}${target.end > target.start ? `-${target.end}` : ''}${target.fn ? ` (${target.fn})` : ''}\n\n${answer}\n`;
}

async function collectContext(root, target, { env, fetchImpl, cache, log }) {
  let remote;
  try {
    remote = detectRemote(root);
  } catch (e) {
    log(`note: no GitHub context (${e.message})`);
    return {};
  }
  if (!env.GITHUB_TOKEN) log('note: GITHUB_TOKEN not set; GitHub requests are unauthenticated and rate-limited.');
  const gh = createGitHub({ token: env.GITHUB_TOKEN, fetchImpl });
  const store = createCache(defaultCacheDir(env), { enabled: cache });
  // Blame once to learn which commits matter, then fetch context per commit.
  const shas = [...new Set(buildEvidence(root, target).commits.map((c) => c.sha))];
  const context = {};
  for (const sha of shas) {
    let result;
    try {
      result = await withCache(store, remote, sha, () => collectCommitContext(gh, remote, sha));
    } catch (e) {
      if (!(e instanceof GitHubError)) throw e;
      // Degrade to what git alone knows rather than failing the whole run; failures are never cached.
      log(`warning: ${e.message} Continuing with local git history only.`);
      break;
    }
    const { value, cached } = result;
    if (cached) log(`cache hit ${sha.slice(0, 7)}`);
    context[sha] = value;
  }
  return context;
}

export async function main(argv, { log = console.log, ...deps } = {}) {
  let opts;
  try {
    opts = parseArgs(argv);
  } catch (e) {
    log(`${e.message}\n${USAGE}`);
    return 2;
  }
  try {
    log((await run(opts, { ...deps, log: (m) => console.error(m) })).trimEnd());
    return 0;
  } catch (e) {
    log(`error: ${e.message}`);
    return 1;
  }
}
