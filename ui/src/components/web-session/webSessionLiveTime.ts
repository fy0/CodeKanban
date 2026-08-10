import type {
  WebSessionApprovalState,
  WebSessionBlock,
  WebSessionLiveState,
  WebSessionUserInputState,
} from '@/stores/webSession';
import type { WebSessionSummary } from '@/types/models';

export interface WebSessionLiveTimeTooltipItem {
  key: string;
  label: string;
  value: string;
}

export interface WebSessionLiveTimeCopy {
  timeText: string;
  tooltipItems: WebSessionLiveTimeTooltipItem[];
}

export interface ResolveWebSessionLiveTimeCopyInput {
  state: WebSessionLiveState;
  blocks: WebSessionBlock[];
  session?: Pick<WebSessionSummary, 'activityAt' | 'workTiming'> | null;
  pendingApproval?: Pick<WebSessionApprovalState, 'requestedAt'> | null;
  pendingUserInput?: Pick<WebSessionUserInputState, 'requestedAt'> | null;
  now: number;
  labels: {
    startedAt: string;
    elapsed: string;
    sinceLastActivity: string;
  };
  formatTime: (timestamp: number) => string;
  formatDateTime: (timestamp: number) => string;
}

function isValidTimestamp(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0;
}

function getEndedAt(state: WebSessionLiveState, now: number) {
  return state.running ? now : state.updatedAt;
}

export function formatElapsedDuration(startedAt: number, endedAt: number) {
  const diff = Math.max(0, endedAt - startedAt);
  const totalSeconds = Math.floor(diff / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
  }

  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function isTurnOriginBlock(block: WebSessionBlock) {
  return (
    block.kind === 'user' ||
    block.detail?.type === 'approval_response' ||
    block.detail?.type === 'user_input_response'
  );
}

function getTurnStartedAt(
  state: WebSessionLiveState,
  blocks: WebSessionBlock[],
  endedAt: number
): number | undefined {
  let latestOriginRunId = '';
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index];
    const timestamp = block.observedAt ?? block.timestamp;
    if (!isValidTimestamp(timestamp) || timestamp > endedAt || !isTurnOriginBlock(block)) {
      continue;
    }
    latestOriginRunId = String(block.runId ?? '').trim();
    break;
  }
  if (latestOriginRunId) {
    let earliestRunOrigin: number | undefined;
    for (const block of blocks) {
      const timestamp = block.observedAt ?? block.timestamp;
      if (
        !isValidTimestamp(timestamp) ||
        timestamp > endedAt ||
        String(block.runId ?? '').trim() !== latestOriginRunId ||
        !isTurnOriginBlock(block)
      ) {
        continue;
      }
      if (earliestRunOrigin == null || timestamp < earliestRunOrigin) {
        earliestRunOrigin = timestamp;
      }
    }
    if (earliestRunOrigin != null) {
      return earliestRunOrigin;
    }
  }
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index];
    const timestamp = block.observedAt ?? block.timestamp;
    if (!isValidTimestamp(timestamp) || timestamp > endedAt) {
      continue;
    }
    if (block.itemType === 'run_st') {
      return timestamp;
    }
  }
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index];
    const timestamp = block.observedAt ?? block.timestamp;
    if (!isValidTimestamp(timestamp) || timestamp > endedAt) {
      continue;
    }
    if (isTurnOriginBlock(block)) {
      return timestamp;
    }
  }
  return isValidTimestamp(state.startedAt) ? state.startedAt : undefined;
}

function getLastActivityAt(
  state: WebSessionLiveState,
  blocks: WebSessionBlock[],
  endedAt: number,
  session?: Pick<WebSessionSummary, 'activityAt'> | null,
  pendingApproval?: Pick<WebSessionApprovalState, 'requestedAt'> | null,
  pendingUserInput?: Pick<WebSessionUserInputState, 'requestedAt'> | null
): number | undefined {
  let latest: number | undefined;

  const consider = (value: unknown) => {
    if (!isValidTimestamp(value) || value > endedAt) {
      return;
    }
    if (latest == null || value > latest) {
      latest = value;
    }
  };

  for (const block of blocks) {
    consider(block.observedAt ?? block.timestamp);
  }

  consider(pendingApproval?.requestedAt);
  consider(pendingUserInput?.requestedAt);
  consider(Date.parse(session?.activityAt ?? ''));
  consider(state.updatedAt);

  return latest;
}

function getCurrentRunTiming(
  session: Pick<WebSessionSummary, 'workTiming'> | null | undefined,
  now: number
) {
  const currentRun = session?.workTiming?.currentRun;
  const startedAt = Date.parse(currentRun?.startedAt ?? '');
  if (!currentRun || !isValidTimestamp(startedAt)) {
    return null;
  }
  const pausedAt = Date.parse(currentRun.pausedAt ?? '');
  const endedAt = isValidTimestamp(pausedAt) ? Math.min(now, pausedAt) : now;
  const pausedDurationMs = Math.max(0, Number(currentRun.pausedDurationMs) || 0);
  return {
    startedAt,
    elapsedMs: Math.max(0, endedAt - startedAt - pausedDurationMs),
  };
}

export function resolveWebSessionLiveTimeCopy(
  input: ResolveWebSessionLiveTimeCopyInput
): WebSessionLiveTimeCopy {
  const endedAt = getEndedAt(input.state, input.now);
  const currentRunTiming = getCurrentRunTiming(input.session, input.now);
  const turnStartedAt =
    currentRunTiming?.startedAt ?? getTurnStartedAt(input.state, input.blocks, endedAt);
  const elapsedText = currentRunTiming
    ? formatElapsedDuration(0, currentRunTiming.elapsedMs)
    : turnStartedAt
      ? formatElapsedDuration(turnStartedAt, endedAt)
      : '';
  const lastActivityAt = getLastActivityAt(
    input.state,
    input.blocks,
    endedAt,
    input.session,
    input.pendingApproval,
    input.pendingUserInput
  );

  const tooltipItems: WebSessionLiveTimeTooltipItem[] = [];
  if (turnStartedAt) {
    tooltipItems.push({
      key: 'started-at',
      label: input.labels.startedAt,
      value: input.formatDateTime(turnStartedAt),
    });
    tooltipItems.push({
      key: 'elapsed',
      label: input.labels.elapsed,
      value: elapsedText,
    });
  }
  if (lastActivityAt) {
    tooltipItems.push({
      key: 'since-last-activity',
      label: input.labels.sinceLastActivity,
      value: formatElapsedDuration(lastActivityAt, endedAt),
    });
  }

  if (turnStartedAt) {
    return {
      timeText: elapsedText,
      tooltipItems,
    };
  }

  return {
    timeText: input.formatTime(input.state.updatedAt),
    tooltipItems:
      tooltipItems.length > 0
        ? tooltipItems
        : isValidTimestamp(input.state.updatedAt)
          ? [
              {
                key: 'updated-at',
                label: input.labels.startedAt,
                value: input.formatDateTime(input.state.updatedAt),
              },
            ]
          : [],
  };
}
