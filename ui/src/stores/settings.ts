import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';
import {
  THEME_PRESETS,
  DEFAULT_PRESET_ID,
  getPresetById,
  getDefaultPreset,
} from '@/constants/themes';
import { DEFAULT_TERMINAL_THEME_ID } from '@/constants/terminalThemes';
import {
  DEFAULT_TERMINAL_RENDER_MODE,
  DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS,
  sanitizeTerminalRenderMode,
  sanitizeTerminalSnapshotIntervalMs,
  type TerminalRenderMode,
} from '@/constants/terminalRenderMode';
import {
  DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS,
  DEFAULT_TERMINAL_CONNECTION_POLICY,
  sanitizeTerminalConnectionPolicy,
  type TerminalConnectionPolicy,
} from '@/constants/terminalConnectionPolicy';
import { http } from '@/api/http';
import {
  DEFAULT_WEB_SESSION_ACTIVITY_DISPLAY_MODE,
  sanitizeWebSessionActivityDisplayMode,
  type WebSessionActivityDisplayMode,
} from '@/constants/webSessionActivityDisplayMode';
import dayjs from 'dayjs';
import i18n from '@/i18n';
import {
  APP_LOCALE_STORAGE_KEY,
  SETTINGS_STORAGE_KEY,
  type SettingsBackupClientPayload,
  type SettingsBackupClientSettings,
} from '@/utils/settingsBackup';

/**
 * 终端主题跟随应用主题的特殊值
 */
export const TERMINAL_THEME_FOLLOW = 'follow-theme';

export interface ThemeSettings {
  primaryColor: string;
  surfaceColor: string;
  bodyColor: string;
  textColor?: string;
  terminalBg: string;
  terminalFg: string;
  terminalTabBg?: string;
  terminalTabActiveBg?: string;
  terminalHeaderBorder?: boolean | string; // 终端 header 边框：false=无边框, true=默认边框, string=自定义边框
  // 完成提醒标签颜色
  terminalTabCompletionBg?: string;
  terminalTabCompletionBorder?: string;
  // 审批提醒标签颜色
  terminalTabApprovalBg?: string;
  terminalTabApprovalBorder?: string;
  // 空终端引导文字颜色
  terminalEmptyGuideFg?: string;
  // AI 通知按钮颜色（边框和图标）
  notificationButtonBorder?: string;
  notificationButtonFg?: string;
  // 看板相关颜色
  kanbanBoardBg?: string;
  kanbanCardBg?: string;
  // 看板边框控制
  kanbanBorderEnabled?: boolean;
}

/**
 * 终端字体设置
 */
export interface TerminalFontSettings {
  fontFamily: string;
  fontSize: number;
  fontWeight: FontWeight;
  fontWeightBold: FontWeight;
  lineHeight: number;
  letterSpacing: number;
}

/**
 * 字体粗细选项
 */
export type FontWeight =
  | 'normal'
  | 'bold'
  | '100'
  | '200'
  | '300'
  | '400'
  | '500'
  | '600'
  | '700'
  | '800'
  | '900';

export const FONT_WEIGHT_OPTIONS = [
  { value: 'normal', label: 'Normal (400)' },
  { value: '100', label: '100 - Thin' },
  { value: '200', label: '200 - Extra Light' },
  { value: '300', label: '300 - Light' },
  { value: '400', label: '400 - Regular' },
  { value: '500', label: '500 - Medium' },
  { value: '600', label: '600 - Semi Bold' },
  { value: '700', label: '700 - Bold' },
  { value: 'bold', label: 'Bold (700)' },
  { value: '800', label: '800 - Extra Bold' },
  { value: '900', label: '900 - Black' },
] as const;

/**
 * 默认字体回退链（考虑中英文显示）
 * 顺序：macOS系统字体 -> Windows流行字体 -> 中文回退 -> 通用回退
 * macOS会使用Menlo/Monaco，Windows上这两个字体不存在会跳过，继续用Cascadia Mono等
 */
export const DEFAULT_TERMINAL_FONT_FAMILY =
  'Menlo, Monaco, Cascadia Mono, JetBrains Mono, Consolas, Microsoft YaHei, PingFang SC, Noto Sans SC, monospace';

/**
 * 常用等宽字体列表
 */
export const TERMINAL_FONT_OPTIONS = [
  { value: '', label: '系统默认' },
  // 推荐字体（排在最前）
  { value: 'Cascadia Mono, Microsoft YaHei, PingFang SC, monospace', label: 'Cascadia Mono' },
  { value: 'JetBrains Mono, Microsoft YaHei, PingFang SC, monospace', label: 'JetBrains Mono' },
  { value: 'Consolas, Microsoft YaHei, PingFang SC, monospace', label: 'Consolas' },
  // macOS 系统字体
  { value: 'Menlo, Monaco, PingFang SC, monospace', label: 'Menlo (macOS)' },
  { value: 'Monaco, Menlo, PingFang SC, monospace', label: 'Monaco (macOS)' },
  { value: 'SF Mono, Menlo, Monaco, PingFang SC, monospace', label: 'SF Mono (macOS)' },
  // 专为中英文设计的等宽字体
  { value: 'Sarasa Mono SC, monospace', label: 'Sarasa Mono SC (更纱黑体)' },
  { value: 'Source Han Mono SC, monospace', label: 'Source Han Mono (思源等宽)' },
  // 其他流行的英文等宽字体
  { value: 'Cascadia Code, Microsoft YaHei, PingFang SC, monospace', label: 'Cascadia Code' },
  { value: 'Fira Code, Microsoft YaHei, PingFang SC, monospace', label: 'Fira Code' },
  { value: 'Source Code Pro, Microsoft YaHei, PingFang SC, monospace', label: 'Source Code Pro' },
  { value: 'Ubuntu Mono, Noto Sans SC, monospace', label: 'Ubuntu Mono' },
  { value: 'Roboto Mono, Noto Sans SC, monospace', label: 'Roboto Mono' },
  { value: 'IBM Plex Mono, IBM Plex Sans SC, monospace', label: 'IBM Plex Mono' },
  { value: 'Hack, Microsoft YaHei, PingFang SC, monospace', label: 'Hack' },
  { value: 'Inconsolata, Microsoft YaHei, PingFang SC, monospace', label: 'Inconsolata' },
] as const;

export const DEFAULT_TERMINAL_FONT: TerminalFontSettings = {
  fontFamily: DEFAULT_TERMINAL_FONT_FAMILY,
  fontSize: 14,
  fontWeight: 'normal',
  fontWeightBold: 'bold',
  lineHeight: 1.1,
  letterSpacing: 0,
};

export interface PanelShortcutSetting {
  code: string;
  display: string;
}

export interface ShortcutSettings {
  terminal: PanelShortcutSetting;
  notepad: PanelShortcutSetting;
}

export const SUPPORTED_EDITORS = ['vscode', 'cursor', 'trae', 'zed', 'custom'] as const;
export type EditorPreference = (typeof SUPPORTED_EDITORS)[number];

export interface EditorSettings {
  defaultEditor: EditorPreference;
  customCommand: string;
}

export interface WebSessionQuickInputSettings {
  pinned: string[];
  recent: string[];
}

export interface DailyTipSettings {
  enabled: boolean;
}

type ItemResponse<T> = {
  item?: T;
};

export type TerminalQuickActionIcon =
  | 'terminal'
  | 'chat'
  | 'code'
  | 'rocket'
  | 'play'
  | 'claude'
  | 'codex'
  | 'qwen'
  | 'gemini'
  | 'cursor'
  | 'copilot';

export interface TerminalQuickAction {
  id: string;
  name: string;
  command: string;
  icon: TerminalQuickActionIcon;
  enabled: boolean;
  stacked: boolean;
}

export type WebSessionAutoContinueScope =
  | 'network_only'
  | 'network_and_rate_limit'
  | 'all_failures';

export type WebSessionAutoContinuePreset = 'gentle_stop' | 'aggressive_stop' | 'sustain_60s';
export type WebSessionStreamingMarkdownThrottleMode = 'default' | 'custom';
export type FollowSystemThemeSetting = -1 | 0 | 1;

interface GeneralSettings {
  theme: ThemeSettings;
  currentPresetId: string;
  followSystemThemeSetting: FollowSystemThemeSetting;
  customTheme: ThemeSettings | null;
  recentProjectsLimit: number;
  maxTerminalsPerProject: number;
  panelShortcuts: ShortcutSettings;
  webSessionQuickInput: WebSessionQuickInputSettings;
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

type PersistedGeneralSettings = Omit<GeneralSettings, 'followSystemThemeSetting'> & {
  version: number;
  followSystemTheme: FollowSystemThemeSetting;
};

type ParsedGeneralSettingsInput = Partial<Omit<PersistedGeneralSettings, 'webSessionQuickInput'>> & {
  webSessionQuickInput?: Partial<WebSessionQuickInputSettings>;
  panelShortcut?: PanelShortcutSetting;
  followSystemTheme?: unknown;
};

type LoadSettingsResult = {
  settings: GeneralSettings;
  shouldPersist: boolean;
};

const STORAGE_VERSION = 4;
const LEGACY_WEB_SESSION_REASONING_STORAGE_KEY = 'kanban-web-show-reasoning';
const DEFAULT_RECENT_PROJECTS_LIMIT = 10;
const DEFAULT_TERMINALS_PER_PROJECT_LIMIT = 12;
const DEFAULT_WEB_SESSION_AUTO_CONTINUE_SCOPE: WebSessionAutoContinueScope = 'network_only';
const DEFAULT_WEB_SESSION_AUTO_CONTINUE_PRESET: WebSessionAutoContinuePreset = 'gentle_stop';
export const DEFAULT_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MS = 100;
export const WEB_SESSION_QUICK_INPUT_RECENT_LIMIT = 6;
const FOLLOW_SYSTEM_THEME_DEFAULT: FollowSystemThemeSetting = -1;
const FOLLOW_SYSTEM_THEME_DISABLED: FollowSystemThemeSetting = 0;
const FOLLOW_SYSTEM_THEME_ENABLED: FollowSystemThemeSetting = 1;

const defaultTheme: ThemeSettings = getDefaultPreset().colors;

export const DEFAULT_TERMINAL_SHORTCUT: PanelShortcutSetting = {
  code: 'Backquote',
  display: '`',
};

export const DEFAULT_NOTEPAD_SHORTCUT: PanelShortcutSetting = {
  code: 'Digit1',
  display: '1',
};

const DEFAULT_SHORTCUTS: ShortcutSettings = {
  terminal: { ...DEFAULT_TERMINAL_SHORTCUT },
  notepad: { ...DEFAULT_NOTEPAD_SHORTCUT },
};

const DEFAULT_EDITOR_SETTINGS: EditorSettings = {
  defaultEditor: 'vscode',
  customCommand: '',
};

export const DEFAULT_WEB_SESSION_QUICK_INPUT_PINNED = ['continue'] as const;

const DEFAULT_WEB_SESSION_QUICK_INPUT: WebSessionQuickInputSettings = {
  pinned: [...DEFAULT_WEB_SESSION_QUICK_INPUT_PINNED],
  recent: [],
};
const DEFAULT_WEB_SESSION_QUICK_INPUT_DIRECT_SEND = false;
const DEFAULT_DAILY_TIP_SETTINGS: DailyTipSettings = {
  enabled: true,
};

export const DEFAULT_TERMINAL_QUICK_ACTIONS: TerminalQuickAction[] = [
  {
    id: 'claude',
    name: 'Claude Code',
    command: 'claude',
    icon: 'claude',
    enabled: true,
    stacked: false,
  },
  {
    id: 'ccr',
    name: 'Claude Code Router',
    command: 'ccr code',
    icon: 'claude',
    enabled: true,
    stacked: false,
  },
  {
    id: 'codex',
    name: 'Codex',
    command: 'codex',
    icon: 'codex',
    enabled: true,
    stacked: false,
  },
];

const defaultSettings: GeneralSettings = {
  theme: { ...defaultTheme },
  currentPresetId: DEFAULT_PRESET_ID,
  followSystemThemeSetting: FOLLOW_SYSTEM_THEME_DEFAULT,
  customTheme: null,
  recentProjectsLimit: DEFAULT_RECENT_PROJECTS_LIMIT,
  maxTerminalsPerProject: DEFAULT_TERMINALS_PER_PROJECT_LIMIT,
  panelShortcuts: { ...DEFAULT_SHORTCUTS },
  webSessionQuickInput: {
    pinned: [...DEFAULT_WEB_SESSION_QUICK_INPUT.pinned],
    recent: [...DEFAULT_WEB_SESSION_QUICK_INPUT.recent],
  },
  webSessionQuickInputDirectSend: DEFAULT_WEB_SESSION_QUICK_INPUT_DIRECT_SEND,
  terminalQuickActions: DEFAULT_TERMINAL_QUICK_ACTIONS.map(action => ({ ...action })),
  editor: { ...DEFAULT_EDITOR_SETTINGS },
  confirmBeforeTerminalClose: true,
  showWebSessionReasoning: false,
  webSessionActivityDisplayMode: DEFAULT_WEB_SESSION_ACTIVITY_DISPLAY_MODE,
  webSessionAutoContinueScope: DEFAULT_WEB_SESSION_AUTO_CONTINUE_SCOPE,
  webSessionAutoContinuePreset: DEFAULT_WEB_SESSION_AUTO_CONTINUE_PRESET,
  webSessionStreamingMarkdownThrottleMode: 'default',
  webSessionStreamingMarkdownThrottleCustomMs: DEFAULT_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MS,
  terminalThemeId: TERMINAL_THEME_FOLLOW,
  terminalFont: { ...DEFAULT_TERMINAL_FONT },
  terminalWebGLRenderer: 'auto',
  defaultTerminalRenderMode: DEFAULT_TERMINAL_RENDER_MODE,
  defaultTerminalSnapshotIntervalMs: null,
  defaultTerminalSnapshotZlibCompression: true,
  terminalConnectionPolicy: DEFAULT_TERMINAL_CONNECTION_POLICY,
  inactiveTerminalSnapshotIntervalMs: DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS,
};

export const useSettingsStore = defineStore('settings', () => {
  const loadedSettings = loadSettings();
  const settings = ref<GeneralSettings>(loadedSettings.settings);
  const dailyTipEnabled = ref(DEFAULT_DAILY_TIP_SETTINGS.enabled);
  const dailyTipSettingsLoaded = ref(false);
  const dailyTipSettingsSaving = ref(false);
  let dailyTipSettingsLoadTask: Promise<void> | null = null;
  let dailyTipSettingsSyncTask: Promise<DailyTipSettings> = Promise.resolve({
    ...DEFAULT_DAILY_TIP_SETTINGS,
  });
  const webSessionQuickInputLoaded = ref(false);
  let webSessionQuickInputLoadTask: Promise<void> | null = null;
  let webSessionQuickInputSyncTask: Promise<void> = Promise.resolve();

  if (loadedSettings.shouldPersist) {
    saveSettings(loadedSettings.settings);
  }

  const theme = computed(() => settings.value.theme);
  const currentPresetId = computed(() => settings.value.currentPresetId);
  const followSystemThemeSetting = computed(() => settings.value.followSystemThemeSetting);
  const followSystemTheme = computed(() =>
    isFollowSystemThemeEnabled(settings.value.followSystemThemeSetting)
  );
  const customTheme = computed(() => settings.value.customTheme);
  const recentProjectsLimit = computed(() => settings.value.recentProjectsLimit);
  const maxTerminalsPerProject = computed(() => settings.value.maxTerminalsPerProject);
  const panelShortcuts = computed(() => settings.value.panelShortcuts);
  const terminalShortcut = computed(() => panelShortcuts.value.terminal);
  const notepadShortcut = computed(() => panelShortcuts.value.notepad);
  const webSessionQuickInput = computed(() => settings.value.webSessionQuickInput);
  const webSessionQuickInputDirectSend = computed(
    () => settings.value.webSessionQuickInputDirectSend
  );
  const terminalQuickActions = computed(() => settings.value.terminalQuickActions);
  const editorSettings = computed(() => settings.value.editor);
  const confirmBeforeTerminalClose = computed(() => settings.value.confirmBeforeTerminalClose);
  const showWebSessionReasoning = computed(() => settings.value.showWebSessionReasoning);
  const webSessionActivityDisplayMode = computed(
    () => settings.value.webSessionActivityDisplayMode
  );
  const webSessionAutoContinueScope = computed(() => settings.value.webSessionAutoContinueScope);
  const webSessionAutoContinuePreset = computed(() => settings.value.webSessionAutoContinuePreset);
  const webSessionStreamingMarkdownThrottleMode = computed(
    () => settings.value.webSessionStreamingMarkdownThrottleMode
  );
  const webSessionStreamingMarkdownThrottleCustomMs = computed(
    () => settings.value.webSessionStreamingMarkdownThrottleCustomMs
  );
  const webSessionStreamingMarkdownThrottleMs = computed(() =>
    settings.value.webSessionStreamingMarkdownThrottleMode === 'custom'
      ? settings.value.webSessionStreamingMarkdownThrottleCustomMs
      : DEFAULT_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MS
  );
  const terminalThemeId = computed(() => settings.value.terminalThemeId);
  const terminalFont = computed(() => settings.value.terminalFont);
  const terminalWebGLRenderer = computed(() => settings.value.terminalWebGLRenderer);
  const defaultTerminalRenderMode = computed(() => settings.value.defaultTerminalRenderMode);
  const defaultTerminalSnapshotIntervalMs = computed(
    () => settings.value.defaultTerminalSnapshotIntervalMs
  );
  const defaultTerminalSnapshotZlibCompression = computed(
    () => settings.value.defaultTerminalSnapshotZlibCompression
  );
  const terminalConnectionPolicy = computed(() => settings.value.terminalConnectionPolicy);
  const inactiveTerminalSnapshotIntervalMs = computed(
    () => settings.value.inactiveTerminalSnapshotIntervalMs
  );

  /**
   * 获取有效的终端主题 ID
   * 如果设置为"跟随主题"，则返回当前应用主题预设关联的终端主题
   */
  const effectiveTerminalThemeId = computed(() => {
    if (settings.value.terminalThemeId === TERMINAL_THEME_FOLLOW) {
      // 跟随当前应用主题
      const preset = getPresetById(settings.value.currentPresetId);
      return preset?.terminalThemeId ?? DEFAULT_TERMINAL_THEME_ID;
    }
    return settings.value.terminalThemeId;
  });

  /**
   * 计算当前激活的主题
   * 优先级: 跟随系统主题 > 自定义主题 > 预设主题
   *
   * 注意: 在 computed 中访问 window.matchMedia 是为了响应式地获取系统主题偏好
   * App.vue 中会监听系统主题变化事件并更新 store，从而触发此 computed 重新计算
   */
  const activeTheme = computed<ThemeSettings>(() => {
    // 优先级 1: 跟随系统主题
    if (isFollowSystemThemeEnabled(settings.value.followSystemThemeSetting)) {
      // SSR 安全检查
      if (typeof window === 'undefined') {
        return defaultTheme;
      }
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      const autoPresetId = prefersDark ? 'dark' : 'light';
      const preset = getPresetById(autoPresetId);
      return preset?.colors ?? defaultTheme;
    }

    // 优先级 2: 自定义主题
    if (settings.value.customTheme) {
      return settings.value.customTheme;
    }

    // 优先级 3: 预设主题
    const preset = getPresetById(settings.value.currentPresetId);
    return preset?.colors ?? defaultTheme;
  });

  watch(
    settings,
    newSettings => {
      saveSettings(newSettings);
    },
    { deep: true }
  );

  function updateTheme(partial: Partial<ThemeSettings>) {
    settings.value.theme = sanitizeThemeSettings({
      ...settings.value.theme,
      ...partial,
    });
  }

  function resetTheme() {
    // 重置为默认预设主题，并清理自定义/系统跟随状态，保持与 activeTheme 计算逻辑一致
    const preset = getPresetById(DEFAULT_PRESET_ID) ?? getDefaultPreset();
    settings.value.currentPresetId = preset.id;
    settings.value.followSystemThemeSetting = FOLLOW_SYSTEM_THEME_DISABLED;
    settings.value.customTheme = null;
    settings.value.theme = sanitizeThemeSettings(preset.colors);
    // 重置终端主题为"跟随主题"
    settings.value.terminalThemeId = TERMINAL_THEME_FOLLOW;
  }

  function updateRecentProjectsLimit(limit: number) {
    settings.value.recentProjectsLimit = sanitizeRecentProjectsLimit(limit);
  }

  function updateMaxTerminalsPerProject(limit: number) {
    settings.value.maxTerminalsPerProject = sanitizeTerminalLimit(limit);
  }

  function queueDailyTipSettingsSync(payload: DailyTipSettings) {
    const pendingLoadTask = dailyTipSettingsLoadTask;

    const task = dailyTipSettingsSyncTask.then(async () => {
      if (pendingLoadTask) {
        await pendingLoadTask;
      }

      dailyTipSettingsSaving.value = true;
      try {
        const response = await http
          .Post<ItemResponse<DailyTipSettings>>('/system/daily-tip-settings/update', payload)
          .send();
        const next = sanitizeDailyTipSettings(response?.item ?? payload);
        dailyTipEnabled.value = next.enabled;
        dailyTipSettingsLoaded.value = true;
        return next;
      } finally {
        dailyTipSettingsSaving.value = false;
      }
    });

    dailyTipSettingsSyncTask = task.catch(error => {
      console.warn('Failed to sync daily tip settings.', error);
      return sanitizeDailyTipSettings({
        enabled: dailyTipEnabled.value,
      });
    });

    return task;
  }

  async function loadDailyTipSettings(force = false) {
    if (!force && dailyTipSettingsLoaded.value) {
      return;
    }
    if (!force && dailyTipSettingsLoadTask) {
      return dailyTipSettingsLoadTask;
    }

    const task = http
      .Get<ItemResponse<DailyTipSettings>>('/system/daily-tip-settings')
      .send()
      .then(response => {
        const next = sanitizeDailyTipSettings(response?.item);
        dailyTipEnabled.value = next.enabled;
        dailyTipSettingsLoaded.value = true;
      })
      .catch(error => {
        console.warn('Failed to load daily tip settings.', error);
      })
      .finally(() => {
        dailyTipSettingsLoadTask = null;
      });

    dailyTipSettingsLoadTask = task;
    return task;
  }

  async function updateDailyTipEnabled(value: boolean) {
    if (dailyTipSettingsLoadTask) {
      await dailyTipSettingsLoadTask;
    } else if (!dailyTipSettingsLoaded.value) {
      await loadDailyTipSettings();
    }

    return queueDailyTipSettingsSync(
      sanitizeDailyTipSettings({
        enabled: value,
      })
    );
  }

  function updatePanelShortcuts(partial: Partial<ShortcutSettings>) {
    settings.value.panelShortcuts = {
      terminal: sanitizePanelShortcut(partial.terminal, settings.value.panelShortcuts.terminal),
      notepad: sanitizePanelShortcut(partial.notepad, settings.value.panelShortcuts.notepad),
    };
  }

  function updateTerminalShortcut(shortcut: PanelShortcutSetting) {
    settings.value.panelShortcuts.terminal = sanitizePanelShortcut(
      shortcut,
      settings.value.panelShortcuts.terminal
    );
  }

  function updateNotepadShortcut(shortcut: PanelShortcutSetting) {
    settings.value.panelShortcuts.notepad = sanitizePanelShortcut(
      shortcut,
      settings.value.panelShortcuts.notepad
    );
  }

  function resetTerminalShortcut() {
    settings.value.panelShortcuts.terminal = { ...DEFAULT_TERMINAL_SHORTCUT };
  }

  function resetNotepadShortcut() {
    settings.value.panelShortcuts.notepad = { ...DEFAULT_NOTEPAD_SHORTCUT };
  }

  function updateEditorSettings(partial: Partial<EditorSettings>) {
    settings.value.editor = sanitizeEditorSettings({
      ...settings.value.editor,
      ...partial,
    });
  }

  function updateTerminalQuickActions(actions: TerminalQuickAction[]) {
    settings.value.terminalQuickActions = sanitizeTerminalQuickActions(actions);
  }

  function updateWebSessionQuickInputPinned(items: string[]) {
    settings.value.webSessionQuickInput = {
      ...settings.value.webSessionQuickInput,
      pinned: sanitizeWebSessionQuickInputItems(items),
    };
    webSessionQuickInputLoaded.value = true;
  }

  function recordWebSessionRecentInput(text: string) {
    const [normalized] = sanitizeWebSessionQuickInputItems([text], 1);
    if (!normalized) {
      return;
    }
    settings.value.webSessionQuickInput = {
      ...settings.value.webSessionQuickInput,
      recent: sanitizeWebSessionQuickInputItems(
        [normalized, ...settings.value.webSessionQuickInput.recent],
        WEB_SESSION_QUICK_INPUT_RECENT_LIMIT
      ),
    };
    webSessionQuickInputLoaded.value = true;
  }

  function updateWebSessionQuickInputDirectSend(value: boolean) {
    settings.value.webSessionQuickInputDirectSend = value === true;
  }

  function queueWebSessionQuickInputSync(payload: WebSessionQuickInputSettings) {
    const pendingLoadTask = webSessionQuickInputLoadTask;

    const task = webSessionQuickInputSyncTask.then(async () => {
      if (pendingLoadTask) {
        await pendingLoadTask;
      }
      const response = await http
        .Post<
          ItemResponse<WebSessionQuickInputSettings>
        >('/system/web-session-quick-input/update', payload)
        .send();
      const next = sanitizeWebSessionQuickInput(response?.item ?? payload);
      settings.value.webSessionQuickInput = next;
      webSessionQuickInputLoaded.value = true;
      return next;
    });

    webSessionQuickInputSyncTask = task
      .then(() => undefined)
      .catch(error => {
        console.warn('Failed to sync web session quick input settings.', error);
      });

    return task;
  }

  async function loadWebSessionQuickInput(force = false) {
    if (!force && webSessionQuickInputLoaded.value) {
      return;
    }
    if (!force && webSessionQuickInputLoadTask) {
      return webSessionQuickInputLoadTask;
    }

    const task = http
      .Get<ItemResponse<WebSessionQuickInputSettings>>('/system/web-session-quick-input')
      .send()
      .then(response => {
        const next = sanitizeWebSessionQuickInput(response?.item);
        settings.value.webSessionQuickInput = next;
        webSessionQuickInputLoaded.value = true;
      })
      .catch(error => {
        console.warn('Failed to load web session quick input settings.', error);
      })
      .finally(() => {
        webSessionQuickInputLoadTask = null;
      });

    webSessionQuickInputLoadTask = task;
    return task;
  }

  async function syncWebSessionQuickInputToServer() {
    const payload = sanitizeWebSessionQuickInput(settings.value.webSessionQuickInput);
    settings.value.webSessionQuickInput = payload;
    webSessionQuickInputLoaded.value = true;
    return queueWebSessionQuickInputSync(payload);
  }

  async function saveWebSessionQuickInputPinned(items: string[]) {
    if (webSessionQuickInputLoadTask) {
      await webSessionQuickInputLoadTask;
    } else if (!webSessionQuickInputLoaded.value) {
      await loadWebSessionQuickInput();
    }
    const payload = sanitizeWebSessionQuickInput({
      ...settings.value.webSessionQuickInput,
      pinned: items,
    });
    return queueWebSessionQuickInputSync(payload);
  }

  function updateConfirmBeforeTerminalClose(value: boolean) {
    settings.value.confirmBeforeTerminalClose = value;
  }

  function updateShowWebSessionReasoning(value: boolean) {
    settings.value.showWebSessionReasoning = value;
  }

  function updateWebSessionActivityDisplayMode(value: WebSessionActivityDisplayMode) {
    settings.value.webSessionActivityDisplayMode = sanitizeWebSessionActivityDisplayMode(value);
  }

  function updateWebSessionAutoContinueScope(value: WebSessionAutoContinueScope) {
    settings.value.webSessionAutoContinueScope = sanitizeWebSessionAutoContinueScope(value);
  }

  function updateWebSessionAutoContinuePreset(value: WebSessionAutoContinuePreset) {
    settings.value.webSessionAutoContinuePreset = sanitizeWebSessionAutoContinuePreset(value);
  }

  function updateWebSessionStreamingMarkdownThrottleMode(
    value: WebSessionStreamingMarkdownThrottleMode
  ) {
    settings.value.webSessionStreamingMarkdownThrottleMode =
      sanitizeWebSessionStreamingMarkdownThrottleMode(value);
  }

  function updateWebSessionStreamingMarkdownThrottleCustomMs(value: number) {
    settings.value.webSessionStreamingMarkdownThrottleCustomMs =
      sanitizeWebSessionStreamingMarkdownThrottleCustomMs(value);
  }

  function updateTerminalTheme(themeId: string) {
    settings.value.terminalThemeId = themeId;
  }

  function updateTerminalFont(partial: Partial<TerminalFontSettings>) {
    settings.value.terminalFont = {
      ...settings.value.terminalFont,
      ...partial,
    };
  }

  function updateTerminalWebGLRenderer(mode: 'auto' | 'force' | 'disable') {
    settings.value.terminalWebGLRenderer = mode;
  }

  function updateDefaultTerminalRenderMode(mode: TerminalRenderMode) {
    settings.value.defaultTerminalRenderMode = sanitizeTerminalRenderMode(mode);
  }

  function updateDefaultTerminalSnapshotIntervalMs(value: number | null) {
    settings.value.defaultTerminalSnapshotIntervalMs =
      sanitizeDefaultTerminalSnapshotIntervalMs(value);
  }

  function updateDefaultTerminalSnapshotZlibCompression(value: boolean) {
    settings.value.defaultTerminalSnapshotZlibCompression = value !== false;
  }

  function updateTerminalConnectionPolicy(value: TerminalConnectionPolicy) {
    settings.value.terminalConnectionPolicy = sanitizeTerminalConnectionPolicy(value);
  }

  function updateInactiveTerminalSnapshotIntervalMs(value: number) {
    settings.value.inactiveTerminalSnapshotIntervalMs = sanitizeInactiveSnapshotIntervalMs(value);
  }

  function resetTerminalFont() {
    settings.value.terminalFont = { ...DEFAULT_TERMINAL_FONT };
  }

  function setFollowSystemThemeSetting(value: FollowSystemThemeSetting) {
    const next = sanitizeFollowSystemThemeSetting(value);
    settings.value.followSystemThemeSetting = next;
    if (next === FOLLOW_SYSTEM_THEME_ENABLED) {
      // 切换到跟随系统模式时，清除自定义主题
      settings.value.customTheme = null;
      // 根据当前系统主题更新预设ID
      const prefersDark =
        typeof window !== 'undefined'
          ? window.matchMedia('(prefers-color-scheme: dark)').matches
          : false;
      const autoPresetId = prefersDark ? 'dark' : 'light';
      const preset = getPresetById(autoPresetId);
      if (preset) {
        settings.value.currentPresetId = autoPresetId;
        settings.value.theme = sanitizeThemeSettings(preset.colors);
      }
    }
  }

  function selectPreset(presetId: string) {
    const preset = getPresetById(presetId);
    if (preset) {
      settings.value.currentPresetId = presetId;
      settings.value.theme = sanitizeThemeSettings(preset.colors);
      settings.value.customTheme = null;
      settings.value.followSystemThemeSetting = FOLLOW_SYSTEM_THEME_DISABLED;
      // 终端主题保持用户选择不变
      // 如果是"跟随主题"，effectiveTerminalThemeId 会自动计算正确的值
    }
  }

  // 专门用于系统主题变化时调用，不关闭 followSystemTheme
  function applySystemThemePreset(presetId: string) {
    const preset = getPresetById(presetId);
    if (preset) {
      settings.value.currentPresetId = presetId;
      settings.value.theme = sanitizeThemeSettings(preset.colors);
      settings.value.customTheme = null;
      // 终端主题保持用户选择不变
      // 如果是"跟随主题"，effectiveTerminalThemeId 会自动计算正确的值
    }
  }

  function toggleFollowSystemTheme(enabled: boolean) {
    setFollowSystemThemeSetting(
      enabled ? FOLLOW_SYSTEM_THEME_ENABLED : FOLLOW_SYSTEM_THEME_DISABLED
    );
  }

  function applyCustomTheme(themeColors: Partial<ThemeSettings>) {
    settings.value.customTheme = sanitizeThemeSettings({
      ...activeTheme.value,
      ...themeColors,
    });
    settings.value.theme = settings.value.customTheme;
    settings.value.followSystemThemeSetting = FOLLOW_SYSTEM_THEME_DISABLED;
  }

  function exportClientBackup(
    locale: string,
    options?: { includeQuickInputRecent?: boolean }
  ): SettingsBackupClientPayload {
    const serialized = serializeSettingsForBackup(settings.value);
    if (options?.includeQuickInputRecent === false && serialized.webSessionQuickInput) {
      const { recent: _recent, ...restQuickInput } = serialized.webSessionQuickInput;
      serialized.webSessionQuickInput = restQuickInput;
      if (!('pinned' in serialized.webSessionQuickInput)) {
        delete serialized.webSessionQuickInput;
      }
    }
    return {
      locale: typeof locale === 'string' && locale.trim() ? locale.trim() : 'zh-CN',
      settings: serialized,
    };
  }

  function importClientBackup(payload: SettingsBackupClientPayload) {
    if (payload?.settings) {
      const importedSettings = cloneSettingsBackupClientSettings(payload.settings);
      if (importedSettings) {
        importedSettings.webSessionQuickInput = preserveImportedQuickInputFields(
          importedSettings.webSessionQuickInput,
          settings.value.webSessionQuickInput
        );
        settings.value = deserializeSettingsFromBackup(importedSettings);
      }
    }
    saveSettings(settings.value);
    if (typeof window !== 'undefined' && window.localStorage && payload?.locale) {
      window.localStorage.setItem(APP_LOCALE_STORAGE_KEY, payload.locale);
    }
    if (payload?.locale === 'zh-CN' || payload?.locale === 'en-US') {
      i18n.global.locale = payload.locale;
      dayjs.locale(payload.locale === 'zh-CN' ? 'zh-cn' : 'en');
    }
  }

  return {
    theme,
    currentPresetId,
    followSystemThemeSetting,
    followSystemTheme,
    customTheme,
    activeTheme,
    recentProjectsLimit,
    maxTerminalsPerProject,
    dailyTipEnabled,
    dailyTipSettingsLoaded,
    dailyTipSettingsSaving,
    panelShortcuts,
    terminalShortcut,
    notepadShortcut,
    webSessionQuickInput,
    webSessionQuickInputDirectSend,
    webSessionQuickInputLoaded,
    terminalQuickActions,
    editorSettings,
    confirmBeforeTerminalClose,
    showWebSessionReasoning,
    webSessionActivityDisplayMode,
    webSessionAutoContinueScope,
    webSessionAutoContinuePreset,
    webSessionStreamingMarkdownThrottleMode,
    webSessionStreamingMarkdownThrottleCustomMs,
    webSessionStreamingMarkdownThrottleMs,
    terminalThemeId,
    terminalFont,
    terminalWebGLRenderer,
    defaultTerminalRenderMode,
    defaultTerminalSnapshotIntervalMs,
    defaultTerminalSnapshotZlibCompression,
    terminalConnectionPolicy,
    inactiveTerminalSnapshotIntervalMs,
    effectiveTerminalThemeId,
    updateTheme,
    resetTheme,
    updateRecentProjectsLimit,
    updateMaxTerminalsPerProject,
    loadDailyTipSettings,
    updateDailyTipEnabled,
    updatePanelShortcuts,
    updateTerminalShortcut,
    updateNotepadShortcut,
    resetTerminalShortcut,
    resetNotepadShortcut,
    loadWebSessionQuickInput,
    syncWebSessionQuickInputToServer,
    saveWebSessionQuickInputPinned,
    updateWebSessionQuickInputPinned,
    updateWebSessionQuickInputDirectSend,
    recordWebSessionRecentInput,
    updateTerminalQuickActions,
    updateEditorSettings,
    updateConfirmBeforeTerminalClose,
    updateShowWebSessionReasoning,
    updateWebSessionActivityDisplayMode,
    updateWebSessionAutoContinueScope,
    updateWebSessionAutoContinuePreset,
    updateWebSessionStreamingMarkdownThrottleMode,
    updateWebSessionStreamingMarkdownThrottleCustomMs,
    updateTerminalTheme,
    updateTerminalFont,
    updateTerminalWebGLRenderer,
    updateDefaultTerminalRenderMode,
    updateDefaultTerminalSnapshotIntervalMs,
    updateDefaultTerminalSnapshotZlibCompression,
    updateTerminalConnectionPolicy,
    updateInactiveTerminalSnapshotIntervalMs,
    resetTerminalFont,
    setFollowSystemThemeSetting,
    selectPreset,
    applySystemThemePreset,
    toggleFollowSystemTheme,
    applyCustomTheme,
    exportClientBackup,
    importClientBackup,
  };
});

function loadSettings(): LoadSettingsResult {
  try {
    const stored = localStorage.getItem(SETTINGS_STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as ParsedGeneralSettingsInput | null;
      return loadSettingsFromParsed(parsed);
    }
  } catch (error) {
    console.warn('Failed to load settings, falling back to defaults.', error);
  }
  return {
    settings: cloneDefaultSettings(),
    shouldPersist: false,
  };
}

function saveSettings(settings: GeneralSettings) {
  try {
    const { followSystemThemeSetting, ...restSettings } = settings;
    const persisted: PersistedGeneralSettings = {
      version: STORAGE_VERSION,
      ...restSettings,
      followSystemTheme: followSystemThemeSetting,
    };
    localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(persisted));
  } catch (error) {
    console.error('Failed to persist settings:', error);
  }
}

function serializeSettingsForBackup(settings: GeneralSettings): SettingsBackupClientSettings {
  const { followSystemThemeSetting, ...restSettings } = settings;
  return {
    version: STORAGE_VERSION,
    ...restSettings,
    followSystemTheme: followSystemThemeSetting,
  };
}

function deserializeSettingsFromBackup(value: unknown): GeneralSettings {
  const parsed = value as ParsedGeneralSettingsInput | null | undefined;
  const loadResult = loadSettingsFromParsed(parsed);
  return loadResult.settings;
}

function cloneSettingsBackupClientSettings(
  value: SettingsBackupClientPayload['settings']
): SettingsBackupClientPayload['settings'] {
  if (!value || typeof value !== 'object') {
    return value;
  }
  return JSON.parse(JSON.stringify(value)) as SettingsBackupClientPayload['settings'];
}

function preserveImportedQuickInputFields(
  imported: Partial<WebSessionQuickInputSettings> | undefined,
  current: WebSessionQuickInputSettings
) {
  const next = imported ? { ...imported } : {};
  if (!('pinned' in next)) {
    next.pinned = [...current.pinned];
  }
  if (!('recent' in next)) {
    next.recent = [...current.recent];
  }
  return next;
}

function cloneDefaultSettings(): GeneralSettings {
  return {
    theme: { ...defaultSettings.theme },
    currentPresetId: defaultSettings.currentPresetId,
    followSystemThemeSetting: defaultSettings.followSystemThemeSetting,
    terminalThemeId: defaultSettings.terminalThemeId,
    customTheme: defaultSettings.customTheme,
    recentProjectsLimit: defaultSettings.recentProjectsLimit,
    maxTerminalsPerProject: defaultSettings.maxTerminalsPerProject,
    panelShortcuts: {
      terminal: { ...defaultSettings.panelShortcuts.terminal },
      notepad: { ...defaultSettings.panelShortcuts.notepad },
    },
    webSessionQuickInput: {
      pinned: [...defaultSettings.webSessionQuickInput.pinned],
      recent: [...defaultSettings.webSessionQuickInput.recent],
    },
    webSessionQuickInputDirectSend: defaultSettings.webSessionQuickInputDirectSend,
    terminalQuickActions: defaultSettings.terminalQuickActions.map(action => ({ ...action })),
    editor: { ...defaultSettings.editor },
    confirmBeforeTerminalClose: defaultSettings.confirmBeforeTerminalClose,
    showWebSessionReasoning: defaultSettings.showWebSessionReasoning,
    webSessionActivityDisplayMode: defaultSettings.webSessionActivityDisplayMode,
    webSessionAutoContinueScope: defaultSettings.webSessionAutoContinueScope,
    webSessionAutoContinuePreset: defaultSettings.webSessionAutoContinuePreset,
    webSessionStreamingMarkdownThrottleMode:
      defaultSettings.webSessionStreamingMarkdownThrottleMode,
    webSessionStreamingMarkdownThrottleCustomMs:
      defaultSettings.webSessionStreamingMarkdownThrottleCustomMs,
    terminalFont: { ...defaultSettings.terminalFont },
    terminalWebGLRenderer: defaultSettings.terminalWebGLRenderer,
    defaultTerminalRenderMode: defaultSettings.defaultTerminalRenderMode,
    defaultTerminalSnapshotIntervalMs: sanitizeDefaultTerminalSnapshotIntervalMs(
      defaultSettings.defaultTerminalSnapshotIntervalMs
    ),
    defaultTerminalSnapshotZlibCompression: defaultSettings.defaultTerminalSnapshotZlibCompression,
    terminalConnectionPolicy: defaultSettings.terminalConnectionPolicy,
    inactiveTerminalSnapshotIntervalMs: sanitizeInactiveSnapshotIntervalMs(
      defaultSettings.inactiveTerminalSnapshotIntervalMs
    ),
  };
}

function loadSettingsFromParsed(
  parsed: ParsedGeneralSettingsInput | null | undefined
): LoadSettingsResult {
  if (!parsed || typeof parsed !== 'object') {
    return {
      settings: cloneDefaultSettings(),
      shouldPersist: false,
    };
  }

  const parsedVersion = typeof parsed.version === 'number' ? parsed.version : undefined;

  if (parsedVersion != null && parsedVersion > STORAGE_VERSION) {
    console.warn(`Unsupported settings version "${parsedVersion}", falling back to defaults.`);
    return {
      settings: cloneDefaultSettings(),
      shouldPersist: false,
    };
  }

  const legacyParsed = parsed as Partial<GeneralSettings> & {
    panelShortcut?: PanelShortcutSetting;
  };

  let currentPresetId = parsed.currentPresetId ?? legacyParsed.currentPresetId ?? DEFAULT_PRESET_ID;
  if (!parsed.currentPresetId && !legacyParsed.currentPresetId && parsed.theme) {
    const matchedPreset = THEME_PRESETS.find(
      p => p.colors.primaryColor === parsed.theme?.primaryColor
    );
    if (matchedPreset) {
      currentPresetId = matchedPreset.id;
    }
  }
  const currentPresetTheme = getPresetById(currentPresetId)?.colors ?? defaultTheme;

  return {
    settings: {
      theme: sanitizeThemeSettings(parsed.theme ?? currentPresetTheme),
      currentPresetId,
      followSystemThemeSetting:
        parsedVersion != null && parsedVersion >= 2
          ? sanitizeFollowSystemThemeSetting(parsed.followSystemTheme)
          : FOLLOW_SYSTEM_THEME_DEFAULT,
      customTheme: sanitizeOptionalThemeSettings(parsed.customTheme),
      recentProjectsLimit: sanitizeRecentProjectsLimit(parsed.recentProjectsLimit),
      maxTerminalsPerProject: sanitizeTerminalLimit(parsed.maxTerminalsPerProject),
      panelShortcuts: sanitizePanelShortcuts(parsed.panelShortcuts ?? parsed.panelShortcut),
      webSessionQuickInput: sanitizeWebSessionQuickInput(parsed.webSessionQuickInput),
      webSessionQuickInputDirectSend: sanitizeWebSessionQuickInputDirectSend(
        parsed.webSessionQuickInputDirectSend
      ),
      terminalQuickActions: sanitizeTerminalQuickActions(parsed.terminalQuickActions),
      editor: sanitizeEditorSettings(parsed.editor),
      confirmBeforeTerminalClose:
        parsed.confirmBeforeTerminalClose ?? defaultSettings.confirmBeforeTerminalClose,
      showWebSessionReasoning: sanitizeShowWebSessionReasoning(
        parsed.showWebSessionReasoning,
        loadLegacyShowWebSessionReasoning()
      ),
      webSessionActivityDisplayMode: sanitizeWebSessionActivityDisplayMode(
        parsed.webSessionActivityDisplayMode
      ),
      webSessionAutoContinueScope: sanitizeWebSessionAutoContinueScope(
        parsed.webSessionAutoContinueScope
      ),
      webSessionAutoContinuePreset: sanitizeWebSessionAutoContinuePreset(
        parsed.webSessionAutoContinuePreset
      ),
      webSessionStreamingMarkdownThrottleMode: sanitizeWebSessionStreamingMarkdownThrottleMode(
        parsed.webSessionStreamingMarkdownThrottleMode
      ),
      webSessionStreamingMarkdownThrottleCustomMs:
        sanitizeWebSessionStreamingMarkdownThrottleCustomMs(
          parsed.webSessionStreamingMarkdownThrottleCustomMs
        ),
      terminalThemeId: parsed.terminalThemeId ?? defaultSettings.terminalThemeId,
      terminalFont: sanitizeTerminalFont(parsed.terminalFont),
      terminalWebGLRenderer: sanitizeWebGLRenderer(parsed.terminalWebGLRenderer),
      defaultTerminalRenderMode: sanitizeTerminalRenderMode(parsed.defaultTerminalRenderMode),
      defaultTerminalSnapshotIntervalMs: sanitizeDefaultTerminalSnapshotIntervalMs(
        parsed.defaultTerminalSnapshotIntervalMs
      ),
      defaultTerminalSnapshotZlibCompression:
        parsed.defaultTerminalSnapshotZlibCompression !== false,
      terminalConnectionPolicy: sanitizeTerminalConnectionPolicy(parsed.terminalConnectionPolicy),
      inactiveTerminalSnapshotIntervalMs: sanitizeInactiveSnapshotIntervalMs(
        parsed.inactiveTerminalSnapshotIntervalMs
      ),
    },
    shouldPersist: parsedVersion !== STORAGE_VERSION,
  };
}

function sanitizeFollowSystemThemeSetting(value: unknown): FollowSystemThemeSetting {
  if (value === FOLLOW_SYSTEM_THEME_DEFAULT) {
    return FOLLOW_SYSTEM_THEME_DEFAULT;
  }
  if (value === FOLLOW_SYSTEM_THEME_DISABLED) {
    return FOLLOW_SYSTEM_THEME_DISABLED;
  }
  if (value === FOLLOW_SYSTEM_THEME_ENABLED) {
    return FOLLOW_SYSTEM_THEME_ENABLED;
  }
  return FOLLOW_SYSTEM_THEME_DEFAULT;
}

function isFollowSystemThemeEnabled(value: FollowSystemThemeSetting) {
  return value === FOLLOW_SYSTEM_THEME_ENABLED;
}

function sanitizeDefaultTerminalSnapshotIntervalMs(value: unknown) {
  if (value == null) {
    return null;
  }
  return sanitizeTerminalSnapshotIntervalMs(value, DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS);
}

function sanitizeInactiveSnapshotIntervalMs(value: unknown) {
  return sanitizeTerminalSnapshotIntervalMs(value, DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS);
}

function sanitizeRecentProjectsLimit(value: number | undefined) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return DEFAULT_RECENT_PROJECTS_LIMIT;
  }
  return Math.min(Math.max(Math.round(parsed), 1), 20);
}

function sanitizeTerminalLimit(value: number | undefined) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return DEFAULT_TERMINALS_PER_PROJECT_LIMIT;
  }
  return Math.min(Math.max(Math.round(parsed), 1), 24);
}

function sanitizeDailyTipSettings(value?: Partial<DailyTipSettings> | null): DailyTipSettings {
  return {
    enabled: value?.enabled !== false,
  };
}

function sanitizeShowWebSessionReasoning(value: unknown, fallback = false) {
  if (typeof value === 'boolean') {
    return value;
  }
  return fallback;
}

function sanitizeThemeSettings(value: unknown): ThemeSettings {
  const source = value && typeof value === 'object' ? (value as Partial<ThemeSettings>) : {};
  return {
    primaryColor: source.primaryColor ?? defaultTheme.primaryColor,
    surfaceColor: source.surfaceColor ?? defaultTheme.surfaceColor,
    bodyColor: source.bodyColor ?? defaultTheme.bodyColor,
    textColor: source.textColor ?? defaultTheme.textColor,
    terminalBg: source.terminalBg ?? defaultTheme.terminalBg,
    terminalFg: source.terminalFg ?? defaultTheme.terminalFg,
    terminalTabBg: source.terminalTabBg ?? defaultTheme.terminalTabBg,
    terminalTabActiveBg: source.terminalTabActiveBg ?? defaultTheme.terminalTabActiveBg,
    terminalHeaderBorder: source.terminalHeaderBorder ?? defaultTheme.terminalHeaderBorder,
    terminalTabCompletionBg: source.terminalTabCompletionBg ?? defaultTheme.terminalTabCompletionBg,
    terminalTabCompletionBorder:
      source.terminalTabCompletionBorder ?? defaultTheme.terminalTabCompletionBorder,
    terminalTabApprovalBg: source.terminalTabApprovalBg ?? defaultTheme.terminalTabApprovalBg,
    terminalTabApprovalBorder:
      source.terminalTabApprovalBorder ?? defaultTheme.terminalTabApprovalBorder,
    terminalEmptyGuideFg: source.terminalEmptyGuideFg ?? defaultTheme.terminalEmptyGuideFg,
    notificationButtonBorder:
      source.notificationButtonBorder ?? defaultTheme.notificationButtonBorder,
    notificationButtonFg: source.notificationButtonFg ?? defaultTheme.notificationButtonFg,
    kanbanBoardBg: source.kanbanBoardBg ?? defaultTheme.kanbanBoardBg,
    kanbanCardBg: source.kanbanCardBg ?? defaultTheme.kanbanCardBg,
    kanbanBorderEnabled: source.kanbanBorderEnabled ?? defaultTheme.kanbanBorderEnabled,
  };
}

function sanitizeOptionalThemeSettings(value: unknown) {
  if (!value || typeof value !== 'object') {
    return null;
  }
  return sanitizeThemeSettings(value);
}

const VALID_WEB_SESSION_AUTO_CONTINUE_SCOPES: WebSessionAutoContinueScope[] = [
  'network_only',
  'network_and_rate_limit',
  'all_failures',
];

function sanitizeWebSessionAutoContinueScope(value: unknown): WebSessionAutoContinueScope {
  if (
    typeof value === 'string' &&
    VALID_WEB_SESSION_AUTO_CONTINUE_SCOPES.includes(value as WebSessionAutoContinueScope)
  ) {
    return value as WebSessionAutoContinueScope;
  }
  return DEFAULT_WEB_SESSION_AUTO_CONTINUE_SCOPE;
}

const VALID_WEB_SESSION_AUTO_CONTINUE_PRESETS: WebSessionAutoContinuePreset[] = [
  'gentle_stop',
  'aggressive_stop',
  'sustain_60s',
];

function sanitizeWebSessionAutoContinuePreset(value: unknown): WebSessionAutoContinuePreset {
  if (
    typeof value === 'string' &&
    VALID_WEB_SESSION_AUTO_CONTINUE_PRESETS.includes(value as WebSessionAutoContinuePreset)
  ) {
    return value as WebSessionAutoContinuePreset;
  }
  return DEFAULT_WEB_SESSION_AUTO_CONTINUE_PRESET;
}

const VALID_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MODES: WebSessionStreamingMarkdownThrottleMode[] =
  ['default', 'custom'];

function sanitizeWebSessionStreamingMarkdownThrottleMode(
  value: unknown
): WebSessionStreamingMarkdownThrottleMode {
  if (
    typeof value === 'string' &&
    VALID_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MODES.includes(
      value as WebSessionStreamingMarkdownThrottleMode
    )
  ) {
    return value as WebSessionStreamingMarkdownThrottleMode;
  }
  return defaultSettings.webSessionStreamingMarkdownThrottleMode;
}

function sanitizeWebSessionStreamingMarkdownThrottleCustomMs(value: unknown) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return DEFAULT_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MS;
  }
  return Math.max(1, Math.round(parsed));
}

function sanitizeWebSessionQuickInputItems(value: unknown, limit?: number) {
  if (!Array.isArray(value)) {
    return [];
  }

  const sanitized: string[] = [];
  const seen = new Set<string>();

  for (const raw of value) {
    if (typeof raw !== 'string') {
      continue;
    }
    const normalized = raw.trim();
    if (!normalized || seen.has(normalized)) {
      continue;
    }
    sanitized.push(normalized);
    seen.add(normalized);
    if (typeof limit === 'number' && limit > 0 && sanitized.length >= limit) {
      break;
    }
  }

  return sanitized;
}

function sanitizeWebSessionQuickInput(
  value?: Partial<WebSessionQuickInputSettings> | null
): WebSessionQuickInputSettings {
  if (value == null) {
    return {
      pinned: [...DEFAULT_WEB_SESSION_QUICK_INPUT.pinned],
      recent: [...DEFAULT_WEB_SESSION_QUICK_INPUT.recent],
    };
  }
  return {
    pinned: sanitizeWebSessionQuickInputItems(value?.pinned),
    recent: sanitizeWebSessionQuickInputItems(value?.recent, WEB_SESSION_QUICK_INPUT_RECENT_LIMIT),
  };
}

function sanitizeWebSessionQuickInputDirectSend(value: unknown) {
  return value === true;
}

function loadLegacyShowWebSessionReasoning() {
  try {
    const stored = localStorage.getItem(LEGACY_WEB_SESSION_REASONING_STORAGE_KEY);
    if (stored === null) {
      return defaultSettings.showWebSessionReasoning;
    }
    return JSON.parse(stored) === true;
  } catch (error) {
    console.warn('Failed to load legacy web session reasoning setting.', error);
    return defaultSettings.showWebSessionReasoning;
  }
}

function sanitizeEditorSettings(value?: Partial<EditorSettings>): EditorSettings {
  if (!value) {
    return { ...DEFAULT_EDITOR_SETTINGS };
  }
  const normalized =
    typeof value.defaultEditor === 'string' ? value.defaultEditor.toLowerCase().trim() : '';
  const supported = SUPPORTED_EDITORS.includes(normalized as EditorPreference)
    ? (normalized as EditorPreference)
    : DEFAULT_EDITOR_SETTINGS.defaultEditor;
  const customCommand =
    typeof value.customCommand === 'string'
      ? value.customCommand
      : DEFAULT_EDITOR_SETTINGS.customCommand;
  return {
    defaultEditor: supported,
    customCommand,
  };
}

function sanitizePanelShortcuts(
  value?: Partial<ShortcutSettings> | PanelShortcutSetting
): ShortcutSettings {
  if (value && 'terminal' in (value as ShortcutSettings)) {
    const partial = value as Partial<ShortcutSettings>;
    return {
      terminal: sanitizePanelShortcut(partial.terminal, DEFAULT_TERMINAL_SHORTCUT),
      notepad: sanitizePanelShortcut(partial.notepad, DEFAULT_NOTEPAD_SHORTCUT),
    };
  }
  if (value && 'code' in (value as PanelShortcutSetting)) {
    const shortcut = sanitizePanelShortcut(
      value as PanelShortcutSetting,
      DEFAULT_TERMINAL_SHORTCUT
    );
    return {
      terminal: shortcut,
      notepad: { ...DEFAULT_NOTEPAD_SHORTCUT },
    };
  }
  return {
    terminal: { ...DEFAULT_TERMINAL_SHORTCUT },
    notepad: { ...DEFAULT_NOTEPAD_SHORTCUT },
  };
}

function sanitizePanelShortcut(
  value: Partial<PanelShortcutSetting> | undefined,
  fallback: PanelShortcutSetting
): PanelShortcutSetting {
  const base = fallback ?? DEFAULT_TERMINAL_SHORTCUT;
  const code = typeof value?.code === 'string' && value.code.trim().length ? value.code : base.code;
  const display =
    typeof value?.display === 'string' && value.display.trim().length
      ? value.display
      : deriveDisplayFromCode(code);
  return {
    code,
    display,
  };
}

function deriveDisplayFromCode(code?: string) {
  if (!code) {
    return DEFAULT_TERMINAL_SHORTCUT.display;
  }
  if (code === 'Backquote') {
    return '`';
  }
  if (code.startsWith('Digit')) {
    return code.replace('Digit', '');
  }
  if (code.startsWith('Key')) {
    return code.replace('Key', '');
  }
  if (code.startsWith('Numpad')) {
    return code.replace('Numpad', 'Num ');
  }
  return code;
}

function sanitizeTerminalQuickActionIcon(value: unknown): TerminalQuickActionIcon {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : '';
  switch (normalized) {
    case 'terminal':
    case 'chat':
    case 'code':
    case 'rocket':
    case 'play':
    case 'claude':
    case 'codex':
    case 'qwen':
    case 'gemini':
    case 'cursor':
    case 'copilot':
      return normalized as TerminalQuickActionIcon;
    default:
      return 'terminal';
  }
}

function sanitizeTerminalQuickActions(value?: unknown): TerminalQuickAction[] {
  if (!Array.isArray(value)) {
    return DEFAULT_TERMINAL_QUICK_ACTIONS.map(action => ({ ...action }));
  }

  const sanitized: TerminalQuickAction[] = [];
  const usedIds = new Set<string>();

  for (let index = 0; index < value.length; index += 1) {
    const raw = value[index] as Partial<TerminalQuickAction> | null | undefined;
    if (!raw || typeof raw !== 'object') {
      continue;
    }

    const name = typeof raw.name === 'string' ? raw.name.trim() : '';
    const command = typeof raw.command === 'string' ? raw.command : '';
    const icon = sanitizeTerminalQuickActionIcon(raw.icon);
    const enabled = typeof raw.enabled === 'boolean' ? raw.enabled : true;
    const stacked = typeof raw.stacked === 'boolean' ? raw.stacked : false;

    const baseId =
      typeof raw.id === 'string' && raw.id.trim() ? raw.id.trim() : `quick-${index + 1}`;
    let id = baseId;
    let suffix = 1;
    while (usedIds.has(id)) {
      suffix += 1;
      id = `${baseId}-${suffix}`;
    }
    usedIds.add(id);

    sanitized.push({
      id,
      name,
      command,
      icon,
      enabled,
      stacked,
    });
  }

  if (sanitized.length === 0) {
    return DEFAULT_TERMINAL_QUICK_ACTIONS.map(action => ({ ...action }));
  }

  for (const defaultAction of DEFAULT_TERMINAL_QUICK_ACTIONS) {
    if (sanitized.length >= 12) {
      break;
    }
    if (!usedIds.has(defaultAction.id)) {
      sanitized.push({ ...defaultAction });
      usedIds.add(defaultAction.id);
    }
  }

  return sanitized.slice(0, 12);
}

function sanitizeTerminalFont(value?: Partial<TerminalFontSettings>): TerminalFontSettings {
  if (!value) {
    return { ...DEFAULT_TERMINAL_FONT };
  }
  return {
    fontFamily:
      typeof value.fontFamily === 'string' ? value.fontFamily : DEFAULT_TERMINAL_FONT.fontFamily,
    fontSize: sanitizeFontSize(value.fontSize),
    fontWeight: sanitizeFontWeight(value.fontWeight, DEFAULT_TERMINAL_FONT.fontWeight),
    fontWeightBold: sanitizeFontWeight(value.fontWeightBold, DEFAULT_TERMINAL_FONT.fontWeightBold),
    lineHeight: sanitizeLineHeight(value.lineHeight),
    letterSpacing: sanitizeLetterSpacing(value.letterSpacing),
  };
}

const VALID_FONT_WEIGHTS: FontWeight[] = [
  'normal',
  'bold',
  '100',
  '200',
  '300',
  '400',
  '500',
  '600',
  '700',
  '800',
  '900',
];

function sanitizeFontWeight(value: FontWeight | undefined, fallback: FontWeight): FontWeight {
  if (value && VALID_FONT_WEIGHTS.includes(value)) {
    return value;
  }
  return fallback;
}

function sanitizeFontSize(value: number | undefined): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return DEFAULT_TERMINAL_FONT.fontSize;
  }
  return Math.min(Math.max(Math.round(parsed), 8), 32);
}

function sanitizeLineHeight(value: number | undefined): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return DEFAULT_TERMINAL_FONT.lineHeight;
  }
  return Math.min(Math.max(parsed, 1.0), 2.0);
}

function sanitizeLetterSpacing(value: number | undefined): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return DEFAULT_TERMINAL_FONT.letterSpacing;
  }
  return Math.min(Math.max(parsed, -2), 5);
}

const VALID_WEBGL_MODES = ['auto', 'force', 'disable'] as const;

function sanitizeWebGLRenderer(value: string | undefined): 'auto' | 'force' | 'disable' {
  if (value && VALID_WEBGL_MODES.includes(value as 'auto' | 'force' | 'disable')) {
    return value as 'auto' | 'force' | 'disable';
  }
  return defaultSettings.terminalWebGLRenderer;
}
