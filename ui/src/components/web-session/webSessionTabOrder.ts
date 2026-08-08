import {
  selectMostRecentWebSession,
  type WebSessionRecencySessionLike,
} from '@/utils/webSessionRecency';

export type OrderedTabSessionLike = {
  id: string;
};

export type MobileCurrentSessionLike = OrderedTabSessionLike & {
  orderIndex: number;
  isDraft?: boolean;
};

export type CloseFallbackSessionLike = OrderedTabSessionLike & WebSessionRecencySessionLike;

export function clampTabAnchorIndex(anchorIndex: number, baseLength: number) {
  if (!Number.isFinite(anchorIndex)) {
    return Math.max(0, baseLength);
  }
  const normalizedBaseLength = Math.max(0, Math.trunc(baseLength));
  return Math.min(normalizedBaseLength, Math.max(0, Math.trunc(anchorIndex)));
}

function normalizeSessionId(sessionId = '') {
  return String(sessionId || '').trim();
}

function normalizeSessionIdList(sessionIds: string[] | undefined) {
  if (!Array.isArray(sessionIds) || sessionIds.length === 0) {
    return [];
  }
  const next: string[] = [];
  sessionIds.forEach(sessionId => {
    const normalized = normalizeSessionId(sessionId);
    if (!normalized || next.includes(normalized)) {
      return;
    }
    next.push(normalized);
  });
  return next;
}

export function resolveUnderlyingTabSessionId(options: {
  activeDraftSessionId?: string;
  activeRealSessionId?: string;
}) {
  return (
    normalizeSessionId(options.activeDraftSessionId) ||
    normalizeSessionId(options.activeRealSessionId)
  );
}

export function resolveActiveTabSessionId(options: {
  activeArchivedPreviewId?: string;
  activeDraftSessionId?: string;
  activeRealSessionId?: string;
}) {
  if (normalizeSessionId(options.activeArchivedPreviewId)) {
    return '';
  }
  return resolveUnderlyingTabSessionId(options);
}

export function resolveTabAnchorInsertIndex<T extends OrderedTabSessionLike>(
  orderedSessions: T[],
  anchorId = ''
) {
  const normalizedAnchorId = String(anchorId || '').trim();
  if (!normalizedAnchorId) {
    return orderedSessions.length;
  }
  const anchorIndex = orderedSessions.findIndex(session => session.id === normalizedAnchorId);
  return anchorIndex >= 0 ? anchorIndex + 1 : orderedSessions.length;
}

export function buildOrderedTabSessions<T extends OrderedTabSessionLike>(
  orderedIds: string[],
  baseSessions: T[],
  fixedSession?: T | null,
  fixedAnchorIndex = baseSessions.length
) {
  const sessionById = new Map<string, T>();
  baseSessions.forEach(session => {
    sessionById.set(session.id, session);
  });

  const ordered: T[] = [];
  const seen = new Set<string>();

  orderedIds.forEach(sessionId => {
    const session = sessionById.get(sessionId);
    if (!session || seen.has(session.id)) {
      return;
    }
    ordered.push(session);
    seen.add(session.id);
  });

  baseSessions.forEach(session => {
    if (seen.has(session.id)) {
      return;
    }
    ordered.push(session);
    seen.add(session.id);
  });

  if (!fixedSession) {
    return ordered;
  }

  const anchored = [...ordered];
  anchored.splice(clampTabAnchorIndex(fixedAnchorIndex, anchored.length), 0, fixedSession);
  return anchored;
}

export function resolveNextWebSessionTabAfterClose<T extends CloseFallbackSessionLike>(options: {
  closingSessionId: string;
  sessions: T[];
  mruIds?: string[];
}) {
  const closingSessionId = normalizeSessionId(options.closingSessionId);
  const remainingSessions = options.sessions.filter(session => session.id !== closingSessionId);
  const remainingSessionById = new Map(remainingSessions.map(session => [session.id, session]));
  const mruTargetId = normalizeSessionIdList(options.mruIds).find(sessionId =>
    remainingSessionById.has(sessionId)
  );
  if (mruTargetId) {
    return mruTargetId;
  }
  return selectMostRecentWebSession(remainingSessions)?.id ?? '';
}

function sortableNumber(value: number) {
  return Number.isFinite(value) ? value : 0;
}

export function sortMobileCurrentSessions<T extends MobileCurrentSessionLike>(
  sessions: T[],
  resolveSortTimestamp: (session: T) => number
) {
  const drafts: T[] = [];
  const realSessions: T[] = [];

  sessions.forEach(session => {
    if (session.isDraft) {
      drafts.push(session);
      return;
    }
    realSessions.push(session);
  });

  const sortedRealSessions = [...realSessions].sort((left, right) => {
    const rightTimestamp = sortableNumber(resolveSortTimestamp(right));
    const leftTimestamp = sortableNumber(resolveSortTimestamp(left));
    if (rightTimestamp !== leftTimestamp) {
      return rightTimestamp - leftTimestamp;
    }
    if (left.orderIndex !== right.orderIndex) {
      return left.orderIndex - right.orderIndex;
    }
    return left.id.localeCompare(right.id);
  });

  return [...drafts, ...sortedRealSessions];
}
