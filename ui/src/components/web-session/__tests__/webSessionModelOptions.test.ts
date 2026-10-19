import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import {
  CLAUDE_MODEL_OPTIONS,
  CLAUDE_RUNTIME_OPTIONS,
  CODEX_ADDITIONAL_MODEL_OPTIONS,
  CODEX_MODEL_OPTIONS,
  CODEX_PRIMARY_MODEL_OPTIONS,
  CUSTOM_MODEL_VALUE,
  MORE_MODELS_VALUE,
  defaultModelForAgent,
  defaultPermissionLevelForAgent,
  defaultReasoningEffortForAgent,
  duplicateDevinModelOptionValue,
  filterPiModelOptionGroups,
  normalizeDevinModelOptionValue,
  parseDevinCostSummary,
  rememberCustomModel,
  rememberPiFrequentModel,
  removeCustomModel,
  resolveCCRModelOptions,
  resolveCodexModelMenuOptions,
  resolveCodexModelOption,
  resolveCodexModelOptions,
  resolveCodexReasoningEfforts,
  resolveCustomModelOptions,
  resolveDevinFusionModel,
  resolveDevinFusionModelConfigs,
  resolveDevinModelForReasoning,
  resolveDevinModelOptionGroups,
  resolveDevinModelOptions,
  resolveDevinReasoningEfforts,
  resolveDevinSelectedReasoningEffort,
  resolveDevinSpecialModelOptions,
  resolvePiModelOptionGroups,
  resolvePiModelOptions,
  resolvePiPrimaryModelOptions,
  resolvePiReasoningEfforts,
  shouldSuppressPiModelMenuClose,
} from '@/components/web-session/webSessionModelOptions';

const panelSource = readFileSync(new URL('../WebSessionPanel.vue', import.meta.url), 'utf8');

function codexModel(model: string, displayName = model) {
  return {
    model,
    displayName,
    defaultReasoningEffort: 'low' as const,
    supportedReasoningEfforts: ['low' as const, 'high' as const],
  };
}

describe('webSessionModelOptions', () => {
  it('opens the shared model search catalog directly and keeps searches when entering the menu', () => {
    expect(panelSource).toContain('<template v-if="hasModelSearch" #header>');
    expect(panelSource).not.toContain('showAllPiModels');
    expect(panelSource).toContain('show === showModelSelector.value ||');
  });

  it('controls model detail popovers and dismisses them through the select scroll event', () => {
    expect(panelSource).toContain('@scroll="clearModelOptionPopovers"');
    expect(panelSource).toMatch(
      /show:\s*showModelSelector\.value &&\s*modelOptionPopoverValues\.value\.has/
    );
    expect(panelSource).toMatch(
      /function clearModelOptionPopovers\(\) \{\s*modelOptionPopoverValues\.value = new Set\(\);/
    );
    expect(panelSource).toContain('animated: false');
    expect(panelSource.match(/@update:value="clearModelOptionPopovers"/g)).toHaveLength(1);
  });

  it('uses the Devin catalog for model, reasoning, and pricing metadata', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
        supportedReasoningEfforts: ['high' as const],
        costTier: 'Free',
      },
      {
        model: 'swe-2-medium',
        displayName: 'SWE-2 Medium',
        family: 'SWE-2',
        defaultReasoningEffort: 'medium' as const,
        supportedReasoningEfforts: ['medium' as const],
        costSummary: '$1 / 1M Output',
      },
    ];
    expect(resolveDevinModelOptions(models)[1].description).toBe('$1 / 1M Output');
    expect(resolveDevinReasoningEfforts(models, 'swe-2-high')).toEqual(['high', 'medium']);
    expect(resolveDevinModelForReasoning(models, 'swe-2-high', 'medium')).toBe('swe-2-medium');
    expect(resolveDevinSelectedReasoningEffort(models, 'swe-2-high', 'default')).toBe('high');
    expect(resolveDevinModelOptionGroups(models).map(group => group.label)).toEqual(['All Models']);
  });

  it('collapses Devin family variants into a single option named by the family', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
      },
      {
        model: 'swe-2-medium',
        displayName: 'SWE-2 Medium',
        family: 'SWE-2',
        defaultReasoningEffort: 'medium' as const,
      },
      {
        model: 'glm-5-2-max',
        displayName: 'GLM-5.2 Max',
        family: 'GLM-5.2',
        defaultReasoningEffort: 'max' as const,
        recommended: true,
      },
    ];
    const groups = resolveDevinModelOptionGroups(models);
    expect(groups.map(group => group.label)).toEqual(['Recommended', 'All Models']);
    const all = groups.find(group => group.key === 'devin-all');
    expect(all?.children.map(option => option.label)).toEqual(['SWE-2']);
    expect(all?.children[0].value).toBe('swe-2-high');
    expect(all?.children[0].detail?.efforts).toEqual(['medium', 'high']);
  });

  it('points a Devin family option at the currently selected variant', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
      },
      {
        model: 'swe-2-low',
        displayName: 'SWE-2 Low',
        family: 'SWE-2',
        defaultReasoningEffort: 'low' as const,
      },
    ];
    const groups = resolveDevinModelOptionGroups(models, [], 'swe-2-low');
    expect(groups[0].children[0].value).toBe('swe-2-low');
    expect(groups[0].children[0].label).toBe('SWE-2');
  });

  it('maps Devin catalog status fields to option badges without implying recommended', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
        isNew: true,
      },
      {
        model: 'glm-5-2-max',
        displayName: 'GLM-5.2 Max',
        family: 'GLM-5.2',
        defaultReasoningEffort: 'max' as const,
        isBeta: true,
        costTier: 'promotion',
      },
    ];
    const options = resolveDevinModelOptions(models);
    expect(options[0].badges).toEqual(['new']);
    expect(options[1].badges).toEqual(['promo', 'beta']);
    // Status badges do not populate the Recommended group.
    expect(resolveDevinModelOptionGroups(models).map(group => group.label)).toEqual(['All Models']);
  });

  it('dedupes Devin recent models and resolves recently used variants', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
      },
      {
        model: 'swe-2-low',
        displayName: 'SWE-2 Low',
        family: 'SWE-2',
        defaultReasoningEffort: 'low' as const,
      },
    ];
    const groups = resolveDevinModelOptionGroups(
      models,
      ['swe-2-low', 'swe-2-low', 'missing-model', ' swe-2-high '],
      'swe-2-low'
    );
    const recent = groups.find(group => group.key === 'devin-recent');
    expect(recent?.label).toBe('Recently Used');
    expect(recent?.children.map(option => option.value)).toEqual(['swe-2-low', 'swe-2-high']);
    expect(recent?.children[0].label).toBe('SWE-2');
    expect(recent?.children[0].menuLabel).toBe('SWE-2');
    expect(recent?.children[0].accentLabel).toBe('Low');
    expect(recent?.children[0].removable).toBe(true);
  });

  it('keeps a recently used Devin model as the only option carrying its value', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
      },
      {
        model: 'swe-2-low',
        displayName: 'SWE-2 Low',
        family: 'SWE-2',
        defaultReasoningEffort: 'low' as const,
      },
      {
        model: 'glm-5-2-max',
        displayName: 'GLM-5.2 Max',
        family: 'GLM-5.2',
        defaultReasoningEffort: 'max' as const,
      },
    ];
    const groups = resolveDevinModelOptionGroups(models, ['swe-2-low'], 'swe-2-low');
    const all = groups.find(group => group.key === 'devin-all');
    const values = groups.flatMap(group => group.children.map(option => option.value));
    expect(values.filter(value => value === 'swe-2-low')).toHaveLength(1);
    expect(all?.children.map(option => option.value)).toEqual([
      duplicateDevinModelOptionValue('swe-2-low'),
      'glm-5-2-max',
    ]);
    expect(normalizeDevinModelOptionValue(duplicateDevinModelOptionValue('swe-2-low'))).toBe(
      'swe-2-low'
    );
    expect(normalizeDevinModelOptionValue('swe-2-low')).toBe('swe-2-low');
  });

  it('marks removable Devin recent options without an accent for special families', () => {
    const models = [
      {
        model: 'fusion-a',
        displayName: 'Fusion (Claude Fable 5.1 Medium + SWE-2 Medium)',
        family: 'Fusion',
      },
      {
        model: 'adaptive-a',
        displayName: 'Adaptive (Auto)',
        family: 'Adaptive',
      },
    ];
    const groups = resolveDevinModelOptionGroups(models, ['fusion-a', 'adaptive-a']);
    const recent = groups.find(group => group.key === 'devin-recent');
    expect(recent?.children.map(option => option.value)).toEqual(['fusion-a', 'adaptive-a']);
    expect(recent?.children[0].removable).toBe(true);
    expect(recent?.children[0].menuLabel).toBe('Fusion (Claude Fable 5.1 Medium + SWE-2 Medium)');
    expect(recent?.children[0].accentLabel).toBeUndefined();
    expect(recent?.children[1].accentLabel).toBeUndefined();
  });

  it('exposes the Devin input price on model options', () => {
    const models = [
      {
        model: 'glm-5-2-max',
        displayName: 'GLM-5.2 Max',
        family: 'GLM-5.2',
        costSummary:
          '$10 / 1M Input · $30 / 1M Output · $1 / 1M Cache Read · $5 / 1M Cache Write · $2 / 1M Refresh · $4 / 1M Long Context',
      },
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
      },
    ];
    const options = resolveDevinModelOptions(models);
    expect(options[0].inputPrice).toBe('$10/1M');
    expect(options[1].inputPrice).toBeUndefined();
  });

  it('follows the selected special Devin model for fusion and adaptive options', () => {
    const models = [
      {
        model: 'fusion-a',
        displayName: 'Fusion (Claude Fable 5.1 Medium + SWE-2 Medium)',
        family: 'Fusion',
      },
      {
        model: 'fusion-b',
        displayName: 'Fusion (Claude Fable 5.1 High + SWE-2 Low)',
        family: 'Fusion',
      },
    ];
    expect(resolveDevinSpecialModelOptions(models)[0].value).toBe('fusion-a');
    expect(resolveDevinSpecialModelOptions(models, 'fusion-b')[0].value).toBe('fusion-b');
  });

  it('parses Devin fusion configs with sidekick effort and resolves selections', () => {
    const models = [
      {
        model: 'fusion-a',
        displayName: 'Fusion (Claude Fable 5.1 Medium + SWE-2 Medium)',
        family: 'Fusion',
      },
      {
        model: 'fusion-b',
        displayName: 'Fusion (Claude Fable 5.1 Medium + SWE-2 High)',
        family: 'Fusion',
      },
      {
        model: 'fusion-c',
        displayName: 'Fusion (Claude Fable 5.1 Medium Thinking + SWE-2 Medium Fast)',
        family: 'Fusion',
      },
    ];
    const configs = resolveDevinFusionModelConfigs(models);
    expect(configs).toEqual([
      {
        model: 'fusion-a',
        lead: 'Claude Fable 5.1',
        effort: 'Medium',
        sidekick: 'SWE-2',
        sidekickEffort: 'Medium',
        fast: false,
      },
      {
        model: 'fusion-b',
        lead: 'Claude Fable 5.1',
        effort: 'Medium',
        sidekick: 'SWE-2',
        sidekickEffort: 'High',
        fast: false,
      },
      {
        model: 'fusion-c',
        lead: 'Claude Fable 5.1',
        effort: 'Medium',
        sidekick: 'SWE-2',
        sidekickEffort: 'Medium',
        fast: true,
      },
    ]);
    expect(
      resolveDevinFusionModel(configs, {
        lead: 'Claude Fable 5.1',
        effort: 'Medium',
        sidekick: 'SWE-2',
        sidekickEffort: 'High',
        fast: false,
      })
    ).toBe('fusion-b');
    expect(
      resolveDevinFusionModel(configs, {
        lead: 'Missing',
        effort: 'Low',
        sidekick: 'None',
        fast: false,
      })
    ).toBe('fusion-a');
  });
  it('replaces local models with the runtime catalog while retaining compact labels', () => {
    const options = resolveCodexModelOptions([
      codexModel('gpt-6.1-sol', 'GPT-6.1 Sol'),
      codexModel('gpt-5.4-pro'),
      codexModel('future-model', 'Future Model'),
    ]);

    expect(options).toEqual([
      { label: '6.1S', value: 'gpt-6.1-sol', menuLabel: 'GPT-6.1 Sol' },
      { label: '5.4Pro', value: 'gpt-5.4-pro', menuLabel: 'GPT-5.4 Pro' },
      { label: 'Future Model', value: 'future-model', menuLabel: 'Future Model' },
    ]);
    expect(options.some(option => option.value === 'gpt-5.6-sol')).toBe(false);
  });

  it('normalizes runtime entries and removes duplicates without adding local models', () => {
    expect(
      resolveCodexModelOptions([
        codexModel(' '),
        codexModel(' gpt-6.1-sol ', ' Custom display name '),
        codexModel('GPT-6.1-SOL', 'Duplicate'),
        codexModel('custom-model', ''),
      ])
    ).toEqual([
      { label: '6.1S', value: 'gpt-6.1-sol', menuLabel: 'Custom display name' },
      { label: 'custom-model', value: 'custom-model', menuLabel: 'custom-model' },
    ]);
  });

  it.each(['gpt-daybreak-blue-latest', 'gpt-daybreak-red-latest'])(
    'uses the returned visibility for %s without blocking its name',
    model => {
      expect(resolveCodexModelOptions([{ ...codexModel(model), hidden: false }])).toEqual([
        resolveCodexModelOption(model),
      ]);
      expect(resolveCodexModelOptions([{ ...codexModel(model), hidden: true }])).toEqual([]);
    }
  );

  it('does not restore a model missing from the live catalog just because it is selected', () => {
    const catalog = [codexModel('gpt-6.1-sol')];
    for (const current of ['gpt-daybreak-blue-latest', 'retired-model', 'codex-auto-review']) {
      const menu = resolveCodexModelMenuOptions(catalog, [], current);
      expect(menu.primary.map(option => option.value)).toEqual(['gpt-6.1-sol']);
      expect(menu.additional).toEqual([]);
    }
  });

  it('hides Auto Review from catalog, custom, and current options even when marked visible', () => {
    const catalog = [
      codexModel('gpt-6.1-sol'),
      { ...codexModel('codex-auto-review'), hidden: false },
    ];
    const menu = resolveCodexModelMenuOptions(catalog, ['codex-auto-review'], 'codex-auto-review');
    expect(menu.primary.map(option => option.value)).toEqual(['gpt-6.1-sol']);
    expect(menu.additional).toEqual([]);
  });

  it('respects hidden metadata in stored custom options while preserving explicit custom models', () => {
    const catalog = [
      codexModel('gpt-6.1-sol'),
      { ...codexModel('gpt-daybreak-blue-latest'), hidden: true },
    ];
    const menu = resolveCodexModelMenuOptions(
      catalog,
      ['custom-provider-model', 'gpt-daybreak-blue-latest'],
      'gpt-daybreak-blue-latest'
    );
    expect(menu.primary.map(option => option.value)).toEqual([
      'gpt-6.1-sol',
      'custom-provider-model',
    ]);
  });

  it('keeps an available current model in the main menu without duplicating it under more', () => {
    const menu = resolveCodexModelMenuOptions(
      [codexModel('gpt-6.1-sol'), codexModel('gpt-5.4-pro')],
      [],
      'gpt-5.4-pro'
    );
    expect(menu.primary.map(option => option.value)).toEqual(['gpt-6.1-sol', 'gpt-5.4-pro']);
    expect(menu.additional).toEqual([]);
  });

  it.each([
    ['gpt-6.2-sol', '6.2S'],
    ['gpt-6.2-terra', '6.2T'],
    ['gpt-6.2-luna', '6.2L'],
    ['gpt-6.2-astra', '6.2A'],
    ['gpt-7', '7'],
    ['gpt-7-codex', '7Codex'],
    ['gpt-7-codex-spark', '7Spark'],
    ['gpt-7-mini', '7mini'],
    ['gpt-7-pro', '7Pro'],
  ])('applies existing abbreviation rules to newly discovered %s', (model, label) => {
    expect(resolveCodexModelOption(model, 'Runtime model name')).toEqual({
      label,
      value: model,
      menuLabel: 'Runtime model name',
    });
  });

  it('keeps new runtime models in the main menu and only available legacy models under more', () => {
    const menu = resolveCodexModelMenuOptions([
      codexModel('gpt-7-sol'),
      codexModel('gpt-5.4-pro'),
      codexModel('gpt-6.1-sol'),
    ]);
    expect(menu.primary.map(option => option.value)).toEqual(['gpt-7-sol', 'gpt-6.1-sol']);
    expect(menu.additional.map(option => option.value)).toEqual(['gpt-5.4-pro']);
    expect(resolveCodexModelMenuOptions([codexModel('gpt-7-sol')]).additional).toEqual([]);
  });

  it('uses local templates only when no usable runtime catalog is available', () => {
    expect(resolveCodexModelOptions()).toEqual(CODEX_MODEL_OPTIONS);
    expect(resolveCodexModelOptions([codexModel(' ')])).toEqual(CODEX_MODEL_OPTIONS);
    expect(resolveCodexModelMenuOptions()).toEqual({
      primary: CODEX_PRIMARY_MODEL_OPTIONS,
      additional: CODEX_ADDITIONAL_MODEL_OPTIONS,
    });
  });

  it('shows only the popular codex models by default', () => {
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.value)).toEqual([
      'gpt-5.5',
      'gpt-5.6-luna',
      'gpt-5.6-terra',
      'gpt-5.6-sol',
      'gpt-6-luna',
      'gpt-6-sol',
      'gpt-6-astra',
      'gpt-6.1-sol',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.label)).toEqual([
      '5.5',
      '5.6L',
      '5.6T',
      '5.6S',
      '6L',
      '6S',
      '6A',
      '6.1S',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.menuLabel)).toEqual([
      'GPT-5.5',
      'GPT-5.6 Luna',
      'GPT-5.6 Terra',
      'GPT-5.6 Sol',
      'GPT-6 Luna',
      'GPT-6 Sol',
      'GPT-6 Astra',
      'GPT-6.1 Sol',
    ]);
  });

  it('keeps less common codex models available without nano', () => {
    const additionalValues = CODEX_ADDITIONAL_MODEL_OPTIONS.map(option => option.value);
    const additionalLabels = CODEX_ADDITIONAL_MODEL_OPTIONS.map(option => option.label);
    const allValues = CODEX_MODEL_OPTIONS.map(option => option.value);

    expect(additionalValues).toEqual([
      'gpt-5.3-codex',
      'gpt-5.3-codex-spark',
      'gpt-5.4-mini',
      'gpt-5.4-pro',
      'gpt-5.5-pro',
    ]);
    expect(additionalLabels).toEqual(['5.3Codex', '5.3Spark', '5.4mini', '5.4Pro', '5.5Pro']);
    expect(CODEX_ADDITIONAL_MODEL_OPTIONS.map(option => option.menuLabel)).toEqual([
      'GPT-5.3 Codex',
      'GPT-5.3 Codex Spark',
      'GPT-5.4 mini',
      'GPT-5.4 Pro',
      'GPT-5.5 Pro',
    ]);
    expect(allValues).toEqual(
      expect.arrayContaining(['gpt-5.6-sol', 'gpt-5.6-luna', 'gpt-5.6-terra'])
    );
    expect(allValues).not.toContain('gpt-5.6');
    expect(allValues).not.toContain('gpt-5.4');
    expect(allValues).not.toContain('gpt-5.4-nano');
  });

  it('keeps claude models unchanged', () => {
    expect(CLAUDE_MODEL_OPTIONS.map(option => option.value)).toEqual([
      'fable',
      'opus',
      'sonnet',
      'haiku',
    ]);
  });

  it('uses compact claude runtime labels with full menu labels', () => {
    expect(CLAUDE_RUNTIME_OPTIONS.map(option => option.value)).toEqual(['claude', 'ccr']);
    expect(CLAUDE_RUNTIME_OPTIONS.map(option => option.label)).toEqual(['CC', 'CCR']);
    expect(CLAUDE_RUNTIME_OPTIONS.map(option => option.menuLabel)).toEqual([
      'Claude Code',
      'Claude Code Router',
    ]);
  });

  it('exports model picker sentinels', () => {
    expect(CUSTOM_MODEL_VALUE).toBe('__custom_model__');
    expect(MORE_MODELS_VALUE).toBe('__more_models__');
  });

  it('resolves per-agent configured models and reasoning efforts', () => {
    expect(defaultModelForAgent('codex')).toBe('gpt-5.6-sol');
    expect(defaultModelForAgent('codex', 'custom-codex-model')).toBe('custom-codex-model');
    expect(defaultModelForAgent('claude')).toBe('opus');
    expect(defaultModelForAgent('claude', 'sonnet')).toBe('sonnet');
    expect(defaultReasoningEffortForAgent('codex')).toBe('xhigh');
    expect(defaultReasoningEffortForAgent('codex', 'high')).toBe('high');
    expect(defaultReasoningEffortForAgent('codex', 'model_default')).toBe('default');
    expect(defaultReasoningEffortForAgent('claude')).toBe('default');
    expect(defaultReasoningEffortForAgent('claude', 'high')).toBe('high');
    expect(defaultPermissionLevelForAgent('codex')).toBe('elevated');
    expect(defaultPermissionLevelForAgent('codex', 'standard')).toBe('default');
    expect(defaultPermissionLevelForAgent('codex', 'yolo')).toBe('yolo');
    expect(defaultPermissionLevelForAgent('claude', 'standard')).toBe('elevated');
    expect(defaultModelForAgent('pi')).toBe('');
    expect(defaultModelForAgent('pi', 'openai/gpt-5.5')).toBe('openai/gpt-5.5');
    expect(defaultModelForAgent('devin')).toBe('swe-2-high');
    expect(defaultModelForAgent('devin', 'swe-2')).toBe('swe-2');
    expect(defaultReasoningEffortForAgent('pi')).toBe('default');
    expect(defaultReasoningEffortForAgent('pi', 'high')).toBe('high');
    expect(defaultReasoningEffortForAgent('devin')).toBe('high');
    expect(defaultReasoningEffortForAgent('devin', 'low')).toBe('low');
    expect(defaultReasoningEffortForAgent('devin', 'model_default')).toBe('default');
    expect(defaultPermissionLevelForAgent('pi', 'standard')).toBe('elevated');
    expect(defaultPermissionLevelForAgent('devin')).toBe('elevated');
    expect(defaultPermissionLevelForAgent('devin', 'standard')).toBe('default');
    expect(defaultPermissionLevelForAgent('devin', 'elevated')).toBe('elevated');
    expect(defaultPermissionLevelForAgent('devin', 'yolo')).toBe('yolo');
  });

  it('uses model-specific reasoning efforts from the Codex catalog', () => {
    const catalog = [
      {
        model: 'gpt-5.6-sol',
        supportedReasoningEfforts: ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'] as const,
      },
      {
        model: 'gpt-5.6-luna',
        supportedReasoningEfforts: ['low', 'medium', 'high', 'xhigh', 'max'] as const,
      },
    ].map(item => ({ ...item, supportedReasoningEfforts: [...item.supportedReasoningEfforts] }));

    expect(resolveCodexReasoningEfforts('gpt-5.6-sol', catalog)).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-5.6-luna', catalog)).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
    ]);
  });

  it('uses runtime reasoning capabilities even when they differ from local templates', () => {
    expect(
      resolveCodexReasoningEfforts('gpt-6.1-sol', [
        { model: 'gpt-6.1-sol', supportedReasoningEfforts: ['none', 'high', 'high'] },
      ])
    ).toEqual(['none', 'high']);
    expect(
      resolveCodexReasoningEfforts('gpt-6.1-sol', [
        { model: 'gpt-6.1-sol', supportedReasoningEfforts: [] },
      ])
    ).toEqual([]);
  });

  it('maps the Pi catalog to stable provider/model values and supported thinking levels', () => {
    const catalog = [
      {
        provider: 'anthropic',
        id: 'claude-sonnet-4',
        name: 'Claude Sonnet 4',
        reasoning: true,
        input: ['text', 'image'],
        contextWindow: 200000,
      },
      {
        provider: 'openai',
        id: 'gpt-4.1',
        name: 'GPT-4.1',
        reasoning: false,
        input: ['text'],
        contextWindow: 1000000,
      },
    ];

    expect(resolvePiModelOptions(catalog)).toEqual([
      {
        label: 'Claude Sonnet 4',
        value: 'anthropic/claude-sonnet-4',
        menuLabel: 'Claude Sonnet 4',
      },
      { label: 'GPT-4.1', value: 'openai/gpt-4.1', menuLabel: 'GPT-4.1' },
    ]);
    expect(resolvePiModelOptionGroups(catalog)).toEqual([
      {
        type: 'group',
        key: 'pi-provider-anthropic',
        label: 'anthropic',
        children: [
          {
            label: 'Claude Sonnet 4',
            value: 'anthropic/claude-sonnet-4',
            menuLabel: 'Claude Sonnet 4',
          },
        ],
      },
      {
        type: 'group',
        key: 'pi-provider-openai',
        label: 'openai',
        children: [{ label: 'GPT-4.1', value: 'openai/gpt-4.1', menuLabel: 'GPT-4.1' }],
      },
    ]);
    expect(resolvePiReasoningEfforts(catalog, 'anthropic/claude-sonnet-4')).toContain('max');
    expect(resolvePiReasoningEfforts(catalog, 'openai/gpt-4.1')).toEqual(['default', 'none']);
  });

  it('builds a concise Pi primary list and keeps selected catalog models there', () => {
    const model = (provider: string, id: string, name = id) => ({
      provider,
      id,
      name,
      reasoning: true,
      input: ['text'],
      contextWindow: 200000,
      maxTokens: 32000,
    });
    const catalog = [
      model('openrouter', 'gpt-5.4', 'Routed GPT-5.4'),
      model('openai', 'gpt-5.4', 'GPT-5.4'),
      model('openai', 'gpt-5.5', 'GPT-5.5'),
      model('openai', 'gpt-5.6-sol', 'GPT-5.6 Sol'),
      model('openai', 'gpt-5.6-terra', 'GPT-5.6 Terra'),
      model('openai', 'gpt-5.6-luna', 'GPT-5.6 Luna'),
      model('deepseek', 'deepseek-v4-flash', 'DeepSeek V4 Flash'),
      model('deepseek', 'deepseek-v4-pro', 'DeepSeek V4 Pro'),
      model('moonshot', 'kimi-k2.5', 'Kimi K2.5'),
    ];

    expect(resolvePiPrimaryModelOptions(catalog, ['moonshot/kimi-k2.5'])).toEqual([
      { label: 'GPT-5.4', value: 'openai/gpt-5.4', menuLabel: 'GPT-5.4' },
      { label: 'GPT-5.5', value: 'openai/gpt-5.5', menuLabel: 'GPT-5.5' },
      { label: 'GPT-5.6 Sol', value: 'openai/gpt-5.6-sol', menuLabel: 'GPT-5.6 Sol' },
      { label: 'GPT-5.6 Terra', value: 'openai/gpt-5.6-terra', menuLabel: 'GPT-5.6 Terra' },
      { label: 'GPT-5.6 Luna', value: 'openai/gpt-5.6-luna', menuLabel: 'GPT-5.6 Luna' },
      {
        label: 'DeepSeek V4 Flash',
        value: 'deepseek/deepseek-v4-flash',
        menuLabel: 'DeepSeek V4 Flash',
      },
      {
        label: 'DeepSeek V4 Pro',
        value: 'deepseek/deepseek-v4-pro',
        menuLabel: 'DeepSeek V4 Pro',
      },
      { label: 'Kimi K2.5', value: 'moonshot/kimi-k2.5', menuLabel: 'Kimi K2.5' },
    ]);

    expect(
      resolvePiPrimaryModelOptions([
        model('gateway', 'deepseek-next-vision-exp', 'DeepSeek Next Vision Exp'),
        model('gateway', 'deepseek-next', 'DeepSeek Next'),
      ]).map(option => option.value)
    ).toEqual(['gateway/deepseek-next']);

    const customModels = Array.from({ length: 7 }, (_, index) => model('custom', `model-${index}`));
    const frequentValues = customModels.map(item => `${item.provider}/${item.id}`);
    expect(
      resolvePiPrimaryModelOptions([...catalog, ...customModels], frequentValues)
        .map(option => option.value)
        .filter(value => value.startsWith('custom/'))
    ).toEqual(frequentValues.slice(0, 6));
  });

  it('limits Pi reasoning choices to the selected model capability map', () => {
    const efforts = resolvePiReasoningEfforts(
      [
        {
          provider: 'commandcode',
          id: 'deepseek/deepseek-v4.1-flash',
          name: 'DeepSeek V4.1 Flash',
          reasoning: true,
          reasoningEfforts: ['none', 'low', 'high', 'max'],
          input: ['text', 'image'],
          contextWindow: 1_000_000,
          maxTokens: 65_536,
        },
      ],
      'commandcode/deepseek/deepseek-v4.1-flash'
    );
    expect(efforts).toEqual(['default', 'none', 'low', 'high', 'max']);
    expect(efforts).not.toContain('medium');
    expect(efforts).not.toContain('xhigh');
  });

  it('keeps the Pi catalog open for IME composition without blocking explicit closes', () => {
    expect(
      shouldSuppressPiModelMenuClose('show-change', {
        catalogOpen: true,
        searchFocused: true,
        searchComposing: true,
      })
    ).toBe(true);
    expect(
      shouldSuppressPiModelMenuClose('pointer-leave', {
        catalogOpen: true,
        searchFocused: true,
        searchComposing: false,
      })
    ).toBe(true);
    expect(
      shouldSuppressPiModelMenuClose('show-change', {
        catalogOpen: true,
        searchFocused: true,
        searchComposing: false,
      })
    ).toBe(false);
    expect(
      shouldSuppressPiModelMenuClose('pointer-leave', {
        catalogOpen: false,
        searchFocused: true,
        searchComposing: true,
      })
    ).toBe(false);
  });

  it('filters the full Pi catalog by provider, model name, or model id', () => {
    const groups = resolvePiModelOptionGroups([
      {
        provider: 'anthropic',
        id: 'claude-sonnet-4',
        name: 'Claude Sonnet 4',
        reasoning: true,
        input: ['text'],
        contextWindow: 200000,
        maxTokens: 32000,
      },
      {
        provider: 'openai',
        id: 'gpt-4.1',
        name: 'GPT 4.1',
        reasoning: false,
        input: ['text'],
        contextWindow: 1000000,
        maxTokens: 32000,
      },
    ]);

    expect(filterPiModelOptionGroups(groups, 'OPENAI')).toEqual([groups[1]]);
    expect(filterPiModelOptionGroups(groups, 'sonnet')).toEqual([groups[0]]);
    expect(filterPiModelOptionGroups(groups, 'gpt-4.1')).toEqual([groups[1]]);
    expect(filterPiModelOptionGroups(groups, 'missing')).toEqual([]);
  });

  it('shows Pi recent models before provider categories without duplicate values', () => {
    const catalog = [
      { provider: 'openai', id: 'gpt-5.4', name: 'GPT-5.4', reasoning: true },
      { provider: 'openai', id: 'gpt-5.5', name: 'GPT-5.5', reasoning: true },
      { provider: 'anthropic', id: 'sonnet', name: 'Sonnet', reasoning: true },
    ];
    const groups = resolvePiModelOptionGroups(
      catalog,
      [' anthropic/sonnet ', 'openai/gpt-5.4', 'anthropic/sonnet', 'missing/model'],
      '最近使用'
    );

    expect(groups.map(group => group.label)).toEqual(['最近使用', 'openai']);
    expect(groups[0]?.children).toEqual([
      {
        label: 'Sonnet',
        value: 'anthropic/sonnet',
        menuLabel: 'Sonnet',
        accentLabel: 'anthropic',
        removable: true,
      },
      {
        label: 'GPT-5.4',
        value: 'openai/gpt-5.4',
        menuLabel: 'GPT-5.4',
        accentLabel: 'openai',
        removable: true,
      },
    ]);
    const values = groups.flatMap(group => group.children.map(option => option.value));
    expect(new Set(values).size).toBe(catalog.length);
    expect(filterPiModelOptionGroups(groups, 'anthropic')[0]?.children[0]?.value).toBe(
      'anthropic/sonnet'
    );
    expect(filterPiModelOptionGroups(groups, 'gpt-5.5')[0]?.children[0]?.value).toBe(
      'openai/gpt-5.5'
    );
    expect(resolvePiModelOptionGroups(catalog, ['missing/model'])).toEqual(
      resolvePiModelOptionGroups(catalog)
    );
  });

  it('bounds Pi recent entries and normalizes catalog values', () => {
    const catalog = Array.from({ length: 8 }, (_, index) => ({
      provider: ' provider ',
      id: ` model-${index} `,
      name: `Model ${index}`,
      reasoning: false,
    }));
    const groups = resolvePiModelOptionGroups(
      catalog,
      catalog.map((_, index) => `provider/model-${index}`)
    );
    expect(groups[0]?.children).toHaveLength(6);
    expect(groups[1]?.children.map(option => option.value)).toEqual([
      'provider/model-6',
      'provider/model-7',
    ]);
  });

  it('keeps Pi user-selected primary models unique and bounded', () => {
    expect(rememberPiFrequentModel(['provider/one', 'provider/two'], ' provider/two ')).toEqual([
      'provider/two',
      'provider/one',
    ]);
    expect(rememberPiFrequentModel(['provider/one', 'provider/two'], 'provider/three', 2)).toEqual([
      'provider/three',
      'provider/one',
    ]);
  });

  it('remembers custom models at the front without duplicates', () => {
    expect(rememberCustomModel(['glm-5.3', 'gpt-oss'], 'glm-5.3')).toEqual(['glm-5.3', 'gpt-oss']);
    expect(rememberCustomModel(['glm-5.3'], ' new-model ')).toEqual(['new-model', 'glm-5.3']);
    expect(rememberCustomModel([], '   ')).toEqual([]);
    expect(
      rememberCustomModel(
        Array.from({ length: 20 }, (_, index) => `model-${index}`),
        'model-new'
      )
    ).toEqual(['model-new', ...Array.from({ length: 19 }, (_, index) => `model-${index}`)]);
  });

  it('removes custom models while ignoring blank or missing entries', () => {
    expect(removeCustomModel(['glm-5.3', 'gpt-oss'], ' glm-5.3 ')).toEqual(['gpt-oss']);
    expect(removeCustomModel(['glm-5.3'], 'missing')).toEqual(['glm-5.3']);
    expect(removeCustomModel(['glm-5.3'], '')).toEqual(['glm-5.3']);
    expect(removeCustomModel(undefined as unknown as string[], 'glm-5.3')).toEqual([]);
  });

  it('maps stored custom models to options and skips reserved or duplicate values', () => {
    expect(resolveCustomModelOptions(['glm-5.3', ' opus ', 'glm-5.3', ''], ['opus'])).toEqual([
      { label: 'glm-5.3', value: 'glm-5.3' },
    ]);
    expect(resolveCustomModelOptions(undefined as unknown as string[])).toEqual([]);
  });

  it('falls back per current Codex model without exposing unsupported efforts', () => {
    expect(resolveCodexReasoningEfforts('gpt-6.1-sol')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-6-astra')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-6-sol')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-6-luna')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-6-luna')).not.toContain('ultra');
    expect(resolveCodexReasoningEfforts('gpt-5.6-terra')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-5.6-luna')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-5.6-luna')).not.toContain('none');
    expect(resolveCodexReasoningEfforts('gpt-5.6-luna')).not.toContain('ultra');
  });

  it('offers max and ultra for custom Codex models without catalog metadata', () => {
    expect(resolveCodexReasoningEfforts('custom-codex-model')).toEqual([
      'none',
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('')).toBeNull();
  });
});

describe('resolveCCRModelOptions', () => {
  it('prefers display names as labels and keeps gateway ids as values', () => {
    expect(
      resolveCCRModelOptions([
        { model: 'ZCode API/GLM-5.3', provider: 'ZCode API' },
        {
          model: 'DeepSeek/deepseek-flash',
          provider: 'DeepSeek',
          displayName: 'DeepSeek V4.1 Flash',
        },
        { model: '   ', provider: 'broken' },
      ])
    ).toEqual([
      {
        label: 'ZCode API/GLM-5.3',
        value: 'ZCode API/GLM-5.3',
        menuLabel: 'ZCode API/GLM-5.3',
      },
      {
        label: 'DeepSeek V4.1 Flash',
        value: 'DeepSeek/deepseek-flash',
        menuLabel: 'DeepSeek/deepseek-flash',
      },
    ]);
  });

  it('returns no options when nothing has been sniffed', () => {
    expect(resolveCCRModelOptions([])).toEqual([]);
  });
});

describe('parseDevinCostSummary', () => {
  it('splits the real fusion cost summary into labeled price items', () => {
    const summary =
      '$10 / 1M Input · $0.25 / 1M Cached input · $50 / 1M Output · $0.2 / 1M Sidekick input · $0.02 / 1M Sidekick cached input · $1.2 / 1M Sidekick output';
    expect(parseDevinCostSummary(summary)).toEqual([
      { price: '$10', unit: '1M', label: 'Input' },
      { price: '$0.25', unit: '1M', label: 'Cached input' },
      { price: '$50', unit: '1M', label: 'Output' },
      { price: '$0.2', unit: '1M', label: 'Sidekick input' },
      { price: '$0.02', unit: '1M', label: 'Sidekick cached input' },
      { price: '$1.2', unit: '1M', label: 'Sidekick output' },
    ]);
  });

  it('returns unmatched entries as label-only items with empty price and unit', () => {
    expect(parseDevinCostSummary('Flat rate · $1 / 1M Output')).toEqual([
      { price: '', unit: '', label: 'Flat rate' },
      { price: '$1', unit: '1M', label: 'Output' },
    ]);
  });

  it('returns an empty list for undefined or blank summaries', () => {
    expect(parseDevinCostSummary(undefined)).toEqual([]);
    expect(parseDevinCostSummary('   ')).toEqual([]);
    expect(parseDevinCostSummary('')).toEqual([]);
  });
});
