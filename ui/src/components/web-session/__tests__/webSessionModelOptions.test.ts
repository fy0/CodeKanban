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
} from '@/components/web-session/webSessionModelOptions';

describe('webSessionModelOptions', () => {
  it('shows only the popular codex models by default', () => {
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.value)).toEqual([
      'gpt-5.4',
      'gpt-5.5',
      'gpt-5.6',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.label)).toEqual(['5.4', '5.5', '5.6']);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.menuLabel)).toEqual([
      'GPT-5.4',
      'GPT-5.5',
      'GPT-5.6',
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
    expect(allValues).toContain('gpt-5.6');
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

  it('uses gpt-5.5 as the codex default model', () => {
    expect(defaultModelForAgent('codex')).toBe('gpt-5.5');
    expect(defaultModelForAgent('claude')).toBe('opus');
  });
});
