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

export function hasPendingGitChangesUpdate(currentChangeToken: string, nextChangeToken: string) {
  return currentChangeToken !== nextChangeToken;
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
