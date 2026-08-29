import {
  computed,
  nextTick,
  onMounted,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue';
import { NInput } from 'naive-ui';
import { webSessionApi, type SessionConversationSearchMatch } from '@/api/webSession';
import type { WebSessionBlock } from '@/stores/webSession';
import {
  findWebSessionConversationSearchMatches,
  matchesWebSessionConversationSearchTarget,
  mergeWebSessionConversationSearchMatches,
  resolveWebSessionConversationSearchMatchIndex,
  shouldHandleWebSessionConversationSearchShortcut,
  type WebSessionConversationSearchFilters,
  type WebSessionConversationSearchMatch,
} from '@/components/web-session/webSessionConversationSearch';

const REMOTE_SEARCH_DELAY_MS = 3000;

type Translate = (key: string) => string;
type SearchSession = {
  id: string;
  projectId: string;
};

export type WebSessionConversationSearchOptions = {
  currentSession: Readonly<Ref<SearchSession | null>>;
  visibleBlocks: Readonly<Ref<WebSessionBlock[]>>;
  allBlocks: Readonly<Ref<WebSessionBlock[]>>;
  isActive: MaybeRefOrGetter<boolean>;
  translate: Translate;
  onOpen: () => void;
  loadEarlierHistory: (sessionId: string) => Promise<boolean>;
  scrollToBlock: (blockKey: string) => void;
};

export function useWebSessionConversationSearch({
  currentSession,
  visibleBlocks,
  allBlocks,
  isActive,
  translate,
  onOpen,
  loadEarlierHistory,
  scrollToBlock,
}: WebSessionConversationSearchOptions) {
  const inputRef = ref<InstanceType<typeof NInput> | null>(null);
  const openState = ref(false);
  const query = ref('');
  const filters = ref<WebSessionConversationSearchFilters>({
    user: true,
    assistant: true,
    tools: false,
    system: false,
  });
  const remoteMatches = ref<SessionConversationSearchMatch[]>([]);
  const currentIndex = ref(0);
  const remoteLoading = ref(false);
  const remoteError = ref(false);
  let requestVersion = 0;
  let abortController: AbortController | null = null;
  let searchTimer: number | null = null;

  function createMatchFromBlock(block: WebSessionBlock): WebSessionConversationSearchMatch {
    return {
      key: block.key,
      id: block.id,
      sourceThreadId: block.sourceThreadId ?? undefined,
      sourceTurnId: block.sourceTurnId ?? undefined,
      sourceItemId: block.sourceItemId ?? undefined,
      orderIndex: block.orderIndex,
      kind: block.kind,
      toolId: block.tool?.id,
      commandGroupId: block.tool?.commandGroup?.id,
    };
  }

  const visibleRemoteMatches = computed(() =>
    remoteMatches.value.map(match => {
      const visibleBlock = visibleBlocks.value.find(block =>
        matchesWebSessionConversationSearchTarget(block, match)
      );
      return visibleBlock ? createMatchFromBlock(visibleBlock) : match;
    })
  );
  const normalizedQuery = computed(() => String(query.value ?? '').trim());
  const localMatches = computed(() =>
    findWebSessionConversationSearchMatches(
      visibleBlocks.value,
      normalizedQuery.value,
      filters.value
    )
  );
  const matches = computed(() =>
    mergeWebSessionConversationSearchMatches(localMatches.value, visibleRemoteMatches.value)
  );
  const currentMatch = computed(() => matches.value[currentIndex.value] ?? null);
  const hasPrevious = computed(() => currentIndex.value > 0);
  const hasNext = computed(() => currentIndex.value < matches.value.length - 1);
  const hasNonDefaultFilters = computed(() => {
    const currentFilters = filters.value;
    return (
      !currentFilters.user ||
      !currentFilters.assistant ||
      currentFilters.tools ||
      currentFilters.system
    );
  });
  const resultLabel = computed(() => {
    const total = matches.value.length;
    if (total === 0) {
      if (remoteError.value) {
        return translate('webSession.conversationSearchFailed');
      }
      return remoteLoading.value
        ? translate('webSession.conversationSearchSearching')
        : translate('webSession.conversationSearchNoResults');
    }
    const suffix = remoteLoading.value
      ? ` (${translate('webSession.conversationSearchSearching')})`
      : remoteError.value
        ? ` (${translate('webSession.conversationSearchFailed')})`
        : '';
    return `${currentIndex.value + 1} / ${total}${suffix}`;
  });
  const visibleState = computed(() => {
    const activeMatch = currentMatch.value;
    const state = new Map<string, { match: boolean; active: boolean }>();
    for (const block of visibleBlocks.value) {
      const matching = matches.value.some(match =>
        matchesWebSessionConversationSearchTarget(block, match)
      );
      if (!matching) {
        continue;
      }
      state.set(block.key, {
        match: true,
        active: Boolean(
          activeMatch && matchesWebSessionConversationSearchTarget(block, activeMatch)
        ),
      });
    }
    return state;
  });

  function clearTimer() {
    if (searchTimer != null && typeof window !== 'undefined') {
      window.clearTimeout(searchTimer);
    }
    searchTimer = null;
  }

  function invalidateRequest() {
    requestVersion += 1;
    abortController?.abort();
    abortController = null;
  }

  function clearRemoteState() {
    invalidateRequest();
    remoteMatches.value = [];
    remoteLoading.value = false;
    remoteError.value = false;
  }

  function open() {
    onOpen();
    openState.value = true;
    void nextTick().then(() => inputRef.value?.focus?.());
  }

  function close() {
    openState.value = false;
    query.value = '';
    filters.value = {
      user: true,
      assistant: true,
      tools: false,
      system: false,
    };
    currentIndex.value = 0;
    clearTimer();
    clearRemoteState();
  }

  function resetForSessionChange() {
    currentIndex.value = 0;
    clearTimer();
    clearRemoteState();
  }

  function handleShortcut(event: KeyboardEvent) {
    if (
      !toValue(isActive) ||
      !currentSession.value ||
      !shouldHandleWebSessionConversationSearchShortcut(event)
    ) {
      return;
    }
    event.preventDefault();
    open();
  }

  function filtersEqual(
    left: WebSessionConversationSearchFilters,
    right: WebSessionConversationSearchFilters
  ) {
    return (
      left.user === right.user &&
      left.assistant === right.assistant &&
      left.tools === right.tools &&
      left.system === right.system
    );
  }

  function requestIsCurrent(
    version: number,
    sessionId: string,
    requestedQuery: string,
    requestedFilters: WebSessionConversationSearchFilters
  ) {
    return (
      version === requestVersion &&
      openState.value &&
      currentSession.value?.id === sessionId &&
      normalizedQuery.value === requestedQuery &&
      filtersEqual(filters.value, requestedFilters)
    );
  }

  function isAbortLikeError(error: unknown) {
    return Boolean(
      error &&
        typeof error === 'object' &&
        'name' in error &&
        String((error as { name?: unknown }).name || '') === 'AbortError'
    );
  }

  function findBlock(match: WebSessionConversationSearchMatch) {
    const exact = visibleBlocks.value.find(block =>
      matchesWebSessionConversationSearchTarget(block, match)
    );
    if (exact) {
      return exact;
    }
    if (!allBlocks.value.some(block => matchesWebSessionConversationSearchTarget(block, match))) {
      return null;
    }
    return visibleBlocks.value.reduce<WebSessionBlock | null>((closest, block) => {
      if (!closest) {
        return block;
      }
      return Math.abs(block.orderIndex - match.orderIndex) <
        Math.abs(closest.orderIndex - match.orderIndex)
        ? block
        : closest;
    }, null);
  }

  async function ensureMatchLoaded(match: WebSessionConversationSearchMatch) {
    const session = currentSession.value;
    if (!session) {
      return;
    }
    while (
      !allBlocks.value.some(block => matchesWebSessionConversationSearchTarget(block, match))
    ) {
      if (!(await loadEarlierHistory(session.id))) {
        return;
      }
    }
  }

  async function locateMatch(match: WebSessionConversationSearchMatch) {
    await ensureMatchLoaded(match);
    await nextTick();
    const block = findBlock(match);
    if (block) {
      scrollToBlock(block.key);
    }
  }

  async function loadRemote() {
    const session = currentSession.value;
    const requestedQuery = normalizedQuery.value;
    const requestedFilters = { ...filters.value };
    if (
      !openState.value ||
      !session ||
      !requestedQuery ||
      !Object.values(requestedFilters).some(Boolean)
    ) {
      return;
    }

    invalidateRequest();
    const version = requestVersion;
    const controller = new AbortController();
    abortController = controller;
    remoteMatches.value = [];
    remoteLoading.value = true;
    remoteError.value = false;
    let locateInitialRemoteMatch = matches.value.length === 0;

    try {
      let cursor = '';
      while (true) {
        const result = await webSessionApi.searchConversation(
          session.projectId,
          session.id,
          {
            query: requestedQuery,
            includeUser: requestedFilters.user,
            includeAssistant: requestedFilters.assistant,
            includeTools: requestedFilters.tools,
            includeSystem: requestedFilters.system,
            cursor: cursor || undefined,
            limit: 100,
          },
          { signal: controller.signal }
        );
        if (!requestIsCurrent(version, session.id, requestedQuery, requestedFilters)) {
          return;
        }

        remoteMatches.value = mergeWebSessionConversationSearchMatches(
          [],
          [...remoteMatches.value, ...result.items]
        );
        if (locateInitialRemoteMatch && remoteMatches.value.length > 0) {
          locateInitialRemoteMatch = false;
          await nextTick();
          const latestMatchIndex = matches.value.length - 1;
          const latestMatch = matches.value[latestMatchIndex];
          if (latestMatch) {
            currentIndex.value = latestMatchIndex;
            await locateMatch(latestMatch);
          }
        }
        if (result.done || !result.nextCursor) {
          break;
        }
        cursor = result.nextCursor;
      }
    } catch (error) {
      if (
        !requestIsCurrent(version, session.id, requestedQuery, requestedFilters) ||
        isAbortLikeError(error)
      ) {
        return;
      }
      remoteError.value = true;
      console.error('[Web Session] Failed to search session conversation', error);
    } finally {
      if (abortController === controller) {
        abortController = null;
        remoteLoading.value = false;
      }
    }
  }

  function scheduleRemoteSearch() {
    clearTimer();
    if (
      !openState.value ||
      !normalizedQuery.value ||
      !currentSession.value ||
      !Object.values(filters.value).some(Boolean) ||
      typeof window === 'undefined'
    ) {
      return;
    }
    searchTimer = window.setTimeout(() => {
      searchTimer = null;
      void loadRemote();
    }, REMOTE_SEARCH_DELAY_MS);
  }

  async function navigate(direction: 'previous' | 'next') {
    if (matches.value.length === 0) {
      return;
    }
    const nextIndex = currentIndex.value + (direction === 'previous' ? -1 : 1);
    if (nextIndex < 0 || nextIndex >= matches.value.length) {
      return;
    }
    currentIndex.value = nextIndex;
    const match = matches.value[nextIndex];
    if (match) {
      await locateMatch(match);
    }
  }

  function selectPage(page: number) {
    const nextIndex = Math.trunc(page) - 1;
    const match = matches.value[nextIndex];
    if (!match) {
      return;
    }
    currentIndex.value = nextIndex;
    void locateMatch(match);
  }

  function isBlockMatch(block: WebSessionBlock) {
    return visibleState.value.get(block.key)?.match === true;
  }

  function isBlockActive(block: WebSessionBlock) {
    return visibleState.value.get(block.key)?.active === true;
  }

  watch(
    [query, filters],
    () => {
      currentIndex.value = 0;
      clearTimer();
      clearRemoteState();
      if (openState.value && normalizedQuery.value) {
        void nextTick().then(() => {
          const latestMatchIndex = matches.value.length - 1;
          const latestMatch = matches.value[latestMatchIndex];
          if (latestMatch) {
            currentIndex.value = latestMatchIndex;
            void locateMatch(latestMatch);
          }
        });
        scheduleRemoteSearch();
      }
    },
    { deep: true }
  );

  watch(matches, (nextMatches, previousMatches) => {
    if (nextMatches.length === 0) {
      currentIndex.value = 0;
      return;
    }
    const previousMatch = previousMatches[currentIndex.value] ?? null;
    currentIndex.value = resolveWebSessionConversationSearchMatchIndex(
      nextMatches,
      previousMatch,
      currentIndex.value
    );
  });

  onMounted(() => window.addEventListener('keydown', handleShortcut));
  onScopeDispose(() => {
    window.removeEventListener('keydown', handleShortcut);
    clearTimer();
    invalidateRequest();
  });

  return {
    inputRef,
    openState,
    query,
    filters,
    currentIndex,
    matches,
    hasPrevious,
    hasNext,
    hasNonDefaultFilters,
    resultLabel,
    normalizedQuery,
    open,
    close,
    resetForSessionChange,
    scheduleRemoteSearch,
    navigate,
    selectPage,
    isBlockMatch,
    isBlockActive,
  };
}
