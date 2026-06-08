import type { WebSessionLiveState, WebSessionLiveSubAgent } from '@/stores/webSession';

export interface WebSessionLiveSubAgentCopyInput {
  phase: WebSessionLiveState['phase'];
  activeSubAgents: WebSessionLiveSubAgent[];
  activeSubAgentCount?: number;
}

export interface WebSessionLiveSubAgentCopyResult {
  hasActiveSubAgents: boolean;
  count: number;
  labelKey: 'webSession.liveSubAgentCount' | null;
  labelParams?: { count: number };
  detail: string;
}

function normalizeCount(input: WebSessionLiveSubAgentCopyInput) {
  const count = Number(input.activeSubAgentCount ?? input.activeSubAgents.length);
  return Number.isFinite(count) && count > 0 ? Math.max(0, Math.trunc(count)) : 0;
}

function summarizeAgents(agents: WebSessionLiveSubAgent[]) {
  return agents
    .map(agent => String(agent.summary || agent.title || '').trim())
    .filter(Boolean)
    .slice(0, 3)
    .join(' · ');
}

export function resolveWebSessionLiveSubAgentCopy(
  input: WebSessionLiveSubAgentCopyInput
): WebSessionLiveSubAgentCopyResult {
  const count = normalizeCount(input);
  if (count <= 0) {
    return {
      hasActiveSubAgents: false,
      count: 0,
      labelKey: null,
      detail: '',
    };
  }

  const firstAgent = input.activeSubAgents[0];
  if (count === 1 && firstAgent?.summary?.trim()) {
    return {
      hasActiveSubAgents: true,
      count,
      labelKey: 'webSession.liveSubAgentCount',
      labelParams: { count },
      detail: firstAgent.summary.trim(),
    };
  }

  return {
    hasActiveSubAgents: true,
    count,
    labelKey: 'webSession.liveSubAgentCount',
    labelParams: { count },
    detail: summarizeAgents(input.activeSubAgents),
  };
}
