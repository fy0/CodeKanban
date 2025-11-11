<script setup lang="ts">
import { computed } from 'vue';
import { RouterView } from 'vue-router';
import { zhCN, dateZhCN, type GlobalThemeOverrides } from 'naive-ui';
import AppInitializer from '@/components/common/AppInitializer.vue';
import NotePad from '@/components/notepad/NotePad.vue';

const themeOverrides = computed<GlobalThemeOverrides>(() => ({
  common: {
    bodyColor: '#ffffff',
  },
  Layout: {
    color: '#ffffff',
    siderColor: '#ffffff',
  },
  Scrollbar: {
    width: '8px',
    height: '8px',
  },
}));
</script>

<template>
  <n-config-provider :locale="zhCN" :date-locale="dateZhCN" :theme-overrides="themeOverrides">
    <n-loading-bar-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <n-message-provider>
            <n-modal-provider>
              <AppInitializer />
              <RouterView />
              <NotePad />
            </n-modal-provider>
          </n-message-provider>
        </n-notification-provider>
      </n-dialog-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>
