import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));

describe('webSessionContextPopover', () => {
  it('shows the context card on hover with both cumulative usage metrics', () => {
    const source = readFileSync(webSessionPanelPath, 'utf8');
    const popoverStart = source.indexOf('v-if="contextUsageIndicator"');
    const popoverEnd = source.indexOf('</n-popover>', popoverStart);
    const popoverSource = source.slice(popoverStart, popoverEnd);

    expect(popoverStart).toBeGreaterThan(-1);
    expect(popoverEnd).toBeGreaterThan(popoverStart);
    expect(popoverSource).toContain('trigger="hover"');
    expect(popoverSource).toContain('contextUsageCumulativeNonCached');
    expect(popoverSource).toContain('contextUsageTotalUsage');
  });
});
