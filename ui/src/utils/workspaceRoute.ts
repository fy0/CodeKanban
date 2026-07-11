import type { LocationQuery, LocationQueryRaw } from 'vue-router';

import { getWebSessionRouteSessionId } from '@/utils/webSessionRoute';

export const WORKSPACE_TAB_QUERY_KEY = 'tab';

type RouteQueryLike = LocationQuery | LocationQueryRaw;

export type WorkspaceRouteTab =
  | 'projects'
  | 'terminal'
  | 'web'
  | 'changes'
  | 'files'
  | 'kanban'
  | 'notifications';

export type DesktopWorkspaceRouteTab = Extract<
  WorkspaceRouteTab,
  'terminal' | 'web' | 'changes' | 'files'
>;
export type MobileWorkspaceRouteTab = Extract<
  WorkspaceRouteTab,
  'projects' | 'terminal' | 'web' | 'files' | 'changes' | 'notifications'
>;

const DESKTOP_WORKSPACE_ROUTE_TAB_SET = new Set<DesktopWorkspaceRouteTab>([
  'terminal',
  'web',
  'changes',
  'files',
]);

const MOBILE_WORKSPACE_ROUTE_TAB_SET = new Set<MobileWorkspaceRouteTab>([
  'projects',
  'terminal',
  'web',
  'files',
  'changes',
  'notifications',
]);

export function normalizeWorkspaceRouteTab(value: unknown): WorkspaceRouteTab | '' {
  if (Array.isArray(value)) {
    for (const item of value) {
      const normalized = normalizeWorkspaceRouteTab(item);
      if (normalized) {
        return normalized;
      }
    }
    return '';
  }

  const normalizedValue = typeof value === 'string' ? value.trim() : '';

  switch (normalizedValue) {
    case 'projects':
    case 'terminal':
    case 'web':
    case 'changes':
    case 'files':
    case 'kanban':
    case 'notifications':
      return normalizedValue as WorkspaceRouteTab;
    default:
      return '';
  }
}

export function getWorkspaceRouteTab(query?: RouteQueryLike | null): WorkspaceRouteTab | '' {
  if (!query) {
    return '';
  }
  return normalizeWorkspaceRouteTab(query[WORKSPACE_TAB_QUERY_KEY]);
}

export function inferWorkspaceRouteTab(query?: RouteQueryLike | null): WorkspaceRouteTab | '' {
  const explicitTab = getWorkspaceRouteTab(query);
  if (explicitTab) {
    return explicitTab;
  }
  return getWebSessionRouteSessionId(query) ? 'web' : '';
}

export function buildWorkspaceRouteQuery(
  query: RouteQueryLike = {},
  tab?: WorkspaceRouteTab | ''
): LocationQueryRaw {
  const nextQuery: LocationQueryRaw = { ...query };
  const normalizedTab = normalizeWorkspaceRouteTab(tab);
  if (normalizedTab) {
    nextQuery[WORKSPACE_TAB_QUERY_KEY] = normalizedTab;
  } else {
    delete nextQuery[WORKSPACE_TAB_QUERY_KEY];
  }
  return nextQuery;
}

export function isWorkspaceRouteTabQuerySynced(
  query?: RouteQueryLike | null,
  tab?: WorkspaceRouteTab | ''
): boolean {
  return getWorkspaceRouteTab(query) === normalizeWorkspaceRouteTab(tab);
}

export function normalizeDesktopWorkspaceRouteTab(value: unknown): DesktopWorkspaceRouteTab {
  const normalized = normalizeWorkspaceRouteTab(value);
  return normalized && DESKTOP_WORKSPACE_ROUTE_TAB_SET.has(normalized as DesktopWorkspaceRouteTab)
    ? (normalized as DesktopWorkspaceRouteTab)
    : 'terminal';
}

export function normalizeMobileWorkspaceRouteTab(value: unknown): MobileWorkspaceRouteTab {
  const normalized = normalizeWorkspaceRouteTab(value);
  return normalized && MOBILE_WORKSPACE_ROUTE_TAB_SET.has(normalized as MobileWorkspaceRouteTab)
    ? (normalized as MobileWorkspaceRouteTab)
    : 'projects';
}

export function resolveDesktopWorkspaceRouteTab(
  query?: RouteQueryLike | null,
  fallback?: unknown
): DesktopWorkspaceRouteTab {
  const requestedTab = inferWorkspaceRouteTab(query);
  if (requestedTab) {
    return normalizeDesktopWorkspaceRouteTab(requestedTab);
  }
  return normalizeDesktopWorkspaceRouteTab(fallback);
}

export function resolveMobileWorkspaceRouteTab(
  query?: RouteQueryLike | null,
  fallback?: unknown
): MobileWorkspaceRouteTab {
  const requestedTab = inferWorkspaceRouteTab(query);
  if (requestedTab) {
    return normalizeMobileWorkspaceRouteTab(requestedTab);
  }
  return normalizeMobileWorkspaceRouteTab(fallback);
}
