import type { Worktree } from '@/types/models';

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
