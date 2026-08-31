// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { describe, expect, it } from 'vitest';

import WebSessionComposerEditor from '@/components/web-session/WebSessionComposerEditor.vue';

describe('WebSessionComposerEditor', () => {
  it('keeps restored text without emitting an update when editing is unlocked', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'restored draft',
        disabled: true,
      },
    });
    await nextTick();
    await nextTick();

    const editor = wrapper.get('[role="textbox"]');
    expect(editor.text()).toBe('restored draft');
    expect(editor.attributes('contenteditable')).toBe('false');
    expect(editor.attributes('aria-disabled')).toBe('true');

    await wrapper.setProps({ disabled: false });
    await nextTick();

    expect(editor.text()).toBe('restored draft');
    expect(editor.attributes('contenteditable')).toBe('true');
    expect(editor.attributes('aria-disabled')).toBe('false');
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
  });
});
