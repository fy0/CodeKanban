import { describe, expect, it } from 'vitest';
import type { WebSessionCodexModelInfo } from '@/types/models';
import {
  codexModelSelectionValue,
  parseCodexModelSelection,
  rememberCodexModelSelection,
  resolveCodexModelOptionGroups,
} from '../webSessionCodexModelPicker';

const models: WebSessionCodexModelInfo[] = [
  {
    model: 'gpt-6.1-sol',
    displayName: 'GPT-6.1 Sol',
    defaultReasoningEffort: 'low',
    supportedReasoningEfforts: ['low', 'medium', 'high', 'ultra'],
  },
  {
    model: 'gpt-5.5',
    displayName: 'GPT-5.5',
    defaultReasoningEffort: 'high',
    supportedReasoningEfforts: ['low', 'high'],
  },
  {
    model: 'gpt-5.4-mini',
    displayName: 'GPT-5.4 mini',
    defaultReasoningEffort: 'low',
    supportedReasoningEfforts: ['low', 'high'],
  },
];

describe('Codex model picker', () => {
  it('keeps different strengths of the same model and promotes only the exact reused pair', () => {
    let recent = rememberCodexModelSelection([], 'gpt-6.1-sol', 'high');
    recent = rememberCodexModelSelection(recent, 'gpt-6.1-sol', 'low');
    recent = rememberCodexModelSelection(recent, ' gpt-6.1-sol ', 'high');
    expect(recent.map(parseCodexModelSelection)).toEqual([
      { model: 'gpt-6.1-sol', reasoningEffort: 'high' },
      { model: 'gpt-6.1-sol', reasoningEffort: 'low' },
    ]);
    for (let index = 0; index < 8; index++) {
      recent = rememberCodexModelSelection(recent, 'custom-' + index, 'low');
    }
    expect(recent).toHaveLength(6);
  });

  it('round trips custom IDs without mistaking raw models or corrupt history for combinations', () => {
    const model = 'provider/model:with#dup';
    expect(parseCodexModelSelection(codexModelSelectionValue(model, 'ultra'))).toEqual({
      model,
      reasoningEffort: 'ultra',
    });
    for (const value of [
      'gpt-6.1-sol',
      '__codex_selection__:broken',
      '__codex_selection__:["m","invalid"]',
      '__codex_selection__:["","low"]',
    ]) {
      expect(parseCodexModelSelection(value)).toBeNull();
    }
  });

  it('groups recent combinations before all model families with unique values', () => {
    const recentValues = [
      codexModelSelectionValue('gpt-6.1-sol', 'high'),
      codexModelSelectionValue('gpt-6.1-sol', 'low'),
    ];
    const groups = resolveCodexModelOptionGroups({
      models,
      recentValues,
      labels: { recent: '最近使用' },
    });
    expect(groups.map(group => group.label)).toEqual(['最近使用', 'All Models']);
    expect(
      groups[0]?.children.map(option => [option.menuLabel, option.accentLabel, option.removable])
    ).toEqual([
      ['GPT-6.1 Sol', 'High', true],
      ['GPT-6.1 Sol', 'Low', true],
    ]);
    expect(groups[1]?.children.map(option => option.value)).toEqual([
      'gpt-6.1-sol',
      'gpt-5.5',
      'gpt-5.4-mini',
    ]);
    const values = groups.flatMap(group => group.children.map(option => option.value));
    expect(new Set(values).size).toBe(values.length);
  });

  it('keeps search results to one row per model while retaining recently used combinations', () => {
    const groups = resolveCodexModelOptionGroups({
      models,
      query: '  SOL ',
      recentValues: [codexModelSelectionValue('gpt-6.1-sol', 'ultra')],
    });
    expect(groups.map(group => group.key)).toEqual(['codex-recent', 'codex-all']);
    expect(parseCodexModelSelection(groups[0]?.children[0]?.value ?? '')).toEqual({
      model: 'gpt-6.1-sol',
      reasoningEffort: 'ultra',
    });
    expect(groups[1]?.children.map(option => option.value)).toEqual(['gpt-6.1-sol']);
    expect(groups[1]?.children[0]?.accentLabel).toBeUndefined();
    expect(resolveCodexModelOptionGroups({ models, query: 'mini ultra' })).toEqual([]);
  });

  it('searches translated strength labels within recently used combinations', () => {
    const groups = resolveCodexModelOptionGroups({
      models,
      recentValues: [codexModelSelectionValue('gpt-6.1-sol', 'default')],
      query: '6.1 默认',
      effortLabel: effort => (effort === 'default' ? '默认' : effort),
    });
    expect(groups).toHaveLength(1);
    expect(groups[0]?.key).toBe('codex-recent');
    expect(groups[0]?.children[0]?.accentLabel).toBe('默认');
  });

  it('drops unsupported, hidden and removed model combinations while retaining explicit custom models', () => {
    const hidden = { ...models[1]!, hidden: true };
    const recentValues = [
      codexModelSelectionValue('gpt-5.5', 'high'),
      codexModelSelectionValue('gpt-6.1-sol', 'max'),
      codexModelSelectionValue('removed-model', 'high'),
      codexModelSelectionValue('custom-model', 'high'),
      codexModelSelectionValue('codex-auto-review', 'high'),
      'corrupt',
    ];
    const groups = resolveCodexModelOptionGroups({
      models: [models[0]!, hidden],
      recentValues,
      customModels: ['custom-model', 'gpt-5.5', 'codex-auto-review'],
      currentModel: 'removed-model',
    });
    expect(groups[0]?.children.map(option => parseCodexModelSelection(option.value))).toEqual([
      { model: 'custom-model', reasoningEffort: 'high' },
    ]);
    const values = groups.flatMap(group => group.children.map(option => option.value));
    for (const excluded of ['removed-model', 'gpt-5.5', 'codex-auto-review']) {
      expect(values).not.toContain(excluded);
    }
  });

  it('uses the offline catalog and its known reasoning strengths when runtime discovery is unavailable', () => {
    const groups = resolveCodexModelOptionGroups({ query: '.6' });
    const options = groups.flatMap(group => group.children);
    expect(options.map(option => option.value)).toEqual([
      'gpt-5.6-luna',
      'gpt-5.6-terra',
      'gpt-5.6-sol',
    ]);
    expect(options.every(option => option.accentLabel === undefined)).toBe(true);
    expect(options.find(option => option.value === 'gpt-5.6-sol')?.detail?.efforts).toContain(
      'ultra'
    );
  });
});
