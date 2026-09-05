import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/api', async () => {
  const { createAlova } = await import('alova');
  const { default: fetchAdapter } = await import('alova/fetch');
  return {
    urlBase: '',
    ApiError: class extends Error {},
    alovaInstance: createAlova({
      requestAdapter: fetchAdapter(),
      responded: response => response.json(),
    }),
  };
});

import { webSessionApi } from '@/api/webSession';

describe('web session snapshot transport cancellation', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([
    { mode: 'snapshot', abortedIndex: 0 },
    { mode: 'snapshot', abortedIndex: 1 },
    { mode: 'catch-up', abortedIndex: 0 },
    { mode: 'catch-up', abortedIndex: 1 },
  ])(
    'isolates $mode transport when consumer $abortedIndex aborts',
    async ({ mode, abortedIndex }) => {
      const requests: Array<{ signal: AbortSignal; resolve: (response: Response) => void }> = [];
      vi.stubGlobal(
        'fetch',
        vi.fn(
          (_url: string, init: RequestInit) =>
            new Promise<Response>((resolve, reject) => {
              const signal = init.signal!;
              requests.push({ signal, resolve });
              signal.addEventListener('abort', () =>
                reject(new DOMException('Aborted', 'AbortError'))
              );
            })
        )
      );
      const load = (signal: AbortSignal) =>
        mode === 'snapshot'
          ? webSessionApi.snapshot('project-transport', 'session-transport', { signal })
          : webSessionApi.catchUp('project-transport', 'session-transport', {
              signal,
              afterEventCursor: '0:0',
              historyEpoch: '1',
            });
      const firstController = new AbortController();
      const first = load(firstController.signal);
      const firstResult = first.catch(error => error);
      await vi.waitFor(() => expect(requests).toHaveLength(1));

      const secondController = new AbortController();
      const second = load(secondController.signal);
      const secondResult = second.catch(error => error);
      await new Promise(resolve => setTimeout(resolve, 0));
      const controllers = [firstController, secondController];
      controllers[abortedIndex]!.abort();
      await new Promise(resolve => setTimeout(resolve, 0));

      for (const request of requests) {
        if (!request.signal.aborted) {
          request.resolve(Response.json({ item: { revision: '1', unchanged: true } }));
        }
      }

      const results = await Promise.all([firstResult, secondResult]);
      expect(results[abortedIndex]).toMatchObject({ name: 'AbortError' });
      expect(results[1 - abortedIndex]).toEqual({ revision: '1', unchanged: true });
      expect(requests).toHaveLength(2);
    }
  );
});
