import { describe, expect, it, vi } from 'vitest';

import { shouldLoadWebSessionSnapshotOnActivation } from '@/components/web-session/webSessionPanelSession';

describe('webSessionPanelSession activation', () => {
  it('catches up a switched session even when its cached revision appears current', () => {
    const isSnapshotCurrent = vi.fn().mockReturnValue(true);

    expect(
      shouldLoadWebSessionSnapshotOnActivation(
        { id: 'session-background', revision: '8' },
        isSnapshotCurrent,
        'ready',
        true
      )
    ).toBe(true);
  });

  it('reuses a complete snapshot when its revision matches the session summary', () => {
    const isSnapshotCurrent = vi.fn().mockReturnValue(true);

    expect(
      shouldLoadWebSessionSnapshotOnActivation(
        { id: 'session-1', revision: '8' },
        isSnapshotCurrent
      )
    ).toBe(false);
    expect(isSnapshotCurrent).toHaveBeenCalledWith('session-1', '8');
  });

  it('requests conditional hydration when the summary is newer or no snapshot exists', () => {
    const isSnapshotCurrent = vi.fn().mockReturnValue(false);

    expect(
      shouldLoadWebSessionSnapshotOnActivation(
        { id: 'session-1', revision: '9' },
        isSnapshotCurrent
      )
    ).toBe(true);
    expect(shouldLoadWebSessionSnapshotOnActivation(null, isSnapshotCurrent)).toBe(true);
  });

  it('loads an uninitialized timeline even when its revision clock is current', () => {
    const isSnapshotCurrent = vi.fn().mockReturnValue(true);

    expect(
      shouldLoadWebSessionSnapshotOnActivation(
        { id: 'session-1', revision: '9' },
        isSnapshotCurrent,
        'uninitialized'
      )
    ).toBe(true);
    expect(
      shouldLoadWebSessionSnapshotOnActivation(
        { id: 'session-1', revision: '9' },
        isSnapshotCurrent,
        'ready'
      )
    ).toBe(false);
  });
});
