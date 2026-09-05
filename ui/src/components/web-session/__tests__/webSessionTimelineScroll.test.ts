import { describe, expect, it } from 'vitest';

import {
  createWebSessionMobileComposerScrollState,
  createWebSessionTimelineFollowState,
  resolveWebSessionMobileComposerBottomScrollAction,
  resolveWebSessionMobileComposerScrollState,
  resolveWebSessionTimelineFollowState,
  resolveWebSessionTimelineVisualAnchorScrollTop,
  shouldApplyWebSessionTimelineAutoScroll,
} from '@/components/web-session/webSessionTimelineScroll';

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

  it('keeps a visible pending input card fixed when content grows above it', () => {
    expect(
      resolveWebSessionTimelineVisualAnchorScrollTop({
        autoFollowBottom: false,
        anchorMatches: true,
        anchorWasVisible: true,
        previousOffsetPx: 180,
        currentOffsetPx: 260,
        previousClientHeight: 600,
        metrics: {
          scrollTop: 900,
          scrollHeight: 2200,
          clientHeight: 600,
        },
      })
    ).toBe(980);
  });

  it('preserves a pending input anchor across repeated small layout growth', () => {
    const firstScrollTop = resolveWebSessionTimelineVisualAnchorScrollTop({
      autoFollowBottom: false,
      anchorMatches: true,
      anchorWasVisible: true,
      previousOffsetPx: 180,
      currentOffsetPx: 192,
      previousClientHeight: 600,
      metrics: {
        scrollTop: 900,
        scrollHeight: 2200,
        clientHeight: 600,
      },
    });
    const secondScrollTop = resolveWebSessionTimelineVisualAnchorScrollTop({
      autoFollowBottom: false,
      anchorMatches: true,
      anchorWasVisible: true,
      previousOffsetPx: 180,
      currentOffsetPx: 194,
      previousClientHeight: 600,
      metrics: {
        scrollTop: firstScrollTop ?? 0,
        scrollHeight: 2202,
        clientHeight: 600,
      },
    });

    expect(firstScrollTop).toBe(912);
    expect(secondScrollTop).toBe(926);
  });

  it('supports shrinking content and clamps pending input anchor compensation', () => {
    expect(
      resolveWebSessionTimelineVisualAnchorScrollTop({
        autoFollowBottom: false,
        anchorMatches: true,
        anchorWasVisible: true,
        previousOffsetPx: 180,
        currentOffsetPx: 120,
        previousClientHeight: 600,
        metrics: {
          scrollTop: 40,
          scrollHeight: 1200,
          clientHeight: 600,
        },
      })
    ).toBe(0);
  });

  it('does not preserve an invalid pending input visual anchor', () => {
    const input = {
      autoFollowBottom: false,
      anchorMatches: true,
      anchorWasVisible: true,
      previousOffsetPx: 180,
      currentOffsetPx: 260,
      previousClientHeight: 600,
      metrics: {
        scrollTop: 900,
        scrollHeight: 2200,
        clientHeight: 600,
      },
    };

    expect(
      resolveWebSessionTimelineVisualAnchorScrollTop({ ...input, autoFollowBottom: true })
    ).toBeNull();
    expect(
      resolveWebSessionTimelineVisualAnchorScrollTop({ ...input, anchorMatches: false })
    ).toBeNull();
    expect(
      resolveWebSessionTimelineVisualAnchorScrollTop({ ...input, anchorWasVisible: false })
    ).toBeNull();
    expect(
      resolveWebSessionTimelineVisualAnchorScrollTop({
        ...input,
        metrics: { ...input.metrics, clientHeight: 640 },
      })
    ).toBeNull();
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
});
