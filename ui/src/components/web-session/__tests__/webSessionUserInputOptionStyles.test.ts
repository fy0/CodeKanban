import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const webSessionTimelineStylePath = fileURLToPath(
  new URL('../styles/webSessionPanelTimeline.css', import.meta.url)
);
const webSessionTimelineStyleSource = readFileSync(webSessionTimelineStylePath, 'utf8');

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
    expect(webSessionTimelineStyleSource).toContain('.user-input-option.is-selected');
    expect(webSessionTimelineStyleSource).toContain('.user-input-option:focus-within');
    expect(webSessionTimelineStyleSource).toContain('.user-input-option.is-disabled');
    expect(webSessionTimelineStyleSource).toContain('.history-option-row');
  });

  it('renders the selected radio marker without suspended transitions', () => {
    expect(webSessionTimelineStyleSource).toMatch(
      /\.user-input-option :deep\(\.n-radio__dot\),\s*\.user-input-option :deep\(\.n-radio__dot::before\)\s*{\s*transition: none;\s*}/
    );
    expect(webSessionTimelineStyleSource).toMatch(
      /\.user-input-option\.is-selected :deep\(\.n-radio__dot\)\s*{\s*background-color: var\(--n-color-active\);\s*}/
    );
    expect(webSessionTimelineStyleSource).toMatch(
      /\.user-input-option\.is-selected :deep\(\.n-radio__dot::before\)\s*{\s*opacity: 1;\s*background-color: var\(--n-dot-color-active\);\s*transform: scale\(1\);\s*}/
    );
  });

  it('does not add an inset accent line to the selected option card', () => {
    expect(webSessionTimelineStyleSource).not.toContain('box-shadow: inset 3px 0 0');
    expect(webSessionTimelineStyleSource).not.toContain('box-shadow 0.16s ease');
  });
});
