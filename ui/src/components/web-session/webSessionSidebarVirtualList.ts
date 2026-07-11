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
  archivedLabel: string;
};

export type WebSessionSidebarSessionEntry<T> = {
  source: T;
  row: WebSessionSidebarRowView;
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
  current: WebSessionSidebarSessionEntry<T>[];
  archived: WebSessionSidebarSessionEntry<T>[];
  currentLabel: string;
  currentEmptyLabel: string;
  archivedLabel: string;
  archivedEmptyLabel: string;
  archivedLoadingLabel: string;
  archivedTotal: number;
  archivedLoading: boolean;
  archivedHasMore: boolean;
  loadMoreLabel: string;
};

export const WEB_SESSION_SIDEBAR_VIRTUAL_ITEM_SIZE = 40;

export function buildWebSessionSidebarVirtualItems<T>({
  current,
  archived,
  currentLabel,
  currentEmptyLabel,
  archivedLabel,
  archivedEmptyLabel,
  archivedLoadingLabel,
  archivedTotal,
  archivedLoading,
  archivedHasMore,
  loadMoreLabel,
}: BuildWebSessionSidebarVirtualItemsOptions<T>): WebSessionSidebarVirtualItem<T>[] {
  const items: WebSessionSidebarVirtualItem<T>[] = [
    {
      key: 'section:current',
      type: 'section',
      label: currentLabel,
      count: current.length,
      separated: false,
    },
  ];

  if (current.length > 0) {
    current.forEach(entry => {
      items.push({
        key: entry.row.key,
        type: 'session',
        entry,
      });
    });
  } else {
    items.push({
      key: 'empty:current',
      type: 'empty',
      label: currentEmptyLabel,
    });
  }

  items.push({
    key: 'section:archived',
    type: 'section',
    label: archivedLabel,
    count: archivedTotal,
    separated: true,
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
