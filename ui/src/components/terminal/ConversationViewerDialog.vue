<template>
  <n-modal
    v-model:show="showModal"
    preset="card"
    :title="modalTitle"
    style="width: 800px; max-width: 90vw; max-height: 85vh"
    :mask-closable="true"
    :closable="true"
    @close="handleClose"
  >
    <template #header-extra>
      <n-space :size="6" align="center">
        <n-tooltip>
          <template #trigger>
            <n-button
              quaternary
              circle
              size="small"
              :disabled="!navState.hasPrev"
              @click="viewerRef?.goToPrevUserMessage()"
            >
              <template #icon>
                <n-icon><ChevronUpOutline /></n-icon>
              </template>
            </n-button>
          </template>
          {{ t('terminal.prevUserMessage') }}
        </n-tooltip>
        <span class="conversation-nav-indicator">
          {{
            t('terminal.userMessagePosition', {
              current: navState.currentUserPosition,
              total: navState.totalUserMessages,
            })
          }}
        </span>
        <n-tooltip>
          <template #trigger>
            <n-button
              quaternary
              circle
              size="small"
              :disabled="!navState.hasNext"
              @click="viewerRef?.goToNextUserMessage()"
            >
              <template #icon>
                <n-icon><ChevronDownOutline /></n-icon>
              </template>
            </n-button>
          </template>
          {{ t('terminal.nextUserMessage') }}
        </n-tooltip>
      </n-space>
    </template>
    <ConversationViewer
      ref="viewerRef"
      :messages="messages"
      :loading="loading"
      :loading-earlier="loadingEarlier"
      :conversation-window="windowState"
      :session-info="sessionInfo"
      :load-tool-result="loadToolResult"
      :load-earlier="loadEarlier"
      @nav-state-change="updateNavState"
    />
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { ChevronDownOutline, ChevronUpOutline } from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import { aiSessionApi } from '@/api/aiSession';
import {
  AI_CONVERSATION_WINDOW_LIMIT,
  useAiConversationWindow,
} from '@/composables/useAiConversationWindow';
import ConversationViewer, {
  type ConversationViewerNavState,
  type SessionInfo,
} from '@/components/common/ConversationViewer.vue';

const props = defineProps<{
  sessionId: string | null;
}>();

const showModal = defineModel<boolean>('show', { default: false });

const { t } = useLocale();
const message = useMessage();

const viewerRef = ref<{
  goToPrevUserMessage: () => void;
  goToNextUserMessage: () => void;
  syncNavigationState?: () => void;
} | null>(null);
const navState = ref<ConversationViewerNavState>({
  currentUserPosition: 0,
  totalUserMessages: 0,
  hasPrev: false,
  hasNext: false,
});
const {
  conversation,
  messages,
  title,
  windowState,
  loading,
  loadingEarlier,
  load,
  loadEarlier,
  reset,
} = useAiConversationWindow(
  options => aiSessionApi.conversationWindowBySessionID(props.sessionId || '', options),
  null
);

const modalTitle = computed(() => title.value || t('terminal.viewConversation'));

const sessionInfo = computed<SessionInfo | null>(() => {
  if (!props.sessionId) return null;
  return {
    sessionId: props.sessionId,
  };
});

watch(
  () => [showModal.value, props.sessionId],
  async ([show, sessionId]) => {
    if (show && sessionId) {
      await load(AI_CONVERSATION_WINDOW_LIMIT);
    }
  },
  { immediate: true }
);

async function loadToolResult(toolUseId: string) {
  const sessionId = props.sessionId;
  if (!sessionId || !toolUseId) return null;

  try {
    const content = await aiSessionApi.toolResultBySessionID(sessionId, toolUseId);
    if (!content || !conversation.value) return null;

    const msg = conversation.value.messages.find(m => m.toolUseId === toolUseId);
    if (msg) {
      msg.full = content;
    }
    return content;
  } catch (error) {
    console.error('Failed to load tool result:', error);
    message.error(t('terminal.loadConversationFailed'));
    return null;
  }
}

function updateNavState(value: ConversationViewerNavState) {
  navState.value = value;
}

function handleClose() {
  showModal.value = false;
  reset();
  navState.value = {
    currentUserPosition: 0,
    totalUserMessages: 0,
    hasPrev: false,
    hasNext: false,
  };
}
</script>

<style scoped>
.conversation-nav-indicator {
  min-width: 52px;
  text-align: center;
  font-size: 12px;
  color: var(--n-text-color-3);
}
</style>
