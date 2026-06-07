import type { FileManagerChangeEntry } from '@/types/fileManager';
import type { DesktopWorkspaceRouteTab } from '@/utils/workspaceRoute';

export interface GitChangeSelectionAfterLoad {
  entry: FileManagerChangeEntry | null;
  selectedPath: string;
  shouldLoadEntry: boolean;
}

export function resolveRetainedGitChangeEntry(
  entries: FileManagerChangeEntry[],
  selectedPath: string
) {
  const normalizedPath = selectedPath.trim();
  if (!normalizedPath) {
    return null;
  }
  return entries.find(entry => entry.path === normalizedPath) ?? null;
}

export function resolveGitChangeSelectionAfterLoad(
  entries: FileManagerChangeEntry[],
  selectedPath: string
): GitChangeSelectionAfterLoad {
  const entry = resolveRetainedGitChangeEntry(entries, selectedPath);
  return {
    entry,
    selectedPath: entry?.path ?? '',
    shouldLoadEntry: entry !== null,
  };
}

export function buildGitChangesFingerprint(
  entries: FileManagerChangeEntry[],
  ignoreUntracked = false
) {
  const visibleEntries = ignoreUntracked
    ? entries.filter(entry => entry.status.kind !== 'untracked')
    : entries;
  return JSON.stringify(
    [...visibleEntries]
      .sort((left, right) =>
        left.path.localeCompare(right.path, undefined, {
          sensitivity: 'base',
        })
      )
      .map(entry => ({
        path: entry.path,
        status: entry.status.kind,
        previousPath: entry.status.previousPath ?? '',
        exists: entry.exists,
        additions: Math.max(0, Math.trunc(entry.additions ?? 0)),
        deletions: Math.max(0, Math.trunc(entry.deletions ?? 0)),
        statsAvailable: entry.statsAvailable === true,
      }))
  );
}

export function hasPendingGitChangesUpdate(
  currentEntries: FileManagerChangeEntry[],
  nextEntries: FileManagerChangeEntry[],
  ignoreUntracked = false
) {
  return (
    buildGitChangesFingerprint(currentEntries, ignoreUntracked) !==
    buildGitChangesFingerprint(nextEntries, ignoreUntracked)
  );
}

export function shouldLoadWorkspaceChangesSummary(
  projectId: string,
  changesTabDisabled: boolean,
  activeTab: DesktopWorkspaceRouteTab
) {
  return Boolean(projectId) && !changesTabDisabled && activeTab !== 'changes';
}

export function canShowWorkspaceChangesSummary(projectId: string, changesTabDisabled: boolean) {
  return Boolean(projectId) && !changesTabDisabled;
}
