import { describe, expect, it, vi } from 'vitest';

import { createWebSessionSnapshotLoadController } from '@/utils/webSessionSnapshotLoadController';

describe('webSessionSnapshotLoadController', () => {
  it('shares activation work when route synchronization selects the same session again', async () => {
    const controller = createWebSessionSnapshotLoadController();
    let resolveLoad!: () => void;
    const pending = new Promise<void>(resolve => {
      resolveLoad = resolve;
    });
    let signal!: AbortSignal;
    const load = vi.fn(async handle => {
      signal = handle.signal;
      await pending;
    });

    const first = controller.run('project:session', load);
    await Promise.resolve();
    const second = controller.run('project:session', load);

    expect(second).toBe(first);
    expect(signal.aborted).toBe(false);
    expect(load).toHaveBeenCalledOnce();
    resolveLoad();
    expect(await first).toBe(true);
  });

  it('ignores a late completion after switching away and back to the same session', async () => {
    const controller = createWebSessionSnapshotLoadController();
    let finishOld!: () => void;
    let finishNew!: () => void;
    let oldSignal!: AbortSignal;
    let newSignal!: AbortSignal;
    const old = controller.run('project:first', async handle => {
      oldSignal = handle.signal;
      await new Promise<void>(resolve => {
        finishOld = resolve;
      });
    });
    await Promise.resolve();
    const middle = controller.run('project:second', async () => undefined);
    const newest = controller.run('project:first', async handle => {
      newSignal = handle.signal;
      await new Promise<void>(resolve => {
        finishNew = resolve;
      });
    });
    await Promise.resolve();

    expect(oldSignal.aborted).toBe(true);
    finishOld();
    expect(await old).toBe(false);
    expect(await middle).toBe(false);
    expect(newSignal.aborted).toBe(false);
    expect(controller.run('project:first', async () => undefined)).toBe(newest);
    finishNew();
    expect(await newest).toBe(true);
  });

  it('invalidates shared work on cancellation and permits retry after failure', async () => {
    const controller = createWebSessionSnapshotLoadController();
    const canceled = controller.run('project:session', async () => undefined);
    controller.cancel();
    expect(await canceled).toBe(false);

    await expect(
      controller.run('project:session', async () => {
        throw new Error('load failed');
      })
    ).rejects.toThrow('load failed');
    expect(await controller.run('project:session', async () => undefined)).toBe(true);
  });

  it('keeps only the latest snapshot load current and aborts the previous one', () => {
    const controller = createWebSessionSnapshotLoadController();

    const first = controller.begin();
    const second = controller.begin();

    expect(first.signal.aborted).toBe(true);
    expect(controller.isCurrent(first)).toBe(false);
    expect(second.signal.aborted).toBe(false);
    expect(controller.isCurrent(second)).toBe(true);
  });

  it('cancels the active load and invalidates its handle', () => {
    const controller = createWebSessionSnapshotLoadController();

    const active = controller.begin();
    controller.cancel();

    expect(active.signal.aborted).toBe(true);
    expect(controller.isCurrent(active)).toBe(false);
  });

  it('releasing a stale handle does not clear the current load', () => {
    const controller = createWebSessionSnapshotLoadController();

    const first = controller.begin();
    const second = controller.begin();

    controller.release(first);

    expect(controller.isCurrent(second)).toBe(true);

    controller.release(second);

    expect(controller.isCurrent(second)).toBe(false);
  });
});
