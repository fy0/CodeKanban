import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { THEME_PRESETS } from '@/constants/themes';
import { useAppearanceStore } from '@/stores/appearance';
import { useSettingsStore } from '@/stores/settings';
import { getRegisteredThemes } from '@/themes/registry';
import { resolveTheme } from '@/themes/core';
import {
  activateUnifiedThemeChoice,
  buildUnifiedThemeChoices,
  selectedUnifiedThemeKey,
} from '@/composables/useUnifiedThemeChoices';

describe('flat theme choices', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const storage = new Map<string, string>();
    const localStorage = {
      getItem: (key: string) => storage.get(key) || null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    };
    vi.stubGlobal('window', {
      localStorage,
      matchMedia: () => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    });
    vi.stubGlobal('localStorage', localStorage);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('lists the old color presets and every registered theme at one sorted level', () => {
    const themes = getRegisteredThemes();
    const choices = buildUnifiedThemeChoices(themes, THEME_PRESETS, {}, true);
    expect(choices.length).toBe(themes.length + THEME_PRESETS.length);
    expect(new Set(choices.map(choice => choice.key)).size).toBe(choices.length);
    expect(choices.some(choice => choice.key === 'theme:studio')).toBe(true);
    expect(choices.some(choice => choice.key === 'preset:dark')).toBe(true);
    expect(choices.map(choice => choice.label)).toEqual(
      choices.map(choice => choice.label).sort((a, b) => a.localeCompare(b, 'zh-CN'))
    );
    expect(choices.find(choice => choice.key === 'preset:dark')?.preview.canvas).toBe(
      THEME_PRESETS.find(preset => preset.id === 'dark')?.colors.bodyColor
    );
  });

  it('switches directly from a new theme to the same older preset last used in Classic', () => {
    const appearance = useAppearanceStore();
    const settings = useSettingsStore();
    settings.selectPreset('light');
    appearance.selectTheme('forge');
    const choices = buildUnifiedThemeChoices(appearance.themes, THEME_PRESETS, {}, false);
    activateUnifiedThemeChoice('preset:light', choices, appearance, settings);
    expect(appearance.themeId).toBe('classic');
    expect(settings.currentPresetId).toBe('light');
    expect(settings.customTheme).toBeNull();
    expect(
      selectedUnifiedThemeKey(appearance.themeId, settings.currentPresetId, false, false)
    ).toBe('preset:light');
  });

  it('selects Classic as a peer rather than simultaneously checking Classic and Dark', () => {
    const appearance = useAppearanceStore();
    const settings = useSettingsStore();
    settings.selectPreset('dark');
    const choices = buildUnifiedThemeChoices(appearance.themes, THEME_PRESETS, {}, false);
    activateUnifiedThemeChoice('theme:classic', choices, appearance, settings);
    expect(settings.customTheme).not.toBeNull();
    expect(settings.activeTheme.bodyColor).toBe(
      resolveTheme('classic', appearance.themes, 'light').palette.canvas
    );
    expect(selectedUnifiedThemeKey(appearance.themeId, settings.currentPresetId, true, false)).toBe(
      'theme:classic'
    );
    activateUnifiedThemeChoice('preset:dark', choices, appearance, settings);
    expect(settings.customTheme).toBeNull();
    expect(
      selectedUnifiedThemeKey(appearance.themeId, settings.currentPresetId, false, false)
    ).toBe('preset:dark');
  });

  it('preserves the active imported or registered theme as the sole selected choice', () => {
    expect(selectedUnifiedThemeKey('tide', 'light', true, true)).toBe('theme:tide');
    expect(selectedUnifiedThemeKey('classic', 'dark', false, true)).toBe('preset:dark');
    expect(selectedUnifiedThemeKey('classic', 'light', true, false)).toBe('theme:classic');
  });
});
