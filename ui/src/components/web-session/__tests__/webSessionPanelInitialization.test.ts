import { readFileSync } from 'node:fs';
import { computed, ref } from 'vue';
import ts from 'typescript';
import { describe, expect, it } from 'vitest';

const panelSource = readFileSync(new URL('../WebSessionPanel.vue', import.meta.url), 'utf8');
const setupSource = panelSource.split('<script setup lang="ts">')[1]!.split('</script>')[0]!;
const parsedSource = ts.createSourceFile('panel.ts', setupSource, ts.ScriptTarget.Latest, true);
const selectedNames = new Set([
  'liveState',
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
});
