<template>
  <div class="history-cleanup-entry">
    <div class="history-cleanup-entry__copy">
      <div class="history-cleanup-entry__title">{{ t('settings.historyCleanupTitle') }}</div>
      <div class="form-tip">{{ t('settings.historyCleanupDescription') }}</div>
    </div>
    <n-tooltip trigger="hover" placement="top">
      <template #trigger>
        <n-button
          type="error"
          secondary
          :aria-label="t('settings.historyCleanupAction')"
          @click="openDialog"
        >
          <template #icon>
            <n-icon><TrashOutline /></n-icon>
          </template>
          {{ t('settings.historyCleanupAction') }}
        </n-button>
      </template>
      {{ t('settings.historyCleanupAction') }}
    </n-tooltip>
  </div>

  <n-modal
    v-model:show="showDialog"
    preset="card"
    class="history-cleanup-modal"
    style="width: min(680px, calc(100vw - 32px))"
    :title="t('settings.historyCleanupDialogTitle')"
    :mask-closable="!previewLoading && !cleanupRunning"
    :closable="!previewLoading && !cleanupRunning"
  >
    <n-space vertical size="large">
      <n-form label-placement="top">
        <n-form-item :label="t('settings.historyCleanupScope')">
          <n-radio-group v-model:value="scope" :disabled="previewLoading || cleanupRunning">
            <n-space>
              <n-radio value="all">{{ t('settings.historyCleanupScopeAll') }}</n-radio>
              <n-radio value="projects">{{ t('settings.historyCleanupScopeProjects') }}</n-radio>
            </n-space>
          </n-radio-group>
        </n-form-item>

        <n-form-item
          v-if="scope === 'projects'"
          :label="t('settings.historyCleanupProjects')"
          :validation-status="selectedProjectIds.length === 0 ? 'error' : undefined"
        >
          <n-select
            v-model:value="selectedProjectIds"
            multiple
            filterable
            clearable
            :loading="projectStore.loading"
            :options="projectOptions"
            :placeholder="t('settings.historyCleanupProjectsPlaceholder')"
            :disabled="previewLoading || cleanupRunning"
          />
        </n-form-item>

        <div class="history-cleanup-fields">
          <n-form-item :label="t('settings.historyCleanupOlderThanDays')">
            <n-input-number
              v-model:value="olderThanDays"
              :min="0"
              :max="36500"
              :step="1"
              :disabled="previewLoading || cleanupRunning"
            />
          </n-form-item>
          <n-form-item :label="t('settings.historyCleanupRetainPerProject')">
            <n-input-number
              v-model:value="retainPerProject"
              :min="0"
              :max="10000"
              :step="1"
              :disabled="previewLoading || cleanupRunning"
            />
          </n-form-item>
        </div>
      </n-form>

      <n-alert v-if="isFullCleanup" type="warning" :bordered="false">
        {{ t('settings.historyCleanupFullWarning') }}
      </n-alert>

      <div v-if="preview" class="history-cleanup-preview">
        <div class="history-cleanup-stats">
          <n-statistic
            :label="t('settings.historyCleanupSessions')"
            :value="preview.historySessionCount"
          />
          <n-statistic
            :label="t('settings.historyCleanupRows')"
            :value="preview.itemRowCount + preview.turnRowCount"
          />
          <n-statistic
            :label="t('settings.historyCleanupObsoleteRows')"
            :value="preview.obsoleteItemRowCount + preview.obsoleteTurnRowCount"
          />
          <n-statistic
            :label="t('settings.historyCleanupReusableSpace')"
            :value="formatBytes(preview.storage.reusableBytes)"
          />
        </div>
        <n-alert v-if="preview.skippedBusySessionCount > 0" type="info" :bordered="false">
          {{
            t('settings.historyCleanupSkippedBusy', {
              count: preview.skippedBusySessionCount,
            })
          }}
        </n-alert>
        <n-alert v-if="preview.nonSyncableSessionCount > 0" type="warning" :bordered="false">
          {{
            t('settings.historyCleanupNonSyncable', {
              count: preview.nonSyncableSessionCount,
            })
          }}
        </n-alert>
      </div>

      <n-space justify="end">
        <n-button :disabled="previewLoading || cleanupRunning" @click="showDialog = false">
          {{ t('common.cancel') }}
        </n-button>
        <n-button
          :loading="previewLoading"
          :disabled="!canPreview || cleanupRunning"
          @click="previewCleanup"
        >
          {{ t('settings.historyCleanupPreviewAction') }}
        </n-button>
        <n-button
          type="error"
          :loading="cleanupRunning"
          :disabled="!canRunCleanup || previewLoading"
          @click="confirmCleanup"
        >
          <template #icon>
            <n-icon><TrashOutline /></n-icon>
          </template>
          {{ t('settings.historyCleanupRunAction') }}
        </n-button>
      </n-space>
    </n-space>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import { TrashOutline } from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import {
  webSessionApi,
  type WebSessionHistoryCleanupParams,
  type WebSessionHistoryCleanupStats,
} from '@/api/webSession';
import { useProjectStore } from '@/stores/project';
import { useWebSessionStore } from '@/stores/webSession';

const { t } = useLocale();
const dialog = useDialog();
const message = useMessage();
const projectStore = useProjectStore();
const webSessionStore = useWebSessionStore();

const showDialog = ref(false);
const scope = ref<'all' | 'projects'>('all');
const selectedProjectIds = ref<string[]>([]);
const olderThanDays = ref<number | null>(30);
const retainPerProject = ref<number | null>(10);
const preview = ref<WebSessionHistoryCleanupStats | null>(null);
const previewRequestKey = ref('');
const previewLoading = ref(false);
const cleanupRunning = ref(false);

const projectOptions = computed(() =>
  [...projectStore.projects]
    .sort((left, right) => left.name.localeCompare(right.name))
    .map(project => ({
      label: project.hidePath ? project.name : `${project.name} - ${project.path}`,
      value: project.id,
    }))
);

const normalizedOlderThanDays = computed(() =>
  Math.max(0, Math.trunc(Number(olderThanDays.value ?? 0)))
);
const normalizedRetainPerProject = computed(() =>
  Math.max(0, Math.trunc(Number(retainPerProject.value ?? 0)))
);
const isFullCleanup = computed(
  () => normalizedOlderThanDays.value === 0 && normalizedRetainPerProject.value === 0
);
const canPreview = computed(() => scope.value === 'all' || selectedProjectIds.value.length > 0);

const cleanupParams = computed<WebSessionHistoryCleanupParams>(() => ({
  scope: scope.value,
  projectIds: scope.value === 'projects' ? [...selectedProjectIds.value] : [],
  olderThanDays: normalizedOlderThanDays.value,
  retainPerProject: normalizedRetainPerProject.value,
}));
const cleanupRequestKey = computed(() => JSON.stringify(cleanupParams.value));
const canRunCleanup = computed(
  () => Boolean(preview.value) && previewRequestKey.value === cleanupRequestKey.value
);

watch(
  [scope, selectedProjectIds, olderThanDays, retainPerProject],
  () => {
    preview.value = null;
    previewRequestKey.value = '';
  },
  { deep: true }
);

onMounted(() => {
  if (projectStore.projects.length === 0) {
    void projectStore.fetchProjects({ silent: true });
  }
});

async function openDialog() {
  showDialog.value = true;
  preview.value = null;
  if (projectStore.projects.length === 0) {
    try {
      await projectStore.fetchProjects({ silent: true });
    } catch (error) {
      console.error('Failed to load projects for history cleanup:', error);
      message.error(t('settings.historyCleanupProjectsFailed'));
    }
  }
}

async function previewCleanup() {
  if (!canPreview.value || previewLoading.value) {
    return;
  }
  previewLoading.value = true;
  try {
    preview.value = await webSessionApi.previewHistoryCleanup(cleanupParams.value);
    previewRequestKey.value = cleanupRequestKey.value;
  } catch (error) {
    console.error('Failed to preview web session history cleanup:', error);
    message.error(t('settings.historyCleanupPreviewFailed'));
  } finally {
    previewLoading.value = false;
  }
}

function confirmCleanup() {
  if (!preview.value || !canRunCleanup.value || cleanupRunning.value) {
    return;
  }
  const stats = preview.value;
  dialog.warning({
    title: t('settings.historyCleanupConfirmTitle'),
    content: t('settings.historyCleanupConfirmContent', {
      sessions: stats.historySessionCount,
      rows: stats.itemRowCount + stats.turnRowCount,
    }),
    positiveText: t('settings.historyCleanupRunAction'),
    negativeText: t('common.cancel'),
    onPositiveClick: runCleanup,
  });
}

async function runCleanup() {
  cleanupRunning.value = true;
  try {
    const result = await webSessionApi.runHistoryCleanup(cleanupParams.value);
    preview.value = result;
    previewRequestKey.value = cleanupRequestKey.value;
    webSessionStore.invalidateCleanedHistories(result.clearedSessionIds);
    message.success(
      t('settings.historyCleanupSuccess', {
        sessions: result.historySessionCount,
        rows: result.itemRowCount + result.turnRowCount,
      })
    );
    if (result.historyFileFailureCount > 0) {
      message.warning(
        t('settings.historyCleanupFileFailures', { count: result.historyFileFailureCount })
      );
    }
    return true;
  } catch (error) {
    console.error('Failed to run web session history cleanup:', error);
    message.error(t('settings.historyCleanupFailed'));
    return false;
  } finally {
    cleanupRunning.value = false;
  }
}

function formatBytes(value: number) {
  const bytes = Math.max(0, Number(value) || 0);
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  const units = ['KB', 'MB', 'GB', 'TB'];
  let size = bytes / 1024;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[unitIndex]}`;
}
</script>

<style scoped>
.history-cleanup-entry {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.history-cleanup-entry__copy {
  min-width: 0;
}

.history-cleanup-entry__title {
  margin-bottom: 4px;
  font-size: 14px;
  font-weight: 600;
}

.history-cleanup-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.history-cleanup-fields :deep(.n-input-number) {
  width: 100%;
}

.history-cleanup-preview {
  display: grid;
  gap: 12px;
}

.history-cleanup-stats {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  padding: 16px;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
}

@media (max-width: 640px) {
  .history-cleanup-entry {
    align-items: flex-start;
    flex-direction: column;
  }

  .history-cleanup-fields,
  .history-cleanup-stats {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
