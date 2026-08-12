import { describe, expect, it } from 'vitest';
import { isPiProjectTrusted, piProjectTrustNeedsRenewal } from './piProjectTrust';

const trustedStatus = {
  projectId: 'project-1',
  agent: 'pi' as const,
  projectPath: 'D:/repo',
  trustedPath: 'D:/repo',
  trusted: true,
};

describe('Pi project trust helpers', () => {
  it('accepts only an explicit Pi trust for the active project', () => {
    expect(isPiProjectTrusted(trustedStatus, 'project-1')).toBe(true);
    expect(isPiProjectTrusted(trustedStatus, 'project-2')).toBe(false);
    expect(isPiProjectTrusted({ ...trustedStatus, trusted: false }, 'project-1')).toBe(false);
    expect(isPiProjectTrusted({ ...trustedStatus, agent: 'codex' as never }, 'project-1')).toBe(
      false
    );
    expect(isPiProjectTrusted(null, 'project-1')).toBe(false);
  });

  it('detects a stale or revoked path that needs renewed confirmation', () => {
    expect(piProjectTrustNeedsRenewal({ ...trustedStatus, trusted: false })).toBe(true);
    expect(piProjectTrustNeedsRenewal({ ...trustedStatus, trusted: false, trustedPath: '' })).toBe(
      false
    );
    expect(piProjectTrustNeedsRenewal(trustedStatus)).toBe(false);
  });
});
