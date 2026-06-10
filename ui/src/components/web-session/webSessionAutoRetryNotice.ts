import type { WebSessionSummary } from '@/types/models';

const AUTO_RETRY_RATE_LIMIT_PATTERN = /\b429\b|too many requests|rate limit/i;

export function shouldShowAutoRetryRateLimitNotice(
  session:
    | Pick<WebSessionSummary, 'status' | 'autoRetryEnabled' | 'autoRetryScope'>
    | null
    | undefined,
  errorMessage?: string | null
) {
  if (!session || session.status !== 'err' || session.autoRetryEnabled !== true) {
    return false;
  }
  if (session.autoRetryScope !== 'network_only') {
    return false;
  }
  return AUTO_RETRY_RATE_LIMIT_PATTERN.test(String(errorMessage ?? '').trim());
}
