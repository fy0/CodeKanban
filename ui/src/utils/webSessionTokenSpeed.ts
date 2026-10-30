import type {
  WebSessionTokenSpeed,
  WebSessionTokenSpeedRecord,
  WebSessionTokenSpeedStats,
  WebSessionTokenSpeedSummary,
} from '@/types/models';

function nonNegativeNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0;
}

function normalizeSummary(value: unknown): WebSessionTokenSpeedSummary | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  if (
    !nonNegativeNumber(record.outputTokens) ||
    !nonNegativeNumber(record.durationMs) ||
    !nonNegativeNumber(record.sampleCount) ||
    !Number.isSafeInteger(record.sampleCount)
  )
    return null;
  return {
    tokensPerSecond: nonNegativeNumber(record.tokensPerSecond) ? record.tokensPerSecond : null,
    outputTokens: record.outputTokens,
    durationMs: record.durationMs,
    sampleCount: record.sampleCount,
    estimated: record.estimated !== false,
  };
}

function normalizeStats(value: unknown): WebSessionTokenSpeedStats | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  const currentRun = normalizeSummary(record.currentRun);
  const recent5 = normalizeSummary(record.recent5);
  const recent20 = normalizeSummary(record.recent20);
  const session = normalizeSummary(record.session);
  return currentRun && recent5 && recent20 && session
    ? { currentRun, recent5, recent20, session }
    : null;
}

function normalizeHistoryRecord(value: unknown): WebSessionTokenSpeedRecord | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  if (
    typeof record.runId !== 'string' ||
    !record.runId ||
    typeof record.sampleId !== 'string' ||
    !record.sampleId ||
    typeof record.startedAt !== 'string' ||
    !Number.isFinite(Date.parse(record.startedAt)) ||
    typeof record.endedAt !== 'string' ||
    !Number.isFinite(Date.parse(record.endedAt)) ||
    !nonNegativeNumber(record.outputTokens) ||
    !nonNegativeNumber(record.durationMs) ||
    record.durationMs < 500 ||
    !nonNegativeNumber(record.tokensPerSecond)
  )
    return null;
  return {
    runId: record.runId,
    sampleId: record.sampleId,
    agent: typeof record.agent === 'string' ? record.agent : '',
    model: typeof record.model === 'string' ? record.model : '',
    startedAt: record.startedAt,
    endedAt: record.endedAt,
    outputTokens: record.outputTokens,
    durationMs: record.durationMs,
    tokensPerSecond: record.tokensPerSecond,
    estimated: record.estimated !== false,
  };
}

export function normalizeTokenSpeed(value: unknown): WebSessionTokenSpeed | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  if (
    typeof record.epoch !== 'string' ||
    !record.epoch ||
    typeof record.version !== 'number' ||
    !Number.isSafeInteger(record.version) ||
    record.version < 0
  )
    return null;
  const stats = normalizeStats(record.stats);
  return {
    epoch: record.epoch,
    version: record.version,
    runId: typeof record.runId === 'string' ? record.runId : '',
    sampleId: typeof record.sampleId === 'string' ? record.sampleId : '',
    tokensPerSecond:
      typeof record.tokensPerSecond === 'number' &&
      Number.isFinite(record.tokensPerSecond) &&
      record.tokensPerSecond >= 0
        ? record.tokensPerSecond
        : null,
    estimated: record.estimated !== false,
    generating: record.generating === true,
    updatedAt: typeof record.updatedAt === 'string' ? record.updatedAt : '',
    ...(stats ? { stats } : {}),
    ...(Array.isArray(record.history)
      ? {
          history: record.history
            .map(normalizeHistoryRecord)
            .filter((item): item is WebSessionTokenSpeedRecord => item !== null)
            .slice(0, 20),
        }
      : {}),
  };
}

export function formatTokenSpeed(
  speed?: Pick<WebSessionTokenSpeed, 'tokensPerSecond' | 'estimated'> | null
) {
  const value = speed?.tokensPerSecond;
  if (value == null || !Number.isFinite(value) || value < 0) return '-- t/s';
  return `${Math.round(value)} t/s`;
}

export type TokenSpeedMetric = 'latest' | keyof WebSessionTokenSpeedStats;

export function selectTokenSpeedMetric(
  speed: WebSessionTokenSpeed | null | undefined,
  metric: TokenSpeedMetric
) {
  if (metric === 'latest') return { metric, value: speed };
  const summary = speed?.stats?.[metric];
  if (summary?.tokensPerSecond != null) return { metric, value: summary };
  if (metric === 'recent5' && speed?.stats?.currentRun.tokensPerSecond != null) {
    return { metric: 'currentRun' as const, value: speed.stats.currentRun };
  }
  return { metric: summary ? metric : ('latest' as const), value: summary ?? speed };
}

// Telemetry has its own clock. Retired process epochs prevent late
// HTTP responses from undoing data received after a server restart.
export class WebSessionTokenSpeedState {
  private epoch = '';
  private retiredEpochs = new Set<string>();
  private samples = new Map<string, WebSessionTokenSpeed>();

  apply(sessionId: string, value: unknown) {
    const next = normalizeTokenSpeed(value);
    if (!next || this.retiredEpochs.has(next.epoch)) return;
    if (this.epoch !== next.epoch) {
      if (this.epoch) this.retiredEpochs.add(this.epoch);
      this.epoch = next.epoch;
      this.samples.clear();
    }
    const current = this.samples.get(sessionId);
    if (!current || next.version > current.version) this.samples.set(sessionId, next);
  }

  get(sessionId: string) {
    return this.samples.get(sessionId) ?? null;
  }
  remove(sessionId: string) {
    this.samples.delete(sessionId);
  }
}
