import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getMethodMock, getSendMock, postMethodMock, postSendMock } = vi.hoisted(() => {
  const getSendMock = vi.fn();
  const postSendMock = vi.fn();
  return {
    getMethodMock: vi.fn(() => ({ send: getSendMock })),
    getSendMock,
    postMethodMock: vi.fn(() => ({ send: postSendMock })),
    postSendMock,
  };
});

vi.mock('@/api/http', () => ({
  http: {
    Get: getMethodMock,
    Post: postMethodMock,
  },
}));

import { useDeveloperConfigStore } from '@/stores/developerConfig';
import { sanitizeDeveloperConfig } from '@/utils/developerConfig';

describe('developer config store', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    getMethodMock.mockClear();
    getSendMock.mockReset();
    postMethodMock.mockClear();
    postSendMock.mockReset();
  });

  it('loads and caches normalized server defaults', async () => {
    getSendMock.mockResolvedValue({
      item: {
        webSessionCodexDefaultModel: ' custom-model ',
        webSessionCodexDefaultReasoningEffort: 'HIGH',
        webSessionCodexDefaultPermissionLevel: 'YOLO',
      },
    });
    const store = useDeveloperConfigStore();

    await store.load();
    await store.load();

    expect(getMethodMock).toHaveBeenCalledTimes(1);
    expect(store.config.webSessionCodexDefaultModel).toBe('custom-model');
    expect(store.config.webSessionCodexDefaultReasoningEffort).toBe('high');
    expect(store.config.webSessionCodexDefaultPermissionLevel).toBe('yolo');
    expect(store.loaded).toBe(true);
  });

  it('updates the shared snapshot used by all mounted consumers', async () => {
    postSendMock.mockResolvedValue({ message: 'ok' });
    const store = useDeveloperConfigStore();
    const next = sanitizeDeveloperConfig({
      webSessionCodexDefaultModel: 'gpt-5.6-terra',
      webSessionCodexDefaultReasoningEffort: 'max',
      webSessionCodexDefaultPermissionLevel: 'standard',
    });

    await store.update(next);

    expect(postMethodMock).toHaveBeenCalledWith('/system/developer-config/update', next);
    expect(store.config.webSessionCodexDefaultModel).toBe('gpt-5.6-terra');
    expect(store.config.webSessionCodexDefaultReasoningEffort).toBe('max');
    expect(store.config.webSessionCodexDefaultPermissionLevel).toBe('standard');
  });
});
