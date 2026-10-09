<template>
  <n-dropdown scrollable :options="themeOptions" :trigger="resolvedTrigger" @select="handleSelect">
    <n-button quaternary circle size="small" aria-label="Theme / 外观" @click="handleButtonClick">
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
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { NIcon, useDialog } from 'naive-ui';
import { MoonOutline, SunnyOutline, CheckmarkOutline } from '@vicons/ionicons5';
import { useSettingsStore } from '@/stores/settings';
import { useAppearanceStore } from '@/stores/appearance';
import { isSupportedThemePreset } from '@/constants/themes';
import { useLocale } from '@/composables/useLocale';
import { useUnifiedThemeChoices } from '@/composables/useUnifiedThemeChoices';
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

const { t, locale } = useLocale();
const appearance = useAppearanceStore();
const router = useRouter();
const dialog = useDialog();
const settingsStore = useSettingsStore();
const { activeTheme, currentPresetId, followSystemTheme } = storeToRefs(settingsStore);
const themeWarningController = createThemeMaintenanceWarningController({
  t,
  warning: options => dialog.warning(options),
});
const themeSelectionController = createThemeSelectionController({
  getCurrentPresetId: () =>
    appearance.isClassic && !settingsStore.customTheme ? currentPresetId.value : '',
  isFollowSystemTheme: () => followSystemTheme.value,
  selectPreset: presetId => settingsStore.selectPreset(presetId),
  toggleFollowSystemTheme: enabled => settingsStore.toggleFollowSystemTheme(enabled),
  confirmPresetThemeChange: themeWarningController.confirmPresetThemeChange,
  confirmFollowSystemEnable: themeWarningController.confirmFollowSystemEnable,
  shouldConfirmPresetThemeChange: presetId => !isSupportedThemePreset(presetId),
  shouldConfirmFollowSystemEnable: () => false,
});

const isDarkTheme = computed(() =>
  appearance.isClassic ? isDarkHex(activeTheme.value.bodyColor) : appearance.scheme === 'dark'
);

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

const { choices, selectedKey, select } = useUnifiedThemeChoices();
const resolvedTrigger = computed(
  () => props.trigger ?? (props.quickToggleOnClick ? 'hover' : 'click')
);

// Every choice lives in the same theme list, regardless of its renderer or palette source.
const themeOptions = computed<DropdownOption[]>(() => {
  return [
    {
      type: 'group',
      key: 'themes',
      label: locale.value === 'zh-CN' ? '主题' : 'Themes',
      children: choices.value.map(choice => ({
        label: choice.label,
        key: choice.key,
        icon:
          selectedKey.value === choice.key
            ? renderCheckIcon
            : () => renderColorDot(choice.preview.accent),
      })),
    },
    {
      type: 'divider',
      key: 'divider',
    },
    {
      label: t('theme.followSystem'),
      key: 'follow-system',
      icon: (appearance.isClassic ? followSystemTheme.value : appearance.mode === 'system')
        ? renderCheckIcon
        : undefined,
    },
    {
      key: 'appearance-settings',
      label: locale.value === 'zh-CN' ? '界面设置…' : 'Appearance settings…',
    },
  ];
});

const handleSelect = async (key: string) => {
  if (key.startsWith('theme:')) {
    select(key);
    return;
  }
  if (key.startsWith('preset:')) {
    if (selectedKey.value !== key) {
      await themeSelectionController.selectPresetWithConfirmation(key.slice('preset:'.length));
    }
    return;
  }
  if (key === 'appearance-settings') {
    await router.push({ name: 'settings', query: { section: 'theme' } });
    return;
  }
  if (key === 'follow-system' && !appearance.isClassic) {
    appearance.setMode(appearance.mode === 'system' ? appearance.scheme : 'system');
    return;
  }
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
  if (!appearance.isClassic) {
    appearance.setMode(appearance.scheme === 'dark' ? 'light' : 'dark');
    return;
  }
  await themeSelectionController.quickToggleLightDark();
};

const handleButtonClick = async () => {
  if (!props.quickToggleOnClick) {
    return;
  }
  await handleQuickToggle();
};
</script>
