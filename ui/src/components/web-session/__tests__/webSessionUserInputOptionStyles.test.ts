import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const webSessionUserInputQuestionPath = fileURLToPath(
  new URL('../WebSessionUserInputQuestion.vue', import.meta.url)
);
const webSessionUserInputQuestionSource = readFileSync(webSessionUserInputQuestionPath, 'utf8');
const webSessionUserInputQuestionStylePath = fileURLToPath(
  new URL('../styles/webSessionUserInputQuestion.css', import.meta.url)
);
const webSessionUserInputQuestionStyleSource = readFileSync(
  webSessionUserInputQuestionStylePath,
  'utf8'
);

describe('web session user input option styles', () => {
  it('exposes selected and disabled state classes for single and multi-select options', () => {
    const optionBlocks = webSessionUserInputQuestionSource.match(
      /<div\s+v-for="option in question\.options"[\s\S]*?class="user-input-option"[\s\S]*?<\/div>/g
    );

    expect(optionBlocks).toHaveLength(2);
    for (const optionBlock of optionBlocks ?? []) {
      expect(optionBlock).toContain("'is-selected': selection.includes(option.label)");
      expect(optionBlock).toContain("'is-disabled': disabled");
    }
  });

  it('keeps the option card states scoped to the pending user input surface', () => {
    expect(webSessionUserInputQuestionStyleSource).toContain('.user-input-option.is-selected');
    expect(webSessionUserInputQuestionStyleSource).toContain('.user-input-option:focus-within');
    expect(webSessionUserInputQuestionStyleSource).toContain('.user-input-option.is-disabled');
    expect(webSessionPanelSource).toContain('<WebSessionUserInputQuestionField');
  });

  it('leaves radio marker rendering to the controlled Naive UI state', () => {
    expect(webSessionUserInputQuestionStyleSource).not.toContain('.n-radio__dot');
    expect(webSessionUserInputQuestionSource).toContain(':value="selection[0] || null"');
    expect(webSessionUserInputQuestionSource).toContain('@update:value="handleSingleSelect"');
  });

  it('does not add an inset accent line to the selected option card', () => {
    expect(webSessionUserInputQuestionStyleSource).not.toContain('box-shadow: inset 3px 0 0');
    expect(webSessionUserInputQuestionStyleSource).not.toContain('box-shadow 0.16s ease');
  });
});
