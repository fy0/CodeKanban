import { createPinia, setActivePinia } from 'pinia';
import { nextTick } from 'vue';
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
  http: { Get: getMethodMock, Post: postMethodMock },
}));

import { useSettingsStore } from '@/stores/settings';

describe('recent projects settings persistence', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const storage = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
      clear: () => storage.clear(),
    });
    getMethodMock.mockClear();
    postMethodMock.mockClear();
    getSendMock.mockReset().mockResolvedValue({ item: { limit: 7 } });
    postSendMock.mockReset().mockResolvedValue({ item: { limit: 4 } });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('replaces the browser cache with the server value and restores it in a fresh browser', async () => {
    localStorage.setItem('general_settings', JSON.stringify({ recentProjectsLimit: 2 }));
    let store = useSettingsStore();
    expect(store.recentProjectsLimit).toBe(2);
    expect(store.recentProjectsSettingsLoaded).toBe(false);

    await store.loadRecentProjectsSettings();

    expect(getMethodMock).toHaveBeenCalledWith('/system/recent-projects-settings', { cacheFor: 0 });
    expect(getSendMock).toHaveBeenCalledWith(true);
    expect(store.recentProjectsLimit).toBe(7);
    expect(store.recentProjectsSettingsLoaded).toBe(true);
    await nextTick();
    expect(JSON.parse(localStorage.getItem('general_settings')!).recentProjectsLimit).toBe(7);

    localStorage.clear();
    setActivePinia(createPinia());
    store = useSettingsStore();
    await store.loadRecentProjectsSettings();
    expect(store.recentProjectsLimit).toBe(7);
  });

  it('sends changes immediately and applies the value only after a successful save', async () => {
    let finishSave!: (value: { item: { limit: number } }) => void;
    postSendMock.mockImplementationOnce(
      () =>
        new Promise(resolve => {
          finishSave = resolve;
        })
    );
    const store = useSettingsStore();
    const saving = store.updateRecentProjectsLimit(4);

    expect(postMethodMock).toHaveBeenCalledWith('/system/recent-projects-settings/update', {
      limit: 4,
    });
    expect(store.recentProjectsSettingsSaving).toBe(true);
    expect(store.recentProjectsLimit).toBe(10);

    finishSave({ item: { limit: 4 } });
    await expect(saving).resolves.toBe(4);
    expect(store.recentProjectsSettingsSaving).toBe(false);
    expect(store.recentProjectsLimit).toBe(4);
  });

  it('keeps the saved value when an update fails', async () => {
    const store = useSettingsStore();
    await store.loadRecentProjectsSettings();
    postSendMock.mockRejectedValueOnce(new Error('disk unavailable'));

    await expect(store.updateRecentProjectsLimit(4)).rejects.toThrow('disk unavailable');

    expect(store.recentProjectsLimit).toBe(7);
    expect(store.recentProjectsSettingsSaving).toBe(false);
  });

  it('deduplicates startup loads and waits for them before saving a new value', async () => {
    let finishLoad!: (value: { item: { limit: number } }) => void;
    getSendMock.mockImplementationOnce(
      () =>
        new Promise(resolve => {
          finishLoad = resolve;
        })
    );
    const store = useSettingsStore();
    const loading = store.loadRecentProjectsSettings();
    const secondLoading = store.loadRecentProjectsSettings();
    const saving = store.updateRecentProjectsLimit(4);

    expect(getSendMock).toHaveBeenCalledTimes(1);
    expect(postSendMock).not.toHaveBeenCalled();
    finishLoad({ item: { limit: 7 } });
    await Promise.all([loading, secondLoading, saving]);
    expect(store.recentProjectsLimit).toBe(4);
  });

  it('allows a failed load to be retried', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    getSendMock.mockRejectedValueOnce(new Error('offline'));
    const store = useSettingsStore();
    await store.loadRecentProjectsSettings();
    expect(store.recentProjectsSettingsLoaded).toBe(false);

    await store.loadRecentProjectsSettings();
    expect(store.recentProjectsSettingsLoaded).toBe(true);
    expect(store.recentProjectsLimit).toBe(7);
  });

  it('reloads the server value after a settings backup is imported', async () => {
    const store = useSettingsStore();
    await store.loadRecentProjectsSettings();
    getSendMock.mockResolvedValueOnce({ item: { limit: 3 } });
    await store.loadRecentProjectsSettings(true);

    expect(store.recentProjectsLimit).toBe(3);
    expect(getSendMock).toHaveBeenCalledTimes(2);
  });
});
