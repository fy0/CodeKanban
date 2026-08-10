import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const componentPath = fileURLToPath(
  new URL('../WebSessionWorkTimingBackfill.vue', import.meta.url)
);
const componentSource = readFileSync(componentPath, 'utf8');
const settingsPath = fileURLToPath(new URL('../../../views/GeneralSettings.vue', import.meta.url));
const settingsSource = readFileSync(settingsPath, 'utf8');

describe('web session work timing backfill settings', () => {
  it('uses a bounded user-sized batch with the confirmed default', () => {
    expect(componentSource).toContain('batchSize = ref<number | null>(50)');
    expect(componentSource).toContain(':min="1"');
    expect(componentSource).toContain(':max="500"');
    expect(componentSource).toContain('webSessionApi.runWorkTimingBackfill');
  });

  it('loads only indexed status until the user runs a batch', () => {
    expect(componentSource).toContain('webSessionApi.workTimingBackfillStatus');
    expect(componentSource).toContain('@click="runBatch"');
    expect(componentSource).not.toContain('while (');
  });

  it('appears above chat-cache cleanup without nesting another card', () => {
    const timingIndex = settingsSource.indexOf('<WebSessionWorkTimingBackfill />');
    const cleanupIndex = settingsSource.indexOf('<WebSessionHistoryCleanup />');
    expect(timingIndex).toBeGreaterThan(0);
    expect(cleanupIndex).toBeGreaterThan(timingIndex);
    expect(componentSource).not.toContain('<n-card');
  });
});
