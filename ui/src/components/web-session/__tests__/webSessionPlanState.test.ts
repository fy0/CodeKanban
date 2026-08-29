import { describe, expect, it } from 'vitest';
import { isWebSessionLatestPlanActionable } from '../webSessionPlanState';

describe('isWebSessionLatestPlanActionable', () => {
  it('keeps a plan actionable when the authoritative state is waiting for plan approval', () => {
    expect(
      isWebSessionLatestPlanActionable({
        hasPlan: true,
        hasUserMessageAfter: true,
        phase: 'waiting_plan_approval',
      })
    ).toBe(true);
  });

  it('treats a later user message as superseding the plan outside that state', () => {
    expect(
      isWebSessionLatestPlanActionable({
        hasPlan: true,
        hasUserMessageAfter: true,
        phase: 'done',
      })
    ).toBe(false);
  });
});
