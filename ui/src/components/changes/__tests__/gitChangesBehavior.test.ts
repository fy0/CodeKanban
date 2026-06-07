import { describe, expect, it } from 'vitest';

import {
  buildGitChangesFingerprint,
  canShowWorkspaceChangesSummary,
  hasPendingGitChangesUpdate,
  resolveGitChangeSelectionAfterLoad,
  resolveRetainedGitChangeEntry,
  shouldLoadWorkspaceChangesSummary,
} from '@/components/changes/gitChangesBehavior';
import type { FileManagerChangeEntry } from '@/types/fileManager';

function createEntry(
  path: string,
  overrides: Partial<FileManagerChangeEntry> = {}
): FileManagerChangeEntry {
  return {
    name: path.split('/').at(-1) ?? path,
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

describe('gitChangesBehavior', () => {
  it('does not auto-select the first entry when the panel loads without a prior selection', () => {
    const selection = resolveGitChangeSelectionAfterLoad([createEntry('README.md')], '');

    expect(selection.entry).toBeNull();
    expect(selection.selectedPath).toBe('');
    expect(selection.shouldLoadEntry).toBe(false);
  });

  it('retains and reloads the previously selected entry when it still exists', () => {
    const entry = createEntry('docs/guide.md');
    const selection = resolveGitChangeSelectionAfterLoad(
      [createEntry('README.md'), entry],
      'docs/guide.md'
    );

    expect(selection.entry).toEqual(entry);
    expect(selection.selectedPath).toBe('docs/guide.md');
    expect(selection.shouldLoadEntry).toBe(true);
  });

  it('clears the selection when the previously selected entry disappears', () => {
    const selection = resolveGitChangeSelectionAfterLoad(
      [createEntry('README.md')],
      'docs/guide.md'
    );

    expect(selection.entry).toBeNull();
    expect(selection.selectedPath).toBe('');
    expect(selection.shouldLoadEntry).toBe(false);
  });

  it('finds a retained selection only when the path is still visible', () => {
    expect(
      resolveRetainedGitChangeEntry([createEntry('README.md')], 'README.md')?.path
    ).toBe('README.md');
    expect(resolveRetainedGitChangeEntry([createEntry('README.md')], 'docs/guide.md')).toBeNull();
  });

  it('suppresses workspace badge summary loading while the changes tab is active', () => {
    expect(shouldLoadWorkspaceChangesSummary('project-1', false, 'changes')).toBe(false);
    expect(shouldLoadWorkspaceChangesSummary('project-1', false, 'files')).toBe(true);
    expect(shouldLoadWorkspaceChangesSummary('', false, 'files')).toBe(false);
    expect(shouldLoadWorkspaceChangesSummary('project-1', true, 'files')).toBe(false);
  });

  it('keeps workspace badge display eligibility independent from the active tab', () => {
    expect(canShowWorkspaceChangesSummary('project-1', false)).toBe(true);
    expect(canShowWorkspaceChangesSummary('', false)).toBe(false);
    expect(canShowWorkspaceChangesSummary('project-1', true)).toBe(false);
  });

  it('builds a stable fingerprint independent from entry order', () => {
    const left = [createEntry('b.ts'), createEntry('a.ts')];
    const right = [createEntry('a.ts'), createEntry('b.ts')];

    expect(buildGitChangesFingerprint(left)).toBe(buildGitChangesFingerprint(right));
    expect(hasPendingGitChangesUpdate(left, right)).toBe(false);
  });

  it('detects pending updates when visible git changes differ', () => {
    expect(
      hasPendingGitChangesUpdate([createEntry('a.ts')], [createEntry('a.ts'), createEntry('b.ts')])
    ).toBe(true);

    expect(
      hasPendingGitChangesUpdate(
        [createEntry('a.ts')],
        [
          createEntry('a.ts', {
            additions: 2,
          }),
        ]
      )
    ).toBe(true);

    expect(
      hasPendingGitChangesUpdate(
        [
          createEntry('a.ts', {
            status: {
              kind: 'modified',
            },
          }),
        ],
        [
          createEntry('a.ts', {
            status: {
              kind: 'renamed',
              previousPath: 'old-a.ts',
            },
          }),
        ]
      )
    ).toBe(true);
  });

  it('ignores untracked-only updates when untracked files are hidden', () => {
    const current = [createEntry('a.ts')];
    const next = [
      createEntry('a.ts'),
      createEntry('scratch.log', {
        status: {
          kind: 'untracked',
        },
      }),
    ];

    expect(hasPendingGitChangesUpdate(current, next, true)).toBe(false);
    expect(hasPendingGitChangesUpdate(current, next, false)).toBe(true);
  });
});
