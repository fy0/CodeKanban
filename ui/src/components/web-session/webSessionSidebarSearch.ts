import type { WebSessionSummary } from '@/types/models';

type SearchableWebSession = Pick<WebSessionSummary, 'title' | 'threadPreview'>;

export function normalizeWebSessionSidebarSearchQuery(value: unknown) {
  return String(value ?? '')
    .trim()
    .toLocaleLowerCase();
}

export function matchesWebSessionSidebarSearch(
  session: SearchableWebSession,
  normalizedQuery: string
) {
  if (!normalizedQuery) {
    return true;
  }
  return [session.title, session.threadPreview].some(value =>
    String(value ?? '')
      .toLocaleLowerCase()
      .includes(normalizedQuery)
  );
}

export function mergeWebSessionSidebarSearchPage(
  current: WebSessionSummary[],
  incoming: WebSessionSummary[]
) {
  const seen = new Set<string>();
  return [...current, ...incoming].filter(session => {
    if (!session.id || seen.has(session.id)) {
      return false;
    }
    seen.add(session.id);
    return true;
  });
}
