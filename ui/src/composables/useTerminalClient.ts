import { computed, onScopeDispose, reactive, ref, watch, type Ref } from 'vue';
import EventEmitter from 'eventemitter3';
import Apis, { urlBase } from '@/api';
import type { TerminalSession } from '@/types/models';
import { resolveWsUrl } from '@/utils/ws';

type ClientStatus = 'connecting' | 'ready' | 'closed' | 'error';

export interface TerminalTabState extends TerminalSession {
  clientStatus: ClientStatus;
}

export type ServerMessage = {
  type: 'ready' | 'data' | 'exit' | 'error';
  data?: string;
  cols?: number;
  rows?: number;
};

export type TerminalCreateOptions = {
  worktreeId: string;
  workingDir?: string;
  title?: string;
  rows?: number;
  cols?: number;
};

type SessionRecord = {
  projectId: string;
  tab: TerminalTabState;
};

export function useTerminalClient(projectIdRef: Ref<string>) {
  const tabStore = reactive(new Map<string, TerminalTabState[]>());
  const sessionIndex = new Map<string, SessionRecord>();
  const activeTabByProject = reactive(new Map<string, string>());
  const emitter = new EventEmitter();
  const sockets = new Map<string, WebSocket>();
  const manualCloseIds = new Set<string>();
  let globalLoadToken = 0;
  const projectLoadTokens = new Map<string, number>();

  const tabs = computed(() => tabStore.get(projectIdRef.value) ?? []);

  const activeTabId = computed<string>({
    get: () => {
      const projectId = projectIdRef.value;
      if (!projectId) {
        return '';
      }
      const current = activeTabByProject.get(projectId);
      const bucket = tabStore.get(projectId);
      if (current && bucket?.some(tab => tab.id === current)) {
        return current;
      }
      const fallback = bucket?.[0]?.id ?? '';
      if (fallback) {
        activeTabByProject.set(projectId, fallback);
      }
      return fallback;
    },
    set: value => {
      const projectId = projectIdRef.value;
      if (!projectId) {
        return;
      }
      if (value) {
        activeTabByProject.set(projectId, value);
      } else {
        activeTabByProject.delete(projectId);
      }
    },
  });

  const hasSessions = computed(() => tabs.value.length > 0);

  watch(
    () => projectIdRef.value,
    id => {
      if (!id) {
        return;
      }
      ensureBucket(id);
      ensureActiveTab(id);
      void loadSessions(id);
    },
    { immediate: true }
  );

  onScopeDispose(() => {
    sockets.forEach((socket, sessionId) => {
      manualCloseIds.add(sessionId);
      socket.close();
    });
    sockets.clear();
    tabStore.clear();
    sessionIndex.clear();
    activeTabByProject.clear();
  });

  async function loadSessions(targetProjectId?: string) {
    const projectId = ensureProjectSelected(targetProjectId);
    const token = ++globalLoadToken;
    projectLoadTokens.set(projectId, token);
    try {
      const response = await Apis.terminalSession
        .list({
          pathParams: { projectId },
          cacheFor: 0,
        })
        .send();
      if (projectLoadTokens.get(projectId) !== token) {
        return;
      }
      const items = response?.items ?? [];
      reconcileSessions(projectId, items as unknown as TerminalSession[]);
    } catch (error) {
      console.error('Failed to load terminal sessions', error);
    }
  }

  function ensureProjectSelected(id?: string) {
    const resolved = id ?? projectIdRef.value;
    if (!resolved) {
      throw new Error('请先选择项目');
    }
    return resolved;
  }

  async function createSession(payload: TerminalCreateOptions) {
    const projectId = ensureProjectSelected();
    const rows = payload.rows ?? 24;
    const cols = payload.cols ?? 80;
    const method = Apis.terminalSession.create({
      pathParams: {
        projectId,
        worktreeId: payload.worktreeId,
      },
      data: {
        workingDir: payload.workingDir ?? '',
        title: payload.title ?? 'Terminal',
        rows,
        cols,
      },
      cacheFor: 0,
    });
    const response = await method.send();
    if (projectId !== projectIdRef.value) {
      return;
    }
    if (!response?.item) {
      throw new Error('创建终端失败');
    }
    attachOrUpdateSession(response.item as unknown as TerminalSession, {
      activate: true,
      projectIdOverride: projectId,
    });
  }

  async function closeSession(sessionId: string) {
    const projectId = ensureProjectSelected();
    await Apis.terminalSession
      .close({
        pathParams: { projectId, sessionId },
        cacheFor: 0,
      })
      .send();
    disconnectTab(sessionId, true);
  }

  function attachOrUpdateSession(
    session: TerminalSession,
    options?: { activate?: boolean; projectIdOverride?: string }
  ) {
    const projectId = options?.projectIdOverride ?? session.projectId ?? projectIdRef.value;
    if (!projectId) {
      return;
    }
    const bucket = ensureBucket(projectId);
    const existing = sessionIndex.get(session.id);
    if (existing) {
      Object.assign(existing.tab, session);
      if (existing.projectId !== projectId) {
        moveTab(existing, projectId);
      }
      if (options?.activate) {
        setActiveTab(projectId, session.id);
      }
      return existing.tab;
    }
    const tab: TerminalTabState = {
      ...session,
      projectId,
      clientStatus: 'connecting',
    };
    console.log('[Terminal] Creating new tab, sessionId:', tab.id, 'status:', tab.clientStatus);
    bucket.push(tab);
    sessionIndex.set(tab.id, { projectId, tab });
    if (options?.activate || projectId === projectIdRef.value) {
      setActiveTab(projectId, tab.id);
    } else if (!activeTabByProject.get(projectId)) {
      setActiveTab(projectId, tab.id);
    }
    connect(tab);
    return tab;
  }

  function moveTab(record: SessionRecord, nextProjectId: string) {
    const currentBucket = tabStore.get(record.projectId);
    if (currentBucket) {
      const idx = currentBucket.findIndex(tab => tab.id === record.tab.id);
      if (idx !== -1) {
        currentBucket.splice(idx, 1);
      }
      if (currentBucket.length === 0) {
        tabStore.delete(record.projectId);
      }
    }
    const nextBucket = ensureBucket(nextProjectId);
    nextBucket.push(record.tab);
    record.projectId = nextProjectId;
  }

  function updateTabStatus(sessionId: string, status: ClientStatus) {
    const record = sessionIndex.get(sessionId);
    if (!record) return;

    const bucket = tabStore.get(record.projectId);
    if (!bucket) return;

    const index = bucket.findIndex(t => t.id === sessionId);
    if (index === -1) return;

    // 使用数组替换来触发响应式更新
    bucket[index] = { ...bucket[index], clientStatus: status };
    // 同时更新 record 中的引用
    record.tab = bucket[index];
    console.log('[Terminal] Status updated:', { sessionId, status });
  }

  function connect(tab: TerminalTabState) {
    const wsURL = resolveWsUrl(tab.wsUrl || tab.wsPath, urlBase);
    console.log('[Terminal WS] Connecting to:', wsURL, 'sessionId:', tab.id);
    const socket = new WebSocket(wsURL);
    sockets.set(tab.id, socket);

    socket.addEventListener('open', () => {
      console.log('[Terminal WS] Connected, sessionId:', tab.id);
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
        console.log('[Terminal WS] Message received:', payload.type, 'sessionId:', tab.id);
        if (payload.type === 'ready') {
          updateTabStatus(tab.id, 'ready');
        } else if (payload.type === 'exit') {
          updateTabStatus(tab.id, 'closed');
        } else if (payload.type === 'error') {
          updateTabStatus(tab.id, 'error');
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
    for (const session of sessions) {
      attachOrUpdateSession(session, { projectIdOverride: projectId });
    }
    ensureActiveTab(projectId);
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
        }
        if (bucket.length === 0) {
          tabStore.delete(record.projectId);
        }
      }
      sessionIndex.delete(sessionId);
      if (activeTabByProject.get(record.projectId) === sessionId) {
        const next = tabStore.get(record.projectId)?.[0];
        if (next) {
          activeTabByProject.set(record.projectId, next.id);
        } else {
          activeTabByProject.delete(record.projectId);
        }
      }
    }
  }

  function ensureActiveTab(projectId: string) {
    if (!projectId) {
      return;
    }
    const bucket = tabStore.get(projectId);
    if (!bucket || bucket.length === 0) {
      activeTabByProject.delete(projectId);
      return;
    }
    const current = activeTabByProject.get(projectId);
    if (current && bucket.some(tab => tab.id === current)) {
      return;
    }
    activeTabByProject.set(projectId, bucket[0].id);
  }

  function setActiveTab(projectId: string | undefined, tabId: string) {
    if (!projectId) {
      return;
    }
    activeTabByProject.set(projectId, tabId);
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

  return {
    tabs,
    activeTabId,
    hasSessions,
    emitter,
    createSession,
    closeSession,
    send,
    disconnectTab,
  };
}
