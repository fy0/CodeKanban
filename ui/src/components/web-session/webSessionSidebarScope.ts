export type WebSessionSidebarScope = 'all' | 'current';

function normalizeProjectId(value: unknown) {
  return String(value || '').trim();
}

export function normalizeWebSessionSidebarScope(value: unknown): WebSessionSidebarScope {
  return value === 'current' ? 'current' : 'all';
}

export function resolveWebSessionSidebarToggleScope(scope: unknown): WebSessionSidebarScope {
  return normalizeWebSessionSidebarScope(scope) === 'current' ? 'all' : 'current';
}

export function resolveWebSessionSidebarProjectIds(input: {
  scope: WebSessionSidebarScope;
  currentProjectId?: string | null;
  allProjectIds: string[];
}) {
  const currentProjectId = normalizeProjectId(input.currentProjectId);
  if (normalizeWebSessionSidebarScope(input.scope) === 'current') {
    return currentProjectId ? [currentProjectId] : [];
  }

  const ordered: string[] = [];
  input.allProjectIds.forEach(projectId => {
    const normalizedProjectId = normalizeProjectId(projectId);
    if (normalizedProjectId && !ordered.includes(normalizedProjectId)) {
      ordered.push(normalizedProjectId);
    }
  });
  return ordered;
}

/**
 * Returns a key for operations whose result depends on the project set rather
 * than its presentation order. All-scope lists intentionally put the active
 * project first, so that order changes when the user switches sessions across
 * projects even though the search scope is unchanged.
 */
export function resolveWebSessionSidebarProjectIdsKey(input: {
  scope: WebSessionSidebarScope;
  projectIds: readonly string[];
}) {
  const normalized = input.projectIds
    .map(projectId => normalizeProjectId(projectId))
    .filter((projectId, index, values) => projectId && values.indexOf(projectId) === index);
  if (normalizeWebSessionSidebarScope(input.scope) === 'all') {
    normalized.sort();
  }
  return normalized.join('|');
}
