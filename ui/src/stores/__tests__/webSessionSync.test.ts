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

  it('skips covered revisions and backs off before retrying a failed revision', async () => {
    let now = 1_000;
    const hydrate = vi
      .fn<(request: WebSessionHydrationRequest) => Promise<void>>()
      .mockRejectedValueOnce(new Error('temporary failure'))
      .mockImplementationOnce(async request => {
        sync.markHydrated(request.sessionId, request.revision);
      });
    const onError = vi.fn();
    const sync = new WebSessionSync({
      hydrate,
      onError,
      now: () => now,
      retryBaseDelayMs: 1_000,
    });
    sync.markHydrated('session-1', '5');

    expect(sync.requestHydration('session-1', '5')).toBeNull();
    await sync.requestHydration('session-1', '6', 'tree_navigation');
    expect(onError).toHaveBeenCalledTimes(1);

    expect(sync.requestHydration('session-1', '6', 'tree_navigation')).toBeNull();
    now += 1_000;
    await sync.requestHydration('session-1', '6', 'tree_navigation');
    expect(hydrate).toHaveBeenCalledTimes(2);
    expect(sync.getHydrated('session-1')).toBe('6');
  });

  it('stops repeated stale hydration notices after a bounded attempt sequence', async () => {
    let now = 0;
    const hydrate = vi.fn(async () => undefined);
    const sync = new WebSessionSync({
      hydrate,
      now: () => now,
      retryBaseDelayMs: 100,
      maxAttemptsPerRevision: 3,
    });

    await sync.requestHydration('session-1', '10', 'history_reconciled');
    for (let index = 0; index < 20; index += 1) {
      expect(sync.requestHydration('session-1', '10', 'history_reconciled')).toBeNull();
    }
    expect(hydrate).toHaveBeenCalledTimes(1);

    now = 100;
    await sync.requestHydration('session-1', '10', 'history_reconciled');
    now = 300;
    await sync.requestHydration('session-1', '10', 'history_reconciled');
    now = 10_000;
    expect(sync.requestHydration('session-1', '10', 'history_reconciled')).toBeNull();
    expect(hydrate).toHaveBeenCalledTimes(3);

    await sync.requestHydration('session-1', '11', 'history_reconciled');
    expect(hydrate).toHaveBeenCalledTimes(3);
  });

  it('keeps a newer pending revision when an older covered notice arrives', async () => {
    const waits = [deferred(), deferred()];
    const requests: WebSessionHydrationRequest[] = [];
    const sync = new WebSessionSync({
      hydrate: async request => {
        const index = requests.push(request) - 1;
        await waits[index]!.promise;
        sync.markHydrated(request.sessionId, request.revision);
      },
      retryBaseDelayMs: 0,
      trailingDelayMs: 0,
    });
    sync.markHydrated('session-1', '5');

    sync.requestHydration('session-1', '6', 'runtime_reconciled', { mode: 'snapshot' });
    await vi.waitFor(() => expect(requests).toHaveLength(1));
    sync.requestHydration('session-1', '7');
    sync.requestHydration('session-1', '5');

    waits[0]!.resolve();
    await vi.waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[1]).toMatchObject({ revision: '7', mode: 'snapshot' });

    waits[1]!.resolve();
    await vi.waitFor(() => expect(sync.getHydrated('session-1')).toBe('7'));
  });

  it('hydrates the highest observed revision when a late notice is older', async () => {
    const requests: WebSessionHydrationRequest[] = [];
    const syncRef: { current?: WebSessionSync } = {};
    const sync = new WebSessionSync({
      hydrate: async request => {
        requests.push(request);
        syncRef.current?.markHydrated(request.sessionId, request.revision);
      },
    });
    syncRef.current = sync;
    sync.observe('session-1', '10');

    await sync.requestHydration('session-1', '6', 'late_notice', { mode: 'snapshot' });

    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({ revision: '10', mode: 'snapshot' });
    expect(sync.getHydrated('session-1')).toBe('10');
  });

  it('automatically retries one unresolved revision only within the bounded budget', async () => {
    vi.useFakeTimers();
    try {
      const hydrate = vi.fn(async () => undefined);
      const sync = new WebSessionSync({
        hydrate,
        retryBaseDelayMs: 10,
        trailingDelayMs: 0,
        maxAttemptsPerWindow: 3,
        attemptWindowMs: 1_000,
      });

      sync.requestHydration('session-1', '10', 'stale_response', { mode: 'snapshot' });
      await vi.runAllTicks();
      expect(hydrate).toHaveBeenCalledTimes(1);

      await vi.advanceTimersByTimeAsync(10);
      await vi.runAllTicks();
      await vi.advanceTimersByTimeAsync(20);
      await vi.runAllTicks();

      expect(hydrate).toHaveBeenCalledTimes(3);
      await vi.advanceTimersByTimeAsync(1_000);
      expect(hydrate).toHaveBeenCalledTimes(3);
    } finally {
      vi.useRealTimers();
    }
  });

  it('bounds a hot stream of newer revisions without resetting the attempt budget', async () => {
    vi.useFakeTimers();
    try {
      const syncRef: { current?: WebSessionSync } = {};
      const hydrate = vi.fn(async (request: WebSessionHydrationRequest) => {
        syncRef.current?.requestHydration(
          request.sessionId,
          (BigInt(request.revision) + 1n).toString(),
          'runtime_reconciled'
        );
      });
      const sync = new WebSessionSync({
        hydrate,
        retryBaseDelayMs: 10,
        trailingDelayMs: 1,
        maxAttemptsPerWindow: 3,
        attemptWindowMs: 10_000,
      });
      syncRef.current = sync;

      sync.requestHydration('session-1', '10');
      await vi.runAllTicks();
      for (let revision = 11; revision < 100; revision += 1) {
        sync.requestHydration('session-1', String(revision));
      }

      await vi.advanceTimersByTimeAsync(1_000);
      expect(hydrate).toHaveBeenCalledTimes(3);
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not reset the budget when each snapshot is one revision behind', async () => {
    vi.useFakeTimers();
    try {
      const syncRef: { current?: WebSessionSync } = {};
      const hydrate = vi.fn(async (request: WebSessionHydrationRequest) => {
        const laggingRevision = BigInt(request.revision) - 1n;
        syncRef.current?.markHydrated(request.sessionId, laggingRevision.toString());
        syncRef.current?.requestHydration(
          request.sessionId,
          (BigInt(request.revision) + 1n).toString(),
          'history_reconciled',
          { mode: 'snapshot' }
        );
      });
      const sync = new WebSessionSync({
        hydrate,
        retryBaseDelayMs: 10,
        trailingDelayMs: 1,
        maxAttemptsPerWindow: 3,
        attemptWindowMs: 10_000,
      });
      syncRef.current = sync;
      sync.markHydrated('session-1', '1');
      sync.requestHydration('session-1', '2', 'history_reconciled', { mode: 'snapshot' });

      await vi.runAllTicks();
      await vi.advanceTimersByTimeAsync(1_000);
      expect(hydrate).toHaveBeenCalledTimes(3);
    } finally {
      vi.useRealTimers();
    }
  });

  it('aborts queued hydration when its session loses focus', async () => {
    let requestSignal: AbortSignal | null = null;
    const onError = vi.fn();
    const sync = new WebSessionSync({
      hydrate: request =>
        new Promise<void>((_resolve, reject) => {
          requestSignal = request.signal;
          request.signal.addEventListener(
            'abort',
            () => {
              const error = new Error('aborted');
              error.name = 'AbortError';
              reject(error);
            },
            { once: true }
          );
        }),
      onError,
    });

    const hydration = sync.requestHydration('session-1', '10', 'history_reconciled');
    await vi.waitFor(() => expect(requestSignal).not.toBeNull());

    sync.cancelHydration('session-1');

    expect(requestSignal?.aborted).toBe(true);
    await hydration;
    expect(onError).not.toHaveBeenCalled();
  });
});
