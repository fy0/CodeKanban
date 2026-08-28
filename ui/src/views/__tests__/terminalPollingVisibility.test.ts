import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const workspaceTabViewSource = readFileSync(
  fileURLToPath(new URL('../../components/workspace/WorkspaceTabView.vue', import.meta.url)),
  'utf8'
);
const projectWorkspaceSource = readFileSync(
  fileURLToPath(new URL('../ProjectWorkspace.vue', import.meta.url)),
  'utf8'
);
const terminalPanelSource = readFileSync(
  fileURLToPath(new URL('../../components/terminal/TerminalPanel.vue', import.meta.url)),
  'utf8'
);

describe('terminal polling visibility', () => {
  it('marks the terminal panel active only while its workspace view is visible', () => {
    expect(workspaceTabViewSource).toMatch(
      /<TerminalPanel\b[^>]*:is-active="activeTab === 'terminal'"[^>]*\/>/s
    );
    expect(projectWorkspaceSource).toMatch(
      /<TerminalPanel\b[^>]*:is-active="mobileActiveView === 'terminal'"[^>]*\/>/s
    );
  });

  it('releases terminal session snapshot polling while the panel is inactive', () => {
    expect(terminalPanelSource).toContain('isActive?: boolean;');
    expect(terminalPanelSource).toContain('[projectIdRef, shouldPollSessionSnapshots]');
    expect(terminalPanelSource).toContain('if (!projectId || !shouldPoll)');
    expect(terminalPanelSource).toContain(
      'sessionSnapshotStore.releaseScope(terminalSessionSnapshotScopeId);'
    );
  });
});
