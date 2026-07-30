import { describe, expect, it } from 'vitest';
import { cloneDeveloperConfig, sanitizeDeveloperConfig } from '@/utils/developerConfig';

describe('developer config defaults', () => {
  it('preserves version-managed sentinels for missing server fields', () => {
    const config = sanitizeDeveloperConfig();

    expect(config.webSessionCodexDefaultModel).toBe('default');
    expect(config.webSessionCodexDefaultReasoningEffort).toBe('default');
    expect(config.webSessionCodexDefaultPermissionLevel).toBe('default');
    expect(config.webSessionCodexDefaultSyncMode).toBe('default');
  });

  it('trims custom models and normalizes explicit settings', () => {
    const config = sanitizeDeveloperConfig({
      webSessionCodexDefaultModel: '  custom-codex-model  ',
      webSessionCodexDefaultReasoningEffort: 'model_default',
      webSessionCodexDefaultPermissionLevel: 'standard',
      webSessionCodexDefaultSyncMode: 'deep',
    });

    expect(config.webSessionCodexDefaultModel).toBe('custom-codex-model');
    expect(config.webSessionCodexDefaultReasoningEffort).toBe('model_default');
    expect(config.webSessionCodexDefaultPermissionLevel).toBe('standard');
    expect(config.webSessionCodexDefaultSyncMode).toBe('deep');
  });

  it('falls back from invalid values and returns independent clones', () => {
    const source = sanitizeDeveloperConfig({
      webSessionCodexDefaultReasoningEffort: 'invalid' as never,
      webSessionCodexDefaultPermissionLevel: 'invalid' as never,
      webSessionCodexDefaultSyncMode: 'invalid' as never,
    });
    const clone = cloneDeveloperConfig(source);

    expect(source.webSessionCodexDefaultReasoningEffort).toBe('default');
    expect(source.webSessionCodexDefaultPermissionLevel).toBe('default');
    expect(source.webSessionCodexDefaultSyncMode).toBe('default');
    clone.webSessionActiveCallTimeout.callKinds.mcp = false;
    expect(source.webSessionActiveCallTimeout.callKinds.mcp).toBe(true);
  });
});
