// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { defineComponent, h, nextTick, reactive, ref } from 'vue';
import { describe, expect, it } from 'vitest';

import WebSessionUserInputQuestion from '@/components/web-session/WebSessionUserInputQuestion.vue';

const question = {
  id: 'history-count',
  header: 'History count',
  question: 'How many recent items should remain?',
  multiSelect: false,
  isOther: true,
  isSecret: false,
  options: [
    { label: 'Recent 10', description: 'Keep more context.' },
    { label: 'Recent 5', description: 'Keep less context.' },
    { label: 'Recent 20', description: 'Keep the largest context.' },
  ],
};

describe('WebSessionUserInputQuestion', () => {
  it('retains multiple selections with numbered options', async () => {
    const wrapper = mount(WebSessionUserInputQuestion, {
      props: {
        question: { ...question, multiSelect: true },
        selection: [],
        draft: '',
        disabled: false,
        placeholder: 'Other',
      },
    });
    await wrapper.findAll('[role="checkbox"]')[0]!.trigger('click');
    expect(wrapper.emitted('update:selection')?.[0]).toEqual([['Recent 10']]);
    await wrapper.setProps({ selection: ['Recent 10'] });
    await wrapper.findAll('[role="checkbox"]')[2]!.trigger('click');
    expect(wrapper.emitted('update:selection')?.[1]).toEqual([['Recent 10', 'Recent 20']]);
    expect(wrapper.find('.user-input-number').text()).toBe('✓');
  });

  it('keeps secret answers masked and disables stale inputs', async () => {
    const wrapper = mount(WebSessionUserInputQuestion, {
      props: {
        question: { ...question, options: [], isSecret: true },
        selection: [],
        draft: 'secret',
        disabled: true,
        placeholder: 'Secret',
      },
    });
    const input = wrapper.find<HTMLInputElement>('input[type="password"]');
    expect(input.element.value).toBe('secret');
    expect(input.element.disabled).toBe(true);
    expect(input.attributes('aria-label')).toBe(question.question);
  });

  it('restores editable state after switching sessions and keeps one radio selected', async () => {
    const activeSessionId = ref<'session-a' | 'session-b'>('session-a');
    const stateBySession = reactive({
      'session-a': { selection: [] as string[], draft: '' },
      'session-b': { selection: [] as string[], draft: '' },
    });
    const Harness = defineComponent({
      setup() {
        return () => {
          const sessionId = activeSessionId.value;
          const state = stateBySession[sessionId];
          return h(WebSessionUserInputQuestion, {
            key: `${sessionId}:request-1:${question.id}`,
            question,
            selection: state.selection,
            draft: state.draft,
            disabled: false,
            placeholder: 'Add extra detail',
            'onUpdate:selection': (value: string[]) => {
              state.selection = value;
            },
            'onUpdate:draft': (value: string) => {
              state.draft = value;
            },
          });
        };
      },
    });
    const wrapper = mount(Harness);

    await wrapper.findAll<HTMLInputElement>('input[type="radio"]')[0]!.setValue(true);
    await wrapper.find<HTMLInputElement>('input[type="text"]').setValue('Keep active items');
    expect(stateBySession['session-a']).toEqual({
      selection: ['Recent 10'],
      draft: 'Keep active items',
    });

    activeSessionId.value = 'session-b';
    await nextTick();
    expect(wrapper.find<HTMLInputElement>('input[type="text"]').element.value).toBe('');

    activeSessionId.value = 'session-a';
    await nextTick();
    const restoredInput = wrapper.find<HTMLInputElement>('input[type="text"]');
    const restoredRadios = wrapper.findAll<HTMLInputElement>('input[type="radio"]');
    expect(restoredInput.element.disabled).toBe(false);
    expect(restoredInput.element.value).toBe('Keep active items');
    expect(restoredRadios.map(radio => radio.element.checked)).toEqual([true, false, false]);

    await restoredRadios[1]!.setValue(true);
    const updatedRadios = wrapper.findAll<HTMLInputElement>('input[type="radio"]');
    expect(stateBySession['session-a'].selection).toEqual(['Recent 5']);
    expect(updatedRadios.map(radio => radio.element.checked)).toEqual([false, true, false]);
    expect(updatedRadios.filter(radio => radio.element.checked)).toHaveLength(1);
  });
});
