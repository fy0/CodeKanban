import { describe, expect, it } from 'vitest';

import {
  buildWebSessionClearedWorkspaceRouteQuery,
  buildWebSessionProjectLocation,
  buildWebSessionRouteQuery,
  getWebSessionRouteSessionId,
  isWebSessionRouteActivationCurrent,
  isWebSessionRouteQuerySynced,
  isWebSessionOnlyRouteChange,
  resolveWebSessionDeepLinkTarget,
  shouldPreserveWebSessionRouteSessionId,
} from '@/utils/webSessionRoute';

describe('webSessionRoute', () => {
  it('reads the first valid webSessionId from the route query', () => {
    expect(getWebSessionRouteSessionId({ webSessionId: ' session-1 ' })).toBe('session-1');
    expect(getWebSessionRouteSessionId({ webSessionId: ['', 'session-2'] })).toBe('session-2');
    expect(getWebSessionRouteSessionId({ webSessionId: null })).toBe('');
  });

  it('builds route queries while preserving unrelated query parameters', () => {
    expect(
      buildWebSessionRouteQuery(
        {
          filter: 'active',
          tab: 'web',
          webSessionId: 'old-session',
        },
        'new-session'
      )
    ).toEqual({
      filter: 'active',
      tab: 'web',
      webSessionId: 'new-session',
    });
  });

  it('clears webSessionId without removing unrelated query parameters', () => {
    expect(
      buildWebSessionRouteQuery({
        filter: 'archived',
        tab: 'terminal',
        webSessionId: 'session-1',
      })
    ).toEqual({
      filter: 'archived',
      tab: 'terminal',
    });
  });

  it('builds workspace project switch queries without carrying stale webSessionId', () => {
    expect(
      buildWebSessionClearedWorkspaceRouteQuery(
        {
          filter: 'active',
          tab: 'web',
          webSessionId: 'session-1',
        },
        'terminal'
      )
    ).toEqual({
      filter: 'active',
      tab: 'terminal',
    });
  });

  it('builds a project route location for jumping into a web session', () => {
    expect(
      buildWebSessionProjectLocation({
        projectId: ' project-2 ',
        sessionId: ' session-9 ',
        query: {
          filter: 'active',
          tab: 'notifications',
          webSessionId: 'old-session',
        },
      })
    ).toEqual({
      name: 'project',
      params: { id: 'project-2' },
      query: {
        filter: 'active',
        tab: 'web',
        webSessionId: 'session-9',
      },
    });
  });

  it('returns null when the notification route target is incomplete', () => {
    expect(
      buildWebSessionProjectLocation({
        projectId: 'project-1',
        sessionId: '',
        query: {
          tab: 'files',
        },
      })
    ).toBeNull();

    expect(
      buildWebSessionProjectLocation({
        projectId: '',
        sessionId: 'session-1',
      })
    ).toBeNull();
  });

  it('compares the current query and target session id after normalization', () => {
    expect(isWebSessionRouteQuerySynced({ webSessionId: ' session-1 ' }, 'session-1')).toBe(true);
    expect(isWebSessionRouteQuerySynced({ webSessionId: 'session-1' }, 'session-2')).toBe(false);
  });

  it('preserves explicit web deep links while a different remembered session is selected', () => {
    expect(
      shouldPreserveWebSessionRouteSessionId({
        workspaceTab: 'web',
        pendingRouteSessionId: 'session-b',
        currentProjectId: 'project-1',
        currentSessionId: 'session-a',
        currentSessionProjectId: 'project-1',
      })
    ).toBe(true);
  });

  it('preserves explicit web deep links while no real session is active yet', () => {
    expect(
      shouldPreserveWebSessionRouteSessionId({
        workspaceTab: 'web',
        pendingRouteSessionId: 'session-b',
        currentProjectId: 'project-1',
      })
    ).toBe(true);

    expect(
      shouldPreserveWebSessionRouteSessionId({
        workspaceTab: 'web',
        pendingRouteSessionId: 'session-b',
        currentProjectId: 'project-1',
        currentSessionId: 'draft-1',
        currentSessionIsDraft: true,
      })
    ).toBe(true);
  });

  it('allows route sync once the requested session is active, including archived previews', () => {
    expect(
      shouldPreserveWebSessionRouteSessionId({
        workspaceTab: 'web',
        pendingRouteSessionId: 'session-b',
        currentProjectId: 'project-1',
        currentSessionId: 'session-b',
        currentSessionProjectId: 'project-1',
      })
    ).toBe(false);
  });

  it('keeps preserving the deep link when the current session belongs to a different project', () => {
    expect(
      shouldPreserveWebSessionRouteSessionId({
        workspaceTab: 'web',
        pendingRouteSessionId: 'session-b',
        currentProjectId: 'project-1',
        currentSessionId: 'session-b',
        currentSessionProjectId: 'project-2',
      })
    ).toBe(true);
  });

  it('does not preserve the route when there is no pending route-driven activation', () => {
    expect(
      shouldPreserveWebSessionRouteSessionId({
        workspaceTab: 'web',
        currentProjectId: 'project-1',
        currentSessionId: 'session-a',
        currentSessionProjectId: 'project-1',
      })
    ).toBe(false);
  });

  it('does not preserve a webSessionId when the workspace is not on the web tab', () => {
    expect(
      shouldPreserveWebSessionRouteSessionId({
        workspaceTab: 'terminal',
        pendingRouteSessionId: 'session-b',
        currentProjectId: 'project-1',
        currentSessionId: 'session-a',
        currentSessionProjectId: 'project-1',
      })
    ).toBe(false);
  });

  it('accepts route-driven activation only while project and session still match the route', () => {
    expect(
      isWebSessionRouteActivationCurrent({
        currentProjectId: 'project-1',
        routeProjectId: 'project-1',
        requestedProjectId: 'project-1',
        routeSessionId: 'session-1',
        requestedSessionId: 'session-1',
      })
    ).toBe(true);

    expect(
      isWebSessionRouteActivationCurrent({
        currentProjectId: 'project-2',
        routeProjectId: 'project-2',
        requestedProjectId: 'project-1',
        routeSessionId: 'session-1',
        requestedSessionId: 'session-1',
      })
    ).toBe(false);

    expect(
      isWebSessionRouteActivationCurrent({
        currentProjectId: 'project-1',
        routeProjectId: 'project-1',
        requestedProjectId: 'project-1',
        routeSessionId: 'session-2',
        requestedSessionId: 'session-1',
      })
    ).toBe(false);
  });

  it('detects query-only session changes on the same route', () => {
    expect(
      isWebSessionOnlyRouteChange(
        {
          name: 'project',
          path: '/project/project-1',
          params: { id: 'project-1' },
          query: { webSessionId: 'session-1', filter: 'active' },
        },
        {
          name: 'project',
          path: '/project/project-1',
          params: { id: 'project-1' },
          query: { webSessionId: 'session-2', filter: 'active' },
        }
      )
    ).toBe(true);
  });

  it('does not treat project switches or other query changes as silent session changes', () => {
    expect(
      isWebSessionOnlyRouteChange(
        {
          name: 'project',
          path: '/project/project-1',
          params: { id: 'project-1' },
          query: { webSessionId: 'session-1', filter: 'active' },
        },
        {
          name: 'project',
          path: '/project/project-2',
          params: { id: 'project-2' },
          query: { webSessionId: 'session-2', filter: 'active' },
        }
      )
    ).toBe(false);

    expect(
      isWebSessionOnlyRouteChange(
        {
          name: 'project',
          path: '/project/project-1',
          params: { id: 'project-1' },
          query: { webSessionId: 'session-1', filter: 'active' },
        },
        {
          name: 'project',
          path: '/project/project-1',
          params: { id: 'project-1' },
          query: { webSessionId: 'session-2', filter: 'archived' },
        }
      )
    ).toBe(false);
  });

  it('activates a loaded session without requesting an extra snapshot', () => {
    expect(
      resolveWebSessionDeepLinkTarget({
        currentProjectId: 'project-1',
        requestedSessionId: 'session-1',
        loadedSessions: [{ id: 'session-1' }],
      })
    ).toEqual({
      action: 'activate-loaded',
      sessionId: 'session-1',
    });
  });

  it('opens an archived preview when the snapshot belongs to the current project', () => {
    expect(
      resolveWebSessionDeepLinkTarget({
        currentProjectId: 'project-1',
        requestedSessionId: 'session-2',
        loadedSessions: [],
        snapshotSession: {
          id: 'session-2',
          projectId: 'project-1',
          archivedAt: '2026-04-11T10:00:00.000Z',
        },
      })
    ).toEqual({
      action: 'open-archived-preview',
      sessionId: 'session-2',
    });
  });

  it('activates a live session returned by snapshot loading', () => {
    expect(
      resolveWebSessionDeepLinkTarget({
        currentProjectId: 'project-1',
        requestedSessionId: 'session-3',
        loadedSessions: [],
        snapshotSession: {
          id: 'session-3',
          projectId: 'project-1',
          archivedAt: null,
        },
      })
    ).toEqual({
      action: 'activate-real',
      sessionId: 'session-3',
    });
  });

  it('clears invalid deep links when the snapshot is missing or mismatched', () => {
    expect(
      resolveWebSessionDeepLinkTarget({
        currentProjectId: 'project-1',
        requestedSessionId: 'session-4',
        loadedSessions: [],
        snapshotSession: null,
      })
    ).toEqual({
      action: 'clear-invalid',
    });

    expect(
      resolveWebSessionDeepLinkTarget({
        currentProjectId: 'project-1',
        requestedSessionId: 'session-4',
        loadedSessions: [],
        snapshotSession: {
          id: 'session-4',
          projectId: 'project-2',
          archivedAt: null,
        },
      })
    ).toEqual({
      action: 'clear-invalid',
    });
  });
});
