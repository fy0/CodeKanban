import type { WebSessionSummary } from '@/types/models';
import type { ProjectBadge } from '@/utils/projectBadge';

export type CrossProjectSessionItem = {
  session: WebSessionSummary;
  projectId: string;
  projectName: string;
  isCurrent: boolean;
  projectBadge?: ProjectBadge;
};
