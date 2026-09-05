import { describe, expect, it } from 'vitest';

import type { WebSessionPiTreeNode } from '@/api/webSession';
import {
  canForkWebSessionPiTreeNode,
  canMutateWebSessionPiTree,
  canOpenWebSessionPiTree,
  projectWebSessionPiTreeRows,
} from '@/components/web-session/webSessionTree';

function node(
  id: string,
  parentId?: string,
  options: Partial<WebSessionPiTreeNode> = {}
): WebSessionPiTreeNode {
  return {
    id,
    parentId,
    type: 'message',
    role: 'assistant',
    active: false,
    children: [],
    ...options,
  };
}

describe('web session Pi tree rules', () => {
  it('projects stable row depths and bounds cyclic parent references', () => {
    const rows = projectWebSessionPiTreeRows([
      node('root'),
      node('child', 'root'),
      node('leaf', 'child', { active: true }),
      node('cycle-a', 'cycle-b'),
      node('cycle-b', 'cycle-a'),
    ]);

    expect(rows.slice(0, 3).map(row => row.depth)).toEqual([0, 1, 2]);
    expect(rows.slice(3).every(row => row.depth <= 64)).toBe(true);
  });

  it('only allows native user message nodes to fork', () => {
    expect(canForkWebSessionPiTreeNode(node('user', undefined, { role: 'user' }))).toBe(true);
    expect(
      canForkWebSessionPiTreeNode(
        node('custom-user', undefined, { type: 'custom_message', role: 'user' })
      )
    ).toBe(false);
    expect(canForkWebSessionPiTreeNode(node('assistant'))).toBe(false);
    expect(canForkWebSessionPiTreeNode(node('tool', undefined, { type: 'custom' }))).toBe(false);
  });

  it('requires explicit Pi tree capability and native identity to open', () => {
    const base = {
      archived: false,
      agent: 'pi',
      supportsTree: true,
      nativeSessionId: 'native-1',
      threadPath: 'session.jsonl',
    };
    expect(canOpenWebSessionPiTree(base)).toBe(true);
    expect(canOpenWebSessionPiTree({ ...base, supportsTree: false })).toBe(false);
    expect(canOpenWebSessionPiTree({ ...base, agent: 'codex' })).toBe(false);
    expect(canOpenWebSessionPiTree({ ...base, threadPath: '' })).toBe(false);
    expect(canOpenWebSessionPiTree({ ...base, archived: true })).toBe(false);
  });

  it('allows reads while running but blocks every mutation with a run or pending input', () => {
    expect(canMutateWebSessionPiTree({ canOpen: true, running: false, pendingCount: 0 })).toBe(
      true
    );
    expect(canMutateWebSessionPiTree({ canOpen: true, running: true, pendingCount: 0 })).toBe(
      false
    );
    expect(canMutateWebSessionPiTree({ canOpen: true, running: false, pendingCount: 1 })).toBe(
      false
    );
  });
});
