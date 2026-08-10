import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const projectWorkspacePath = fileURLToPath(new URL('../ProjectWorkspace.vue', import.meta.url));
const webSessionPanelPath = fileURLToPath(
  new URL('../../components/web-session/WebSessionPanel.vue', import.meta.url)
);
const projectWorkspaceSource = readFileSync(projectWorkspacePath, 'utf8');
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

describe('project workspace mobile chrome', () => {
  it('places project and Git context above the mobile session selector', () => {
    const projectContextIndex = webSessionPanelSource.indexOf('class="mobile-project-context-bar"');
    const sessionSelectorIndex = webSessionPanelSource.indexOf('class="mobile-tab-selector"');

    expect(projectContextIndex).toBeGreaterThan(-1);
    expect(sessionSelectorIndex).toBeGreaterThan(projectContextIndex);
    expect(webSessionPanelSource).toContain('mobileGitStatus.conflicts');
    expect(webSessionPanelSource).toContain("emit('request-mobile-view', 'changes')");
  });

  it('shows current-project counts on the session and terminal navigation icons', () => {
    expect(projectWorkspaceSource).toContain('v-if="mobileWebSessionBadge"');
    expect(projectWorkspaceSource).toContain('{{ mobileWebSessionBadge }}');
    expect(projectWorkspaceSource).toContain('v-if="mobileTerminalBadge"');
    expect(projectWorkspaceSource).toContain('{{ mobileTerminalBadge }}');
    expect(projectWorkspaceSource).toContain('MOBILE_GIT_STATUS_REFRESH_INTERVAL_MS = 10_000');
  });
});
