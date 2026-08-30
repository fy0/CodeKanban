import { createPinia, setActivePinia, storeToRefs } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

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

import { useSettingsStore } from '@/stores/settings';

function createStorageMock() {
  const values = new Map<string, string>();
  return {
    getItem(key: string) {
      return values.has(key) ? values.get(key)! : null;
    },
    setItem(key: string, value: string) {
      values.set(key, String(value));
    },
    removeItem(key: string) {
      values.delete(key);
    },
    clear() {
      values.clear();
    },
  };
}

describe('settings web session quick input', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', createStorageMock());
    getMethodMock.mockClear();
    getSendMock.mockReset();
    postMethodMock.mockClear();
    postSendMock.mockReset();
    getSendMock.mockResolvedValue({
      item: {
        pinned: ['continue'],
        globalRecent: [],
        projectRecent: [],
      },
    });
    postSendMock.mockResolvedValue(undefined);
  });

  afterEach(async () => {
    await Promise.resolve();
    vi.unstubAllGlobals();
  });

  it('falls back to default quick input settings when storage is missing', () => {
    const store = useSettingsStore();
    const { webSessionQuickInput, webSessionQuickInputDirectSend } = storeToRefs(store);

    expect(webSessionQuickInput.value).toEqual({
      pinned: ['continue'],
      recent: [],
      recentByProject: {},
    });
    expect(webSessionQuickInputDirectSend.value).toBe(false);
  });

  it('ignores and removes server-owned quick input data from local storage', () => {
    localStorage.setItem(
      'general_settings',
      JSON.stringify({
        version: 9,
        webSessionQuickInput: {
          pinned: ['Alpha', 'Beta'],
          recent: ['One'],
          recentByProject: { 'project-1': ['Project one'] },
        },
        webSessionQuickInputDirectSend: true,
      })
    );

    const store = useSettingsStore();
    const { webSessionQuickInput, webSessionQuickInputDirectSend } = storeToRefs(store);

    expect(webSessionQuickInput.value).toEqual({
      pinned: ['continue'],
      recent: [],
      recentByProject: {},
    });
    expect(webSessionQuickInputDirectSend.value).toBe(true);
    const persisted = JSON.parse(localStorage.getItem('general_settings') || '{}');
    expect(persisted.version).toBe(9);
    expect(persisted.webSessionQuickInput).toBeUndefined();
  });

  it('sanitizes pinned quick input updates', () => {
    const store = useSettingsStore();
    const { webSessionQuickInput } = storeToRefs(store);

    store.updateWebSessionQuickInputPinned(['  Build plan  ', '', 'Build plan', 'Ship it']);

    expect(webSessionQuickInput.value.pinned).toEqual(['Build plan', 'Ship it']);
  });

  it('records project prompts in both global and project histories with a limit of thirty', async () => {
    const store = useSettingsStore();
    const { webSessionQuickInput } = storeToRefs(store);

    for (let index = 1; index <= 32; index += 1) {
      await store.recordWebSessionRecentInput(`item ${index}`);
    }
    expect(webSessionQuickInput.value.recent).toHaveLength(30);
    expect(webSessionQuickInput.value.recent.slice(0, 2)).toEqual(['item 32', 'item 31']);
    expect(webSessionQuickInput.value.recent.at(-1)).toBe('item 3');

    await store.recordWebSessionRecentInput('project item 1', 'project-1');
    await store.recordWebSessionRecentInput('project item 2', 'project-1');
    await store.recordWebSessionRecentInput('project item 1', 'project-1');

    expect(webSessionQuickInput.value.recent.slice(0, 2)).toEqual([
      'project item 1',
      'project item 2',
    ]);
    expect(webSessionQuickInput.value.recentByProject['project-1']).toEqual([
      'project item 1',
      'project item 2',
    ]);
    expect(postMethodMock).toHaveBeenLastCalledWith('/system/web-session-quick-input/recent', {
      text: 'project item 1',
      projectId: 'project-1',
    });
  });

  it('loads global and project history from the scoped server view', async () => {
    getSendMock.mockResolvedValueOnce({
      item: {
        pinned: ['Plan'],
        globalRecent: ['Global prompt'],
        projectId: 'project-1',
        projectRecent: ['Project prompt'],
      },
    });
    const store = useSettingsStore();

    await store.loadWebSessionQuickInput('project-1');

    expect(getMethodMock).toHaveBeenCalledWith('/system/web-session-quick-input', {
      params: { projectId: 'project-1' },
    });
    expect(store.webSessionQuickInput).toEqual({
      pinned: ['Plan'],
      recent: ['Global prompt'],
      recentByProject: { 'project-1': ['Project prompt'] },
    });
  });

  it('updates quick input direct-send setting', () => {
    const store = useSettingsStore();
    const { webSessionQuickInputDirectSend } = storeToRefs(store);

    store.updateWebSessionQuickInputDirectSend(true);
    expect(webSessionQuickInputDirectSend.value).toBe(true);

    store.updateWebSessionQuickInputDirectSend(false);
    expect(webSessionQuickInputDirectSend.value).toBe(false);
  });

  it('saves sanitized pinned items through the dedicated endpoint', async () => {
    const store = useSettingsStore();
    await store.recordWebSessionRecentInput('item 1');
    await store.recordWebSessionRecentInput('item 2');
    postSendMock.mockResolvedValueOnce({
      item: {
        pinned: ['Build plan', 'Ship it'],
        globalRecent: ['item 2', 'item 1'],
        projectRecent: [],
      },
    });

    const saved = await store.saveWebSessionQuickInputPinned([
      '  Build plan  ',
      '',
      'Build plan',
      'Ship it',
    ]);

    expect(postMethodMock).toHaveBeenLastCalledWith(
      '/system/web-session-quick-input/pinned/update',
      { items: ['Build plan', 'Ship it'] }
    );
    expect(saved).toEqual({
      pinned: ['Build plan', 'Ship it'],
      recent: ['item 2', 'item 1'],
      recentByProject: {},
    });
    expect(store.webSessionQuickInput).toEqual(saved);
  });

  it('keeps saved pinned quick input unchanged when manual save fails', async () => {
    const store = useSettingsStore();
    postSendMock.mockRejectedValueOnce(new Error('save failed'));

    await expect(store.saveWebSessionQuickInputPinned(['Draft next step'])).rejects.toThrow(
      'save failed'
    );

    expect(store.webSessionQuickInput).toEqual({
      pinned: ['continue'],
      recent: [],
      recentByProject: {},
    });
  });
});
