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
  resolveCodexReasoningEfforts,
  resolvePiModelOptionGroups,
  resolvePiModelOptions,
  resolvePiReasoningEfforts,
} from '@/components/web-session/webSessionModelOptions';

describe('webSessionModelOptions', () => {
  it('shows only the popular codex models by default', () => {
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.value)).toEqual([
      'gpt-5.4',
      'gpt-5.5',
      'gpt-5.6-sol',
      'gpt-5.6-luna',
      'gpt-5.6-terra',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.label)).toEqual([
      '5.4',
      '5.5',
      '5.6S',
      '5.6L',
      '5.6T',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.menuLabel)).toEqual([
      'GPT-5.4',
      'GPT-5.5',
      'GPT-5.6 Sol',
      'GPT-5.6 Luna',
      'GPT-5.6 Terra',
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

  it('falls back per 5.6 model without exposing none or Luna ultra', () => {
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
