import { describe, expect, it } from 'vitest';

import { resolveWebSessionLiveSubAgentCopy } from '@/components/web-session/webSessionLiveSubAgentSummary';

describe('webSessionLiveSubAgentSummary', () => {
  it('returns no sub-agent copy when none are active', () => {
    expect(
      resolveWebSessionLiveSubAgentCopy({
        phase: 'tool',
        activeSubAgents: [],
      })
    ).toEqual({
      hasActiveSubAgents: false,
      count: 0,
      labelKey: null,
      detail: '',
    });
  });

  it('uses the single sub-agent summary as runtime detail', () => {
    expect(
      resolveWebSessionLiveSubAgentCopy({
        phase: 'tool',
        activeSubAgents: [
          {
            id: 'agent-1',
            title: 'Research agent',
            summary: 'Inspect current sub-agent support',
            startedAt: 100,
          },
        ],
      })
    ).toEqual({
      hasActiveSubAgents: true,
      count: 1,
      labelKey: 'webSession.liveSubAgentCount',
      labelParams: { count: 1 },
      detail: 'Inspect current sub-agent support',
    });
  });

  it('joins multiple sub-agent summaries and respects the derived count', () => {
    expect(
      resolveWebSessionLiveSubAgentCopy({
        phase: 'tool',
        activeSubAgentCount: 3,
        activeSubAgents: [
          {
            id: 'agent-1',
            title: 'Research agent',
            summary: 'Inspect current sub-agent support',
            startedAt: 100,
          },
          {
            id: 'agent-2',
            title: 'Refactor agent',
            summary: 'Update timeout filtering',
            startedAt: 110,
          },
          {
            id: 'agent-3',
            title: 'Test agent',
            summary: '',
            startedAt: 120,
          },
        ],
      })
    ).toEqual({
      hasActiveSubAgents: true,
      count: 3,
      labelKey: 'webSession.liveSubAgentCount',
      labelParams: { count: 3 },
      detail: 'Inspect current sub-agent support · Update timeout filtering · Test agent',
    });
  });
});
