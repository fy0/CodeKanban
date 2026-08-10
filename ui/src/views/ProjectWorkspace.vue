<template>
  <div
    class="project-workspace"
    :class="{
      'is-mobile': isMobileLayout,
      'is-websession-composing':
        isMobileLayout && mobileActiveView === 'webSession' && isMobileWebSessionComposerFocused,
    }"
  >
    <!-- 桌面端布局 -->
    <template v-if="!isMobileLayout">
      <div class="workspace-desktop-shell">
        <!-- 左侧最近项目侧边栏 -->
        <aside class="project-sidebar-shell" :style="{ width: `${effectiveLeftSidebarWidth}px` }">
          <div class="project-sidebar">
            <RecentProjects
              :current-project-id="currentProjectId"
              :compact="isProjectSidebarCompact"
              @edit-current="openProjectEditDialog"
              @show-terminal="showTerminalTab"
            />
          </div>
        </aside>
        <div
          class="project-sidebar-resizer"
          :class="{
            'is-dragging': isProjectSidebarResizing,
            'is-compact': isProjectSidebarCompact,
          }"
          @mousedown="startProjectSidebarResize"
        >
          <div class="project-sidebar-resizer-handle"></div>
        </div>

        <n-layout has-sider class="workspace-main-shell">
          <!-- 右侧工作树侧边栏 -->
          <n-layout-sider
            v-model:collapsed="worktreeSiderCollapsed"
            bordered
            :width="320"
            :collapsed-width="0"
            show-trigger="arrow-circle"
          >
            <WorktreeList @open-terminal="handleOpenTerminal" />
          </n-layout-sider>

          <n-layout-content content-style="height: 100%;">
            <WorkspaceTabView :project-id="currentProjectId" />
          </n-layout-content>
        </n-layout>
      </div>
    </template>

    <!-- 移动端布局 -->
    <template v-else>
      <div class="mobile-workspace">
        <!-- 看板视图 -->
        <div
          v-if="mobileKanbanEnabled"
          v-show="mobileActiveView === 'kanban'"
          class="mobile-view mobile-kanban-view"
        >
          <KanbanBoard :project-id="currentProjectId" />
        </div>

        <!-- 终端视图占位（实际终端由 TerminalPanel 控制） -->
        <div v-show="mobileActiveView === 'terminal'" class="mobile-view mobile-terminal-view">
          <!-- 终端面板会覆盖此区域 -->
        </div>

        <div v-show="mobileActiveView === 'webSession'" class="mobile-view mobile-websession-view">
          <WebSessionPanel
            ref="webSessionPanelRef"
            :project-id="currentProjectId"
            :is-active="mobileActiveView === 'webSession'"
            @mobile-composer-focus-change="handleMobileWebSessionComposerFocusChange"
            @request-mobile-view="handleWebSessionPanelMobileViewRequest"
          />
        </div>

        <div v-show="mobileActiveView === 'files'" class="mobile-view mobile-files-view">
          <FileManagerPanel
            :project-id="currentProjectId"
            :is-active="mobileActiveView === 'files'"
          />
        </div>

        <div v-show="mobileActiveView === 'changes'" class="mobile-view mobile-changes-view">
          <GitChangesPanel
            :project-id="currentProjectId"
            :is-active="mobileActiveView === 'changes'"
          />
        </div>

        <!-- 项目视图 -->
        <div
          ref="mobileProjectsViewRef"
          v-show="mobileActiveView === 'projects'"
          class="mobile-view mobile-projects-view"
        >
          <ProjectBrowser
            mode="mobile-workspace"
            :current-project-id="currentProjectId"
            @mobile-project-select="handleMobileProjectSelect"
          />
        </div>

        <!-- 移动端底部导航 -->
        <div class="mobile-bottom-nav safe-area-bottom">
          <button
            type="button"
            class="nav-item"
            :class="{ active: mobileActiveView === 'projects' }"
            @click="setMobileView('projects')"
          >
            <span class="nav-icon-shell">
              <n-icon size="20"><FolderOutline /></n-icon>
            </span>
            <span>{{ t('nav.projects') }}</span>
          </button>
          <button type="button" class="nav-item" @click="handleGoToSettings">
            <span class="nav-icon-shell">
              <n-icon size="20"><SettingsOutline /></n-icon>
            </span>
            <span>{{ t('nav.settingsShort') }}</span>
          </button>
          <button
            type="button"
            class="nav-item"
            :class="{ active: mobileActiveView === 'files' }"
            @click="setMobileView('files')"
          >
            <span class="nav-icon-shell">
              <n-icon size="20"><FolderOpenOutline /></n-icon>
            </span>
            <span>{{ t('nav.files') }}</span>
          </button>
          <button
            type="button"
            class="nav-item"
            :class="{ active: mobileActiveView === 'changes' }"
            @click="setMobileView('changes')"
          >
            <span class="nav-icon-shell">
              <n-icon size="20"><GitBranchOutline /></n-icon>
            </span>
            <span>{{ t('nav.changes') }}</span>
          </button>
          <button
            ref="webSessionNavButtonRef"
            type="button"
            class="nav-item"
            :class="{
              active: mobileActiveView === 'webSession',
              'is-pressed': isWebSessionNavPressed,
            }"
            :aria-label="`${t('nav.webSession')} · ${t('project.webSessionCount')}: ${mobileWebSessionCount}`"
            @click="handleWebSessionNavClick"
            @contextmenu.prevent
            @pointerdown="handleWebSessionNavPointerDown"
            @pointermove="handleWebSessionNavPointerMove"
            @pointerup="handleWebSessionNavPointerUp"
            @pointercancel="handleWebSessionNavPointerCancel"
          >
            <span class="nav-icon-shell">
              <n-icon size="20"><ChatbubblesOutline /></n-icon>
              <span v-if="mobileWebSessionBadge" class="nav-count-badge" aria-hidden="true">
                {{ mobileWebSessionBadge }}
              </span>
            </span>
            <span>{{ t('nav.webSession') }}</span>
          </button>
          <button
            type="button"
            class="nav-item"
            :class="{ active: mobileActiveView === 'terminal' }"
            :aria-label="`${t('nav.terminal')} · ${t('project.terminalCount')}: ${mobileTerminalCount}`"
            @click="setMobileView('terminal')"
          >
            <span class="nav-icon-shell">
              <n-icon size="20"><TerminalOutline /></n-icon>
              <span v-if="mobileTerminalBadge" class="nav-count-badge" aria-hidden="true">
                {{ mobileTerminalBadge }}
              </span>
            </span>
            <span>{{ t('nav.terminal') }}</span>
          </button>
        </div>
      </div>
    </template>
    <TerminalPanel
      v-if="isMobileLayout"
      :project-id="currentProjectId"
      :is-mobile="true"
      :hidden="mobileActiveView !== 'terminal'"
    />
    <ProjectEditDialog
      v-model:show="showEditDialog"
      :project="projectStore.currentProject"
      @success="handleProjectUpdated"
    />
    <DailyTipDialog
      v-if="activeDailyTip"
      v-model:show="showDailyTipDialog"
      :tip="activeDailyTip"
      :tip-index="activeDailyTipIndex"
      :total-tips="dailyTipCount"
      @next="handleShowAnotherDailyTip"
      @acknowledge="handleDailyTipAcknowledge"
      @disable="handleDailyTipDisable"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useStorage } from '@vueuse/core';
import { useDialog, useMessage } from 'naive-ui';
import {
  ChatbubblesOutline,
  FolderOpenOutline,
  FolderOutline,
  GitBranchOutline,
  SettingsOutline,
  TerminalOutline,
} from '@vicons/ionicons5';
import { useProjectStore } from '@/stores/project';
import { useSettingsStore } from '@/stores/settings';
import { useTerminalStore } from '@/stores/terminal';
import { useWebSessionStore } from '@/stores/webSession';
import { useResponsive } from '@/composables/useResponsive';
import { useLocale } from '@/composables/useLocale';
import WorktreeList from '@/components/worktree/WorktreeList.vue';
import KanbanBoard from '@/components/kanban/KanbanBoard.vue';
import RecentProjects from '@/components/project/RecentProjects.vue';
import TerminalPanel from '@/components/terminal/TerminalPanel.vue';
import WorkspaceTabView from '@/components/workspace/WorkspaceTabView.vue';
import ProjectEditDialog from '@/components/project/ProjectEditDialog.vue';
import WebSessionPanel from '@/components/web-session/WebSessionPanel.vue';
import GitChangesPanel from '@/components/changes/GitChangesPanel.vue';
import FileManagerPanel from '@/components/files/FileManagerPanel.vue';
import ProjectBrowser from '@/components/project/ProjectBrowser.vue';
import { buildProjectBrowserProjectLocation } from '@/components/project/projectBrowserNavigation';
import DailyTipDialog from '@/components/common/DailyTipDialog.vue';
import type { Worktree } from '@/types/models';
import {
  DEFAULT_MOBILE_VIEW,
  formatMobileNavBadge,
  mobileViewToRouteTab,
  normalizeMobileView,
  resolveMobileProjectSelectionAction,
  resolveMobileProjectSourceViewChange,
  routeTabToMobileView,
  restorePersistedMobileView,
  type MobileProjectSourceView,
  type MobileView,
} from '@/views/projectWorkspaceMobileView';
import {
  buildWorkspaceRouteQuery,
  isWorkspaceRouteTabQuerySynced,
  resolveMobileWorkspaceRouteTab,
} from '@/utils/workspaceRoute';
import { gitOperationAvailable } from '@/utils/projectGitCapability';
import { createLongPressTracker } from '@/utils/longPress';
import {
  PROJECT_SIDEBAR_DEFAULT_WIDTH,
  clampProjectSidebarWidth,
  isProjectSidebarCompact as isCompactProjectSidebarWidth,
  PROJECT_SIDEBAR_COMPACT_WIDTH,
  resolveProjectSidebarDragWidth,
  resolveProjectSidebarMaxWidth,
} from '@/views/projectWorkspaceSidebar';
import {
  formatLocalDateKey,
  getDailyTips,
  loadDailyTipState,
  saveDailyTipState,
  selectAnotherRandomDailyTipIndex,
  selectDailyTipIndex,
  shouldShowDailyTip,
  type DailyTipDefinition,
} from '@/utils/dailyTips';

const WORKSPACE_MOBILE_MAX_WIDTH = 900;
const PROJECT_SIDEBAR_WIDTH_STORAGE_KEY = 'workspace-left-project-sidebar-width';
const MOBILE_ACTIVE_VIEW_STORAGE_KEY = 'workspace-mobile-active-view-by-project';
const MOBILE_GIT_STATUS_REFRESH_INTERVAL_MS = 10_000;

const route = useRoute();
const router = useRouter();
const dialog = useDialog();
const message = useMessage();
const projectStore = useProjectStore();
const settingsStore = useSettingsStore();
const terminalStore = useTerminalStore();
const webSessionStore = useWebSessionStore();
const { windowWidth } = useResponsive();
const { t, locale } = useLocale();
type WebSessionPanelControl = {
  openMobileSessionSelectorFromElement: (
    anchorEl: HTMLElement,
    source: 'header' | 'bottom-nav'
  ) => void;
  closeMobileSessionSelector: () => void;
};

const showEditDialog = ref(false);
const isMobileWebSessionComposerFocused = ref(false);
const showDailyTipDialog = ref(false);
const activeDailyTipIndex = ref(0);
const isWebSessionNavPressed = ref(false);
const mobileKanbanEnabled = false;
const webSessionPanelRef = ref<WebSessionPanelControl | null>(null);
const webSessionNavButtonRef = ref<HTMLButtonElement | null>(null);
const mobileProjectsViewRef = ref<HTMLElement | null>(null);
const mobileProjectSourceView = ref<MobileProjectSourceView | ''>('');
const mobileRouteSyncPaused = ref(false);
let mobileWebSessionComposerFocusFrame: number | null = null;
let mobileGitStatusRefreshTimer: number | null = null;
let mobileGitStatusRefreshPending = false;

const isMobileLayout = computed(() => windowWidth.value <= WORKSPACE_MOBILE_MAX_WIDTH);

const WORKTREE_SIDER_COLLAPSED_KEY = 'worktree-sider-collapsed';
const getInitialWorktreeSiderCollapsedState = (): boolean => {
  const stored = localStorage.getItem(WORKTREE_SIDER_COLLAPSED_KEY);
  return stored ? Boolean(JSON.parse(stored)) : true;
};
const worktreeSiderCollapsed = ref(getInitialWorktreeSiderCollapsedState());
watch(worktreeSiderCollapsed, collapsed => {
  localStorage.setItem(WORKTREE_SIDER_COLLAPSED_KEY, JSON.stringify(collapsed));
});

const leftProjectSidebarWidth = useStorage<number>(
  PROJECT_SIDEBAR_WIDTH_STORAGE_KEY,
  PROJECT_SIDEBAR_DEFAULT_WIDTH
);
const isProjectSidebarResizing = ref(false);

const maxLeftProjectSidebarWidth = computed(() => {
  return resolveProjectSidebarMaxWidth({
    windowWidth: windowWidth.value,
    worktreeCollapsed: worktreeSiderCollapsed.value,
  });
});

const effectiveLeftSidebarWidth = computed(() =>
  resolveProjectSidebarDragWidth(leftProjectSidebarWidth.value, maxLeftProjectSidebarWidth.value)
);
const isProjectSidebarCompact = computed(() =>
  isCompactProjectSidebarWidth(effectiveLeftSidebarWidth.value)
);

watch(
  [windowWidth, worktreeSiderCollapsed],
  () => {
    leftProjectSidebarWidth.value = resolveProjectSidebarDragWidth(
      leftProjectSidebarWidth.value,
      maxLeftProjectSidebarWidth.value
    );
  },
  { immediate: true }
);

let cleanupProjectSidebarResize: (() => void) | null = null;

function stopProjectSidebarResize() {
  cleanupProjectSidebarResize?.();
  cleanupProjectSidebarResize = null;
}

function startProjectSidebarResize(event: MouseEvent) {
  if (isMobileLayout.value) {
    return;
  }
  event.preventDefault();
  stopProjectSidebarResize();

  isProjectSidebarResizing.value = true;
  const startX = event.clientX;
  const startWidth = effectiveLeftSidebarWidth.value;

  const onMouseMove = (moveEvent: MouseEvent) => {
    const delta = moveEvent.clientX - startX;
    leftProjectSidebarWidth.value = Math.round(
      clampProjectSidebarWidth(
        PROJECT_SIDEBAR_COMPACT_WIDTH,
        startWidth + delta,
        maxLeftProjectSidebarWidth.value
      )
    );
  };

  const onMouseUp = () => {
    stopProjectSidebarResize();
  };

  cleanupProjectSidebarResize = () => {
    isProjectSidebarResizing.value = false;
    window.removeEventListener('mousemove', onMouseMove);
    window.removeEventListener('mouseup', onMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  };

  window.addEventListener('mousemove', onMouseMove);
  window.addEventListener('mouseup', onMouseUp);
  document.body.style.cursor = 'col-resize';
  document.body.style.userSelect = 'none';
}

const currentProjectId = computed(() =>
  typeof route.params.id === 'string' ? route.params.id : ''
);
const mobileTerminalCount = computed(() => {
  const projectId = currentProjectId.value;
  if (!projectId) {
    return 0;
  }
  return Math.max(
    terminalStore.getTerminalCount(projectId),
    terminalStore.terminalCounts.get(projectId) ?? 0
  );
});
const mobileWebSessionCount = computed(() => {
  const projectId = currentProjectId.value;
  if (!projectId) {
    return 0;
  }
  return Math.max(
    webSessionStore.getSessions(projectId).length,
    webSessionStore.getSessionCount(projectId)
  );
});
const mobileTerminalBadge = computed(() => formatMobileNavBadge(mobileTerminalCount.value));
const mobileWebSessionBadge = computed(() => formatMobileNavBadge(mobileWebSessionCount.value));
const dailyTipCount = computed(() => getDailyTips(locale.value).length);
const activeDailyTip = computed<DailyTipDefinition | null>(() => {
  const tips = getDailyTips(locale.value);
  if (tips.length === 0) {
    return null;
  }
  return tips[activeDailyTipIndex.value] ?? tips[0] ?? null;
});

const storedMobileViews = useStorage<Record<string, MobileView>>(
  MOBILE_ACTIVE_VIEW_STORAGE_KEY,
  {}
);
const mobileActiveView = ref<MobileView>(DEFAULT_MOBILE_VIEW);
const mobileWebSessionLongPress = createLongPressTracker({
  onLongPress: () => {
    if (!isMobileLayout.value) {
      return;
    }
    const anchorEl = webSessionNavButtonRef.value;
    if (!anchorEl) {
      return;
    }
    webSessionPanelRef.value?.openMobileSessionSelectorFromElement(anchorEl, 'bottom-nav');
  },
});

function syncMobileRouteTab(view: MobileView) {
  if (mobileRouteSyncPaused.value) {
    return;
  }

  const routeTab = mobileViewToRouteTab(view);
  if (isWorkspaceRouteTabQuerySynced(route.query, routeTab)) {
    return;
  }
  void router.replace({
    query: buildWorkspaceRouteQuery(route.query, routeTab),
  });
}

watch(
  [currentProjectId, () => route.query, () => isMobileLayout.value],
  ([projectId, query, mobile]) => {
    if (!projectId) {
      mobileActiveView.value = DEFAULT_MOBILE_VIEW;
      return;
    }

    const storedView = storedMobileViews.value[projectId];
    const restoredView = restorePersistedMobileView(storedView);
    const nextView = routeTabToMobileView(
      resolveMobileWorkspaceRouteTab(query, mobileViewToRouteTab(restoredView))
    );
    mobileActiveView.value = nextView;

    if (storedView !== restoredView) {
      storedMobileViews.value = {
        ...storedMobileViews.value,
        [projectId]: restoredView,
      };
    }

    if (mobile) {
      syncMobileRouteTab(nextView);
    }
  },
  { immediate: true }
);

watch(
  [currentProjectId, mobileActiveView, () => isMobileLayout.value],
  ([projectId, view, mobile]) => {
    if (!projectId) {
      return;
    }

    const normalizedView = normalizeMobileView(view);
    if (storedMobileViews.value[projectId] !== normalizedView) {
      storedMobileViews.value = {
        ...storedMobileViews.value,
        [projectId]: normalizedView,
      };
    }

    if (mobile) {
      syncMobileRouteTab(normalizedView);
    }
  }
);

watch(mobileActiveView, view => {
  if (view !== 'webSession') {
    isMobileWebSessionComposerFocused.value = false;
  }
});

watch(mobileActiveView, (nextView, previousView) => {
  if (!isMobileLayout.value) {
    mobileProjectSourceView.value = '';
    return;
  }

  mobileProjectSourceView.value = resolveMobileProjectSourceViewChange({
    previousView,
    nextView,
    currentSource: mobileProjectSourceView.value,
  });
});

watch(
  () => isMobileLayout.value,
  mobile => {
    if (!mobile) {
      isMobileWebSessionComposerFocused.value = false;
      mobileProjectSourceView.value = '';
    }
  }
);

function stopMobileGitStatusRefresh() {
  if (mobileGitStatusRefreshTimer != null) {
    window.clearInterval(mobileGitStatusRefreshTimer);
    mobileGitStatusRefreshTimer = null;
  }
}

function canRefreshMobileGitStatus() {
  return Boolean(
    isMobileLayout.value &&
      currentProjectId.value &&
      projectStore.currentProject?.id === currentProjectId.value &&
      !projectStore.projectDetailLoading &&
      gitOperationAvailable(projectStore.gitCapabilities, 'status')
  );
}

async function refreshMobileGitStatus() {
  if (
    !canRefreshMobileGitStatus() ||
    mobileGitStatusRefreshPending ||
    (typeof document !== 'undefined' && document.visibilityState === 'hidden')
  ) {
    return;
  }

  mobileGitStatusRefreshPending = true;
  try {
    await projectStore.refreshWorktreeCommitInfo(currentProjectId.value);
  } finally {
    mobileGitStatusRefreshPending = false;
  }
}

watch(
  () =>
    [
      isMobileLayout.value,
      currentProjectId.value,
      projectStore.currentProject?.id,
      projectStore.projectDetailLoading,
      gitOperationAvailable(projectStore.gitCapabilities, 'status'),
    ] as const,
  () => {
    stopMobileGitStatusRefresh();
    if (!canRefreshMobileGitStatus() || typeof window === 'undefined') {
      return;
    }
    mobileGitStatusRefreshTimer = window.setInterval(() => {
      void refreshMobileGitStatus();
    }, MOBILE_GIT_STATUS_REFRESH_INTERVAL_MS);
  },
  { immediate: true }
);

const loadProject = (id: string) => {
  if (!id) {
    return;
  }
  void projectStore.fetchProject(id).catch((error: unknown) => {
    message.error(error instanceof Error ? error.message : t('project.loadProjectFailed'));
  });
  projectStore.addRecentProject(id);
  void maybeShowDailyTip(id);
};

onMounted(() => {
  if (currentProjectId.value) {
    loadProject(currentProjectId.value);
  }
});

onBeforeUnmount(() => {
  if (mobileWebSessionComposerFocusFrame != null) {
    window.cancelAnimationFrame(mobileWebSessionComposerFocusFrame);
    mobileWebSessionComposerFocusFrame = null;
  }
  mobileWebSessionLongPress.pointerCancel();
  stopMobileGitStatusRefresh();
  stopProjectSidebarResize();
});

watch(
  () => route.params.id,
  newId => {
    if (typeof newId === 'string') {
      loadProject(newId);
    }
  }
);

// 监听路由变化，当从分支管理等页面返回到项目工作区时刷新 worktrees
watch(
  () => route.name,
  (newName, oldName) => {
    // 当从分支管理页面返回到项目工作区时，重新加载 worktrees
    // 这样可以确保在分支管理页面创建的新 worktree 能够立即显示
    if (newName === 'project' && oldName === 'project-branches' && currentProjectId.value) {
      projectStore.fetchWorktrees(currentProjectId.value);
    }
  }
);

function handleOpenTerminal(worktree: Worktree) {
  if (!currentProjectId.value) {
    return;
  }
  terminalStore
    .createSession(currentProjectId.value, {
      worktreeId: worktree.id,
      workingDir: worktree.path,
      title: worktree.branchName,
    })
    .catch((error: unknown) => {
      message.error(error instanceof Error ? error.message : t('terminal.createFailed'));
    });
}

function openProjectEditDialog() {
  showEditDialog.value = true;
}

async function handleProjectUpdated() {
  if (currentProjectId.value) {
    await projectStore.fetchProject(currentProjectId.value);
  }
}

function showTerminalTab() {
  if (isMobileLayout.value) {
    setMobileView('terminal');
    return;
  }
  if (!currentProjectId.value) {
    return;
  }
  terminalStore.emitter.emit('terminal:ensure-expanded', {
    projectId: currentProjectId.value,
  });
}

function handleMobileWebSessionComposerFocusChange(focused: boolean) {
  if (!isMobileLayout.value || mobileActiveView.value !== 'webSession') {
    isMobileWebSessionComposerFocused.value = false;
    return;
  }
  if (mobileWebSessionComposerFocusFrame != null) {
    window.cancelAnimationFrame(mobileWebSessionComposerFocusFrame);
    mobileWebSessionComposerFocusFrame = null;
  }
  if (!focused) {
    isMobileWebSessionComposerFocused.value = false;
    return;
  }
  mobileWebSessionComposerFocusFrame = window.requestAnimationFrame(() => {
    mobileWebSessionComposerFocusFrame = null;
    if (!isMobileLayout.value || mobileActiveView.value !== 'webSession') {
      isMobileWebSessionComposerFocused.value = false;
      return;
    }
    isMobileWebSessionComposerFocused.value = true;
  });
}

function handleWebSessionPanelMobileViewRequest(view: 'webSession' | 'changes') {
  setMobileView(view);
}

function scrollMobileProjectsToTop() {
  if (!isMobileLayout.value) {
    return;
  }
  mobileProjectsViewRef.value?.scrollTo({ top: 0, behavior: 'smooth' });
}

async function navigateToMobileProject(projectId: string, targetView: MobileView) {
  const previousView = mobileActiveView.value;
  mobileRouteSyncPaused.value = true;
  setMobileView(targetView);

  const location = buildProjectBrowserProjectLocation({
    mode: 'mobile-workspace',
    projectId,
    currentProjectId: currentProjectId.value,
    query: route.query,
    workspaceTab: mobileViewToRouteTab(targetView),
  });

  if (!location) {
    await nextTick();
    mobileRouteSyncPaused.value = false;
    syncMobileRouteTab(targetView);
    return;
  }

  try {
    await router.push(location);
  } catch (error) {
    mobileActiveView.value = previousView;
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    await nextTick();
    mobileRouteSyncPaused.value = false;
    syncMobileRouteTab(mobileActiveView.value);
  }
}

async function handleMobileProjectSelect(payload: { projectId: string }) {
  if (!isMobileLayout.value) {
    return;
  }

  const targetProjectId = payload.projectId.trim();
  if (!targetProjectId) {
    return;
  }

  const selectionAction = resolveMobileProjectSelectionAction(mobileProjectSourceView.value);
  await nextTick();
  scrollMobileProjectsToTop();

  if (selectionAction.type === 'navigate') {
    mobileProjectSourceView.value = '';
    await navigateToMobileProject(targetProjectId, selectionAction.targetView);
    return;
  }

  const sourceView = selectionAction.sourceView;

  dialog.info({
    title: t('project.mobileReturnPromptTitle'),
    content: t('project.mobileReturnPromptContent'),
    positiveText: t('project.mobileReturnPromptConfirm', {
      view: t(`nav.${sourceView}`),
    }),
    negativeText: t('project.mobileReturnPromptStay'),
    closable: false,
    maskClosable: false,
    onPositiveClick: () => {
      mobileProjectSourceView.value = '';
      void navigateToMobileProject(targetProjectId, sourceView);
    },
    onNegativeClick: () => {
      mobileProjectSourceView.value = '';
      void navigateToMobileProject(targetProjectId, 'projects');
    },
  });
}

function releaseWebSessionNavPointerCapture(event: PointerEvent) {
  const target = event.currentTarget;
  if (!(target instanceof HTMLElement) || !target.hasPointerCapture(event.pointerId)) {
    return;
  }
  try {
    target.releasePointerCapture(event.pointerId);
  } catch {
    // Ignore capture release failures from browsers that race pointer cleanup.
  }
}

function handleWebSessionNavPointerDown(event: PointerEvent) {
  if (!isMobileLayout.value || !event.isPrimary || event.pointerType === 'mouse') {
    return;
  }
  isWebSessionNavPressed.value = true;
  const target = event.currentTarget;
  if (target instanceof HTMLElement) {
    try {
      target.setPointerCapture(event.pointerId);
    } catch {
      // Ignore capture failures on browsers that do not support it for this element.
    }
  }
  mobileWebSessionLongPress.pointerDown(event.pointerId, {
    clientX: event.clientX,
    clientY: event.clientY,
  });
}

function handleWebSessionNavPointerMove(event: PointerEvent) {
  if (!isMobileLayout.value || !event.isPrimary || event.pointerType === 'mouse') {
    return;
  }
  mobileWebSessionLongPress.pointerMove(event.pointerId, {
    clientX: event.clientX,
    clientY: event.clientY,
  });
  isWebSessionNavPressed.value = mobileWebSessionLongPress.isPressing();
}

function handleWebSessionNavPointerUp(event: PointerEvent) {
  if (!isMobileLayout.value || !event.isPrimary || event.pointerType === 'mouse') {
    return;
  }
  mobileWebSessionLongPress.pointerUp(event.pointerId);
  isWebSessionNavPressed.value = false;
  releaseWebSessionNavPointerCapture(event);
}

function handleWebSessionNavPointerCancel(event: PointerEvent) {
  if (!isMobileLayout.value || !event.isPrimary || event.pointerType === 'mouse') {
    return;
  }
  mobileWebSessionLongPress.pointerCancel(event.pointerId);
  isWebSessionNavPressed.value = false;
  releaseWebSessionNavPointerCapture(event);
}

function handleGoToSettings() {
  void router.push('/settings');
}

async function maybeShowDailyTip(projectId: string) {
  await settingsStore.loadDailyTipSettings();
  if (!settingsStore.dailyTipSettingsLoaded || currentProjectId.value !== projectId) {
    return;
  }

  const tips = getDailyTips(locale.value);
  const todayDateKey = formatLocalDateKey();
  const state = loadDailyTipState();

  if (
    !shouldShowDailyTip({
      routeName: route.name,
      projectId,
      enabled: settingsStore.dailyTipEnabled,
      lastShownDate: state.lastShownDate,
      todayDateKey,
      tipCount: tips.length,
    })
  ) {
    return;
  }

  saveDailyTipState({
    ...state,
    lastShownDate: todayDateKey,
  });
  activeDailyTipIndex.value = selectDailyTipIndex(todayDateKey, tips.length);
  showDailyTipDialog.value = true;
}

function handleDailyTipAcknowledge() {
  showDailyTipDialog.value = false;
}

function handleShowAnotherDailyTip() {
  activeDailyTipIndex.value = selectAnotherRandomDailyTipIndex(
    activeDailyTipIndex.value,
    Math.random(),
    dailyTipCount.value
  );
}

function handleDailyTipDisable() {
  dialog.warning({
    title: t('dailyTip.disableConfirmTitle'),
    content: t('dailyTip.disableConfirmContent'),
    positiveText: t('dailyTip.disableForever'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await settingsStore.updateDailyTipEnabled(false);
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('common.saveFailed'));
        return false;
      }
      showDailyTipDialog.value = false;
      return true;
    },
  });
}

function handleWebSessionNavClick(event: MouseEvent) {
  isWebSessionNavPressed.value = false;
  if (mobileWebSessionLongPress.consumeClick()) {
    event.preventDefault();
    event.stopPropagation();
    return;
  }
  setMobileView('webSession');
}

// 移动端视图切换
function setMobileView(view: MobileView, options: { syncRoute?: boolean } = {}) {
  const normalizedView = normalizeMobileView(view);
  if (normalizedView !== 'webSession') {
    isMobileWebSessionComposerFocused.value = false;
    webSessionPanelRef.value?.closeMobileSessionSelector();
  }
  mobileActiveView.value = normalizedView;
  if (isMobileLayout.value && options.syncRoute !== false) {
    syncMobileRouteTab(normalizedView);
  }
}
</script>

<style scoped>
.project-workspace {
  height: 100vh;
  height: 100dvh;
  overflow: hidden;
  background-color: var(--app-canvas, var(--app-body-color, #f7f8fa));
  color: var(--app-text-primary, var(--app-text-color, #333333));
}

.workspace-desktop-shell {
  display: flex;
  height: 100%;
  min-height: 0;
}

.project-sidebar-shell {
  flex: 0 0 auto;
  height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  background-color: var(--app-surface, var(--app-surface-color, #ffffff));
  border-right: 1px solid var(--app-border, #e0e0e0);
}

.project-sidebar {
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.project-sidebar-resizer {
  flex-shrink: 0;
  width: 6px;
  margin: 0 -3px;
  cursor: col-resize;
  position: relative;
  z-index: 2;
}

.project-sidebar-resizer-handle {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  width: 2px;
  height: 32px;
  border-radius: 999px;
  background-color: transparent;
  opacity: 0;
  transition:
    background-color 0.15s ease,
    height 0.15s ease,
    opacity 0.15s ease;
}

.project-sidebar-resizer:hover .project-sidebar-resizer-handle {
  background-color: var(--app-border-strong, #d0d0d0);
  height: 48px;
  opacity: 1;
}

.project-sidebar-resizer.is-compact .project-sidebar-resizer-handle {
  background-color: var(--app-border-strong, #d0d0d0);
  height: 40px;
  opacity: 0.72;
}

.project-sidebar-resizer.is-dragging .project-sidebar-resizer-handle {
  background-color: var(--app-accent, #18a058);
  height: 64px;
  opacity: 1;
}

.workspace-main-shell {
  flex: 1;
  min-width: 0;
  height: 100%;
}

.workspace-content {
  padding: 24px;
  height: 100%;
  min-height: 0;
  overflow-y: auto;
  background-color: var(--app-surface, var(--app-surface-color, #ffffff));
}

/* 移动端布局 */
.project-workspace.is-mobile {
  --workspace-mobile-safe-area-bottom: env(safe-area-inset-bottom, 0px);
  --workspace-mobile-bottom-nav-height: 60px;
  --workspace-mobile-bottom-nav-space: calc(
    var(--workspace-mobile-bottom-nav-height) + var(--workspace-mobile-safe-area-bottom)
  );
  --workspace-mobile-websession-inset: var(--workspace-mobile-bottom-nav-space);
  height: 100vh;
  height: 100dvh;
  display: flex;
  flex-direction: column;
}

.project-workspace.is-mobile.is-websession-composing {
  --workspace-mobile-websession-inset: var(--workspace-mobile-safe-area-bottom);
}

.mobile-workspace {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.mobile-view {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

.mobile-kanban-view {
  padding-bottom: var(--workspace-mobile-bottom-nav-space);
}

.mobile-projects-view {
  min-height: 0;
}

.mobile-websession-view {
  display: flex;
  min-height: 0;
  overflow: hidden;
}

.mobile-websession-view > * {
  flex: 1;
  min-height: 0;
}

.mobile-files-view {
  display: flex;
  min-height: 0;
  overflow: hidden;
  padding-bottom: var(--workspace-mobile-bottom-nav-space);
}

.mobile-files-view > * {
  flex: 1;
  min-height: 0;
}

/* 移动端底部导航 */
.mobile-bottom-nav {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  box-sizing: border-box;
  height: var(--workspace-mobile-bottom-nav-space);
  padding-bottom: var(--workspace-mobile-safe-area-bottom);
  display: flex;
  align-items: stretch;
  justify-content: space-around;
  background-color: var(--app-surface, var(--app-surface-color, #ffffff));
  border-top: 1px solid var(--app-border, #e0e0e0);
  z-index: 200;
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
}

.project-workspace.is-mobile.is-websession-composing .mobile-bottom-nav {
  opacity: 0;
  transform: translateY(100%);
  pointer-events: none;
}

.mobile-bottom-nav .nav-item {
  flex: 1 1 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  min-height: var(--workspace-mobile-bottom-nav-height);
  padding: 6px 2px;
  border: none;
  background: transparent;
  color: var(--app-text-muted, #999);
  font-size: 11px;
  cursor: pointer;
  transition: color 0.2s;
  min-width: 0;
}

.mobile-bottom-nav .nav-item > span:last-child {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-bottom-nav .nav-item.active {
  color: var(--app-accent, #18a058);
}

.mobile-bottom-nav .nav-item:focus-visible {
  outline: 2px solid var(--app-focus-ring, #2080f0);
  outline-offset: -3px;
}

.mobile-bottom-nav .nav-item:active {
  opacity: 0.7;
}

.mobile-bottom-nav .nav-item.is-pressed {
  opacity: 0.88;
  transform: translateY(1px);
}

.mobile-bottom-nav .nav-icon-shell {
  width: 28px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  position: relative;
}

.mobile-bottom-nav .nav-count-badge {
  box-sizing: border-box;
  position: absolute;
  top: -7px;
  left: 16px;
  min-width: 17px;
  height: 17px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 4px;
  border: 2px solid var(--app-surface-color, #ffffff);
  border-radius: 9px;
  background: var(--n-primary-color, #18a058);
  color: #ffffff;
  font-size: 9px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1;
  letter-spacing: 0;
  pointer-events: none;
}

/* 平板端适配 */
@media (min-width: 768px) and (max-width: 1023px) {
  .workspace-content {
    padding: 16px;
  }
}
</style>
