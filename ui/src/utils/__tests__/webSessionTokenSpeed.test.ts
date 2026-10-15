import { describe, expect, it } from 'vitest';
import { reactive } from 'vue';
import {
  formatTokenSpeed,
  normalizeTokenSpeed,
  WebSessionTokenSpeedState,
} from '../webSessionTokenSpeed';

const sample = (version = 1, epoch = 'process-1') => ({
  epoch,
  version,
  runId: 'run-1',
  sampleId: '1',
  tokensPerSecond: 42.4,
  estimated: true,
  generating: true,
  updatedAt: '2026-10-15T01:00:00Z',
});

describe('token speed telemetry', () => {
  it('formats unavailable, estimated and calibrated rates', () => {
    expect(formatTokenSpeed()).toBe('-- t/s');
    expect(formatTokenSpeed(sample())).toBe('≈42 t/s');
    expect(formatTokenSpeed({ ...sample(), estimated: false, tokensPerSecond: 42.6 })).toBe(
      '43 t/s'
    );
    expect(formatTokenSpeed({ ...sample(), tokensPerSecond: NaN })).toBe('-- t/s');
    expect(formatTokenSpeed({ ...sample(), estimated: false, tokensPerSecond: 0 })).toBe('0 t/s');
    expect(
      normalizeTokenSpeed({ ...sample(), tokensPerSecond: Infinity })?.tokensPerSecond
    ).toBeNull();
    expect(normalizeTokenSpeed(undefined)).toBeNull();
  });

  it('keeps the newest sample across duplicate, delayed and unchanged snapshots', () => {
    const state = reactive(new WebSessionTokenSpeedState());
    state.apply('s1', sample(5));
    state.apply('s1', { ...sample(4), estimated: false });
    state.apply('s1', { ...sample(5), tokensPerSecond: 900 });
    state.apply('s1', undefined);
    expect(state.get('s1')).toEqual(sample(5));
    state.apply('s1', { ...sample(6), estimated: false });
    expect(state.get('s1')?.estimated).toBe(false);
    state.apply('s2', { ...sample(), tokensPerSecond: 7 });
    expect(state.get('s2')?.tokensPerSecond).toBe(7);
    expect(state.get('s1')?.tokensPerSecond).toBe(42.4);
  });

  it('clears stale sessions on a process restart and rejects retired epochs', () => {
    const state = new WebSessionTokenSpeedState();
    state.apply('s1', sample(10));
    state.apply('s2', sample(8));
    state.apply('s1', { ...sample(0, 'process-2'), tokensPerSecond: null });
    state.apply('s1', sample(11));
    state.apply('s2', sample(12));
    expect(state.get('s1')?.tokensPerSecond).toBeNull();
    expect(state.get('s2')).toBeNull();
    state.apply('s2', sample(1, 'process-2'));
    state.remove('s2');
    expect(state.get('s2')).toBeNull();
  });
});
