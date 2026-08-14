import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const componentPath = fileURLToPath(new URL('../WebSessionHistoryCleanup.vue', import.meta.url));
const componentSource = readFileSync(componentPath, 'utf8');

describe('web session history cleanup settings', () => {
  it('supports all-project and multi-project cleanup scopes', () => {
    expect(componentSource).toContain("scope = ref<'all' | 'projects'>('all')");
    expect(componentSource).toContain('v-model:value="selectedProjectIds"');
    expect(componentSource).toContain('multiple');
    expect(componentSource).toContain('filterable');
  });

  it('requires a preview before running cleanup and invalidates local histories', () => {
    expect(componentSource).toContain('webSessionApi.previewHistoryCleanup');
    expect(componentSource).toContain(':disabled="!canRunCleanup || previewLoading"');
    expect(componentSource).toContain('previewRequestKey.value === cleanupRequestKey.value');
    expect(componentSource).toContain('webSessionApi.runHistoryCleanup');
    expect(componentSource).toContain('webSessionStore.invalidateCleanedHistories');
  });

  it('uses the confirmed default retention values', () => {
    expect(componentSource).toContain('olderThanDays = ref<number | null>(30)');
    expect(componentSource).toContain('retainPerProject = ref<number | null>(10)');
  });

  it('explains storage concepts and exposes archive cache actions', () => {
    expect(componentSource).toContain('settings.historyCleanupConcept');
    expect(componentSource).toContain('settings.historyStorageDatabase');
    expect(componentSource).toContain('settings.historyStorageArchivedCache');
    expect(componentSource).toContain('settings.historyCleanupModeArchive');
    expect(componentSource).toContain('settings.historyCleanupModeArchivedCache');
    expect(componentSource).toContain('webSessionApi.previewHistoryArchive');
    expect(componentSource).toContain('webSessionApi.runHistoryArchive');
    expect(componentSource).toContain('archivedOnly: true');
  });
});
