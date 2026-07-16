import type { ProjectBadge } from '@/utils/projectBadge';

export type WebSessionSidebarRowView = {
  key: string;
  sessionId: string;
  title: string;
  iconHtml: string;
  subtitle: string;
  tooltip: string;
  accentColor: string;
  toneClass: string;
  active: boolean;
  archived: boolean;
  archiving: boolean;
  hasWorkflowPlanBadge: boolean;
  singleProject: boolean;
  projectBadge?: ProjectBadge;
  currentIndicatorTitle: string;
  activityTimeLabel: string;
  activityTimeTitle: string;
  moreActionsLabel?: string;
};

export type WebSessionSidebarSessionEntry<T> = {
  source: T;
  row: WebSessionSidebarRowView;
};

export type WebSessionSidebarSection<T> = {
  key: string;
  label: string;
  entries: WebSessionSidebarSessionEntry<T>[];
};

export type WebSessionSidebarDateGroupLabels = {
  today: string;
  yesterday: string;
  lastSevenDays: string;
  earlier: string;
};

export type WebSessionSidebarVirtualItem<T> =
  | {
      key: string;
      type: 'section';
      label: string;
      count: number;
      separated: boolean;
    }
  | {
      key: string;
      type: 'empty';
      label: string;
    }
  | {
      key: string;
      type: 'session';
      entry: WebSessionSidebarSessionEntry<T>;
    }
  | {
      key: string;
      type: 'action';
      label: string;
      disabled: boolean;
    };

type BuildWebSessionSidebarVirtualItemsOptions<T> = {
  currentSections: WebSessionSidebarSection<T>[];
  archived: WebSessionSidebarSessionEntry<T>[];
  archivedLabel: string;
  archivedEmptyLabel: string;
  archivedLoadingLabel: string;
  archivedTotal: number;
  archivedLoading: boolean;
  archivedHasMore: boolean;
  loadMoreLabel: string;
};

export const WEB_SESSION_SIDEBAR_VIRTUAL_ITEM_SIZE = 38;

function startOfLocalDay(timestamp: number, daysAgo = 0) {
  const date = new Date(timestamp);
  date.setHours(0, 0, 0, 0);
  date.setDate(date.getDate() - daysAgo);
  return date.getTime();
}

export function groupWebSessionSidebarEntriesByDate<T>({
  entries,
  getTimestamp,
  labels,
  now = Date.now(),
}: {
  entries: WebSessionSidebarSessionEntry<T>[];
  getTimestamp: (entry: WebSessionSidebarSessionEntry<T>) => number;
  labels: WebSessionSidebarDateGroupLabels;
  now?: number;
}): WebSessionSidebarSection<T>[] {
  const groups: WebSessionSidebarSection<T>[] = [
    { key: 'today', label: labels.today, entries: [] },
    { key: 'yesterday', label: labels.yesterday, entries: [] },
    { key: 'last-seven-days', label: labels.lastSevenDays, entries: [] },
    { key: 'earlier', label: labels.earlier, entries: [] },
  ];
  const todayStart = startOfLocalDay(now);
  const yesterdayStart = startOfLocalDay(now, 1);
  const lastSevenDaysStart = startOfLocalDay(now, 6);

  entries.forEach(entry => {
    const timestamp = getTimestamp(entry);
    if (timestamp >= todayStart) {
      groups[0].entries.push(entry);
    } else if (timestamp >= yesterdayStart) {
      groups[1].entries.push(entry);
    } else if (timestamp >= lastSevenDaysStart) {
      groups[2].entries.push(entry);
    } else {
      groups[3].entries.push(entry);
    }
  });

  return groups;
}

export function buildWebSessionSidebarVirtualItems<T>({
  currentSections,
  archived,
  archivedLabel,
  archivedEmptyLabel,
  archivedLoadingLabel,
  archivedTotal,
  archivedLoading,
  archivedHasMore,
  loadMoreLabel,
}: BuildWebSessionSidebarVirtualItemsOptions<T>): WebSessionSidebarVirtualItem<T>[] {
  const items: WebSessionSidebarVirtualItem<T>[] = [];
  const visibleCurrentSections = currentSections.filter(section => section.entries.length > 0);

  visibleCurrentSections.forEach(section => {
    items.push({
      key: `section:${section.key}`,
      type: 'section',
      label: section.label,
      count: section.entries.length,
      separated: false,
    });
    section.entries.forEach(entry => {
      items.push({
        key: entry.row.key,
        type: 'session',
        entry,
      });
    });
  });

  items.push({
    key: 'section:archived',
    type: 'section',
    label: archivedLabel,
    count: archivedTotal,
    separated: visibleCurrentSections.length > 0,
  });

  if (archived.length > 0) {
    archived.forEach(entry => {
      items.push({
        key: entry.row.key,
        type: 'session',
        entry,
      });
    });
  } else {
    items.push({
      key: archivedLoading ? 'loading:archived' : 'empty:archived',
      type: 'empty',
      label: archivedLoading ? archivedLoadingLabel : archivedEmptyLabel,
    });
  }

  if (archivedHasMore) {
    items.push({
      key: 'action:archived:load-more',
      type: 'action',
      label: loadMoreLabel,
      disabled: archivedLoading,
    });
  }

  return items;
}
