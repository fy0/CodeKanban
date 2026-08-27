import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const mobileSessionDrawerPath = fileURLToPath(
  new URL('../WebSessionMobileSessionDrawer.vue', import.meta.url)
);
const mobileSessionDrawerSource = readFileSync(mobileSessionDrawerPath, 'utf8');
const mobileSessionDrawerStylePath = fileURLToPath(
  new URL('../styles/webSessionMobileSessionDrawer.css', import.meta.url)
);
const mobileSessionDrawerStyleSource = readFileSync(mobileSessionDrawerStylePath, 'utf8');
const webSessionPanelLayoutStylePath = fileURLToPath(
  new URL('../styles/webSessionPanelLayout.css', import.meta.url)
);
const webSessionPanelLayoutStyleSource = readFileSync(webSessionPanelLayoutStylePath, 'utf8');
const scheduledSendDialogPath = fileURLToPath(
  new URL('../WebSessionScheduledSendDialog.vue', import.meta.url)
);
const scheduledSendDialogSource = readFileSync(scheduledSendDialogPath, 'utf8');
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
    expect(scheduledSendDialogSource).toContain(
      "purpose === 'message' || purpose === 'edit_message'"
    );
    expect(webSessionPanelSource).toContain("openScheduledSendDialog('execute_plan', target)");
    expect(webSessionPanelSource).toContain('scheduledDialogConfirmLabel');
    expect(scheduledSendDialogSource).toContain('value="when_idle"');
    expect(webSessionPanelSource).toContain("{ scheduleKind: 'when_idle' }");
  });

  it('offers the shared idle condition and its definition for delayed messages', () => {
    expect(scheduledSendDialogSource).toContain('name="scheduled-schedule-kind"');
    expect(scheduledSendDialogSource).toContain('v-if="scheduleKind === \'when_idle\'"');
    expect(scheduledSendDialogSource).toContain("t('webSession.scheduleWhenIdleDescription')");
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

  it('uses a dedicated clock marker on sessions with scheduled input', () => {
    expect(webSessionPanelSource.match(/hasScheduledPlanExecution/g)?.length).toBeGreaterThan(4);
    expect(webSessionPanelSource).toContain('function shouldHighlightScheduledInputSession');
    expect(webSessionPanelSource).toContain('webSessionStore.getScheduledInputs(session.id)');
    expect(webSessionPanelSource).toContain("item.status === 'scheduled'");
    expect(webSessionPanelSource).not.toContain(
      '!webSessionStore.getLiveState(session.id).running'
    );
    expect(`${webSessionPanelLayoutStyleSource}\n${mobileSessionDrawerStyleSource}`).toContain(
      'var(--web-session-scheduled-plan-flag-color, #d97706)'
    );
    expect(webSessionPanelLayoutStyleSource).toContain('.tab-workflow-plan-flag.is-scheduled');
    expect(webSessionPanelLayoutStyleSource).toContain(
      '.mobile-tab-trigger-plan-badge.is-scheduled'
    );
    expect(mobileSessionDrawerStyleSource).toContain(
      '.mobile-session-drawer-plan-badge.is-scheduled'
    );
    expect(mobileSessionDrawerSource).toContain('class="mobile-session-drawer-scheduled-marker"');
    expect(webSessionPanelSource).not.toContain('is-plan-execution');
    expect(webSessionSidebarRowSource).toContain("'is-scheduled': row.hasScheduledPlanExecution");
    expect(webSessionSidebarRowSource).toContain(
      'var(--web-session-scheduled-plan-flag-color, #d97706)'
    );
    expect(webSessionSidebarRowSource).toContain('v-if="row.hasScheduledInput"');
    expect(webSessionSidebarRowSource).toContain('.session-sidebar-scheduled-marker');
  });
});
