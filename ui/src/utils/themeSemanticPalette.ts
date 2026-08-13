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
  plan: string;
  planSoft: string;
  working: string;
  workingSoft: string;
  completion: string;
  completionSoft: string;
  approval: string;
  approvalSoft: string;
  planApproval: string;
  planApprovalSoft: string;
  redirect: string;
  redirectSoft: string;
  queue: string;
  queueSoft: string;
  success: string;
  successSoft: string;
  warning: string;
  warningSoft: string;
  warningContrast: string;
  error: string;
  errorSoft: string;
  info: string;
  infoSoft: string;
  projectTerminal: string;
  projectTerminalSoft: string;
  projectWebSession: string;
  projectWebSessionSoft: string;
  changeAddition: string;
  changeDeletion: string;
  changeWarning: string;
  overlay: string;
  shadow: string;
  shadowSubtle: string;
  activeOutlineMix: string;
  workspaceBackground: string;
  sidebarFooter: string;
}

function optionalColor(value: string | undefined, fallback: string): string {
  return value?.trim() || fallback;
}

function translucentColor(color: string, alpha: number): string {
  if (/^#?[0-9a-f]{3}([0-9a-f]{3})?$/i.test(color)) {
    return hexToRgba(color, alpha);
  }
  return `color-mix(in srgb, ${color} ${Math.round(alpha * 100)}%, transparent)`;
}

export function createThemeSemanticPalette(
  theme: ThemeSettings,
  resolvedTextColor: string
): ThemeSemanticPalette {
  const isDark = isDarkHex(theme.bodyColor || '#ffffff');
  const surface = theme.surfaceColor || (isDark ? '#252526' : '#ffffff');
  const accent = theme.primaryColor || (isDark ? '#007acc' : '#3b69a9');
  const textPrimary = resolvedTextColor || (isDark ? '#d4d4d4' : '#333333');
  const surfaceRaised = optionalColor(
    theme.surfaceRaisedColor,
    lightenColor(surface, isDark ? 0.035 : 0.018)
  );
  const surfaceSunken = optionalColor(
    theme.surfaceSunkenColor,
    isDark ? darkenColor(theme.bodyColor, 0.025) : darkenColor(theme.bodyColor, 0.018)
  );
  const surfaceHover = optionalColor(
    theme.surfaceHoverColor,
    isDark ? lightenColor(surface, 0.06) : darkenColor(surface, 0.04)
  );
  const surfaceActive = optionalColor(
    theme.surfaceActiveColor,
    isDark ? lightenColor(surface, 0.1) : darkenColor(surface, 0.055)
  );
  const border = optionalColor(theme.borderColor, isDark ? '#3c3c3c' : '#e0e0e0');
  const borderStrong = optionalColor(theme.borderStrongColor, isDark ? '#5a5a5a' : '#b8bcc8');
  const controlBorder = optionalColor(theme.controlBorderColor, isDark ? '#686868' : '#d0d5dd');
  const link = optionalColor(theme.linkColor, isDark ? '#75beff' : '#255a8f');
  const plan = optionalColor(theme.planColor, isDark ? '#75beff' : '#6366f1');
  const working = optionalColor(theme.workingColor, isDark ? accent : '#8b5cf6');
  const completion = optionalColor(theme.completionColor, isDark ? '#89d185' : '#10b981');
  const approval = optionalColor(theme.approvalColor, isDark ? '#cca700' : '#f79009');
  const planApproval = optionalColor(theme.planApprovalColor, isDark ? '#4ec9b0' : '#0891b2');
  const redirect = optionalColor(theme.redirectColor, isDark ? '#75beff' : '#2563eb');
  const queue = optionalColor(theme.queueColor, isDark ? '#a5b4fc' : '#4f46e5');
  const success = optionalColor(theme.successColor, isDark ? '#89d185' : '#52c41a');
  const warning = optionalColor(theme.warningColor, isDark ? '#cca700' : '#fa8c16');
  const error = optionalColor(theme.errorColor, isDark ? '#f48771' : '#f5222d');
  const info = optionalColor(theme.infoColor, isDark ? '#75beff' : accent);
  const projectTerminal = optionalColor(theme.projectTerminalColor, isDark ? '#89d185' : '#18a058');
  const projectWebSession = optionalColor(
    theme.projectWebSessionColor,
    isDark ? '#75beff' : '#2080f0'
  );
  const changeAddition = optionalColor(theme.changeAdditionColor, isDark ? '#89d185' : '#15803d');
  const changeDeletion = optionalColor(theme.changeDeletionColor, isDark ? '#f48771' : '#dc2626');
  const changeWarning = optionalColor(theme.changeWarningColor, isDark ? '#cca700' : '#b45309');
  const shadow = optionalColor(
    theme.shadowColor,
    isDark ? 'rgba(0, 0, 0, 0.38)' : 'rgba(0, 0, 0, 0.12)'
  );
  const shadowSubtle = optionalColor(
    theme.shadowSubtleColor,
    isDark ? 'rgba(0, 0, 0, 0.19)' : 'rgba(15, 23, 42, 0.06)'
  );
  const workspaceTop = optionalColor(theme.workspaceTopColor, isDark ? theme.bodyColor : '#f6f1e8');
  const workspaceBottom = optionalColor(
    theme.workspaceBottomColor,
    isDark ? theme.bodyColor : surface
  );

  return {
    colorScheme: isDark ? 'dark' : 'light',
    canvas: theme.bodyColor,
    surface,
    surfaceRaised,
    surfaceSunken,
    surfaceHover,
    surfaceActive,
    textPrimary,
    textSecondary: optionalColor(
      theme.secondaryTextColor,
      isDark ? hexToRgba(textPrimary, 0.82) : 'rgba(0, 0, 0, 0.65)'
    ),
    textMuted: optionalColor(
      theme.mutedTextColor,
      isDark ? hexToRgba(textPrimary, 0.66) : 'rgba(0, 0, 0, 0.45)'
    ),
    textInverse: isDark ? '#1e1e1e' : '#ffffff',
    border,
    borderStrong,
    controlBorder,
    controlBorderHover: optionalColor(
      theme.controlBorderHoverColor,
      isDark ? '#878787' : '#b8bcc8'
    ),
    focusRing: optionalColor(theme.focusRingColor, isDark ? '#75beff' : accent),
    accent,
    accentHover: lightenColor(accent, 0.08),
    accentPressed: darkenColor(accent, 0.12),
    accentSoft: hexToRgba(accent, isDark ? 0.2 : 0.1),
    accentContrast: isDarkHex(accent) ? '#ffffff' : '#1e1e1e',
    link,
    linkHover: isDark
      ? lightenColor(link, 0.08)
      : theme.linkColor
        ? darkenColor(link, 0.08)
        : '#1f4c7f',
    plan,
    planSoft: translucentColor(plan, isDark ? 0.2 : 0.12),
    working,
    workingSoft: translucentColor(working, isDark ? 0.2 : 0.12),
    completion,
    completionSoft: translucentColor(completion, isDark ? 0.16 : 0.12),
    approval,
    approvalSoft: translucentColor(approval, isDark ? 0.16 : 0.12),
    planApproval,
    planApprovalSoft: translucentColor(planApproval, isDark ? 0.16 : 0.14),
    redirect,
    redirectSoft: translucentColor(redirect, isDark ? 0.16 : 0.12),
    queue,
    queueSoft: translucentColor(queue, isDark ? 0.16 : 0.12),
    success,
    successSoft: translucentColor(success, isDark ? 0.16 : 0.12),
    warning,
    warningSoft: translucentColor(warning, isDark ? 0.16 : 0.12),
    warningContrast: optionalColor(theme.warningContrastColor, isDark ? '#1e1e1e' : '#1f2328'),
    error,
    errorSoft: translucentColor(error, isDark ? 0.16 : 0.12),
    info,
    infoSoft: translucentColor(info, isDark ? 0.16 : 0.12),
    projectTerminal,
    projectTerminalSoft: optionalColor(
      theme.projectTerminalSoftColor,
      isDark ? translucentColor(projectTerminal, 0.16) : '#eaf8e3'
    ),
    projectWebSession,
    projectWebSessionSoft: optionalColor(
      theme.projectWebSessionSoftColor,
      isDark ? translucentColor(projectWebSession, 0.16) : '#e7edf5'
    ),
    changeAddition,
    changeDeletion,
    changeWarning,
    overlay: isDark ? 'rgba(0, 0, 0, 0.62)' : 'rgba(15, 23, 42, 0.38)',
    shadow,
    shadowSubtle,
    activeOutlineMix: isDark ? optionalColor(theme.focusRingColor, '#75beff') : '#ffffff',
    workspaceBackground: isDark
      ? workspaceTop
      : `linear-gradient(180deg, ${translucentColor(workspaceTop, 0.78)}, ${translucentColor(workspaceBottom, 0.94)})`,
    sidebarFooter: optionalColor(theme.sidebarFooterColor, isDark ? surfaceSunken : surface),
  };
}
