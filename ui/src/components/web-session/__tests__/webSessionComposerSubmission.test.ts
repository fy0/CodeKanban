import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

function sourceBetween(start: string, end: string) {
  const startIndex = webSessionPanelSource.indexOf(start);
  const endIndex = webSessionPanelSource.indexOf(end, startIndex + start.length);
  expect(startIndex).toBeGreaterThanOrEqual(0);
  expect(endIndex).toBeGreaterThan(startIndex);
  return webSessionPanelSource.slice(startIndex, endIndex);
}

describe('web session composer submission', () => {
  it('locks both running-session actions and shows feedback on the action being submitted', () => {
    const stageGuardSource = sourceBetween(
      'const canStageDuringRun = computed(',
      'const canOpenSendQuickActions = computed('
    );

    expect(stageGuardSource).toContain('!isSubmittingMessage.value');
    expect(webSessionPanelSource).toContain(':loading="isSubmittingRedirectedMessage"');
    expect(webSessionPanelSource).toContain(':loading="isSubmittingQueuedMessage"');
  });

  it('uses the captured session and draft after the user switches tabs', () => {
    const handlerSource = sourceBetween(
      "async function handlePreinput(mode: 'redirect' | 'queue')",
      'async function triggerPrimaryComposerAction()'
    );

    expect(handlerSource).toContain('const session = currentRealSession.value;');
    expect(handlerSource).toContain('const draftSessionId = currentDraftSessionId.value;');
    expect(handlerSource).toContain(
      'isWebSessionSubmitting(submitStateBySessionId.value, draftSessionId)'
    );
    expect(handlerSource).toContain('beginSessionSubmit(draftSessionId');
    expect(handlerSource).toMatch(/webSessionStore\.sendMessage\(\s*session\.id,/);
    expect(handlerSource).toContain(
      'clearComposerDraftAfterSubmit(draftSessionId, submitProjectId);'
    );
    expect(handlerSource).toContain(
      'restoreComposerDraftAfterFailedSubmit(draftSessionId, draft, submitProjectId);'
    );
    expect(handlerSource).toContain('endSessionSubmit(draftSessionId);');
    expect(handlerSource).not.toContain(
      'clearComposerDraftAfterSubmit(currentRealSession.value.id)'
    );
  });

  it('snapshots a regular send before its first asynchronous operation', () => {
    const handlerSource = sourceBetween(
      'async function handleSubmit()',
      'async function handleConfirmScheduledSend()'
    );
    const snapshotIndex = handlerSource.indexOf(
      'const draft = webSessionStore.getDraft(submitProjectId, initialSubmitOwnerId);'
    );
    const firstAwaitIndex = handlerSource.indexOf('await ');
    const clearIndex = handlerSource.indexOf(
      'clearComposerDraftAfterSubmit(draftSessionId, submitProjectId);'
    );

    expect(snapshotIndex).toBeGreaterThanOrEqual(0);
    expect(clearIndex).toBeGreaterThan(snapshotIndex);
    expect(firstAwaitIndex).toBeGreaterThan(clearIndex);
    expect(handlerSource).toContain(
      'isWebSessionSubmitting(submitStateBySessionId.value, initialSubmitOwnerId)'
    );
    expect(handlerSource).toContain(
      'clearComposerDraftAfterSubmit(draftSessionId, submitProjectId);'
    );
  });

  it('routes exact Pi compact commands before session creation and rejects attachments', () => {
    const handlerSource = sourceBetween(
      'async function handleSubmit()',
      'async function handleConfirmScheduledSend()'
    );
    const compactIndex = handlerSource.indexOf("const isPiCompactCommand = submitAgent === 'pi'");
    const createIndex = handlerSource.indexOf('await handleCreateSession(');

    expect(compactIndex).toBeGreaterThanOrEqual(0);
    expect(compactIndex).toBeLessThan(createIndex);
    expect(handlerSource).toContain("draftText.trim() === '/compact'");
    expect(handlerSource).toContain('if (!initialRealSession || isDraftSession(initialSession))');
    expect(handlerSource).toContain(
      "throw new Error(t('webSession.compactAttachmentsUnsupported'))"
    );
    expect(handlerSource).toContain('await webSessionStore.compactSession(initialRealSession.id);');
  });

  it('renders Pi native queued inputs as read-only and keeps local queue controls separate', () => {
    const pendingTemplateSource = sourceBetween(
      '<div v-if="pendingInputs.length > 0" class="pending-inputs">',
      '<div v-if="scheduledInputs.length > 0" class="scheduled-inputs">'
    );
    const pendingHandlerSource = sourceBetween(
      'async function startPendingEdit(item: WebSessionPendingInput)',
      'function scheduledModeLabel('
    );
    const removeHandlerSource = sourceBetween(
      'async function handleRemovePendingInput(pendingId: string)',
      'async function handleRemoveScheduledInput('
    );

    expect(webSessionPanelSource).toContain('const localPendingInputs = computed(() =>');
    expect(webSessionPanelSource).toContain("!item.nativeQueued && item.status !== 'persisting'");
    expect(pendingTemplateSource).toContain('v-if="item.nativeQueued"');
    expect(pendingTemplateSource).toContain("t('webSession.pendingNativeQueued')");
    expect(pendingTemplateSource).toContain('v-else class="pending-input-popover-actions"');
    expect(pendingTemplateSource).toContain(
      'v-if="!item.nativeQueued && item.status !== \'persisting\'"'
    );
    expect(pendingHandlerSource).toContain('item.nativeQueued');
    expect(pendingHandlerSource).toContain('const currentItems = localPendingInputs.value;');
    expect(removeHandlerSource).toContain("item.nativeQueued || item.status === 'persisting'");
  });

  it('does not reactivate a session after creation or catch-up becomes stale', () => {
    const createSource = sourceBetween(
      'async function handleCreateSession(',
      'async function handleStartDraftSession('
    );
    const catchUpSource = sourceBetween(
      'async function refreshWebSessionCatchUp(',
      'function scheduleWebSessionCatchUp('
    );

    expect(createSource).toContain('rememberActive: false');
    expect(createSource).toContain('const shouldActivateCreatedSession');
    expect(createSource).toContain('if (shouldActivateCreatedSession)');
    expect(catchUpSource).toContain('const isCurrentCatchUp');
    expect(catchUpSource).toContain('await webSessionStore.reconcileRecentSessions()');
    expect(catchUpSource).not.toContain('loadSessions(session.projectId, true)');
    expect(catchUpSource).toContain('webSessionStore.catchUpSession');
    expect(catchUpSource).toContain('signal: abortController.signal');
  });

  it('guards delayed send completion side effects by the target session', () => {
    const handlerSource = sourceBetween(
      'async function handleSubmit()',
      'async function handleConfirmScheduledSend()'
    );

    expect(handlerSource).toContain('const isCurrentSubmissionSession');
    expect(handlerSource).toContain(
      'if (prepared.navigateProjectId && isCurrentSubmissionSession)'
    );
    expect(handlerSource).toContain('if (isCurrentSubmissionSession)');
  });
});
