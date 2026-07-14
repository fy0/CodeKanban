import { describe, expect, it } from 'vitest';

import { formatBrowserTabTitle } from '@/utils/browserTitle';

describe('formatBrowserTabTitle', () => {
  it('returns the app name when there is no status summary or project name', () => {
    expect(
      formatBrowserTabTitle({
        summary: { working: 0, blocking: 0, unreadCompleted: 0 },
        appName: 'Code Kanban',
      })
    ).toBe('Code Kanban');
  });

  it('returns the legacy status format when only the summary is present', () => {
    expect(
      formatBrowserTabTitle({
        summary: { working: 0, blocking: 1, unreadCompleted: 0 },
        appName: 'Code Kanban',
      })
    ).toBe('[0/1/0] Code Kanban');
  });

  it('returns project name and app name when workspace summary is empty', () => {
    expect(
      formatBrowserTabTitle({
        summary: { working: 0, blocking: 0, unreadCompleted: 0 },
        appName: 'Code Kanban',
        projectName: '标准版',
      })
    ).toBe('标准版 - Code Kanban');
  });

  it('returns summary, project name, and app name for workspace pages', () => {
    expect(
      formatBrowserTabTitle({
        summary: { working: 0, blocking: 1, unreadCompleted: 0 },
        appName: 'Code Kanban',
        projectName: '标准版',
      })
    ).toBe('[0/1/0] - 标准版 - Code Kanban');
  });

  it('replaces only the app name suffix with a custom instance title', () => {
    expect(
      formatBrowserTabTitle({
        summary: { working: 0, blocking: 1, unreadCompleted: 0 },
        appName: 'Staging Board',
        projectName: 'test1',
      })
    ).toBe('[0/1/0] - test1 - Staging Board');
  });

  it('ignores blank project names', () => {
    expect(
      formatBrowserTabTitle({
        summary: { working: 2, blocking: 0, unreadCompleted: 1 },
        appName: 'Code Kanban',
        projectName: '   ',
      })
    ).toBe('[2/0/1] Code Kanban');
  });
});
