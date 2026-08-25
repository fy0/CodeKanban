import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useWebSessionStore } from '@/stores/webSession';
import type { WebSessionSummary } from '@/types/models';

const { listMock, reconcileMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
  reconcileMock: vi.fn(),
}));

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    list: listMock,
    reconcile: reconcileMock,
  },
}));

vi.mock('@/utils/ws', () => ({
  resolveWsUrl: (path: string) => path,
}));

function createStorageMock() {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, String(value)),
    removeItem: (key: string) => values.delete(key),
    clear: () => values.clear(),
  };
}

function makeSession(overrides: Partial<WebSessionSummary> = {}): WebSessionSummary {
  const now = new Date().toISOString();
  return {
    id: 'session-1',
    revision: '1',
    projectId: 'project-1',
    worktreeId: null,
    orderIndex: 1000,
    agent: 'codex',
    title: 'Session',
    model: 'gpt-5.4',
    reasoningEffort: 'medium',
    workflowMode: 'default',
    permissionLevel: 'elevated',
    activeCallTimeoutEnabled: false,
    autoRetryEnabled: false,
    autoRetryPolicyMode: 'default',
    autoRetryScope: 'network_only',
    autoRetryPreset: 'gentle_stop',
    autoRetryMaxAttempts: 0,
    autoRetryDispatchPendingOnFailure: false,
    cwd: '/tmp/project',
    nativeSessionId: 'native-1',
    status: 'idle',
    assistantState: null,
    hasUnread: false,
    archivedAt: null,
    activityAt: now,
    statusUpdatedAt: now,
    lastMessageAt: now,
    assistantStateUpdatedAt: null,
    sourceKind: 'codex_app_server',
    syncState: 'fresh',
    lastSyncMode: 'fast',
    sourceCreatedAt: now,
    sourceUpdatedAt: now,
    lastSyncedAt: now,
    threadPath: '/tmp/session.jsonl',
    threadPreview: 'preview',
    turnCount: 1,
    itemCount: 1,
    syncError: null,
    createdAt: now,
    updatedAt: now,
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
    contextWindowTokens: null,
    contextWindowSource: 'default',
    ...overrides,
  };
}

describe('web session resume reconciliation', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const localStorage = createStorageMock();
    vi.stubGlobal('localStorage', localStorage);
    vi.stubGlobal('window', {
      localStorage,
      location: { protocol: 'http:', host: 'localhost:5173' },
      setTimeout,
      clearTimeout,
      setInterval,
      clearInterval,
    });
    listMock.mockReset();
    reconcileMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reconciles all locally active sessions plus only the 48 most recent idle sessions', async () => {
    const now = Date.now();
    const oldTimestamp = new Date(now - 24 * 60 * 60 * 1000).toISOString();
    const activeChanged = makeSession({
      id: 'active-changed',
      projectId: 'project-1',
      status: 'running',
      assistantState: 'working',
      activityAt: oldTimestamp,
      statusUpdatedAt: oldTimestamp,
      updatedAt: oldTimestamp,
    });
    const activeUnchanged = makeSession({
      id: 'active-unchanged',
      projectId: 'project-2',
      status: 'waiting_approval',
      activityAt: oldTimestamp,
      statusUpdatedAt: oldTimestamp,
      updatedAt: oldTimestamp,
    });
    const activeDeleted = makeSession({
      id: 'active-deleted',
      projectId: 'project-2',
      status: 'aborting',
      activityAt: oldTimestamp,
      statusUpdatedAt: oldTimestamp,
      updatedAt: oldTimestamp,
    });
    const oldIdle = makeSession({
      id: 'old-idle',
      projectId: 'project-1',
      activityAt: oldTimestamp,
      statusUpdatedAt: oldTimestamp,
      updatedAt: oldTimestamp,
    });
    const recentIdle = Array.from({ length: 60 }, (_, index) => {
      const timestamp = new Date(now - index * 60_000).toISOString();
      return makeSession({
        id: `recent-${index}`,
        projectId: index % 2 === 0 ? 'project-1' : 'project-2',
        activityAt: timestamp,
        statusUpdatedAt: timestamp,
        updatedAt: timestamp,
      });
    });
    const projectOne = [
      activeChanged,
      oldIdle,
      ...recentIdle.filter(session => session.projectId === 'project-1'),
    ];
    const projectTwo = [
      activeUnchanged,
      activeDeleted,
      ...recentIdle.filter(session => session.projectId === 'project-2'),
    ];
    listMock.mockImplementation((projectId: string) =>
      Promise.resolve(projectId === 'project-1' ? projectOne : projectTwo)
    );
    reconcileMock.mockResolvedValue({
      items: [
        makeSession({
          ...activeChanged,
          revision: '2',
          status: 'done',
          assistantState: null,
          hasUnread: true,
          updatedAt: new Date(now).toISOString(),
          statusUpdatedAt: new Date(now).toISOString(),
        }),
      ],
      missingIds: [activeDeleted.id],
    });

    const store = useWebSessionStore();
    await store.loadSessions('project-1');
    await store.loadSessions('project-2');
    await store.reconcileRecentSessions();

    const targets = reconcileMock.mock.calls[0]?.[0] as Array<{ id: string; revision?: string }>;
    expect(targets).toHaveLength(51);
    expect(targets).toEqual(
      expect.arrayContaining([
        { id: activeChanged.id, revision: '1' },
        { id: activeUnchanged.id, revision: '1' },
        { id: activeDeleted.id, revision: '1' },
      ])
    );
    expect(targets.some(target => target.id === oldIdle.id)).toBe(false);
    expect(targets.filter(target => target.id.startsWith('recent-'))).toHaveLength(48);
    expect(
      store.getSessions('project-1').find(session => session.id === activeChanged.id)
    ).toMatchObject({
      revision: '2',
      status: 'done',
      assistantState: null,
      hasUnread: true,
    });
    expect(store.getSessions('project-2').some(session => session.id === activeDeleted.id)).toBe(
      false
    );
    expect(
      store.getSessions('project-2').find(session => session.id === activeUnchanged.id)?.status
    ).toBe('waiting_approval');
    expect(listMock).toHaveBeenCalledTimes(2);
  });

  it('allows an immediate retry when resume reconciliation fails', async () => {
    const running = makeSession({
      id: 'retry-running',
      status: 'running',
      assistantState: 'working',
    });
    const completed = makeSession({
      ...running,
      revision: '2',
      status: 'done',
      assistantState: null,
    });
    listMock.mockResolvedValue([running]);
    reconcileMock
      .mockRejectedValueOnce(new Error('temporary resume failure'))
      .mockResolvedValueOnce({ items: [completed], missingIds: [] });

    const store = useWebSessionStore();
    await store.loadSessions(running.projectId);
    await expect(store.reconcileRecentSessions()).rejects.toThrow('temporary resume failure');
    await store.reconcileRecentSessions();

    expect(reconcileMock).toHaveBeenCalledTimes(2);
    expect(
      store.getSessions(running.projectId).find(session => session.id === running.id)
    ).toMatchObject({
      revision: '2',
      status: 'done',
      assistantState: null,
    });
  });
});
