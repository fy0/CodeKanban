import { defineStore } from 'pinia';
import { reactive } from 'vue';
import EventEmitter from 'eventemitter3';
import Apis, { urlBase } from '@/api';
import type { TerminalSession } from '@/types/models';
import { resolveWsUrl } from '@/utils/ws';
import { clearTerminalSnapshot } from '@/utils/terminalSnapshotCache';
import { useProjectStore } from '@/stores/project';

export type ClientStatus = 'connecting' | 'ready' | 'closed' | 'error';

export interface TerminalTabState extends TerminalSession {
  clientStatus: ClientStatus;
}

export type ServerMessage = {
  type: 'ready' | 'data' | 'exit' | 'error' | 'metadata';
  data?: string;
  cols?: number;
  rows?: number;
  metadata?: {
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
      state?: string;
      stateUpdatedAt?: string;
      interrupted?: boolean;
    };
  };
};

export type TerminalCreateOptions = {
  worktreeId?: string;
  workingDir?: string;
  title?: string;
  rows?: number;
  cols?: number;
};

type SessionRecord = {
  projectId: string;
  tab: TerminalTabState;
};

const TAB_ORDER_STORAGE_KEY = 'kanban-terminal-tab-order';
const LAST_ACTIVE_TAB_STORAGE_KEY = 'kanban-terminal-last-active';

const storedTabOrders = loadStoredTabOrders();
const storedActiveTabs = loadStoredActiveTabs();

function loadStoredTabOrders() {
  if (typeof window === 'undefined' || !window.localStorage) {
    return new Map<string, string[]>();
  }
  try {
    const raw = window.localStorage.getItem(TAB_ORDER_STORAGE_KEY);
    if (!raw) {
      return new Map<string, string[]>();
    }
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const result = new Map<string, string[]>();
    Object.entries(parsed).forEach(([projectId, value]) => {
      if (!projectId || !Array.isArray(value)) {
        return;
      }
      const ids = value
        .map(id => (typeof id === 'string' ? id.trim() : ''))
        .filter((id): id is string => Boolean(id));
      if (ids.length) {
        result.set(projectId, ids);
      }
    });
    return result;
  } catch (error) {
    console.warn('[Terminal Store] Failed to parse stored tab order', error);
    return new Map<string, string[]>();
  }
}

function persistStoredTabOrders() {
  if (typeof window === 'undefined' || !window.localStorage) {
    return;
  }
  if (!storedTabOrders.size) {
    window.localStorage.removeItem(TAB_ORDER_STORAGE_KEY);
    return;
  }
  const payload: Record<string, string[]> = {};
  storedTabOrders.forEach((order, projectId) => {
    if (order.length) {
      payload[projectId] = order;
    }
  });
  if (Object.keys(payload).length === 0) {
    window.localStorage.removeItem(TAB_ORDER_STORAGE_KEY);
    return;
  }
  window.localStorage.setItem(TAB_ORDER_STORAGE_KEY, JSON.stringify(payload));
}

function captureProjectOrder(projectId: string, bucket?: TerminalTabState[]) {
  if (!projectId) {
    return;
  }
  const nextOrder = bucket?.map(tab => tab.id).filter(Boolean) ?? [];
  if (!nextOrder.length) {
    if (storedTabOrders.delete(projectId)) {
      persistStoredTabOrders();
    }
    return;
  }
  const currentOrder = storedTabOrders.get(projectId);
  if (ordersEqual(currentOrder, nextOrder)) {
    return;
  }
  storedTabOrders.set(projectId, nextOrder);
  persistStoredTabOrders();
}

function ordersEqual(current: string[] | undefined, next: string[]) {
  if (!current || current.length !== next.length) {
    return false;
  }
  for (let index = 0; index < current.length; index += 1) {
    if (current[index] !== next[index]) {
      return false;
    }
  }
  return true;
}

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

function sortSessionsWithStoredOrder(projectId: string, sessions: TerminalSession[]) {
  if (!sessions.length) {
    return sessions;
  }
  const storedOrder = storedTabOrders.get(projectId);
  const ordered = [...sessions];
  if (!storedOrder || storedOrder.length === 0) {
    ordered.sort((a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id));
    return ordered;
  }
  const orderIndex = new Map<string, number>();
  storedOrder.forEach((id, index) => {
    if (id) {
      orderIndex.set(id, index);
    }
  });
  ordered.sort((a, b) => {
    const indexA = orderIndex.get(a.id);
    const indexB = orderIndex.get(b.id);
    if (indexA != null && indexB != null) {
      if (indexA !== indexB) {
        return indexA - indexB;
      }
      return a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id);
    }
    if (indexA != null) {
      return -1;
    }
    if (indexB != null) {
      return 1;
    }
    return a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id);
  });
  return ordered;
}

export const useTerminalStore = defineStore('terminal', () => {
  const tabStore = reactive(new Map<string, TerminalTabState[]>());
  const sessionIndex = new Map<string, SessionRecord>();
  const activeTabByProject = reactive(new Map<string, string>());
  const sockets = new Map<string, WebSocket>();
  const manualCloseIds = new Set<string>();
  let globalLoadToken = 0;
  const projectLoadTokens = new Map<string, number>();
  const emitter = new EventEmitter();
  const cachedCounts = reactive(new Map<string, number>());
  // Track AI assistant state for each session to detect state changes
  const aiPreviousStates = new Map<string, string>();
  // Get project store for looking up project names
  const projectStore = useProjectStore();

  // Helper function to get project name by ID
  function getProjectName(projectId: string): string | undefined {
    const project = projectStore.projects.find((p) => p.id === projectId);
    return project?.name;
  }

  function getTabs(projectId?: string) {
    if (!projectId) {
      return [];
    }
    return tabStore.get(projectId) ?? [];
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
      return;
    }
    if (activeTabByProject.has(projectId)) {
      activeTabByProject.delete(projectId);
    }
    forgetStoredActiveTab(projectId);
  }

  function prepareProject(projectId: string) {
    ensureBucket(projectId);
    ensureActiveTab(projectId);
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
      const items = response?.items ?? [];
      reconcileSessions(resolved, items as unknown as TerminalSession[]);
      // 更新终端计数缓存
      cachedCounts.set(resolved, items.length);
    } catch (error) {
      console.error('Failed to load terminal sessions', error);
    }
  }

  async function createSession(projectId: string | undefined, options: TerminalCreateOptions) {
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

    const payload = {
      workingDir: options.workingDir ?? '',
      title: options.title ?? '',
      rows: options.rows ?? 0,
      cols: options.cols ?? 0,
    };
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
    attachOrUpdateSession(response.item as unknown as TerminalSession, {
      activate: true,
      projectIdOverride: resolved,
    });
    // 更新终端计数缓存
    const currentCount = cachedCounts.get(resolved) ?? 0;
    cachedCounts.set(resolved, currentCount + 1);
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

  function send(sessionId: string, message: any) {
    const socket = sockets.get(sessionId);
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(message));
    }
  }

  function disconnectTab(sessionId: string, remove = true) {
    const socket = sockets.get(sessionId);
    if (socket) {
      manualCloseIds.add(sessionId);
      socket.close();
      sockets.delete(sessionId);
    }
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
        captureProjectOrder(record.projectId, bucket);
        if (bucket.length === 0) {
          tabStore.delete(record.projectId);
        }
      }
      sessionIndex.delete(sessionId);
      aiPreviousStates.delete(sessionId); // Clean up AI state tracking
      if (activeTabByProject.get(record.projectId) === sessionId) {
        const nextId = tabStore.get(record.projectId)?.[0]?.id;
        setActiveTab(record.projectId, nextId);
      }
      clearTerminalSnapshot(sessionId);
    }
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
    const [tab] = bucket.splice(fromIndex, 1);
    if (!tab) {
      return;
    }
    bucket.splice(clampedToIndex, 0, tab);
    captureProjectOrder(projectId, bucket);
  }

  function attachOrUpdateSession(
    session: TerminalSession,
    options?: { activate?: boolean; projectIdOverride?: string },
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
          immutableProjectId,
        );
      }
      Object.assign(existing.tab, {
        ...session,
        projectId: immutableProjectId,
      });
      if (options?.activate) {
        setActiveTab(immutableProjectId, session.id);
      }
      return existing.tab;
    }

    const resolvedProjectId = resolveSessionProjectId(session, options?.projectIdOverride);
    if (!resolvedProjectId) {
      console.warn('[Terminal Store] Skip terminal session with unknown projectId', session.id);
      return;
    }

    const bucket = ensureBucket(resolvedProjectId);
    const tab: TerminalTabState = {
      ...session,
      projectId: resolvedProjectId,
      clientStatus: 'connecting',
    };
    bucket.push(tab);
    sessionIndex.set(tab.id, { projectId: resolvedProjectId, tab });
    captureProjectOrder(resolvedProjectId, bucket);
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
    connect(tab);
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

    bucket[index] = {
      ...bucket[index],
      processPid: metadata.processPid,
      processStatus: metadata.processStatus as 'idle' | 'busy' | 'unknown' | undefined,
      processHasChildren: metadata.processHasChildren,
      runningCommand: metadata.runningCommand,
      aiAssistant: metadata.aiAssistant,
    };
    record.tab = bucket[index];
  }

  function connect(tab: TerminalTabState) {
    const wsURL = resolveWsUrl(tab.wsUrl || tab.wsPath, urlBase);
    const socket = new WebSocket(wsURL);
    sockets.set(tab.id, socket);

    socket.addEventListener('open', () => {
      updateTabStatus(tab.id, 'ready');
      socket.send(
        JSON.stringify({
          type: 'resize',
          cols: tab.cols,
          rows: tab.rows,
        }),
      );
    });

    socket.addEventListener('message', event => {
      try {
        const payload = JSON.parse(event.data as string) as ServerMessage;
        if (payload.type === 'ready') {
          updateTabStatus(tab.id, 'ready');
        } else if (payload.type === 'exit') {
          updateTabStatus(tab.id, 'closed');
        } else if (payload.type === 'error') {
          updateTabStatus(tab.id, 'error');
        } else if (payload.type === 'metadata' && payload.metadata) {
          // Update tab metadata in realtime
          updateTabMetadata(tab.id, payload.metadata);

          // 🎯 Detect AI assistant completion
          // Only trigger notification when transitioning from working state to waiting_input
          const currentState = payload.metadata.aiAssistant?.state;
          const previousState = aiPreviousStates.get(tab.id);

          // 🔍 Detect AI assistant closure (detected: false)
          // When agent is closed, clear any existing notifications
          const isAgentDetected = payload.metadata.aiAssistant?.detected;
          if (isAgentDetected === false) {
            console.log(
              `[Terminal] AI Agent Closed: Clearing notifications for session ${tab.id}`,
              {
                sessionId: tab.id,
                sessionTitle: tab.title,
                assistant: payload.metadata.aiAssistant,
              }
            );
            emitter.emit('ai:closed', {
              sessionId: tab.id,
            });
          }

          // Detect approval requests
          if (currentState === 'waiting_approval') {
            console.log(
              `[Terminal] AI Approval Needed: ${payload.metadata.aiAssistant?.displayName || 'AI'} is waiting for approval`,
              {
                sessionId: tab.id,
                sessionTitle: tab.title,
                previousState,
                currentState,
                assistant: payload.metadata.aiAssistant,
              }
            );
            emitter.emit('ai:approval-needed', {
              sessionId: tab.id,
              sessionTitle: tab.title,
              projectId: tab.projectId,
              projectName: getProjectName(tab.projectId),
              assistant: payload.metadata.aiAssistant,
            });
          }

          // Detect AI starting to work again (after being idle/completed)
          if (currentState === 'working' && previousState && previousState !== 'working') {
            console.log(
              `[Terminal] AI Started Working: ${payload.metadata.aiAssistant?.displayName || 'AI'} resumed work`,
              {
                sessionId: tab.id,
                sessionTitle: tab.title,
                previousState,
                currentState,
                assistant: payload.metadata.aiAssistant,
              }
            );
            emitter.emit('ai:working', {
              sessionId: tab.id,
              sessionTitle: tab.title,
              projectId: tab.projectId,
              projectName: getProjectName(tab.projectId),
              assistant: payload.metadata.aiAssistant,
            });
          }

          if (currentState === 'waiting_input' && previousState) {
            // Check if transitioning from working state
            const isFromWorkingState = previousState === 'working';
            const wasInterrupted = payload.metadata.aiAssistant?.interrupted === true;

            if (isFromWorkingState && !wasInterrupted) {
              // Valid completion: working state → waiting input (NOT interrupted)
              console.log(
                `[Terminal] AI Completion Detected: ${payload.metadata.aiAssistant?.displayName || 'AI'} completed execution`,
                {
                  sessionId: tab.id,
                  sessionTitle: tab.title,
                  previousState,
                  currentState,
                  assistant: payload.metadata.aiAssistant,
                }
              );
              emitter.emit('ai:completed', {
                sessionId: tab.id,
                sessionTitle: tab.title,
                projectId: tab.projectId,
                projectName: getProjectName(tab.projectId),
                assistant: payload.metadata.aiAssistant,
              });
            } else if (isFromWorkingState && wasInterrupted) {
              // User interrupted the execution
              console.log(
                `[Terminal] AI Interrupted: ${payload.metadata.aiAssistant?.displayName || 'AI'} was interrupted by user`,
                {
                  sessionId: tab.id,
                  sessionTitle: tab.title,
                  previousState,
                  currentState,
                  assistant: payload.metadata.aiAssistant,
                }
              );
              // Don't emit ai:completed for interrupted executions
            }
          }

          // Update previous state for next comparison
          if (currentState) {
            aiPreviousStates.set(tab.id, currentState);
          }
        }
        emitter.emit(tab.id, payload);
      } catch {
        // ignore malformed payloads
      }
    });

    socket.addEventListener('close', () => {
      sockets.delete(tab.id);
      if (manualCloseIds.has(tab.id)) {
        manualCloseIds.delete(tab.id);
        updateTabStatus(tab.id, 'closed');
        return;
      }
      if (sessionIndex.has(tab.id)) {
        updateTabStatus(tab.id, 'connecting');
        setTimeout(() => {
          if (sessionIndex.has(tab.id)) {
            connect(tab);
          }
        }, 1000);
      } else {
        updateTabStatus(tab.id, 'closed');
      }
    });

    socket.addEventListener('error', () => {
      updateTabStatus(tab.id, 'error');
    });
  }

  function reconcileSessions(projectId: string, sessions: TerminalSession[]) {
    const bucket = ensureBucket(projectId);
    const incomingIds = new Set(sessions.map(session => session.id));
    for (const tab of [...bucket]) {
      if (!incomingIds.has(tab.id)) {
        disconnectTab(tab.id, true);
      }
    }
    const orderedSessions = sortSessionsWithStoredOrder(projectId, sessions);
    for (const session of orderedSessions) {
      attachOrUpdateSession(session, { projectIdOverride: projectId });
    }
    const finalBucket = tabStore.get(projectId);
    if (!finalBucket || finalBucket.length === 0) {
      setActiveTab(projectId, undefined);
    } else {
      ensureActiveTab(projectId);
    }
    captureProjectOrder(projectId, finalBucket);
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
        requestedProjectId,
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

  return {
    emitter,
    getTabs,
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
  };
});
