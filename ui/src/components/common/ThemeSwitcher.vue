<template>
  <n-dropdown :options="themeOptions" @select="handleSelect">
    <n-button quaternary circle>
      <template #icon>
        <n-icon>
          <component :is="isDarkTheme ? MoonOutline : SunnyOutline" />
        </n-icon>
      </template>
    </n-button>
  </n-dropdown>
</template>

<script setup lang="ts">
import { computed, h } from 'vue';
import { storeToRefs } from 'pinia';
import { NIcon } from 'naive-ui';
import { MoonOutline, SunnyOutline, CheckmarkOutline } from '@vicons/ionicons5';
import { useSettingsStore } from '@/stores/settings';
import { THEME_PRESETS, getPresetById } from '@/constants/themes';
import { useLocale } from '@/composables/useLocale';
import { useThemeOptions } from '@/composables/useThemeOptions';
import type { DropdownOption } from 'naive-ui';

const { t } = useLocale();
const settingsStore = useSettingsStore();
const { currentPresetId, followSystemTheme } = storeToRefs(settingsStore);

// 判断当前是否为暗色主题
const isDarkTheme = computed(() => {
  const preset = getPresetById(currentPresetId.value);
  return preset?.isDark ?? false;
});

// 渲染颜色圆点
const renderColorDot = (color: string) =>
  h('div', {
    style: {
      width: '12px',
      height: '12px',
      borderRadius: '50%',
      backgroundColor: color,
      border: '1px solid rgba(0, 0, 0, 0.1)',
    },
  });

// 渲染选中图标
const renderCheckIcon = () => h(NIcon, null, { default: () => h(CheckmarkOutline) });

// 获取主题选项（使用 composable）
const baseThemeOptions = useThemeOptions();

// 下拉菜单选项
const themeOptions = computed<DropdownOption[]>(() => {
  const presetOptions: DropdownOption[] = THEME_PRESETS.map((preset, index) => ({
    label: baseThemeOptions.value[index].label,
    key: preset.id,
    icon:
      currentPresetId.value === preset.id && !followSystemTheme.value
        ? renderCheckIcon
        : () => renderColorDot(preset.colors.primaryColor),
  }));

  return [
    {
      type: 'group',
      label: t('theme.presetTheme'),
      key: 'presets',
      children: presetOptions,
    },
    {
      type: 'divider',
      key: 'divider',
    },
    {
      label: t('theme.followSystem'),
      key: 'follow-system',
      icon: followSystemTheme.value ? renderCheckIcon : undefined,
    },
  ];
});

const handleSelect = (key: string) => {
  if (key === 'follow-system') {
    settingsStore.toggleFollowSystemTheme(!followSystemTheme.value);
  } else {
    settingsStore.selectPreset(key);
  }
};
</script>
