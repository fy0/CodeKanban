<template>
  <div class="file-manager-panel">
    <div class="file-manager-toolbar">
      <n-select
        class="scope-select"
        :value="activeScope?.id || null"
        :options="scopeOptions"
        :disabled="loading"
        @update:value="handleScopeChange"
      />
      <div class="file-manager-breadcrumbs">
        <span class="file-manager-breadcrumb-label">{{ t('fileManager.currentDirectory') }}</span>
        <n-breadcrumb>
          <n-breadcrumb-item
            v-for="crumb in displayBreadcrumbs"
            :key="crumb.path || 'root'"
            @click="handleNavigate(crumb.path)"
          >
            {{ crumb.name }}
          </n-breadcrumb-item>
        </n-breadcrumb>
      </div>
      <div class="file-manager-toolbar-actions">
        <input
          ref="fileInputRef"
          class="file-upload-input"
          type="file"
          multiple
          @change="handleFileInputChange"
        />
        <n-popover trigger="click" placement="bottom-end" :show-arrow="false">
          <template #trigger>
            <n-button tertiary>
              {{ t('fileManager.displayFields') }}
            </n-button>
          </template>
          <div class="file-display-menu">
            <button
              v-for="option in treeMetaFieldOptions"
              :key="option.key"
              type="button"
              class="file-display-option"
              :class="{ 'is-selected': visibleTreeMetaFields.includes(option.key) }"
              @click="toggleTreeMetaField(option.key)"
            >
              <span class="file-display-option-check">{{
                visibleTreeMetaFields.includes(option.key) ? '✓' : ''
              }}</span>
              <span>{{ option.label }}</span>
            </button>
          </div>
        </n-popover>
        <n-button tertiary @click="handleRefresh" :loading="loading">
          {{ t('fileManager.refresh') }}
        </n-button>
        <n-button tertiary @click="openCreateDirectoryDialog" :disabled="!activeScope">
          {{ t('fileManager.newFolder') }}
        </n-button>
        <n-button type="primary" @click="openFilePicker" :disabled="!activeScope">
          {{ t('fileManager.upload') }}
        </n-button>
      </div>
    </div>

    <div class="file-manager-action-bar">
      <div class="file-search-controls">
        <n-input
          v-model:value="searchKeyword"
          clearable
          class="file-search-input"
          :placeholder="t('fileManager.searchPlaceholder')"
        />
        <n-checkbox v-model:checked="searchUseRegex" size="small">
          {{ t('fileManager.regexSearch') }}
        </n-checkbox>
      </div>
      <div class="file-manager-action-controls">
        <span class="selection-count">{{
          t('fileManager.selectedCount', { count: selectedEntries.length })
        }}</span>
        <n-button
          tertiary
          size="small"
          @click="clearSelection"
          :disabled="selectedEntries.length === 0"
        >
          {{ t('fileManager.clearSelection') }}
        </n-button>
        <n-button
          tertiary
          size="small"
          @click="handleDownloadSelected"
          :disabled="selectedEntries.length === 0"
        >
          {{ t('fileManager.download') }}
        </n-button>
        <n-button
          tertiary
          size="small"
          type="error"
          @click="confirmDeleteSelected"
          :disabled="selectedEntries.length === 0"
        >
          {{ t('fileManager.delete') }}
        </n-button>
        <n-dropdown :options="bulkActionOptions" @select="handleBulkActionSelect">
          <n-button tertiary size="small" :disabled="selectedEntries.length === 0">
            {{ t('fileManager.moreActions') }}
          </n-button>
        </n-dropdown>
      </div>
    </div>

    <div class="file-manager-body">
      <div
        class="file-browser"
        :class="[browserWidthClass, { 'is-drag-over': isDragOver }]"
        :data-drop-label="t('fileManager.dropFiles')"
        @dragenter.prevent="handleDragEnter"
        @dragover.prevent="handleDragOver"
        @dragleave.prevent="handleDragLeave"
        @drop.prevent="handleDrop"
      >
        <n-spin :show="loading || searchLoading" class="file-list-spin">
          <div class="file-list-scroll">
            <div v-if="displayErrorMessage" class="file-manager-error">
              <n-alert type="error" :show-icon="false">{{ displayErrorMessage }}</n-alert>
            </div>
            <div v-else-if="displayTreeNodes.length === 0" class="file-manager-empty">
              <n-empty :description="t('fileManager.empty')" />
            </div>
            <div v-else class="file-tree">
              <n-alert
                v-if="isSearchActive && searchResult?.truncated"
                class="file-search-warning"
                type="warning"
                :show-icon="false"
              >
                {{ t('fileManager.searchResultTruncated') }}
              </n-alert>
              <button
                v-for="node in displayTreeNodes"
                :key="node.path"
                type="button"
                class="file-tree-row"
                :class="{ 'is-active': isTreeNodeActive(node) }"
                @click="handleTreeNodeClick(node)"
              >
                <label class="file-list-checkbox" @click.stop>
                  <input
                    type="checkbox"
                    :checked="selectedPaths.includes(node.path)"
                    @change="event => handleEntryCheckboxChange(node.path, event)"
                  />
                </label>
                <span class="file-tree-main">
                  <span
                    class="file-name-cell tree-name-cell"
                    :style="{ paddingLeft: `${node.depth * 18 + 8}px` }"
                  >
                    <span
                      class="tree-expand-hit"
                      :class="{ 'is-placeholder': !node.isDirectory }"
                      @click.stop="toggleTreeNode(node)"
                    >
                      <span v-if="node.isDirectory && !isSearchActive">{{
                        node.expanded ? '▾' : '▸'
                      }}</span>
                    </span>
                    <n-icon size="18" class="file-kind-icon">
                      <component :is="entryIcon(node.entry)" />
                    </n-icon>
                    <span class="file-name-text">{{ node.entry.name }}</span>
                    <n-tag v-if="node.entry.hidden" size="small" round :bordered="false">
                      {{ t('fileManager.hidden') }}
                    </n-tag>
                  </span>
                  <span v-if="buildTreeMeta(node.entry, node.path)" class="file-row-meta">{{
                    buildTreeMeta(node.entry, node.path)
                  }}</span>
                </span>
              </button>
            </div>
          </div>
        </n-spin>
      </div>

      <aside v-if="!useMobilePreview" class="file-preview-pane">
        <FilePreviewSurface
          :preview-result="previewResult"
          :preview-loading="previewLoading"
          :preview-error="previewError"
          :preview-fallback-text="previewFallbackText"
          :rendered-markdown="renderedMarkdown"
          :rendered-diff="''"
          :preview-meta="previewMetaText"
          :empty-label="t('fileManager.previewEmpty')"
          :binary-preview-hint="t('fileManager.binaryPreviewHint')"
          :preview-truncated-label="t('fileManager.previewTruncated')"
          :download-label="t('fileManager.download')"
          preview-mode="file"
          :show-diff-toggle="false"
          :file-label="t('fileManager.fileView')"
          :diff-label="t('fileManager.diffView')"
          :diff-result="null"
          :diff-loading="false"
          diff-error=""
          diff-unavailable-text=""
          @download="downloadPreviewItem"
          @image-preview="openImagePreviewModal"
        />
      </aside>
    </div>

    <n-modal
      :show="mobilePreviewVisible"
      preset="card"
      class="file-mobile-preview-modal"
      :bordered="false"
      :closable="false"
      @update:show="handleMobilePreviewVisibilityChange"
    >
      <FilePreviewSurface
        class="file-mobile-preview-surface"
        :preview-result="previewResult"
        :preview-loading="previewLoading"
        :preview-error="previewError"
        :preview-fallback-text="previewFallbackText"
        :rendered-markdown="renderedMarkdown"
        :rendered-diff="''"
        :preview-meta="previewMetaText"
        :empty-label="t('fileManager.previewEmpty')"
        :binary-preview-hint="t('fileManager.binaryPreviewHint')"
        :preview-truncated-label="t('fileManager.previewTruncated')"
        :download-label="t('fileManager.download')"
        preview-mode="file"
        :show-diff-toggle="false"
        :file-label="t('fileManager.fileView')"
        :diff-label="t('fileManager.diffView')"
        :diff-result="null"
        :diff-loading="false"
        diff-error=""
        diff-unavailable-text=""
        :back-label="t('common.back')"
        :fallback-title="t('fileManager.previewTitle')"
        :mobile="true"
        :show-back-button="true"
        @close="closeMobilePreview"
        @download="downloadPreviewItem"
        @image-preview="openImagePreviewModal"
      />
    </n-modal>

    <n-modal
      v-model:show="imagePreviewVisible"
      preset="card"
      class="file-image-modal"
      :bordered="false"
    >
      <template #header>
        <span class="file-image-modal-title">{{
          previewResult?.entry.name || t('fileManager.previewKind.image')
        }}</span>
      </template>
      <img
        v-if="previewResult?.previewKind === 'image'"
        :src="previewResult.inlineUrl"
        :alt="previewResult.entry.name"
        class="file-image-modal-image"
      />
    </n-modal>

    <div v-if="projectTasks.length" class="file-transfer-queue">
      <div class="file-transfer-queue-header">
        <span>{{ t('fileManager.transferQueue') }}</span>
        <n-button text size="small" @click="fileManagerStore.clearFinishedTasks(props.projectId)">
          {{ t('fileManager.clearFinished') }}
        </n-button>
      </div>
      <div class="file-transfer-items">
        <div v-for="task in projectTasks" :key="task.id" class="file-transfer-item">
          <div class="file-transfer-main">
            <div class="file-transfer-name">{{ task.name }}</div>
            <div class="file-transfer-meta">
              {{ t(`fileManager.transferDirection.${task.direction}`) }}
              ·
              {{ formatTransferStatus(task) }}
              <span v-if="task.total"
                >· {{ formatBytes(task.loaded) }} / {{ formatBytes(task.total) }}</span
              >
              <span v-else>· {{ formatBytes(task.loaded) }}</span>
              <span v-if="task.speed > 0">· {{ formatSpeed(task.speed) }}</span>
            </div>
            <n-progress
              :type="'line'"
              :percentage="task.progress ?? 0"
              :show-indicator="false"
              :status="resolveTransferProgressStatus(task)"
            />
            <div v-if="task.error" class="file-transfer-error">{{ task.error }}</div>
          </div>
          <div class="file-transfer-actions">
            <n-button
              v-if="task.direction === 'upload' && task.status === 'running'"
              tertiary
              size="small"
              @click="fileManagerStore.pauseTask(task.id)"
            >
              {{ t('fileManager.pause') }}
            </n-button>
            <n-button
              v-if="task.direction === 'upload' && task.status === 'paused'"
              tertiary
              size="small"
              @click="fileManagerStore.resumeTask(task.id)"
            >
              {{ t('fileManager.resume') }}
            </n-button>
            <n-button
              v-if="
                task.status === 'queued' || task.status === 'running' || task.status === 'paused'
              "
              tertiary
              size="small"
              @click="fileManagerStore.cancelTask(task.id)"
            >
              {{ t('common.cancel') }}
            </n-button>
            <n-button
              v-if="task.status === 'failed' || task.status === 'canceled'"
              tertiary
              size="small"
              @click="fileManagerStore.retryTask(task.id)"
            >
              {{ t('fileManager.retry') }}
            </n-button>
            <n-button
              v-if="
                task.status === 'completed' ||
                task.status === 'failed' ||
                task.status === 'canceled'
              "
              tertiary
              size="small"
              @click="fileManagerStore.removeTask(task.id)"
            >
              {{ t('common.close') }}
            </n-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useDialog, useMessage, NInput } from 'naive-ui';
import {
  DocumentOutline,
  FolderOpenOutline,
  ImageOutline,
  LinkOutline,
  MusicalNotesOutline,
  VideocamOutline,
} from '@vicons/ionicons5';
import { storeToRefs } from 'pinia';
import { useLocale } from '@/composables/useLocale';
import { useResponsive } from '@/composables/useResponsive';
import { useProjectStore } from '@/stores/project';
import { useFileManagerStore, type FileManagerOpenRequest } from '@/stores/fileManager';
import { fileManagerApi } from '@/api/fileManager';
import { renderMarkdown } from '@/utils/markdown';
import FilePreviewSurface from '@/components/files/FilePreviewSurface.vue';
import type {
  FileManagerEntry,
  FileManagerListResult,
  FileManagerPreviewResult,
  FileManagerSearchResult,
  FileTransferTask,
} from '@/types/fileManager';

const MOBILE_PREVIEW_MAX_WIDTH = 900;
const MOBILE_PREVIEW_HISTORY_STATE_KEY = '__codekanbanFilePreview';
const SEARCH_DEBOUNCE_MS = 300;

const props = withDefaults(
  defineProps<{
    projectId: string;
    isActive?: boolean;
  }>(),
  {
    isActive: true,
  }
);

const message = useMessage();
const dialog = useDialog();
const projectStore = useProjectStore();
const fileManagerStore = useFileManagerStore();
const { selectedWorktreeId } = storeToRefs(projectStore);
const { t } = useLocale();
const { windowWidth } = useResponsive();

const fileInputRef = ref<HTMLInputElement | null>(null);
const selectedPaths = ref<string[]>([]);
const previewResult = ref<FileManagerPreviewResult | null>(null);
const previewLoading = ref(false);
const previewError = ref('');
const previewFallbackText = ref('');
const previewPath = ref('');
const mobilePreviewVisible = ref(false);
const mobilePreviewHistoryActive = ref(false);
const mobilePreviewClosingFromHistory = ref(false);
const imagePreviewVisible = ref(false);
const searchKeyword = ref('');
const searchUseRegex = ref(false);
const searchResult = ref<FileManagerSearchResult | null>(null);
const searchLoading = ref(false);
const searchErrorMessage = ref('');
const isDragOver = ref(false);
let dragDepth = 0;
let previewRequestToken = 0;
let searchRequestToken = 0;
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

const treeMetaFieldOrder = ['type', 'size', 'modifiedAt'] as const;

type TreeMetaField = (typeof treeMetaFieldOrder)[number];

type TreeNodeState = {
  path: string;
  entry: FileManagerEntry;
  isDirectory: boolean;
  expanded: boolean;
  loaded: boolean;
  loading: boolean;
  children: string[];
};

type VisibleTreeNode = {
  path: string;
  depth: number;
  entry: FileManagerEntry;
  isDirectory: boolean;
  expanded: boolean;
};

const treeNodeMap = ref<Record<string, TreeNodeState>>({});
const treeRoots = ref<string[]>([]);

const listResult = computed(() => fileManagerStore.getList(props.projectId));
const activeScope = computed(() => fileManagerStore.getActiveScope(props.projectId));
const scopes = computed(() => fileManagerStore.getScopes(props.projectId));
const loading = computed(() => fileManagerStore.getLoading(props.projectId));
const errorMessage = computed(() => fileManagerStore.getError(props.projectId));
const breadcrumbs = computed(() => listResult.value?.breadcrumbs ?? []);
const displayBreadcrumbs = computed(() =>
  breadcrumbs.value.map((crumb, index) =>
    index === 0
      ? {
          ...crumb,
          name: t('fileManager.rootLabel'),
        }
      : crumb
  )
);
const currentPath = computed(() => listResult.value?.currentPath ?? '');
const projectTasks = computed(() => fileManagerStore.getTransferTasks(props.projectId));
const scopeOptions = computed(() =>
  scopes.value.map(scope => ({
    label: `${scope.label} · ${scope.rootPath}`,
    value: scope.id,
  }))
);

const normalizedSearch = computed(() => searchKeyword.value.trim().toLowerCase());
const selectedEntries = computed(() =>
  selectedPaths.value
    .map(
      path =>
        treeNodeMap.value[path]?.entry ??
        searchResult.value?.entries.find(entry => entry.path === path)
    )
    .filter((entry): entry is FileManagerEntry => Boolean(entry))
);
const visibleTreeMetaFields = ref<TreeMetaField[]>([...treeMetaFieldOrder]);
const treeMetaFieldOptions = computed(() =>
  treeMetaFieldOrder.map(key => ({
    key,
    label: t(`fileManager.columns.${key}`),
  }))
);
const browserWidthClass = computed(
  () => `file-browser--meta-${visibleTreeMetaFields.value.length}`
);
const isSearchActive = computed(() => normalizedSearch.value.length > 0);
const displayErrorMessage = computed(() => searchErrorMessage.value || errorMessage.value);
const bulkActionOptions = computed(() => [
  {
    label: t('fileManager.zipDownload'),
    key: 'zip-download',
  },
  {
    label: t('fileManager.copyTo'),
    key: 'copy',
  },
  {
    label: t('fileManager.moveTo'),
    key: 'move',
  },
  {
    label: t('fileManager.rename'),
    key: 'rename',
    disabled: selectedEntries.value.length !== 1,
  },
]);

const renderedMarkdown = computed(() =>
  previewResult.value?.previewKind === 'markdown'
    ? renderMarkdown(previewResult.value.textContent ?? '')
    : ''
);
const previewMetaText = computed(() =>
  previewResult.value ? buildPreviewMeta(previewResult.value.entry) : ''
);
const useMobilePreview = computed(() => windowWidth.value <= MOBILE_PREVIEW_MAX_WIDTH);

const visibleTreeNodes = computed<VisibleTreeNode[]>(() => {
  const output: VisibleTreeNode[] = [];
  const keyword = normalizedSearch.value;
  const visit = (path: string, depth: number): boolean => {
    const node = treeNodeMap.value[path];
    if (!node) {
      return false;
    }
    const nameMatched = keyword.length === 0 || node.entry.name.toLowerCase().includes(keyword);
    const childStartIndex = output.length;
    if (nameMatched) {
      output.push({
        path: node.path,
        depth,
        entry: node.entry,
        isDirectory: node.isDirectory,
        expanded: node.expanded,
      });
    }
    let childMatched = false;
    if (node.isDirectory && node.expanded) {
      for (const childPath of node.children) {
        childMatched = visit(childPath, depth + 1) || childMatched;
      }
    }
    const matched = nameMatched || childMatched;
    if (!nameMatched && childMatched) {
      const children = output.splice(childStartIndex);
      output.push({
        path: node.path,
        depth,
        entry: node.entry,
        isDirectory: node.isDirectory,
        expanded: node.expanded,
      });
      output.push(...children);
    }
    return matched;
  };
  for (const rootPath of treeRoots.value) {
    visit(rootPath, 0);
  }
  return output;
});

const searchTreeNodes = computed<VisibleTreeNode[]>(() =>
  (searchResult.value?.entries ?? []).map(entry => ({
    path: entry.path,
    depth: 0,
    entry,
    isDirectory: entry.kind === 'directory',
    expanded: false,
  }))
);

const displayTreeNodes = computed<VisibleTreeNode[]>(() =>
  isSearchActive.value ? searchTreeNodes.value : visibleTreeNodes.value
);

function upsertTreeNodes(entriesToSync: FileManagerEntry[], parentPath = '') {
  const parentKey = parentPath;
  if (parentKey) {
    const parentNode = treeNodeMap.value[parentKey];
    if (parentNode) {
      parentNode.children = entriesToSync.map(item => item.path);
      parentNode.loaded = true;
      parentNode.loading = false;
    }
  } else {
    treeRoots.value = entriesToSync.map(item => item.path);
  }
  for (const entry of entriesToSync) {
    const current = treeNodeMap.value[entry.path];
    treeNodeMap.value[entry.path] = {
      path: entry.path,
      entry,
      isDirectory: entry.kind === 'directory',
      expanded: current?.expanded ?? false,
      loaded: current?.loaded ?? false,
      loading: false,
      children: current?.children ?? [],
    };
  }
}

function syncTreeFromList(result: FileManagerListResult | null) {
  if (!result) {
    treeNodeMap.value = {};
    treeRoots.value = [];
    return;
  }
  if (result.currentPath && !treeNodeMap.value[result.currentPath]) {
    treeNodeMap.value[result.currentPath] = {
      path: result.currentPath,
      entry: {
        name: result.currentPath.split('/').pop() || '/',
        path: result.currentPath,
        kind: 'directory',
        size: 0,
        modifiedAt: '',
        previewKind: 'binary',
        hidden: false,
      },
      isDirectory: true,
      expanded: true,
      loaded: false,
      loading: false,
      children: [],
    };
  }
  if (result.currentPath && treeRoots.value.length === 0) {
    treeRoots.value = [result.currentPath];
  }
  upsertTreeNodes(result.entries, result.currentPath);
}

function clearPreviewState() {
  previewRequestToken += 1;
  previewPath.value = '';
  previewResult.value = null;
  previewLoading.value = false;
  previewError.value = '';
  previewFallbackText.value = '';
  mobilePreviewVisible.value = false;
  imagePreviewVisible.value = false;
}

function clearSearchState() {
  searchRequestToken += 1;
  searchResult.value = null;
  searchErrorMessage.value = '';
  searchLoading.value = false;
  selectedPaths.value = selectedPaths.value.filter(path => Boolean(treeNodeMap.value[path]));
  if (searchDebounceTimer) {
    clearTimeout(searchDebounceTimer);
    searchDebounceTimer = null;
  }
}

async function runSearch() {
  const keyword = searchKeyword.value.trim();
  if (!keyword || !activeScope.value) {
    clearSearchState();
    return;
  }

  const requestToken = ++searchRequestToken;
  searchLoading.value = true;
  searchErrorMessage.value = '';
  try {
    const result = await fileManagerApi.search(
      props.projectId,
      activeScope.value.id,
      currentPath.value,
      keyword,
      searchUseRegex.value
    );
    if (requestToken !== searchRequestToken) {
      return;
    }
    searchResult.value = result;
  } catch (error) {
    if (requestToken !== searchRequestToken) {
      return;
    }
    searchResult.value = null;
    searchErrorMessage.value = error instanceof Error ? error.message : t('common.error');
  } finally {
    if (requestToken === searchRequestToken) {
      searchLoading.value = false;
    }
  }
}

function scheduleSearch() {
  if (searchDebounceTimer) {
    clearTimeout(searchDebounceTimer);
  }
  if (!searchKeyword.value.trim()) {
    clearSearchState();
    return;
  }
  searchDebounceTimer = setTimeout(() => {
    searchDebounceTimer = null;
    void runSearch();
  }, SEARCH_DEBOUNCE_MS);
}

async function ensureLoaded() {
  if (!props.projectId || !props.isActive) {
    return;
  }
  await fileManagerStore.loadDirectory(props.projectId, {
    preferredWorktreeId: selectedWorktreeId.value ?? undefined,
  });
  syncTreeFromList(listResult.value);
}

async function handleRefresh() {
  try {
    const result = await fileManagerStore.refreshProject(props.projectId);
    syncTreeFromList(result);
    if (isSearchActive.value) {
      void runSearch();
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleNavigate(path: string) {
  if (!activeScope.value) {
    return;
  }
  try {
    const result = await fileManagerStore.loadDirectory(props.projectId, {
      scopeId: activeScope.value.id,
      path,
    });
    syncTreeFromList(result);
    selectedPaths.value = [];
    clearPreviewState();
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleScopeChange(scopeId: string | null) {
  if (!scopeId) {
    return;
  }
  try {
    treeNodeMap.value = {};
    treeRoots.value = [];
    const result = await fileManagerStore.loadDirectory(props.projectId, {
      scopeId,
      path: '',
    });
    syncTreeFromList(result);
    selectedPaths.value = [];
    clearPreviewState();
    clearSearchState();
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

function toggleSelection(path: string, checked: boolean) {
  if (checked) {
    selectedPaths.value = Array.from(new Set([...selectedPaths.value, path]));
    return;
  }
  selectedPaths.value = selectedPaths.value.filter(item => item !== path);
}

function handleEntryCheckboxChange(path: string, event: Event) {
  toggleSelection(path, (event.target as HTMLInputElement | null)?.checked === true);
}

function clearSelection() {
  selectedPaths.value = [];
}

function handleBulkActionSelect(key: string) {
  switch (key) {
    case 'zip-download':
      void handleZipDownloadSelected();
      break;
    case 'copy':
      openCopyDialog();
      break;
    case 'move':
      openMoveDialog();
      break;
    case 'rename':
      openRenameDialog();
      break;
    default:
      break;
  }
}

function toggleTreeMetaField(field: TreeMetaField) {
  if (visibleTreeMetaFields.value.includes(field)) {
    visibleTreeMetaFields.value = visibleTreeMetaFields.value.filter(item => item !== field);
    return;
  }
  visibleTreeMetaFields.value = treeMetaFieldOrder.filter(item =>
    [...visibleTreeMetaFields.value, field].includes(item)
  );
}

async function openFilePath(path: string, scopeId: string) {
  if (!scopeId || !path) {
    return;
  }
  if (useMobilePreview.value) {
    mobilePreviewVisible.value = true;
  }
  const requestToken = ++previewRequestToken;
  const targetDirectoryPath = parentDirectoryPath(path);
  const scopeChanged = activeScope.value?.id !== scopeId;
  if (scopeChanged) {
    treeNodeMap.value = {};
    treeRoots.value = [];
    clearSearchState();
  }
  previewPath.value = scopeChanged ? '' : path;
  previewResult.value = null;
  previewLoading.value = true;
  previewError.value = '';
  previewFallbackText.value = '';
  try {
    if (scopeChanged || targetDirectoryPath !== currentPath.value) {
      const result = await fileManagerStore.loadDirectory(props.projectId, {
        scopeId,
        path: targetDirectoryPath,
      });
      if (requestToken !== previewRequestToken) {
        return;
      }
      syncTreeFromList(result);
    }
    previewPath.value = path;
    const result = await fileManagerApi.preview(props.projectId, scopeId, path);
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

async function handleRowClick(entry: FileManagerEntry) {
  if (entry.kind === 'directory' || !activeScope.value) {
    return;
  }
  await openFilePath(entry.path, activeScope.value.id);
}

async function openRequestedFile(request: FileManagerOpenRequest) {
  selectedPaths.value = [];
  if (!request.path) {
    const result = await fileManagerStore.loadDirectory(props.projectId, {
      scopeId: request.scopeId,
      path: '',
    });
    syncTreeFromList(result);
    clearPreviewState();
    clearSearchState();
    return;
  }
  await openFilePath(request.path, request.scopeId);
}

async function toggleTreeNode(node: VisibleTreeNode) {
  if (!node.isDirectory || isSearchActive.value) {
    return;
  }
  const state = treeNodeMap.value[node.path];
  if (!state) {
    return;
  }
  state.expanded = !state.expanded;
  if (!state.expanded || state.loaded || state.loading || !activeScope.value) {
    return;
  }
  state.loading = true;
  try {
    const result = await fileManagerApi.list(props.projectId, activeScope.value.id, state.path);
    upsertTreeNodes(result.entries, state.path);
  } catch {
    state.loading = false;
  }
}

async function handleTreeNodeClick(node: VisibleTreeNode) {
  if (node.isDirectory) {
    if (isSearchActive.value) {
      await handleNavigate(node.path);
      return;
    }
    if (currentPath.value === node.path) {
      const state = treeNodeMap.value[node.path];
      if (state) {
        state.expanded = false;
      }
      await handleNavigate(parentDirectoryPath(node.path));
      return;
    }
    const state = treeNodeMap.value[node.path];
    if (state) {
      state.expanded = true;
      state.loading = false;
    }
    await handleNavigate(node.path);
    return;
  }
  await handleRowClick(node.entry);
}

function isTreeNodeActive(node: VisibleTreeNode) {
  if (node.isDirectory) {
    if (previewPath.value) {
      return false;
    }
    return currentPath.value === node.path;
  }
  return previewPath.value === node.path;
}

function parentDirectoryPath(path: string) {
  const segments = path.split('/').filter(Boolean);
  segments.pop();
  return segments.join('/');
}

function entryIcon(entry: FileManagerEntry) {
  if (entry.kind === 'directory') {
    return FolderOpenOutline;
  }
  if (entry.kind === 'symlink') {
    return LinkOutline;
  }
  switch (entry.previewKind) {
    case 'image':
      return ImageOutline;
    case 'markdown':
      return DocumentOutline;
    case 'audio':
      return MusicalNotesOutline;
    case 'video':
      return VideocamOutline;
    default:
      return DocumentOutline;
  }
}

function formatBytes(value?: number) {
  const bytes = Number(value ?? 0);
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let normalized = bytes;
  let unitIndex = 0;
  while (normalized >= 1024 && unitIndex < units.length - 1) {
    normalized /= 1024;
    unitIndex += 1;
  }
  const digits = normalized >= 100 ? 0 : normalized >= 10 ? 1 : 2;
  return `${normalized.toFixed(digits)} ${units[unitIndex]}`;
}

function formatSpeed(value?: number) {
  return `${formatBytes(value)}/s`;
}

function formatTransferStatus(task: FileTransferTask) {
  if (task.status === 'running' && task.retryAttempt && task.retryMaxAttempts) {
    return t('fileManager.autoRetryProgress', {
      attempt: task.retryAttempt,
      max: task.retryMaxAttempts,
    });
  }
  return t(`fileManager.transferStatus.${task.status}`);
}

function resolveTransferProgressStatus(task: FileTransferTask) {
  if (task.status === 'failed') {
    return 'error' as const;
  }
  if (task.status === 'completed') {
    return 'success' as const;
  }
  return 'default' as const;
}

function formatEntrySize(entry: FileManagerEntry) {
  if (entry.kind === 'directory') {
    return t('fileManager.folderLabel');
  }
  if (entry.kind === 'symlink') {
    return t('fileManager.symlinkLabel');
  }
  return formatBytes(entry.size);
}

function formatTimestamp(value: string) {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');
  const seconds = String(date.getSeconds()).padStart(2, '0');
  return `${year}/${month}/${day} ${hours}:${minutes}:${seconds}`;
}

function formatEntryType(entry: FileManagerEntry) {
  if (entry.kind === 'directory') {
    return t('fileManager.folderLabel');
  }
  if (entry.kind === 'symlink') {
    return t('fileManager.symlinkLabel');
  }
  return t(`fileManager.previewKind.${entry.previewKind}`);
}

function resolveTreeMetaValue(entry: FileManagerEntry, field: TreeMetaField) {
  switch (field) {
    case 'type':
      return formatEntryType(entry);
    case 'size':
      return entry.kind === 'file' ? formatBytes(entry.size) : '';
    case 'modifiedAt': {
      const timestamp = formatTimestamp(entry.modifiedAt);
      return timestamp === '-' ? '' : timestamp;
    }
    default:
      return '';
  }
}

function buildTreeMeta(entry: FileManagerEntry, path = '') {
  const parts = visibleTreeMetaFields.value
    .map(field => resolveTreeMetaValue(entry, field))
    .filter(Boolean);
  if (isSearchActive.value && path) {
    parts.unshift(path);
  }
  return parts.join(' · ');
}

function buildPreviewMeta(entry: FileManagerEntry) {
  const parts = [formatEntrySize(entry)];
  if (entry.mime) {
    parts.push(entry.mime);
  }
  const timestamp = formatTimestamp(entry.modifiedAt);
  if (timestamp !== '-') {
    parts.push(timestamp);
  }
  return parts.filter(Boolean).join(' · ');
}

function openImagePreviewModal() {
  if (previewResult.value?.previewKind !== 'image') {
    return;
  }
  imagePreviewVisible.value = true;
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
  imagePreviewVisible.value = false;
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
  if (useMobilePreview.value) {
    imagePreviewVisible.value = false;
  }
}

function openFilePicker() {
  fileInputRef.value?.click();
}

async function queueFiles(files: File[]) {
  if (!activeScope.value) {
    return;
  }
  const uploadable = files.filter(file => file.size > 0);
  if (uploadable.length === 0) {
    return;
  }
  await fileManagerStore.enqueueUploads(
    props.projectId,
    activeScope.value.id,
    currentPath.value,
    uploadable
  );
}

async function handleFileInputChange(event: Event) {
  const target = event.target as HTMLInputElement | null;
  const files = Array.from(target?.files ?? []);
  try {
    await queueFiles(files);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    if (target) {
      target.value = '';
    }
  }
}

function handleDragEnter(event: DragEvent) {
  if (!(event.dataTransfer?.files?.length ?? 0)) {
    return;
  }
  dragDepth += 1;
  isDragOver.value = true;
}

function handleDragOver(event: DragEvent) {
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'copy';
  }
  isDragOver.value = true;
}

function handleDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) {
    isDragOver.value = false;
  }
}

async function handleDrop(event: DragEvent) {
  dragDepth = 0;
  isDragOver.value = false;
  const files = Array.from(event.dataTransfer?.files ?? []);
  if (files.length === 0) {
    return;
  }
  try {
    await queueFiles(files);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

function promptForText(
  title: string,
  placeholder: string,
  initialValue = '',
  onConfirm?: (value: string) => Promise<void> | void
) {
  const inputValue = ref(initialValue);
  dialog.create({
    title,
    content: () =>
      h(NInput, {
        value: inputValue.value,
        autofocus: true,
        placeholder,
        'onUpdate:value': (value: string) => {
          inputValue.value = value;
        },
      }),
    positiveText: t('common.confirm'),
    negativeText: t('common.cancel'),
    showIcon: false,
    onPositiveClick: async () => {
      const value = inputValue.value.trim();
      if (!value) {
        message.warning(t('fileManager.inputRequired'));
        return false;
      }
      try {
        await onConfirm?.(value);
        return true;
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('common.error'));
        return false;
      }
    },
  });
}

function openCreateDirectoryDialog() {
  if (!activeScope.value) {
    return;
  }
  promptForText(
    t('fileManager.newFolder'),
    t('fileManager.folderNamePlaceholder'),
    '',
    async value => {
      await fileManagerStore.createDirectory(
        props.projectId,
        activeScope.value!.id,
        currentPath.value,
        value
      );
      if (isSearchActive.value) {
        void runSearch();
      }
    }
  );
}

function openRenameDialog() {
  const entry = selectedEntries.value[0];
  if (!activeScope.value || !entry) {
    return;
  }
  promptForText(
    t('fileManager.rename'),
    t('fileManager.renamePlaceholder'),
    entry.name,
    async value => {
      await fileManagerStore.renameEntry(props.projectId, activeScope.value!.id, entry.path, value);
      selectedPaths.value = [];
      if (isSearchActive.value) {
        void runSearch();
      }
    }
  );
}

function openCopyDialog() {
  if (!activeScope.value || selectedEntries.value.length === 0) {
    return;
  }
  promptForText(
    t('fileManager.copyTo'),
    t('fileManager.destinationPlaceholder'),
    currentPath.value,
    async value => {
      const result = await fileManagerStore.copyEntries(
        props.projectId,
        activeScope.value!.id,
        selectedEntries.value.map(entry => entry.path),
        value
      );
      if (result.failed.length > 0) {
        message.warning(result.failed[0]?.message || t('common.warning'));
      } else {
        message.success(t('fileManager.copySuccess'));
      }
      if (isSearchActive.value) {
        void runSearch();
      }
    }
  );
}

function openMoveDialog() {
  if (!activeScope.value || selectedEntries.value.length === 0) {
    return;
  }
  promptForText(
    t('fileManager.moveTo'),
    t('fileManager.destinationPlaceholder'),
    currentPath.value,
    async value => {
      const result = await fileManagerStore.moveEntries(
        props.projectId,
        activeScope.value!.id,
        selectedEntries.value.map(entry => entry.path),
        value
      );
      if (result.failed.length > 0) {
        message.warning(result.failed[0]?.message || t('common.warning'));
      } else {
        selectedPaths.value = [];
        message.success(t('fileManager.moveSuccess'));
      }
      if (isSearchActive.value) {
        void runSearch();
      }
    }
  );
}

function confirmDeleteSelected() {
  if (!activeScope.value || selectedEntries.value.length === 0) {
    return;
  }
  dialog.warning({
    title: t('fileManager.deleteConfirmTitle'),
    content: t('fileManager.deleteConfirmText', { count: selectedEntries.value.length }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      const result = await fileManagerStore.deleteEntries(
        props.projectId,
        activeScope.value!.id,
        selectedEntries.value.map(entry => entry.path)
      );
      selectedPaths.value = [];
      if (result.failed.length > 0) {
        message.warning(result.failed[0]?.message || t('common.warning'));
      } else {
        message.success(t('fileManager.deleteSuccess'));
      }
      if (isSearchActive.value) {
        void runSearch();
      }
    },
  });
}

async function handleDownloadSelected() {
  if (!activeScope.value || selectedEntries.value.length === 0) {
    return;
  }
  try {
    await fileManagerStore.enqueueDownloads(
      props.projectId,
      activeScope.value.id,
      currentPath.value,
      selectedEntries.value
    );
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleZipDownloadSelected() {
  if (!activeScope.value || selectedEntries.value.length === 0) {
    return;
  }
  try {
    await fileManagerStore.enqueueDownloads(
      props.projectId,
      activeScope.value.id,
      currentPath.value,
      selectedEntries.value,
      { forceArchive: true }
    );
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function downloadPreviewItem() {
  if (!previewResult.value || !activeScope.value) {
    return;
  }
  try {
    await fileManagerStore.enqueueDownloads(
      props.projectId,
      activeScope.value.id,
      currentPath.value,
      [previewResult.value.entry]
    );
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

watch(
  () => [props.projectId, props.isActive, selectedWorktreeId.value] as const,
  async ([projectId, isActive]) => {
    if (!projectId || !isActive) {
      return;
    }
    if (fileManagerStore.getOpenRequest(projectId)) {
      return;
    }
    await ensureLoaded();
  },
  { immediate: true }
);

watch(
  () =>
    [props.projectId, props.isActive, fileManagerStore.getOpenRequest(props.projectId)] as const,
  async ([projectId, isActive, request]) => {
    if (!projectId || !isActive || !request) {
      return;
    }
    try {
      await openRequestedFile(request);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('common.error'));
    } finally {
      fileManagerStore.consumeFileOpenRequest(projectId, request.id);
    }
  },
  { immediate: true }
);

watch(
  () => listResult.value,
  result => {
    syncTreeFromList(result);
  }
);

watch(
  () => Object.keys(treeNodeMap.value).join('|'),
  () => {
    const searchablePaths = new Set((searchResult.value?.entries ?? []).map(entry => entry.path));
    selectedPaths.value = selectedPaths.value.filter(
      path => Boolean(treeNodeMap.value[path]) || searchablePaths.has(path)
    );
    if (previewPath.value && !treeNodeMap.value[previewPath.value]) {
      clearPreviewState();
    }
  }
);

watch(
  () => (searchResult.value?.entries ?? []).map(entry => entry.path).join('|'),
  () => {
    if (!isSearchActive.value) {
      return;
    }
    const searchablePaths = new Set((searchResult.value?.entries ?? []).map(entry => entry.path));
    selectedPaths.value = selectedPaths.value.filter(
      path => Boolean(treeNodeMap.value[path]) || searchablePaths.has(path)
    );
    if (
      previewPath.value &&
      !treeNodeMap.value[previewPath.value] &&
      !searchablePaths.has(previewPath.value)
    ) {
      clearPreviewState();
    }
  }
);

watch(
  () =>
    [
      normalizedSearch.value,
      searchUseRegex.value,
      activeScope.value?.id ?? '',
      currentPath.value,
    ] as const,
  () => {
    scheduleSearch();
  }
);

watch(
  () => previewResult.value?.previewKind,
  previewKind => {
    if (previewKind !== 'image') {
      imagePreviewVisible.value = false;
    }
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

onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('popstate', handleMobilePreviewPopState);
  }
  void nextTick(async () => {
    await ensureLoaded();
  });
});

onBeforeUnmount(() => {
  clearSearchState();
  if (typeof window !== 'undefined') {
    window.removeEventListener('popstate', handleMobilePreviewPopState);
  }
});
</script>

<style scoped>
.file-manager-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  background:
    linear-gradient(180deg, rgba(246, 248, 252, 0.98), rgba(238, 242, 247, 0.96)),
    radial-gradient(circle at top right, rgba(53, 119, 186, 0.12), transparent 30%);
}

.file-manager-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(24, 35, 51, 0.08);
}

.scope-select {
  width: 280px;
  min-width: 220px;
}

.file-manager-breadcrumbs {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.file-manager-breadcrumb-label {
  flex: 0 0 auto;
  color: rgba(34, 46, 67, 0.62);
  font-size: 12px;
  font-weight: 600;
}

.file-manager-breadcrumbs :deep(.n-breadcrumb) {
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
}

.file-manager-breadcrumbs :deep(.n-breadcrumb-item) {
  max-width: 100%;
}

.file-manager-toolbar-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.file-search-controls {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.file-search-controls :deep(.n-checkbox) {
  flex: 0 0 auto;
  white-space: nowrap;
}

.file-search-controls :deep(.n-checkbox__label) {
  white-space: nowrap;
}

.file-search-input {
  width: min(320px, 100%);
  flex: 1 1 260px;
  min-width: 0;
}

.file-search-warning {
  margin-bottom: 6px;
}

.file-upload-input {
  display: none;
}

.file-manager-action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  padding: 8px 16px;
  border-bottom: 1px solid rgba(24, 35, 51, 0.08);
  background: rgba(255, 255, 255, 0.72);
}

.file-manager-action-controls {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.selection-count {
  min-width: 72px;
  font-size: 13px;
  font-weight: 600;
  color: rgba(34, 46, 67, 0.8);
}

.file-manager-body {
  display: flex;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.file-browser {
  position: relative;
  display: flex;
  flex-direction: column;
  flex: 0 0 clamp(320px, 30%, 420px);
  max-width: 420px;
  min-width: 0;
  min-height: 0;
  padding: 12px 12px 0;
  overflow: hidden;
}

.file-browser.file-browser--meta-0 {
  flex-basis: clamp(280px, 26%, 360px);
  max-width: 360px;
}

.file-browser.file-browser--meta-1 {
  flex-basis: clamp(320px, 30%, 420px);
  max-width: 420px;
}

.file-browser.file-browser--meta-2 {
  flex-basis: clamp(380px, 34%, 500px);
  max-width: 500px;
}

.file-browser.file-browser--meta-3 {
  flex-basis: clamp(430px, 38%, 580px);
  max-width: 580px;
}

.file-browser.is-drag-over::after {
  content: attr(data-drop-label);
  position: absolute;
  inset: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2px dashed rgba(53, 119, 186, 0.45);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.85);
  color: #1f4c7f;
  font-weight: 600;
  pointer-events: none;
}

.file-tree {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-bottom: 16px;
}

.file-tree-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 44px;
  padding: 8px 10px;
  border: 1px solid rgba(24, 35, 51, 0.08);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.88);
  text-align: left;
  cursor: pointer;
  transition:
    transform 120ms ease,
    border-color 120ms ease,
    box-shadow 120ms ease;
}

.file-tree-row:hover,
.file-tree-row.is-active {
  border-color: rgba(53, 119, 186, 0.32);
  background: rgba(240, 247, 255, 0.9);
  box-shadow: 0 10px 24px rgba(40, 73, 118, 0.08);
}

.tree-expand-hit {
  width: 16px;
  flex: 0 0 16px;
  color: rgba(34, 46, 67, 0.72);
  text-align: center;
}

.tree-expand-hit.is-placeholder {
  color: transparent;
}

.file-list-spin {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.file-list-spin :deep(.n-spin-body),
.file-list-spin :deep(.n-spin-container),
.file-list-spin :deep(.n-spin-content) {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.file-list-scroll {
  flex: 1;
  height: auto;
  max-height: 100%;
  min-height: 0;
  overflow: auto;
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;
  padding-bottom: 8px;
}

.file-list-scroll,
.file-preview-content,
.file-transfer-items {
  scrollbar-width: thin;
  scrollbar-color: rgba(37, 90, 143, 0.38) transparent;
}

.file-list-scroll::-webkit-scrollbar,
.file-preview-content::-webkit-scrollbar,
.file-transfer-items::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.file-list-scroll::-webkit-scrollbar-track,
.file-preview-content::-webkit-scrollbar-track,
.file-transfer-items::-webkit-scrollbar-track {
  background: transparent;
}

.file-list-scroll::-webkit-scrollbar-thumb,
.file-preview-content::-webkit-scrollbar-thumb,
.file-transfer-items::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(37, 90, 143, 0.28);
}

.file-list-scroll::-webkit-scrollbar-thumb:hover,
.file-preview-content::-webkit-scrollbar-thumb:hover,
.file-transfer-items::-webkit-scrollbar-thumb:hover {
  background: rgba(37, 90, 143, 0.42);
}

.file-list-checkbox {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 32px;
  height: 32px;
  margin: -6px 0 -6px -4px;
  border-radius: 8px;
}

.file-list-checkbox:hover {
  background: rgba(37, 90, 143, 0.08);
}

.file-list-checkbox :deep(input[type='checkbox']) {
  pointer-events: none;
}

.file-tree-main {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}

.tree-name-cell {
  min-width: 0;
}

.file-kind-icon {
  color: #255a8f;
}

.file-name-text {
  flex: 1;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.file-row-meta {
  flex: 0 0 auto;
  max-width: 52%;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: rgba(34, 46, 67, 0.78);
  font-size: 12px;
  text-align: right;
}

.file-display-menu {
  display: flex;
  min-width: 180px;
  flex-direction: column;
  gap: 4px;
}

.file-display-option {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 10px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: rgba(34, 46, 67, 0.88);
  cursor: pointer;
  text-align: left;
}

.file-display-option:hover,
.file-display-option.is-selected {
  background: rgba(37, 90, 143, 0.08);
}

.file-display-option-check {
  width: 14px;
  flex: 0 0 14px;
  color: #255a8f;
  font-size: 13px;
  font-weight: 700;
  text-align: center;
}

.file-manager-empty,
.file-manager-error {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 220px;
}

.file-preview-pane {
  flex: 1;
  min-width: 0;
  border-left: 1px solid rgba(24, 35, 51, 0.08);
  background: rgba(255, 255, 255, 0.78);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.file-image-modal {
  width: min(92vw, 1100px);
}

.file-mobile-preview-modal {
  width: min(100vw, 720px);
}

.file-mobile-preview-modal :deep(.n-card) {
  width: 100%;
}

.file-mobile-preview-modal :deep(.n-card__content) {
  padding: 0;
}

.file-mobile-preview-surface {
  min-height: min(100vh, 100dvh);
  background: rgba(255, 255, 255, 0.98);
}

.file-image-modal-title {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-image-modal-image {
  display: block;
  width: 100%;
  max-height: 78vh;
  object-fit: contain;
  border-radius: 12px;
  background: #f4f7fb;
}

.file-transfer-queue {
  flex-shrink: 0;
  border-top: 1px solid rgba(24, 35, 51, 0.08);
  background: rgba(255, 255, 255, 0.88);
  padding: 12px 16px;
}

.file-transfer-queue-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 700;
}

.file-transfer-items {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 220px;
  overflow: auto;
  -webkit-overflow-scrolling: touch;
}

.file-transfer-item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 10px 12px;
  border: 1px solid rgba(24, 35, 51, 0.08);
  border-radius: 14px;
  background: rgba(248, 250, 253, 0.96);
}

.file-transfer-main {
  flex: 1;
  min-width: 0;
}

.file-transfer-name {
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-transfer-meta {
  margin: 4px 0 8px;
  color: rgba(34, 46, 67, 0.66);
  font-size: 12px;
}

.file-transfer-actions {
  display: flex;
  gap: 6px;
  align-items: center;
}

.file-transfer-error {
  margin-top: 6px;
  color: #b42318;
  font-size: 12px;
}

@media (max-width: 1080px) {
  .file-manager-body {
    flex-direction: column;
  }

  .file-browser,
  .file-browser.file-browser--meta-0,
  .file-browser.file-browser--meta-1,
  .file-browser.file-browser--meta-2,
  .file-browser.file-browser--meta-3 {
    flex: 1 1 auto;
    width: 100%;
    max-width: none;
  }

  .file-preview-pane {
    width: 100%;
    min-width: 0;
    min-height: 220px;
    border-left: none;
    border-top: 1px solid rgba(24, 35, 51, 0.08);
  }
}

@media (max-width: 1320px) {
  .file-tree-main {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }

  .file-row-meta {
    max-width: 100%;
    text-align: left;
  }
}

@media (max-width: 820px) {
  .file-mobile-preview-modal {
    width: 100vw;
    margin: 0;
  }

  .file-mobile-preview-modal :deep(.n-card) {
    min-height: 100dvh;
    border-radius: 0;
  }

  .file-manager-toolbar {
    flex-wrap: wrap;
  }

  .scope-select,
  .file-manager-breadcrumbs {
    width: 100%;
  }

  .file-manager-toolbar-actions {
    width: 100%;
    flex-wrap: wrap;
  }

  .file-manager-action-bar {
    flex-wrap: wrap;
  }

  .file-manager-action-controls {
    width: 100%;
    justify-content: flex-start;
  }

  .file-search-controls,
  .file-search-input {
    width: 100%;
  }

  .file-transfer-item {
    flex-direction: column;
  }

  .file-transfer-actions {
    width: 100%;
    flex-wrap: wrap;
  }

  .file-manager-body {
    overflow: hidden;
  }
}
</style>
