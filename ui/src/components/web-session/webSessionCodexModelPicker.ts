import type { WebSessionCodexModelInfo, WebSessionReasoningEffort } from '@/types/models';
import { WEB_SESSION_REASONING_EFFORTS } from '@/constants/webSessionDefaults';
import {
  rememberPiFrequentModel,
  resolveCodexModelMenuOptions,
  resolveCodexReasoningEfforts,
  type WebSessionModelOption,
  type WebSessionModelOptionGroup,
} from './webSessionModelOptions';

const CODEX_MODEL_SELECTION_PREFIX = '__codex_selection__:';

export function codexModelSelectionValue(model: string, effort: WebSessionReasoningEffort) {
  return `${CODEX_MODEL_SELECTION_PREFIX}${JSON.stringify([model.trim(), effort])}`;
}

export function parseCodexModelSelection(value: string) {
  if (!value.startsWith(CODEX_MODEL_SELECTION_PREFIX)) return null;
  try {
    const selection: unknown = JSON.parse(value.slice(CODEX_MODEL_SELECTION_PREFIX.length));
    if (
      !Array.isArray(selection) ||
      selection.length !== 2 ||
      typeof selection[0] !== 'string' ||
      !selection[0].trim() ||
      !WEB_SESSION_REASONING_EFFORTS.includes(selection[1])
    ) {
      return null;
    }
    return {
      model: selection[0].trim(),
      reasoningEffort: selection[1] as WebSessionReasoningEffort,
    };
  } catch {
    return null;
  }
}

export function rememberCodexModelSelection(
  values: string[],
  model: string,
  effort: WebSessionReasoningEffort
) {
  if (!model.trim()) return values;
  return rememberPiFrequentModel(values, codexModelSelectionValue(model, effort));
}

export function resolveCodexModelOptionGroups({
  models = [],
  customModels = [],
  recentValues = [],
  currentModel = '',
  query = '',
  labels = {},
  effortLabel = effort => effort.charAt(0).toUpperCase() + effort.slice(1),
}: {
  models?: WebSessionCodexModelInfo[];
  customModels?: string[];
  recentValues?: string[];
  currentModel?: string;
  query?: string;
  labels?: { recent?: string; all?: string };
  effortLabel?: (effort: WebSessionReasoningEffort) => string;
} = {}): WebSessionModelOptionGroup[] {
  const { primary, additional } = resolveCodexModelMenuOptions(models, customModels, currentModel);
  const options = [...primary, ...additional].map(option => ({
    ...option,
    detail: {
      title: option.menuLabel ?? option.label,
      efforts: resolveCodexReasoningEfforts(option.value, models) ?? [],
    },
  }));
  const optionsByValue = new Map(options.map(option => [option.value, option]));
  const combinationOption = (
    option: WebSessionModelOption,
    effort: WebSessionReasoningEffort
  ): WebSessionModelOption => ({
    ...option,
    value: codexModelSelectionValue(option.value, effort),
    accentLabel: effortLabel(effort),
    searchText: `${option.value} ${option.menuLabel} ${option.label} ${effort} ${effortLabel(effort)}`,
  });
  const recent: WebSessionModelOption[] = [];
  for (const value of new Set(recentValues)) {
    const selection = parseCodexModelSelection(value);
    if (!selection) continue;
    const option = optionsByValue.get(selection.model);
    if (
      !option ||
      (selection.reasoningEffort !== 'default' &&
        !option.detail.efforts.includes(selection.reasoningEffort))
    ) {
      continue;
    }
    recent.push({ ...combinationOption(option, selection.reasoningEffort), removable: true });
  }

  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const matches = (option: WebSessionModelOption) => {
    const text = [option.value, option.menuLabel, option.label, option.searchText]
      .filter(Boolean)
      .join(' ')
      .toLowerCase();
    return terms.every(term => text.includes(term));
  };
  return [
    {
      type: 'group' as const,
      key: 'codex-recent',
      label: labels.recent ?? 'Recently Used',
      children: terms.length ? recent.filter(matches) : recent,
    },
    {
      type: 'group' as const,
      key: 'codex-all',
      label: labels.all ?? 'All Models',
      children: options.filter(matches),
    },
  ].filter(group => group.children.length);
}
