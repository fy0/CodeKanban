import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { WebSessionSummary } from '@/types/models';
import { useWebSessionStore } from '@/stores/webSession';

const { importSessionMock } = vi.hoisted(() => ({
  importSessionMock: vi.fn(),
}));

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    importSession: importSessionMock,
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
    id: 'session-imported',
    projectId: 'project-1',
    worktreeId: null,
    orderIndex: 1000,
    agent: 'codex',
    title: 'Imported Session',
    model: 'gpt-5.4',
    reasoningEffort: 'medium',
    workflowMode: 'default',
    permissionLevel: 'elevated',
    activeCallTimeoutEnabled: false,
    autoRetryEnabled: false,
    autoRetryScope: 'network_only',
    autoRetryPreset: 'gentle_stop',
    autoRetryDispatchPendingOnFailure: false,
    cwd: '/tmp/project',
    nativeSessionId: 'thread-imported',
    status: 'idle',
    assistantState: null,
    hasUnread: false,
    archivedAt: null,
    activityAt: '2026-04-11T10:00:00.000Z',
    lastMessageAt: '2026-04-11T10:00:00.000Z',
    assistantStateUpdatedAt: null,
    sourceKind: 'codex_app_server',
    syncState: 'fresh',
    lastSyncMode: 'deep',
    sourceCreatedAt: '2026-04-11T09:00:00.000Z',
    sourceUpdatedAt: '2026-04-11T10:00:00.000Z',
    lastSyncedAt: '2026-04-11T10:00:00.000Z',
    threadPath: '/tmp/project/session.jsonl',
    threadPreview: 'Imported Session',
    turnCount: 0,
    itemCount: 1,
    syncError: null,
    createdAt: '2026-04-11T09:00:00.000Z',
    updatedAt: '2026-04-11T10:00:00.000Z',
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

describe('webSession import', () => {
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
    importSessionMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('applies the imported snapshot and activates the imported session', async () => {
    const store = useWebSessionStore();
    const session = makeSession();

    importSessionMock.mockResolvedValue({
      created: true,
      reused: false,
      synced: true,
      session,
      history: {
        items: [
          {
            id: 'history-1',
            oi: 1,
            kd: 'assistant',
            tp: 'agent_message',
            txt: 'imported reply',
            ts2: Date.parse('2026-04-11T10:00:00.000Z'),
          },
        ],
        hasMore: false,
        total: 1,
      },
      pendingInputs: [],
    });

    const result = await store.importSession(session.projectId, 'thread-imported', 'fast');

    expect(result.created).toBe(true);
    expect(importSessionMock).toHaveBeenCalledWith(session.projectId, {
      sessionId: 'thread-imported',
      mode: 'fast',
    });
    expect(store.getActiveSessionId(session.projectId)).toBe(session.id);
    expect(store.getSessions(session.projectId)[0]?.id).toBe(session.id);
    expect(store.getHistoryMeta(session.id).total).toBe(1);
    expect(store.getBlocks(session.id)).toHaveLength(1);
    expect(store.getBlocks(session.id)[0]?.text).toBe('imported reply');
  });
});
