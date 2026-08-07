import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';

import { sanitizeSettingsSectionId, useSettingsUiStore } from '@/stores/settingsUi';

describe('settingsUi store', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('sanitizes invalid section ids to the default section', () => {
    expect(sanitizeSettingsSectionId('theme')).toBe('theme');
    expect(sanitizeSettingsSectionId('git')).toBe('git');
    expect(sanitizeSettingsSectionId('maintenance')).toBe('maintenance');
    expect(sanitizeSettingsSectionId('project-terminal')).toBe('project-workspace');
    expect(sanitizeSettingsSectionId('unknown-section')).toBe('project-workspace');
    expect(sanitizeSettingsSectionId(undefined)).toBe('project-workspace');
  });

  it('opens the overlay with section overrides and clears stale query by default', () => {
    const store = useSettingsUiStore();

    store.openSettings({ section: 'theme', query: 'font' });
    expect(store.isOpen).toBe(true);
    expect(store.activeSection).toBe('theme');
    expect(store.searchQuery).toBe('font');

    store.closeSettings();
    store.openSettings({ section: 'security' });
    expect(store.isOpen).toBe(true);
    expect(store.activeSection).toBe('security');
    expect(store.searchQuery).toBe('');
  });

  it('resets state to the default card and empty query', () => {
    const store = useSettingsUiStore();

    store.openSettings({ section: 'developer', query: 'timeout' });
    store.resetState();

    expect(store.activeSection).toBe('project-workspace');
    expect(store.searchQuery).toBe('');
  });
});
