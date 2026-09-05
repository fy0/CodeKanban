import { describe, expect, it } from 'vitest';

import {
  buildOrderedTabSessions,
  clampTabAnchorIndex,
  resolveActiveTabSessionId,
  resolveUnderlyingTabSessionId,
  sortMobileCurrentSessions,
} from '@/components/web-session/webSessionTabOrder';

function makeSessions(ids: string[]) {
  return ids.map(id => ({ id }));
}

describe('webSessionTabOrder', () => {
  it('inserts a fixed archived preview at the anchored index', () => {
    const baseSessions = makeSessions(['real-1', 'draft-1', 'real-2']);
    const archivedPreview = { id: 'archived-1' };

    const ordered = buildOrderedTabSessions(
      ['real-2', 'draft-1', 'real-1'],
      baseSessions,
      archivedPreview,
      1
    );

    expect(ordered.map(session => session.id)).toEqual([
      'real-2',
      'archived-1',
      'draft-1',
      'real-1',
    ]);
  });

  it('keeps the fixed archived preview anchored even if an archived id appears in order ids', () => {
    const baseSessions = makeSessions(['real-1', 'draft-1', 'real-2']);
    const archivedPreview = { id: 'archived-1' };

    const ordered = buildOrderedTabSessions(
      ['real-2', 'archived-1', 'real-1', 'draft-1'],
      baseSessions,
      archivedPreview,
      1
    );

    expect(ordered.map(session => session.id)).toEqual([
      'real-2',
      'archived-1',
      'real-1',
      'draft-1',
    ]);
  });

  it('clamps archived anchor indexes into the current base range', () => {
    expect(clampTabAnchorIndex(-5, 3)).toBe(0);
    expect(clampTabAnchorIndex(2, 3)).toBe(2);
    expect(clampTabAnchorIndex(99, 3)).toBe(3);
    expect(clampTabAnchorIndex(Number.NaN, 3)).toBe(3);
  });

  it('hides current-tab selection while an archived preview is open', () => {
    expect(
      resolveActiveTabSessionId({
        activeArchivedPreviewId: 'archived-1',
        activeDraftSessionId: 'draft-1',
        activeRealSessionId: 'real-1',
      })
    ).toBe('');
  });

  it('keeps the underlying draft or real tab id for non-visual tab actions', () => {
    expect(
      resolveUnderlyingTabSessionId({
        activeDraftSessionId: 'draft-1',
        activeRealSessionId: 'real-1',
      })
    ).toBe('draft-1');
    expect(
      resolveUnderlyingTabSessionId({
        activeDraftSessionId: '',
        activeRealSessionId: 'real-1',
      })
    ).toBe('real-1');
  });

  it('pins drafts first and sorts real sessions by recency for mobile navigation', () => {
    const sessions = [
      { id: 'real-older', orderIndex: 20 },
      { id: 'draft-a', orderIndex: 100, isDraft: true as const },
      { id: 'real-newer', orderIndex: 10 },
      { id: 'draft-b', orderIndex: 200, isDraft: true as const },
    ];
    const timestamps = new Map<string, number>([
      ['real-older', 100],
      ['real-newer', 300],
    ]);

    const ordered = sortMobileCurrentSessions(sessions, session => timestamps.get(session.id) ?? 0);

    expect(ordered.map(session => session.id)).toEqual([
      'draft-a',
      'draft-b',
      'real-newer',
      'real-older',
    ]);
  });

  it('breaks real-session recency ties by orderIndex and id', () => {
    const sessions = [
      { id: 'real-b', orderIndex: 30 },
      { id: 'real-a', orderIndex: 30 },
      { id: 'real-c', orderIndex: 10 },
    ];

    const ordered = sortMobileCurrentSessions(sessions, () => 500);

    expect(ordered.map(session => session.id)).toEqual(['real-c', 'real-a', 'real-b']);
  });
});
