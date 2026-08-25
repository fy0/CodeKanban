import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { WebSessionSummary } from '@/types/models';
import { useWebSessionStore } from '@/stores/webSession';

const { listMock, archiveMock, deleteMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
  archiveMock: vi.fn(),
  deleteMock: vi.fn(),
}));

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    list: listMock,
    archive: archiveMock,
    delete: deleteMock,
  },
}));

vi.mock('@/utils/ws', () => ({
  resolveWsUrl: (path: string) => path,
}));

function createStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
    removeItem(key: string) {
      store.delete(key);
    },
    clear() {
      store.clear();
    },
  };
}

function makeSession(overrides: Partial<WebSessionSummary> = {}): WebSessionSummary {
  return {
    id: 'session-1',
    projectId: 'project-1',
    worktreeId: null,
    orderIndex: 1000,
    agent: 'codex',
    title: 'Codex Session',
    model: 'gpt-5.4',
    reasoningEffort: 'medium',
    workflowMode: 'default',
    permissionLevel: 'elevated',
    cwd: '/tmp/project',
    nativeSessionId: 'native-1',
    status: 'running',
    assistantState: 'waiting_input',
    hasUnread: false,
    archivedAt: null,
    activityAt: '2026-04-10T10:00:00.000Z',
    lastMessageAt: '2026-04-10T10:00:00.000Z',
    assistantStateUpdatedAt: '2026-04-10T10:00:00.000Z',
    sourceKind: 'codex_app_server',
    syncState: 'fresh',
    lastSyncMode: 'fast',
    sourceCreatedAt: '2026-04-10T09:00:00.000Z',
    sourceUpdatedAt: '2026-04-10T10:00:00.000Z',
    lastSyncedAt: '2026-04-10T10:00:00.000Z',
    threadPath: '/tmp/session.jsonl',
    threadPreview: 'preview',
    turnCount: 1,
    itemCount: 1,
    syncError: null,
    createdAt: '2026-04-10T09:00:00.000Z',
    updatedAt: '2026-04-10T10:00:00.000Z',
    usage: {
      inputTokens: 1,
      cachedInputTokens: 0,
      outputTokens: 1,
      cost: 0,
    },
    contextEstimate: {
      inputTokens: 1,
      cachedInputTokens: 0,
      outputTokens: 1,
      usedTokens: 2,
    },
    contextEstimateMode: 'cumulative_total',
    lastContextCompactionAt: null,
    contextWindowTokens: null,
    contextWindowSource: 'default',
    ...overrides,
  };
}

class FakeWebSocket {
  static OPEN = 1;
  static instances: FakeWebSocket[] = [];

  url: string;
  readyState = 0;
  onopen: ((event: unknown) => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: ((event: unknown) => void) | null = null;
  onclose: (() => void) | null = null;
  sentFrames: Array<Record<string, unknown>> = [];

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
    queueMicrotask(() => {
      this.readyState = FakeWebSocket.OPEN;
      this.onopen?.({});
    });
  }

  send(payload: string) {
    const frame = JSON.parse(payload) as Record<string, unknown>;
    this.sentFrames.push(frame);
    queueMicrotask(() => {
      this.onmessage?.({
        data: JSON.stringify({
          v: 1,
          k: 'ack',
          rid: frame.rid,
          sid: frame.sid,
          rev: '2',
          ts: Date.now(),
          op: frame.op,
          ok: 1,
        }),
      });
    });
  }

  close() {
    this.readyState = 3;
    this.onclose?.();
  }
}

function findSocket(url: string) {
  return FakeWebSocket.instances.find(instance => instance.url === url) ?? null;
}

describe('webSession tab ordering behavior', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const localStorage = createStorageMock();
    vi.stubGlobal('localStorage', localStorage);
    vi.stubGlobal('window', {
      localStorage,
      location: {
        protocol: 'http:',
        host: 'localhost:5173',
      },
      setTimeout,
      clearTimeout,
      setInterval,
      clearInterval,
    });
    vi.stubGlobal('WebSocket', FakeWebSocket);
    FakeWebSocket.instances = [];
    listMock.mockReset();
    archiveMock.mockReset();
    deleteMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('moves sessions using neighboring session ids instead of array indexes', async () => {
    const store = useWebSessionStore();
    const sessionA = makeSession({ id: 'session-a', title: 'A', orderIndex: 1000 });
    const sessionB = makeSession({ id: 'session-b', title: 'B', orderIndex: 2000 });
    const sessionC = makeSession({ id: 'session-c', title: 'C', orderIndex: 3000 });

    listMock.mockResolvedValue([sessionA, sessionB, sessionC]);

    await store.loadSessions(sessionA.projectId);
    await store.moveSession(sessionA.projectId, sessionC.id, '', sessionA.id);

    expect(store.getSessions(sessionA.projectId).map(session => session.id)).toEqual([
      sessionC.id,
      sessionA.id,
      sessionB.id,
    ]);

    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    const moveFrame = commandSocket?.sentFrames.at(-1);
    expect(moveFrame).toMatchObject({
      op: 'move',
      sid: sessionC.id,
      p: {
        prv: '',
        nxt: sessionA.id,
      },
    });
  });

  it('clears the active real session after archiving instead of selecting the first remaining tab', async () => {
    const store = useWebSessionStore();
    const sessionA = makeSession({ id: 'session-a', title: 'A', orderIndex: 1000 });
    const sessionB = makeSession({ id: 'session-b', title: 'B', orderIndex: 2000 });
    const archivedB = makeSession({
      ...sessionB,
      archivedAt: '2026-04-10T11:00:00.000Z',
      status: 'done',
    });

    listMock.mockResolvedValue([sessionA, sessionB]);
    archiveMock.mockResolvedValue(archivedB);

    await store.loadSessions(sessionA.projectId);
    store.setActiveSession(sessionA.projectId, sessionB.id);

    await store.archiveSession(sessionA.projectId, sessionB.id);

    expect(store.getActiveSessionId(sessionA.projectId)).toBe('');
    expect(store.getSessions(sessionA.projectId).map(session => session.id)).toEqual([sessionA.id]);
  });
});
