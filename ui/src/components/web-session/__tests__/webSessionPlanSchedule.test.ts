import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

describe('webSession plan scheduling', () => {
  it('uses an accessible split action for immediate and delayed implementation', () => {
    expect(webSessionPanelSource).toContain('class="plan-tool-action-split"');
    expect(webSessionPanelSource).toContain(':aria-expanded="showPlanQuickActions"');
    expect(webSessionPanelSource).toContain("key: 'schedule-plan'");
    expect(webSessionPanelSource).toContain("t('webSession.planActionSchedule')");
  });

  it('reuses the scheduling dialog without message delivery modes', () => {
    expect(webSessionPanelSource).toContain("scheduledSendPurpose === 'message'");
    expect(webSessionPanelSource).toContain("openScheduledSendDialog('execute_plan', target)");
    expect(webSessionPanelSource).toContain('scheduledDialogConfirmLabel');
  });

  it('binds a scheduled action to the current plan and hides duplicate actions', () => {
    expect(webSessionPanelSource).toContain('const planItemId = latestPlanItemId.value;');
    expect(webSessionPanelSource).toContain(
      '!activeScheduledPlanTargetIds.value.has(latestPlanItemId.value)'
    );
    expect(webSessionPanelSource).toContain('webSessionStore.schedulePlanExecution');
  });
});
