import type { WebSessionBlock } from '@/stores/webSession';

const reconnectProgressPattern = /^reconnecting\.\.\.\s*\d+\s*\/\s*\d+/i;

export function isTransportRetryActivityText(value: string) {
  return reconnectProgressPattern.test(value.trim());
}

export function isSubAgentActivityBlock(block: WebSessionBlock) {
  return !(
    (block.itemType === 'note' &&
      String(block.payload?.code ?? '').trim() === 'transport_retrying') ||
    isTransportRetryActivityText(block.text)
  );
}

export function findLatestSubAgentActivityBlock(
  blocks: readonly WebSessionBlock[],
  threadId: string
) {
  const normalizedThreadId = threadId.trim();
  if (!normalizedThreadId) {
    return undefined;
  }
  return [...blocks]
    .reverse()
    .find(
      block =>
        String(block.sourceThreadId ?? '').trim() === normalizedThreadId &&
        isSubAgentActivityBlock(block)
    );
}

export function subAgentActivitySummary(block: WebSessionBlock) {
  if (block.tool) {
    const kind = String(block.tool.kind ?? block.tool.meta?.kind ?? '')
      .trim()
      .toLowerCase();
    if (kind === 'file_change') {
      const input = activityRecord(block.tool.input);
      const directPath = activityPath(input);
      const changes = Array.isArray(input?.changes) ? input.changes : [];
      const changedPath = changes.map(change => activityPath(activityRecord(change))).find(Boolean);
      if (directPath || changedPath) {
        return directPath || changedPath;
      }
    }
    return (
      String(block.tool.meta?.subtitle ?? '').trim() ||
      String(block.tool.name ?? '').trim() ||
      block.text.trim()
    );
  }
  return block.text.trim();
}

function activityRecord(value: unknown) {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function activityPath(record: Record<string, unknown> | undefined) {
  if (!record) {
    return '';
  }
  return String(
    record.path ??
      record.file_path ??
      record.filePath ??
      record.new_path ??
      record.newPath ??
      record.old_path ??
      record.oldPath ??
      ''
  ).trim();
}
