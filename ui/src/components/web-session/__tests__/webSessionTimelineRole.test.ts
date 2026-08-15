import { describe, expect, it } from 'vitest';

import type { WebSessionSubAgent } from '@/stores/webSession';
import { resolveWebSessionTimelineSubAgent } from '@/components/web-session/webSessionTimelineRole';

function agent(id: string): WebSessionSubAgent {
  return {
    id,
    title: 'Atlas [worker]',
    summary: 'Inspect the repository',
    path: 'review/atlas',
    nickname: 'Atlas',
    role: 'worker',
    status: 'running',
    latestOrderIndex: 1,
  };
}

describe('webSessionTimelineRole', () => {
  it('keeps root-thread blocks on the assistant role even if the registry contains the root id', () => {
    const root = agent('thread-root');
    const agents = new Map([[root.id, root]]);

    expect(resolveWebSessionTimelineSubAgent('thread-root', 'thread-root', agents)).toBeNull();
  });

  it('resolves known child-thread blocks to their Agent entry', () => {
    const child = agent('thread-child');
    const agents = new Map([[child.id, child]]);

    expect(resolveWebSessionTimelineSubAgent('thread-child', 'thread-root', agents)).toBe(child);
  });

  it('does not label an unknown source thread as a known Agent', () => {
    expect(resolveWebSessionTimelineSubAgent('thread-unknown', 'thread-root', new Map())).toBeNull();
  });
});
