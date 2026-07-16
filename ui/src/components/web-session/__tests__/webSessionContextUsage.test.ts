import { describe, expect, it } from 'vitest';

import {
  calculateBillableTokenUsage,
  calculateCodexRemainingContext,
} from '@/components/web-session/webSessionContextUsage';

describe('webSessionContextUsage', () => {
  it('excludes cached input from cumulative billable usage', () => {
    expect(calculateBillableTokenUsage(4_380_698, 4_032_512, 96_629)).toBe(444_815);
  });

  it('uses the Codex baseline when estimating remaining context', () => {
    expect(
      calculateCodexRemainingContext({
        compactLimitTokens: 480_000,
        usedTokens: 126_000,
      })
    ).toEqual({
      remainingTokens: 354_000,
      remainingPercent: 76,
    });
  });

  it('does not return negative remaining context near or above the compact limit', () => {
    expect(
      calculateCodexRemainingContext({
        compactLimitTokens: 480_000,
        usedTokens: 500_000,
      })
    ).toEqual({
      remainingTokens: 0,
      remainingPercent: 0,
    });
  });
});
