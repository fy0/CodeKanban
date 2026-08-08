export const WEB_SESSION_ACTIVITY_DISPLAY_MODES = ['default', 'text', 'card'] as const;

export type WebSessionActivityDisplayMode = (typeof WEB_SESSION_ACTIVITY_DISPLAY_MODES)[number];

export const DEFAULT_WEB_SESSION_ACTIVITY_DISPLAY_MODE: WebSessionActivityDisplayMode = 'default';

const ACTIVITY_DISPLAY_TOOL_KINDS = [
  'command_execution',
  'file_change',
  'mcp_tool_call',
  'dynamic_tool_call',
  'sub_agent_tool_call',
  'web_search',
  'reasoning',
] as const;

export function sanitizeWebSessionActivityDisplayMode(
  value: unknown
): WebSessionActivityDisplayMode {
  return WEB_SESSION_ACTIVITY_DISPLAY_MODES.includes(value as WebSessionActivityDisplayMode)
    ? (value as WebSessionActivityDisplayMode)
    : DEFAULT_WEB_SESSION_ACTIVITY_DISPLAY_MODE;
}

export function normalizeWebSessionActivityToolKind(value: string | undefined) {
  const normalized = String(value ?? '').trim();
  if (normalized === 'commandExecution') {
    return 'command_execution';
  }
  if (normalized === 'contextCompaction') {
    return 'context_compaction';
  }
  if (
    normalized === 'collabAgentToolCall' ||
    normalized === 'collab_agent_tool_call' ||
    normalized === 'sub_agent_tool_call'
  ) {
    return 'sub_agent_tool_call';
  }
  if (normalized === 'mcpToolCall') {
    return 'mcp_tool_call';
  }
  if (normalized === 'fileChange') {
    return 'file_change';
  }
  if (normalized === 'webSearch') {
    return 'web_search';
  }
  return normalized;
}

export function resolveWebSessionActivityDisplayMode(mode: WebSessionActivityDisplayMode) {
  return mode === 'card' ? 'card' : 'text';
}

export function shouldUseWebSessionActivityDisplayMode(mode: WebSessionActivityDisplayMode) {
  return mode === 'default' || mode === 'text' || mode === 'card';
}

export function isWebSessionActivityDisplayToolKind(value: string | undefined) {
  const normalized = normalizeWebSessionActivityToolKind(value);
  return ACTIVITY_DISPLAY_TOOL_KINDS.includes(
    normalized as (typeof ACTIVITY_DISPLAY_TOOL_KINDS)[number]
  );
}
