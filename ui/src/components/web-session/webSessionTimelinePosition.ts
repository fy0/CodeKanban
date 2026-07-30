export const WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY =
  'workspace-web-session-timeline-position-v1';

const STORAGE_VERSION = 1;
export const WEB_SESSION_TIMELINE_POSITION_MAX_RECORDS = 200;

export interface WebSessionTimelinePosition {
  anchorKey: string;
  anchorOrderIndex: number | null;
  anchorOffsetPx: number;
  scrollTop: number;
  followBottom: boolean;
  updatedAt: number;
}

export interface WebSessionTimelineAnchorCandidate {
  key: string;
  orderIndex: number;
  top: number;
  bottom: number;
}

export interface WebSessionTimelinePositionState {
  version: 1;
  projects: Record<string, Record<string, WebSessionTimelinePosition>>;
}

type TimelinePositionStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

function createEmptyState(): WebSessionTimelinePositionState {
  return {
    version: STORAGE_VERSION,
    projects: {},
  };
}

function normalizeFiniteNumber(value: unknown, fallback = 0) {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function normalizePosition(value: unknown): WebSessionTimelinePosition | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return null;
  }
  const candidate = value as Partial<WebSessionTimelinePosition>;
  if (typeof candidate.followBottom !== 'boolean') {
    return null;
  }
  const anchorOrderIndex =
    candidate.anchorOrderIndex == null
      ? null
      : normalizeFiniteNumber(candidate.anchorOrderIndex, Number.NaN);
  if (anchorOrderIndex != null && !Number.isFinite(anchorOrderIndex)) {
    return null;
  }
  return {
    anchorKey: typeof candidate.anchorKey === 'string' ? candidate.anchorKey : '',
    anchorOrderIndex,
    anchorOffsetPx: normalizeFiniteNumber(candidate.anchorOffsetPx),
    scrollTop: Math.max(0, normalizeFiniteNumber(candidate.scrollTop)),
    followBottom: candidate.followBottom,
    updatedAt: Math.max(0, normalizeFiniteNumber(candidate.updatedAt, Date.now())),
  };
}

function pruneTimelinePositions(
  state: WebSessionTimelinePositionState,
  maxRecords = WEB_SESSION_TIMELINE_POSITION_MAX_RECORDS
) {
  const entries: Array<{
    projectId: string;
    sessionId: string;
    position: WebSessionTimelinePosition;
  }> = [];
  Object.entries(state.projects).forEach(([projectId, sessions]) => {
    Object.entries(sessions).forEach(([sessionId, position]) => {
      entries.push({ projectId, sessionId, position });
    });
  });
  entries.sort((left, right) => right.position.updatedAt - left.position.updatedAt);

  const projects: WebSessionTimelinePositionState['projects'] = {};
  entries.slice(0, Math.max(0, maxRecords)).forEach(({ projectId, sessionId, position }) => {
    projects[projectId] = projects[projectId] ?? {};
    projects[projectId][sessionId] = position;
  });
  state.projects = projects;
  return state;
}

export function loadWebSessionTimelinePositionState(
  storage?: TimelinePositionStorage | null
): WebSessionTimelinePositionState {
  if (!storage) {
    return createEmptyState();
  }
  try {
    const raw = storage.getItem(WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY);
    if (!raw) {
      return createEmptyState();
    }
    const parsed = JSON.parse(raw) as Partial<WebSessionTimelinePositionState>;
    if (
      parsed.version !== STORAGE_VERSION ||
      !parsed.projects ||
      typeof parsed.projects !== 'object'
    ) {
      storage.removeItem(WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY);
      return createEmptyState();
    }

    const state = createEmptyState();
    Object.entries(parsed.projects).forEach(([projectId, rawSessions]) => {
      if (
        !projectId ||
        !rawSessions ||
        typeof rawSessions !== 'object' ||
        Array.isArray(rawSessions)
      ) {
        return;
      }
      Object.entries(rawSessions).forEach(([sessionId, rawPosition]) => {
        const position = normalizePosition(rawPosition);
        if (!sessionId || !position) {
          return;
        }
        state.projects[projectId] = state.projects[projectId] ?? {};
        state.projects[projectId][sessionId] = position;
      });
    });
    return pruneTimelinePositions(state);
  } catch (error) {
    console.warn('[Web Session] Failed to load timeline positions', error);
    try {
      storage.removeItem(WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY);
    } catch {
      // Ignore cleanup failures and continue with an in-memory default.
    }
    return createEmptyState();
  }
}

export function persistWebSessionTimelinePositionState(
  state: WebSessionTimelinePositionState,
  storage?: TimelinePositionStorage | null
) {
  if (!storage) {
    return;
  }
  try {
    pruneTimelinePositions(state);
    if (Object.keys(state.projects).length === 0) {
      storage.removeItem(WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY);
      return;
    }
    storage.setItem(WEB_SESSION_TIMELINE_POSITION_STORAGE_KEY, JSON.stringify(state));
  } catch (error) {
    console.warn('[Web Session] Failed to persist timeline positions', error);
  }
}

export function getWebSessionTimelinePosition(
  state: WebSessionTimelinePositionState,
  projectId: string,
  sessionId: string
) {
  return state.projects[projectId]?.[sessionId] ?? null;
}

export function rememberWebSessionTimelinePosition(
  state: WebSessionTimelinePositionState,
  projectId: string,
  sessionId: string,
  position: WebSessionTimelinePosition,
  maxRecords = WEB_SESSION_TIMELINE_POSITION_MAX_RECORDS
) {
  if (!projectId || !sessionId) {
    return state;
  }
  const normalized = normalizePosition(position);
  if (!normalized) {
    return state;
  }
  state.projects[projectId] = state.projects[projectId] ?? {};
  state.projects[projectId][sessionId] = normalized;
  return pruneTimelinePositions(state, maxRecords);
}

export function forgetWebSessionTimelinePosition(
  state: WebSessionTimelinePositionState,
  projectId: string,
  sessionId: string
) {
  const sessions = state.projects[projectId];
  if (!sessions?.[sessionId]) {
    return state;
  }
  delete sessions[sessionId];
  if (Object.keys(sessions).length === 0) {
    delete state.projects[projectId];
  }
  return state;
}

export function resolveWebSessionTimelineAnchor(
  candidates: WebSessionTimelineAnchorCandidate[],
  viewportTop: number,
  viewportBottom: number
) {
  return (
    candidates.find(
      candidate => candidate.bottom > viewportTop && candidate.top < viewportBottom
    ) ?? null
  );
}

export function resolveWebSessionTimelineRestoreScrollTop(
  currentScrollTop: number,
  currentAnchorTop: number,
  savedAnchorOffsetPx: number,
  maxScrollTop: number
) {
  const target =
    normalizeFiniteNumber(currentScrollTop) +
    normalizeFiniteNumber(currentAnchorTop) -
    normalizeFiniteNumber(savedAnchorOffsetPx);
  return Math.min(Math.max(0, normalizeFiniteNumber(maxScrollTop)), Math.max(0, target));
}

export function findClosestWebSessionTimelineAnchor<T extends { key: string; orderIndex: number }>(
  candidates: T[],
  anchorKey: string,
  anchorOrderIndex: number | null
) {
  const exact = anchorKey ? candidates.find(candidate => candidate.key === anchorKey) : undefined;
  if (exact || anchorOrderIndex == null || candidates.length === 0) {
    return exact ?? null;
  }
  return candidates.reduce((closest, candidate) => {
    const closestDistance = Math.abs(closest.orderIndex - anchorOrderIndex);
    const candidateDistance = Math.abs(candidate.orderIndex - anchorOrderIndex);
    return candidateDistance < closestDistance ? candidate : closest;
  });
}
