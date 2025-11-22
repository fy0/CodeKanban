<template>
  <div class="terminal-viewport">
    <div ref="containerRef" class="terminal-shell"></div>
    <div v-if="overlayMessage" class="terminal-overlay" :style="terminalOverlayStyle">
      <span>{{ overlayMessage }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, toRef } from 'vue';
import { storeToRefs } from 'pinia';
import type EventEmitter from 'eventemitter3';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { SearchAddon } from '@xterm/addon-search';
import { SerializeAddon } from '@xterm/addon-serialize';
import '@/styles/terminal.css';
import type { TerminalTabState, ServerMessage } from '@/composables/useTerminalClient';
import {
  getTerminalSnapshot,
  saveTerminalSnapshot,
  clearTerminalSnapshot,
} from '@/utils/terminalSnapshotCache';
import { useSettingsStore } from '@/stores/settings';
import { getTerminalThemeById, getDefaultTerminalTheme } from '@/constants/terminalThemes';
import { hexToRgba } from '@/utils/color';

const props = defineProps<{
  tab: TerminalTabState;
  emitter: EventEmitter;
  send: (sessionId: string, payload: any) => void;
  shouldAutoFocus?: boolean;
}>();

const settingsStore = useSettingsStore();
const { terminalThemeId } = storeToRefs(settingsStore);

const activeTerminalTheme = computed(() => {
  return getTerminalThemeById(terminalThemeId.value) || getDefaultTerminalTheme();
});

const terminalOverlayStyle = computed(() => {
  const theme = activeTerminalTheme.value.theme;
  return {
    '--terminal-overlay-bg': hexToRgba(theme.background || '#0f111a', 0.7),
    '--terminal-overlay-color': theme.foreground ?? '#f6f8ff',
  };
});

const containerRef = ref<HTMLDivElement>();
let terminal: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let serializeAddon: SerializeAddon | null = null;
let dragOverHandler: ((event: DragEvent) => void) | null = null;
let dropHandler: ((event: DragEvent) => void) | null = null;
const textDecoder = typeof TextDecoder !== 'undefined' ? new TextDecoder('utf-8') : null;
const SNAPSHOT_SCROLLBACK = 1200;

// 监听 clientStatus 变化
watch(
  () => props.tab.clientStatus,
  (newStatus, oldStatus) => {
    console.log('[Terminal Watch] ClientStatus changed:', {
      sessionId: props.tab.id,
      from: oldStatus,
      to: newStatus,
    });
  }
);

const shouldAutoFocus = computed(() => props.shouldAutoFocus !== false);

const overlayMessage = computed(() => {
  const status = props.tab.clientStatus;
  // Removed debug log to avoid confusion with AI completion detection
  switch (status) {
    case 'connecting':
      return '正在连接终端…';
    case 'error':
      return '连接异常，稍后重试';
    case 'closed':
      return '会话已结束';
    default:
      return '';
  }
});

function handleMessage(payload: ServerMessage) {
  if (!terminal) {
    return;
  }
  switch (payload.type) {
    case 'data':
      if (payload.data) {
        terminal.write(decodeChunk(payload.data));
      }
      break;
    case 'exit':
      if (payload.data) {
        terminal.writeln(`\r\n${payload.data}`);
      }
      break;
    case 'error':
      if (payload.data) {
        terminal.writeln(`\r\n错误: ${payload.data}`);
      }
      break;
    case 'metadata':
      // Forward metadata to parent component via emitter
      if (payload.metadata) {
        props.emitter.emit('metadata', props.tab.id, payload.metadata);
      }
      break;
    default:
      break;
  }
}

function decodeChunk(chunk: string) {
  if (!chunk) {
    return '';
  }
  if (textDecoder) {
    try {
      const bytes = base64ToUint8Array(chunk);
      return textDecoder.decode(bytes);
    } catch {
      // fall through to legacy atob for unexpected errors
    }
  }
  try {
    return window.atob(chunk);
  } catch {
    return chunk;
  }
}

function base64ToUint8Array(value: string) {
  const binary = window.atob(value);
  const len = binary.length;
  const bytes = new Uint8Array(len);
  for (let i = 0; i < len; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function restoreSnapshotIfAvailable() {
  if (!terminal) {
    return false;
  }
  const snapshot = getTerminalSnapshot(props.tab.id);
  if (!snapshot) {
    return false;
  }
  try {
    terminal.reset();
    terminal.write(snapshot.serialized);
    console.log('[Terminal Snapshot] Restored cache for session:', props.tab.id);
    return true;
  } catch (error) {
    console.warn('[Terminal Snapshot] Failed to restore cache', error);
    clearTerminalSnapshot(props.tab.id);
    return false;
  }
}

function persistSnapshot() {
  if (!terminal || !serializeAddon) {
    return;
  }
  try {
    const serialized = serializeAddon.serialize({
      scrollback: SNAPSHOT_SCROLLBACK,
    });
    if (!serialized) {
      clearTerminalSnapshot(props.tab.id);
      return;
    }
    saveTerminalSnapshot(props.tab.id, {
      serialized,
      cols: terminal.cols,
      rows: terminal.rows,
    });
    console.log('[Terminal Snapshot] Saved cache for session:', props.tab.id);
  } catch (error) {
    console.warn('[Terminal Snapshot] Failed to serialize terminal contents', error);
  }
}

function handleResize() {
  if (!terminal || !fitAddon) {
    console.log('[Terminal Resize] Skipped: terminal or fitAddon not ready');
    return;
  }

  // 检查容器是否可见（v-show="false" 时容器尺寸为 0）
  if (
    !containerRef.value ||
    containerRef.value.offsetWidth === 0 ||
    containerRef.value.offsetHeight === 0
  ) {
    console.log('[Terminal Resize] Skipped: container not visible', {
      sessionId: props.tab.id,
      title: props.tab.title,
      containerSize: containerRef.value
        ? {
            width: containerRef.value.offsetWidth,
            height: containerRef.value.offsetHeight,
          }
        : null,
    });
    return;
  }

  try {
    fitAddon.fit();
    props.tab.cols = terminal.cols;
    props.tab.rows = terminal.rows;
    console.log('[Terminal Resize]', {
      sessionId: props.tab.id,
      title: props.tab.title,
      cols: terminal.cols,
      rows: terminal.rows,
      containerSize: containerRef.value
        ? {
            width: containerRef.value.offsetWidth,
            height: containerRef.value.offsetHeight,
          }
        : null,
    });
    props.send(props.tab.id, {
      type: 'resize',
      cols: terminal.cols,
      rows: terminal.rows,
    });
  } catch (error) {
    // 忽略 fit 可能出现的错误
    console.warn('Terminal resize failed:', error);
  }
}

function handleTerminalResizeAll() {
  console.log('[Terminal Resize Event]', {
    sessionId: props.tab.id,
    title: props.tab.title,
  });
  // 延迟一下确保 DOM 更新完成
  setTimeout(() => {
    handleResize();
  }, 10);
}

onMounted(() => {
  // 获取当前选择的终端主题
  const selectedTheme = activeTerminalTheme.value;

  terminal = new Terminal({
    allowProposedApi: true,
    convertEol: true,
    rows: props.tab.rows || 24,
    cols: props.tab.cols || 80,
    cursorBlink: true,
    fontSize: 14,
    fontWeight: 'bold',
    fontWeightBold: 'bold',
    lineHeight: 1.1,
    letterSpacing: 0,
    theme: selectedTheme.theme,
  });
  // terminal = new Terminal(terminalOptions);
  console.log('[Terminal] Created terminal object:', terminal);

  fitAddon = new FitAddon();
  const webLinksAddon = new WebLinksAddon();
  const searchAddon = new SearchAddon();
  const webglAddon = new WebglAddon();
  serializeAddon = new SerializeAddon();

  terminal.loadAddon(fitAddon);
  terminal.loadAddon(webLinksAddon);
  terminal.loadAddon(searchAddon);
  terminal.loadAddon(serializeAddon);
  try {
    terminal.loadAddon(webglAddon);
    console.log('[Terminal] WebGL renderer loaded successfully');
  } catch (error) {
    console.warn('[Terminal] WebGL renderer failed to load, using Canvas fallback', error);
  }

  const restoredFromCache = restoreSnapshotIfAvailable();

  const container = containerRef.value;
  if (container) {
    terminal.open(container);
    if (restoredFromCache) {
      setTimeout(() => {
        terminal?.scrollToBottom();
      }, 0);
    }
    // 延迟执行 fit，确保 DOM 完全渲染且面板动画完成
    // 面板展开动画 200ms + 额外缓冲 150ms = 350ms
    const performFit = (retryIfSmall = true) => {
      if (!fitAddon || !terminal) return;

      // 检查容器是否可见
      if (
        !containerRef.value ||
        containerRef.value.offsetWidth === 0 ||
        containerRef.value.offsetHeight === 0
      ) {
        console.log('[Terminal Init Fit] Skipped: container not visible', {
          sessionId: props.tab.id,
          title: props.tab.title,
          retryIfSmall,
          containerSize: containerRef.value
            ? {
                width: containerRef.value.offsetWidth,
                height: containerRef.value.offsetHeight,
              }
            : null,
        });
        // 容器不可见，稍后重试
        if (retryIfSmall) {
          setTimeout(() => performFit(false), 200);
        }
        return;
      }

      fitAddon.fit();
      const cols = terminal.cols;
      const rows = terminal.rows;

      console.log('[Terminal Init Fit]', {
        sessionId: props.tab.id,
        title: props.tab.title,
        cols,
        rows,
        retryIfSmall,
        containerSize: containerRef.value
          ? {
              width: containerRef.value.offsetWidth,
              height: containerRef.value.offsetHeight,
            }
          : null,
      });

      // 检查计算出的尺寸是否合理
      if ((cols < 20 || rows < 5) && retryIfSmall) {
        console.warn('[Terminal Init] Size too small, will retry:', { cols, rows });
        // 容器可能还没准备好，延迟再试一次
        setTimeout(() => performFit(false), 200);
        return;
      }

      // 更新状态并通知服务器
      props.tab.cols = cols;
      props.tab.rows = rows;
      props.send(props.tab.id, {
        type: 'resize',
        cols,
        rows,
      });
      if (shouldAutoFocus.value) {
        terminal.focus();
      }
    };

    setTimeout(() => performFit(), 350);
  }

  terminal.onData(data => {
    props.send(props.tab.id, { type: 'input', data });
  });

  // 智能粘贴处理：根据剪贴板内容类型决定行为
  // - 图片等非文本内容：发送 Ctrl+V 按键给终端，让终端程序自己处理（如 Windows Terminal、cmd）
  // - 普通文本：拦截并发送文本内容（兼容 Claude Code 等依赖前端发送内容的终端）
  terminal.attachCustomKeyEventHandler(event => {
    // 拦截 Ctrl+V (Windows/Linux) 或 Cmd+V (Mac)
    if ((event.ctrlKey || event.metaKey) && event.key === 'v' && event.type === 'keydown') {
      event.preventDefault();

      // 异步处理剪贴板内容
      (async () => {
        try {
          // 读取剪贴板所有内容
          const clipboardItems = await navigator.clipboard.read();

          // 检查是否包含非文本内容（图片等）
          let hasNonText = false;
          for (const item of clipboardItems) {
            // 如果包含图片类型，标记为非文本
            if (item.types.some(type => type.startsWith('image/'))) {
              hasNonText = true;
              break;
            }
          }

          if (hasNonText) {
            // 剪贴板包含图片，发送 Ctrl+V 控制字符 (ASCII 0x16)
            // 让终端程序自己监听系统剪贴板并处理
            props.send(props.tab.id, { type: 'input', data: '\x16' });
          } else {
            // 剪贴板只有文本，读取并发送文本内容
            const text = await navigator.clipboard.readText();
            if (text && terminal) {
              props.send(props.tab.id, { type: 'input', data: text });
            }
          }
        } catch (err) {
          console.warn('[Terminal] Failed to read clipboard:', err);
          // 失败时fallback：尝试读取文本
          try {
            const text = await navigator.clipboard.readText();
            if (text && terminal) {
              props.send(props.tab.id, { type: 'input', data: text });
            }
          } catch (e) {
            console.warn('[Terminal] Fallback clipboard read also failed:', e);
          }
        }
      })();

      return false; // 阻止 xterm.js 默认处理
    }
    return true; // 其他按键正常处理
  });

  // 支持拖放图片文件到终端
  dragOverHandler = (event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    // 设置拖放效果
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = 'copy';
    }
  };

  dropHandler = async (event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();

    const files = event.dataTransfer?.files;
    if (!files || files.length === 0) {
      return;
    }

    // 处理所有拖放的文件
    for (const file of Array.from(files)) {
      // 只处理图片文件
      if (file.type.startsWith('image/')) {
        try {
          // 读取图片为 base64
          const arrayBuffer = await file.arrayBuffer();
          const base64 = btoa(
            new Uint8Array(arrayBuffer).reduce((data, byte) => data + String.fromCharCode(byte), '')
          );

          // 上传图片到服务器
          const response = await fetch('/api/v1/upload/clipboard-image', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              fileName: file.name,
              data: base64,
            }),
          });

          if (response.ok) {
            const result = await response.json();
            const filePath = result.data.path;
            // 将文件路径作为输入发送
            props.send(props.tab.id, { type: 'input', data: filePath + ' ' });
          }
        } catch (error) {
          console.warn('[Terminal] Failed to upload dropped image:', error);
        }
      }
    }
  };

  container?.addEventListener('dragover', dragOverHandler);
  container?.addEventListener('drop', dropHandler);

  props.emitter.on(props.tab.id, handleMessage);
  props.emitter.on('terminal-resize-all', handleTerminalResizeAll);
  props.emitter.on(`terminal-resize-${props.tab.id}`, handleTerminalResizeAll);
  props.emitter.on('terminal-blur-all', handleTerminalBlurEvent);
  window.addEventListener('resize', handleResize);
});

function handleTerminalBlurEvent() {
  terminal?.blur();
}

onBeforeUnmount(() => {
  persistSnapshot();
  props.emitter.off(props.tab.id, handleMessage);
  props.emitter.off('terminal-resize-all', handleTerminalResizeAll);
  props.emitter.off(`terminal-resize-${props.tab.id}`, handleTerminalResizeAll);
  props.emitter.off('terminal-blur-all', handleTerminalBlurEvent);
  window.removeEventListener('resize', handleResize);
  if (containerRef.value) {
    if (dragOverHandler) {
      containerRef.value.removeEventListener('dragover', dragOverHandler);
    }
    if (dropHandler) {
      containerRef.value.removeEventListener('drop', dropHandler);
    }
  }
  terminal?.dispose();
  terminal = null;
  fitAddon?.dispose();
  fitAddon = null;
  serializeAddon?.dispose();
  serializeAddon = null;
  dragOverHandler = null;
  dropHandler = null;
});
</script>

<style scoped>
.terminal-viewport {
  position: relative;
  height: 100%;
  width: 100%;
  background-color: var(--kanban-terminal-bg, #0f111a);
}

.terminal-shell {
  height: 100%;
  width: 100%;
}

.terminal-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--terminal-overlay-bg, rgba(0, 0, 0, 0.35));
  color: var(--terminal-overlay-color, var(--kanban-terminal-fg, #f6f8ff));
  font-size: 13px;
}
</style>
