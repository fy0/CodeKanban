import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import {
  isWebSessionDevMode,
  shouldShowCyberPolicyWarning,
} from '@/components/web-session/webSessionDevMode';

const panelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const storePath = fileURLToPath(new URL('../../../stores/webSession.ts', import.meta.url));

describe('web session cyber policy state', () => {
  it('renders a persistent warning from the session summary flag', () => {
    const source = readFileSync(panelPath, 'utf8');
    const timelineScrollIndex = source.indexOf('class="timeline-scroll"');
    const warningIndex = source.indexOf('class="cyber-policy-alert"');

    expect(source).toContain('v-if="showCyberPolicyWarning"');
    expect(source).toContain("t('webSession.cyberPolicyFlagged')");
    expect(warningIndex).toBeGreaterThan(timelineScrollIndex);
  });

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

  it('renders the DEV switch without mutating the persisted session flag', () => {
    const source = readFileSync(panelPath, 'utf8');

    expect(source).toContain('v-if="webSessionDevMode"');
    expect(source).toContain('v-model:value="devCyberPolicyWarning"');
    expect(source).toContain("const devCyberPolicySessionId = ref('')");
    expect(source).toContain("'has-cyber-policy-warning': showCyberPolicyWarning");
    expect(source).not.toContain('currentRealSession.cyberPolicyFlagged =');
  });

  it('allows users to dismiss the warning without clearing the server flag', () => {
    const source = readFileSync(panelPath, 'utf8');

    expect(source).toContain('closable');
    expect(source).toContain('@close="dismissCyberPolicyWarning"');
    expect(source).toContain('CYBER_POLICY_DISMISSALS_STORAGE_KEY');
    expect(source).not.toContain('cyberPolicyFlagged = false');
  });

  it('uses a flat warning background without color mixing', () => {
    const source = readFileSync(panelPath, 'utf8');
    const themeStart = source.indexOf('const cyberPolicyAlertThemeOverrides');
    const themeEnd = source.indexOf('function dismissCyberPolicyWarning', themeStart);
    const themeSource = source.slice(themeStart, themeEnd);

    expect(themeSource).toContain("colorWarning: dark ? '#332b1f' : '#fff1d6'");
    expect(themeSource).not.toContain('color-mix');
  });

  it('maps the compact websocket flag into the session summary', () => {
    const source = readFileSync(storePath, 'utf8');

    expect(source).toContain('cpf?: boolean;');
    expect(source).toContain('cyberPolicyFlagged: session.cpf === true');
  });
});
