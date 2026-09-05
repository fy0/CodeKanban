import { describe, expect, it } from 'vitest';

import {
  isProcessRestartRecoveryBlock,
  resolveContinuableRecoveryBlockKey,
} from '@/components/web-session/webSessionRecoveryAction';
import type { WebSessionBlock } from '@/stores/webSession';

function block(overrides: Partial<WebSessionBlock> = {}): WebSessionBlock {
  return {
    key: 'block-1',
    id: 'block-1',
    orderIndex: 1,
    kind: 'system',
    itemType: 'run_abort',
    text: '',
    timestamp: 1,
    attachments: [],
    payload: { reason: 'process_restart' },
    ...overrides,
  };
}

describe('web session recovery action', () => {
  it('recognizes the structured process-restart interruption', () => {
    expect(isProcessRestartRecoveryBlock(block())).toBe(true);
    expect(isProcessRestartRecoveryBlock(block({ payload: { reason: 'user_abort' } }))).toBe(false);
    expect(isProcessRestartRecoveryBlock(block({ itemType: 'run_fail' }))).toBe(false);
  });

  it('returns the restart interruption only when it is the latest terminal run block', () => {
    const restart = block({ key: 'restart' });
    expect(resolveContinuableRecoveryBlockKey([restart])).toBe('restart');
    expect(
      resolveContinuableRecoveryBlockKey([
        restart,
        block({ key: 'later-start', itemType: 'run_st' }),
        block({ key: 'later-failure', itemType: 'run_fail', payload: {} }),
      ])
    ).toBe('');
  });

  it('ignores non-terminal history after the restart interruption', () => {
    expect(
      resolveContinuableRecoveryBlockKey([
        block({ key: 'restart' }),
        block({ key: 'note', itemType: 'note', payload: {} }),
      ])
    ).toBe('restart');
  });
});
