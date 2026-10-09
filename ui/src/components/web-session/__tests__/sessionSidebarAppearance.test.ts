// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import WebSessionSidebarRow from '../WebSessionSidebarRow.vue';
import { WEB_SESSION_SIDEBAR_VIRTUAL_ITEM_SIZE, type WebSessionSidebarRowView } from '../webSessionSidebarVirtualList';

const title = '0100 · 检查大量会话的标题扫描 / Review long session titles without losing keyboard access';
function row(active = false): WebSessionSidebarRowView {
  return { key: 'session-100', sessionId: 'session-100', title, iconHtml: '', subtitle: 'Not a second visible line', tooltip: title, accentColor: '#315FC4', toneClass: '', active, archived: false, archiving: false, hasWorkflowPlanBadge: false, hasScheduledPlanExecution: false, hasScheduledInput: false, scheduledInputTitle: '', singleProject: true, currentIndicatorTitle: 'Current', activityTimeLabel: '12:00', activityTimeTitle: '12:00', moreActionsLabel: 'More' };
}
function create(active = false) {
  return mount(WebSessionSidebarRow, { props: { row: row(active), actionOptions: [] }, global: { stubs: { NDropdown: true, NIcon: true } } });
}
describe('title-first compact session rows', () => {
  it('retains the original 38px virtual slot instead of expanding rows for a theme', () => {
    expect(WEB_SESSION_SIDEBAR_VIRTUAL_ITEM_SIZE).toBe(38);
  });
  it('exposes the complete title to keyboard and assistive technology without adding a subtitle', () => {
    const wrapper = create();
    const button = wrapper.get('button.session-sidebar-main');
    expect(button.attributes('aria-label')).toBe(title);
    expect(button.attributes('title')).toBe(title);
    expect(wrapper.findAll('.session-sidebar-item-title')).toHaveLength(1);
    expect(wrapper.text()).not.toContain('Not a second visible line');
    wrapper.unmount();
  });
  it('selects exactly once through the native title button', async () => {
    const wrapper = create();
    await wrapper.get('button.session-sidebar-main').trigger('click');
    expect(wrapper.emitted('select')).toHaveLength(1);
    wrapper.unmount();
  });
  it('keeps the actions menu independent from session selection', async () => {
    const wrapper = create();
    await wrapper.get('.session-sidebar-menu-button').trigger('click');
    expect(wrapper.emitted('select')).toBeUndefined();
    wrapper.unmount();
  });
  it('updates current-state semantics without replacing the title button', async () => {
    const wrapper = create();
    const element = wrapper.get('.session-sidebar-main').element;
    expect(wrapper.get('.session-sidebar-main').attributes('aria-current')).toBeUndefined();
    await wrapper.setProps({ row: row(true) });
    expect(wrapper.get('.session-sidebar-main').attributes('aria-current')).toBe('true');
    expect(wrapper.get('.session-sidebar-main').element).toBe(element);
    wrapper.unmount();
  });
});
