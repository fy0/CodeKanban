import {
  DEFAULT_WEB_SESSION_CODEX_MODEL,
  DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL,
  DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT,
  DEFAULT_WEB_SESSION_CODEX_SYNC_MODE,
  normalizeConfiguredCodexPermissionLevel,
  normalizeConfiguredCodexReasoningEffort,
} from '@/constants/webSessionDefaults';
import type { DeveloperConfig, WebSessionActiveCallTimeoutConfig } from '@/types/models';

export const DEFAULT_ACTIVE_CALL_TIMEOUT_CUSTOM_SECONDS = 120;
export const DEFAULT_ACTIVE_CALL_TIMEOUT_CALL_KINDS = {
  useDefault: true,
  mcp: true,
  command: false,
  tool: true,
} as const;
export const DEFAULT_ACTIVE_CALL_TIMEOUT_PROMPT =
  'The current ${call} call has been running for ${duration} and may be stuck. It was interrupted automatically. Continue.';

export function sanitizeActiveCallTimeoutConfig(
  value?: Partial<WebSessionActiveCallTimeoutConfig> | null
): WebSessionActiveCallTimeoutConfig {
  const useDefaultCallKinds = value?.callKinds?.useDefault !== false;
  return {
    enabledMode:
      value?.enabledMode === 'on' || value?.enabledMode === 'off' ? value.enabledMode : 'default',
    timeoutMode: value?.timeoutMode === 'custom' ? 'custom' : 'default',
    customTimeoutSeconds: Math.max(
      10,
      Number(value?.customTimeoutSeconds) || DEFAULT_ACTIVE_CALL_TIMEOUT_CUSTOM_SECONDS
    ),
    promptTemplate: value?.promptTemplate?.trim() || DEFAULT_ACTIVE_CALL_TIMEOUT_PROMPT,
    callKinds: useDefaultCallKinds
      ? { ...DEFAULT_ACTIVE_CALL_TIMEOUT_CALL_KINDS }
      : {
          useDefault: false,
          mcp: value?.callKinds?.mcp !== false,
          command: value?.callKinds?.command === true,
          tool: value?.callKinds?.tool !== false,
        },
  };
}

export function sanitizeDeveloperConfig(value?: Partial<DeveloperConfig> | null): DeveloperConfig {
  const configuredModel = value?.webSessionCodexDefaultModel?.trim();
  return {
    enableTerminalScrollback: value?.enableTerminalScrollback ?? false,
    renameSessionTitleEachCommand: value?.renameSessionTitleEachCommand ?? false,
    enableTerminalStateSnapshot: value?.enableTerminalStateSnapshot ?? false,
    webSessionCodexDefaultModel:
      configuredModel?.toLowerCase() === DEFAULT_WEB_SESSION_CODEX_MODEL
        ? DEFAULT_WEB_SESSION_CODEX_MODEL
        : configuredModel || DEFAULT_WEB_SESSION_CODEX_MODEL,
    webSessionCodexDefaultReasoningEffort: normalizeConfiguredCodexReasoningEffort(
      value?.webSessionCodexDefaultReasoningEffort ?? DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT
    ),
    webSessionCodexDefaultPermissionLevel: normalizeConfiguredCodexPermissionLevel(
      value?.webSessionCodexDefaultPermissionLevel ?? DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL
    ),
    webSessionCodexDefaultSyncMode:
      value?.webSessionCodexDefaultSyncMode === 'fast' ||
      value?.webSessionCodexDefaultSyncMode === 'deep'
        ? value.webSessionCodexDefaultSyncMode
        : DEFAULT_WEB_SESSION_CODEX_SYNC_MODE,
    webSessionActiveCallTimeout: sanitizeActiveCallTimeoutConfig(
      value?.webSessionActiveCallTimeout
    ),
  };
}

export function cloneDeveloperConfig(value?: Partial<DeveloperConfig> | null): DeveloperConfig {
  return sanitizeDeveloperConfig(value);
}

export function applyDeveloperConfig(target: DeveloperConfig, source: DeveloperConfig) {
  Object.assign(target, cloneDeveloperConfig(source));
}
