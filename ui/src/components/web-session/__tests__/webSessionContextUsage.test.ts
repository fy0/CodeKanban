import { describe, expect, it } from 'vitest';

import {
  CODEX_CONTEXT_BASELINE_TOKENS,
  calculateBillableTokenUsage,
  calculateRemainingContext,
  calculateTotalTokenUsage,
  contextUsageBaselineTokens,
  supportsContextUsageIndicator,
} from '@/components/web-session/webSessionContextUsage';

describe('webSessionContextUsage', () => {
  it('supports Pi with a zero context baseline', () => {
    expect(supportsContextUsageIndicator('pi')).toBe(true);
    expect(contextUsageBaselineTokens('pi')).toBe(0);
    expect(contextUsageBaselineTokens('claude')).toBe(0);
    expect(contextUsageBaselineTokens('codex')).toBe(CODEX_CONTEXT_BASELINE_TOKENS);
  });

  it('excludes cached input from cumulative billable usage', () => {
    expect(calculateBillableTokenUsage(4_380_698, 4_032_512, 96_629)).toBe(444_815);
  });

  it('counts cached input once in total usage', () => {
    expect(calculateTotalTokenUsage(4_380_698, 96_629)).toBe(4_477_327);
  });

  it('uses the Codex baseline when estimating remaining context', () => {
    expect(
      calculateRemainingContext({
        compactLimitTokens: 480_000,
        usedTokens: 126_000,
      })
    ).toEqual({
      remainingTokens: 354_000,
      remainingPercent: 76,
    });
  });

  it('uses the full context window when the baseline is zero', () => {
    expect(
      calculateRemainingContext({
        compactLimitTokens: 200_000,
        usedTokens: 150_000,
        baselineTokens: contextUsageBaselineTokens('pi'),
      })
    ).toEqual({
      remainingTokens: 50_000,
      remainingPercent: 25,
    });
  });

  it('does not return negative remaining context near or above the compact limit', () => {
    expect(
      calculateRemainingContext({
        compactLimitTokens: 480_000,
        usedTokens: 500_000,
      })
    ).toEqual({
      remainingTokens: 0,
      remainingPercent: 0,
    });
  });
});
