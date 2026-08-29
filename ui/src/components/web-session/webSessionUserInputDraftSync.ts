import type { WebSessionUserInputQuestion } from '@/stores/webSession';

export interface WebSessionUserInputDraftSyncRequest {
  itemId: string;
  questions: Pick<WebSessionUserInputQuestion, 'id'>[];
}

export interface WebSessionUserInputLocalState {
  selections: Record<string, string[]>;
  drafts: Record<string, string>;
}

function normalizeQuestionId(questionId: string) {
  return String(questionId ?? '');
}

function getQuestionIds(questions: Pick<WebSessionUserInputQuestion, 'id'>[]) {
  return questions.map(question => normalizeQuestionId(question.id));
}

// Ignore prompt/timestamp churn so streaming refreshes do not reset active input state.
export function buildWebSessionUserInputDraftSyncKey(
  sessionId: string | null | undefined,
  request: WebSessionUserInputDraftSyncRequest | null | undefined
) {
  const normalizedSessionId = String(sessionId ?? '').trim();
  const normalizedItemId = String(request?.itemId ?? '').trim();
  if (!normalizedSessionId || !normalizedItemId) {
    return '';
  }
  return JSON.stringify([
    normalizedSessionId,
    normalizedItemId,
    getQuestionIds(request?.questions ?? []),
  ]);
}

export function buildWebSessionUserInputDraftStorageKey(
  sessionId: string | null | undefined,
  request: WebSessionUserInputDraftSyncRequest | null | undefined
) {
  const normalizedSessionId = String(sessionId ?? '').trim();
  const normalizedItemId = String(request?.itemId ?? '').trim();
  if (!normalizedSessionId || !normalizedItemId) {
    return '';
  }
  return JSON.stringify([normalizedSessionId, normalizedItemId]);
}

export function reconcileWebSessionUserInputLocalState(
  questions: Pick<WebSessionUserInputQuestion, 'id'>[],
  currentState: WebSessionUserInputLocalState
): WebSessionUserInputLocalState {
  const selections: Record<string, string[]> = {};
  const drafts: Record<string, string> = {};

  questions.forEach(question => {
    const questionId = normalizeQuestionId(question.id);
    selections[questionId] = [...(currentState.selections[questionId] ?? [])];
    drafts[questionId] = currentState.drafts[questionId] ?? '';
  });

  return {
    selections,
    drafts,
  };
}
