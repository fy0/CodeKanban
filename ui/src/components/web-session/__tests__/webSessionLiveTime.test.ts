import { describe, expect, it } from 'vitest';

import { resolveWebSessionLiveTimeCopy } from '@/components/web-session/webSessionLiveTime';
import type { WebSessionBlock, WebSessionLiveState } from '@/stores/webSession';

function block(input: Partial<WebSessionBlock> & Pick<WebSessionBlock, 'kind'>): WebSessionBlock {
  return {
    key: String(input.key ?? `${input.kind}-${input.timestamp ?? 0}`),
    id: String(input.id ?? input.key ?? `${input.kind}-${input.timestamp ?? 0}`),
    sourceTurnId: input.sourceTurnId ?? null,
    sourceItemId: input.sourceItemId ?? null,
    orderIndex: Number(input.orderIndex ?? 0),
    kind: input.kind,
    itemType: String(input.itemType ?? ''),
    text: String(input.text ?? ''),
    timestamp: Number(input.timestamp ?? 0),
    observedAt: input.observedAt ?? null,
    attachments: input.attachments ?? [],
    tool: input.tool,
    level: input.level,
    done: input.done,
    detail: input.detail,
    payload: input.payload,
  };
}

function liveState(input: Partial<WebSessionLiveState>): WebSessionLiveState {
  return {
    phase: input.phase ?? 'starting',
    running: input.running ?? true,
    updatedAt: input.updatedAt ?? 0,
    startedAt: input.startedAt,
    tool: input.tool,
    activeSubAgents: input.activeSubAgents,
    activeSubAgentCount: input.activeSubAgentCount,
    approval: input.approval,
    userInput: input.userInput,
    errorMessage: input.errorMessage,
    retry: input.retry,
  };
}

function resolve(input: {
  state: WebSessionLiveState;
  blocks: WebSessionBlock[];
  now: number;
  activityAt?: string;
  pendingApprovalAt?: number;
  pendingUserInputAt?: number;
}) {
  return resolveWebSessionLiveTimeCopy({
    state: input.state,
    blocks: input.blocks,
    session: input.activityAt ? { activityAt: input.activityAt } : null,
    pendingApproval: input.pendingApprovalAt
      ? { requestedAt: input.pendingApprovalAt }
      : null,
    pendingUserInput: input.pendingUserInputAt
      ? { requestedAt: input.pendingUserInputAt }
      : null,
    now: input.now,
    labels: {
      startedAt: 'Started at',
      elapsed: 'Elapsed',
      sinceLastActivity: 'Since last activity',
    },
    formatTime: timestamp => `time:${timestamp}`,
    formatDateTime: timestamp => `dt:${timestamp}`,
  });
}

describe('webSessionLiveTime', () => {
  it('anchors started time and elapsed time to the latest user-originated turn start', () => {
    const result = resolve({
      state: liveState({
        phase: 'starting',
        running: true,
        updatedAt: 10_000,
        startedAt: 10_000,
      }),
      blocks: [block({ kind: 'user', timestamp: 10_000, orderIndex: 1, text: 'start' })],
      now: 61_000,
      activityAt: new Date(10_000).toISOString(),
    });

    expect(result.timeText).toBe('00:51');
    expect(result.tooltipItems).toEqual([
      { key: 'started-at', label: 'Started at', value: 'dt:10000' },
      { key: 'elapsed', label: 'Elapsed', value: '00:51' },
      { key: 'since-last-activity', label: 'Since last activity', value: '00:51' },
    ]);
  });

  it('keeps started time anchored to the user turn while resetting last activity on tool start', () => {
    const result = resolve({
      state: liveState({
        phase: 'tool',
        running: true,
        updatedAt: 10_001,
        startedAt: 10_000,
        tool: {
          id: 'tool-1',
          name: 'Bash',
          startedAt: 10_001,
        },
      }),
      blocks: [
        block({ kind: 'user', timestamp: 10_000, orderIndex: 1, text: 'do work' }),
        block({
          kind: 'tool',
          timestamp: 10_001,
          observedAt: 10_001,
          orderIndex: 2,
          tool: {
            id: 'tool-1',
            name: 'Bash',
            status: 'running',
            startedAt: 10_001,
          },
        }),
      ],
      now: 61_000,
      activityAt: new Date(10_001).toISOString(),
    });

    expect(result.timeText).toBe('00:51');
    expect(result.tooltipItems).toEqual([
      { key: 'started-at', label: 'Started at', value: 'dt:10000' },
      { key: 'elapsed', label: 'Elapsed', value: '00:51' },
      { key: 'since-last-activity', label: 'Since last activity', value: '00:50' },
    ]);
  });

  it('treats pending approval as the latest activity while preserving the turn start', () => {
    const result = resolve({
      state: liveState({
        phase: 'waiting_approval',
        running: true,
        updatedAt: 15_000,
        startedAt: 10_000,
      }),
      blocks: [block({ kind: 'user', timestamp: 10_000, orderIndex: 1, text: 'deploy' })],
      now: 61_000,
      pendingApprovalAt: 15_000,
      activityAt: new Date(15_000).toISOString(),
    });

    expect(result.tooltipItems).toEqual([
      { key: 'started-at', label: 'Started at', value: 'dt:10000' },
      { key: 'elapsed', label: 'Elapsed', value: '00:51' },
      { key: 'since-last-activity', label: 'Since last activity', value: '00:46' },
    ]);
  });

  it('uses session activityAt when hidden activity refreshed the runtime without a visible block', () => {
    const result = resolve({
      state: liveState({
        phase: 'thinking',
        running: true,
        updatedAt: 25_000,
        startedAt: 10_000,
      }),
      blocks: [block({ kind: 'user', timestamp: 10_000, orderIndex: 1, text: 'analyze' })],
      now: 61_000,
      activityAt: new Date(25_000).toISOString(),
    });

    expect(result.tooltipItems).toEqual([
      { key: 'started-at', label: 'Started at', value: 'dt:10000' },
      { key: 'elapsed', label: 'Elapsed', value: '00:51' },
      { key: 'since-last-activity', label: 'Since last activity', value: '00:36' },
    ]);
  });

  it('falls back to liveState.startedAt when no user-originated block exists', () => {
    const result = resolve({
      state: liveState({
        phase: 'error',
        running: false,
        updatedAt: 21_000,
        startedAt: 12_000,
      }),
      blocks: [
        block({
          kind: 'tool',
          timestamp: 18_000,
          orderIndex: 1,
          tool: {
            id: 'tool-1',
            name: 'Bash',
            status: 'error',
            startedAt: 18_000,
          },
        }),
      ],
      now: 99_000,
      activityAt: new Date(21_000).toISOString(),
    });

    expect(result.timeText).toBe('00:09');
    expect(result.tooltipItems).toEqual([
      { key: 'started-at', label: 'Started at', value: 'dt:12000' },
      { key: 'elapsed', label: 'Elapsed', value: '00:09' },
      { key: 'since-last-activity', label: 'Since last activity', value: '00:00' },
    ]);
  });
});
