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

describe('settings page title', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', createStorageMock());
    getMethodMock.mockClear();
    getSendMock.mockReset();
    postMethodMock.mockClear();
    postSendMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('loads the instance title from the server', async () => {
    getSendMock.mockResolvedValue({ item: { title: 'Staging Board' } });
    const store = useSettingsStore();
    const { pageTitle, pageTitleSettingsLoaded } = storeToRefs(store);

    expect(pageTitle.value).toBe('Code Kanban');
    expect(pageTitleSettingsLoaded.value).toBe(false);

    await store.loadPageTitleSettings();

    expect(getMethodMock).toHaveBeenCalledWith('/system/page-title-settings');
    expect(pageTitle.value).toBe('Staging Board');
    expect(pageTitleSettingsLoaded.value).toBe(true);
  });

  it('falls back to the built-in title when the response is empty', async () => {
    getSendMock.mockResolvedValue({ item: { title: '   ' } });
    const store = useSettingsStore();

    await store.loadPageTitleSettings();

    expect(store.pageTitle).toBe('Code Kanban');
    expect(store.pageTitleSettingsLoaded).toBe(true);
  });

  it('updates the title and exposes saving state', async () => {
    let resolveSave: ((value: unknown) => void) | undefined;
    postSendMock.mockReturnValue(
      new Promise(resolve => {
        resolveSave = resolve;
      })
    );
    const store = useSettingsStore();

    const saveTask = store.updatePageTitle('  Production Board  ');
    expect(store.pageTitleSettingsSaving).toBe(true);

    resolveSave?.({ item: { title: 'Production Board' } });
    await expect(saveTask).resolves.toBe('Production Board');

    expect(postMethodMock).toHaveBeenCalledWith('/system/page-title-settings/update', {
      title: '  Production Board  ',
    });
    expect(store.pageTitle).toBe('Production Board');
    expect(store.pageTitleSettingsLoaded).toBe(true);
    expect(store.pageTitleSettingsSaving).toBe(false);
  });

  it('keeps the current title when saving fails', async () => {
    getSendMock.mockResolvedValue({ item: { title: 'Original' } });
    postSendMock.mockRejectedValue(new Error('save failed'));
    const store = useSettingsStore();
    await store.loadPageTitleSettings();

    await expect(store.updatePageTitle('Replacement')).rejects.toThrow('save failed');

    expect(store.pageTitle).toBe('Original');
    expect(store.pageTitleSettingsSaving).toBe(false);
  });
});
