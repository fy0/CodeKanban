import { computed } from 'vue';
import { THEME_PRESETS, type ThemePreset } from '@/constants/themes';
import { useAppearanceStore } from '@/stores/appearance';
import { useSettingsStore } from '@/stores/settings';
import { toLegacyTheme } from '@/themes/adapter';
import { resolveTheme, type ThemeManifest, type ThemeOverride } from '@/themes/core';
import { useLocale } from './useLocale';

export interface UnifiedThemeChoice {
  key: string;
  id: string;
  kind: 'theme' | 'preset';
  label: string;
  description: string;
  design: string;
  preview: {
    canvas: string;
    surface: string;
    border: string;
    accent: string;
    text: string;
  };
}

/** A single, name-sorted list: built-ins, plugins, imports and older palettes get equal treatment. */
export function buildUnifiedThemeChoices(
  themes: readonly ThemeManifest[],
  presets: readonly ThemePreset[],
  overrides: Readonly<Record<string, ThemeOverride>>,
  zh: boolean
): UnifiedThemeChoice[] {
  const manifestChoices: UnifiedThemeChoice[] = themes.map(theme => {
    const sample = resolveTheme(
      theme.id,
      themes,
      theme.defaultMode === 'dark' ? 'dark' : 'light',
      overrides[theme.id]
    );
    return {
      key: `theme:${theme.id}`,
      id: theme.id,
      kind: 'theme',
      label: zh ? theme.name : theme.nameEn,
      description: (zh ? theme.description : theme.descriptionEn) || '',
      design: theme.design || 'custom',
      preview: sample.palette,
    };
  });
  const presetChoices: UnifiedThemeChoice[] = presets.map(preset => ({
    key: `preset:${preset.id}`,
    id: preset.id,
    kind: 'preset',
    label: zh ? preset.name : preset.nameEn,
    description: '',
    design: 'classic',
    preview: {
      canvas: preset.colors.bodyColor,
      surface: preset.colors.surfaceColor,
      border: preset.colors.borderColor || preset.colors.terminalTabBg || preset.colors.bodyColor,
      accent: preset.colors.primaryColor,
      text: preset.colors.textColor || (preset.isDark ? '#FFFFFF' : '#222222'),
    },
  }));
  return [...manifestChoices, ...presetChoices].sort(
    (a, b) => a.label.localeCompare(b.label, zh ? 'zh-CN' : 'en') || a.key.localeCompare(b.key)
  );
}

/** Only one item is selected, even when older presets use the Classic rendering engine. */
export function selectedUnifiedThemeKey(
  themeId: string,
  presetId: string,
  hasCustomLegacyPalette: boolean,
  followsSystem: boolean
): string {
  if (themeId !== 'classic') return `theme:${themeId}`;
  return hasCustomLegacyPalette && !followsSystem ? 'theme:classic' : `preset:${presetId}`;
}

/** Resolve and apply one shared choice from either the menu or the settings page. */
export function activateUnifiedThemeChoice(
  key: string,
  choices: readonly UnifiedThemeChoice[],
  appearance: ReturnType<typeof useAppearanceStore>,
  settings: ReturnType<typeof useSettingsStore>
): void {
  const choice = choices.find(item => item.key === key);
  if (!choice) throw new Error(`Unknown theme: ${key}`);
  if (choice.kind === 'preset') {
    settings.selectPreset(choice.id);
  } else if (choice.id === 'classic') {
    // Classic remains a selectable theme; old palettes are independent peers in the list.
    // Apply its own palette via the legacy adapter instead of reverting to the last preset.
    const sample = resolveTheme(
      'classic',
      appearance.themes,
      'light',
      appearance.preferences.overrides.classic
    );
    settings.applyCustomTheme(toLegacyTheme(sample.palette));
  } else {
    appearance.selectTheme(choice.id);
  }
}

export function useUnifiedThemeChoices() {
  const appearance = useAppearanceStore();
  const settings = useSettingsStore();
  const { locale } = useLocale();
  const choices = computed(() =>
    buildUnifiedThemeChoices(
      appearance.themes,
      THEME_PRESETS,
      appearance.preferences.overrides,
      locale.value === 'zh-CN'
    )
  );
  const selectedKey = computed(() =>
    selectedUnifiedThemeKey(
      appearance.themeId,
      settings.currentPresetId,
      settings.customTheme !== null,
      settings.followSystemTheme
    )
  );

  function select(key: string): void {
    activateUnifiedThemeChoice(key, choices.value, appearance, settings);
  }

  return { choices, selectedKey, select };
}
