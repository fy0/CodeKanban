import { normalizeWebSessionActivityToolKind } from '@/constants/webSessionActivityDisplayMode';
import type { WebSessionBlock } from '@/stores/webSession';
import { parseImageViewToolOutput, resolveImageViewDisplayName } from '@/utils/webSessionImages';

type WebSessionTool = NonNullable<WebSessionBlock['tool']>;
type Translate = (key: string) => string;

export type WebSessionToolPresentationOptions = {
  translate: Translate;
  shouldShowPending: (tool: WebSessionTool) => boolean;
};

export function createWebSessionToolPresentation({
  translate,
  shouldShowPending,
}: WebSessionToolPresentationOptions) {
  function stringifyValue(value: unknown): string {
    if (typeof value === 'string') {
      return value;
    }
    try {
      const serialized = JSON.stringify(value, null, 2);
      return typeof serialized === 'string' ? serialized : String(value ?? '');
    } catch {
      return String(value ?? '');
    }
  }

  function asRecord(value: unknown): Record<string, unknown> | undefined {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      return undefined;
    }
    return value as Record<string, unknown>;
  }

  function extractToolWorkingDirectory(input: unknown) {
    const record = asRecord(input);
    if (!record) {
      return '';
    }
    const direct = String(record.cwd ?? record.workdir ?? '').trim();
    if (direct) {
      return direct;
    }
    const args = asRecord(record.arguments);
    return String(args?.cwd ?? args?.workdir ?? '').trim();
  }

  function getImageViewToolData(tool?: WebSessionTool) {
    return tool ? parseImageViewToolOutput(tool.output) : null;
  }

  function isImageViewTool(tool?: WebSessionTool) {
    return Boolean(getImageViewToolData(tool));
  }

  function getImageViewDisplayName(tool?: WebSessionTool) {
    const data = getImageViewToolData(tool);
    return data ? resolveImageViewDisplayName(data.path) : '';
  }

  function getImageViewDisplayPath(tool?: WebSessionTool) {
    return getImageViewToolData(tool)?.path ?? '';
  }

  function normalizeToolKindValue(value: string | undefined) {
    return normalizeWebSessionActivityToolKind(value);
  }

  function isContextCompactionToolKind(value: string | undefined) {
    return normalizeToolKindValue(value) === 'context_compaction';
  }

  function isCompactToolKind(value: string | undefined) {
    return [
      'command_execution',
      'file_change',
      'mcp_tool_call',
      'web_search',
      'dynamic_tool_call',
    ].includes(normalizeToolKindValue(value));
  }

  function compactToolLabel(tool?: {
    kind?: string;
    name?: string;
    title?: string;
    meta?: Record<string, unknown>;
  }) {
    const kind = normalizeToolKindValue(tool?.kind || String(tool?.meta?.kind ?? ''));
    if (kind === 'command_execution') {
      return translate('webSession.toolCommandExecution');
    }
    if (kind === 'file_change') {
      return translate('webSession.toolFileChange');
    }
    if (kind === 'mcp_tool_call') {
      return translate('webSession.toolMcpToolCall');
    }
    if (kind === 'sub_agent_tool_call') {
      return translate('webSession.toolSubAgentCall');
    }
    if (kind === 'web_search') {
      return translate('webSession.toolWebSearch');
    }
    if (kind === 'dynamic_tool_call') {
      return (
        String(tool?.name || tool?.title || tool?.meta?.title || '').trim() ||
        translate('webSession.toolKindDefault')
      );
    }
    return translate('webSession.toolKindDefault');
  }

  function isInteractiveDynamicTool(tool: {
    kind?: string;
    name?: string;
    meta?: Record<string, unknown>;
  }) {
    return (
      normalizeToolKindValue(tool.kind || String(tool.meta?.kind ?? '')) === 'dynamic_tool_call' &&
      String(tool.name || tool.meta?.title || '')
        .trim()
        .toLowerCase() === 'askuserquestion'
    );
  }

  function isCompactTool(tool: Pick<WebSessionTool, 'kind' | 'name' | 'meta' | 'commandGroup'>) {
    const kind = normalizeToolKindValue(tool.kind || String(tool.meta?.kind ?? ''));
    return kind === 'dynamic_tool_call' ? !isInteractiveDynamicTool(tool) : isCompactToolKind(kind);
  }

  function getCompactToolKind(tool: Pick<WebSessionTool, 'kind' | 'meta'>) {
    return normalizeToolKindValue(tool.kind || String(tool.meta?.kind ?? ''));
  }

  function toolCardClass(tool: Pick<WebSessionTool, 'kind' | 'meta'>) {
    return {
      'is-context-compaction-tool': isContextCompactionToolKind(
        tool.kind || String(tool.meta?.kind ?? '')
      ),
    };
  }

  function getDynamicToolSummary(
    input: unknown,
    meta: Record<string, unknown> | undefined,
    output?: string
  ) {
    const record = asRecord(input);
    const toolName = String(meta?.title ?? meta?.name ?? '')
      .trim()
      .toLowerCase();
    const path =
      [record?.file_path, record?.path, record?.notebook_path]
        .map(value => String(value ?? '').trim())
        .find(Boolean) ?? '';
    const pattern = String(record?.pattern ?? '').trim();
    const query =
      [record?.query, record?.url].map(value => String(value ?? '').trim()).find(Boolean) ?? '';

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
    return String(meta?.subtitle ?? output ?? '').trim();
  }

  function getCompactToolSummary(tool: WebSessionTool) {
    const kind = getCompactToolKind(tool);
    const input = asRecord(tool.input);
    const subtitle = String(tool.meta?.subtitle ?? '').trim();

    if (kind === 'command_execution') {
      return String(input?.command ?? '').trim() || subtitle;
    }
    if (kind === 'file_change') {
      const path =
        String(
          input?.path ?? input?.file_path ?? input?.new_path ?? input?.old_path ?? ''
        ).trim() || subtitle;
      if (path) {
        return path;
      }
      const changes = Array.isArray(input?.changes) ? input.changes.length : 0;
      return changes > 0 ? `${changes} change${changes > 1 ? 's' : ''}` : '';
    }
    if (kind === 'mcp_tool_call') {
      const toolName = String(input?.tool_name ?? input?.name ?? '').trim();
      const args = asRecord(input?.arguments);
      const target =
        String(
          args?.url ??
            args?.query ??
            args?.path ??
            args?.file ??
            args?.name ??
            args?.id ??
            input?.server ??
            input?.path ??
            ''
        ).trim() || subtitle;
      return toolName && target && toolName !== target
        ? `${toolName} · ${target}`
        : toolName || target;
    }
    if (kind === 'web_search') {
      const query = String(input?.query ?? '').trim();
      if (query) {
        return query;
      }
      const action = asRecord(input?.action);
      const queries = Array.isArray(action?.queries)
        ? action.queries
            .map(value => String(value ?? '').trim())
            .filter((value): value is string => Boolean(value))
        : [];
      return queries[0] ?? subtitle;
    }
    if (kind === 'dynamic_tool_call') {
      return getDynamicToolSummary(tool.input, tool.meta, tool.output);
    }
    return subtitle;
  }

  function contextCompactionPreview(tool: { output?: string; meta?: Record<string, unknown> }) {
    const preview = String(tool.output ?? tool.meta?.subtitle ?? '')
      .replace(/\s+/g, ' ')
      .trim();
    return preview
      ? preview.slice(0, 120)
      : translate('webSession.contextCompactionFallbackPreview');
  }

  function getCompactToolDisplaySummary(tool: WebSessionTool) {
    const summary = getCompactToolSummary(tool).trim();
    if (summary) {
      return summary;
    }
    return shouldShowPending(tool)
      ? translate('common.loading')
      : translate('webSession.compactToolNoSummary');
  }

  function toolKindLabel(tool: { name: string; kind?: string; output?: string }) {
    if (isImageViewTool(tool as WebSessionTool)) {
      return translate('webSession.toolImageView');
    }
    const kind = normalizeToolKindValue(tool.kind);
    if (!kind) {
      return translate('webSession.toolKindDefault');
    }
    const labels: Record<string, string> = {
      command_execution: 'webSession.toolCommandExecution',
      file_change: 'webSession.toolFileChange',
      mcp_tool_call: 'webSession.toolMcpToolCall',
      context_compaction: 'webSession.toolContextCompaction',
      tool_use: 'webSession.toolKindTool',
    };
    if (kind === 'shell') {
      return 'Shell';
    }
    return labels[kind] ? translate(labels[kind]) : kind;
  }

  function formatToolPreview(tool: {
    input?: unknown;
    output?: string;
    kind?: string;
    meta?: Record<string, unknown>;
    commandGroup?: { count: number };
  }) {
    if (isContextCompactionToolKind(tool.kind || String(tool.meta?.kind ?? ''))) {
      return contextCompactionPreview(tool);
    }
    const imageViewData = getImageViewToolData(tool as WebSessionTool);
    if (imageViewData) {
      return resolveImageViewDisplayName(imageViewData.path);
    }
    if (isCompactTool(tool as WebSessionTool)) {
      return getCompactToolSummary(tool as WebSessionTool);
    }
    const source =
      typeof tool.output === 'string' && tool.output.trim()
        ? tool.output
        : stringifyValue(tool.input);
    const preview = String(source ?? '')
      .replace(/\s+/g, ' ')
      .trim()
      .slice(0, 120);
    if (preview) {
      return preview;
    }
    return shouldShowPending(tool as WebSessionTool) ? translate('common.loading') : '';
  }

  return {
    stringifyValue,
    asRecord,
    extractToolWorkingDirectory,
    getImageViewToolData,
    isImageViewTool,
    getImageViewDisplayName,
    getImageViewDisplayPath,
    normalizeToolKindValue,
    isContextCompactionToolKind,
    isCompactToolKind,
    compactToolLabel,
    isCompactTool,
    isInteractiveDynamicTool,
    getCompactToolKind,
    toolCardClass,
    getCompactToolSummary,
    getCompactToolDisplaySummary,
    toolKindLabel,
    formatToolPreview,
  };
}
