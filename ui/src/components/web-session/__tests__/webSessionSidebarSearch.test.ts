import { describe, expect, it } from 'vitest';

import {
  mergeWebSessionSearchMatchSources,
  mergeWebSessionSidebarSearchPage,
  normalizeWebSessionSidebarSearchQuery,
  resolveWebSessionSidebarSearchMatchSources,
} from '@/components/web-session/webSessionSidebarSearch';
import type { WebSessionSummary } from '@/types/models';

describe('webSession sidebar search', () => {
  it('normalizes surrounding whitespace and letter case', () => {
    expect(normalizeWebSessionSidebarSearchQuery('  Release NOTES  ')).toBe('release notes');
    expect(normalizeWebSessionSidebarSearchQuery(null)).toBe('');
  });

  it('reports title and body match sources in display order', () => {
    const session = {
      title: 'Migration checklist',
      threadPreview: 'Investigate the migration failure',
    };

    expect(resolveWebSessionSidebarSearchMatchSources(session, 'migration')).toEqual([
      'title',
      'body',
    ]);
    expect(resolveWebSessionSidebarSearchMatchSources(session, 'migration', false)).toEqual([
      'title',
    ]);
    expect(mergeWebSessionSearchMatchSources(['body'], ['title', 'body'])).toEqual([
      'title',
      'body',
    ]);
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
