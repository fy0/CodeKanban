import type { WebSessionReasoningEffort } from '@/types/models';

export type WebSessionAgentOption = 'claude' | 'codex';
export type WebSessionClaudeRuntimeOption = 'claude' | 'ccr';
export type { WebSessionReasoningEffort } from '@/types/models';

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
  { label: '5.6S', value: 'gpt-5.6-sol', menuLabel: 'GPT-5.6 Sol' },
  { label: '5.6L', value: 'gpt-5.6-luna', menuLabel: 'GPT-5.6 Luna' },
  { label: '5.6T', value: 'gpt-5.6-terra', menuLabel: 'GPT-5.6 Terra' },
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

const CODEX_REASONING_EFFORT_FALLBACKS: Record<string, WebSessionReasoningEffort[]> = {
  'gpt-5.6-sol': ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'],
  'gpt-5.6-terra': ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'],
  'gpt-5.6-luna': ['low', 'medium', 'high', 'xhigh', 'max'],
};

export function resolveCodexReasoningEfforts(
  model: string,
  catalog: Array<{ model: string; supportedReasoningEfforts: WebSessionReasoningEffort[] }> = []
): WebSessionReasoningEffort[] | null {
  const normalizedModel = model.trim().toLowerCase();
  if (!normalizedModel) {
    return null;
  }
  const catalogModel = catalog.find(item => item.model.trim().toLowerCase() === normalizedModel);
  if (catalogModel) {
    return [
      ...new Set(catalogModel.supportedReasoningEfforts.filter(effort => effort !== 'default')),
    ];
  }
  const fallback = CODEX_REASONING_EFFORT_FALLBACKS[normalizedModel];
  return fallback ? [...fallback] : null;
}

export function defaultModelForAgent(agent: WebSessionAgentOption) {
  return agent === 'claude' ? 'opus' : 'gpt-5.5';
}
