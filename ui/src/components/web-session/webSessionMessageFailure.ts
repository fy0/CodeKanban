import type { WebSessionBlock } from '@/stores/webSession';

export type WebSessionFailureTrackingBlock = Pick<
  WebSessionBlock,
  'kind' | 'deliveryState' | 'runOutcome' | 'itemType'
>;

export function isFailedWebSessionUserMessage(block: WebSessionFailureTrackingBlock) {
  return block.kind === 'user' && block.deliveryState === 'failed';
}
