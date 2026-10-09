/** Data-only theme API. No DOM, Vue, network, CSS evaluation or user code execution. */
export const THEME_SCHEMA_VERSION = 1 as const;
export const MAX_THEME_BYTES = 64 * 1024;
export const MAX_CUSTOM_THEMES = 24;
export const COLOR_KEYS = [
  'canvas', 'surface', 'raised', 'sunken', 'text', 'muted', 'subtle', 'border',
  'accent', 'success', 'warning', 'error', 'info', 'plan', 'working', 'addition',
  'deletion', 'terminalBg', 'terminalFg',
] as const;
export type ColorKey = (typeof COLOR_KEYS)[number];
export type Palette = Record<ColorKey, string>;
export type Mode = 'light' | 'dark' | 'system';
export type Scheme = Exclude<Mode, 'system'>;
export type BuiltinDesign = 'classic' | 'studio' | 'forge' | 'tide';
/** Additional names are CSS namespaces supplied by trusted build-time plugins. */
export type Design = BuiltinDesign | (string & {});
export interface LayoutTokens {
  navigation: 'original' | 'sidebar' | 'rail' | 'top';
  radius: number;
  spacing: number;
  font: 'system' | 'humanist' | 'mono';
  fontSize: number;
  lineHeight: number;
  sidebarWidth: number;
  contentWidth: number;
  density: 'comfortable' | 'compact';
  panelStyle: 'flat' | 'outlined' | 'elevated';
  motion: 'full' | 'reduced';
  material: 'solid' | 'glass' | 'paper';
  texture: 'none' | 'grid' | 'ruled';
  headingFont: 'sans' | 'serif' | 'mono';
  borderWidth: number;
}
export interface ThemeManifest {
  schemaVersion: 1;
  id: string;
  name: string;
  nameEn: string;
  description?: string;
  descriptionEn?: string;
  extends?: string;
  design?: Design;
  defaultMode?: Mode;
  colors?: Partial<Record<Scheme, Partial<Palette>>>;
  layout?: Partial<LayoutTokens>;
}
export interface ThemeOverride {
  colors?: Partial<Record<Scheme, Partial<Palette>>>;
  layout?: Partial<LayoutTokens>;
}
export interface ResolvedTheme {
  id: string;
  design: Design;
  palette: Palette;
  layout: LayoutTokens;
}
export interface AppearanceState {
  version: 1;
  themeId: string;
  mode: Mode;
  customThemes: ThemeManifest[];
  overrides: Record<string, ThemeOverride>;
}
export const DEFAULT_LAYOUT: LayoutTokens = {
  navigation: 'original', radius: 6, spacing: 16, font: 'system', fontSize: 14,
  lineHeight: 1.65, sidebarWidth: 208, contentWidth: 1200, density: 'comfortable',
  panelStyle: 'flat', motion: 'full',
  material: 'solid', texture: 'none', headingFont: 'sans', borderWidth: 1,
};
export const FONT_STACKS: Record<LayoutTokens['font'], string> = {
  system: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans CJK SC", "PingFang SC", "Microsoft YaHei", sans-serif',
  humanist: 'Optima, Candara, "Noto Sans", "Noto Sans CJK SC", "PingFang SC", "Microsoft YaHei", sans-serif',
  mono: '"SFMono-Regular", Consolas, "Liberation Mono", "Noto Sans Mono CJK SC", "Microsoft YaHei", monospace',
};
const ID = /^[a-z][a-z0-9-]{1,47}$/;
const HEX = /^#[0-9a-f]{6}$/i;
const MODES = ['light', 'dark', 'system'];
const NUMBER_RANGES: Record<string, [number, number]> = {
  radius: [0, 24], spacing: [8, 32], fontSize: [12, 20], lineHeight: [1.3, 2],
  sidebarWidth: [180, 300], contentWidth: [640, 1600], borderWidth: [1, 2],
};
const ENUMS: Record<string, readonly string[]> = {
  navigation: ['original', 'sidebar', 'rail', 'top'], font: ['system', 'humanist', 'mono'],
  density: ['comfortable', 'compact'], panelStyle: ['flat', 'outlined', 'elevated'],
  motion: ['full', 'reduced'],
  material: ['solid', 'glass', 'paper'], texture: ['none', 'grid', 'ruled'],
  headingFont: ['sans', 'serif', 'mono'],
};
function record(value: unknown, label: string): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${label}: expected an object`);
  const proto = Object.getPrototypeOf(value);
  if (proto !== Object.prototype && proto !== null) throw new Error(`${label}: invalid prototype`);
  return value as Record<string, unknown>;
}
function keys(value: Record<string, unknown>, allowed: readonly string[], label: string) {
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) throw new Error(`${label}: unsupported field "${key}"`);
  }
}
function string(value: unknown, max: number, label: string): string {
  // oxlint-disable-next-line no-control-regex -- Reject control characters in imported metadata.
  if (typeof value !== 'string' || !value.trim() || value.length > max || /[\u0000-\u001f]/.test(value)) {
    throw new Error(`${label}: expected 1–${max} printable characters`);
  }
  return value.trim();
}
export function validateLayout(input: unknown): Partial<LayoutTokens> {
  const obj = record(input, 'layout');
  keys(obj, [...Object.keys(NUMBER_RANGES), ...Object.keys(ENUMS)], 'layout');
  const result: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(obj)) {
    const range = NUMBER_RANGES[key];
    if (range) {
      if (typeof value !== 'number' || !Number.isFinite(value) || value < range[0] || value > range[1]) {
        throw new Error(`layout.${key}: expected ${range[0]}–${range[1]}`);
      }
    } else if (typeof value !== 'string' || !ENUMS[key]?.includes(value)) {
      throw new Error(`layout.${key}: invalid option`);
    }
    result[key] = value;
  }
  return result as Partial<LayoutTokens>;
}
export function validateColors(input: unknown): ThemeOverride['colors'] {
  const obj = record(input, 'colors');
  keys(obj, ['light', 'dark'], 'colors');
  const result: NonNullable<ThemeOverride['colors']> = {};
  for (const scheme of ['light', 'dark'] as const) {
    if (obj[scheme] === undefined) continue;
    const colors = record(obj[scheme], `colors.${scheme}`);
    keys(colors, COLOR_KEYS, `colors.${scheme}`);
    result[scheme] = {};
    for (const key of COLOR_KEYS) {
      if (colors[key] === undefined) continue;
      if (typeof colors[key] !== 'string' || !HEX.test(colors[key] as string)) {
        throw new Error(`colors.${scheme}.${key}: expected #RRGGBB`);
      }
      result[scheme]![key] = (colors[key] as string).toUpperCase();
    }
  }
  return result;
}
export function validateManifest(input: unknown): ThemeManifest {
  const obj = record(input, 'theme');
  keys(obj, ['schemaVersion', 'id', 'name', 'nameEn', 'description', 'descriptionEn', 'extends', 'design', 'defaultMode', 'colors', 'layout'], 'theme');
  if (obj.schemaVersion !== THEME_SCHEMA_VERSION) throw new Error('Unsupported theme schemaVersion');
  const id = string(obj.id, 48, 'id');
  if (!ID.test(id)) throw new Error('id: use lowercase letters, digits and hyphens (2–48 characters)');
  if (id === 'paper') throw new Error('The paper theme has been retired; choose a new theme id');
  const out: ThemeManifest = { schemaVersion: 1, id, name: string(obj.name, 80, 'name'), nameEn: string(obj.nameEn ?? obj.name, 80, 'nameEn') };
  for (const key of ['description', 'descriptionEn'] as const) {
    if (obj[key] !== undefined) out[key] = string(obj[key], 300, key);
  }
  if (obj.extends !== undefined) {
    out.extends = string(obj.extends, 48, 'extends');
    if (out.extends === 'paper') out.extends = 'tide';
    if (!ID.test(out.extends) || out.extends === id) throw new Error('extends: invalid parent');
  }
  if (obj.design !== undefined) {
    if (typeof obj.design !== 'string' || !ID.test(obj.design)) throw new Error('design: expected a safe CSS namespace (2–48 lowercase letters, digits or hyphens)');
    out.design = obj.design === 'paper' ? 'tide' : obj.design as Design;
  }
  if (obj.defaultMode !== undefined) {
    if (!MODES.includes(obj.defaultMode as string)) throw new Error('defaultMode: unknown mode');
    out.defaultMode = obj.defaultMode as Mode;
  }
  if (obj.colors !== undefined) out.colors = validateColors(obj.colors);
  if (obj.layout !== undefined) out.layout = validateLayout(obj.layout);
  return out;
}
export function parseThemeJson(text: string): ThemeManifest {
  if (new TextEncoder().encode(text).length > MAX_THEME_BYTES) throw new Error('Theme file exceeds 64 KiB');
  return validateManifest(JSON.parse(text));
}
/** Resolve inheritance with bounded traversal; invalid manifests never partially apply. */
export function resolveTheme(id: string, themes: readonly ThemeManifest[], scheme: Scheme, override: ThemeOverride = {}): ResolvedTheme {
  const chain: ThemeManifest[] = [];
  const visited = new Set<string>();
  let next: string | undefined = id;
  while (next) {
    if (visited.has(next) || chain.length >= 12) throw new Error('Theme inheritance cycle or depth limit');
    visited.add(next);
    const theme = themes.find(item => item.id === next);
    if (!theme) throw new Error(`Unknown theme: ${next}`);
    chain.unshift(theme);
    next = theme.extends;
  }
  let design: Design = 'classic';
  const layout = { ...DEFAULT_LAYOUT };
  const palette: Partial<Palette> = {};
  for (const theme of chain) {
    if (theme.design) design = theme.design;
    Object.assign(layout, theme.layout);
    Object.assign(palette, theme.colors?.[scheme]);
  }
  Object.assign(layout, validateLayout(override.layout ?? {}));
  Object.assign(palette, validateColors(override.colors ?? {})?.[scheme]);
  for (const key of COLOR_KEYS) {
    if (!palette[key] || !HEX.test(palette[key]!)) throw new Error(`Missing ${scheme} color: ${key}`);
  }
  return { id, design, layout, palette: palette as Palette };
}
export function defaultAppearance(): AppearanceState {
  return { version: 1, themeId: 'classic', mode: 'system', customThemes: [], overrides: {} };
}
export function validateState(input: unknown, builtin: readonly ThemeManifest[]): AppearanceState {
  const obj = record(input, 'appearance');
  keys(obj, ['version', 'themeId', 'mode', 'customThemes', 'overrides'], 'appearance');
  if (obj.version !== 1 || !MODES.includes(obj.mode as string)) throw new Error('Invalid appearance version or mode');
  if (!Array.isArray(obj.customThemes) || obj.customThemes.length > MAX_CUSTOM_THEMES) throw new Error('Too many custom themes');
  const customThemes = obj.customThemes.map(validateManifest);
  const ids = new Set(builtin.map(item => item.id));
  for (const theme of customThemes) {
    if (ids.has(theme.id)) throw new Error(`Duplicate or reserved theme id: ${theme.id}`);
    ids.add(theme.id);
  }
  const themes = [...builtin, ...customThemes];
  for (const theme of customThemes) for (const scheme of ['light', 'dark'] as const) resolveTheme(theme.id, themes, scheme);
  const overrides: AppearanceState['overrides'] = {};
  // Upgrade persisted selections and backups without losing explicit user colors.
  // If both keys exist, the newer Tide overrides take precedence per field.
  const entries = Object.entries(record(obj.overrides, 'overrides'));
  entries.sort(([left], [right]) => left === 'paper' ? -1 : right === 'paper' ? 1 : 0);
  for (const [savedId, value] of entries) {
    const id = savedId === 'paper' ? 'tide' : savedId;
    if (!ids.has(id)) throw new Error(`Unknown override theme: ${id}`);
    const override = record(value, 'override');
    keys(override, ['colors', 'layout'], 'override');
    const colors = validateColors(override.colors ?? {});
    const layout = validateLayout(override.layout ?? {});
    const previous = overrides[id];
    overrides[id] = previous ? {
      colors: {
        light: { ...previous.colors?.light, ...colors?.light },
        dark: { ...previous.colors?.dark, ...colors?.dark },
      },
      layout: { ...previous.layout, ...layout },
    } : { colors, layout };
  }
  const themeId = obj.themeId === 'paper' ? 'tide' : obj.themeId;
  if (typeof themeId !== 'string' || !ids.has(themeId)) throw new Error('Unknown selected theme');
  return { version: 1, themeId, mode: obj.mode as Mode, customThemes, overrides };
}
/** Improve generated terminal ANSI colors without changing the user's surface palette. */
export function ensureContrast(foreground: string, background: string, minimum = 4.5): string {
  if (!HEX.test(foreground) || !HEX.test(background) || !Number.isFinite(minimum) || minimum < 1 || minimum > 21) {
    throw new Error('Expected #RRGGBB colors and a contrast ratio between 1 and 21');
  }
  if (contrastRatio(foreground, background) >= minimum) return foreground;
  const target = contrastRatio('#FFFFFF', background) >= contrastRatio('#000000', background) ? '#FFFFFF' : '#000000';
  const rgb = (hex: string) => [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16));
  const start = rgb(foreground), end = rgb(target);
  for (let step = 1; step <= 100; step++) {
    const color = '#' + start.map((v, i) => Math.round(v + (end[i]! - v) * step / 100).toString(16).padStart(2, '0')).join('');
    if (contrastRatio(color, background) >= minimum) return color.toUpperCase();
  }
  // Some backgrounds cannot reach the requested ratio; return the highest possible contrast.
  return target;
}

export function contrastRatio(a: string, b: string): number {
  function luminance(hex: string) {
    const rgb = [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16) / 255).map(c => c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
    return rgb[0]! * 0.2126 + rgb[1]! * 0.7152 + rgb[2]! * 0.0722;
  }
  const la = luminance(a), lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}
