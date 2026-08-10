import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useWebSessionStore } from '@/stores/webSession';
import type { WebSessionSummary } from '@/types/models';

const { listMock, snapshotMock, calculateWorkTimingMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
  snapshotMock: vi.fn(),
  calculateWorkTimingMock: vi.fn(),
}));

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    list: listMock,
    snapshot: snapshotMock,
    calculateWorkTiming: calculateWorkTimingMock,
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
  return {
    id: 'session-1',
    revision: '1',
    projectId: 'project-1',
    worktreeId: null,
    orderIndex: 1000,
    agent: 'codex',
    title: 'Timed session',
    model: 'gpt-5.4',
    reasoningEffort: 'medium',
    workflowMode: 'default',
    permissionLevel: 'elevated',
    autoRetryEnabled: false,
    autoRetryScope: 'network_only',
    autoRetryPreset: 'gentle_stop',
    autoRetryDispatchPendingOnFailure: false,
    cwd: '/tmp/project',
    nativeSessionId: 'native-1',
    status: 'idle',
    assistantState: null,
    hasUnread: false,
    archivedAt: null,
    activityAt: '2026-08-10T10:00:00.000Z',
    sourceKind: 'codex_app_server',
    syncState: 'fresh',
    turnCount: 1,
    itemCount: 1,
    createdAt: '2026-08-10T09:00:00.000Z',
    updatedAt: '2026-08-10T10:00:00.000Z',
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
    contextWindowSource: 'default',
    workTiming: {
      completedDurationMs: 0,
      currentRun: null,
      backfillState: 'pending',
      backfillVersion: 0,
    },
    ...overrides,
  };
}

describe('webSessionStore work timing', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', createStorageMock());
    listMock.mockReset();
    snapshotMock.mockReset();
    calculateWorkTimingMock.mockReset();
  });

  it('coalesces lazy calculations and patches the loaded anchor item', async () => {
    const pendingSession = makeSession();
    listMock.mockResolvedValue([pendingSession]);
    snapshotMock.mockResolvedValue({
      revision: '1',
      session: pendingSession,
      history: {
        items: [
          {
            id: 'assistant-1',
            orderIndex: 1,
            kind: 'assistant',
            itemType: 'message',
            text: 'done',
            timestamp: '2026-08-10T10:00:00.000Z',
            done: true,
            attachments: [],
          },
        ],
        hasMore: false,
        total: 1,
      },
    });

    let resolveCalculation!: (value: unknown) => void;
    calculateWorkTimingMock.mockReturnValue(
      new Promise(resolve => {
        resolveCalculation = resolve;
      })
    );

    const store = useWebSessionStore();
    await store.loadSessions('project-1');
    await store.loadSessionSnapshot('project-1', 'session-1');

    const first = store.calculateSessionWorkTiming('project-1', 'session-1');
    const second = store.calculateSessionWorkTiming('project-1', 'session-1');
    expect(calculateWorkTimingMock).toHaveBeenCalledOnce();

    resolveCalculation({
      status: 'calculated',
      session: makeSession({
        revision: '2',
        workTiming: {
          completedDurationMs: 18_000,
          currentRun: null,
          backfillState: 'complete',
          backfillVersion: 1,
        },
      }),
      items: [
        {
          itemId: 'assistant-1',
          runId: 'run-1',
          runDurationMs: 18_000,
          runOutcome: 'completed',
        },
      ],
    });
    const [firstResult, secondResult] = await Promise.all([first, second]);
    expect(firstResult).toBe(secondResult);

    expect(store.getBlocks('session-1')[0]).toMatchObject({
      id: 'assistant-1',
      runId: 'run-1',
      runDurationMs: 18_000,
      runOutcome: 'completed',
    });
    expect(store.getSessions('project-1')[0]?.workTiming).toMatchObject({
      completedDurationMs: 18_000,
      backfillState: 'complete',
    });
  });
});
