<template>
  <n-card :title="t('settings.gitSettings')" size="huge">
    <template #header-extra>
      <n-button
        type="primary"
        size="small"
        :loading="saving"
        :disabled="loading || !dirty"
        @click="save"
      >
        {{ t('common.save') }}
      </n-button>
    </template>

    <n-form label-placement="top">
      <n-form-item :label="t('settings.gitReadEngine')">
        <n-radio-group v-model:value="form.readEngine" size="small">
          <n-radio-button v-for="option in engineOptions" :key="option.value" :value="option.value">
            {{ option.label }}
          </n-radio-button>
        </n-radio-group>
        <template #feedback>
          <span class="form-tip">{{ t('settings.gitReadEngineTip') }}</span>
        </template>
      </n-form-item>

      <n-form-item :label="t('settings.gitWriteEngine')">
        <n-radio-group v-model:value="form.writeEngine" size="small">
          <n-radio-button v-for="option in engineOptions" :key="option.value" :value="option.value">
            {{ option.label }}
          </n-radio-button>
        </n-radio-group>
        <template #feedback>
          <span class="form-tip">{{ t('settings.gitWriteEngineTip') }}</span>
        </template>
      </n-form-item>

      <n-form-item :label="t('settings.gitExecutable')">
        <n-space vertical size="small" style="width: 100%">
          <n-space :wrap="false">
            <n-input
              v-model:value="form.executable"
              clearable
              :placeholder="t('settings.gitExecutablePlaceholder')"
            />
            <n-button
              quaternary
              circle
              :loading="probing"
              :title="t('settings.gitProbe')"
              @click="probe"
            >
              <template #icon>
                <n-icon><RefreshOutline /></n-icon>
              </template>
            </n-button>
          </n-space>
          <span class="form-tip">{{ t('settings.gitExecutableTip') }}</span>
        </n-space>
      </n-form-item>

      <n-form-item :label="t('settings.gitSystemStatus')">
        <n-alert :type="systemGit.available ? 'success' : 'warning'" :show-icon="false">
          <template v-if="systemGit.available">
            <div>{{ systemGit.version }}</div>
            <n-ellipsis v-if="systemGit.executable" class="git-path">
              {{ systemGit.executable }}
            </n-ellipsis>
          </template>
          <template v-else>
            {{ systemGit.error || t('settings.gitUnavailable') }}
          </template>
        </n-alert>
      </n-form-item>
    </n-form>
  </n-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useMessage } from 'naive-ui';
import { RefreshOutline } from '@vicons/ionicons5';
import { http } from '@/api/http';
import { useLocale } from '@/composables/useLocale';
import { useProjectStore } from '@/stores/project';

type GitEnginePreference = 'auto' | 'builtin' | 'system';

interface SystemGitInfo {
  available: boolean;
  executable?: string;
  version?: string;
  error?: string;
}

interface GitSettingsResult {
  readEngine: GitEnginePreference;
  writeEngine: GitEnginePreference;
  executable?: string;
  systemGit: SystemGitInfo;
}

type ItemResponse<T> = { item?: T };

const { t } = useLocale();
const message = useMessage();
const projectStore = useProjectStore();
const loading = ref(false);
const saving = ref(false);
const probing = ref(false);
const original = ref<GitSettingsResult | null>(null);
const systemGit = reactive<SystemGitInfo>({ available: false });
const form = reactive({
  readEngine: 'auto' as GitEnginePreference,
  writeEngine: 'auto' as GitEnginePreference,
  executable: '',
});

const engineOptions = computed(() => [
  { value: 'auto' as const, label: t('settings.gitEngineAuto') },
  { value: 'builtin' as const, label: t('settings.gitEngineBuiltin') },
  { value: 'system' as const, label: t('settings.gitEngineSystem') },
]);

const dirty = computed(() => {
  const current = original.value;
  return (
    !current ||
    current.readEngine !== form.readEngine ||
    current.writeEngine !== form.writeEngine ||
    (current.executable ?? '') !== form.executable.trim()
  );
});

function applyResult(result: GitSettingsResult) {
  form.readEngine = result.readEngine;
  form.writeEngine = result.writeEngine;
  form.executable = result.executable ?? '';
  Object.assign(systemGit, result.systemGit ?? { available: false });
  original.value = {
    ...result,
    executable: result.executable ?? '',
  };
}

async function load(refresh = false) {
  loading.value = true;
  try {
    const response = await http
      .Get<ItemResponse<GitSettingsResult>>(`/system/git-settings${refresh ? '?refresh=true' : ''}`)
      .send();
    if (response.item) {
      applyResult(response.item);
    }
  } finally {
    loading.value = false;
  }
}

async function probe() {
  probing.value = true;
  try {
    await load(true);
  } catch (error: any) {
    message.error(error?.message ?? t('settings.gitProbeFailed'));
  } finally {
    probing.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    const response = await http
      .Post<ItemResponse<GitSettingsResult>>('/system/git-settings/update', {
        readEngine: form.readEngine,
        writeEngine: form.writeEngine,
        executable: form.executable.trim(),
      })
      .send();
    if (response.item) {
      applyResult(response.item);
    }
    if (projectStore.currentProject?.id) {
      await projectStore.fetchGitCapabilities(projectStore.currentProject.id);
    }
    message.success(t('settings.gitSettingsSaved'));
  } catch (error: any) {
    message.error(error?.message ?? t('common.error'));
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  void load().catch((error: any) => {
    message.error(error?.message ?? t('common.error'));
  });
});
</script>

<style scoped>
.git-path {
  display: block;
  margin-top: 4px;
  max-width: 100%;
  font-family: var(--font-mono, monospace);
  font-size: 12px;
  opacity: 0.72;
}
</style>
