<template>
  <div
    v-if="tabs.length"
    v-show="expanded"
    class="terminal-panel"
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
              <span class="tab-label" :title="tab.title">
                <span v-if="!hideStatusDots" class="status-dot" :class="tab.clientStatus" />
                <span class="tab-title" :style="tabTitleStyle">
                  {{ tab.title }}
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
        <n-button text size="small" @click="toggleExpanded">
          <template #icon>
            <n-icon>
              <component :is="expanded ? ChevronDownOutline : ChevronUpOutline" />
            </n-icon>
          </template>
          {{ expanded ? '折叠' : '展开' }}
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
    :style="{ zIndex: floatingButtonZIndex }"
    @pointerdown="handleFloatingButtonPointerDown"
    @click="toggleExpanded"
  >
    <span class="floating-button-label">展开</span>
    <n-icon :size="18">
      <TerminalOutline />
    </n-icon>
  </button>
</template>

<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, toRef, watch } from 'vue';
import type { HTMLAttributes } from 'vue';
import { storeToRefs } from 'pinia';
import { useDialog, useMessage, NIcon, NInput } from 'naive-ui';
import { useDebounceFn, useEventListener, useResizeObserver, useStorage } from '@vueuse/core';
import { ChevronDownOutline, ChevronUpOutline, TerminalOutline, CopyOutline, CreateOutline, SettingsOutline, CheckmarkOutline } from '@vicons/ionicons5';
import TerminalViewport from './TerminalViewport.vue';
import { useTerminalClient, type TerminalCreateOptions, type TerminalTabState } from '@/composables/useTerminalClient';
import type { DropdownOption } from 'naive-ui';
import { useSettingsStore } from '@/stores/settings';
import Sortable, { type SortableEvent } from 'sortablejs';
import { usePanelStack } from '@/composables/usePanelStack';

const props = defineProps<{
  projectId: string;
}>();

const projectIdRef = toRef(props, 'projectId');
const message = useMessage();
const dialog = useDialog();
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
const contextMenuOptions = ref<DropdownOption[]>([
  {
    label: '复制标签',
    key: 'duplicate',
    icon: () => h(NIcon, null, { default: () => h(CopyOutline) }),
  },
  {
    label: '重命名',
    key: 'rename',
    icon: () => h(NIcon, null, { default: () => h(CreateOutline) }),
  },
]);

// 设置菜单相关状态
const showSettingsMenu = ref(false);
const settingsMenuOptions = computed<DropdownOption[]>(() => [
  {
    label: '缩放时自动改变终端大小',
    key: 'auto-resize',
    icon: autoResize.value ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) }) : undefined,
  },
  {
    label: '关闭终端时需要确认',
    key: 'confirm-close',
    icon: confirmBeforeTerminalClose.value ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) }) : undefined,
  },
]);

const MIN_HEIGHT = 200;
const MAX_HEIGHT = 800;
const MIN_MARGIN = 12;
const MAX_MARGIN_PERCENT = 0.4; // 最大边距占窗口宽度的40%
const DUPLICATE_SUFFIX = ' 副本';
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
});

onBeforeUnmount(() => {
  destroyTabSorting();
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
    message.warning('请先选择项目');
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
    message.error(error?.message ?? '终端创建失败');
  }
}

async function handleClose(sessionId: string) {
  // 如果开启了关闭确认，先弹出确认对话框
  if (confirmBeforeTerminalClose.value) {
    const tab = tabs.value.find(t => t.id === sessionId);
    const tabTitle = tab?.title || '终端';

    dialog.warning({
      title: '确认关闭终端',
      content: `确定要关闭"${tabTitle}"吗？`,
      positiveText: '确认关闭',
      negativeText: '取消',
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
    message.success('终端已关闭');
  } catch (error: any) {
    message.error(error?.message ?? '关闭终端失败');
    disconnectTab(sessionId);
  }
}

function createTabProps(tab: TerminalTabState): HTMLAttributes {
  return {
    onContextmenu: (event: MouseEvent) => handleTabContextMenu(event, tab),
  };
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
    message.success('已复制标签');
  } catch (error: any) {
    message.error(error?.message ?? '复制失败');
  }
}

function ensureTerminalCapacity() {
  if (isTerminalLimitReached.value) {
    message.warning('当前项目终端数量已达上限（' + terminalLimit.value + '），可在全局设置中调整。');
    return false;
  }
  return true;
}

function promptRenameTab(tab: TerminalTabState) {
  const inputValue = ref(tab.title);
  dialog.create({
    title: '重命名标签',
    content: () =>
      h(NInput, {
        value: inputValue.value,
        'onUpdate:value': (value: string) => {
          inputValue.value = value;
        },
        maxlength: 64,
        autofocus: true,
        placeholder: '请输入新的标签名',
      }),
    positiveText: '保存',
    negativeText: '取消',
    showIcon: false,
    maskClosable: false,
    closeOnEsc: true,
    onPositiveClick: async () => {
      const nextTitle = inputValue.value.trim();
      if (!nextTitle) {
        message.warning('标签名称不能为空');
        return false;
      }
      if (nextTitle === tab.title) {
        return true;
      }
      try {
        await renameSession(tab.id, nextTitle);
        message.success('标签已更新');
        return true;
      } catch (error: any) {
        message.error(error?.message ?? '重命名失败');
        return false;
      }
    },
  });
}

function buildDuplicateTitle(rawTitle: string) {
  const base = rawTitle.trim() || 'Terminal';
  const baseCandidate = `${base}${DUPLICATE_SUFFIX}`;
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
  box-shadow: 0 -4px 16px rgba(0, 0, 0, 0.15);
  display: flex;
  flex-direction: column;
  transition: height 0.2s ease;
  overflow: hidden;
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
  background-color: var(--n-card-color, #fff);
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

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  background-color: var(--n-text-color-disabled, #c0c4d8);
  box-shadow: 0 0 0 1px rgba(15, 17, 26, 0.08);
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
  border: 1px solid rgba(255, 255, 255, 0.2);
  background-color: #1a1a1a;
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  box-shadow: 0 4px 10px rgba(0, 0, 0, 0.25);
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease;
  font-size: 13px;
  font-weight: 600;
}

.terminal-floating-button:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 14px rgba(0, 0, 0, 0.3);
}

.terminal-floating-button:active {
  transform: translateY(1px);
}

.floating-button-label {
  line-height: 1;
}
</style>
