import { readFileSync } from 'node:fs';
import { Window } from 'happy-dom';
import { describe, expect, it } from 'vitest';

describe('context window layout', () => {
  it('keeps value and edit-button alignment outside mobile media queries', () => {
    const window = new Window();
    const style = window.document.createElement('style');
    style.textContent = readFileSync(
      new URL('../styles/webSessionPanelComposer.css', import.meta.url),
      'utf8'
    );
    window.document.head.appendChild(style);
    const topLevelRules = Array.from(style.sheet?.cssRules ?? []).map(rule => rule.cssText);
    expect(
      topLevelRules.some(
        rule => rule.startsWith('.context-usage-window-value {') && rule.includes('display: flex')
      )
    ).toBe(true);
    expect(
      topLevelRules.some(
        rule => rule.startsWith('.context-window-edit {') && rule.includes('display: inline-flex')
      )
    ).toBe(true);
    window.close();
  });
});
