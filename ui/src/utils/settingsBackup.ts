import { extractItem } from '@/api/response';
import { http } from '@/api/http';
import type { AIAssistantStatusConfig, DeveloperConfig, WorktreeConfig } from '@/types/models';
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

export const SETTINGS_BACKUP_SCHEMA_VERSION = 2;
export const SETTINGS_BACKUP_KIND = 'settings';
export const SETTINGS_STORAGE_KEY = 'general_settings';
export const APP_LOCALE_STORAGE_KEY = 'app-locale';

export type SettingsBackupFileNameRule =
  | 'app-version-date'
  | 'app-version-datetime'
  | 'channel-app-version-datetime';

export type SettingsBackupImportMode = 'preview-only' | 'preview-confirm' | 'direct-import';
export type SettingsBackupVersionWarningMode = 'standard' | 'strict';

export interface SettingsBackupShellConfig {
  platform: string;
  shell: string;
}

export interface SettingsBackupQuickInputSection {
  pinned?: string[];
  recent?: string[];
}

export interface SettingsBackupServerPayload {
  aiAssistantStatus?: AIAssistantStatusConfig;
  developer?: DeveloperConfig;
  pageTitle?: string;
  dailyTip?: {
    enabled: boolean;
  };
  webSessionQuickInput?: SettingsBackupQuickInputSection;
  worktree?: WorktreeConfig;
  terminalShell?: SettingsBackupShellConfig;
  authAccess?: AuthAccessConfig;
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
  webSessionQuickInput?: Partial<WebSessionQuickInputSettings>;
  webSessionQuickInputDirectSend: boolean;
  terminalQuickActions: TerminalQuickAction[];
  editor: EditorSettings;
  confirmBeforeTerminalClose: boolean;
  showWebSessionReasoning: boolean;
  webSessionActivityDisplayMode: WebSessionActivityDisplayMode;
  webSessionAutoContinueScope: WebSessionAutoContinueScope;
  webSessionAutoContinuePreset: WebSessionAutoContinuePreset;
  webSessionAutoContinueMaxAttempts: number;
  webSessionAutoRetryDispatchPendingOnFailure: boolean;
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
  locale?: string;
  settings?: SettingsBackupClientSettings;
}

export interface SettingsBackupSourceApp {
  name: string;
  version: string;
  channel: string;
}

export interface SettingsBackupExportOptions {
  includeServer: boolean;
  includeClient: boolean;
  includeSecurityAccess: boolean;
  includeQuickInputRecent: boolean;
  fileNameRule: SettingsBackupFileNameRule;
  includeMetadata: boolean;
}

export interface SettingsBackupMeta {
  description?: string;
  exportOptions?: SettingsBackupExportOptions;
}

export interface SettingsBackupFile {
  backupSchemaVersion: number;
  backupKind: string;
  createdAt?: string;
  sourceApp: SettingsBackupSourceApp;
  meta?: SettingsBackupMeta;
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
  exportOptions: SettingsBackupExportOptions;
}): SettingsBackupFile {
  const appStore = useAppStore();
  const filteredServer = filterServerPayloadForExport(
    options.serverBackup.payload.server,
    options.exportOptions
  );
  const filteredClient = filterClientPayloadForExport(options.clientPayload, options.exportOptions);
  const createdAt = options.exportOptions.includeMetadata ? new Date().toISOString() : undefined;
  const meta = options.exportOptions.includeMetadata
    ? {
        description: buildSettingsBackupDescription(options.exportOptions),
        exportOptions: { ...options.exportOptions },
      }
    : undefined;

  return {
    backupSchemaVersion: SETTINGS_BACKUP_SCHEMA_VERSION,
    backupKind: SETTINGS_BACKUP_KIND,
    createdAt,
    sourceApp: {
      name: appStore.appInfo.name,
      version: appStore.appInfo.version,
      channel: appStore.appInfo.channel,
    },
    meta,
    payload: {
      ...(filteredServer ? { server: filteredServer } : {}),
      ...(filteredClient ? { client: filteredClient } : {}),
    },
  };
}

export function hasSettingsBackupContent(backup: SettingsBackupFile | null | undefined) {
  if (!backup) {
    return false;
  }
  return Boolean(
    hasServerPayloadContent(backup.payload.server) || hasClientPayloadContent(backup.payload.client)
  );
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

export function formatSettingsBackupFileName(options: {
  appInfo?: Partial<SettingsBackupSourceApp> | null;
  rule: SettingsBackupFileNameRule;
  createdAt?: string | null;
}) {
  const version = sanitizeFileSegment(options.appInfo?.version || '0.0.0');
  const channel = sanitizeFileSegment(options.appInfo?.channel || 'unknown');
  const createdAt = options.createdAt ? new Date(options.createdAt) : new Date();
  const date = `${createdAt.getUTCFullYear()}${String(createdAt.getUTCMonth() + 1).padStart(2, '0')}${String(createdAt.getUTCDate()).padStart(2, '0')}`;
  const datetime = `${date}-${String(createdAt.getUTCHours()).padStart(2, '0')}${String(createdAt.getUTCMinutes()).padStart(2, '0')}${String(createdAt.getUTCSeconds()).padStart(2, '0')}`;

  switch (options.rule) {
    case 'app-version-date':
      return `codekanban-settings-v${version}-${date}.json`;
    case 'channel-app-version-datetime':
      return `codekanban-settings-${channel}-v${version}-${datetime}.json`;
    default:
      return `codekanban-settings-v${version}-${datetime}.json`;
  }
}

export function buildImportableSettingsBackup(
  backup: SettingsBackupFile,
  selectedSectionKeys: string[]
): SettingsBackupFile {
  const cloned = cloneBackup(backup);
  const selectedKeys = new Set(selectedSectionKeys);
  const server = cloned.payload.server;
  const client = cloned.payload.client;

  if (server) {
    if (!selectedKeys.has('server.aiAssistantStatus')) {
      delete server.aiAssistantStatus;
    }
    if (!selectedKeys.has('server.developer')) {
      delete server.developer;
    }
    if (!selectedKeys.has('server.pageTitle')) {
      delete server.pageTitle;
    }
    if (!selectedKeys.has('server.dailyTip')) {
      delete server.dailyTip;
    }
    if (server.webSessionQuickInput) {
      if (!selectedKeys.has('server.webSessionQuickInput.pinned')) {
        delete server.webSessionQuickInput.pinned;
      }
      if (!selectedKeys.has('server.webSessionQuickInput.recent')) {
        delete server.webSessionQuickInput.recent;
      }
      if (!hasQuickInputSectionContent(server.webSessionQuickInput)) {
        delete server.webSessionQuickInput;
      }
    }
    if (!selectedKeys.has('server.worktree')) {
      delete server.worktree;
    }
    if (!selectedKeys.has('server.terminalShell')) {
      delete server.terminalShell;
    }
    if (!selectedKeys.has('server.authAccess')) {
      delete server.authAccess;
    }
    if (!hasServerPayloadContent(server)) {
      delete cloned.payload.server;
    }
  }

  if (client) {
    if (!selectedKeys.has('client.locale')) {
      delete client.locale;
    }
    if (!selectedKeys.has('client.settings')) {
      delete client.settings;
    } else if (client.settings?.webSessionQuickInput) {
      if (!selectedKeys.has('server.webSessionQuickInput.pinned')) {
        delete client.settings.webSessionQuickInput.pinned;
      }
      if (!selectedKeys.has('server.webSessionQuickInput.recent')) {
        delete client.settings.webSessionQuickInput.recent;
      }
      if (!hasQuickInputSectionContent(client.settings.webSessionQuickInput)) {
        delete client.settings.webSessionQuickInput;
      }
    }
    if (!hasClientPayloadContent(client)) {
      delete cloned.payload.client;
    }
  }

  return cloned;
}

function filterServerPayloadForExport(
  payload: SettingsBackupServerPayload | undefined,
  options: SettingsBackupExportOptions
) {
  if (!payload || !options.includeServer) {
    return undefined;
  }

  const cloned = cloneBackup({ payload: { server: payload } } as SettingsBackupFile).payload.server;
  if (!cloned) {
    return undefined;
  }

  if (!options.includeSecurityAccess) {
    delete cloned.authAccess;
  }
  if (cloned.webSessionQuickInput && !options.includeQuickInputRecent) {
    delete cloned.webSessionQuickInput.recent;
    if (!hasQuickInputSectionContent(cloned.webSessionQuickInput)) {
      delete cloned.webSessionQuickInput;
    }
  }

  return hasServerPayloadContent(cloned) ? cloned : undefined;
}

function filterClientPayloadForExport(
  payload: SettingsBackupClientPayload | undefined,
  options: SettingsBackupExportOptions
) {
  if (!payload || !options.includeClient) {
    return undefined;
  }

  const cloned = cloneBackup({ payload: { client: payload } } as SettingsBackupFile).payload.client;
  if (!cloned) {
    return undefined;
  }

  if (cloned.settings?.webSessionQuickInput && !options.includeQuickInputRecent) {
    delete cloned.settings.webSessionQuickInput.recent;
    if (!hasQuickInputSectionContent(cloned.settings.webSessionQuickInput)) {
      delete cloned.settings.webSessionQuickInput;
    }
  }

  return hasClientPayloadContent(cloned) ? cloned : undefined;
}

function buildSettingsBackupDescription(options: SettingsBackupExportOptions) {
  return `Settings backup: server=${options.includeServer ? 'yes' : 'no'}, client=${options.includeClient ? 'yes' : 'no'}, securityAccess=${options.includeSecurityAccess ? 'yes' : 'no'}, quickInputRecent=${options.includeQuickInputRecent ? 'yes' : 'no'}.`;
}

function hasServerPayloadContent(payload: SettingsBackupServerPayload | undefined) {
  if (!payload) {
    return false;
  }
  return Boolean(
    payload.aiAssistantStatus ||
      payload.developer ||
      typeof payload.pageTitle === 'string' ||
      payload.dailyTip ||
      hasQuickInputSectionContent(payload.webSessionQuickInput) ||
      payload.worktree ||
      payload.terminalShell ||
      payload.authAccess
  );
}

function hasClientPayloadContent(payload: SettingsBackupClientPayload | undefined) {
  if (!payload) {
    return false;
  }
  return Boolean(payload.locale || payload.settings);
}

function hasQuickInputSectionContent(
  value: Partial<WebSessionQuickInputSettings> | SettingsBackupQuickInputSection | undefined
) {
  if (!value) {
    return false;
  }
  return 'pinned' in value || 'recent' in value;
}

function sanitizeFileSegment(value: string) {
  return (
    String(value || '')
      .trim()
      .replace(/[^a-zA-Z0-9._-]+/g, '-')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '') || 'unknown'
  );
}

function cloneBackup<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}
