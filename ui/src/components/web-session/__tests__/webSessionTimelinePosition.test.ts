import { describe, expect, it, vi } from 'vitest';

import {
  findClosestWebSessionTimelineAnchor,
  forgetWebSessionTimelinePosition,
  getWebSessionTimelinePosition,
  loadWebSessionTimelinePositionState,
  persistWebSessionTimelinePositionState,
  rememberWebSessionTimelinePosition,
  resolveWebSessionTimelineAnchor,
  resolveWebSessionTimelineRestoreScrollTop,
  WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY,
  type WebSessionTimelinePosition,
} from '@/components/web-session/webSessionTimelinePosition';

function createStorageMock(initial: Record<string, string> = {}) {
  const values = new Map(Object.entries(initial));
  return {
    getItem(key: string) {
      return values.get(key) ?? null;
    },
    setItem(key: string, value: string) {
      values.set(key, value);
    },
    removeItem(key: string) {
      values.delete(key);
    },
  };
}

function makePosition(updatedAt: number, overrides: Partial<WebSessionTimelinePosition> = {}) {
  return {
    anchorKey: 'item-2:2',
    anchorOrderIndex: 2,
    anchorOffsetPx: -12,
    scrollTop: 320,
    followBottom: false,
    updatedAt,
    ...overrides,
  };
}

describe('webSessionTimelinePosition', () => {
  it('round-trips positions through browser storage and isolates projects', () => {
    const storage = createStorageMock();
    const state = loadWebSessionTimelinePositionState(storage);
    rememberWebSessionTimelinePosition(state, 'project-a', 'session-1', makePosition(10));
    rememberWebSessionTimelinePosition(
      state,
      'project-b',
      'session-1',
      makePosition(20, { scrollTop: 640 })
    );
    persistWebSessionTimelinePositionState(state, storage);

    const restored = loadWebSessionTimelinePositionState(storage);
    expect(getWebSessionTimelinePosition(restored, 'project-a', 'session-1')?.scrollTop).toBe(320);
    expect(getWebSessionTimelinePosition(restored, 'project-b', 'session-1')?.scrollTop).toBe(640);
  });

  it('drops malformed storage and falls back to an empty state', () => {
    const storage = createStorageMock({
      [WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY]: '{broken',
    });
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});

    expect(loadWebSessionTimelinePositionState(storage)).toEqual({ version: 1, projects: {} });
    expect(storage.getItem(WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY)).toBeNull();

    warn.mockRestore();
  });

  it('keeps only the most recently updated records', () => {
    const state = loadWebSessionTimelinePositionState(null);
    rememberWebSessionTimelinePosition(state, 'project', 'old', makePosition(1), 2);
    rememberWebSessionTimelinePosition(state, 'project', 'newest', makePosition(3), 2);
    rememberWebSessionTimelinePosition(state, 'project', 'middle', makePosition(2), 2);

    expect(getWebSessionTimelinePosition(state, 'project', 'old')).toBeNull();
    expect(Object.keys(state.projects.project)).toEqual(['newest', 'middle']);
  });

  it('forgets deleted sessions and removes empty project buckets', () => {
    const state = loadWebSessionTimelinePositionState(null);
    rememberWebSessionTimelinePosition(state, 'project', 'session', makePosition(1));
    forgetWebSessionTimelinePosition(state, 'project', 'session');

    expect(state.projects).toEqual({});
  });

  it('selects the first timeline block intersecting the viewport', () => {
    expect(
      resolveWebSessionTimelineAnchor(
        [
          { key: 'above', orderIndex: 1, top: 20, bottom: 90 },
          { key: 'visible', orderIndex: 2, top: 90, bottom: 180 },
          { key: 'below', orderIndex: 3, top: 180, bottom: 260 },
        ],
        100,
        200
      )
    ).toEqual({ key: 'visible', orderIndex: 2, top: 90, bottom: 180 });
  });

  it('restores the saved anchor offset and clamps to the scroll range', () => {
    expect(resolveWebSessionTimelineRestoreScrollTop(400, 80, -20, 1000)).toBe(500);
    expect(resolveWebSessionTimelineRestoreScrollTop(950, 100, 0, 980)).toBe(980);
    expect(resolveWebSessionTimelineRestoreScrollTop(10, -50, 0, 980)).toBe(0);
  });

  it('prefers an exact key and otherwise falls back to the nearest order index', () => {
    const candidates = [
      { key: 'one', orderIndex: 10 },
      { key: 'two', orderIndex: 20 },
      { key: 'three', orderIndex: 30 },
    ];

    expect(findClosestWebSessionTimelineAnchor(candidates, 'two', 29)?.key).toBe('two');
    expect(findClosestWebSessionTimelineAnchor(candidates, 'missing', 27)?.key).toBe('three');
    expect(findClosestWebSessionTimelineAnchor(candidates, 'missing', null)).toBeNull();
  });
});
