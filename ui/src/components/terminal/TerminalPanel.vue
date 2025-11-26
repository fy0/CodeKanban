<template>
  <div
    class="terminal-panel"
    :class="{ 'is-collapsed': !expanded }"
    :style="panelStyle"
    @pointerdown.capture="handlePanelPointerDown"
  >
    <!-- 拖动调整高度的手柄 -->
    <div class="resize-handle resize-handle-top" @mousedown="startResize">
      <div class="resize-indicator"></div>
    </div>

    <!-- 左侧拖动手柄 -->
    <div class="resize-handle resize-handle-left" @mousedown="startResizeLeft"></div>

    <!-- 右侧拖动手柄 -->
    <div class="resize-handle resize-handle-right" @mousedown="startResizeRight"></div>

    <div class="panel-header">
      <div v-if="tabs.length" ref="tabsContainerRef" class="tabs-container">
        <n-tabs
          v-model:value="activeId"
          type="card"
          :closable="true"
          size="small"
          :theme-overrides="tabsThemeOverrides"
          @close="handleClose"
        >
          <n-tab-pane
            v-for="tab in tabs"
            :key="tab.id"
            :name="tab.id"
            :tab-props="createTabProps(tab)"
          >
            <template #tab>
              <span class="tab-label" :title="getTabTooltip(tab)">
                <span v-if="!hideStatusDots" class="status-dot" :class="tab.clientStatus" />
                <span class="tab-title" :style="tabTitleStyle">
                  {{ tab.title }}
                </span>
                <span
                  v-if="showAssistantStatus(tab)"
                  class="ai-status-pill"
                  :class="[
                    `state-${getAssistantStateClass(tab)}`,
                    getAssistantPillSizeClass(tab)
                  ]"
                  :title="getAssistantTooltip(tab)"
                >
                  <span class="ai-status-icon" v-html="getAssistantIcon(tab)"></span>
                  <span class="ai-status-text">{{ getAssistantStatusLabel(tab) }}</span>
                  <span class="ai-status-emoji">{{ getAssistantStatusEmoji(tab) }}</span>
                </span>
              </span>
            </template>
          </n-tab-pane>
        </n-tabs>
        <!-- 激活标签指示器 -->
        <div
          class="active-tab-indicator"
          :style="activeTabIndicatorStyle"
        ></div>
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
        <!-- 创建终端按钮 - 始终显示 -->
        <n-dropdown
          v-if="worktrees.length > 1"
          trigger="click"
          :options="createTerminalOptionsWithHeader"
          @select="handleCreateTerminalSelect"
        >
          <n-button text size="small">
            <template #icon>
              <n-icon>
                <Add />
              </n-icon>
            </template>
          </n-button>
        </n-dropdown>
        <n-button
          v-else
          text
          size="small"
          @click="handleCreateTerminalClick"
        >
          <template #icon>
            <n-icon>
              <Add />
            </n-icon>
          </template>
        </n-button>
        <n-dropdown
          trigger="click"
          placement="bottom-end"
          :show="showSettingsMenu"
          :options="settingsMenuOptions"
          @select="handleSettingsMenuSelect"
          @clickoutside="showSettingsMenu = false"
        >
          <n-button text size="small" @click="showSettingsMenu = !showSettingsMenu">
            <template #icon>
              <n-icon>
                <SettingsOutline />
              </n-icon>
            </template>
          </n-button>
        </n-dropdown>
        <n-button text size="small" class="toggle-button" @click="toggleExpanded">
          <span>{{ expanded ? t('terminal.collapse') : t('terminal.expand') }}</span>
          <n-icon class="toggle-icon" :class="{ 'is-expanded': expanded }">
            <component :is="expanded ? ChevronDownOutline : ChevronUpOutline" />
          </n-icon>
        </n-button>
      </div>
    </div>

    <div v-if="expanded" class="panel-body">
      <div v-if="!tabs.length" class="empty-guide">
        <div class="empty-guide-content">
          <n-icon :size="48" class="empty-guide-icon">
            <TerminalOutline />
          </n-icon>
          <h3 class="empty-guide-title">{{ t('terminal.emptyGuideTitle') }}</h3>
          <p class="empty-guide-description">{{ t('terminal.emptyGuideDescription') }}</p>
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
          <n-button
            v-else
            type="primary"
            @click="handleCreateTerminalClick"
          >
            {{ t('terminal.createNewTerminal') }}
          </n-button>
        </div>
      </div>
      <TerminalViewport
        v-for="tab in tabs"
        v-show="tab.id === activeId"
        :key="tab.id"
        :tab="tab"
        :emitter="emitter"
        :send="send"
        :should-auto-focus="shouldAutoFocusTerminal"
      />
    </div>
  </div>
  <button
    v-if="!expanded"
    type="button"
    class="terminal-floating-button"
    :class="{ 'has-notifications': totalUnviewedCount > 0 }"
    :style="{ zIndex: floatingButtonZIndex }"
    @pointerdown="handleFloatingButtonPointerDown"
    @click="toggleExpanded"
  >
    <span class="floating-button-label">{{ t('terminal.expand') }}</span>
    <n-icon :size="18" class="floating-button-icon">
      <TerminalOutline />
    </n-icon>
    <span v-if="totalUnviewedCount > 0" class="notification-badge">{{ totalUnviewedCount }}</span>
  </button>
</template>

<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, toRef, watch } from 'vue';
import type { HTMLAttributes } from 'vue';
import { storeToRefs } from 'pinia';
import { useDialog, useMessage, NIcon, NInput } from 'naive-ui';
import { useDebounceFn, useEventListener, useResizeObserver, useStorage } from '@vueuse/core';
import { ChevronDownOutline, ChevronUpOutline, TerminalOutline, CopyOutline, CreateOutline, SettingsOutline, CheckmarkOutline, InformationCircleOutline, Add } from '@vicons/ionicons5';
import TerminalViewport from './TerminalViewport.vue';
import { useTerminalClient, type TerminalCreateOptions, type TerminalTabState } from '@/composables/useTerminalClient';
import type { DropdownOption } from 'naive-ui';
import { useSettingsStore } from '@/stores/settings';
import { useProjectStore } from '@/stores/project';
import { getPresetById } from '@/constants/themes';
import { getAssistantIconByType } from '@/utils/assistantIcon';
import Sortable, { type SortableEvent } from 'sortablejs';
import { usePanelStack } from '@/composables/usePanelStack';
import { useLocale } from '@/composables/useLocale';

const props = defineProps<{
  projectId: string;
}>();

const projectIdRef = toRef(props, 'projectId');
const message = useMessage();
const dialog = useDialog();
const { t } = useLocale();
const projectStore = useProjectStore();
const { worktrees } = storeToRefs(projectStore);
const expanded = useStorage('terminal-panel-expanded', true);
const panelHeight = useStorage('terminal-panel-height', 470);
const panelLeft = useStorage('terminal-panel-left', 220);
const panelRight = useStorage('terminal-panel-right', 170);
const autoResize = useStorage('terminal-auto-resize', true);
const isResizing = ref(false);
const shouldAutoFocusTerminal = ref(true);

// 右键菜单相关状态
const contextMenuTab = ref<string | null>(null);
const contextMenuX = ref(0);
const contextMenuY = ref(0);
const contextMenuOptions = computed<DropdownOption[]>(() => {
  const tab = contextMenuTab.value ? tabs.value.find(t => t.id === contextMenuTab.value) : null;
  const hasProcessInfo = tab?.processPid != null;

  return [
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
      label: t('terminal.copyProcessInfo'),
      key: 'copy-process-info',
      icon: () => h(NIcon, null, { default: () => h(InformationCircleOutline) }),
      disabled: !hasProcessInfo,
    },
  ];
});

// 设置菜单相关状态
const showSettingsMenu = ref(false);
const settingsMenuOptions = computed<DropdownOption[]>(() => [
  {
    label: t('terminal.autoResize'),
    key: 'auto-resize',
    icon: autoResize.value ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) }) : undefined,
  },
  {
    label: t('terminal.confirmClose'),
    key: 'confirm-close',
    icon: confirmBeforeTerminalClose.value ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) }) : undefined,
  },
  {
    label: t('terminal.resetPosition'),
    key: 'reset-position',
  },
]);

// 创建终端下拉菜单选项
const createTerminalOptions = computed<DropdownOption[]>(() => {
  return worktrees.value.map(worktree => ({
    label: worktree.branchName,
    key: worktree.id,
  }));
});

// 创建终端下拉菜单选项（带提示头）
const createTerminalOptionsWithHeader = computed<DropdownOption[]>(() => {
  return [
    {
      label: t('terminal.createNewTerminal'),
      key: 'header',
      disabled: true,
      type: 'render',
      render: () => h('div', {
        style: {
          color: 'var(--n-text-color-3, #999)',
          fontSize: '12px',
          fontWeight: '500',
          padding: '8px 12px 4px 12px',
          borderBottom: '1px solid var(--n-divider-color, #eee)',
          marginBottom: '4px',
          cursor: 'default',
          userSelect: 'none'
        }
      }, t('terminal.createNewTerminal'))
    },
    ...worktrees.value.map(worktree => ({
      label: worktree.branchName,
      key: worktree.id,
    }))
  ];
});

const MIN_HEIGHT = 200;
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
const FLOATING_BUTTON_Z_OFFSET = 10;

const { zIndex: terminalPanelZIndex, bringToFront: bringTerminalPanelToFront } = usePanelStack('terminal-panel');
const floatingButtonZIndex = computed(() => terminalPanelZIndex.value + FLOATING_BUTTON_Z_OFFSET);

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
} =
  useTerminalClient(projectIdRef);

const settingsStore = useSettingsStore();
const { maxTerminalsPerProject, terminalShortcut, confirmBeforeTerminalClose, activeTheme, currentPresetId } = storeToRefs(settingsStore);

// Tabs 主题覆盖 - 用于控制标签背景色
const tabsThemeOverrides = computed(() => {
  const theme = activeTheme.value;
  const preset = getPresetById(currentPresetId.value);

  // 获取标签背景色，优先使用主题设置，然后是预设，最后是默认值
  const tabBg = theme.terminalTabBg || preset?.colors.terminalTabBg || theme.bodyColor;
  const tabActiveBg = theme.terminalTabActiveBg || preset?.colors.terminalTabActiveBg || theme.surfaceColor;

  return {
    tabColor: tabBg,
    tabColorSegment: tabActiveBg,
  };
});

const terminalLimit = computed(() => Math.max(maxTerminalsPerProject.value || 1, 1));
const isTerminalLimitReached = computed(() => tabs.value.length >= terminalLimit.value);
const toggleShortcutCode = computed(() => terminalShortcut.value.code);
const toggleShortcutText = computed(() => terminalShortcut.value.display || terminalShortcut.value.code);
const toggleShortcutLabel = computed(() => `快捷键：${toggleShortcutText.value}`);

const tabsContainerRef = ref<HTMLElement | null>(null);
const tabsContainerWidth = ref(0);
const tabTitleMaxWidth = ref(MAX_TAB_TITLE_WIDTH);
const hideStatusDots = ref(false);
const tabTitleStyle = computed(() => ({
  maxWidth: `${tabTitleMaxWidth.value}px`,
}));
const tabDragSortable = shallowRef<Sortable | null>(null);
const refreshTabSortable = useDebounceFn(
  () => {
    nextTick(() => {
      setupTabSorting();
    });
  },
  100,
);

// 激活标签指示器的位置和宽度
const activeTabIndicatorStyle = ref({
  transform: 'translateX(0px)',
  width: '0px',
  opacity: '0',
});

// 更新激活标签指示器的位置
function updateActiveTabIndicator() {
  nextTick(() => {
    const container = tabsContainerRef.value;
    if (!container || !activeId.value) {
      activeTabIndicatorStyle.value = {
        transform: 'translateX(0px)',
        width: '0px',
        opacity: '0',
      };
      return;
    }

    // 查找激活的标签元素
    const wrapper = container.querySelector('.n-tabs-wrapper') as HTMLElement | null;
    if (!wrapper) {
      activeTabIndicatorStyle.value = {
        transform: 'translateX(0px)',
        width: '0px',
        opacity: '0',
      };
      return;
    }

    // 找到所有的标签元素
    const tabElements = wrapper.querySelectorAll('.n-tabs-tab');
    let activeTabElement: Element | null = null;

    // 找到激活的标签
    tabElements.forEach((el) => {
      if (el.classList.contains('n-tabs-tab--active')) {
        activeTabElement = el;
      }
    });

    if (!activeTabElement) {
      activeTabIndicatorStyle.value = {
        transform: 'translateX(0px)',
        width: '0px',
        opacity: '0',
      };
      return;
    }

    // 计算激活标签的位置和宽度
    const wrapperRect = wrapper.getBoundingClientRect();
    const activeRect = (activeTabElement as HTMLElement).getBoundingClientRect();
    const tabWidth = activeRect.width;

    // 根据标签宽度动态计算指示器宽度
    // 标签越宽，指示器占比越小；标签越窄，指示器占比越大
    let indicatorWidth: number;
    if (tabWidth > 150) {
      // 宽标签：使用 35% 的宽度
      indicatorWidth = tabWidth * 0.35;
    } else if (tabWidth > 100) {
      // 中等宽度：使用 45% 的宽度
      indicatorWidth = tabWidth * 0.45;
    } else if (tabWidth > 60) {
      // 较窄标签：使用 60% 的宽度
      indicatorWidth = tabWidth * 0.6;
    } else {
      // 很窄的标签：使用 75% 的宽度
      indicatorWidth = tabWidth * 0.75;
    }

    // 限制指示器的最小和最大宽度
    indicatorWidth = Math.max(20, Math.min(80, indicatorWidth));

    // 计算居中偏移量
    const offsetLeft = activeRect.left - wrapperRect.left + (tabWidth - indicatorWidth) / 2;

    activeTabIndicatorStyle.value = {
      transform: `translateX(${offsetLeft}px)`,
      width: `${indicatorWidth}px`,
      opacity: '1',
    };
  });
}

const activeId = computed({
  get: () => activeTabId.value,
  set: value => {
    activeTabId.value = value;
  },
});

const panelStyle = computed(() => ({
  height: expanded.value ? `${panelHeight.value}px` : 'auto',
  left: `${panelLeft.value}px`,
  right: `${panelRight.value}px`,
  zIndex: terminalPanelZIndex.value,
}));

function recalcTabTitleWidth(explicitWidth?: number) {
  if (typeof explicitWidth === 'number') {
    tabsContainerWidth.value = explicitWidth;
  }
  const containerWidth = typeof explicitWidth === 'number' ? explicitWidth : tabsContainerWidth.value;
  if (!containerWidth) {
    tabTitleMaxWidth.value = MAX_TAB_TITLE_WIDTH;
    return;
  }
  const tabCount = Math.max(tabs.value.length, 1);
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
  () => tabs.value.length,
  () => {
    nextTick(() => {
      recalcTabTitleWidth();
      updateActiveTabIndicator();
    });
    refreshTabSortable();
  },
);

watch(
  () => expanded.value,
  value => {
    if (value) {
      nextTick(() => {
        recalcTabTitleWidth();
        updateActiveTabIndicator();
        adjustPanelMarginsForMinWidth();
      });
      refreshTabSortable();
    } else {
      destroyTabSorting();
    }
  },
);

watch(
  () => tabsContainerRef.value,
  element => {
    if (element) {
      refreshTabSortable();
    } else {
      destroyTabSorting();
    }
  },
);

nextTick(() => {
  recalcTabTitleWidth();
});

onMounted(() => {
  refreshTabSortable();
  updateActiveTabIndicator();

  // Listen for AI completion events
  emitter.on('ai:completed', handleAICompletion);

  // Listen for AI approval events
  emitter.on('ai:approval-needed', handleAIApproval);

  // 初始化时检查并调整边距
  adjustPanelMarginsForMinWidth();
});

function handleAICompletion(event: any) {
  const { sessionId } = event;
  if (sessionId && activeId.value !== sessionId) {
    // Only mark as unviewed if the tab is not currently active
    const newSet = new Set(unviewedCompletions.value);
    newSet.add(sessionId);
    unviewedCompletions.value = newSet;
    console.log('[Terminal Panel] Marked session as having unviewed completion:', {
      sessionId,
      totalUnviewed: newSet.size,
    });
  }
}

function handleAIApproval(event: any) {
  const { sessionId } = event;
  if (sessionId && activeId.value !== sessionId) {
    // Only mark as needing approval if the tab is not currently active
    const newSet = new Set(unviewedApprovals.value);
    newSet.add(sessionId);
    unviewedApprovals.value = newSet;
    console.log('[Terminal Panel] Marked session as needing approval:', {
      sessionId,
      totalUnviewedApprovals: newSet.size,
    });
  }
}

onBeforeUnmount(() => {
  destroyTabSorting();
  emitter.off('ai:completed', handleAICompletion);
  emitter.off('ai:approval-needed', handleAIApproval);
});

// 处理窗口大小变化，当窗口缩小时自动调整边距以维持最小宽度
function adjustPanelMarginsForMinWidth() {
  if (typeof window === 'undefined' || !expanded.value) {
    return;
  }

  const windowWidth = window.innerWidth;
  const currentWidth = windowWidth - panelLeft.value - panelRight.value;

  // 如果当前宽度小于最小宽度，需要调整边距
  if (currentWidth < MIN_PANEL_WIDTH) {
    const shortage = MIN_PANEL_WIDTH - currentWidth;

    // 优先缩小左侧边距
    const availableLeftReduction = panelLeft.value - MIN_MARGIN;
    if (availableLeftReduction >= shortage) {
      // 左侧空间足够
      const newLeft = panelLeft.value - shortage;
      if (newLeft !== panelLeft.value) {
        panelLeft.value = newLeft;
      }
    } else {
      // 左侧空间不够，需要同时调整右侧
      const newLeft = MIN_MARGIN;
      const remainingShortage = shortage - availableLeftReduction;
      const newRight = Math.max(MIN_MARGIN, panelRight.value - remainingShortage);

      // 只在值真的改变时才赋值，避免触发不必要的响应式更新
      if (newLeft !== panelLeft.value) {
        panelLeft.value = newLeft;
      }
      if (newRight !== panelRight.value) {
        panelRight.value = newRight;
      }
    }
  }
}

// 使用防抖函数包装，避免频繁调用（200ms防抖）
const debouncedAdjustMargins = useDebounceFn(adjustPanelMarginsForMinWidth, 200);

if (typeof window !== 'undefined') {
  useEventListener(window, 'keydown', handleTerminalToggleShortcut);
  useEventListener(window, 'resize', debouncedAdjustMargins);
}

function setupTabSorting() {
  const container = tabsContainerRef.value;
  if (!container || tabs.value.length <= 1) {
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
      tabDragSortable.value.option('disabled', tabs.value.length <= 1);
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
  tabDragSortable.value.option('disabled', tabs.value.length <= 1);
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
  if (
    fromIndex === -1 ||
    toIndex === -1 ||
    fromIndex === toIndex ||
    fromIndex >= tabs.value.length ||
    toIndex >= tabs.value.length
  ) {
    return;
  }
  reorderTabsInStore(fromIndex, toIndex);
  nextTick(() => {
    scheduleResizeAll();
    updateActiveTabIndicator();
  });
}

// 节流的终端 resize 函数
const scheduleResizeAll = useDebounceFn(
  () => {
    if (autoResize.value && expanded.value && tabs.value.length > 0) {
      emitter.emit('terminal-resize-all');
    }
  },
  150,
);

const scheduleActiveTabResize = useDebounceFn(
  (tabId: string) => {
    if (autoResize.value && expanded.value && tabId) {
      emitter.emit(`terminal-resize-${tabId}`);
    }
  },
  150,
);

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
  { flush: 'post' },
);

// 监听标签页切换，立即刷新终端尺寸
watch(
  activeId,
  (newId, oldId) => {
    console.log('[Terminal Panel] Tab switched:', { from: oldId, to: newId });
    if (!newId) {
      return;
    }

    // Clear unviewed completion indicator when user views the tab
    if (unviewedCompletions.value.has(newId)) {
      const newSet = new Set(unviewedCompletions.value);
      newSet.delete(newId);
      unviewedCompletions.value = newSet;
      console.log('[Terminal Panel] Cleared unviewed completion for session:', {
        sessionId: newId,
        remainingUnviewed: newSet.size,
      });
    }

    // Clear unviewed approval indicator when user views the tab
    if (unviewedApprovals.value.has(newId)) {
      const newSet = new Set(unviewedApprovals.value);
      newSet.delete(newId);
      unviewedApprovals.value = newSet;
      console.log('[Terminal Panel] Cleared unviewed approval for session:', {
        sessionId: newId,
        remainingUnviewedApprovals: newSet.size,
      });
    }

    // Update active tab indicator
    updateActiveTabIndicator();

    nextTick(() => {
      console.log('[Terminal Panel] Queued resize for active terminal:', newId);
      scheduleActiveTabResize(newId);
    });
  },
  { flush: 'post' },
);

type ToggleOptions = {
  skipFocus?: boolean;
};

function isToggleOptions(value: unknown): value is ToggleOptions {
  return Boolean(value && typeof value === 'object' && 'skipFocus' in value);
}

function handlePanelPointerDown() {
  bringTerminalPanelToFront();
}

function handleFloatingButtonPointerDown() {
  bringTerminalPanelToFront();
}

function toggleExpanded(arg?: ToggleOptions | MouseEvent) {
  const options = isToggleOptions(arg) ? arg : undefined;
  const willExpand = !expanded.value;
  if (willExpand) {
    bringTerminalPanelToFront();
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

function handleTerminalToggleShortcut(event: KeyboardEvent) {
  if (event.defaultPrevented) {
    return;
  }
  if (event.repeat || !isToggleShortcut(event)) {
    return;
  }
  const activeElement = (typeof document !== 'undefined' ? document.activeElement : null) as HTMLElement | null;
  if (isTerminalElement(activeElement) || isEditableElement(activeElement)) {
    return;
  }
  event.preventDefault();
  toggleExpanded({ skipFocus: true });
}

function isToggleShortcut(event: KeyboardEvent) {
  if (event.metaKey || event.ctrlKey || event.altKey) {
    return false;
  }
  return event.code === toggleShortcutCode.value;
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

function startResize(event: MouseEvent) {
  if (!expanded.value) return;

  event.preventDefault();
  isResizing.value = true;

  const startY = event.clientY;
  const startHeight = panelHeight.value;

  const handleMouseMove = (e: MouseEvent) => {
    if (!isResizing.value) return;

    const deltaY = startY - e.clientY;
    const newHeight = Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, startHeight + deltaY));
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
    let newLeft = Math.max(MIN_MARGIN, Math.min(maxMargin, startLeft + deltaX));
    let newRight = panelRight.value;

    // 计算当前宽度
    const currentWidth = windowWidth - newLeft - newRight;

    // 如果宽度小于最小宽度，尝试缩小右侧边距
    if (currentWidth < MIN_PANEL_WIDTH) {
      const shortage = MIN_PANEL_WIDTH - currentWidth;
      const minRight = Math.max(MIN_MARGIN, newRight - shortage);
      const actualReduction = newRight - minRight;

      // 调整右侧边距
      newRight = minRight;

      // 如果右侧无法完全补偿，则限制左侧的移动
      if (actualReduction < shortage) {
        newLeft = windowWidth - MIN_PANEL_WIDTH - newRight;
      }

      panelRight.value = newRight;
    }

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
    let newRight = Math.max(MIN_MARGIN, Math.min(maxMargin, startRight + deltaX));
    let newLeft = panelLeft.value;

    // 计算当前宽度
    const currentWidth = windowWidth - newLeft - newRight;

    // 如果宽度小于最小宽度，尝试缩小左侧边距
    if (currentWidth < MIN_PANEL_WIDTH) {
      const shortage = MIN_PANEL_WIDTH - currentWidth;
      const minLeft = Math.max(MIN_MARGIN, newLeft - shortage);
      const actualReduction = newLeft - minLeft;

      // 调整左侧边距
      newLeft = minLeft;

      // 如果左侧无法完全补偿，则限制右侧的移动
      if (actualReduction < shortage) {
        newRight = windowWidth - MIN_PANEL_WIDTH - newLeft;
      }

      panelLeft.value = newLeft;
    }

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

// 处理创建终端按钮点击 - 如果只有一个分支，直接创建
function handleCreateTerminalClick() {
  if (worktrees.value.length === 1) {
    openTerminal({ worktreeId: worktrees.value[0].id });
  }
  // 如果有多个分支，下拉菜单会自动显示
}

// 处理创建终端下拉菜单选择
function handleCreateTerminalSelect(worktreeId: string) {
  openTerminal({ worktreeId });
}

async function openTerminal(options: TerminalCreateOptions) {
  if (!props.projectId) {
    message.warning(t('terminal.pleaseSelectProject'));
    return;
  }
  if (!ensureTerminalCapacity()) {
    return;
  }
  shouldAutoFocusTerminal.value = true;
  expanded.value = true;
  try {
    await createSession(options);
    // 创建成功后，等待面板展开动画完成（200ms）+ 缓冲时间，再触发 resize
    // 确保终端尺寸计算时容器已经是最终尺寸
    setTimeout(() => {
      scheduleResizeAll();
    }, 400);
  } catch (error: any) {
    message.error(error?.message ?? t('terminal.createFailed'));
  }
}

async function handleClose(sessionId: string) {
  // 如果开启了关闭确认，先弹出确认对话框
  if (confirmBeforeTerminalClose.value) {
    const tab = tabs.value.find(t => t.id === sessionId);
    const tabTitle = tab?.title || t('terminal.defaultTerminalTitle');

    dialog.warning({
      title: t('terminal.confirmCloseTitle'),
      content: t('terminal.confirmCloseContent', { title: tabTitle }),
      positiveText: t('terminal.confirmCloseButton'),
      negativeText: t('common.cancel'),
      onPositiveClick: async () => {
        await performClose(sessionId);
      },
    });
  } else {
    await performClose(sessionId);
  }
}

async function performClose(sessionId: string) {
  try {
    await closeSession(sessionId);
    message.success(t('terminal.terminalClosed'));
  } catch (error: any) {
    message.error(error?.message ?? t('terminal.closeFailed'));
    disconnectTab(sessionId);
  }
}

// 获取完成/审批提醒的颜色
const completionColors = computed(() => {
  const theme = activeTheme.value;
  const preset = getPresetById(currentPresetId.value);
  return {
    bg: theme.terminalTabCompletionBg || preset?.colors.terminalTabCompletionBg || 'rgba(16, 185, 129, 0.25)',
    border: theme.terminalTabCompletionBorder || preset?.colors.terminalTabCompletionBorder || 'rgba(16, 185, 129, 0.5)',
  };
});

const approvalColors = computed(() => {
  const theme = activeTheme.value;
  const preset = getPresetById(currentPresetId.value);
  return {
    bg: theme.terminalTabApprovalBg || preset?.colors.terminalTabApprovalBg || 'rgba(247, 144, 9, 0.25)',
    border: theme.terminalTabApprovalBorder || preset?.colors.terminalTabApprovalBorder || 'rgba(247, 144, 9, 0.5)',
  };
});

function createTabProps(tab: TerminalTabState): HTMLAttributes {
  const props: HTMLAttributes = {
    onContextmenu: (event: MouseEvent) => handleTabContextMenu(event, tab),
  };

  const isActive = activeId.value === tab.id;
  const theme = activeTheme.value;
  const preset = getPresetById(currentPresetId.value);

  // 检查是否需要隐藏边框
  const hideHeaderBorder = theme.terminalHeaderBorder === false;

  // 构建 class 列表
  const classes: string[] = [];

  // 优先级: 审批提醒 > 完成提醒 > 激活/非激活状态的默认颜色
  if (hasUnviewedApproval(tab)) {
    classes.push('has-unviewed-approval');
    props.style = {
      backgroundColor: approvalColors.value.bg,
      borderColor: approvalColors.value.border,
      ...(isActive && hideHeaderBorder ? { borderBottom: 'none' } : {}),
    };
  } else if (hasUnviewedCompletion(tab)) {
    classes.push('has-unviewed-completion');
    props.style = {
      backgroundColor: completionColors.value.bg,
      borderColor: completionColors.value.border,
      ...(isActive && hideHeaderBorder ? { borderBottom: 'none' } : {}),
    };
  } else {
    // 设置普通标签的背景色（根据激活状态）
    if (isActive) {
      const bgColor = theme.terminalTabActiveBg || preset?.colors.terminalTabActiveBg || theme.surfaceColor;
      props.style = {
        backgroundColor: bgColor,
        ...(hideHeaderBorder ? { borderBottom: 'none' } : {}),
      };
    } else {
      const bgColor = theme.terminalTabBg || preset?.colors.terminalTabBg || theme.bodyColor;
      props.style = {
        backgroundColor: bgColor,
      };
    }
  }

  // 添加 class 到 props
  if (classes.length > 0) {
    props.class = classes.join(' ');
  }

  return props;
}

// Format duration from nanoseconds to human-readable string
function formatDuration(ns: number): string {
  if (!ns || ns <= 0) return '0s';

  const seconds = Math.floor(ns / 1e9);
  if (seconds < 60) {
    return `${seconds}s`;
  }

  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) {
    return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  }

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}

function getTabTooltip(tab: TerminalTabState): string {
  const lines: string[] = [tab.title];

  // Add AI Assistant information if detected
  if (tab.aiAssistant && tab.aiAssistant.detected) {
    lines.push('');
    lines.push(`🤖 ${getAssistantTooltip(tab)}`);
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

  return lines.join('\n');
}

function showAssistantStatus(tab: TerminalTabState) {
  return Boolean(tab.aiAssistant?.detected);
}

function getAssistantStateClass(tab: TerminalTabState) {
  const state = tab.aiAssistant?.state?.toLowerCase();
  if (!state || state === 'unknown') {
    return 'unknown';
  }
  return state;
}

function getAssistantStatusLabel(tab: TerminalTabState) {
  const state = tab.aiAssistant?.state?.toLowerCase();
  switch (state) {
    case 'working':
      return t('terminal.aiStatusWorking');
    case 'waiting_approval':
      return t('terminal.aiStatusWaitingApproval');
    case 'waiting_input':
      return t('terminal.aiStatusWaitingInput');
    default:
      return ''; // unknown or disabled - no label
  }
}

function getAssistantTooltip(tab: TerminalTabState) {
  const label = getAssistantStatusLabel(tab);
  const name = tab.aiAssistant?.displayName || tab.aiAssistant?.name || tab.aiAssistant?.type || '';
  if (!label) {
    return name || t('terminal.aiAssistantDetected');
  }
  if (!name) {
    return label;
  }
  return `${name} · ${label}`;
}

// Track unviewed AI completions
const unviewedCompletions = ref<Set<string>>(new Set());

// Computed map for better reactivity
const unviewedCompletionsMap = computed(() => {
  const map: Record<string, boolean> = {};
  unviewedCompletions.value.forEach(id => {
    map[id] = true;
  });
  return map;
});

function hasUnviewedCompletion(tab: TerminalTabState): boolean {
  return unviewedCompletionsMap.value[tab.id] === true;
}

// Track unviewed AI approvals
const unviewedApprovals = ref<Set<string>>(new Set());

// Computed map for better reactivity
const unviewedApprovalsMap = computed(() => {
  const map: Record<string, boolean> = {};
  unviewedApprovals.value.forEach(id => {
    map[id] = true;
  });
  return map;
});

function hasUnviewedApproval(tab: TerminalTabState): boolean {
  return unviewedApprovalsMap.value[tab.id] === true;
}

// Total count of unviewed completions and approvals
const totalUnviewedCount = computed(() => {
  return unviewedCompletions.value.size + unviewedApprovals.value.size;
});

function getAssistantIcon(tab: TerminalTabState): string {
  return getAssistantIconByType(tab.aiAssistant?.type);
}

function getAssistantStatusEmoji(tab: TerminalTabState): string {
  const state = tab.aiAssistant?.state?.toLowerCase();
  switch (state) {
    case 'working':
      return '🤔';
    case 'waiting_approval':
      return '✋';
    case 'waiting_input':
      return '✓';
    default:
      return ''; // unknown - no emoji
  }
}

function getAssistantPillSizeClass(tab: TerminalTabState): string {
  // Use tab title max width as a proxy for available space
  const width = tabTitleMaxWidth.value;

  if (width < 60) {
    return 'pill-size-icon-only';
  } else if (width < 100) {
    return 'pill-size-icon-emoji';
  }
  return 'pill-size-full';
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
    lines.push(`🤖 ${t('terminal.aiAssistantLabel')}: ${getAssistantTooltip(tab)}`);
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

  return lines.join('\n');
}

async function copyProcessInfo(tab: TerminalTabState) {
  if (!tab.processPid) {
    message.warning(t('terminal.noProcessInfo'));
    return;
  }

  const info = formatProcessInfo(tab);

  try {
    await navigator.clipboard.writeText(info);
    message.success(t('terminal.processInfoCopied'));
  } catch (error) {
    console.error('Failed to copy process info:', error);
    message.error(t('terminal.copyFailed'));
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
  if (key === 'duplicate') {
    await duplicateTab(tab);
    return;
  }
  if (key === 'rename') {
    promptRenameTab(tab);
    return;
  }
  if (key === 'copy-process-info') {
    copyProcessInfo(tab);
  }
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
  } else if (key === 'confirm-close') {
    settingsStore.updateConfirmBeforeTerminalClose(!confirmBeforeTerminalClose.value);
  } else if (key === 'reset-position') {
    resetTerminalPosition();
  }
}

function resetTerminalPosition() {
  // 重置为默认值
  panelHeight.value = 470;
  panelLeft.value = 220;
  panelRight.value = 170;

  // 重置后触发终端大小调整
  nextTick(() => {
    scheduleResizeAll();
  });
}

defineExpose({
  createTerminal: openTerminal,
  reloadSessions,
  toggleExpanded,
});
</script>

<style scoped>
.terminal-panel {
  position: fixed;
  bottom: 12px;
  min-width: 375px;
  background-color: var(--n-card-color, #fff);
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  box-shadow: 0 -4px 16px var(--n-box-shadow-color, rgba(0, 0, 0, 0.15));
  display: flex;
  flex-direction: column;
  transition: height 0.3s cubic-bezier(0.4, 0, 0.2, 1),
              opacity 0.3s ease,
              transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  overflow: hidden;
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
  background-color: var(--n-color-primary);
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
  background: var(--n-color-primary);
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
  background: var(--n-color-primary);
}

.resize-indicator {
  width: 40px;
  height: 3px;
  border-radius: 2px;
  background-color: var(--n-border-color);
  opacity: 0.5;
  transition: all 0.2s ease;
}

.panel-header {
  display: flex;
  justify-content: flex-start;
  align-items: center;
  gap: 12px;
  padding: 6px 12px 0;
  flex-shrink: 0;
  background-color: var(--app-surface-color, var(--n-card-color, #fff));
  color: var(--app-text-color, var(--n-text-color-1, #1f1f1f));
  border-bottom: var(--kanban-terminal-header-border, 1px solid var(--n-border-color));
  z-index: 1;
  position: relative;
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
  background-color: var(--n-primary-color);
  border-radius: 1px;
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1),
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
  --n-tab-border-color: var(--n-border-color, rgba(0, 0, 0, 0.1));
  --n-tab-text-color: var(--app-text-color, var(--n-text-color-2, #666));
  --n-tab-text-color-hover: var(--app-text-color, var(--n-text-color-1, #333));
  --n-tab-text-color-active: var(--app-text-color, var(--n-text-color-1, #333));
}

.panel-header :deep(.n-tabs .n-tabs-card-tabs) {
  background-color: transparent;
}

/* 非选中标签 */
.panel-header :deep(.n-tabs .n-tabs-nav--card-type .n-tabs-tab) {
  background-color: var(--kanban-terminal-tab-bg, #FFFFFF) !important;
  color: var(--n-tab-text-color);
  border-color: var(--n-tab-border-color);
  transition: background-color 0.2s ease, color 0.2s ease;
}

/* 选中标签 - 覆盖 Naive UI 硬编码的 #0000 */
.panel-header :deep(.n-tabs .n-tabs-nav--card-type .n-tabs-tab.n-tabs-tab--active) {
  background-color: var(--kanban-terminal-tab-active-bg, #E8E8E8) !important;
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

.panel-body {
  flex: 1;
  min-height: 0;
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

.ai-status-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 6px;
  margin-bottom: 2px;
  border-radius: 999px;
  font-size: 10px;
  line-height: 16px;
  background-color: #eef2ff;
  color: #6366f1;
  transition: all 0.2s ease;
}

/* Responsive pill states */
.ai-status-pill.pill-size-full .ai-status-emoji {
  display: none;
}

.ai-status-pill.pill-size-icon-emoji .ai-status-text {
  display: none;
}

.ai-status-pill.pill-size-icon-emoji .ai-status-emoji {
  display: inline;
  font-size: 10px;
  line-height: 1;
}

.ai-status-pill.pill-size-icon-only .ai-status-text,
.ai-status-pill.pill-size-icon-only .ai-status-emoji {
  display: none;
}

.ai-status-pill.pill-size-icon-only {
  padding: 0 4px;
}

/* State colors */
.ai-status-pill.state-working {
  background-color: #eadffc;
  color: #7c3aed;
}

.ai-status-pill.state-waiting_approval {
  background-color: #fed7aa;
  color: #f79009;
}

.ai-status-pill.state-waiting_input {
  background-color: #eceef2;
  color: #475467;
}

.ai-status-pill.state-unknown {
  background-color: #f1f5f9;
  color: #94a3b8;
  padding: 0 4px;
}

.ai-status-pill.state-unknown .ai-status-text,
.ai-status-pill.state-unknown .ai-status-emoji {
  display: none;
}

.ai-status-icon {
  display: inline-flex;
  align-items: center;
  line-height: 1;
}

.ai-status-icon :deep(svg) {
  display: block;
}

.ai-status-emoji {
  font-size: 10px;
  line-height: 1;
}

/* Tab with unviewed completion - green background */
:deep(.n-tabs-tab.has-unviewed-completion) {
  background-color: var(--kanban-terminal-tab-completion-bg, rgba(16, 185, 129, 0.2)) !important;
  border-color: var(--kanban-terminal-tab-completion-border, rgba(16, 185, 129, 0.5)) !important;
}

:deep(.n-tabs-tab.has-unviewed-completion.n-tabs-tab--active) {
  background-color: var(--kanban-terminal-tab-completion-active-bg, rgba(16, 185, 129, 0.25)) !important;
  border-color: var(--kanban-terminal-tab-completion-active-border, rgba(16, 185, 129, 0.6)) !important;
}

/* Tab with unviewed approval - orange background (higher priority than completion) */
:deep(.n-tabs-tab.has-unviewed-approval) {
  background-color: var(--kanban-terminal-tab-approval-bg, rgba(247, 144, 9, 0.2)) !important;
  border-color: var(--kanban-terminal-tab-approval-border, rgba(247, 144, 9, 0.5)) !important;
}

:deep(.n-tabs-tab.has-unviewed-approval.n-tabs-tab--active) {
  background-color: var(--kanban-terminal-tab-approval-active-bg, rgba(247, 144, 9, 0.25)) !important;
  border-color: var(--kanban-terminal-tab-approval-active-border, rgba(247, 144, 9, 0.6)) !important;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
  background-color: var(--n-text-color-disabled, #c0c4d8);
  box-shadow: 0 0 0 1px var(--n-box-shadow-color, rgba(15, 17, 26, 0.08));
}

.status-dot.ready {
  background-color: var(--kanban-terminal-status-ready, var(--n-color-success, #12b76a));
  box-shadow: 0 0 0 1px rgba(18, 183, 106, 0.25);
}

.status-dot.connecting {
  background-color: var(--kanban-terminal-status-connecting, var(--n-color-warning, #f79009));
  box-shadow: 0 0 0 1px rgba(247, 144, 9, 0.25);
}

.status-dot.error {
  background-color: var(--kanban-terminal-status-error, var(--n-color-error, #f04438));
  box-shadow: 0 0 0 1px rgba(240, 68, 56, 0.25);
}

:global(.terminal-tab-ghost) {
  opacity: 0.4;
}

:global(.terminal-tab-chosen .n-tabs-tab) {
  box-shadow: 0 0 0 1px var(--n-color-primary);
}

:global(.terminal-tab-dragging .n-tabs-tab) {
  cursor: grabbing !important;
}

.terminal-floating-button {
  position: fixed;
  bottom: 16px;
  right: 16px;
  min-height: 42px;
  padding: 0 16px;
  border-radius: 21px;
  border: 1px solid var(--n-border-color, rgba(255, 255, 255, 0.2));
  background-color: var(--kanban-terminal-floating-button-bg, var(--n-card-color, #1a1a1a));
  color: var(--kanban-terminal-floating-button-fg, var(--n-text-color-1, #fff));
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  box-shadow: 0 4px 10px var(--n-box-shadow-color, rgba(0, 0, 0, 0.25));
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  animation: fadeInUp 0.3s ease-out;
  transition: all 0.3s ease;
}

.terminal-floating-button.has-notifications {
  animation: flashGlow 2s ease-in-out infinite;
  background-color: #12b76a;
  border-color: rgba(18, 183, 106, 0.5);
}

.notification-badge {
  position: absolute;
  top: -6px;
  right: -6px;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background-color: #f04438;
  color: white;
  font-size: 11px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
  animation: bounceIn 0.5s ease-out;
}


.floating-button-label {
  line-height: 1;
}

/* 折叠/展开按钮样式 */
.toggle-button {
  transition: none;
}

.toggle-icon {
  transition: none;
}

/* 浮动按钮图标动画 */
.floating-button-icon {
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.8;
    transform: scale(0.95);
  }
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes flashGlow {
  0%, 100% {
    box-shadow: 0 4px 10px rgba(0, 0, 0, 0.25);
  }
  50% {
    box-shadow: 0 4px 20px rgba(18, 183, 106, 0.6), 0 0 30px rgba(18, 183, 106, 0.4);
  }
}

@keyframes bounceIn {
  0% {
    opacity: 0;
    transform: scale(0.3);
  }
  50% {
    opacity: 1;
    transform: scale(1.1);
  }
  100% {
    transform: scale(1);
  }
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
  margin: 0 0 24px 0;
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
  color: var(--app-text-color, var(--n-text-color-2, #666));
  opacity: 0.8;
}
</style>

<style scoped>
/* 隐藏终端tab上下 */
.n-tabs.n-tabs--top .n-tab-pane  {
  padding: 0 !important;
}
</style>
