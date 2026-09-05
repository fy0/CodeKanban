import { describe, expect, it } from 'vitest';

import type { WebSessionBlock } from '@/stores/webSession';
import {
  findLatestSubAgentActivityBlock,
  isTransportRetryActivityText,
  subAgentActivitySummary,
} from '@/components/web-session/webSessionSubAgentActivity';

function block(input: Partial<WebSessionBlock> & Pick<WebSessionBlock, 'id' | 'orderIndex'>) {
  return {
    key: `${input.id}:${input.orderIndex}`,
    sourceThreadId: 'thread-child',
    sourceTurnId: null,
    sourceItemId: null,
    runId: null,
    runDurationMs: null,
    runOutcome: null,
    kind: 'system',
    itemType: 'note',
    text: '',
    timestamp: input.orderIndex,
    observedAt: input.orderIndex,
    attachments: [],
    done: true,
    payload: {},
    ...input,
  } satisfies WebSessionBlock;
}

describe('webSessionSubAgentActivity', () => {
  it('keeps the latest real activity when a transport retry follows it', () => {
    const fileChange = block({
      id: 'file-change',
      orderIndex: 1,
      kind: 'tool',
      itemType: 'file_change',
      tool: {
        id: 'tool-file-change',
        name: 'File change',
        kind: 'file_change',
        status: 'done',
        input: { changes: [{ path: 'ui/src/App.vue' }] },
      },
    });
    const retry = block({
      id: 'retry',
      orderIndex: 2,
      text: 'Reconnecting... 1/5',
      payload: { code: 'transport_retrying' },
    });

    const latest = findLatestSubAgentActivityBlock([fileChange, retry], 'thread-child');

    expect(latest).toBe(fileChange);
    expect(subAgentActivitySummary(latest!)).toBe('ui/src/App.vue');
    expect(isTransportRetryActivityText('Reconnecting... 1/5')).toBe(true);
  });

  it('only searches activity from the requested sub-agent', () => {
    const child = block({ id: 'child', orderIndex: 1, text: 'child activity' });
    const other = block({
      id: 'other',
      orderIndex: 2,
      sourceThreadId: 'thread-other',
      text: 'other activity',
    });

    expect(findLatestSubAgentActivityBlock([child, other], 'thread-child')).toBe(child);
  });
});
