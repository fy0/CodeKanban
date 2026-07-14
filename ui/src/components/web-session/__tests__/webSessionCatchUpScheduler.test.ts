import { afterEach, describe, expect, it, vi } from 'vitest';

import { createWebSessionCatchUpScheduler } from '@/components/web-session/webSessionCatchUpScheduler';

describe('webSessionCatchUpScheduler', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('coalesces recovery bursts into one trailing call', async () => {
    vi.useFakeTimers();
    const run = vi.fn();
    const scheduler = createWebSessionCatchUpScheduler(run, 150);

    scheduler.schedule('document-visible');
    scheduler.schedule('window-focus');
    scheduler.schedule('window-pageshow');
    scheduler.schedule('panel-active');
    scheduler.schedule('event-stream-recovered');

    await vi.advanceTimersByTimeAsync(149);
    expect(run).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(run).toHaveBeenCalledOnce();
    expect(run).toHaveBeenCalledWith('event-stream-recovered');
  });

  it('cancels a pending recovery', async () => {
    vi.useFakeTimers();
    const run = vi.fn();
    const scheduler = createWebSessionCatchUpScheduler(run, 150);
    scheduler.schedule('window-focus');
    scheduler.cancel();

    await vi.advanceTimersByTimeAsync(150);
    expect(run).not.toHaveBeenCalled();
  });
});
