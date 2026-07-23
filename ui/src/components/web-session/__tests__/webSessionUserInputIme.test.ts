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

describe('web session user input IME protection', () => {
  it('creates a fresh answer card for each request', () => {
    expect(webSessionPanelSource).toMatch(
      /v-else-if="pendingUserInput && !inlinePlanChoice"[\s\S]*?:key="pendingUserInput\.itemId"/
    );
  });

  it('memoizes the freeform input against unrelated live refreshes', () => {
    const fieldSource = extractUserInputFieldSource();

    expect(fieldSource).toContain('v-memo="[');
    expect(fieldSource).toContain('pendingUserInput.itemId');
    expect(fieldSource).toContain('question.id');
    expect(fieldSource).toContain('userInputDrafts[question.id]');
    expect(fieldSource).toContain('isUserInputInteractionDisabled');
    expect(fieldSource).toContain('question.isSecret');
    expect(fieldSource).toContain('userInputPlaceholder(question)');
  });
});
