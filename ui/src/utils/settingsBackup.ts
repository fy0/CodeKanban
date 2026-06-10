import { extractItem } from '@/api/response';
import { http } from '@/api/http';
import type {
  AIAssistantStatusConfig,
  DeveloperConfig,
  WorktreeConfig,
} from '@/types/models';
import type {
  EditorSettings,
  ShortcutSettings,
  TerminalFontSettings,
  TerminalQuickAction,
  ThemeSettings,
  WebSessionQuickInputSettings,
  WebSessionAutoContinuePreset,
  WebSessionAutoContinueScope,
  WebSessionStreamingMarkdownThrottleMode,
  FollowSystemThemeSetting,
} from '@/stores/settings';
import type { AuthAccessConfig } from '@/stores/auth';
import type { TerminalRenderMode } from '@/constants/terminalRenderMode';
import type { TerminalConnectionPolicy } from '@/constants/terminalConnectionPolicy';
import type { WebSessionActivityDisplayMode } from '@/constants/webSessionActivityDisplayMode';
import { useAppStore } from '@/stores/app';

export const SETTINGS_BACKUP_SCHEMA_VERSION = 1;
export const SETTINGS_BACKUP_KIND = 'settings';
export const SETTINGS_STORAGE_KEY = 'general_settings';
export const APP_LOCALE_STORAGE_KEY = 'app-locale';

export interface SettingsBackupShellConfig {
  platform: string;
  shell: string;
}

export interface SettingsBackupServerPayload {
  aiAssistantStatus: AIAssistantStatusConfig;
  developer: DeveloperConfig;
  dailyTip: {
    enabled: boolean;
  };
  webSessionQuickInput: WebSessionQuickInputSettings;
  worktree: WorktreeConfig;
  terminalShell: SettingsBackupShellConfig;
  authAccess: AuthAccessConfig;
}

export interface SettingsBackupClientSettings {
  version: number;
  theme: ThemeSettings;
  currentPresetId: string;
  followSystemTheme: FollowSystemThemeSetting;
  customTheme: ThemeSettings | null;
  recentProjectsLimit: number;
  maxTerminalsPerProject: number;
  panelShortcuts: ShortcutSettings;
  webSessionQuickInputDirectSend: boolean;
  terminalQuickActions: TerminalQuickAction[];
  editor: EditorSettings;
  confirmBeforeTerminalClose: boolean;
  showWebSessionReasoning: boolean;
  webSessionActivityDisplayMode: WebSessionActivityDisplayMode;
  webSessionAutoContinueScope: WebSessionAutoContinueScope;
  webSessionAutoContinuePreset: WebSessionAutoContinuePreset;
  webSessionStreamingMarkdownThrottleMode: WebSessionStreamingMarkdownThrottleMode;
  webSessionStreamingMarkdownThrottleCustomMs: number;
  terminalThemeId: string;
  terminalFont: TerminalFontSettings;
  terminalWebGLRenderer: 'auto' | 'force' | 'disable';
  defaultTerminalRenderMode: TerminalRenderMode;
  defaultTerminalSnapshotIntervalMs: number | null;
  defaultTerminalSnapshotZlibCompression: boolean;
  terminalConnectionPolicy: TerminalConnectionPolicy;
  inactiveTerminalSnapshotIntervalMs: number;
}

export interface SettingsBackupClientPayload {
  locale: string;
  settings: SettingsBackupClientSettings;
}

export interface SettingsBackupSourceApp {
  name: string;
  version: string;
  channel: string;
}

export interface SettingsBackupFile {
  backupSchemaVersion: number;
  backupKind: string;
  createdAt: string;
  sourceApp: SettingsBackupSourceApp;
  payload: {
    server?: SettingsBackupServerPayload;
    client?: SettingsBackupClientPayload;
  };
}

export interface SettingsBackupPreviewIssue {
  code: string;
  level: 'warning' | 'error';
  message: string;
}

export interface SettingsBackupPreviewSection {
  key: string;
  label: string;
  action: string;
  target: 'server' | 'client';
  changedKeys?: string[];
  warningCodes?: string[];
}

export interface SettingsBackupPreviewResult {
  backupSchemaVersion: number;
  backupKind: string;
  sourceApp: SettingsBackupSourceApp;
  currentApp: SettingsBackupSourceApp;
  canImport: boolean;
  errors: SettingsBackupPreviewIssue[] | null;
  warnings: SettingsBackupPreviewIssue[] | null;
  sections: SettingsBackupPreviewSection[] | null;
  migrated: boolean;
}

type ItemResponse<T> = {
  item?: T;
};

export async function exportServerSettingsBackup(): Promise<SettingsBackupFile> {
  const response = await http
    .Get<ItemResponse<SettingsBackupFile>>('/system/settings-backup/export')
    .send();
  const item = extractItem<SettingsBackupFile>(response);
  if (!item) {
    throw new Error('failed to export settings backup');
  }
  return item;
}

export async function previewSettingsBackup(
  backup: SettingsBackupFile
): Promise<SettingsBackupPreviewResult> {
  const response = await http
    .Post<ItemResponse<SettingsBackupPreviewResult>>('/system/settings-backup/preview', backup)
    .send();
  const item = extractItem<SettingsBackupPreviewResult>(response);
  if (!item) {
    throw new Error('failed to preview settings backup');
  }
  return item;
}

export async function importSettingsBackup(
  backup: SettingsBackupFile
): Promise<SettingsBackupPreviewResult> {
  const response = await http
    .Post<ItemResponse<SettingsBackupPreviewResult>>('/system/settings-backup/import', backup)
    .send();
  const item = extractItem<SettingsBackupPreviewResult>(response);
  if (!item) {
    throw new Error('failed to import settings backup');
  }
  return item;
}

export function buildSettingsBackupFile(options: {
  serverBackup: SettingsBackupFile;
  clientPayload: SettingsBackupClientPayload;
}): SettingsBackupFile {
  const appStore = useAppStore();
  return {
    backupSchemaVersion: SETTINGS_BACKUP_SCHEMA_VERSION,
    backupKind: SETTINGS_BACKUP_KIND,
    createdAt: new Date().toISOString(),
    sourceApp: {
      name: appStore.appInfo.name,
      version: appStore.appInfo.version,
      channel: appStore.appInfo.channel,
    },
    payload: {
      server: options.serverBackup.payload.server,
      client: options.clientPayload,
    },
  };
}

export function parseSettingsBackupJSON(text: string): SettingsBackupFile {
  const parsed = JSON.parse(text) as SettingsBackupFile;
  if (!parsed || typeof parsed !== 'object') {
    throw new Error('invalid backup JSON');
  }
  return parsed;
}

export function downloadSettingsBackupFile(backup: SettingsBackupFile, fileName: string) {
  const blob = new Blob([JSON.stringify(backup, null, 2)], { type: 'application/json' });
  const objectURL = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = objectURL;
  anchor.download = fileName;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(objectURL), 0);
}

