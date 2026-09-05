import { describe, expect, it } from 'vitest';

import { pickLatestWebSessionPendingInputEditDraft } from '@/components/web-session/webSessionPendingInputEdit';

describe('webSessionPendingInputEdit', () => {
  it('selects the most recently edited existing paused input', () => {
    const latest = pickLatestWebSessionPendingInputEditDraft(
      {
        stale: { text: 'stale', updatedAt: 300 },
        'pending-1': { text: 'older', updatedAt: 100 },
        'pending-2': { text: 'latest', updatedAt: 200 },
      },
      ['pending-1', 'pending-2']
    );

    expect(latest).toEqual({
      pendingId: 'pending-2',
      draft: { text: 'latest', updatedAt: 200 },
    });
  });

  it('uses a deterministic id tie-breaker and returns null without matching inputs', () => {
    expect(
      pickLatestWebSessionPendingInputEditDraft(
        {
          'pending-b': { text: 'B', updatedAt: 100 },
          'pending-a': { text: 'A', updatedAt: 100 },
        },
        ['pending-a', 'pending-b']
      )
    ).toEqual({
      pendingId: 'pending-b',
      draft: { text: 'B', updatedAt: 100 },
    });
    expect(
      pickLatestWebSessionPendingInputEditDraft({ 'pending-1': { text: 'text', updatedAt: 100 } }, [
        'pending-2',
      ])
    ).toBeNull();
  });
});
