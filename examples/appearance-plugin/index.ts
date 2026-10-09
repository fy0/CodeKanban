/** Copy this folder under ui/src, then import it from main.ts BEFORE app creation.
 * Trusted build-time code only. This example is not installed automatically.
 */
import { defineComponent, h } from 'vue';
import { registerAppearancePlugin } from '@/themes';
import './forest.css';

const TeamBadge = defineComponent({
  name: 'TeamBadge',
  setup: () => () => h('span', { class: 'forest-team-badge' }, 'Engineering'),
});
registerAppearancePlugin({
  id: 'forest-team',
  themes: [{
    schemaVersion: 1,
    id: 'forest-team-studio',
    name: '团队森林',
    nameEn: 'Forest Team Studio',
    extends: 'studio',
    design: 'forest-team',
    defaultMode: 'system',
    colors: { light: { accent: '#28684B' }, dark: { accent: '#A3D8B8' } },
    layout: { navigation: 'sidebar', radius: 10 },
  }],
  contributions: [{ id: 'forest-team-badge', slot: 'shell.toolbar', order: 20, component: TeamBadge }],
});
