export type WebSessionComposerStartupSource =
  | 'route'
  | 'active-draft'
  | 'remembered-session'
  | 'none';

export type WebSessionComposerStartupTarget = {
  ownerId: string;
  ready: boolean;
  source: WebSessionComposerStartupSource;
};

function normalizeId(value: string | null | undefined) {
  return String(value || '').trim();
}

export function resolveWebSessionComposerStartupTarget(input: {
  routeSessionId?: string | null;
  activeDraftId?: string | null;
  rememberedSessionId?: string | null;
}): WebSessionComposerStartupTarget {
  const routeSessionId = normalizeId(input.routeSessionId);
  if (routeSessionId) {
    return {
      ownerId: routeSessionId,
      ready: false,
      source: 'route',
    };
  }

  const activeDraftId = normalizeId(input.activeDraftId);
  if (activeDraftId) {
    return {
      ownerId: activeDraftId,
      ready: true,
      source: 'active-draft',
    };
  }

  const rememberedSessionId = normalizeId(input.rememberedSessionId);
  if (rememberedSessionId) {
    return {
      ownerId: rememberedSessionId,
      ready: false,
      source: 'remembered-session',
    };
  }

  return {
    ownerId: '',
    ready: false,
    source: 'none',
  };
}

export function hasWebSessionComposerDraftContent(draft: {
  text?: string | null;
  attachments?: unknown[] | null;
}) {
  return Boolean(String(draft.text || '').trim() || (draft.attachments?.length ?? 0) > 0);
}
