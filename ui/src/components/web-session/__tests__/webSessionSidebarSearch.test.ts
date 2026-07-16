import { describe, expect, it } from 'vitest';

import {
  matchesWebSessionSidebarSearch,
  mergeWebSessionSidebarSearchPage,
  normalizeWebSessionSidebarSearchQuery,
} from '@/components/web-session/webSessionSidebarSearch';
import type { WebSessionSummary } from '@/types/models';

describe('webSession sidebar search', () => {
  it('normalizes surrounding whitespace and letter case', () => {
    expect(normalizeWebSessionSidebarSearchQuery('  Release NOTES  ')).toBe('release notes');
    expect(normalizeWebSessionSidebarSearchQuery(null)).toBe('');
  });

  it('matches session titles and thread previews', () => {
    const session = {
      title: 'Release checklist',
      threadPreview: 'Investigate the migration failure',
    };

    expect(matchesWebSessionSidebarSearch(session, 'release')).toBe(true);
    expect(matchesWebSessionSidebarSearch(session, 'MIGRATION'.toLocaleLowerCase())).toBe(true);
    expect(matchesWebSessionSidebarSearch(session, 'missing')).toBe(false);
    expect(matchesWebSessionSidebarSearch(session, '')).toBe(true);
  });

  it('merges archived search pages without duplicating sessions', () => {
    const session = (id: string) => ({ id }) as WebSessionSummary;

    expect(
      mergeWebSessionSidebarSearchPage(
        [session('session-1'), session('session-2')],
        [session('session-2'), session('session-3')]
      ).map(item => item.id)
    ).toEqual(['session-1', 'session-2', 'session-3']);
  });
});
