import { storeToRefs } from 'pinia';
import { createPinia, setActivePinia } from 'pinia';
import { nextTick } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { getPresetById } from '@/constants/themes';
import { useSettingsStore } from '@/stores/settings';

const SETTINGS_STORAGE_KEY = 'general_settings';

function createStorageMock(initial: Record<string, string> = {}) {
  const store = new Map<string, string>(Object.entries(initial));
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

describe('settings theme storage', () => {
  let localStorageMock: ReturnType<typeof createStorageMock>;

  beforeEach(() => {
    setActivePinia(createPinia());
    localStorageMock = createStorageMock();
    vi.stubGlobal('localStorage', localStorageMock);
    vi.stubGlobal('window', {
      localStorage: localStorageMock,
      matchMedia: vi.fn().mockReturnValue({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('defaults follow-system theme to disabled when no settings exist', () => {
    const store = useSettingsStore();
    const { followSystemTheme } = storeToRefs(store);

    expect(followSystemTheme.value).toBe(false);
  });

  it('migrates legacy settings to the default follow-system tier without resetting the active preset', () => {
    const darkPreset = getPresetById('dark');
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        currentPresetId: 'dark',
        followSystemTheme: true,
        theme: darkPreset?.colors,
      })
    );

    const store = useSettingsStore();
    const { activeTheme, currentPresetId, followSystemTheme } = storeToRefs(store);

    expect(followSystemTheme.value).toBe(false);
    expect(currentPresetId.value).toBe('dark');
    expect(activeTheme.value.bodyColor).toBe(darkPreset?.colors.bodyColor);

    const persisted = JSON.parse(localStorageMock.getItem(SETTINGS_STORAGE_KEY) ?? '{}') as {
      version?: number;
      followSystemTheme?: number;
    };

    expect(persisted.version).toBe(7);
    expect(persisted.followSystemTheme).toBe(-1);
  });

  it('persists the explicit enabled tier after a migrated user re-enables follow-system mode', async () => {
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        currentPresetId: 'dark',
        followSystemTheme: true,
      })
    );

    let store = useSettingsStore();
    let refs = storeToRefs(store);

    expect(refs.followSystemTheme.value).toBe(false);

    store.toggleFollowSystemTheme(true);
    await nextTick();

    const persisted = JSON.parse(localStorageMock.getItem(SETTINGS_STORAGE_KEY) ?? '{}') as {
      version?: number;
      followSystemTheme?: number;
    };

    expect(persisted.version).toBe(7);
    expect(persisted.followSystemTheme).toBe(1);

    setActivePinia(createPinia());
    store = useSettingsStore();
    refs = storeToRefs(store);

    expect(refs.followSystemTheme.value).toBe(true);
  });

  it('migrates version-1 boolean settings to the default tier', () => {
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        currentPresetId: 'light',
        followSystemTheme: true,
      })
    );

    const store = useSettingsStore();
    const { followSystemTheme } = storeToRefs(store);

    expect(followSystemTheme.value).toBe(false);

    const persisted = JSON.parse(localStorageMock.getItem(SETTINGS_STORAGE_KEY) ?? '{}') as {
      version?: number;
      followSystemTheme?: number;
    };

    expect(persisted.version).toBe(7);
    expect(persisted.followSystemTheme).toBe(-1);
  });

  it('keeps follow-system mode enabled for version-2 settings', () => {
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        version: 2,
        currentPresetId: 'light',
        followSystemTheme: 1,
      })
    );

    const store = useSettingsStore();
    const { followSystemTheme } = storeToRefs(store);

    expect(followSystemTheme.value).toBe(true);

    const persisted = JSON.parse(localStorageMock.getItem(SETTINGS_STORAGE_KEY) ?? '{}') as {
      version?: number;
      followSystemTheme?: number;
    };

    expect(persisted.version).toBe(7);
    expect(persisted.followSystemTheme).toBe(1);
  });

  it('drops floating terminal settings and floating theme colors during v3 migration', () => {
    const lightPreset = getPresetById('light');
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        version: 2,
        currentPresetId: 'light',
        followSystemTheme: 1,
        terminalDisplayMode: 'floating',
        theme: {
          ...lightPreset?.colors,
          terminalFloatingButtonBg: '#111111',
          terminalFloatingButtonFg: '#fefefe',
        },
        customTheme: {
          ...lightPreset?.colors,
          terminalFloatingButtonBg: '#222222',
          terminalFloatingButtonFg: '#ededed',
        },
      })
    );

    const store = useSettingsStore();
    const { activeTheme, customTheme, followSystemTheme } = storeToRefs(store);

    expect(followSystemTheme.value).toBe(true);
    expect((activeTheme.value as Record<string, unknown>).terminalFloatingButtonBg).toBeUndefined();
    expect((customTheme.value as Record<string, unknown>).terminalFloatingButtonFg).toBeUndefined();

    const persisted = JSON.parse(localStorageMock.getItem(SETTINGS_STORAGE_KEY) ?? '{}') as {
      version?: number;
      terminalDisplayMode?: string;
      theme?: Record<string, unknown>;
      customTheme?: Record<string, unknown>;
    };

    expect(persisted.version).toBe(7);
    expect(persisted.terminalDisplayMode).toBeUndefined();
    expect(persisted.theme?.terminalFloatingButtonBg).toBeUndefined();
    expect(persisted.customTheme?.terminalFloatingButtonFg).toBeUndefined();
  });

  it('defaults web session activity display mode to the built-in style', () => {
    const store = useSettingsStore();
    const { webSessionActivityDisplayMode } = storeToRefs(store);

    expect(webSessionActivityDisplayMode.value).toBe('default');
  });

  it('preserves custom web session activity display mode from storage', () => {
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        version: 4,
        webSessionActivityDisplayMode: 'card',
      })
    );

    const store = useSettingsStore();
    const { webSessionActivityDisplayMode } = storeToRefs(store);

    expect(webSessionActivityDisplayMode.value).toBe('card');
  });

  it('sanitizes invalid web session activity display mode from storage', () => {
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        version: 4,
        webSessionActivityDisplayMode: 'floating',
      })
    );

    const store = useSettingsStore();
    const { webSessionActivityDisplayMode } = storeToRefs(store);

    expect(webSessionActivityDisplayMode.value).toBe('default');
  });

  it('persists updated web session activity display mode', async () => {
    const store = useSettingsStore();
    const { webSessionActivityDisplayMode } = storeToRefs(store);

    store.updateWebSessionActivityDisplayMode('text');
    await nextTick();

    expect(webSessionActivityDisplayMode.value).toBe('text');

    const persisted = JSON.parse(localStorageMock.getItem(SETTINGS_STORAGE_KEY) ?? '{}') as {
      webSessionActivityDisplayMode?: string;
    };

    expect(persisted.webSessionActivityDisplayMode).toBe('text');
  });

  it('drops legacy browser auto-retry defaults during the v7 migration', () => {
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        version: 6,
        webSessionAutoContinueScope: 'all_failures',
        webSessionAutoContinuePreset: 'sustain_60s',
        webSessionAutoContinueMaxAttempts: 88,
        webSessionAutoRetryDispatchPendingOnFailure: true,
      })
    );

    useSettingsStore();

    const persisted = JSON.parse(localStorageMock.getItem(SETTINGS_STORAGE_KEY) ?? '{}') as {
      version?: number;
      webSessionAutoContinueScope?: string;
      webSessionAutoContinuePreset?: string;
      webSessionAutoContinueMaxAttempts?: number;
      webSessionAutoRetryDispatchPendingOnFailure?: boolean;
    };

    expect(persisted.version).toBe(7);
    expect(persisted.webSessionAutoContinueScope).toBeUndefined();
    expect(persisted.webSessionAutoContinuePreset).toBeUndefined();
    expect(persisted.webSessionAutoContinueMaxAttempts).toBeUndefined();
    expect(persisted.webSessionAutoRetryDispatchPendingOnFailure).toBeUndefined();
  });

  it('defaults web session streaming markdown cadence to the built-in profile', () => {
    const store = useSettingsStore();
    const {
      webSessionStreamingMarkdownThrottleMode,
      webSessionStreamingMarkdownThrottleCustomMs,
      webSessionStreamingMarkdownThrottleMs,
    } = storeToRefs(store);

    expect(webSessionStreamingMarkdownThrottleMode.value).toBe('default');
    expect(webSessionStreamingMarkdownThrottleCustomMs.value).toBe(100);
    expect(webSessionStreamingMarkdownThrottleMs.value).toBe(100);
  });

  it('drops the legacy local daily tip setting during settings migration', () => {
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        version: 3,
        currentPresetId: 'light',
        dailyTipEnabled: true,
      })
    );

    const store = useSettingsStore();
    const { dailyTipEnabled } = storeToRefs(store);

    expect(dailyTipEnabled.value).toBe(false);

    const persisted = JSON.parse(localStorageMock.getItem(SETTINGS_STORAGE_KEY) ?? '{}') as {
      version?: number;
      dailyTipEnabled?: boolean;
    };

    expect(persisted.version).toBe(7);
    expect(persisted.dailyTipEnabled).toBeUndefined();
  });

  it('preserves custom web session streaming markdown cadence from storage', () => {
    localStorageMock.setItem(
      SETTINGS_STORAGE_KEY,
      JSON.stringify({
        version: 4,
        webSessionStreamingMarkdownThrottleMode: 'custom',
        webSessionStreamingMarkdownThrottleCustomMs: 137,
      })
    );

    const store = useSettingsStore();
    const {
      webSessionStreamingMarkdownThrottleMode,
      webSessionStreamingMarkdownThrottleCustomMs,
      webSessionStreamingMarkdownThrottleMs,
    } = storeToRefs(store);

    expect(webSessionStreamingMarkdownThrottleMode.value).toBe('custom');
    expect(webSessionStreamingMarkdownThrottleCustomMs.value).toBe(137);
    expect(webSessionStreamingMarkdownThrottleMs.value).toBe(137);
  });
});
