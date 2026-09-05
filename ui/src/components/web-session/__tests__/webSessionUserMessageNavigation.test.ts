import { describe, expect, it, vi } from 'vitest';

import {
  canNavigateWebSessionUserMessage,
  findAdjacentWebSessionUserMessageKey,
  findViewportAdjacentWebSessionUserMessageKey,
  resolveWebSessionTimelineStartConfirmation,
  resolveWebSessionUserMessageTarget,
  type WebSessionUserMessageNavigationBlock,
} from '@/components/web-session/webSessionUserMessageNavigation';

const blocks: WebSessionUserMessageNavigationBlock[] = [
  { key: 'user-1', kind: 'user' },
  { key: 'assistant-1', kind: 'assistant' },
  { key: 'tool-1', kind: 'tool' },
  { key: 'user-2', kind: 'user' },
  { key: 'system-1', kind: 'system' },
  { key: 'user-3', kind: 'user' },
];

describe('webSessionUserMessageNavigation', () => {
  it('requires two clicks on the same session to jump to the timeline start', () => {
    const first = resolveWebSessionTimelineStartConfirmation({
      sessionId: 'session-1',
      currentState: null,
      now: 1000,
      ttlMs: 5000,
    });
    expect(first).toEqual({
      shouldProceed: false,
      nextState: { sessionId: 'session-1', expiresAt: 6000 },
    });

    expect(
      resolveWebSessionTimelineStartConfirmation({
        sessionId: 'session-1',
        currentState: first.nextState,
        now: 1500,
        ttlMs: 5000,
      })
    ).toEqual({ shouldProceed: true, nextState: null });

    expect(
      resolveWebSessionTimelineStartConfirmation({
        sessionId: 'session-2',
        currentState: first.nextState,
        now: 1500,
        ttlMs: 5000,
      }).shouldProceed
    ).toBe(false);
  });

  it('finds adjacent user messages relative to the clicked message', () => {
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'user-2', 'previous')).toBe('user-1');
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'user-2', 'next')).toBe('user-3');
  });

  it('ignores non-user blocks and handles loaded boundaries', () => {
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'assistant-1', 'previous')).toBeNull();
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'user-1', 'previous')).toBeNull();
    expect(findAdjacentWebSessionUserMessageKey(blocks, 'user-3', 'next')).toBeNull();

    expect(
      canNavigateWebSessionUserMessage({
        blocks,
        currentKey: 'user-1',
        direction: 'previous',
        canLoadEarlier: true,
      })
    ).toBe(true);
    expect(
      canNavigateWebSessionUserMessage({
        blocks,
        currentKey: 'user-3',
        direction: 'next',
        canLoadEarlier: true,
        canLoadLater: true,
      })
    ).toBe(true);
  });

  it('finds viewport-relative user messages without treating an aligned item as adjacent', () => {
    const candidates = [
      { key: 'user-1', top: -500 },
      { key: 'user-2', top: 0 },
      { key: 'user-3', top: 420 },
    ];

    expect(findViewportAdjacentWebSessionUserMessageKey(candidates, 0, 'previous')).toBe('user-1');
    expect(findViewportAdjacentWebSessionUserMessageKey(candidates, 0, 'next')).toBe('user-3');
    expect(findViewportAdjacentWebSessionUserMessageKey(candidates, 240, 'previous')).toBe(
      'user-2'
    );
    expect(findViewportAdjacentWebSessionUserMessageKey(candidates, 240, 'next')).toBe('user-3');
  });

  it('loads multiple earlier pages until it finds a previous user message', async () => {
    let currentBlocks: WebSessionUserMessageNavigationBlock[] = [
      { key: 'user-current', kind: 'user' },
      { key: 'assistant-current', kind: 'assistant' },
    ];
    const pages: WebSessionUserMessageNavigationBlock[][] = [
      [
        { key: 'system-old', kind: 'system' },
        { key: 'tool-old', kind: 'tool' },
      ],
      [
        { key: 'user-previous', kind: 'user' },
        { key: 'assistant-previous', kind: 'assistant' },
      ],
    ];
    let loadState = 0;
    const loadEarlier = vi.fn(async () => {
      currentBlocks = [...(pages.shift() ?? []), ...currentBlocks];
      loadState += 1;
    });

    await expect(
      resolveWebSessionUserMessageTarget({
        currentKey: 'user-current',
        direction: 'previous',
        getBlocks: () => currentBlocks,
        canLoadEarlier: () => pages.length > 0,
        getLoadStateKey: () => String(loadState),
        loadEarlier,
      })
    ).resolves.toBe('user-previous');
    expect(loadEarlier).toHaveBeenCalledTimes(2);
  });

  it('stops when an earlier-history request makes no progress', async () => {
    const loadEarlier = vi.fn(async () => undefined);

    await expect(
      resolveWebSessionUserMessageTarget({
        currentKey: 'user-1',
        direction: 'previous',
        getBlocks: () => blocks,
        canLoadEarlier: () => true,
        getLoadStateKey: () => 'unchanged',
        loadEarlier,
      })
    ).resolves.toBeNull();
    expect(loadEarlier).toHaveBeenCalledOnce();
  });

  it('never loads earlier history while navigating forward', async () => {
    const loadEarlier = vi.fn(async () => undefined);

    await expect(
      resolveWebSessionUserMessageTarget({
        currentKey: 'user-3',
        direction: 'next',
        getBlocks: () => blocks,
        canLoadEarlier: () => true,
        getLoadStateKey: () => 'unchanged',
        loadEarlier,
      })
    ).resolves.toBeNull();
    expect(loadEarlier).not.toHaveBeenCalled();
  });

  it('loads later chronological pages when navigating forward from the start window', async () => {
    let currentBlocks: WebSessionUserMessageNavigationBlock[] = [
      { key: 'user-current', kind: 'user' },
      { key: 'assistant-current', kind: 'assistant' },
    ];
    let canLoadLater = true;
    let loadState = 0;
    const loadLater = vi.fn(async () => {
      currentBlocks = [...currentBlocks, { key: 'user-next', kind: 'user' }];
      canLoadLater = false;
      loadState += 1;
    });

    await expect(
      resolveWebSessionUserMessageTarget({
        currentKey: 'user-current',
        direction: 'next',
        getBlocks: () => currentBlocks,
        canLoadEarlier: () => false,
        canLoadLater: () => canLoadLater,
        getLoadStateKey: () => String(loadState),
        loadEarlier: vi.fn(async () => undefined),
        loadLater,
      })
    ).resolves.toBe('user-next');
    expect(loadLater).toHaveBeenCalledOnce();
  });
});
