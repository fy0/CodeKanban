import { storeToRefs } from 'pinia';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import i18n from '@/i18n';
import { useSettingsStore } from '@/stores/settings';

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

describe('settings backup helpers in store', () => {
  beforeEach(() => {
    const localStorageMock = createStorageMock();
    setActivePinia(createPinia());
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

  it('exports client backup with persisted settings shape', () => {
    const store = useSettingsStore();
    store.updateRecentProjectsLimit(7);
    store.updateShowWebSessionReasoning(true);
    store.updateWebSessionAutoRetryDispatchPendingOnFailure(true);
    store.recordWebSessionRecentInput('Ship it');

    const payload = store.exportClientBackup('en-US', {
      includeQuickInputRecent: false,
    });

    expect(payload.locale).toBe('en-US');
    expect(payload.settings.version).toBe(6);
    expect(payload.settings.recentProjectsLimit).toBe(7);
    expect(payload.settings.showWebSessionReasoning).toBe(true);
    expect(payload.settings.webSessionAutoRetryDispatchPendingOnFailure).toBe(true);
    expect(payload.settings.webSessionQuickInput?.recent).toBeUndefined();
    expect(payload.settings.webSessionQuickInput?.recentByProject).toBeUndefined();
  });

  it('imports client backup and updates locale plus persisted storage', () => {
    const store = useSettingsStore();
    const {
      recentProjectsLimit,
      showWebSessionReasoning,
      webSessionAutoRetryDispatchPendingOnFailure,
    } = storeToRefs(store);
    store.recordWebSessionRecentInput('Keep this');
    store.recordWebSessionRecentInput('Keep project this', 'project-1');

    store.importClientBackup({
      locale: 'en-US',
      settings: {
        version: 6,
        theme: {
          primaryColor: '#123456',
          surfaceColor: '#ffffff',
          bodyColor: '#eeeeee',
          textColor: '#111111',
          terminalBg: '#000000',
          terminalFg: '#ffffff',
        },
        currentPresetId: 'light',
        followSystemTheme: 0,
        customTheme: null,
        recentProjectsLimit: 6,
        maxTerminalsPerProject: 4,
        panelShortcuts: {
          terminal: { code: 'Backquote', display: '`' },
          notepad: { code: 'Digit1', display: '1' },
        },
        webSessionQuickInput: {
          pinned: ['Plan'],
        },
        webSessionQuickInputDirectSend: true,
        terminalQuickActions: [],
        editor: {
          defaultEditor: 'vscode',
          customCommand: '',
        },
        confirmBeforeTerminalClose: false,
        showWebSessionReasoning: true,
        webSessionActivityDisplayMode: 'card',
        webSessionAutoContinueScope: 'network_only',
        webSessionAutoContinuePreset: 'gentle_stop',
        webSessionAutoContinueMaxAttempts: 0,
        webSessionAutoRetryDispatchPendingOnFailure: true,
        webSessionStreamingMarkdownThrottleMode: 'default',
        webSessionStreamingMarkdownThrottleCustomMs: 100,
        terminalThemeId: 'follow-theme',
        terminalFont: {
          fontFamily: 'Menlo',
          fontSize: 14,
          fontWeight: 'normal',
          fontWeightBold: 'bold',
          lineHeight: 1.1,
          letterSpacing: 0,
        },
        terminalWebGLRenderer: 'auto',
        defaultTerminalRenderMode: 'live',
        defaultTerminalSnapshotIntervalMs: null,
        defaultTerminalSnapshotZlibCompression: true,
        terminalConnectionPolicy: 'active-only',
        inactiveTerminalSnapshotIntervalMs: 1000,
      },
    });

    expect(recentProjectsLimit.value).toBe(6);
    expect(showWebSessionReasoning.value).toBe(true);
    expect(webSessionAutoRetryDispatchPendingOnFailure.value).toBe(true);
    expect(store.webSessionQuickInput.recent).toEqual(['Keep this']);
    expect(store.webSessionQuickInput.pinned).toEqual(['Plan']);
    expect(store.webSessionQuickInput.recentByProject).toEqual({
      'project-1': ['Keep project this'],
    });
    expect(localStorage.getItem('app-locale')).toBe('en-US');
    expect(i18n.global.locale).toBe('en-US');
  });
});
