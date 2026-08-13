export type WebSessionMobilePendingSummaryKind = 'redirect' | 'queue' | 'scheduled';

export type WebSessionMobilePendingSummaryItem = {
  kind: WebSessionMobilePendingSummaryKind;
  count: number;
};

export function buildWebSessionMobilePendingSummary(
  pendingInputs: Array<{ mode: 'redirect' | 'queue' }>,
  scheduledInputs: Array<{ status: string }>
): WebSessionMobilePendingSummaryItem[] {
  const counts: Record<WebSessionMobilePendingSummaryKind, number> = {
    redirect: 0,
    queue: 0,
    scheduled: 0,
  };
  pendingInputs.forEach(item => {
    counts[item.mode] += 1;
  });
  counts.scheduled = scheduledInputs.filter(item => item.status === 'scheduled').length;

  return (['redirect', 'queue', 'scheduled'] as const)
    .map(kind => ({ kind, count: counts[kind] }))
    .filter(item => item.count > 0);
}
