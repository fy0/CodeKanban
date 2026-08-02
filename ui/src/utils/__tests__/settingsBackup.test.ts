import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';

import { useAppStore } from '@/stores/app';
import {
  SETTINGS_BACKUP_KIND,
  SETTINGS_BACKUP_SCHEMA_VERSION,
  buildImportableSettingsBackup,
  buildSettingsBackupFile,
  formatSettingsBackupFileName,
  hasSettingsBackupContent,
  parseSettingsBackupJSON,
} from '@/utils/settingsBackup';

describe('settingsBackup helpers', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-06-10T08:00:00Z'));
  });

  it('builds a complete backup file with server and client payloads', () => {
    const appStore = useAppStore();
    appStore.setAppInfo({
      name: 'Code Kanban',
      version: '1.2.3',
      channel: 'stable',
    });

    const backup = buildSettingsBackupFile({
      serverBackup: {
        backupSchemaVersion: SETTINGS_BACKUP_SCHEMA_VERSION,
        backupKind: SETTINGS_BACKUP_KIND,
        createdAt: '2026-06-09T00:00:00Z',
        sourceApp: {
          name: 'Code Kanban',
          version: '1.2.3',
          channel: 'stable',
        },
        payload: {
          server: {
            aiAssistantStatus: {
              claudeCode: true,
              codex: true,
              qwenCode: true,
              gemini: false,
              cursor: false,
              copilot: false,
            },
            developer: {
              enableTerminalScrollback: false,
              renameSessionTitleEachCommand: false,
              enableTerminalStateSnapshot: true,
              webSessionCodexDefaultModel: 'default',
              webSessionCodexDefaultReasoningEffort: 'default',
              webSessionCodexDefaultPermissionLevel: 'default',
              webSessionCodexDefaultSyncMode: 'default',
              webSessionActiveCallTimeout: {
                enabledMode: 'default',
                timeoutMode: 'default',
                customTimeoutSeconds: 120,
                promptTemplate: 'Continue',
                callKinds: {
                  useDefault: true,
                  mcp: true,
                  command: false,
                  tool: true,
                },
              },
            },
            pageTitle: 'Staging Board',
            dailyTip: { enabled: true },
            webSessionQuickInput: { pinned: ['continue'], recent: [] },
            worktree: { globalBaseDir: '', globalDirNamePattern: '{projectName}-{branch}' },
            terminalShell: { platform: 'linux', shell: '/bin/bash' },
            authAccess: {
              accessRules: {
                bypassIPs: [],
                bypassDomains: [],
                forceAuthIPs: [],
                forceAuthDomains: [],
              },
              proxyHeader: 'X-Forwarded-For',
              trustedProxies: [],
            },
          },
        },
      },
      clientPayload: {
        locale: 'zh-CN',
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
          recentProjectsLimit: 10,
          maxTerminalsPerProject: 12,
          panelShortcuts: {
            terminal: { code: 'Backquote', display: '`' },
            notepad: { code: 'Digit1', display: '1' },
          },
          webSessionQuickInputDirectSend: false,
          terminalQuickActions: [],
          editor: { defaultEditor: 'vscode', customCommand: '' },
          confirmBeforeTerminalClose: true,
          showWebSessionReasoning: false,
          webSessionActivityDisplayMode: 'default',
          webSessionAutoContinueScope: 'network_only',
          webSessionAutoContinuePreset: 'gentle_stop',
          webSessionAutoContinueMaxAttempts: 0,
          webSessionAutoRetryDispatchPendingOnFailure: false,
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
      },
      exportOptions: {
        includeServer: true,
        includeClient: true,
        includeSecurityAccess: false,
        includeQuickInputRecent: false,
        fileNameRule: 'app-version-datetime',
        includeMetadata: true,
      },
    });

    expect(backup.backupSchemaVersion).toBe(SETTINGS_BACKUP_SCHEMA_VERSION);
    expect(backup.backupKind).toBe(SETTINGS_BACKUP_KIND);
    expect(backup.sourceApp.version).toBe('1.2.3');
    expect(backup.payload.server?.terminalShell.shell).toBe('/bin/bash');
    expect(backup.payload.server?.pageTitle).toBe('Staging Board');
    expect(backup.payload.server?.authAccess).toBeUndefined();
    expect(backup.payload.server?.webSessionQuickInput?.recent).toBeUndefined();
    expect(backup.payload.client?.locale).toBe('zh-CN');
    expect(backup.createdAt).toBe('2026-06-10T08:00:00.000Z');
    expect(backup.meta?.description).toContain('quickInputRecent=no');
    expect(hasSettingsBackupContent(backup)).toBe(true);
  });

  it('parses backup JSON text', () => {
    const parsed = parseSettingsBackupJSON(
      JSON.stringify({
        backupSchemaVersion: SETTINGS_BACKUP_SCHEMA_VERSION,
        backupKind: 'settings',
        createdAt: '2026-06-10T08:00:00Z',
        sourceApp: { name: 'Code Kanban', version: '1.0.0', channel: 'stable' },
        payload: {
          client: {
            locale: 'en-US',
          },
        },
      })
    );

    expect(parsed.backupSchemaVersion).toBe(SETTINGS_BACKUP_SCHEMA_VERSION);
    expect(parsed.backupKind).toBe('settings');
  });

  it('builds importable backup filtered by selected sections', () => {
    const filtered = buildImportableSettingsBackup(
      {
        backupSchemaVersion: SETTINGS_BACKUP_SCHEMA_VERSION,
        backupKind: SETTINGS_BACKUP_KIND,
        sourceApp: { name: 'Code Kanban', version: '1.2.3', channel: 'stable' },
        payload: {
          server: {
            pageTitle: 'Staging Board',
            dailyTip: { enabled: false },
            webSessionQuickInput: {
              pinned: ['continue'],
              recent: ['draft'],
            },
          },
          client: {
            locale: 'zh-CN',
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
              recentProjectsLimit: 10,
              maxTerminalsPerProject: 12,
              panelShortcuts: {
                terminal: { code: 'Backquote', display: '`' },
                notepad: { code: 'Digit1', display: '1' },
              },
              webSessionQuickInput: {
                pinned: ['continue'],
                recent: ['draft'],
              },
              webSessionQuickInputDirectSend: false,
              terminalQuickActions: [],
              editor: { defaultEditor: 'vscode', customCommand: '' },
              confirmBeforeTerminalClose: true,
              showWebSessionReasoning: false,
              webSessionActivityDisplayMode: 'default',
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
          },
        },
      },
      [
        'server.pageTitle',
        'server.dailyTip',
        'server.webSessionQuickInput.pinned',
        'client.settings',
      ]
    );

    expect(filtered.payload.server?.pageTitle).toBe('Staging Board');
    expect(filtered.payload.server?.dailyTip?.enabled).toBe(false);
    expect(filtered.payload.server?.webSessionQuickInput?.pinned).toEqual(['continue']);
    expect(filtered.payload.server?.webSessionQuickInput?.recent).toBeUndefined();
    expect(filtered.payload.client?.locale).toBeUndefined();
    expect(filtered.payload.client?.settings?.webSessionQuickInput?.recent).toBeUndefined();
  });

  it('formats backup file names using selected rule', () => {
    expect(
      formatSettingsBackupFileName({
        appInfo: { version: '1.2.3', channel: 'stable' },
        rule: 'channel-app-version-datetime',
        createdAt: '2026-06-10T08:00:00Z',
      })
    ).toBe('codekanban-settings-stable-v1.2.3-20260610-080000.json');
  });
});
