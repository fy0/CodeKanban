import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionTimelineStylePath = fileURLToPath(
  new URL('../styles/webSessionPanelTimeline.css', import.meta.url)
);
const webSessionTimelineStyleSource = readFileSync(webSessionTimelineStylePath, 'utf8');

describe('webSession plan layout', () => {
  it('fills narrow session panes without relying on the viewport breakpoint', () => {
    expect(webSessionTimelineStyleSource).toMatch(
      /\.timeline-tool-shell\.plan-tool-shell\s*\{[^}]*width:\s*min\(860px, 100%\);/s
    );
    expect(webSessionTimelineStyleSource).not.toMatch(
      /@media \(max-width: 767px\)\s*\{\s*\.timeline-tool-shell\.plan-tool-shell/s
    );
  });
});
