import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useWebSessionStore } from '@/stores/webSession';
import type { WebSessionSummary } from '@/types/models';

const { editUserMessageMock } = vi.hoisted(() => ({
  editUserMessageMock: vi.fn(),
}));

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    editUserMessage: editUserMessageMock,
  },
}));

vi.mock('@/utils/ws', () => ({
  resolveWsUrl: (path: string) => path,
}));

function createStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, String(value)),
    removeItem: (key: string) => store.delete(key),
    clear: () => store.clear(),
  };
}

function makeBranchSession(): WebSessionSummary {
  return {
    id: 'session-branch',
    projectId: 'project-1',
    worktreeId: null,
    orderIndex: 2000,
    agent: 'codex',
    title: 'Revised prompt',
    model: 'gpt-5.4',
    reasoningEffort: 'high',
    workflowMode: 'default',
    permissionLevel: 'elevated',
    activeCallTimeoutEnabled: true,
    autoRetryEnabled: false,
    autoRetryScope: 'network_only',
    autoRetryPreset: 'gentle_stop',
    autoRetryDispatchPendingOnFailure: false,
    cwd: '/tmp/project',
    nativeSessionId: 'thread-branch',
    status: 'running',
    assistantState: 'working',
    hasUnread: false,
    archivedAt: null,
    activityAt: '2026-07-23T09:00:00.000Z',
    lastMessageAt: '2026-07-23T09:00:00.000Z',
    assistantStateUpdatedAt: '2026-07-23T09:00:00.000Z',
    sourceKind: 'codex_app_server',
    syncState: 'fresh',
    lastSyncMode: 'fast',
    sourceCreatedAt: '2026-07-23T08:00:00.000Z',
    sourceUpdatedAt: '2026-07-23T09:00:00.000Z',
    lastSyncedAt: '2026-07-23T09:00:00.000Z',
    threadPath: '/tmp/thread-branch.jsonl',
    threadPreview: 'Revised prompt',
    turnCount: 1,
    itemCount: 2,
    syncError: null,
    createdAt: '2026-07-23T09:00:00.000Z',
    updatedAt: '2026-07-23T09:00:00.000Z',
    usage: { inputTokens: 0, cachedInputTokens: 0, outputTokens: 0, cost: 0 },
    contextEstimate: {
      inputTokens: 0,
      cachedInputTokens: 0,
      outputTokens: 0,
      usedTokens: 0,
    },
    contextEstimateMode: 'cumulative_total',
    lastContextCompactionAt: null,
    contextWindowTokens: null,
    contextWindowSource: 'default',
  };
}

describe('webSession edited message branch', () => {
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
    editUserMessageMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('applies and activates the returned branch snapshot', async () => {
    const session = makeBranchSession();
    editUserMessageMock.mockResolvedValue({
      session,
      history: {
        items: [
          { id: 'item-1', oi: 1, kd: 'user', tp: 'userMessage', txt: 'Earlier prompt' },
          { id: 'item-2', oi: 2, kd: 'user', tp: 'user_message', txt: 'Revised prompt' },
        ],
        hasMore: false,
        total: 2,
      },
      pendingInputs: [],
      scheduledInputs: [],
    });
    const store = useWebSessionStore();

    const result = await store.editUserMessage(
      'project-1',
      'session-source',
      'source-item-2',
      'Revised prompt'
    );

    expect(editUserMessageMock).toHaveBeenCalledWith(
      'project-1',
      'session-source',
      'source-item-2',
      'Revised prompt'
    );
    expect(result.session?.id).toBe(session.id);
    expect(store.getActiveSessionId('project-1')).toBe(session.id);
    expect(store.getSessions('project-1')[0]?.id).toBe(session.id);
    expect(store.getBlocks(session.id).map(block => block.text)).toEqual([
      'Earlier prompt',
      'Revised prompt',
    ]);
    expect(store.getHistoryMeta(session.id).total).toBe(2);
  });
});
