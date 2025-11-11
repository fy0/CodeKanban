<template>
  <div v-if="tabs.length && expanded" class="terminal-panel" :style="panelStyle">
    <!-- 拖动调整高度的手柄 -->
    <div class="resize-handle resize-handle-top" @mousedown="startResize">
      <div class="resize-indicator"></div>
    </div>

    <!-- 左侧拖动手柄 -->
    <div class="resize-handle resize-handle-left" @mousedown="startResizeLeft"></div>

    <!-- 右侧拖动手柄 -->
    <div class="resize-handle resize-handle-right" @mousedown="startResizeRight"></div>

    <div class="panel-header">
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
            <span class="tab-label">
              <span class="status-dot" :class="tab.clientStatus" />
              {{ tab.title }}
            </span>
          </template>
        </n-tab-pane>
      </n-tabs>
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
        <n-checkbox v-model:checked="autoResize" size="small">
          缩放时自动改变终端大小
        </n-checkbox>
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
      />
    </div>
  </div>
  <button
    v-if="tabs.length && !expanded"
    type="button"
    class="terminal-floating-button"
    @click="toggleExpanded"
  >
    <span class="floating-button-label">展开</span>
    <n-icon :size="18">
      <TerminalOutline />
    </n-icon>
  </button>
</template>

<script setup lang="ts">
import { computed, h, ref, toRef, watch } from 'vue';
import type { HTMLAttributes } from 'vue';
import { useDialog, useMessage, NIcon, NInput } from 'naive-ui';
import { useStorage, useThrottleFn } from '@vueuse/core';
import { ChevronDownOutline, ChevronUpOutline, TerminalOutline, CopyOutline, CreateOutline } from '@vicons/ionicons5';
import TerminalViewport from './TerminalViewport.vue';
import { useTerminalClient, type TerminalCreateOptions, type TerminalTabState } from '@/composables/useTerminalClient';
import type { DropdownOption } from 'naive-ui';

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

const MIN_HEIGHT = 200;
const MAX_HEIGHT = 800;
const MIN_MARGIN = 12;
const MAX_MARGIN_PERCENT = 0.4; // 最大边距占窗口宽度的40%
const DUPLICATE_SUFFIX = ' 副本';

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
} =
  useTerminalClient(projectIdRef);

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
}));

// 节流的终端 resize 函数
const throttledTerminalResize = useThrottleFn(() => {
  if (autoResize.value && expanded.value && tabs.value.length > 0) {
    emitter.emit('terminal-resize-all');
  }
}, 100);

// 移除自动收缩逻辑，让用户手动控制展开/收缩状态
// 这样切换项目时不会自动收缩面板

// 监听面板高度变化，自动调整终端大小
watch(
  [panelHeight, expanded],
  () => {
    if (autoResize.value && expanded.value && tabs.value.length > 0) {
      // 延迟一下，等待 DOM 更新
      setTimeout(() => {
        throttledTerminalResize();
      }, 50);
    }
  },
  { flush: 'post' },
);

// 监听标签页切换，立即刷新终端尺寸
watch(
  activeId,
  (newId, oldId) => {
    console.log('[Terminal Panel] Tab switched:', { from: oldId, to: newId });
    if (autoResize.value && expanded.value && newId) {
      setTimeout(() => {
        console.log('[Terminal Panel] Resizing active terminal only:', newId);
        emitter.emit(`terminal-resize-${newId}`);
      }, 50);
    }
  },
);

function toggleExpanded() {
  expanded.value = !expanded.value;
  // 展开时触发 resize，确保终端尺寸正确
  if (expanded.value && autoResize.value && tabs.value.length > 0) {
    setTimeout(() => {
      emitter.emit('terminal-resize-all');
    }, 100);
  }
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
    if (autoResize.value) {
      throttledTerminalResize();
    }
  };

  const handleMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';

    // 拖动结束后再调整一次，确保精确
    if (autoResize.value && expanded.value && tabs.value.length > 0) {
      setTimeout(() => {
        emitter.emit('terminal-resize-all');
      }, 50);
    }
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
    if (autoResize.value) {
      throttledTerminalResize();
    }
  };

  const handleMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';

    // 拖动结束后再调整一次，确保精确
    if (autoResize.value && expanded.value && tabs.value.length > 0) {
      setTimeout(() => {
        emitter.emit('terminal-resize-all');
      }, 50);
    }
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
    if (autoResize.value) {
      throttledTerminalResize();
    }
  };

  const handleMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';

    // 拖动结束后再调整一次，确保精确
    if (autoResize.value && expanded.value && tabs.value.length > 0) {
      setTimeout(() => {
        emitter.emit('terminal-resize-all');
      }, 50);
    }
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
  expanded.value = true;
  try {
    await createSession(options);
    // 创建成功后，等待面板展开动画完成（200ms）+ 缓冲时间，再触发 resize
    // 确保终端尺寸计算时容器已经是最终尺寸
    setTimeout(() => {
      emitter.emit('terminal-resize-all');
    }, 400);
  } catch (error: any) {
    message.error(error?.message ?? '终端创建失败');
  }
}

async function handleClose(sessionId: string) {
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

defineExpose({
  createTerminal: openTerminal,
  reloadSessions,
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
  z-index: 1000;
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
  justify-content: space-between;
  align-items: center;
  padding: 6px 12px 0;
  flex-shrink: 0;
  background-color: var(--n-card-color, #fff);
  border-bottom: 1px solid var(--n-border-color);
  z-index: 1;
  position: relative;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  padding-right: 4px;
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
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  background-color: var(--n-text-color-disabled);
}

.status-dot.ready {
  background-color: var(--n-color-success);
}

.status-dot.connecting {
  background-color: var(--n-color-warning);
}

.status-dot.error {
  background-color: var(--n-color-error);
}

.terminal-floating-button {
  position: fixed;
  bottom: 16px;
  right: 16px;
  min-height: 42px;
  padding: 0 16px;
  border-radius: 21px;
  border: 1px solid var(--n-border-color, rgba(0, 0, 0, 0.12));
  background-color: #fff;
  color: var(--n-text-color, #222);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  box-shadow: 0 4px 10px rgba(0, 0, 0, 0.25);
  cursor: pointer;
  z-index: 1000;
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
