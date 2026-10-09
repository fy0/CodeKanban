import { onBeforeUnmount, watchEffect } from 'vue';
import { useAppearanceStore } from '@/stores/appearance';
import { FONT_STACKS } from '@/themes/core';

/** The document root also contains Naive UI's teleported dialogs, drawers and tooltips. */
export function useAppearanceRuntime(): void {
  const appearance = useAppearanceStore();
  watchEffect(() => {
    if (typeof document === 'undefined') return;
    const root = document.documentElement;
    const { design, layout } = appearance.resolved;
    root.dataset.interface = design;
    root.dataset.appearance = appearance.themeId;
    root.dataset.customized = String(appearance.customized);
    root.dataset.navigation = layout.navigation;
    root.dataset.density = layout.density;
    root.dataset.motion = layout.motion;
    root.dataset.panelStyle = layout.panelStyle;
    root.dataset.material = layout.material;
    root.dataset.texture = layout.texture;
    root.dataset.headingFont = layout.headingFont;
    const vars = {
      '--ck-radius': `${layout.radius}px`, '--ck-spacing': `${layout.spacing}px`,
      '--ck-border-width': `${layout.borderWidth}px`,
      '--ck-heading-font': layout.headingFont === 'serif' ? 'Georgia, "Noto Serif CJK SC", "Songti SC", SimSun, serif' : layout.headingFont === 'mono' ? FONT_STACKS.mono : FONT_STACKS.system,
      '--ck-font': FONT_STACKS[layout.font], '--ck-font-size': `${layout.fontSize}px`,
      '--ck-line-height': String(layout.lineHeight), '--ck-sidebar-width': `${layout.sidebarWidth}px`,
      '--ck-content-width': `${layout.contentWidth}px`,
      '--ck-control-height': layout.density === 'compact' ? '28px' : '34px',
      '--ck-panel-shadow': layout.panelStyle === 'elevated' ? '0 8px 28px var(--app-shadow-subtle)' : 'none',
    };
    for (const [key, value] of Object.entries(vars)) root.style.setProperty(key, value);
  });
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', appearance.receiveStorage);
    onBeforeUnmount(() => window.removeEventListener('storage', appearance.receiveStorage));
  }
}
