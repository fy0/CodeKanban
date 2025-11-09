<template>
  <div class="terminal-viewport">
    <div ref="containerRef" class="terminal-shell"></div>
    <div v-if="overlayMessage" class="terminal-overlay">
      <span>{{ overlayMessage }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, toRef } from 'vue';
import type EventEmitter from 'eventemitter3';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { SearchAddon } from '@xterm/addon-search';
import '@/styles/terminal.css';
import type { TerminalTabState, ServerMessage } from '@/composables/useTerminalClient';

const props = defineProps<{
  tab: TerminalTabState;
  emitter: EventEmitter;
  send: (sessionId: string, payload: any) => void;
}>();

const containerRef = ref<HTMLDivElement>();
let terminal: Terminal | null = null;
let fitAddon: FitAddon | null = null;
const textDecoder = typeof TextDecoder !== 'undefined' ? new TextDecoder('utf-8') : null;

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

const overlayMessage = computed(() => {
  const status = props.tab.clientStatus;
  console.log('[Terminal Overlay] Status check:', status, 'sessionId:', props.tab.id);
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
    theme: {
      background: 'var(--kanban-terminal-bg, #0f111a)',
      foreground: 'var(--kanban-terminal-fg, #f6f8ff)',
      cursor: '#66d9ef',
    },
  });
  // terminal = new Terminal(terminalOptions);
  console.log('[Terminal] Created terminal object:', terminal);

  fitAddon = new FitAddon();
  const webLinksAddon = new WebLinksAddon();
  const searchAddon = new SearchAddon();
  const webglAddon = new WebglAddon();

  terminal.loadAddon(fitAddon);
  terminal.loadAddon(webLinksAddon);
  terminal.loadAddon(searchAddon);
  try {
    terminal.loadAddon(webglAddon);
    console.log('[Terminal] WebGL renderer loaded successfully');
  } catch (error) {
    console.warn('[Terminal] WebGL renderer failed to load, using Canvas fallback', error);
  }

  const container = containerRef.value;
  if (container) {
    terminal.open(container);
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
      terminal.focus();
    };

    setTimeout(() => performFit(), 350);
  }

  terminal.onData(data => {
    props.send(props.tab.id, { type: 'input', data });
  });

  props.emitter.on(props.tab.id, handleMessage);
  props.emitter.on('terminal-resize-all', handleTerminalResizeAll);
  props.emitter.on(`terminal-resize-${props.tab.id}`, handleTerminalResizeAll);
  window.addEventListener('resize', handleResize);
});

onBeforeUnmount(() => {
  props.emitter.off(props.tab.id, handleMessage);
  props.emitter.off('terminal-resize-all', handleTerminalResizeAll);
  props.emitter.off(`terminal-resize-${props.tab.id}`, handleTerminalResizeAll);
  window.removeEventListener('resize', handleResize);
  terminal?.dispose();
  terminal = null;
  fitAddon?.dispose();
  fitAddon = null;
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
  background: rgba(0, 0, 0, 0.35);
  color: var(--kanban-terminal-fg, #f6f8ff);
  font-size: 13px;
}
</style>
