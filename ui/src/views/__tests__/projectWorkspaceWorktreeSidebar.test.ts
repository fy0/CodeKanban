import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const projectWorkspaceSource = readFileSync(
  fileURLToPath(new URL('../ProjectWorkspace.vue', import.meta.url)),
  'utf8'
);
const worktreeListSource = readFileSync(
  fileURLToPath(new URL('../../components/worktree/WorktreeList.vue', import.meta.url)),
  'utf8'
);
const worktreeCardSource = readFileSync(
  fileURLToPath(new URL('../../components/worktree/WorktreeCard.vue', import.meta.url)),
  'utf8'
);

describe('project workspace Worktree sidebar', () => {
  it('uses a persisted, adjustable width instead of a fixed width', () => {
    expect(projectWorkspaceSource).toContain("'workspace-worktree-sidebar-width'");
    expect(projectWorkspaceSource).toContain(':width="effectiveWorktreeSidebarWidth"');
    expect(projectWorkspaceSource).toContain('@mousedown="startWorktreeSidebarResize"');
    expect(projectWorkspaceSource).toContain('@keydown="handleWorktreeSidebarResizeKeydown"');
    expect(projectWorkspaceSource).toContain('@dblclick="resetWorktreeSidebarWidth"');
    expect(projectWorkspaceSource).not.toContain(':width="320"');
  });

  it('keeps the list header on one line and hides its title when space runs out', () => {
    expect(worktreeListSource).toMatch(/\.worktree-list\s*\{[^}]*container-type:\s*inline-size;/s);
    expect(worktreeListSource).toMatch(/\.list-header__row\s*\{[^}]*flex-wrap:\s*nowrap;/s);
    expect(worktreeListSource).toMatch(
      /\.list-header__actions\s*\{[^}]*flex:\s*0 0 auto;[^}]*flex-wrap:\s*nowrap;/s
    );
    expect(worktreeListSource).toMatch(
      /@container \(max-width:\s*270px\)\s*\{\s*\.list-header__title\s*\{[^}]*display:\s*none;/s
    );
  });

  it('puts card actions on their own row', () => {
    expect(worktreeCardSource).toMatch(
      /\.worktree-card__header\s*\{[^}]*flex-direction:\s*column;[^}]*align-items:\s*stretch;/s
    );
    expect(worktreeCardSource).toMatch(
      /\.worktree-card__actions-header\s*\{[^}]*justify-content:\s*flex-start;[^}]*width:\s*100%;/s
    );
  });
});
