import type { WebSessionSummary } from '@/types/models';

type SearchableWebSession = Pick<WebSessionSummary, 'title' | 'threadPreview'>;
export type WebSessionSearchMatchSource = 'title' | 'body';

export type WebSessionSidebarSearchKeyInput = {
  query: unknown;
  projectIds: readonly unknown[];
  includeArchived?: boolean;
  includeBody?: boolean;
};

export function normalizeWebSessionSidebarSearchQuery(value: unknown) {
  return String(value ?? '')
    .trim()
    .toLocaleLowerCase();
}

export function normalizeWebSessionSidebarSearchProjectIds(projectIds: readonly unknown[]) {
  return Array.from(
    new Set(projectIds.map(projectId => String(projectId ?? '').trim()).filter(Boolean))
  ).sort((left, right) => left.localeCompare(right));
}

export function buildWebSessionSidebarSearchKey(input: WebSessionSidebarSearchKeyInput) {
  return JSON.stringify({
    query: normalizeWebSessionSidebarSearchQuery(input.query),
    projectIds: normalizeWebSessionSidebarSearchProjectIds(input.projectIds),
    includeArchived: input.includeArchived === true,
    includeBody: input.includeBody !== false,
  });
}

export function matchesWebSessionSidebarSearch(
  session: SearchableWebSession,
  normalizedQuery: string,
  includeBody = true
) {
  if (!normalizedQuery) {
    return true;
  }
  return (
    resolveWebSessionSidebarSearchMatchSources(session, normalizedQuery, includeBody).length > 0
  );
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
