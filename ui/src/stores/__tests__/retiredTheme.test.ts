import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { APPEARANCE_STORAGE_KEY, useAppearanceStore } from '../appearance';
import { defaultAppearance, validateState } from '@/themes/core';
import { getRegisteredThemes } from '@/themes/registry';

const legacy = () => ({
  ...defaultAppearance(),
  themeId: 'paper',
  mode: 'dark',
  overrides: { paper: { colors: { dark: { accent: '#AABBCC' } }, layout: { radius: 9 } } },
  customThemes: [
    { schemaVersion: 1, id: 'my-paper', name: 'My theme', extends: 'paper', design: 'paper' },
  ],
});

describe('retired Paper preferences', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const data = new Map([[APPEARANCE_STORAGE_KEY, JSON.stringify(legacy())]]);
    vi.stubGlobal('window', {
      localStorage: {
        getItem: (key: string) => data.get(key) ?? null,
        setItem: (key: string, value: string) => data.set(key, value),
      },
    });
  });
  afterEach(() => vi.unstubAllGlobals());

  it('restores the replacement without falling back or losing overrides', () => {
    const store = useAppearanceStore();
    expect(store.themeId).toBe('tide');
    expect(store.mode).toBe('dark');
    expect(store.notice).toBe('');
    expect(store.resolved.design).toBe('tide');
    expect(store.resolved.palette.accent).toBe('#AABBCC');
    expect(store.resolved.layout.radius).toBe(9);
    expect(store.themes.some(theme => theme.id === 'paper')).toBe(false);
    expect(store.snapshot().overrides.paper).toBeUndefined();
    store.setMode('light');
    setActivePinia(createPinia());
    expect(useAppearanceStore().themeId).toBe('tide');
  });

  it('keeps derived themes usable and exports the new design namespace', () => {
    const store = useAppearanceStore();
    store.selectTheme('my-paper');
    expect(store.selected.extends).toBe('tide');
    expect(store.resolved.design).toBe('tide');
    expect(store.exportTheme().design).toBe('tide');
    store.importTheme(
      JSON.stringify({
        schemaVersion: 1,
        id: 'old-export',
        name: 'Old export',
        extends: 'paper',
        design: 'paper',
      })
    );
    expect(store.resolved.design).toBe('tide');
    expect(store.selected.extends).toBe('tide');
  });

  it('migrates storage events and backup previews without changing the input', () => {
    const store = useAppearanceStore();
    store.selectTheme('studio');
    const input = JSON.stringify(legacy());
    const preview = store.parseBundle(input);
    expect(preview.themeId).toBe('tide');
    expect(store.themeId).toBe('studio');
    store.receiveStorage({ key: APPEARANCE_STORAGE_KEY, newValue: input });
    expect(store.themeId).toBe('tide');
    expect(store.notice).toBe('');
    expect(JSON.parse(input).themeId).toBe('paper');
  });

  it('prefers new overrides while keeping nonconflicting old fields', () => {
    const input = legacy();
    const state = validateState(
      {
        ...input,
        overrides: { tide: { colors: { dark: { accent: '#123456' } } }, ...input.overrides },
      },
      getRegisteredThemes()
    );
    expect(state.overrides.tide?.colors?.dark?.accent).toBe('#123456');
    expect(state.overrides.tide?.layout?.radius).toBe(9);
  });

  it('still rejects invalid legacy overrides atomically', () => {
    const store = useAppearanceStore();
    const before = store.snapshot();
    expect(() =>
      store.commit({
        ...legacy(),
        overrides: { paper: { colors: { dark: { accent: 'url(x)' } } } },
      })
    ).toThrow();
    expect(store.snapshot()).toEqual(before);
  });
});
