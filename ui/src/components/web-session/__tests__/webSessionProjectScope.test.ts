import { describe, expect, it } from 'vitest';

import {
  createWebSessionProjectInitializationGate,
  resolveWebSessionDraftProjectPresentation,
  resolveWebSessionScopedProject,
} from '@/components/web-session/webSessionProjectScope';

const projectA = {
  id: 'project-a',
  name: 'Project A',
  path: 'D:/projects/a',
};
const projectB = {
  id: 'project-b',
  name: 'Project B',
  path: 'D:/projects/b',
};

describe('webSessionProjectScope', () => {
  it('resolves the target project from the project list instead of a stale current project', () => {
    expect(resolveWebSessionScopedProject('project-b', projectA, [projectA, projectB])).toBe(
      projectB
    );
    expect(
      resolveWebSessionDraftProjectPresentation({
        projectId: 'project-b',
        agent: 'codex',
        currentProject: projectA,
        projects: [projectA, projectB],
        worktreesReady: false,
      })
    ).toEqual({
      title: 'Codex · Project B',
      cwd: 'D:/projects/b',
      worktreeId: null,
    });
  });

  it('uses the Pi identity in draft presentation', () => {
    expect(
      resolveWebSessionDraftProjectPresentation({
        projectId: 'project-b',
        agent: 'pi',
        currentProject: projectB,
        projects: [projectA, projectB],
        worktreesReady: true,
      })
    ).toEqual({
      title: 'Pi · Project B',
      cwd: 'D:/projects/b',
      worktreeId: null,
    });
  });

  it('never accepts a worktree from another project', () => {
    expect(
      resolveWebSessionDraftProjectPresentation({
        projectId: 'project-b',
        agent: 'claude',
        worktreeId: 'shared-id',
        currentProject: projectB,
        projects: [projectA, projectB],
        worktrees: [
          {
            id: 'shared-id',
            projectId: 'project-a',
            path: 'D:/projects/a/worktree',
          },
          {
            id: 'shared-id',
            projectId: 'project-b',
            path: 'D:/projects/b/worktree',
          },
        ],
        worktreesReady: true,
      })
    ).toEqual({
      title: 'Claude · Project B',
      cwd: 'D:/projects/b/worktree',
      worktreeId: 'shared-id',
    });
  });

  it('keeps a pending worktree id without showing a stale path while worktrees load', () => {
    expect(
      resolveWebSessionDraftProjectPresentation({
        projectId: 'project-b',
        agent: 'codex',
        worktreeId: 'worktree-b',
        currentProject: projectA,
        projects: [],
        worktreesReady: false,
      })
    ).toEqual({
      title: 'Codex',
      cwd: '',
      worktreeId: 'worktree-b',
    });
  });

  it('clears an invalid worktree and falls back to the target project after loading', () => {
    expect(
      resolveWebSessionDraftProjectPresentation({
        projectId: 'project-b',
        agent: 'codex',
        worktreeId: 'missing-worktree',
        currentProject: projectB,
        projects: [projectA, projectB],
        worktrees: [],
        worktreesReady: true,
      })
    ).toEqual({
      title: 'Codex · Project B',
      cwd: 'D:/projects/b',
      worktreeId: null,
    });
  });

  it('invalidates older initialization tokens when projects change', () => {
    const gate = createWebSessionProjectInitializationGate();
    const projectAToken = gate.begin('project-a');
    const projectBToken = gate.begin('project-b');

    expect(gate.isCurrent(projectAToken, 'project-a')).toBe(false);
    expect(gate.isCurrent(projectBToken, 'project-a')).toBe(false);
    expect(gate.isCurrent(projectBToken, 'project-b')).toBe(true);

    gate.invalidate();
    expect(gate.isCurrent(projectBToken, 'project-b')).toBe(false);
  });
});
