import { defineStore } from 'pinia';
import { reactive } from 'vue';
import EventEmitter from 'eventemitter3';
import Apis, { urlBase } from '@/api';
import type { TerminalSession } from '@/types/models';
import { resolveWsUrl } from '@/utils/ws';
import { clearTerminalSnapshot } from '@/utils/terminalSnapshotCache';

export type ClientStatus = 'connecting' | 'ready' | 'closed' | 'error';

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

export const useTerminalStore = defineStore('terminal', () => {
  const tabStore = reactive(new Map<string, TerminalTabState[]>());
  const sessionIndex = new Map<string, SessionRecord>();
  const activeTabByProject = reactive(new Map<string, string>());
  const sockets = new Map<string, WebSocket>();
  const manualCloseIds = new Set<string>();
  let globalLoadToken = 0;
  const projectLoadTokens = new Map<string, number>();
  const emitter = new EventEmitter();

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
      activeTabByProject.delete(projectId);
      return '';
    }
    const current = activeTabByProject.get(projectId);
    if (current && bucket.some(tab => tab.id === current)) {
      return current;
    }
    const fallback = bucket[0]?.id ?? '';
    if (fallback) {
      activeTabByProject.set(projectId, fallback);
    }
    return fallback;
  }

  function setActiveTab(projectId: string | undefined, tabId: string) {
    if (!projectId) {
      return;
    }
    if (tabId) {
      activeTabByProject.set(projectId, tabId);
    } else {
      activeTabByProject.delete(projectId);
    }
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
    } catch (error) {
      console.error('Failed to load terminal sessions', error);
    }
  }

  async function createSession(projectId: string | undefined, options: TerminalCreateOptions) {
    const resolved = ensureProjectSelected(projectId);
    if (!options.worktreeId) {
      throw new Error('����ѡ�� Worktree');
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
          worktreeId: options.worktreeId,
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
    if (options?.activate) {
      setActiveTab(resolvedProjectId, tab.id);
    } else if (!activeTabByProject.get(resolvedProjectId)) {
      setActiveTab(resolvedProjectId, tab.id);
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
    send,
    disconnectTab,
  };
});
