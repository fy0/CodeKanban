import type { ThemeSettings } from '@/stores/settings';
import { darkenColor, hexToRgba, isDarkHex, lightenColor } from './color';

export interface ThemeSemanticPalette {
  colorScheme: 'light' | 'dark';
  canvas: string;
  surface: string;
  surfaceRaised: string;
  surfaceSunken: string;
  surfaceHover: string;
  surfaceActive: string;
  textPrimary: string;
  textSecondary: string;
  textMuted: string;
  textInverse: string;
  border: string;
  borderStrong: string;
  controlBorder: string;
  controlBorderHover: string;
  focusRing: string;
  accent: string;
  accentHover: string;
  accentPressed: string;
  accentSoft: string;
  accentContrast: string;
  link: string;
  linkHover: string;
  success: string;
  successSoft: string;
  warning: string;
  warningSoft: string;
  error: string;
  errorSoft: string;
  info: string;
  infoSoft: string;
  overlay: string;
  shadow: string;
}

export function createThemeSemanticPalette(
  theme: ThemeSettings,
  resolvedTextColor: string
): ThemeSemanticPalette {
  const isDark = isDarkHex(theme.bodyColor || '#ffffff');
  const surface = theme.surfaceColor || (isDark ? '#252526' : '#ffffff');
  const accent = theme.primaryColor || (isDark ? '#007acc' : '#3b69a9');
  const textPrimary = resolvedTextColor || (isDark ? '#d4d4d4' : '#333333');

  return {
    colorScheme: isDark ? 'dark' : 'light',
    canvas: theme.bodyColor,
    surface,
    surfaceRaised: lightenColor(surface, isDark ? 0.035 : 0.018),
    surfaceSunken: isDark
      ? darkenColor(theme.bodyColor, 0.025)
      : darkenColor(theme.bodyColor, 0.018),
    surfaceHover: isDark ? lightenColor(surface, 0.06) : darkenColor(surface, 0.025),
    surfaceActive: isDark ? lightenColor(surface, 0.1) : darkenColor(surface, 0.055),
    textPrimary,
    textSecondary: hexToRgba(textPrimary, isDark ? 0.82 : 0.74),
    textMuted: hexToRgba(textPrimary, isDark ? 0.66 : 0.68),
    textInverse: isDark ? '#1e1e1e' : '#ffffff',
    border: isDark ? '#3c3c3c' : '#e0e0e0',
    borderStrong: isDark ? '#5a5a5a' : '#b8bcc8',
    controlBorder: isDark ? '#686868' : '#d0d5dd',
    controlBorderHover: isDark ? '#878787' : '#b8bcc8',
    focusRing: isDark ? '#75beff' : accent,
    accent,
    accentHover: lightenColor(accent, 0.08),
    accentPressed: darkenColor(accent, 0.12),
    accentSoft: hexToRgba(accent, isDark ? 0.2 : 0.1),
    accentContrast: isDarkHex(accent) ? '#ffffff' : '#1e1e1e',
    link: isDark ? '#75beff' : '#255a8f',
    linkHover: isDark ? '#9cdcfe' : '#1f4c7f',
    success: isDark ? '#89d185' : '#137333',
    successSoft: isDark ? 'rgba(137, 209, 133, 0.16)' : 'rgba(19, 115, 51, 0.1)',
    warning: isDark ? '#cca700' : '#8a5d00',
    warningSoft: isDark ? 'rgba(204, 167, 0, 0.16)' : 'rgba(138, 93, 0, 0.1)',
    error: isDark ? '#f48771' : '#b42318',
    errorSoft: isDark ? 'rgba(244, 135, 113, 0.16)' : 'rgba(180, 35, 24, 0.1)',
    info: isDark ? '#75beff' : '#255a8f',
    infoSoft: isDark ? 'rgba(117, 190, 255, 0.16)' : 'rgba(37, 90, 143, 0.1)',
    overlay: isDark ? 'rgba(0, 0, 0, 0.62)' : 'rgba(15, 23, 42, 0.38)',
    shadow: isDark ? 'rgba(0, 0, 0, 0.38)' : 'rgba(24, 35, 51, 0.12)',
  };
}
