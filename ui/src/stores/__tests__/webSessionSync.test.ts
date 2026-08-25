import { describe, expect, it, vi } from 'vitest';

import { WebSessionSync, type WebSessionHydrationRequest } from '@/stores/webSessionSync';

function deferred() {
  let resolve!: () => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<void>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function createSync(
  hydrate: (request: WebSessionHydrationRequest) => Promise<void> = async () => {}
) {
  return new WebSessionSync({ hydrate });
}

describe('WebSessionSync revisions', () => {
  it('keeps incremental progress separate from the full hydration baseline', () => {
    const sync = createSync();

    sync.markHydrated('session-1', '10');
    sync.markApplied('session-1', '12');

    expect(sync.getApplied('session-1')).toBe('12');
    expect(sync.getHydrated('session-1')).toBe('10');
    expect(sync.isSnapshotCurrent('session-1', '10')).toBe(true);
    expect(sync.isSnapshotCurrent('session-1', '11')).toBe(false);
  });

  it('never moves observed, applied, or hydrated revisions backwards', () => {
    const sync = createSync();

    sync.markHydrated('session-1', '20');
    sync.markApplied('session-1', '19');
    sync.observe('session-1', '18');
    sync.markHydrated('session-1', '17');

    expect(sync.getObserved('session-1')).toBe('20');
    expect(sync.getApplied('session-1')).toBe('20');
    expect(sync.getHydrated('session-1')).toBe('20');
    expect(sync.shouldApply('session-1', '19')).toBe(false);
    expect(sync.shouldApply('session-1', '20')).toBe(true);
  });

  it('clears all state for a removed session', () => {
    const sync = createSync();
    sync.markHydrated('session-1', '4');

    sync.clear('session-1');

    expect(sync.getObserved('session-1')).toBe('');
    expect(sync.getApplied('session-1')).toBe('');
    expect(sync.getHydrated('session-1')).toBe('');
  });
});

describe('WebSessionSync hydration', () => {
  it('singleflights duplicate revisions and schedules one trailing newer revision', async () => {
    const requests: WebSessionHydrationRequest[] = [];
    const waits = [deferred(), deferred()];
    const sync = createSync(async request => {
      const index = requests.push(request) - 1;
      await waits[index]!.promise;
      sync.markHydrated(request.sessionId, request.revision);
    });

    const first = sync.requestHydration('session-1', '10', 'history_reconciled');
    const duplicate = sync.requestHydration('session-1', '10', 'history_reconciled');
    const newer = sync.requestHydration('session-1', '11', 'runtime_reconciled');

    expect(duplicate).toBe(first);
    expect(newer).toBe(first);
    await vi.waitFor(() => expect(requests).toHaveLength(1));

    waits[0]!.resolve();
    await vi.waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[1]).toMatchObject({ revision: '11', reason: 'runtime_reconciled' });

    waits[1]!.resolve();
    await vi.waitFor(() => expect(sync.getHydrated('session-1')).toBe('11'));
  });

  it('skips covered revisions and allows a failed revision to be requested again', async () => {
    const hydrate = vi
      .fn<(request: WebSessionHydrationRequest) => Promise<void>>()
      .mockRejectedValueOnce(new Error('temporary failure'))
      .mockImplementationOnce(async request => {
        sync.markHydrated(request.sessionId, request.revision);
      });
    const onError = vi.fn();
    const sync = new WebSessionSync({ hydrate, onError });
    sync.markHydrated('session-1', '5');

    expect(sync.requestHydration('session-1', '5')).toBeNull();
    await sync.requestHydration('session-1', '6', 'tree_navigation');
    expect(onError).toHaveBeenCalledTimes(1);

    await sync.requestHydration('session-1', '6', 'tree_navigation');
    expect(hydrate).toHaveBeenCalledTimes(2);
    expect(sync.getHydrated('session-1')).toBe('6');
  });
});
