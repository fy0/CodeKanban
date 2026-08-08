export type WebSessionRecencySessionLike = {
  id: string;
  orderIndex?: number | null;
  activityAt?: string | null;
  lastMessageAt?: string | null;
  updatedAt?: string | null;
  createdAt?: string | null;
};

export function parseWebSessionRecencyTimestamp(value?: string | null) {
  if (!value) {
    return 0;
  }
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

export function resolveWebSessionRecencyTimestamp(session: WebSessionRecencySessionLike) {
  return Math.max(
    parseWebSessionRecencyTimestamp(session.activityAt),
    parseWebSessionRecencyTimestamp(session.lastMessageAt),
    parseWebSessionRecencyTimestamp(session.updatedAt),
    parseWebSessionRecencyTimestamp(session.createdAt)
  );
}

function sortableOrderIndex(value: WebSessionRecencySessionLike['orderIndex']) {
  return typeof value === 'number' && Number.isFinite(value) ? value : Number.POSITIVE_INFINITY;
}

export function compareWebSessionsByRecency(
  left: WebSessionRecencySessionLike,
  right: WebSessionRecencySessionLike
) {
  const rightTimestamp = resolveWebSessionRecencyTimestamp(right);
  const leftTimestamp = resolveWebSessionRecencyTimestamp(left);
  if (rightTimestamp !== leftTimestamp) {
    return rightTimestamp - leftTimestamp;
  }
  const leftOrder = sortableOrderIndex(left.orderIndex);
  const rightOrder = sortableOrderIndex(right.orderIndex);
  if (leftOrder !== rightOrder) {
    return leftOrder - rightOrder;
  }
  return left.id.localeCompare(right.id);
}

export function sortWebSessionsByRecency<T extends WebSessionRecencySessionLike>(sessions: T[]) {
  return [...sessions].sort(compareWebSessionsByRecency);
}

export function selectMostRecentWebSession<T extends WebSessionRecencySessionLike>(
  sessions: T[],
  options?: { excludeIds?: Iterable<string> }
) {
  const excluded = new Set(options?.excludeIds ?? []);
  return sortWebSessionsByRecency(sessions.filter(session => !excluded.has(session.id)))[0] ?? null;
}
