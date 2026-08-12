<script setup lang="ts">
import { computed, watch, onMounted, onBeforeUnmount } from 'vue';
import { RouterView, useRoute } from 'vue-router';
import { storeToRefs } from 'pinia';
import { zhCN, dateZhCN, enUS, dateEnUS, darkTheme, type GlobalThemeOverrides } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import AppInitializer from '@/components/common/AppInitializer.vue';
import NotePad from '@/components/notepad/NotePad.vue';
import { useAuthStore } from '@/stores/auth';
import { useSettingsStore } from '@/stores/settings';
import { useProjectStore } from '@/stores/project';
import { useResponsive } from '@/composables/useResponsive';
import { useAiStatusSummary } from '@/composables/useAiStatusSummary';
import { isDarkHex } from '@/utils/color';
import { formatBrowserTabTitle } from '@/utils/browserTitle';
import { createThemeOverrides } from '@/utils/themeOverrides';
import { createThemeSemanticPalette } from '@/utils/themeSemanticPalette';
import { getPresetById } from '@/constants/themes';
import { APP_NAME } from '@/constants/app';

const settingsStore = useSettingsStore();
const authStore = useAuthStore();
const projectStore = useProjectStore();
const {
  activeTheme: theme,
  followSystemTheme,
  currentPresetId,
  pageTitle,
} = storeToRefs(settingsStore);
const { totalSummary } = useAiStatusSummary();
const isDarkTheme = computed(() => isDarkHex(theme.value.bodyColor || '#ffffff'));
const { isMobile } = useResponsive();
const route = useRoute();
const currentRouteProjectId = computed(() =>
  typeof route.params.id === 'string' ? route.params.id : ''
);
const workspaceProjectName = computed(() => {
  if (route.name !== 'project' || !currentRouteProjectId.value) {
    return '';
  }

  if (projectStore.currentProject?.id === currentRouteProjectId.value) {
    return projectStore.currentProject.name?.trim() ?? '';
  }

  const matchedProject = projectStore.projects.find(
    project => project.id === currentRouteProjectId.value
  );
  return matchedProject?.name?.trim() ?? '';
});
const browserTabTitle = computed(() =>
  formatBrowserTabTitle({
    summary: totalSummary.value,
    appName: pageTitle.value || APP_NAME,
    projectName: workspaceProjectName.value,
  })
);
const canLoadProtectedContent = computed(() => authStore.canAccessProtectedContent);
const shouldRenderWorkspaceOverlays = computed(() => canLoadProtectedContent.value);
const shouldShowGlobalNotepad = computed(() => {
  if (isMobile.value) {
    return false;
  }
  return true;
});

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
  return isDarkTheme.value ? '#D9D9D9' : '#333333';
});

const semanticPalette = computed(() =>
  createThemeSemanticPalette(theme.value, resolvedTextColor.value)
);
const inputBorderColor = computed(() => semanticPalette.value.controlBorder);
const inputBorderHoverColor = computed(() => semanticPalette.value.controlBorderHover);

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
    inputBorderHoverColor.value
  );
});

// 使用 watch 直接设置全局 CSS 变量到 :root，确保所有组件都能访问
// （useCssVars 只设置在组件根元素上，无法被 :deep() 选择器访问）
// 解析终端头部边框值：支持 boolean | string
const resolvedTerminalHeaderBorder = computed(() => {
  const borderValue =
    theme.value.terminalHeaderBorder ?? presetTerminalTabColors.value.headerBorder;

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
  '--kanban-terminal-tab-bg':
    theme.value.terminalTabBg || presetTerminalTabColors.value.tabBg || theme.value.bodyColor,
  '--kanban-terminal-tab-active-bg':
    theme.value.terminalTabActiveBg ||
    presetTerminalTabColors.value.tabActiveBg ||
    theme.value.surfaceColor,
  '--kanban-terminal-header-border': resolvedTerminalHeaderBorder.value,
  // 空终端引导文字颜色
  '--kanban-terminal-empty-guide-fg': theme.value.terminalEmptyGuideFg || theme.value.terminalFg,
  // 看板颜色
  '--kanban-board-bg':
    theme.value.kanbanBoardBg ||
    presetTerminalTabColors.value.kanbanBoardBg ||
    theme.value.bodyColor,
  '--kanban-card-bg':
    theme.value.kanbanCardBg ||
    presetTerminalTabColors.value.kanbanCardBg ||
    theme.value.surfaceColor,
  '--kanban-border':
    (theme.value.kanbanBorderEnabled ?? presetTerminalTabColors.value.kanbanBorderEnabled ?? true)
      ? '1px solid var(--n-border-color)'
      : 'none',
  '--app-text-color': resolvedTextColor.value,
  '--app-input-border-color': inputBorderColor.value,
  '--app-input-border-hover-color': inputBorderHoverColor.value,
  '--app-canvas': semanticPalette.value.canvas,
  '--app-surface': semanticPalette.value.surface,
  '--app-surface-raised': semanticPalette.value.surfaceRaised,
  '--app-surface-sunken': semanticPalette.value.surfaceSunken,
  '--app-surface-hover': semanticPalette.value.surfaceHover,
  '--app-surface-active': semanticPalette.value.surfaceActive,
  '--app-text-primary': semanticPalette.value.textPrimary,
  '--app-text-secondary': semanticPalette.value.textSecondary,
  '--app-text-muted': semanticPalette.value.textMuted,
  '--app-text-inverse': semanticPalette.value.textInverse,
  '--app-border': semanticPalette.value.border,
  '--app-border-strong': semanticPalette.value.borderStrong,
  '--app-control-border': semanticPalette.value.controlBorder,
  '--app-control-border-hover': semanticPalette.value.controlBorderHover,
  '--app-focus-ring': semanticPalette.value.focusRing,
  '--app-accent': semanticPalette.value.accent,
  '--app-accent-hover': semanticPalette.value.accentHover,
  '--app-accent-pressed': semanticPalette.value.accentPressed,
  '--app-accent-soft': semanticPalette.value.accentSoft,
  '--app-accent-contrast': semanticPalette.value.accentContrast,
  '--app-link': semanticPalette.value.link,
  '--app-link-hover': semanticPalette.value.linkHover,
  '--app-success': semanticPalette.value.success,
  '--app-success-soft': semanticPalette.value.successSoft,
  '--app-warning': semanticPalette.value.warning,
  '--app-warning-soft': semanticPalette.value.warningSoft,
  '--app-error': semanticPalette.value.error,
  '--app-error-soft': semanticPalette.value.errorSoft,
  '--app-info': semanticPalette.value.info,
  '--app-info-soft': semanticPalette.value.infoSoft,
  '--app-overlay': semanticPalette.value.overlay,
  '--app-shadow': semanticPalette.value.shadow,
  '--n-primary-color': semanticPalette.value.accent,
  '--n-color-primary': semanticPalette.value.accent,
  '--n-primary-color-hover': semanticPalette.value.accentHover,
  '--n-primary-color-pressed': semanticPalette.value.accentPressed,
  '--n-primary-color-suppl': semanticPalette.value.accentHover,
  '--n-text-color-base': semanticPalette.value.textPrimary,
  '--n-text-color': semanticPalette.value.textPrimary,
  '--n-text-color-1': semanticPalette.value.textPrimary,
  '--n-text-color-2': semanticPalette.value.textSecondary,
  '--n-text-color-3': semanticPalette.value.textMuted,
  '--n-text-color-disabled': semanticPalette.value.textMuted,
  '--n-body-color': semanticPalette.value.canvas,
  '--n-color': semanticPalette.value.surface,
  '--n-color-hover': semanticPalette.value.surfaceHover,
  '--n-color-target': semanticPalette.value.accentSoft,
  '--n-color-embedded': semanticPalette.value.surfaceSunken,
  '--n-card-color': semanticPalette.value.surface,
  '--n-modal-color': semanticPalette.value.surfaceRaised,
  '--n-popover-color': semanticPalette.value.surfaceRaised,
  '--n-border-color': semanticPalette.value.border,
  '--n-divider-color': semanticPalette.value.border,
  '--n-box-shadow-color': semanticPalette.value.shadow,
  '--n-error-color': semanticPalette.value.error,
  '--n-warning-color': semanticPalette.value.warning,
  '--n-success-color': semanticPalette.value.success,
  '--n-info-color': semanticPalette.value.info,
}));

watch(
  cssVarsToSet,
  vars => {
    if (typeof document !== 'undefined') {
      const root = document.documentElement;
      root.dataset.colorScheme = semanticPalette.value.colorScheme;
      root.style.colorScheme = semanticPalette.value.colorScheme;
      root.style.backgroundColor = semanticPalette.value.canvas;
      Object.entries(vars).forEach(([key, value]) => {
        root.style.setProperty(key, value ?? '');
      });
    }
  },
  { immediate: true, deep: true }
);

// 只更新 body 背景色（CSS变量已由 useCssVars 处理）
watch(
  () => theme.value.bodyColor,
  newColor => {
    if (typeof document !== 'undefined') {
      document.body.style.backgroundColor = newColor;
    }
  },
  { immediate: true }
);

// 监听系统主题变化
let mediaQuery: MediaQueryList | null = null;
let handleChange: (() => void) | null = null;

onMounted(() => {
  if (typeof window === 'undefined') {
    return;
  }

  if (canLoadProtectedContent.value) {
  }
  if (canLoadProtectedContent.value && projectStore.projects.length === 0) {
    void projectStore.fetchProjects({ silent: true }).catch(error => {
      console.error('[App] Failed to preload projects for browser title status', error);
    });
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

watch(canLoadProtectedContent, value => {
  if (!value) {
    return;
  }
  if (projectStore.projects.length > 0) {
    return;
  }
  void projectStore.fetchProjects({ silent: true }).catch(error => {
    console.error('[App] Failed to preload projects after authentication', error);
  });
});

onBeforeUnmount(() => {
  if (mediaQuery && handleChange) {
    mediaQuery.removeEventListener('change', handleChange);
  }
});

watch(
  browserTabTitle,
  title => {
    if (typeof document !== 'undefined') {
      document.title = title;
    }
  },
  { immediate: true }
);
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
        <n-notification-provider
          :scrollable="false"
          :max="3"
          container-class="global-notification-host"
        >
          <n-message-provider>
            <n-modal-provider>
              <AppInitializer />
              <RouterView />
              <NotePad v-if="shouldRenderWorkspaceOverlays && shouldShowGlobalNotepad" />
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
  transition:
    background-color 0.2s ease,
    color 0.2s ease,
    border-color 0.2s ease;
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

.global-notification-host {
  pointer-events: none;
}

.global-notification-host .n-notification-wrapper,
.global-notification-host .n-notification,
.global-notification-host .n-notification__close {
  pointer-events: auto;
}
</style>
