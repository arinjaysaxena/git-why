// Provider-neutral LLM client: any OpenAI-compatible chat-completions endpoint, Ollama by default.
export const DEFAULTS = {
  baseUrl: 'http://localhost:11434/v1',
  model: 'llama3.1',
  apiKey: 'ollama',
};

export function loadLlmConfig(env = process.env, overrides = {}) {
  return {
    baseUrl: (overrides.baseUrl ?? env.GITWHY_BASE_URL ?? DEFAULTS.baseUrl).replace(/\/+$/, ''),
    model: overrides.model ?? env.GITWHY_MODEL ?? DEFAULTS.model,
    apiKey: overrides.apiKey ?? env.GITWHY_API_KEY ?? DEFAULTS.apiKey,
  };
}

export class LlmError extends Error {}

/** Create a client with `complete(messages) -> string`. `fetchImpl` is injectable for tests. */
export function createLlm(config, { fetchImpl = globalThis.fetch } = {}) {
  return {
    async complete(messages) {
      let res;
      try {
        res = await fetchImpl(`${config.baseUrl}/chat/completions`, {
          method: 'POST',
          headers: { 'content-type': 'application/json', authorization: `Bearer ${config.apiKey}` },
          body: JSON.stringify({ model: config.model, messages, temperature: 0.2 }),
        });
      } catch (e) {
        throw new LlmError(`Cannot reach the model at ${config.baseUrl} (${e.message}). Is it running? Use --raw to skip the model.`);
      }
      if (!res.ok) throw new LlmError(`Model request failed (${res.status}) at ${config.baseUrl}`);
      const data = await res.json();
      const text = data?.choices?.[0]?.message?.content;
      if (typeof text !== 'string' || !text.trim()) throw new LlmError('Model returned an empty answer.');
      return text.trim();
    },
  };
}
