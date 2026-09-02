import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const panelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const panelSource = readFileSync(panelPath, 'utf8');

function sourceBetween(start: string, end: string) {
  const startIndex = panelSource.indexOf(start);
  const endIndex = panelSource.indexOf(end, startIndex + start.length);
  expect(startIndex).toBeGreaterThanOrEqual(0);
  expect(endIndex).toBeGreaterThan(startIndex);
  return panelSource.slice(startIndex, endIndex);
}

describe('web session route coordination', () => {
  it('locks the deep-link target before loading sessions', () => {
    const initialization = sourceBetween(
      'async function initializeProjectSessions',
      'async function handleSessionSelect'
    );
    const pendingIndex = initialization.indexOf('pendingRouteActivationSessionId.value =');
    const loadIndex = initialization.indexOf('await webSessionStore.loadSessions(projectId)');

    expect(pendingIndex).toBeGreaterThanOrEqual(0);
    expect(pendingIndex).toBeLessThan(loadIndex);
    expect(initialization).toContain('currentRouteSessionId !== routeSessionId');
    expect(initialization).toContain('requestSessionActivationFromRoute');
  });

  it('defers route activation instead of restarting project initialization', () => {
    const routeWatcher = sourceBetween(
      '[routeWebSessionId, () => props.isActive, routeWorkspaceTab, isProjectSessionInitializing]',
      'watch([sendConfirmationSignature, planImplementConfirmationSignature]'
    );

    expect(routeWatcher).toContain('if (!isActive)');
    expect(routeWatcher).toContain('if (initializing)');
    expect(routeWatcher).toContain('requestSessionActivationFromRoute');
    expect(routeWatcher).not.toContain('initializeProjectSessions(props.projectId)');
  });

  it('serializes URL writes and budgets unknown-session snapshots', () => {
    const routeFunctions = sourceBetween(
      'async function drainWebSessionRouteWrites()',
      'async function openArchivedPreviewSession'
    );
    const activation = sourceBetween(
      'function requestSessionActivationFromRoute',
      'function removeDraftSessionRecord'
    );

    expect(routeFunctions).toContain('if (routeWriteFlight)');
    expect(routeFunctions).toContain('await router.replace');
    expect(activation).toContain('if (routeActivationFlight?.key === key)');
    expect(activation).toContain('routeSnapshotBudget.tryAcquire');
    expect(activation).toContain('routeSnapshotBudget.markResolved');
  });
});
