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
  });

  it('guards goal mode actions behind the Codex version capability check', () => {
    expect(webSessionPanelSource).toContain('ensureGoalModeAvailable');
    expect(webSessionPanelSource).toContain('goalModeUnavailableMessage');
    expect(webSessionPanelSource).toContain('isCurrentSessionGoalModeBlocked');
  });

  it('renders the goal card for codex draft sessions on the initial screen', () => {
    expect(webSessionPanelSource).toContain("v-if=\"currentSession?.agent === 'codex' && showGoalCard\"");
    expect(webSessionPanelSource).toContain(
      'Draft session goal will be created when you send the /goal command.'
    );
  });
});
