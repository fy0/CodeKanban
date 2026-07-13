import type { FileManagerChangeEntry, FileManagerChangesResult } from '@/types/fileManager';

export const GIT_CHANGES_REQUEST_TIMEOUT_MS = 5000;
export const GIT_CHANGES_MAX_ENTRIES = 1000;

export interface GitChangesWarningDescriptor {
  key: string;
  type: 'warning' | 'info';
  i18nKey: string;
  params?: Record<string, number | string>;
}

export function buildGitChangesRequestOptions(ignoreUntracked: boolean) {
  return {
    includeUntracked: !ignoreUntracked,
    withStats: true,
    timeoutMs: GIT_CHANGES_REQUEST_TIMEOUT_MS,
    maxEntries: GIT_CHANGES_MAX_ENTRIES,
  };
}

export function buildGitChangesProbeOptions(ignoreUntracked: boolean) {
  return {
    ...buildGitChangesRequestOptions(ignoreUntracked),
    withStats: false,
  };
}

export function formatGitChangeCount(prefix: '+' | '-', value: number | null) {
  if (value == null) {
    return `${prefix}?`;
  }
  return `${prefix}${Math.max(0, Math.trunc(value ?? 0))}`;
}

export function formatGitChangeStat(
  entry: Pick<FileManagerChangeEntry, 'additions' | 'deletions' | 'statsAvailable'>
) {
  if (!entry.statsAvailable) {
    return '+? -?';
  }
  return `${formatGitChangeCount('+', entry.additions)} ${formatGitChangeCount('-', entry.deletions)}`;
}

export function getGitChangesWarnings(
  result: FileManagerChangesResult | null
): GitChangesWarningDescriptor[] {
  if (!result) {
    return [];
  }

  const warnings: GitChangesWarningDescriptor[] = [];
  if (result.truncated) {
    if (result.warningReason === 'entry_limit_exceeded') {
      warnings.push({
        key: 'entry-limit',
        type: 'warning',
        i18nKey: 'gitChanges.partialEntryLimit',
        params: {
          limit: GIT_CHANGES_MAX_ENTRIES,
        },
      });
    } else {
      warnings.push({
        key: 'partial-results',
        type: 'warning',
        i18nKey: 'gitChanges.partialResults',
      });
    }
  }

  if (result.statsTimedOut) {
    warnings.push({
      key: 'stats-timed-out',
      type: 'warning',
      i18nKey: 'gitChanges.statsTimedOut',
    });
  } else if (!result.statsComplete && result.entries.length > 0) {
    warnings.push({
      key: 'stats-partial',
      type: 'info',
      i18nKey: 'gitChanges.statsPartial',
    });
  }

  return warnings;
}
