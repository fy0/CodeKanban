<template></template>

<script setup lang="ts">
import { onBeforeUnmount } from 'vue';
import { useRouter } from 'vue-router';
import { useLoadingBar } from 'naive-ui';
import { setupErrorHandler } from '@/utils/errorHandler';
import { useAppStore } from '@/stores/app';
import Apis from '@/api';
import { useReq, useInit } from '@/api';

const router = useRouter();
const loadingBar = useLoadingBar();
const teardownErrorHandler = setupErrorHandler();
const appStore = useAppStore();

const { send: fetchAppInfo } = useReq(() => Apis.system.version({}));

useInit(async () => {
  try {
    const info = await fetchAppInfo();
    if (info) {
      appStore.setAppInfo(info);
    }
  } catch (error) {
    console.error('Failed to fetch app info:', error);
  }
});

const removeBeforeEach = router.beforeEach((to, from, next) => {
  loadingBar?.start();
  next();
});
const removeAfterEach = router.afterEach(() => {
  loadingBar?.finish();
});
const removeOnError = router.onError(() => {
  loadingBar?.error();
});

onBeforeUnmount(() => {
  teardownErrorHandler?.();
  removeBeforeEach();
  removeAfterEach();
  removeOnError();
});
</script>
