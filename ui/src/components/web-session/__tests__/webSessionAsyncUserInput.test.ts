// @vitest-environment happy-dom

import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import WebSessionAsyncUserInput from '../WebSessionAsyncUserInput.vue';
import type { WebSessionBlock } from '@/stores/webSession';

const { answer, savedDrafts } = vi.hoisted(() => ({
  answer: vi.fn(),
  savedDrafts: new Map<
    string,
    { selections: Record<string, string[]>; drafts: Record<string, string> }
  >(),
}));

vi.mock('@/stores/webSession', () => ({
  useWebSessionStore: () => ({
    answerAsyncUserInput: answer,
    getPendingUserInputDraft: (key: string) => savedDrafts.get(key) ?? null,
    setPendingUserInputDraft: (
      key: string,
      value: { selections: Record<string, string[]>; drafts: Record<string, string> }
    ) => {
      savedDrafts.set(key, JSON.parse(JSON.stringify(value)));
    },
    clearPendingUserInputDraft: (key: string) => savedDrafts.delete(key),
  }),
}));

function makeItem(): WebSessionBlock {
  return {
    key: 'question:1',
    id: 'question',
    orderIndex: 1,
    kind: 'assistant',
    itemType: 'agent_message',
    text: 'Which deployment?',
    timestamp: 1,
    attachments: [],
    done: true,
    detail: {
      type: 'async_user_input_request',
      questions: [
        {
          id: '0',
          header: '',
          question: 'Which deployment?',
          isOther: true,
          isSecret: false,
          options: [
            { label: 'Subdirectory', description: '' },
            { label: 'Copy folder', description: '' },
          ],
        },
        {
          id: '1',
          header: '',
          question: 'What is the error?',
          isOther: true,
          isSecret: false,
          options: [],
        },
      ],
    },
  };
}

function render(item = makeItem(), disabled = false) {
  return mount(WebSessionAsyncUserInput, {
    props: { sessionId: 'session-1', item, disabled },
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
                asyncInputTitle: 'Question',
                asyncInputWorkingHint: 'Work continues',
                asyncInputNext: 'Next',
                asyncInputPrevious: 'Previous',
                asyncInputLater: 'Not now',
                asyncInputResume: 'Continue',
                asyncInputSubmitting: 'Submitting',
                asyncInputProgress: 'Question {current} of {total}',
                asyncInputPlaceholder: 'Your answer',
                asyncInputSubmitted: 'Submitted',
                userInputSubmit: 'Submit',
              },
            },
          },
        }),
      ],
    },
  });
}

describe('WebSessionAsyncUserInput', () => {
  beforeEach(() => {
    answer.mockReset();
    savedDrafts.clear();
  });

  it('preselects a suggestion without sending and submits all answers including free text', async () => {
    answer.mockResolvedValue(undefined);
    const wrapper = render();
    const options = wrapper.findAll('.question-card-option');
    expect(options.map(option => option.attributes('aria-pressed'))).toEqual(['true', 'false']);
    expect(answer).not.toHaveBeenCalled();
    await options[1]!.trigger('click');
    await wrapper.find('form').trigger('submit');
    expect(answer).not.toHaveBeenCalled();
    expect(wrapper.find('.question-card-header h3').text()).toBe('What is the error?');
    expect(wrapper.find('.question-card-submit').attributes('disabled')).toBeDefined();
    await wrapper.find('textarea').setValue('404 on assets');
    await wrapper.find('form').trigger('submit');
    await flushPromises();
    expect(answer).toHaveBeenCalledExactlyOnceWith('session-1', 'question', {
      '0': ['Copy folder'],
      '1': ['404 on assets'],
    });
    expect(wrapper.text()).toContain('Submitted');
    expect(wrapper.find('form').exists()).toBe(false);
    wrapper.unmount();
  });

  it('lets a custom answer replace the suggestion and restores drafts after remount', async () => {
    const wrapper = render();
    await wrapper.find('textarea').setValue('Both deployments fail');
    expect(
      wrapper
        .findAll('.question-card-option')
        .some(option => option.attributes('aria-pressed') === 'true')
    ).toBe(false);
    await wrapper.find('form').trigger('submit');
    await wrapper.find('textarea').setValue('Missing WASM');
    await flushPromises();
    wrapper.unmount();
    const restored = render();
    expect(restored.find<HTMLTextAreaElement>('textarea').element.value).toBe(
      'Both deployments fail'
    );
    await restored.findAll('.question-card-option')[0]!.trigger('click');
    expect(restored.find<HTMLTextAreaElement>('textarea').element.value).toBe('');
    await restored.find('form').trigger('submit');
    expect(restored.find<HTMLTextAreaElement>('textarea').element.value).toBe('Missing WASM');
    restored.unmount();
  });

  it('prevents duplicate submission and retains answers when sending fails', async () => {
    let rejectSend!: (reason: Error) => void;
    answer.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectSend = reject;
        })
    );
    const wrapper = render();
    await wrapper.find('form').trigger('submit');
    await wrapper.find('textarea').setValue('404');
    await wrapper.find('form').trigger('submit');
    await wrapper.find('form').trigger('submit');
    expect(answer).toHaveBeenCalledTimes(1);
    rejectSend(new Error('Disconnected'));
    await flushPromises();
    expect(wrapper.find('[role="alert"]').text()).toBe('Disconnected');
    expect(wrapper.find<HTMLTextAreaElement>('textarea').element.value).toBe('404');
    answer.mockResolvedValueOnce(undefined);
    await wrapper.find('form').trigger('submit');
    await flushPromises();
    expect(answer).toHaveBeenCalledTimes(2);
    expect(savedDrafts.size).toBe(0);
    wrapper.unmount();
  });

  it('allows closing and resuming without sending or losing the current answer', async () => {
    const wrapper = render();
    await wrapper.find('textarea').setValue('Both');
    await wrapper.find('.question-card-close').trigger('click');
    expect(wrapper.find('form').exists()).toBe(false);
    expect(answer).not.toHaveBeenCalled();
    await wrapper.find('.question-card-later').trigger('click');
    expect(wrapper.find<HTMLTextAreaElement>('textarea').element.value).toBe('Both');
    expect(wrapper.find('.question-card-progress').text()).toBe('1 / 2');
    wrapper.unmount();
  });

  it('restores submitted answers as read-only history', () => {
    const item = makeItem();
    item.detail!.action = 'submitted';
    item.detail!.answers = [{ id: '0', label: 'Which deployment?', values: ['Copy folder'] }];
    const wrapper = render(item);
    expect(wrapper.text()).toContain('Submitted');
    expect(wrapper.text()).toContain('Copy folder');
    expect(wrapper.find('form').exists()).toBe(false);
    expect(answer).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('does not send when the session is unavailable', async () => {
    const wrapper = render(makeItem(), true);
    await wrapper.find('form').trigger('submit');
    expect(answer).not.toHaveBeenCalled();
    expect(
      wrapper.findAll<HTMLTextAreaElement>('textarea').every(input => input.element.disabled)
    ).toBe(true);
    wrapper.unmount();
  });
});
