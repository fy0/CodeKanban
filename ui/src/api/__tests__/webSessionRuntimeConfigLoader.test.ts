import { describe, expect, it, vi } from 'vitest';

import { createRuntimeConfigRequestLoader } from '@/api/webSessionRuntimeConfigLoader';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe('web-session runtime config request loader', () => {
  it('coalesces mounted, focus, pageshow, and visibility requests and honors its TTL', async () => {
    let now = 1_000;
    const loader = createRuntimeConfigRequestLoader<string>(60_000, () => now);
    let pending = deferred<string>();
    const fetchConfig = vi.fn((_force: boolean) => pending.promise);

    const lifecycleRequests = [
      loader.load(fetchConfig),
      loader.load(fetchConfig),
      loader.load(fetchConfig),
      loader.load(fetchConfig),
    ];
    await Promise.resolve();
    expect(fetchConfig).toHaveBeenCalledTimes(1);
    expect(fetchConfig).toHaveBeenLastCalledWith(false);

    pending.resolve('initial');
    await expect(Promise.all(lifecycleRequests)).resolves.toEqual([
      'initial',
      'initial',
      'initial',
      'initial',
    ]);

    now += 30_000;
    await expect(loader.load(fetchConfig)).resolves.toBe('initial');
    expect(fetchConfig).toHaveBeenCalledTimes(1);

    now += 31_000;
    pending = deferred<string>();
    const expiredRequests = [loader.load(fetchConfig), loader.load(fetchConfig)];
    await Promise.resolve();
    expect(fetchConfig).toHaveBeenCalledTimes(2);
    pending.resolve('refreshed');
    await expect(Promise.all(expiredRequests)).resolves.toEqual(['refreshed', 'refreshed']);
  });

  it('bypasses the client TTL for an explicit forced refresh and shares that request', async () => {
    const loader = createRuntimeConfigRequestLoader<string>(60_000);
    const initial = deferred<string>();
    const forced = deferred<string>();
    const fetchConfig = vi
      .fn<(force: boolean) => Promise<string>>()
      .mockReturnValueOnce(initial.promise)
      .mockReturnValueOnce(forced.promise);

    const initialRequest = loader.load(fetchConfig);
    initial.resolve('initial');
    await expect(initialRequest).resolves.toBe('initial');

    const forcedRequests = [
      loader.load(fetchConfig, { force: true }),
      loader.load(fetchConfig, { force: true }),
    ];
    await Promise.resolve();
    expect(fetchConfig).toHaveBeenCalledTimes(2);
    expect(fetchConfig).toHaveBeenLastCalledWith(true);
    forced.resolve('forced');
    await expect(Promise.all(forcedRequests)).resolves.toEqual(['forced', 'forced']);
  });
});
