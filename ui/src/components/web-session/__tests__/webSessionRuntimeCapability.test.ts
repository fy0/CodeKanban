import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

describe('webSession runtime capability guards', () => {
  it('blocks send flows when runtime capability checks fail', () => {
    expect(webSessionPanelSource).toContain('!isMessageCapabilityBlocked.value');
    expect(webSessionPanelSource).toContain('ensureMessageCapabilityAvailable');
  });

  it('shows dedicated missing-runtime composer hints', () => {
    expect(webSessionPanelSource).toContain("t('webSession.composerHintCodexMissing')");
    expect(webSessionPanelSource).toContain("t('webSession.composerHintClaudeMissing')");
    expect(webSessionPanelSource).toContain('codexWebSessionUnavailableMessage()');
    expect(webSessionPanelSource).toContain('runtimeSupportsWebSession');
  });

  it('blocks unsupported Codex versions before creating a persisted session', () => {
    expect(webSessionPanelSource).toContain('config.supportsWebSession === true');
    expect(webSessionPanelSource).toContain('if (!(await ensureMessageCapabilityAvailable(agent)))');
    expect(webSessionPanelSource).toContain("option.value === 'codex'");
  });

  it('does not redirect a blocked draft submission into another active session', () => {
    expect(webSessionPanelSource).not.toContain(
      'handleCreateSession()) ?? webSessionStore.getActiveSession(props.projectId)'
    );
  });

  it('guards goal mode actions behind the Codex version capability check', () => {
    expect(webSessionPanelSource).toContain('ensureGoalModeAvailable');
    expect(webSessionPanelSource).toContain('goalModeUnavailableMessage');
    expect(webSessionPanelSource).toContain('isCurrentSessionGoalModeBlocked');
  });

  it('hides the goal card for codex draft sessions and inserts /goal from the button instead', () => {
    expect(webSessionPanelSource).toContain('v-if="isGoalCardVisible"');
    expect(webSessionPanelSource).toContain("showGoalCard.value = false;");
    expect(webSessionPanelSource).toContain('await handleGoalCompose();');
    expect(webSessionPanelSource).toContain("'Insert /goal'");
  });
});
