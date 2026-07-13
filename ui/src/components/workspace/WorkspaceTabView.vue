<template>
  <div class="workspace-tab-view">
    <!-- 顶部Tab栏 -->
    <div class="tab-header">
      <div class="tab-list">
        <button
          type="button"
          class="tab-item"
          :class="{ active: activeTab === 'terminal' }"
          @click="activateTab('terminal')"
        >
          <n-icon size="16">
            <TerminalOutline />
          </n-icon>
          <span class="tab-label">{{ t('nav.terminal') }}</span>
          <span v-if="terminalCount > 0" class="tab-badge">{{ terminalCount }}</span>
        </button>
        <button
          type="button"
          class="tab-item"
          :class="{ active: activeTab === 'web' }"
          @click="activateTab('web')"
        >
          <n-icon size="16">
            <ChatbubblesOutline />
          </n-icon>
          <span class="tab-label">{{ t('nav.webSession') }}</span>
          <span class="tab-badge session-summary-badge">
            {{ webSessionSummaryText }}
          </span>
        </button>
        <button
          type="button"
          class="tab-item"
          :class="{ active: activeTab === 'changes' }"
          :disabled="changesTabDisabled"
          @click="activateTab('changes')"
        >
          <n-icon size="16">
            <GitBranchOutline />
          </n-icon>
          <span class="tab-label">{{ t('nav.changes') }}</span>
          <n-tooltip v-if="showChangesSummaryBadge" :disabled="!changesSummaryIncomplete">
            <template #trigger>
              <span class="tab-badge changes-summary-badge">
                <span class="changes-summary-count">{{ changesSummaryDisplay.count }}</span>
                <template v-if="changesSummaryIncomplete">
                  <n-icon :size="12" class="changes-summary-warning">
                    <WarningOutline />
                  </n-icon>
                </template>
                <template v-else>
                  <span class="changes-summary-separator">,</span>
                  <span class="changes-summary-add">{{ changesSummaryDisplay.additions }}</span>
                  <span class="changes-summary-separator">,</span>
                  <span class="changes-summary-del">{{ changesSummaryDisplay.deletions }}</span>
                  <n-icon v-if="changesSummaryLoading" :size="11" class="changes-summary-loading">
                    <SyncOutline />
                  </n-icon>
                </template>
              </span>
            </template>
            {{ changesSummaryStatusText }}
          </n-tooltip>
        </button>
        <button
          type="button"
          class="tab-item"
          :class="{ active: activeTab === 'files' }"
          @click="activateTab('files')"
        >
          <n-icon size="16">
            <FolderOpenOutline />
          </n-icon>
          <span class="tab-label">{{ t('nav.files') }}</span>
        </button>
      </div>
      <div v-if="activeTab === 'terminal' || activeTab === 'web'" class="tab-actions">
        <n-tooltip placement="bottom" :delay="250">
          <template #trigger>
            <button
              type="button"
              class="header-action-btn"
              :aria-label="rightSidebarToggleLabel"
              :aria-pressed="isRightSidebarVisible"
              @click="toggleRightSidebar"
            >
              <svg
                v-if="isRightSidebarVisible"
                class="sidebar-toggle-icon"
                viewBox="0 0 20 20"
                fill="none"
                aria-hidden="true"
              >
                <rect
                  x="2.75"
                  y="3.25"
                  width="14.5"
                  height="13.5"
                  rx="2.25"
                  stroke="currentColor"
                  stroke-width="1.5"
                />
                <path
                  d="M12.25 4v12"
                  stroke="currentColor"
                  stroke-width="1.5"
                  stroke-linecap="round"
                />
                <path
                  d="M14 8.25L15.75 10L14 11.75"
                  stroke="currentColor"
                  stroke-width="1.5"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
              <svg
                v-else
                class="sidebar-toggle-icon"
                viewBox="0 0 20 20"
                fill="none"
                aria-hidden="true"
              >
                <rect
                  x="2.75"
                  y="3.25"
                  width="14.5"
                  height="13.5"
                  rx="2.25"
                  stroke="currentColor"
                  stroke-width="1.5"
                />
                <path
                  d="M12.25 4v12"
                  stroke="currentColor"
                  stroke-width="1.5"
                  stroke-linecap="round"
                  stroke-dasharray="1.5 2"
                  opacity="0.5"
                />
                <path
                  d="M15.75 8.25L14 10L15.75 11.75"
                  stroke="currentColor"
                  stroke-width="1.5"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            </button>
          </template>
          {{ rightSidebarToggleLabel }}
        </n-tooltip>
      </div>
    </div>

    <!-- Tab内容 -->
    <div class="tab-content">
      <div v-show="activeTab === 'terminal'" class="tab-pane terminal-pane">
        <div class="terminal-split">
          <div class="terminal-main">
            <TerminalPanel :project-id="projectId" />
          </div>
          <DockedNotificationSidebar v-if="isRightSidebarVisible" />
        </div>
      </div>
      <div v-show="activeTab === 'web'" class="tab-pane web-pane">
        <div class="terminal-split">
          <div class="terminal-main web-main">
            <WebSessionPanel
              :project-id="projectId"
              :show-sidebar="isRightSidebarVisible"
              :is-active="activeTab === 'web'"
            />
          </div>
        </div>
      </div>
      <div v-show="activeTab === 'changes'" class="tab-pane changes-pane">
        <GitChangesPanel
          :project-id="projectId"
          :is-active="activeTab === 'changes'"
          @summary-change="handleChangesPanelSummaryChange"
        />
      </div>
      <div v-show="activeTab === 'files'" class="tab-pane files-pane">
        <FileManagerPanel :project-id="projectId" :is-active="activeTab === 'files'" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useEventListener, useStorage } from '@vueuse/core';
import { NIcon } from 'naive-ui';
import {
  ChatbubblesOutline,
  FolderOpenOutline,
  GitBranchOutline,
  SyncOutline,
  TerminalOutline,
  WarningOutline,
} from '@vicons/ionicons5';
import { storeToRefs } from 'pinia';
import { useRoute, useRouter } from 'vue-router';
import {
  formatAiStatusTripletWithTotal,
  summarizeWebSessions,
} from '@/composables/useAiStatusSummary';
import { useLocale } from '@/composables/useLocale';
import { useProjectStore } from '@/stores/project';
import { useSettingsStore } from '@/stores/settings';
import { useTerminalStore } from '@/stores/terminal';
import { useWebSessionStore } from '@/stores/webSession';
import {
  chooseGitChangesScope,
  formatGitChangesBadgeDelta,
  GIT_CHANGES_IGNORE_UNTRACKED_DEFAULT,
  GIT_CHANGES_IGNORE_UNTRACKED_STORAGE_KEY,
  shouldLoadGitChangesStats,
  shouldShowGitChangesBadge,
  type GitChangesBadgeSummary,
} from '@/components/changes/gitChangesSummary';
import {
  canShowWorkspaceChangesSummary,
  shouldLoadWorkspaceChangesSummary,
} from '@/components/changes/gitChangesBehavior';
import GitChangesPanel from '@/components/changes/GitChangesPanel.vue';
import { createGitChangesLoadController } from '@/components/changes/gitChangesLoadController';
import FileManagerPanel from '@/components/files/FileManagerPanel.vue';
import TerminalPanel from '@/components/terminal/TerminalPanel.vue';
import DockedNotificationSidebar from '@/components/workspace/DockedNotificationSidebar.vue';
import WebSessionPanel from '@/components/web-session/WebSessionPanel.vue';
import {
  buildWorkspaceRouteQuery,
  isWorkspaceRouteTabQuerySynced,
  resolveDesktopWorkspaceRouteTab,
  type DesktopWorkspaceRouteTab,
} from '@/utils/workspaceRoute';
import { resolveWorkspaceShortcutTarget } from '@/utils/workspaceTabShortcut';
import { projectSupportsGit } from '@/utils/projectGitCapability';
import { fileManagerApi } from '@/api/fileManager';

const props = defineProps<{
  projectId: string;
}>();

type WorkspaceTab = DesktopWorkspaceRouteTab;

const WORKSPACE_ACTIVE_TAB_STORAGE_KEY = 'workspace-active-tab';
const CHANGES_SUMMARY_STATS_TIMEOUT_MS = 5_000;

const { t } = useLocale();
const route = useRoute();
const router = useRouter();
const projectStore = useProjectStore();
const settingsStore = useSettingsStore();
const terminalStore = useTerminalStore();
const webSessionStore = useWebSessionStore();
const { terminalShortcut } = storeToRefs(settingsStore);

function normalizeWorkspaceTab(value: unknown): WorkspaceTab {
  return resolveDesktopWorkspaceRouteTab(null, value);
}

const changesTabDisabled = computed(
  () =>
    Boolean(projectStore.currentProject) &&
    !projectStore.loading &&
    !projectSupportsGit(projectStore.currentProject, projectStore.worktrees)
);

function coerceWorkspaceTab(tab: WorkspaceTab): WorkspaceTab {
  if (changesTabDisabled.value && tab === 'changes') {
    return 'files';
  }
  return tab;
}

const storedActiveTab = useStorage<WorkspaceTab>(WORKSPACE_ACTIVE_TAB_STORAGE_KEY, 'terminal');
const activeTab = ref<WorkspaceTab>(
  coerceWorkspaceTab(normalizeWorkspaceTab(storedActiveTab.value))
);
const previousTab = ref<WorkspaceTab | null>(null);
const isRightSidebarVisible = useStorage('workspace-right-sidebar-visible', true);
const ignoreUntracked = useStorage<boolean>(
  GIT_CHANGES_IGNORE_UNTRACKED_STORAGE_KEY,
  GIT_CHANGES_IGNORE_UNTRACKED_DEFAULT
);
const changesBadgeSummary = ref<GitChangesBadgeSummary | null>(null);
const changesSummaryScopeId = ref('');
let changesSummaryTimer: number | null = null;
const changesSummaryScopeController = createGitChangesLoadController();
const changesSummaryLoadController = createGitChangesLoadController();
const canShowChangesSummaryBadge = computed(() =>
  canShowWorkspaceChangesSummary(props.projectId, changesTabDisabled.value)
);
const shouldTrackChangesSummary = computed(() =>
  shouldLoadWorkspaceChangesSummary(props.projectId, changesTabDisabled.value, activeTab.value)
);

function syncWorkspaceRouteTab(tab: WorkspaceTab) {
  if (isWorkspaceRouteTabQuerySynced(route.query, tab)) {
    return;
  }
  void router.replace({
    query: buildWorkspaceRouteQuery(route.query, tab),
  });
}

watch(
  [() => route.query, storedActiveTab, changesTabDisabled],
  ([query, storedTab]) => {
    const nextTab = coerceWorkspaceTab(resolveDesktopWorkspaceRouteTab(query, storedTab));
    if (storedActiveTab.value !== nextTab) {
      storedActiveTab.value = nextTab;
    }
    if (activeTab.value !== nextTab) {
      previousTab.value = activeTab.value;
      activeTab.value = nextTab;
    }
    syncWorkspaceRouteTab(nextTab);
  },
  { immediate: true }
);

watch(
  () => [props.projectId, projectStore.selectedWorktreeId, changesTabDisabled.value] as const,
  async () => {
    stopChangesSummaryTimer();
    changesSummaryScopeController.cancel();
    changesSummaryLoadController.cancel();
    changesSummaryScopeId.value = '';
    clearChangesBadgeSummary();
    if (!canShowChangesSummaryBadge.value || !shouldTrackChangesSummary.value) {
      return;
    }
    await initializeChangesSummaryTracking();
  },
  { immediate: true }
);

watch(
  () => activeTab.value,
  async () => {
    stopChangesSummaryTimer();
    changesSummaryLoadController.cancel();
    if (!canShowChangesSummaryBadge.value || !shouldTrackChangesSummary.value) {
      return;
    }
    await initializeChangesSummaryTracking();
  }
);

watch(
  () => ignoreUntracked.value,
  async () => {
    if (!canShowChangesSummaryBadge.value) {
      clearChangesBadgeSummary();
      return;
    }
    if (!shouldTrackChangesSummary.value) {
      return;
    }
    changesSummaryLoadController.cancel();
    await loadChangesSummary();
  }
);

function activateTab(nextTab: WorkspaceTab) {
  const normalized = coerceWorkspaceTab(normalizeWorkspaceTab(nextTab));
  if (storedActiveTab.value !== normalized) {
    storedActiveTab.value = normalized;
  }
  if (normalized === activeTab.value) {
    syncWorkspaceRouteTab(normalized);
    return;
  }
  previousTab.value = activeTab.value;
  activeTab.value = normalized;
  syncWorkspaceRouteTab(normalized);
}

function togglePreviousWorkspaceTab() {
  const targetTab = resolveWorkspaceShortcutTarget(activeTab.value, previousTab.value);
  if (targetTab === activeTab.value) {
    return;
  }
  previousTab.value = activeTab.value;
  activeTab.value = targetTab;
  storedActiveTab.value = targetTab;
  syncWorkspaceRouteTab(targetTab);
}

// 终端数量
const terminalCount = computed(() => {
  return terminalStore.getTabs(props.projectId).length;
});

const webSessionSummary = computed(() =>
  summarizeWebSessions(webSessionStore.getSessions(props.projectId), sessionId =>
    webSessionStore.getLiveState(sessionId)
  )
);
const webSessionSummaryText = computed(() =>
  formatAiStatusTripletWithTotal(
    webSessionSummary.value,
    webSessionStore.getSessions(props.projectId).length
  )
);
const changesSummaryDisplay = computed(() => {
  const summary = changesBadgeSummary.value ?? {
    count: 0,
    additions: 0,
    deletions: 0,
    state: 'complete' as const,
    changeToken: '',
    scopeId: '',
  };
  return {
    count: summary.count,
    additions: formatGitChangesBadgeDelta('+', summary.additions),
    deletions: formatGitChangesBadgeDelta('-', summary.deletions),
  };
});
const changesSummaryLoading = computed(() => changesBadgeSummary.value?.state === 'loading');
const changesSummaryIncomplete = computed(
  () =>
    changesBadgeSummary.value?.state === 'partial' ||
    changesBadgeSummary.value?.state === 'timedOut'
);
const changesSummaryStatusText = computed(() =>
  t(
    changesBadgeSummary.value?.state === 'timedOut'
      ? 'gitChanges.statsTimedOut'
      : 'gitChanges.statsPartial'
  )
);
const showChangesSummaryBadge = computed(
  () => canShowChangesSummaryBadge.value && shouldShowGitChangesBadge(changesBadgeSummary.value)
);

const rightSidebarToggleLabel = computed(() =>
  t(isRightSidebarVisible.value ? 'webSession.hideSidebar' : 'webSession.showSidebar')
);

function isToggleShortcut(event: KeyboardEvent) {
  if (event.metaKey || event.ctrlKey || event.altKey) {
    return false;
  }
  return event.code === terminalShortcut.value.code;
}

function isTerminalElement(element: HTMLElement | null) {
  if (!element) {
    return false;
  }
  return Boolean(element.closest('.terminal-shell'));
}

function isEditableElement(element: HTMLElement | null) {
  if (!element) {
    return false;
  }
  if (element.isContentEditable) {
    return true;
  }
  const tagName = element.tagName;
  if (tagName === 'INPUT' || tagName === 'TEXTAREA') {
    const input = element as HTMLInputElement | HTMLTextAreaElement;
    return !input.readOnly && !input.disabled;
  }
  return false;
}

function handleDockedTerminalToggleShortcut(event: KeyboardEvent) {
  if (event.defaultPrevented) {
    return;
  }
  if (event.repeat || !isToggleShortcut(event)) {
    return;
  }
  const activeElement = (
    typeof document !== 'undefined' ? document.activeElement : null
  ) as HTMLElement | null;
  if (isTerminalElement(activeElement) || isEditableElement(activeElement)) {
    return;
  }
  event.preventDefault();
  togglePreviousWorkspaceTab();
}

function toggleRightSidebar() {
  isRightSidebarVisible.value = !isRightSidebarVisible.value;
}

function stopChangesSummaryTimer() {
  if (changesSummaryTimer !== null && typeof window !== 'undefined') {
    window.clearInterval(changesSummaryTimer);
  }
  changesSummaryTimer = null;
}

function clearChangesBadgeSummary() {
  changesBadgeSummary.value = null;
}

async function resolveChangesSummaryScope() {
  if (changesSummaryScopeId.value) {
    return changesSummaryScopeId.value;
  }

  const loadHandle = changesSummaryScopeController.begin();
  try {
    const scopes = await fileManagerApi.listScopes(props.projectId, {
      signal: loadHandle.signal,
    });
    if (!changesSummaryScopeController.isCurrent(loadHandle)) {
      return '';
    }
    const scope = chooseGitChangesScope(scopes, {
      preferredWorktreeId: projectStore.selectedWorktreeId,
    });
    changesSummaryScopeId.value = scope?.id ?? '';
    return changesSummaryScopeId.value;
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') {
      return '';
    }
    throw error;
  } finally {
    changesSummaryScopeController.release(loadHandle);
  }
}

async function initializeChangesSummaryTracking() {
  try {
    const scopeId = await resolveChangesSummaryScope();
    if (!scopeId || !shouldTrackChangesSummary.value) {
      return;
    }
    await loadChangesSummary();
    startChangesSummaryTimer();
  } catch {
    // Keep the last successful badge when scope discovery is temporarily unavailable.
  }
}

async function loadChangesSummary() {
  if (!canShowChangesSummaryBadge.value) {
    clearChangesBadgeSummary();
    return;
  }
  if (!shouldTrackChangesSummary.value) {
    return;
  }

  const scopeId = await resolveChangesSummaryScope().catch(() => '');
  if (!scopeId || !shouldTrackChangesSummary.value) {
    return;
  }

  const loadHandle = changesSummaryLoadController.begin();
  const previousSummary = changesBadgeSummary.value;

  try {
    const fastSummary = await fileManagerApi.changesSummary(props.projectId, scopeId, {
      includeUntracked: !ignoreUntracked.value,
      withStats: false,
      signal: loadHandle.signal,
    });
    if (!changesSummaryLoadController.isCurrent(loadHandle)) {
      return;
    }

    if (fastSummary.count <= 0) {
      changesBadgeSummary.value = {
        count: 0,
        additions: 0,
        deletions: 0,
        state: 'complete',
        changeToken: fastSummary.changeToken,
        scopeId,
      };
      return;
    }

    if (!shouldLoadGitChangesStats(previousSummary, scopeId, fastSummary.changeToken)) {
      return;
    }

    const retainedSummary =
      previousSummary?.scopeId === scopeId && previousSummary.state === 'complete'
        ? previousSummary
        : null;
    changesBadgeSummary.value = {
      count: retainedSummary?.count ?? fastSummary.count,
      additions: retainedSummary?.additions ?? null,
      deletions: retainedSummary?.deletions ?? null,
      state: 'loading',
      changeToken: fastSummary.changeToken,
      scopeId,
    };

    const statsSummary = await fileManagerApi.changesSummary(props.projectId, scopeId, {
      includeUntracked: !ignoreUntracked.value,
      withStats: true,
      timeoutMs: CHANGES_SUMMARY_STATS_TIMEOUT_MS,
      signal: loadHandle.signal,
    });
    if (!changesSummaryLoadController.isCurrent(loadHandle)) {
      return;
    }

    changesBadgeSummary.value = {
      count: statsSummary.count > 0 ? statsSummary.count : fastSummary.count,
      additions: statsSummary.statsComplete ? (statsSummary.additions ?? 0) : null,
      deletions: statsSummary.statsComplete ? (statsSummary.deletions ?? 0) : null,
      state: statsSummary.statsComplete
        ? 'complete'
        : statsSummary.statsTimedOut
          ? 'timedOut'
          : 'partial',
      changeToken: statsSummary.changeToken,
      scopeId,
    };
  } catch (error) {
    if (!changesSummaryLoadController.isCurrent(loadHandle)) {
      return;
    }
    if (error instanceof Error && error.name === 'AbortError') {
      return;
    }
    changesBadgeSummary.value = previousSummary;
  } finally {
    changesSummaryLoadController.release(loadHandle);
  }
}

function handleChangesPanelSummaryChange(summary: GitChangesBadgeSummary | null) {
  if (activeTab.value !== 'changes') {
    return;
  }
  if (!canShowChangesSummaryBadge.value) {
    clearChangesBadgeSummary();
    return;
  }
  changesBadgeSummary.value = summary;
  changesSummaryScopeId.value = summary?.scopeId ?? '';
}

function startChangesSummaryTimer() {
  stopChangesSummaryTimer();
  if (typeof window === 'undefined' || !shouldTrackChangesSummary.value) {
    return;
  }
  changesSummaryTimer = window.setInterval(() => {
    void loadChangesSummary();
  }, 10_000);
}

const handleEnsureExpandedEvent = (payload?: { projectId?: string }) => {
  if (payload?.projectId && payload.projectId !== props.projectId) {
    return;
  }
  activateTab('terminal');
};

const handleTerminalCreatedEvent = (payload?: { projectId?: string }) => {
  if (payload?.projectId && payload.projectId !== props.projectId) {
    return;
  }
  activateTab('terminal');
};

const handleWebSessionCreatedEvent = (payload?: { projectId?: string }) => {
  if (payload?.projectId && payload.projectId !== props.projectId) {
    return;
  }
  activateTab('web');
};

terminalStore.emitter.on('terminal:ensure-expanded', handleEnsureExpandedEvent);
terminalStore.emitter.on('terminal:created', handleTerminalCreatedEvent);
webSessionStore.emitter.on('web-session:created', handleWebSessionCreatedEvent);
onBeforeUnmount(() => {
  stopChangesSummaryTimer();
  changesSummaryScopeController.cancel();
  changesSummaryLoadController.cancel();
  terminalStore.emitter.off('terminal:ensure-expanded', handleEnsureExpandedEvent);
  terminalStore.emitter.off('terminal:created', handleTerminalCreatedEvent);
  webSessionStore.emitter.off('web-session:created', handleWebSessionCreatedEvent);
});

if (typeof window !== 'undefined') {
  useEventListener(window, 'keydown', handleDockedTerminalToggleShortcut);
}
</script>

<style scoped>
.workspace-tab-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.tab-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  height: 40px;
  border-bottom: 1px solid var(--n-border-color, var(--app-input-border-color, rgba(0, 0, 0, 0.12)));
  background-color: var(--app-surface-color, var(--n-card-color, #ffffff));
  flex-shrink: 0;
}

.tab-list {
  display: flex;
  gap: 4px;
}

.tab-actions {
  display: flex;
  align-items: center;
}

.tab-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--n-text-color-2);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.tab-item:disabled {
  color: var(--n-text-color-disabled, rgba(15, 23, 42, 0.38));
  cursor: not-allowed;
  opacity: 0.6;
}

.tab-item:hover {
  background-color: var(--n-color-hover);
  color: var(--n-text-color);
}

.tab-item:disabled:hover {
  background: transparent;
  color: var(--n-text-color-disabled, rgba(15, 23, 42, 0.38));
}

.tab-item.active {
  background-color: var(--n-color-target);
  color: var(--n-primary-color);
  font-weight: 500;
}

.tab-item:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--n-primary-color);
}

.tab-label {
  white-space: nowrap;
}

.tab-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background-color: var(--n-primary-color);
  color: #fff;
  font-size: 11px;
  font-weight: 500;
}

.session-summary-badge {
  min-width: auto;
  padding: 0 7px;
  font-variant-numeric: tabular-nums;
}

.changes-summary-badge {
  min-width: auto;
  padding: 0 7px;
  font-variant-numeric: tabular-nums;
  background: rgba(37, 90, 143, 0.12);
  color: var(--n-text-color);
  gap: 0;
}

.changes-summary-warning {
  margin-left: 3px;
  color: #b45309;
}

.changes-summary-loading {
  margin-left: 3px;
  color: var(--n-primary-color);
  animation: changes-summary-spin 1s linear infinite;
}

@keyframes changes-summary-spin {
  to {
    transform: rotate(360deg);
  }
}

.tab-item.active .changes-summary-badge {
  background: rgba(37, 90, 143, 0.16);
  color: var(--n-primary-color);
}

.changes-summary-count {
  color: rgba(43, 59, 81, 0.9);
}

.changes-summary-separator {
  color: rgba(43, 59, 81, 0.78);
}

.changes-summary-add {
  color: #15803d;
}

.changes-summary-del {
  color: #dc2626;
}

.tab-item.active .changes-summary-count {
  color: rgba(31, 76, 127, 0.92);
}

.tab-item.active .changes-summary-separator {
  color: rgba(31, 76, 127, 0.72);
}

.tab-item.active .changes-summary-add {
  color: #15803d;
}

.tab-item.active .changes-summary-del {
  color: #dc2626;
}

.header-action-btn {
  width: 30px;
  height: 30px;
  border: none;
  border-radius: 8px;
  background-color: transparent;
  color: var(--n-text-color-2);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  opacity: 0.82;
  transition:
    color 0.2s ease,
    background-color 0.2s ease,
    opacity 0.2s ease,
    box-shadow 0.2s ease;
}

.header-action-btn:hover {
  color: var(--n-text-color);
  background-color: var(--n-color-hover);
  opacity: 1;
}

.header-action-btn[aria-pressed='true'] {
  color: var(--n-text-color);
  background-color: transparent;
  opacity: 0.94;
}

.header-action-btn[aria-pressed='true']:hover {
  color: var(--n-primary-color);
}

.sidebar-toggle-icon {
  width: 18px;
  height: 18px;
  display: block;
}

.header-action-btn:focus-visible {
  outline: 2px solid var(--n-primary-color);
  outline-offset: 2px;
}

.tab-content {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  position: relative;
}

.tab-pane {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.terminal-pane {
  display: flex;
  flex-direction: column;
}

.web-pane {
  display: flex;
  flex-direction: column;
}

.changes-pane,
.files-pane {
  background-color: var(--app-surface-color, #ffffff);
}

.web-main {
  background: linear-gradient(180deg, rgba(246, 241, 232, 0.78), rgba(255, 255, 255, 0.94));
}

.terminal-split {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 12px;
  padding: 12px;
}

.terminal-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
}
</style>
