import { flushPromises } from '@vue/test-utils';
import { effectScope, ref } from 'vue';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { webSessionApi, type PromptCacheRenewalState } from '@/api/webSession';
import type { WebSessionSummary } from '@/types/models';
import { usePromptCacheRenewal } from '../usePromptCacheRenewal';

vi.mock('@/api/webSession', () => ({
  webSessionApi: { promptCacheRenewal: vi.fn() },
}));

const request = vi.mocked(webSessionApi.promptCacheRenewal);
const off: PromptCacheRenewalState = { availability: 'ready', remaining: 0, infinite: false };

function render() {
  const scope = effectScope();
  const session = ref({ agent: 'codex', projectId: 'project', id: 'session' } as WebSessionSummary);
  const renewal = scope.run(() => usePromptCacheRenewal(session))!;
  return { renewal, stop: () => scope.stop() };
}

afterEach(() => request.mockReset());

describe('usePromptCacheRenewal', () => {
  it.each([
    { name: 'sends queued clicks', cancel: false, actions: ['cycle', 'cycle', 'cycle'] },
    { name: 'drops queued clicks on cancel', cancel: true, actions: ['cycle', 'off'] },
  ])('$name', async ({ cancel, actions }) => {
    let resolveFirst: (state: PromptCacheRenewalState) => void;
    const firstAction = new Promise<PromptCacheRenewalState>(resolve => {
      resolveFirst = resolve;
    });
    request.mockResolvedValue(off).mockResolvedValueOnce(off).mockReturnValueOnce(firstAction);
    const { renewal, stop } = render();
    renewal.setDetailsOpen(true);
    await flushPromises();
    expect(request.mock.calls.map(call => call[2])).toEqual([undefined]);
    const submitted = renewal.setMode('cycle');
    void renewal.setMode('cycle');
    void renewal.setMode('cycle');
    if (cancel) void renewal.setMode('off');
    expect(request.mock.calls.map(call => call[2])).toEqual([undefined, 'cycle']);
    resolveFirst!({ ...off, remaining: 1 });
    await submitted;
    expect(request.mock.calls.slice(1).map(call => call[2])).toEqual(actions);
    stop();
  });
});
