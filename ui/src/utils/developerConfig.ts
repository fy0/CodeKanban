import {
  DEFAULT_WEB_SESSION_CODEX_MODEL,
  DEFAULT_WEB_SESSION_CODEX_PERMISSION_LEVEL,
  DEFAULT_WEB_SESSION_CODEX_REASONING_EFFORT,
  DEFAULT_WEB_SESSION_CODEX_SYNC_MODE,
  normalizeConfiguredCodexPermissionLevel,
  normalizeConfiguredCodexReasoningEffort,
} from '@/constants/webSessionDefaults';
import type {
  DeveloperConfig,
  WebSessionActiveCallTimeoutConfig,
  WebSessionAutoRetryDefaultsConfig,
} from '@/types/models';

export const DEFAULT_WEB_SESSION_AUTO_RETRY_DEFAULTS = {
  scope: 'network_only',
  preset: 'gentle_stop',
  maxAttempts: 0,
  dispatchPendingOnFailure: false,
} as const satisfies WebSessionAutoRetryDefaultsConfig;

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

export function sanitizeAutoRetryDefaultsConfig(
  value?: Partial<WebSessionAutoRetryDefaultsConfig> | null
): WebSessionAutoRetryDefaultsConfig {
  return {
    scope:
      value?.scope === 'network_and_rate_limit' || value?.scope === 'all_failures'
        ? value.scope
        : DEFAULT_WEB_SESSION_AUTO_RETRY_DEFAULTS.scope,
    preset:
      value?.preset === 'aggressive_stop' || value?.preset === 'sustain_60s'
        ? value.preset
        : DEFAULT_WEB_SESSION_AUTO_RETRY_DEFAULTS.preset,
    maxAttempts: Math.min(100, Math.max(0, Math.trunc(Number(value?.maxAttempts) || 0))),
    dispatchPendingOnFailure: value?.dispatchPendingOnFailure === true,
  };
}

export function sanitizeDeveloperConfig(value?: Partial<DeveloperConfig> | null): DeveloperConfig {
  const configuredModel = value?.webSessionCodexDefaultModel?.trim();
  return {
    enableTerminalScrollback: value?.enableTerminalScrollback ?? false,
    enableTerminalStateSnapshot: value?.enableTerminalStateSnapshot ?? false,
    webSessionCodexContextWindow: [0, 512000, 768000, 1000000].includes(
      value?.webSessionCodexContextWindow ?? 0
    )
      ? (value?.webSessionCodexContextWindow ?? 0)
      : 0,
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
    webSessionAutoRetryDefaults: sanitizeAutoRetryDefaultsConfig(
      value?.webSessionAutoRetryDefaults
    ),
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
