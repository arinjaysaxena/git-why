// Build the decision-history prompt from collected evidence.
import { renderRaw } from './evidence.js';

export const SYSTEM_PROMPT = [
  'You explain why a piece of code looks the way it does, using only the evidence provided:',
  'git blame, commit messages, pull requests, linked issues and review comments.',
  'Write a short decision history in Markdown with exactly these sections:',
  '## Why it looks like this',
  '## What was tried',
  '## Constraints that still apply',
  'Rules: cite commits as short shas and PRs/issues as #number. If the evidence does not say, write "Not stated in the history." Never invent reasons.',
].join('\n');

const MAX_EVIDENCE_CHARS = 24000;

export function buildMessages(evidence) {
  let text = renderRaw(evidence);
  if (text.length > MAX_EVIDENCE_CHARS) text = `${text.slice(0, MAX_EVIDENCE_CHARS)}\n[evidence truncated]\n`;
  return [
    { role: 'system', content: SYSTEM_PROMPT },
    { role: 'user', content: `Evidence:\n\n${text}\nWrite the decision history.` },
  ];
}

/** Ask the model for the decision history of `evidence`. */
export async function explain(llm, evidence) {
  return llm.complete(buildMessages(evidence));
}
