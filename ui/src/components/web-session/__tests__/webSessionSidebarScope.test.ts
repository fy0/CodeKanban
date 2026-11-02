import { describe, expect, it } from 'vitest';

import {
  normalizeWebSessionSidebarScope,
  resolveWebSessionSidebarProjectIds,
  resolveWebSessionSidebarProjectIdsKey,
  resolveWebSessionSidebarToggleScope,
} from '@/components/web-session/webSessionSidebarScope';

describe('webSessionSidebarScope', () => {
  it('defaults unknown values to all', () => {
    expect(normalizeWebSessionSidebarScope(undefined)).toBe('all');
    expect(normalizeWebSessionSidebarScope('')).toBe('all');
    expect(normalizeWebSessionSidebarScope('invalid')).toBe('all');
  });

  it('keeps current scope when the stored value is valid', () => {
    expect(normalizeWebSessionSidebarScope('current')).toBe('current');
  });

  it('resolves the next scope for the toggle button', () => {
    expect(resolveWebSessionSidebarToggleScope('all')).toBe('current');
    expect(resolveWebSessionSidebarToggleScope('current')).toBe('all');
    expect(resolveWebSessionSidebarToggleScope('unexpected')).toBe('current');
  });

  it('returns only the active project in current scope', () => {
    expect(
      resolveWebSessionSidebarProjectIds({
        scope: 'current',
        currentProjectId: 'project-2',
        allProjectIds: ['project-1', 'project-2', 'project-3'],
      })
    ).toEqual(['project-2']);
  });

  it('preserves unique ordered projects in all scope', () => {
    expect(
      resolveWebSessionSidebarProjectIds({
        scope: 'all',
        currentProjectId: 'project-2',
        allProjectIds: ['project-2', 'project-1', 'project-2', '', 'project-3'],
      })
    ).toEqual(['project-2', 'project-1', 'project-3']);
  });

  it('returns an empty list when current scope has no active project', () => {
    expect(
      resolveWebSessionSidebarProjectIds({
        scope: 'current',
        currentProjectId: '',
        allProjectIds: ['project-1'],
      })
    ).toEqual([]);
  });

  it('keeps the all-scope key stable when projects are reordered', () => {
    expect(
      resolveWebSessionSidebarProjectIdsKey({
        scope: 'all',
        projectIds: ['project-2', 'project-1', 'project-3'],
      })
    ).toBe('project-1|project-2|project-3');
    expect(
      resolveWebSessionSidebarProjectIdsKey({
        scope: 'all',
        projectIds: ['project-3', 'project-1', 'project-2'],
      })
    ).toBe('project-1|project-2|project-3');
  });

  it('keeps current-scope keys tied to the active project', () => {
    expect(
      resolveWebSessionSidebarProjectIdsKey({ scope: 'current', projectIds: ['project-2'] })
    ).toBe('project-2');
  });
});
