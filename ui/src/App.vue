<script setup lang="ts">
import { computed, useCssVars, watchEffect } from 'vue';
import { RouterView } from 'vue-router';
import { storeToRefs } from 'pinia';
import { zhCN, dateZhCN, enUS, dateEnUS, type GlobalThemeOverrides } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import AppInitializer from '@/components/common/AppInitializer.vue';
import NotePad from '@/components/notepad/NotePad.vue';
import AICompletionNotifier from '@/components/terminal/AICompletionNotifier.vue';
import AIApprovalNotifier from '@/components/terminal/AIApprovalNotifier.vue';
import { useSettingsStore } from '@/stores/settings';
import { darkenColor, lightenColor } from '@/utils/color';

const settingsStore = useSettingsStore();
const { theme } = storeToRefs(settingsStore);

const { locale } = useI18n();

// 根据当前语言动态切换 Naive UI 的 locale
const naiveLocale = computed(() => (locale.value === 'zh-CN' ? zhCN : enUS));
const naiveDateLocale = computed(() => (locale.value === 'zh-CN' ? dateZhCN : dateEnUS));

const themeOverrides = computed<GlobalThemeOverrides>(() => {
  const { primaryColor, bodyColor, surfaceColor } = theme.value;
  const primaryHover = lightenColor(primaryColor, 0.08);
  const primaryPressed = darkenColor(primaryColor, 0.12);

  return {
    common: {
      bodyColor,
      cardColor: surfaceColor,
      modalColor: surfaceColor,
      popoverColor: surfaceColor,
      primaryColor,
      primaryColorHover: primaryHover,
      primaryColorPressed: primaryPressed,
      primaryColorSuppl: primaryHover,
    },
    Layout: {
      color: surfaceColor,
      siderColor: surfaceColor,
      headerColor: surfaceColor,
      footerColor: surfaceColor,
    },
    Scrollbar: {
      width: '8px',
      height: '8px',
    },
  };
});

useCssVars(() => ({
  'app-body-color': theme.value.bodyColor,
  'app-surface-color': theme.value.surfaceColor,
}));

watchEffect(() => {
  if (typeof document === 'undefined') {
    return;
  }
  document.body.style.backgroundColor = theme.value.bodyColor;
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
