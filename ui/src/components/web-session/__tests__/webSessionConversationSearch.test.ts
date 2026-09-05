// @vitest-environment happy-dom

import { afterEach, describe, expect, it } from 'vitest';

import type { WebSessionBlock } from '@/stores/webSession';
import {
  findWebSessionConversationSearchMatches,
  matchesWebSessionConversationSearchTarget,
  mergeWebSessionConversationSearchMatches,
  resolveWebSessionConversationSearchMatchIndex,
  shouldHandleWebSessionConversationSearchShortcut,
  type WebSessionConversationSearchFilters,
} from '@/components/web-session/webSessionConversationSearch';

const dialogueFilters: WebSessionConversationSearchFilters = {
  user: true,
  assistant: true,
  tools: false,
  system: false,
};

function makeBlock(
  id: string,
  kind: WebSessionBlock['kind'],
  text: string,
  orderIndex: number,
  extra: Partial<WebSessionBlock> = {}
): WebSessionBlock {
  return {
    key: id,
    id,
    orderIndex,
    kind,
    itemType: `${kind}_message`,
    text,
    timestamp: orderIndex,
    attachments: [],
    ...extra,
  };
}

describe('webSessionConversationSearch', () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it('handles Ctrl+F and Cmd+F in the regular conversation interface', () => {
    const input = document.createElement('input');
    document.body.append(input);
    input.focus();

    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', ctrlKey: true })
      )
    ).toBe(true);
    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', metaKey: true })
      )
    ).toBe(true);
  });

  it('leaves Ctrl+F to the browser while a dialog is open', () => {
    const dialog = document.createElement('div');
    dialog.setAttribute('role', 'dialog');
    dialog.setAttribute('aria-modal', 'true');
    document.body.append(dialog);

    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', ctrlKey: true })
      )
    ).toBe(false);
  });

  it('leaves Ctrl+F to the browser when focus is inside a non-modal overlay', () => {
    const menu = document.createElement('div');
    menu.setAttribute('role', 'menu');
    const button = document.createElement('button');
    menu.append(button);
    document.body.append(menu);
    button.focus();

    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', ctrlKey: true })
      )
    ).toBe(false);
  });

  it('ignores events that are not the unmodified find shortcut', () => {
    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, altKey: true })
      )
    ).toBe(false);
    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'g', ctrlKey: true })
      )
    ).toBe(false);
  });

  it('searches user and assistant text with the dialogue defaults', () => {
    const matches = findWebSessionConversationSearchMatches(
      [
        makeBlock('user-1', 'user', 'Please inspect the release notes', 1),
        makeBlock('assistant-1', 'assistant', 'I found the release notes', 2),
        makeBlock('tool-1', 'tool', 'release notes tool', 3),
        makeBlock('system-1', 'system', 'release notes approval', 4),
      ],
      'RELEASE',
      dialogueFilters
    );

    expect(matches.map(match => match.id)).toEqual(['user-1', 'assistant-1']);
  });

  it('searches tool input, output, and compacted group items when enabled', () => {
    const block = makeBlock('tool-latest', 'tool', '', 2, {
      itemType: 'command_execution',
      tool: {
        id: 'tool-latest',
        name: 'CommandExecution',
        kind: 'command_execution',
        input: { command: 'git status' },
        output: 'working tree clean',
        status: 'done',
        commandGroup: { id: 'group-1', count: 2 },
      },
      payload: {
        groupItems: [
          {
            toolId: 'tool-old',
            command: 'pnpm test',
            output: 'all tests passed',
          },
        ],
      },
    });
    const matches = findWebSessionConversationSearchMatches([block], 'pnpm test', {
      ...dialogueFilters,
      tools: true,
    });

    expect(matches).toHaveLength(1);
    expect(matches[0]).toMatchObject({
      id: 'tool-latest',
      toolId: 'tool-latest',
      commandGroupId: 'group-1',
    });
  });

  it('searches system prompts only when system interactions are enabled', () => {
    const block = makeBlock('system-1', 'system', '', 1, {
      itemType: 'approval_request',
      detail: {
        type: 'approval_request',
        prompt: 'Allow the deployment command?',
      },
    });

    expect(findWebSessionConversationSearchMatches([block], 'deployment', dialogueFilters)).toEqual(
      []
    );
    expect(
      findWebSessionConversationSearchMatches([block], 'deployment', {
        ...dialogueFilters,
        system: true,
      })
    ).toHaveLength(1);
  });

  it('deduplicates local and remote matches by item or command group', () => {
    const local = findWebSessionConversationSearchMatches(
      [
        makeBlock('tool-latest', 'tool', '', 2, {
          tool: {
            id: 'tool-latest',
            name: 'CommandExecution',
            status: 'done',
            commandGroup: { id: 'group-1', count: 2 },
          },
        }),
      ],
      'command',
      { ...dialogueFilters, tools: true }
    );
    const merged = mergeWebSessionConversationSearchMatches(local, [
      {
        id: 'tool-old',
        orderIndex: 1,
        kind: 'tool',
        toolId: 'tool-old',
        commandGroupId: 'group-1',
      },
      {
        id: 'tool-latest',
        orderIndex: 2,
        kind: 'tool',
        toolId: 'tool-latest',
        commandGroupId: 'group-1',
      },
    ]);

    expect(merged).toHaveLength(1);
    expect(merged[0].key).toBe('tool-latest');
    expect(
      matchesWebSessionConversationSearchTarget(
        {
          ...makeBlock('tool-latest', 'tool', '', 2),
          tool: {
            id: 'tool-latest',
            name: 'CommandExecution',
            status: 'done',
            commandGroup: { id: 'group-1', count: 2 },
          },
        },
        merged[0]
      )
    ).toBe(true);
  });

  it('maps an ungrouped compacted command item to its visible card', () => {
    const compactedBlock = makeBlock('tool-latest', 'tool', '', 2, {
      tool: {
        id: 'tool-latest',
        name: 'CommandExecution',
        status: 'done',
      },
      payload: {
        groupItems: [{ toolId: 'tool-old', command: 'pnpm test' }],
      },
    });

    expect(
      matchesWebSessionConversationSearchTarget(compactedBlock, {
        id: 'tool-old',
        orderIndex: 1,
        kind: 'tool',
        toolId: 'tool-old',
      })
    ).toBe(true);
  });

  it('keeps the active result when older remote matches are inserted before it', () => {
    const activeMatch = {
      id: 'local-latest',
      orderIndex: 20,
      kind: 'assistant',
    };
    const matches = mergeWebSessionConversationSearchMatches(
      [activeMatch],
      [
        { id: 'remote-oldest', orderIndex: 1, kind: 'user' },
        { id: 'remote-older', orderIndex: 10, kind: 'assistant' },
        activeMatch,
      ]
    );

    expect(resolveWebSessionConversationSearchMatchIndex(matches, activeMatch, 0)).toBe(2);
  });

  it('clamps a missing active result to the requested fallback index', () => {
    const matches = [
      { id: 'first', orderIndex: 1, kind: 'user' },
      { id: 'last', orderIndex: 2, kind: 'assistant' },
    ];

    expect(resolveWebSessionConversationSearchMatchIndex(matches, null, Number.MAX_VALUE)).toBe(1);
  });
});
