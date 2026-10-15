import type { WebSessionTokenSpeed } from '@/types/models';

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
  };
}

export function formatTokenSpeed(speed?: WebSessionTokenSpeed | null) {
  const value = speed?.tokensPerSecond;
  if (value == null || !Number.isFinite(value) || value < 0) return '-- t/s';
  return `${speed?.estimated ? '≈' : ''}${Math.round(value)} t/s`;
}

// Ephemeral telemetry has its own clock. Retired process epochs prevent late
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
