import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const projectWorkspacePath = fileURLToPath(new URL('../ProjectWorkspace.vue', import.meta.url));
const webSessionPanelPath = fileURLToPath(
  new URL('../../components/web-session/WebSessionPanel.vue', import.meta.url)
);
const projectWorkspaceSource = readFileSync(projectWorkspacePath, 'utf8');
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const mobileChangesSummaryPath = fileURLToPath(
  new URL('../../components/web-session/useWebSessionMobileChangesSummary.ts', import.meta.url)
);
const mobileProjectSwitchPath = fileURLToPath(
  new URL('../../components/web-session/useWebSessionMobileProjectSwitch.ts', import.meta.url)
);
const mobileChangesSummarySource = readFileSync(mobileChangesSummaryPath, 'utf8');
const mobileProjectSwitchSource = readFileSync(mobileProjectSwitchPath, 'utf8');

describe('project workspace mobile chrome', () => {
  it('keeps the project context row and adds a compact project switcher to the mobile header', () => {
    const projectContextIndex = webSessionPanelSource.indexOf('class="mobile-project-context-bar"');
    const sessionSelectorIndex = webSessionPanelSource.indexOf('class="mobile-tab-selector"');
    const projectSwitcherIndex = webSessionPanelSource.indexOf('class="mobile-project-switch"');

    expect(projectContextIndex).toBeGreaterThan(-1);
    expect(sessionSelectorIndex).toBeGreaterThan(projectContextIndex);
    expect(projectSwitcherIndex).toBeGreaterThan(projectContextIndex);
    expect(projectSwitcherIndex).toBeLessThan(sessionSelectorIndex);
    expect(webSessionPanelSource).toContain('mobileChangesSummaryDisplay.count');
    expect(mobileChangesSummarySource).toContain(
      "formatGitChangesBadgeDelta('+', current.additions)"
    );
    expect(mobileChangesSummarySource).toContain(
      "formatGitChangesBadgeDelta('-', current.deletions)"
    );
    expect(webSessionPanelSource).toContain(':options="mobileProjectSwitchOptions"');
    expect(webSessionPanelSource).toContain("t('terminal.projectSearchPlaceholder')");
    expect(mobileProjectSwitchSource).toContain('MAX_PROJECT_SWITCH_ITEMS = 10');
    expect(mobileProjectSwitchSource).toContain('projects.slice(0, MAX_PROJECT_SWITCH_ITEMS)');
    expect(webSessionPanelSource).not.toContain('@click="handleMobileChangesOpen"');
    expect(webSessionPanelSource).not.toContain("emit('request-mobile-view', 'changes')");
  });

  it('shows current-project counts on the session and terminal navigation icons', () => {
    expect(projectWorkspaceSource).toContain('v-if="mobileWebSessionBadge"');
    expect(projectWorkspaceSource).toContain('{{ mobileWebSessionBadge }}');
    expect(projectWorkspaceSource).toContain('v-if="mobileTerminalBadge"');
    expect(projectWorkspaceSource).toContain('{{ mobileTerminalBadge }}');
    expect(projectWorkspaceSource).toContain('v-if="mobileTrackedChangesBadge"');
    expect(projectWorkspaceSource).toContain('{{ mobileTrackedChangesBadge }}');
    expect(projectWorkspaceSource).toContain('resolveMobileTrackedModificationCount(');
    expect(projectWorkspaceSource).toContain('class="nav-count-badge nav-count-badge--changes"');
    expect(projectWorkspaceSource).toMatch(
      /\.mobile-bottom-nav \.nav-count-badge--changes\s*\{[^}]*background:\s*#f59e0b;/s
    );
    expect(projectWorkspaceSource).toContain('MOBILE_GIT_STATUS_REFRESH_INTERVAL_MS = 10_000');
  });
});
