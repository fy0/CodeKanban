import { defineStore } from 'pinia';
import { ref } from 'vue';
import { systemApi } from '@/api/project';

export interface AppInfo {
  name: string;
  version: string;
  channel: string;
}

export const useAppStore = defineStore('app', () => {
  const appInfo = ref<AppInfo>({
    name: 'Code Kanban',
    version: '0.0.0',
    channel: 'unknown',
  });

  const loading = ref(false);
  const loaded = ref(false);

  async function fetchAppInfo() {
    if (loaded.value) {
      return;
    }

    try {
      loading.value = true;
      const info = await systemApi.getVersion();
      appInfo.value = info;
      loaded.value = true;
    } catch (error) {
      console.error('Failed to fetch app info:', error);
    } finally {
      loading.value = false;
    }
  }

  return {
    appInfo,
    loading,
    loaded,
    fetchAppInfo,
  };
});
