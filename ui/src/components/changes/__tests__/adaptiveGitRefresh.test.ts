import { afterEach, describe, expect, it, vi } from 'vitest';

import { AdaptiveGitRefreshLoop } from '@/components/changes/adaptiveGitRefresh';

async function flushPromises() {
  await Promise.resolve();
  await Promise.resolve();
}

describe('AdaptiveGitRefreshLoop', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('backs foreground refreshes off after stable results', async () => {
    vi.useFakeTimers();
    const refresh = vi.fn().mockResolvedValue({ changed: false });
    const loop = new AdaptiveGitRefreshLoop({
      mode: 'foreground',
      refresh,
      random: () => 0.5,
      isHidden: () => false,
    });

    loop.start({ immediate: true });
    await flushPromises();
    expect(refresh).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(30_000);
    expect(refresh).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(refresh).toHaveBeenCalledTimes(3);
    loop.stop();
  });

  it('coalesces triggers received while a refresh is running', async () => {
    let release: (() => void) | null = null;
    const refresh = vi.fn(
      () =>
        new Promise<{ changed: boolean }>(resolve => {
          release = () => resolve({ changed: false });
        })
    );
    const loop = new AdaptiveGitRefreshLoop({
      mode: 'background',
      refresh,
      isHidden: () => false,
    });

    loop.start({ immediate: true });
    loop.trigger();
    loop.trigger();
    expect(refresh).toHaveBeenCalledTimes(1);

    release?.();
    await flushPromises();
    expect(refresh).toHaveBeenCalledTimes(2);
    loop.stop();
  });

  it('pauses while hidden and refreshes immediately when visible', async () => {
    let hidden = true;
    const refresh = vi.fn().mockResolvedValue({ changed: false });
    const loop = new AdaptiveGitRefreshLoop({
      mode: 'background',
      refresh,
      isHidden: () => hidden,
    });

    loop.start({ immediate: true });
    expect(refresh).not.toHaveBeenCalled();
    hidden = false;
    loop.handleVisibilityChange();
    await flushPromises();
    expect(refresh).toHaveBeenCalledTimes(1);
    loop.stop();
  });
});
