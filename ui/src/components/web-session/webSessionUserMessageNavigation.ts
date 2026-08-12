export type WebSessionUserMessageNavigationDirection = 'previous' | 'next';

export interface WebSessionUserMessageNavigationBlock {
  key: string;
  kind: string;
}

export interface ResolveWebSessionUserMessageTargetOptions {
  currentKey: string;
  direction: WebSessionUserMessageNavigationDirection;
  getBlocks: () => readonly WebSessionUserMessageNavigationBlock[];
  canLoadEarlier: () => boolean;
  canLoadLater?: () => boolean;
  getLoadStateKey: () => string;
  loadEarlier: () => Promise<void>;
  loadLater?: () => Promise<void>;
}

export interface WebSessionViewportUserMessageCandidate {
  key: string;
  top: number;
}

export interface WebSessionTimelineStartConfirmationState {
  sessionId: string;
  expiresAt: number;
}

export function resolveWebSessionTimelineStartConfirmation(input: {
  sessionId: string;
  currentState?: WebSessionTimelineStartConfirmationState | null;
  now: number;
  ttlMs: number;
}) {
  const confirmed =
    input.currentState?.sessionId === input.sessionId && input.currentState.expiresAt > input.now;
  if (confirmed) {
    return {
      shouldProceed: true,
      nextState: null,
    };
  }
  return {
    shouldProceed: false,
    nextState: {
      sessionId: input.sessionId,
      expiresAt: input.now + Math.max(0, Math.trunc(input.ttlMs)),
    },
  };
}

function getUserMessageBlocks(blocks: readonly WebSessionUserMessageNavigationBlock[]) {
  return blocks.filter(block => block.kind === 'user');
}

export function findAdjacentWebSessionUserMessageKey(
  blocks: readonly WebSessionUserMessageNavigationBlock[],
  currentKey: string,
  direction: WebSessionUserMessageNavigationDirection
) {
  const userMessages = getUserMessageBlocks(blocks);
  const currentIndex = userMessages.findIndex(block => block.key === currentKey);
  if (currentIndex < 0) {
    return null;
  }

  const targetIndex = direction === 'previous' ? currentIndex - 1 : currentIndex + 1;
  return userMessages[targetIndex]?.key ?? null;
}

export function canNavigateWebSessionUserMessage(input: {
  blocks: readonly WebSessionUserMessageNavigationBlock[];
  currentKey: string;
  direction: WebSessionUserMessageNavigationDirection;
  canLoadEarlier: boolean;
  canLoadLater?: boolean;
}) {
  const adjacentKey = findAdjacentWebSessionUserMessageKey(
    input.blocks,
    input.currentKey,
    input.direction
  );
  if (adjacentKey) {
    return true;
  }

  const currentIsLoadedUserMessage = input.blocks.some(
    block => block.kind === 'user' && block.key === input.currentKey
  );
  return (
    currentIsLoadedUserMessage &&
    (input.direction === 'previous' ? input.canLoadEarlier : Boolean(input.canLoadLater))
  );
}

export function findViewportAdjacentWebSessionUserMessageKey(
  candidates: readonly WebSessionViewportUserMessageCandidate[],
  viewportTop: number,
  direction: WebSessionUserMessageNavigationDirection,
  epsilon = 1
) {
  const sorted = [...candidates].sort((left, right) => left.top - right.top);
  if (direction === 'next') {
    return sorted.find(candidate => candidate.top > viewportTop + epsilon)?.key ?? null;
  }
  for (let index = sorted.length - 1; index >= 0; index -= 1) {
    if (sorted[index].top < viewportTop - epsilon) {
      return sorted[index].key;
    }
  }
  return null;
}

export async function resolveWebSessionUserMessageTarget(
  options: ResolveWebSessionUserMessageTargetOptions
) {
  while (true) {
    const adjacentKey = findAdjacentWebSessionUserMessageKey(
      options.getBlocks(),
      options.currentKey,
      options.direction
    );
    if (adjacentKey) {
      return adjacentKey;
    }

    const canLoad =
      options.direction === 'previous'
        ? options.canLoadEarlier()
        : Boolean(options.canLoadLater?.());
    const load = options.direction === 'previous' ? options.loadEarlier : options.loadLater;
    if (!canLoad || !load) {
      return null;
    }

    const previousLoadStateKey = options.getLoadStateKey();
    await load();
    if (options.getLoadStateKey() === previousLoadStateKey) {
      return null;
    }
  }
}
