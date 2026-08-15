import type { WebSessionSubAgent } from '@/stores/webSession';

export function resolveWebSessionTimelineSubAgent(
  sourceThreadId: string | null | undefined,
  rootThreadId: string | null | undefined,
  agents: ReadonlyMap<string, WebSessionSubAgent>
) {
  const threadId = String(sourceThreadId ?? '').trim();
  const rootId = String(rootThreadId ?? '').trim();
  if (!threadId || (rootId && threadId === rootId)) {
    return null;
  }
  return agents.get(threadId) ?? null;
}
