import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

function extractUserInputFieldSource() {
  const matched = webSessionPanelSource.match(
    /<n-input\s+v-if="question\.isOther \|\| question\.options\.length === 0"[\s\S]*?\/>/
  );
  return matched?.[0] ?? '';
}

function extractUserInputQuestionOpeningTag() {
  const matched = webSessionPanelSource.match(
    /<div\s+v-for="question in pendingUserInput\.questions"[\s\S]*?class="user-input-question"\s*>/
  );
  return matched?.[0] ?? '';
}

describe('web session user input IME protection', () => {
  it('creates a fresh answer card for each request', () => {
    expect(webSessionPanelSource).toMatch(
      /v-else-if="pendingUserInput && !inlinePlanChoice"[\s\S]*?:key="pendingUserInput\.itemId"/
    );
  });

  it('memoizes each question at the v-for boundary', () => {
    const questionOpeningTag = extractUserInputQuestionOpeningTag();
    const fieldSource = extractUserInputFieldSource();

    expect(questionOpeningTag).toContain('v-memo="userInputQuestionMemoDeps(question)"');
    expect(fieldSource).not.toContain('v-memo');
  });
});
