import { describe, expect, it } from 'vitest';

import {
  buildGitChangesProbeOptions,
  buildGitChangesRequestOptions,
  formatGitChangeStat,
  getGitChangesWarnings,
  GIT_CHANGES_MAX_ENTRIES,
  GIT_CHANGES_REQUEST_TIMEOUT_MS,
} from '@/components/changes/gitChangesSupport';
import type { FileManagerChangeEntry, FileManagerChangesResult } from '@/types/fileManager';

function makeChangeEntry(
  path: string,
  overrides?: Partial<FileManagerChangeEntry>
): FileManagerChangeEntry {
  return {
    name: path.split('/').pop() ?? path,
    path,
    previewKind: 'text',
    hidden: false,
    exists: true,
    status: {
      kind: 'modified',
    },
    additions: 1,
    deletions: 0,
    statsAvailable: true,
    ...overrides,
  };
}

function makeChangesResult(
  overrides?: Partial<FileManagerChangesResult>
): FileManagerChangesResult {
  return {
    scope: {
      id: 'scope-1',
      kind: 'project',
      label: 'Project',
      rootPath: '/tmp/project',
    },
    entries: [makeChangeEntry('README.md')],
    changeToken: 'token-1',
    truncated: false,
    statsComplete: true,
    statsTimedOut: false,
    untrackedIncluded: true,
    ...overrides,
  };
}

describe('gitChangesSupport', () => {
  it('builds safe backend request options when untracked files are ignored', () => {
    expect(buildGitChangesRequestOptions(true)).toEqual({
      includeUntracked: false,
      withStats: true,
      timeoutMs: GIT_CHANGES_REQUEST_TIMEOUT_MS,
      maxEntries: GIT_CHANGES_MAX_ENTRIES,
    });
  });

  it('builds status-only options for change polling', () => {
    expect(buildGitChangesProbeOptions(true)).toEqual({
      includeUntracked: false,
      withStats: false,
      timeoutMs: GIT_CHANGES_REQUEST_TIMEOUT_MS,
      maxEntries: GIT_CHANGES_MAX_ENTRIES,
    });
  });

  it('formats missing per-entry stats with question marks', () => {
    expect(
      formatGitChangeStat(
        makeChangeEntry('README.md', {
          additions: 0,
          deletions: 0,
          statsAvailable: false,
        })
      )
    ).toBe('+? -?');
  });

  it('returns warnings for truncated results and timed out stats', () => {
    expect(
      getGitChangesWarnings(
        makeChangesResult({
          truncated: true,
          warningReason: 'entry_limit_exceeded',
          statsComplete: false,
          statsTimedOut: true,
        })
      )
    ).toEqual([
      {
        key: 'entry-limit',
        type: 'warning',
        i18nKey: 'gitChanges.partialEntryLimit',
        params: {
          limit: GIT_CHANGES_MAX_ENTRIES,
        },
      },
      {
        key: 'stats-timed-out',
        type: 'warning',
        i18nKey: 'gitChanges.statsTimedOut',
      },
    ]);
  });

  it('returns an info warning for incomplete stats without a timeout', () => {
    expect(
      getGitChangesWarnings(
        makeChangesResult({
          statsComplete: false,
          statsTimedOut: false,
        })
      )
    ).toEqual([
      {
        key: 'stats-partial',
        type: 'info',
        i18nKey: 'gitChanges.statsPartial',
      },
    ]);
  });
});
