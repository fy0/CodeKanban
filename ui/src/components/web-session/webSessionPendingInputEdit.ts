import type { WebSessionPendingInputEditDraft } from '@/stores/webSession';

export function pickLatestWebSessionPendingInputEditDraft(
  drafts: Record<string, WebSessionPendingInputEditDraft>,
  pendingIds: readonly string[]
): { pendingId: string; draft: WebSessionPendingInputEditDraft } | null {
  const validIds = new Set(pendingIds.map(id => String(id ?? '').trim()).filter(Boolean));
  let latest: { pendingId: string; draft: WebSessionPendingInputEditDraft } | null = null;

  for (const [pendingId, draft] of Object.entries(drafts)) {
    if (!validIds.has(pendingId)) {
      continue;
    }
    if (
      !latest ||
      draft.updatedAt > latest.draft.updatedAt ||
      (draft.updatedAt === latest.draft.updatedAt && pendingId > latest.pendingId)
    ) {
      latest = { pendingId, draft: { ...draft } };
    }
  }

  return latest;
}
