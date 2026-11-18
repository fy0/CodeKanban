import { describe, expect, it } from 'vitest';
import {
  CODEX_PROMPT_CACHE_ESTIMATE_MS,
  estimateWebSessionPromptCache,
  formatPromptCacheCountdown,
  promptCacheRecordEvent,
} from '@/components/web-session/webSessionPromptCache';
import type { WebSessionTokenSpeedRecord } from '@/types/models';

const observedAt = Date.parse('2026-11-17T15:00:00Z');
const session = {
  agent: 'codex' as const,
  model: 'gpt-6.1-sol',
  contextEstimateMode: 'latest_token_count' as const,
  contextEstimate: {
    inputTokens: 20_000,
    cachedInputTokens: 18_000,
    outputTokens: 100,
    usedTokens: 20_100,
  },
  latestTokenCountUpdatedAt: new Date(observedAt).toISOString(),
};
const record: WebSessionTokenSpeedRecord = {
  runId: 'run',
  sampleId: '1',
  agent: 'codex',
  model: session.model,
  startedAt: new Date(observedAt - 1000).toISOString(),
  endedAt: session.latestTokenCountUpdatedAt,
  inputTokens: 20_000,
  cachedInputTokens: 18_000,
  outputTokens: 100,
  durationMs: 1000,
  tokensPerSecond: 100,
  estimated: false,
};

describe('Codex prompt cache estimates', () => {
  it.each(['gpt-5.6-sol', 'gpt-6-astra', 'gpt-6.1-sol', 'gpt-6.1-terra'])(
    '%s uses the documented default window',
    model => {
      expect(
        estimateWebSessionPromptCache({ ...session, model }, observedAt + 60_000)
      ).toMatchObject({
        observedAt,
        hitPercent: 90,
        inputTokens: 20_000,
        cachedTokens: 18_000,
        windowMs: CODEX_PROMPT_CACHE_ESTIMATE_MS,
        remainingMs: 29 * 60_000,
        state: 'estimated',
      });
    }
  );
  it('counts down from the persisted observation across reloads, without asserting expiry', () => {
    const restored = JSON.parse(JSON.stringify(session));
    expect(estimateWebSessionPromptCache(restored, observedAt + 26 * 60_000)).toMatchObject({
      remainingMs: 4 * 60_000,
      state: 'ending',
    });
    expect(estimateWebSessionPromptCache(restored, observedAt + 31 * 60_000)).toMatchObject({
      remainingMs: 0,
      state: 'elapsed',
      hitPercent: 90,
    });
  });
  it('refreshes the estimate only when a new request observation arrives', () => {
    const now = observedAt + 20 * 60_000;
    expect(estimateWebSessionPromptCache(session, now)?.remainingMs).toBe(10 * 60_000);
    expect(
      estimateWebSessionPromptCache(
        { ...session, latestTokenCountUpdatedAt: new Date(now).toISOString() },
        now
      )?.remainingMs
    ).toBe(CODEX_PROMPT_CACHE_ESTIMATE_MS);
  });
  it.each(['gpt-5.5', 'gpt-5.4', 'gpt-5.1-codex-max', 'custom-model'])(
    'does not invent a retention policy for %s',
    model => {
      expect(estimateWebSessionPromptCache({ ...session, model }, observedAt)).toMatchObject({
        hitPercent: 90,
        windowMs: null,
        remainingMs: null,
        state: 'unknown',
      });
    }
  );
  it('shows hits without inventing a timestamp for older servers', () => {
    for (const latestTokenCountUpdatedAt of [null, 'invalid']) {
      expect(
        estimateWebSessionPromptCache({ ...session, latestTokenCountUpdatedAt }, observedAt)
      ).toMatchObject({ observedAt: null, hitPercent: 90, remainingMs: null });
    }
  });
  it('does not use cumulative usage or pre-compaction observations to start a countdown', () => {
    for (const override of [
      { contextEstimateMode: 'cumulative_total' as const },
      { lastContextCompactionAt: new Date(observedAt + 1000).toISOString() },
    ]) {
      expect(estimateWebSessionPromptCache({ ...session, ...override }, observedAt)).toMatchObject({
        observedAt: null,
        hitPercent: null,
        remainingMs: null,
        state: 'unknown',
      });
    }
  });
  it('does not estimate small inputs and clamps reported hits', () => {
    expect(
      estimateWebSessionPromptCache(
        {
          ...session,
          contextEstimate: { ...session.contextEstimate, inputTokens: 100, cachedInputTokens: 200 },
        },
        observedAt
      )
    ).toMatchObject({ cachedTokens: 100, hitPercent: 100, remainingMs: null });
  });
  it('does not attach Codex estimates to another agent', () => {
    expect(estimateWebSessionPromptCache({ ...session, agent: 'devin' }, observedAt)).toBeNull();
    expect(estimateWebSessionPromptCache(null, observedAt)).toBeNull();
  });
  it('formats the countdown without negative seconds or rounding down early', () => {
    expect(formatPromptCacheCountdown(29 * 60_000 + 1000)).toBe('29:01');
    expect(formatPromptCacheCountdown(1)).toBe('0:01');
    expect(formatPromptCacheCountdown(-1000)).toBe('0:00');
  });
  it('distinguishes a complete loss of reuse from unknown usage or small requests', () => {
    expect(promptCacheRecordEvent(record)).toBe('hit');
    expect(promptCacheRecordEvent({ ...record, cachedInputTokens: 0 })).toBe('miss');
    expect(promptCacheRecordEvent({ ...record, cachedInputTokens: 0 }, record)).toBe('rebuilt');
    for (const cachedInputTokens of [null, undefined, NaN, -1]) {
      expect(promptCacheRecordEvent({ ...record, cachedInputTokens }, record)).toBeNull();
    }
    expect(
      promptCacheRecordEvent({ ...record, inputTokens: 100, cachedInputTokens: 0 }, record)
    ).toBeNull();
    expect(promptCacheRecordEvent({ ...record, agent: 'claude' })).toBeNull();
  });
  it.each([
    [0, 'rebuilt'],
    [1000, 'rebuilt'],
    [1200, 'rebuilt'],
    [9000, 'rebuilt'],
    [9001, 'suspected'],
    [17_999, 'suspected'],
    [18_000, 'hit'],
    [19_000, 'hit'],
    [19_800, 'hit'],
  ])('classifies decreases from 18,000 cached tokens at %i tokens', (cachedInputTokens, event) => {
    expect(promptCacheRecordEvent({ ...record, cachedInputTokens }, record)).toBe(event);
    expect(promptCacheRecordEvent({ ...record, cachedInputTokens })).toBe(
      cachedInputTokens ? 'hit' : 'miss'
    );
  });
  it('retains the inferred rebuild and comparison values across reloads', () => {
    const partial = { ...record, cachedInputTokens: 1000 };
    const restored = JSON.parse(JSON.stringify([partial, record]));
    expect(estimateWebSessionPromptCache(session, observedAt, restored)).toMatchObject({
      event: 'rebuilt',
      hitPercent: 5,
      cachedTokens: 1000,
      previousCachedTokens: 18_000,
      remainingMs: CODEX_PROMPT_CACHE_ESTIMATE_MS,
    });
    expect(promptCacheRecordEvent({ ...record, cachedInputTokens: 0 }, partial)).toBe('rebuilt');
  });
  it('does not infer a rebuild from a lower percentage if absolute reuse did not decrease', () => {
    expect(
      promptCacheRecordEvent({ ...record, inputTokens: 400_000, cachedInputTokens: 20_000 }, record)
    ).toBe('hit');
  });
  it('does not compare different models, agents, missing usage, or invalid previous usage', () => {
    const current = { ...record, cachedInputTokens: 1000 };
    for (const previous of [
      { ...record, model: 'other-model' },
      { ...record, agent: 'claude' },
      { ...record, cachedInputTokens: undefined },
      { ...record, cachedInputTokens: Infinity },
      { ...record, inputTokens: 100 },
    ]) {
      expect(promptCacheRecordEvent(current, previous)).toBe('hit');
    }
    // Invalid telemetry above the input size is clamped before comparing.
    expect(promptCacheRecordEvent(record, { ...record, cachedInputTokens: 100_000 })).toBe(
      'suspected'
    );
  });
  it('uses durable request usage and ignores stale, pre-compaction or other-model records', () => {
    const next = {
      ...record,
      sampleId: '2',
      cachedInputTokens: 0,
      endedAt: new Date(observedAt + 60_000).toISOString(),
    };
    expect(
      estimateWebSessionPromptCache(session, observedAt + 60_000, [next, record])
    ).toMatchObject({
      observationKey: 'run:2',
      event: 'rebuilt',
      cachedTokens: 0,
      remainingMs: CODEX_PROMPT_CACHE_ESTIMATE_MS,
    });
    expect(
      estimateWebSessionPromptCache(session, observedAt, [{ ...record, model: 'other' }])?.event
    ).toBeNull();
    expect(
      estimateWebSessionPromptCache(
        { ...session, lastContextCompactionAt: new Date(observedAt + 120_000).toISOString() },
        observedAt + 120_000,
        [next]
      )?.observedAt
    ).toBeNull();
    expect(
      estimateWebSessionPromptCache(
        { ...session, latestTokenCountUpdatedAt: new Date(observedAt + 120_000).toISOString() },
        observedAt + 120_000,
        [next]
      )?.cachedTokens
    ).toBe(18_000);
  });
});
