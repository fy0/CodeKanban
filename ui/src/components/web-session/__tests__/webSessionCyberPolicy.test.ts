import { describe, expect, it } from 'vitest';

import {
  isWebSessionDevMode,
  shouldShowCyberPolicyWarning,
} from '@/components/web-session/webSessionDevMode';

describe('web session cyber policy state', () => {
  it('enables a local warning simulator only when the URL contains the DEV parameter', () => {
    expect(isWebSessionDevMode({ DEV: null })).toBe(true);
    expect(isWebSessionDevMode({ DEV: '1' })).toBe(true);
    expect(isWebSessionDevMode({ dev: null })).toBe(false);
    expect(isWebSessionDevMode({})).toBe(false);

    expect(
      shouldShowCyberPolicyWarning({
        sessionFlagged: false,
        sessionDismissed: false,
        devMode: true,
        simulatedWarning: true,
      })
    ).toBe(true);
    expect(
      shouldShowCyberPolicyWarning({
        sessionFlagged: false,
        sessionDismissed: false,
        devMode: false,
        simulatedWarning: true,
      })
    ).toBe(false);
    expect(
      shouldShowCyberPolicyWarning({
        sessionFlagged: true,
        sessionDismissed: false,
        devMode: false,
        simulatedWarning: false,
      })
    ).toBe(true);
    expect(
      shouldShowCyberPolicyWarning({
        sessionFlagged: true,
        sessionDismissed: true,
        devMode: false,
        simulatedWarning: false,
      })
    ).toBe(false);
    expect(
      shouldShowCyberPolicyWarning({
        sessionFlagged: true,
        sessionDismissed: true,
        devMode: true,
        simulatedWarning: true,
      })
    ).toBe(true);
  });
});
