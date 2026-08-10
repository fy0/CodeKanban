import type { Worktree } from '@/types/models';

export type WebSessionMobileGitStatusState = 'loading' | 'unavailable' | 'clean' | 'dirty';

export type WebSessionMobileGitStatus = {
  state: WebSessionMobileGitStatusState;
  ahead: number;
  behind: number;
  conflicts: number;
  modified: number;
  staged: number;
  untracked: number;
};

const EMPTY_GIT_STATUS_COUNTS = {
  ahead: 0,
  behind: 0,
  conflicts: 0,
  modified: 0,
  staged: 0,
  untracked: 0,
} as const;

export function resolveWebSessionMobileContextWorktree(
  worktrees: Worktree[],
  options: {
    projectId: string;
    sessionWorktreeId?: string | null;
    selectedWorktreeId?: string | null;
  }
): Worktree | null {
  const projectWorktrees = worktrees.filter(worktree => worktree.projectId === options.projectId);
  if (projectWorktrees.length === 0) {
    return null;
  }

  const preferredIds = [options.sessionWorktreeId, options.selectedWorktreeId].filter(
    (value): value is string => Boolean(value)
  );
  for (const worktreeId of preferredIds) {
    const matched = projectWorktrees.find(worktree => worktree.id === worktreeId);
    if (matched) {
      return matched;
    }
  }

  return projectWorktrees.find(worktree => worktree.isMain) ?? projectWorktrees[0] ?? null;
}

export function summarizeWebSessionMobileGitStatus(
  worktree: Worktree | null,
  options: {
    gitAvailable: boolean;
    loading?: boolean;
  }
): WebSessionMobileGitStatus {
  if (options.loading) {
    return { state: 'loading', ...EMPTY_GIT_STATUS_COUNTS };
  }
  if (!worktree || !options.gitAvailable) {
    return { state: 'unavailable', ...EMPTY_GIT_STATUS_COUNTS };
  }

  const rawStatusValues = [
    worktree.statusAhead,
    worktree.statusBehind,
    worktree.statusConflicts,
    worktree.statusModified,
    worktree.statusStaged,
    worktree.statusUntracked,
  ];
  if (!worktree.statusUpdatedAt && rawStatusValues.every(value => value == null)) {
    return { state: 'loading', ...EMPTY_GIT_STATUS_COUNTS };
  }

  const counts = {
    ahead: normalizeStatusCount(worktree.statusAhead),
    behind: normalizeStatusCount(worktree.statusBehind),
    conflicts: normalizeStatusCount(worktree.statusConflicts),
    modified: normalizeStatusCount(worktree.statusModified),
    staged: normalizeStatusCount(worktree.statusStaged),
    untracked: normalizeStatusCount(worktree.statusUntracked),
  };
  const hasChanges =
    counts.conflicts > 0 || counts.modified > 0 || counts.staged > 0 || counts.untracked > 0;

  return {
    state: hasChanges ? 'dirty' : 'clean',
    ...counts,
  };
}

function normalizeStatusCount(value: number | null | undefined) {
  const numericValue = Number(value ?? 0);
  return Number.isFinite(numericValue) ? Math.max(0, Math.trunc(numericValue)) : 0;
}
