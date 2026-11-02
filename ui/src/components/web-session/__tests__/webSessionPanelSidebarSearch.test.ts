import { readFileSync } from 'node:fs';
import { useDebounceFn } from '@vueuse/core';
import { flushPromises } from '@vue/test-utils';
import { computed, effectScope, nextTick, reactive, ref, watch, type Ref } from 'vue';
import ts from 'typescript';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { SessionSearchChunkResult, webSessionApi } from '@/api/webSession';
import {
  mergeWebSessionSidebarSearchPage,
  normalizeWebSessionSidebarSearchQuery,
} from '@/components/web-session/webSessionSidebarSearch';
import {
  resolveWebSessionSidebarProjectIds,
  resolveWebSessionSidebarProjectIdsKey,
  type WebSessionSidebarScope,
} from '@/components/web-session/webSessionSidebarScope';
import type { WebSessionSummary } from '@/types/models';

const panelSource = readFileSync(new URL('../WebSessionPanel.vue', import.meta.url), 'utf8');
const setupSource = panelSource.split('<script setup lang="ts">')[1]!.split('</script>')[0]!;
const parsedSource = ts.createSourceFile('panel.ts', setupSource, ts.ScriptTarget.Latest, true);
const selectedNames = new Set([
  'createSidebarSearchState',
  'sidebarSearchState',
  'sidebarSearchRequestVersion',
  'sidebarSearchAbortController',
  'normalizedSidebarSearchQuery',
  'sidebarProjectIdsToLoad',
  'sidebarVisibleProjectIds',
  'sidebarSearchScopeKey',
  'cancelSidebarSearchRequest',
  'clearSidebarSearchState',
  'loadSidebarSearch',
  'scheduleSidebarSearch',
]);
// Exercise the panel's request loop and watcher without mounting its unrelated UI.
const searchSource = parsedSource.statements
  .filter(statement => {
    if (ts.isFunctionDeclaration(statement)) return selectedNames.has(statement.name?.text ?? '');
    if (ts.isVariableStatement(statement)) {
      return statement.declarationList.declarations.some(
        declaration => ts.isIdentifier(declaration.name) && selectedNames.has(declaration.name.text)
      );
    }
    return (
      ts.isExpressionStatement(statement) &&
      ts.isCallExpression(statement.expression) &&
      statement.expression.expression.getText(parsedSource) === 'watch' &&
      statement.getText(parsedSource).includes('scheduleSidebarSearch()')
    );
  })
  .map(statement => statement.getText(parsedSource))
  .join('\n');
const compiledSource = ts.transpileModule(searchSource, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
}).outputText;

type SearchState = {
  items: WebSessionSummary[];
  scanned: number;
  total: number;
  loading: boolean;
  done: boolean;
  error: boolean;
};
type SearchRequest = {
  data: Parameters<typeof webSessionApi.search>[0];
  signal: AbortSignal;
  resolve: (result: SessionSearchChunkResult) => void;
};

const cleanups: Array<() => void> = [];
afterEach(() => {
  cleanups.splice(0).forEach(cleanup => cleanup());
  vi.useRealTimers();
});

function createHarness(scope: WebSessionSidebarScope = 'all') {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  const requests: SearchRequest[] = [];
  const search = vi.fn<typeof webSessionApi.search>((data, options) => {
    return new Promise(resolve => requests.push({ data, signal: options!.signal!, resolve }));
  });
  const dependencies = {
    computed,
    ref,
    watch,
    useDebounceFn,
    normalizeWebSessionSidebarSearchQuery,
    resolveWebSessionSidebarProjectIds,
    resolveWebSessionSidebarProjectIdsKey,
    mergeWebSessionSidebarSearchPage,
    isAbortLikeError: (error: unknown) => error instanceof Error && error.name === 'AbortError',
    SIDEBAR_SEARCH_SCAN_LIMIT: 50,
    props: reactive({ projectId: 'project-1' }),
    projectStore: reactive({
      projects: [{ id: 'project-1' }, { id: 'project-2' }, { id: 'project-3' }],
      recentProjects: [{ id: 'project-1' }, { id: 'project-2' }],
    }),
    sidebarScope: ref(scope),
    sidebarSearchQuery: ref(''),
    sidebarSearchArchived: ref(false),
    sidebarSearchBody: ref(true),
    webSessionApi: { search },
  };
  const initialize = new Function(
    ...Object.keys(dependencies),
    compiledSource + '\nreturn { sidebarSearchState, cancelSidebarSearchRequest };'
  );
  const effect = effectScope();
  const bindings = effect.run(() => initialize(...Object.values(dependencies))) as {
    sidebarSearchState: Ref<SearchState>;
    cancelSidebarSearchRequest: () => void;
  };
  cleanups.push(() => {
    effect.stop();
    bindings.cancelSidebarSearchRequest();
    vi.clearAllTimers();
  });
  return { ...dependencies, ...bindings, search, requests };
}

async function startSearch(harness: ReturnType<typeof createHarness>) {
  harness.sidebarSearchQuery.value = '10.126.126.4';
  await nextTick();
  await vi.advanceTimersByTimeAsync(300);
  expect(harness.search).toHaveBeenCalledOnce();
}

function chunk(id: string, nextCursor?: string): SessionSearchChunkResult {
  return {
    items: [{ id }] as WebSessionSummary[],
    scanned: 50,
    total: 150,
    done: !nextCursor,
    nextCursor,
  };
}

describe('WebSessionPanel sidebar search across session switches', () => {
  it('continues pagination and preserves progress when all-scope projects reorder', async () => {
    const harness = createHarness();
    await startSearch(harness);
    harness.requests[0]!.resolve(chunk('match-1', 'page-2'));
    await flushPromises();
    const pending = harness.requests[1]!;
    const progress = harness.sidebarSearchState.value;
    expect(progress.scanned).toBe(50);

    harness.props.projectId = 'project-2';
    await nextTick();
    harness.projectStore.recentProjects.reverse();
    await nextTick();
    await vi.advanceTimersByTimeAsync(300);
    expect(pending.signal.aborted).toBe(false);
    expect(harness.sidebarSearchState.value).toBe(progress);
    expect(harness.search).toHaveBeenCalledTimes(2);

    pending.resolve(chunk('match-2', 'page-3'));
    await flushPromises();
    expect(harness.sidebarSearchState.value.scanned).toBe(100);
    expect(harness.requests[2]!.data).toMatchObject({
      projectIds: harness.requests[0]!.data.projectIds,
      cursor: 'page-3',
    });
    harness.requests[2]!.resolve(chunk('match-3'));
    await flushPromises();
    expect(harness.sidebarSearchState.value).toMatchObject({
      items: [{ id: 'match-1' }, { id: 'match-2' }, { id: 'match-3' }],
      scanned: 150,
      loading: false,
      done: true,
      error: false,
    });
  });

  it('keeps completed results when switching projects in all scope', async () => {
    const harness = createHarness();
    await startSearch(harness);
    harness.requests[0]!.resolve(chunk('match-1'));
    await flushPromises();
    const result = harness.sidebarSearchState.value;

    harness.props.projectId = 'project-3';
    harness.projectStore.projects.reverse();
    await nextTick();
    await vi.advanceTimersByTimeAsync(300);
    expect(harness.search).toHaveBeenCalledOnce();
    expect(harness.sidebarSearchState.value).toBe(result);
  });

  it.each(['query', 'body', 'archived', 'projects', 'current project'] as const)(
    'restarts for a changed %s and discards the old response',
    async change => {
      const harness = createHarness(change === 'current project' ? 'current' : 'all');
      await startSearch(harness);
      const previous = harness.requests[0]!;
      if (change === 'query') harness.sidebarSearchQuery.value = 'another address';
      if (change === 'body') harness.sidebarSearchBody.value = false;
      if (change === 'archived') harness.sidebarSearchArchived.value = true;
      if (change === 'projects') harness.projectStore.projects.push({ id: 'project-4' });
      if (change === 'current project') harness.props.projectId = 'project-2';
      await nextTick();
      expect(previous.signal.aborted).toBe(true);
      await vi.advanceTimersByTimeAsync(300);
      expect(harness.search).toHaveBeenCalledTimes(2);
      expect(harness.requests[1]!.data.cursor).toBe('');
      if (change === 'current project') {
        expect(harness.requests[1]!.data.projectIds).toEqual(['project-2']);
      }

      previous.resolve(chunk('stale-match', 'stale-page'));
      await flushPromises();
      expect(harness.sidebarSearchState.value.items).toEqual([]);
      harness.requests[1]!.resolve(chunk('new-match'));
      await flushPromises();
      expect(harness.sidebarSearchState.value.items).toEqual([{ id: 'new-match' }]);
      expect(harness.search).toHaveBeenCalledTimes(2);
    }
  );
});
