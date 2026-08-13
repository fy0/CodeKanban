import type { WebSessionAgent } from '@/types/models';

export type WebSessionNativeIdSource = {
  agent: WebSessionAgent;
  nativeSessionId?: string | null;
};

export function resolveCopyableAgentSessionId(
  session: WebSessionNativeIdSource | null | undefined,
  isDraft: boolean
) {
  if (!session || isDraft) {
    return '';
  }

  return session.nativeSessionId?.trim() ?? '';
}
