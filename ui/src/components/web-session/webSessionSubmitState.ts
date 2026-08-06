import type { WebSessionLiveState } from '@/stores/webSession';

export type WebSessionSubmitKind =
  | 'execute_send'
  | 'execute_plan'
  | 'plan_message'
  | 'redirect_message'
  | 'queue_message';

export interface WebSessionSubmitEntry {
  kind: WebSessionSubmitKind;
  startedAt: number;
}

export type WebSessionSubmitState = Record<string, WebSessionSubmitEntry>;

function normalizeSubmitOwnerId(ownerId: string) {
  return String(ownerId || '').trim();
}

export function buildWebSessionSubmitOwnerId(...ownerIdParts: string[]) {
  return ownerIdParts.map(normalizeSubmitOwnerId).filter(Boolean).join('::');
}

function normalizeSubmitStartedAt(startedAt?: number) {
  return typeof startedAt === 'number' && Number.isFinite(startedAt) && startedAt > 0
    ? startedAt
    : Date.now();
}

function normalizeSubmitEntry(
  entry?: Pick<WebSessionSubmitEntry, 'kind'> & Partial<Pick<WebSessionSubmitEntry, 'startedAt'>>
): WebSessionSubmitEntry {
  return {
    kind: entry?.kind ?? 'plan_message',
    startedAt: normalizeSubmitStartedAt(entry?.startedAt),
  };
}

export function beginWebSessionSubmit(
  state: WebSessionSubmitState,
  ownerId: string,
  entry?: Pick<WebSessionSubmitEntry, 'kind'> & Partial<Pick<WebSessionSubmitEntry, 'startedAt'>>
): WebSessionSubmitState {
  const normalizedOwnerId = normalizeSubmitOwnerId(ownerId);
  if (!normalizedOwnerId || state[normalizedOwnerId]) {
    return state;
  }
  return {
    ...state,
    [normalizedOwnerId]: normalizeSubmitEntry(entry),
  };
}

export function endWebSessionSubmit(
  state: WebSessionSubmitState,
  ownerId: string
): WebSessionSubmitState {
  const normalizedOwnerId = normalizeSubmitOwnerId(ownerId);
  if (!normalizedOwnerId || !state[normalizedOwnerId]) {
    return state;
  }
  const nextState = { ...state };
  delete nextState[normalizedOwnerId];
  return nextState;
}

export function isWebSessionSubmitting(state: WebSessionSubmitState, ownerId: string) {
  const normalizedOwnerId = normalizeSubmitOwnerId(ownerId);
  return Boolean(normalizedOwnerId && state[normalizedOwnerId]);
}

export function getWebSessionSubmitEntry(
  state: WebSessionSubmitState,
  ownerId: string
): WebSessionSubmitEntry | null {
  const normalizedOwnerId = normalizeSubmitOwnerId(ownerId);
  return normalizedOwnerId ? (state[normalizedOwnerId] ?? null) : null;
}

export function shouldShowWebSessionExecuteFeedback(
  entry: Pick<WebSessionSubmitEntry, 'kind'> | null | undefined
) {
  return entry?.kind === 'execute_send' || entry?.kind === 'execute_plan';
}

export function resolveOptimisticWebSessionLiveState(
  state: WebSessionLiveState,
  entry: WebSessionSubmitEntry | null | undefined
): WebSessionLiveState {
  if (!entry || !shouldShowWebSessionExecuteFeedback(entry) || state.running) {
    return state;
  }

  return {
    phase: 'starting',
    running: true,
    updatedAt: entry.startedAt,
    startedAt: entry.startedAt,
  };
}

export function transferWebSessionSubmit(
  state: WebSessionSubmitState,
  fromOwnerId: string,
  toOwnerId: string
): WebSessionSubmitState {
  const normalizedFromOwnerId = normalizeSubmitOwnerId(fromOwnerId);
  const normalizedToOwnerId = normalizeSubmitOwnerId(toOwnerId);
  if (
    !normalizedFromOwnerId ||
    normalizedFromOwnerId === normalizedToOwnerId ||
    !state[normalizedFromOwnerId]
  ) {
    return state;
  }

  const nextState = { ...state };
  const entry = nextState[normalizedFromOwnerId];
  delete nextState[normalizedFromOwnerId];
  if (normalizedToOwnerId && entry) {
    nextState[normalizedToOwnerId] = entry;
  }
  return nextState;
}
