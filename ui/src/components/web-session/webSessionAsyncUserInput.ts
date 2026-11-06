import type { WebSessionBlock } from '@/stores/webSession';
import type { WebSessionSummary } from '@/types/models';

export function getPendingWebSessionAsyncUserInputs(
  blocks: readonly WebSessionBlock[],
  session: Pick<WebSessionSummary, 'agent' | 'archivedAt' | 'nativeSessionId'> | null
) {
  if (!session || session.agent !== 'codex' || session.archivedAt) return [];

  const pending: WebSessionBlock[] = [];
  for (const block of blocks) {
    if (
      block.sourceThreadId &&
      session.nativeSessionId &&
      block.sourceThreadId !== session.nativeSessionId
    ) {
      continue;
    }
    if (
      block.kind === 'user' &&
      block.deliveryState !== 'sending' &&
      block.deliveryState !== 'failed'
    ) {
      // A normal reply moves the conversation on too. Older cards remain in
      // history, but should no longer keep the unanswered reminder visible.
      pending.length = 0;
    } else if (
      block.detail?.type === 'async_user_input_request' &&
      block.detail.action !== 'submitted' &&
      block.detail.questions?.length
    ) {
      pending.push(block);
    }
  }
  return pending;
}
