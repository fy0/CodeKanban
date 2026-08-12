import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import type { WebSessionPiTreeNode } from '@/api/webSession';
import {
  canForkWebSessionPiTreeNode,
  canMutateWebSessionPiTree,
  canOpenWebSessionPiTree,
  projectWebSessionPiTreeRows,
} from '@/components/web-session/webSessionTree';

const panelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const drawerPath = fileURLToPath(new URL('../WebSessionTreeDrawer.vue', import.meta.url));
const panelSource = readFileSync(panelPath, 'utf8');
const drawerSource = readFileSync(drawerPath, 'utf8');

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

describe('web session Pi tree panel contract', () => {
  it('restores navigate text only to the captured source session draft', () => {
    const handler = panelSource.slice(
      panelSource.indexOf('async function handlePiTreeNavigated('),
      panelSource.indexOf('async function handlePiTreeCreated(')
    );
    expect(handler).toContain(
      'webSessionStore.setDraftText(sourceProjectId, sourceSessionId, result.editorText);'
    );
    expect(handler).not.toContain('if (result.editorText)');
    expect(handler).toContain('props.projectId === sourceProjectId');
    expect(handler).toContain('currentRealSession.value?.id === sourceSessionId');
    expect(handler).toContain(
      'await webSessionStore.loadSessionSnapshot(sourceProjectId, sourceSessionId, {'
    );
    expect(drawerSource).toContain('const projectId = props.projectId;');
    expect(drawerSource).toContain('const sessionId = props.sessionId;');
  });

  it('restores fork text to the target draft before selecting the new session', () => {
    const handler = panelSource.slice(
      panelSource.indexOf('async function handlePiTreeCreated('),
      panelSource.indexOf('const tabsThemeOverrides')
    );
    expect(handler).toContain(
      "webSessionStore.setDraftText(sourceProjectId, targetSessionId, result.editorText ?? '');"
    );
    expect(handler).toContain('await webSessionStore.loadSessions(sourceProjectId, true);');
    expect(handler).toContain('if (!activate || !sourceStillActive())');
    expect(handler).toContain('if (!sourceStillActive())');
    expect(handler).toContain('const activated = await activateTabById(targetSessionId);');
    expect(handler.indexOf('setDraftText')).toBeLessThan(
      handler.indexOf('if (!activate || !sourceStillActive())')
    );
    expect(handler.indexOf('if (!activate || !sourceStillActive())')).toBeLessThan(
      handler.indexOf('loadSessions')
    );
    expect(handler.indexOf('setDraftText')).toBeLessThan(handler.indexOf('activateTabById'));
    expect(drawerSource).toContain("emit('created', { projectId, sessionId, result, activate });");
  });

  it('keeps the drawer actions capability and idle guarded', () => {
    expect(panelSource).toContain(':can-mutate="canMutatePiTree"');
    expect(panelSource).toContain('runtimePiCapability.value.supportsTree');
    expect(drawerSource).toContain('canForkWebSessionPiTreeNode(selectedNode.value)');
    expect(drawerSource).toContain(':disabled="!canNavigate"');
    expect(drawerSource).toContain(':disabled="!canFork"');
    expect(drawerSource).toContain(':disabled="!canClone"');
    expect(drawerSource).toContain(':body-content-style="treeBodyContentStyle"');
    expect(drawerSource).toContain("height: '100%'");
    expect(drawerSource).toContain("overflow: 'hidden'");
  });
});
