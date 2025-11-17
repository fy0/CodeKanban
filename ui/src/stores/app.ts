import { defineStore } from 'pinia';
import { ref } from 'vue';

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

  function setAppInfo(info: AppInfo) {
    appInfo.value = info;
  }

  return {
    appInfo,
    setAppInfo,
  };
});
