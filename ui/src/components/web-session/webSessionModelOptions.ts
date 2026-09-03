import type {
  WebSessionAgent,
  WebSessionCodexDefaultPermissionLevel,
  WebSessionCodexDefaultReasoningEffort,
  WebSessionPiModelInfo,
  WebSessionReasoningEffort,
} from '@/types/models';
import {
  DEFAULT_WEB_SESSION_CODEX_MODEL,
  DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL,
  DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT,
  EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_MODEL,
  EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL,
  EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT,
} from '@/constants/webSessionDefaults';

export type WebSessionAgentOption = WebSessionAgent;
export type WebSessionClaudeRuntimeOption = 'claude' | 'ccr';
export type { WebSessionReasoningEffort } from '@/types/models';

export type WebSessionModelOption = {
  label: string;
  value: string;
  menuLabel?: string;
};

export type WebSessionModelOptionGroup = {
  type: 'group';
  key: string;
  label: string;
  children: WebSessionModelOption[];
};

export const CUSTOM_MODEL_VALUE = '__custom_model__';
export const MORE_MODELS_VALUE = '__more_models__';
export const PI_FREQUENT_MODEL_LIMIT = 6;

export type PiModelMenuCloseSource = 'show-change' | 'pointer-leave';

export function shouldSuppressPiModelMenuClose(
  source: PiModelMenuCloseSource,
  state: {
    catalogOpen: boolean;
    searchFocused: boolean;
    searchComposing: boolean;
  }
) {
  if (!state.catalogOpen) {
    return false;
  }
  if (source === 'pointer-leave') {
    return state.searchFocused || state.searchComposing;
  }
  return state.searchComposing;
}

const PI_PRIMARY_MODEL_ID_ORDER = [
  'gpt-5.4',
  'gpt-5.5',
  'gpt-5.6-sol',
  'gpt-5.6-terra',
  'gpt-5.6-luna',
  'deepseek-v4-flash',
  'deepseek-v4-pro',
  'deepseek-chat',
  'deepseek-reasoner',
  'deepseek-v3.2',
  'deepseek-r1',
];

function piModelValue(model: WebSessionPiModelInfo) {
  return `${model.provider.trim()}/${model.id.trim()}`;
}

function piModelOption(model: WebSessionPiModelInfo): WebSessionModelOption {
  return {
    label: model.name || model.id,
    value: piModelValue(model),
    menuLabel: model.name || model.id,
  };
}

function piProviderPreference(model: WebSessionPiModelInfo, modelId: string) {
  const provider = model.provider.trim().toLowerCase();
  const preferredProvider = modelId.startsWith('gpt-') ? 'openai' : 'deepseek';
  if (provider === preferredProvider) {
    return 0;
  }
  if (provider.startsWith(`${preferredProvider}-`) || provider.includes(preferredProvider)) {
    return 1;
  }
  return 2;
}

export function resolvePiModelOptions(models: WebSessionPiModelInfo[]): WebSessionModelOption[] {
  return models.filter(model => model.provider.trim() && model.id.trim()).map(piModelOption);
}

export function resolvePiPrimaryModelOptions(
  models: WebSessionPiModelInfo[],
  frequentModelValues: string[] = []
): WebSessionModelOption[] {
  const selectedValues: string[] = [];
  const addValue = (value: string) => {
    if (value && !selectedValues.includes(value)) {
      selectedValues.push(value);
    }
  };

  for (const modelId of PI_PRIMARY_MODEL_ID_ORDER) {
    const candidate = models
      .filter(model => model.id.trim().toLowerCase() === modelId)
      .sort(
        (left, right) => piProviderPreference(left, modelId) - piProviderPreference(right, modelId)
      )[0];
    if (candidate) {
      addValue(piModelValue(candidate));
    }
  }

  if (!selectedValues.some(value => value.toLowerCase().includes('deepseek'))) {
    const deepSeekModels = models.filter(model =>
      `${model.provider}/${model.id}/${model.name}`.toLowerCase().includes('deepseek')
    );
    const deepSeekCandidate =
      deepSeekModels.find(model => !/(vision|\bexp\b)/i.test(`${model.id} ${model.name}`)) ??
      deepSeekModels[0];
    if (deepSeekCandidate) {
      addValue(piModelValue(deepSeekCandidate));
    }
  }

  const optionsByValue = new Map(
    resolvePiModelOptions(models).map(option => [option.value, option] as const)
  );
  const boundedFrequentValues = Array.isArray(frequentModelValues)
    ? frequentModelValues.slice(0, PI_FREQUENT_MODEL_LIMIT)
    : [];
  for (const value of boundedFrequentValues) {
    const normalizedValue = String(value || '').trim();
    if (optionsByValue.has(normalizedValue)) {
      addValue(normalizedValue);
    }
  }
  return selectedValues.flatMap(value => {
    const option = optionsByValue.get(value);
    return option ? [option] : [];
  });
}

export function rememberPiFrequentModel(
  values: string[],
  modelValue: string,
  limit = PI_FREQUENT_MODEL_LIMIT
) {
  const normalizedValue = modelValue.trim();
  const normalizedLimit = Math.max(0, Math.floor(limit));
  if (!normalizedValue || normalizedLimit === 0) {
    return [];
  }
  const currentValues = Array.isArray(values) ? values : [];
  return [
    normalizedValue,
    ...currentValues
      .map(value => String(value || '').trim())
      .filter(value => value && value !== normalizedValue),
  ].slice(0, normalizedLimit);
}

export function resolvePiModelOptionGroups(
  models: WebSessionPiModelInfo[]
): WebSessionModelOptionGroup[] {
  const groups = new Map<string, WebSessionModelOption[]>();
  for (const model of models) {
    const provider = model.provider.trim();
    if (!provider || !model.id.trim()) {
      continue;
    }
    const options = groups.get(provider) ?? [];
    options.push({
      label: model.name || model.id,
      value: `${provider}/${model.id}`,
      menuLabel: model.name || model.id,
    });
    groups.set(provider, options);
  }
  return [...groups.entries()].map(([provider, children]) => ({
    type: 'group',
    key: `pi-provider-${provider}`,
    label: provider,
    children,
  }));
}

export function filterPiModelOptionGroups(
  groups: WebSessionModelOptionGroup[],
  query: string
): WebSessionModelOptionGroup[] {
  const normalizedQuery = query.trim().toLowerCase();
  if (!normalizedQuery) {
    return groups;
  }
  return groups.flatMap(group => {
    const children = group.children.filter(option =>
      [group.label, option.label, option.menuLabel, option.value]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(normalizedQuery)
    );
    return children.length > 0 ? [{ ...group, children }] : [];
  });
}

export function resolvePiReasoningEfforts(
  models: WebSessionPiModelInfo[],
  selectedModel: string
): WebSessionReasoningEffort[] {
  const selected = models.find(model => `${model.provider}/${model.id}` === selectedModel);
  return selected?.reasoning
    ? ['default', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max']
    : ['default', 'none'];
}

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

export function defaultModelForAgent(
  agent: WebSessionAgentOption,
  configuredCodexModel = DEFAULT_WEB_SESSION_CODEX_MODEL
) {
  if (agent === 'claude') {
    return 'opus';
  }
  if (agent === 'pi') {
    return '';
  }
  const configured = configuredCodexModel.trim();
  return !configured || configured.toLowerCase() === DEFAULT_WEB_SESSION_CODEX_MODEL
    ? EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_MODEL
    : configured;
}

export function defaultReasoningEffortForAgent(
  agent: WebSessionAgentOption,
  configuredCodexEffort: WebSessionCodexDefaultReasoningEffort = DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT
): WebSessionReasoningEffort {
  if (agent !== 'codex' || configuredCodexEffort === 'model_default') {
    return 'default';
  }
  return configuredCodexEffort === 'default'
    ? EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT
    : configuredCodexEffort;
}

export function defaultPermissionLevelForAgent(
  agent: WebSessionAgentOption,
  configuredCodexPermission: WebSessionCodexDefaultPermissionLevel = DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL
): 'default' | 'elevated' | 'yolo' {
  if (agent !== 'codex') {
    return 'elevated';
  }
  if (configuredCodexPermission === 'standard') {
    return 'default';
  }
  return configuredCodexPermission === 'default'
    ? EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL
    : configuredCodexPermission;
}
