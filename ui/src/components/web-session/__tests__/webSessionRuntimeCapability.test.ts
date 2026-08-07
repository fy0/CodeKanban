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
    expect(webSessionPanelSource).toContain('codexCompatibilityModeMessage()');
    expect(webSessionPanelSource).toContain('isCodexCompatibilityMode');
  });

  it('keeps pre-V2 Codex versions usable in compatibility mode', () => {
    expect(webSessionPanelSource).toContain('runtimeSupportsMultiAgentV2');
    expect(webSessionPanelSource).toContain(
      'if (!(await ensureMessageCapabilityAvailable(agent)))'
    );
    expect(webSessionPanelSource).toContain('return !runtimeHasCodex.value');
    expect(webSessionPanelSource).toContain("t('webSession.codexCompatibilityAgentLabel')");
    expect(webSessionPanelSource).toContain('function maybeNotifyCodexCompatibilityMode()');
    expect(webSessionPanelSource).toContain('message.warning(codexCompatibilityModeMessage());');
    expect(webSessionPanelSource).not.toContain('class="composer-compatibility-notice"');
    expect(webSessionPanelSource).not.toContain('.composer-compatibility-notice');
    expect(webSessionPanelSource).not.toContain(
      'message.warning(codexWebSessionUnavailableMessage())'
    );
  });

  it('only shows the compatibility notice while creating a new Codex session', () => {
    const createSessionSource = webSessionPanelSource.slice(
      webSessionPanelSource.indexOf('async function handleCreateSession('),
      webSessionPanelSource.indexOf('async function handleStartDraftSession(')
    );
    const capabilityCheckSource = webSessionPanelSource.slice(
      webSessionPanelSource.indexOf('async function ensureMessageCapabilityAvailable('),
      webSessionPanelSource.indexOf('async function ensureGoalModeAvailable(')
    );
    const agentSelectSource = webSessionPanelSource.slice(
      webSessionPanelSource.indexOf('function handleAgentDropdownSelect('),
      webSessionPanelSource.indexOf('function getKnownModelLabel(')
    );

    expect(createSessionSource).toContain("if (agent === 'codex')");
    expect(createSessionSource).toContain('maybeNotifyCodexCompatibilityMode();');
    expect(capabilityCheckSource).not.toContain('maybeNotifyCodexCompatibilityMode();');
    expect(agentSelectSource).not.toContain('maybeNotifyCodexCompatibilityMode();');
    expect(webSessionPanelSource).not.toContain('notifiedCodexCompatibilityKey');
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
    expect(webSessionPanelSource).toContain('showGoalCard.value = false;');
    expect(webSessionPanelSource).toContain('await handleGoalCompose();');
    expect(webSessionPanelSource).toContain("'Insert /goal'");
  });
});
