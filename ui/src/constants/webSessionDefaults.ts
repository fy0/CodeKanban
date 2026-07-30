import type {
  WebSessionCodexDefaultPermissionLevel,
  WebSessionCodexDefaultReasoningEffort,
  WebSessionReasoningEffort,
} from '@/types/models';

export const DEFAULT_WEB_SESSION_CODEX_MODEL = 'default';
export const DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT: WebSessionCodexDefaultReasoningEffort =
  'default';
export const DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL: WebSessionCodexDefaultPermissionLevel =
  'default';
export const DEFAULT_WEB_SESSION_CODEX_SYNC_MODE = 'default' as const;

export const EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_MODEL = 'gpt-5.6-sol';
export const EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT: WebSessionReasoningEffort =
  'xhigh';
export const EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL = 'elevated' as const;
export const EFFECTIVE_DEFAULT_WEB_SESSION_CODEX_SYNC_MODE = 'fast' as const;

export const WEB_SESSION_REASONING_EFFORTS: WebSessionReasoningEffort[] = [
  'default',
  'none',
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max',
  'ultra',
];

export const GENERIC_CODEX_REASONING_EFFORTS: WebSessionReasoningEffort[] = [
  'default',
  'none',
  'low',
  'medium',
  'high',
  'xhigh',
];

export const WEB_SESSION_CODEX_DEFAULT_REASONING_EFFORTS: WebSessionCodexDefaultReasoningEffort[] =
  ['model_default', ...WEB_SESSION_REASONING_EFFORTS];

export const WEB_SESSION_CODEX_DEFAULT_PERMISSION_LEVELS: WebSessionCodexDefaultPermissionLevel[] =
  ['default', 'standard', 'elevated', 'yolo'];

export function normalizeConfiguredCodexReasoningEffort(
  value: unknown
): WebSessionCodexDefaultReasoningEffort {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : '';
  return WEB_SESSION_CODEX_DEFAULT_REASONING_EFFORTS.includes(
    normalized as WebSessionCodexDefaultReasoningEffort
  )
    ? (normalized as WebSessionCodexDefaultReasoningEffort)
    : DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT;
}

export function normalizeConfiguredCodexPermissionLevel(
  value: unknown
): WebSessionCodexDefaultPermissionLevel {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : '';
  return WEB_SESSION_CODEX_DEFAULT_PERMISSION_LEVELS.includes(
    normalized as WebSessionCodexDefaultPermissionLevel
  )
    ? (normalized as WebSessionCodexDefaultPermissionLevel)
    : DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL;
}
