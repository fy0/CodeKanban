<template>
  <div class="terminal-viewport">
    <div ref="containerRef" class="terminal-shell"></div>
    <div v-if="statusOverlayMessage" class="terminal-overlay" :style="terminalOverlayStyle">
      <span>{{ statusOverlayMessage }}</span>
    </div>
    <TransferProgressDialog
      v-if="transferCardMessage"
      :message="transferCardMessage"
      :progress="transferProgress"
      :tone="transferCardTone"
      :card-style="transferCardStyle"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useDebounceFn } from '@vueuse/core';
import { storeToRefs } from 'pinia';
import type EventEmitter from 'eventemitter3';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { SerializeAddon } from '@xterm/addon-serialize';
import { WebglAddon } from '@xterm/addon-webgl';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { SearchAddon } from '@xterm/addon-search';
import { useMessage } from 'naive-ui';
import '@/styles/terminal.css';
import type {
  ReplayBufferedMessagesResult,
  TerminalRemoteSnapshot,
  TerminalSerializedSnapshot,
  TerminalTabState,
  ServerMessage,
} from '@/composables/useTerminalClient';
import { useSettingsStore, DEFAULT_TERMINAL_FONT_FAMILY } from '@/stores/settings';
import { useTerminalStore } from '@/stores/terminal';
import { hexToRgba } from '@/utils/color';
import {
  formatTerminalPathInput,
  uploadTerminalImage,
  type TerminalImageUploadSource,
} from '@/utils/terminalImageUpload';
import { useLocale } from '@/composables/useLocale';
import { useMobileKeyboard } from '@/composables/useMobileKeyboard';
import TransferProgressDialog from '@/components/common/TransferProgressDialog.vue';

type TerminalClientPayload = {
  type: string;
  [key: string]: unknown;
};

const props = defineProps<{
  tab: TerminalTabState;
  emitter: EventEmitter;
  send: (sessionId: string, payload: TerminalClientPayload) => void;
  shouldAutoFocus?: boolean;
  isMobile?: boolean;
}>();

// 移动端使用较小的字体
// 移动端固定使用 11px，桌面端使用用户设置
const MOBILE_FONT_SIZE = 11;

function getTerminalFontSize(baseFontSize: number): number {
  if (props.isMobile) {
    return MOBILE_FONT_SIZE;
  }
  return baseFontSize;
}

const settingsStore = useSettingsStore();
const terminalStore = useTerminalStore();
const message = useMessage();
const { t } = useLocale();
const { activeTerminalTheme, terminalFont, terminalWebGLRenderer } = storeToRefs(settingsStore);

const terminalOverlayStyle = computed(() => {
  const theme = activeTerminalTheme.value;
  return {
    '--terminal-overlay-bg': hexToRgba(theme.background || '#0f111a', 0.7),
    '--terminal-overlay-color': theme.foreground ?? '#f6f8ff',
  };
});

const transferCardStyle = computed(() => {
  const theme = activeTerminalTheme.value;
  const background = theme.background || '#0f111a';
  const foreground = theme.foreground || '#f6f8ff';

  return {
    '--terminal-transfer-card-bg': hexToRgba(background, 0.94),
    '--terminal-transfer-card-fg': foreground,
    '--terminal-transfer-card-border': hexToRgba(foreground, 0.18),
    '--terminal-transfer-card-track': hexToRgba(foreground, 0.14),
  };
});

const containerRef = ref<HTMLDivElement>();
let terminal: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let serializeAddon: SerializeAddon | null = null;
let webglAddon: WebglAddon | null = null;
let pasteHandler: ((event: ClipboardEvent) => void) | null = null;
let keydownCaptureHandler: ((event: KeyboardEvent) => void) | null = null;
let dragOverHandler: ((event: DragEvent) => void) | null = null;
let dropHandler: ((event: DragEvent) => void) | null = null;
let terminalFocusHandler: (() => void) | null = null;
let terminalBlurHandler: (() => void) | null = null;
let containerResizeObserver: ResizeObserver | null = null;
let transferOverlayTimer: number | null = null;
let initialViewportRepairTimer: number | null = null;
let initialViewportReady = false;
let lastReportedCols = 0;
let lastReportedRows = 0;
let pendingServerResizeTimer: number | null = null;
const pendingTerminalMessages: ServerMessage[] = [];
let pendingServerSnapshot: TerminalRemoteSnapshot | null = null;
let pendingFrontendSnapshot: TerminalSerializedSnapshot | null = null;
let pendingServerSnapshotUpdatedAt = 0;
let debugRefreshHandler: (() => boolean) | null = null;
let terminalTaskQueue: Promise<void> = Promise.resolve();
let initialRestorePromise: Promise<void> | null = null;
let deferredViewportRefresh: {
  reason: string;
  options: {
    clearTextureAtlas?: boolean;
    retry?: boolean;
  };
} | null = null;
let replayBufferedMessagesResult: ReplayBufferedMessagesResult | null = null;
let isDisposed = false;
const textDecoder = typeof TextDecoder !== 'undefined' ? new TextDecoder('utf-8') : null;
const INITIAL_OUTPUT_BUFFER_MAX = 5000;
const TERMINAL_SERVER_RESIZE_SETTLE_MS = 100;
const TERMINAL_CATCH_UP_SETTLE_MS = 180;
const TERMINAL_CATCH_UP_MAX_SNAPSHOT_ATTEMPTS = 3;
const transferCardMessage = ref('');
const transferCardTone = ref<'progress' | 'error'>('progress');
const transferProgress = ref<number | null>(null);
let terminalCatchUpActive = false;
let terminalCatchUpTimer: number | null = null;
let terminalCatchUpSnapshotRequestedAt = 0;
let terminalCatchUpSnapshotAttempts = 0;
const TERMINAL_DEBUG_FN = '__codeKanbanForceRefreshVisibleTerminal';
const TERMINAL_DEBUG_REGISTRY = '__codeKanbanTerminalDebugHandlers';
const TERMINAL_RESTORE_DEBUG_REGISTRY = '__codeKanbanTerminalRestoreDebug';

type TerminalDebugWindow = Window & {
  [TERMINAL_DEBUG_FN]?: () => boolean;
  [TERMINAL_DEBUG_REGISTRY]?: Map<string, () => boolean>;
  [TERMINAL_RESTORE_DEBUG_REGISTRY]?: Map<string, TerminalRestoreDebugState>;
};

type TerminalRestorePhase = 'idle' | 'restoring' | 'replaying' | 'settled';
type TerminalRestoreSource = 'frontend' | 'server' | 'none';

type TerminalRestoreDebugState = {
  sessionId: string;
  phase: TerminalRestorePhase;
  source: TerminalRestoreSource;
  mountStartedAt: number;
  settledAt: number;
  lastReason: string;
  replayCompleteAt: number;
  frontendSnapshotUpdatedAt: number;
  serverSnapshotCapturedAt: number;
  bufferedReplayCount: number;
  bufferedReplayFirstReceivedAt: number;
  bufferedReplayLastReceivedAt: number;
  bufferedReplayLastLocalOrder: number;
  liveBufferedCount: number;
};

const restoreDebugState: TerminalRestoreDebugState = {
  sessionId: props.tab.id,
  phase: 'idle',
  source: 'none',
  mountStartedAt: 0,
  settledAt: 0,
  lastReason: '',
  replayCompleteAt: 0,
  frontendSnapshotUpdatedAt: 0,
  serverSnapshotCapturedAt: 0,
  bufferedReplayCount: 0,
  bufferedReplayFirstReceivedAt: 0,
  bufferedReplayLastReceivedAt: 0,
  bufferedReplayLastLocalOrder: 0,
  liveBufferedCount: 0,
};

type PendingServerResize = {
  cols: number;
  rows: number;
  force: boolean;
};

type TerminalSizeSyncOptions = {
  forceServerResize?: boolean;
  serverSync?: 'deferred' | 'immediate' | 'skip';
  finalizeReason?: string;
};

let pendingServerResize: PendingServerResize | null = null;

/**
 * 替换 True Color #000000 为可见颜色
 * xterm.js 的 extendedAnsi 只对 256 色索引模式生效，
 * 但很多程序使用 True Color (24-bit RGB) 直接输出颜色，绕过了映射表。
 * 这里对 True Color 前景色和背景色 RGB(0,0,0) 进行替换，避免在深色背景上不可见。
 * 这对 oh-my-zsh 主题（如 agnoster）尤为重要，因为它们大量使用背景色。
 *
 * True Color 前景色格式: \x1b[38;2;R;G;Bm 或 \x1b[38;2;R;G;B;...m
 * True Color 背景色格式: \x1b[48;2;R;G;Bm 或 \x1b[48;2;R;G;B;...m
 */
const TRUE_COLOR_FG_BLACK_REGEX = /\x1b\[38;2;0;0;0([;m])/g;
const TRUE_COLOR_BG_BLACK_REGEX = /\x1b\[48;2;0;0;0([;m])/g;
const TRUE_COLOR_FG_BLACK_REPLACEMENT = '\x1b[38;2;74;74;74$1'; // #4a4a4a
const TRUE_COLOR_BG_BLACK_REPLACEMENT = '\x1b[48;2;40;40;40$1'; // #282828 - 背景用更深的灰

function remapInvisibleColors(data: string): string {
  return data
    .replace(TRUE_COLOR_FG_BLACK_REGEX, TRUE_COLOR_FG_BLACK_REPLACEMENT)
    .replace(TRUE_COLOR_BG_BLACK_REGEX, TRUE_COLOR_BG_BLACK_REPLACEMENT);
}

function publishRestoreDebugState() {
  if (typeof window === 'undefined') {
    return;
  }

  const debugWindow = window as TerminalDebugWindow;
  const registry =
    debugWindow[TERMINAL_RESTORE_DEBUG_REGISTRY] ?? new Map<string, TerminalRestoreDebugState>();
  debugWindow[TERMINAL_RESTORE_DEBUG_REGISTRY] = registry;
  registry.set(props.tab.id, { ...restoreDebugState });
}

function setRestorePhase(phase: TerminalRestorePhase, reason?: string) {
  restoreDebugState.phase = phase;
  if (reason) {
    restoreDebugState.lastReason = reason;
  }
  if (phase === 'settled') {
    restoreDebugState.settledAt = Date.now();
  }
  publishRestoreDebugState();
}

function setRestoreSource(source: TerminalRestoreSource) {
  restoreDebugState.source = source;
  publishRestoreDebugState();
}

function updateSnapshotDebugState() {
  restoreDebugState.frontendSnapshotUpdatedAt = pendingFrontendSnapshot?.updatedAt ?? 0;
  restoreDebugState.serverSnapshotCapturedAt = parseSnapshotCapturedAt(pendingServerSnapshot);
  publishRestoreDebugState();
}

function updateReplayDebugState(result: ReplayBufferedMessagesResult | null) {
  restoreDebugState.bufferedReplayCount = result?.count ?? 0;
  restoreDebugState.bufferedReplayFirstReceivedAt = result?.firstReceivedAt ?? 0;
  restoreDebugState.bufferedReplayLastReceivedAt = result?.lastReceivedAt ?? 0;
  restoreDebugState.bufferedReplayLastLocalOrder = result?.lastLocalOrder ?? 0;
  publishRestoreDebugState();
}

function updateLiveBufferedCount() {
  restoreDebugState.liveBufferedCount = pendingTerminalMessages.length;
  publishRestoreDebugState();
}

function bufferPendingTerminalMessage(payload: ServerMessage) {
  if (pendingTerminalMessages.length >= INITIAL_OUTPUT_BUFFER_MAX) {
    pendingTerminalMessages.shift();
  }
  pendingTerminalMessages.push(payload);
  updateLiveBufferedCount();
}

function clearPendingTerminalMessages() {
  if (pendingTerminalMessages.length === 0) {
    return;
  }
  pendingTerminalMessages.length = 0;
  updateLiveBufferedCount();
}

function logRestoreTrace(reason: string, extra: Record<string, unknown> = {}) {
  console.debug('[Terminal Restore]', {
    sessionId: props.tab.id,
    phase: restoreDebugState.phase,
    source: restoreDebugState.source,
    reason,
    mountStartedAt: restoreDebugState.mountStartedAt,
    replayCompleteAt: restoreDebugState.replayCompleteAt,
    frontendSnapshotUpdatedAt: restoreDebugState.frontendSnapshotUpdatedAt,
    serverSnapshotCapturedAt: restoreDebugState.serverSnapshotCapturedAt,
    bufferedReplayCount: restoreDebugState.bufferedReplayCount,
    bufferedReplayFirstReceivedAt: restoreDebugState.bufferedReplayFirstReceivedAt,
    bufferedReplayLastReceivedAt: restoreDebugState.bufferedReplayLastReceivedAt,
    bufferedReplayLastLocalOrder: restoreDebugState.bufferedReplayLastLocalOrder,
    liveBufferedCount: restoreDebugState.liveBufferedCount,
    ...extra,
  });
}

function isDocumentVisible() {
  return typeof document === 'undefined' || document.visibilityState === 'visible';
}

function clearTerminalCatchUpTimer() {
  if (terminalCatchUpTimer != null) {
    window.clearTimeout(terminalCatchUpTimer);
    terminalCatchUpTimer = null;
  }
}

function scheduleTerminalCatchUpSettle(reason: string) {
  clearTerminalCatchUpTimer();
  terminalCatchUpTimer = window.setTimeout(() => {
    terminalCatchUpTimer = null;
    void settleTerminalCatchUp(reason);
  }, TERMINAL_CATCH_UP_SETTLE_MS);
}

function beginTerminalCatchUp(
  reason: string,
  options: {
    requestSnapshot?: boolean;
  } = {}
) {
  if (!terminal || !initialViewportReady) {
    return;
  }

  terminalCatchUpActive = true;
  logRestoreTrace('terminal-catch-up-start', {
    reason,
    requestSnapshot: options.requestSnapshot === true,
  });

  if (options.requestSnapshot) {
    terminalCatchUpSnapshotRequestedAt = Date.now();
    terminalCatchUpSnapshotAttempts = 1;
    requestSnapshot(`${reason}-catch-up`, {
      allowLive: true,
      continueAfterResize: true,
    });
  }

  scheduleTerminalCatchUpSettle(reason);
}

async function settleTerminalCatchUp(reason: string) {
  if (!terminalCatchUpActive) {
    return;
  }

  if (!isDocumentVisible()) {
    scheduleTerminalCatchUpSettle(`${reason}-hidden`);
    return;
  }

  const waitingForFreshSnapshot =
    terminalCatchUpSnapshotRequestedAt > 0 &&
    pendingServerSnapshotUpdatedAt < terminalCatchUpSnapshotRequestedAt;

  if (
    waitingForFreshSnapshot &&
    terminalCatchUpSnapshotAttempts < TERMINAL_CATCH_UP_MAX_SNAPSHOT_ATTEMPTS
  ) {
    terminalCatchUpSnapshotAttempts += 1;
    requestSnapshot(`${reason}-retry`, {
      allowLive: true,
      continueAfterResize: true,
    });
    scheduleTerminalCatchUpSettle(`${reason}-await-snapshot`);
    return;
  }

  const restored = await restoreServerSnapshotIfAvailable();
  terminalCatchUpActive = false;
  terminalCatchUpSnapshotRequestedAt = 0;
  terminalCatchUpSnapshotAttempts = 0;
  clearPendingTerminalMessages();

  if (restored) {
    setRestoreSource('server');
    logRestoreTrace('terminal-catch-up-restored', { reason });
    scheduleInitialViewportRepair('terminal-catch-up-restored');
    return;
  }

  logRestoreTrace('terminal-catch-up-settled-without-snapshot', { reason });
  scheduleInitialViewportRepair('terminal-catch-up-settled');
}

function isRestoreBlockingRefresh() {
  return initialRestorePromise != null;
}

function enqueueTerminalTask(label: string, task: () => Promise<void> | void) {
  const run = terminalTaskQueue.then(async () => {
    if (isDisposed || !terminal) {
      return;
    }

    try {
      await task();
    } catch (error) {
      console.warn('[Terminal Queue] Task failed', {
        sessionId: props.tab.id,
        label,
        error,
      });
      throw error;
    }
  });

  terminalTaskQueue = run.catch(() => {});
  return run;
}

function writeTerminalRaw(data: string) {
  if (!terminal || !data) {
    return Promise.resolve();
  }

  return new Promise<void>(resolve => {
    terminal?.write(data, () => resolve());
  });
}

function writelnTerminalRaw(data: string) {
  if (!terminal || !data) {
    return Promise.resolve();
  }

  return new Promise<void>(resolve => {
    terminal?.writeln(data, () => resolve());
  });
}

function flushDeferredViewportRefresh() {
  if (!deferredViewportRefresh) {
    return;
  }

  const pending = deferredViewportRefresh;
  deferredViewportRefresh = null;
  refreshTerminalViewport(pending.reason, pending.options);
}

// 内存中记录已经访问过的终端（刷新后清空）
// 用于检测刷新后首次切换到终端时滚动到底部
const visitedTerminals = new Set<string>();

// 监听终端主题变化，动态更新终端主题
watch(activeTerminalTheme, newTheme => {
  if (terminal) {
    terminal.options.theme = newTheme;
  }
});

// 监听终端字体设置变化，动态更新终端字体
watch(
  terminalFont,
  newFont => {
    if (terminal) {
      const actualFontSize = getTerminalFontSize(newFont.fontSize);
      terminal.options.fontFamily = newFont.fontFamily || DEFAULT_TERMINAL_FONT_FAMILY;
      terminal.options.fontSize = actualFontSize;
      terminal.options.fontWeight = newFont.fontWeight;
      terminal.options.fontWeightBold = newFont.fontWeightBold;
      terminal.options.lineHeight = newFont.lineHeight;
      terminal.options.letterSpacing = newFont.letterSpacing;
      // 字体变化后需要重新 fit 以适应新的尺寸
      if (fitAddon) {
        setTimeout(() => {
          handleResize();
        }, 50);
      }
    }
  },
  { deep: true }
);

const shouldAutoFocus = computed(() => props.shouldAutoFocus !== false);

const statusOverlayMessage = computed(() => {
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

function clearTransferOverlay() {
  if (transferOverlayTimer != null) {
    window.clearTimeout(transferOverlayTimer);
    transferOverlayTimer = null;
  }
  transferCardMessage.value = '';
  transferCardTone.value = 'progress';
  transferProgress.value = null;
}

function showTransferOverlay(
  messageText: string,
  options?: {
    duration?: number;
    tone?: 'progress' | 'error';
    progress?: number | null;
  }
) {
  if (transferOverlayTimer != null) {
    window.clearTimeout(transferOverlayTimer);
    transferOverlayTimer = null;
  }

  transferCardMessage.value = messageText;
  transferCardTone.value = options?.tone ?? 'progress';
  transferProgress.value =
    typeof options?.progress === 'number'
      ? Math.max(0, Math.min(100, Math.round(options.progress)))
      : options?.progress === null
        ? null
        : transferProgress.value;

  if ((options?.duration ?? 0) > 0) {
    transferOverlayTimer = window.setTimeout(() => {
      transferOverlayTimer = null;
      clearTransferOverlay();
    }, options?.duration ?? 0);
  }
}

function isDeferredTerminalMessage(payload: ServerMessage) {
  return (
    payload.type === 'data' ||
    payload.type === 'mode-prefix' ||
    payload.type === 'exit' ||
    payload.type === 'error'
  );
}

function buildAlternateScreenSequence(fallbackAltScreen?: boolean) {
  if (fallbackAltScreen === true) {
    return '\x1b[?1049h';
  }
  if (fallbackAltScreen === false) {
    return '\x1b[?1049l';
  }
  return '';
}

function parseSnapshotCapturedAt(snapshot: TerminalRemoteSnapshot | null) {
  const raw = snapshot?.capturedAt;
  if (!raw) {
    return 0;
  }
  const parsed = Date.parse(raw);
  return Number.isFinite(parsed) ? parsed : 0;
}

function shouldPreferFrontendSnapshot(
  frontendSnapshot: TerminalSerializedSnapshot | null,
  serverSnapshot: TerminalRemoteSnapshot | null
) {
  if (!frontendSnapshot) {
    return false;
  }
  const serverCapturedAt = parseSnapshotCapturedAt(serverSnapshot);
  if (!serverCapturedAt) {
    return true;
  }
  // Keep a small tolerance for client/server clock skew.
  return frontendSnapshot.updatedAt + 1000 >= serverCapturedAt;
}

async function restoreServerSnapshotIfAvailable() {
  if (!terminal || !pendingServerSnapshot) {
    return false;
  }

  const snapshot = pendingServerSnapshot;
  pendingServerSnapshot = null;
  updateSnapshotDebugState();

  try {
    await enqueueTerminalTask('restore-server-snapshot', async () => {
      if (!terminal) {
        return;
      }
      if (snapshot.cols > 0 && snapshot.rows > 0) {
        terminal.resize(snapshot.cols, snapshot.rows);
      }
      const modeSequence = buildAlternateScreenSequence(snapshot.altScreen);
      await writeTerminalRaw(`${modeSequence}${remapInvisibleColors(snapshot.content)}`);
    });
    return true;
  } catch (error) {
    console.warn('[Terminal Snapshot] Failed to restore server snapshot', error);
    return false;
  }
}

async function restoreFrontendSnapshotIfAvailable() {
  if (!terminal || !pendingFrontendSnapshot) {
    return false;
  }

  const snapshot = pendingFrontendSnapshot;
  pendingFrontendSnapshot = null;
  updateSnapshotDebugState();

  try {
    await enqueueTerminalTask('restore-frontend-snapshot', async () => {
      if (!terminal) {
        return;
      }
      terminal.reset();
      if (snapshot.cols > 0 && snapshot.rows > 0) {
        terminal.resize(snapshot.cols, snapshot.rows);
      }
      await writeTerminalRaw(remapInvisibleColors(snapshot.content));
    });
    return true;
  } catch (error) {
    console.warn('[Terminal Snapshot] Failed to restore frontend snapshot', error);
    return false;
  }
}

async function restorePreferredSnapshotIfAvailable(): Promise<TerminalRestoreSource> {
  const useFrontendSnapshot = shouldPreferFrontendSnapshot(
    pendingFrontendSnapshot,
    pendingServerSnapshot
  );

  if (useFrontendSnapshot && (await restoreFrontendSnapshotIfAvailable())) {
    pendingServerSnapshot = null;
    return 'frontend';
  }

  if (await restoreServerSnapshotIfAvailable()) {
    pendingFrontendSnapshot = null;
    return 'server';
  }

  if (await restoreFrontendSnapshotIfAvailable()) {
    return 'frontend';
  }

  return 'none';
}

function persistFrontendSnapshot() {
  if (!terminal || !serializeAddon) {
    return;
  }

  try {
    terminalStore.saveSerializedSnapshot(props.tab.id, {
      content: serializeAddon.serialize(),
      updatedAt: Date.now(),
      rows: terminal.rows,
      cols: terminal.cols,
    });
  } catch (error) {
    console.warn('[Terminal Snapshot] Failed to persist frontend snapshot', error);
  }
}

async function applyTerminalMessage(payload: ServerMessage, reason = 'live') {
  if (!terminal) {
    return;
  }

  switch (payload.type) {
    case 'data':
      if (props.tab.renderMode === 'snapshot') {
        break;
      }
      if (payload.data) {
        const data = payload.data;
        await enqueueTerminalTask(`${reason}:data`, async () => {
          await writeTerminalRaw(remapInvisibleColors(decodeChunk(data)));
        });
      }
      break;
    case 'mode-prefix':
      if (payload.data) {
        const data = payload.data;
        await enqueueTerminalTask(`${reason}:mode-prefix`, async () => {
          await writeTerminalRaw(remapInvisibleColors(decodeChunk(data)));
        });
      }
      break;
    case 'exit':
      if (payload.data) {
        const data = payload.data;
        await enqueueTerminalTask(`${reason}:exit`, async () => {
          await writelnTerminalRaw(`\r\n${data}`);
        });
      }
      break;
    case 'error':
      if (payload.data) {
        const data = payload.data;
        await enqueueTerminalTask(`${reason}:error`, async () => {
          await writelnTerminalRaw(`\r\n错误: ${data}`);
        });
      }
      break;
    case 'metadata':
      if (payload.metadata) {
        props.emitter.emit('metadata', props.tab.id, payload.metadata);
      }
      break;
    default:
      break;
  }
}

function scheduleInitialViewportRepair(reason: string, delay = 48) {
  if (initialViewportRepairTimer != null) {
    window.clearTimeout(initialViewportRepairTimer);
  }

  initialViewportRepairTimer = window.setTimeout(() => {
    initialViewportRepairTimer = null;
    if (!initialViewportReady || !terminal || !isContainerVisible()) {
      return;
    }
    refreshTerminalViewport(reason);
  }, delay);
}

async function flushPendingTerminalMessages(reason: string) {
  if (!terminal || pendingTerminalMessages.length === 0) {
    return 0;
  }

  setRestorePhase('replaying', reason);
  const buffered = pendingTerminalMessages.splice(0, pendingTerminalMessages.length);
  updateLiveBufferedCount();

  for (const message of buffered) {
    await applyTerminalMessage(message, `${reason}-replay`);
  }

  updateLiveBufferedCount();
  scheduleInitialViewportRepair(`${reason}-flush`);
  return buffered.length;
}

function dispatchResizeToServer(
  cols: number,
  rows: number,
  options: { force?: boolean; reason?: string } = {}
): boolean {
  if (cols <= 0 || rows <= 0) {
    return false;
  }

  const force = options.force === true;
  if (!force && lastReportedCols === cols && lastReportedRows === rows) {
    return false;
  }

  lastReportedCols = cols;
  lastReportedRows = rows;
  props.send(props.tab.id, {
    type: 'resize',
    cols,
    rows,
  });
  return true;
}

function clearPendingServerResize() {
  if (pendingServerResizeTimer != null) {
    window.clearTimeout(pendingServerResizeTimer);
    pendingServerResizeTimer = null;
  }
  pendingServerResize = null;
}

function flushPendingServerResize() {
  if (pendingServerResizeTimer != null) {
    window.clearTimeout(pendingServerResizeTimer);
    pendingServerResizeTimer = null;
  }

  if (!pendingServerResize) {
    return false;
  }

  const { cols, rows, force } = pendingServerResize;
  pendingServerResize = null;
  return dispatchResizeToServer(cols, rows, {
    force,
    reason: force ? 'settled-force-fit' : 'settled-fit',
  });
}

function scheduleServerResize(
  cols: number,
  rows: number,
  options: { force?: boolean; immediate?: boolean } = {}
) {
  if (cols <= 0 || rows <= 0) {
    return;
  }

  const force = options.force === true;
  if (options.immediate) {
    clearPendingServerResize();
    dispatchResizeToServer(cols, rows, {
      force,
      reason: force ? 'immediate-force-fit' : 'immediate-fit',
    });
    return;
  }

  if (
    !force &&
    pendingServerResize == null &&
    lastReportedCols === cols &&
    lastReportedRows === rows
  ) {
    return;
  }

  pendingServerResize = {
    cols,
    rows,
    force: force || pendingServerResize?.force === true,
  };

  if (pendingServerResizeTimer != null) {
    window.clearTimeout(pendingServerResizeTimer);
  }
  pendingServerResizeTimer = window.setTimeout(() => {
    flushPendingServerResize();
  }, TERMINAL_SERVER_RESIZE_SETTLE_MS);
}

function commitTerminalSize(options: TerminalSizeSyncOptions = {}) {
  if (!terminal) {
    return;
  }

  const serverSync = options.serverSync ?? 'deferred';
  // `tab` points at the shared terminal store entry for this viewport.
  // Keeping cols/rows in sync here avoids stale size reads elsewhere.
  // eslint-disable-next-line vue/no-mutating-props
  props.tab.cols = terminal.cols;
  // eslint-disable-next-line vue/no-mutating-props
  props.tab.rows = terminal.rows;

  if (serverSync !== 'skip') {
    scheduleServerResize(terminal.cols, terminal.rows, {
      force: options.forceServerResize === true,
      immediate: serverSync === 'immediate',
    });
  }

  if (!initialViewportReady) {
    finalizeInitialViewport(options.finalizeReason ?? 'resize');
  }
}

async function runInitialViewportRestore(reason: string) {
  if (!terminal || initialViewportReady || !isContainerVisible()) {
    return;
  }

  setRestorePhase('restoring', reason);
  updateSnapshotDebugState();
  logRestoreTrace('initial-restore-start', { reason });

  const restoredSource = await restorePreferredSnapshotIfAvailable();
  setRestoreSource(restoredSource);

  while (pendingTerminalMessages.length > 0) {
    await flushPendingTerminalMessages(reason);
  }

  initialViewportReady = true;
  setRestorePhase('settled', reason);
  updateLiveBufferedCount();
  logRestoreTrace('initial-restore-settled', { reason, restoredSource });

  requestSnapshot('initial-restore-settled', {
    allowLive: true,
    continueAfterResize: true,
  });

  scheduleInitialViewportRepair(
    restoredSource !== 'none' ? `${restoredSource}-snapshot-restored` : reason
  );
}

function finalizeInitialViewport(reason: string) {
  if (!terminal || initialViewportReady || !isContainerVisible()) {
    return;
  }
  if (initialRestorePromise) {
    logRestoreTrace('initial-restore-already-running', { reason });
    return;
  }

  initialRestorePromise = runInitialViewportRestore(reason)
    .catch(error => {
      console.warn('[Terminal Restore] Initial restore failed', {
        sessionId: props.tab.id,
        reason,
        error,
      });
      setRestorePhase('idle', `${reason}-failed`);
      logRestoreTrace('initial-restore-failed', { reason, error });
    })
    .finally(() => {
      initialRestorePromise = null;
      flushDeferredViewportRefresh();
    });
}

function handleMessage(payload: ServerMessage) {
  if (!terminal) {
    return;
  }

  if (payload.type === 'ready') {
    // The mount-time request can run before the websocket reaches OPEN. Retry
    // once the server confirms the connection so Unix terminals receive the
    // initial redraw/snapshot request reliably.
    requestSnapshot('ready', {
      allowLive: true,
      continueAfterResize: true,
    });
    return;
  }

  if (payload.type === 'snapshot' && payload.snapshot) {
    pendingServerSnapshot = payload.snapshot;
    pendingServerSnapshotUpdatedAt = Date.now();
    updateSnapshotDebugState();
    if (terminalCatchUpActive || !isDocumentVisible()) {
      scheduleTerminalCatchUpSettle('live-server-snapshot-catch-up');
      return;
    }
    if (initialViewportReady) {
      void restoreServerSnapshotIfAvailable().then(restored => {
        if (!restored) {
          return;
        }
        setRestoreSource('server');
        logRestoreTrace('live-server-snapshot-restored', {
          reason: 'server-snapshot-live',
        });
        scheduleInitialViewportRepair('server-snapshot-live');
      });
    }
    return;
  }

  if (payload.type === 'replay-complete') {
    restoreDebugState.replayCompleteAt = Date.now();
    publishRestoreDebugState();
    logRestoreTrace('replay-complete');
    if (!initialViewportReady) {
      finalizeInitialViewport('replay-complete');
    } else {
      void flushPendingTerminalMessages('replay-complete').then(() => {
        scheduleInitialViewportRepair('replay-complete');
      });
    }
    return;
  }

  if (
    initialViewportReady &&
    (terminalCatchUpActive || !isDocumentVisible()) &&
    isDeferredTerminalMessage(payload)
  ) {
    bufferPendingTerminalMessage(payload);
    scheduleTerminalCatchUpSettle('deferred-terminal-message');
    return;
  }

  if (!initialViewportReady && isDeferredTerminalMessage(payload)) {
    bufferPendingTerminalMessage(payload);
    return;
  }

  void applyTerminalMessage(payload, 'live');
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

function sendTerminalInput(data: string) {
  if (!data) {
    return;
  }

  props.send(props.tab.id, { type: 'input', data });
}

function requestSnapshot(
  reason: string,
  options: {
    allowLive?: boolean;
    continueAfterResize?: boolean;
  } = {}
) {
  if (props.tab.renderMode !== 'snapshot' && options.allowLive !== true) {
    return;
  }
  if (pendingServerResize) {
    const resizeSent = flushPendingServerResize();
    if (resizeSent && options.continueAfterResize !== true) {
      return;
    }
  }
  props.send(props.tab.id, { type: 'snapshot-request', reason });
}

function shouldUseBrowserPasteShortcut(event: KeyboardEvent) {
  if (event.type !== 'keydown') {
    return false;
  }

  return (event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'v';
}

function shouldBlockCodexClipboardShortcut(event: KeyboardEvent) {
  if (event.type !== 'keydown') {
    return false;
  }

  if (!(props.tab.aiAssistant?.detected && props.tab.aiAssistant?.type === 'codex')) {
    return false;
  }

  return (
    event.altKey &&
    !event.ctrlKey &&
    !event.metaKey &&
    !event.shiftKey &&
    event.key.toLowerCase() === 'v'
  );
}

function getClipboardImage(clipboardData: DataTransfer | null) {
  if (!clipboardData) {
    return null;
  }

  for (const item of Array.from(clipboardData.items || [])) {
    if (!item.type.startsWith('image/')) {
      continue;
    }

    const file = item.getAsFile();
    if (file) {
      return file;
    }
  }

  for (const file of Array.from(clipboardData.files || [])) {
    if (file.type.startsWith('image/')) {
      return file;
    }
  }

  return null;
}

async function uploadImageAndInsert(
  blob: Blob | File,
  source: TerminalImageUploadSource,
  explicitFileName?: string
) {
  showTransferOverlay(t('terminal.imageUploading'), {
    tone: 'progress',
    progress: 0,
  });

  try {
    const result = await uploadTerminalImage({
      blob,
      fileName: explicitFileName,
      source,
      onProgress: progress => {
        showTransferOverlay(t('terminal.imageUploading'), {
          tone: 'progress',
          progress: progress.percent ?? transferProgress.value ?? 0,
        });
      },
    });

    sendTerminalInput(formatTerminalPathInput(result.path));
    clearTransferOverlay();
  } catch (error) {
    console.warn('[Terminal] Failed to upload image:', error);
    showTransferOverlay(t('terminal.imageUploadFailed'), {
      tone: 'error',
      progress: null,
      duration: 900,
    });
    message.error(t('terminal.imageUploadFailed'));
  }
}

function handlePaste(event: ClipboardEvent) {
  const clipboardData = event.clipboardData;
  if (!clipboardData) {
    return;
  }

  const image = getClipboardImage(clipboardData);
  if (image) {
    event.preventDefault();
    void uploadImageAndInsert(image, 'paste', image.name);
    return;
  }
  // Let xterm/browser handle text paste natively so it is only inserted once.
}

function isContainerVisible() {
  return Boolean(
    containerRef.value && containerRef.value.offsetWidth > 0 && containerRef.value.offsetHeight > 0
  );
}

const mobileKeyboard = useMobileKeyboard({
  enabled: () => Boolean(props.isMobile),
  onDismissed: () => {
    if (isDisposed || !terminal) {
      return;
    }

    syncTerminalSize({
      forceServerResize: props.tab.renderMode === 'snapshot',
      serverSync: 'immediate',
      finalizeReason: 'mobile-keyboard-dismissed',
    });

    refreshTerminalViewport('mobile-keyboard-dismissed', {
      retry: false,
      skipFit: true,
    });

    if (props.tab.renderMode === 'snapshot') {
      requestSnapshot('mobile-keyboard-dismissed', {
        continueAfterResize: true,
      });
    }
  },
});

function shouldFreezeTerminalResize() {
  return Boolean(props.isMobile) && mobileKeyboard.sync().isResizeFrozen;
}

function performLocalViewportRefresh(
  reason: string,
  options: {
    clearTextureAtlas?: boolean;
  } = {}
) {
  if (!terminal || !isContainerVisible()) {
    return;
  }

  if (options.clearTextureAtlas !== false) {
    try {
      terminal.clearTextureAtlas();
    } catch (error) {
      console.warn('[Terminal Refresh] Failed to clear texture atlas', {
        sessionId: props.tab.id,
        reason,
        error,
      });
    }
  }

  try {
    terminal.refresh(0, Math.max(terminal.rows - 1, 0));
  } catch (error) {
    console.warn('[Terminal Refresh] Failed to refresh terminal viewport', {
      sessionId: props.tab.id,
      reason,
      error,
    });
  }
}

function refreshTerminalViewport(
  reason: string,
  options: {
    clearTextureAtlas?: boolean;
    forceServerResize?: boolean;
    retry?: boolean;
    skipFit?: boolean;
  } = {}
) {
  if (!terminal) {
    return;
  }

  let forceServerResize = options.forceServerResize === true;
  if (isRestoreBlockingRefresh()) {
    deferredViewportRefresh = {
      reason,
      options: { ...options },
    };
    logRestoreTrace('refresh-deferred', { reason });
    return;
  }

  let initialRefreshFrozen = false;
  let runCount = 0;
  const runRefresh = () => {
    if (!terminal || !isContainerVisible()) {
      return;
    }

    runCount += 1;
    const frozen = options.skipFit !== true && shouldFreezeTerminalResize();
    if (runCount === 1) {
      initialRefreshFrozen = frozen;
    }

    if (!frozen && options.skipFit !== true) {
      handleResize({
        forceServerResize,
        serverSync: 'deferred',
      });
      forceServerResize = false;
    } else {
      forceServerResize = false;
    }

    performLocalViewportRefresh(reason, options);
  };

  runRefresh();

  if (options.retry !== false && !initialRefreshFrozen) {
    window.setTimeout(runRefresh, 160);
  }
}

function handleTerminalContainerResize() {
  if (!terminal || !isContainerVisible() || typeof window === 'undefined') {
    return;
  }

  // Terminal viewports can be mounted while their parent is hidden by v-show.
  // Fit and redraw as soon as the pane gets a real size instead of relying on
  // a later keypress or window resize event to wake xterm up.
  window.setTimeout(() => {
    if (!terminal || !isContainerVisible()) {
      return;
    }
    refreshTerminalViewport('container-visible', {
      forceServerResize: true,
    });
    requestSnapshot('container-visible', {
      allowLive: true,
      continueAfterResize: true,
    });
  }, 0);
}

function syncTerminalSize(options: TerminalSizeSyncOptions = {}) {
  if (!terminal || !fitAddon) {
    return false;
  }

  // 检查容器是否可见（v-show="false" 时容器尺寸为 0）
  if (
    !containerRef.value ||
    containerRef.value.offsetWidth === 0 ||
    containerRef.value.offsetHeight === 0
  ) {
    return false;
  }

  if (shouldFreezeTerminalResize()) {
    return false;
  }

  try {
    fitAddon.fit();
    commitTerminalSize(options);
    return true;
  } catch (error) {
    // 忽略 fit 可能出现的错误
    console.warn('Terminal resize failed:', error);
    return false;
  }
}

function handleResize(options: TerminalSizeSyncOptions = {}) {
  syncTerminalSize(options);
}

// 防抖版本的 resize 处理，避免窗口调整时发送大量 resize 消息阻塞输入
const debouncedResize = useDebounceFn(() => {
  handleResize({
    forceServerResize: props.tab.renderMode === 'snapshot',
    serverSync: 'deferred',
  });
}, 100);

function handleTerminalResizeAll() {
  // 延迟一下确保 DOM 更新完成，使用防抖版本避免阻塞输入
  setTimeout(() => {
    refreshTerminalViewport('terminal-resize-event', {
      forceServerResize: props.tab.renderMode === 'snapshot',
    });
  }, 10);
}

function installDebugForceRefreshHook() {
  if (typeof window === 'undefined') {
    return;
  }

  const debugWindow = window as TerminalDebugWindow;
  const registry = debugWindow[TERMINAL_DEBUG_REGISTRY] ?? new Map<string, () => boolean>();
  debugWindow[TERMINAL_DEBUG_REGISTRY] = registry;

  debugRefreshHandler = () => {
    if (!terminal || !containerRef.value || !isContainerVisible()) {
      return false;
    }
    logRestoreTrace('debug-force-refresh', {
      note: 'force refresh redraws the viewport and re-syncs the server terminal size',
    });
    syncTerminalSize({
      forceServerResize: true,
      serverSync: 'immediate',
      finalizeReason: 'debug-force-refresh',
    });
    refreshTerminalViewport('debug-force-refresh');
    terminal.scrollToTop();
    window.setTimeout(() => {
      terminal?.scrollToBottom();
    }, 60);
    return true;
  };

  registry.set(props.tab.id, debugRefreshHandler);
  debugWindow[TERMINAL_DEBUG_FN] = () => {
    for (const handler of registry.values()) {
      if (handler()) {
        return true;
      }
    }
    return false;
  };
}

onMounted(() => {
  isDisposed = false;
  restoreDebugState.mountStartedAt = Date.now();
  restoreDebugState.replayCompleteAt = 0;
  restoreDebugState.settledAt = 0;
  replayBufferedMessagesResult = null;
  setRestorePhase('idle', 'mount');
  setRestoreSource('none');

  // 获取当前选择的终端主题
  const selectedTheme = activeTerminalTheme.value;
  // 获取当前的字体设置
  const fontSettings = terminalFont.value;

  // 移动端使用固定较小字体
  const actualFontSize = getTerminalFontSize(fontSettings.fontSize);

  terminal = new Terminal({
    allowProposedApi: true,
    convertEol: true,
    rows: props.tab.rows || 24,
    cols: props.tab.cols || 80,
    cursorBlink: true,
    cursorStyle: 'block',
    cursorInactiveStyle: 'block',
    scrollOnUserInput: true,
    fontFamily: fontSettings.fontFamily || DEFAULT_TERMINAL_FONT_FAMILY,
    fontSize: actualFontSize,
    fontWeight: fontSettings.fontWeight,
    fontWeightBold: fontSettings.fontWeightBold,
    lineHeight: fontSettings.lineHeight,
    letterSpacing: fontSettings.letterSpacing,
    theme: selectedTheme,
  });

  const container = containerRef.value;
  if (container) {
    terminal.open(container);
  }

  fitAddon = new FitAddon();
  serializeAddon = new SerializeAddon();
  const webLinksAddon = new WebLinksAddon();
  const searchAddon = new SearchAddon();

  terminal.loadAddon(fitAddon);
  terminal.loadAddon(serializeAddon);
  terminal.loadAddon(webLinksAddon);
  terminal.loadAddon(searchAddon);

  // 根据设置决定是否使用 WebGL 渲染器
  // - auto: 桌面端使用 WebGL，移动端使用 Canvas（避免 DPR 缩放问题）
  // - force: 强制使用 WebGL
  // - disable: 强制禁用 WebGL
  const webglMode = terminalWebGLRenderer.value;
  const shouldUseWebGL = webglMode === 'force' || (webglMode === 'auto' && !props.isMobile);

  if (shouldUseWebGL) {
    try {
      webglAddon = new WebglAddon();
      webglAddon.onContextLoss(() => {
        console.warn('[Terminal] WebGL context lost, falling back to canvas renderer', {
          sessionId: props.tab.id,
          title: props.tab.title,
        });
        webglAddon?.dispose();
        webglAddon = null;
        window.setTimeout(() => {
          refreshTerminalViewport('webgl-context-loss', {
            clearTextureAtlas: false,
          });
        }, 0);
      });
      terminal.loadAddon(webglAddon);
    } catch (error) {
      console.warn('[Terminal] WebGL renderer failed to load, using Canvas fallback', error);
    }
  }

  if (container) {
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
        // 容器不可见，稍后重试
        if (retryIfSmall) {
          setTimeout(() => performFit(false), 200);
        }
        return;
      }

      fitAddon.fit();

      const cols = terminal.cols;
      const rows = terminal.rows;

      // 检查计算出的尺寸是否合理
      if ((cols < 20 || rows < 5) && retryIfSmall) {
        console.warn('[Terminal Init] Size too small, will retry:', { cols, rows });
        // 容器可能还没准备好，延迟再试一次
        setTimeout(() => performFit(false), 200);
        return;
      }

      finalizeInitialViewport('initial-fit');

      // 等待数据写入完成后滚动到底部
      let lastLength = terminal.buffer.active.length;
      let stableCount = 0;
      let totalChecks = 0;
      const maxChecks = 50;

      const checkStableAndScroll = () => {
        if (!terminal) return;
        totalChecks++;
        if (totalChecks >= maxChecks) {
          return;
        }

        const currentLength = terminal.buffer.active.length;
        if (currentLength === lastLength) {
          stableCount++;
          if (stableCount >= 3) {
            terminal.scrollToBottom();
            return;
          }
        } else {
          stableCount = 0;
          lastLength = currentLength;
        }
        setTimeout(checkStableAndScroll, 20);
      };
      setTimeout(checkStableAndScroll, 20);

      // 标记为已访问（初始化时就可见的终端）
      visitedTerminals.add(props.tab.id);

      commitTerminalSize({
        forceServerResize: true,
        serverSync: 'immediate',
        finalizeReason: 'initial-fit',
      });
      if (shouldAutoFocus.value) {
        terminal.focus();
      }
    };

    setTimeout(() => performFit(), 350);
  }

  terminal.onData(data => {
    sendTerminalInput(data);
  });

  terminalFocusHandler = () => {
    mobileKeyboard.setFocused(true);
  };

  terminalBlurHandler = () => {
    mobileKeyboard.setFocused(false);
  };

  terminal.textarea?.addEventListener('focus', terminalFocusHandler);
  terminal.textarea?.addEventListener('blur', terminalBlurHandler);

  keydownCaptureHandler = (event: KeyboardEvent) => {
    if (shouldUseBrowserPasteShortcut(event)) {
      // Keep browser paste enabled, but stop xterm/Codex from consuming Ctrl/Cmd+V first.
      event.stopImmediatePropagation();
      event.stopPropagation();
      return;
    }
  };

  terminal.attachCustomKeyEventHandler(event => {
    if (shouldBlockCodexClipboardShortcut(event)) {
      event.preventDefault();
      message.warning(t('terminal.nativePasteUnavailable'));
      return false;
    }

    return true;
  });

  pasteHandler = (event: ClipboardEvent) => {
    handlePaste(event);
  };

  dragOverHandler = (event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
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

    for (const file of Array.from(files)) {
      if (!file.type.startsWith('image/')) {
        continue;
      }

      await uploadImageAndInsert(file, 'drop', file.name);
    }
  };

  container?.addEventListener('keydown', keydownCaptureHandler, true);
  container?.addEventListener('paste', pasteHandler, true);
  container?.addEventListener('dragover', dragOverHandler);
  container?.addEventListener('drop', dropHandler);

  pendingFrontendSnapshot = terminalStore.getSerializedSnapshot(props.tab.id) ?? null;
  pendingServerSnapshot = terminalStore.getLatestServerSnapshot(props.tab.id) ?? null;
  updateSnapshotDebugState();

  props.emitter.on(props.tab.id, handleMessage);
  props.emitter.on('terminal-resize-all', handleTerminalResizeAll);
  props.emitter.on(`terminal-resize-${props.tab.id}`, handleTerminalResizeAll);
  props.emitter.on(`terminal-activated-${props.tab.id}`, handleTerminalActivated);
  props.emitter.on('terminal-blur-all', handleTerminalBlurEvent);
  window.addEventListener('resize', debouncedResize);
  window.addEventListener('focus', handleTerminalWindowFocus);
  window.addEventListener('pageshow', handleTerminalWindowPageShow);
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleTerminalDocumentVisibilityChange);
  }
  installDebugForceRefreshHook();

  // Replay any buffered messages that were received while this component was unmounted
  // This ensures no data is lost when switching between projects
  replayBufferedMessagesResult = terminalStore.replayBufferedMessages(props.tab.id);
  updateReplayDebugState(replayBufferedMessagesResult);
  logRestoreTrace('mount-replayed-buffered-messages', replayBufferedMessagesResult ?? {});
  requestSnapshot('mount', {
    allowLive: true,
    continueAfterResize: true,
  });

  if (container && typeof ResizeObserver !== 'undefined') {
    containerResizeObserver = new ResizeObserver(() => {
      handleTerminalContainerResize();
    });
    containerResizeObserver.observe(container);
  }
});

function handleTerminalBlurEvent() {
  mobileKeyboard.setFocused(false);
  terminal?.blur();
}

// 处理终端激活事件，首次访问时滚动到底部
function handleTerminalActivated() {
  if (!terminal || !fitAddon) return;

  if (isRestoreBlockingRefresh()) {
    logRestoreTrace('terminal-activated-deferred');
    scheduleInitialViewportRepair('terminal-activated');
    return;
  }

  refreshTerminalViewport('terminal-activated');
  requestSnapshot('activate', {
    allowLive: true,
    continueAfterResize: true,
  });

  const isFirstVisit = !visitedTerminals.has(props.tab.id);
  if (isFirstVisit) {
    visitedTerminals.add(props.tab.id);
    // 先 fit 确保终端尺寸正确，然后滚动到底部
    try {
      fitAddon.fit();
    } catch {
      // ignore
    }
    // 延迟执行滚动，确保 fit 完成
    setTimeout(() => {
      if (!terminal) return;
      // 先滚动到顶部，再滚动到底部，确保滚动生效
      terminal.scrollToTop();
      setTimeout(() => {
        terminal?.scrollToBottom();
      }, 100);
    }, 50);
  }
}

function handleTerminalDocumentVisibilityChange() {
  if (!isDocumentVisible()) {
    beginTerminalCatchUp('document-hidden');
    return;
  }
  beginTerminalCatchUp('document-visible', { requestSnapshot: true });
}

function handleTerminalWindowFocus() {
  if (!isDocumentVisible()) {
    return;
  }
  beginTerminalCatchUp('window-focus', { requestSnapshot: true });
}

function handleTerminalWindowPageShow() {
  if (!isDocumentVisible()) {
    return;
  }
  beginTerminalCatchUp('window-pageshow', { requestSnapshot: true });
}

onBeforeUnmount(() => {
  isDisposed = true;
  persistFrontendSnapshot();
  props.emitter.off(props.tab.id, handleMessage);
  props.emitter.off('terminal-resize-all', handleTerminalResizeAll);
  props.emitter.off(`terminal-resize-${props.tab.id}`, handleTerminalResizeAll);
  props.emitter.off(`terminal-activated-${props.tab.id}`, handleTerminalActivated);
  props.emitter.off('terminal-blur-all', handleTerminalBlurEvent);
  window.removeEventListener('resize', debouncedResize);
  window.removeEventListener('focus', handleTerminalWindowFocus);
  window.removeEventListener('pageshow', handleTerminalWindowPageShow);
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleTerminalDocumentVisibilityChange);
  }
  containerResizeObserver?.disconnect();
  containerResizeObserver = null;
  if (containerRef.value) {
    if (terminal?.textarea && terminalFocusHandler) {
      terminal.textarea.removeEventListener('focus', terminalFocusHandler);
    }
    if (terminal?.textarea && terminalBlurHandler) {
      terminal.textarea.removeEventListener('blur', terminalBlurHandler);
    }
    if (keydownCaptureHandler) {
      containerRef.value.removeEventListener('keydown', keydownCaptureHandler, true);
    }
    if (pasteHandler) {
      containerRef.value.removeEventListener('paste', pasteHandler, true);
    }
    if (dragOverHandler) {
      containerRef.value.removeEventListener('dragover', dragOverHandler);
    }
    if (dropHandler) {
      containerRef.value.removeEventListener('drop', dropHandler);
    }
  }
  if (initialViewportRepairTimer != null) {
    window.clearTimeout(initialViewportRepairTimer);
    initialViewportRepairTimer = null;
  }
  clearPendingServerResize();
  clearTerminalCatchUpTimer();
  terminalCatchUpActive = false;
  terminalCatchUpSnapshotRequestedAt = 0;
  terminalCatchUpSnapshotAttempts = 0;
  initialViewportReady = false;
  initialRestorePromise = null;
  terminalTaskQueue = Promise.resolve();
  deferredViewportRefresh = null;
  lastReportedCols = 0;
  lastReportedRows = 0;
  clearPendingTerminalMessages();
  pendingServerSnapshot = null;
  pendingFrontendSnapshot = null;
  pendingServerSnapshotUpdatedAt = 0;
  if (typeof window !== 'undefined') {
    const debugWindow = window as TerminalDebugWindow;
    debugWindow[TERMINAL_DEBUG_REGISTRY]?.delete(props.tab.id);
    debugWindow[TERMINAL_RESTORE_DEBUG_REGISTRY]?.delete(props.tab.id);
  }
  serializeAddon?.dispose();
  serializeAddon = null;
  webglAddon?.dispose();
  webglAddon = null;
  terminal?.dispose();
  terminal = null;
  fitAddon?.dispose();
  fitAddon = null;
  keydownCaptureHandler = null;
  pasteHandler = null;
  dragOverHandler = null;
  dropHandler = null;
  terminalFocusHandler = null;
  terminalBlurHandler = null;
  debugRefreshHandler = null;
  clearTransferOverlay();
  mobileKeyboard.setFocused(false);
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
  pointer-events: none;
  background: var(--terminal-overlay-bg, rgba(0, 0, 0, 0.35));
  color: var(--terminal-overlay-color, var(--kanban-terminal-fg, #f6f8ff));
  font-size: 13px;
}
</style>

<style>
.terminal.xterm {
  height: 100%;
}
</style>
