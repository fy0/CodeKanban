<template>
  <div v-if="tabs.length && expanded" class="terminal-panel" :style="panelStyle">
    <!-- 拖动调整高度的手柄 -->
    <div class="resize-handle" @mousedown="startResize">
      <div class="resize-indicator"></div>
    </div>

    <div class="panel-header">
      <n-tabs
        v-model:value="activeId"
        type="card"
        :closable="true"
        size="small"
        @close="handleClose"
      >
        <n-tab-pane v-for="tab in tabs" :key="tab.id" :name="tab.id">
          <template #tab>
            <span class="tab-label">
              <span class="status-dot" :class="tab.clientStatus" />
              {{ tab.title }}
            </span>
          </template>
        </n-tab-pane>
      </n-tabs>
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
import { computed, ref, toRef, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { useStorage, useThrottleFn } from '@vueuse/core';
import { ChevronDownOutline, ChevronUpOutline, TerminalOutline } from '@vicons/ionicons5';
import TerminalViewport from './TerminalViewport.vue';
import { useTerminalClient, type TerminalCreateOptions } from '@/composables/useTerminalClient';

const props = defineProps<{
  projectId: string;
}>();

const projectIdRef = toRef(props, 'projectId');
const message = useMessage();
const expanded = useStorage('terminal-panel-expanded', true);
const panelHeight = useStorage('terminal-panel-height', 320);
const autoResize = useStorage('terminal-auto-resize', true);
const isResizing = ref(false);

const MIN_HEIGHT = 200;
const MAX_HEIGHT = 800;

const { tabs, activeTabId, emitter, createSession, closeSession, send, disconnectTab } =
  useTerminalClient(projectIdRef);

const activeId = computed({
  get: () => activeTabId.value,
  set: value => {
    activeTabId.value = value;
  },
});

const panelStyle = computed(() => ({
  height: expanded.value ? `${panelHeight.value}px` : 'auto',
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

defineExpose({
  createTerminal: openTerminal,
});
</script>

<style scoped>
.terminal-panel {
  position: fixed;
  bottom: 12px;
  left: 12px;
  right: 12px;
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
  top: 0;
  left: 0;
  right: 0;
  height: 6px;
  cursor: ns-resize;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
}

.resize-handle:hover .resize-indicator {
  background-color: var(--n-color-primary);
  opacity: 1;
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
