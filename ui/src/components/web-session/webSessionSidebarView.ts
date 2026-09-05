import type { WebSessionSummary } from '@/types/models';
import type { ProjectBadge } from '@/utils/projectBadge';
import { resolveWebSessionSidebarSortTimestamp } from './webSessionSessionState';

export function collectWebSessionSidebarSessions(
  projectIds: string[],
  getSessions: (projectId: string) => WebSessionSummary[]
): WebSessionSummary[] {
  return [...new Set(projectIds)]
    .flatMap(projectId => getSessions(projectId))
    .filter(session => !session.archivedAt)
    .sort((left, right) => {
      const timestampDifference =
        resolveWebSessionSidebarSortTimestamp(right) - resolveWebSessionSidebarSortTimestamp(left);
      return (
        timestampDifference || left.orderIndex - right.orderIndex || left.id.localeCompare(right.id)
      );
    });
}

export type CrossProjectSessionItem = {
  session: WebSessionSummary;
  projectId: string;
  projectName: string;
  isCurrent: boolean;
  projectBadge?: ProjectBadge;
};
