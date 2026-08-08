import type { SessionConversationSearchMatch } from '@/api/webSession';
import type { WebSessionBlock } from '@/stores/webSession';

export type WebSessionConversationSearchFilters = {
  user: boolean;
  assistant: boolean;
  tools: boolean;
  system: boolean;
};

export type WebSessionConversationSearchMatch = {
  key?: string;
  id: string;
  sourceThreadId?: string;
  sourceTurnId?: string;
  sourceItemId?: string;
  orderIndex: number;
  kind: 'user' | 'assistant' | 'tool' | 'system' | string;
  toolId?: string;
  commandGroupId?: string;
};

export function normalizeWebSessionConversationSearchQuery(value: unknown) {
  return String(value ?? '')
    .trim()
    .toLowerCase();
}

export function isWebSessionConversationSearchKindEnabled(
  kind: WebSessionBlock['kind'],
  filters: WebSessionConversationSearchFilters
) {
  if (kind === 'user') {
    return filters.user;
  }
  if (kind === 'assistant') {
    return filters.assistant;
  }
  if (kind === 'tool') {
    return filters.tools;
  }
  return filters.system;
}

export function resolveWebSessionConversationSearchText(block: WebSessionBlock) {
  const values: string[] = [block.text];
  if (block.kind === 'tool' && block.tool) {
    values.push(block.itemType);
    values.push(block.tool.name, block.tool.kind ?? '', block.tool.output ?? '');
    values.push(stringifySearchValue(block.tool.input));
    values.push(stringifySearchValue(block.tool.meta));
    values.push(stringifySearchValue(block.payload?.groupItems));
  } else if (block.kind === 'system') {
    values.push(
      block.itemType,
      block.detail?.type ?? '',
      block.detail?.prompt ?? '',
      block.detail?.approvalKind ?? '',
      block.detail?.command ?? '',
      stringifySearchValue(block.detail?.questions),
      stringifySearchValue(block.detail?.answers),
      stringifySearchValue(block.payload)
    );
  }
  return values.filter(Boolean).join('\n').toLowerCase();
}

export function matchesWebSessionConversationSearch(
  block: WebSessionBlock,
  normalizedQuery: string,
  filters: WebSessionConversationSearchFilters
) {
  if (!normalizedQuery || !isWebSessionConversationSearchKindEnabled(block.kind, filters)) {
    return false;
  }
  return resolveWebSessionConversationSearchText(block).includes(normalizedQuery);
}

export function findWebSessionConversationSearchMatches(
  blocks: WebSessionBlock[],
  query: unknown,
  filters: WebSessionConversationSearchFilters
): WebSessionConversationSearchMatch[] {
  const normalizedQuery = normalizeWebSessionConversationSearchQuery(query);
  if (!normalizedQuery) {
    return [];
  }
  return blocks
    .filter(block => matchesWebSessionConversationSearch(block, normalizedQuery, filters))
    .map(block => ({
      key: block.key,
      id: block.id,
      sourceThreadId: block.sourceThreadId ?? undefined,
      sourceTurnId: block.sourceTurnId ?? undefined,
      sourceItemId: block.sourceItemId ?? undefined,
      orderIndex: block.orderIndex,
      kind: block.kind,
      toolId: block.tool?.id,
      commandGroupId: block.tool?.commandGroup?.id,
    }));
}

export function mergeWebSessionConversationSearchMatches(
  localMatches: WebSessionConversationSearchMatch[],
  remoteMatches: SessionConversationSearchMatch[]
): WebSessionConversationSearchMatch[] {
  const merged: WebSessionConversationSearchMatch[] = [];
  for (const match of [...localMatches, ...remoteMatches]) {
    const existingIndex = merged.findIndex(existing =>
      areWebSessionConversationSearchMatchesEquivalent(existing, match)
    );
    if (existingIndex < 0) {
      merged.push({ ...match });
      continue;
    }
    merged[existingIndex] = {
      ...merged[existingIndex],
      ...match,
      key: merged[existingIndex].key,
    };
  }
  return merged.sort((left, right) => {
    if (left.orderIndex !== right.orderIndex) {
      return left.orderIndex - right.orderIndex;
    }
    return left.id.localeCompare(right.id);
  });
}

export function areWebSessionConversationSearchMatchesEquivalent(
  left: WebSessionConversationSearchMatch,
  right: WebSessionConversationSearchMatch
) {
  if (left.sourceThreadId && right.sourceThreadId && left.sourceThreadId !== right.sourceThreadId) {
    return false;
  }
  if (left.id && right.id && left.id === right.id) {
    return true;
  }
  if (left.toolId && right.toolId && left.toolId === right.toolId) {
    return true;
  }
  return Boolean(
    left.commandGroupId && right.commandGroupId && left.commandGroupId === right.commandGroupId
  );
}

export function matchesWebSessionConversationSearchTarget(
  block: WebSessionBlock,
  match: WebSessionConversationSearchMatch
) {
  if (match.id && block.id === match.id) {
    return true;
  }
  if (match.toolId && block.tool?.id === match.toolId) {
    return true;
  }
  if (match.commandGroupId && block.tool?.commandGroup?.id === match.commandGroupId) {
    return true;
  }
  if (match.toolId && hasGroupedTool(block, match.toolId)) {
    return true;
  }
  return block.orderIndex === match.orderIndex && block.kind === match.kind;
}

function hasGroupedTool(block: WebSessionBlock, toolId: string) {
  const groupItems = block.payload?.groupItems;
  if (!Array.isArray(groupItems)) {
    return false;
  }
  return groupItems.some(item => {
    if (!item || typeof item !== 'object') {
      return false;
    }
    return String((item as { toolId?: unknown }).toolId ?? '').trim() === toolId;
  });
}

function stringifySearchValue(value: unknown) {
  if (value == null) {
    return '';
  }
  if (typeof value === 'string') {
    return value;
  }
  try {
    return JSON.stringify(value) ?? '';
  } catch {
    return String(value);
  }
}
