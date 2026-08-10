import { describe, expect, it } from 'vitest';

import {
  DEFAULT_MOBILE_VIEW,
  formatMobileNavBadge,
  mobileViewToRouteTab,
  normalizeMobileView,
  resolveMobileProjectSelectionAction,
  resolveMobileProjectSourceViewChange,
  routeTabToMobileView,
  restorePersistedMobileView,
} from '@/views/projectWorkspaceMobileView';

describe('projectWorkspaceMobileView', () => {
  it('formats compact mobile navigation badges', () => {
    expect(formatMobileNavBadge(0)).toBe('');
    expect(formatMobileNavBadge(7)).toBe('7');
    expect(formatMobileNavBadge(99)).toBe('99');
    expect(formatMobileNavBadge(100)).toBe('99+');
    expect(formatMobileNavBadge('invalid')).toBe('');
  });

  it('keeps visible mobile views unchanged', () => {
    expect(normalizeMobileView('projects')).toBe('projects');
    expect(normalizeMobileView('terminal')).toBe('terminal');
    expect(normalizeMobileView('webSession')).toBe('webSession');
    expect(normalizeMobileView('files')).toBe('files');
    expect(normalizeMobileView('changes')).toBe('changes');
  });

  it('blocks kanban and falls back invalid values to the default mobile view', () => {
    expect(normalizeMobileView('kanban')).toBe(DEFAULT_MOBILE_VIEW);
    expect(normalizeMobileView('unknown')).toBe(DEFAULT_MOBILE_VIEW);
    expect(normalizeMobileView(null)).toBe(DEFAULT_MOBILE_VIEW);
    expect(normalizeMobileView(undefined)).toBe(DEFAULT_MOBILE_VIEW);
  });

  it('maps hidden persisted views back to projects', () => {
    expect(restorePersistedMobileView('notifications')).toBe(DEFAULT_MOBILE_VIEW);
    expect(restorePersistedMobileView('kanban')).toBe(DEFAULT_MOBILE_VIEW);
  });

  it('keeps non-hidden persisted views unchanged', () => {
    expect(restorePersistedMobileView('projects')).toBe('projects');
    expect(restorePersistedMobileView('terminal')).toBe('terminal');
    expect(restorePersistedMobileView('webSession')).toBe('webSession');
    expect(restorePersistedMobileView('files')).toBe('files');
    expect(restorePersistedMobileView('changes')).toBe('changes');
  });

  it('maps mobile views to route tabs', () => {
    expect(mobileViewToRouteTab('projects')).toBe('projects');
    expect(mobileViewToRouteTab('terminal')).toBe('terminal');
    expect(mobileViewToRouteTab('webSession')).toBe('web');
    expect(mobileViewToRouteTab('files')).toBe('files');
    expect(mobileViewToRouteTab('changes')).toBe('changes');
    expect(mobileViewToRouteTab('kanban')).toBe('projects');
  });

  it('maps route tabs back to visible mobile views', () => {
    expect(routeTabToMobileView('projects')).toBe('projects');
    expect(routeTabToMobileView('terminal')).toBe('terminal');
    expect(routeTabToMobileView('web')).toBe('webSession');
    expect(routeTabToMobileView('files')).toBe('files');
    expect(routeTabToMobileView('changes')).toBe('changes');
    expect(routeTabToMobileView('kanban')).toBe(DEFAULT_MOBILE_VIEW);
    expect(routeTabToMobileView('unknown')).toBe(DEFAULT_MOBILE_VIEW);
  });

  it('records the previous panel when entering the mobile projects view', () => {
    expect(
      resolveMobileProjectSourceViewChange({
        previousView: 'terminal',
        nextView: 'projects',
      })
    ).toBe('terminal');
    expect(
      resolveMobileProjectSourceViewChange({
        previousView: 'webSession',
        nextView: 'projects',
      })
    ).toBe('webSession');
  });

  it('keeps an existing mobile project source while staying in projects', () => {
    expect(
      resolveMobileProjectSourceViewChange({
        previousView: 'projects',
        nextView: 'projects',
        currentSource: 'terminal',
      })
    ).toBe('terminal');
  });

  it('clears the mobile project source when leaving projects', () => {
    expect(
      resolveMobileProjectSourceViewChange({
        previousView: 'projects',
        nextView: 'terminal',
        currentSource: 'webSession',
      })
    ).toBe('');
  });

  it('does not create a mobile project source from invalid or hidden views', () => {
    expect(
      resolveMobileProjectSourceViewChange({
        previousView: 'projects',
        nextView: 'projects',
      })
    ).toBe('');
    expect(
      resolveMobileProjectSourceViewChange({
        previousView: 'kanban',
        nextView: 'projects',
      })
    ).toBe('');
  });

  it('opens web sessions when selecting a project without a previous workspace view', () => {
    expect(resolveMobileProjectSelectionAction('')).toEqual({
      type: 'navigate',
      targetView: 'webSession',
    });
    expect(resolveMobileProjectSelectionAction('projects')).toEqual({
      type: 'navigate',
      targetView: 'webSession',
    });
  });

  it('preserves the return prompt when project selection came from another view', () => {
    expect(resolveMobileProjectSelectionAction('terminal')).toEqual({
      type: 'prompt-return',
      sourceView: 'terminal',
    });
    expect(resolveMobileProjectSelectionAction('webSession')).toEqual({
      type: 'prompt-return',
      sourceView: 'webSession',
    });
  });
});
