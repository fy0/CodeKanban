import type {
  WebSessionAgent,
  WebSessionAgentDefaultPermissionLevel,
  WebSessionCCRModelInfo,
  WebSessionCodexDefaultReasoningEffort,
  WebSessionCodexModelInfo,
  WebSessionDevinModelInfo,
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
  GENERIC_CODEX_REASONING_EFFORTS,
} from '@/constants/webSessionDefaults';

export type WebSessionAgentOption = WebSessionAgent;
export type WebSessionClaudeRuntimeOption = 'claude' | 'ccr';
export type { WebSessionReasoningEffort } from '@/types/models';

export type WebSessionModelBadge = 'new' | 'beta' | 'promo';

export type WebSessionModelOptionDetail = {
  title?: string;
  description?: string;
  cost?: string;
  contextTokens?: number;
  outputTokens?: number;
  efforts?: WebSessionReasoningEffort[];
};

export type WebSessionModelOption = {
  label: string;
  value: string;
  menuLabel?: string;
  accentLabel?: string;
  description?: string;
  modelDescription?: string;
  recommended?: boolean;
  removable?: boolean;
  badges?: WebSessionModelBadge[];
  inputPrice?: string;
  detail?: WebSessionModelOptionDetail;
  searchText?: string;
};

export type WebSessionModelOptionGroup = {
  type: 'group';
  key: string;
  label: string;
  children: WebSessionModelOption[];
};

export type WebSessionModelCostItem = { label: string; price: string; unit: string };

export function parseDevinCostSummary(summary: string | undefined): WebSessionModelCostItem[] {
  const raw = String(summary ?? '').trim();
  if (!raw) {
    return [];
  }
  return raw.split('·').map(entry => {
    const trimmed = entry.trim();
    if (!trimmed) {
      return { label: '', price: '', unit: '' };
    }
    const match = trimmed.match(/^(\S+)\s*\/\s*(\S+)\s+(.+)$/);
    if (!match) {
      return { label: trimmed, price: '', unit: '' };
    }
    return { price: match[1], unit: match[2], label: match[3].trim() };
  });
}

export const CUSTOM_MODEL_VALUE = '__custom_model__';
export const MORE_MODELS_VALUE = '__more_models__';
export const CUSTOM_MODEL_STORAGE_LIMIT = 20;
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

export function rememberCustomModel(
  values: string[],
  modelValue: string,
  limit = CUSTOM_MODEL_STORAGE_LIMIT
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

export function removeCustomModel(values: string[], modelValue: string) {
  const normalizedValue = modelValue.trim();
  const currentValues = Array.isArray(values) ? values : [];
  if (!normalizedValue) {
    return [...currentValues];
  }
  return currentValues
    .map(value => String(value || '').trim())
    .filter(value => value && value !== normalizedValue);
}

export function resolveCustomModelOptions(
  values: string[],
  reservedValues: string[] = []
): WebSessionModelOption[] {
  const reserved = new Set(Array.isArray(reservedValues) ? reservedValues : []);
  const seen = new Set<string>();
  const options: WebSessionModelOption[] = [];
  for (const rawValue of Array.isArray(values) ? values : []) {
    const normalizedValue = String(rawValue || '').trim();
    if (!normalizedValue || reserved.has(normalizedValue) || seen.has(normalizedValue)) {
      continue;
    }
    seen.add(normalizedValue);
    options.push({ label: normalizedValue, value: normalizedValue });
  }
  return options;
}

export function resolvePiModelOptionGroups(
  models: WebSessionPiModelInfo[],
  recentModelValues: string[] = [],
  recentLabel = 'Recently Used'
): WebSessionModelOptionGroup[] {
  const optionsByValue = new Map(
    resolvePiModelOptions(models).map(option => [option.value, option] as const)
  );
  const recentValues = [...new Set(recentModelValues.map(value => String(value || '').trim()))]
    .filter(value => optionsByValue.has(value))
    .slice(0, PI_FREQUENT_MODEL_LIMIT);
  const recent = new Set(recentValues);
  const providers = new Map<string, WebSessionModelOption[]>();
  for (const model of models) {
    const provider = model.provider.trim();
    if (!provider || !model.id.trim() || recent.has(piModelValue(model))) {
      continue;
    }
    const options = providers.get(provider) ?? [];
    options.push(piModelOption(model));
    providers.set(provider, options);
  }
  const groups: WebSessionModelOptionGroup[] = recentValues.length
    ? [
        {
          type: 'group',
          key: 'pi-recent',
          label: recentLabel,
          children: recentValues.map(value => ({
            ...optionsByValue.get(value)!,
            accentLabel: value.slice(0, value.indexOf('/')),
            removable: true,
          })),
        },
      ]
    : [];
  return [
    ...groups,
    ...[...providers.entries()].map(([provider, children]) => ({
      type: 'group' as const,
      key: `pi-provider-${provider}`,
      label: provider,
      children,
    })),
  ];
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
  if (!selected?.reasoning) {
    return ['default', 'none'];
  }
  const supported = Array.isArray(selected.reasoningEfforts)
    ? selected.reasoningEfforts.filter(effort => effort !== 'default')
    : [];
  if (supported.length === 0) {
    return ['default', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'];
  }
  const normalized: WebSessionReasoningEffort[] = ['none', ...supported];
  return ['default', ...new Set(normalized)];
}

export const CLAUDE_MODEL_OPTIONS: WebSessionModelOption[] = [
  { label: 'Fable', value: 'fable' },
  { label: 'Opus', value: 'opus' },
  { label: 'Sonnet', value: 'sonnet' },
  { label: 'Haiku', value: 'haiku' },
];

export const DEVIN_MODEL_OPTIONS: WebSessionModelOption[] = [
  { label: 'SWE-2 High', value: 'swe-2-high', menuLabel: 'SWE-2 High' },
];

export function resolveDevinModelOptions(models: WebSessionDevinModelInfo[] = []) {
  return models
    .filter(model => Boolean(model.model?.trim()))
    .map(model => {
      const name = model.displayName?.trim() || model.model;
      const cost = model.costSummary?.trim() || model.costTier?.trim() || undefined;
      const modelDescription = model.description?.trim() || undefined;
      const inputCost = parseDevinCostSummary(model.costSummary).find(
        item => item.price && /input/i.test(item.label)
      );
      const badges: WebSessionModelBadge[] = [];
      if (model.isNew === true) badges.push('new');
      if (model.isPromo === true || model.costTier?.trim().toLowerCase() === 'promotion') {
        badges.push('promo');
      }
      if (model.isBeta === true) badges.push('beta');
      return {
        label: name,
        value: model.model,
        menuLabel: name,
        description: cost,
        modelDescription,
        recommended: model.recommended === true,
        badges,
        inputPrice: inputCost ? `${inputCost.price}/${inputCost.unit}` : undefined,
        detail: {
          title: name,
          description: modelDescription,
          cost,
          contextTokens: model.maxContextTokens || undefined,
          outputTokens: model.maxOutputTokens || undefined,
        },
      };
    });
}

function devinRepresentativeModel(models: WebSessionDevinModelInfo[]) {
  return [...models].sort((left, right) => {
    const rank = (model: WebSessionDevinModelInfo) => {
      const value = model.model.toLowerCase();
      if (value === 'swe-2-high') return 0;
      if (value.endsWith('-medium')) return 1;
      if (value.endsWith('-high')) return 2;
      if (value.endsWith('-max')) return 3;
      return 4;
    };
    return rank(left) - rank(right);
  })[0];
}

export function resolveDevinSpecialModelOptions(
  models: WebSessionDevinModelInfo[] = [],
  currentModel = ''
): WebSessionModelOption[] {
  const normalizedCurrent = currentModel.trim();
  return ['Adaptive', 'Fusion'].flatMap(family => {
    const entries = models.filter(model => model.family?.trim() === family);
    const representative = entries.find(model => model.model === normalizedCurrent) ?? entries[0];
    if (!representative) return [];
    const option = resolveDevinModelOptions([representative])[0];
    return option ? [option] : [];
  });
}

export type DevinFusionModelConfig = {
  model: string;
  lead: string;
  effort: string;
  sidekick: string;
  sidekickEffort: string;
  fast: boolean;
};

export type DevinFusionSelection = {
  lead: string;
  effort: string;
  sidekick: string;
  sidekickEffort?: string;
  fast: boolean;
};

const DEVIN_FUSION_EFFORT_NAMES = ['Low', 'Medium', 'High', 'XHigh', 'Max'];

function normalizeDevinFusionEffort(raw: string | undefined) {
  const value = String(raw || '')
    .trim()
    .toLowerCase();
  return DEVIN_FUSION_EFFORT_NAMES.find(name => name.toLowerCase() === value) ?? '';
}

function parseDevinFusionMember(raw: string) {
  const match = raw.trim().match(/^(.*?)(?: (Low|Medium|High|XHigh|Max)(?: Thinking)?)?( Fast)?$/i);
  if (!match) {
    return { name: raw.trim(), effort: '', fast: false };
  }
  return {
    name: match[1].trim(),
    effort: normalizeDevinFusionEffort(match[2]),
    fast: Boolean(match[3]),
  };
}

export function resolveDevinFusionModelConfigs(
  models: WebSessionDevinModelInfo[] = []
): DevinFusionModelConfig[] {
  return models
    .filter(model => model.family?.trim() === 'Fusion')
    .flatMap(model => {
      const label = model.displayName?.trim() || '';
      const parts = label.match(/^Fusion \((.+) \+ (.+)\)$/);
      if (!parts) return [];
      const lead = parseDevinFusionMember(parts[1]);
      const sidekick = parseDevinFusionMember(parts[2]);
      return [
        {
          model: model.model,
          lead: lead.name,
          effort: lead.effort || 'Medium',
          sidekick: sidekick.name,
          sidekickEffort: sidekick.effort,
          fast: lead.fast || sidekick.fast,
        },
      ];
    });
}

export function resolveDevinFusionModel(
  configs: DevinFusionModelConfig[],
  selection: DevinFusionSelection
) {
  const { lead, effort, sidekick, fast } = selection;
  const sidekickEffort = selection.sidekickEffort ?? '';
  return (
    configs.find(
      config =>
        config.lead === lead &&
        config.effort === effort &&
        config.sidekick === sidekick &&
        config.sidekickEffort === sidekickEffort &&
        config.fast === fast
    )?.model ??
    configs.find(
      config =>
        config.lead === lead &&
        config.effort === effort &&
        config.sidekick === sidekick &&
        config.fast === fast
    )?.model ??
    configs.find(
      config => config.lead === lead && config.effort === effort && config.sidekick === sidekick
    )?.model ??
    configs.find(config => config.lead === lead && config.effort === effort)?.model ??
    configs.find(config => config.lead === lead)?.model ??
    configs[0]?.model ??
    ''
  );
}

export type DevinModelGroupLabels = {
  recent?: string;
  recommended?: string;
  all?: string;
};

export const DEVIN_DUPLICATE_MODEL_VALUE_SUFFIX = '#dup';

export function duplicateDevinModelOptionValue(value: string) {
  return `${value}${DEVIN_DUPLICATE_MODEL_VALUE_SUFFIX}`;
}

export function normalizeDevinModelOptionValue(value: string) {
  return value.endsWith(DEVIN_DUPLICATE_MODEL_VALUE_SUFFIX)
    ? value.slice(0, -DEVIN_DUPLICATE_MODEL_VALUE_SUFFIX.length)
    : value;
}

const DEVIN_EFFORT_DISPLAY_ORDER: WebSessionReasoningEffort[] = [
  'none',
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max',
  'ultra',
];

function devinFamilyEfforts(entries: WebSessionDevinModelInfo[]) {
  const efforts = [
    ...new Set(
      entries
        .map(model => model.defaultReasoningEffort)
        .filter((effort): effort is WebSessionReasoningEffort =>
          Boolean(effort && effort !== 'default')
        )
    ),
  ];
  return efforts.sort(
    (left, right) =>
      DEVIN_EFFORT_DISPLAY_ORDER.indexOf(left) - DEVIN_EFFORT_DISPLAY_ORDER.indexOf(right)
  );
}

export function resolveDevinModelOptionGroups(
  models: WebSessionDevinModelInfo[] = [],
  recentModelValues: string[] = [],
  currentModel = '',
  labels: DevinModelGroupLabels = {}
): WebSessionModelOptionGroup[] {
  const families = new Map<string, WebSessionDevinModelInfo[]>();
  for (const model of models) {
    const family = model.family?.trim() || 'Other models';
    if (family === 'Adaptive' || family === 'Fusion') continue;
    const entries = families.get(family) ?? [];
    entries.push(model);
    families.set(family, entries);
  }
  const normalizedCurrent = currentModel.trim();
  const familyEntries = [...families.entries()].flatMap(([family, entries]) => {
    const chosen =
      entries.find(model => model.model === normalizedCurrent) ?? devinRepresentativeModel(entries);
    if (!chosen) return [];
    const base = resolveDevinModelOptions([chosen])[0];
    if (!base) return [];
    return [
      {
        family,
        option: {
          ...base,
          label: family,
          menuLabel: family,
          recommended: entries.some(model => model.recommended === true),
          detail: {
            ...base.detail,
            title: family,
            efforts: devinFamilyEfforts(entries),
          },
          searchText: entries
            .flatMap(model => [model.model, model.displayName])
            .filter(Boolean)
            .join(' '),
        } as WebSessionModelOption,
      },
    ];
  });
  const optionsByValue = new Map(
    resolveDevinModelOptions(models).map(option => [option.value, option] as const)
  );
  const seenRecent = new Set<string>();
  const recent: WebSessionModelOption[] = [];
  for (const rawValue of recentModelValues) {
    const value = String(rawValue || '').trim();
    if (!value || seenRecent.has(value)) continue;
    seenRecent.add(value);
    const option = optionsByValue.get(value);
    if (!option) continue;
    const model = models.find(entry => entry.model === value);
    const family = model?.family?.trim();
    const menuLabel = String(option.menuLabel ?? option.label ?? '');
    let recentMenuLabel = option.menuLabel;
    let accentLabel: string | undefined;
    if (family && family !== 'Adaptive' && family !== 'Fusion' && menuLabel.startsWith(family)) {
      const remainder = menuLabel.slice(family.length).trim();
      const effort = model?.defaultReasoningEffort;
      const effortLabel =
        effort && effort !== 'default' ? effort.charAt(0).toUpperCase() + effort.slice(1) : '';
      recentMenuLabel = family;
      accentLabel = remainder || effortLabel || undefined;
    }
    recent.push({
      ...option,
      label: recentMenuLabel ?? option.label,
      menuLabel: recentMenuLabel,
      accentLabel,
      removable: true,
    });
  }
  const recentFamilies = new Set(
    recent
      .map(option => models.find(model => model.model === option.value)?.family?.trim() ?? '')
      .filter(Boolean)
  );
  const recommendedEntries = familyEntries.filter(
    entry => entry.option.recommended === true && !recentFamilies.has(entry.family)
  );
  const recommended = recommendedEntries.map(entry => entry.option);
  const recommendedFamilies = new Set(recommendedEntries.map(entry => entry.family));
  const recentOptionValues = new Set(recent.map(option => option.value));
  const allModels = familyEntries
    .filter(entry => !recommendedFamilies.has(entry.family))
    .map(entry =>
      recentOptionValues.has(entry.option.value)
        ? { ...entry.option, value: duplicateDevinModelOptionValue(entry.option.value) }
        : entry.option
    );
  const groups: WebSessionModelOptionGroup[] = [];
  if (recent.length) {
    groups.push({
      type: 'group',
      key: 'devin-recent',
      label: labels.recent ?? 'Recently Used',
      children: recent,
    });
  }
  if (recommended.length) {
    groups.push({
      type: 'group',
      key: 'devin-recommended',
      label: labels.recommended ?? 'Recommended',
      children: recommended,
    });
  }
  if (allModels.length) {
    groups.push({
      type: 'group',
      key: 'devin-all',
      label: labels.all ?? 'All Models',
      children: allModels,
    });
  }
  return groups;
}

export function filterDevinModelOptionGroups(
  groups: WebSessionModelOptionGroup[],
  query: string
): WebSessionModelOptionGroup[] {
  const normalizedQuery = query.trim().toLowerCase();
  if (!normalizedQuery) return groups;
  return groups.flatMap(group => {
    const children = group.children.filter(option =>
      [
        group.label,
        option.label,
        option.menuLabel,
        option.value,
        option.description,
        option.searchText,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(normalizedQuery)
    );
    return children.length ? [{ ...group, children }] : [];
  });
}

export function resolveDevinReasoningEfforts(
  models: WebSessionDevinModelInfo[],
  selectedModel: string
): WebSessionReasoningEffort[] {
  const selected = models.find(model => model.model.trim() === selectedModel.trim());
  if (selected?.family) {
    const familyEfforts = models
      .filter(model => model.family === selected.family)
      .map(model => model.defaultReasoningEffort)
      .filter((effort): effort is WebSessionReasoningEffort =>
        Boolean(effort && effort !== 'default')
      );
    if (familyEfforts.length) {
      return [...new Set(familyEfforts)];
    }
  }
  if (selected?.supportedReasoningEfforts?.length) {
    return [...new Set(selected.supportedReasoningEfforts)];
  }
  const suffix = selectedModel.trim().toLowerCase().split('-').pop();
  return ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'].includes(suffix || '')
    ? [suffix as WebSessionReasoningEffort]
    : ['default'];
}

export function resolveDevinModelForReasoning(
  models: WebSessionDevinModelInfo[],
  currentModel: string,
  effort: WebSessionReasoningEffort
) {
  const selected = models.find(model => model.model.trim() === currentModel.trim());
  if (!selected?.family) {
    return currentModel;
  }
  return (
    models.find(
      model =>
        model.family === selected.family && (model.defaultReasoningEffort ?? 'default') === effort
    )?.model ?? currentModel
  );
}

export function resolveDevinSelectedReasoningEffort(
  models: WebSessionDevinModelInfo[],
  selectedModel: string,
  currentEffort: WebSessionReasoningEffort
) {
  if (currentEffort !== 'default') return currentEffort;
  return (
    models.find(model => model.model.trim() === selectedModel.trim())?.defaultReasoningEffort ??
    currentEffort
  );
}

// CCR routes claude through the Claude Code Router gateway, so the selectable
// models are the gateway's provider/model ids instead of Anthropic aliases.
export function resolveCCRModelOptions(
  models: WebSessionCCRModelInfo[] = []
): WebSessionModelOption[] {
  return models
    .filter(model => Boolean(model.model?.trim()))
    .map(model => ({
      label: model.displayName?.trim() || model.model,
      value: model.model,
      menuLabel: model.model,
    }));
}

export const CLAUDE_RUNTIME_OPTIONS: WebSessionModelOption[] = [
  { label: 'CC', value: 'claude', menuLabel: 'Claude Code' },
  { label: 'CCR', value: 'ccr', menuLabel: 'Claude Code Router' },
];

export function resolveDefaultClaudeRuntime(
  configured: string | null | undefined
): WebSessionClaudeRuntimeOption {
  return String(configured ?? '')
    .trim()
    .toLowerCase() === 'ccr'
    ? 'ccr'
    : 'claude';
}

export const CODEX_PRIMARY_MODEL_OPTIONS: WebSessionModelOption[] = [
  { label: '5.5', value: 'gpt-5.5', menuLabel: 'GPT-5.5' },
  { label: '5.6L', value: 'gpt-5.6-luna', menuLabel: 'GPT-5.6 Luna' },
  { label: '5.6T', value: 'gpt-5.6-terra', menuLabel: 'GPT-5.6 Terra' },
  { label: '5.6S', value: 'gpt-5.6-sol', menuLabel: 'GPT-5.6 Sol' },
  { label: '6L', value: 'gpt-6-luna', menuLabel: 'GPT-6 Luna' },
  { label: '6S', value: 'gpt-6-sol', menuLabel: 'GPT-6 Sol' },
  { label: '6A', value: 'gpt-6-astra', menuLabel: 'GPT-6 Astra' },
  { label: '6.1S', value: 'gpt-6.1-sol', menuLabel: 'GPT-6.1 Sol' },
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

const CODEX_MODEL_SUFFIX_LABELS: Record<string, string> = {
  astra: 'A',
  sol: 'S',
  terra: 'T',
  luna: 'L',
  codex: 'Codex',
  'codex-spark': 'Spark',
  mini: 'mini',
  nano: 'nano',
  pro: 'Pro',
};

// Local templates supply presentation and an offline fallback. A populated
// runtime catalog is authoritative about which models are available.
export function resolveCodexModelOption(model: string, displayName = ''): WebSessionModelOption {
  const value = model.trim();
  const normalizedModel = value.toLowerCase();
  const template = CODEX_MODEL_OPTIONS.find(option => option.value === normalizedModel);
  const match = /^gpt-(\d+(?:\.\d+)*)(?:-(.+))?$/.exec(normalizedModel);
  const suffix = match?.[2] ?? '';
  const shortLabel =
    match && (!suffix || Object.prototype.hasOwnProperty.call(CODEX_MODEL_SUFFIX_LABELS, suffix))
      ? `${match[1]}${CODEX_MODEL_SUFFIX_LABELS[suffix] ?? ''}`
      : '';
  const name = displayName.trim();
  return {
    label: template?.label || shortLabel || name || value,
    value,
    menuLabel:
      name && name.toLowerCase() !== normalizedModel ? name : template?.menuLabel || name || value,
  };
}

export function resolveCodexModelOptions(
  models: WebSessionCodexModelInfo[] = []
): WebSessionModelOption[] {
  const hasCatalog = models.some(model => model.model.trim());
  const seen = new Set<string>();
  const options: WebSessionModelOption[] = [];
  for (const model of models) {
    const value = model.model.trim();
    const key = value.toLowerCase();
    if (!value || model.hidden || key === 'codex-auto-review' || seen.has(key)) {
      continue;
    }
    seen.add(key);
    options.push(resolveCodexModelOption(value, model.displayName));
  }
  return hasCatalog ? options : CODEX_MODEL_OPTIONS.map(option => ({ ...option }));
}

export function resolveCodexModelMenuOptions(
  models: WebSessionCodexModelInfo[] = [],
  customModels: string[] = [],
  currentModel = ''
) {
  const additionalValues = new Set(CODEX_ADDITIONAL_MODEL_OPTIONS.map(option => option.value));
  const catalogOptions = resolveCodexModelOptions(models);
  const primary: WebSessionModelOption[] = [];
  let additional: WebSessionModelOption[] = [];
  for (const option of catalogOptions) {
    (additionalValues.has(option.value.toLowerCase()) ? additional : primary).push(option);
  }
  const excludedValues = new Set([
    'codex-auto-review',
    ...models.filter(model => model.hidden).map(model => model.model.trim().toLowerCase()),
  ]);
  const knownValues = new Set(catalogOptions.map(option => option.value.toLowerCase()));
  primary.push(
    ...resolveCustomModelOptions(customModels).filter(
      option =>
        !excludedValues.has(option.value.toLowerCase()) &&
        !knownValues.has(option.value.toLowerCase())
    )
  );
  const currentValue = currentModel.trim();
  const currentKey = currentValue.toLowerCase();
  if (
    currentValue &&
    !excludedValues.has(currentKey) &&
    !primary.some(option => option.value.toLowerCase() === currentKey)
  ) {
    const currentOption = catalogOptions.find(option => option.value.toLowerCase() === currentKey);
    // An old selection must not restore a model removed from the live picker.
    // Explicit custom models remain available; keep the selected label via the UI fallback.
    if (currentOption || !models.some(model => model.model.trim())) {
      primary.push(currentOption ?? resolveCodexModelOption(currentValue));
      additional = additional.filter(option => option.value.toLowerCase() !== currentKey);
    }
  }
  return { primary, additional };
}

const CODEX_REASONING_EFFORT_FALLBACKS: Record<string, WebSessionReasoningEffort[]> = {
  'gpt-6.1-sol': ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'],
  'gpt-6-astra': ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'],
  'gpt-6-sol': ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'],
  'gpt-6-luna': ['low', 'medium', 'high', 'xhigh', 'max'],
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
  return fallback
    ? [...fallback]
    : GENERIC_CODEX_REASONING_EFFORTS.filter(effort => effort !== 'default');
}

export const BUILTIN_DEFAULT_MODELS: Record<WebSessionAgentOption, string> = {
  claude: 'opus',
  codex: EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_MODEL,
  pi: '',
  devin: 'swe-2-high',
};

export const BUILTIN_DEFAULT_REASONING_EFFORTS: Record<
  WebSessionAgentOption,
  WebSessionReasoningEffort
> = {
  claude: 'default',
  codex: EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT,
  pi: 'default',
  devin: 'high',
};

export function defaultModelForAgent(
  agent: WebSessionAgentOption,
  configuredModel = DEFAULT_WEB_SESSION_CODEX_MODEL
) {
  const configured = configuredModel.trim();
  return !configured || configured.toLowerCase() === DEFAULT_WEB_SESSION_CODEX_MODEL
    ? BUILTIN_DEFAULT_MODELS[agent]
    : configured;
}

export function defaultReasoningEffortForAgent(
  agent: WebSessionAgentOption,
  configuredEffort: WebSessionCodexDefaultReasoningEffort = DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT
): WebSessionReasoningEffort {
  if (configuredEffort === 'model_default') {
    return 'default';
  }
  return configuredEffort === 'default'
    ? BUILTIN_DEFAULT_REASONING_EFFORTS[agent]
    : configuredEffort;
}

export function defaultPermissionLevelForAgent(
  agent: WebSessionAgentOption,
  configuredPermission: WebSessionAgentDefaultPermissionLevel = DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL
): 'default' | 'elevated' | 'yolo' {
  if (agent !== 'codex' && agent !== 'devin') {
    return 'elevated';
  }
  if (configuredPermission === 'standard') {
    return 'default';
  }
  return configuredPermission === 'default'
    ? EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL
    : configuredPermission;
}
