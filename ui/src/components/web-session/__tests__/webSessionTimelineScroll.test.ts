import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import {
  createWebSessionMobileComposerScrollState,
  createWebSessionTimelineFollowState,
  resolveWebSessionMobileComposerBottomScrollAction,
  resolveWebSessionMobileComposerScrollState,
  resolveWebSessionTimelineFollowState,
  shouldApplyWebSessionTimelineAutoScroll,
} from '@/components/web-session/webSessionTimelineScroll';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));

describe('webSessionTimelineScroll', () => {
  it('leaves bottom follow mode when the user scrolls upward from the bottom', () => {
    const previous = createWebSessionTimelineFollowState({
      scrollTop: 800,
      scrollHeight: 1000,
      clientHeight: 200,
    });

    const next = resolveWebSessionTimelineFollowState(previous, {
      scrollTop: 780,
      scrollHeight: 1000,
      clientHeight: 200,
    });

    expect(next).toEqual({
      autoFollowBottom: false,
      showJumpToBottom: true,
      lastScrollTop: 780,
    });
  });

  it('does not re-enable follow mode until the timeline reaches the real bottom', () => {
    const previous = {
      autoFollowBottom: false,
      showJumpToBottom: true,
      lastScrollTop: 780,
    };

    expect(
      resolveWebSessionTimelineFollowState(previous, {
        scrollTop: 796,
        scrollHeight: 1000,
        clientHeight: 200,
      })
    ).toEqual({
      autoFollowBottom: true,
      showJumpToBottom: false,
      lastScrollTop: 796,
    });

    expect(
      resolveWebSessionTimelineFollowState(previous, {
        scrollTop: 795,
        scrollHeight: 1000,
        clientHeight: 200,
      })
    ).toEqual({
      autoFollowBottom: false,
      showJumpToBottom: true,
      lastScrollTop: 795,
    });
  });

  it('keeps follow mode after programmatic bottom sync', () => {
    const previous = {
      autoFollowBottom: true,
      showJumpToBottom: false,
      lastScrollTop: 760,
    };

    expect(
      resolveWebSessionTimelineFollowState(previous, {
        scrollTop: 800,
        scrollHeight: 1000,
        clientHeight: 200,
      })
    ).toEqual({
      autoFollowBottom: true,
      showJumpToBottom: false,
      lastScrollTop: 800,
    });
  });

  it('does not apply a stale bottom sync after user interaction', () => {
    expect(shouldApplyWebSessionTimelineAutoScroll(1, 2, true, true)).toBe(false);
    expect(shouldApplyWebSessionTimelineAutoScroll(2, 2, true, false)).toBe(true);
    expect(shouldApplyWebSessionTimelineAutoScroll(2, 2, false, false)).toBe(false);
  });

  it('waits for enough upward scrolling before collapsing the mobile composer', () => {
    const initial = createWebSessionMobileComposerScrollState({
      scrollTop: 800,
      scrollHeight: 1000,
      clientHeight: 200,
    });

    const first = resolveWebSessionMobileComposerScrollState(initial, {
      scrollTop: 760,
      scrollHeight: 1000,
      clientHeight: 200,
    });

    expect(first).toEqual({
      action: 'none',
      state: {
        lastScrollTop: 760,
        lastClientHeight: 200,
        upwardDistance: 40,
      },
    });

    expect(
      resolveWebSessionMobileComposerScrollState(first.state, {
        scrollTop: 720,
        scrollHeight: 1000,
        clientHeight: 200,
      })
    ).toEqual({
      action: 'collapse',
      state: {
        lastScrollTop: 720,
        lastClientHeight: 200,
        upwardDistance: 0,
      },
    });
  });

  it('resets mobile composer upward distance when scrolling downward', () => {
    const previous = {
      lastScrollTop: 760,
      lastClientHeight: 200,
      upwardDistance: 40,
    };

    expect(
      resolveWebSessionMobileComposerScrollState(previous, {
        scrollTop: 780,
        scrollHeight: 1000,
        clientHeight: 200,
      })
    ).toEqual({
      action: 'none',
      state: {
        lastScrollTop: 780,
        lastClientHeight: 200,
        upwardDistance: 0,
      },
    });
  });

  it('ignores scroll position changes caused by mobile composer height changes', () => {
    const previous = createWebSessionMobileComposerScrollState({
      scrollTop: 1501,
      scrollHeight: 2000,
      clientHeight: 499,
    });

    expect(
      resolveWebSessionMobileComposerScrollState(previous, {
        scrollTop: 1393,
        scrollHeight: 2000,
        clientHeight: 607,
      })
    ).toEqual({
      action: 'none',
      state: {
        lastScrollTop: 1393,
        lastClientHeight: 607,
        upwardDistance: 0,
      },
    });
  });

  it('only expands the mobile composer after an extra downward scroll at the bottom', () => {
    expect(
      resolveWebSessionMobileComposerBottomScrollAction(
        {
          scrollTop: 700,
          scrollHeight: 1000,
          clientHeight: 200,
        },
        24
      )
    ).toBe('none');

    expect(
      resolveWebSessionMobileComposerBottomScrollAction(
        {
          scrollTop: 800,
          scrollHeight: 1000,
          clientHeight: 200,
        },
        0
      )
    ).toBe('none');

    expect(
      resolveWebSessionMobileComposerBottomScrollAction(
        {
          scrollTop: 800,
          scrollHeight: 1000,
          clientHeight: 200,
        },
        24
      )
    ).toBe('expand');
  });

  it('opts the runtime strip out of browser scroll anchoring', () => {
    const source = readFileSync(webSessionPanelPath, 'utf8');

    expect(source).toMatch(/\.runtime-strip\s*\{[^}]*overflow-anchor:\s*none;/s);
  });

  it('only restores timeline position when project or session identity changes', () => {
    const source = readFileSync(webSessionPanelPath, 'utf8');

    expect(source).toContain("[() => props.projectId, () => currentSession.value?.id ?? '']");
    expect(source).not.toContain(
      "() => [props.projectId, currentSession.value?.id ?? ''] as const"
    );
  });
});
