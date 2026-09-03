export const WEB_SESSION_FRESH_CONTEXT_PLAN_PREFIX =
  "A previous agent produced the plan below to accomplish the user's task. " +
  'Implement the plan in a fresh context. Treat the plan as the source of ' +
  'user intent, re-read files as needed, and carry the work through ' +
  'implementation and verification.';

export function buildWebSessionFreshContextPlanPrompt(planMarkdown: string) {
  const normalizedPlan = String(planMarkdown || '').trim();
  if (!normalizedPlan) {
    return '';
  }
  return `${WEB_SESSION_FRESH_CONTEXT_PLAN_PREFIX}\n\n${normalizedPlan}`;
}
