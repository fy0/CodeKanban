<template>
  <div
    ref="panelRef"
    class="terminal-panel"
    :class="{
      'is-docked': isDocked,
      'is-mobile': isMobile,
      'is-hidden': hidden,
    }"
  >
    <div v-if="shouldShowBranchFilter" class="branch-filter-strip">
      <button
        type="button"
        class="branch-filter-item"
        :class="{ active: branchFilter === 'all' }"
        @click="handleBranchFilterSelect('all')"
      >
        {{ t('terminal.allBranches') }}
      </button>
      <button
        v-for="option in branchFilterOptions"
        :key="option.id"
        type="button"
        class="branch-filter-item"
        :class="{ active: branchFilter === option.id }"
        @click="handleBranchFilterSelect(option.id)"
      >
        {{ option.label }}
      </button>
    </div>
    <div class="panel-header">
      <!-- 移动端：下拉选择终端 + 上一个/下一个 -->
      <div v-if="isMobile && (tabs.length || emptyTabs.length)" class="mobile-tab-selector">
        <button type="button" class="mobile-nav-btn" :disabled="!hasPrevTab" @click="goToPrevTab">
          <n-icon size="18">
            <ChevronBackOutline />
          </n-icon>
        </button>
        <n-dropdown
          trigger="manual"
          placement="bottom-start"
          :show="showMobileTabSelector"
          :options="mobileTabOptions"
          @select="handleMobileTabSelect"
          @clickoutside="showMobileTabSelector = false"
        >
          <button
            type="button"
            class="mobile-tab-trigger"
            @click="showMobileTabSelector = !showMobileTabSelector"
          >
            <span class="mobile-tab-title">{{ activeTabTitle }}</span>
            <n-icon
              size="16"
              class="mobile-tab-arrow"
              :class="{ 'is-open': showMobileTabSelector }"
            >
              <ChevronDownOutline />
            </n-icon>
          </button>
        </n-dropdown>
        <button type="button" class="mobile-nav-btn" :disabled="!hasNextTab" @click="goToNextTab">
          <n-icon size="18">
            <ChevronForwardOutline />
          </n-icon>
        </button>
      </div>
      <!-- 桌面端：标签页切换 -->
      <div
        v-else-if="tabs.length || emptyTabs.length"
        ref="tabsContainerRef"
        class="tabs-container"
      >
        <n-tabs
          v-model:value="activeId"
          type="card"
          :closable="true"
          size="small"
          :theme-overrides="tabsThemeOverrides"
          @close="handleClose"
        >
          <n-tab-pane
            v-for="tab in visibleTabs"
            :key="tab.id"
            :name="tab.id"
            :tab-props="createTabProps(tab)"
          >
            <template #tab>
              <!-- 空标签的简化显示 -->
              <span v-if="isEmptyTabItem(tab)" class="tab-label">
                <span class="tab-title" :style="tabTitleStyle">
                  {{ tab.title }}
                </span>
              </span>
              <!-- 正常终端标签的完整显示 -->
              <span v-else class="tab-label" :title="getTabTooltip(tab)">
                <span
                  v-if="!hideStatusDots"
                  class="status-dot"
                  :class="(tab as TerminalTabState).clientStatus"
                />
                <span class="tab-title" :style="tabTitleStyle">
                  {{ tab.title }}
                </span>
                <span
                  v-if="(tab as TerminalTabState).renderMode === 'snapshot'"
                  class="tab-render-badge"
                  :title="getSnapshotModeTooltip(tab as TerminalTabState)"
                >
                  {{ t('terminal.snapshotModeTabBadge') }}
                </span>
                <span
                  v-if="showAssistantIdentity(tab as TerminalTabState)"
                  class="agent-identity"
                  :title="getAssistantName(tab as TerminalTabState)"
                >
                  <span
                    class="agent-identity-icon"
                    v-html="getAssistantIcon(tab as TerminalTabState)"
                  ></span>
                  <span class="agent-identity-name">{{
                    getAssistantName(tab as TerminalTabState)
                  }}</span>
                </span>
              </span>
            </template>
          </n-tab-pane>
        </n-tabs>
        <!-- 激活标签指示器 -->
        <div class="active-tab-indicator" :style="activeTabIndicatorStyle"></div>
      </div>
      <div v-else class="empty-tabs-placeholder">
        <span class="empty-tabs-text">{{ t('terminal.emptyGuideTitle') }}</span>
      </div>
      <n-dropdown
        trigger="manual"
        placement="bottom-start"
        :show="!!contextMenuTab"
        :options="contextMenuOptions"
        :x="contextMenuX"
        :y="contextMenuY"
        @select="handleContextMenuSelect"
        @clickoutside="contextMenuTab = null"
      />
      <div class="header-actions">
        <!-- 移动端：切换当前项目 -->
        <n-dropdown
          v-if="isMobile"
          trigger="click"
          placement="bottom-end"
          :options="mobileProjectSwitchOptions"
          @select="handleProjectSwitchSelect"
        >
          <n-button
            text
            size="small"
            class="mobile-project-switch"
            :aria-label="t('terminal.switchProject')"
          >
            <template #icon>
              <span
                v-if="currentProjectBadge"
                class="mobile-project-switch-badge"
                :style="{ background: currentProjectBadge.color }"
              >
                {{ currentProjectBadge.label }}
              </span>
            </template>
            <n-icon size="14"><ChevronDownOutline /></n-icon>
          </n-button>
        </n-dropdown>
        <!-- 创建终端按钮 - 始终显示 -->
        <n-dropdown
          v-if="worktrees.length > 1"
          trigger="manual"
          :show="showCreateTerminalMenu"
          :options="createTerminalOptionsWithHeader"
          @select="handleCreateTerminalSelect"
          @clickoutside="handleCreateTerminalMenuClose"
        >
          <n-tooltip trigger="hover" placement="bottom" :delay="100">
            <template #trigger>
              <n-button text size="small" @click="handleCreateTerminalButtonClick">
                <template #icon>
                  <n-icon>
                    <Add />
                  </n-icon>
                </template>
              </n-button>
            </template>
            {{ t('terminal.createNewTerminal') }}
          </n-tooltip>
        </n-dropdown>
        <n-tooltip v-else trigger="hover" placement="bottom" :delay="100">
          <template #trigger>
            <n-button text size="small" @click="handleCreateTerminalClick">
              <template #icon>
                <n-icon>
                  <Add />
                </n-icon>
              </template>
            </n-button>
          </template>
          {{ t('terminal.createNewTerminal') }}
        </n-tooltip>
        <n-button-group size="small" class="terminal-editor-actions">
          <n-tooltip trigger="hover" placement="bottom" :delay="100">
            <template #trigger>
              <n-button
                text
                size="small"
                :disabled="!canOpenEditor"
                @click="handleEditorButtonClick"
              >
                <template #icon>
                  <n-icon>
                    <CodeSlashOutline />
                  </n-icon>
                </template>
              </n-button>
            </template>
            {{ t('worktree.openWith', { editor: defaultEditorLabel }) }}
          </n-tooltip>
          <n-dropdown
            :options="editorDropdownOptions"
            :disabled="!canOpenEditor"
            @select="handleEditorSelect"
          >
            <n-button text size="small" :disabled="!canOpenEditor">
              <template #icon>
                <n-icon>
                  <ChevronDownOutline />
                </n-icon>
              </template>
            </n-button>
          </n-dropdown>
        </n-button-group>
        <template v-if="enabledQuickActions.length">
          <n-dropdown
            v-if="stackedQuickActions.length"
            trigger="manual"
            :show="showQuickActionsMenu"
            :options="quickActionDropdownOptions"
            @select="handleQuickActionSelect"
            @clickoutside="showQuickActionsMenu = false"
          >
            <n-tooltip trigger="hover" placement="bottom" :delay="100">
              <template #trigger>
                <n-button text size="small" @click="showQuickActionsMenu = !showQuickActionsMenu">
                  <template #icon>
                    <n-icon>
                      <PlayOutline />
                    </n-icon>
                  </template>
                </n-button>
              </template>
              {{ t('terminal.quickActions') }}
            </n-tooltip>
          </n-dropdown>
          <n-tooltip
            v-for="action in standaloneQuickActions"
            :key="action.id"
            trigger="hover"
            placement="bottom"
            :delay="100"
          >
            <template #trigger>
              <n-button text size="small" @click="handleRunQuickAction(action)">
                <template #icon>
                  <span
                    v-if="getQuickActionSvg(action.icon)"
                    class="terminal-quick-action-button-svg"
                    v-html="getQuickActionSvg(action.icon)"
                  ></span>
                  <n-icon v-else>
                    <component :is="resolveQuickActionIcon(action.icon)" />
                  </n-icon>
                </template>
              </n-button>
            </template>
            {{ formatQuickActionLabel(action) }}
          </n-tooltip>
        </template>
        <n-tooltip v-if="projectIdRef" trigger="hover" placement="bottom" :delay="100">
          <template #trigger>
            <n-button text size="small" @click="showAISessionHistory = true">
              <template #icon>
                <n-icon>
                  <TimeOutline />
                </n-icon>
              </template>
            </n-button>
          </template>
          {{ t('terminal.viewAISessions') }}
        </n-tooltip>
        <n-dropdown
          trigger="click"
          placement="bottom-end"
          :show="showSettingsMenu"
          :options="settingsMenuOptions"
          @select="handleSettingsMenuSelect"
          @clickoutside="showSettingsMenu = false"
        >
          <n-tooltip trigger="hover" placement="bottom" :delay="100">
            <template #trigger>
              <n-button text size="small" @click="showSettingsMenu = !showSettingsMenu">
                <template #icon>
                  <n-icon>
                    <SettingsOutline />
                  </n-icon>
                </template>
              </n-button>
            </template>
            {{ t('nav.settings') }}
          </n-tooltip>
        </n-dropdown>
      </div>
    </div>

    <div class="panel-body">
      <!-- 全局空状态：没有任何标签（包括空标签）时显示 -->
      <div v-if="!tabs.length && !emptyTabs.length" class="empty-guide">
        <div class="empty-guide-content">
          <n-icon :size="48" class="empty-guide-icon">
            <TerminalOutline />
          </n-icon>
          <h3 class="empty-guide-title">{{ t('terminal.emptyGuideTitle') }}</h3>
          <p class="empty-guide-description">{{ t('terminal.emptyGuideDescription') }}</p>
          <p class="empty-guide-hint">{{ t('terminal.emptyGuidePasteHint') }}</p>
          <n-dropdown
            v-if="worktrees.length > 1"
            trigger="click"
            :options="createTerminalOptions"
            @select="handleCreateTerminalSelect"
          >
            <n-button type="primary" icon-placement="right">
              {{ t('terminal.createNewTerminal') }}
              <template #icon>
                <n-icon>
                  <ChevronDownOutline />
                </n-icon>
              </template>
            </n-button>
          </n-dropdown>
          <n-button v-else type="primary" @click="handleCreateTerminalClick">
            {{ t('terminal.createNewTerminal') }}
          </n-button>
          <n-button
            v-if="projectIdRef"
            quaternary
            class="view-sessions-btn"
            @click="showAISessionHistory = true"
          >
            {{ t('terminal.viewAISessions') }}
          </n-button>
        </div>
      </div>
      <!-- 空标签内容：当激活的是空标签时显示 -->
      <div
        v-for="emptyTab in emptyTabs"
        v-show="emptyTab.id === activeId"
        :key="emptyTab.id"
        class="empty-guide"
      >
        <div class="empty-guide-content">
          <n-icon :size="48" class="empty-guide-icon">
            <TerminalOutline />
          </n-icon>
          <h3 class="empty-guide-title">{{ t('terminal.emptyGuideTitle') }}</h3>
          <p class="empty-guide-description">{{ t('terminal.emptyGuideDescription') }}</p>
          <p class="empty-guide-hint">{{ t('terminal.emptyGuidePasteHint') }}</p>
          <n-dropdown
            v-if="worktrees.length > 1"
            trigger="click"
            :options="createTerminalOptions"
            @select="handleCreateTerminalSelect"
          >
            <n-button type="primary" icon-placement="right">
              {{ t('terminal.createNewTerminal') }}
              <template #icon>
                <n-icon>
                  <ChevronDownOutline />
                </n-icon>
              </template>
            </n-button>
          </n-dropdown>
          <n-button v-else type="primary" @click="handleCreateTerminalClick">
            {{ t('terminal.createNewTerminal') }}
          </n-button>
          <n-button
            v-if="projectIdRef"
            quaternary
            class="view-sessions-btn"
            @click="showAISessionHistory = true"
          >
            {{ t('terminal.viewAISessions') }}
          </n-button>
        </div>
      </div>
      <!-- 正常终端视图：只渲染非空标签 -->
      <TerminalViewport
        v-for="tab in visibleTabs.filter(t => !isEmptyTabItem(t))"
        v-show="tab.id === activeId"
        :key="tab.id"
        :tab="tab as TerminalTabState"
        :emitter="emitter"
        :send="send"
        :should-auto-focus="shouldAutoFocusTerminal"
        :is-mobile="isMobile"
      />
    </div>
  </div>

  <!-- AI 会话历史对话框 -->
  <AISessionHistoryDialog
    v-if="projectIdRef"
    v-model:show="showAISessionHistory"
    :project-id="projectIdRef"
    :claude-command="resolveAgentCommand('claude')"
    @resume="handleResumeSession"
  />
</template>

<script setup lang="ts">
import {
  computed,
  h,
  nextTick,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
  shallowRef,
  toRef,
  watch,
} from 'vue';
import type { HTMLAttributes } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useDialog, useMessage, NIcon, NInput, NButton, NSpace, NTooltip } from 'naive-ui';
import { useDebounceFn, useEventListener, useResizeObserver, useStorage } from '@vueuse/core';
import {
  ChevronBackOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  TerminalOutline,
  CopyOutline,
  CreateOutline,
  SettingsOutline,
  CheckmarkOutline,
  InformationCircleOutline,
  Add,
  FolderOpenOutline,
  TimeOutline,
  ChatbubblesOutline,
  PlayOutline,
  CodeSlashOutline,
  CodeOutline,
  RocketOutline,
  LogoGoogle,
  LogoGithub,
  NavigateOutline,
  SparklesOutline,
  AlbumsOutline,
} from '@vicons/ionicons5';
import TerminalViewport from './TerminalViewport.vue';
import AISessionHistoryDialog from './AISessionHistoryDialog.vue';
import {
  useTerminalClient,
  type TerminalCreateOptions,
  type TerminalTabState,
  type ServerMessage,
} from '@/composables/useTerminalClient';
import { useAppClipboard } from '@/composables/useAppClipboard';
import type { DropdownOption } from 'naive-ui';
import { useSettingsStore } from '@/stores/settings';
import { useDeveloperConfigStore } from '@/stores/developerConfig';
import { useProjectStore } from '@/stores/project';
import { useTerminalSessionSnapshotStore } from '@/stores/terminalSessionSnapshot';
import {
  DEFAULT_EDITOR,
  EDITOR_LABEL_MAP,
  EDITOR_OPTIONS,
  isEditorPreference,
} from '@/constants/editor';
import {
  TERMINAL_SNAPSHOT_INTERVAL_OPTIONS,
  formatTerminalSnapshotInterval,
  type TerminalRenderMode,
} from '@/constants/terminalRenderMode';
import Sortable, { type SortableEvent } from 'sortablejs';
import { useLocale } from '@/composables/useLocale';
import type {
  EditorPreference,
  TerminalQuickAction,
  TerminalQuickActionIcon,
} from '@/stores/settings';
import { getAssistantIconByType } from '@/utils/assistantIcon';
import { buildProjectBadgeMap, type ProjectBadge } from '@/utils/projectBadge';
import { buildWorkspaceRouteQuery } from '@/utils/workspaceRoute';
import {
  calculateCardTabIndicatorStyle,
  hiddenCardTabIndicatorStyle,
} from '@/utils/cardTabIndicator';

const props = defineProps<{
  projectId: string;
  isMobile?: boolean;
  hidden?: boolean;
}>();

const projectIdRef = toRef(props, 'projectId');
const isMobile = computed(() => Boolean(props.isMobile));
const hidden = computed(() => Boolean(props.hidden));
const isDocked = computed(() => true);
const message = useMessage();
const dialog = useDialog();
const router = useRouter();
const route = useRoute();
const { t } = useLocale();
const { copyText } = useAppClipboard();
const panelRef = ref<HTMLElement | null>(null);
const projectStore = useProjectStore();
const { worktrees } = storeToRefs(projectStore);

// 移动端：切换当前项目
const recentProjectIds = computed(() => projectStore.recentProjects.map(project => project.id));
const projectSwitchBadges = computed(() => {
  const ordered = recentProjectIds.value.slice();
  if (!ordered.includes(projectIdRef.value)) {
    ordered.push(projectIdRef.value);
  }
  return buildProjectBadgeMap(ordered, projectId => {
    const project = projectStore.projects.find(item => item.id === projectId);
    return project?.name?.trim() || projectId;
  });
});
const currentProjectBadge = computed<ProjectBadge | null>(
  () => projectSwitchBadges.value.get(projectIdRef.value) ?? null
);
const mobileProjectSwitchOptions = computed<DropdownOption[]>(() => [
  {
    label: t('terminal.switchProject'),
    key: '__header__',
    disabled: true,
  },
  ...recentProjectIds.value.map(projectId => {
    const badge = projectSwitchBadges.value.get(projectId);
    return {
      label:
        projectStore.projects.find(project => project.id === projectId)?.name?.trim() || projectId,
      key: projectId,
      disabled: projectId === projectIdRef.value,
      icon: badge ? () => renderProjectBadge(badge) : undefined,
    };
  }),
  {
    type: 'divider',
    key: '__divider__',
  },
  {
    label: t('terminal.openProjectList'),
    key: '__open_project_list__',
    icon: () => h(NIcon, null, { default: () => h(AlbumsOutline) }),
  },
]);

function renderProjectBadge(badge: ProjectBadge) {
  return h(
    'span',
    {
      class: 'mobile-project-option-badge',
      style: {
        background: badge.color,
        color: '#ffffff',
        width: '20px',
        height: '20px',
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        borderRadius: '6px',
        fontSize: '10px',
        fontWeight: '700',
        lineHeight: '1',
      } as any,
    },
    badge.label
  );
}

function handleProjectSwitchSelect(key: string | number) {
  if (typeof key !== 'string') {
    return;
  }
  if (key === '__open_project_list__') {
    void router.push({ name: 'projects' });
    return;
  }
  if (key === projectIdRef.value || key === '__header__') {
    return;
  }
  projectStore.addRecentProject(key);
  void router.push({
    name: 'project',
    params: { id: key },
    query: buildWorkspaceRouteQuery(route.query, 'terminal'),
  });
}
const sessionSnapshotStore = useTerminalSessionSnapshotStore();
const { sessionsById: terminalSessionsById } = storeToRefs(sessionSnapshotStore);
const terminalSessionSnapshotScopeId = `terminal-panel-${Math.random().toString(36).slice(2, 8)}`;
const expanded = ref(true);
const panelHeight = ref(470);
const panelLeft = ref(220);
const panelRight = ref(170);
const panelBottom = ref(12);
const mobilePanelTop = ref(15);
const autoResize = useStorage('terminal-auto-resize', true);
const sendResizeOnSwitch = useStorage('terminal-send-resize-on-switch', true);
const showBranchFilter = useStorage('terminal-show-branch-filter', true);
const panelSize = reactive({
  width: 0,
  height: 0,
});
const isResizing = ref(false);
const shouldAutoFocusTerminal = ref(true);
const developerConfigStore = useDeveloperConfigStore();
const { config: developerConfigState } = storeToRefs(developerConfigStore);

// 右键菜单相关状态
const contextMenuTab = ref<string | null>(null);
const contextMenuX = ref(0);
const contextMenuY = ref(0);

// AI 会话历史对话框状态
const showAISessionHistory = ref(false);

// 空终端标签状态
const EMPTY_TAB_PREFIX = 'empty-';
let emptyTabCounter = 0;

interface EmptyTab {
  id: string;
  title: string;
  isEmpty: true;
}

const emptyTabs = ref<EmptyTab[]>([]);

function createEmptyTab() {
  emptyTabCounter += 1;
  const newTab: EmptyTab = {
    id: `${EMPTY_TAB_PREFIX}${emptyTabCounter}`,
    title: t('terminal.emptyGuideTitle'),
    isEmpty: true,
  };
  emptyTabs.value.push(newTab);
  expanded.value = true;
  activeId.value = newTab.id;
}

function closeEmptyTab(tabId: string) {
  const index = emptyTabs.value.findIndex(tab => tab.id === tabId);
  if (index !== -1) {
    emptyTabs.value.splice(index, 1);
    // 如果当前激活的是被关闭的空标签，切换到其他标签
    if (activeId.value === tabId) {
      const nextTab = tabs.value[0] || emptyTabs.value[0];
      activeId.value = nextTab?.id || '';
    }
  }
}

function isEmptyTab(tabId: string): boolean {
  return tabId.startsWith(EMPTY_TAB_PREFIX);
}

const snapshotModeSupported = computed(
  () => developerConfigState.value.enableTerminalStateSnapshot
);

function formatSnapshotIntervalLabel(intervalMs: number | null | undefined) {
  return formatTerminalSnapshotInterval(intervalMs);
}

function getSnapshotModeTooltip(tab: TerminalTabState) {
  return t('terminal.snapshotModeTooltip', {
    interval: formatSnapshotIntervalLabel(tab.snapshotIntervalMs),
  });
}

function buildSnapshotModeMenuOptions(tab: TerminalTabState | null | undefined): DropdownOption[] {
  const globalIntervalLabel = formatSnapshotIntervalLabel(defaultTerminalSnapshotIntervalMs.value);
  const globalRenderModeLabel = t(
    defaultTerminalRenderMode.value === 'snapshot'
      ? 'terminal.snapshotModeGlobalSnapshot'
      : 'terminal.snapshotModeGlobalLive'
  );
  const intervalOptions: DropdownOption[] = TERMINAL_SNAPSHOT_INTERVAL_OPTIONS.map(interval => ({
    label: formatSnapshotIntervalLabel(interval),
    key: `snapshot-interval:${interval}`,
    icon:
      tab &&
      tab.renderMode === 'snapshot' &&
      !tab.useGlobalSnapshotInterval &&
      tab.snapshotIntervalMs === interval
        ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
        : undefined,
  }));

  intervalOptions.push({
    type: 'divider',
    key: 'snapshot-interval-divider',
  });
  intervalOptions.push({
    label: t('terminal.useGlobalSnapshotInterval', { interval: globalIntervalLabel }),
    key: 'snapshot-interval:global',
    icon: tab?.useGlobalSnapshotInterval
      ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
      : undefined,
  });

  return [
    {
      label: t('terminal.enableSnapshotMode'),
      key: 'snapshot-mode:enable',
      icon:
        tab?.renderMode === 'snapshot'
          ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
          : undefined,
      disabled: !snapshotModeSupported.value,
    },
    {
      label: t('terminal.disableSnapshotMode'),
      key: 'snapshot-mode:disable',
      icon:
        tab?.renderMode === 'live'
          ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
          : undefined,
    },
    {
      label: t('terminal.useGlobalRenderMode', { mode: globalRenderModeLabel }),
      key: 'snapshot-mode:global',
      icon: tab?.useGlobalRenderMode
        ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
        : undefined,
    },
    {
      type: 'divider',
      key: 'snapshot-mode-divider',
    },
    {
      label: t('terminal.snapshotRefreshInterval'),
      key: 'snapshot-interval',
      disabled: !snapshotModeSupported.value,
      children: intervalOptions,
    },
  ];
}

const contextMenuOptions = computed<DropdownOption[]>(() => {
  const tabId = contextMenuTab.value;
  const tab = tabId ? tabs.value.find(t => t.id === tabId) : null;
  const hasProcessInfo = tab?.processPid != null;
  const canOpenEditorForTab = Boolean(resolveEditorPath(tab));
  const editorMenuOptions = editorOptions.value.map(option => ({
    label: option.label,
    key: `open-editor:${option.value}`,
    disabled: !canOpenEditorForTab || option.disabled,
  }));

  const options: DropdownOption[] = [
    {
      label: t('terminal.duplicateTab'),
      key: 'duplicate',
      icon: () => h(NIcon, null, { default: () => h(CopyOutline) }),
    },
    {
      label: t('terminal.rename'),
      key: 'rename',
      icon: () => h(NIcon, null, { default: () => h(CreateOutline) }),
    },
    {
      label: t('terminal.snapshotMode'),
      key: 'snapshot-mode',
      icon: () => h(NIcon, null, { default: () => h(AlbumsOutline) }),
      children: buildSnapshotModeMenuOptions(tab),
    },
    {
      label: t('terminal.copyProcessInfo'),
      key: 'copy-process-info',
      icon: () => h(NIcon, null, { default: () => h(InformationCircleOutline) }),
      disabled: !hasProcessInfo,
    },
    {
      label: t('terminal.copyPath'),
      key: 'copy-path',
      icon: () => h(NIcon, null, { default: () => h(CopyOutline) }),
    },
    {
      label: t('terminal.browseDirectory'),
      key: 'browse-directory',
      icon: () => h(NIcon, null, { default: () => h(FolderOpenOutline) }),
    },
    {
      label: t('worktree.openWith', { editor: defaultEditorLabel.value }),
      key: 'open-editor',
      icon: () => h(NIcon, null, { default: () => h(CodeSlashOutline) }),
      disabled: !canOpenEditorForTab,
      children: editorMenuOptions,
    },
    {
      type: 'divider',
      key: 'close-tabs-divider',
    },
    {
      label: t('terminal.closeRightTabs'),
      key: 'close-right-tabs',
      icon: () => h(NIcon, null, { default: () => h(ChevronForwardOutline) }),
      disabled: !tab || tabs.value.indexOf(tab) >= tabs.value.length - 1,
    },
  ];

  return options;
});

// 创建终端下拉菜单相关状态
const showCreateTerminalMenu = ref(false);
const createTerminalMenuClosedAt = ref(0); // 记录菜单关闭时间

// 设置菜单相关状态
const showSettingsMenu = ref(false);

const isFullscreen = ref(false);
const savedPanelState = ref<{ left: number; right: number; bottom: number; height: number } | null>(
  null
);

watch(
  isDocked,
  docked => {
    if (docked && isFullscreen.value) {
      isFullscreen.value = false;
      savedPanelState.value = null;
    }
  },
  { immediate: true }
);

const settingsMenuOptions = computed<DropdownOption[]>(() => [
  {
    label: t('terminal.autoResize'),
    key: 'auto-resize',
    icon: autoResize.value
      ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
      : undefined,
  },
  {
    label: t('terminal.sendResizeOnSwitch'),
    key: 'send-resize-on-switch',
    icon: sendResizeOnSwitch.value
      ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
      : undefined,
  },
  {
    label: t('terminal.confirmClose'),
    key: 'confirm-close',
    icon: confirmBeforeTerminalClose.value
      ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
      : undefined,
  },
  {
    label: t('terminal.showBranchFilter'),
    key: 'branch-filter-toggle',
    icon: showBranchFilter.value
      ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
      : undefined,
  },
  {
    label: t('terminal.defaultOpenInMirrorMode'),
    key: 'default-open-in-mirror-mode',
    icon:
      defaultTerminalRenderMode.value === 'snapshot'
        ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
        : undefined,
  },
]);

async function ensureDeveloperConfigLoaded(force = false) {
  try {
    await developerConfigStore.load(force);
    return true;
  } catch (error) {
    console.error('Failed to load developer config', error);
    message.error(t('common.loadFailed'));
    return false;
  }
}

// 创建终端下拉菜单选项
const createTerminalOptions = computed<DropdownOption[]>(() => {
  return worktrees.value.map(worktree => ({
    label: worktree.branchName,
    key: worktree.id,
  }));
});

// 创建终端下拉菜单选项（带提示头）
const EMPTY_TAB_KEY = '__empty_tab__';

const createTerminalOptionsWithHeader = computed<DropdownOption[]>(() => {
  return [
    {
      label: t('terminal.createNewTerminal'),
      key: 'header',
      disabled: true,
      type: 'render',
      render: () =>
        h(
          'div',
          {
            style: {
              color: 'var(--n-text-color-3, #999)',
              fontSize: '12px',
              fontWeight: '500',
              padding: '8px 12px 4px 12px',
              borderBottom: '1px solid var(--n-divider-color, #eee)',
              marginBottom: '4px',
              cursor: 'default',
              userSelect: 'none',
            },
          },
          t('terminal.createNewTerminal')
        ),
    },
    ...worktrees.value.map(worktree => ({
      label: worktree.branchName,
      key: worktree.id,
    })),
    {
      key: 'divider',
      type: 'divider',
    },
    {
      label: t('terminal.emptyTab'),
      key: EMPTY_TAB_KEY,
    },
  ];
});

const MIN_HEIGHT = 40; // 只保留一条缝
const MAX_HEIGHT = 800;
const MIN_MARGIN = 12;
const MAX_MARGIN_PERCENT = 0.4; // 最大边距占窗口宽度的40%
const MIN_PANEL_WIDTH = 375; // 终端面板最小宽度
const DUPLICATE_SUFFIX = computed(() => t('terminal.duplicateSuffix'));
const MAX_TAB_TITLE_WIDTH = 160;
const TAB_LABEL_EXTRA_SPACE = 40;
const TABS_CONTAINER_STATIC_OFFSET = 320;
const TABS_CONTAINER_MIN_OFFSET = 200;
const SHARED_WIDTH_HIDE_THRESHOLD = 1000;

const {
  tabs,
  activeTabId,
  emitter,
  reloadSessions,
  createSession,
  renameSession,
  closeSession,
  send,
  disconnectTab,
  reorderTabs: reorderTabsInStore,
  setRenderMode,
  setSnapshotInterval,
  focusSession: focusSessionInStore,
} = useTerminalClient(projectIdRef);

const settingsStore = useSettingsStore();
const {
  maxTerminalsPerProject,
  confirmBeforeTerminalClose,
  activeTheme,
  terminalQuickActions,
  editorSettings,
  defaultTerminalRenderMode,
  defaultTerminalSnapshotIntervalMs,
} = storeToRefs(settingsStore);

const defaultEditorPreference = computed<EditorPreference>(() =>
  editorSettings.value?.defaultEditor && isEditorPreference(editorSettings.value.defaultEditor)
    ? editorSettings.value.defaultEditor
    : DEFAULT_EDITOR
);
const customEditorCommand = computed(() => editorSettings.value?.customCommand?.trim() ?? '');
const editorOptions = computed(() =>
  EDITOR_OPTIONS.map(option => ({
    ...option,
    disabled: option.value === 'custom' && !customEditorCommand.value,
  }))
);
const editorDropdownOptions = computed<DropdownOption[]>(() =>
  editorOptions.value.map(option => ({
    label: option.label,
    key: option.value,
    disabled: option.disabled,
  }))
);
const defaultEditorLabel = computed(
  () => EDITOR_LABEL_MAP[defaultEditorPreference.value] ?? t('worktree.editor')
);

// Tabs 主题覆盖 - 用于控制标签背景色
const tabsThemeOverrides = computed(() => {
  const theme = activeTheme.value;
  const tabBg = theme.terminalTabBg || theme.bodyColor;
  const tabActiveBg = theme.terminalTabActiveBg || theme.surfaceColor;

  return {
    tabColor: tabBg,
    tabColorSegment: tabActiveBg,
  };
});

const terminalLimit = computed(() => Math.max(maxTerminalsPerProject.value || 1, 1));
const isTerminalLimitReached = computed(() => tabs.value.length >= terminalLimit.value);

const tabsContainerRef = ref<HTMLElement | null>(null);
const tabsContainerWidth = ref(0);
const tabTitleMaxWidth = ref(MAX_TAB_TITLE_WIDTH);
const hideStatusDots = ref(false);
const tabTitleStyle = computed(() => ({
  maxWidth: `${tabTitleMaxWidth.value}px`,
}));
const tabDragSortable = shallowRef<Sortable | null>(null);
const refreshTabSortable = useDebounceFn(() => {
  nextTick(() => {
    setupTabSorting();
  });
}, 100);

const worktreeBranchMap = computed(() => {
  const map = new Map<string, string>();
  worktrees.value.forEach(worktree => {
    map.set(worktree.id, worktree.branchName || '');
  });
  return map;
});

// 分支过滤器按项目持久化存储
const BRANCH_FILTER_STORAGE_KEY = 'terminal-branch-filter-by-project';

function loadBranchFilterMap(): Map<string, string> {
  if (typeof window === 'undefined' || !window.localStorage) {
    return new Map();
  }
  try {
    const raw = window.localStorage.getItem(BRANCH_FILTER_STORAGE_KEY);
    if (!raw) {
      return new Map();
    }
    const parsed = JSON.parse(raw) as Record<string, string>;
    const result = new Map<string, string>();
    Object.entries(parsed).forEach(([projectId, value]) => {
      if (projectId && typeof value === 'string') {
        result.set(projectId, value);
      }
    });
    return result;
  } catch {
    return new Map();
  }
}

function saveBranchFilterMap(map: Map<string, string>) {
  if (typeof window === 'undefined' || !window.localStorage) {
    return;
  }
  if (!map.size) {
    window.localStorage.removeItem(BRANCH_FILTER_STORAGE_KEY);
    return;
  }
  const payload: Record<string, string> = {};
  map.forEach((value, projectId) => {
    if (value && value !== 'all') {
      payload[projectId] = value;
    }
  });
  if (Object.keys(payload).length === 0) {
    window.localStorage.removeItem(BRANCH_FILTER_STORAGE_KEY);
    return;
  }
  window.localStorage.setItem(BRANCH_FILTER_STORAGE_KEY, JSON.stringify(payload));
}

const branchFilterMap = loadBranchFilterMap();

function getStoredBranchFilter(projectId: string): string {
  return branchFilterMap.get(projectId) || 'all';
}

function saveCurrentBranchFilter(projectId: string, value: string) {
  if (value === 'all') {
    branchFilterMap.delete(projectId);
  } else {
    branchFilterMap.set(projectId, value);
  }
  saveBranchFilterMap(branchFilterMap);
}

const branchFilter = ref<string>(getStoredBranchFilter(props.projectId));
const lastActiveBeforeFilter = ref<string>('');

const branchFilterOptions = computed(() => {
  const seen = new Map<string, { id: string; label: string }>();
  tabs.value.forEach(tab => {
    const key = tab.worktreeId;
    if (!key || seen.has(key)) {
      return;
    }
    seen.set(key, {
      id: key,
      label: resolveWorktreeBranchName(key),
    });
  });
  return Array.from(seen.values());
});

const shouldShowBranchFilter = computed(
  () => showBranchFilter.value && branchFilterOptions.value.length > 1
);

// 合并真实终端标签和空标签
type CombinedTab = TerminalTabState | EmptyTab;

function resolveDisplayTab(tab: TerminalTabState): TerminalTabState {
  const snapshot = terminalSessionsById.value.get(tab.id);
  if (!snapshot) {
    return tab;
  }
  return {
    ...tab,
    ...snapshot,
    projectId: tab.projectId || snapshot.projectId,
    clientStatus: tab.clientStatus,
    connectionRole: tab.connectionRole,
    renderMode: tab.renderMode,
    connectionRenderMode: tab.connectionRenderMode,
    snapshotIntervalMs: tab.snapshotIntervalMs,
    useGlobalRenderMode: tab.useGlobalRenderMode,
    useGlobalSnapshotInterval: tab.useGlobalSnapshotInterval,
  };
}

const displayTabs = computed(() => tabs.value.map(resolveDisplayTab));

const visibleTabs = computed<CombinedTab[]>(() => {
  let realTabs: TerminalTabState[];
  if (branchFilter.value === 'all') {
    realTabs = displayTabs.value;
  } else {
    realTabs = displayTabs.value.filter(tab => tab.worktreeId === branchFilter.value);
  }
  // 将空标签和真实标签合并，空标签放在最后
  return [...realTabs, ...emptyTabs.value];
});

// 移动端下拉选择终端的选项
const mobileTabOptions = computed<DropdownOption[]>(() => {
  return visibleTabs.value.map(tab => ({
    label: tab.title,
    key: tab.id,
  }));
});

// 当前激活的终端标题
const activeTabTitle = computed(() => {
  const tab = visibleTabs.value.find(t => t.id === activeId.value);
  return tab?.title || t('terminal.emptyGuideTitle');
});

// 移动端下拉选中处理
const showMobileTabSelector = ref(false);
function handleMobileTabSelect(key: string) {
  activeId.value = key;
  showMobileTabSelector.value = false;
}

// 移动端上一个/下一个终端
const currentTabIndex = computed(() => {
  return visibleTabs.value.findIndex(t => t.id === activeId.value);
});

const hasPrevTab = computed(() => currentTabIndex.value > 0);
const hasNextTab = computed(() => currentTabIndex.value < visibleTabs.value.length - 1);

function goToPrevTab() {
  if (hasPrevTab.value) {
    activeId.value = visibleTabs.value[currentTabIndex.value - 1].id;
  }
}

function goToNextTab() {
  if (hasNextTab.value) {
    activeId.value = visibleTabs.value[currentTabIndex.value + 1].id;
  }
}

// 检查是否是空标签
function isEmptyTabItem(tab: CombinedTab): tab is EmptyTab {
  return 'isEmpty' in tab && tab.isEmpty === true;
}

function resolveWorktreeBranchName(worktreeId: string) {
  const label = worktreeBranchMap.value.get(worktreeId)?.trim();
  if (label) {
    return label;
  }
  return t('terminal.unknownBranch');
}

function handleBranchFilterSelect(value: string) {
  if (branchFilter.value === value) {
    return;
  }
  branchFilter.value = value;
  // 保存当前项目的分支过滤器设置
  saveCurrentBranchFilter(props.projectId, value);
}

// 激活标签指示器的位置和宽度
const activeTabIndicatorStyle = ref(hiddenCardTabIndicatorStyle());

// 更新激活标签指示器的位置
function updateActiveTabIndicator() {
  nextTick(() => {
    activeTabIndicatorStyle.value = activeId.value
      ? calculateCardTabIndicatorStyle(tabsContainerRef.value)
      : hiddenCardTabIndicatorStyle();
  });
}

// 本地激活的空终端 ID
const localActiveEmptyTabId = ref<string>('');

const activeId = computed({
  get: () => {
    // 如果有本地激活的空终端，优先返回
    if (
      localActiveEmptyTabId.value &&
      emptyTabs.value.some(t => t.id === localActiveEmptyTabId.value)
    ) {
      return localActiveEmptyTabId.value;
    }
    return activeTabId.value;
  },
  set: value => {
    // 检查是否是空终端 ID
    if (isEmptyTab(value)) {
      localActiveEmptyTabId.value = value;
    } else {
      // 切换到真实终端时，清除空终端激活状态
      localActiveEmptyTabId.value = '';
      activeTabId.value = value;
    }
  },
});

function resolveEditorPath(tab: TerminalTabState | null | undefined): string {
  if (!tab) {
    return '';
  }
  const worktreePath = worktrees.value.find(worktree => worktree.id === tab.worktreeId)?.path;
  if (worktreePath && worktreePath.trim()) {
    return worktreePath.trim();
  }
  return tab.workingDir?.trim() ?? '';
}

const activeTerminalTab = computed(
  () => displayTabs.value.find(tab => tab.id === activeId.value) ?? null
);
const activeEditorPath = computed(() => resolveEditorPath(activeTerminalTab.value));
const canOpenEditor = computed(() => Boolean(activeEditorPath.value));

function ensureActiveTabMatchesFilter() {
  const allTabs = tabs.value;
  if (!allTabs.length) {
    lastActiveBeforeFilter.value = '';
    return;
  }
  if (branchFilter.value === 'all') {
    if (lastActiveBeforeFilter.value) {
      const stored = allTabs.find(tab => tab.id === lastActiveBeforeFilter.value);
      if (stored) {
        activeId.value = stored.id;
      } else if (!allTabs.some(tab => tab.id === activeId.value)) {
        activeId.value = allTabs[0].id;
      }
    } else if (!allTabs.some(tab => tab.id === activeId.value)) {
      activeId.value = allTabs[0].id;
    }
    lastActiveBeforeFilter.value = '';
    return;
  }
  const visible = visibleTabs.value;
  if (!visible.length) {
    branchFilter.value = 'all';
    saveCurrentBranchFilter(props.projectId, 'all');
    return;
  }
  if (!visible.some(tab => tab.id === activeId.value)) {
    activeId.value = visible[0].id;
  }
}

function recalcTabTitleWidth(explicitWidth?: number) {
  if (typeof explicitWidth === 'number') {
    tabsContainerWidth.value = explicitWidth;
  }
  const containerWidth =
    typeof explicitWidth === 'number' ? explicitWidth : tabsContainerWidth.value;
  if (!containerWidth) {
    tabTitleMaxWidth.value = MAX_TAB_TITLE_WIDTH;
    return;
  }
  const tabCount = Math.max(visibleTabs.value.length, 1);
  let activeOffset = TABS_CONTAINER_STATIC_OFFSET;
  if (containerWidth - activeOffset < SHARED_WIDTH_HIDE_THRESHOLD) {
    activeOffset = TABS_CONTAINER_MIN_OFFSET;
  }
  const availableWidth = Math.max(containerWidth - activeOffset, 0);
  hideStatusDots.value = availableWidth < SHARED_WIDTH_HIDE_THRESHOLD;
  const rawWidth = availableWidth / tabCount - TAB_LABEL_EXTRA_SPACE;
  const constrainedWidth = Math.min(MAX_TAB_TITLE_WIDTH, Math.max(0, rawWidth));
  tabTitleMaxWidth.value = Math.round(constrainedWidth);
}

useResizeObserver(tabsContainerRef, entries => {
  const entry = entries[0];
  if (!entry) {
    return;
  }
  const width = entry.contentRect.width;
  if (width !== tabsContainerWidth.value) {
    recalcTabTitleWidth(width);
    updateActiveTabIndicator();
  }
});

watch(
  () => visibleTabs.value.length,
  () => {
    nextTick(() => {
      recalcTabTitleWidth();
      updateActiveTabIndicator();
    });
    refreshTabSortable();
  }
);

watch(
  tabs,
  () => {
    if (!tabs.value.length) {
      lastActiveBeforeFilter.value = '';
      if (branchFilter.value !== 'all') {
        branchFilter.value = 'all';
        saveCurrentBranchFilter(props.projectId, 'all');
      }
    } else {
      ensureActiveTabMatchesFilter();
    }
    nextTick(() => {
      updateActiveTabIndicator();
    });
  },
  { deep: true }
);

watch(
  () => expanded.value,
  value => {
    if (value) {
      nextTick(() => {
        recalcTabTitleWidth();
        updateActiveTabIndicator();
      });
      refreshTabSortable();
    } else {
      destroyTabSorting();
    }
  }
);

watch(
  () => tabsContainerRef.value,
  element => {
    if (element) {
      refreshTabSortable();
      setupTabScrollListener();
    } else {
      destroyTabSorting();
      cleanupTabScrollListener();
    }
  }
);

watch(branchFilter, (next, prev) => {
  if (next !== 'all' && prev === 'all') {
    lastActiveBeforeFilter.value = activeId.value;
  }
  ensureActiveTabMatchesFilter();
  nextTick(() => {
    recalcTabTitleWidth();
    updateActiveTabIndicator();
  });
  refreshTabSortable();
});

// 项目切换时恢复对应的分支过滤器设置
watch(projectIdRef, (nextProjectId, prevProjectId) => {
  if (nextProjectId && nextProjectId !== prevProjectId) {
    const storedFilter = getStoredBranchFilter(nextProjectId);
    // 检查存储的过滤器值是否仍然有效（对应的分支是否还存在）
    if (storedFilter !== 'all') {
      // 延迟检查，等待 tabs 数据加载
      nextTick(() => {
        const validOptions = branchFilterOptions.value.map(opt => opt.id);
        if (validOptions.includes(storedFilter)) {
          branchFilter.value = storedFilter;
        } else {
          branchFilter.value = 'all';
        }
      });
    } else {
      branchFilter.value = 'all';
    }
  }
});

watch(
  () => tabs.value.length,
  length => {
    if (length <= 1 && branchFilter.value !== 'all') {
      branchFilter.value = 'all';
      saveCurrentBranchFilter(props.projectId, 'all');
    }
  }
);

nextTick(() => {
  recalcTabTitleWidth();
});

// 监听标签滚动以更新指示器位置
let tabScrollContainer: HTMLElement | null = null;

function setupTabScrollListener() {
  nextTick(() => {
    const container = tabsContainerRef.value;
    if (!container) return;
    // NaiveUI 使用 .v-x-scroll 作为滚动容器
    const scrollContainer = container.querySelector('.v-x-scroll') as HTMLElement | null;
    if (scrollContainer) {
      tabScrollContainer = scrollContainer;
      scrollContainer.addEventListener('scroll', updateActiveTabIndicator);
    }
  });
}

function cleanupTabScrollListener() {
  if (tabScrollContainer) {
    tabScrollContainer.removeEventListener('scroll', updateActiveTabIndicator);
    tabScrollContainer = null;
  }
}

onMounted(() => {
  refreshTabSortable();
  updateActiveTabIndicator();
  setupTabScrollListener();
  void ensureDeveloperConfigLoaded();
});

onBeforeUnmount(() => {
  destroyTabSorting();
  cleanupTabScrollListener();
  sessionSnapshotStore.releaseScope(terminalSessionSnapshotScopeId);
});

watch(
  projectIdRef,
  projectId => {
    if (!projectId) {
      sessionSnapshotStore.releaseScope(terminalSessionSnapshotScopeId);
      return;
    }
    sessionSnapshotStore.retainScope(terminalSessionSnapshotScopeId, [projectId]);
  },
  { immediate: true }
);

function triggerGlobalTerminalRefresh() {
  window.setTimeout(() => {
    emitter.emit('terminal-resize-all');
  }, 60);
}

function handleWindowFocusRefresh() {
  if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
    return;
  }
  triggerGlobalTerminalRefresh();
}

function handleDocumentVisibilityRefresh() {
  if (typeof document === 'undefined' || document.visibilityState !== 'visible') {
    return;
  }
  triggerGlobalTerminalRefresh();
}

if (typeof window !== 'undefined') {
  useEventListener(window, 'focus', handleWindowFocusRefresh);
  useEventListener(window, 'pageshow', handleWindowFocusRefresh);
  if (typeof document !== 'undefined') {
    useEventListener(document, 'visibilitychange', handleDocumentVisibilityRefresh);
  }
}

function setupTabSorting() {
  const container = tabsContainerRef.value;
  if (!container || visibleTabs.value.length <= 1) {
    destroyTabSorting();
    return;
  }
  const wrapper = container.querySelector('.n-tabs-wrapper') as HTMLElement | null;
  if (!wrapper) {
    destroyTabSorting();
    return;
  }
  if (tabDragSortable.value) {
    if (tabDragSortable.value.el === wrapper) {
      tabDragSortable.value.option('disabled', visibleTabs.value.length <= 1);
      return;
    }
    destroyTabSorting();
  }
  tabDragSortable.value = Sortable.create(wrapper, {
    animation: 150,
    direction: 'horizontal',
    draggable: '.n-tabs-tab-wrapper',
    handle: '.n-tabs-tab',
    filter: '.n-tabs-tab__close',
    preventOnFilter: false,
    ghostClass: 'terminal-tab-ghost',
    chosenClass: 'terminal-tab-chosen',
    dragClass: 'terminal-tab-dragging',
    onEnd: handleTabDragEnd,
  });
  tabDragSortable.value.option('disabled', visibleTabs.value.length <= 1);
}

function destroyTabSorting() {
  if (tabDragSortable.value) {
    tabDragSortable.value.destroy();
    tabDragSortable.value = null;
  }
}

function handleTabDragEnd(event: SortableEvent) {
  const fromIndex = event.oldDraggableIndex ?? event.oldIndex ?? -1;
  const toIndex = event.newDraggableIndex ?? event.newIndex ?? -1;
  if (fromIndex === -1 || toIndex === -1 || fromIndex === toIndex) {
    return;
  }
  const visible = visibleTabs.value;
  const fromTab = visible[fromIndex];
  const toTab = visible[toIndex];
  if (!fromTab || !toTab) {
    return;
  }
  const absoluteFromIndex = tabs.value.findIndex(tab => tab.id === fromTab.id);
  const absoluteToIndex = tabs.value.findIndex(tab => tab.id === toTab.id);
  if (absoluteFromIndex === -1 || absoluteToIndex === -1) {
    return;
  }
  reorderTabsInStore(absoluteFromIndex, absoluteToIndex);
  nextTick(() => {
    scheduleResizeAll();
    updateActiveTabIndicator();
  });
}

// 节流的终端 resize 函数 - 只 resize 当前活动的终端，避免影响隐藏标签的滚动位置
const scheduleResizeAll = useDebounceFn(() => {
  if (autoResize.value && expanded.value && activeId.value) {
    emitter.emit(`terminal-resize-${activeId.value}`);
  }
}, 150);

const scheduleActiveTabResize = useDebounceFn((tabId: string) => {
  if (autoResize.value && expanded.value && tabId) {
    emitter.emit(`terminal-resize-${tabId}`);
  }
}, 150);

useResizeObserver(panelRef, entries => {
  const entry = entries[0];
  if (!entry) {
    return;
  }
  const { width, height } = entry.contentRect;
  const roundedWidth = Math.round(width);
  const roundedHeight = Math.round(height);
  if (roundedWidth === panelSize.width && roundedHeight === panelSize.height) {
    return;
  }
  panelSize.width = roundedWidth;
  panelSize.height = roundedHeight;
  scheduleResizeAll();
});

// 切换标签时强制发送 resize，不受 autoResize 设置影响
const forceResizeTab = (tabId: string) => {
  if (expanded.value && tabId) {
    emitter.emit(`terminal-resize-${tabId}`);
  }
};

// 移除自动收缩逻辑，让用户手动控制展开/收缩状态
// 这样切换项目时不会自动收缩面板

// 监听面板高度变化，自动调整终端大小
watch(
  [panelHeight, panelLeft, panelRight, expanded],
  () => {
    nextTick(() => {
      scheduleResizeAll();
    });
  },
  { flush: 'post' }
);

// 监听标签页切换，立即刷新终端尺寸
watch(
  activeId,
  (newId, oldId) => {
    console.log('[Terminal Panel] Tab switched:', { from: oldId, to: newId });
    if (!newId) {
      return;
    }

    // Update active tab indicator
    updateActiveTabIndicator();

    // 通知终端已激活（用于首次访问时滚动到底部）
    nextTick(() => {
      emitter.emit(`terminal-activated-${newId}`);
    });

    // 根据设置决定是否在切换标签时发送 resize 指令
    if (sendResizeOnSwitch.value) {
      nextTick(() => {
        console.log('[Terminal Panel] Forcing resize for switched terminal:', newId);
        forceResizeTab(newId);
      });
    }
  },
  { flush: 'post' }
);

type ToggleOptions = {
  skipFocus?: boolean;
};

function isToggleOptions(value: unknown): value is ToggleOptions {
  return Boolean(value && typeof value === 'object' && 'skipFocus' in value);
}

function toggleExpanded(arg?: ToggleOptions | MouseEvent) {
  const options = isToggleOptions(arg) ? arg : undefined;
  const willExpand = !expanded.value;
  if (willExpand) {
    shouldAutoFocusTerminal.value = !options?.skipFocus;
  } else {
    emitter.emit('terminal-blur-all');
  }
  expanded.value = !expanded.value;
  // 展开时触发 resize，确保终端尺寸正确
  if (expanded.value) {
    nextTick(() => {
      scheduleResizeAll();
    });
  }
}

function ensureExpanded(options?: ToggleOptions) {
  if (expanded.value) {
    return;
  }
  toggleExpanded(options);
}

function expand(options?: ToggleOptions) {
  if (!expanded.value) {
    toggleExpanded(options);
  }
}

function collapse() {
  if (expanded.value) {
    toggleExpanded();
  }
}

type EnsureExpandedEvent = ToggleOptions & { projectId?: string };

function startResize(event: MouseEvent) {
  if (!expanded.value) return;

  event.preventDefault();
  isResizing.value = true;

  const startY = event.clientY;
  const startHeight = panelHeight.value;
  const windowHeight = window.innerHeight;

  const handleMouseMove = (e: MouseEvent) => {
    if (!isResizing.value) return;

    const deltaY = startY - e.clientY;
    // 限制在边界内：高度不能小于0，顶部不能超出屏幕
    const maxHeight = windowHeight - panelBottom.value;
    const newHeight = Math.max(0, Math.min(maxHeight, startHeight + deltaY));
    panelHeight.value = newHeight;

    // 拖动时实时调整终端大小（使用节流函数）
    scheduleResizeAll();
  };

  const handleMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';

    // 拖动结束后再调整一次，确保精确
    scheduleResizeAll();
  };

  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
  document.body.style.cursor = 'ns-resize';
  document.body.style.userSelect = 'none';
}

// 移动端触摸拖动调整高度
function startMobileResize(event: TouchEvent) {
  if (!expanded.value || !isMobile.value) return;

  const touch = event.touches[0];
  const startY = touch.clientY;
  const startTop = mobilePanelTop.value;
  const windowHeight = window.innerHeight;

  const handleTouchMove = (e: TouchEvent) => {
    const currentTouch = e.touches[0];
    const deltaY = currentTouch.clientY - startY;
    // 计算新的顶部位置 (vh)
    const deltaVh = (deltaY / windowHeight) * 100;
    // 限制范围: 10vh - 70vh
    const newTop = Math.max(10, Math.min(70, startTop + deltaVh));
    mobilePanelTop.value = newTop;
    scheduleResizeAll();
  };

  const handleTouchEnd = () => {
    document.removeEventListener('touchmove', handleTouchMove);
    document.removeEventListener('touchend', handleTouchEnd);
    scheduleResizeAll();
  };

  document.addEventListener('touchmove', handleTouchMove, { passive: true });
  document.addEventListener('touchend', handleTouchEnd);
}

function startResizeLeft(event: MouseEvent) {
  if (!expanded.value) return;

  event.preventDefault();
  isResizing.value = true;

  const startX = event.clientX;
  const startLeft = panelLeft.value;
  const windowWidth = window.innerWidth;
  const maxMargin = windowWidth * MAX_MARGIN_PERCENT;

  const handleMouseMove = (e: MouseEvent) => {
    if (!isResizing.value) return;

    const deltaX = e.clientX - startX;
    // 限制在边界内：不能小于0，也不能让面板宽度为负
    const maxLeft = windowWidth - panelRight.value;
    const newLeft = Math.max(0, Math.min(maxLeft, startLeft + deltaX));

    panelLeft.value = newLeft;

    // 拖动时实时调整终端大小（使用节流函数）
    scheduleResizeAll();
  };

  const handleMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';

    // 拖动结束后再调整一次，确保精确
    scheduleResizeAll();
  };

  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
  document.body.style.cursor = 'ew-resize';
  document.body.style.userSelect = 'none';
}

function startResizeRight(event: MouseEvent) {
  if (!expanded.value) return;

  event.preventDefault();
  isResizing.value = true;

  const startX = event.clientX;
  const startRight = panelRight.value;
  const windowWidth = window.innerWidth;
  const maxMargin = windowWidth * MAX_MARGIN_PERCENT;

  const handleMouseMove = (e: MouseEvent) => {
    if (!isResizing.value) return;

    const deltaX = startX - e.clientX;
    // 限制在边界内：不能小于0，也不能让面板宽度为负
    const maxRight = windowWidth - panelLeft.value;
    const newRight = Math.max(0, Math.min(maxRight, startRight + deltaX));

    panelRight.value = newRight;

    // 拖动时实时调整终端大小（使用节流函数）
    scheduleResizeAll();
  };

  const handleMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';

    // 拖动结束后再调整一次，确保精确
    scheduleResizeAll();
  };

  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
  document.body.style.cursor = 'ew-resize';
  document.body.style.userSelect = 'none';
}

function startResizeBottom(event: MouseEvent) {
  if (!expanded.value) return;

  event.preventDefault();
  isResizing.value = true;

  const startY = event.clientY;
  const startHeight = panelHeight.value;
  const startBottom = panelBottom.value;
  const windowHeight = window.innerHeight;

  // 固定顶部位置（从屏幕底部算起）
  const fixedTopPosition = startBottom + startHeight;

  const handleMouseMove = (e: MouseEvent) => {
    if (!isResizing.value) return;

    // 向上拖动底部手柄 -> deltaY为负 -> bottom增加（底部向上移）
    // 向下拖动底部手柄 -> deltaY为正 -> bottom减小（底部向下移）
    const deltaY = e.clientY - startY;
    // 限制在边界内：bottom不能小于0，也不能让高度为负
    const maxBottom = fixedTopPosition;
    let newBottom = Math.max(0, Math.min(maxBottom, startBottom - deltaY));

    // 根据固定的顶部位置计算新高度
    const newHeight = fixedTopPosition - newBottom;

    panelBottom.value = newBottom;
    panelHeight.value = newHeight;

    // 拖动时实时调整终端大小（使用节流函数）
    scheduleResizeAll();
  };

  const handleMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';

    // 拖动结束后再调整一次，确保精确
    scheduleResizeAll();
  };

  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
  document.body.style.cursor = 'ns-resize';
  document.body.style.userSelect = 'none';
}

const isDragging = ref(false);
const DRAG_THRESHOLD = 5; // 鼠标移动超过这个距离才算拖动

function startPanelDrag(event: MouseEvent) {
  if (!expanded.value) return;

  // 全屏模式下不允许拖动
  if (isFullscreen.value) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();

  const startX = event.clientX;
  const startY = event.clientY;
  const startLeft = panelLeft.value;
  const startRight = panelRight.value;
  const startBottom = panelBottom.value;
  const windowWidth = window.innerWidth;
  const windowHeight = window.innerHeight;
  const maxMargin = windowWidth * MAX_MARGIN_PERCENT;

  let hasMoved = false;

  const handleMouseMove = (e: MouseEvent) => {
    const deltaX = e.clientX - startX;
    const deltaY = e.clientY - startY;

    // 检查是否超过拖动阈值
    if (!hasMoved && (Math.abs(deltaX) > DRAG_THRESHOLD || Math.abs(deltaY) > DRAG_THRESHOLD)) {
      hasMoved = true;
      isDragging.value = true;
      document.body.style.cursor = 'move';
      document.body.style.userSelect = 'none';
    }

    if (!isDragging.value) return;

    const deltaYInverted = startY - e.clientY; // Y轴向上为正

    // 计算新的位置 - 限制有效的deltaX，确保不超出边界
    // 左边界限制：newLeft >= 0 => deltaX >= -startLeft
    // 右边界限制：newRight >= 0 => deltaX <= startRight
    const effectiveDeltaX = Math.max(-startLeft, Math.min(startRight, deltaX));

    const newLeft = startLeft + effectiveDeltaX;
    const newRight = startRight - effectiveDeltaX;

    // 底部边界：不能小于0，顶部不能超出屏幕
    const maxBottom = windowHeight - panelHeight.value;
    const newBottom = Math.max(0, Math.min(maxBottom, startBottom + deltaYInverted));

    panelLeft.value = newLeft;
    panelRight.value = newRight;
    panelBottom.value = newBottom;

    // 拖动时实时调整终端大小（使用节流函数）
    scheduleResizeAll();
  };

  const handleMouseUp = (e: MouseEvent) => {
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);

    if (hasMoved) {
      // 拖动结束后再调整一次，确保精确
      scheduleResizeAll();
    }

    isDragging.value = false;
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  };

  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
}

// 处理创建终端按钮点击 - 如果只有一个分支，直接创建
function handleCreateTerminalClick() {
  if (worktrees.value.length === 1) {
    openTerminal({ worktreeId: worktrees.value[0].id });
  }
  // 如果有多个分支，下拉菜单会自动显示
}

// 处理创建终端下拉菜单关闭
function handleCreateTerminalMenuClose() {
  showCreateTerminalMenu.value = false;
  createTerminalMenuClosedAt.value = Date.now();
}

// 处理创建终端按钮点击（双击逻辑）
function handleCreateTerminalButtonClick() {
  // 如果菜单刚刚关闭（150ms内），说明是点击按钮导致的clickoutside关闭
  // 这种情况视为"双击"，应该创建终端
  const justClosed = Date.now() - createTerminalMenuClosedAt.value < 150;

  if (justClosed) {
    // 双击：创建和当前激活终端一样分支的终端
    const activeTab = tabs.value.find(tab => tab.id === activeTabId.value);
    let targetWorktreeId: string | undefined;

    if (activeTab?.worktreeId) {
      // 使用当前激活终端的分支
      targetWorktreeId = activeTab.worktreeId;
    } else {
      // 没有激活终端或没有分支，使用默认分支（isMain为true或第一个）
      const mainWorktree = worktrees.value.find(w => w.isMain);
      targetWorktreeId = mainWorktree?.id ?? worktrees.value[0]?.id;
    }

    if (targetWorktreeId) {
      openTerminal({ worktreeId: targetWorktreeId });
    }
  } else if (!showCreateTerminalMenu.value) {
    // 菜单关闭状态，打开菜单
    showCreateTerminalMenu.value = true;
  }
}

// 处理创建终端下拉菜单选择
function handleCreateTerminalSelect(key: string) {
  showCreateTerminalMenu.value = false;
  if (key === EMPTY_TAB_KEY) {
    createEmptyTab();
    return;
  }
  openTerminal({ worktreeId: key });
}

async function openEditorForTab(tab: TerminalTabState | null, editor: EditorPreference) {
  const path = resolveEditorPath(tab);
  if (!path) {
    return;
  }
  if (editor === 'custom' && !customEditorCommand.value) {
    message.warning(t('worktree.configureCustomCommandFirst'));
    return;
  }
  try {
    await projectStore.openInEditor(
      path,
      editor,
      editor === 'custom' ? customEditorCommand.value : undefined
    );
    const label = EDITOR_LABEL_MAP[editor] ?? t('worktree.editor');
    message.success(t('worktree.openedInEditor', { editor: label }));
  } catch (error: any) {
    message.error(error?.message ?? t('worktree.openEditorFailed'));
  }
}

function handleEditorButtonClick() {
  if (!canOpenEditor.value) {
    return;
  }
  void openEditorForTab(activeTerminalTab.value, defaultEditorPreference.value);
}

function handleEditorSelect(key: string | number) {
  if (!canOpenEditor.value) {
    return;
  }
  if (typeof key !== 'string' || !isEditorPreference(key)) {
    return;
  }
  void openEditorForTab(activeTerminalTab.value, key);
}

const enabledQuickActions = computed(() =>
  terminalQuickActions.value.filter(action => action.enabled && action.command.trim())
);

const stackedQuickActions = computed(() =>
  enabledQuickActions.value.filter(action => action.stacked)
);

const standaloneQuickActions = computed(() =>
  enabledQuickActions.value.filter(action => !action.stacked)
);

const showQuickActionsMenu = ref(false);
const QUICK_ACTION_SETTINGS_KEY = '__settings__';

watch(stackedQuickActions, actions => {
  if (actions.length === 0) {
    showQuickActionsMenu.value = false;
  }
});

function formatQuickActionLabel(action: TerminalQuickAction) {
  const name = (action.name || '').trim() || action.id;
  return `${name}: ${t('terminal.quickActionStart')}`;
}

function resolveQuickActionIcon(icon: TerminalQuickActionIcon) {
  switch (icon) {
    case 'chat':
      return ChatbubblesOutline;
    case 'code':
      return CodeOutline;
    case 'rocket':
      return RocketOutline;
    case 'play':
      return PlayOutline;
    case 'claude':
      return ChatbubblesOutline;
    case 'codex':
      return CodeOutline;
    case 'qwen':
      return SparklesOutline;
    case 'gemini':
      return LogoGoogle;
    case 'cursor':
      return NavigateOutline;
    case 'copilot':
      return LogoGithub;
    default:
      return TerminalOutline;
  }
}

function normalizeSvgSize(svg: string, sizePx: number) {
  const size = `${sizePx}px`;
  return svg
    .replace(/width:\s*12px;\s*height:\s*12px;/g, `width: ${size}; height: ${size};`)
    .replace(/width="12px"/g, `width="${size}"`)
    .replace(/height="12px"/g, `height="${size}"`);
}

function getQuickActionSvg(icon: TerminalQuickActionIcon): string {
  switch (icon) {
    case 'claude':
      return normalizeSvgSize(getAssistantIconByType('claude-code'), 16);
    case 'codex':
      return normalizeSvgSize(getAssistantIconByType('codex'), 16);
    case 'qwen':
      return normalizeSvgSize(getAssistantIconByType('qwen-code'), 16);
    case 'gemini':
      return normalizeSvgSize(getAssistantIconByType('gemini'), 16);
    default:
      return '';
  }
}

const quickActionDropdownOptions = computed<DropdownOption[]>(() => [
  ...stackedQuickActions.value.map(action => ({
    label: formatQuickActionLabel(action),
    key: action.id,
    icon: () => {
      const svg = getQuickActionSvg(action.icon);
      if (svg) {
        return h('span', { class: 'terminal-quick-action-menu-svg', innerHTML: svg });
      }
      return h(NIcon, null, {
        default: () => h(resolveQuickActionIcon(action.icon)),
      });
    },
  })),
  { key: '__divider__', type: 'divider' },
  {
    label: t('terminal.quickActionsManage'),
    key: QUICK_ACTION_SETTINGS_KEY,
    icon: () =>
      h(NIcon, null, {
        default: () => h(SettingsOutline),
      }),
  },
]);

function handleQuickActionSelect(key: string) {
  showQuickActionsMenu.value = false;
  if (key === QUICK_ACTION_SETTINGS_KEY) {
    void router.push('/settings?section=terminal');
    return;
  }
  const action = stackedQuickActions.value.find(item => item.id === key);
  if (!action) {
    return;
  }
  void handleRunQuickAction(action);
}

function normalizeTerminalEnter(value: string) {
  const trimmed = value.replace(/\s+$/, '');
  if (!trimmed) {
    return '';
  }
  if (trimmed.endsWith('\n') || trimmed.endsWith('\r')) {
    return trimmed;
  }
  return trimmed + '\r';
}

function resolveQuickActionCommand(id: string) {
  return (
    terminalQuickActions.value
      .find(action => action.id === id && action.enabled)
      ?.command?.trim() ?? ''
  );
}

function resolveAgentCommand(agent: 'claude' | 'codex') {
  if (agent === 'claude') {
    return resolveQuickActionCommand('claude') || resolveQuickActionCommand('ccr') || 'claude';
  }
  return resolveQuickActionCommand('codex') || 'codex';
}

async function handleRunQuickAction(action: TerminalQuickAction) {
  if (!props.projectId) {
    message.warning(t('terminal.pleaseSelectProject'));
    return;
  }
  if (!ensureTerminalCapacity()) {
    return;
  }

  const input = normalizeTerminalEnter(action.command);
  if (!input) {
    message.warning(t('terminal.quickActionMissingCommand'));
    return;
  }

  let worktreeId: string | undefined;
  if (!isEmptyTab(activeId.value)) {
    const activeTab = tabs.value.find(tab => tab.id === activeId.value);
    worktreeId = activeTab?.worktreeId;
  }
  if (!worktreeId) {
    worktreeId = worktrees.value.find(w => w.isMain)?.id ?? worktrees.value[0]?.id;
  }
  if (!worktreeId) {
    message.warning(t('terminal.pleaseSelectProject'));
    return;
  }

  shouldAutoFocusTerminal.value = true;
  expanded.value = true;
  localActiveEmptyTabId.value = '';

  try {
    const newSessionId = await createSession({ worktreeId });

    const startAt = Date.now();
    const timeoutMs = 8000;
    const sendPayload = { type: 'input', data: input };

    if (!send(newSessionId, sendPayload)) {
      const timer = window.setInterval(() => {
        if (send(newSessionId, sendPayload) || Date.now() - startAt > timeoutMs) {
          window.clearInterval(timer);
        }
      }, 200);
    }

    scheduleResizeAll();
    message.success(t('terminal.quickActionTriggered', { name: action.name }));
  } catch (error: any) {
    message.error(error?.message ?? t('terminal.quickActionFailed', { name: action.name }));
  }
}

async function openTerminal(options: TerminalCreateOptions): Promise<string | undefined> {
  if (!props.projectId) {
    message.warning(t('terminal.pleaseSelectProject'));
    return;
  }
  if (!ensureTerminalCapacity()) {
    return;
  }
  shouldAutoFocusTerminal.value = true;
  expanded.value = true;
  // 创建真实终端时，清除空终端激活状态
  localActiveEmptyTabId.value = '';
  try {
    // 如果没有指定插入位置，默认插入到当前激活标签之后
    const finalOptions = { ...options };
    if (!finalOptions.insertAfterSessionId && activeTabId.value) {
      finalOptions.insertAfterSessionId = activeTabId.value;
    }
    const sessionId = await createSession(finalOptions);
    // 创建成功后，等待面板展开动画完成（200ms）+ 缓冲时间，再触发 resize
    // 确保终端尺寸计算时容器已经是最终尺寸
    setTimeout(() => {
      scheduleResizeAll();
    }, 400);
    return sessionId;
  } catch (error: any) {
    message.error(error?.message ?? t('terminal.createFailed'));
  }
}

// Handle resume session from AI Session History dialog
async function handleResumeSession(claudeSessionId: string, sessionType: string) {
  if (!props.projectId) {
    message.warning(t('terminal.pleaseSelectProject'));
    return;
  }
  if (!ensureTerminalCapacity()) {
    return;
  }

  // Get the first worktree (default)
  const worktreeId = worktrees.value[0]?.id;
  if (!worktreeId) {
    message.error(t('terminal.createFailed'));
    return;
  }

  shouldAutoFocusTerminal.value = true;
  expanded.value = true;
  localActiveEmptyTabId.value = '';

  try {
    await createSession({ worktreeId });

    // Get the newly created terminal session ID
    const newSessionId = activeId.value;
    if (!newSessionId) {
      return;
    }

    // Build the resume command using the configured Claude terminal launcher.
    const resumeCommand = `${resolveAgentCommand('claude')} --resume ${claudeSessionId}`;

    // Listen for the terminal ready event, then send the command
    const handleReady = (payload: ServerMessage) => {
      if (payload.type === 'ready') {
        // Remove listener after handling
        emitter.off(newSessionId, handleReady);

        // Send the resume command after a small delay to ensure terminal is fully ready
        setTimeout(() => {
          send(newSessionId, { type: 'input', data: resumeCommand + '\r' });
        }, 100);
      }
    };

    // Subscribe to terminal events
    emitter.on(newSessionId, handleReady);

    // Fallback: if ready event was already fired, try sending after delay
    setTimeout(() => {
      emitter.off(newSessionId, handleReady);
      const tab = tabs.value.find(t => t.id === newSessionId);
      if (tab && tab.clientStatus === 'ready') {
        send(newSessionId, { type: 'input', data: resumeCommand + '\r' });
      }
    }, 1500);

    scheduleResizeAll();
  } catch (error: any) {
    message.error(error?.message ?? t('terminal.createFailed'));
  }
}

function handleClose(sessionId: string) {
  // 处理空标签的关闭
  if (isEmptyTab(sessionId)) {
    closeEmptyTab(sessionId);
    return;
  }

  const tab = tabs.value.find(t => t.id === sessionId);
  const tabTitle = tab?.title || t('terminal.defaultTerminalTitle');

  // 如果开启了关闭确认，先弹出确认对话框
  if (confirmBeforeTerminalClose.value) {
    dialog.warning({
      title: t('terminal.confirmCloseTitle'),
      content: () =>
        h('div', { class: 'terminal-close-confirm' }, [
          h('div', { class: 'terminal-close-confirm__message' }, [
            t('terminal.confirmCloseContent', { title: tabTitle }),
          ]),
        ]),
      positiveText: t('terminal.confirmCloseButton'),
      negativeText: t('common.cancel'),
      onPositiveClick: async () => performClose(sessionId),
    });
    return;
  }

  void performClose(sessionId);
}

async function performClose(sessionId: string): Promise<boolean> {
  try {
    await closeSession(sessionId);
    message.success(t('terminal.terminalClosed'));
    return true;
  } catch (error: any) {
    message.error(error?.message ?? t('terminal.closeFailed'));
    disconnectTab(sessionId);
    return false;
  }
}

function createTabProps(tab: CombinedTab): HTMLAttributes {
  const isActive = activeId.value === tab.id;
  const theme = activeTheme.value;

  // 空标签的简化处理
  if (isEmptyTabItem(tab)) {
    const props: HTMLAttributes = {};
    // 检查是否需要隐藏边框
    const hideHeaderBorder = theme.terminalHeaderBorder === false;
    // 设置空标签的背景色（根据激活状态）
    if (isActive) {
      const bgColor = theme.terminalTabActiveBg || theme.surfaceColor;
      props.style = {
        backgroundColor: bgColor,
        ...(hideHeaderBorder ? { borderBottom: 'none' } : {}),
      };
    } else {
      const bgColor = theme.terminalTabBg || theme.bodyColor;
      props.style = {
        backgroundColor: bgColor,
      };
    }
    return props;
  }

  // 真实终端标签的完整处理
  const realTab = tab as TerminalTabState;
  const props: HTMLAttributes = {
    onContextmenu: (event: MouseEvent) => handleTabContextMenu(event, realTab),
  };

  // 检查是否需要隐藏边框
  const hideHeaderBorder = theme.terminalHeaderBorder === false;

  // 设置普通标签的背景色（根据激活状态）
  if (isActive) {
    const bgColor = theme.terminalTabActiveBg || theme.surfaceColor;
    props.style = {
      backgroundColor: bgColor,
      ...(hideHeaderBorder ? { borderBottom: 'none' } : {}),
    };
  } else {
    const bgColor = theme.terminalTabBg || theme.bodyColor;
    props.style = {
      backgroundColor: bgColor,
    };
  }

  return props;
}

function getTabTooltip(tab: TerminalTabState): string {
  const lines: string[] = [tab.title];

  if (tab.renderMode === 'snapshot') {
    lines.push('');
    lines.push(getSnapshotModeTooltip(tab));
  }

  if (tab.aiAssistant && tab.aiAssistant.detected) {
    lines.push('');
    lines.push(`${t('terminal.aiAssistantLabel')}: ${getAssistantName(tab)}`);
  }

  // Add process information if available
  if (tab.processPid) {
    lines.push('');
    lines.push(`PID: ${tab.processPid}`);

    // Add process status
    if (tab.processStatus === 'idle') {
      lines.push(t('terminal.processStatusIdle'));
    } else if (tab.processStatus === 'busy') {
      lines.push(t('terminal.processStatusBusy'));

      // Add running command if available (but not if already shown as AI assistant)
      if (tab.runningCommand && !tab.aiAssistant) {
        lines.push(`${t('terminal.runningCommand')}: ${tab.runningCommand}`);
      }
    }
  }

  if (tab.traffic) {
    lines.push('');
    lines.push(`${t('terminal.trafficTotal')}: ${formatTrafficBytes(tab.traffic.totalBytes)}`);
    lines.push(
      `${t('terminal.trafficUpstream')}: ${formatTrafficBytes(tab.traffic.upstreamBytes)}`
    );
    lines.push(
      `${t('terminal.trafficDownstream')}: ${formatTrafficBytes(tab.traffic.downstreamBytes)}`
    );
    lines.push(
      `${t('terminal.trafficAverage')}: ${formatTrafficRate(
        tab.traffic.upstreamAvgBytesPerSec
      )} ↑ / ${formatTrafficRate(tab.traffic.downstreamAvgBytesPerSec)} ↓`
    );
    lines.push(
      `${t('terminal.trafficTotal5m')}: ${formatTrafficBytes(
        tab.traffic.totalRecentBytes
      )} (${formatTrafficBytes(tab.traffic.upstreamRecentBytes)} ↑ / ${formatTrafficBytes(
        tab.traffic.downstreamRecentBytes
      )} ↓)`
    );
    lines.push(
      `${t('terminal.trafficAverage5m')}: ${formatTrafficRate(
        tab.traffic.upstreamRecentAvgBytesPerSec
      )} ↑ / ${formatTrafficRate(tab.traffic.downstreamRecentAvgBytesPerSec)} ↓`
    );
  }

  return lines.join('\n');
}

function showAssistantIdentity(tab: TerminalTabState) {
  return Boolean(tab.aiAssistant?.detected);
}

function getAssistantIcon(tab: TerminalTabState): string {
  return getAssistantIconByType(tab.aiAssistant?.type);
}

function getAssistantName(tab: TerminalTabState): string {
  return tab.aiAssistant?.displayName || tab.aiAssistant?.name || tab.aiAssistant?.type || '';
}

function formatProcessInfo(tab: TerminalTabState): string {
  const lines: string[] = [];

  lines.push(`=== ${t('terminal.processInfo')} ===`);
  lines.push(`${t('terminal.sessionId')}: ${tab.id}`);
  lines.push(`${t('terminal.terminalTitle')}: ${tab.title}`);
  lines.push(`${t('terminal.workingDirectory')}: ${tab.workingDir}`);

  // Add AI Assistant info if detected
  if (tab.aiAssistant && tab.aiAssistant.detected) {
    lines.push('');
    lines.push(`${t('terminal.aiAssistantLabel')}: ${getAssistantName(tab)}`);
  }

  if (tab.processPid) {
    lines.push('');
    lines.push(`PID: ${tab.processPid}`);

    // Add status
    let statusText = t('terminal.processStatusUnknown');
    if (tab.processStatus === 'idle') {
      statusText = t('terminal.processStatusIdle');
    } else if (tab.processStatus === 'busy') {
      statusText = t('terminal.processStatusBusy');
    }
    lines.push(`${t('terminal.statusLabel')}: ${statusText}`);

    // Add running command if available (but not if already shown as AI assistant)
    if (tab.runningCommand && !tab.aiAssistant) {
      lines.push(`${t('terminal.runningCommand')}: ${tab.runningCommand}`);
    }
  } else {
    lines.push('');
    lines.push(t('terminal.processInfoUnavailable'));
  }

  if (tab.traffic) {
    lines.push('');
    lines.push(`=== ${t('terminal.trafficStats')} ===`);
    lines.push(
      `${t('terminal.trafficUpstream')}: ${formatTrafficBytes(tab.traffic.upstreamBytes)}`
    );
    lines.push(
      `${t('terminal.trafficDownstream')}: ${formatTrafficBytes(tab.traffic.downstreamBytes)}`
    );
    lines.push(`${t('terminal.trafficTotal')}: ${formatTrafficBytes(tab.traffic.totalBytes)}`);
    lines.push(
      `${t('terminal.trafficAverage')}: ${formatTrafficRate(
        tab.traffic.upstreamAvgBytesPerSec
      )} ↑ / ${formatTrafficRate(tab.traffic.downstreamAvgBytesPerSec)} ↓`
    );
    lines.push(
      `${t('terminal.trafficTotal5m')}: ${formatTrafficBytes(
        tab.traffic.totalRecentBytes
      )} (${formatTrafficBytes(tab.traffic.upstreamRecentBytes)} ↑ / ${formatTrafficBytes(
        tab.traffic.downstreamRecentBytes
      )} ↓)`
    );
    lines.push(
      `${t('terminal.trafficAverage5m')}: ${formatTrafficRate(
        tab.traffic.upstreamRecentAvgBytesPerSec
      )} ↑ / ${formatTrafficRate(tab.traffic.downstreamRecentAvgBytesPerSec)} ↓`
    );
  }

  return lines.join('\n');
}

function formatTrafficBytes(value: number | undefined) {
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

function formatTrafficRate(value: number | undefined) {
  return `${formatTrafficBytes(value)}/s`;
}

function showProcessInfoDialog(tab: TerminalTabState) {
  if (!tab.processPid) {
    message.warning(t('terminal.noProcessInfo'));
    return;
  }

  const info = formatProcessInfo(tab);

  dialog.create({
    title: t('terminal.processInfo'),
    content: () =>
      h(
        'pre',
        {
          style: {
            margin: '0',
            maxHeight: '60vh',
            overflow: 'auto',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
            fontSize: '12px',
            lineHeight: '1.6',
          },
        },
        info
      ),
    positiveText: t('common.confirm'),
    showIcon: false,
  });
}

async function copyWorkingDirectory(tab: TerminalTabState) {
  const path = tab.workingDir;
  if (!path) {
    return;
  }
  await copyText(path, {
    failureMessage: t('terminal.copyFailed'),
    successMessage: t('terminal.pathCopied'),
    onError: error => {
      console.error('Failed to copy path:', error);
    },
  });
}

async function browseDirectory(tab: TerminalTabState) {
  const path = tab.workingDir;
  if (!path) {
    return;
  }
  try {
    await projectStore.openInExplorer(path);
  } catch (error: any) {
    message.error(error?.message ?? t('worktree.openExplorerFailed'));
  }
}

function handleTabContextMenu(event: MouseEvent, tab: TerminalTabState) {
  event.preventDefault();
  contextMenuX.value = event.clientX;
  contextMenuY.value = event.clientY;
  contextMenuTab.value = tab.id;
}

async function handleContextMenuSelect(key: string) {
  if (!contextMenuTab.value) {
    return;
  }
  const tab = tabs.value.find(t => t.id === contextMenuTab.value);
  contextMenuTab.value = null;
  if (!tab) {
    return;
  }
  if (key === 'snapshot-mode:enable') {
    setRenderMode(tab.id, 'snapshot');
    return;
  }
  if (key === 'snapshot-mode:disable') {
    setRenderMode(tab.id, 'live');
    return;
  }
  if (key === 'snapshot-mode:global') {
    setRenderMode(tab.id, null);
    return;
  }
  if (key === 'snapshot-interval:global') {
    setSnapshotInterval(tab.id, null);
    return;
  }
  if (key.startsWith('snapshot-interval:')) {
    const raw = key.replace('snapshot-interval:', '');
    const parsed = Number(raw);
    if (Number.isFinite(parsed)) {
      setSnapshotInterval(tab.id, parsed);
    }
    return;
  }
  if (key === 'duplicate') {
    await duplicateTab(tab);
    return;
  }
  if (key === 'rename') {
    promptRenameTab(tab);
    return;
  }
  if (key === 'copy-process-info') {
    showProcessInfoDialog(tab);
    return;
  }
  if (key === 'copy-path') {
    copyWorkingDirectory(tab);
    return;
  }
  if (key === 'browse-directory') {
    browseDirectory(tab);
    return;
  }
  if (key === 'open-editor') {
    await openEditorForTab(tab, defaultEditorPreference.value);
    return;
  }
  if (key.startsWith('open-editor:')) {
    const editorKey = key.replace('open-editor:', '');
    if (isEditorPreference(editorKey)) {
      await openEditorForTab(tab, editorKey);
    }
    return;
  }
  if (key === 'close-right-tabs') {
    promptCloseRightTabs(tab);
    return;
  }
}

function promptCloseRightTabs(tab: TerminalTabState) {
  const tabIndex = tabs.value.indexOf(tab);
  if (tabIndex < 0 || tabIndex >= tabs.value.length - 1) {
    return;
  }
  const rightTabs = tabs.value.slice(tabIndex + 1);
  const count = rightTabs.length;
  if (count === 0) {
    return;
  }
  dialog.warning({
    title: t('terminal.closeRightTabs'),
    content: t('terminal.closeRightTabsConfirm', { count }),
    positiveText: t('common.confirm'),
    negativeText: t('common.cancel'),
    showIcon: false,
    maskClosable: false,
    onPositiveClick: async () => {
      for (const rightTab of rightTabs) {
        try {
          await closeSession(rightTab.id);
        } catch (error: any) {
          console.error('Failed to close tab:', rightTab.id, error);
        }
      }
      message.success(t('terminal.closeRightTabsSuccess', { count }));
    },
  });
}

async function duplicateTab(tab: TerminalTabState) {
  const title = buildDuplicateTitle(tab.title);
  if (!ensureTerminalCapacity()) {
    return;
  }
  try {
    await createSession({
      worktreeId: tab.worktreeId,
      workingDir: tab.workingDir,
      title,
      rows: tab.rows > 0 ? tab.rows : undefined,
      cols: tab.cols > 0 ? tab.cols : undefined,
      insertAfterSessionId: tab.id,
    });
    message.success(t('terminal.duplicateSuccess'));
  } catch (error: any) {
    message.error(error?.message ?? t('terminal.duplicateFailed'));
  }
}

function ensureTerminalCapacity() {
  if (isTerminalLimitReached.value) {
    message.warning(t('terminal.limitReached', { limit: terminalLimit.value }));
    return false;
  }
  return true;
}

function promptRenameTab(tab: TerminalTabState) {
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
    maskClosable: false,
    closeOnEsc: true,
    onPositiveClick: async () => {
      const nextTitle = inputValue.value.trim();
      if (!nextTitle) {
        message.warning(t('terminal.emptyName'));
        return false;
      }
      if (nextTitle === tab.title) {
        return true;
      }
      try {
        await renameSession(tab.id, nextTitle);
        message.success(t('terminal.renameSuccess'));
        return true;
      } catch (error: any) {
        message.error(error?.message ?? t('terminal.renameFailed'));
        return false;
      }
    },
  });
}

function buildDuplicateTitle(rawTitle: string) {
  const base = rawTitle.trim() || t('terminal.defaultTerminalTitle');
  const baseCandidate = `${base}${DUPLICATE_SUFFIX.value}`;
  const titles = new Set(tabs.value.map(t => t.title));
  if (!titles.has(baseCandidate)) {
    return baseCandidate;
  }
  let counter = 2;
  while (titles.has(`${baseCandidate} ${counter}`)) {
    counter += 1;
  }
  return `${baseCandidate} ${counter}`;
}

function handleSettingsMenuSelect(key: string) {
  showSettingsMenu.value = false;
  if (key === 'auto-resize') {
    autoResize.value = !autoResize.value;
  } else if (key === 'send-resize-on-switch') {
    sendResizeOnSwitch.value = !sendResizeOnSwitch.value;
  } else if (key === 'confirm-close') {
    settingsStore.updateConfirmBeforeTerminalClose(!confirmBeforeTerminalClose.value);
  } else if (key === 'branch-filter-toggle') {
    const nextValue = !showBranchFilter.value;
    showBranchFilter.value = nextValue;
    if (!nextValue && branchFilter.value !== 'all') {
      branchFilter.value = 'all';
      saveCurrentBranchFilter(props.projectId, 'all');
    }
  } else if (key === 'default-open-in-mirror-mode') {
    settingsStore.updateDefaultTerminalRenderMode(
      defaultTerminalRenderMode.value === 'snapshot' ? 'live' : 'snapshot'
    );
  }
}

function focusTerminal(sessionId?: string) {
  if (!sessionId) {
    return;
  }
  focusSessionInStore(sessionId);
}

defineExpose({
  createTerminal: openTerminal,
  reloadSessions,
  focusTerminal,
});
</script>

<style scoped>
.terminal-panel {
  position: fixed;
  min-width: 375px;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background-color: var(--app-surface, var(--n-card-color, #fff));
  color: var(--app-text-primary, var(--app-text-color, #1f1f1f));
  border: 1px solid var(--app-border, var(--n-border-color));
  border-radius: 8px;
  box-shadow: 0 -4px 16px var(--app-shadow, var(--n-box-shadow-color, rgba(0, 0, 0, 0.15)));

  transition:
    height 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.3s ease,
    transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.terminal-panel.is-docked {
  position: relative;
  left: auto !important;
  right: auto !important;
  bottom: auto !important;
  width: 100% !important;
  height: 100% !important;
  min-width: 0;
  border: none;
  border-radius: 0;
  box-shadow: none;
}

.terminal-panel.is-collapsed {
  height: 0 !important;
  opacity: 0;
  pointer-events: none;
  transform: translateY(20px);
}

.terminal-panel:not(.is-collapsed) {
  animation: expandPanel 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

/* 拖动时禁用过渡动画，确保立即响应 */
.terminal-panel.is-resizing {
  transition: none !important;
}

.resize-handle {
  position: absolute;
  z-index: 10;
}

.resize-handle-top {
  top: 0;
  left: 0;
  right: 0;
  height: 6px;
  cursor: ns-resize;
  display: flex;
  align-items: center;
  justify-content: center;
}

.resize-handle-top:hover .resize-indicator {
  background-color: var(--app-accent, var(--n-color-primary));
  opacity: 1;
}

.resize-handle-left {
  left: 0;
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: ew-resize;
  background: transparent;
  transition: background-color 0.2s;
}

.resize-handle-left:hover {
  background: var(--app-accent, var(--n-color-primary));
}

.resize-handle-right {
  right: 0;
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: ew-resize;
  background: transparent;
  transition: background-color 0.2s;
}

.resize-handle-right:hover {
  background: var(--app-accent, var(--n-color-primary));
}

.resize-handle-bottom {
  bottom: 0;
  left: 0;
  right: 0;
  height: 6px;
  cursor: ns-resize;
  display: flex;
  align-items: center;
  justify-content: center;
}

.resize-handle-bottom:hover .resize-indicator {
  background-color: var(--app-accent, var(--n-color-primary));
  opacity: 1;
}

.resize-indicator {
  width: 40px;
  height: 3px;
  border-radius: 2px;
  background-color: var(--app-border-strong, var(--n-border-color));
  opacity: 0.5;
  transition: all 0.2s ease;
}

.panel-drag-handle {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 4px 6px;
  cursor: move;
  opacity: 0.4;
  transition: opacity 0.2s;
  user-select: none;
}

.panel-drag-handle:hover {
  opacity: 1;
}

.panel-header {
  display: flex;
  justify-content: flex-start;
  align-items: center;
  gap: 12px;
  padding: 6px 12px 0;
  flex-shrink: 0;
  background-color: var(--app-surface, var(--app-surface-color, var(--n-card-color, #fff)));
  color: var(--app-text-primary, var(--app-text-color, var(--n-text-color-1, #1f1f1f)));
  border-bottom: var(
    --kanban-terminal-header-border,
    1px solid var(--app-border, var(--n-border-color))
  );
  z-index: 1;
  position: relative;
}

.branch-filter-strip {
  position: absolute;
  bottom: 2px;
  right: 12px;
  min-height: 24px;
  border-radius: 4px;
  background-color: var(
    --kanban-terminal-filter-bg,
    var(--app-surface-raised, var(--n-card-color, #fff))
  );
  border: 1px solid var(--app-border, var(--n-border-color));
  box-shadow: 0 6px 16px var(--app-shadow, rgba(15, 17, 26, 0.16));
  display: inline-flex;
  justify-content: center;
  align-items: center;
  padding: 0 8px;
  gap: 0px;
  font-size: 12px;
  color: var(--app-text-secondary, var(--app-text-color, var(--n-text-color-2, #666)));
  z-index: 11;
}

.branch-filter-item {
  background: transparent;
  border: none;
  color: var(--app-text-muted, var(--n-text-color-4, rgba(0, 0, 0, 0.4)));
  padding: 0;
  margin: 0;
  font: inherit;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 0px;
  line-height: 1;
  transition: color 0.2s ease;
}

.branch-filter-item:focus-visible {
  outline: none;
  color: var(--app-focus-ring, var(--n-color-primary));
  text-decoration: underline;
}

.branch-filter-item:hover {
  color: var(--app-text-secondary, var(--n-text-color-2, #4c4f55));
}

.branch-filter-item.active {
  color: var(--app-accent, var(--n-color-primary, #3b82f6));
  font-weight: 600;
}

.branch-filter-item::after {
  content: '|';
  margin: 0 8px;
  color: var(--app-text-muted, var(--n-text-color-4, rgba(0, 0, 0, 0.35)));
}

.branch-filter-item:last-of-type::after {
  content: '';
  margin: 0;
}

.tabs-container {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  padding-right: 8px;
  position: relative;
}

.tabs-container :deep(.n-tabs) {
  width: 100%;
}

/* 激活标签指示器 */
.active-tab-indicator {
  position: absolute;
  bottom: 8px;
  left: 0;
  height: 2px;
  background-color: var(--app-accent, var(--n-primary-color));
  border-radius: 1px;
  transition:
    transform 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    width 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.3s ease;
  z-index: 2;
}

.tabs-container :deep(.n-tabs-tab) {
  cursor: grab;
  user-select: none;
}

.tabs-container :deep(.n-tabs-tab:active) {
  cursor: grabbing;
}

.panel-header :deep(.n-tabs) {
  --n-tab-border-color: var(--app-border, var(--n-border-color, rgba(0, 0, 0, 0.1)));
  --n-tab-text-color: var(--app-text-secondary, var(--app-text-color, var(--n-text-color-2, #666)));
  --n-tab-text-color-hover: var(
    --app-text-primary,
    var(--app-text-color, var(--n-text-color-1, #333))
  );
  --n-tab-text-color-active: var(
    --app-text-primary,
    var(--app-text-color, var(--n-text-color-1, #333))
  );
}

.panel-header :deep(.n-tabs .n-tabs-card-tabs) {
  background-color: transparent;
}

/* 非选中标签 */
.panel-header :deep(.n-tabs .n-tabs-nav--card-type .n-tabs-tab) {
  background-color: var(--kanban-terminal-tab-bg, var(--app-surface, #ffffff)) !important;
  color: var(--n-tab-text-color);
  border-color: var(--n-tab-border-color);
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
}

/* 选中标签 - 覆盖 Naive UI 硬编码的 #0000 */
.panel-header :deep(.n-tabs .n-tabs-nav--card-type .n-tabs-tab.n-tabs-tab--active) {
  background-color: var(
    --kanban-terminal-tab-active-bg,
    var(--app-surface-active, #e8e8e8)
  ) !important;
  color: var(--n-tab-text-color-active);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  padding-right: 4px;
  margin-left: auto;
}

.mobile-project-switch-badge {
  width: 18px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  color: #fff;
  font-size: 9px;
  font-weight: 700;
  line-height: 1;
}

.terminal-quick-action-button-svg,
.terminal-quick-action-menu-svg {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
}

.terminal-quick-action-button-svg :deep(svg),
.terminal-quick-action-menu-svg :deep(svg) {
  display: block;
}

.panel-body {
  flex: 1;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  background-color: var(--kanban-terminal-bg, #1e1e1e);
}

.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
}

.tab-title {
  display: inline-block;
  max-width: min(160px, 20vw);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tab-render-badge {
  display: inline-flex;
  align-items: center;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--app-accent-soft, rgba(59, 130, 246, 0.14));
  color: var(--app-accent, rgba(29, 78, 216, 0.92));
  font-size: 10px;
  line-height: 16px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.agent-identity {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  max-width: min(128px, 18vw);
  font-size: 10px;
  line-height: 16px;
}

.agent-identity-icon {
  display: inline-flex;
  align-items: center;
  flex: 0 0 auto;
  line-height: 1;
}

.agent-identity-icon :deep(svg) {
  display: block;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
  background-color: var(--app-text-muted, var(--n-text-color-disabled, #c0c4d8));
  box-shadow: 0 0 0 1px var(--app-border, var(--n-box-shadow-color, rgba(15, 17, 26, 0.08)));
}

.status-dot.ready {
  background-color: var(
    --kanban-terminal-status-ready,
    var(--app-success, var(--n-color-success, #12b76a))
  );
  box-shadow: 0 0 0 1px rgba(18, 183, 106, 0.25);
}

.status-dot.connecting {
  background-color: var(
    --kanban-terminal-status-connecting,
    var(--app-warning, var(--n-color-warning, #f79009))
  );
  box-shadow: 0 0 0 1px rgba(247, 144, 9, 0.25);
}

.status-dot.error {
  background-color: var(
    --kanban-terminal-status-error,
    var(--app-error, var(--n-color-error, #f04438))
  );
  box-shadow: 0 0 0 1px rgba(240, 68, 56, 0.25);
}

:global(.terminal-tab-ghost) {
  opacity: 0.4;
}

:global(.terminal-tab-chosen .n-tabs-tab) {
  box-shadow: 0 0 0 1px var(--app-focus-ring, var(--n-color-primary));
}

:global(.terminal-tab-dragging .n-tabs-tab) {
  cursor: grabbing !important;
}

/* 折叠/展开按钮样式 */
.toggle-button {
  transition: none;
}

.toggle-icon {
  transition: none;
}

@keyframes expandPanel {
  from {
    opacity: 0;
    transform: translateY(20px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

/* 空状态引导界面 */
.empty-guide {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 40px;
}

.empty-guide-content {
  text-align: center;
  max-width: 400px;
}

.empty-guide-icon {
  color: var(--kanban-terminal-empty-guide-fg, rgba(255, 255, 255, 0.7));
  opacity: 0.7;
  margin-bottom: 16px;
}

.empty-guide-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--kanban-terminal-empty-guide-fg, rgba(255, 255, 255, 0.95));
  opacity: 0.95;
  margin: 0 0 8px 0;
}

.empty-guide-description {
  font-size: 14px;
  color: var(--kanban-terminal-empty-guide-fg, rgba(255, 255, 255, 0.8));
  opacity: 0.8;
  margin: 0 0 10px 0;
}

.empty-guide-hint {
  font-size: 13px;
  line-height: 1.6;
  color: var(--kanban-terminal-empty-guide-fg, rgba(255, 255, 255, 0.72));
  opacity: 0.72;
  margin: 0 0 24px 0;
}

.view-sessions-btn {
  margin-top: 12px;
  color: var(--kanban-terminal-empty-guide-fg, rgba(255, 255, 255, 0.7));
  opacity: 0.8;
}

.view-sessions-btn:hover {
  opacity: 1;
}

/* 空标签页占位符 */
.empty-tabs-placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  padding: 0 16px;
  min-height: 36px;
}

.empty-tabs-text {
  font-size: 14px;
  color: var(--app-text-secondary, var(--app-text-color, var(--n-text-color-2, #666)));
  opacity: 0.8;
}

/* ========================================
   移动端样式
   ======================================== */

/* 隐藏状态 - 用于移动端视图切换 */
.terminal-panel.is-hidden {
  display: none !important;
}

.terminal-panel.is-mobile {
  position: fixed;
  left: 0 !important;
  right: 0 !important;
  top: 0 !important;
  bottom: 60px !important; /* 为底部导航留出空间 */
  width: 100% !important;
  height: auto !important;
  min-width: unset;
  border-radius: 0;
  border: none;
  z-index: 100;
}

/* 移动端不使用折叠动画 */
.terminal-panel.is-mobile.is-collapsed {
  display: block; /* 移动端通过父组件 v-show 控制 */
}

.terminal-panel.is-mobile .resize-handle {
  display: none;
}

.terminal-panel.is-mobile .panel-header {
  padding: 8px 12px;
}

.terminal-panel.is-mobile .panel-body {
  width: 100%;
}

.terminal-panel.is-mobile .tabs-container {
  max-width: calc(100vw - 120px);
}

.terminal-panel.is-mobile .header-actions {
  gap: 4px;
}

/* 移动端隐藏折叠按钮 */
.terminal-panel.is-mobile .toggle-button {
  display: none;
}

/* 移动端终端选择下拉 */
.mobile-tab-selector {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 4px;
}

.mobile-nav-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--app-text-primary, var(--n-text-color, #333));
  border-radius: 6px;
  cursor: pointer;
  flex-shrink: 0;
}

.mobile-nav-btn:active:not(:disabled) {
  background: var(--app-surface-active, var(--n-color-hover, rgba(0, 0, 0, 0.05)));
}

.mobile-nav-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.mobile-tab-trigger {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: var(--app-surface-hover, var(--n-color-hover, rgba(0, 0, 0, 0.05)));
  border: 1px solid var(--app-border, var(--n-border-color, #e0e0e0));
  border-radius: 6px;
  font-size: 14px;
  color: var(--app-text-primary, var(--n-text-color, #333));
  cursor: pointer;
  max-width: 100%;
  min-width: 0;
}

.mobile-tab-trigger:active {
  opacity: 0.7;
}

.mobile-tab-title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: left;
}

.mobile-tab-arrow {
  flex-shrink: 0;
  transition: transform 0.2s;
}

.mobile-tab-arrow.is-open {
  transform: rotate(180deg);
}
</style>

<style scoped>
/* 隐藏终端tab上下 */
.n-tabs.n-tabs--top .n-tab-pane {
  padding: 0 !important;
}
</style>
