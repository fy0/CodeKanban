import { describe, expect, it } from 'vitest';

import {
  buildWebSessionSidebarVirtualItems,
  type WebSessionSidebarRowView,
  type WebSessionSidebarSessionEntry,
} from '@/components/web-session/webSessionSidebarVirtualList';

function makeEntry(id: string, archived = false): WebSessionSidebarSessionEntry<{ id: string }> {
  const row: WebSessionSidebarRowView = {
    key: `${archived ? 'archived' : 'current'}:project-1:${id}`,
    sessionId: id,
    title: id,
    iconHtml: '',
    subtitle: '',
    tooltip: id,
    accentColor: '#9ca3af',
    toneClass: 'session-sidebar-idle',
    active: false,
    archived,
    archiving: false,
    hasWorkflowPlanBadge: false,
    singleProject: true,
    currentIndicatorTitle: 'Current session',
    archivedLabel: 'Archived',
  };
  return {
    source: { id },
    row,
  };
}

function buildItems(
  current: WebSessionSidebarSessionEntry<{ id: string }>[],
  archived: WebSessionSidebarSessionEntry<{ id: string }>[] = [],
  options: { loading?: boolean; hasMore?: boolean; total?: number } = {}
) {
  return buildWebSessionSidebarVirtualItems({
    current,
    archived,
    currentLabel: 'Current',
    currentEmptyLabel: 'No current sessions',
    archivedLabel: 'Archived',
    archivedEmptyLabel: 'No archived sessions',
    archivedLoadingLabel: 'Loading',
    archivedTotal: options.total ?? archived.length,
    archivedLoading: options.loading ?? false,
    archivedHasMore: options.hasMore ?? false,
    loadMoreLabel: options.loading ? 'Loading' : 'Load more',
  });
}

describe('webSession sidebar virtual list', () => {
  it('builds stable keyed items for thousands of sessions without cloning their sources', () => {
    const current = Array.from({ length: 2000 }, (_, index) => makeEntry(`session-${index}`));
    const archived = Array.from({ length: 20 }, (_, index) => makeEntry(`archived-${index}`, true));

    const items = buildItems(current, archived, { hasMore: true, total: 200 });
    const sessionItems = items.filter(item => item.type === 'session');

    expect(items).toHaveLength(2023);
    expect(new Set(items.map(item => item.key)).size).toBe(items.length);
    expect(sessionItems).toHaveLength(2020);
    expect(sessionItems[0]?.type === 'session' && sessionItems[0].entry.source).toBe(
      current[0]?.source
    );
    expect(items.at(-1)).toMatchObject({
      type: 'action',
      key: 'action:archived:load-more',
      disabled: false,
    });
  });

  it('represents empty and loading states inside the same virtualized collection', () => {
    const items = buildItems([], [], { loading: true, hasMore: true, total: 12 });

    expect(items).toEqual([
      expect.objectContaining({ key: 'section:current', type: 'section', count: 0 }),
      expect.objectContaining({ key: 'empty:current', type: 'empty' }),
      expect.objectContaining({ key: 'section:archived', type: 'section', count: 12 }),
      expect.objectContaining({ key: 'loading:archived', type: 'empty', label: 'Loading' }),
      expect.objectContaining({
        key: 'action:archived:load-more',
        type: 'action',
        disabled: true,
      }),
    ]);
  });
});
