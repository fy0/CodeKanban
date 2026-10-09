import type { ThemeSettings } from '@/stores/settings';
import type { TerminalThemeSettings } from '@/constants/terminalThemes';
import { contrastRatio, ensureContrast, type Palette } from './core';

/** Map semantic theme tokens to the existing business-component contract. */
export function toLegacyTheme(p: Palette): ThemeSettings {
  return {
    primaryColor: p.accent, bodyColor: p.canvas, surfaceColor: p.surface, textColor: p.text,
    surfaceRaisedColor: p.raised, surfaceSunkenColor: p.sunken, surfaceHoverColor: p.sunken,
    secondaryTextColor: p.muted, mutedTextColor: p.subtle, borderColor: p.border,
    borderStrongColor: p.muted, controlBorderColor: p.border, controlBorderHoverColor: p.accent,
    focusRingColor: p.accent, linkColor: p.accent, planColor: p.plan, workingColor: p.working,
    completionColor: p.success, approvalColor: p.warning, planApprovalColor: p.plan,
    redirectColor: p.info, queueColor: p.muted, successColor: p.success, warningColor: p.warning,
    errorColor: p.error, infoColor: p.info, projectTerminalColor: p.working,
    projectWebSessionColor: p.accent, changeAdditionColor: p.addition, changeDeletionColor: p.deletion,
    changeWarningColor: p.warning, workspaceTopColor: p.canvas, workspaceBottomColor: p.canvas,
    sidebarFooterColor: p.sunken, terminalTabBg: p.sunken, terminalTabActiveBg: p.surface,
    terminalTabTextColor: p.muted, terminalTabActiveTextColor: p.text,
    terminalHeaderBorder: true, terminalHeaderBorderColor: p.border,
    terminalEmptyGuideFg: p.terminalFg, terminalStatusReadyColor: p.success,
    terminalStatusConnectingColor: p.warning, terminalStatusErrorColor: p.error,
  };
}
export function toTerminalTheme(p: Palette): TerminalThemeSettings {
  const dark = contrastRatio(p.terminalBg, '#FFFFFF') > contrastRatio(p.terminalBg, '#000000');
  const ink = (color: string) => ensureContrast(color, p.terminalBg);
  return {
    background: p.terminalBg, foreground: p.terminalFg, cursor: ink(p.accent),
    cursorAccent: p.terminalBg, selectionBackground: `${p.accent}40`,
    black: ink(dark ? '#64756C' : '#243B30'), red: ink(p.error), green: ink(p.success), yellow: ink(p.warning),
    blue: ink(p.info), magenta: ink(p.plan), cyan: ink(p.working), white: ink(p.terminalFg),
    brightBlack: ink(p.muted), brightRed: ink(p.error), brightGreen: ink(p.success), brightYellow: ink(p.warning),
    brightBlue: ink(p.info), brightMagenta: ink(p.plan), brightCyan: ink(p.working), brightWhite: ink(p.terminalFg),
  };
}
