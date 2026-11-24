import test from 'node:test';
import assert from 'node:assert/strict';
import { createLlm, loadLlmConfig, LlmError } from '../src/llm.js';
import { buildMessages, explain, SYSTEM_PROMPT } from '../src/prompt.js';
import { buildEvidence } from '../src/evidence.js';
import { makeRepo } from './helpers.js';

const ok = (content) => ({ ok: true, status: 200, json: async () => ({ choices: [{ message: { content } }] }) });

test('loadLlmConfig defaults to local Ollama and honours env and overrides', () => {
  assert.equal(loadLlmConfig({}).baseUrl, 'http://localhost:11434/v1');
  const c = loadLlmConfig({ GITWHY_BASE_URL: 'http://h/v1/', GITWHY_MODEL: 'm1' }, { model: 'm2' });
  assert.deepEqual(c, { baseUrl: 'http://h/v1', model: 'm2', apiKey: 'ollama' });
});

test('complete posts chat-completions and returns trimmed text', async () => {
  let seen;
  const llm = createLlm(loadLlmConfig({}), { fetchImpl: async (url, init) => { seen = { url, init }; return ok('  hi \n'); } });
  assert.equal(await llm.complete([{ role: 'user', content: 'x' }]), 'hi');
  assert.equal(seen.url, 'http://localhost:11434/v1/chat/completions');
  assert.equal(JSON.parse(seen.init.body).model, 'llama3.1');
});

test('complete reports unreachable, HTTP and empty answers', async () => {
  const cfg = loadLlmConfig({});
  await assert.rejects(createLlm(cfg, { fetchImpl: async () => { throw new Error('ECONNREFUSED'); } }).complete([]), /Cannot reach the model.*--raw/);
  await assert.rejects(createLlm(cfg, { fetchImpl: async () => ({ ok: false, status: 500 }) }).complete([]), LlmError);
  await assert.rejects(createLlm(cfg, { fetchImpl: async () => ok('  ') }).complete([]), /empty/);
});

test('buildMessages embeds the evidence and explain returns the model answer', async () => {
  const dir = makeRepo([{ message: 'Cap retries at 3 to avoid hammering the API', files: { 'a.js': 'const retries = 3;\n' } }]);
  const ev = buildEvidence(dir, { file: 'a.js', start: 1 });
  const msgs = buildMessages(ev);
  assert.equal(msgs[0].content, SYSTEM_PROMPT);
  assert.match(msgs[1].content, /Cap retries at 3/);
  assert.match(msgs[1].content, /const retries = 3;/);
  assert.equal(await explain({ complete: async () => 'answer' }, ev), 'answer');
});
