// "Build" step: usertwin ships as plain ESM, so verify every module parses and imports.
import { readdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const root = resolve(import.meta.dirname, '..', 'src');
let failed = 0;
for (const f of readdirSync(root).filter((n) => n.endsWith('.js'))) {
  try {
    await import(pathToFileURL(join(root, f)).href);
  } catch (e) {
    failed++;
    console.error(`FAIL ${f}: ${e.message}`);
  }
}
if (failed) process.exit(1);
console.log('all modules import cleanly');
