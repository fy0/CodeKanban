import type { WebSessionBlock } from '@/stores/webSession';

const PROCESS_RESTART_REASON = 'process_restart';

export function isProcessRestartRecoveryBlock(block: WebSessionBlock) {
  return (
    block.kind === 'system' &&
    block.itemType === 'run_abort' &&
    String(block.payload?.reason ?? '') === PROCESS_RESTART_REASON
  );
}

export function resolveContinuableRecoveryBlockKey(blocks: WebSessionBlock[]) {
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index];
    if (!block || !['run_done', 'run_abort', 'run_fail'].includes(block.itemType)) {
      continue;
    }
    return isProcessRestartRecoveryBlock(block) ? block.key : '';
  }
  return '';
}
