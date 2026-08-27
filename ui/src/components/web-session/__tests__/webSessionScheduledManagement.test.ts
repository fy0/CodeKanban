import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const scheduledSendDialogPath = fileURLToPath(
  new URL('../WebSessionScheduledSendDialog.vue', import.meta.url)
);
const scheduledSendDialogSource = readFileSync(scheduledSendDialogPath, 'utf8');

describe('webSession scheduled input management', () => {
  it('opens an accessible details popover with contextual actions', () => {
    expect(webSessionPanelSource).toContain('class="scheduled-input-trigger"');
    expect(webSessionPanelSource).toContain(
      ':aria-expanded="activeScheduledInputPopoverId === item.id"'
    );
    expect(webSessionPanelSource).toContain('scheduledImmediateActionLabel(item)');
    expect(webSessionPanelSource).toContain('openScheduledInputEditDialog(item)');
    expect(webSessionPanelSource).toContain('scheduledFailureReason(item)');
  });

  it('reuses the scheduling dialog for message and plan edits', () => {
    expect(scheduledSendDialogSource).toContain("'edit_message'");
    expect(scheduledSendDialogSource).toContain("'edit_plan'");
    expect(webSessionPanelSource).toContain('webSessionStore.updateScheduledInput');
    expect(scheduledSendDialogSource).toContain('scheduledAttachmentsPreserved');
  });

  it('only confirms immediate delivery when interrupting an active message run', () => {
    expect(webSessionPanelSource).toContain("item.action === 'message'");
    expect(webSessionPanelSource).toContain("item.mode === 'interrupt'");
    expect(webSessionPanelSource).toContain('isRunActive.value');
    expect(webSessionPanelSource).toContain('scheduledInterruptNowTitle');
    expect(webSessionPanelSource).toContain('webSessionStore.dispatchScheduledInputNow');
  });
});
