import { readFileSync } from 'node:fs';
import { computed, nextTick, ref, watch } from 'vue';
import ts from 'typescript';
import { describe, expect, it, vi } from 'vitest';

import * as modelHelpers from '../webSessionModelOptions';

const panelSource = readFileSync(new URL('../WebSessionPanel.vue', import.meta.url), 'utf8');
const setupSource = panelSource.split('<script setup lang="ts">')[1]!.split('</script>')[0]!;
const parsedSource = ts.createSourceFile('panel.ts', setupSource, ts.ScriptTarget.Latest, true);
const pickerNames = new Set([
  'selectedAgent',
  'selectedModel',
  'devinModelOptionGroups',
  'devinSpecialModelOptions',
  'filteredDevinModelOptionGroups',
  'modelOptions',
]);
const pickerSource = parsedSource.statements
  .filter(
    statement =>
      ts.isVariableStatement(statement) &&
      statement.declarationList.declarations.some(
        declaration => ts.isIdentifier(declaration.name) && pickerNames.has(declaration.name.text)
      )
  )
  .map(statement => statement.getText(parsedSource))
  .join('\n');
const compiledSource = ts.transpileModule(pickerSource, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
}).outputText;

function createPicker() {
  const currentSession = ref({
    id: 'session-1',
    agent: 'devin',
    model: 'swe-2-high',
    status: 'running',
    lastEventSeq: 1,
    totalOutputTokens: 100,
  });
  const runtimeConfig = ref({
    devinModels: [
      { model: 'swe-2-high', displayName: 'SWE-2 High', family: 'SWE-2' },
      { model: 'swe-2-max', displayName: 'SWE-2 Max', family: 'SWE-2' },
      { model: 'claude-medium', displayName: 'Claude Medium', family: 'Claude' },
    ],
  });
  const modelSearchQuery = ref('');
  const devinFrequentModelValues = ref(['swe-2-high']);
  const dependencies = {
    ...modelHelpers,
    computed,
    currentSession,
    draftAgent: ref('devin'),
    draftModel: ref('swe-2-high'),
    runtimeConfig,
    devinFrequentModelValues,
    modelSearchQuery,
    customModelOptions: ref([]),
    t: (key: string) => key,
  };
  const initialize = new Function(
    ...Object.keys(dependencies),
    compiledSource + '\nreturn modelOptions;'
  );
  const options = initialize(...Object.values(dependencies)) as ReturnType<
    typeof computed<
      Array<modelHelpers.WebSessionModelOption | modelHelpers.WebSessionModelOptionGroup>
    >
  >;
  return { currentSession, runtimeConfig, modelSearchQuery, devinFrequentModelValues, options };
}

describe('model picker during session updates', () => {
  it('keeps the open menu options stable across streaming session summaries', async () => {
    const { currentSession, options } = createPicker();
    const initialOptions = options.value;
    const onOptionsChange = vi.fn();
    const stop = watch(options, onOptionsChange);

    for (let index = 2; index <= 5; index += 1) {
      currentSession.value = {
        ...currentSession.value,
        lastEventSeq: index,
        totalOutputTokens: index * 100,
        status: index === 5 ? 'done' : 'running',
      };
      await nextTick();
      expect(options.value).toBe(initialOptions);
    }
    expect(onOptionsChange).not.toHaveBeenCalled();
    stop();
  });

  it('updates family options when the selected model changes', () => {
    const { currentSession, options } = createPicker();
    const initialOptions = options.value;
    currentSession.value = { ...currentSession.value, model: 'swe-2-max' };

    expect(options.value).not.toBe(initialOptions);
    const all = options.value.find(option => 'key' in option && option.key === 'devin-all');
    expect(all && 'children' in all && all.children.map(option => option.value)).toContain(
      'swe-2-max'
    );
  });

  it('continues to update the menu for searches, recent models, and catalog refreshes', () => {
    const { modelSearchQuery, devinFrequentModelValues, runtimeConfig, options } = createPicker();
    const initialOptions = options.value;
    modelSearchQuery.value = 'Claude';
    expect(options.value).not.toBe(initialOptions);
    expect(options.value.flatMap(option => ('children' in option ? option.children : []))).toEqual([
      expect.objectContaining({ value: 'claude-medium' }),
    ]);

    modelSearchQuery.value = '';
    const unfilteredOptions = options.value;
    devinFrequentModelValues.value = ['claude-medium', 'swe-2-high'];
    expect(options.value).not.toBe(unfilteredOptions);
    const recent = options.value.find(option => 'key' in option && option.key === 'devin-recent');
    expect(recent && 'children' in recent && recent.children[0]?.value).toBe('claude-medium');

    const recentOptions = options.value;
    runtimeConfig.value = {
      devinModels: [
        ...runtimeConfig.value.devinModels,
        { model: 'new-model', displayName: 'New model', family: 'New family' },
      ],
    };
    expect(options.value).not.toBe(recentOptions);
    expect(
      options.value.flatMap(option => ('children' in option ? option.children : []))
    ).toContainEqual(expect.objectContaining({ value: 'new-model' }));
  });
});
