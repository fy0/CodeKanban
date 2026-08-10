import { describe, expect, it } from 'vitest';

import { resolveWorkspaceShortcutTarget } from '@/utils/workspaceTabShortcut';

describe('workspaceTabShortcut', () => {
  it('switches back to the previous visited tab when it differs from the current tab', () => {
    expect(resolveWorkspaceShortcutTarget('terminal', 'files')).toBe('files');
    expect(resolveWorkspaceShortcutTarget('web', 'files')).toBe('files');
  });

  it('falls back to web when only one non-web tab has been visited', () => {
    expect(resolveWorkspaceShortcutTarget('terminal', null)).toBe('web');
    expect(resolveWorkspaceShortcutTarget('files', null)).toBe('web');
    expect(resolveWorkspaceShortcutTarget('terminal', 'terminal')).toBe('web');
  });

  it('falls back to terminal when web is the only visited tab', () => {
    expect(resolveWorkspaceShortcutTarget('web', null)).toBe('terminal');
    expect(resolveWorkspaceShortcutTarget('web', 'web')).toBe('terminal');
  });
});
