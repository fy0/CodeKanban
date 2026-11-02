import { readFileSync } from 'node:fs';
import { computed, ref } from 'vue';
import ts from 'typescript';
import { describe, expect, it } from 'vitest';

const panelSource = readFileSync(new URL('../WebSessionPanel.vue', import.meta.url), 'utf8');
const setupSource = panelSource.split('<script setup lang="ts">')[1]!.split('</script>')[0]!;
const parsedSource = ts.createSourceFile('panel.ts', setupSource, ts.ScriptTarget.Latest, true);
const selectedNames = new Set([
  'liveState',
  'pendingApproval',
  'pendingUserInput',
  'isPendingUserInputRequestBlock',
  'isPendingApprovalRequestBlock',
  'shouldRenderToolBlockInTimeline',
  'filteredTimelineBlocks',
  'visibleBlocks',
]);
const initializationSource = parsedSource.statements
  .flatMap(statement => {
    if (ts.isFunctionDeclaration(statement) && selectedNames.has(statement.name?.text ?? '')) {
      return [statement.getText(parsedSource)];
    }
    if (!ts.isVariableStatement(statement)) {
      return [];
    }
    if (statement.getText(parsedSource).includes('= useWebSessionConversationSearch(')) {
      return ['captureVisibleBlocks(visibleBlocks.value);'];
    }
    return statement.declarationList.declarations.some(
      declaration => ts.isIdentifier(declaration.name) && selectedNames.has(declaration.name.text)
    )
      ? [statement.getText(parsedSource)]
      : [];
  })
  .join('\n');
const compiledSource = ts.transpileModule(initializationSource, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
}).outputText;

describe('WebSessionPanel initialization with cached tools', () => {
  it.each([false, true])(
    'shows the pending question once and restores its history (source ID: %s)',
    hasSourceId => {
      const question = {
        id: 'row-1',
        key: 'row-1',
        kind: 'system',
        sourceItemId: hasSourceId ? 'question-1' : undefined,
        detail: { type: 'user_input_request' },
      };
      const pending = ref<{ itemId: string } | null>({
        itemId: hasSourceId ? 'question-1' : 'row-1',
      });
      const otherQuestion = {
        id: 'other',
        key: 'other',
        kind: 'system',
        detail: { type: 'user_input_request' },
      };
      const asyncQuestion = {
        id: 'async',
        key: 'async',
        kind: 'assistant',
        detail: { type: 'async_user_input_request' },
      };
      let initialBlocks: unknown;
      const dependencies = {
        computed,
        currentRealSession: ref({ id: 'session-1' }),
        currentSession: ref({ agent: 'codex' }),
        webSessionStore: {
          getLiveState: () => ({ running: false }),
          getPendingApproval: () => null,
          getPendingUserInput: () => pending.value,
        },
        timelineBlocks: ref([question, otherQuestion, asyncQuestion]),
        showWebSessionReasoning: ref(true),
        isInteractiveDynamicTool: () => false,
        isReasoningBlock: () => false,
        isPlanChoiceRequestBlock: () => false,
        shouldShowToolPendingPlaceholder: () => false,
        projectWebSessionVisibleTimelineBlocks: (blocks: unknown) => blocks,
        captureVisibleBlocks: (blocks: unknown) => {
          initialBlocks = blocks;
        },
      };
      const initialize = new Function(
        ...Object.keys(dependencies),
        compiledSource + '\nreturn visibleBlocks;'
      );
      const visible = initialize(...Object.values(dependencies));
      expect(initialBlocks).toEqual([otherQuestion, asyncQuestion]);
      pending.value = null;
      expect(visible.value).toEqual([question, otherQuestion, asyncQuestion]);
    }
  );

  it.each([false, true])(
    'initializes timeline dependencies before search (running: %s)',
    running => {
      const cachedBlock = {
        key: 'tool-1',
        kind: 'tool',
        tool: { id: 'tool-1', status: running ? 'running' : 'completed' },
      };
      let visibleBlocks: unknown;
      const dependencies = {
        computed,
        currentRealSession: ref({ id: 'session-1' }),
        currentSession: ref({ agent: 'codex' }),
        webSessionStore: {
          getLiveState: () => ({ running, tool: { id: 'tool-1' } }),
          getPendingApproval: () => null,
          getPendingUserInput: () => null,
        },
        timelineBlocks: ref([cachedBlock]),
        showWebSessionReasoning: ref(true),
        isInteractiveDynamicTool: () => false,
        isReasoningBlock: () => false,
        isPlanChoiceRequestBlock: () => false,
        shouldShowToolPendingPlaceholder: () => false,
        projectWebSessionVisibleTimelineBlocks: (blocks: unknown) => blocks,
        captureVisibleBlocks: (blocks: unknown) => {
          visibleBlocks = blocks;
        },
      };

      const initialize = new Function(...Object.keys(dependencies), compiledSource);
      expect(() => initialize(...Object.values(dependencies))).not.toThrow();
      expect(visibleBlocks).toEqual(running ? [] : [cachedBlock]);
    }
  );

  it.each([null, 'approval-1', 'approval-other'])(
    'initializes cached approval history before search (pending: %s)',
    pendingItemId => {
      const message = { key: 'message-1', kind: 'message' };
      const approval = {
        id: 'approval-1',
        key: 'approval-1',
        kind: 'system',
        sourceItemId: 'approval-1',
        detail: { type: 'approval_request' },
      };
      const pending = ref(pendingItemId ? { itemId: pendingItemId } : null);
      const currentRealSession = ref<{ id: string } | null>({ id: 'session-1' });
      let initialBlocks: unknown;
      const dependencies = {
        computed,
        currentRealSession,
        currentSession: ref({ agent: 'codex' }),
        webSessionStore: {
          getLiveState: () => ({ running: false }),
          getPendingApproval: () => pending.value,
          getPendingUserInput: () => null,
        },
        timelineBlocks: ref([message, approval]),
        showWebSessionReasoning: ref(true),
        isInteractiveDynamicTool: () => false,
        isReasoningBlock: () => false,
        isPlanChoiceRequestBlock: () => false,
        shouldShowToolPendingPlaceholder: () => false,
        projectWebSessionVisibleTimelineBlocks: (blocks: unknown) => blocks,
        captureVisibleBlocks: (blocks: unknown) => {
          initialBlocks = blocks;
        },
      };
      const initialize = new Function(
        ...Object.keys(dependencies),
        compiledSource + '\nreturn visibleBlocks;'
      );
      const visible = initialize(...Object.values(dependencies));
      expect(initialBlocks).toEqual(
        pendingItemId === 'approval-1' ? [message] : [message, approval]
      );
      pending.value = { itemId: 'approval-1' };
      expect(visible.value).toEqual([message]);
      pending.value = null;
      expect(visible.value).toEqual([message, approval]);
      pending.value = { itemId: 'approval-1' };
      currentRealSession.value = null;
      expect(visible.value).toEqual([message, approval]);
    }
  );
});
