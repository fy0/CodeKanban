<template>
  <div
    v-if="tabs.length"
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
      <div ref="tabsContainerRef" class="tabs-container">
        <n-tabs
          v-model:value="activeId"
          type="card"
          :closable="true"
          size="small"
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
    v-if="tabs.length && !expanded"
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
import { ChevronDownOutline, ChevronUpOutline, TerminalOutline, CopyOutline, CreateOutline, SettingsOutline, CheckmarkOutline, ClipboardOutline } from '@vicons/ionicons5';
import TerminalViewport from './TerminalViewport.vue';
import { useTerminalClient, type TerminalCreateOptions, type TerminalTabState } from '@/composables/useTerminalClient';
import type { DropdownOption } from 'naive-ui';
import { useSettingsStore } from '@/stores/settings';
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
const expanded = useStorage('terminal-panel-expanded', true);
const panelHeight = useStorage('terminal-panel-height', 320);
const panelLeft = useStorage('terminal-panel-left', 12);
const panelRight = useStorage('terminal-panel-right', 12);
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
      icon: () => h(NIcon, null, { default: () => h(ClipboardOutline) }),
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
]);

const MIN_HEIGHT = 200;
const MAX_HEIGHT = 800;
const MIN_MARGIN = 12;
const MAX_MARGIN_PERCENT = 0.4; // 最大边距占窗口宽度的40%
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
const { maxTerminalsPerProject, terminalShortcut, confirmBeforeTerminalClose } = storeToRefs(settingsStore);
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
  }
});

watch(
  () => tabs.value.length,
  () => {
    nextTick(() => {
      recalcTabTitleWidth();
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

  // Listen for AI completion events
  emitter.on('ai:completed', handleAICompletion);

  // Listen for AI approval events
  emitter.on('ai:approval-needed', handleAIApproval);
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

if (typeof window !== 'undefined') {
  useEventListener(window, 'keydown', handleTerminalToggleShortcut);
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
  if (!tabs.value.length || event.defaultPrevented) {
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
    const newLeft = Math.max(MIN_MARGIN, Math.min(maxMargin, startLeft + deltaX));
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
    const newRight = Math.max(MIN_MARGIN, Math.min(maxMargin, startRight + deltaX));
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

function createTabProps(tab: TerminalTabState): HTMLAttributes {
  const props: HTMLAttributes = {
    onContextmenu: (event: MouseEvent) => handleTabContextMenu(event, tab),
  };

  // Add class for unviewed approval (higher priority than completion)
  if (hasUnviewedApproval(tab)) {
    props.class = 'has-unviewed-approval';
  } else if (hasUnviewedCompletion(tab)) {
    // Add class for unviewed completion
    props.class = 'has-unviewed-completion';
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

    // Add state duration statistics
    const stats = tab.aiAssistant.stats;
    if (stats) {
      const currentState = (tab.aiAssistant.state || 'waiting_input').toLowerCase();
      lines.push('');
      lines.push(t('terminal.aiStatsDurations'));

      if (stats.thinkingDuration > 0 || currentState === 'thinking') {
        const duration = currentState === 'thinking'
          ? stats.thinkingDuration + stats.currentStateDuration
          : stats.thinkingDuration;
        lines.push(`  ${t('terminal.aiStatusThinking')}: ${formatDuration(duration)}`);
      }
      if (stats.executingDuration > 0 || currentState === 'executing') {
        const duration = currentState === 'executing'
          ? stats.executingDuration + stats.currentStateDuration
          : stats.executingDuration;
        lines.push(`  ${t('terminal.aiStatusExecuting')}: ${formatDuration(duration)}`);
      }
      if (stats.waitingApprovalDuration > 0 || currentState === 'waiting_approval') {
        const duration = currentState === 'waiting_approval'
          ? stats.waitingApprovalDuration + stats.currentStateDuration
          : stats.waitingApprovalDuration;
        lines.push(`  ${t('terminal.aiStatusWaitingApproval')}: ${formatDuration(duration)}`);
      }
      if (stats.waitingInputDuration > 0 || currentState === 'waiting_input') {
        const duration = currentState === 'waiting_input'
          ? stats.waitingInputDuration + stats.currentStateDuration
          : stats.waitingInputDuration;
        lines.push(`  ${t('terminal.aiStatusWaitingInput')}: ${formatDuration(duration)}`);
      }
    }
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
    case 'thinking':
      return t('terminal.aiStatusThinking');
    case 'executing':
      return t('terminal.aiStatusExecuting');
    case 'waiting_approval':
      return t('terminal.aiStatusWaitingApproval');
    case 'replying':
      return t('terminal.aiStatusReplying');
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
  const type = tab.aiAssistant?.type || '';

  if (type === 'claude-code') {
    // Claude icon - orange starburst
    return `<svg viewBox="0 0 24 24" fill="currentColor" style="width: 12px; height: 12px;">
      <path d="M12 2L13.09 8.26L19 6.27L15.18 11.18L21 13L15.18 14.82L19 19.73L13.09 17.74L12 24L10.91 17.74L5 19.73L8.82 14.82L3 13L8.82 11.18L5 6.27L10.91 8.26L12 2Z"/>
    </svg>`;
  } else if (type === 'codex') {
    // OpenAI icon
    return `<svg viewBox="0 0 24 24" fill="currentColor" style="width: 12px; height: 12px;">
      <path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z"/>
    </svg>`;
  } else if (type === 'qwen-code') {
    // Qwen official icon
    return `<svg height="12px" style="flex:none;line-height:1" viewBox="0 0 24 24" width="12px" xmlns="http://www.w3.org/2000/svg"><path d="M12.604 1.34c.393.69.784 1.382 1.174 2.075a.18.18 0 00.157.091h5.552c.174 0 .322.11.446.327l1.454 2.57c.19.337.24.478.024.837-.26.43-.513.864-.76 1.3l-.367.658c-.106.196-.223.28-.04.512l2.652 4.637c.172.301.111.494-.043.77-.437.785-.882 1.564-1.335 2.34-.159.272-.352.375-.68.37-.777-.016-1.552-.01-2.327.016a.099.099 0 00-.081.05 575.097 575.097 0 01-2.705 4.74c-.169.293-.38.363-.725.364-.997.003-2.002.004-3.017.002a.537.537 0 01-.465-.271l-1.335-2.323a.09.09 0 00-.083-.049H4.982c-.285.03-.553-.001-.805-.092l-1.603-2.77a.543.543 0 01-.002-.54l1.207-2.12a.198.198 0 000-.197 550.951 550.951 0 01-1.875-3.272l-.79-1.395c-.16-.31-.173-.496.095-.965.465-.813.927-1.625 1.387-2.436.132-.234.304-.334.584-.335a338.3 338.3 0 012.589-.001.124.124 0 00.107-.063l2.806-4.895a.488.488 0 01.422-.246c.524-.001 1.053 0 1.583-.006L11.704 1c.341-.003.724.032.9.34zm-3.432.403a.06.06 0 00-.052.03L6.254 6.788a.157.157 0 01-.135.078H3.253c-.056 0-.07.025-.041.074l5.81 10.156c.025.042.013.062-.034.063l-2.795.015a.218.218 0 00-.2.116l-1.32 2.31c-.044.078-.021.118.068.118l5.716.008c.046 0 .08.02.104.061l1.403 2.454c.046.081.092.082.139 0l5.006-8.76.783-1.382a.055.055 0 01.096 0l1.424 2.53a.122.122 0 00.107.062l2.763-.02a.04.04 0 00.035-.02.041.041 0 000-.04l-2.9-5.086a.108.108 0 010-.113l.293-.507 1.12-1.977c.024-.041.012-.062-.035-.062H9.2c-.059 0-.073-.026-.043-.077l1.434-2.505a.107.107 0 000-.114L9.225 1.774a.06.06 0 00-.053-.031zm6.29 8.02c.046 0 .058.02.034.06l-.832 1.465-2.613 4.585a.056.056 0 01-.05.029.058.058 0 01-.05-.029L8.498 9.841c-.02-.034-.01-.052.028-.054l.216-.012 6.722-.012z" fill="url(#lobe-icons-qwen-fill)" fill-rule="nonzero"></path><defs><linearGradient id="lobe-icons-qwen-fill" x1="0%" x2="100%" y1="0%" y2="0%"><stop offset="0%" stop-color="#6336E7" stop-opacity=".84"></stop><stop offset="100%" stop-color="#6F69F7" stop-opacity=".84"></stop></linearGradient></defs></svg>`;
  }

  // Fallback: use a generic AI icon
  return `<svg viewBox="0 0 24 24" fill="currentColor" style="width: 12px; height: 12px;">
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8zm-1-13h2v6h-2zm0 8h2v2h-2z"/>
  </svg>`;
}

function getAssistantStatusEmoji(tab: TerminalTabState): string {
  const state = tab.aiAssistant?.state?.toLowerCase();
  switch (state) {
    case 'thinking':
      return '🧠';
    case 'executing':
      return '⚙️';
    case 'waiting_approval':
      return '✋';
    case 'replying':
      return '💬';
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
  }
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
  background-color: var(--kanban-terminal-bg, var(--app-surface-color, var(--n-card-color, #fff)));
  color: var(--kanban-terminal-fg, var(--n-text-color-1, #1f1f1f));
  border-bottom: 1px solid var(--n-border-color);
  z-index: 1;
  position: relative;
}

.tabs-container {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  padding-right: 8px;
}

.tabs-container :deep(.n-tabs) {
  width: 100%;
}

.tabs-container :deep(.n-tabs-tab) {
  cursor: grab;
  user-select: none;
}

.tabs-container :deep(.n-tabs-tab:active) {
  cursor: grabbing;
}

.panel-header :deep(.n-tabs) {
  --n-tab-color: var(--kanban-terminal-bg, var(--app-surface-color, var(--n-card-color, #fff)));
  --n-tab-color-segment: var(--kanban-terminal-bg, var(--app-surface-color, var(--n-card-color, #fff)));
  --n-tab-border-color: var(--n-border-color, rgba(255, 255, 255, 0.1));
  --n-tab-text-color: var(--kanban-terminal-fg, var(--n-text-color-2, #d0d0d0));
  --n-tab-text-color-hover: var(--kanban-terminal-fg, var(--n-text-color-1, #ffffff));
  --n-tab-text-color-active: var(--kanban-terminal-fg, var(--n-text-color-1, #ffffff));
}

.panel-header :deep(.n-tabs .n-tabs-card-tabs) {
  background-color: transparent;
}

.panel-header :deep(.n-tabs .n-tabs-card-tabs .n-tabs-tab) {
  background-color: color-mix(in srgb, var(--n-tab-color) 90%, transparent);
  color: var(--n-tab-text-color);
  border-color: var(--n-tab-border-color);
  transition: background-color 0.2s ease, color 0.2s ease;
}

.panel-header :deep(.n-tabs .n-tabs-card-tabs .n-tabs-tab.n-tabs-tab--active) {
  background-color: var(--app-surface-color, color-mix(in srgb, var(--kanban-terminal-bg, #1e1e1e) 50%, var(--app-surface-color, #ffffff) 50%));
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
  border-radius: 999px;
  font-size: 10px;
  line-height: 16px;
  background-color: rgba(99, 102, 241, 0.15);
  color: var(--n-color-primary);
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
.ai-status-pill.state-thinking {
  background-color: rgba(124, 58, 237, 0.16);
  color: #7c3aed;
}

.ai-status-pill.state-executing {
  background-color: rgba(14, 165, 233, 0.16);
  color: #0ea5e9;
}

.ai-status-pill.state-waiting_approval {
  background-color: rgba(247, 144, 9, 0.18);
  color: #f79009;
}

.ai-status-pill.state-replying {
  background-color: rgba(16, 185, 129, 0.16);
  color: #12b76a;
}

.ai-status-pill.state-waiting_input {
  background-color: rgba(148, 163, 184, 0.18);
  color: #475467;
}

.ai-status-pill.state-unknown {
  background-color: rgba(148, 163, 184, 0.12);
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
  background-color: rgba(16, 185, 129, 0.1) !important;
  border-color: rgba(16, 185, 129, 0.3) !important;
}

:deep(.n-tabs-tab.has-unviewed-completion.n-tabs-tab--active) {
  background-color: rgba(16, 185, 129, 0.15) !important;
  border-color: rgba(16, 185, 129, 0.4) !important;
}

/* Tab with unviewed approval - orange background (higher priority than completion) */
:deep(.n-tabs-tab.has-unviewed-approval) {
  background-color: rgba(247, 144, 9, 0.12) !important;
  border-color: rgba(247, 144, 9, 0.35) !important;
}

:deep(.n-tabs-tab.has-unviewed-approval.n-tabs-tab--active) {
  background-color: rgba(247, 144, 9, 0.18) !important;
  border-color: rgba(247, 144, 9, 0.45) !important;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
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
  background-color: var(--n-card-color, #1a1a1a);
  color: var(--n-text-color-1, #fff);
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
</style>

<style scoped>
/* 隐藏终端tab上下 */
.n-tabs.n-tabs--top .n-tab-pane  {
  padding: 0 !important;
}
</style>
