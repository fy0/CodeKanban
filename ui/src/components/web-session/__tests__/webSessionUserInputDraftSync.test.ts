import { describe, expect, it } from 'vitest';

import {
  buildWebSessionUserInputDraftStorageKey,
  buildWebSessionUserInputDraftSyncKey,
  reconcileWebSessionUserInputLocalState,
} from '@/components/web-session/webSessionUserInputDraftSync';

function makeRequest(
  overrides: Partial<{
    itemId: string;
    prompt: string;
    requestedAt: number;
    stale: boolean;
    questions: Array<{ id: string; header?: string; question?: string }>;
  }> = {}
) {
  return {
    itemId: 'request-1',
    prompt: 'Choose a timeout',
    requestedAt: 1000,
    stale: false,
    questions: [
      {
        id: 'timeout',
        header: 'Timeout',
        question: 'How long should the timeout be?',
      },
      {
        id: 'scope',
        header: 'Scope',
        question: 'Which scope should I use?',
      },
    ],
    ...overrides,
  };
}

describe('webSessionUserInputDraftSync', () => {
  it('keeps the sync key stable when only prompt metadata changes', () => {
    const first = buildWebSessionUserInputDraftSyncKey('session-1', makeRequest());
    const second = buildWebSessionUserInputDraftSyncKey(
      'session-1',
      makeRequest({
        prompt: 'Choose a timeout before continuing',
        requestedAt: 2000,
        stale: true,
      })
    );

    expect(first).toBe(second);
  });

  it('changes the sync key when the request id or question ids change', () => {
    const base = buildWebSessionUserInputDraftSyncKey('session-1', makeRequest());
    const changedRequestId = buildWebSessionUserInputDraftSyncKey(
      'session-1',
      makeRequest({ itemId: 'request-2' })
    );
    const changedQuestions = buildWebSessionUserInputDraftSyncKey(
      'session-1',
      makeRequest({
        questions: [{ id: 'timeout' }, { id: 'approval-level' }],
      })
    );

    expect(changedRequestId).not.toBe(base);
    expect(changedQuestions).not.toBe(base);
  });

  it('keeps the storage key stable when only question ids change', () => {
    const base = buildWebSessionUserInputDraftStorageKey('session-1', makeRequest());
    const changedQuestions = buildWebSessionUserInputDraftStorageKey(
      'session-1',
      makeRequest({
        questions: [{ id: 'timeout' }, { id: 'approval-level' }],
      })
    );

    expect(changedQuestions).toBe(base);
  });

  it('reconciles local state by preserving current question drafts and dropping removed ones', () => {
    const nextState = reconcileWebSessionUserInputLocalState(
      [{ id: 'timeout' }, { id: 'scope' }, { id: 'notes' }],
      {
        selections: {
          timeout: ['10 minutes'],
          scope: ['Current session'],
          removed: ['stale choice'],
        },
        drafts: {
          scope: 'Keep this',
          removed: 'Drop this',
        },
      }
    );

    expect(nextState).toEqual({
      selections: {
        timeout: ['10 minutes'],
        scope: ['Current session'],
        notes: [],
      },
      drafts: {
        timeout: '',
        scope: 'Keep this',
        notes: '',
      },
    });
  });
});
