import type {
  WebSessionAgent,
  WebSessionAgentCapability,
  WebSessionRuntimeConfig,
} from '@/types/models';

const UNAVAILABLE_PERMISSION_MODES = [
  { id: 'unrestricted', available: false },
  { id: 'approval', available: false },
  { id: 'sandbox', available: false },
];

function unavailableCapability(): WebSessionAgentCapability {
  return {
    installed: false,
    supportsWebSession: false,
    supportsTree: false,
    supportsImages: false,
    supportsCompaction: false,
    supportsSteer: false,
    supportsFollowUp: false,
    supportsGoal: false,
    supportsSubAgentRegistry: false,
    permissionModes: UNAVAILABLE_PERMISSION_MODES.map(mode => ({ ...mode })),
  };
}

export function isPiRuntimeCapabilityPending(
  configReady: boolean,
  config: WebSessionRuntimeConfig | null | undefined
) {
  if (!configReady) {
    return true;
  }
  if (!config || config.capabilitiesRefreshing !== true) {
    return false;
  }
  if (resolveWebSessionAgentCapability(config, 'pi').supportsWebSession) {
    return false;
  }
  return !String(config.piDiagnostics ?? '').trim();
}

export function resolveWebSessionAgentCapability(
  config: WebSessionRuntimeConfig | null | undefined,
  agent: WebSessionAgent
): WebSessionAgentCapability {
  const explicit = config?.agents?.[agent];
  if (explicit) {
    return {
      ...unavailableCapability(),
      ...explicit,
      permissionModes: Array.isArray(explicit.permissionModes)
        ? explicit.permissionModes.map(mode => ({ ...mode }))
        : [],
    };
  }

  const fallback = unavailableCapability();
  if (!config) {
    return fallback;
  }
  if (agent === 'claude') {
    return {
      ...fallback,
      installed: config.hasClaudeCode === true,
      supportsWebSession: config.hasClaudeCode === true,
      supportsImages: true,
      supportsCompaction: true,
      supportsSteer: true,
      supportsFollowUp: true,
    };
  }
  if (agent === 'codex') {
    return {
      ...fallback,
      installed: config.hasCodex === true,
      version: config.codexVersion,
      supportsWebSession: config.hasCodex === true && config.supportsWebSession !== false,
      supportsImages: true,
      supportsCompaction: true,
      supportsSteer: true,
      supportsFollowUp: true,
      supportsGoal: config.supportsGoalMode === true,
      supportsSubAgentRegistry: config.supportsMultiAgentV2 === true,
    };
  }
  if (agent === 'pi') {
    return {
      ...fallback,
      installed: config.hasPi === true,
      version: config.piVersion,
      supportsWebSession: config.hasPi === true && config.supportsPiWebSession === true,
      supportsTree: config.supportsPiWebSession === true,
      supportsImages: config.supportsPiWebSession === true,
      supportsCompaction: config.supportsPiWebSession === true,
      supportsSteer: config.supportsPiWebSession === true,
      supportsFollowUp: config.supportsPiWebSession === true,
    };
  }
  return fallback;
}
