<script setup lang="ts">
import { computed, useCssVars, watch, onMounted, onBeforeUnmount } from 'vue';
import { RouterView } from 'vue-router';
import { storeToRefs } from 'pinia';
import { zhCN, dateZhCN, enUS, dateEnUS, type GlobalThemeOverrides } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import AppInitializer from '@/components/common/AppInitializer.vue';
import NotePad from '@/components/notepad/NotePad.vue';
import AICompletionNotifier from '@/components/terminal/AICompletionNotifier.vue';
import AIApprovalNotifier from '@/components/terminal/AIApprovalNotifier.vue';
import { useSettingsStore } from '@/stores/settings';
import { darkenColor, lightenColor, isDarkHex } from '@/utils/color';
import { createThemeOverrides } from '@/utils/themeOverrides';

const settingsStore = useSettingsStore();
const { activeTheme: theme, followSystemTheme } = storeToRefs(settingsStore);
const isDarkTheme = computed(() => isDarkHex(theme.value.bodyColor || '#ffffff'));

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

// 使用提取的主题配置函数，简化 App.vue 代码
const themeOverrides = computed<GlobalThemeOverrides>(() => {
  return createThemeOverrides(
    theme.value,
    resolvedTextColor.value,
    inputBorderColor.value,
    inputBorderHoverColor.value,
  );
});

useCssVars(() => ({
  'app-body-color': theme.value.bodyColor,
  'app-surface-color': theme.value.surfaceColor,
  'kanban-terminal-bg': theme.value.terminalBg,
  'kanban-terminal-fg': theme.value.terminalFg,
  'app-text-color': resolvedTextColor.value,
  'app-input-border-color': inputBorderColor.value,
  'app-input-border-hover-color': inputBorderHoverColor.value,
}));

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
      // 系统主题变化时，重新应用主题
      const prefersDark = mediaQuery!.matches;
      const autoPresetId = prefersDark ? 'dark' : 'light';
      settingsStore.selectPreset(autoPresetId);
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
  <n-config-provider :locale="naiveLocale" :date-locale="naiveDateLocale" :theme-overrides="themeOverrides">
    <n-loading-bar-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <n-message-provider>
            <n-modal-provider>
              <AppInitializer />
              <RouterView />
              <NotePad />
              <AICompletionNotifier />
              <AIApprovalNotifier />
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
