import type { WebSessionBlock } from '@/stores/webSession';
import type { WebSessionSummary } from '@/types/models';

export function getPendingWebSessionAsyncUserInputs(
  blocks: readonly WebSessionBlock[],
  session: Pick<WebSessionSummary, 'agent' | 'archivedAt' | 'nativeSessionId'> | null
) {
  if (!session || session.agent !== 'codex' || session.archivedAt) return [];

  return blocks.filter(
    block =>
      block.detail?.type === 'async_user_input_request' &&
      block.detail.action !== 'submitted' &&
      Boolean(block.detail.questions?.length) &&
      (!block.sourceThreadId ||
        !session.nativeSessionId ||
        block.sourceThreadId === session.nativeSessionId)
  );
}
