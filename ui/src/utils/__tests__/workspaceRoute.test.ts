import { describe, expect, it } from 'vitest';

import {
  buildWorkspaceRouteQuery,
  getWorkspaceRouteTab,
  inferWorkspaceRouteTab,
  isWorkspaceRouteTabQuerySynced,
  normalizeDesktopWorkspaceRouteTab,
  normalizeMobileWorkspaceRouteTab,
  resolveDesktopWorkspaceRouteTab,
  resolveMobileWorkspaceRouteTab,
} from '@/utils/workspaceRoute';

describe('workspaceRoute', () => {
  it('reads the first valid workspace tab from the route query', () => {
    expect(getWorkspaceRouteTab({ tab: ' web ' })).toBe('web');
    expect(getWorkspaceRouteTab({ tab: ['', 'files'] })).toBe('files');
    expect(getWorkspaceRouteTab({ tab: ['bad', 'changes'] })).toBe('changes');
    expect(getWorkspaceRouteTab({ tab: null })).toBe('');
  });

  it('infers web when a legacy webSessionId deep link is present', () => {
    expect(inferWorkspaceRouteTab({ webSessionId: 'session-1' })).toBe('web');
    expect(inferWorkspaceRouteTab({ tab: 'terminal', webSessionId: 'session-1' })).toBe('terminal');
  });

  it('builds route queries while preserving unrelated query parameters', () => {
    expect(
      buildWorkspaceRouteQuery(
        {
          filter: 'active',
          tab: 'terminal',
        },
        'web'
      )
    ).toEqual({
      filter: 'active',
      tab: 'web',
    });
  });

  it('clears tab without removing unrelated query parameters', () => {
    expect(
      buildWorkspaceRouteQuery({
        filter: 'active',
        tab: 'web',
      })
    ).toEqual({
      filter: 'active',
    });
  });

  it('compares the current query and target tab after normalization', () => {
    expect(isWorkspaceRouteTabQuerySynced({ tab: ' web ' }, 'web')).toBe(true);
    expect(isWorkspaceRouteTabQuerySynced({ tab: 'web' }, 'terminal')).toBe(false);
  });

  it('normalizes invalid desktop and mobile tabs to their defaults', () => {
    expect(normalizeDesktopWorkspaceRouteTab('projects')).toBe('terminal');
    expect(normalizeDesktopWorkspaceRouteTab('kanban')).toBe('terminal');
    expect(normalizeMobileWorkspaceRouteTab('kanban')).toBe('projects');
  });

  it('resolves desktop tabs from explicit query values, legacy deep links, and fallback state', () => {
    expect(resolveDesktopWorkspaceRouteTab({ tab: 'files' }, 'terminal')).toBe('files');
    expect(resolveDesktopWorkspaceRouteTab({ tab: 'changes' }, 'terminal')).toBe('changes');
    expect(resolveDesktopWorkspaceRouteTab({ tab: 'kanban' }, 'web')).toBe('terminal');
    expect(resolveDesktopWorkspaceRouteTab({ webSessionId: 'session-1' }, 'terminal')).toBe('web');
    expect(resolveDesktopWorkspaceRouteTab({}, 'web')).toBe('web');
    expect(resolveDesktopWorkspaceRouteTab({}, undefined)).toBe('web');
  });

  it('resolves mobile tabs with a projects fallback for unsupported values', () => {
    expect(resolveMobileWorkspaceRouteTab({ tab: 'changes' }, 'terminal')).toBe('changes');
    expect(resolveMobileWorkspaceRouteTab({ tab: 'notifications' }, 'terminal')).toBe(
      'notifications'
    );
    expect(resolveMobileWorkspaceRouteTab({ tab: 'kanban' }, 'web')).toBe('projects');
    expect(resolveMobileWorkspaceRouteTab({}, 'web')).toBe('web');
  });
});
