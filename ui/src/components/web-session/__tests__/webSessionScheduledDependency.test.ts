import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const panelSource = readFileSync(
  fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url)),
  'utf8'
);
const dialogSource = readFileSync(
  fileURLToPath(new URL('../WebSessionScheduledSendDialog.vue', import.meta.url)),
  'utf8'
);
const storeSource = readFileSync(
  fileURLToPath(new URL('../../../stores/webSession.ts', import.meta.url)),
  'utf8'
);

describe('web session scheduled dependencies', () => {
  it('offers an optional prerequisite only inside the delayed-send dialog', () => {
    expect(dialogSource).toContain("t('webSession.scheduledDependencyTitle')");
    expect(dialogSource).toContain(':options="dependencyOptions"');
    expect(dialogSource).toContain("emit('update:dependency-id', $event ?? '')");
    expect(panelSource).toContain(':dependency-options="scheduledDependencyOptions"');
    expect(panelSource).toContain('scheduledDependencyWouldCreateCycle');
  });

  it('sends dependency data for delayed messages, plans, edits, and explicit bypasses', () => {
    expect(storeSource).toContain('...(options.dependsOnId ? { dep: options.dependsOnId } : {})');
    expect(storeSource).toContain(
      "...(typeof update.dependsOnId === 'string' ? { dep: update.dependsOnId } : {})"
    );
    expect(storeSource).toContain('...(bypassDependency ? { bd: true } : {})');
    expect(panelSource).toContain('dependsOnId: scheduledDependsOnId.value || undefined');
    expect(panelSource).toContain('dependsOnId: scheduledDependsOnId.value');
  });

  it('shows dependency state and confirms before manually bypassing it', () => {
    expect(panelSource).toContain('class="scheduled-input-dependency"');
    expect(panelSource).toContain('scheduledDependencyStatusLabel(item.dependencyStatus)');
    expect(panelSource).toContain("t('webSession.scheduledDependencyBypassTitle')");
    expect(panelSource).toContain(
      'onPositiveClick: () => dispatchScheduledInputNowAfterDependencyCheck(item, true)'
    );
  });
});
