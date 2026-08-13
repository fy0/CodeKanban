import type { ProjectBadge } from '@/utils/projectBadge';

export type WebSessionSidebarRowView = {
  key: string;
  sessionId: string;
  title: string;
  searchMatchLabel?: string;
  iconHtml: string;
  subtitle: string;
  tooltip: string;
  accentColor: string;
  toneClass: string;
  active: boolean;
  archived: boolean;
  archiving: boolean;
  hasWorkflowPlanBadge: boolean;
  hasScheduledPlanExecution: boolean;
  hasScheduledInput: boolean;
  scheduledInputTitle: string;
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

export type WebSessionDateGroup<T> = {
  key: string;
  label: string;
  items: T[];
};

export type WebSessionSidebarVirtualItem<T> =
  | {
      key: string;
      type: 'section';
      sectionKey: string;
      label: string;
      count: number;
      separated: boolean;
      collapsed: boolean;
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
  collapsedSectionKeys?: ReadonlySet<string>;
  showArchived?: boolean;
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
const EMPTY_COLLAPSED_SECTION_KEYS: ReadonlySet<string> = new Set();

export function resolveWebSessionSidebarCollapsedKeys({
  collapsedSectionKeys,
  searchActive,
}: {
  collapsedSectionKeys: ReadonlySet<string>;
  searchActive: boolean;
}) {
  return searchActive ? EMPTY_COLLAPSED_SECTION_KEYS : collapsedSectionKeys;
}

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
  return groupWebSessionItemsByDate({
    items: entries,
    getTimestamp,
    labels,
    now,
  }).map(group => ({
    key: group.key,
    label: group.label,
    entries: group.items,
  }));
}

export function groupWebSessionItemsByDate<T>({
  items,
  getTimestamp,
  labels,
  now = Date.now(),
}: {
  items: T[];
  getTimestamp: (item: T) => number;
  labels: WebSessionSidebarDateGroupLabels;
  now?: number;
}): WebSessionDateGroup<T>[] {
  const groups: WebSessionDateGroup<T>[] = [
    { key: 'today', label: labels.today, items: [] },
    { key: 'yesterday', label: labels.yesterday, items: [] },
    { key: 'last-seven-days', label: labels.lastSevenDays, items: [] },
    { key: 'earlier', label: labels.earlier, items: [] },
  ];
  const todayStart = startOfLocalDay(now);
  const yesterdayStart = startOfLocalDay(now, 1);
  const lastSevenDaysStart = startOfLocalDay(now, 6);

  items.forEach(item => {
    const timestamp = getTimestamp(item);
    if (timestamp >= todayStart) {
      groups[0].items.push(item);
    } else if (timestamp >= yesterdayStart) {
      groups[1].items.push(item);
    } else if (timestamp >= lastSevenDaysStart) {
      groups[2].items.push(item);
    } else {
      groups[3].items.push(item);
    }
  });

  return groups;
}

export function buildWebSessionSidebarVirtualItems<T>({
  currentSections,
  collapsedSectionKeys = new Set<string>(),
  showArchived = true,
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
    const collapsed = collapsedSectionKeys.has(section.key);
    items.push({
      key: `section:${section.key}`,
      type: 'section',
      sectionKey: section.key,
      label: section.label,
      count: section.entries.length,
      separated: false,
      collapsed,
    });
    if (collapsed) {
      return;
    }
    section.entries.forEach(entry => {
      items.push({
        key: entry.row.key,
        type: 'session',
        entry,
      });
    });
  });

  if (!showArchived) {
    return items;
  }

  const archivedCollapsed = collapsedSectionKeys.has('archived');
  items.push({
    key: 'section:archived',
    type: 'section',
    sectionKey: 'archived',
    label: archivedLabel,
    count: archivedTotal,
    separated: visibleCurrentSections.length > 0,
    collapsed: archivedCollapsed,
  });

  if (archivedCollapsed) {
    return items;
  }

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
