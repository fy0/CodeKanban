import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useAppearanceStore, APPEARANCE_STORAGE_KEY, MAX_APPEARANCE_BYTES } from '../appearance';
import { useSettingsStore } from '../settings';
import { defaultAppearance, resolveTheme } from '@/themes/core';
import { getRegisteredThemes } from '@/themes/registry';
import { buildImportableSettingsBackup, parseSettingsBackupJSON, type SettingsBackupFile } from '@/utils/settingsBackup';

function storage() {
  const data = new Map<string, string>();
  return {
    getItem: vi.fn((key: string) => data.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => { data.set(key, String(value)); }),
    removeItem: vi.fn((key: string) => { data.delete(key); }),
    clear: vi.fn(() => data.clear()),
  };
}
const custom = (id = 'test-theme', parent = 'studio') => JSON.stringify({ schemaVersion: 1, id, name: 'Test theme', extends: parent, defaultMode: 'dark', colors: { dark: { accent: '#ABCDEF' } } });

describe('appearance state and legacy integration', () => {
  let local: ReturnType<typeof storage>;
  beforeEach(() => {
    setActivePinia(createPinia()); local = storage();
    vi.stubGlobal('localStorage', local);
    vi.stubGlobal('window', { localStorage: local, matchMedia: vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })) });
  });
  afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

  it('defaults to Classic without writing or migrating legacy settings', () => {
    local.setItem('general_settings', 'original'); local.setItem.mockClear();
    const store = useAppearanceStore();
    expect(store.themeId).toBe('classic'); expect(local.getItem('general_settings')).toBe('original');
    expect(local.setItem).not.toHaveBeenCalled();
  });
  it('recovers from corrupt storage without overwriting the bad payload', () => {
    local.setItem(APPEARANCE_STORAGE_KEY, '{broken');
    const store = useAppearanceStore();
    expect(store.themeId).toBe('classic'); expect(store.notice).not.toBe('');
    expect(local.getItem(APPEARANCE_STORAGE_KEY)).toBe('{broken');
  });
  it('persists selection and restores it in a fresh Pinia', () => {
    const store = useAppearanceStore(); store.selectTheme('forge'); store.updateLayout({ radius: 10 });
    setActivePinia(createPinia()); const restored = useAppearanceStore();
    expect(restored.themeId).toBe('forge'); expect(restored.mode).toBe('dark'); expect(restored.resolved.layout.radius).toBe(10);
  });
  it('keeps per-theme and per-variant overrides independent', () => {
    const store = useAppearanceStore(); store.selectTheme('studio');
    store.updateColor('light', 'accent', '#123456'); store.updateColor('dark', 'accent', '#654321'); store.updateLayout({ radius: 9 });
    store.selectTheme('tide'); expect(store.resolved.palette.accent).not.toBe('#123456');
    store.selectTheme('studio'); expect(store.resolved.palette.accent).toBe('#123456'); expect(store.resolved.layout.radius).toBe(9);
    store.setMode('dark'); expect(store.resolved.palette.accent).toBe('#654321');
  });
  it('follows the operating system only in system mode', () => {
    const store = useAppearanceStore(); store.selectTheme('studio'); store.setMode('system');
    store.setSystemDark(true); expect(store.scheme).toBe('dark'); store.setSystemDark(false); expect(store.scheme).toBe('light');
    store.setMode('dark'); store.setSystemDark(false); expect(store.scheme).toBe('dark');
  });
  it('validates imports without applying them until explicit commit', () => {
    const store = useAppearanceStore(); const before = store.snapshot();
    const preview = store.previewImport(custom()); expect(preview.themeId).toBe('test-theme'); expect(store.snapshot()).toEqual(before);
    store.importTheme(custom()); expect(store.themeId).toBe('test-theme'); expect(store.resolved.palette.accent).toBe('#ABCDEF');
  });
  it('requires explicit replacement and rejects built-in IDs', () => {
    const store = useAppearanceStore(); store.importTheme(custom()); store.updateLayout({ radius: 20 });
    expect(() => store.importTheme(custom())).toThrow();
    store.importTheme(custom(), true);
    expect(store.resolved.layout.radius).toBe(resolveTheme('studio', getRegisteredThemes(), 'dark').layout.radius);
    expect(store.snapshot().overrides['test-theme']).toBeUndefined();
    expect(() => store.importTheme(custom('studio'), true)).toThrow();
  });
  it('rejects invalid imports atomically', () => {
    const store = useAppearanceStore(); store.selectTheme('forge'); const before = store.snapshot();
    expect(() => store.importTheme(custom('bad-parent', 'missing'))).toThrow(); expect(store.snapshot()).toEqual(before);
    expect(() => store.commit({ ...before, mode: 'unknown' })).toThrow(); expect(store.snapshot()).toEqual(before);
  });
  it('exports a self-contained theme including both variants and overrides', () => {
    const store = useAppearanceStore(); store.importTheme(custom()); store.updateLayout({ radius: 7 });
    const exported = store.exportTheme('portable-theme'); expect(exported.extends).toBeUndefined();
    expect(resolveTheme(exported.id, [exported], 'dark').palette.accent).toBe('#ABCDEF');
    expect(resolveTheme(exported.id, [exported], 'light').layout.radius).toBe(7);
    store.importTheme(JSON.stringify(exported)); expect(store.themeId).toBe('portable-theme');
  });
  it('refuses to remove a parent theme with dependent children', () => {
    const store = useAppearanceStore(); store.importTheme(custom()); store.importTheme(custom('child-theme', 'test-theme'));
    expect(() => store.removeTheme('test-theme')).toThrow(); store.removeTheme('child-theme'); expect(store.themeId).toBe('test-theme');
    store.removeTheme('test-theme'); expect(store.themeId).toBe('studio');
    expect(() => store.removeTheme('classic')).toThrow();
  });
  it('reset removes only current overrides', () => {
    const store = useAppearanceStore(); store.selectTheme('studio'); store.updateLayout({ radius: 20 });
    store.selectTheme('tide'); store.updateLayout({ radius: 9 });
    expect(store.resolved.layout.radius).toBe(9);
    store.resetOverrides();
    expect(store.resolved.layout.radius).toBe(resolveTheme('tide', getRegisteredThemes(), 'light').layout.radius);
    expect(store.snapshot().overrides.tide).toBeUndefined();
    store.selectTheme('studio'); expect(store.resolved.layout.radius).toBe(20);
  });
  it('keeps an applied in-memory theme and warns on storage quota failure', () => {
    const store = useAppearanceStore(); local.setItem.mockImplementation(() => { throw new Error('quota'); });
    store.selectTheme('tide'); expect(store.themeId).toBe('tide'); expect(store.notice).not.toBe('');
  });
  it('synchronizes valid storage events without an echo write, ignores invalid events', () => {
    const store = useAppearanceStore(); local.setItem.mockClear();
    store.receiveStorage({ key: APPEARANCE_STORAGE_KEY, newValue: JSON.stringify({ ...defaultAppearance(), themeId: 'forge' }) });
    expect(store.themeId).toBe('forge'); expect(local.setItem).not.toHaveBeenCalled();
    store.receiveStorage({ key: APPEARANCE_STORAGE_KEY, newValue: '{broken' }); expect(store.themeId).toBe('forge');
    store.receiveStorage({ key: 'other', newValue: null }); expect(store.themeId).toBe('forge');
    store.receiveStorage({ key: null, newValue: null }); expect(store.themeId).toBe('classic');
  });
  it('bounds complete preference bundle size', () => {
    expect(() => useAppearanceStore().parseBundle(' '.repeat(MAX_APPEARANCE_BYTES + 1))).toThrow();
  });
  it('returns to the unchanged legacy palette after visiting new interfaces', () => {
    const settings = useSettingsStore(); const store = useAppearanceStore(); settings.selectPreset('dark');
    const original = JSON.stringify(settings.activeTheme); store.selectTheme('studio');
    expect(settings.activeTheme.bodyColor).toBe(resolveTheme('studio', getRegisteredThemes(), 'light').palette.canvas); store.selectTheme('classic'); expect(JSON.stringify(settings.activeTheme)).toBe(original);
    settings.applyCustomTheme({ primaryColor: '#123456' }); store.selectTheme('forge'); store.selectTheme('classic');
    expect(settings.activeTheme.primaryColor).toBe('#123456');
  });
  it('keeps explicit terminal themes independent and maps following terminals', () => {
    const settings = useSettingsStore(); const store = useAppearanceStore(); settings.updateTerminalTheme('github-light');
    const terminal = JSON.stringify(settings.activeTerminalTheme); store.selectTheme('forge'); expect(JSON.stringify(settings.activeTerminalTheme)).toBe(terminal);
    settings.updateTerminalTheme('follow-theme'); expect(settings.activeTerminalTheme.background).toBe(resolveTheme('forge', getRegisteredThemes(), 'dark').palette.terminalBg);
    store.updateColor('dark', 'terminalBg', '#123456'); expect(settings.activeTerminalTheme.background).toBe('#123456');
  });
  it('manual old preset selection deliberately returns to Classic', () => {
    const store = useAppearanceStore(); const settings = useSettingsStore(); store.selectTheme('tide'); settings.selectPreset('dark'); expect(store.themeId).toBe('classic');
  });
  it('round trips appearance in client settings backups and accepts old backups', () => {
    const settings = useSettingsStore(); const store = useAppearanceStore(); store.selectTheme('tide');
    const backup = settings.exportClientBackup('zh-CN'); store.selectTheme('forge'); settings.importClientBackup(backup); expect(store.themeId).toBe('tide');
    settings.importClientBackup({ locale: 'zh-CN' }); expect(store.themeId).toBe('tide');
  });
  it('excludes appearance when the client settings section is unchecked', () => {
    const source: SettingsBackupFile = { backupSchemaVersion: 2, backupKind: 'settings', sourceApp: { name: 'CodeKanban', version: '1', channel: 'test' }, payload: { client: useSettingsStore().exportClientBackup('zh-CN') } };
    const filtered = buildImportableSettingsBackup(source, ['client.locale']); expect(filtered.payload.client?.appearance).toBeUndefined();
    expect(buildImportableSettingsBackup(source, ['client.settings']).payload.client?.appearance).toBeDefined();
    expect(source.payload.client?.appearance).toBeDefined();
  });
  it('validates appearance before a general backup reaches server import', () => {
    const source = { payload: { client: { appearance: { ...defaultAppearance(), themeId: 'missing' } } } };
    expect(() => parseSettingsBackupJSON(JSON.stringify(source))).toThrow();
    expect(getRegisteredThemes().length).toBeGreaterThanOrEqual(4);
  });
});
