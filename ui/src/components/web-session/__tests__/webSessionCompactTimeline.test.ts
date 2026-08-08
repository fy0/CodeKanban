import { describe, expect, it } from 'vitest';

import type { WebSessionBlock } from '@/stores/webSession';
import {
  isTransportRetryNoteBlock,
  projectWebSessionCompactTimelineBlocks,
  projectWebSessionVisibleTimelineBlocks,
} from '@/components/web-session/webSessionCompactTimeline';

function buildFileChangeBlock(
  id: string,
  options: {
    key?: string;
    orderIndex?: number;
    path?: string;
    groupId?: string;
    count?: number;
    groupItems?: Array<Record<string, unknown>>;
    status?: 'running' | 'done' | 'error';
    timestamp?: number;
    sourceThreadId?: string;
  } = {}
): WebSessionBlock {
  const timestamp = options.timestamp ?? Date.UTC(2026, 3, 20, 12, 0, 0);
  return {
    key: options.key ?? id,
    id,
    orderIndex: options.orderIndex ?? 1,
    kind: 'tool',
    itemType: 'file_change',
    text: '',
    timestamp,
    sourceThreadId: options.sourceThreadId,
    attachments: [],
    tool: {
      id,
      name: 'FileChange',
      kind: 'file_change',
      input: {
        path: options.path ?? `src/${id}.ts`,
        changes: [{ path: options.path ?? `src/${id}.ts` }],
      },
      status: options.status ?? 'done',
      meta: {
        kind: 'file_change',
        title: 'FileChange',
        subtitle: options.path ?? `src/${id}.ts`,
      },
      ...(options.groupId
        ? {
            commandGroup: {
              id: options.groupId,
              count: options.count ?? 1,
            },
          }
        : {}),
    },
    ...(options.groupItems
      ? {
          payload: {
            groupItems: options.groupItems,
          },
        }
      : {}),
  };
}

function buildCommandBlock(
  id: string,
  options: {
    orderIndex?: number;
    command?: string;
    groupId?: string;
    count?: number;
    status?: 'running' | 'done' | 'error';
    timestamp?: number;
    sourceThreadId?: string;
  } = {}
): WebSessionBlock {
  const command = options.command ?? id;
  const timestamp = options.timestamp ?? Date.UTC(2026, 3, 20, 12, 0, 0);
  return {
    key: id,
    id,
    orderIndex: options.orderIndex ?? 1,
    kind: 'tool',
    itemType: 'command_execution',
    text: '',
    timestamp,
    sourceThreadId: options.sourceThreadId,
    attachments: [],
    tool: {
      id,
      name: 'CommandExecution',
      kind: 'command_execution',
      input: { command },
      output: `${command} output`,
      status: options.status ?? 'done',
      meta: {
        kind: 'command_execution',
        title: 'CommandExecution',
        subtitle: command,
      },
      ...(options.groupId
        ? {
            commandGroup: {
              id: options.groupId,
              count: options.count ?? 1,
            },
          }
        : {}),
    },
  };
}

function buildDynamicToolBlock(
  id: string,
  name: string,
  input: Record<string, unknown>,
  options: {
    orderIndex?: number;
    groupId?: string;
    count?: number;
    status?: 'running' | 'done' | 'error';
    timestamp?: number;
    sourceThreadId?: string;
  } = {}
): WebSessionBlock {
  const timestamp = options.timestamp ?? Date.UTC(2026, 3, 20, 12, 0, 0);
  return {
    key: id,
    id,
    orderIndex: options.orderIndex ?? 1,
    kind: 'tool',
    itemType: 'dynamic_tool_call',
    text: '',
    timestamp,
    sourceThreadId: options.sourceThreadId,
    attachments: [],
    tool: {
      id,
      name,
      kind: 'dynamic_tool_call',
      input,
      output: `${name} output`,
      status: options.status ?? 'done',
      meta: {
        kind: 'dynamic_tool_call',
        title: name,
      },
      ...(options.groupId
        ? {
            commandGroup: {
              id: options.groupId,
              count: options.count ?? 1,
            },
          }
        : {}),
    },
  };
}

function buildMessageBlock(id: string, orderIndex: number): WebSessionBlock {
  return {
    key: id,
    id,
    orderIndex,
    kind: 'assistant',
    itemType: 'agent_message',
    text: `message-${id}`,
    timestamp: Date.UTC(2026, 3, 20, 12, 0, orderIndex),
    attachments: [],
  };
}

function readGroupItems(block: WebSessionBlock): Array<Record<string, unknown>> {
  const raw = block.payload?.groupItems;
  return Array.isArray(raw) ? (raw as Array<Record<string, unknown>>) : [];
}

describe('webSessionCompactTimeline', () => {
  it('identifies transport retry notes without treating ordinary notes as retries', () => {
    const retryNote: WebSessionBlock = {
      key: 'retry-note',
      id: 'retry-note',
      orderIndex: 1,
      kind: 'system',
      itemType: 'note',
      text: 'Claude API retry 2/10 after 1s (server_error)',
      timestamp: Date.UTC(2026, 3, 20, 12, 0, 0),
      attachments: [],
      payload: { code: 'transport_retrying' },
    };
    const ordinaryNote = { ...retryNote, key: 'note', id: 'note', payload: { lvl: 'warn' } };

    expect(isTransportRetryNoteBlock(retryNote)).toBe(true);
    expect(isTransportRetryNoteBlock(ordinaryNote)).toBe(false);
  });

  it('hides retry notes after preserving their boundaries between tool groups', () => {
    const retryNote: WebSessionBlock = {
      key: 'retry-note',
      id: 'retry-note',
      orderIndex: 2,
      kind: 'system',
      itemType: 'note',
      text: 'Claude API retry 2/10 after 1s (server_error)',
      timestamp: Date.UTC(2026, 3, 20, 12, 0, 1),
      attachments: [],
      payload: { code: 'transport_retrying' },
    };
    const visible = projectWebSessionVisibleTimelineBlocks([
      buildCommandBlock('before-retry', { orderIndex: 1, command: 'git status' }),
      retryNote,
      buildCommandBlock('after-retry', { orderIndex: 3, command: 'git diff' }),
    ]);

    expect(visible).toHaveLength(2);
    expect(visible.map(block => block.tool?.id)).toEqual(['before-retry', 'after-retry']);
    expect(visible.every(block => !block.tool?.commandGroup)).toBe(true);
  });

  it('folds consecutive file_change blocks that share a command group id', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildMessageBlock('intro', 1),
      buildFileChangeBlock('fc-1', {
        orderIndex: 2,
        path: 'ui/src/App.vue',
        groupId: 'fc-group-1',
        count: 1,
      }),
      buildFileChangeBlock('fc-2', {
        orderIndex: 3,
        path: 'ui/src/components/Panel.vue',
        groupId: 'fc-group-1',
        count: 2,
      }),
      buildMessageBlock('after', 4),
    ]);

    expect(projected).toHaveLength(3);
    expect(projected[1].tool?.commandGroup?.id).toBe('fc-group-1');
    expect(projected[1].tool?.commandGroup?.count).toBe(2);
    expect(projected[1].tool?.meta?.subtitle).toBe('ui/src/components/Panel.vue');

    const groupItems = readGroupItems(projected[1]);
    expect(groupItems).toHaveLength(2);
    expect(groupItems.map(item => item.summary)).toEqual([
      'ui/src/App.vue',
      'ui/src/components/Panel.vue',
    ]);
  });

  it('builds a synthetic compact group for consecutive ungrouped file changes', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('fc-a', {
        orderIndex: 1,
        path: 'ui/src/a.ts',
      }),
      buildFileChangeBlock('fc-b', {
        orderIndex: 2,
        path: 'ui/src/b.ts',
      }),
    ]);

    expect(projected).toHaveLength(1);
    expect(projected[0].tool?.commandGroup?.count).toBe(2);
    expect(projected[0].tool?.commandGroup?.id).toContain('timeline-file-change:');

    const groupItems = readGroupItems(projected[0]);
    expect(groupItems).toHaveLength(2);
    expect(groupItems[0].summary).toBe('ui/src/a.ts');
    expect(groupItems[1].summary).toBe('ui/src/b.ts');
  });

  it('folds consecutive file_change blocks when stale group ids differ', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('fc-1', {
        orderIndex: 1,
        path: 'ui/src/App.vue',
        groupId: 'fc-group-1',
      }),
      buildFileChangeBlock('fc-2', {
        orderIndex: 2,
        path: 'ui/src/components/Panel.vue',
        groupId: 'fc-group-2',
      }),
    ]);

    expect(projected).toHaveLength(1);
    expect(projected[0].tool?.commandGroup?.id).toBe('fc-group-1');
    expect(projected[0].tool?.commandGroup?.count).toBe(2);
    expect(readGroupItems(projected[0]).map(item => item.summary)).toEqual([
      'ui/src/App.vue',
      'ui/src/components/Panel.vue',
    ]);
  });

  it('folds consecutive command execution blocks when stale group ids differ', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildCommandBlock('cmd-1', {
        orderIndex: 1,
        command: 'git status',
        groupId: 'cmd-group-1',
      }),
      buildCommandBlock('cmd-2', {
        orderIndex: 2,
        command: 'git diff',
        groupId: 'cmd-group-2',
      }),
    ]);

    expect(projected).toHaveLength(1);
    expect(projected[0].tool?.commandGroup?.id).toBe('cmd-group-1');
    expect(projected[0].tool?.commandGroup?.count).toBe(2);
    expect(readGroupItems(projected[0]).map(item => item.command)).toEqual([
      'git status',
      'git diff',
    ]);
  });

  it('folds dynamic tools by name and keeps different built-ins separate', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildDynamicToolBlock('read-1', 'Read', { file_path: 'src/App.vue' }, { orderIndex: 1 }),
      buildDynamicToolBlock('read-2', 'Read', { file_path: 'src/main.ts' }, { orderIndex: 2 }),
      buildDynamicToolBlock(
        'grep-1',
        'Grep',
        { pattern: 'WebSessionPanel', path: 'ui/src' },
        { orderIndex: 3 }
      ),
    ]);

    expect(projected).toHaveLength(2);
    expect(projected[0].tool?.name).toBe('Read');
    expect(projected[0].tool?.commandGroup?.count).toBe(2);
    expect(readGroupItems(projected[0]).map(item => item.summary)).toEqual([
      'src/App.vue',
      'src/main.ts',
    ]);
    expect(projected[1].tool?.name).toBe('Grep');
    expect(projected[1].tool?.commandGroup).toBeUndefined();
    expect(projected[1].tool?.meta?.title).toBe('Grep');
  });

  it('uses dynamic tool targets as compact summaries', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildDynamicToolBlock('grep-1', 'Grep', {
        pattern: 'dynamic_tool_call',
        path: 'service/websession',
      }),
      buildDynamicToolBlock('grep-2', 'Grep', {
        pattern: 'AskUserQuestion',
        path: 'service/websession/manager.go',
      }),
    ]);

    const groupItems = readGroupItems(projected[0]);
    expect(groupItems.map(item => item.summary)).toEqual([
      'dynamic_tool_call · service/websession',
      'AskUserQuestion · service/websession/manager.go',
    ]);
  });

  it('does not merge compact tool blocks across non-tool blocks or different kinds', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('fc-a', {
        orderIndex: 1,
        path: 'ui/src/a.ts',
      }),
      buildMessageBlock('split', 2),
      buildFileChangeBlock('fc-b', {
        orderIndex: 3,
        path: 'ui/src/b.ts',
      }),
      buildFileChangeBlock('fc-c', {
        orderIndex: 4,
        path: 'ui/src/c.ts',
        groupId: 'fc-group-c',
      }),
      buildCommandBlock('cmd-a', {
        orderIndex: 5,
        command: 'git status',
      }),
    ]);

    expect(projected).toHaveLength(4);
    expect(projected[0].tool?.commandGroup).toBeUndefined();
    expect(projected[2].tool?.commandGroup?.id).toBe('fc-group-c');
    expect(projected[2].tool?.commandGroup?.count).toBe(2);
    expect(projected[3].tool?.kind).toBe('command_execution');
  });

  it('does not merge adjacent compact tools from different agent threads', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildCommandBlock('agent-a-command', {
        orderIndex: 1,
        command: 'git status',
        sourceThreadId: 'thread-agent-a',
      }),
      buildCommandBlock('agent-b-command', {
        orderIndex: 2,
        command: 'git diff',
        sourceThreadId: 'thread-agent-b',
      }),
    ]);

    expect(projected).toHaveLength(2);
    expect(projected[0].sourceThreadId).toBe('thread-agent-a');
    expect(projected[1].sourceThreadId).toBe('thread-agent-b');
  });

  it('deduplicates merged group detail items by tool id while keeping the latest state', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('fc-1', {
        orderIndex: 1,
        groupId: 'fc-group-1',
        path: 'ui/src/App.vue',
        groupItems: [
          {
            toolId: 'fc-1',
            kind: 'file_change',
            title: 'FileChange',
            summary: 'ui/src/App.vue',
            command: 'ui/src/App.vue',
            status: 'running',
            timestamp: '2026-04-20T12:00:00.000Z',
          },
        ],
      }),
      buildFileChangeBlock('fc-2', {
        orderIndex: 2,
        groupId: 'fc-group-1',
        count: 2,
        path: 'ui/src/components/Panel.vue',
        groupItems: [
          {
            toolId: 'fc-1',
            kind: 'file_change',
            title: 'FileChange',
            summary: 'ui/src/App.vue',
            command: 'ui/src/App.vue',
            status: 'done',
            timestamp: '2026-04-20T12:00:00.000Z',
          },
          {
            toolId: 'fc-2',
            kind: 'file_change',
            title: 'FileChange',
            summary: 'ui/src/components/Panel.vue',
            command: 'ui/src/components/Panel.vue',
            status: 'done',
            timestamp: '2026-04-20T12:00:01.000Z',
          },
        ],
      }),
    ]);

    const groupItems = readGroupItems(projected[0]);
    expect(groupItems).toHaveLength(2);
    expect(groupItems.map(item => item.toolId)).toEqual(['fc-1', 'fc-2']);
    expect(groupItems[0].status).toBe('done');
  });
});
