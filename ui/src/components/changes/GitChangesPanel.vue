<template>
  <div class="git-changes-panel" :class="{ 'is-mobile-preview': useMobilePreview }">
    <div class="git-changes-toolbar">
      <n-select
        class="git-changes-scope"
        :value="activeScopeId || null"
        :options="scopeOptions"
        :disabled="panelLoading"
        @update:value="handleScopeChange"
      />
      <n-input
        v-model:value="searchKeyword"
        clearable
        class="git-changes-search"
        :placeholder="t('gitChanges.searchPlaceholder')"
      />
      <n-checkbox v-model:checked="ignoreUntracked">
        {{ t('gitChanges.ignoreUntracked') }}
      </n-checkbox>
      <div class="git-changes-summary">
        <n-tag round :bordered="false" type="info">
          {{ t('gitChanges.summaryFiles', { count: visibleEntries.length }) }}
        </n-tag>
        <n-tag round :bordered="false" type="success" class="git-changes-summary-stat">
          {{ formatSignedCount('+', totalAdditions) }}
        </n-tag>
        <n-tag round :bordered="false" type="error" class="git-changes-summary-stat">
          {{ formatSignedCount('-', totalDeletions) }}
        </n-tag>
      </div>
      <n-button tertiary :loading="panelLoading" @click="reloadChanges">
        {{ t('gitChanges.refresh') }}
      </n-button>
    </div>

    <n-alert
      v-if="pendingChangesUpdate"
      type="info"
      class="git-changes-warning git-changes-update-alert"
      :show-icon="false"
    >
      <div class="git-changes-update-alert__content">
        <span>{{ t('gitChanges.updateAvailable') }}</span>
        <n-button size="tiny" tertiary :loading="panelLoading" @click="reloadChanges">
          {{ t('gitChanges.refresh') }}
        </n-button>
      </div>
    </n-alert>
    <n-alert v-if="showGitWarning" type="warning" class="git-changes-warning" :show-icon="false">
      <n-space align="center" size="small">
        <span>{{ t('gitChanges.notGitRepoShort') }}</span>
        <n-popover trigger="hover" placement="bottom-start">
          <template #trigger>
            <n-button text circle size="tiny" class="git-changes-warning__details-btn">
              <n-icon :size="16"><InformationCircleOutline /></n-icon>
            </n-button>
          </template>
          <span>{{ t('gitChanges.notGitRepoDetails') }}</span>
        </n-popover>
      </n-space>
    </n-alert>
    <n-alert
      v-for="warning in panelWarnings"
      :key="warning.key"
      :type="warning.type"
      class="git-changes-warning"
      :show-icon="false"
    >
      {{ t(warning.i18nKey, warning.params ?? {}) }}
    </n-alert>

    <div class="git-changes-body">
      <aside class="git-changes-sidebar">
        <n-spin :show="panelLoading" class="git-changes-list-spin">
          <div class="git-changes-list">
            <div v-if="panelError" class="git-changes-error">
              <n-alert type="error" :show-icon="false">{{ panelError }}</n-alert>
            </div>
            <div v-else-if="!gitFeaturesAvailable" class="git-changes-empty">
              <n-empty :description="t('gitChanges.notGitRepoShort')" />
            </div>
            <div v-else-if="filteredEntries.length === 0" class="git-changes-empty">
              <n-empty :description="emptyDescription" />
            </div>
            <div v-else class="git-changes-items">
              <button
                v-for="entry in filteredEntries"
                :key="entry.path"
                type="button"
                class="git-change-row"
                :class="{ 'is-active': selectedChangePath === entry.path }"
                @click="selectChange(entry)"
              >
                <div class="git-change-main">
                  <div class="git-change-title-row">
                    <span class="git-change-icon">
                      <n-icon size="16">
                        <component :is="entryIcon(entry)" />
                      </n-icon>
                    </span>
                    <span class="git-change-name">{{ entry.name }}</span>
                    <n-tag
                      size="small"
                      round
                      :bordered="false"
                      :type="resolveGitStatusTagType(entry.status.kind)"
                      class="git-change-status-tag"
                    >
                      {{ resolveGitStatusLetter(entry.status.kind) }}
                    </n-tag>
                    <n-tag size="small" round :bordered="false" class="git-change-stat-tag">
                      {{ formatChangeStat(entry) }}
                    </n-tag>
                  </div>
                  <div class="git-change-path">{{ entry.path }}</div>
                  <div
                    v-if="entry.status.kind === 'renamed' && entry.status.previousPath"
                    class="git-change-previous"
                  >
                    {{ t('fileManager.renamedFrom', { path: entry.status.previousPath }) }}
                  </div>
                </div>
              </button>
            </div>
          </div>
        </n-spin>
      </aside>

      <section v-if="!useMobilePreview" class="git-changes-preview">
        <FilePreviewSurface
          :preview-result="previewResult"
          :preview-loading="previewLoading"
          :preview-error="previewError"
          :preview-fallback-text="previewFallbackText"
          :rendered-markdown="renderedMarkdown"
          :rendered-diff="renderedDiff"
          :preview-meta="previewMetaText"
          :empty-label="t('gitChanges.previewEmpty')"
          :binary-preview-hint="t('fileManager.binaryPreviewHint')"
          :preview-truncated-label="t('fileManager.previewTruncated')"
          :download-label="t('fileManager.download')"
          :preview-mode="previewMode"
          :show-diff-toggle="showDiffToggle"
          :file-label="t('fileManager.fileView')"
          :diff-label="t('fileManager.diffView')"
          :diff-result="diffResult"
          :diff-loading="diffLoading"
          :diff-error="diffError"
          :diff-unavailable-text="diffUnavailableText"
          :fallback-title="selectedChange?.name || t('gitChanges.previewTitle')"
          @mode-change="handlePreviewModeChange"
          @download="downloadSelectedFile"
        />
      </section>
    </div>

    <n-modal
      :show="mobilePreviewVisible"
      class="git-changes-mobile-preview-modal"
      :closable="false"
      :mask-closable="false"
      @update:show="handleMobilePreviewVisibilityChange"
    >
      <div class="git-changes-mobile-preview-surface">
        <FilePreviewSurface
          :preview-result="previewResult"
          :preview-loading="previewLoading"
          :preview-error="previewError"
          :preview-fallback-text="previewFallbackText"
          :rendered-markdown="renderedMarkdown"
          :rendered-diff="renderedDiff"
          :preview-meta="previewMetaText"
          :empty-label="t('gitChanges.previewEmpty')"
          :binary-preview-hint="t('fileManager.binaryPreviewHint')"
          :preview-truncated-label="t('fileManager.previewTruncated')"
          :download-label="t('fileManager.download')"
          :preview-mode="previewMode"
          :show-diff-toggle="showDiffToggle"
          :file-label="t('fileManager.fileView')"
          :diff-label="t('fileManager.diffView')"
          :diff-result="diffResult"
          :diff-loading="diffLoading"
          :diff-error="diffError"
          :diff-unavailable-text="diffUnavailableText"
          :back-label="t('common.back')"
          :fallback-title="selectedChange?.name || t('gitChanges.previewTitle')"
          :mobile="true"
          :show-back-button="true"
          @close="closeMobilePreview"
          @mode-change="handlePreviewModeChange"
          @download="downloadSelectedFile"
        />
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import {
  DocumentOutline,
  ImageOutline,
  InformationCircleOutline,
  LinkOutline,
  MusicalNotesOutline,
  VideocamOutline,
} from '@vicons/ionicons5';
import { useStorage } from '@vueuse/core';
import { storeToRefs } from 'pinia';

import FilePreviewSurface from '@/components/files/FilePreviewSurface.vue';
import {
  hasPendingGitChangesUpdate,
  resolveGitChangeSelectionAfterLoad,
  resolveRetainedGitChangeEntry,
} from '@/components/changes/gitChangesBehavior';
import { createGitChangesLoadController } from '@/components/changes/gitChangesLoadController';
import {
  buildGitChangesBadgeSummary,
  chooseGitChangesScope,
  GIT_CHANGES_IGNORE_UNTRACKED_DEFAULT,
  GIT_CHANGES_IGNORE_UNTRACKED_STORAGE_KEY,
  orderGitChangesEntries,
  summarizeGitChangesEntries,
  type GitChangesBadgeSummary,
} from '@/components/changes/gitChangesSummary';
import {
  buildGitChangesProbeOptions,
  buildGitChangesRequestOptions,
  formatGitChangeCount,
  formatGitChangeStat,
  getGitChangesWarnings,
} from '@/components/changes/gitChangesSupport';
import {
  resolveDiffUnavailableReasonKey,
  resolveGitStatusLetter,
  resolveGitStatusTagType,
  resolveInitialChangePreviewMode,
  type FilePreviewMode,
} from '@/components/files/fileManagerDiff';
import { useLocale } from '@/composables/useLocale';
import { useResponsive } from '@/composables/useResponsive';
import { useProjectStore } from '@/stores/project';
import { fileManagerApi } from '@/api/fileManager';
import { renderHighlightedCodeBlock, renderMarkdown } from '@/utils/markdown';
import { projectSupportsGit } from '@/utils/projectGitCapability';
import type {
  FileManagerChangeEntry,
  FileManagerChangesResult,
  FileManagerDiffResult,
  FileManagerPreviewResult,
  FileManagerScope,
} from '@/types/fileManager';

const props = withDefaults(
  defineProps<{
    projectId: string;
    isActive?: boolean;
  }>(),
  {
    isActive: true,
  }
);
const emit = defineEmits<{
  'summary-change': [summary: GitChangesBadgeSummary | null];
}>();

const { t } = useLocale();
const projectStore = useProjectStore();
const { selectedWorktreeId } = storeToRefs(projectStore);
const { windowWidth } = useResponsive();

const MOBILE_PREVIEW_MAX_WIDTH = 900;
const MOBILE_PREVIEW_HISTORY_STATE_KEY = '__codekanbanGitChangesPreview';

const scopes = ref<FileManagerScope[]>([]);
const activeScopeId = ref('');
const changesResult = ref<FileManagerChangesResult | null>(null);
const panelLoading = ref(false);
const panelError = ref('');
const searchKeyword = ref('');
const ignoreUntracked = useStorage<boolean>(
  GIT_CHANGES_IGNORE_UNTRACKED_STORAGE_KEY,
  GIT_CHANGES_IGNORE_UNTRACKED_DEFAULT
);
const selectedChangePath = ref('');
const previewMode = ref<FilePreviewMode>('file');
const previewResult = ref<FileManagerPreviewResult | null>(null);
const previewLoading = ref(false);
const previewError = ref('');
const previewFallbackText = ref('');
const mobilePreviewVisible = ref(false);
const mobilePreviewHistoryActive = ref(false);
const mobilePreviewClosingFromHistory = ref(false);
const diffLoading = ref(false);
const diffError = ref('');
const diffResult = ref<FileManagerDiffResult | null>(null);
const pendingChangesUpdate = ref(false);
let previewRequestToken = 0;
const changesLoadController = createGitChangesLoadController();
const changesUpdateCheckController = createGitChangesLoadController();

const gitFeaturesAvailable = computed(() =>
  projectSupportsGit(projectStore.currentProject, projectStore.worktrees)
);
const showGitWarning = computed(
  () => Boolean(projectStore.currentProject) && !projectStore.loading && !gitFeaturesAvailable.value
);
const scopeOptions = computed(() =>
  scopes.value.map(scope => ({
    label: `${scope.label} · ${scope.rootPath}`,
    value: scope.id,
  }))
);
const normalizedSearch = computed(() => searchKeyword.value.trim().toLowerCase());
const useMobilePreview = computed(() => windowWidth.value <= MOBILE_PREVIEW_MAX_WIDTH);
const visibleEntries = computed(() =>
  orderGitChangesEntries(changesResult.value?.entries ?? [], ignoreUntracked.value)
);
const filteredEntries = computed(() => {
  const keyword = normalizedSearch.value;
  if (!keyword) {
    return visibleEntries.value;
  }
  return visibleEntries.value.filter(entry => entry.path.toLowerCase().includes(keyword));
});
const selectedChange = computed(
  () => filteredEntries.value.find(entry => entry.path === selectedChangePath.value) ?? null
);
const renderedMarkdown = computed(() =>
  previewResult.value?.previewKind === 'markdown'
    ? renderMarkdown(previewResult.value.textContent ?? '')
    : ''
);
const renderedDiff = computed(() =>
  diffResult.value?.available && diffResult.value.diffText
    ? renderHighlightedCodeBlock(diffResult.value.diffText, 'diff')
    : ''
);
const showDiffToggle = computed(() => Boolean(selectedChange.value?.exists && previewResult.value));
const diffUnavailableText = computed(() =>
  t(resolveDiffUnavailableReasonKey(diffResult.value?.reason))
);
const previewMetaText = computed(() => buildPreviewMeta(selectedChange.value));
const emptyDescription = computed(() =>
  normalizedSearch.value ? t('gitChanges.emptySearch') : t('gitChanges.empty')
);
const panelWarnings = computed(() => getGitChangesWarnings(changesResult.value));
const visibleSummary = computed(() =>
  summarizeGitChangesEntries(changesResult.value?.entries ?? [], ignoreUntracked.value)
);
const totalAdditions = computed(() =>
  !changesResult.value ? 0 : changesResult.value.statsComplete ? visibleSummary.value.additions : null
);
const totalDeletions = computed(() =>
  !changesResult.value ? 0 : changesResult.value.statsComplete ? visibleSummary.value.deletions : null
);
let refreshTimer: number | null = null;

function chooseScope(
  scopeList: FileManagerScope[],
  preferredWorktreeId?: string | null,
  requestedScopeId?: string
) {
  return chooseGitChangesScope(scopeList, {
    activeScopeId: activeScopeId.value,
    preferredWorktreeId,
    requestedScopeId,
  });
}

async function ensureLoaded(options?: { scopeId?: string }) {
  changesUpdateCheckController.cancel();
  if (!props.projectId || !props.isActive) {
    changesLoadController.cancel();
    panelLoading.value = false;
    return;
  }

  const loadHandle = changesLoadController.begin();
  panelLoading.value = true;
  panelError.value = '';
  try {
    const nextScopes = await fileManagerApi.listScopes(props.projectId, {
      signal: loadHandle.signal,
    });
    if (!changesLoadController.isCurrent(loadHandle)) {
      return;
    }
    scopes.value = nextScopes;
    const scope = chooseScope(nextScopes, selectedWorktreeId.value, options?.scopeId);
    if (!scope) {
      if (!changesLoadController.isCurrent(loadHandle)) {
        return;
      }
      changesResult.value = null;
      selectedChangePath.value = '';
      clearPreviewState();
      emitSummaryChange();
      return;
    }

    const result = await fileManagerApi.listChanges(
      props.projectId,
      scope.id,
      {
        ...buildGitChangesRequestOptions(ignoreUntracked.value),
        signal: loadHandle.signal,
      }
    );
    if (!changesLoadController.isCurrent(loadHandle)) {
      return;
    }
    activeScopeId.value = result.scope.id;
    changesResult.value = result;
    pendingChangesUpdate.value = false;
    emitSummaryChange();
    await syncSelectionAfterLoad();
  } catch (error) {
    if (!changesLoadController.isCurrent(loadHandle)) {
      return;
    }
    if (error instanceof Error && error.name === 'AbortError') {
      return;
    }
    panelError.value = error instanceof Error ? error.message : t('common.error');
  } finally {
    if (changesLoadController.isCurrent(loadHandle)) {
      panelLoading.value = false;
      changesLoadController.release(loadHandle);
    }
  }
}

async function syncSelectionAfterLoad() {
  const selection = resolveGitChangeSelectionAfterLoad(
    filteredEntries.value,
    selectedChangePath.value
  );
  if (!selection.shouldLoadEntry || !selection.entry) {
    if (selectedChangePath.value !== selection.selectedPath) {
      selectedChangePath.value = selection.selectedPath;
    }
    clearPreviewState();
    return;
  }

  selectedChangePath.value = selection.selectedPath;
  await selectChange(selection.entry, {
    openMobilePreview: !useMobilePreview.value || mobilePreviewVisible.value,
  });
}

function syncSelectionWithFilter() {
  const retainedEntry = resolveRetainedGitChangeEntry(
    filteredEntries.value,
    selectedChangePath.value
  );
  if (retainedEntry) {
    return;
  }
  if (!selectedChangePath.value) {
    return;
  }
  selectedChangePath.value = '';
  clearPreviewState();
}

function clearPreviewState() {
  previewRequestToken += 1;
  previewMode.value = 'file';
  previewResult.value = null;
  previewLoading.value = false;
  previewError.value = '';
  previewFallbackText.value = '';
  mobilePreviewVisible.value = false;
  diffResult.value = null;
  diffLoading.value = false;
  diffError.value = '';
}

function emitSummaryChange() {
  emit('summary-change', buildGitChangesBadgeSummary(changesResult.value, ignoreUntracked.value));
}

function pushMobilePreviewHistoryEntry() {
  if (
    typeof window === 'undefined' ||
    mobilePreviewHistoryActive.value ||
    !useMobilePreview.value
  ) {
    return;
  }
  const nextState =
    window.history.state && typeof window.history.state === 'object'
      ? { ...window.history.state, [MOBILE_PREVIEW_HISTORY_STATE_KEY]: true }
      : { [MOBILE_PREVIEW_HISTORY_STATE_KEY]: true };
  window.history.pushState(nextState, '', window.location.href);
  mobilePreviewHistoryActive.value = true;
}

function handleMobilePreviewPopState() {
  if (!mobilePreviewHistoryActive.value) {
    return;
  }
  mobilePreviewClosingFromHistory.value = true;
  mobilePreviewHistoryActive.value = false;
  mobilePreviewVisible.value = false;
}

function stopRefreshTimer() {
  if (refreshTimer !== null && typeof window !== 'undefined') {
    window.clearInterval(refreshTimer);
  }
  refreshTimer = null;
}

function startRefreshTimer() {
  stopRefreshTimer();
  if (
    typeof window === 'undefined' ||
    !props.isActive ||
    !gitFeaturesAvailable.value ||
    !props.projectId
  ) {
    return;
  }
  refreshTimer = window.setInterval(() => {
    void checkForChangesUpdate();
  }, 10_000);
}

async function reloadChanges() {
  changesUpdateCheckController.cancel();
  await ensureLoaded({
    scopeId: activeScopeId.value,
  });
}

async function checkForChangesUpdate() {
  if (
    !props.projectId ||
    !props.isActive ||
    !activeScopeId.value ||
    !changesResult.value ||
    pendingChangesUpdate.value ||
    panelLoading.value
  ) {
    return;
  }

  const loadHandle = changesUpdateCheckController.begin();
  try {
    const result = await fileManagerApi.listChanges(props.projectId, activeScopeId.value, {
      ...buildGitChangesProbeOptions(ignoreUntracked.value),
      signal: loadHandle.signal,
    });
    if (!changesUpdateCheckController.isCurrent(loadHandle)) {
      return;
    }
    pendingChangesUpdate.value = hasPendingGitChangesUpdate(
      changesResult.value.changeToken,
      result.changeToken
    );
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') {
      return;
    }
  } finally {
    if (changesUpdateCheckController.isCurrent(loadHandle)) {
      changesUpdateCheckController.release(loadHandle);
    }
  }
}

async function handleScopeChange(scopeId: string | null) {
  if (!scopeId) {
    return;
  }
  pendingChangesUpdate.value = false;
  selectedChangePath.value = '';
  clearPreviewState();
  await ensureLoaded({ scopeId });
}

async function selectChange(
  entry: FileManagerChangeEntry,
  options?: {
    openMobilePreview?: boolean;
  }
) {
  selectedChangePath.value = entry.path;
  const requestToken = ++previewRequestToken;
  previewMode.value = resolveInitialChangePreviewMode(entry);
  const shouldOpenMobilePreview = options?.openMobilePreview ?? true;
  if (useMobilePreview.value && shouldOpenMobilePreview) {
    mobilePreviewVisible.value = true;
  }
  previewResult.value = null;
  previewLoading.value = entry.exists;
  previewError.value = '';
  previewFallbackText.value = '';
  diffResult.value = null;
  diffLoading.value = true;
  diffError.value = '';

  const diffPromise = loadDiff(entry, requestToken);
  const previewPromise = entry.exists ? loadPreview(entry, requestToken) : Promise.resolve();
  await Promise.all([diffPromise, previewPromise]);
}

async function loadDiff(entry: FileManagerChangeEntry, requestToken: number) {
  if (!activeScopeId.value) {
    return;
  }
  try {
    const result = await fileManagerApi.diff(props.projectId, activeScopeId.value, entry.path);
    if (requestToken !== previewRequestToken) {
      return;
    }
    diffResult.value = result;
  } catch (error) {
    if (requestToken !== previewRequestToken) {
      return;
    }
    diffResult.value = null;
    diffError.value = error instanceof Error ? error.message : t('common.error');
  } finally {
    if (requestToken === previewRequestToken) {
      diffLoading.value = false;
    }
  }
}

async function loadPreview(entry: FileManagerChangeEntry, requestToken: number) {
  if (!activeScopeId.value) {
    return;
  }
  try {
    const result = await fileManagerApi.preview(props.projectId, activeScopeId.value, entry.path);
    if (requestToken !== previewRequestToken) {
      return;
    }
    previewResult.value = result;
    if (
      result.previewKind === 'binary' &&
      result.entry.size > 0 &&
      result.entry.size <= 64 * 1024
    ) {
      try {
        const response = await fetch(result.inlineUrl);
        if (response.ok) {
          previewFallbackText.value = await response.text();
        }
      } catch {
        previewFallbackText.value = '';
      }
    }
  } catch (error) {
    if (requestToken !== previewRequestToken) {
      return;
    }
    previewResult.value = null;
    previewError.value = error instanceof Error ? error.message : t('common.error');
  } finally {
    if (requestToken === previewRequestToken) {
      previewLoading.value = false;
    }
  }
}

function handlePreviewModeChange(mode: FilePreviewMode) {
  previewMode.value = mode;
}

function handleMobilePreviewVisibilityChange(show: boolean) {
  if (show) {
    mobilePreviewVisible.value = true;
    return;
  }
  closeMobilePreview();
}

function closeMobilePreview() {
  if (!mobilePreviewVisible.value && !mobilePreviewHistoryActive.value) {
    return;
  }
  mobilePreviewVisible.value = false;
}

function formatStatusLabel(status: FileManagerChangeEntry['status']) {
  return t(`fileManager.gitStatus.${status.kind}`);
}

function formatSignedCount(prefix: '+' | '-', value: number | null) {
  return formatGitChangeCount(prefix, value);
}

function formatChangeStat(
  entry: Pick<FileManagerChangeEntry, 'additions' | 'deletions' | 'statsAvailable'>
) {
  return formatGitChangeStat(entry);
}

function buildPreviewMeta(entry: FileManagerChangeEntry | null) {
  if (!entry) {
    return '';
  }
  const parts = [formatStatusLabel(entry.status), formatChangeStat(entry), 'HEAD'];
  if (entry.status.kind === 'renamed' && entry.status.previousPath) {
    parts.push(t('fileManager.renamedFrom', { path: entry.status.previousPath }));
  }
  return parts.filter(Boolean).join(' · ');
}

function entryIcon(entry: FileManagerChangeEntry) {
  switch (entry.previewKind) {
    case 'image':
      return ImageOutline;
    case 'audio':
      return MusicalNotesOutline;
    case 'video':
      return VideocamOutline;
    case 'markdown':
      return DocumentOutline;
    case 'binary':
      return LinkOutline;
    default:
      return DocumentOutline;
  }
}

function downloadSelectedFile() {
  if (!selectedChange.value?.exists || !activeScopeId.value) {
    return;
  }
  fileManagerApi.startBrowserDownload(
    fileManagerApi.buildContentUrl(
      props.projectId,
      activeScopeId.value,
      selectedChange.value.path,
      'attachment'
    )
  );
}

watch(
  () => [props.projectId, selectedWorktreeId.value] as const,
  (current, previous) => {
    if (!previous) {
      return;
    }
    if (current[0] === previous[0] && current[1] === previous[1]) {
      return;
    }
    changesResult.value = null;
    selectedChangePath.value = '';
    pendingChangesUpdate.value = false;
    clearPreviewState();
  }
);

watch(
  () => [props.projectId, props.isActive] as const,
  async ([projectId, isActive]) => {
    if (!projectId || !isActive) {
      stopRefreshTimer();
      changesLoadController.cancel();
      changesUpdateCheckController.cancel();
      panelLoading.value = false;
      return;
    }
    await ensureLoaded();
    startRefreshTimer();
  },
  { immediate: true }
);

watch(
  () => normalizedSearch.value,
  () => {
    syncSelectionWithFilter();
  }
);

watch(
  () => ignoreUntracked.value,
  () => {
    pendingChangesUpdate.value = false;
    syncSelectionWithFilter();
    emitSummaryChange();
    void ensureLoaded({
      scopeId: activeScopeId.value,
    });
  }
);

watch(
  () => useMobilePreview.value,
  useMobile => {
    if (!useMobile) {
      mobilePreviewVisible.value = false;
    }
  }
);

watch(
  () => mobilePreviewVisible.value,
  (visible, wasVisible) => {
    if (visible) {
      pushMobilePreviewHistoryEntry();
      return;
    }
    if (!wasVisible) {
      return;
    }
    if (mobilePreviewClosingFromHistory.value) {
      mobilePreviewClosingFromHistory.value = false;
      return;
    }
    if (mobilePreviewHistoryActive.value && typeof window !== 'undefined') {
      window.history.back();
      return;
    }
    mobilePreviewHistoryActive.value = false;
  }
);

watch(
  () => gitFeaturesAvailable.value,
  enabled => {
    if (!enabled) {
      stopRefreshTimer();
      return;
    }
    startRefreshTimer();
  }
);

onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('popstate', handleMobilePreviewPopState);
  }
});

onBeforeUnmount(() => {
  stopRefreshTimer();
  changesLoadController.cancel();
  changesUpdateCheckController.cancel();
  if (typeof window !== 'undefined') {
    window.removeEventListener('popstate', handleMobilePreviewPopState);
  }
});
</script>

<style scoped>
.git-changes-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  background:
    linear-gradient(180deg, rgba(246, 248, 252, 0.98), rgba(238, 242, 247, 0.96)),
    radial-gradient(circle at top right, rgba(53, 119, 186, 0.12), transparent 32%);
}

.git-changes-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(24, 35, 51, 0.08);
}

.git-changes-scope {
  width: 280px;
  min-width: 220px;
}

.git-changes-search {
  width: min(360px, 100%);
}

.git-changes-summary {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.git-changes-summary-stat {
  font-variant-numeric: tabular-nums;
}

.git-changes-warning {
  margin: 12px 16px 0;
}

.git-changes-warning + .git-changes-warning {
  margin-top: 8px;
}

.git-changes-update-alert__content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}

.git-changes-update-alert__content span {
  min-width: 0;
}

.git-changes-body {
  display: flex;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.git-changes-sidebar {
  flex: 0 0 360px;
  min-width: 280px;
  max-width: 440px;
  border-right: 1px solid rgba(24, 35, 51, 0.08);
  background: rgba(255, 255, 255, 0.72);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.git-changes-list-spin,
.git-changes-list,
.git-changes-items,
.git-changes-preview {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.git-changes-list-spin {
  flex: 1;
}

.git-changes-list-spin :deep(.n-spin-body),
.git-changes-list-spin :deep(.n-spin-container),
.git-changes-list-spin :deep(.n-spin-content) {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

.git-changes-list {
  flex: 1;
  overflow: auto;
  padding: 12px;
  scrollbar-width: thin;
  scrollbar-color: rgba(37, 90, 143, 0.38) transparent;
}

.git-changes-list::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.git-changes-list::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(37, 90, 143, 0.28);
}

.git-changes-empty,
.git-changes-error {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 220px;
}

.git-changes-items {
  gap: 6px;
}

.git-change-row {
  display: flex;
  width: 100%;
  padding: 12px 14px;
  border: 1px solid rgba(24, 35, 51, 0.08);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.92);
  text-align: left;
  cursor: pointer;
  transition:
    transform 120ms ease,
    border-color 120ms ease,
    box-shadow 120ms ease;
}

.git-change-row:hover,
.git-change-row.is-active {
  border-color: rgba(53, 119, 186, 0.32);
  background: rgba(240, 247, 255, 0.9);
  box-shadow: 0 10px 24px rgba(40, 73, 118, 0.08);
}

.git-change-main {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.git-change-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.git-change-icon {
  color: #255a8f;
  flex: 0 0 auto;
}

.git-change-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}

.git-change-status-tag,
.git-change-stat-tag {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
}

.git-change-stat-tag {
  color: rgba(34, 46, 67, 0.78);
}

.git-change-path,
.git-change-previous {
  color: rgba(34, 46, 67, 0.7);
  font-size: 12px;
  line-height: 1.45;
  word-break: break-all;
}

.git-changes-preview {
  flex: 1;
  background: rgba(255, 255, 255, 0.82);
}

.git-changes-mobile-preview-modal {
  width: 100vw;
  max-width: 100vw;
  margin: 0;
}

.git-changes-mobile-preview-surface {
  width: 100vw;
  min-height: min(100vh, 100dvh);
  background: rgba(255, 255, 255, 0.98);
  border-radius: 0;
  overflow: hidden;
}

@media (max-width: 1100px) {
  .git-changes-panel:not(.is-mobile-preview) .git-changes-body {
    flex-direction: column;
  }

  .git-changes-panel:not(.is-mobile-preview) .git-changes-sidebar {
    flex: 0 0 auto;
    width: 100%;
    max-width: none;
    border-right: none;
    border-bottom: 1px solid rgba(24, 35, 51, 0.08);
  }

  .git-changes-panel:not(.is-mobile-preview) .git-changes-list {
    max-height: 40vh;
  }
}

@media (max-width: 820px) {
  .git-changes-panel.is-mobile-preview .git-changes-body {
    display: block;
  }

  .git-changes-panel.is-mobile-preview .git-changes-sidebar {
    width: 100%;
    max-width: none;
    min-width: 0;
    border-right: none;
  }

  .git-changes-scope,
  .git-changes-search {
    width: 100%;
  }

  .git-changes-summary {
    width: 100%;
    flex-wrap: wrap;
  }

  .git-changes-mobile-preview-modal {
    width: 100vw;
    max-width: 100vw;
    margin: 0;
  }

  .git-changes-mobile-preview-surface {
    min-height: 100dvh;
  }
}
</style>
