import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionPanelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const webSessionPanelSource = readFileSync(webSessionPanelPath, 'utf8');
const mobileSessionDrawerPath = fileURLToPath(
  new URL('../WebSessionMobileSessionDrawer.vue', import.meta.url)
);
const mobileSessionDrawerSource = readFileSync(mobileSessionDrawerPath, 'utf8');

describe('webSession mobile selector layout', () => {
  it('renders mobile session selection inside a bottom drawer', () => {
    expect(mobileSessionDrawerSource).toContain('<n-drawer');
    expect(mobileSessionDrawerSource).toContain('class="mobile-session-drawer"');
    expect(mobileSessionDrawerSource).toContain(':show="show"');
    expect(webSessionPanelSource).toContain(':show="showMobileTabSelector"');
    expect(webSessionPanelSource).toContain(
      '@update:show="handleMobileSessionSelectorVisibilityChange"'
    );
  });

  it('virtualizes mobile session rows instead of using the dropdown menu list', () => {
    expect(mobileSessionDrawerSource).toContain('class="mobile-session-drawer-list"');
    expect(mobileSessionDrawerSource).toContain(':items="items"');
    expect(webSessionPanelSource).toContain(':items="mobileTabDescriptors"');
    expect(mobileSessionDrawerSource).toContain(':item-size="MOBILE_TAB_VIRTUAL_ITEM_SIZE"');
    expect(mobileSessionDrawerSource).toContain('item-resizable');
  });

  it('shows desktop-equivalent date groups and activity times', () => {
    expect(mobileSessionDrawerSource).toContain("item.kind === 'date-group'");
    expect(webSessionPanelSource).toContain('mobileCurrentSessionGroups');
    expect(webSessionPanelSource).toContain('groupWebSessionItemsByDate');
    expect(webSessionPanelSource).toContain('timeLabel: formatWebSessionSidebarTime(');
    expect(webSessionPanelSource).toContain('timeTitle: formatWebSessionDateTime(');
  });

  it('keeps scope and new-session actions together in the fixed drawer footer', () => {
    expect(mobileSessionDrawerSource).toContain('<template #footer>');
    expect(mobileSessionDrawerSource).toContain('class="mobile-session-drawer-footer"');
    expect(mobileSessionDrawerSource).toContain('class="mobile-session-drawer-scope"');
    expect(mobileSessionDrawerSource).toContain('class="mobile-session-drawer-new-session"');
    expect(mobileSessionDrawerSource).not.toContain("item.kind === 'new-session'");
  });
});
