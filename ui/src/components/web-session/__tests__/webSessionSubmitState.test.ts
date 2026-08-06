import { describe, expect, it } from 'vitest';

import {
  buildWebSessionSubmitOwnerId,
  beginWebSessionSubmit,
  endWebSessionSubmit,
  getWebSessionSubmitEntry,
  isWebSessionSubmitting,
  resolveOptimisticWebSessionLiveState,
  shouldShowWebSessionExecuteFeedback,
  transferWebSessionSubmit,
} from '@/components/web-session/webSessionSubmitState';

describe('webSessionSubmitState', () => {
  it('tracks submit state per session', () => {
    const state = beginWebSessionSubmit({}, 'session-a', {
      kind: 'execute_send',
      startedAt: 100,
    });

    expect(isWebSessionSubmitting(state, 'session-a')).toBe(true);
    expect(isWebSessionSubmitting(state, 'session-b')).toBe(false);
    expect(getWebSessionSubmitEntry(state, 'session-a')).toEqual({
      kind: 'execute_send',
      startedAt: 100,
    });
  });

  it('transfers submit ownership from draft to created session', () => {
    const initial = beginWebSessionSubmit({}, 'draft-session', {
      kind: 'execute_plan',
      startedAt: 220,
    });
    const transferred = transferWebSessionSubmit(initial, 'draft-session', 'session-1');

    expect(isWebSessionSubmitting(transferred, 'draft-session')).toBe(false);
    expect(isWebSessionSubmitting(transferred, 'session-1')).toBe(true);
    expect(getWebSessionSubmitEntry(transferred, 'session-1')).toEqual({
      kind: 'execute_plan',
      startedAt: 220,
    });
  });

  it('clears only the final owner when finishing', () => {
    const initial = beginWebSessionSubmit({}, 'draft-session', {
      kind: 'execute_send',
      startedAt: 300,
    });
    const transferred = transferWebSessionSubmit(initial, 'draft-session', 'session-1');
    const finished = endWebSessionSubmit(transferred, 'session-1');

    expect(isWebSessionSubmitting(finished, 'draft-session')).toBe(false);
    expect(isWebSessionSubmitting(finished, 'session-1')).toBe(false);
  });

  it('ignores duplicate finish and invalid transfers', () => {
    const initial = beginWebSessionSubmit({}, 'session-a', {
      kind: 'execute_send',
      startedAt: 400,
    });
    const duplicateFinish = endWebSessionSubmit(initial, 'session-b');
    const invalidTransfer = transferWebSessionSubmit(initial, 'session-b', 'session-c');
    const blankTargetTransfer = transferWebSessionSubmit(initial, 'session-a', '');

    expect(duplicateFinish).toEqual(initial);
    expect(invalidTransfer).toEqual(initial);
    expect(isWebSessionSubmitting(blankTargetTransfer, 'session-a')).toBe(false);
    expect(isWebSessionSubmitting(blankTargetTransfer, 'session-c')).toBe(false);
  });

  it('builds stable composite owner ids', () => {
    expect(buildWebSessionSubmitOwnerId(' user_input ', 'session-1', ' item-2 ')).toBe(
      'user_input::session-1::item-2'
    );
    expect(buildWebSessionSubmitOwnerId(' ', 'session-1', '')).toBe('session-1');
  });

  it('distinguishes execute feedback from plan-only messages', () => {
    expect(shouldShowWebSessionExecuteFeedback({ kind: 'execute_send' })).toBe(true);
    expect(shouldShowWebSessionExecuteFeedback({ kind: 'execute_plan' })).toBe(true);
    expect(shouldShowWebSessionExecuteFeedback({ kind: 'plan_message' })).toBe(false);
    expect(shouldShowWebSessionExecuteFeedback({ kind: 'redirect_message' })).toBe(false);
    expect(shouldShowWebSessionExecuteFeedback({ kind: 'queue_message' })).toBe(false);
  });

  it('keeps the first in-flight staged-message lock when submit is clicked repeatedly', () => {
    const initial = beginWebSessionSubmit({}, 'session-a', {
      kind: 'redirect_message',
      startedAt: 100,
    });
    const duplicate = beginWebSessionSubmit(initial, 'session-a', {
      kind: 'queue_message',
      startedAt: 200,
    });

    expect(duplicate).toBe(initial);
    expect(getWebSessionSubmitEntry(duplicate, 'session-a')).toEqual({
      kind: 'redirect_message',
      startedAt: 100,
    });
  });

  it('creates an optimistic starting state for execute-plan submissions', () => {
    const optimistic = resolveOptimisticWebSessionLiveState(
      {
        phase: 'waiting_plan_approval',
        running: false,
        updatedAt: 500,
        startedAt: 450,
      },
      {
        kind: 'execute_plan',
        startedAt: 600,
      }
    );

    expect(optimistic).toEqual({
      phase: 'starting',
      running: true,
      updatedAt: 600,
      startedAt: 600,
    });
  });

  it('does not create optimistic run state for plan-mode sends or already running sessions', () => {
    const planModeState = resolveOptimisticWebSessionLiveState(
      {
        phase: 'done',
        running: false,
        updatedAt: 500,
        startedAt: 450,
      },
      {
        kind: 'plan_message',
        startedAt: 700,
      }
    );
    const runningState = resolveOptimisticWebSessionLiveState(
      {
        phase: 'tool',
        running: true,
        updatedAt: 800,
        startedAt: 750,
        tool: {
          id: 'tool-1',
          name: 'Edit',
        },
      },
      {
        kind: 'execute_send',
        startedAt: 900,
      }
    );

    expect(planModeState).toEqual({
      phase: 'done',
      running: false,
      updatedAt: 500,
      startedAt: 450,
    });
    expect(runningState).toEqual({
      phase: 'tool',
      running: true,
      updatedAt: 800,
      startedAt: 750,
      tool: {
        id: 'tool-1',
        name: 'Edit',
      },
    });
  });
});
