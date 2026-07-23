export type MobileSessionCategory = 'current' | 'archived';

export const MOBILE_ARCHIVED_LOAD_MORE_OPTION_KEY = 'mobile-session-load-more-archived';

export type WebSessionMobileTabGroup<TSession extends { id: string }> = {
  key: string;
  label: string;
  sessions: TSession[];
};

export type WebSessionMobileTabDescriptor<TSession extends { id: string }> =
  | {
      kind: 'header';
      key: string;
      section: MobileSessionCategory;
    }
  | {
      kind: 'date-group';
      key: string;
      groupKey: string;
      label: string;
      count: number;
      collapsed: boolean;
      section: 'current';
    }
  | {
      kind: 'session';
      key: string;
      section: MobileSessionCategory;
      session: TSession;
    }
  | {
      kind: 'empty';
      key: string;
      section: MobileSessionCategory;
    }
  | {
      kind: 'load-more';
      key: typeof MOBILE_ARCHIVED_LOAD_MORE_OPTION_KEY;
      section: 'archived';
      loading: boolean;
    };

export function buildWebSessionMobileTabDescriptors<TSession extends { id: string }>(input: {
  section: MobileSessionCategory;
  sessions: TSession[];
  sessionGroups?: WebSessionMobileTabGroup<TSession>[];
  collapsedGroupKeys?: ReadonlySet<string>;
  hasArchivedLoadMore?: boolean;
  isArchivedLoading?: boolean;
}) {
  const descriptors: WebSessionMobileTabDescriptor<TSession>[] = [
    {
      kind: 'header',
      key: `mobile-session-switcher:${input.section}`,
      section: input.section,
    },
  ];

  const visibleGroups = (input.sessionGroups ?? []).filter(group => group.sessions.length > 0);
  if (input.section === 'current' && visibleGroups.length > 0) {
    visibleGroups.forEach(group => {
      const collapsed = input.collapsedGroupKeys?.has(group.key) === true;
      descriptors.push({
        kind: 'date-group',
        key: `mobile-session-date-group:${group.key}`,
        groupKey: group.key,
        label: group.label,
        count: group.sessions.length,
        collapsed,
        section: 'current',
      });
      if (collapsed) {
        return;
      }
      group.sessions.forEach(session => {
        descriptors.push({
          kind: 'session',
          key: session.id,
          section: 'current',
          session,
        });
      });
    });
  } else if (input.sessions.length > 0) {
    input.sessions.forEach(session => {
      descriptors.push({
        kind: 'session',
        key: session.id,
        section: input.section,
        session,
      });
    });
  } else {
    descriptors.push({
      kind: 'empty',
      key: `mobile-session-empty:${input.section}`,
      section: input.section,
    });
  }

  if (input.section === 'current') {
    return descriptors;
  }

  if (input.hasArchivedLoadMore || input.isArchivedLoading) {
    descriptors.push({
      kind: 'load-more',
      key: MOBILE_ARCHIVED_LOAD_MORE_OPTION_KEY,
      section: 'archived',
      loading: input.isArchivedLoading === true,
    });
  }

  return descriptors;
}
