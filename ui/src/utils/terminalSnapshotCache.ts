export type TerminalSnapshotRecord = {
  serialized: string;
  cols: number;
  rows: number;
  savedAt: number;
};

const snapshotStore = new Map<string, TerminalSnapshotRecord>();
const MAX_ENTRIES = 12;
const SNAPSHOT_TTL_MS = 30 * 60 * 1000; // 30 minutes

function prune(now = Date.now()) {
  for (const [sessionId, record] of snapshotStore) {
    if (now - record.savedAt > SNAPSHOT_TTL_MS) {
      snapshotStore.delete(sessionId);
    }
  }
  while (snapshotStore.size > MAX_ENTRIES) {
    const oldestKey = snapshotStore.keys().next().value;
    if (oldestKey) {
      snapshotStore.delete(oldestKey);
    } else {
      break;
    }
  }
}

export function saveTerminalSnapshot(
  sessionId: string,
  payload: { serialized: string; cols: number; rows: number }
) {
  if (!sessionId || !payload.serialized) {
    return;
  }
  const now = Date.now();
  snapshotStore.set(sessionId, {
    serialized: payload.serialized,
    cols: payload.cols,
    rows: payload.rows,
    savedAt: now,
  });
  prune(now);
}

export function getTerminalSnapshot(sessionId: string): TerminalSnapshotRecord | undefined {
  if (!sessionId) {
    return undefined;
  }
  const now = Date.now();
  const record = snapshotStore.get(sessionId);
  if (!record) {
    return undefined;
  }
  if (now - record.savedAt > SNAPSHOT_TTL_MS) {
    snapshotStore.delete(sessionId);
    return undefined;
  }
  return record;
}

export function clearTerminalSnapshot(sessionId: string) {
  if (!sessionId) {
    return;
  }
  snapshotStore.delete(sessionId);
}
