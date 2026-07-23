import { describe, expect, it } from 'vitest';

import {
  buildWebSessionMobileTabDescriptors,
  MOBILE_ARCHIVED_LOAD_MORE_OPTION_KEY,
} from '@/components/web-session/webSessionMobileTabOptions';

function makeSessions(ids: string[]) {
  return ids.map(id => ({ id }));
}

describe('webSessionMobileTabOptions', () => {
  it('only includes current sessions in the virtualized list', () => {
    const descriptors = buildWebSessionMobileTabDescriptors({
      section: 'current',
      sessions: makeSessions(['session-1', 'session-2']),
    });

    expect(descriptors.map(item => item.key)).toEqual([
      'mobile-session-switcher:current',
      'session-1',
      'session-2',
    ]);
  });

  it('keeps the current empty state in the virtualized list', () => {
    const descriptors = buildWebSessionMobileTabDescriptors({
      section: 'current',
      sessions: [],
    });

    expect(
      descriptors.map(item => ({
        kind: item.kind,
        key: item.key,
      }))
    ).toEqual([
      {
        kind: 'header',
        key: 'mobile-session-switcher:current',
      },
      {
        kind: 'empty',
        key: 'mobile-session-empty:current',
      },
    ]);
  });

  it('keeps archived load-more as the final archived item', () => {
    const descriptors = buildWebSessionMobileTabDescriptors({
      section: 'archived',
      sessions: makeSessions(['archived-1']),
      hasArchivedLoadMore: true,
    });

    expect(descriptors.map(item => item.key)).toEqual([
      'mobile-session-switcher:archived',
      'archived-1',
      MOBILE_ARCHIVED_LOAD_MORE_OPTION_KEY,
    ]);
  });

  it('keeps archived empty states in the virtualized list', () => {
    const descriptors = buildWebSessionMobileTabDescriptors({
      section: 'archived',
      sessions: [],
    });

    expect(
      descriptors.map(item => ({
        kind: item.kind,
        key: item.key,
      }))
    ).toEqual([
      {
        kind: 'header',
        key: 'mobile-session-switcher:archived',
      },
      {
        kind: 'empty',
        key: 'mobile-session-empty:archived',
      },
    ]);
  });

  it('marks the archived load-more descriptor as loading when needed', () => {
    const descriptors = buildWebSessionMobileTabDescriptors({
      section: 'archived',
      sessions: makeSessions(['archived-1']),
      isArchivedLoading: true,
    });

    expect(descriptors.at(-1)).toEqual({
      kind: 'load-more',
      key: MOBILE_ARCHIVED_LOAD_MORE_OPTION_KEY,
      loading: true,
      section: 'archived',
    });
  });

  it('inserts date group headings before grouped current sessions', () => {
    const sessions = makeSessions(['today-1', 'today-2', 'earlier-1']);
    const descriptors = buildWebSessionMobileTabDescriptors({
      section: 'current',
      sessions,
      sessionGroups: [
        { key: 'today', label: 'Today', sessions: sessions.slice(0, 2) },
        { key: 'yesterday', label: 'Yesterday', sessions: [] },
        { key: 'earlier', label: 'Earlier', sessions: sessions.slice(2) },
      ],
    });

    expect(descriptors.map(item => item.key)).toEqual([
      'mobile-session-switcher:current',
      'mobile-session-date-group:today',
      'today-1',
      'today-2',
      'mobile-session-date-group:earlier',
      'earlier-1',
    ]);
    expect(descriptors[1]).toMatchObject({
      kind: 'date-group',
      label: 'Today',
      count: 2,
    });
  });
});
