<template>
  <div class="history-cleanup-entry">
    <div class="history-cleanup-entry__copy">
      <div class="history-cleanup-entry__title">{{ t('settings.historyCleanupTitle') }}</div>
      <div class="form-tip">{{ t('settings.historyCleanupDescription') }}</div>
      <div class="history-cleanup-entry__hint">
        <n-icon size="14"><InformationCircleOutline /></n-icon>
        <span>{{ t('settings.historyCleanupEntryHint') }}</span>
      </div>
    </div>
    <n-button secondary :aria-label="t('settings.historyCleanupAction')" @click="openDialog">
      <template #icon>
        <n-icon><SettingsOutline /></n-icon>
      </template>
      {{ t('settings.historyCleanupAction') }}
    </n-button>
  </div>

  <n-modal
    v-model:show="showDialog"
    preset="card"
    class="history-cleanup-modal"
    style="width: min(760px, calc(100vw - 32px))"
    :title="t('settings.historyCleanupDialogTitle')"
    :mask-closable="!previewLoading && !cleanupRunning"
    :closable="!previewLoading && !cleanupRunning"
  >
    <n-space vertical size="large">
      <n-alert type="info" :bordered="false">
        {{ t('settings.historyCleanupConcept') }}
      </n-alert>

      <section class="history-storage-overview" aria-live="polite">
        <div class="history-storage-overview__header">
          <div>
            <div class="history-storage-overview__title">
              {{ t('settings.historyStorageOverviewTitle') }}
            </div>
            <div class="form-tip">{{ t('settings.historyStorageOverviewDescription') }}</div>
          </div>
          <n-tooltip trigger="hover" placement="top">
            <template #trigger>
              <n-button
                quaternary
                circle
                :loading="storageLoading"
                :aria-label="t('settings.historyStorageRefresh')"
                @click="refreshStorageOverview"
              >
                <template #icon>
                  <n-icon><RefreshOutline /></n-icon>
                </template>
              </n-button>
            </template>
            {{ t('settings.historyStorageRefresh') }}
          </n-tooltip>
        </div>
        <div v-if="storageOverview" class="history-storage-overview__stats">
          <n-statistic
            :label="t('settings.historyStorageDatabase')"
            :value="formatBytes(storageOverview.databaseBytes)"
          />
          <n-statistic
            :label="t('settings.historyStorageWal')"
            :value="formatBytes(storageOverview.walBytes)"
          />
          <n-statistic
            :label="t('settings.historyStorageReusable')"
            :value="formatBytes(storageOverview.reusableBytes)"
          />
          <n-statistic
            :label="t('settings.historyStorageFreeDisk')"
            :value="formatBytes(storageOverview.freeDiskBytes)"
          />
        </div>
        <div v-else class="history-storage-overview__empty">
          {{
            storageLoading
              ? t('settings.historyStorageLoading')
              : t('settings.historyStorageUnavailable')
          }}
        </div>

        <div class="history-storage-details__header">
          <div>
            <div class="history-storage-overview__title">
              {{ t('settings.historyStorageDetailsTitle') }}
            </div>
            <div class="form-tip">{{ t('settings.historyStorageDetailsDescription') }}</div>
          </div>
          <n-button secondary :loading="storageDetailsLoading" @click="loadStorageDetails">
            <template #icon>
              <n-icon><AnalyticsOutline /></n-icon>
            </template>
            {{
              storageDetails
                ? t('settings.historyStorageDetailsRefresh')
                : t('settings.historyStorageDetailsAction')
            }}
          </n-button>
        </div>
        <div v-if="storageDetails" class="history-storage-overview__stats">
          <n-statistic
            :label="t('settings.historyStorageHistory')"
            :value="formatBytes(storageDetails.historyBytes)"
          />
          <n-statistic
            :label="t('settings.historyStorageItems')"
            :value="formatBytes(storageDetails.itemBytes)"
          />
          <n-statistic
            :label="t('settings.historyStorageTurns')"
            :value="formatBytes(storageDetails.turnBytes)"
          />
          <n-statistic
            :label="t('settings.historyStorageSubAgents')"
            :value="formatBytes(storageDetails.subAgentBytes)"
          />
          <n-statistic
            :label="t('settings.historyStorageArchivedCache')"
            :value="formatBytes(storageDetails.archivedCacheBytes)"
          />
        </div>
        <div v-else-if="storageDetailsLoading" class="history-storage-overview__empty">
          {{ t('settings.historyStorageDetailsLoading') }}
        </div>
      </section>

      <n-radio-group
        v-model:value="mode"
        name="history-cleanup-mode"
        :disabled="previewLoading || cleanupRunning"
      >
        <n-space>
          <n-radio-button value="cleanup">
            <template #default>{{ t('settings.historyCleanupModeCleanup') }}</template>
          </n-radio-button>
          <n-radio-button value="archive">
            <template #default>{{ t('settings.historyCleanupModeArchive') }}</template>
          </n-radio-button>
          <n-radio-button value="archived-cache">
            <template #default>{{ t('settings.historyCleanupModeArchivedCache') }}</template>
          </n-radio-button>
        </n-space>
      </n-radio-group>

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

        <div v-if="mode === 'cleanup'" class="history-cleanup-fields">
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

        <n-form-item
          v-else-if="mode === 'archive'"
          :label="t('settings.historyArchiveOlderThanDays')"
        >
          <n-input-number
            v-model:value="archiveOlderThanDays"
            :min="0"
            :max="36500"
            :step="1"
            :disabled="previewLoading || cleanupRunning"
          />
          <template #feedback>{{ t('settings.historyArchiveOlderThanDaysTip') }}</template>
        </n-form-item>

        <n-form-item v-else :label="t('settings.historyArchivedCacheOlderThanDays')">
          <n-input-number
            v-model:value="archivedCacheOlderThanDays"
            :min="0"
            :max="36500"
            :step="1"
            :disabled="previewLoading || cleanupRunning"
          />
          <template #feedback>{{ t('settings.historyArchivedCacheOlderThanDaysTip') }}</template>
        </n-form-item>
      </n-form>

      <n-alert v-if="isFullCleanup" type="warning" :bordered="false">
        {{ t('settings.historyCleanupFullWarning') }}
      </n-alert>
      <n-alert v-else-if="mode === 'archive'" type="info" :bordered="false">
        {{ t('settings.historyArchiveWarning') }}
      </n-alert>
      <n-alert v-else-if="mode === 'archived-cache'" type="warning" :bordered="false">
        {{ t('settings.historyArchivedCacheWarning') }}
      </n-alert>

      <div v-if="preview" class="history-cleanup-preview">
        <div class="history-cleanup-stats">
          <n-statistic :label="t('settings.historyCleanupSessions')" :value="previewSessionCount" />
          <n-statistic
            v-if="mode !== 'archive'"
            :label="t('settings.historyCleanupRows')"
            :value="previewRows"
          />
          <n-statistic
            v-if="mode !== 'archive'"
            :label="t('settings.historyCleanupEstimatedBytes')"
            :value="formatBytes(previewEstimatedBytes)"
          />
          <n-statistic
            v-if="mode === 'cleanup'"
            :label="t('settings.historyCleanupObsoleteRows')"
            :value="previewObsoleteRows"
          />
          <n-statistic
            v-if="mode === 'cleanup'"
            :label="t('settings.historyCleanupReusableSpace')"
            :value="formatBytes(previewReusableBytes)"
          />
        </div>
        <n-alert v-if="previewSkippedBusy > 0" type="info" :bordered="false">
          {{ t('settings.historyCleanupSkippedBusy', { count: previewSkippedBusy }) }}
        </n-alert>
        <n-alert v-if="previewNonSyncable > 0" type="warning" :bordered="false">
          {{
            t('settings.historyCleanupNonSyncable', {
              count: previewNonSyncable,
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
          :type="mode === 'archive' ? 'primary' : 'error'"
          :loading="cleanupRunning"
          :disabled="!canRunCleanup || previewLoading"
          @click="confirmCleanup"
        >
          <template #icon>
            <n-icon><ArchiveOutline v-if="mode === 'archive'" /><TrashOutline v-else /></n-icon>
          </template>
          {{
            mode === 'archive'
              ? t('settings.historyArchiveRunAction')
              : t('settings.historyCleanupRunAction')
          }}
        </n-button>
      </n-space>
    </n-space>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import {
  AnalyticsOutline,
  ArchiveOutline,
  InformationCircleOutline,
  RefreshOutline,
  SettingsOutline,
  TrashOutline,
} from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import {
  webSessionApi,
  type WebSessionHistoryArchiveParams,
  type WebSessionHistoryArchiveResult,
  type WebSessionHistoryArchiveStats,
  type WebSessionHistoryCleanupParams,
  type WebSessionHistoryCleanupResult,
  type WebSessionHistoryCleanupStats,
  type WebSessionHistoryCleanupStorageStats,
} from '@/api/webSession';
import { useProjectStore } from '@/stores/project';
import { useWebSessionStore } from '@/stores/webSession';

type HistoryCleanupMode = 'cleanup' | 'archive' | 'archived-cache';
type HistoryActionPreview = WebSessionHistoryCleanupStats | WebSessionHistoryArchiveStats;

const { t } = useLocale();
const dialog = useDialog();
const message = useMessage();
const projectStore = useProjectStore();
const webSessionStore = useWebSessionStore();

const showDialog = ref(false);
const mode = ref<HistoryCleanupMode>('cleanup');
const scope = ref<'all' | 'projects'>('all');
const selectedProjectIds = ref<string[]>([]);
const olderThanDays = ref<number | null>(30);
const retainPerProject = ref<number | null>(10);
const archiveOlderThanDays = ref<number | null>(30);
const archivedCacheOlderThanDays = ref<number | null>(30);
const preview = ref<HistoryActionPreview | null>(null);
const previewMode = ref<HistoryCleanupMode | null>(null);
const previewRequestKey = ref('');
const previewLoading = ref(false);
const cleanupRunning = ref(false);
const storageOverview = ref<WebSessionHistoryCleanupStorageStats | null>(null);
const storageLoading = ref(false);
const storageDetails = ref<WebSessionHistoryCleanupStorageStats | null>(null);
const storageDetailsLoading = ref(false);

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
const normalizedArchiveOlderThanDays = computed(() =>
  Math.max(0, Math.trunc(Number(archiveOlderThanDays.value ?? 0)))
);
const normalizedArchivedCacheOlderThanDays = computed(() =>
  Math.max(0, Math.trunc(Number(archivedCacheOlderThanDays.value ?? 0)))
);
const isFullCleanup = computed(
  () =>
    mode.value === 'cleanup' &&
    normalizedOlderThanDays.value === 0 &&
    normalizedRetainPerProject.value === 0
);
const canPreview = computed(() => scope.value === 'all' || selectedProjectIds.value.length > 0);

const cleanupParams = computed<WebSessionHistoryCleanupParams>(() => ({
  scope: scope.value,
  projectIds: scope.value === 'projects' ? [...selectedProjectIds.value] : [],
  olderThanDays: normalizedOlderThanDays.value,
  retainPerProject: normalizedRetainPerProject.value,
}));
const archiveParams = computed<WebSessionHistoryArchiveParams>(() => ({
  scope: scope.value,
  projectIds: scope.value === 'projects' ? [...selectedProjectIds.value] : [],
  olderThanDays: normalizedArchiveOlderThanDays.value,
}));
const archivedCacheParams = computed<WebSessionHistoryCleanupParams>(() => ({
  scope: scope.value,
  projectIds: scope.value === 'projects' ? [...selectedProjectIds.value] : [],
  olderThanDays: 0,
  retainPerProject: 0,
  archivedOnly: true,
  archivedOlderThanDays: normalizedArchivedCacheOlderThanDays.value,
}));
const cleanupRequestKey = computed(() => JSON.stringify(cleanupParams.value));
const actionRequestKey = computed(() => {
  if (mode.value === 'archive') {
    return JSON.stringify(archiveParams.value);
  }
  if (mode.value === 'archived-cache') {
    return JSON.stringify(archivedCacheParams.value);
  }
  return cleanupRequestKey.value;
});
const canRunCleanup = computed(
  () =>
    Boolean(preview.value) &&
    previewMode.value === mode.value &&
    previewRequestKey.value === actionRequestKey.value &&
    (mode.value !== 'cleanup' || previewRequestKey.value === cleanupRequestKey.value)
);
const previewSessionCount = computed(() => {
  if (!preview.value) return 0;
  return mode.value === 'archive'
    ? (preview.value as WebSessionHistoryArchiveStats).candidateSessionCount
    : (preview.value as WebSessionHistoryCleanupStats).historySessionCount;
});
const previewRows = computed(() => {
  if (!preview.value || mode.value === 'archive') return 0;
  const stats = preview.value as WebSessionHistoryCleanupStats;
  return stats.itemRowCount + stats.turnRowCount;
});
const previewObsoleteRows = computed(() => {
  if (!preview.value || mode.value !== 'cleanup') return 0;
  const stats = preview.value as WebSessionHistoryCleanupStats;
  return stats.obsoleteItemRowCount + stats.obsoleteTurnRowCount;
});
const previewEstimatedBytes = computed(() => {
  if (!preview.value || mode.value === 'archive') return 0;
  return (preview.value as WebSessionHistoryCleanupStats).estimatedBytes;
});
const previewReusableBytes = computed(() => {
  if (!preview.value || mode.value === 'archive') return 0;
  return (preview.value as WebSessionHistoryCleanupStats).storage.reusableBytes;
});
const previewSkippedBusy = computed(() => {
  if (!preview.value) return 0;
  return preview.value.skippedBusySessionCount;
});
const previewNonSyncable = computed(() => {
  if (!preview.value || mode.value === 'archive') return 0;
  return (preview.value as WebSessionHistoryCleanupStats).nonSyncableSessionCount;
});

watch(
  [
    mode,
    scope,
    selectedProjectIds,
    olderThanDays,
    retainPerProject,
    archiveOlderThanDays,
    archivedCacheOlderThanDays,
  ],
  () => {
    preview.value = null;
    previewMode.value = null;
    previewRequestKey.value = '';
  },
  { deep: true }
);

onMounted(() => {
  if (projectStore.projects.length === 0) {
    void projectStore.fetchProjects({ silent: true });
  }
});

async function refreshStorageOverview() {
  if (storageLoading.value) return;
  storageLoading.value = true;
  try {
    storageOverview.value = await webSessionApi.historyStorageOverview();
  } catch (error) {
    console.error('Failed to load web session storage overview:', error);
    message.error(t('settings.historyStorageFailed'));
  } finally {
    storageLoading.value = false;
  }
}

async function loadStorageDetails() {
  if (storageDetailsLoading.value) return;
  storageDetailsLoading.value = true;
  try {
    storageDetails.value = await webSessionApi.historyStorageDetails();
  } catch (error) {
    console.error('Failed to analyze web session storage details:', error);
    message.error(t('settings.historyStorageDetailsFailed'));
  } finally {
    storageDetailsLoading.value = false;
  }
}

async function openDialog() {
  showDialog.value = true;
  preview.value = null;
  previewMode.value = null;
  void refreshStorageOverview();
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
  if (!canPreview.value || previewLoading.value) return;
  previewLoading.value = true;
  try {
    if (mode.value === 'archive') {
      preview.value = await webSessionApi.previewHistoryArchive(archiveParams.value);
    } else if (mode.value === 'archived-cache') {
      preview.value = await webSessionApi.previewHistoryCleanup(archivedCacheParams.value);
    } else {
      preview.value = await webSessionApi.previewHistoryCleanup(cleanupParams.value);
    }
    previewMode.value = mode.value;
    previewRequestKey.value = actionRequestKey.value;
  } catch (error) {
    console.error('Failed to preview web session history action:', error);
    message.error(t('settings.historyCleanupPreviewFailed'));
  } finally {
    previewLoading.value = false;
  }
}

function confirmCleanup() {
  if (!preview.value || !canRunCleanup.value || cleanupRunning.value) return;
  dialog.warning({
    title:
      mode.value === 'archive'
        ? t('settings.historyArchiveConfirmTitle')
        : t('settings.historyCleanupConfirmTitle'),
    content:
      mode.value === 'archive'
        ? t('settings.historyArchiveConfirmContent', { sessions: previewSessionCount.value })
        : t('settings.historyCleanupConfirmContent', {
            sessions: previewSessionCount.value,
            rows: previewRows.value,
          }),
    positiveText:
      mode.value === 'archive'
        ? t('settings.historyArchiveRunAction')
        : t('settings.historyCleanupRunAction'),
    negativeText: t('common.cancel'),
    onPositiveClick: runCleanup,
  });
}

async function runCleanup() {
  cleanupRunning.value = true;
  try {
    if (mode.value === 'archive') {
      const result: WebSessionHistoryArchiveResult = await webSessionApi.runHistoryArchive(
        archiveParams.value
      );
      preview.value = result;
      previewMode.value = mode.value;
      previewRequestKey.value = actionRequestKey.value;
      await refreshSessionListsAfterArchive();
      message.success(
        t('settings.historyArchiveSuccess', { sessions: result.archivedSessionIds.length })
      );
    } else {
      const result: WebSessionHistoryCleanupResult = await webSessionApi.runHistoryCleanup(
        mode.value === 'archived-cache' ? archivedCacheParams.value : cleanupParams.value
      );
      preview.value = result;
      previewMode.value = mode.value;
      previewRequestKey.value = actionRequestKey.value;
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
    }
    storageDetails.value = null;
    void refreshStorageOverview();
    return true;
  } catch (error) {
    console.error('Failed to run web session history action:', error);
    message.error(
      mode.value === 'archive'
        ? t('settings.historyArchiveFailed')
        : t('settings.historyCleanupFailed')
    );
    return false;
  } finally {
    cleanupRunning.value = false;
  }
}

async function refreshSessionListsAfterArchive() {
  const projectIds =
    scope.value === 'projects'
      ? [...selectedProjectIds.value]
      : projectStore.projects.map(project => project.id);
  if (projectIds.length === 0) {
    return;
  }
  try {
    await Promise.all(projectIds.map(projectId => webSessionStore.loadSessions(projectId, true)));
    webSessionStore.invalidateArchivedSessions();
  } catch (error) {
    console.warn('Failed to refresh session lists after batch archive:', error);
  }
}

function formatBytes(value: number) {
  const bytes = Math.max(0, Number(value) || 0);
  if (bytes < 1024) return `${bytes} B`;
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

.history-cleanup-entry__hint {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: 6px;
  color: var(--n-text-color-3);
  font-size: 12px;
  line-height: 1.5;
}

.history-cleanup-entry__hint .n-icon {
  flex: 0 0 auto;
  margin-top: 2px;
}

.history-storage-overview {
  display: grid;
  gap: 14px;
  padding: 16px;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
}

.history-storage-overview__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.history-storage-details__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--n-border-color);
}

.history-storage-overview__title {
  font-size: 14px;
  font-weight: 600;
}

.history-storage-overview__stats,
.history-cleanup-fields,
.history-cleanup-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.history-storage-overview__empty {
  color: var(--n-text-color-3);
  font-size: 13px;
}

.history-cleanup-fields :deep(.n-input-number) {
  width: 100%;
}

.history-cleanup-preview {
  display: grid;
  gap: 12px;
}

.history-cleanup-stats {
  padding: 16px;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
}

@media (max-width: 760px) {
  .history-storage-overview__stats,
  .history-cleanup-fields,
  .history-cleanup-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .history-cleanup-entry {
    align-items: flex-start;
    flex-direction: column;
  }

  .history-storage-details__header {
    align-items: stretch;
    flex-direction: column;
  }

  .history-storage-overview__stats,
  .history-cleanup-fields,
  .history-cleanup-stats {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
