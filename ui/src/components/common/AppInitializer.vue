<template></template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useLoadingBar } from 'naive-ui';
import { setupErrorHandler } from '@/utils/errorHandler';
import { useAppStore } from '@/stores/app';

const router = useRouter();
const loadingBar = useLoadingBar();
const teardownErrorHandler = setupErrorHandler();
const appStore = useAppStore();

onMounted(() => {
  appStore.fetchAppInfo();
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
