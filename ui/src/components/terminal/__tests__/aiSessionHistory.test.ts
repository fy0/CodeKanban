import { describe, expect, it } from 'vitest';

import { isProjectAISessionScanning, resolvePreferredAISessionType } from '../aiSessionHistory';

describe('AI session history provider selection', () => {
  it('selects Pi when it is the only provider with history', () => {
    expect(
      resolvePreferredAISessionType({
        hasClaudeCode: false,
        hasCodex: false,
        hasPi: true,
      })
    ).toBe('pi');
  });

  it('preserves the existing Claude then Codex preference order', () => {
    expect(
      resolvePreferredAISessionType({
        hasClaudeCode: true,
        hasCodex: true,
        hasPi: true,
      })
    ).toBe('claude_code');
    expect(
      resolvePreferredAISessionType({
        hasClaudeCode: false,
        hasCodex: true,
        hasPi: true,
      })
    ).toBe('codex');
  });

  it('keeps polling while Pi history discovery is incomplete', () => {
    expect(
      isProjectAISessionScanning({
        claudeScanPhase: 'complete',
        codexScanPhase: 'complete',
        piScanPhase: 'extended',
      })
    ).toBe(true);
    expect(
      isProjectAISessionScanning({
        claudeScanPhase: 'complete',
        codexScanPhase: 'complete',
        piScanPhase: 'complete',
      })
    ).toBe(false);
  });
});
