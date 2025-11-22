import type { GlobalThemeOverrides } from 'naive-ui';
import type { ThemeSettings } from '@/stores/settings';
import { darkenColor, lightenColor, isDarkHex } from './color';

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
  inputBorderHoverColor: string,
): GlobalThemeOverrides {
  const { primaryColor, bodyColor, surfaceColor } = theme;

  // 计算主色调的 hover 和 pressed 状态
  const primaryHover = lightenColor(primaryColor, 0.08);
  const primaryPressed = darkenColor(primaryColor, 0.12);

  const isDark = isDarkHex(bodyColor || '#ffffff');

  // 设置文字颜色（如果主题提供了 textColor 则使用，否则根据暗色/亮色自动设置）
  const primaryTextColor = resolvedTextColor;
  const secondaryTextColor = isDark ? '#FFFFFFA6' : '#000000A6';
  const mutedTextColor = isDark ? '#FFFFFF73' : '#00000073';

  return {
    common: {
      bodyColor,
      cardColor: surfaceColor,
      modalColor: surfaceColor,
      popoverColor: surfaceColor,
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
      // Tab 配置
      tabColor: isDark ? '#262626' : '#FFFFFF',
      tabColorSegment: isDark ? '#1F1F1F' : '#FFFFFF',
      tabBorderColor: isDark ? '#3C3C3C' : '#E0E0E0',
    },
    Layout: {
      color: surfaceColor,
      siderColor: surfaceColor,
      headerColor: surfaceColor,
      footerColor: surfaceColor,
      textColor: primaryTextColor,
    },
    Input: {
      color: isDark ? '#1C1C1C' : '#FFFFFF',
      colorFocus: isDark ? '#1C1C1C' : '#FFFFFF',
      textColor: primaryTextColor,
      placeholderColor: mutedTextColor,
      border: `1px solid ${inputBorderColor}`,
      borderHover: `1px solid ${inputBorderHoverColor}`,
      borderFocus: `1px solid ${primaryColor}`,
    },
    InputNumber: {
      color: isDark ? '#1C1C1C' : '#FFFFFF',
      colorFocus: isDark ? '#1C1C1C' : '#FFFFFF',
      textColor: primaryTextColor,
      border: `1px solid ${inputBorderColor}`,
      borderHover: `1px solid ${inputBorderHoverColor}`,
      borderFocus: `1px solid ${primaryColor}`,
    },
    Select: {
      peers: {
        InternalSelection: {
          color: isDark ? '#1C1C1C' : '#FFFFFF',
          colorActive: isDark ? '#1C1C1C' : '#FFFFFF',
          textColor: primaryTextColor,
          placeholderColor: mutedTextColor,
          border: `1px solid ${inputBorderColor}`,
          borderHover: `1px solid ${inputBorderHoverColor}`,
          borderActive: `1px solid ${primaryColor}`,
          borderFocus: `1px solid ${primaryColor}`,
        },
        InternalSelectMenu: {
          color: surfaceColor,
          optionTextColor: primaryTextColor,
          optionTextColorActive: primaryColor,
          optionColorActive: isDark ? '#2A2A2A' : '#F5F5F5',
          optionColorHover: isDark ? '#2A2A2A' : '#F5F5F5',
        },
      },
    },
    Tag: {
      textColor: primaryTextColor,
      color: isDark ? '#2A2A2A' : '#F5F5F5',
      colorBordered: isDark ? '#2A2A2A' : '#F5F5F5',
      border: isDark ? '1px solid #3C3C3C' : '1px solid #E0E0E0',
    },
    Button: {
      textColor: primaryTextColor,
      textColorText: primaryTextColor,
      textColorTextHover: primaryColor,
      textColorTextPressed: primaryPressed,
      textColorTextDisabled: mutedTextColor,
      colorPrimary: primaryColor,
      colorHoverPrimary: primaryHover,
      colorPressedPrimary: primaryPressed,
      borderPrimary: `1px solid ${primaryColor}`,
      borderHoverPrimary: `1px solid ${primaryHover}`,
      borderPressedPrimary: `1px solid ${primaryPressed}`,
    },
    Scrollbar: {
      width: '8px',
      height: '8px',
    },
  };
}
