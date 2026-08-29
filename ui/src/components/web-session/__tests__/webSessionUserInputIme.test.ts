import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const webSessionUserInputQuestionPath = fileURLToPath(
  new URL('../WebSessionUserInputQuestion.vue', import.meta.url)
);
const webSessionUserInputQuestionSource = readFileSync(webSessionUserInputQuestionPath, 'utf8');

function extractUserInputFieldSource() {
  const matched = webSessionUserInputQuestionSource.match(
    /<n-input\s+v-if="question\.isOther \|\| question\.options\.length === 0"[\s\S]*?\/>/
  );
  return matched?.[0] ?? '';
}

function extractUserInputQuestionComponent() {
  const matched = webSessionPanelSource.match(
    /<WebSessionUserInputQuestionField\s+v-for="question in pendingUserInput\.questions"[\s\S]*?\/>/
  );
  return matched?.[0] ?? '';
}

describe('web session user input IME protection', () => {
  it('creates a fresh answer card for each request', () => {
    expect(webSessionPanelSource).toMatch(
      /v-else-if="pendingUserInput && !inlinePlanChoice"[\s\S]*?:key="pendingUserInput\.itemId"/
    );
  });

  it('keeps interactive controls out of memoized subtrees', () => {
    const questionComponent = extractUserInputQuestionComponent();
    const fieldSource = extractUserInputFieldSource();

    expect(questionComponent).toContain(':key="`${pendingUserInputSyncKey}:${question.id}`"');
    expect(questionComponent).not.toContain('v-memo');
    expect(fieldSource).not.toContain('v-memo');
    expect(fieldSource).toContain(':value="draft"');
    expect(fieldSource).toContain('@update:value="emit(\'update:draft\', $event)"');
  });
});
