// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { reactive } from 'vue';
import { createI18n } from 'vue-i18n';
import { describe, expect, it } from 'vitest';

import WebSessionUserInputCard from '../WebSessionUserInputCard.vue';
import type { WebSessionUserInputQuestion } from '@/stores/webSession';

function render() {
  const questions: WebSessionUserInputQuestion[] = [
    {
      id: 'scope',
      header: 'Scope',
      question: 'What should change?',
      options: [
        { label: 'UI', description: 'Update the interface' },
        { label: 'All', description: '' },
      ],
      isOther: true,
      isSecret: false,
    },
    {
      id: 'details',
      header: 'Details',
      question: 'Any details?',
      options: [],
      isOther: true,
      isSecret: false,
    },
  ];
  const selections = reactive<Record<string, string[]>>({});
  const drafts = reactive<Record<string, string>>({});
  const wrapper = mount(WebSessionUserInputCard, {
    props: {
      requestKey: 'session:request',
      questions,
      selections,
      drafts,
      disabled: false,
      submitting: false,
      'onUpdate:selection': (id: string, value: string[]) => {
        selections[id] = value;
      },
      'onUpdate:draft': (id: string, value: string) => {
        drafts[id] = value;
      },
    },
    global: {
      plugins: [
        createI18n({
          legacy: false,
          locale: 'en',
          missingWarn: false,
          fallbackWarn: false,
          messages: {
            en: {
              webSession: {
                userInputTitle: 'Question',
                userInputWaitingHint: 'Waiting for your answer',
                userInputSubmit: 'Submit',
                userInputOtherPlaceholder: 'Other',
                userInputAnswerPlaceholder: 'Answer',
                asyncInputNext: 'Next',
                asyncInputPrevious: 'Previous',
                asyncInputProgress: 'Question {current} of {total}',
                asyncInputSubmitting: 'Submitting',
                stop: 'Stop',
              },
            },
          },
        }),
      ],
    },
  });
  return { wrapper, selections, drafts };
}

describe('WebSessionUserInputCard', () => {
  it('answers within one paged card and retains selections when navigating', async () => {
    const { wrapper, selections, drafts } = render();
    expect(wrapper.findAll('h3')).toHaveLength(1);
    expect(wrapper.text()).toContain('1 / 2');
    expect(wrapper.emitted('submit')).toBeUndefined();
    await wrapper.findAll<HTMLInputElement>('input[type="radio"]')[1]!.setValue(true);
    await wrapper.find('form').trigger('submit');
    expect(wrapper.text()).toContain('2 / 2');
    expect(wrapper.emitted('submit')).toBeUndefined();
    await wrapper.find<HTMLInputElement>('input[type="text"]').setValue('Keep mobile support');
    await wrapper.find('[aria-label="Previous"]').trigger('click');
    expect(wrapper.findAll<HTMLInputElement>('input[type="radio"]')[1]!.element.checked).toBe(true);
    await wrapper.find('[aria-label="Next"]').trigger('click');
    expect(wrapper.find<HTMLInputElement>('input[type="text"]').element.value).toBe(
      'Keep mobile support'
    );
    await wrapper.find('form').trigger('submit');
    expect(wrapper.emitted('submit')).toHaveLength(1);
    expect(selections.scope).toEqual(['All']);
    expect(drafts.details).toBe('Keep mobile support');
  });

  it('returns to an unanswered question instead of submitting a partial response', async () => {
    const { wrapper } = render();
    await wrapper.find('[aria-label="Next"]').trigger('click');
    await wrapper.find<HTMLInputElement>('input[type="text"]').setValue('Details');
    await wrapper.find('form').trigger('submit');
    expect(wrapper.text()).toContain('1 / 2');
    expect(wrapper.emitted('submit')).toBeUndefined();
  });

  it('respects IME composition and resets pagination for a new request', async () => {
    const { wrapper } = render();
    const input = wrapper.find<HTMLInputElement>('input[type="text"]');
    await input.trigger('keydown', { key: 'Enter', isComposing: true });
    expect(wrapper.text()).toContain('1 / 2');
    await input.trigger('keydown', { key: 'Enter' });
    expect(wrapper.text()).toContain('2 / 2');
    await wrapper.setProps({ requestKey: 'session:new-request' });
    expect(wrapper.text()).toContain('1 / 2');
  });

  it('prevents editing, stopping, and duplicate submission while sending', async () => {
    const { wrapper } = render();
    await wrapper.setProps({ submitting: true });
    expect(wrapper.find<HTMLInputElement>('input[type="text"]').element.disabled).toBe(true);
    expect(wrapper.find<HTMLButtonElement>('.question-card-later').element.disabled).toBe(true);
    await wrapper.find('form').trigger('submit');
    expect(wrapper.emitted('submit')).toBeUndefined();
    expect(wrapper.text()).toContain('1 / 2');
  });
});
