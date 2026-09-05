import type { WebSessionSummary } from '@/types/models';

type SearchableWebSession = Pick<WebSessionSummary, 'title' | 'threadPreview'>;
export type WebSessionSearchMatchSource = 'title' | 'body';

export function normalizeWebSessionSidebarSearchQuery(value: unknown) {
  return String(value ?? '')
    .trim()
    .toLocaleLowerCase();
}

export function resolveWebSessionSidebarSearchMatchSources(
  session: SearchableWebSession,
  normalizedQuery: string,
  includeBody = true
): WebSessionSearchMatchSource[] {
  if (!normalizedQuery) {
    return [];
  }
  const sources: WebSessionSearchMatchSource[] = [];
  if (
    String(session.title ?? '')
      .toLocaleLowerCase()
      .includes(normalizedQuery)
  ) {
    sources.push('title');
  }
  if (
    includeBody &&
    String(session.threadPreview ?? '')
      .toLocaleLowerCase()
      .includes(normalizedQuery)
  ) {
    sources.push('body');
  }
  return sources;
}

export function mergeWebSessionSearchMatchSources(
  ...sourceGroups: Array<ReadonlyArray<WebSessionSearchMatchSource> | null | undefined>
) {
  const sources = new Set(sourceGroups.flatMap(group => group ?? []));
  return (['title', 'body'] as const).filter(source => sources.has(source));
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
