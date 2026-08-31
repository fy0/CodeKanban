import { describe, expect, it } from 'vitest';

import {
  hasWebSessionComposerDraftContent,
  resolveWebSessionComposerStartupTarget,
} from '@/components/web-session/webSessionComposerStartup';

describe('web session composer startup target', () => {
  it('gives an explicit route precedence without unlocking it before validation', () => {
    expect(
      resolveWebSessionComposerStartupTarget({
        routeSessionId: 'route-session',
        activeDraftId: 'draft-session',
        rememberedSessionId: 'remembered-session',
      })
    ).toEqual({
      ownerId: 'route-session',
      ready: false,
      source: 'route',
    });
  });

  it('unlocks a restored local draft immediately when no route overrides it', () => {
    expect(
      resolveWebSessionComposerStartupTarget({
        activeDraftId: 'draft-session',
        rememberedSessionId: 'remembered-session',
      })
    ).toEqual({
      ownerId: 'draft-session',
      ready: true,
      source: 'active-draft',
    });
  });

  it('inherits the remembered real session but waits for server validation', () => {
    expect(
      resolveWebSessionComposerStartupTarget({ rememberedSessionId: 'remembered-session' })
    ).toEqual({
      ownerId: 'remembered-session',
      ready: false,
      source: 'remembered-session',
    });
  });

  it('detects both text and attachment-only drafts', () => {
    expect(hasWebSessionComposerDraftContent({ text: '  ', attachments: [] })).toBe(false);
    expect(hasWebSessionComposerDraftContent({ text: 'pending', attachments: [] })).toBe(true);
    expect(hasWebSessionComposerDraftContent({ text: '', attachments: [{}] })).toBe(true);
  });
});
