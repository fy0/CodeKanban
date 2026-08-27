<template>
  <n-drawer
    :show="settingsUiStore.isOpen"
    :placement="drawerPlacement"
    :width="drawerPlacement === 'right' ? drawerWidth : undefined"
    :height="drawerPlacement === 'bottom' ? drawerHeight : undefined"
    @update:show="handleUpdateShow"
  >
    <n-drawer-content
      :title="t('settings.title')"
      closable
      :native-scrollbar="false"
      body-content-style="padding: 0;"
    >
      <GeneralSettings embedded />
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue';
import { useLocale } from '@/composables/useLocale';
import { useResponsive } from '@/composables/useResponsive';
import { useSettingsUiStore } from '@/stores/settingsUi';

const GeneralSettings = defineAsyncComponent(() => import('@/views/GeneralSettings.vue'));

const settingsUiStore = useSettingsUiStore();
const { t } = useLocale();
const { isMobile, windowWidth } = useResponsive();

const drawerPlacement = computed(() => (isMobile.value ? 'bottom' : 'right'));
const drawerWidth = computed(() => {
  const target = Math.round(windowWidth.value * 0.7);
  return Math.max(650, Math.min(900, target));
});
const drawerHeight = computed(() => '100vh');

function handleUpdateShow(value: boolean) {
  if (!value) {
    settingsUiStore.closeSettings();
  }
}
</script>
