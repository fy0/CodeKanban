<template></template>

<script setup lang="ts">
import { onBeforeUnmount } from 'vue';
import { useRouter } from 'vue-router';
import { useLoadingBar } from 'naive-ui';
import { setupErrorHandler } from '@/utils/errorHandler';

const router = useRouter();
const loadingBar = useLoadingBar();
const teardownErrorHandler = setupErrorHandler();

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
