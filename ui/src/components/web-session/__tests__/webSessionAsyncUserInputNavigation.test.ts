import { describe, expect, it } from 'vitest';

import { getPendingWebSessionAsyncUserInputs } from '../webSessionAsyncUserInput';
import type { WebSessionBlock } from '@/stores/webSession';
import type { WebSessionSummary } from '@/types/models';

const session = { agent: 'codex', nativeSessionId: 'thread-1' } as const;

function question(id: string, action?: string, sourceThreadId?: string): WebSessionBlock {
  return {
    key: id,
    id,
    orderIndex: 1,
    kind: 'assistant',
    itemType: 'agent_message',
    text: '',
    timestamp: 1,
    attachments: [],
    done: true,
    sourceThreadId,
    detail: {
      type: 'async_user_input_request',
      action,
      questions: [
        {
          id: '0',
          header: '',
          question: 'Which option?',
          isOther: true,
          isSecret: false,
          options: [],
        },
      ],
    },
  };
}

function userMessage(overrides: Partial<WebSessionBlock> = {}): WebSessionBlock {
  return {
    ...question('reply'),
    kind: 'user',
    itemType: 'user_message',
    text: 'Continue with the suggested option',
    detail: undefined,
    ...overrides,
  };
}

describe('pending async question navigation', () => {
  it('keeps unanswered questions in timeline order and removes submitted answers', () => {
    const first = question('first');
    const second = question('second', undefined, 'thread-1');
    const submitted = question('submitted', 'submitted');
    expect(getPendingWebSessionAsyncUserInputs([first, submitted, second], session)).toEqual([
      first,
      second,
    ]);
    first.detail!.action = 'submitted';
    expect(getPendingWebSessionAsyncUserInputs([first, submitted, second], session)).toEqual([
      second,
    ]);
  });

  it('ignores other threads, empty cards, and regular input requests', () => {
    const empty = question('empty');
    empty.detail!.questions = [];
    const regular = question('regular');
    regular.detail!.type = 'user_input_request';
    expect(
      getPendingWebSessionAsyncUserInputs(
        [question('old-thread', undefined, 'thread-0'), empty, regular],
        session
      )
    ).toEqual([]);
  });

  it('stops reminding about old questions after a normal reply and keeps new questions', () => {
    const oldQuestion = question('old');
    const newQuestion = question('new');
    const blocks = [oldQuestion, userMessage(), newQuestion];

    expect(getPendingWebSessionAsyncUserInputs(blocks, session)).toEqual([newQuestion]);
    expect(oldQuestion.detail?.action).toBeUndefined();
  });

  it.each(['sending', 'failed'] as const)('keeps reminders when a reply is %s', deliveryState => {
    const pending = question('pending');
    expect(
      getPendingWebSessionAsyncUserInputs([pending, userMessage({ deliveryState })], session)
    ).toEqual([pending]);
  });

  it('stops reminding once an outgoing reply is accepted', () => {
    expect(
      getPendingWebSessionAsyncUserInputs(
        [question('pending'), userMessage({ deliveryState: 'accepted' })],
        session
      )
    ).toEqual([]);
  });

  it('keeps root reminders when a sub-agent receives a message', () => {
    const pending = question('pending', undefined, session.nativeSessionId);
    expect(
      getPendingWebSessionAsyncUserInputs(
        [pending, userMessage({ sourceThreadId: 'child-thread' })],
        session
      )
    ).toEqual([pending]);
  });

  it.each([
    null,
    { ...session, agent: 'claude' },
    { ...session, archivedAt: '2026-11-04T00:00:00Z' },
  ] as (Pick<WebSessionSummary, 'agent' | 'archivedAt' | 'nativeSessionId'> | null)[])(
    'hides reminders for unavailable sessions: %j',
    unavailable => {
      expect(getPendingWebSessionAsyncUserInputs([question('first')], unavailable)).toEqual([]);
    }
  );
});
