import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

describe('web session user input option styles', () => {
  it('exposes selected and disabled state classes for single and multi-select options', () => {
    const optionBlocks = webSessionPanelSource.match(
      /<div\s+v-for="option in question\.options"[\s\S]*?class="user-input-option"[\s\S]*?<\/div>/g
    );

    expect(optionBlocks).toHaveLength(2);
    for (const optionBlock of optionBlocks ?? []) {
      expect(optionBlock).toContain(
        "'is-selected': userInputSelections[question.id]?.includes(option.label)"
      );
      expect(optionBlock).toContain("'is-disabled': isUserInputInteractionDisabled");
    }
  });

  it('keeps the option card states scoped to the pending user input surface', () => {
    expect(webSessionPanelSource).toContain('.user-input-option.is-selected');
    expect(webSessionPanelSource).toContain('.user-input-option:focus-within');
    expect(webSessionPanelSource).toContain('.user-input-option.is-disabled');
    expect(webSessionPanelSource).toContain('.history-option-row');
  });
});
