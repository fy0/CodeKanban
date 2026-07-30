import { defineStore } from 'pinia';
import { ref } from 'vue';
import { http } from '@/api/http';
import type { DeveloperConfig } from '@/types/models';
import { cloneDeveloperConfig, sanitizeDeveloperConfig } from '@/utils/developerConfig';

type ItemResponse<T> = {
  item?: T;
};

export const useDeveloperConfigStore = defineStore('developerConfig', () => {
  const config = ref<DeveloperConfig>(sanitizeDeveloperConfig());
  const loaded = ref(false);
  const loading = ref(false);
  const saving = ref(false);
  let loadPromise: Promise<DeveloperConfig> | null = null;

  async function load(force = false): Promise<DeveloperConfig> {
    if (loaded.value && !force) {
      return cloneDeveloperConfig(config.value);
    }
    if (loadPromise) {
      return loadPromise;
    }
    loadPromise = (async () => {
      loading.value = true;
      try {
        const response = await http
          .Get<ItemResponse<DeveloperConfig>>('/system/developer-config', { cacheFor: 0 })
          .send(true);
        config.value = sanitizeDeveloperConfig(response?.item);
        loaded.value = true;
        return cloneDeveloperConfig(config.value);
      } finally {
        loading.value = false;
        loadPromise = null;
      }
    })();
    return loadPromise;
  }

  async function update(value: DeveloperConfig): Promise<DeveloperConfig> {
    const normalized = sanitizeDeveloperConfig(value);
    saving.value = true;
    try {
      await http.Post('/system/developer-config/update', normalized).send();
      config.value = cloneDeveloperConfig(normalized);
      loaded.value = true;
      return cloneDeveloperConfig(config.value);
    } finally {
      saving.value = false;
    }
  }

  return {
    config,
    loaded,
    loading,
    saving,
    load,
    update,
  };
});
