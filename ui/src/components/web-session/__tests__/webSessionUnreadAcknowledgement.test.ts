import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const panelSource = readFileSync(
  fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url)),
  'utf8'
);

function sourceBetween(start: string, end: string) {
  const startIndex = panelSource.indexOf(start);
  const endIndex = panelSource.indexOf(end, startIndex + start.length);
  expect(startIndex).toBeGreaterThanOrEqual(0);
  expect(endIndex).toBeGreaterThan(startIndex);
  return panelSource.slice(startIndex, endIndex);
}

describe('web session unread acknowledgement', () => {
  it('acknowledges every successful real-session activation', () => {
    const activationSource = sourceBetween(
      'async function activateTabById(',
      'function buildProjectRouteLocation('
    );

    expect(activationSource).toContain(
      'activated = await connectVisibleRealSession(props.projectId, session.id);'
    );
    expect(activationSource).toContain('void acknowledgeVisibleSessionView(session.id);');
  });

  it('waits for rendering and rechecks visibility before marking read', () => {
    const acknowledgementSource = sourceBetween(
      'async function acknowledgeVisibleSessionView(',
      'function getSessionAttentionRevision('
    );

    expect(acknowledgementSource.indexOf('await nextTick();')).toBeLessThan(
      acknowledgementSource.indexOf('!isCurrentVisibleSession(sessionId)')
    );
    expect(acknowledgementSource.indexOf('!isCurrentVisibleSession(sessionId)')).toBeLessThan(
      acknowledgementSource.indexOf('markSessionViewed(sessionId);')
    );
    expect(acknowledgementSource.indexOf('markSessionViewed(sessionId);')).toBeLessThan(
      acknowledgementSource.indexOf('void webSessionStore.markSessionRead(sessionId);')
    );
  });

  it('uses the attention clock for optimistic unread state', () => {
    expect(panelSource).toContain('optimisticUnreadClearedRevisionBySession');
    expect(panelSource).toContain('compareWebSessionAttentionRevisions(');
    expect(panelSource).not.toContain('function getSessionUnreadVersion(');
  });
});
