import { describe, expect, it } from 'vitest';

import { shouldShowAutoRetryRateLimitNotice } from '@/components/web-session/webSessionAutoRetryNotice';

function makeSession(
  overrides: Partial<{
    status: 'idle' | 'running' | 'waiting_approval' | 'done' | 'err' | 'aborting';
    autoRetryEnabled: boolean;
    autoRetryScope: 'network_only' | 'network_and_rate_limit' | 'all_failures';
  }> = {}
) {
  return {
    status: 'err' as const,
    autoRetryEnabled: true,
    autoRetryScope: 'network_only' as const,
    ...overrides,
  };
}

describe('webSessionAutoRetryNotice', () => {
  it('shows a notice for 429/rate limit failures when auto retry uses the default scope', () => {
    expect(
      shouldShowAutoRetryRateLimitNotice(
        makeSession(),
        'exceeded retry limit, last status: 429 Too Many Requests'
      )
    ).toBe(true);
    expect(
      shouldShowAutoRetryRateLimitNotice(makeSession(), 'rate limit hit while sending request')
    ).toBe(true);
  });

  it('hides the notice when the session scope already includes rate limits', () => {
    expect(
      shouldShowAutoRetryRateLimitNotice(
        makeSession({ autoRetryScope: 'network_and_rate_limit' }),
        '429 Too Many Requests'
      )
    ).toBe(false);
    expect(
      shouldShowAutoRetryRateLimitNotice(
        makeSession({ autoRetryScope: 'all_failures' }),
        '429 Too Many Requests'
      )
    ).toBe(false);
  });

  it('hides the notice for non-error sessions or non-rate-limit errors', () => {
    expect(
      shouldShowAutoRetryRateLimitNotice(
        makeSession({ status: 'running' }),
        '429 Too Many Requests'
      )
    ).toBe(false);
    expect(
      shouldShowAutoRetryRateLimitNotice(makeSession({ autoRetryEnabled: false }), '429')
    ).toBe(false);
    expect(
      shouldShowAutoRetryRateLimitNotice(makeSession(), 'unexpected status 502 Bad Gateway')
    ).toBe(false);
  });
});
