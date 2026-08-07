import { describe, expect, it } from 'vitest';

import {
  gitOperationAvailable,
  gitOperationEngine,
  projectSupportsGit,
} from '@/utils/projectGitCapability';

const capabilities = {
  repository: true,
  mode: 'read_write' as const,
  operations: {
    branchesRead: true,
    branchesWrite: true,
    status: true,
    diff: true,
    worktreesRead: true,
    worktreesWrite: true,
    commit: true,
    fastForwardMerge: true,
    merge: true,
    rebase: true,
    squash: true,
  },
  engines: {
    branchesRead: 'builtin' as const,
    branchesWrite: 'system' as const,
    status: 'builtin' as const,
    diff: 'builtin' as const,
    worktreesRead: 'builtin' as const,
    worktreesWrite: 'system' as const,
    commit: 'system' as const,
    fastForwardMerge: 'builtin' as const,
    merge: 'system' as const,
    rebase: 'system' as const,
    squash: 'system' as const,
  },
  reasons: [],
  worktrees: [],
};

describe('projectGitCapability', () => {
  it('uses the server capability report as the source of truth', () => {
    expect(projectSupportsGit(capabilities)).toBe(true);
    expect(gitOperationAvailable(capabilities, 'fastForwardMerge')).toBe(true);
    expect(gitOperationEngine(capabilities, 'commit')).toBe('system');
  });

  it('honors operation-specific and worktree-specific restrictions', () => {
    const restricted = {
      ...capabilities,
      operations: { ...capabilities.operations, commit: false },
      worktrees: [
        {
          id: 'wt-1',
          operations: { ...capabilities.operations, fastForwardMerge: false },
          engines: { ...capabilities.engines, fastForwardMerge: 'unavailable' as const },
          reasons: [{ code: 'worktree_dirty' }],
        },
      ],
    };
    expect(gitOperationAvailable(restricted, 'commit')).toBe(false);
    expect(gitOperationAvailable(restricted, 'fastForwardMerge', 'wt-1')).toBe(false);
    expect(gitOperationAvailable(restricted, 'status', 'missing-worktree')).toBe(false);
  });

  it('treats unavailable reports as unsupported', () => {
    expect(projectSupportsGit(null)).toBe(false);
    expect(projectSupportsGit({ ...capabilities, repository: false })).toBe(false);
    expect(projectSupportsGit({ ...capabilities, mode: 'unavailable' })).toBe(false);
  });
});
