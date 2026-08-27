import type { WebSessionSummary } from '@/types/models';
import type { ProjectBadge } from '@/utils/projectBadge';
import type { WebSessionMobileTabDescriptor } from '@/components/web-session/webSessionMobileTabOptions';

export type DraftSessionTab = WebSessionSummary & {
  isDraft: true;
};

export type ArchivedPreviewSessionTab = WebSessionSummary & {
  isArchivedPreview: true;
};

export type SessionTab =
  | (WebSessionSummary & { isDraft?: false; isArchivedPreview?: false })
  | DraftSessionTab
  | ArchivedPreviewSessionTab;

export type MobileTabListDescriptor = Exclude<
  WebSessionMobileTabDescriptor<SessionTab>,
  { kind: 'header' }
>;

export type MobileSessionDrawerView = {
  rowClass: Record<string, boolean>;
  statusTooltip: string;
  agentBadgeClass: string;
  assistantIcon: string;
  showStatusDot: boolean;
  statusDotClass?: string;
  showWorkflowPlanBadge: boolean;
  scheduledPlan: boolean;
  showScheduledInput: boolean;
  projectBadge: ProjectBadge | null;
  projectName: string;
  timeLabel: string;
  timeTitle: string;
};

export function isDraftSession(session: SessionTab | null | undefined): session is DraftSessionTab {
  return Boolean(session && 'isDraft' in session && session.isDraft);
}

export function isArchivedPreviewSession(
  session: SessionTab | null | undefined
): session is ArchivedPreviewSessionTab {
  return Boolean(session && 'isArchivedPreview' in session && session.isArchivedPreview);
}
