/** Stable public entry point for trusted, bundled appearance extensions. */
export { registerAppearancePlugin, getRegisteredThemes, getAppearanceContributions } from './registry';
export type { AppearancePlugin, AppearanceContribution, AppearanceSlot } from './registry';
export { validateManifest, parseThemeJson, resolveTheme, contrastRatio, COLOR_KEYS, FONT_STACKS } from './core';
export type { ThemeManifest, ThemeOverride, ResolvedTheme, Palette, ColorKey, LayoutTokens, Design, Mode, Scheme } from './core';
