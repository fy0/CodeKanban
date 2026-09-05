import { computed, ref } from 'vue';
import { describe, expect, it } from 'vitest';

import { collectWebSessionSidebarSessions } from '@/components/web-session/webSessionSidebarView';
import type { WebSessionSummary } from '@/types/models';

function session(id: string, projectId: string, hour: number): WebSessionSummary {
  return {
    id,
    projectId,
    statusUpdatedAt: `2026-09-05T${String(hour).padStart(2, '0')}:00:00Z`,
    orderIndex: 0,
    archivedAt: null,
  } as WebSessionSummary;
}

describe('web session sidebar live sessions', () => {
  it('shows newly created sessions and reorders restarted sessions without another page request', () => {
    const sessions = ref<Record<string, WebSessionSummary[]>>({
      project: [session('older', 'project', 8), session('recent', 'project', 9)],
    });
    const rows = computed(() =>
      collectWebSessionSidebarSessions(['project'], id => sessions.value[id] ?? [])
    );
    const ids = () => rows.value.map(item => item.id);

    expect(ids()).toEqual(['recent', 'older']);
    sessions.value.project = [...sessions.value.project!, session('new', 'project', 10)];
    expect(ids()).toEqual(['new', 'recent', 'older']);

    sessions.value.project = sessions.value.project!.map(item =>
      item.id === 'older' ? session('older', 'project', 11) : item
    );
    expect(ids()).toEqual(['older', 'new', 'recent']);
  });

  it('reacts to scope changes, archiving and deletion without retaining stale page rows', () => {
    const sessions = ref<Record<string, WebSessionSummary[]>>({
      first: [session('one', 'first', 8)],
      second: [session('two', 'second', 9)],
    });
    const scope = ref(['first']);
    const rows = computed(() =>
      collectWebSessionSidebarSessions(scope.value, id => sessions.value[id] ?? [])
    );
    expect(rows.value.map(item => item.id)).toEqual(['one']);
    scope.value = ['first', 'second', 'first'];
    expect(rows.value.map(item => item.id)).toEqual(['two', 'one']);

    sessions.value.second![0]!.archivedAt = '2026-09-05T10:00:00Z';
    expect(rows.value.map(item => item.id)).toEqual(['one']);
    sessions.value.first = [];
    expect(rows.value).toEqual([]);
  });

  it('preserves deterministic ordering without changing the store order', () => {
    const sessions = [session('b', 'project', 8), session('a', 'project', 8)];
    expect(
      collectWebSessionSidebarSessions(['project'], () => sessions).map(item => item.id)
    ).toEqual(['a', 'b']);
    expect(sessions.map(item => item.id)).toEqual(['b', 'a']);
  });
});
