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
  filterPiModelOptionGroups,
  rememberPiFrequentModel,
  resolveCodexReasoningEfforts,
  resolvePiModelOptionGroups,
  resolvePiModelOptions,
  resolvePiPrimaryModelOptions,
  resolvePiReasoningEfforts,
  shouldSuppressPiModelMenuClose,
} from '@/components/web-session/webSessionModelOptions';

describe('webSessionModelOptions', () => {
  it('shows only the popular codex models by default', () => {
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.value)).toEqual([
      'gpt-5.5',
      'gpt-5.6-luna',
      'gpt-5.6-terra',
      'gpt-5.6-sol',
      'gpt-6-astra',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.label)).toEqual([
      '5.5',
      '5.6L',
      '5.6T',
      '5.6S',
      '6A',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.menuLabel)).toEqual([
      'GPT-5.5',
      'GPT-5.6 Luna',
      'GPT-5.6 Terra',
      'GPT-5.6 Sol',
      'GPT-6 Astra',
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
    expect(CLAUDE_MODEL_OPTIONS.map(option => option.value)).toEqual(['opus', 'sonnet', 'haiku']);
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

  it('uses configurable Codex defaults without changing Claude or Pi defaults', () => {
    expect(defaultModelForAgent('codex')).toBe('gpt-5.6-sol');
    expect(defaultModelForAgent('codex', 'custom-codex-model')).toBe('custom-codex-model');
    expect(defaultModelForAgent('claude')).toBe('opus');
    expect(defaultReasoningEffortForAgent('codex')).toBe('xhigh');
    expect(defaultReasoningEffortForAgent('codex', 'high')).toBe('high');
    expect(defaultReasoningEffortForAgent('codex', 'model_default')).toBe('default');
    expect(defaultReasoningEffortForAgent('claude', 'high')).toBe('default');
    expect(defaultPermissionLevelForAgent('codex')).toBe('elevated');
    expect(defaultPermissionLevelForAgent('codex', 'standard')).toBe('default');
    expect(defaultPermissionLevelForAgent('codex', 'yolo')).toBe('yolo');
    expect(defaultPermissionLevelForAgent('claude', 'standard')).toBe('elevated');
    expect(defaultModelForAgent('pi')).toBe('');
    expect(defaultReasoningEffortForAgent('pi', 'high')).toBe('default');
    expect(defaultPermissionLevelForAgent('pi', 'standard')).toBe('elevated');
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

  it('falls back per current Codex model without exposing unsupported efforts', () => {
    expect(resolveCodexReasoningEfforts('gpt-6-astra')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
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
});
