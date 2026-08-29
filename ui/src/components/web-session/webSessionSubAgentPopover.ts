import type { WebSessionLiveSubAgent, WebSessionSubAgent } from '@/stores/webSession';

export const WEB_SESSION_SUB_AGENT_RECENT_LIMIT = 10;

export interface WebSessionSubAgentPopoverModel {
  items: WebSessionSubAgent[];
  activeIds: Set<string>;
  recentCount: number;
}

function subAgentActivityTime(agent: WebSessionSubAgent) {
  return agent.lastActivityAt ?? agent.endedAt ?? agent.startedAt ?? 0;
}

function compareRecentSubAgents(left: WebSessionSubAgent, right: WebSessionSubAgent) {
  const activityDifference = subAgentActivityTime(right) - subAgentActivityTime(left);
  if (activityDifference !== 0) {
    return activityDifference;
  }
  if (left.latestOrderIndex !== right.latestOrderIndex) {
    return right.latestOrderIndex - left.latestOrderIndex;
  }
  return left.id.localeCompare(right.id);
}

export function resolveWebSessionSubAgentPopover(
  knownSubAgents: readonly WebSessionSubAgent[],
  activeSubAgents: readonly WebSessionLiveSubAgent[],
  recentLimit = WEB_SESSION_SUB_AGENT_RECENT_LIMIT
): WebSessionSubAgentPopoverModel {
  const knownById = new Map(knownSubAgents.map(agent => [agent.id, agent] as const));
  const activeIds = new Set<string>();
  const activeItems: WebSessionSubAgent[] = [];

  activeSubAgents.forEach(agent => {
    if (activeIds.has(agent.id)) {
      return;
    }
    activeIds.add(agent.id);
    const knownAgent = knownById.get(agent.id);
    if (knownAgent) {
      activeItems.push(knownAgent);
    }
  });

  const normalizedLimit = Number.isFinite(recentLimit)
    ? Math.max(0, Math.trunc(recentLimit))
    : WEB_SESSION_SUB_AGENT_RECENT_LIMIT;
  const recentItems = [...knownById.values()]
    .filter(agent => !activeIds.has(agent.id))
    .sort(compareRecentSubAgents)
    .slice(0, normalizedLimit);

  return {
    items: [...activeItems, ...recentItems],
    activeIds,
    recentCount: recentItems.length,
  };
}
