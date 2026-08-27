import type { WebSessionBlock } from '@/stores/webSession';
import { normalizeWebSessionActivityToolKind } from '@/constants/webSessionActivityDisplayMode';

interface CompactTimelineGroupItem {
  toolId: string;
  kind: string;
  title: string;
  summary: string;
  command: string;
  input?: unknown;
  output?: string;
  status: 'running' | 'done' | 'error';
  timestamp?: string;
  startedAt?: string;
  completedAt?: string;
}

const FILE_CHANGE_KIND = 'file_change';
const COMPACT_TOOL_KINDS = new Set([
  'command_execution',
  FILE_CHANGE_KIND,
  'mcp_tool_call',
  'web_search',
  'dynamic_tool_call',
]);
const SYNTHETIC_FILE_CHANGE_GROUP_PREFIX = 'timeline-file-change:';
const SYNTHETIC_COMPACT_TOOL_GROUP_PREFIX = 'timeline-compact-tool:';
const PROJECTED_FILE_CHANGE_KEY_PREFIX = 'compact-file-change:';
const PROJECTED_COMPACT_TOOL_KEY_PREFIX = 'compact-tool:';

export function isTransportRetryNoteBlock(block: WebSessionBlock): boolean {
  return (
    block.itemType === 'note' && String(block.payload?.code ?? '').trim() === 'transport_retrying'
  );
}

export function projectWebSessionCompactTimelineBlocks(
  blocks: WebSessionBlock[]
): WebSessionBlock[] {
  if (blocks.length === 0) {
    return blocks;
  }

  const projected: WebSessionBlock[] = [];
  for (let index = 0; index < blocks.length; ) {
    const block = blocks[index];
    if (!isCompactToolBlock(block)) {
      projected.push(block);
      index += 1;
      continue;
    }

    const groupKey = getCompactToolGroupKey(block);
    const group = [block];
    let nextIndex = index + 1;

    while (nextIndex < blocks.length) {
      const candidate = blocks[nextIndex];
      if (
        !isCompactToolBlock(candidate) ||
        getCompactToolGroupKey(candidate) !== groupKey ||
        getSourceThreadId(candidate) !== getSourceThreadId(block)
      ) {
        break;
      }

      group.push(candidate);
      nextIndex += 1;
    }

    if (group.length === 1) {
      projected.push(block);
      index = nextIndex;
      continue;
    }

    const groupId = findGroupId(group) || buildSyntheticGroupId(group[0], projected.length);
    projected.push(buildGroupedCompactToolBlock(group, groupId));
    index = nextIndex;
  }

  return projected;
}

export function projectWebSessionVisibleTimelineBlocks(
  blocks: WebSessionBlock[]
): WebSessionBlock[] {
  return projectWebSessionCompactTimelineBlocks(blocks).filter(
    block => !isTransportRetryNoteBlock(block)
  );
}

function getSourceThreadId(block: WebSessionBlock): string {
  return String(block.sourceThreadId ?? '').trim();
}

function isCompactToolBlock(block: WebSessionBlock): boolean {
  return (
    block.kind === 'tool' &&
    COMPACT_TOOL_KINDS.has(getCompactToolKind(block)) &&
    !isInteractiveDynamicToolBlock(block)
  );
}

function getCompactToolKind(block: WebSessionBlock): string {
  return normalizeWebSessionActivityToolKind(String(block.tool?.kind || '').trim());
}

function getCompactToolGroupKey(block: WebSessionBlock): string {
  const kind = getCompactToolKind(block);
  if (kind !== 'dynamic_tool_call') {
    return kind;
  }
  const name = String(block.tool?.name || block.tool?.meta?.title || '')
    .trim()
    .toLowerCase();
  return name ? `${kind}\u0000${name}` : kind;
}

function isInteractiveDynamicToolBlock(block: WebSessionBlock): boolean {
  return (
    getCompactToolKind(block) === 'dynamic_tool_call' &&
    String(block.tool?.name || block.tool?.meta?.title || '')
      .trim()
      .toLowerCase() === 'askuserquestion'
  );
}

function getCommandGroupId(block: WebSessionBlock): string {
  return String(block.tool?.commandGroup?.id || '').trim();
}

function findGroupId(group: WebSessionBlock[]): string {
  for (const block of group) {
    const groupId = getCommandGroupId(block);
    if (groupId) {
      return groupId;
    }
  }
  return '';
}

function buildSyntheticGroupId(block: WebSessionBlock, fallbackIndex: number): string {
  const anchor = String(block.id || block.key || '').trim() || `index-${fallbackIndex}`;
  const kind = getCompactToolKind(block);
  if (kind === FILE_CHANGE_KIND) {
    return `${SYNTHETIC_FILE_CHANGE_GROUP_PREFIX}${anchor}`;
  }
  return `${SYNTHETIC_COMPACT_TOOL_GROUP_PREFIX}${kind}:${anchor}`;
}

function buildGroupedCompactToolBlock(group: WebSessionBlock[], groupId: string): WebSessionBlock {
  const first = group[0];
  const last = group[group.length - 1];
  const projected = cloneBlock(last);
  const mergedGroupItems: CompactTimelineGroupItem[] = [];

  let count = group.length;
  let firstSeq: number | undefined;
  let lastSeq: number | undefined;

  for (const block of group) {
    const commandGroup = block.tool?.commandGroup;
    if (commandGroup?.count) {
      count = Math.max(count, Math.trunc(commandGroup.count));
    }
    if (Number.isFinite(commandGroup?.firstSeq)) {
      firstSeq =
        firstSeq == null
          ? Number(commandGroup?.firstSeq)
          : Math.min(firstSeq, Number(commandGroup?.firstSeq));
    }
    if (Number.isFinite(commandGroup?.lastSeq)) {
      lastSeq =
        lastSeq == null
          ? Number(commandGroup?.lastSeq)
          : Math.max(lastSeq, Number(commandGroup?.lastSeq));
    }
    mergeGroupItemsInto(mergedGroupItems, getBlockGroupItems(block));
  }

  count = Math.max(count, mergedGroupItems.length);

  if (!projected.tool) {
    return projected;
  }

  const nextCommandGroup = {
    ...(projected.tool.commandGroup ?? {
      id: groupId,
      count,
    }),
    id: groupId,
    count,
    latestToolId: String(projected.tool.id || last.tool?.id || '').trim() || undefined,
    compacted: true,
    ...(firstSeq != null ? { firstSeq } : {}),
    ...(lastSeq != null ? { lastSeq } : {}),
  };

  projected.key = `${projectedKeyPrefix(projected)}${groupId}`;
  projected.payload = {
    ...projected.payload,
    groupItems: mergedGroupItems,
  };
  projected.tool = {
    ...projected.tool,
    meta: {
      ...projected.tool.meta,
      commandGroup: nextCommandGroup,
    },
    commandGroup: nextCommandGroup,
  };

  // Keep the original ordering anchor from the first block in the merged run.
  projected.orderIndex = first.orderIndex;

  return projected;
}

function projectedKeyPrefix(block: WebSessionBlock): string {
  if (getCompactToolKind(block) === FILE_CHANGE_KIND) {
    return PROJECTED_FILE_CHANGE_KEY_PREFIX;
  }
  return PROJECTED_COMPACT_TOOL_KEY_PREFIX;
}

function cloneBlock(block: WebSessionBlock): WebSessionBlock {
  return {
    ...block,
    attachments: block.attachments.map(attachment => ({ ...attachment })),
    tool: block.tool
      ? {
          ...block.tool,
          meta: block.tool.meta ? { ...block.tool.meta } : undefined,
          commandGroup: block.tool.commandGroup ? { ...block.tool.commandGroup } : undefined,
        }
      : undefined,
    payload: block.payload ? { ...block.payload } : undefined,
  };
}

function getBlockGroupItems(block: WebSessionBlock): CompactTimelineGroupItem[] {
  const payloadItems = normalizeGroupItems(block.payload?.groupItems);
  if (payloadItems.length > 0) {
    return payloadItems;
  }

  if (!block.tool) {
    return [];
  }

  const summary = resolveCompactToolSummary(block);
  const completedAt =
    block.tool.status === 'running' ? undefined : toISOString(block.observedAt ?? block.timestamp);

  return [
    {
      toolId: String(block.tool.id || '').trim(),
      kind: String(block.tool.kind || '').trim(),
      title: String(block.tool.name || '').trim(),
      summary,
      command: summary,
      input: block.tool.input,
      output: typeof block.tool.output === 'string' ? block.tool.output : undefined,
      status: normalizeStatus(block.tool.status),
      timestamp: toISOString(block.timestamp),
      startedAt: toISOString(block.tool.startedAt ?? block.timestamp),
      completedAt,
    },
  ];
}

function normalizeGroupItems(raw: unknown): CompactTimelineGroupItem[] {
  if (!Array.isArray(raw)) {
    return [];
  }

  const items: CompactTimelineGroupItem[] = [];
  for (const entry of raw) {
    const record = asRecord(entry);
    if (!record) {
      continue;
    }

    items.push({
      toolId: stringValue(record.toolId),
      kind: stringValue(record.kind),
      title: stringValue(record.title),
      summary: stringValue(record.summary),
      command: stringValue(record.command),
      input: record.input,
      output: optionalString(record.output),
      status: normalizeStatus(record.status),
      timestamp: toISOString(record.timestamp),
      startedAt: toISOString(record.startedAt),
      completedAt: toISOString(record.completedAt),
    });
  }

  return items;
}

function mergeGroupItemsInto(
  target: CompactTimelineGroupItem[],
  nextItems: CompactTimelineGroupItem[]
): void {
  for (const nextItem of nextItems) {
    const toolId = nextItem.toolId.trim();
    if (!toolId) {
      target.push(nextItem);
      continue;
    }

    const existingIndex = target.findIndex(item => item.toolId.trim() === toolId);
    if (existingIndex < 0) {
      target.push(nextItem);
      continue;
    }

    target.splice(existingIndex, 1, {
      ...target[existingIndex],
      ...nextItem,
    });
  }
}

function resolveCompactToolSummary(block: WebSessionBlock): string {
  if (!block.tool) {
    return '';
  }
  const kind = getCompactToolKind(block);
  const input = asRecord(block.tool.input);
  const meta = asRecord(block.tool.meta);
  if (kind === FILE_CHANGE_KIND) {
    return resolveFileChangeSummary(block);
  }
  if (kind === 'command_execution') {
    return firstNonEmpty(
      stringValue(input?.command),
      stringValue(meta?.subtitle),
      stringValue(meta?.command),
      stringValue(block.tool.output)
    );
  }
  if (kind === 'mcp_tool_call') {
    return firstNonEmpty(
      stringValue(input?.tool_name),
      stringValue(input?.name),
      stringValue(meta?.subtitle),
      stringValue(block.tool.output)
    );
  }
  if (kind === 'web_search') {
    return firstNonEmpty(
      stringValue(input?.query),
      stringValue(meta?.subtitle),
      stringValue(block.tool.output)
    );
  }
  if (kind === 'dynamic_tool_call') {
    return resolveDynamicToolSummary(block.tool.input, block.tool.meta, block.tool.output);
  }
  return firstNonEmpty(stringValue(meta?.subtitle), stringValue(block.tool.output));
}

function resolveDynamicToolSummary(
  input: unknown,
  meta: Record<string, unknown> | undefined,
  output: string | undefined
): string {
  const record = asRecord(input);
  const toolName = String(meta?.title ?? meta?.name ?? '')
    .trim()
    .toLowerCase();
  const path = firstNonEmpty(
    stringValue(record?.file_path),
    stringValue(record?.path),
    stringValue(record?.notebook_path)
  );
  const pattern = stringValue(record?.pattern);
  const query = firstNonEmpty(stringValue(record?.query), stringValue(record?.url));

  if (toolName === 'read' || toolName === 'notebookread' || toolName === 'ls') {
    if (path) {
      return path;
    }
  } else if (toolName === 'grep' || toolName === 'glob') {
    if (pattern && path) {
      return `${pattern} · ${path}`;
    }
    if (pattern || path) {
      return pattern || path;
    }
  } else if (path || query || pattern) {
    return path || query || pattern;
  }

  return firstNonEmpty(stringValue(meta?.subtitle), stringValue(output));
}

function resolveFileChangeSummary(block: WebSessionBlock): string {
  if (!block.tool) {
    return '';
  }

  const directPath = resolveFileChangePath(block.tool.input);
  if (directPath) {
    return directPath;
  }

  return stringValue(asRecord(block.tool.meta)?.subtitle);
}

function resolveFileChangePath(raw: unknown): string {
  const record = asRecord(raw);
  if (!record) {
    return '';
  }

  const directPath = firstNonEmpty(
    stringValue(record.path),
    stringValue(record.file_path),
    stringValue(record.new_path),
    stringValue(record.old_path),
    stringValue(record.newPath),
    stringValue(record.oldPath),
    stringValue(record.move_path),
    stringValue(record.movePath)
  );
  if (directPath) {
    return directPath;
  }

  const changes = Array.isArray(record.changes) ? record.changes : [];
  for (const change of changes) {
    const changePath = resolveFileChangePath(change);
    if (changePath) {
      return changePath;
    }
  }

  return '';
}

function normalizeStatus(value: unknown): 'running' | 'done' | 'error' {
  switch (String(value || '').trim()) {
    case 'done':
    case 'completed':
      return 'done';
    case 'error':
      return 'error';
    default:
      return 'running';
  }
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return undefined;
  }
  return value as Record<string, unknown>;
}

function optionalString(value: unknown): string | undefined {
  const normalized = stringValue(value);
  return normalized || undefined;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function firstNonEmpty(...values: string[]): string {
  for (const value of values) {
    if (value) {
      return value;
    }
  }
  return '';
}

function toISOString(value: unknown): string | undefined {
  if (typeof value === 'string') {
    const normalized = value.trim();
    return normalized || undefined;
  }

  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return undefined;
  }

  try {
    return new Date(value).toISOString();
  } catch {
    return undefined;
  }
}
