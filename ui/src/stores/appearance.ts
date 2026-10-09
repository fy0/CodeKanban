import { computed, readonly, ref } from 'vue';
import { defineStore } from 'pinia';
import {
  defaultAppearance,
  parseThemeJson,
  resolveTheme,
  validateState,
  validateLayout,
  validateColors,
  validateManifest,
  type AppearanceState,
  type ColorKey,
  type LayoutTokens,
  type Mode,
  type Scheme,
  type ThemeManifest,
} from '@/themes/core';
import { getRegisteredThemes, sealAppearanceRegistry } from '@/themes/registry';

export const APPEARANCE_STORAGE_KEY = 'codekanban.appearance.v1';
export const MAX_APPEARANCE_BYTES = 2 * 1024 * 1024;
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

export const useAppearanceStore = defineStore('appearance', () => {
  sealAppearanceRegistry();
  const state = ref<AppearanceState>(defaultAppearance());
  const notice = ref('');
  const systemDark = ref(
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
      : false
  );
  function parseBundle(text: string): AppearanceState {
    if (new TextEncoder().encode(text).length > MAX_APPEARANCE_BYTES)
      throw new Error('Appearance bundle exceeds 2 MiB');
    return validateState(JSON.parse(text), getRegisteredThemes());
  }
  if (typeof window !== 'undefined') {
    try {
      const saved = window.localStorage.getItem(APPEARANCE_STORAGE_KEY);
      if (saved) state.value = parseBundle(saved);
    } catch {
      notice.value =
        '外观配置无法读取，已使用经典界面；原始设置未被修改。 / Could not load appearance preferences.';
    }
  }
  const themes = computed(() => [...getRegisteredThemes(), ...state.value.customThemes]);
  const themeId = computed(() => state.value.themeId);
  const mode = computed(() => state.value.mode);
  const scheme = computed<Scheme>(() =>
    mode.value === 'system' ? (systemDark.value ? 'dark' : 'light') : mode.value
  );
  const selected = computed(() => themes.value.find(t => t.id === themeId.value)!);
  const resolved = computed(() =>
    resolveTheme(themeId.value, themes.value, scheme.value, state.value.overrides[themeId.value])
  );
  const isClassic = computed(() => themeId.value === 'classic');
  const customized = computed(
    () => !isClassic.value || !!Object.keys(state.value.overrides.classic?.layout ?? {}).length
  );
  function snapshot(): AppearanceState {
    return copy(state.value);
  }
  function commit(input: unknown): void {
    const next = validateState(input, getRegisteredThemes());
    state.value = next;
    notice.value = '';
    if (typeof window === 'undefined') return;
    try {
      window.localStorage.setItem(APPEARANCE_STORAGE_KEY, JSON.stringify(next));
    } catch {
      notice.value =
        '已应用，但浏览器无法保存；刷新后可能丢失。 / Applied, but browser storage is unavailable.';
    }
  }
  function selectTheme(id: string): void {
    const theme = themes.value.find(t => t.id === id);
    if (!theme) throw new Error(`Unknown theme: ${id}`);
    if (id === themeId.value) return;
    commit({ ...snapshot(), themeId: id, mode: theme.defaultMode ?? 'system' });
  }
  function setMode(value: Mode): void {
    commit({ ...snapshot(), mode: value });
  }
  function setSystemDark(value: boolean): void {
    systemDark.value = value;
  }
  function updateLayout(values: Partial<LayoutTokens>): void {
    const patch = validateLayout(values);
    const next = snapshot();
    const current = next.overrides[next.themeId] ?? {};
    next.overrides[next.themeId] = { ...current, layout: { ...current.layout, ...patch } };
    commit(next);
  }
  function updateColor(variant: Scheme, key: ColorKey, value: string): void {
    const patch = validateColors({ [variant]: { [key]: value } });
    const next = snapshot();
    const current = next.overrides[next.themeId] ?? {};
    next.overrides[next.themeId] = {
      ...current,
      colors: {
        ...current.colors,
        [variant]: { ...current.colors?.[variant], ...patch?.[variant] },
      },
    };
    commit(next);
  }
  function resetOverrides(): void {
    const next = snapshot();
    delete next.overrides[next.themeId];
    commit(next);
  }
  function previewImport(text: string, replace = false): AppearanceState {
    const theme = parseThemeJson(text);
    if (getRegisteredThemes().some(t => t.id === theme.id))
      throw new Error('Built-in and plugin theme IDs are reserved; choose a new id');
    const next = snapshot();
    const index = next.customThemes.findIndex(t => t.id === theme.id);
    if (index >= 0 && !replace)
      throw new Error(
        'A theme with this id exists. Enable explicit replacement or choose a new id'
      );
    if (index >= 0) next.customThemes[index] = theme;
    else next.customThemes.push(theme);
    // An explicit replacement starts from the imported definition, not stale local overrides.
    delete next.overrides[theme.id];
    next.themeId = theme.id;
    next.mode = theme.defaultMode ?? 'system';
    return validateState(next, getRegisteredThemes());
  }
  function importTheme(text: string, replace = false): void {
    commit(previewImport(text, replace));
  }
  function removeTheme(id: string): void {
    const next = snapshot();
    const theme = next.customThemes.find(t => t.id === id);
    if (!theme) throw new Error('Only imported themes can be removed');
    if (next.customThemes.some(t => t.extends === id))
      throw new Error('Other themes inherit this theme; remove or reparent them first');
    next.customThemes = next.customThemes.filter(t => t.id !== id);
    delete next.overrides[id];
    if (next.themeId === id) {
      next.themeId = theme.extends ?? 'classic';
      next.mode = 'system';
    }
    commit(next);
  }
  /** Export a portable, flattened definition: no dependency on other custom themes. */
  function exportTheme(id = `custom-${themeId.value}`): ThemeManifest {
    const light = resolveTheme(
      themeId.value,
      themes.value,
      'light',
      state.value.overrides[themeId.value]
    );
    const dark = resolveTheme(
      themeId.value,
      themes.value,
      'dark',
      state.value.overrides[themeId.value]
    );
    return validateManifest({
      schemaVersion: 1,
      id: id.slice(0, 48),
      name: `${selected.value.name} · Custom`.slice(0, 80),
      nameEn: `${selected.value.nameEn} Custom`.slice(0, 80),
      design: light.design,
      defaultMode: mode.value,
      colors: { light: light.palette, dark: dark.palette },
      layout: light.layout,
    });
  }
  function receiveStorage(event: Pick<StorageEvent, 'key' | 'newValue'>): void {
    if (event.key !== APPEARANCE_STORAGE_KEY && event.key !== null) return;
    try {
      state.value = event.newValue ? parseBundle(event.newValue) : defaultAppearance();
      notice.value = '';
    } catch {
      notice.value =
        '另一窗口写入的主题配置无效，已保留当前界面。 / Ignored invalid preferences from another tab.';
    }
  }
  return {
    preferences: readonly(state),
    notice: readonly(notice),
    themes,
    themeId,
    selected,
    mode,
    scheme,
    resolved,
    isClassic,
    customized,
    snapshot,
    commit,
    parseBundle,
    selectTheme,
    setMode,
    setSystemDark,
    updateLayout,
    updateColor,
    resetOverrides,
    previewImport,
    importTheme,
    removeTheme,
    exportTheme,
    receiveStorage,
  };
});
