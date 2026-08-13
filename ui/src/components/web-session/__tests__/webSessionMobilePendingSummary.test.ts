import { describe, expect, it } from 'vitest';

import { buildWebSessionMobilePendingSummary } from '@/components/web-session/webSessionMobilePendingSummary';

describe('webSessionMobilePendingSummary', () => {
  it('shows a delayed summary when there are no next-step or queued inputs', () => {
    expect(
      buildWebSessionMobilePendingSummary([], [
        { status: 'scheduled' },
        { status: 'failed' },
        { status: 'expired' },
      ])
    ).toEqual([{ kind: 'scheduled', count: 1 }]);
  });

  it('counts each active pending category and omits empty categories', () => {
    expect(
      buildWebSessionMobilePendingSummary(
        [{ mode: 'redirect' }, { mode: 'queue' }, { mode: 'queue' }],
        []
      )
    ).toEqual([
      { kind: 'redirect', count: 1 },
      { kind: 'queue', count: 2 },
    ]);
  });
});
