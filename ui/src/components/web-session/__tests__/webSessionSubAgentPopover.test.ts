import { describe, expect, it } from 'vitest';

import {
  resolveWebSessionSubAgentPopover,
  WEB_SESSION_SUB_AGENT_RECENT_LIMIT,
} from '@/components/web-session/webSessionSubAgentPopover';
import type { WebSessionSubAgent } from '@/stores/webSession';

function agent(id: string, input: Partial<WebSessionSubAgent> = {}): WebSessionSubAgent {
  return {
    id,
    title: id,
    summary: '',
    path: '',
    nickname: '',
    role: '',
    status: 'completed',
    latestOrderIndex: 0,
    ...input,
  };
}

describe('webSessionSubAgentPopover', () => {
  it('keeps every active item first and limits history to the 10 most recent items', () => {
    const known = Array.from({ length: 15 }, (_, index) =>
      agent(`history-${index}`, {
        lastActivityAt: index,
        latestOrderIndex: index,
      })
    );
    known.push(
      agent('active-a', { status: 'running' }),
      agent('active-b', { status: 'pending_init' })
    );

    const result = resolveWebSessionSubAgentPopover(known, [
      { id: 'active-b', title: 'B', summary: '' },
      { id: 'active-a', title: 'A', summary: '' },
      { id: 'active-b', title: 'B duplicate', summary: '' },
    ]);

    expect(result.items.slice(0, 2).map(item => item.id)).toEqual(['active-b', 'active-a']);
    expect(result.items.slice(2).map(item => item.id)).toEqual([
      'history-14',
      'history-13',
      'history-12',
      'history-11',
      'history-10',
      'history-9',
      'history-8',
      'history-7',
      'history-6',
      'history-5',
    ]);
    expect(result.recentCount).toBe(WEB_SESSION_SUB_AGENT_RECENT_LIMIT);
    expect(new Set(result.items.map(item => item.id)).size).toBe(result.items.length);
  });

  it('reduces 60 terminal items to the latest 10 with stable tie breaking', () => {
    const known = Array.from({ length: 60 }, (_, index) =>
      agent(`history-${String(index).padStart(2, '0')}`, {
        lastActivityAt: index < 58 ? index : 100,
        latestOrderIndex: index < 58 ? index : 5,
      })
    );

    const result = resolveWebSessionSubAgentPopover(known, []);

    expect(result.items).toHaveLength(10);
    expect(result.items.slice(0, 2).map(item => item.id)).toEqual(['history-58', 'history-59']);
    expect(result.recentCount).toBe(10);
    expect(result.activeIds.size).toBe(0);
  });
});
