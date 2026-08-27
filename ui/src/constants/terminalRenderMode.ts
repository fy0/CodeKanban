export type TerminalRenderMode = 'live' | 'snapshot';

export const DEFAULT_TERMINAL_RENDER_MODE: TerminalRenderMode = 'live';
export const DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS = 50;
export const MIN_TERMINAL_SNAPSHOT_INTERVAL_MS = 33;
export const MAX_TERMINAL_SNAPSHOT_INTERVAL_MS = 10000;

export const TERMINAL_SNAPSHOT_INTERVAL_OPTIONS = [
  33, 50, 100, 250, 500, 1000, 2000, 5000, 10000,
] as const;

export function sanitizeTerminalRenderMode(value: unknown): TerminalRenderMode {
  return value === 'snapshot' ? 'snapshot' : 'live';
}

export function sanitizeTerminalSnapshotIntervalMs(
  value: unknown,
  fallback = DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS
) {
  const numericFallback = Number.isFinite(Number(fallback))
    ? Number(fallback)
    : DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS;
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return clampSnapshotInterval(numericFallback);
  }
  return clampSnapshotInterval(parsed);
}

export function formatTerminalSnapshotInterval(intervalMs: number | null | undefined) {
  const normalized = sanitizeTerminalSnapshotIntervalMs(intervalMs);
  if (normalized < 1000) {
    return `${normalized}ms`;
  }
  if (normalized % 1000 === 0) {
    return `${normalized / 1000}s`;
  }
  return `${normalized / 1000}s`;
}

function clampSnapshotInterval(value: number) {
  return Math.min(
    Math.max(Math.round(value), MIN_TERMINAL_SNAPSHOT_INTERVAL_MS),
    MAX_TERMINAL_SNAPSHOT_INTERVAL_MS
  );
}
