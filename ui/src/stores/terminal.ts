import { defineStore } from 'pinia';
import { reactive, watch } from 'vue';
import EventEmitter from 'eventemitter3';
import Apis, { alovaInstance, urlBase } from '@/api';
import { extractItem } from '@/api/response';
import type { TerminalCreateInputBody } from '@/api/globals';
import type { TerminalSession } from '@/types/models';
import {
  DEFAULT_TERMINAL_RENDER_MODE,
  DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS,
  sanitizeTerminalRenderMode,
  sanitizeTerminalSnapshotIntervalMs,
  type TerminalRenderMode,
} from '@/constants/terminalRenderMode';
import {
  DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS,
  DEFAULT_TERMINAL_CONNECTION_POLICY,
  sanitizeTerminalConnectionPolicy,
  type TerminalConnectionPolicy,
} from '@/constants/terminalConnectionPolicy';
import { resolveWsUrl } from '@/utils/ws';
import { useProjectStore } from '@/stores/project';
import { useSettingsStore } from '@/stores/settings';

export type ClientStatus = 'connecting' | 'ready' | 'closed' | 'error';
export type TerminalConnectionRole = 'active' | 'mirror' | 'detached';

export type TerminalRemoteSnapshot = {
  kind: 'full';
  content: string;
  rows: number;
  cols: number;
  sequence: number;
  baseSequence: number;
  altScreen: boolean;
  cursorVisible: boolean;
  modeFlags: number;
  capturedAt?: string;
  lines?: string[];
  cursor?: string;
};

type TerminalRemoteSnapshotDelta = {
  kind: 'delta';
  rows: number;
  cols: number;
  sequence: number;
  baseSequence: number;
  altScreen: boolean;
  cursorVisible: boolean;
  modeFlags: number;
  capturedAt?: string;
  changedLines: Array<{
    index: number;
    content: string;
  }>;
  cursor: string;
};

type TerminalRemoteSnapshotFrame = TerminalRemoteSnapshot | TerminalRemoteSnapshotDelta;

export interface TerminalTabState extends TerminalSession {
  clientStatus: ClientStatus;
  connectionRole: TerminalConnectionRole;
  renderMode: TerminalRenderMode;
  /** Mode confirmed by the currently attached websocket. */
  connectionRenderMode?: TerminalRenderMode;
  snapshotIntervalMs: number;
  useGlobalRenderMode: boolean;
  useGlobalSnapshotInterval: boolean;
}

export type TerminalSerializedSnapshot = {
  content: string;
  updatedAt: number;
  rows: number;
  cols: number;
};

export type ServerMessage = {
  type:
    | 'ready'
    | 'data'
    | 'mode-prefix'
    | 'exit'
    | 'error'
    | 'metadata'
    | 'snapshot'
    | 'replay-complete'
    | 'render-mode';
  data?: string;
  cols?: number;
  rows?: number;
  mode?: TerminalRenderMode;
  snapshotIntervalMs?: number;
  snapshotCompressionEnabled?: boolean;
  snapshotIncrementalEnabled?: boolean;
  snapshot?: TerminalRemoteSnapshot;
	metadata?: {
    title?: string;
    processPid?: number;
    processStatus?: string;
    processHasChildren?: boolean;
		runningCommand?: string;
		aiAssistant?: {
      type: string;
      name: string;
      displayName: string;
      detected: boolean;
      command?: string;
    };
  };
};

const TERMINAL_SNAPSHOT_PREFIX = '\x1b[0m\x1b[2J\x1b[3J\x1b[H';
const TERMINAL_SNAPSHOT_FRAME_VERSION = 6;
export type BufferedTerminalMessage = {
  payload: ServerMessage;
  receivedAt: number;
  localOrder: number;
};

export type ReplayBufferedMessagesResult = {
  count: number;
  firstReceivedAt?: number;
  lastReceivedAt?: number;
  lastLocalOrder?: number;
};

export type TerminalCreateOptions = {
  worktreeId?: string;
  workingDir?: string;
  title?: string;
	rows?: number;
	cols?: number;
	/** 插入到指定 sessionId 之后，用于复制标签时保持位置 */
  insertAfterSessionId?: string;
};

type SessionRecord = {
  projectId: string;
  tab: TerminalTabState;
};

type TerminalRenderPreference = {
  useGlobalRenderMode: boolean;
  renderMode?: TerminalRenderMode;
  useGlobalSnapshotInterval: boolean;
  snapshotIntervalMs?: number;
};

type TerminalSessionListEvent = {
  type?: string;
  projectId?: string;
  sessions?: TerminalSession[];
};

const LAST_ACTIVE_TAB_STORAGE_KEY = 'kanban-terminal-last-active';
const TAB_RENDER_PREFERENCE_STORAGE_KEY = 'kanban-terminal-render-preferences';
const TERMINAL_EVENTS_WS_PATH = '/api/v1/terminals/events';
const TERMINAL_EVENT_RECONNECT_BASE_DELAY_MS = 1200;
const TERMINAL_EVENT_RECONNECT_MAX_DELAY_MS = 15000;

const storedActiveTabs = loadStoredActiveTabs();
const storedRenderPreferences = loadStoredRenderPreferences();

function loadStoredActiveTabs() {
  if (typeof window === 'undefined' || !window.localStorage) {
    return new Map<string, string>();
  }
  try {
    const raw = window.localStorage.getItem(LAST_ACTIVE_TAB_STORAGE_KEY);
    if (!raw) {
      return new Map<string, string>();
    }
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const result = new Map<string, string>();
    Object.entries(parsed).forEach(([projectId, value]) => {
      if (!projectId || typeof value !== 'string') {
        return;
      }
      const normalized = value.trim();
      if (normalized) {
        result.set(projectId, normalized);
      }
    });
    return result;
  } catch (error) {
    console.warn('[Terminal Store] Failed to parse stored active tabs', error);
    return new Map<string, string>();
  }
}

function persistStoredActiveTabs() {
  if (typeof window === 'undefined' || !window.localStorage) {
    return;
  }
  if (!storedActiveTabs.size) {
    window.localStorage.removeItem(LAST_ACTIVE_TAB_STORAGE_KEY);
    return;
  }
  const payload: Record<string, string> = {};
  storedActiveTabs.forEach((tabId, projectId) => {
    if (tabId) {
      payload[projectId] = tabId;
    }
  });
  if (Object.keys(payload).length === 0) {
    window.localStorage.removeItem(LAST_ACTIVE_TAB_STORAGE_KEY);
    return;
  }
  window.localStorage.setItem(LAST_ACTIVE_TAB_STORAGE_KEY, JSON.stringify(payload));
}

function rememberStoredActiveTab(projectId: string, tabId: string) {
  if (!projectId) {
    return;
  }
  const normalized = tabId.trim();
  if (!normalized) {
    forgetStoredActiveTab(projectId);
    return;
  }
  const current = storedActiveTabs.get(projectId);
  if (current === normalized) {
    return;
  }
  storedActiveTabs.set(projectId, normalized);
  persistStoredActiveTabs();
}

function forgetStoredActiveTab(projectId: string) {
  if (!projectId) {
    return;
  }
  if (!storedActiveTabs.has(projectId)) {
    return;
  }
  storedActiveTabs.delete(projectId);
  persistStoredActiveTabs();
}

function loadStoredRenderPreferences() {
  if (typeof window === 'undefined' || !window.localStorage) {
    return new Map<string, TerminalRenderPreference>();
  }
  try {
    const raw = window.localStorage.getItem(TAB_RENDER_PREFERENCE_STORAGE_KEY);
    if (!raw) {
      return new Map<string, TerminalRenderPreference>();
    }
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const result = new Map<string, TerminalRenderPreference>();
    Object.entries(parsed).forEach(([key, value]) => {
      if (!key || typeof value !== 'object' || value == null) {
        return;
      }
      const preference = sanitizeRenderPreference(value as Partial<TerminalRenderPreference>);
      result.set(key, preference);
    });
    return result;
  } catch (error) {
    console.warn('[Terminal Store] Failed to parse stored render preferences', error);
    return new Map<string, TerminalRenderPreference>();
  }
}

function persistStoredRenderPreferences() {
  if (typeof window === 'undefined' || !window.localStorage) {
    return;
  }
  if (!storedRenderPreferences.size) {
    window.localStorage.removeItem(TAB_RENDER_PREFERENCE_STORAGE_KEY);
    return;
  }
  const payload: Record<string, TerminalRenderPreference> = {};
  storedRenderPreferences.forEach((preference, key) => {
    payload[key] = {
      useGlobalRenderMode: preference.useGlobalRenderMode,
      renderMode: preference.renderMode,
      useGlobalSnapshotInterval: preference.useGlobalSnapshotInterval,
      snapshotIntervalMs: preference.snapshotIntervalMs,
    };
  });
  window.localStorage.setItem(TAB_RENDER_PREFERENCE_STORAGE_KEY, JSON.stringify(payload));
}

function sanitizeRenderPreference(
  value?: Partial<TerminalRenderPreference>
): TerminalRenderPreference {
  return {
    useGlobalRenderMode: value?.useGlobalRenderMode !== false,
    renderMode: sanitizeTerminalRenderMode(value?.renderMode),
    useGlobalSnapshotInterval: value?.useGlobalSnapshotInterval !== false,
    snapshotIntervalMs: sanitizeTerminalSnapshotIntervalMs(value?.snapshotIntervalMs),
  };
}

function buildRenderPreferenceKey(projectId: string, sessionId: string) {
  return `${projectId}:${sessionId}`;
}

function sortableOrderIndex(session: TerminalSession) {
  const value = Number(session.orderIndex ?? 0);
  return Number.isFinite(value) ? value : 0;
}

function sortSessionsWithServerOrder(_projectId: string, sessions: TerminalSession[]) {
  if (!sessions.length) {
    return sessions;
  }
  return [...sessions].sort((a, b) => {
    const orderA = sortableOrderIndex(a);
    const orderB = sortableOrderIndex(b);
    if ((orderA > 0 || orderB > 0) && orderA !== orderB) {
      return orderA - orderB;
    }
    return a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id);
  });
}

function supportsSnapshotZlibCompression() {
  return typeof DecompressionStream !== 'undefined';
}

async function inflateSnapshotPayload(payload: Uint8Array) {
  if (!supportsSnapshotZlibCompression()) {
    throw new Error('zlib snapshot compression is not supported in this browser');
  }

  const stream = new Blob([payload]).stream().pipeThrough(new DecompressionStream('deflate'));
  const buffer = await new Response(stream).arrayBuffer();
  return new Uint8Array(buffer);
}

function decodeSnapshotText(decoder: TextDecoder, bytes: Uint8Array) {
  if (bytes.byteLength === 0) {
    return '';
  }
  return decoder.decode(bytes);
}

function readSnapshotString(
  view: DataView,
  bytes: Uint8Array,
  offset: number,
  decoder: TextDecoder
) {
  if (offset + 4 > bytes.byteLength) {
    return null;
  }

  const length = view.getUint32(offset, false);
  offset += 4;
  if (offset + length > bytes.byteLength) {
    return null;
  }

  const value = decodeSnapshotText(decoder, bytes.subarray(offset, offset + length));
  return {
    value,
    nextOffset: offset + length,
  };
}

function buildSnapshotContent(lines?: string[], cursor = '', fallbackContent = '') {
  if (!Array.isArray(lines)) {
    return fallbackContent;
  }
  return `${TERMINAL_SNAPSHOT_PREFIX}${lines.join('\r\n')}${cursor}`;
}

function parseSnapshotFramePayload(
  rows: number,
  cols: number,
  frameKind: number,
  sequence: number,
  baseSequence: number,
  altScreen: boolean,
  cursorVisible: boolean,
  modeFlags: number,
  capturedAt: string | undefined,
  encodedContent: Uint8Array
): TerminalRemoteSnapshotFrame | null {
  const view = new DataView(
    encodedContent.buffer,
    encodedContent.byteOffset,
    encodedContent.byteLength
  );
  const decoder = new TextDecoder('utf-8');
  let offset = 0;

  if (frameKind === 1) {
    if (offset + 2 > encodedContent.byteLength) {
      return null;
    }

    const changeCount = view.getUint16(offset, false);
    offset += 2;
    const changedLines: TerminalRemoteSnapshotDelta['changedLines'] = [];
    for (let index = 0; index < changeCount; index += 1) {
      if (offset + 2 > encodedContent.byteLength) {
        return null;
      }
      const rowIndex = view.getUint16(offset, false);
      offset += 2;
      const rowValue = readSnapshotString(view, encodedContent, offset, decoder);
      if (!rowValue) {
        return null;
      }
      offset = rowValue.nextOffset;
      changedLines.push({
        index: rowIndex,
        content: rowValue.value,
      });
    }

    const cursorValue = readSnapshotString(view, encodedContent, offset, decoder);
    if (!cursorValue) {
      return null;
    }
    return {
      kind: 'delta',
      rows,
      cols,
      sequence,
      baseSequence,
      altScreen,
      cursorVisible,
      modeFlags,
      capturedAt,
      changedLines,
      cursor: cursorValue.value,
    };
  }

  const lines: string[] = [];
  for (let row = 0; row < rows; row += 1) {
    const rowValue = readSnapshotString(view, encodedContent, offset, decoder);
    if (!rowValue) {
      return null;
    }
    offset = rowValue.nextOffset;
    lines.push(rowValue.value);
  }

  const cursorValue = readSnapshotString(view, encodedContent, offset, decoder);
  if (!cursorValue) {
    return null;
  }
  return {
    kind: 'full',
    rows,
    cols,
    sequence,
    baseSequence,
    altScreen,
    cursorVisible,
    modeFlags,
    capturedAt,
    lines,
    cursor: cursorValue.value,
    content: buildSnapshotContent(lines, cursorValue.value),
  };
}

async function parseBinarySnapshotFrame(
  payload: ArrayBuffer
): Promise<TerminalRemoteSnapshotFrame | null> {
  if (!(payload instanceof ArrayBuffer) || payload.byteLength < 27) {
    return null;
  }

  const view = new DataView(payload);
  const version = view.getUint8(0);
  if (version !== TERMINAL_SNAPSHOT_FRAME_VERSION) {
    return null;
  }

  const rows = view.getUint16(1, false);
  const cols = view.getUint16(3, false);
  const capturedAtMs = Number(view.getBigUint64(5, false));
  const headerSize = 27;
  if (payload.byteLength < headerSize) {
    return null;
  }
  const flags = view.getUint8(13);
  const altScreen = (flags & (1 << 0)) !== 0;
  const cursorVisible = (flags & (1 << 1)) !== 0;
  const modeFlags = view.getUint32(14, false);
  const compressed = (flags & (1 << 2)) !== 0;
  const sequence = view.getUint32(18, false);
  const baseSequence = view.getUint32(22, false);
  const frameKind = view.getUint8(26);
  const encodedContent = new Uint8Array(payload, headerSize);
  const contentBytes = compressed ? await inflateSnapshotPayload(encodedContent) : encodedContent;
  const capturedAt =
    capturedAtMs > 0 && Number.isFinite(capturedAtMs)
      ? new Date(capturedAtMs).toISOString()
      : undefined;

  return parseSnapshotFramePayload(
    rows,
    cols,
    frameKind,
    sequence,
    baseSequence,
    altScreen,
    cursorVisible,
    modeFlags,
    capturedAt,
    contentBytes
  );
}

function assembleServerSnapshotFrame(
  previous: TerminalRemoteSnapshot | undefined,
  frame: TerminalRemoteSnapshotFrame
): TerminalRemoteSnapshot | null {
  if (frame.kind === 'full') {
    return {
      ...frame,
      kind: 'full',
      content: buildSnapshotContent(frame.lines, frame.cursor, frame.content),
    };
  }

  if (!previous || !Array.isArray(previous.lines)) {
    return null;
  }
  if (previous.sequence !== frame.baseSequence) {
    return null;
  }
  if (
    previous.rows !== frame.rows ||
    previous.cols !== frame.cols ||
    previous.altScreen !== frame.altScreen ||
    previous.modeFlags !== frame.modeFlags
  ) {
    return null;
  }

  const lines = [...previous.lines];
  for (const patch of frame.changedLines) {
    if (patch.index < 0 || patch.index >= lines.length) {
      return null;
    }
    lines[patch.index] = patch.content;
  }

  const cursor = frame.cursor ?? previous.cursor ?? '';
  return {
    kind: 'full',
    rows: frame.rows,
    cols: frame.cols,
    sequence: frame.sequence,
    baseSequence: frame.baseSequence,
    altScreen: frame.altScreen,
    cursorVisible: frame.cursorVisible,
    modeFlags: frame.modeFlags,
    capturedAt: frame.capturedAt,
    lines,
    cursor,
    content: buildSnapshotContent(lines, cursor),
  };
}

export const useTerminalStore = defineStore('terminal', () => {
  const tabStore = reactive(new Map<string, TerminalTabState[]>());
  const sessionIndex = new Map<string, SessionRecord>();
  const activeTabByProject = reactive(new Map<string, string>());
  const sockets = new Map<string, WebSocket>();
  const manualCloseIds = new Set<string>();
  const pausedSocketIds = new Set<string>();
  let globalLoadToken = 0;
  const projectLoadTokens = new Map<string, number>();
  const emitter = new EventEmitter();
	const cachedCounts = reactive(new Map<string, number>());
	const projectConnectionRefCounts = reactive(new Map<string, number>());
	const settingsStore = useSettingsStore();
  // Buffer for WebSocket messages when no listener is attached
  // This prevents data loss when TerminalViewport is unmounted but WebSocket is still active
  const messageBuffers = new Map<string, BufferedTerminalMessage[]>();
  const MESSAGE_BUFFER_MAX_SIZE = 5000; // Limit buffer size to prevent memory issues
  const latestServerSnapshots = new Map<string, TerminalRemoteSnapshot>();
  const latestServerSnapshotSequence = new Map<string, number>();
  const serializedSnapshots = new Map<string, TerminalSerializedSnapshot>();
  let nextBufferedMessageOrder = 0;
  let eventSocket: WebSocket | null = null;
  let eventPendingSocket: WebSocket | null = null;
  let eventConnectPromise: Promise<void> | null = null;
  let eventReconnectTimer: number | null = null;
  let eventReconnectAttempt = 0;

  function hasRetainedProjects() {
    return projectConnectionRefCounts.size > 0;
  }

  function clearTerminalEventReconnectTimer() {
    if (eventReconnectTimer != null && typeof window !== 'undefined') {
      window.clearTimeout(eventReconnectTimer);
      eventReconnectTimer = null;
    }
  }

  function closeTerminalEventStream() {
    clearTerminalEventReconnectTimer();
    eventConnectPromise = null;
    eventReconnectAttempt = 0;
    const socketsToClose = [eventSocket, eventPendingSocket].filter((socket): socket is WebSocket =>
      Boolean(socket)
    );
    eventSocket = null;
    eventPendingSocket = null;
    socketsToClose.forEach(socket => {
      try {
        socket.close();
      } catch (error) {
        console.warn('[Terminal Store] Failed to close terminal event stream', error);
      }
    });
  }

  function scheduleTerminalEventReconnect() {
    clearTerminalEventReconnectTimer();
    if (!hasRetainedProjects() || typeof window === 'undefined') {
      return;
    }
    const delayMs = Math.min(
      TERMINAL_EVENT_RECONNECT_BASE_DELAY_MS * 2 ** eventReconnectAttempt,
      TERMINAL_EVENT_RECONNECT_MAX_DELAY_MS
    );
    eventReconnectAttempt += 1;
    eventReconnectTimer = window.setTimeout(() => {
      eventReconnectTimer = null;
      void openTerminalEventStream().catch(() => undefined);
    }, delayMs);
  }

  function applySessionListEvent(frame: TerminalSessionListEvent) {
    const projectId = String(frame.projectId || '').trim();
    if (!projectId || frame.type !== 'sessions') {
      return;
    }
    const sessions = Array.isArray(frame.sessions)
      ? frame.sessions.map(session => ({
          ...session,
          projectId: session.projectId || projectId,
        }))
      : [];
    cachedCounts.set(projectId, sessions.length);
    if (!projectConnectionRefCounts.has(projectId) && !tabStore.has(projectId)) {
      return;
    }
    reconcileSessions(projectId, sessions);
  }

  function openTerminalEventStream(): Promise<void> {
    if (eventSocket && eventSocket.readyState === WebSocket.OPEN) {
      return Promise.resolve();
    }
    if (eventConnectPromise) {
      return eventConnectPromise;
    }
    if (!hasRetainedProjects()) {
      return Promise.resolve();
    }
    clearTerminalEventReconnectTimer();
    const connectPromise = new Promise<void>((resolve, reject) => {
      let settled = false;
      let socket: WebSocket;
      try {
        socket = new WebSocket(resolveWsUrl(TERMINAL_EVENTS_WS_PATH));
        eventPendingSocket = socket;
      } catch (error) {
        scheduleTerminalEventReconnect();
        reject(error);
        return;
      }
      socket.onopen = () => {
        settled = true;
        eventPendingSocket = null;
        eventConnectPromise = null;
        if (!hasRetainedProjects()) {
          socket.close();
          resolve();
          return;
        }
        eventSocket = socket;
        eventReconnectAttempt = 0;
        resolve();
      };
      socket.onmessage = event => {
        try {
          applySessionListEvent(JSON.parse(event.data) as TerminalSessionListEvent);
        } catch (error) {
          console.error('[Terminal Store] Failed to parse terminal event frame', error);
        }
      };
      socket.onerror = event => {
        console.error('[Terminal Store] terminal event websocket error', event);
      };
      socket.onclose = () => {
        if (eventSocket === socket) {
          eventSocket = null;
        }
        if (eventPendingSocket === socket) {
          eventPendingSocket = null;
        }
        eventConnectPromise = null;
        scheduleTerminalEventReconnect();
        if (!settled) {
          reject(new Error('terminal event stream closed before opening'));
        }
      };
    });
    eventConnectPromise = connectPromise;
    return connectPromise;
  }

  function getGlobalRenderMode() {
    return sanitizeTerminalRenderMode(
      settingsStore.defaultTerminalRenderMode ?? DEFAULT_TERMINAL_RENDER_MODE
    );
  }

  function getGlobalSnapshotIntervalMs() {
    return sanitizeTerminalSnapshotIntervalMs(
      settingsStore.defaultTerminalSnapshotIntervalMs ?? DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS
    );
  }

  function getGlobalSnapshotCompressionEnabled() {
    return (
      settingsStore.defaultTerminalSnapshotZlibCompression !== false &&
      supportsSnapshotZlibCompression()
    );
  }

  function getGlobalConnectionPolicy(): TerminalConnectionPolicy {
    return sanitizeTerminalConnectionPolicy(
      settingsStore.terminalConnectionPolicy ?? DEFAULT_TERMINAL_CONNECTION_POLICY
    );
  }

  function getInactiveSnapshotIntervalMs() {
    return sanitizeTerminalSnapshotIntervalMs(
      settingsStore.inactiveTerminalSnapshotIntervalMs ??
        DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS
    );
  }

  function getStoredRenderPreference(projectId: string, sessionId: string) {
    return storedRenderPreferences.get(buildRenderPreferenceKey(projectId, sessionId));
  }

  function getEffectiveRenderMode(projectId: string, sessionId: string) {
    const preference = getStoredRenderPreference(projectId, sessionId);
    if (!preference || preference.useGlobalRenderMode) {
      return getGlobalRenderMode();
    }
    return sanitizeTerminalRenderMode(preference.renderMode);
  }

  function getEffectiveSnapshotIntervalMs(projectId: string, sessionId: string) {
    const preference = getStoredRenderPreference(projectId, sessionId);
    if (!preference || preference.useGlobalSnapshotInterval) {
      return getGlobalSnapshotIntervalMs();
    }
    return sanitizeTerminalSnapshotIntervalMs(preference.snapshotIntervalMs);
  }

  function applyRenderPreferenceToTab(tab: TerminalTabState) {
    const preference = getStoredRenderPreference(tab.projectId, tab.id);
    tab.useGlobalRenderMode = preference?.useGlobalRenderMode !== false;
    tab.useGlobalSnapshotInterval = preference?.useGlobalSnapshotInterval !== false;
    tab.renderMode = getEffectiveRenderMode(tab.projectId, tab.id);
    tab.connectionRenderMode = getRequestedConnectionRenderMode(tab);
    tab.snapshotIntervalMs = getEffectiveSnapshotIntervalMs(tab.projectId, tab.id);
    return tab;
  }

  function persistRenderPreference(
    projectId: string,
    sessionId: string,
    partial: Partial<TerminalRenderPreference>
  ) {
    const storageKey = buildRenderPreferenceKey(projectId, sessionId);
    const current = storedRenderPreferences.get(storageKey);
    const next = sanitizeRenderPreference({
      ...current,
      ...partial,
    });
    storedRenderPreferences.set(storageKey, next);
    persistStoredRenderPreferences();
    return next;
  }

  function clearRenderPreference(projectId: string, sessionId: string) {
    const storageKey = buildRenderPreferenceKey(projectId, sessionId);
    if (storedRenderPreferences.delete(storageKey)) {
      persistStoredRenderPreferences();
    }
  }

  function buildRenderModeMessage(
    sessionId: string
  ): Pick<
    ServerMessage,
    | 'type'
    | 'mode'
    | 'snapshotIntervalMs'
    | 'snapshotCompressionEnabled'
    | 'snapshotIncrementalEnabled'
  > | null {
    const record = sessionIndex.get(sessionId);
    if (!record) {
      return null;
    }
    const tab = record.tab;
    const mode = getRequestedConnectionRenderMode(tab);
    const snapshotIntervalMs =
      tab.connectionRole === 'mirror'
        ? getInactiveSnapshotIntervalMs()
        : getEffectiveSnapshotIntervalMs(record.projectId, sessionId);
    return {
      type: 'render-mode',
      mode,
      snapshotIntervalMs,
      snapshotCompressionEnabled: getGlobalSnapshotCompressionEnabled(),
      snapshotIncrementalEnabled: true,
    };
  }

  function getConnectionRenderMode(tab: TerminalTabState): TerminalRenderMode {
    if (tab.connectionRenderMode) {
      return sanitizeTerminalRenderMode(tab.connectionRenderMode);
    }
    return getRequestedConnectionRenderMode(tab);
  }

  function getRequestedConnectionRenderMode(tab: TerminalTabState): TerminalRenderMode {
    return tab.connectionRole === 'mirror'
      ? 'snapshot'
      : sanitizeTerminalRenderMode(tab.renderMode);
  }

  function setConnectionRenderMode(sessionId: string, mode: TerminalRenderMode) {
    const record = sessionIndex.get(sessionId);
    if (!record) {
      return;
    }
    const bucket = tabStore.get(record.projectId);
    if (!bucket) {
      return;
    }
    const index = bucket.findIndex(tab => tab.id === sessionId);
    if (index === -1) {
      return;
    }
    bucket[index] = {
      ...bucket[index],
      connectionRenderMode: sanitizeTerminalRenderMode(mode),
    };
    record.tab = bucket[index];
  }

  function clearTerminalSnapshotState(sessionId: string) {
    latestServerSnapshots.delete(sessionId);
    latestServerSnapshotSequence.delete(sessionId);
    serializedSnapshots.delete(sessionId);
  }

  function clearSnapshotState(sessionId: string) {
    if (!sessionId) {
      return;
    }
    clearTerminalSnapshotState(sessionId);
  }

  function sendRenderModePreference(sessionId: string) {
    const message = buildRenderModeMessage(sessionId);
    if (!message) {
      return false;
    }
    return send(sessionId, message);
  }

  function updateRenderModeAck(
    sessionId: string,
    mode: TerminalRenderMode | undefined,
    snapshotIntervalMs: number | undefined,
    _snapshotCompressionEnabled: boolean | undefined,
    _snapshotIncrementalEnabled: boolean | undefined
  ) {
    const record = sessionIndex.get(sessionId);
    if (!record) {
      return;
    }
    const bucket = tabStore.get(record.projectId);
    if (!bucket) {
      return;
    }
    const index = bucket.findIndex(tab => tab.id === sessionId);
    if (index === -1) {
      return;
    }
    const actualMode = sanitizeTerminalRenderMode(mode ?? getConnectionRenderMode(bucket[index]));
    bucket[index] = {
      ...bucket[index],
      connectionRenderMode: actualMode,
      snapshotIntervalMs: sanitizeTerminalSnapshotIntervalMs(
        snapshotIntervalMs ?? bucket[index].snapshotIntervalMs
      ),
    };
    record.tab = bucket[index];
    if (actualMode === 'live') {
      clearTerminalSnapshotState(sessionId);
    }
  }

  function applyGlobalRenderDefaultsToTabs() {
    sessionIndex.forEach(({ projectId }, sessionId) => {
      const record = sessionIndex.get(sessionId);
      if (!record) {
        return;
      }
      const bucket = tabStore.get(projectId);
      if (!bucket) {
        return;
      }
      const index = bucket.findIndex(tab => tab.id === sessionId);
      if (index === -1) {
        return;
      }
      const current = bucket[index];
      const nextRenderMode = current.useGlobalRenderMode
        ? getGlobalRenderMode()
        : current.renderMode;
      const nextSnapshotIntervalMs = current.useGlobalSnapshotInterval
        ? getGlobalSnapshotIntervalMs()
        : current.snapshotIntervalMs;
      const nextMode = sanitizeTerminalRenderMode(nextRenderMode);
      const renderModeChanged = nextMode !== current.renderMode;
      const currentConnectionMode = getConnectionRenderMode(current);
      bucket[index] = {
        ...current,
        renderMode: nextMode,
        connectionRenderMode: hasAttachedSocket(sessionId)
          ? currentConnectionMode
          : renderModeChanged
            ? getRequestedConnectionRenderMode({ ...current, renderMode: nextMode })
            : currentConnectionMode,
        snapshotIntervalMs: sanitizeTerminalSnapshotIntervalMs(nextSnapshotIntervalMs),
      };
      record.tab = bucket[index];
      void sendRenderModePreference(sessionId);
    });
  }

  watch(
    () => [
      settingsStore.defaultTerminalRenderMode,
      settingsStore.defaultTerminalSnapshotIntervalMs,
      settingsStore.defaultTerminalSnapshotZlibCompression,
    ],
    () => {
      applyGlobalRenderDefaultsToTabs();
    }
  );

  watch(
    () => [
      settingsStore.terminalConnectionPolicy,
      settingsStore.inactiveTerminalSnapshotIntervalMs,
    ],
    () => {
      projectConnectionRefCounts.forEach((count, projectId) => {
        if (count > 0) {
          applyConnectionPolicy(projectId);
        }
      });
    }
  );

  function getTabs(projectId?: string) {
    if (!projectId) {
      return [];
    }
    return tabStore.get(projectId) ?? [];
  }

  /** 返回所有项目下已加载的终端会话（用于跨项目的「全部终端」视图）。 */
  function getAllTabs(): Array<{ projectId: string; tab: TerminalTabState }> {
    const items: Array<{ projectId: string; tab: TerminalTabState }> = [];
    tabStore.forEach((bucket, projectId) => {
      bucket.forEach(tab => {
        items.push({ projectId, tab });
      });
    });
    return items;
  }

  function getActiveTabId(projectId?: string) {
    if (!projectId) {
      return '';
    }
    const bucket = tabStore.get(projectId);
    if (!bucket || bucket.length === 0) {
      return '';
    }
    const current = activeTabByProject.get(projectId);
    if (current && bucket.some(tab => tab.id === current)) {
      rememberStoredActiveTab(projectId, current);
      return current;
    }
    if (tryRestoreActiveTabFromStorage(projectId)) {
      const restored = activeTabByProject.get(projectId) ?? '';
      if (restored) {
        rememberStoredActiveTab(projectId, restored);
        return restored;
      }
    }
    const fallback = bucket[0]?.id ?? '';
    if (fallback) {
      setActiveTab(projectId, fallback);
      return fallback;
    }
    return '';
  }

  function setActiveTab(projectId: string | undefined, tabId?: string) {
    if (!projectId) {
      return;
    }
    const normalized = typeof tabId === 'string' ? tabId.trim() : '';
    if (normalized) {
      const current = activeTabByProject.get(projectId);
      if (current !== normalized) {
        activeTabByProject.set(projectId, normalized);
      }
      rememberStoredActiveTab(projectId, normalized);
      applyConnectionPolicy(projectId);
      return;
    }
    if (activeTabByProject.has(projectId)) {
      activeTabByProject.delete(projectId);
    }
    forgetStoredActiveTab(projectId);
    applyConnectionPolicy(projectId);
  }

  function prepareProject(projectId: string) {
    ensureBucket(projectId);
    ensureActiveTab(projectId);
  }

  async function restoreSessions(projectId: string) {
    const response = (await alovaInstance
      .Post(`/api/v1/projects/${projectId}/terminals/restore`, {}, { cacheFor: 0 })
      .send()) as { items?: TerminalSession[] } | undefined;
    return (response?.items ?? []).map(session => ({
      ...session,
      projectId,
    }));
  }

  async function loadSessions(projectId?: string) {
    const resolved = ensureProjectSelected(projectId);
    const token = ++globalLoadToken;
    projectLoadTokens.set(resolved, token);
    try {
      const response = await Apis.terminalSession
        .list({
          pathParams: { projectId: resolved },
          cacheFor: 0,
        })
        .send();
      if (projectLoadTokens.get(resolved) !== token) {
        return;
      }
      let items = (response?.items ?? []) as unknown as TerminalSession[];
      if (items.length === 0) {
        try {
          items = await restoreSessions(resolved);
        } catch (restoreError) {
          console.error('Failed to restore terminal sessions', restoreError);
        }
        if (projectLoadTokens.get(resolved) !== token) {
          return;
        }
      }
      reconcileSessions(resolved, items as unknown as TerminalSession[]);
      // 更新终端计数缓存
      cachedCounts.set(resolved, items.length);
    } catch (error) {
      console.error('Failed to load terminal sessions', error);
    }
  }

  async function createSession(
    projectId: string | undefined,
    options: TerminalCreateOptions
  ): Promise<string> {
    const resolved = ensureProjectSelected(projectId);

    // 如果没有提供 worktreeId，自动选择
    let worktreeId = options.worktreeId;
    if (!worktreeId) {
      const projectStore = useProjectStore();
      const worktrees = projectStore.worktrees;

      if (worktrees.length === 0) {
        throw new Error('当前项目没有可用的分支');
      }

      // 优先选择主分支，否则选择第一个
      const mainWorktree = worktrees.find(w => w.isMain);
      worktreeId = mainWorktree ? mainWorktree.id : worktrees[0].id;
    }

    const payload: TerminalCreateInputBody & { insertAfterSessionId?: string } = {
      workingDir: options.workingDir ?? '',
      title: options.title ?? '',
      rows: options.rows ?? 0,
      cols: options.cols ?? 0,
    };
	if (options.insertAfterSessionId) {
      payload.insertAfterSessionId = options.insertAfterSessionId;
    }
    const response = await Apis.terminalSession
      .create({
        pathParams: {
          projectId: resolved,
          worktreeId: worktreeId,
        },
        data: payload,
        cacheFor: 0,
      })
      .send();
    if (!response?.item) {
      throw new Error('�����ն�ʧ��');
    }
    const session = response.item as unknown as TerminalSession;
    attachOrUpdateSession(session, {
      activate: true,
      projectIdOverride: resolved,
      insertAfterSessionId: options.insertAfterSessionId,
    });
    emitter.emit('terminal:created', {
      projectId: resolved,
      sessionId: session.id,
    });
    // 更新终端计数缓存
    const currentCount = cachedCounts.get(resolved) ?? 0;
    cachedCounts.set(resolved, currentCount + 1);
    return session.id;
  }

  async function renameSession(projectId: string | undefined, sessionId: string, title: string) {
    const resolved = ensureProjectSelected(projectId);
    const normalized = title.trim();
    if (!normalized) {
      throw new Error('��������µ��ն˱��⡣');
    }
    const response = await Apis.terminalSession
      .rename({
        pathParams: {
          projectId: resolved,
          sessionId,
        },
        data: {
          title: normalized,
        },
        cacheFor: 0,
      })
      .send();
    if (!response?.item) {
      return;
    }
    attachOrUpdateSession(response.item as unknown as TerminalSession, {
      projectIdOverride: resolved,
    });
  }

  async function closeSession(projectId: string | undefined, sessionId: string) {
    const resolved = ensureProjectSelected(projectId);
    await Apis.terminalSession
      .close({
        pathParams: { projectId: resolved, sessionId },
        cacheFor: 0,
      })
      .send();
    disconnectTab(sessionId, true);
  }

  function setSessionRenderMode(
    projectId: string | undefined,
    sessionId: string,
    mode: TerminalRenderMode | null
  ) {
    const resolved = ensureProjectSelected(projectId);
    const record = sessionIndex.get(sessionId);
    if (!record || record.projectId !== resolved) {
      return false;
    }

    const nextPreference =
      mode == null
        ? { useGlobalRenderMode: true }
        : { useGlobalRenderMode: false, renderMode: sanitizeTerminalRenderMode(mode) };
    persistRenderPreference(resolved, sessionId, nextPreference);

    const bucket = tabStore.get(resolved);
    if (!bucket) {
      return false;
    }
    const index = bucket.findIndex(tab => tab.id === sessionId);
    if (index === -1) {
      return false;
    }
    const currentConnectionMode = getConnectionRenderMode(bucket[index]);
    bucket[index] = applyRenderPreferenceToTab({ ...bucket[index] });
    if (hasAttachedSocket(sessionId)) {
      bucket[index].connectionRenderMode = currentConnectionMode;
    }
    record.tab = bucket[index];
    if (getConnectionRenderMode(record.tab) === 'live') {
      clearTerminalSnapshotState(sessionId);
    }
    return sendRenderModePreference(sessionId);
  }

  function setSessionSnapshotInterval(
    projectId: string | undefined,
    sessionId: string,
    snapshotIntervalMs: number | null
  ) {
    const resolved = ensureProjectSelected(projectId);
    const record = sessionIndex.get(sessionId);
    if (!record || record.projectId !== resolved) {
      return false;
    }

    const nextPreference =
      snapshotIntervalMs == null
        ? { useGlobalSnapshotInterval: true }
        : {
            useGlobalSnapshotInterval: false,
            snapshotIntervalMs: sanitizeTerminalSnapshotIntervalMs(snapshotIntervalMs),
          };
    persistRenderPreference(resolved, sessionId, nextPreference);

    const bucket = tabStore.get(resolved);
    if (!bucket) {
      return false;
    }
    const index = bucket.findIndex(tab => tab.id === sessionId);
    if (index === -1) {
      return false;
    }
    const currentConnectionMode = getConnectionRenderMode(bucket[index]);
    bucket[index] = applyRenderPreferenceToTab({ ...bucket[index] });
    bucket[index].connectionRenderMode = currentConnectionMode;
    record.tab = bucket[index];
    return sendRenderModePreference(sessionId);
  }

  function send(sessionId: string, message: any): boolean {
    const socket = sockets.get(sessionId);
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(message));
      return true;
    }
    return false;
  }

  function hasAttachedSocket(sessionId: string) {
    const socket = sockets.get(sessionId);
    return Boolean(
      socket && socket.readyState !== WebSocket.CLOSING && socket.readyState !== WebSocket.CLOSED
    );
  }

  function isProjectConnectionActive(projectId: string | undefined) {
    if (!projectId) {
      return false;
    }
    return (projectConnectionRefCounts.get(projectId) ?? 0) > 0;
  }

  function pauseSocket(sessionId: string) {
    const socket = sockets.get(sessionId);
    if (!socket) {
      return;
    }
    pausedSocketIds.add(sessionId);
    socket.close();
    sockets.delete(sessionId);
  }

  function disconnectTab(sessionId: string, remove = true) {
    const socket = sockets.get(sessionId);
    if (socket) {
      manualCloseIds.add(sessionId);
      socket.close();
      sockets.delete(sessionId);
    }
    pausedSocketIds.delete(sessionId);
    if (remove) {
      const record = sessionIndex.get(sessionId);
      if (!record) {
        return;
      }
      const bucket = tabStore.get(record.projectId);
      if (bucket) {
        const index = bucket.findIndex(tab => tab.id === sessionId);
        if (index !== -1) {
          bucket.splice(index, 1);
          // 更新终端计数缓存
          const currentCount = cachedCounts.get(record.projectId) ?? 0;
          cachedCounts.set(record.projectId, Math.max(0, currentCount - 1));
        }
        if (bucket.length === 0) {
          tabStore.delete(record.projectId);
        }
      }
      sessionIndex.delete(sessionId);
      if (activeTabByProject.get(record.projectId) === sessionId) {
        const nextId = tabStore.get(record.projectId)?.[0]?.id;
        setActiveTab(record.projectId, nextId);
      }
      messageBuffers.delete(sessionId); // Clean up message buffer
      latestServerSnapshots.delete(sessionId);
      latestServerSnapshotSequence.delete(sessionId);
      serializedSnapshots.delete(sessionId);
      clearRenderPreference(record.projectId, sessionId);
    }
  }

  /**
   * Replay buffered messages for a session and clear the buffer.
   * Called when TerminalViewport remounts after being unmounted (e.g., project switch).
   */
  function replayBufferedMessages(sessionId: string): ReplayBufferedMessagesResult {
    const buffer = messageBuffers.get(sessionId);
    if (!buffer || buffer.length === 0) {
      return { count: 0 };
    }
    const firstReceivedAt = buffer[0]?.receivedAt;
    const lastReceivedAt = buffer[buffer.length - 1]?.receivedAt;
    const lastLocalOrder = buffer[buffer.length - 1]?.localOrder;
    // Emit all buffered messages
    for (const entry of buffer) {
      emitter.emit(sessionId, entry.payload);
    }
    // Clear the buffer after replay
    messageBuffers.delete(sessionId);
    console.log(`[Terminal] Replayed ${buffer.length} buffered messages for session:`, sessionId);
    return {
      count: buffer.length,
      firstReceivedAt,
      lastReceivedAt,
      lastLocalOrder,
    };
  }

  function saveSerializedSnapshot(sessionId: string, snapshot?: TerminalSerializedSnapshot | null) {
    if (!sessionId) {
      return;
    }
    if (!snapshot) {
      serializedSnapshots.delete(sessionId);
      return;
    }
    serializedSnapshots.set(sessionId, snapshot);
  }

  function getSerializedSnapshot(sessionId: string) {
    if (!sessionId) {
      return undefined;
    }
    return serializedSnapshots.get(sessionId);
  }

  function getLatestServerSnapshot(sessionId: string) {
    if (!sessionId) {
      return undefined;
    }
    return latestServerSnapshots.get(sessionId);
  }

  function applyServerSnapshotFrame(sessionId: string, frame: TerminalRemoteSnapshotFrame) {
    if (!sessionId) {
      return null;
    }

    const assembled = assembleServerSnapshotFrame(latestServerSnapshots.get(sessionId), frame);
    if (!assembled) {
      send(sessionId, {
        type: 'snapshot-request',
        reason: 'delta-baseline-miss',
      });
      return null;
    }

    latestServerSnapshots.set(sessionId, assembled);
    return assembled;
  }

  function resolveConnectionRole(projectId: string, sessionId: string): TerminalConnectionRole {
    if (!isProjectConnectionActive(projectId)) {
      return 'detached';
    }

    const activeSessionId = activeTabByProject.get(projectId) ?? '';
    if (activeSessionId && activeSessionId === sessionId) {
      return 'active';
    }

    return getGlobalConnectionPolicy() === 'active-plus-mirror' ? 'mirror' : 'detached';
  }

  function updateConnectionRole(
    sessionId: string,
    role: TerminalConnectionRole
  ): TerminalTabState | undefined {
    const record = sessionIndex.get(sessionId);
    if (!record) {
      return undefined;
    }
    if (record.tab.connectionRole === role) {
      return record.tab;
    }
    const bucket = tabStore.get(record.projectId);
    if (!bucket) {
      return record.tab;
    }
    const index = bucket.findIndex(tab => tab.id === sessionId);
    if (index === -1) {
      return record.tab;
    }

    const currentConnectionMode = getConnectionRenderMode(bucket[index]);
    bucket[index] = {
      ...bucket[index],
      connectionRole: role,
      connectionRenderMode:
        hasAttachedSocket(sessionId)
          ? currentConnectionMode
          : role === 'mirror'
            ? 'snapshot'
            : sanitizeTerminalRenderMode(bucket[index].renderMode),
    };
    record.tab = bucket[index];
    return record.tab;
  }

  function applyConnectionPolicy(projectId: string | undefined) {
    if (!projectId) {
      return;
    }

    const bucket = tabStore.get(projectId);
    if (!bucket || bucket.length === 0) {
      return;
    }

    bucket.forEach(tab => {
      const desiredRole = resolveConnectionRole(projectId, tab.id);
      const nextTab = updateConnectionRole(tab.id, desiredRole) ?? tab;
      const socket = sockets.get(tab.id);

      if (desiredRole === 'detached') {
        if (socket) {
          pauseSocket(tab.id);
        }
        return;
      }

      if (
        !socket ||
        socket.readyState === WebSocket.CLOSING ||
        socket.readyState === WebSocket.CLOSED
      ) {
        connect(nextTab);
        return;
      }

      if (socket.readyState === WebSocket.OPEN) {
        sendRenderModePreference(tab.id);
      }
    });
  }

  function ensureBucket(projectId: string) {
    if (!projectId) {
      return [];
    }
    let bucket = tabStore.get(projectId);
    if (!bucket) {
      bucket = reactive<TerminalTabState[]>([]);
      tabStore.set(projectId, bucket);
    }
    return bucket;
  }

  function tryRestoreActiveTabFromStorage(projectId: string) {
    if (!projectId) {
      return false;
    }
    const storedId = storedActiveTabs.get(projectId);
    if (!storedId) {
      return false;
    }
    const bucket = tabStore.get(projectId);
    if (!bucket || bucket.length === 0) {
      return false;
    }
    const exists = bucket.some(tab => tab.id === storedId);
    if (!exists) {
      forgetStoredActiveTab(projectId);
      return false;
    }
    setActiveTab(projectId, storedId);
    return true;
  }

  function ensureActiveTab(projectId: string) {
    if (!projectId) {
      return;
    }
    const bucket = tabStore.get(projectId);
    if (!bucket || bucket.length === 0) {
      return;
    }
    const current = activeTabByProject.get(projectId);
    if (current && bucket.some(tab => tab.id === current)) {
      rememberStoredActiveTab(projectId, current);
      return;
    }
    if (tryRestoreActiveTabFromStorage(projectId)) {
      return;
    }
    setActiveTab(projectId, bucket[0].id);
  }

  function applyLocalTabOrder(projectId: string, bucket: TerminalTabState[]) {
    bucket.forEach((tab, index) => {
      const nextTab = {
        ...tab,
        orderIndex: (index + 1) * 1000,
      };
      bucket.splice(index, 1, nextTab);
      const record = sessionIndex.get(nextTab.id);
      if (record) {
        record.tab = nextTab;
      }
    });
    applyConnectionPolicy(projectId);
  }

  function restoreProjectTabs(projectId: string, previousTabs: TerminalTabState[]) {
    const bucket = ensureBucket(projectId);
    bucket.splice(0, bucket.length, ...previousTabs);
    previousTabs.forEach(tab => {
      sessionIndex.set(tab.id, { projectId, tab });
    });
    ensureActiveTab(projectId);
    applyConnectionPolicy(projectId);
  }

  async function persistTerminalTabMove(
    projectId: string,
    sessionId: string,
    previousSessionId: string,
    nextSessionId: string,
    previousTabs: TerminalTabState[]
  ) {
    try {
      const response = await alovaInstance
        .Post(
          `/api/v1/projects/${projectId}/terminals/${sessionId}/move`,
          {
            previousSessionId,
            nextSessionId,
          },
          { cacheFor: 0 }
        )
        .send();
      const session = extractItem(response) as unknown as TerminalSession | undefined;
      if (session) {
        attachOrUpdateSession(session, { projectIdOverride: projectId });
      }
    } catch (error) {
      restoreProjectTabs(projectId, previousTabs);
      console.error('[Terminal Store] Failed to persist terminal tab order', error);
    }
  }

  function reorderTabs(projectId: string | undefined, fromIndex: number, toIndex: number) {
    if (!projectId) {
      return;
    }
    const bucket = tabStore.get(projectId);
    if (!bucket || bucket.length < 2) {
      return;
    }
    if (fromIndex === toIndex) {
      return;
    }
    if (fromIndex < 0 || fromIndex >= bucket.length) {
      return;
    }
    const clampedToIndex = Math.max(0, Math.min(bucket.length - 1, toIndex));
    const previousTabs = bucket.map(tab => ({ ...tab }));
    const [tab] = bucket.splice(fromIndex, 1);
    if (!tab) {
      return;
    }
    bucket.splice(clampedToIndex, 0, tab);
    applyLocalTabOrder(projectId, bucket);
    const movedIndex = bucket.findIndex(item => item.id === tab.id);
    const previousSessionId = bucket[movedIndex - 1]?.id ?? '';
    const nextSessionId = bucket[movedIndex + 1]?.id ?? '';
    void persistTerminalTabMove(projectId, tab.id, previousSessionId, nextSessionId, previousTabs);
  }

  function attachOrUpdateSession(
    session: TerminalSession,
    options?: { activate?: boolean; projectIdOverride?: string; insertAfterSessionId?: string }
  ) {
    const existing = sessionIndex.get(session.id);
    if (existing) {
      const immutableProjectId = existing.projectId;
      const payloadProjectId = normalizeProjectId(session.projectId);
      if (payloadProjectId && payloadProjectId !== immutableProjectId) {
        console.warn(
          '[Terminal Store] Received mismatched project for terminal session',
          session.id,
          'payload project:',
          payloadProjectId,
          'tracked as:',
          immutableProjectId
        );
      }
      const updatedTab: TerminalTabState = {
        ...existing.tab,
        ...session,
        projectId: immutableProjectId,
        connectionRole: existing.tab.connectionRole,
        renderMode: existing.tab.renderMode,
        connectionRenderMode: existing.tab.connectionRenderMode,
        snapshotIntervalMs: existing.tab.snapshotIntervalMs,
        useGlobalRenderMode: existing.tab.useGlobalRenderMode,
        useGlobalSnapshotInterval: existing.tab.useGlobalSnapshotInterval,
      };
      // 用 splice 替换以触发 Vue 响应式更新
      const bucket = tabStore.get(immutableProjectId);
      if (bucket) {
        const index = bucket.findIndex(t => t.id === session.id);
        if (index !== -1) {
          bucket.splice(index, 1, updatedTab);
        }
      }
      existing.tab = updatedTab;
      if (options?.activate) {
        setActiveTab(immutableProjectId, session.id);
      }
      applyConnectionPolicy(immutableProjectId);
      return updatedTab;
    }

    const resolvedProjectId = resolveSessionProjectId(session, options?.projectIdOverride);
    if (!resolvedProjectId) {
      console.warn('[Terminal Store] Skip terminal session with unknown projectId', session.id);
      return;
    }

    const bucket = ensureBucket(resolvedProjectId);
    const tab = applyRenderPreferenceToTab({
      ...session,
      projectId: resolvedProjectId,
      clientStatus: 'connecting',
      connectionRole: 'detached',
      renderMode: getEffectiveRenderMode(resolvedProjectId, session.id),
      connectionRenderMode: getEffectiveRenderMode(resolvedProjectId, session.id),
      snapshotIntervalMs: getEffectiveSnapshotIntervalMs(resolvedProjectId, session.id),
      useGlobalRenderMode: true,
      useGlobalSnapshotInterval: true,
    });
    // 如果指定了 insertAfterSessionId，在其后插入；否则添加到末尾
    if (options?.insertAfterSessionId) {
      const insertIndex = bucket.findIndex(t => t.id === options.insertAfterSessionId);
      if (insertIndex !== -1) {
        bucket.splice(insertIndex + 1, 0, tab);
      } else {
        bucket.push(tab);
      }
    } else {
      bucket.push(tab);
    }
    sessionIndex.set(tab.id, { projectId: resolvedProjectId, tab });
    if (options?.activate) {
      setActiveTab(resolvedProjectId, tab.id);
    } else if (!activeTabByProject.get(resolvedProjectId)) {
      const storedId = storedActiveTabs.get(resolvedProjectId);
      if (!storedId) {
        setActiveTab(resolvedProjectId, tab.id);
      } else if (storedId === tab.id) {
        setActiveTab(resolvedProjectId, tab.id);
      }
    }
    applyConnectionPolicy(resolvedProjectId);
    return tab;
  }

  function updateTabStatus(sessionId: string, status: ClientStatus) {
    const record = sessionIndex.get(sessionId);
    if (!record) return;

    const bucket = tabStore.get(record.projectId);
    if (!bucket) return;

    const index = bucket.findIndex(t => t.id === sessionId);
    if (index === -1) return;

    bucket[index] = { ...bucket[index], clientStatus: status };
    record.tab = bucket[index];
  }

  function updateTabMetadata(sessionId: string, metadata: ServerMessage['metadata']) {
    const record = sessionIndex.get(sessionId);
    if (!record || !metadata) return;

    const bucket = tabStore.get(record.projectId);
    if (!bucket) return;

    const index = bucket.findIndex(t => t.id === sessionId);
    if (index === -1) return;

    const nextTitle = metadata.title;
    bucket[index] = {
      ...bucket[index],
      processPid: metadata.processPid,
      processStatus: metadata.processStatus as 'idle' | 'busy' | 'unknown' | undefined,
      processHasChildren: metadata.processHasChildren,
      runningCommand: metadata.runningCommand,
      aiAssistant: metadata.aiAssistant,
      title: typeof nextTitle === 'string' ? nextTitle : bucket[index].title,
    };
    record.tab = bucket[index];
  }

  function connect(tab: TerminalTabState) {
    if (!isProjectConnectionActive(tab.projectId)) {
      return;
    }
    const existingSocket = sockets.get(tab.id);
    if (
      existingSocket &&
      (existingSocket.readyState === WebSocket.OPEN ||
        existingSocket.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }
    pausedSocketIds.delete(tab.id);
    const resolvedWsURL = resolveWsUrl(tab.wsUrl || tab.wsPath, urlBase);
    const wsURL = new URL(resolvedWsURL);
    const initialRenderMode = getRequestedConnectionRenderMode(tab);
    const initialSnapshotIntervalMs =
      tab.connectionRole === 'mirror'
        ? getInactiveSnapshotIntervalMs()
        : getEffectiveSnapshotIntervalMs(tab.projectId, tab.id);
    wsURL.searchParams.set('renderMode', initialRenderMode);
    wsURL.searchParams.set('snapshotIntervalMs', String(initialSnapshotIntervalMs));
    wsURL.searchParams.set(
      'snapshotCompression',
      getGlobalSnapshotCompressionEnabled() ? 'zlib' : 'none'
    );
    // A reconnect starts with the requested mode; the server may acknowledge a
    // live fallback if a snapshot mirror is unavailable.
    setConnectionRenderMode(tab.id, initialRenderMode);
    const socket = new WebSocket(wsURL.toString());
    socket.binaryType = 'arraybuffer';
    sockets.set(tab.id, socket);

    socket.addEventListener('open', () => {
      updateTabStatus(tab.id, 'ready');
      latestServerSnapshotSequence.delete(tab.id);
      const renderModeMessage = buildRenderModeMessage(tab.id);
      if (renderModeMessage) {
        socket.send(JSON.stringify(renderModeMessage));
      }
      socket.send(
        JSON.stringify({
          type: 'resize',
          cols: tab.cols,
          rows: tab.rows,
        })
      );
    });

    socket.addEventListener('message', event => {
      void (async () => {
        try {
          if (sockets.get(tab.id) !== socket) {
            return;
          }

          let payload: ServerMessage;
          if (typeof event.data === 'string') {
            payload = JSON.parse(event.data) as ServerMessage;
          } else {
            const currentRecord = sessionIndex.get(tab.id);
            if (!currentRecord || getConnectionRenderMode(currentRecord.tab) !== 'snapshot') {
              return;
            }
            const frame = await parseBinarySnapshotFrame(event.data as ArrayBuffer);
            const parsedRecord = sessionIndex.get(tab.id);
            if (
              !frame ||
              sockets.get(tab.id) !== socket ||
              !parsedRecord ||
              getConnectionRenderMode(parsedRecord.tab) !== 'snapshot'
            ) {
              return;
            }
            const previousSequence = latestServerSnapshotSequence.get(tab.id) ?? -1;
            if (frame.sequence <= previousSequence) {
              return;
            }
            latestServerSnapshotSequence.set(tab.id, frame.sequence);
            const snapshot = applyServerSnapshotFrame(tab.id, frame);
            if (!snapshot || sockets.get(tab.id) !== socket) {
              return;
            }
            payload = {
              type: 'snapshot',
              snapshot,
            };
          }
          if (payload.type === 'render-mode') {
            updateRenderModeAck(
              tab.id,
              payload.mode,
              payload.snapshotIntervalMs,
              payload.snapshotCompressionEnabled,
              payload.snapshotIncrementalEnabled
            );
          }
          if (payload.type === 'ready') {
            updateTabStatus(tab.id, 'ready');
          } else if (payload.type === 'exit') {
            updateTabStatus(tab.id, 'closed');
          } else if (payload.type === 'error') {
            updateTabStatus(tab.id, 'error');
          } else if (payload.type === 'metadata' && payload.metadata) {
            // Update tab metadata in realtime
            updateTabMetadata(tab.id, payload.metadata);
          }
          // Check if there are any listeners for this session
          // If not, buffer the message for later replay when component remounts
          if (emitter.listenerCount(tab.id) > 0) {
            emitter.emit(tab.id, payload);
          } else if (
            payload.type === 'data' ||
            payload.type === 'mode-prefix' ||
            payload.type === 'exit' ||
            payload.type === 'error'
          ) {
            // Buffer terminal content events while the viewport is unmounted.
            let buffer = messageBuffers.get(tab.id);
            if (!buffer) {
              buffer = [];
              messageBuffers.set(tab.id, buffer);
            }
            nextBufferedMessageOrder += 1;
            buffer.push({
              payload,
              receivedAt: Date.now(),
              localOrder: nextBufferedMessageOrder,
            });
            // Limit buffer size to prevent memory issues
            if (buffer.length > MESSAGE_BUFFER_MAX_SIZE) {
              buffer.shift();
            }
          }
        } catch (error) {
          console.error('[Terminal] Failed to process websocket message', error);
          // ignore malformed payloads
        }
      })();
    });

    socket.addEventListener('close', () => {
      sockets.delete(tab.id);
      if (pausedSocketIds.has(tab.id)) {
        pausedSocketIds.delete(tab.id);
        return;
      }
      if (manualCloseIds.has(tab.id)) {
        manualCloseIds.delete(tab.id);
        updateTabStatus(tab.id, 'closed');
        return;
      }
      const record = sessionIndex.get(tab.id);
      if (record && resolveConnectionRole(record.projectId, tab.id) !== 'detached') {
        updateTabStatus(tab.id, 'connecting');
        void handleUnexpectedSocketClose(record.projectId, tab.id);
      } else {
        if (record && !isProjectConnectionActive(record.projectId)) {
          return;
        }
        updateTabStatus(tab.id, 'closed');
      }
    });

    socket.addEventListener('error', () => {
      updateTabStatus(tab.id, 'error');
    });
  }

  async function handleUnexpectedSocketClose(projectId: string, sessionId: string) {
    await new Promise(resolve => setTimeout(resolve, 1000));

    const initialRecord = sessionIndex.get(sessionId);
    if (initialRecord && resolveConnectionRole(projectId, sessionId) === 'detached') {
      return;
    }

    try {
      await loadSessions(projectId);
    } catch (error) {
      console.error('[Terminal Store] Failed to reload sessions after websocket close', error);
    }

    const nextRecord = sessionIndex.get(sessionId);
    if (nextRecord && resolveConnectionRole(projectId, sessionId) !== 'detached') {
      connect(nextRecord.tab);
      return;
    }

    const nextActiveId = activeTabByProject.get(projectId) ?? '';
    const activeRecord = nextActiveId ? sessionIndex.get(nextActiveId) : undefined;
    if (activeRecord && resolveConnectionRole(projectId, activeRecord.tab.id) !== 'detached') {
      connect(activeRecord.tab);
    }
  }

  function reconcileSessions(projectId: string, sessions: TerminalSession[]) {
    const bucket = ensureBucket(projectId);
    const incomingIds = new Set(sessions.map(session => session.id));
    for (const tab of [...bucket]) {
      if (!incomingIds.has(tab.id)) {
        disconnectTab(tab.id, true);
      }
    }
    const orderedSessions = sortSessionsWithServerOrder(projectId, sessions);
    for (const session of orderedSessions) {
      attachOrUpdateSession(session, { projectIdOverride: projectId });
    }
    const orderById = new Map(orderedSessions.map((session, index) => [session.id, index]));
    const finalBucket = tabStore.get(projectId);
    if (finalBucket) {
      finalBucket.sort((left, right) => {
        const leftIndex = orderById.get(left.id) ?? Number.MAX_SAFE_INTEGER;
        const rightIndex = orderById.get(right.id) ?? Number.MAX_SAFE_INTEGER;
        if (leftIndex !== rightIndex) {
          return leftIndex - rightIndex;
        }
        return left.id.localeCompare(right.id);
      });
    }
    if (!finalBucket || finalBucket.length === 0) {
      setActiveTab(projectId, undefined);
    } else {
      ensureActiveTab(projectId);
    }
  }

  function ensureProjectSelected(projectId?: string) {
    if (!projectId) {
      throw new Error('����ѡ����Ŀ');
    }
    return projectId;
  }

  function normalizeProjectId(value?: string) {
    return typeof value === 'string' ? value.trim() : '';
  }

  function resolveSessionProjectId(session: TerminalSession, requested?: string) {
    const fromPayload = normalizeProjectId(session.projectId);
    const requestedProjectId = normalizeProjectId(requested);
    if (fromPayload && requestedProjectId && fromPayload !== requestedProjectId) {
      console.warn(
        '[Terminal Store] Server response project mismatch for terminal session',
        session.id,
        'payload:',
        fromPayload,
        'requested:',
        requestedProjectId
      );
    }
    return fromPayload || requestedProjectId;
  }

  function getTerminalCount(projectId?: string) {
    if (!projectId) {
      return 0;
    }
    const bucket = tabStore.get(projectId);
    return bucket?.length ?? 0;
  }

  async function loadTerminalCounts() {
    try {
      const response = await Apis.terminalSession.terminalCounts({ cacheFor: 0 }).send();
      const counts = response?.counts ?? {};

      // 更新缓存的终端数量
      cachedCounts.clear();
      Object.entries(counts).forEach(([projectId, count]) => {
        cachedCounts.set(projectId, count);
      });

      return counts;
    } catch (error) {
      console.error('Failed to load terminal counts', error);
      return {};
    }
  }

  async function closeAllSessions(projectId: string | undefined) {
    const resolved = ensureProjectSelected(projectId);
    const tabs = getTabs(resolved);

    // 关闭所有终端
    const closePromises = tabs.map(tab => closeSession(resolved, tab.id));
    await Promise.allSettled(closePromises);
  }

  function getSessionById(sessionId: string) {
    return sessionIndex.get(sessionId)?.tab;
  }

  function focusSession(projectId: string | undefined, sessionId?: string) {
    if (!projectId || !sessionId) {
      return false;
    }
    const bucket = tabStore.get(projectId);
    if (!bucket || !bucket.some(tab => tab.id === sessionId)) {
      return false;
    }
    setActiveTab(projectId, sessionId);
    emitter.emit('terminal:ensure-expanded', { projectId });
    return true;
  }

  function retainProjectConnections(projectId: string | undefined) {
    if (!projectId) {
      return;
    }
    const nextCount = (projectConnectionRefCounts.get(projectId) ?? 0) + 1;
    projectConnectionRefCounts.set(projectId, nextCount);
    void openTerminalEventStream().catch(error => {
      console.error('[Terminal Store] Failed to open terminal event stream', error);
    });
    if (nextCount !== 1) {
      return;
    }
    applyConnectionPolicy(projectId);
  }

  function releaseProjectConnections(projectId: string | undefined) {
    if (!projectId) {
      return;
    }
    const currentCount = projectConnectionRefCounts.get(projectId) ?? 0;
    if (currentCount <= 1) {
      projectConnectionRefCounts.delete(projectId);
      const bucket = tabStore.get(projectId);
      if (!bucket) {
        if (!hasRetainedProjects()) {
          closeTerminalEventStream();
        }
        return;
      }
      bucket.forEach(tab => {
        updateConnectionRole(tab.id, 'detached');
        pauseSocket(tab.id);
      });
      if (!hasRetainedProjects()) {
        closeTerminalEventStream();
      }
      return;
    }
    projectConnectionRefCounts.set(projectId, currentCount - 1);
  }

  return {
    emitter,
    getTabs,
    getAllTabs,
    getActiveTabId,
    setActiveTab,
    prepareProject,
    loadSessions,
    createSession,
    renameSession,
    closeSession,
    closeAllSessions,
    send,
    disconnectTab,
    reorderTabs,
    getTerminalCount,
    terminalCounts: cachedCounts,
    loadTerminalCounts,
    getSessionById,
    setSessionRenderMode,
    setSessionSnapshotInterval,
    focusSession,
    retainProjectConnections,
    releaseProjectConnections,
    replayBufferedMessages,
    saveSerializedSnapshot,
    getSerializedSnapshot,
    getLatestServerSnapshot,
    clearSnapshotState,
  };
});
