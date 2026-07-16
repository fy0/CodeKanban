export const CODEX_CONTEXT_BASELINE_TOKENS = 12000;

export function calculateBillableTokenUsage(
  inputTokens: number,
  cachedInputTokens: number,
  outputTokens: number
) {
  return Math.max(0, inputTokens - cachedInputTokens) + Math.max(0, outputTokens);
}

export function calculateCodexRemainingContext({
  compactLimitTokens,
  usedTokens,
  baselineTokens = CODEX_CONTEXT_BASELINE_TOKENS,
}: {
  compactLimitTokens: number;
  usedTokens: number;
  baselineTokens?: number;
}) {
  const effectiveCompactLimitTokens = Math.max(1, compactLimitTokens - baselineTokens);
  const effectiveUsedTokens = Math.max(0, usedTokens - baselineTokens);
  const remainingTokens = Math.max(0, effectiveCompactLimitTokens - effectiveUsedTokens);
  const remainingPercent =
    effectiveCompactLimitTokens > 0
      ? Math.round((remainingTokens / effectiveCompactLimitTokens) * 100)
      : 0;

  return {
    remainingTokens,
    remainingPercent,
  };
}
