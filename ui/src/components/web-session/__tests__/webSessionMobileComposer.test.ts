import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionComposerEditorPath = fileURLToPath(
  new URL('../WebSessionComposerEditor.vue', import.meta.url)
);
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const webSessionComposerEditorSource = readFileSync(webSessionComposerEditorPath, 'utf8');

describe('webSession mobile composer', () => {
  it('starts the mobile composer editor at one row', () => {
    expect(webSessionPanelSource).toMatch(
      /const composerMinRows = computed\(\(\) => \(isMobile\.value \? 1 : 3\)\);/
    );
  });

  it('lets the editor row count control mobile input height', () => {
    expect(webSessionPanelSource).not.toMatch(
      /\.composer-input-shell\.is-mobile\s*\{[^}]*min-height:/s
    );
  });

  it('uses compact editor chrome on mobile only', () => {
    expect(webSessionPanelSource).toMatch(/:compact="isMobile"/);
    expect(webSessionComposerEditorSource).toMatch(
      /'--composer-editor-extra-height': props\.compact \? '24px' : '28px'/
    );
    expect(webSessionComposerEditorSource).toMatch(
      /'--composer-editor-padding-top': props\.compact \? '8px' : '10px'/
    );
    expect(webSessionComposerEditorSource).toMatch(
      /'--composer-editor-padding-bottom': props\.compact \? '8px' : '12px'/
    );
  });

  it('uses compact mobile composer controls', () => {
    expect(webSessionPanelSource).toMatch(/'is-mobile': isMobile/);
    expect(webSessionPanelSource).toMatch(/\.composer\.is-mobile\s*\{[^}]*padding:\s*6px 8px;/s);
    expect(webSessionPanelSource).toMatch(
      /\.composer\.is-mobile \.composer-icon-btn-mobile\s*\{[^}]*width:\s*36px;[^}]*height:\s*36px;/s
    );
  });

  it('moves session toggles behind a settings gear', () => {
    expect(webSessionPanelSource).toContain('<SettingsOutline />');
    expect(webSessionPanelSource).toContain('composer-settings-popover-card');
    expect(webSessionPanelSource).toContain('webSessionActiveCallTimeoutEnabledValue');
    expect(webSessionPanelSource).not.toContain('composer-auto-continue');
  });
});
