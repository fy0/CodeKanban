import { describe, expect, it } from 'vitest';

import { mergeWebSessionDraftForRestore, type WebSessionDraftState } from '@/stores/webSession';

function draft(overrides: Partial<WebSessionDraftState> = {}): WebSessionDraftState {
  return {
    text: '',
    attachments: [],
    updatedAt: 0,
    ...overrides,
  };
}

function attachment(id: string) {
  return {
    id,
    name: `${id}.png`,
    mime: 'image/png',
    size: 100,
    path: `/attachments/${id}`,
    createdAt: '2026-08-06T00:00:00.000Z',
  };
}

describe('web session draft restore', () => {
  it('restores the submitted draft when no newer input exists', () => {
    const submitted = draft({
      text: 'original message',
      attachments: [attachment('attachment-1')],
      updatedAt: 10,
    });

    expect(mergeWebSessionDraftForRestore(draft(), submitted, 20)).toEqual({
      text: 'original message',
      attachments: [attachment('attachment-1')],
      updatedAt: 20,
    });
  });

  it('keeps newer text and attachments when a failed submission is restored', () => {
    const submitted = draft({
      text: 'original message',
      attachments: [attachment('attachment-1')],
    });
    const current = draft({
      text: 'next message',
      attachments: [attachment('attachment-2')],
    });

    expect(mergeWebSessionDraftForRestore(current, submitted, 30)).toEqual({
      text: 'original message\n\nnext message',
      attachments: [attachment('attachment-1'), attachment('attachment-2')],
      updatedAt: 30,
    });
  });

  it('does not duplicate stale text or attachments', () => {
    const sharedAttachment = attachment('attachment-1');
    const submitted = draft({
      text: 'original message',
      attachments: [sharedAttachment],
    });
    const current = draft({
      text: 'original message',
      attachments: [sharedAttachment],
    });

    expect(mergeWebSessionDraftForRestore(current, submitted, 40)).toEqual({
      text: 'original message',
      attachments: [sharedAttachment],
      updatedAt: 40,
    });
  });
});
