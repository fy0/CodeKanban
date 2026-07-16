import { describe, expect, it } from 'vitest';

import {
  buildWebSessionSidebarVirtualItems,
  groupWebSessionSidebarEntriesByDate,
  type WebSessionSidebarRowView,
  type WebSessionSidebarSessionEntry,
} from '@/components/web-session/webSessionSidebarVirtualList';

type TestSource = { id: string; timestamp?: number };

function makeEntry(
  id: string,
  archived = false,
  timestamp?: number
): WebSessionSidebarSessionEntry<TestSource> {
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
    activityTimeLabel: '09:08',
    activityTimeTitle: '2026/7/16 09:08:00',
  };
  return {
    source: { id, timestamp },
    row,
  };
}

function buildItems(
  current: WebSessionSidebarSessionEntry<TestSource>[],
  archived: WebSessionSidebarSessionEntry<TestSource>[] = [],
  options: { loading?: boolean; hasMore?: boolean; total?: number } = {}
) {
  return buildWebSessionSidebarVirtualItems({
    currentSections: [{ key: 'today', label: 'Today', entries: current }],
    archived,
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
      expect.objectContaining({
        key: 'section:archived',
        type: 'section',
        count: 12,
        separated: false,
      }),
      expect.objectContaining({ key: 'loading:archived', type: 'empty', label: 'Loading' }),
      expect.objectContaining({
        key: 'action:archived:load-more',
        type: 'action',
        disabled: true,
      }),
    ]);
  });

  it('groups current sessions by local calendar-day boundaries', () => {
    const now = new Date(2026, 6, 16, 12, 0, 0).getTime();
    const entries = [
      makeEntry('today', false, new Date(2026, 6, 16, 0, 0, 0).getTime()),
      makeEntry('yesterday', false, new Date(2026, 6, 15, 23, 59, 59).getTime()),
      makeEntry('within-seven-days', false, new Date(2026, 6, 10, 0, 0, 0).getTime()),
      makeEntry('older-than-seven-days', false, new Date(2026, 6, 9, 23, 59, 59).getTime()),
      makeEntry('missing-time'),
    ];

    const groups = groupWebSessionSidebarEntriesByDate({
      entries,
      getTimestamp: entry => entry.source.timestamp ?? 0,
      labels: {
        today: 'Today',
        yesterday: 'Yesterday',
        lastSevenDays: 'Within 7 Days',
        earlier: 'Earlier',
      },
      now,
    });

    expect(groups.map(group => [group.key, group.entries.map(entry => entry.source.id)])).toEqual([
      ['today', ['today']],
      ['yesterday', ['yesterday']],
      ['last-seven-days', ['within-seven-days']],
      ['earlier', ['older-than-seven-days', 'missing-time']],
    ]);
  });

  it('renders only non-empty date groups before the archived section', () => {
    const today = makeEntry('today');
    const earlier = makeEntry('earlier');
    const archived = makeEntry('archived', true);
    const items = buildWebSessionSidebarVirtualItems({
      currentSections: [
        { key: 'today', label: 'Today', entries: [today] },
        { key: 'yesterday', label: 'Yesterday', entries: [] },
        { key: 'last-seven-days', label: 'Within 7 Days', entries: [] },
        { key: 'earlier', label: 'Earlier', entries: [earlier] },
      ],
      archived: [archived],
      archivedLabel: 'Archived',
      archivedEmptyLabel: 'No archived sessions',
      archivedLoadingLabel: 'Loading',
      archivedTotal: 1,
      archivedLoading: false,
      archivedHasMore: false,
      loadMoreLabel: 'Load more',
    });

    expect(
      items
        .filter(item => item.type === 'section')
        .map(item => ({
          key: item.key,
          count: item.count,
          separated: item.separated,
        }))
    ).toEqual([
      { key: 'section:today', count: 1, separated: false },
      { key: 'section:earlier', count: 1, separated: false },
      { key: 'section:archived', count: 1, separated: true },
    ]);
  });
});
