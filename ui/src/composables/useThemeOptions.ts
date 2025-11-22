import { computed } from 'vue';
import type { ComputedRef } from 'vue';
import { THEME_PRESETS } from '@/constants/themes';
import { TERMINAL_THEME_PRESETS } from '@/constants/terminalThemes';
import { useLocale } from './useLocale';

export interface ThemeOption {
  label: string;
  value: string;
}

/**
 * 获取主题预设选项，自动根据当前语言切换名称
 * @returns 主题预设选项列表
 */
export function useThemeOptions(): ComputedRef<ThemeOption[]> {
  const { locale } = useLocale();

  return computed(() => {
    const isZh = locale.value === 'zh-CN';
    return THEME_PRESETS.map(preset => ({
      label: isZh ? preset.name : preset.nameEn,
      value: preset.id,
    }));
  });
}

/**
 * 获取终端配色选项，自动根据当前语言切换名称
 * @returns 终端配色选项列表
 */
export function useTerminalThemeOptions(): ComputedRef<ThemeOption[]> {
  const { locale } = useLocale();

  return computed(() => {
    const isZh = locale.value === 'zh-CN';
    return TERMINAL_THEME_PRESETS.map(preset => ({
      label: isZh ? preset.name : preset.nameEn,
      value: preset.id,
    }));
  });
}
