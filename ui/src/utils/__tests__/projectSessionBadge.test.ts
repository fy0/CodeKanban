import { describe, expect, it } from 'vitest';

import {
  resolvePreferredProjectSessionKind,
  resolveProjectSessionBadge,
} from '@/utils/projectSessionBadge';

describe('projectSessionBadge', () => {
  it('returns terminal when only terminals exist', () => {
    expect(
      resolveProjectSessionBadge({
        terminalCount: 2,
        webSessionCount: 0,
        preferredKind: 'webSession',
      })
    ).toEqual({ kind: 'terminal', count: 2 });
  });

  it('returns web session when only web sessions exist', () => {
    expect(
      resolveProjectSessionBadge({
        terminalCount: 0,
        webSessionCount: 3,
        preferredKind: 'terminal',
      })
    ).toEqual({ kind: 'webSession', count: 3 });
  });

  it('returns combined when both terminal and web sessions exist', () => {
    expect(
      resolveProjectSessionBadge({
        terminalCount: 4,
        webSessionCount: 2,
        preferredKind: 'terminal',
      })
    ).toEqual({
      kind: 'combined',
      terminalCount: 4,
      webSessionCount: 2,
    });
  });

  it('returns combined regardless of preferred kind when both types exist', () => {
    expect(
      resolveProjectSessionBadge({
        terminalCount: 4,
        webSessionCount: 2,
        preferredKind: 'webSession',
      })
    ).toEqual({
      kind: 'combined',
      terminalCount: 4,
      webSessionCount: 2,
    });
  });

  it('returns null when neither type exists', () => {
    expect(
      resolveProjectSessionBadge({
        terminalCount: 0,
        webSessionCount: 0,
        preferredKind: 'terminal',
      })
    ).toBeNull();
  });

  it('normalizes invalid counts before resolving the badge state', () => {
    expect(
      resolveProjectSessionBadge({
        terminalCount: Number.NaN,
        webSessionCount: 2.8,
        preferredKind: 'terminal',
      })
    ).toEqual({ kind: 'webSession', count: 2 });
    expect(
      resolveProjectSessionBadge({
        terminalCount: -3,
        webSessionCount: Number.POSITIVE_INFINITY,
        preferredKind: 'webSession',
      })
    ).toBeNull();
  });

  it('defaults to terminal when current mobile view is not web session', () => {
    expect(
      resolvePreferredProjectSessionKind({
        isMobile: true,
        isProjectWorkspace: false,
        mobileActiveView: 'projects',
      })
    ).toBe('terminal');
  });

  it('returns web session when the current mobile view is web session', () => {
    expect(
      resolvePreferredProjectSessionKind({
        isMobile: true,
        isProjectWorkspace: false,
        mobileActiveView: 'webSession',
      })
    ).toBe('webSession');
  });

  it('returns web session when the active workspace tab is web', () => {
    expect(
      resolvePreferredProjectSessionKind({
        isMobile: false,
        isProjectWorkspace: true,
        workspaceActiveTab: 'web',
      })
    ).toBe('webSession');
  });

  it('defaults to terminal for non-web workspace tabs and non-project pages', () => {
    expect(
      resolvePreferredProjectSessionKind({
        isMobile: false,
        isProjectWorkspace: true,
        workspaceActiveTab: 'files',
      })
    ).toBe('terminal');
    expect(
      resolvePreferredProjectSessionKind({
        isMobile: false,
        isProjectWorkspace: false,
        workspaceActiveTab: 'web',
      })
    ).toBe('terminal');
    expect(
      resolvePreferredProjectSessionKind({
        isMobile: false,
        isProjectWorkspace: false,
      })
    ).toBe('terminal');
  });
});
