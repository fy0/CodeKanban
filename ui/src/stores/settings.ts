import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

export interface ThemeSettings {
  primaryColor: string;
  surfaceColor: string;
  bodyColor: string;
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
  recentProjectsLimit: number;
  maxTerminalsPerProject: number;
  panelShortcuts: ShortcutSettings;
  editor: EditorSettings;
  confirmBeforeTerminalClose: boolean;
}

const STORAGE_KEY = 'general_settings';
const DEFAULT_RECENT_PROJECTS_LIMIT = 10;
const DEFAULT_TERMINALS_PER_PROJECT_LIMIT = 12;

const defaultTheme: ThemeSettings = {
  primaryColor: '#3B69A9',
  surfaceColor: '#ffffff',
  bodyColor: '#f7f8fa',
};

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
  recentProjectsLimit: DEFAULT_RECENT_PROJECTS_LIMIT,
  maxTerminalsPerProject: DEFAULT_TERMINALS_PER_PROJECT_LIMIT,
  panelShortcuts: { ...DEFAULT_SHORTCUTS },
  editor: { ...DEFAULT_EDITOR_SETTINGS },
  confirmBeforeTerminalClose: true,
};

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<GeneralSettings>(loadSettings());

  const theme = computed(() => settings.value.theme);
  const recentProjectsLimit = computed(() => settings.value.recentProjectsLimit);
  const maxTerminalsPerProject = computed(() => settings.value.maxTerminalsPerProject);
  const panelShortcuts = computed(() => settings.value.panelShortcuts);
  const terminalShortcut = computed(() => panelShortcuts.value.terminal);
  const notepadShortcut = computed(() => panelShortcuts.value.notepad);
  const editorSettings = computed(() => settings.value.editor);
  const confirmBeforeTerminalClose = computed(() => settings.value.confirmBeforeTerminalClose);

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
    settings.value.theme = { ...defaultTheme };
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

  return {
    theme,
    recentProjectsLimit,
    maxTerminalsPerProject,
    panelShortcuts,
    terminalShortcut,
    notepadShortcut,
    editorSettings,
    confirmBeforeTerminalClose,
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
  };
});

function loadSettings(): GeneralSettings {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as Partial<GeneralSettings> & {
        panelShortcut?: PanelShortcutSetting;
      };
      return {
        theme: {
          ...defaultTheme,
          ...parsed.theme,
        },
        recentProjectsLimit: sanitizeRecentProjectsLimit(parsed.recentProjectsLimit),
        maxTerminalsPerProject: sanitizeTerminalLimit(parsed.maxTerminalsPerProject),
        panelShortcuts: sanitizePanelShortcuts(parsed.panelShortcuts ?? parsed.panelShortcut),
        editor: sanitizeEditorSettings(parsed.editor),
        confirmBeforeTerminalClose: parsed.confirmBeforeTerminalClose ?? defaultSettings.confirmBeforeTerminalClose,
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
