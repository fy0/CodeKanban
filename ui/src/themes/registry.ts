import { markRaw, shallowRef, type Component } from 'vue';
import builtin from './builtin.json';
import { resolveTheme, validateManifest, type ThemeManifest } from './core';

export const BUILTIN_THEMES: readonly ThemeManifest[] = builtin.map(validateManifest);
for (const theme of BUILTIN_THEMES) {
  resolveTheme(theme.id, BUILTIN_THEMES, 'light');
  resolveTheme(theme.id, BUILTIN_THEMES, 'dark');
}
export type AppearanceSlot = 'shell.brand' | 'shell.toolbar' | 'shell.footer' | 'settings.footer';
export interface AppearanceContribution {
  id: string;
  slot: AppearanceSlot;
  component: Component;
  order?: number;
}
export interface AppearancePlugin {
  id: string;
  themes?: unknown[];
  contributions?: AppearanceContribution[];
}
const themes = shallowRef<readonly ThemeManifest[]>(BUILTIN_THEMES);
const contributions = shallowRef<readonly AppearanceContribution[]>([]);
const plugins = new Set<string>();
let sealed = false;
const slots: AppearanceSlot[] = ['shell.brand', 'shell.toolbar', 'shell.footer', 'settings.footer'];
/** Trusted build-time API. Call before mounting the app; JSON theme imports cannot register code. */
export function registerAppearancePlugin(plugin: AppearancePlugin): void {
  if (sealed) throw new Error('Register appearance plugins before creating the appearance store');
  if (!/^[a-z][a-z0-9-]{1,47}$/.test(plugin.id) || plugins.has(plugin.id)) throw new Error('Invalid or duplicate appearance plugin id');
  const added = (plugin.themes ?? []).map(validateManifest);
  const next = [...themes.value, ...added];
  if (new Set(next.map(t => t.id)).size !== next.length) throw new Error('Duplicate appearance theme id');
  for (const theme of added) for (const scheme of ['light', 'dark'] as const) resolveTheme(theme.id, next, scheme);
  const nextSlots = [...contributions.value];
  for (const item of plugin.contributions ?? []) {
    if (!item.id || nextSlots.some(s => s.id === item.id) || !slots.includes(item.slot) || !item.component) throw new Error('Invalid or duplicate appearance contribution');
    if (item.order !== undefined && !Number.isFinite(item.order)) throw new Error('Invalid contribution order');
    nextSlots.push({ ...item, component: markRaw(item.component) });
  }
  // Commit only after every theme and contribution has passed validation.
  themes.value = next;
  contributions.value = nextSlots.sort((a, b) => (a.order ?? 0) - (b.order ?? 0) || a.id.localeCompare(b.id));
  plugins.add(plugin.id);
}
export function getRegisteredThemes(): readonly ThemeManifest[] { return themes.value; }
export function sealAppearanceRegistry(): void { sealed = true; }
export function getAppearanceContributions(slot: AppearanceSlot): readonly AppearanceContribution[] {
  return contributions.value.filter(item => item.slot === slot);
}
