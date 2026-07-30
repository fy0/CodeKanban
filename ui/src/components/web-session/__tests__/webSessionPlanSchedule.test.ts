import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const webSessionSidebarRowPath = fileURLToPath(
  new URL('../WebSessionSidebarRow.vue', import.meta.url)
);
const webSessionSidebarRowSource = readFileSync(webSessionSidebarRowPath, 'utf8');

describe('webSession plan scheduling', () => {
  it('keeps immediate implementation on the primary button and only delays in the menu', () => {
    expect(webSessionPanelSource).toContain('class="plan-tool-action-split"');
    expect(webSessionPanelSource).toContain(':aria-expanded="showPlanQuickActions"');
    expect(webSessionPanelSource).toContain('@click="handlePlanCardImplement"');
    expect(webSessionPanelSource).not.toContain("key: 'implement'");
    expect(webSessionPanelSource).toContain("key: 'schedule-plan'");
    expect(webSessionPanelSource).toContain("t('webSession.planActionSchedule')");
  });

  it('reuses the scheduling dialog without message delivery modes', () => {
    expect(webSessionPanelSource).toContain("scheduledSendPurpose === 'message'");
    expect(webSessionPanelSource).toContain("openScheduledSendDialog('execute_plan', target)");
    expect(webSessionPanelSource).toContain('scheduledDialogConfirmLabel');
    expect(webSessionPanelSource).toContain('value="when_idle"');
    expect(webSessionPanelSource).toContain("{ scheduleKind: 'when_idle' }");
  });

  it('offers the shared idle condition and its definition for delayed messages', () => {
    expect(webSessionPanelSource).toContain('name="scheduled-schedule-kind"');
    expect(webSessionPanelSource).toContain('v-if="scheduledScheduleKind === \'when_idle\'"');
    expect(webSessionPanelSource).toContain("t('webSession.scheduleWhenIdleDescription')");
    expect(webSessionPanelSource).toContain('webSessionStore.scheduleMessage');
    expect(webSessionPanelSource).toContain('const scheduleKind = scheduledScheduleKind.value;');
  });

  it('binds a scheduled action to the current plan and hides duplicate actions', () => {
    expect(webSessionPanelSource).toContain('const planItemId = latestPlanItemId.value;');
    expect(webSessionPanelSource).toContain(
      '!activeScheduledPlanTargetIds.value.has(latestPlanItemId.value)'
    );
    expect(webSessionPanelSource).toContain('webSessionStore.schedulePlanExecution');
  });

  it('uses the amber scheduled state for every plan flag surface', () => {
    expect(webSessionPanelSource.match(/hasScheduledPlanExecution/g)?.length).toBeGreaterThan(4);
    expect(webSessionPanelSource).toContain(
      'var(--web-session-scheduled-plan-flag-color, #d97706)'
    );
    expect(webSessionPanelSource).toContain('.tab-workflow-plan-flag.is-scheduled');
    expect(webSessionPanelSource).toContain('.mobile-tab-trigger-plan-badge.is-scheduled');
    expect(webSessionPanelSource).toContain('.mobile-session-drawer-plan-badge.is-scheduled');
    expect(webSessionSidebarRowSource).toContain("'is-scheduled': row.hasScheduledPlanExecution");
    expect(webSessionSidebarRowSource).toContain(
      'var(--web-session-scheduled-plan-flag-color, #d97706)'
    );
  });
});
