// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { h } from 'vue';

import WebSessionReasoningSummary from '../WebSessionReasoningSummary.vue';
import { renderMarkdown } from '@/utils/markdown';

const text = '**Checking inverse ReplaceStep handling**\n\n**Refining ReplaceStep checks**';
const props = {
  text,
  label: '思考摘要',
  time: '18:19:11',
  timeTitle: '2026-09-05 18:19:11',
};

function mountSummary(expanded = false) {
  return mount(WebSessionReasoningSummary, {
    props: { ...props, expanded },
    slots: { default: () => h('div', { innerHTML: renderMarkdown(text) }) },
  });
}

describe('WebSessionReasoningSummary', () => {
  it('starts collapsed with a single label and timestamp', () => {
    const wrapper = mountSummary();
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('false');
    expect(wrapper.text()).toContain('思考摘要');
    expect(wrapper.text().match(/思考摘要/g)).toHaveLength(1);
    expect(wrapper.text()).toContain('18:19:11');
    expect(wrapper.find('.reasoning-summary-body').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('Checking');
  });

  it('toggles and renders separate Markdown paragraphs without tool chrome', async () => {
    const wrapper = mountSummary();
    await wrapper.get('button').trigger('click');
    expect(wrapper.emitted('toggle')).toHaveLength(1);
    await wrapper.setProps({ expanded: true });
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('true');
    expect(wrapper.findAll('p strong').map(node => node.text())).toEqual([
      'Checking inverse ReplaceStep handling',
      'Refining ReplaceStep checks',
    ]);
    expect(wrapper.text()).not.toContain('**');
    expect(wrapper.find('pre').exists()).toBe(false);
    await wrapper.get('button').trigger('click');
    expect(wrapper.emitted('toggle')).toHaveLength(2);
    await wrapper.setProps({ expanded: false });
    expect(wrapper.find('.reasoning-summary-body').exists()).toBe(false);
  });

  it('does not render an empty summary', () => {
    const wrapper = mount(WebSessionReasoningSummary, {
      props: { ...props, text: ' \n ', expanded: true },
    });
    expect(wrapper.find('button').exists()).toBe(false);
    expect(wrapper.text()).toBe('');
  });

  it('preserves expansion on refresh and defaults to collapsed when remounted', async () => {
    const wrapper = mountSummary(true);
    await wrapper.setProps({ text: `${text}\n\nUpdated summary` });
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('true');
    wrapper.unmount();
    const refreshed = mountSummary();
    expect(refreshed.get('button').attributes('aria-expanded')).toBe('false');
  });
});
