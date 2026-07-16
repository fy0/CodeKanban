import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

describe('webSession plan layout', () => {
  it('fills narrow session panes without relying on the viewport breakpoint', () => {
    expect(webSessionPanelSource).toMatch(
      /\.timeline-tool-shell\.plan-tool-shell\s*\{[^}]*width:\s*min\(860px, 100%\);/s
    );
    expect(webSessionPanelSource).not.toMatch(
      /@media \(max-width: 767px\)\s*\{\s*\.timeline-tool-shell\.plan-tool-shell/s
    );
  });
});
