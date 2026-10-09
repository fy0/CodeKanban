import * as core from '../core';

export interface ContractCase { name: string; run: () => void }
/** Shared executable contracts: run with Vitest or plain Node after TypeScript compilation. */
export function themeContractCases(input: unknown[]): ContractCase[] {
  const builtin = input.map(core.validateManifest);
  const cases: ContractCase[] = [];
  const test = (name: string, run: () => void) => cases.push({ name, run });
  const equal = (a: unknown, b: unknown) => { if (JSON.stringify(a) !== JSON.stringify(b)) throw new Error(`Expected ${JSON.stringify(b)}, received ${JSON.stringify(a)}`); };
  const ok = (value: unknown) => { if (!value) throw new Error('Assertion failed'); };
  const rejects = (fn: () => unknown) => { let threw = false; try { fn(); } catch { threw = true; } ok(threw); };
  const child = (patch: Record<string, unknown> = {}) => ({ schemaVersion: 1, id: 'my-theme', name: 'My theme', extends: 'studio', ...patch });
  const state = (patch: Record<string, unknown> = {}) => ({ ...core.defaultAppearance(), ...patch });

  test('generated terminal colors have readable contrast in all eight palettes', () => {
    for (const theme of builtin) for (const mode of ['light', 'dark'] as const) {
      const p = core.resolveTheme(theme.id, builtin, mode).palette;
      for (const key of ['accent', 'success', 'warning', 'error', 'info', 'plan', 'working', 'muted'] as const) {
        ok(core.contrastRatio(core.ensureContrast(p[key], p.terminalBg), p.terminalBg) >= 4.5);
      }
    }
  });
  test('terminal contrast correction is stable and validates inputs', () => {
    equal(core.ensureContrast('#FFFFFF', '#000000'), '#FFFFFF');
    equal(core.ensureContrast('#000000', '#FFFFFF'), '#000000');
    rejects(() => core.ensureContrast('red', '#FFFFFF'));
    rejects(() => core.ensureContrast('#112233', '#FFFFFF', Infinity));
    rejects(() => core.ensureContrast('#112233', '#FFFFFF', 22));
  });
  test('built-in designs preserve navigation and use their own visual tokens', () => {
    equal(builtin.map(t => t.id), ['classic', 'studio', 'forge', 'tide']);
    ok(builtin.every(t => t.layout?.navigation === 'original'));
    equal(builtin.slice(1).map(t => t.layout?.material), ['solid', 'solid', 'solid']);
    equal(builtin.slice(1).map(t => t.layout?.texture), ['none', 'grid', 'none']);
    equal(builtin.slice(1).map(t => t.layout?.headingFont), ['sans', 'mono', 'sans']);
  });
  test('legacy v1 themes inherit material defaults without migration', () => {
    const t = core.resolveTheme('classic', builtin, 'light');
    equal(t.layout.material, 'solid'); equal(t.layout.borderWidth, 1);
  });
  test('material, texture and typography are inherited and independently overridable', () => {
    const t = core.resolveTheme('studio', builtin, 'light', { layout: { material: 'paper', texture: 'ruled', headingFont: 'serif', borderWidth: 1.5 } });
    equal(t.layout.material, 'paper'); equal(t.layout.headingFont, 'serif'); equal(t.layout.borderWidth, 1.5);
    equal(t.design, 'studio');
  });
  test('reject CSS injection in new material fields', () => {
    for (const key of ['material', 'texture', 'headingFont']) rejects(() => core.validateLayout({ [key]: 'url(https://example.org)' }));
    rejects(() => core.validateLayout({ borderWidth: 3 }));
  });
  for (const theme of builtin) for (const scheme of ['light', 'dark'] as const) {
    test(`${theme.id}/${scheme}: complete semantic palette`, () => {
      const value = core.resolveTheme(theme.id, builtin, scheme);
      equal(Object.keys(value.palette).length, core.COLOR_KEYS.length);
      for (const key of core.COLOR_KEYS) ok(/^#[0-9a-f]{6}$/i.test(value.palette[key]));
    });
    if (theme.id !== 'classic') test(`${theme.id}/${scheme}: normal and secondary text contrast >= 4.5`, () => {
      const p = core.resolveTheme(theme.id, builtin, scheme).palette;
      ok(core.contrastRatio(p.text, p.surface) >= 4.5);
      ok(core.contrastRatio(p.muted, p.surface) >= 4.5);
    });
  }
  test('inheritance merges parent, child, then user override without mutating manifests', () => {
    const custom = core.validateManifest(child({ layout: { radius: 3 }, colors: { light: { accent: '#123456' } } }));
    const before = JSON.stringify(builtin);
    const resolved = core.resolveTheme(custom.id, [...builtin, custom], 'light', { layout: { radius: 9 }, colors: { light: { accent: '#654321' } } });
    equal(resolved.palette.accent, '#654321'); equal(resolved.layout.radius, 9); equal(resolved.design, 'studio');
    equal(JSON.stringify(builtin), before);
    equal(core.resolveTheme(custom.id, [...builtin, custom], 'dark').palette.accent, builtin[1]!.colors!.dark!.accent);
  });
  test('missing inheritance parent is rejected', () => rejects(() => core.resolveTheme('my-theme', [...builtin, core.validateManifest(child({ extends: 'missing' }))], 'light')));
  test('self inheritance is rejected', () => rejects(() => core.validateManifest(child({ extends: 'my-theme' }))));
  test('multi-node inheritance cycle is rejected', () => {
    const a = core.validateManifest(child({ id: 'aa', extends: 'bb' }));
    const b = core.validateManifest(child({ id: 'bb', extends: 'aa' }));
    rejects(() => core.resolveTheme('aa', [...builtin, a, b], 'light'));
  });
  test('inheritance traversal is bounded', () => {
    const chain = Array.from({ length: 13 }, (_, i) => core.validateManifest(child({ id: `node-${i}`, extends: i === 0 ? 'studio' : `node-${i - 1}` })));
    rejects(() => core.resolveTheme('node-12', [...builtin, ...chain], 'light'));
  });
  test('root themes must contain every semantic color', () => rejects(() => core.resolveTheme('my-theme', [core.validateManifest(child({ extends: undefined }))], 'light')));
  test('missing English name falls back to display name', () => equal(core.validateManifest(child()).nameEn, 'My theme'));
  test('lowercase colors normalize to uppercase', () => equal(core.validateManifest(child({ colors: { light: { accent: '#aabbcc' } } })).colors?.light?.accent, '#AABBCC'));
  for (const invalid of ['red', '#123', '#12345678', 'rgb(0,0,0)', 'url(https://example.org/a)', 'var(--x)', '#fff;display:none', 'transparent', 42, null]) {
    test(`reject unsafe or unsupported color ${String(invalid)}`, () => rejects(() => core.validateManifest(child({ colors: { light: { accent: invalid } } }))));
  }
  for (const invalid of ['../theme', 'MY-THEME', '__proto__', 'a', '', 'a'.repeat(49)]) {
    test(`reject invalid identifier ${JSON.stringify(invalid)}`, () => rejects(() => core.validateManifest(child({ id: invalid }))));
  }
  test('accept plugin design namespaces but reject selector injection', () => {
    equal(core.validateManifest(child({ design: 'my-forest' })).design, 'my-forest');
    rejects(() => core.validateManifest(child({ design: 'x] body' })));
  });
  test('reject unsupported schema version', () => rejects(() => core.validateManifest(child({ schemaVersion: 2 }))));
  test('reject unknown root fields (including scripts and CSS)', () => {
    for (const key of ['script', 'css', 'url', 'assets']) rejects(() => core.validateManifest(child({ [key]: 'injected' })));
  });
  test('reject prototype pollution keys at every data layer', () => {
    rejects(() => core.validateManifest(JSON.parse('{"schemaVersion":1,"id":"my-theme","name":"X","__proto__":{}}')));
    rejects(() => core.validateLayout(JSON.parse('{"__proto__":{}}')));
    rejects(() => core.validateColors(JSON.parse('{"light":{"__proto__":{}}}')));
  });
  test('reject non-plain object prototypes', () => rejects(() => core.validateManifest(Object.assign(Object.create({ hidden: true }), child()))));
  test('reject long and control-character metadata', () => {
    rejects(() => core.validateManifest(child({ name: 'x'.repeat(81) })));
    rejects(() => core.validateManifest(child({ description: 'x'.repeat(301) })));
    rejects(() => core.validateManifest(child({ name: 'bad\u0000name' })));
  });
  const ranges: [string, number, number][] = [['radius', 0, 24], ['spacing', 8, 32], ['fontSize', 12, 20], ['lineHeight', 1.3, 2], ['sidebarWidth', 180, 300], ['contentWidth', 640, 1600]];
  for (const [key, min, max] of ranges) {
    test(`${key}: accepts inclusive bounds, rejects out of range and non-finite values`, () => {
      equal(core.validateLayout({ [key]: min }), { [key]: min }); equal(core.validateLayout({ [key]: max }), { [key]: max });
      for (const invalid of [min - 1, max + 1, NaN, Infinity, '14', null]) rejects(() => core.validateLayout({ [key]: invalid }));
    });
  }
  test('reject arbitrary font, navigation, density and motion values', () => {
    for (const [key, value] of [['font', 'url(font.woff)'], ['navigation', 'remote'], ['density', 'tiny'], ['motion', 'script'], ['panelStyle', 'glass']]) rejects(() => core.validateLayout({ [key!]: value }));
  });
  test('JSON theme file enforces its byte limit', () => rejects(() => core.parseThemeJson(JSON.stringify(child()) + ' '.repeat(core.MAX_THEME_BYTES))));
  test('malformed JSON is rejected', () => rejects(() => core.parseThemeJson('{invalid}')));
  test('default appearance is valid and does not add custom preferences', () => equal(core.validateState(core.defaultAppearance(), builtin), core.defaultAppearance()));
  test('reject reserved built-in IDs', () => rejects(() => core.validateState(state({ customThemes: [child({ id: 'studio' })] }), builtin)));
  test('reject duplicate imported IDs', () => rejects(() => core.validateState(state({ customThemes: [child(), child()] }), builtin)));
  test('reject more than 24 custom themes', () => rejects(() => core.validateState(state({ customThemes: Array.from({ length: 25 }, (_, i) => child({ id: `custom-${i}` })) }), builtin)));
  test('validate both light and dark variants of imported themes', () => rejects(() => core.validateState(state({ customThemes: [child({ extends: undefined, colors: { light: builtin[1]!.colors!.light } })] }), builtin)));
  test('reject unknown selected theme and unknown override target', () => {
    rejects(() => core.validateState(state({ themeId: 'missing' }), builtin));
    rejects(() => core.validateState(state({ overrides: { missing: {} } }), builtin));
  });
  test('reject malformed state and unknown modes', () => {
    for (const data of [null, [], state({ version: 9 }), state({ mode: 'dusk' }), state({ customThemes: {} }), state({ overrides: [] })]) rejects(() => core.validateState(data, builtin));
  });
  test('state round trips with independent variant overrides', () => {
    const value = core.validateState(state({ themeId: 'studio', overrides: { studio: { layout: { radius: 8 }, colors: { light: { accent: '#123456' }, dark: { accent: '#654321' } } } } }), builtin);
    equal(core.validateState(JSON.parse(JSON.stringify(value)), builtin), value);
  });
  test('contrast reference endpoints are 21:1 and 1:1', () => {
    equal(core.contrastRatio('#000000', '#FFFFFF'), 21); equal(core.contrastRatio('#123456', '#123456'), 1);
    equal(core.contrastRatio('#000000', '#FFFFFF'), core.contrastRatio('#FFFFFF', '#000000'));
  });
  test('100 deterministic valid layout overrides resolve without changing palettes', () => {
    for (let i = 0; i < 100; i++) {
      const layout = { radius: i % 25, fontSize: 12 + i % 9, sidebarWidth: 180 + i % 121 };
      const resolved = core.resolveTheme('studio', builtin, 'light', { layout });
      equal(resolved.layout.radius, layout.radius); equal(resolved.palette, builtin[1]!.colors!.light);
    }
  });
  return cases;
}
