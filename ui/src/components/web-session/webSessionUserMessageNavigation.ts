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
  getLoadStateKey: () => string;
  loadEarlier: () => Promise<void>;
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
  return currentIsLoadedUserMessage && input.direction === 'previous' && input.canLoadEarlier;
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
    if (adjacentKey || options.direction === 'next' || !options.canLoadEarlier()) {
      return adjacentKey;
    }

    const previousLoadStateKey = options.getLoadStateKey();
    await options.loadEarlier();
    if (options.getLoadStateKey() === previousLoadStateKey) {
      return null;
    }
  }
}
