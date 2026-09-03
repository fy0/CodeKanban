import { describe, expect, it } from 'vitest';
import {
  buildWebSessionFreshContextPlanPrompt,
  WEB_SESSION_FRESH_CONTEXT_PLAN_PREFIX,
} from '../webSessionPlanImplementation';

describe('webSession fresh-context plan implementation', () => {
  it('hands the approved plan to a fresh implementation context', () => {
    expect(buildWebSessionFreshContextPlanPrompt('  - Step 1\n- Step 2\n')).toBe(
      `${WEB_SESSION_FRESH_CONTEXT_PLAN_PREFIX}\n\n- Step 1\n- Step 2`
    );
  });

  it('does not create a handoff without an approved plan', () => {
    expect(buildWebSessionFreshContextPlanPrompt('  \n')).toBe('');
  });
});
