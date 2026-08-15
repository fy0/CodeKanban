import { describe, expect, it } from 'vitest';

import {
  buildWebSessionPendingInputEditorMemoDeps,
  pickLatestWebSessionPendingInputEditDraft,
} from '@/components/web-session/webSessionPendingInputEdit';

describe('webSessionPendingInputEdit', () => {
  it('keeps editor memo dependencies stable across unrelated refreshes', () => {
    const first = buildWebSessionPendingInputEditorMemoDeps({
      pendingId: 'pending-1',
      editingId: 'pending-1',
      text: 'Keep this text',
      actionId: '',
    });
    const second = buildWebSessionPendingInputEditorMemoDeps({
      pendingId: 'pending-1',
      editingId: 'pending-1',
      text: 'Keep this text',
      actionId: '',
    });

    expect(second).toEqual(first);
  });

  it('invalidates memo dependencies when editor ownership or text changes', () => {
    const input = {
      pendingId: 'pending-1',
      editingId: 'pending-1',
      text: 'Keep this text',
      actionId: '',
    };
    const base = buildWebSessionPendingInputEditorMemoDeps(input);

    expect(
      buildWebSessionPendingInputEditorMemoDeps({ ...input, text: 'Changed text' })
    ).not.toEqual(base);
    expect(
      buildWebSessionPendingInputEditorMemoDeps({ ...input, actionId: 'pending-1' })
    ).not.toEqual(base);
    expect(
      buildWebSessionPendingInputEditorMemoDeps({ ...input, editingId: 'pending-2' })
    ).not.toEqual(base);
  });

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
