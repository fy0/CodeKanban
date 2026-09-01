import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import {
  isFailedWebSessionUserMessage,
  type WebSessionFailureTrackingBlock,
} from '@/components/web-session/webSessionMessageFailure';
import enUS from '@/i18n/locales/en-US';
import zhCN from '@/i18n/locales/zh-CN';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

function sourceBetween(start: string, end: string) {
  const startIndex = webSessionPanelSource.indexOf(start);
  const endIndex = webSessionPanelSource.indexOf(end, startIndex + start.length);
  expect(startIndex).toBeGreaterThanOrEqual(0);
  expect(endIndex).toBeGreaterThan(startIndex);
  return webSessionPanelSource.slice(startIndex, endIndex);
}

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

  it('renders the delivery indicator and retries in the original local bubble', () => {
    expect(webSessionPanelSource).toContain(
      'v-if="shouldShowTimelineUserMessageDeliveryIndicator(item)"'
    );
    expect(webSessionPanelSource).toContain('@click.stop="handleRetryTimelineUserMessage(item)"');
    expect(webSessionPanelSource).toContain(
      '<RefreshOutline v-if="isRetryingTimelineUserMessage(item)" />'
    );
    expect(webSessionPanelSource).toContain('webSessionStore.getTimelineBlocks(');
    expect(webSessionPanelSource).not.toContain('collectFailedWebSessionRunIds');

    const handlerSource = sourceBetween(
      'async function handleRetryTimelineUserMessage(item: WebSessionBlock)',
      'async function continueErroredSession(session: WebSessionSummary)'
    );
    expect(handlerSource).toContain(
      'const attachmentIds = item.attachments.map(attachment => attachment.id).filter(Boolean);'
    );
    expect(handlerSource).toContain('outgoingMessageId: item.id');
    expect(handlerSource).toContain('attachments: item.attachments');
    expect(handlerSource).toContain('freshContext: item.freshContext');
    expect(handlerSource).toContain('beginSessionSubmit(sourceSessionId, submitKind);');
    expect(handlerSource).toContain('ensureSendConflictConfirmed(');
    expect(handlerSource).toContain("message.success(t('webSession.userMessageRetrySuccess'));");
  });

  it('keeps a delivery-failed message in the timeline instead of restoring the draft', () => {
    const handlerSource = sourceBetween(
      'async function handleSubmit()',
      'async function handleConfirmScheduledSend()'
    );
    expect(handlerSource).toContain(
      'messageRetainedForRetry = isWebSessionMessageDeliveryError(error);'
    );
    expect(handlerSource).toContain('if (!submissionSucceeded && !messageRetainedForRetry)');
    expect(handlerSource).toContain('{ attachments }');
    const stageIndex = handlerSource.indexOf('stageComposerMessage(initialRealSession);');
    const firstPreparationAwaitIndex = handlerSource.indexOf('await ensurePiProjectTrust()');
    expect(stageIndex).toBeGreaterThanOrEqual(0);
    expect(firstPreparationAwaitIndex).toBeGreaterThan(stageIndex);
    expect(handlerSource).toContain(
      'webSessionStore.discardOutgoingMessage(outgoingMessageSessionId, outgoingMessageId);'
    );
  });
});
