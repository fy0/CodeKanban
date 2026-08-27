import type { LocationQuery, LocationQueryRaw, RouteLocationRaw } from 'vue-router';

import type { WorkspaceRouteTab } from '@/utils/workspaceRoute';
import { buildWorkspaceRouteQuery } from '@/utils/workspaceRoute';
import { buildWebSessionRouteQuery } from '@/utils/webSessionRoute';

type RouteQueryLike = LocationQuery | LocationQueryRaw;

export type ProjectBrowserMode = 'page' | 'mobile-workspace';

export function isCurrentProjectSelection(
  currentProjectId: string | null | undefined,
  targetProjectId: string
): boolean {
  const normalizedCurrentProjectId =
    typeof currentProjectId === 'string' ? currentProjectId.trim() : '';
  const normalizedTargetProjectId = targetProjectId.trim();

  return (
    Boolean(normalizedCurrentProjectId) && normalizedCurrentProjectId === normalizedTargetProjectId
  );
}

export function buildProjectBrowserRouteQuery(
  query?: RouteQueryLike | null,
  workspaceTab?: WorkspaceRouteTab | ''
): LocationQueryRaw {
  return buildWebSessionRouteQuery(buildWorkspaceRouteQuery(query ?? undefined, workspaceTab));
}

export function buildProjectBrowserProjectLocation(options: {
  mode: ProjectBrowserMode;
  projectId: string;
  currentProjectId?: string | null;
  query?: RouteQueryLike | null;
  workspaceTab?: WorkspaceRouteTab | '';
}): RouteLocationRaw | null {
  const normalizedProjectId = options.projectId.trim();
  if (!normalizedProjectId) {
    return null;
  }

  if (isCurrentProjectSelection(options.currentProjectId, normalizedProjectId)) {
    return null;
  }

  if (options.mode === 'mobile-workspace') {
    return {
      name: 'project' as const,
      params: { id: normalizedProjectId },
      query: buildProjectBrowserRouteQuery(options.query, options.workspaceTab),
    };
  }

  return {
    name: 'project' as const,
    params: { id: normalizedProjectId },
  };
}
