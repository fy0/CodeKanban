<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useLoadingBar } from 'naive-ui';
import { setupErrorHandler } from '@/utils/errorHandler';
import { useAppStore } from '@/stores/app';
import { useAuthStore } from '@/stores/auth';
import { useSettingsStore } from '@/stores/settings';
import Apis from '@/api';
import { useReq, useInit } from '@/api';
import { isWebSessionOnlyRouteChange } from '@/utils/webSessionRoute';

const router = useRouter();
const loadingBar = useLoadingBar();
const teardownErrorHandler = setupErrorHandler();
const appStore = useAppStore();
const authStore = useAuthStore();
const settingsStore = useSettingsStore();

const { send: fetchAppInfo } = useReq(() => Apis.system.version({}));

const canLoadAppInfo = computed(() => authStore.canAccessProtectedContent);
let appInfoLoaded = false;
let routeLoadingActive = false;

const handleUnauthorized = () => {
  authStore.markUnauthorized();
  const current = router.currentRoute.value;
  if (authStore.enabled && current.name !== 'login') {
    void router.push({
      name: 'login',
      query: {
        redirect: current.fullPath || '/',
      },
    });
  }
};

async function ensureAppInfoLoaded() {
  if (!canLoadAppInfo.value || appInfoLoaded) {
    return;
  }

  try {
    const info = await fetchAppInfo();
    if (info) {
      appStore.setAppInfo(info);
      appInfoLoaded = true;
    }
  } catch (error) {
    console.error('Failed to fetch app info:', error);
  }
}

async function ensureSettingsLoaded() {
  if (!canLoadAppInfo.value) {
    return;
  }

  await Promise.all([
    settingsStore.loadWebSessionQuickInput(),
    settingsStore.loadDailyTipSettings(),
  ]);
}

async function ensurePageTitleLoaded() {
  await settingsStore.loadPageTitleSettings();
}

useInit(async () => {
  try {
    await Promise.all([authStore.ensureLoaded(), ensurePageTitleLoaded()]);
    await Promise.all([ensureAppInfoLoaded(), ensureSettingsLoaded()]);
  } catch (error) {
    console.error('Failed to initialize auth status:', error);
  }
});

watch(
  canLoadAppInfo,
  value => {
    if (value) {
      void ensureAppInfoLoaded();
      void ensureSettingsLoaded();
    }
  },
  { immediate: true }
);

const removeBeforeEach = router.beforeEach((to, from, next) => {
  routeLoadingActive = !isWebSessionOnlyRouteChange(to, from);
  if (routeLoadingActive) {
    loadingBar?.start();
  }
  next();
});
const removeAfterEach = router.afterEach(() => {
  if (routeLoadingActive) {
    loadingBar?.finish();
    routeLoadingActive = false;
  }
});
const removeOnError = router.onError(() => {
  if (routeLoadingActive) {
    loadingBar?.error();
    routeLoadingActive = false;
  }
});

onBeforeUnmount(() => {
  teardownErrorHandler?.();
  if (typeof window !== 'undefined') {
    window.removeEventListener('codekanban:unauthorized', handleUnauthorized as EventListener);
  }
  removeBeforeEach();
  removeAfterEach();
  removeOnError();
});

if (typeof window !== 'undefined') {
  window.addEventListener('codekanban:unauthorized', handleUnauthorized as EventListener);
}
</script>
