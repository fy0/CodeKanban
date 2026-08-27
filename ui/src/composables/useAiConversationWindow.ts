import { computed, ref } from 'vue';
import type { ConversationWindowState } from '@/components/common/ConversationViewer.vue';
import type { ConversationWindowResponse as ConversationWindowModel } from '@/types/models';

type ConversationWindowLoader = (options?: {
  beforeCursor?: string;
  limit?: number;
}) => Promise<ConversationWindowModel>;

const DEFAULT_LIMIT = 80;

function mergeWindowMessages(
  previous: ConversationWindowModel | null,
  incoming: ConversationWindowModel
): ConversationWindowModel {
  if (!previous || incoming.windowEnd >= previous.windowEnd) {
    return {
      ...incoming,
      messages: [...incoming.messages],
    };
  }

  return {
    ...incoming,
    windowEnd: previous.windowEnd,
    messages: [...incoming.messages, ...previous.messages],
  };
}

export function useAiConversationWindow(
  loader: ConversationWindowLoader,
  refreshLoader?: ((limit?: number) => Promise<ConversationWindowModel>) | null
) {
  const loading = ref(false);
  const loadingEarlier = ref(false);
  const refreshing = ref(false);
  const conversation = ref<ConversationWindowModel | null>(null);

  const messages = computed(() => conversation.value?.messages ?? []);
  const title = computed(() => conversation.value?.title ?? '');
  const windowState = computed<ConversationWindowState | null>(() => {
    if (!conversation.value) {
      return null;
    }
    return {
      totalMessages: conversation.value.total,
      totalUserMessages: conversation.value.totalUserMessages,
      userMessagesBeforeWindow: conversation.value.userMessagesBeforeWindow,
      windowStart: conversation.value.windowStart,
      hasMoreBefore: conversation.value.hasMoreBefore,
    };
  });

  async function load(limit = DEFAULT_LIMIT) {
    loading.value = true;
    try {
      conversation.value = await loader({ limit });
      return conversation.value;
    } finally {
      loading.value = false;
    }
  }

  async function loadEarlier(limit = DEFAULT_LIMIT) {
    const current = conversation.value;
    if (!current?.hasMoreBefore || loadingEarlier.value) {
      return current;
    }
    loadingEarlier.value = true;
    try {
      const earlier = await loader({
        beforeCursor: current.beforeCursor || String(current.windowStart),
        limit,
      });
      conversation.value = mergeWindowMessages(current, earlier);
      return conversation.value;
    } finally {
      loadingEarlier.value = false;
    }
  }

  async function refresh(limit = DEFAULT_LIMIT) {
    if (!refreshLoader) {
      return load(limit);
    }
    refreshing.value = true;
    try {
      conversation.value = await refreshLoader(limit);
      return conversation.value;
    } finally {
      refreshing.value = false;
    }
  }

  function reset() {
    conversation.value = null;
    loading.value = false;
    loadingEarlier.value = false;
    refreshing.value = false;
  }

  return {
    conversation,
    messages,
    title,
    windowState,
    loading,
    loadingEarlier,
    refreshing,
    load,
    loadEarlier,
    refresh,
    reset,
  };
}

export { DEFAULT_LIMIT as AI_CONVERSATION_WINDOW_LIMIT };
