import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionTimelineStylePath = fileURLToPath(
  new URL('../styles/webSessionPanelTimeline.css', import.meta.url)
);
const webSessionTimelineStyleSource = readFileSync(webSessionTimelineStylePath, 'utf8');

describe('webSession goal layout', () => {
  it('keeps long goals within a bounded card', () => {
    expect(webSessionTimelineStyleSource).toMatch(
      /\.goal-card\s*\{[^}]*flex-shrink:\s*1;[^}]*min-height:\s*0;[^}]*max-height:\s*min\(50dvh, 520px\);[^}]*overflow:\s*hidden;/s
    );
  });

  it('scrolls only the objective while keeping controls and metadata visible', () => {
    expect(webSessionTimelineStyleSource).toMatch(/\.goal-card-header\s*\{[^}]*flex-shrink:\s*0;/s);
    expect(webSessionTimelineStyleSource).toMatch(
      /\.goal-card-body\s*\{[^}]*flex:\s*1 1 auto;[^}]*min-height:\s*0;[^}]*overflow:\s*hidden;/s
    );
    expect(webSessionTimelineStyleSource).toMatch(
      /\.goal-objective\s*\{[^}]*flex:\s*1 1 auto;[^}]*min-height:\s*0;[^}]*overflow-y:\s*auto;[^}]*scrollbar-gutter:\s*stable;/s
    );
    expect(webSessionTimelineStyleSource).toMatch(/\.goal-meta-row\s*\{[^}]*flex-shrink:\s*0;/s);
  });
});
