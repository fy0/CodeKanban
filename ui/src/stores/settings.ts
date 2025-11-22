import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';
import { THEME_PRESETS, DEFAULT_PRESET_ID, getPresetById, getDefaultPreset } from '@/constants/themes';
import { DEFAULT_TERMINAL_THEME_ID } from '@/constants/terminalThemes';

export interface ThemeSettings {
  primaryColor: string;
  surfaceColor: string;
  bodyColor: string;
  textColor?: string;
  terminalBg: string;
  terminalFg: string;
}

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

interface GeneralSettings {
  theme: ThemeSettings;
  currentPresetId: string;
  followSystemTheme: boolean;
  customTheme: ThemeSettings | null;
  recentProjectsLimit: number;
  maxTerminalsPerProject: number;
  panelShortcuts: ShortcutSettings;
  editor: EditorSettings;
  confirmBeforeTerminalClose: boolean;
  terminalThemeId: string;
}

const STORAGE_KEY = 'general_settings';
const DEFAULT_RECENT_PROJECTS_LIMIT = 10;
const DEFAULT_TERMINALS_PER_PROJECT_LIMIT = 12;

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

const defaultSettings: GeneralSettings = {
  theme: { ...defaultTheme },
  currentPresetId: DEFAULT_PRESET_ID,
  followSystemTheme: false,
  customTheme: null,
  recentProjectsLimit: DEFAULT_RECENT_PROJECTS_LIMIT,
  maxTerminalsPerProject: DEFAULT_TERMINALS_PER_PROJECT_LIMIT,
  panelShortcuts: { ...DEFAULT_SHORTCUTS },
  editor: { ...DEFAULT_EDITOR_SETTINGS },
  confirmBeforeTerminalClose: true,
  terminalThemeId: DEFAULT_TERMINAL_THEME_ID,
};

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<GeneralSettings>(loadSettings());

  const theme = computed(() => settings.value.theme);
  const currentPresetId = computed(() => settings.value.currentPresetId);
  const followSystemTheme = computed(() => settings.value.followSystemTheme);
  const customTheme = computed(() => settings.value.customTheme);
  const recentProjectsLimit = computed(() => settings.value.recentProjectsLimit);
  const maxTerminalsPerProject = computed(() => settings.value.maxTerminalsPerProject);
  const panelShortcuts = computed(() => settings.value.panelShortcuts);
  const terminalShortcut = computed(() => panelShortcuts.value.terminal);
  const notepadShortcut = computed(() => panelShortcuts.value.notepad);
  const editorSettings = computed(() => settings.value.editor);
  const confirmBeforeTerminalClose = computed(() => settings.value.confirmBeforeTerminalClose);
  const terminalThemeId = computed(() => settings.value.terminalThemeId);

  /**
   * 计算当前激活的主题
   * 优先级: 跟随系统主题 > 自定义主题 > 预设主题
   *
   * 注意: 在 computed 中访问 window.matchMedia 是为了响应式地获取系统主题偏好
   * App.vue 中会监听系统主题变化事件并更新 store，从而触发此 computed 重新计算
   */
  const activeTheme = computed<ThemeSettings>(() => {
    // 优先级 1: 跟随系统主题
    if (settings.value.followSystemTheme) {
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
    { deep: true },
  );

  function updateTheme(partial: Partial<ThemeSettings>) {
    settings.value.theme = {
      ...settings.value.theme,
      ...partial,
    };
  }

  function resetTheme() {
    // 重置为默认预设主题，并清理自定义/系统跟随状态，保持与 activeTheme 计算逻辑一致
    const preset = getPresetById(DEFAULT_PRESET_ID) ?? getDefaultPreset();
    settings.value.currentPresetId = preset.id;
    settings.value.followSystemTheme = false;
    settings.value.customTheme = null;
    settings.value.theme = { ...preset.colors };
    settings.value.terminalThemeId = preset.terminalThemeId || DEFAULT_TERMINAL_THEME_ID;
  }

  function updateRecentProjectsLimit(limit: number) {
    settings.value.recentProjectsLimit = sanitizeRecentProjectsLimit(limit);
  }

  function updateMaxTerminalsPerProject(limit: number) {
    settings.value.maxTerminalsPerProject = sanitizeTerminalLimit(limit);
  }

  function updatePanelShortcuts(partial: Partial<ShortcutSettings>) {
    settings.value.panelShortcuts = {
      terminal: sanitizePanelShortcut(partial.terminal, settings.value.panelShortcuts.terminal),
      notepad: sanitizePanelShortcut(partial.notepad, settings.value.panelShortcuts.notepad),
    };
  }

  function updateTerminalShortcut(shortcut: PanelShortcutSetting) {
    settings.value.panelShortcuts.terminal = sanitizePanelShortcut(shortcut, settings.value.panelShortcuts.terminal);
  }

  function updateNotepadShortcut(shortcut: PanelShortcutSetting) {
    settings.value.panelShortcuts.notepad = sanitizePanelShortcut(shortcut, settings.value.panelShortcuts.notepad);
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

  function updateConfirmBeforeTerminalClose(value: boolean) {
    settings.value.confirmBeforeTerminalClose = value;
  }

  function updateTerminalTheme(themeId: string) {
    settings.value.terminalThemeId = themeId;
  }

  function selectPreset(presetId: string) {
    const preset = getPresetById(presetId);
    if (preset) {
      settings.value.currentPresetId = presetId;
      settings.value.theme = { ...preset.colors };
      settings.value.customTheme = null;
      // 同步更新终端主题
      if (preset.terminalThemeId) {
        settings.value.terminalThemeId = preset.terminalThemeId;
      }
    }
  }

  function toggleFollowSystemTheme(enabled: boolean) {
    settings.value.followSystemTheme = enabled;
    if (enabled) {
      // 切换到跟随系统模式时，清除自定义主题
      settings.value.customTheme = null;
      // 根据当前系统主题更新预设ID
      const prefersDark = typeof window !== 'undefined'
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
        : false;
      const autoPresetId = prefersDark ? 'dark' : 'light';
      const preset = getPresetById(autoPresetId);
      if (preset) {
        settings.value.currentPresetId = autoPresetId;
        settings.value.theme = { ...preset.colors };
        // 同步更新终端主题
        if (preset.terminalThemeId) {
          settings.value.terminalThemeId = preset.terminalThemeId;
        }
      }
    }
  }

  function applyCustomTheme(themeColors: Partial<ThemeSettings>) {
    settings.value.customTheme = {
      ...activeTheme.value,
      ...themeColors,
    };
    settings.value.theme = settings.value.customTheme;
    settings.value.followSystemTheme = false;
  }

  return {
    theme,
    currentPresetId,
    followSystemTheme,
    customTheme,
    activeTheme,
    recentProjectsLimit,
    maxTerminalsPerProject,
    panelShortcuts,
    terminalShortcut,
    notepadShortcut,
    editorSettings,
    confirmBeforeTerminalClose,
    terminalThemeId,
    updateTheme,
    resetTheme,
    updateRecentProjectsLimit,
    updateMaxTerminalsPerProject,
    updatePanelShortcuts,
    updateTerminalShortcut,
    updateNotepadShortcut,
    resetTerminalShortcut,
    resetNotepadShortcut,
    updateEditorSettings,
    updateConfirmBeforeTerminalClose,
    updateTerminalTheme,
    selectPreset,
    toggleFollowSystemTheme,
    applyCustomTheme,
  };
});

function loadSettings(): GeneralSettings {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as Partial<GeneralSettings> & {
        panelShortcut?: PanelShortcutSetting;
      };

      // 兼容旧版本：如果没有 currentPresetId，根据主题判断
      let currentPresetId = parsed.currentPresetId ?? DEFAULT_PRESET_ID;
      if (!parsed.currentPresetId && parsed.theme) {
        // 尝试匹配现有主题到预设
        const matchedPreset = THEME_PRESETS.find(
          p => p.colors.primaryColor === parsed.theme?.primaryColor
        );
        if (matchedPreset) {
          currentPresetId = matchedPreset.id;
        }
      }

      return {
        theme: {
          ...defaultTheme,
          ...parsed.theme,
        },
        currentPresetId,
        followSystemTheme: parsed.followSystemTheme ?? false,
        customTheme: parsed.customTheme ?? null,
        recentProjectsLimit: sanitizeRecentProjectsLimit(parsed.recentProjectsLimit),
        maxTerminalsPerProject: sanitizeTerminalLimit(parsed.maxTerminalsPerProject),
        panelShortcuts: sanitizePanelShortcuts(parsed.panelShortcuts ?? parsed.panelShortcut),
        editor: sanitizeEditorSettings(parsed.editor),
        confirmBeforeTerminalClose: parsed.confirmBeforeTerminalClose ?? defaultSettings.confirmBeforeTerminalClose,
        terminalThemeId: parsed.terminalThemeId ?? defaultSettings.terminalThemeId,
      };
    }
  } catch (error) {
    console.warn('Failed to load settings, falling back to defaults.', error);
  }
  return cloneDefaultSettings();
}

function saveSettings(settings: GeneralSettings) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch (error) {
    console.error('Failed to persist settings:', error);
  }
}

function cloneDefaultSettings(): GeneralSettings {
  return {
    theme: { ...defaultSettings.theme },
    currentPresetId: defaultSettings.currentPresetId,
    followSystemTheme: defaultSettings.followSystemTheme,
    terminalThemeId: defaultSettings.terminalThemeId,
    customTheme: defaultSettings.customTheme,
    recentProjectsLimit: defaultSettings.recentProjectsLimit,
    maxTerminalsPerProject: defaultSettings.maxTerminalsPerProject,
    panelShortcuts: {
      terminal: { ...defaultSettings.panelShortcuts.terminal },
      notepad: { ...defaultSettings.panelShortcuts.notepad },
    },
    editor: { ...defaultSettings.editor },
    confirmBeforeTerminalClose: defaultSettings.confirmBeforeTerminalClose,
  };
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

function sanitizeEditorSettings(value?: Partial<EditorSettings>): EditorSettings {
  if (!value) {
    return { ...DEFAULT_EDITOR_SETTINGS };
  }
  const normalized = typeof value.defaultEditor === 'string' ? value.defaultEditor.toLowerCase().trim() : '';
  const supported = SUPPORTED_EDITORS.includes(normalized as EditorPreference)
    ? (normalized as EditorPreference)
    : DEFAULT_EDITOR_SETTINGS.defaultEditor;
  const customCommand =
    typeof value.customCommand === 'string' ? value.customCommand : DEFAULT_EDITOR_SETTINGS.customCommand;
  return {
    defaultEditor: supported,
    customCommand,
  };
}

function sanitizePanelShortcuts(value?: Partial<ShortcutSettings> | PanelShortcutSetting): ShortcutSettings {
  if (value && 'terminal' in (value as ShortcutSettings)) {
    const partial = value as Partial<ShortcutSettings>;
    return {
      terminal: sanitizePanelShortcut(partial.terminal, DEFAULT_TERMINAL_SHORTCUT),
      notepad: sanitizePanelShortcut(partial.notepad, DEFAULT_NOTEPAD_SHORTCUT),
    };
  }
  if (value && 'code' in (value as PanelShortcutSetting)) {
    const shortcut = sanitizePanelShortcut(value as PanelShortcutSetting, DEFAULT_TERMINAL_SHORTCUT);
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
  fallback: PanelShortcutSetting,
): PanelShortcutSetting {
  const base = fallback ?? DEFAULT_TERMINAL_SHORTCUT;
  const code = typeof value?.code === 'string' && value.code.trim().length ? value.code : base.code;
  const display =
    typeof value?.display === 'string' && value.display.trim().length ? value.display : deriveDisplayFromCode(code);
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
