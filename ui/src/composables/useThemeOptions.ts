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
 * 终端主题跟随应用主题的特殊值
 */
export const TERMINAL_THEME_FOLLOW = 'follow-theme';

/**
 * 获取终端配色选项，自动根据当前语言切换名称
 * @param includeFollowOption 是否包含"跟随主题"选项，默认 true
 * @returns 终端配色选项列表
 */
export function useTerminalThemeOptions(includeFollowOption = true): ComputedRef<ThemeOption[]> {
  const { locale } = useLocale();

  return computed(() => {
    const isZh = locale.value === 'zh-CN';
    const options: ThemeOption[] = [];

    // 添加"跟随主题"选项
    if (includeFollowOption) {
      options.push({
        label: isZh ? '跟随主题' : 'Follow Theme',
        value: TERMINAL_THEME_FOLLOW,
      });
    }

    // 添加所有终端主题预设
    options.push(
      ...TERMINAL_THEME_PRESETS.map(preset => ({
        label: isZh ? preset.name : preset.nameEn,
        value: preset.id,
      }))
    );

    return options;
  });
}
