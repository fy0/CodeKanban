import { describe, expect, it } from 'vitest';

import { resolveWebSessionAgentCapability } from '../webSessionAgentCapabilities';
import type { WebSessionRuntimeConfig } from '@/types/models';

function legacyRuntimeConfig(): WebSessionRuntimeConfig {
  return {
    contextWindowTokens: 0,
    compactLimitTokens: 0,
    source: 'unavailable',
    models: [],
    hasCodex: true,
    hasClaudeCode: false,
    supportsWebSession: true,
    webSessionMinCodexVersion: '',
    supportsMultiAgentV2: true,
    supportsGoalMode: true,
    goalModeMinCodexVersion: '0.133.0',
  };
}

describe('web session agent capabilities', () => {
  it('prefers the provider-neutral agents map', () => {
    const config = legacyRuntimeConfig();
    config.agents = {
      pi: {
        installed: true,
        version: '0.84.1',
        supportsWebSession: true,
        supportsTree: true,
        supportsImages: true,
        supportsCompaction: true,
        supportsSteer: true,
        supportsFollowUp: true,
        supportsGoal: false,
        supportsSubAgentRegistry: false,
        permissionModes: [{ id: 'unrestricted', available: true }],
      },
    };

    const capability = resolveWebSessionAgentCapability(config, 'pi');
    expect(capability.installed).toBe(true);
    expect(capability.supportsTree).toBe(true);
    expect(capability.permissionModes).toEqual([{ id: 'unrestricted', available: true }]);
  });

  it('keeps old services compatible through legacy top-level fields', () => {
    const config = legacyRuntimeConfig();
    expect(resolveWebSessionAgentCapability(config, 'codex')).toMatchObject({
      installed: true,
      supportsWebSession: true,
      supportsGoal: true,
      supportsSubAgentRegistry: true,
    });
    expect(resolveWebSessionAgentCapability(config, 'claude').supportsWebSession).toBe(false);
    expect(resolveWebSessionAgentCapability(config, 'pi').supportsWebSession).toBe(false);

    config.hasPi = true;
    config.piVersion = '0.84.1';
    config.supportsPiWebSession = false;
    config.piRpcCompatible = true;
    expect(resolveWebSessionAgentCapability(config, 'pi')).toMatchObject({
      installed: true,
      version: '0.84.1',
      supportsWebSession: false,
      supportsTree: false,
      supportsCompaction: false,
      supportsSteer: false,
      supportsFollowUp: false,
    });

    config.supportsPiWebSession = true;
    expect(resolveWebSessionAgentCapability(config, 'pi')).toMatchObject({
      supportsWebSession: true,
      supportsTree: true,
      supportsImages: true,
      supportsCompaction: true,
      supportsSteer: true,
      supportsFollowUp: true,
    });
  });
});
