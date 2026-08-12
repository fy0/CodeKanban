import type { GlobalThemeOverrides } from 'naive-ui';
import type { ThemeSettings } from '@/stores/settings';
import { darkenColor, hexToRgba, lightenColor, isDarkHex } from './color';
import { createThemeSemanticPalette } from './themeSemanticPalette';

/**
 * 根据主题设置创建 Naive UI 全局主题覆盖配置
 * @param theme 主题设置
 * @param resolvedTextColor 解析后的文本颜色
 * @param inputBorderColor 输入框边框颜色
 * @param inputBorderHoverColor 输入框悬停边框颜色
 * @returns Naive UI 主题覆盖配置
 */
export function createThemeOverrides(
  theme: ThemeSettings,
  resolvedTextColor: string,
  inputBorderColor: string,
  inputBorderHoverColor: string
): GlobalThemeOverrides {
  const { primaryColor, bodyColor, surfaceColor } = theme;

  // 计算主色调的 hover 和 pressed 状态
  const primaryHover = lightenColor(primaryColor, 0.08);
  const primaryPressed = darkenColor(primaryColor, 0.12);

  const isDark = isDarkHex(bodyColor || '#ffffff');
  const palette = createThemeSemanticPalette(theme, resolvedTextColor);

  // 设置文字颜色（如果主题提供了 textColor 则使用，否则根据暗色/亮色自动设置）
  const primaryTextColor = resolvedTextColor;
  const secondaryTextColor = palette.textSecondary;
  const mutedTextColor = palette.textMuted;

  return {
    common: {
      // 基础颜色
      bodyColor,
      cardColor: surfaceColor,
      modalColor: palette.surfaceRaised,
      popoverColor: palette.surfaceRaised,
      tableColor: surfaceColor,

      // 主色调配置
      primaryColor,
      primaryColorHover: primaryHover,
      primaryColorPressed: primaryPressed,
      primaryColorSuppl: primaryHover,

      // 文本颜色配置
      textColorBase: primaryTextColor,
      textColor1: primaryTextColor,
      textColor2: secondaryTextColor,
      textColor3: mutedTextColor,

      // 边框颜色
      borderColor: palette.border,

      // Tab 配置
      tabColor: palette.surface,

      // 功能色配置
      errorColor: palette.error,
      warningColor: palette.warning,
      successColor: palette.success,
      infoColor: palette.info,
    },
    Layout: {
      color: surfaceColor,
      siderColor: surfaceColor,
      headerColor: surfaceColor,
      footerColor: surfaceColor,
      textColor: primaryTextColor,
    },
    Input: {
      color: isDark ? palette.surfaceSunken : palette.surface,
      colorFocus: isDark ? palette.surfaceSunken : palette.surface,
      textColor: primaryTextColor,
      placeholderColor: mutedTextColor,
      border: `1px solid ${inputBorderColor}`,
      borderHover: `1px solid ${inputBorderHoverColor}`,
      borderFocus: `1px solid ${palette.focusRing}`,
    },
    InputNumber: {
      color: isDark ? palette.surfaceSunken : palette.surface,
      colorFocus: isDark ? palette.surfaceSunken : palette.surface,
      textColor: primaryTextColor,
      border: `1px solid ${inputBorderColor}`,
      borderHover: `1px solid ${inputBorderHoverColor}`,
      borderFocus: `1px solid ${palette.focusRing}`,
    },
    Select: {
      peers: {
        InternalSelection: {
          color: isDark ? palette.surfaceSunken : palette.surface,
          colorActive: isDark ? palette.surfaceSunken : palette.surface,
          textColor: primaryTextColor,
          placeholderColor: mutedTextColor,
          border: `1px solid ${inputBorderColor}`,
          borderHover: `1px solid ${inputBorderHoverColor}`,
          borderActive: `1px solid ${palette.focusRing}`,
          borderFocus: `1px solid ${palette.focusRing}`,
        },
        InternalSelectMenu: {
          color: palette.surfaceRaised,
          optionTextColor: primaryTextColor,
          optionTextColorActive: primaryColor,
          optionColorActive: palette.surfaceActive,
        },
      },
    },
    Tag: {
      textColor: primaryTextColor,
      color: palette.surfaceHover,
      colorBordered: palette.surfaceHover,
      border: `1px solid ${palette.border}`,
    },
    Button: {
      textColor: primaryTextColor,
      textColorText: primaryTextColor,
      textColorTextHover: primaryColor,
      textColorTextPressed: primaryPressed,
      textColorTextDisabled: mutedTextColor,
      colorPrimary: primaryColor,
      textColorPrimary: palette.accentContrast,
      textColorHoverPrimary: palette.accentContrast,
      textColorPressedPrimary: palette.accentContrast,
      colorHoverPrimary: primaryHover,
      colorPressedPrimary: primaryPressed,
      borderPrimary: `1px solid ${primaryColor}`,
      borderHoverPrimary: `1px solid ${primaryHover}`,
      borderPressedPrimary: `1px solid ${primaryPressed}`,
    },
    Scrollbar: {
      width: '8px',
      height: '8px',
      color: hexToRgba(primaryTextColor, 0.18),
      colorHover: hexToRgba(primaryTextColor, 0.3),
    },
    Card: {
      color: surfaceColor,
      textColor: primaryTextColor,
      titleTextColor: primaryTextColor,
      borderColor: palette.border,
    },
    Divider: {
      color: palette.border,
    },
    Popover: {
      color: palette.surfaceRaised,
      textColor: primaryTextColor,
    },
    Modal: {
      color: palette.surfaceRaised,
      textColor: primaryTextColor,
      titleTextColor: primaryTextColor,
    },
    Drawer: {
      color: surfaceColor,
      textColor: primaryTextColor,
      titleTextColor: primaryTextColor,
    },
  };
}
