import { describe, expect, it } from 'vitest';

import {
  buildWebSessionUserInputDraftStorageKey,
  buildWebSessionUserInputDraftSyncKey,
  buildWebSessionUserInputQuestionMemoDeps,
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

  it('keeps question memo dependencies stable across equivalent live refresh objects', () => {
    const question = {
      id: 'scope',
      header: 'Scope',
      question: 'Which scope should I use?',
      multiSelect: false,
      isOther: true,
      isSecret: false,
      options: [{ label: 'Frontend', description: 'Update the UI only.' }],
    };
    const first = buildWebSessionUserInputQuestionMemoDeps({
      requestKey: 'session-1:request-1',
      question,
      selections: ['Frontend'],
      draft: 'Keep the current behavior',
      disabled: false,
      placeholder: 'Add extra detail',
    });
    const second = buildWebSessionUserInputQuestionMemoDeps({
      requestKey: 'session-1:request-1',
      question: {
        ...question,
        options: question.options.map(option => ({ ...option })),
      },
      selections: ['Frontend'],
      draft: 'Keep the current behavior',
      disabled: false,
      placeholder: 'Add extra detail',
    });

    expect(second).toEqual(first);
  });

  it('invalidates question memo dependencies when visible or editable state changes', () => {
    const input = {
      requestKey: 'session-1:request-1',
      question: {
        id: 'scope',
        header: 'Scope',
        question: 'Which scope should I use?',
        multiSelect: false,
        isOther: true,
        isSecret: false,
        options: [{ label: 'Frontend', description: 'Update the UI only.' }],
      },
      selections: ['Frontend'],
      draft: 'Keep the current behavior',
      disabled: false,
      placeholder: 'Add extra detail',
    };
    const base = buildWebSessionUserInputQuestionMemoDeps(input);

    expect(buildWebSessionUserInputQuestionMemoDeps({ ...input, draft: 'Changed' })).not.toEqual(
      base
    );
    expect(buildWebSessionUserInputQuestionMemoDeps({ ...input, disabled: true })).not.toEqual(
      base
    );
    expect(
      buildWebSessionUserInputQuestionMemoDeps({
        ...input,
        question: { ...input.question, question: 'Updated question?' },
      })
    ).not.toEqual(base);
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
