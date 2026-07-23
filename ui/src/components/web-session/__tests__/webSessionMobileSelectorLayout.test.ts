import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');

describe('webSession mobile selector layout', () => {
  it('renders mobile session selection inside a bottom drawer', () => {
    expect(webSessionPanelSource).toContain('<n-drawer');
    expect(webSessionPanelSource).toContain('class="mobile-session-drawer"');
    expect(webSessionPanelSource).toContain(':show="showMobileTabSelector"');
    expect(webSessionPanelSource).toContain(
      '@update:show="handleMobileSessionSelectorVisibilityChange"'
    );
  });

  it('virtualizes mobile session rows instead of using the dropdown menu list', () => {
    expect(webSessionPanelSource).toContain('class="mobile-session-drawer-list"');
    expect(webSessionPanelSource).toContain(':items="mobileTabDescriptors"');
    expect(webSessionPanelSource).toContain(':item-size="MOBILE_TAB_VIRTUAL_ITEM_SIZE"');
    expect(webSessionPanelSource).toContain('item-resizable');
  });

  it('shows desktop-equivalent date groups and activity times', () => {
    expect(webSessionPanelSource).toContain("item.kind === 'date-group'");
    expect(webSessionPanelSource).toContain('mobileCurrentSessionGroups');
    expect(webSessionPanelSource).toContain('groupWebSessionItemsByDate');
    expect(webSessionPanelSource).toContain('getMobileTabSessionTimeLabel(item.session)');
    expect(webSessionPanelSource).toContain('getMobileTabSessionTimeTitle(item.session)');
  });

  it('keeps scope and new-session actions together in the fixed drawer footer', () => {
    expect(webSessionPanelSource).toContain('<template #footer>');
    expect(webSessionPanelSource).toContain('class="mobile-session-drawer-footer"');
    expect(webSessionPanelSource).toContain('class="mobile-session-drawer-scope"');
    expect(webSessionPanelSource).toContain('class="mobile-session-drawer-new-session"');
    expect(webSessionPanelSource).not.toContain("item.kind === 'new-session'");
  });
});
