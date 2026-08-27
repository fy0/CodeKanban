<template>
  <aside ref="rootRef" class="docked-terminal-sidebar">
    <div class="terminal-resizer" :class="{ 'is-dragging': isResizing }" @mousedown="startResize">
      <div class="resizer-handle"></div>
    </div>

    <section
      class="terminal-sidebar-panel"
      :style="{ width: `${effectiveSidebarWidthPx}px` }"
      :aria-label="t('terminal.title')"
    >
      <header class="terminal-sidebar-header">
        <div class="terminal-sidebar-title-wrap">
          <SplitDropdownControl
            class="terminal-sidebar-scope-control"
            :label="sidebarScopeLabel"
            :options="sidebarScopeOptions"
            :title="sidebarScopeTitle"
            :menu-title="sidebarScopeAriaLabel"
            :aria-label="sidebarScopeAriaLabel"
            flat
            @main-click="toggleSidebarScope"
            @select="handleSidebarScopeSelect"
          >
            <template #prefix>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
                <path
                  d="M4 7h16M4 12h10M4 17h8"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            </template>
          </SplitDropdownControl>
        </div>
        <div class="terminal-sidebar-header-actions">
          <span class="terminal-sidebar-count">{{ rows.length }}</span>
          <span v-if="allProjectSessionsLoading" class="terminal-sidebar-loading-icon">
            <n-icon size="12"><SyncOutline /></n-icon>
          </span>
          <n-tooltip placement="bottom" :delay="250">
            <template #trigger>
              <button type="button" class="terminal-sidebar-reset" @click="resetWidth">
                <n-icon size="14"><RefreshOutline /></n-icon>
              </button>
            </template>
            {{ t('common.reset') }}
          </n-tooltip>
        </div>
      </header>

      <div class="terminal-sidebar-list">
        <TerminalSidebarRow
          v-for="row in rows"
          :key="row.id"
          :row="row"
          :action-options="actionOptions"
          @select="selectRow(row)"
          @action="handleRowAction(row, $event)"
        />
        <div v-if="!rows.length" class="terminal-sidebar-empty">
          {{ t('terminal.emptyGuideTitle') }}
        </div>
      </div>
    </section>
  </aside>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useStorage } from '@vueuse/core';
import { NIcon, NInput, NTooltip, type DropdownOption } from 'naive-ui';
import {
  ChevronForwardOutline,
  CreateOutline,
  CopyOutline,
  RefreshOutline,
  SyncOutline,
} from '@vicons/ionicons5';
import { useDialog, useMessage } from 'naive-ui';
import { useLocale } from '@/composables/useLocale';
import { useSettingsStore } from '@/stores/settings';
import { useProjectStore } from '@/stores/project';
import { useTerminalStore, type TerminalTabState } from '@/stores/terminal';
import { storeToRefs } from 'pinia';
import { getAssistantColorByType, getAssistantIconByType } from '@/utils/assistantIcon';
import { buildProjectBadgeMap } from '@/utils/projectBadge';
import { buildWorkspaceRouteQuery } from '@/utils/workspaceRoute';
import {
  formatWebSessionDateTime,
  formatWebSessionSidebarTime,
} from '@/components/web-session/webSessionTimeFormat';
import TerminalSidebarRow, {
  type TerminalSidebarRowView,
} from '@/components/terminal/TerminalSidebarRow.vue';
import SplitDropdownControl from '@/components/common/SplitDropdownControl.vue';
import { getErrorMessage } from '@/utils/errorHandler';

const props = defineProps<{ projectId: string }>();
const { t, locale } = useLocale();
const message = useMessage();
const dialog = useDialog();
const settingsStore = useSettingsStore();
const { confirmBeforeTerminalClose } = storeToRefs(settingsStore);
const terminalStore = useTerminalStore();
const projectStore = useProjectStore();
const route = useRoute();
const router = useRouter();
const { setActiveTab, getActiveTabId, focusSession, renameSession, closeSession, createSession } =
  terminalStore;
const tabs = computed(() => terminalStore.getTabs(props.projectId));

const SIDEBAR_SCOPE_STORAGE_KEY = 'workspace-terminal-sidebar-scope';
const persistedSidebarScope = useStorage<'current' | 'all'>(SIDEBAR_SCOPE_STORAGE_KEY, 'current');
const sidebarScope = computed<'current' | 'all'>({
  get: () => (persistedSidebarScope.value === 'all' ? 'all' : 'current'),
  set: value => {
    persistedSidebarScope.value = value;
  },
});
const sidebarScopeOptions = computed<DropdownOption[]>(() => [
  { key: 'all', label: t('terminal.sidebarScopeAll') },
  { key: 'current', label: t('terminal.sidebarScopeCurrent') },
]);
const sidebarScopeLabel = computed(() =>
  sidebarScope.value === 'current'
    ? t('terminal.sidebarScopeCurrent')
    : t('terminal.sidebarScopeAll')
);
const sidebarScopeAriaLabel = computed(() =>
  t('terminal.sidebarScopeAria', { scope: sidebarScopeLabel.value })
);
const sidebarScopeTitle = computed(() =>
  t('terminal.sidebarScopeToggle', {
    current: sidebarScopeLabel.value,
    next:
      resolveTerminalSidebarToggleScope(sidebarScope.value) === 'all'
        ? t('terminal.sidebarScopeAll')
        : t('terminal.sidebarScopeCurrent'),
  })
);

function resolveTerminalSidebarToggleScope(scope: 'current' | 'all'): 'current' | 'all' {
  return scope === 'current' ? 'all' : 'current';
}

function toggleSidebarScope() {
  sidebarScope.value = resolveTerminalSidebarToggleScope(sidebarScope.value);
}

function handleSidebarScopeSelect(key: string | number) {
  sidebarScope.value = key === 'all' ? 'all' : 'current';
}

// 「全部终端」需要跨项目会话：进入 all 范围时从服务端拉取各项目终端会话，
// 而不是只显示本会话里打开过的项目（与会话页跨项目列表语义一致）。
const allProjectSessionsLoading = ref(false);
let allProjectSessionsLoadToken = 0;

async function ensureAllProjectSessionsLoaded() {
  const token = ++allProjectSessionsLoadToken;
  allProjectSessionsLoading.value = true;
  try {
    const counts = await terminalStore.loadTerminalCounts();
    if (token !== allProjectSessionsLoadToken) {
      return;
    }
    const projectIds = new Set<string>();
    Object.keys(counts).forEach(projectId => {
      if (Number(counts[projectId]) > 0) {
        projectIds.add(projectId);
      }
    });
    // 补上本会话已加载过、但服务端计数未知的项目
    allProjectTabs.value.forEach(item => projectIds.add(item.projectId));
    await Promise.allSettled(
      [...projectIds].map(projectId => terminalStore.loadSessions(projectId))
    );
  } catch (error) {
    console.error('Failed to load all terminal sessions', error);
  } finally {
    if (token === allProjectSessionsLoadToken) {
      allProjectSessionsLoading.value = false;
    }
  }
}

watch(sidebarScope, scope => {
  if (scope === 'all') {
    void ensureAllProjectSessionsLoaded();
  }
});

const MIN_SIDEBAR_WIDTH = 176;
const MAX_SIDEBAR_WIDTH = 420;
const DEFAULT_SIDEBAR_WIDTH = 252;
const MIN_TERMINAL_MAIN_WIDTH = 420;
const sidebarWidthPx = useStorage('workspace-terminal-sidebar-width', DEFAULT_SIDEBAR_WIDTH);
const rootRef = ref<HTMLElement | null>(null);
const containerWidth = ref(0);
const isResizing = ref(false);

function clamp(min: number, value: number, max: number) {
  return Math.max(min, Math.min(max, value));
}

function updateContainerWidth() {
  const parent = rootRef.value?.parentElement;
  containerWidth.value = parent?.getBoundingClientRect().width ?? 0;
}

const maxWidthByContainer = computed(() => {
  if (!containerWidth.value) {
    return MAX_SIDEBAR_WIDTH;
  }
  return Math.min(
    MAX_SIDEBAR_WIDTH,
    Math.max(MIN_SIDEBAR_WIDTH, containerWidth.value - MIN_TERMINAL_MAIN_WIDTH)
  );
});

const effectiveSidebarWidthPx = computed(() =>
  clamp(MIN_SIDEBAR_WIDTH, Math.round(sidebarWidthPx.value), Math.round(maxWidthByContainer.value))
);

const actionOptions = computed<DropdownOption[]>(() => [
  {
    label: t('terminal.rename'),
    key: 'rename',
    icon: () => hIcon(CreateOutline),
  },
  {
    label: t('terminal.duplicateTab'),
    key: 'duplicate',
    icon: () => hIcon(CopyOutline),
  },
  {
    type: 'divider',
    key: 'divider',
  },
  {
    label: t('terminal.closeTerminal'),
    key: 'close',
    icon: () => hIcon(ChevronForwardOutline),
  },
]);

function hIcon(icon: unknown) {
  return h(NIcon, null, { default: () => h(icon as Component) });
}

type TerminalSidebarEntry = { projectId: string; tab: TerminalTabState };

const allProjectTabs = computed(() => terminalStore.getAllTabs());

const terminalEntries = computed<TerminalSidebarEntry[]>(() => {
  if (sidebarScope.value === 'current') {
    return tabs.value.map(tab => ({ projectId: props.projectId, tab }));
  }
  // 保持加载顺序，不要在切换项目时把当前项目顶到最上
  return allProjectTabs.value.map(item => ({ projectId: item.projectId, tab: item.tab }));
});

const terminalProjectBadges = computed(() =>
  buildProjectBadgeMap(
    [...new Set(terminalEntries.value.map(entry => entry.projectId))],
    projectId => getProjectDisplayName(projectId)
  )
);

function getProjectDisplayName(projectId: string) {
  return projectStore.projects.find(item => item.id === projectId)?.name || projectId;
}

const rows = computed<TerminalSidebarRowView[]>(() =>
  terminalEntries.value.map(entry => {
    const { projectId, tab } = entry;
    const isCurrentProject = projectId === props.projectId;
    const timestamp = parseTimestamp(tab.lastActive || tab.createdAt);
    const agentName = getAgentName(tab);
    const badge = isCurrentProject ? null : (terminalProjectBadges.value.get(projectId) ?? null);
    return {
      id: tab.id,
      projectId,
      title: tab.title || t('terminal.defaultTerminalTitle'),
      agentName,
      iconHtml: getAssistantIconByType(
        tab.aiAssistant?.detected ? tab.aiAssistant.type : undefined
      ),
      iconColor: tab.aiAssistant?.detected
        ? getAssistantColorByType(tab.aiAssistant.type)
        : undefined,
      projectBadge: badge,
      projectName: badge ? getProjectDisplayName(projectId) : undefined,
      active: isCurrentProject && tab.id === getActiveTabId(props.projectId),
      tooltip: [badge ? getProjectDisplayName(projectId) : '', tab.title, agentName, tab.workingDir]
        .filter(Boolean)
        .join(' · '),
      activityTimeLabel: formatWebSessionSidebarTime(timestamp),
      activityTimeTitle: formatWebSessionDateTime(timestamp, locale.value),
    };
  })
);

function parseTimestamp(value?: string) {
  if (!value) {
    return 0;
  }
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function getAgentName(tab: TerminalTabState) {
  return tab.aiAssistant?.detected
    ? tab.aiAssistant.displayName || tab.aiAssistant.name || tab.aiAssistant.type
    : t('terminal.title');
}

function selectRow(row: TerminalSidebarRowView) {
  if (row.projectId === props.projectId) {
    setActiveTab(props.projectId, row.id);
    focusSession(props.projectId, row.id);
    return;
  }
  // 跨项目：激活对应项目的终端会话并跳转到该项目终端页
  setActiveTab(row.projectId, row.id);
  projectStore.addRecentProject(row.projectId);
  void router.push({
    name: 'project',
    params: { id: row.projectId },
    query: buildWorkspaceRouteQuery(route.query, 'terminal'),
  });
}

function promptRename(projectId: string, tab: TerminalTabState) {
  const inputValue = ref(tab.title);
  dialog.create({
    title: t('terminal.renameTitle'),
    content: () =>
      h(NInput, {
        value: inputValue.value,
        'onUpdate:value': (value: string) => {
          inputValue.value = value;
        },
        maxlength: 64,
        autofocus: true,
        placeholder: t('terminal.renamePlaceholder'),
      }),
    positiveText: t('terminal.save'),
    negativeText: t('common.cancel'),
    showIcon: false,
    onPositiveClick: async () => {
      const title = inputValue.value.trim();
      if (!title) {
        message.warning(t('terminal.emptyName'));
        return false;
      }
      if (title === tab.title) {
        return true;
      }
      try {
        await renameSession(projectId, tab.id, title);
        message.success(t('terminal.renameSuccess'));
        return true;
      } catch (error) {
        message.error(getErrorMessage(error, t('terminal.renameFailed')));
        return false;
      }
    },
  });
}

async function handleRowAction(row: TerminalSidebarRowView, key: string | number) {
  const entry = terminalEntries.value.find(item => item.tab.id === row.id);
  if (!entry) {
    return;
  }
  const { projectId, tab } = entry;
  if (key === 'rename') {
    promptRename(projectId, tab);
    return;
  }
  if (key === 'duplicate') {
    try {
      await createSession(projectId, {
        worktreeId: tab.worktreeId,
        workingDir: tab.workingDir,
        title: `${tab.title}${t('terminal.duplicateSuffix')}`,
        rows: tab.rows,
        cols: tab.cols,
        insertAfterSessionId: tab.id,
      });
      message.success(t('terminal.duplicateSuccess'));
    } catch (error) {
      message.error(getErrorMessage(error, t('terminal.duplicateFailed')));
    }
    return;
  }
  if (key === 'close') {
    const close = () => closeSession(projectId, tab.id);
    if (confirmBeforeTerminalClose.value) {
      dialog.warning({
        title: t('terminal.confirmCloseTitle'),
        content: t('terminal.confirmCloseContent', { title: tab.title }),
        positiveText: t('terminal.confirmCloseButton'),
        negativeText: t('common.cancel'),
        onPositiveClick: close,
      });
    } else {
      await close();
    }
  }
}

function resetWidth() {
  sidebarWidthPx.value = DEFAULT_SIDEBAR_WIDTH;
}

function startResize(event: MouseEvent) {
  if (!containerWidth.value) {
    return;
  }
  event.preventDefault();
  isResizing.value = true;
  const startX = event.clientX;
  const startWidth = effectiveSidebarWidthPx.value;
  const onMouseMove = (moveEvent: MouseEvent) => {
    sidebarWidthPx.value = Math.round(
      clamp(MIN_SIDEBAR_WIDTH, startWidth + startX - moveEvent.clientX, maxWidthByContainer.value)
    );
  };
  const onMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  };
  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', onMouseUp);
  document.body.style.cursor = 'col-resize';
  document.body.style.userSelect = 'none';
}

let resizeObserver: ResizeObserver | null = null;
onMounted(() => {
  const parent = rootRef.value?.parentElement;
  if (parent && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(updateContainerWidth);
    resizeObserver.observe(parent);
  }
  updateContainerWidth();
  if (sidebarScope.value === 'all') {
    void ensureAllProjectSessionsLoaded();
  }
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
});

watch(
  () => containerWidth.value,
  () => {
    sidebarWidthPx.value = clamp(
      MIN_SIDEBAR_WIDTH,
      sidebarWidthPx.value,
      maxWidthByContainer.value
    );
  }
);
</script>

<style scoped>
.docked-terminal-sidebar {
  display: flex;
  min-height: 0;
  flex: 0 0 auto;
}

.terminal-sidebar-panel {
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  border-left: 1px solid var(--app-border, var(--n-border-color));
  background: var(--app-surface, var(--app-surface-color, #fff));
  color: var(--app-text-primary, var(--n-text-color-1));
}

.terminal-sidebar-header {
  min-height: 38px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 10px;
  border-bottom: 1px solid
    color-mix(in srgb, var(--app-border, var(--n-border-color)) 70%, transparent);
}

.terminal-sidebar-title-wrap {
  min-width: 0;
}

.terminal-sidebar-header-actions {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 7px;
}

.terminal-sidebar-scope-control {
  flex-shrink: 0;
}

.terminal-sidebar-scope-control:deep(.split-dropdown-control) {
  border-radius: 6px;
}

.terminal-sidebar-scope-control:deep(.split-dropdown-main),
.terminal-sidebar-scope-control:deep(.split-dropdown-menu) {
  height: 28px;
  font-size: 11px;
}

.terminal-sidebar-scope-control:deep(.split-dropdown-main) {
  padding: 0 7px;
  gap: 5px;
}

.terminal-sidebar-scope-control:deep(.split-dropdown-menu) {
  padding: 0 6px;
}

.terminal-sidebar-scope-control:deep(.split-dropdown-icon) {
  width: 12px;
  height: 12px;
}

.terminal-sidebar-scope-control:deep(.split-dropdown-icon svg) {
  width: 12px;
  height: 12px;
}

.terminal-sidebar-count {
  min-width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 4px;
  border-radius: 5px;
  background: var(--app-accent-soft, color-mix(in srgb, var(--n-primary-color) 12%, transparent));
  color: var(--app-accent, var(--n-primary-color));
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}

.terminal-sidebar-loading-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--app-text-muted, var(--n-text-color-3));
  animation: terminal-sidebar-spin 0.9s linear infinite;
}

@keyframes terminal-sidebar-spin {
  to {
    transform: rotate(360deg);
  }
}

.terminal-sidebar-reset {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--app-text-muted, var(--n-text-color-3));
  cursor: pointer;
}

.terminal-sidebar-reset:hover {
  background: var(--app-surface-hover, color-mix(in srgb, var(--n-border-color) 45%, transparent));
  color: var(--app-text-primary, var(--n-text-color-1));
}

.terminal-sidebar-list {
  min-height: 0;
  flex: 1;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding: 7px;
}

.terminal-sidebar-empty {
  min-height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--app-text-muted, var(--n-text-color-3));
  font-size: 12px;
}

.terminal-resizer {
  flex: 0 0 6px;
  width: 6px;
  margin: 0 -3px;
  position: relative;
  z-index: 2;
  cursor: col-resize;
}

.resizer-handle {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 2px;
  height: 24px;
  border-radius: 1px;
  background: transparent;
  opacity: 0;
  transform: translate(-50%, -50%);
  transition:
    background-color 0.15s ease,
    height 0.15s ease,
    opacity 0.15s ease;
}

.terminal-resizer:hover .resizer-handle {
  height: 40px;
  background: var(--app-border-strong, #d0d0d0);
  opacity: 1;
}

.terminal-resizer.is-dragging .resizer-handle {
  height: 60px;
  background: var(--app-accent, #3b82f6);
  opacity: 1;
}
</style>
