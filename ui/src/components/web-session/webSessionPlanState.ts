export function isWebSessionLatestPlanActionable(input: {
  hasPlan: boolean;
  hasUserMessageAfter: boolean;
  phase: string;
}) {
  return input.hasPlan && (input.phase === 'waiting_plan_approval' || !input.hasUserMessageAfter);
}
