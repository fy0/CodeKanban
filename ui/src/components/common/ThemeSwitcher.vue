<template>
  <n-dropdown :options="themeOptions" :trigger="resolvedTrigger" @select="handleSelect">
    <n-button quaternary circle size="small" @click="handleButtonClick">
      <template #icon>
        <n-icon size="18">
          <component :is="isDarkTheme ? MoonOutline : SunnyOutline" />
        </n-icon>
      </template>
    </n-button>
  </n-dropdown>
</template>

<script setup lang="ts">
import { computed, h } from 'vue';
import { storeToRefs } from 'pinia';
import { NIcon, useDialog } from 'naive-ui';
import { MoonOutline, SunnyOutline, CheckmarkOutline } from '@vicons/ionicons5';
import { useSettingsStore } from '@/stores/settings';
import { THEME_PRESETS, isSupportedThemePreset } from '@/constants/themes';
import { useLocale } from '@/composables/useLocale';
import { useThemeOptions } from '@/composables/useThemeOptions';
import { isDarkHex } from '@/utils/color';
import {
  createThemeMaintenanceWarningController,
  createThemeSelectionController,
} from '@/utils/themeMaintenanceWarning';
import type { DropdownOption } from 'naive-ui';

const props = withDefaults(
  defineProps<{
    quickToggleOnClick?: boolean;
    trigger?: 'hover' | 'click';
  }>(),
  {
    quickToggleOnClick: true,
    trigger: undefined,
  }
);

const { t } = useLocale();
const dialog = useDialog();
const settingsStore = useSettingsStore();
const { activeTheme, currentPresetId, followSystemTheme } = storeToRefs(settingsStore);
const themeWarningController = createThemeMaintenanceWarningController({
  t,
  warning: options => dialog.warning(options),
});
const themeSelectionController = createThemeSelectionController({
  getCurrentPresetId: () => currentPresetId.value,
  isFollowSystemTheme: () => followSystemTheme.value,
  selectPreset: presetId => settingsStore.selectPreset(presetId),
  toggleFollowSystemTheme: enabled => settingsStore.toggleFollowSystemTheme(enabled),
  confirmPresetThemeChange: themeWarningController.confirmPresetThemeChange,
  confirmFollowSystemEnable: themeWarningController.confirmFollowSystemEnable,
  shouldConfirmPresetThemeChange: presetId => !isSupportedThemePreset(presetId),
  shouldConfirmFollowSystemEnable: () => false,
});

const isDarkTheme = computed(() => isDarkHex(activeTheme.value.bodyColor));

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
const resolvedTrigger = computed(() => props.trigger ?? (props.quickToggleOnClick ? 'hover' : 'click'));

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

const handleSelect = async (key: string) => {
  if (key === 'follow-system') {
    await themeSelectionController.toggleFollowSystemThemeWithConfirmation(
      !followSystemTheme.value
    );
  } else {
    await themeSelectionController.selectPresetWithConfirmation(key);
  }
};

// 快速切换 dark/light 主题
const handleQuickToggle = async () => {
  await themeSelectionController.quickToggleLightDark();
};

const handleButtonClick = async () => {
  if (!props.quickToggleOnClick) {
    return;
  }
  await handleQuickToggle();
};
</script>
