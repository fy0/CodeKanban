import { describe, expect, it } from 'vitest';

import {
  resolveWebSessionMobileContextWorktree,
  summarizeWebSessionMobileGitStatus,
} from '@/components/web-session/webSessionMobileProjectContext';
import type { Worktree } from '@/types/models';

function makeWorktree(overrides: Partial<Worktree> = {}): Worktree {
  return {
    id: 'main-worktree',
    projectId: 'project-1',
    branchName: 'main',
    path: '/repo',
    isMain: true,
    headCommit: null,
    headCommitDate: null,
    statusAhead: 0,
    statusBehind: 0,
    statusModified: 0,
    statusStaged: 0,
    statusUntracked: 0,
    statusConflicts: 0,
    statusUpdatedAt: '2026-08-10T00:00:00.000Z',
    createdAt: '2026-08-10T00:00:00.000Z',
    updatedAt: '2026-08-10T00:00:00.000Z',
    ...overrides,
  };
}

describe('webSessionMobileProjectContext', () => {
  it('prefers the active session worktree, then the selected and main worktrees', () => {
    const main = makeWorktree();
    const selected = makeWorktree({ id: 'selected', branchName: 'selected', isMain: false });
    const session = makeWorktree({ id: 'session', branchName: 'session', isMain: false });
    const worktrees = [main, selected, session];

    expect(
      resolveWebSessionMobileContextWorktree(worktrees, {
        projectId: 'project-1',
        sessionWorktreeId: 'session',
        selectedWorktreeId: 'selected',
      })?.id
    ).toBe('session');
    expect(
      resolveWebSessionMobileContextWorktree(worktrees, {
        projectId: 'project-1',
        selectedWorktreeId: 'selected',
      })?.id
    ).toBe('selected');
    expect(
      resolveWebSessionMobileContextWorktree(worktrees, {
        projectId: 'project-1',
      })?.id
    ).toBe('main-worktree');
  });

  it('summarizes each visible Git status independently', () => {
    const summary = summarizeWebSessionMobileGitStatus(
      makeWorktree({
        statusAhead: 2,
        statusBehind: 1,
        statusConflicts: 1,
        statusModified: 3,
        statusStaged: 4,
        statusUntracked: 5,
      }),
      { gitAvailable: true }
    );

    expect(summary).toEqual({
      state: 'dirty',
      ahead: 2,
      behind: 1,
      conflicts: 1,
      modified: 3,
      staged: 4,
      untracked: 5,
    });
  });

  it('distinguishes clean, loading, and unavailable states', () => {
    expect(summarizeWebSessionMobileGitStatus(makeWorktree(), { gitAvailable: true }).state).toBe(
      'clean'
    );
    expect(
      summarizeWebSessionMobileGitStatus(
        makeWorktree({
          statusAhead: null,
          statusBehind: null,
          statusConflicts: null,
          statusModified: null,
          statusStaged: null,
          statusUntracked: null,
          statusUpdatedAt: null,
        }),
        { gitAvailable: true }
      ).state
    ).toBe('loading');
    expect(summarizeWebSessionMobileGitStatus(makeWorktree(), { gitAvailable: false }).state).toBe(
      'unavailable'
    );
  });
});
