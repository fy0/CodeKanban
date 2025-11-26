<script setup lang="ts">
import { computed, watch, onMounted, onBeforeUnmount } from 'vue';
import { RouterView } from 'vue-router';
import { storeToRefs } from 'pinia';
import { zhCN, dateZhCN, enUS, dateEnUS, darkTheme, type GlobalThemeOverrides } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import AppInitializer from '@/components/common/AppInitializer.vue';
import NotePad from '@/components/notepad/NotePad.vue';
import AINotificationBar from '@/components/terminal/AINotificationBar.vue';
import { useSettingsStore } from '@/stores/settings';
import { darkenColor, lightenColor, isDarkHex } from '@/utils/color';
import { createThemeOverrides } from '@/utils/themeOverrides';
import { getPresetById } from '@/constants/themes';

const settingsStore = useSettingsStore();
const { activeTheme: theme, followSystemTheme, currentPresetId } = storeToRefs(settingsStore);
const isDarkTheme = computed(() => isDarkHex(theme.value.bodyColor || '#ffffff'));

// 获取预设主题中的终端标签颜色（用于 fallback）
// 当 followSystemTheme 为 true 时，根据系统主题选择预设
const presetTerminalTabColors = computed(() => {
  let presetId = currentPresetId.value;
  if (followSystemTheme.value && typeof window !== 'undefined') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    presetId = prefersDark ? 'dark' : 'light';
  }
  const preset = getPresetById(presetId);
  return {
    tabBg: preset?.colors.terminalTabBg,
    tabActiveBg: preset?.colors.terminalTabActiveBg,
    headerBorder: preset?.colors.terminalHeaderBorder,
    completionBg: preset?.colors.terminalTabCompletionBg,
    completionBorder: preset?.colors.terminalTabCompletionBorder,
    approvalBg: preset?.colors.terminalTabApprovalBg,
    approvalBorder: preset?.colors.terminalTabApprovalBorder,
    kanbanBoardBg: preset?.colors.kanbanBoardBg,
    kanbanCardBg: preset?.colors.kanbanCardBg,
    kanbanBorderEnabled: preset?.colors.kanbanBorderEnabled,
  };
});

const { locale } = useI18n();

const resolvedTextColor = computed(() => {
  const { textColor } = theme.value;
  if (textColor && textColor.trim().length > 0) {
    return textColor;
  }
  return isDarkTheme.value ? '#FFFFFFD9' : '#000000E0';
});

const inputBorderColor = computed(() => (isDarkTheme.value ? '#4B4B4B' : '#D0D5DD'));
const inputBorderHoverColor = computed(() =>
  isDarkTheme.value ? lightenColor(inputBorderColor.value, 0.12) : darkenColor(inputBorderColor.value, 0.12),
);

// 根据当前语言动态切换 Naive UI 的 locale
const naiveLocale = computed(() => (locale.value === 'zh-CN' ? zhCN : enUS));
const naiveDateLocale = computed(() => (locale.value === 'zh-CN' ? dateZhCN : dateEnUS));

// 根据主题配置动态切换 Naive UI 的 theme (亮色/暗色)
const naiveTheme = computed(() => (isDarkTheme.value ? darkTheme : null));

// 使用提取的主题配置函数，简化 App.vue 代码
const themeOverrides = computed<GlobalThemeOverrides>(() => {
  return createThemeOverrides(
    theme.value,
    resolvedTextColor.value,
    inputBorderColor.value,
    inputBorderHoverColor.value,
  );
});

// 使用 watch 直接设置全局 CSS 变量到 :root，确保所有组件都能访问
// （useCssVars 只设置在组件根元素上，无法被 :deep() 选择器访问）
// 默认的完成/审批颜色（暗色主题）
const defaultCompletionBg = 'rgba(16, 185, 129, 0.1)';
const defaultCompletionBorder = 'rgba(16, 185, 129, 0.3)';
const defaultApprovalBg = 'rgba(247, 144, 9, 0.12)';
const defaultApprovalBorder = 'rgba(247, 144, 9, 0.35)';

// 解析终端头部边框值：支持 boolean | string
const resolvedTerminalHeaderBorder = computed(() => {
  const borderValue = theme.value.terminalHeaderBorder ?? presetTerminalTabColors.value.headerBorder;

  if (borderValue === false) {
    return 'none';
  } else if (borderValue === true) {
    return '1px solid rgba(255, 255, 255, 0.09)';
  } else if (typeof borderValue === 'string') {
    // 处理 'transparent' 字符串，将其转换为完整的边框声明或 none
    if (borderValue === 'transparent') {
      return '1px solid transparent';
    }
    return borderValue;
  }

  // 默认值
  return '1px solid rgba(255, 255, 255, 0.09)';
});

const cssVarsToSet = computed(() => ({
  '--app-body-color': theme.value.bodyColor,
  '--app-surface-color': theme.value.surfaceColor,
  '--kanban-terminal-bg': theme.value.terminalBg,
  '--kanban-terminal-fg': theme.value.terminalFg,
  '--kanban-terminal-tab-bg': theme.value.terminalTabBg || presetTerminalTabColors.value.tabBg || theme.value.bodyColor,
  '--kanban-terminal-tab-active-bg': theme.value.terminalTabActiveBg || presetTerminalTabColors.value.tabActiveBg || theme.value.surfaceColor,
  '--kanban-terminal-header-border': resolvedTerminalHeaderBorder.value,
  // 完成提醒颜色
  '--kanban-terminal-tab-completion-bg': theme.value.terminalTabCompletionBg || presetTerminalTabColors.value.completionBg || defaultCompletionBg,
  '--kanban-terminal-tab-completion-border': theme.value.terminalTabCompletionBorder || presetTerminalTabColors.value.completionBorder || defaultCompletionBorder,
  // 审批提醒颜色
  '--kanban-terminal-tab-approval-bg': theme.value.terminalTabApprovalBg || presetTerminalTabColors.value.approvalBg || defaultApprovalBg,
  '--kanban-terminal-tab-approval-border': theme.value.terminalTabApprovalBorder || presetTerminalTabColors.value.approvalBorder || defaultApprovalBorder,
  // 浮动按钮颜色
  '--kanban-terminal-floating-button-bg': theme.value.terminalFloatingButtonBg || theme.value.surfaceColor,
  '--kanban-terminal-floating-button-fg': theme.value.terminalFloatingButtonFg || theme.value.textColor,
  // 空终端引导文字颜色
  '--kanban-terminal-empty-guide-fg': theme.value.terminalEmptyGuideFg || theme.value.terminalFg,
  // AI 通知按钮颜色
  '--kanban-notification-button-border': theme.value.notificationButtonBorder || 'rgba(0, 0, 0, 0.2)',
  '--kanban-notification-button-fg': theme.value.notificationButtonFg || theme.value.textColor,
  // 看板颜色
  '--kanban-board-bg': theme.value.kanbanBoardBg || presetTerminalTabColors.value.kanbanBoardBg || theme.value.bodyColor,
  '--kanban-card-bg': theme.value.kanbanCardBg || presetTerminalTabColors.value.kanbanCardBg || theme.value.surfaceColor,
  '--kanban-border': (theme.value.kanbanBorderEnabled ?? presetTerminalTabColors.value.kanbanBorderEnabled ?? true) ? '1px solid var(--n-border-color)' : 'none',
  '--app-text-color': resolvedTextColor.value,
  '--app-input-border-color': inputBorderColor.value,
  '--app-input-border-hover-color': inputBorderHoverColor.value,
}));

watch(
  cssVarsToSet,
  (vars) => {
    if (typeof document !== 'undefined') {
      const root = document.documentElement;
      Object.entries(vars).forEach(([key, value]) => {
        root.style.setProperty(key, value ?? '');
      });
    }
  },
  { immediate: true, deep: true },
);

// 只更新 body 背景色（CSS变量已由 useCssVars 处理）
watch(
  () => theme.value.bodyColor,
  (newColor) => {
    if (typeof document !== 'undefined') {
      document.body.style.backgroundColor = newColor;
    }
  },
  { immediate: true },
);

// 监听系统主题变化
let mediaQuery: MediaQueryList | null = null;
let handleChange: (() => void) | null = null;

onMounted(() => {
  if (typeof window === 'undefined') {
    return;
  }

  mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
  handleChange = () => {
    if (followSystemTheme.value) {
      // 系统主题变化时，重新应用主题（使用专用方法，不关闭 followSystemTheme）
      const prefersDark = mediaQuery!.matches;
      const autoPresetId = prefersDark ? 'dark' : 'light';
      settingsStore.applySystemThemePreset(autoPresetId);
    }
  };

  mediaQuery.addEventListener('change', handleChange);
});

onBeforeUnmount(() => {
  if (mediaQuery && handleChange) {
    mediaQuery.removeEventListener('change', handleChange);
  }
});
</script>

<template>
  <n-config-provider
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
    :theme="naiveTheme"
    :theme-overrides="themeOverrides"
  >
    <n-global-style />
    <n-loading-bar-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <n-message-provider>
            <n-modal-provider>
              <AppInitializer />
              <RouterView />
              <NotePad />
              <AINotificationBar />
            </n-modal-provider>
          </n-message-provider>
        </n-notification-provider>
      </n-dialog-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>

<style>
.n-layout-toggle-button {
  --n-toggle-button-color: var(--app-surface-color, var(--n-card-color, #ffffff));
  --n-toggle-button-border: 1px solid var(--n-border-color, rgba(255, 255, 255, 0.2));
  --n-toggle-button-icon-color: var(--app-text-color, var(--n-text-color-1, #1f1f1f));
  background-color: var(--app-surface-color, var(--n-card-color, #ffffff));
  color: var(--app-text-color, var(--n-text-color-1, #1f1f1f));
  border-color: var(--n-border-color, transparent);
  box-shadow: 0 2px 8px var(--n-box-shadow-color, rgba(0, 0, 0, 0.12));
  transition: background-color 0.2s ease, color 0.2s ease, border-color 0.2s ease;
}

.n-layout-toggle-button:hover,
.n-layout-toggle-button:focus-visible {
  background-color: var(--app-body-color, var(--n-color-hover, #f5f5f5));
  color: var(--n-primary-color, #3b69a9);
  border-color: var(--n-primary-color, #3b69a9);
}

.n-layout-toggle-button .n-base-icon {
  color: var(--n-toggle-button-icon-color, currentColor);
}

.n-layout-sider .n-layout-toggle-button {
  background-color: var(--app-surface-color, var(--n-card-color, #ffffff));
  border-color: var(--n-border-color, transparent);
  color: var(--n-text-color-1, #1f1f1f);
}

.n-input,
.n-input__input-el,
.n-input__textarea-el,
.n-input__input,
.n-input__textarea {
  color: var(--app-text-color, var(--n-text-color-1, #1f1f1f)) !important;
}

.n-input .n-input__input-el::placeholder,
.n-input .n-input__textarea-el::placeholder {
  color: var(--n-text-color-3, #8c8c8c);
}
</style>
