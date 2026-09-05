import { describe, expect, it } from 'vitest';

import {
  isFailedWebSessionUserMessage,
  type WebSessionFailureTrackingBlock,
} from '@/components/web-session/webSessionMessageFailure';
import enUS from '@/i18n/locales/en-US';
import zhCN from '@/i18n/locales/zh-CN';

function block(
  overrides: Partial<WebSessionFailureTrackingBlock> = {}
): WebSessionFailureTrackingBlock {
  return {
    kind: 'system',
    deliveryState: undefined,
    runOutcome: null,
    itemType: '',
    ...overrides,
  };
}

describe('web session message failure', () => {
  it('marks only a client-side user message whose delivery failed', () => {
    expect(isFailedWebSessionUserMessage(block({ kind: 'user', deliveryState: 'failed' }))).toBe(
      true
    );
    expect(isFailedWebSessionUserMessage(block({ kind: 'user', deliveryState: 'sending' }))).toBe(
      false
    );
    expect(isFailedWebSessionUserMessage(block({ kind: 'user', deliveryState: 'accepted' }))).toBe(
      false
    );
  });

  it.each([
    { itemType: 'run_fail', runOutcome: null, label: 'run_fail such as a 429 response' },
    { itemType: 'run_done', runOutcome: 'failed', label: 'failed Agent run' },
    { itemType: 'run_done', runOutcome: 'timeout', label: 'timed out Agent run' },
  ])('does not mark an accepted user message for $label', ({ itemType, runOutcome }) => {
    expect(isFailedWebSessionUserMessage(block({ kind: 'user', itemType, runOutcome }))).toBe(
      false
    );
  });

  it('does not mark assistant or system blocks', () => {
    expect(
      isFailedWebSessionUserMessage(block({ kind: 'assistant', deliveryState: 'failed' }))
    ).toBe(false);
    expect(isFailedWebSessionUserMessage(block({ kind: 'system', deliveryState: 'failed' }))).toBe(
      false
    );
  });

  it('defines retry copy in the webSession locale namespace', () => {
    expect(zhCN.webSession.userMessageFailed).toBe('消息发送失败，点击重新发送');
    expect(enUS.webSession.userMessageFailed).toBe('Message failed. Click to resend.');
    expect('userMessageFailed' in zhCN.terminal).toBe(false);
    expect('userMessageFailed' in enUS.terminal).toBe(false);
  });
});
