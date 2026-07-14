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
  const store = new Map<string, string>();
  return {
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
    removeItem(key: string) {
      store.delete(key);
    },
    clear() {
      store.clear();
    },
  };
}

describe('settings daily tip', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', createStorageMock());
    getMethodMock.mockClear();
    getSendMock.mockReset();
    postMethodMock.mockClear();
    postSendMock.mockReset();
    getSendMock.mockResolvedValue({
      item: {
        enabled: false,
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('loads the daily tip setting from the server before marking it ready', async () => {
    const store = useSettingsStore();
    const { dailyTipEnabled, dailyTipSettingsLoaded } = storeToRefs(store);

    expect(dailyTipEnabled.value).toBe(false);
    expect(dailyTipSettingsLoaded.value).toBe(false);
    getSendMock.mockResolvedValue({
      item: {
        enabled: true,
      },
    });

    await store.loadDailyTipSettings();

    expect(getMethodMock).toHaveBeenCalledWith('/system/daily-tip-settings');
    expect(dailyTipEnabled.value).toBe(true);
    expect(dailyTipSettingsLoaded.value).toBe(true);
  });

  it('keeps daily tips disabled when the server response omits the setting', async () => {
    getSendMock.mockResolvedValue({});
    const store = useSettingsStore();

    await store.loadDailyTipSettings();

    expect(store.dailyTipEnabled).toBe(false);
    expect(store.dailyTipSettingsLoaded).toBe(true);
  });

  it('ignores the legacy local daily tip value before the server setting is loaded', () => {
    localStorage.setItem(
      'general_settings',
      JSON.stringify({
        dailyTipEnabled: true,
      })
    );

    const store = useSettingsStore();
    const { dailyTipEnabled, dailyTipSettingsLoaded } = storeToRefs(store);

    expect(dailyTipEnabled.value).toBe(false);
    expect(dailyTipSettingsLoaded.value).toBe(false);
  });

  it('updates the daily tip setting through the server and keeps store state in sync', async () => {
    const store = useSettingsStore();
    const { dailyTipEnabled } = storeToRefs(store);

    await store.loadDailyTipSettings();
    postSendMock.mockResolvedValue({
      item: {
        enabled: true,
      },
    });

    const saved = await store.updateDailyTipEnabled(true);

    expect(postMethodMock).toHaveBeenCalledWith('/system/daily-tip-settings/update', {
      enabled: true,
    });
    expect(saved).toEqual({ enabled: true });
    expect(dailyTipEnabled.value).toBe(true);
  });

  it('keeps the current daily tip setting unchanged when saving fails', async () => {
    const store = useSettingsStore();
    const { dailyTipEnabled } = storeToRefs(store);

    await store.loadDailyTipSettings();
    postSendMock.mockRejectedValue(new Error('save failed'));

    await expect(store.updateDailyTipEnabled(true)).rejects.toThrow('save failed');

    expect(dailyTipEnabled.value).toBe(false);
  });
});
