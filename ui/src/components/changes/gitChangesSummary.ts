import type {
  FileManagerChangeEntry,
  FileManagerChangesResult,
  FileManagerScope,
} from '@/types/fileManager';

export const GIT_CHANGES_IGNORE_UNTRACKED_STORAGE_KEY = 'git-changes-ignore-untracked';
export const GIT_CHANGES_IGNORE_UNTRACKED_DEFAULT = true;

export interface GitChangesSummary {
  count: number;
  additions: number;
  deletions: number;
}

export type GitChangesBadgeState = 'loading' | 'complete' | 'partial' | 'timedOut';

export interface GitChangesBadgeSummary {
  count: number;
  additions: number | null;
  deletions: number | null;
  state: GitChangesBadgeState;
  changeToken: string;
  scopeId: string;
}

export function chooseGitChangesScope(
  scopes: FileManagerScope[],
  options?: {
    activeScopeId?: string;
    preferredWorktreeId?: string | null;
    requestedScopeId?: string;
  }
) {
  if (scopes.length === 0) {
    return null;
  }

  if (options?.requestedScopeId) {
    const explicit = scopes.find(scope => scope.id === options.requestedScopeId);
    if (explicit) {
      return explicit;
    }
  }

  if (options?.activeScopeId) {
    const active = scopes.find(scope => scope.id === options.activeScopeId);
    if (active) {
      return active;
    }
  }

  if (options?.preferredWorktreeId) {
    const preferred = scopes.find(scope => scope.worktreeId === options.preferredWorktreeId);
    if (preferred) {
      return preferred;
    }
  }

  return scopes[0];
}

export function orderGitChangesEntries(entries: FileManagerChangeEntry[], ignoreUntracked = false) {
  const filtered = ignoreUntracked
    ? entries.filter(entry => entry.status.kind !== 'untracked')
    : entries;
  return [...filtered].sort((left, right) => {
    const leftUntracked = left.status.kind === 'untracked' ? 1 : 0;
    const rightUntracked = right.status.kind === 'untracked' ? 1 : 0;
    if (leftUntracked !== rightUntracked) {
      return leftUntracked - rightUntracked;
    }
    return left.path.localeCompare(right.path, undefined, {
      sensitivity: 'base',
    });
  });
}

export function summarizeGitChangesEntries(
  entries: FileManagerChangeEntry[],
  ignoreUntracked = false
): GitChangesSummary {
  const orderedEntries = orderGitChangesEntries(entries, ignoreUntracked);
  return orderedEntries.reduce<GitChangesSummary>(
    (summary, entry) => ({
      count: summary.count + 1,
      additions: summary.additions + Math.max(0, Math.trunc(entry.additions ?? 0)),
      deletions: summary.deletions + Math.max(0, Math.trunc(entry.deletions ?? 0)),
    }),
    {
      count: 0,
      additions: 0,
      deletions: 0,
    }
  );
}

export function buildGitChangesBadgeSummary(
  result: FileManagerChangesResult | null,
  ignoreUntracked = false
): GitChangesBadgeSummary | null {
  if (!result) {
    return null;
  }

  const summary = summarizeGitChangesEntries(result.entries ?? [], ignoreUntracked);
  if (summary.count === 0) {
    return {
      count: 0,
      additions: 0,
      deletions: 0,
      state: 'complete',
      changeToken: result.changeToken,
      scopeId: result.scope.id,
    };
  }

  return {
    count: summary.count,
    additions: result.statsComplete ? summary.additions : null,
    deletions: result.statsComplete ? summary.deletions : null,
    state: result.statsComplete ? 'complete' : result.statsTimedOut ? 'timedOut' : 'partial',
    changeToken: result.changeToken,
    scopeId: result.scope.id,
  };
}

export function shouldLoadGitChangesStats(
  current: GitChangesBadgeSummary | null,
  scopeId: string,
  changeToken: string
) {
  if (!current || current.state === 'loading') {
    return true;
  }
  return current.scopeId !== scopeId || current.changeToken !== changeToken;
}

export function formatGitChangesSummary(summary: GitChangesSummary) {
  if (summary.count === 0) {
    return '';
  }
  return `${summary.count},+${summary.additions},-${summary.deletions}`;
}

export function formatGitChangesBadgeDelta(prefix: '+' | '-', value: number | null) {
  if (value == null) {
    return `${prefix}?`;
  }
  return `${prefix}${Math.max(0, Math.trunc(value ?? 0))}`;
}

export function shouldShowGitChangesBadge(summary: GitChangesBadgeSummary | null) {
  if (!summary) {
    return false;
  }
  return summary.count > 0;
}
