import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { WebSessionSummary } from '@/types/models';
import { useWebSessionStore } from '@/stores/webSession';

const { countsMock, listMock, createMock, archiveMock, unarchiveMock, deleteMock } = vi.hoisted(
  () => ({
    countsMock: vi.fn(),
    listMock: vi.fn(),
    createMock: vi.fn(),
    archiveMock: vi.fn(),
    unarchiveMock: vi.fn(),
    deleteMock: vi.fn(),
  })
);

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    counts: countsMock,
    list: listMock,
    create: createMock,
    archive: archiveMock,
    unarchive: unarchiveMock,
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
    title: 'Session',
    model: 'gpt-5.4',
    reasoningEffort: 'medium',
    workflowMode: 'default',
    permissionLevel: 'elevated',
    cwd: '/tmp/project',
    nativeSessionId: 'native-1',
    status: 'idle',
    assistantState: null,
    hasUnread: false,
    archivedAt: null,
    activityAt: '2026-04-10T10:00:00.000Z',
    lastMessageAt: '2026-04-10T10:00:00.000Z',
    assistantStateUpdatedAt: null,
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
    contextWindowTokens: null,
    contextWindowSource: 'default',
    ...overrides,
  };
}

describe('webSession counts', () => {
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
    countsMock.mockReset();
    listMock.mockReset();
    createMock.mockReset();
    archiveMock.mockReset();
    unarchiveMock.mockReset();
    deleteMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('loads cached counts and keeps them in sync with session mutations', async () => {
    const store = useWebSessionStore();

    countsMock.mockResolvedValue({
      'project-1': 4,
      'project-2': 1,
    });
    await store.loadSessionCounts();
    expect(store.sessionCounts.get('project-1')).toBe(4);
    expect(store.sessionCounts.get('project-2')).toBe(1);

    listMock.mockResolvedValue([
      makeSession({ id: 'session-1' }),
      makeSession({ id: 'session-2', orderIndex: 2000 }),
    ]);
    await store.loadSessions('project-1', true);
    expect(store.sessionCounts.get('project-1')).toBe(2);

    createMock.mockResolvedValue(makeSession({ id: 'session-3', orderIndex: 3000 }));
    await store.createSession('project-1', { agent: 'codex' });
    expect(store.sessionCounts.get('project-1')).toBe(3);

    archiveMock.mockResolvedValue(
      makeSession({
        id: 'session-2',
        orderIndex: 2000,
        archivedAt: '2026-04-10T11:00:00.000Z',
      })
    );
    await store.archiveSession('project-1', 'session-2');
    expect(store.sessionCounts.get('project-1')).toBe(2);

    unarchiveMock.mockResolvedValue(makeSession({ id: 'session-2', orderIndex: 2000 }));
    await store.unarchiveSession('project-1', 'session-2');
    expect(store.sessionCounts.get('project-1')).toBe(3);

    deleteMock.mockResolvedValue(undefined);
    await store.deleteSession('project-1', 'session-1');
    expect(store.sessionCounts.get('project-1')).toBe(2);
  });

  it('keeps the selected session when background creation opts out of activation', async () => {
    const store = useWebSessionStore();
    const activeSession = makeSession({ id: 'session-active' });
    const createdSession = makeSession({ id: 'session-created', orderIndex: 2000 });

    listMock.mockResolvedValue([activeSession]);
    createMock.mockResolvedValue(createdSession);

    await store.loadSessions(activeSession.projectId);
    store.setActiveSession(activeSession.projectId, activeSession.id);

    await store.createSession(
      activeSession.projectId,
      { agent: 'codex' },
      { rememberActive: false }
    );

    expect(store.getActiveSessionId(activeSession.projectId)).toBe(activeSession.id);
    expect(store.getSessions(activeSession.projectId).map(session => session.id)).toContain(
      createdSession.id
    );
  });
});
