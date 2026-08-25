import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { WebSessionSummary } from '@/types/models';
import { useWebSessionStore } from '@/stores/webSession';

const { listMock, syncMock, snapshotMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
  syncMock: vi.fn(),
  snapshotMock: vi.fn(),
}));

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    list: listMock,
    sync: syncMock,
    snapshot: snapshotMock,
  },
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
    activityAt: '2026-04-09T10:00:00.000Z',
    lastMessageAt: '2026-04-09T10:00:00.000Z',
    assistantStateUpdatedAt: '2026-04-09T10:00:00.000Z',
    sourceKind: 'codex_app_server',
    syncState: 'fresh',
    lastSyncMode: 'fast',
    sourceCreatedAt: '2026-04-09T09:00:00.000Z',
    sourceUpdatedAt: '2026-04-09T10:00:00.000Z',
    lastSyncedAt: '2026-04-09T10:00:00.000Z',
    threadPath: '/tmp/session.jsonl',
    threadPreview: 'preview',
    turnCount: 1,
    itemCount: 1,
    syncError: null,
    createdAt: '2026-04-09T09:00:00.000Z',
    updatedAt: '2026-04-09T10:00:00.000Z',
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

describe('webSession pending user input', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', createStorageMock());
    listMock.mockReset();
    syncMock.mockReset();
    snapshotMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps pending user input drafts in memory and returns defensive copies', () => {
    const store = useWebSessionStore();
    const storageKey = JSON.stringify(['session-1', 'request-1']);

    store.setPendingUserInputDraft(storageKey, {
      selections: {
        scope: ['Full migration'],
      },
      drafts: {
        notes: 'Keep this while switching sessions',
      },
    });

    expect(localStorage.getItem(storageKey)).toBeNull();

    const firstRead = store.getPendingUserInputDraft(storageKey);
    expect(firstRead?.selections.scope).toEqual(['Full migration']);
    expect(firstRead?.drafts.notes).toBe('Keep this while switching sessions');

    firstRead?.selections.scope.push('mutated');
    firstRead!.drafts.notes = 'mutated';

    const secondRead = store.getPendingUserInputDraft(storageKey);
    expect(secondRead?.selections.scope).toEqual(['Full migration']);
    expect(secondRead?.drafts.notes).toBe('Keep this while switching sessions');

    store.clearPendingUserInputDraft(storageKey);
    expect(store.getPendingUserInputDraft(storageKey)).toBeNull();
  });

  it('persists pending input edit drafts by project, session, and item', () => {
    const store = useWebSessionStore();

    store.setPendingInputEditDraft(
      'project-1',
      'session-1',
      'pending-1',
      'Keep this while switching pages'
    );
    store.setPendingInputEditDraft('project-1', 'session-1', 'pending-2', 'Another edit');

    const firstRead = store.getPendingInputEditDraft('project-1', 'session-1', 'pending-1');
    expect(firstRead?.text).toBe('Keep this while switching pages');
    expect(store.getPendingInputEditDraft('project-2', 'session-1', 'pending-1')).toBeNull();

    firstRead!.text = 'mutated';
    expect(store.getPendingInputEditDraft('project-1', 'session-1', 'pending-1')?.text).toBe(
      'Keep this while switching pages'
    );

    const persisted = JSON.parse(
      localStorage.getItem('kanban-web-session-pending-input-edits') ?? '{}'
    ) as Record<string, Record<string, Record<string, { text: string }>>>;
    expect(persisted['project-1']?.['session-1']?.['pending-1']?.text).toBe(
      'Keep this while switching pages'
    );

    store.clearPendingInputEditDraft('project-1', 'session-1', 'pending-1');
    expect(store.getPendingInputEditDraft('project-1', 'session-1', 'pending-1')).toBeNull();
    expect(store.getPendingInputEditDraft('project-1', 'session-1', 'pending-2')?.text).toBe(
      'Another edit'
    );
  });

  it('clears all edit drafts for a session without affecting another session', () => {
    const store = useWebSessionStore();

    store.setPendingInputEditDraft('project-1', 'session-1', 'pending-1', 'Session one');
    store.setPendingInputEditDraft('project-1', 'session-2', 'pending-2', 'Session two');

    store.clearPendingInputEditDrafts('project-1', 'session-1');

    expect(store.getPendingInputEditDrafts('project-1', 'session-1')).toEqual({});
    expect(store.getPendingInputEditDraft('project-1', 'session-2', 'pending-2')?.text).toBe(
      'Session two'
    );
  });

  it('recovers sourceItemId from payload.iid for older user input history items', async () => {
    const store = useWebSessionStore();
    const session = makeSession();
    const requestID = 'req_input_123';

    listMock.mockResolvedValue([session]);
    syncMock.mockResolvedValue({ revision: '2', session: { ...session, revision: '2' } });
    snapshotMock.mockResolvedValue({
      revision: '2',
      session: { ...session, revision: '2' },
      history: {
        items: [
          {
            id: 'history-item-1',
            oi: 1,
            kd: 'system',
            tp: 'user_input_request',
            txt: 'Please choose a scope',
            ts2: Date.parse('2026-04-09T10:00:00.000Z'),
            dt: {
              type: 'user_input_request',
              prompt: 'Please choose a scope',
              questions: [
                {
                  id: 'scope',
                  header: 'Scope',
                  question: 'Which scope should I use?',
                  isOther: false,
                  isSecret: false,
                  options: [
                    {
                      label: 'Full migration',
                      description: 'Apply all changes',
                    },
                  ],
                },
              ],
            },
            pl: {
              iid: requestID,
            },
          },
        ],
        hasMore: false,
        total: 1,
      },
    });

    await store.loadSessions(session.projectId);
    await store.syncSession(session.projectId, session.id);

    const blocks = store.getBlocks(session.id);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.sourceItemId).toBe(requestID);

    const pending = store.getPendingUserInput(session.id);
    expect(pending?.itemId).toBe(requestID);
    expect(pending?.prompt).toBe('Please choose a scope');
  });

  it('renders submitted user input answers from legacy payloads', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      status: 'done',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    syncMock.mockResolvedValue({ revision: '3', session: { ...session, revision: '3' } });
    snapshotMock.mockResolvedValue({
      revision: '3',
      session: { ...session, revision: '3' },
      history: {
        items: [
          {
            id: 'history-response-1',
            oi: 1,
            kd: 'system',
            tp: 'user_input_response',
            txt: 'Submitted requested input',
            ts2: Date.parse('2026-04-09T10:00:00.000Z'),
            dt: {
              type: 'user_input_response',
            },
            pl: {
              iid: 'req_input_123',
              ans: {
                scope: ['Full migration'],
              },
            },
          },
        ],
        hasMore: false,
        total: 1,
      },
    });

    await store.loadSessions(session.projectId);
    await store.syncSession(session.projectId, session.id);

    const block = store.getBlocks(session.id)[0];
    const answer = block?.detail?.answers?.find(item => item.id === 'scope');
    expect(answer?.values).toEqual(['Full migration']);
  });
});
