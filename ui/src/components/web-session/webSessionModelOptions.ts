export type WebSessionAgentOption = 'claude' | 'codex';
export type WebSessionClaudeRuntimeOption = 'claude' | 'ccr';

export type WebSessionModelOption = {
  label: string;
  value: string;
  menuLabel?: string;
};

export const CUSTOM_MODEL_VALUE = '__custom_model__';
export const MORE_MODELS_VALUE = '__more_models__';

export const CLAUDE_MODEL_OPTIONS: WebSessionModelOption[] = [
  { label: 'Opus', value: 'opus' },
  { label: 'Sonnet', value: 'sonnet' },
  { label: 'Haiku', value: 'haiku' },
];

export const CLAUDE_RUNTIME_OPTIONS: WebSessionModelOption[] = [
  { label: 'CC', value: 'claude', menuLabel: 'Claude Code' },
  { label: 'CCR', value: 'ccr', menuLabel: 'Claude Code Router' },
];

export const CODEX_PRIMARY_MODEL_OPTIONS: WebSessionModelOption[] = [
  { label: '5.4', value: 'gpt-5.4', menuLabel: 'GPT-5.4' },
  { label: '5.5', value: 'gpt-5.5', menuLabel: 'GPT-5.5' },
  { label: '5.6', value: 'gpt-5.6', menuLabel: 'GPT-5.6' },
];

export const CODEX_ADDITIONAL_MODEL_OPTIONS: WebSessionModelOption[] = [
  { label: '5.3Codex', value: 'gpt-5.3-codex', menuLabel: 'GPT-5.3 Codex' },
  { label: '5.3Spark', value: 'gpt-5.3-codex-spark', menuLabel: 'GPT-5.3 Codex Spark' },
  { label: '5.4mini', value: 'gpt-5.4-mini', menuLabel: 'GPT-5.4 mini' },
  { label: '5.4Pro', value: 'gpt-5.4-pro', menuLabel: 'GPT-5.4 Pro' },
  { label: '5.5Pro', value: 'gpt-5.5-pro', menuLabel: 'GPT-5.5 Pro' },
];

export const CODEX_MODEL_OPTIONS: WebSessionModelOption[] = [
  ...CODEX_PRIMARY_MODEL_OPTIONS,
  ...CODEX_ADDITIONAL_MODEL_OPTIONS,
];

export function defaultModelForAgent(agent: WebSessionAgentOption) {
  return agent === 'claude' ? 'opus' : 'gpt-5.5';
}
