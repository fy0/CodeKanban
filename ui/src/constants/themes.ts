import type { ThemeSettings } from '@/stores/settings';

export interface ThemePreset {
  id: string;
  name: string;
  nameEn: string;
  isDark: boolean;
  colors: ThemeSettings;
  terminalThemeId: string;
}

export const THEME_PRESETS: ThemePreset[] = [
  {
    id: 'light',
    name: '亮色',
    nameEn: 'Light',
    isDark: false,
    terminalThemeId: 'light',
    colors: {
      primaryColor: '#3B69A9',
      bodyColor: '#F7F8FA',
      surfaceColor: '#FFFFFF',
      textColor: '#333333',
      terminalBg: '#FAFAFA',
      terminalFg: '#1E1E1E',
    },
  },
  {
    id: 'dark',
    name: '暗色',
    nameEn: 'Dark',
    isDark: true,
    terminalThemeId: 'nord',
    colors: {
      primaryColor: '#007ACC',
      bodyColor: '#1E1E1E',
      surfaceColor: '#252526',
      textColor: '#D4D4D4',
      terminalBg: '#2E3440',
      terminalFg: '#D8DEE9',
    },
  },
  {
    id: 'dim',
    name: '柔和暗色',
    nameEn: 'Dim',
    isDark: true,
    terminalThemeId: 'github-dark',
    colors: {
      primaryColor: '#539BF5',
      bodyColor: '#22272E',
      surfaceColor: '#2D333B',
      textColor: '#BDC9D4',
      terminalBg: '#0D1117',
      terminalFg: '#E6EDF3',
    },
  },
  {
    id: 'warm',
    name: '暖色',
    nameEn: 'Warm',
    isDark: false,
    terminalThemeId: 'warm-light',
    colors: {
      primaryColor: '#D97706',
      bodyColor: '#FEF3C7',
      surfaceColor: '#FFFBEB',
      textColor: '#78350F',
      terminalBg: '#FFF7ED',
      terminalFg: '#7C2D12',
    },
  },
  {
    id: 'material-dark',
    name: 'Material 暗色',
    nameEn: 'Material Dark',
    isDark: true,
    terminalThemeId: 'material-dark',
    colors: {
      primaryColor: '#BB86FC',
      bodyColor: '#121212',
      surfaceColor: '#1F1B24',
      textColor: '#E6E1E5',
      terminalBg: '#0F0F0F',
      terminalFg: '#E6E1E5',
    },
  },
  {
    id: 'xcode-dark',
    name: 'Xcode 暗色',
    nameEn: 'Xcode Dark',
    isDark: true,
    terminalThemeId: 'xcode-dark',
    colors: {
      primaryColor: '#0A84FF',
      bodyColor: '#1E1F22',
      surfaceColor: '#2C2D31',
      textColor: '#F5F5F7',
      terminalBg: '#1A1B1E',
      terminalFg: '#F5F5F7',
    },
  },
  {
    id: 'solarized-light',
    name: 'Solarized 亮色',
    nameEn: 'Solarized Light',
    isDark: false,
    terminalThemeId: 'solarized-light',
    colors: {
      primaryColor: '#268BD2',
      bodyColor: '#FDF6E3',
      surfaceColor: '#F5EFD5',
      textColor: '#073642',
      terminalBg: '#FDF6E3',
      terminalFg: '#586E75',
    },
  },
  {
    id: 'solarized-dark',
    name: 'Solarized 暗色',
    nameEn: 'Solarized Dark',
    isDark: true,
    terminalThemeId: 'solarized-dark',
    colors: {
      primaryColor: '#859900',
      bodyColor: '#002B36',
      surfaceColor: '#073642',
      textColor: '#EEE8D5',
      terminalBg: '#002B36',
      terminalFg: '#93A1A1',
    },
  },
  {
    id: 'nord',
    name: 'Nord',
    nameEn: 'Nord',
    isDark: true,
    terminalThemeId: 'nord',
    colors: {
      primaryColor: '#88C0D0',
      bodyColor: '#2E3440',
      surfaceColor: '#3B4252',
      textColor: '#ECEFF4',
      terminalBg: '#2E3440',
      terminalFg: '#D8DEE9',
    },
  },
  {
    id: 'dracula',
    name: 'Dracula',
    nameEn: 'Dracula',
    isDark: true,
    terminalThemeId: 'dracula',
    colors: {
      primaryColor: '#BD93F9',
      bodyColor: '#282A36',
      surfaceColor: '#343746',
      textColor: '#F8F8F2',
      terminalBg: '#282A36',
      terminalFg: '#F8F8F2',
    },
  },
  {
    id: 'one-dark',
    name: 'One Dark',
    nameEn: 'One Dark',
    isDark: true,
    terminalThemeId: 'one-dark',
    colors: {
      primaryColor: '#61AFEF',
      bodyColor: '#21252B',
      surfaceColor: '#282C34',
      textColor: '#ABB2BF',
      terminalBg: '#1E2127',
      terminalFg: '#ABB2BF',
    },
  },
  {
    id: 'tokyo-night',
    name: 'Tokyo Night',
    nameEn: 'Tokyo Night',
    isDark: true,
    terminalThemeId: 'tokyo-night',
    colors: {
      primaryColor: '#7AA2F7',
      bodyColor: '#1A1B26',
      surfaceColor: '#24283B',
      textColor: '#C0CAF5',
      terminalBg: '#1A1B26',
      terminalFg: '#A9B1D6',
    },
  },
  {
    id: 'github-dark',
    name: 'GitHub 暗色',
    nameEn: 'GitHub Dark',
    isDark: true,
    terminalThemeId: 'github-dark',
    colors: {
      primaryColor: '#58A6FF',
      bodyColor: '#0D1117',
      surfaceColor: '#161B22',
      textColor: '#E6EDF3',
      terminalBg: '#0D1117',
      terminalFg: '#E6EDF3',
    },
  },
  {
    id: 'monokai',
    name: 'Monokai',
    nameEn: 'Monokai',
    isDark: true,
    terminalThemeId: 'monokai',
    colors: {
      primaryColor: '#A6E22E',
      bodyColor: '#272822',
      surfaceColor: '#3E3D32',
      textColor: '#F8F8F2',
      terminalBg: '#272822',
      terminalFg: '#F8F8F2',
    },
  },
  {
    id: 'gruvbox-dark',
    name: 'Gruvbox 暗色',
    nameEn: 'Gruvbox Dark',
    isDark: true,
    terminalThemeId: 'gruvbox-dark',
    colors: {
      primaryColor: '#FABD2F',
      bodyColor: '#282828',
      surfaceColor: '#3C3836',
      textColor: '#EBDBB2',
      terminalBg: '#282828',
      terminalFg: '#EBDBB2',
    },
  },
  {
    id: 'catppuccin-mocha',
    name: 'Catppuccin',
    nameEn: 'Catppuccin Mocha',
    isDark: true,
    terminalThemeId: 'catppuccin',
    colors: {
      primaryColor: '#CBA6F7',
      bodyColor: '#1E1E2E',
      surfaceColor: '#313244',
      textColor: '#CDD6F4',
      terminalBg: '#1E1E2E',
      terminalFg: '#CDD6F4',
    },
  },
  {
    id: 'github-light',
    name: 'GitHub 亮色',
    nameEn: 'GitHub Light',
    isDark: false,
    terminalThemeId: 'github-light',
    colors: {
      primaryColor: '#0969DA',
      bodyColor: '#F6F8FA',
      surfaceColor: '#FFFFFF',
      textColor: '#1F2328',
      terminalBg: '#F6F8FA',
      terminalFg: '#1F2328',
    },
  },
  {
    id: 'gruvbox-light',
    name: 'Gruvbox 亮色',
    nameEn: 'Gruvbox Light',
    isDark: false,
    terminalThemeId: 'gruvbox-light',
    colors: {
      primaryColor: '#B57614',
      bodyColor: '#FBF1C7',
      surfaceColor: '#F2E5BC',
      textColor: '#3C3836',
      terminalBg: '#FBF1C7',
      terminalFg: '#3C3836',
    },
  },
];

export const DEFAULT_PRESET_ID = 'dark';

export function getPresetById(id: string): ThemePreset | undefined {
  return THEME_PRESETS.find(preset => preset.id === id);
}

export function getDefaultPreset(): ThemePreset {
  return getPresetById(DEFAULT_PRESET_ID) || THEME_PRESETS[0];
}
