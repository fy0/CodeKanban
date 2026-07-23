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

  it('keeps the mobile summary limited to agent, model, and workflow', () => {
    const summarySource = webSessionPanelSource.match(
      /const mobileComposerSummaryTokens = computed[\s\S]*?(?=const tokenNumberFormatter)/
    )?.[0];

    expect(summarySource).toContain("key: 'agent'");
    expect(summarySource).toContain("key: 'model'");
    expect(summarySource).toContain("key: 'workflow'");
    expect(summarySource).not.toContain("key: 'reasoning'");
    expect(summarySource).not.toContain("key: 'permission'");
    expect(summarySource).not.toContain("key: 'auto-continue'");
    expect(summarySource).not.toContain("key: 'active-call-timeout'");
  });

  it('uses compact square single-line mobile summary chips', () => {
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-toggle-chip\s*\{[^}]*border-radius:\s*4px;[^}]*white-space:\s*nowrap;/s
    );
  });

  it('replaces the expanded summary strip with a compact button above the settings row', () => {
    expect(webSessionPanelSource).toContain(
      "'is-mobile-settings-expanded': isMobile && isMobileComposerSettingsExpanded"
    );
    expect(webSessionPanelSource).toContain(
      ':class="{ \'is-compact\': isMobileComposerSettingsExpanded }"'
    );
    expect(webSessionPanelSource).toMatch(
      /v-if="!isMobileComposerSettingsExpanded"\s+class="composer-mobile-toggle-copy"/
    );
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-toolbar\.is-settings-expanded\s*\{[^}]*position:\s*absolute;[^}]*right:\s*8px;[^}]*width:\s*auto;/s
    );
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-summary\.is-expanded\s*\{[^}]*flex:\s*0 0 36px;[^}]*width:\s*36px;/s
    );
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-toggle\.is-compact\s*\{[^}]*width:\s*36px;[^}]*height:\s*34px;[^}]*justify-content:\s*center;/s
    );
    expect(webSessionPanelSource).toMatch(
      /@media \(max-width:\s*359px\)[\s\S]*grid-template-columns:\s*38px minmax\(0, 96px\) minmax\(0, 72px\) minmax\(0, 1fr\);/
    );
  });

  it('keeps expanded mobile settings open when the editor gains focus', () => {
    const focusHandlerSource = webSessionPanelSource.match(
      /function handleComposerFocus\(\)[\s\S]*?(?=function handleComposerBlur)/
    )?.[0];

    expect(focusHandlerSource).toContain('ensureMobileComposerVisible()');
    expect(focusHandlerSource).not.toContain('isMobileComposerSettingsExpanded.value = false');
  });

  it('leaves intentional space after both mobile settings rows', () => {
    expect(webSessionPanelSource).toMatch(
      /grid-template-columns:\s*38px minmax\(0, 118px\) minmax\(0, 96px\) minmax\(0, 1fr\);/
    );
    expect(webSessionPanelSource).toMatch(
      /\.composer-config\.is-mobile \.composer-mode-row\s*\{[^}]*width:\s*auto;[^}]*justify-self:\s*start;/s
    );
    expect(webSessionPanelSource).toMatch(
      /\.composer-config\.is-mobile \.composer-mode-switch\s*\{[^}]*width:\s*112px;/s
    );
    expect(webSessionPanelSource).toMatch(
      /\.composer-config\.is-mobile \.permission-select\s*\{[^}]*width:\s*132px;/s
    );
  });

  it('uses an icon-only square composer collapse control', () => {
    expect(webSessionPanelSource).not.toContain('composer-mobile-panel-toggle-text');
    expect(webSessionPanelSource).not.toContain('composer-mobile-panel-toggle-shell');
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-panel-toggle\s*\{[^}]*width:\s*36px;[^}]*height:\s*34px;[^}]*border-radius:\s*8px;/s
    );

    const flatPanelToggleBlocks = [
      /\.composer-mobile-panel-toggle\s*\{[^}]*\}/s,
      /\.composer-mobile-panel-toggle\.is-collapsed\s*\{[^}]*\}/s,
      /\.composer-mobile-panel-toggle:hover,[\s\S]*?\.composer-mobile-panel-toggle:active\s*\{[^}]*\}/s,
    ];
    for (const blockPattern of flatPanelToggleBlocks) {
      expect(webSessionPanelSource.match(blockPattern)?.[0]).not.toContain('box-shadow');
    }
  });

  it('embeds both mobile disclosure controls in one toolbar', () => {
    expect(webSessionPanelSource).toContain('class="composer-mobile-toolbar"');
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-toolbar\s*\{[^}]*width:\s*100%;[^}]*display:\s*flex;[^}]*gap:\s*6px;/s
    );
    expect(webSessionPanelSource).toMatch(
      /class="composer-mobile-toolbar"[\s\S]*class="composer-mobile-summary"[\s\S]*class="composer-mobile-panel-toggle"/
    );
  });

  it('keeps lower composer content outside the inline toggle toolbar', () => {
    const toolbarStart = webSessionPanelSource.indexOf('class="composer-mobile-toolbar"');
    const composerContentStart = webSessionPanelSource.indexOf(
      '<template v-if="!isMobile || !isMobileComposerCollapsed">',
      toolbarStart
    );

    expect(toolbarStart).toBeGreaterThan(-1);
    expect(composerContentStart).toBeGreaterThan(toolbarStart);
    expect(webSessionPanelSource.slice(toolbarStart, composerContentStart)).not.toContain(
      'class="composer-config"'
    );
    expect(webSessionPanelSource.slice(composerContentStart)).toContain(
      'class="composer-input-shell"'
    );
  });

  it('reverses the square panel arrow while preserving the settings disclosure arrow', () => {
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-panel-toggle-arrow\s*\{[^}]*transform:\s*rotate\(0deg\);/s
    );
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-panel-toggle-arrow\.is-collapsed\s*\{[^}]*transform:\s*rotate\(180deg\);/s
    );
    expect(webSessionPanelSource).toMatch(
      /\.composer-mobile-toggle-arrow\.is-open\s*\{[^}]*transform:\s*rotate\(180deg\);/s
    );
  });
});
