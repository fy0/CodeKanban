<template>
  <n-modal
    v-model:show="showModal"
    preset="card"
    :title="t('webSession.importCodexSessionTitle')"
    style="width: 760px; max-width: 94vw; max-height: 84vh"
    :mask-closable="true"
    :closable="true"
    @close="handleClose"
  >
    <template #header-extra>
      <n-button quaternary circle size="small" :loading="loading" @click="loadSources(false)">
        <template #icon>
          <n-icon><RefreshOutline /></n-icon>
        </template>
      </n-button>
    </template>

    <n-spin :show="loading && importSources.length === 0">
      <div class="web-session-import">
        <n-input
          v-model:value="searchQuery"
          size="small"
          clearable
          :placeholder="t('webSession.importCodexSessionSearchPlaceholder')"
        >
          <template #prefix>
            <n-icon><SearchOutline /></n-icon>
          </template>
        </n-input>

        <div class="web-session-import__toolbar">
          <n-checkbox v-model:checked="hideImported">
            {{ t('webSession.importCodexSessionHideImported') }}
          </n-checkbox>
          <span class="web-session-import__summary">
            {{
              t('webSession.importCodexSessionSummary', {
                ready: importableCount,
                duplicated: duplicateCount,
              })
            }}
          </span>
        </div>

        <div v-if="scanPhase !== 'complete'" class="web-session-import__scanning">
          {{ t('webSession.importCodexSessionScanning') }}
        </div>

        <div class="web-session-import__list">
          <template v-if="filteredSources.length > 0">
            <div
              v-for="source in filteredSources"
              :key="`${source.agent}:${source.sessionId}`"
              class="web-session-import__item"
            >
              <div class="web-session-import__copy">
                <div class="web-session-import__title-row">
                  <div class="web-session-import__title">
                    {{ source.title || t('terminal.untitledSession') }}
                  </div>
                  <div class="web-session-import__badges">
                    <n-tag size="small" :type="source.agent === 'pi' ? 'warning' : 'success'">
                      {{ source.agent === 'pi' ? 'Pi' : 'Codex' }}
                    </n-tag>
                    <n-tag v-if="source.duplicate" size="small" type="warning">
                      {{ t('webSession.importCodexSessionDuplicated') }}
                    </n-tag>
                    <n-tag
                      v-if="source.existingSession?.archivedAt"
                      size="small"
                      type="default"
                      bordered
                    >
                      {{ t('webSession.archivedBadge') }}
                    </n-tag>
                  </div>
                </div>

                <div class="web-session-import__meta">
                  <span>{{ source.model || (source.agent === 'pi' ? 'Pi' : 'Codex') }}</span>
                  <span>{{ formatSessionTime(source) }}</span>
                  <span>{{ source.sessionId }}</span>
                </div>

                <div class="web-session-import__path" :title="source.filePath">
                  {{ source.filePath }}
                </div>

                <div
                  v-if="source.duplicate && source.existingSession"
                  class="web-session-import__duplicate"
                >
                  {{
                    t('webSession.importCodexSessionDuplicateHint', {
                      title: source.existingSession.title,
                    })
                  }}
                </div>
              </div>

              <div class="web-session-import__actions">
                <n-button
                  size="small"
                  secondary
                  :disabled="previewLoading && previewingSourceId !== source.sessionId"
                  :loading="previewLoading && previewingSourceId === source.sessionId"
                  @click="openPreview(source)"
                >
                  {{ t('terminal.viewConversation') }}
                </n-button>
                <n-button
                  v-if="source.duplicate && source.existingSession"
                  size="small"
                  type="primary"
                  @click="openExistingSession(source)"
                >
                  {{ t('webSession.importCodexSessionOpenExisting') }}
                </n-button>
              </div>
            </div>
          </template>
          <n-empty v-else :description="t('webSession.importCodexSessionEmpty')" />
        </div>
      </div>
    </n-spin>
  </n-modal>

  <n-modal
    v-model:show="showPreviewModal"
    preset="card"
    style="width: 920px; max-width: 96vw; max-height: 88vh"
    :mask-closable="true"
    :closable="true"
    @close="closePreview"
  >
    <template #header>
      <div class="web-session-import__preview-title" :title="previewSessionTitle">
        {{ previewSessionTitle }}
      </div>
    </template>
    <template #header-extra>
      <n-space :size="6" align="center">
        <n-tooltip>
          <template #trigger>
            <n-button
              quaternary
              circle
              size="small"
              :disabled="!conversationNavState.hasPrev"
              @click="conversationViewerRef?.goToPrevUserMessage()"
            >
              <template #icon>
                <n-icon><ChevronUpOutline /></n-icon>
              </template>
            </n-button>
          </template>
          {{ t('terminal.prevUserMessage') }}
        </n-tooltip>
        <span class="web-session-import__nav-indicator">
          {{
            t('terminal.userMessagePosition', {
              current: conversationNavState.currentUserPosition,
              total: conversationNavState.totalUserMessages,
            })
          }}
        </span>
        <n-tooltip>
          <template #trigger>
            <n-button
              quaternary
              circle
              size="small"
              :disabled="!conversationNavState.hasNext"
              @click="conversationViewerRef?.goToNextUserMessage()"
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

    <div
      v-if="previewSource?.duplicate && previewSource.existingSession"
      class="web-session-import__preview-note"
    >
      {{
        t('webSession.importCodexSessionPreviewDuplicateHint', {
          title: previewSource.existingSession.title,
        })
      }}
    </div>

    <ConversationViewer
      ref="conversationViewerRef"
      :messages="conversationMessages"
      :loading="previewLoading"
      :loading-earlier="previewLoadingEarlier"
      :conversation-window="conversationWindow"
      :session-info="previewSessionInfo"
      :load-earlier="loadEarlierConversationWindow"
      @nav-state-change="updateConversationNavState"
    />

    <template #footer>
      <n-space justify="end">
        <n-button @click="closePreview">{{ t('common.cancel') }}</n-button>
        <n-button
          v-if="previewSource?.duplicate && previewSource.existingSession"
          type="primary"
          :disabled="previewLoading"
          @click="openExistingSession(previewSource)"
        >
          {{ t('webSession.importCodexSessionOpenExisting') }}
        </n-button>
        <n-button
          v-else
          type="primary"
          :disabled="!previewSource || !previewSource.importable || previewLoading"
          :loading="pendingSessionId === previewSource?.sessionId"
          @click="emitImportFromPreview"
        >
          {{
            previewSource?.importable
              ? t('webSession.importCodexSessionAction')
              : t('webSession.importPiSessionUnavailable')
          }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import {
  ChevronDownOutline,
  ChevronUpOutline,
  RefreshOutline,
  SearchOutline,
} from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import { http } from '@/api/http';
import { aiSessionApi } from '@/api/aiSession';
import {
  AI_CONVERSATION_WINDOW_LIMIT,
  useAiConversationWindow,
} from '@/composables/useAiConversationWindow';
import type { WebSessionSummary } from '@/types/models';
import {
  countImportableWebSessionSources,
  normalizeWebSessionImportSources,
  type WebSessionImportSourceSummary,
  type WebSessionImportSourceWire,
} from '@/components/web-session/webSessionImportSources';
import ConversationViewer, {
  type ConversationViewerNavState,
  type SessionInfo,
} from '@/components/common/ConversationViewer.vue';

type ScanPhase = 'recent' | 'extended' | 'complete';

type ImportSourceList = {
  items?: WebSessionImportSourceWire[];
  scanPhase?: ScanPhase;
};

type ItemResponse<T> = {
  item?: T;
};

const props = defineProps<{
  projectId: string;
  pendingSessionId?: string;
}>();

const showModal = defineModel<boolean>('show', { default: false });

const emit = defineEmits<{
  (e: 'import-session', source: Pick<WebSessionImportSourceSummary, 'agent' | 'sessionId'>): void;
  (e: 'open-existing-session', session: WebSessionSummary): void;
}>();

const { t } = useLocale();
const message = useMessage();

const loading = ref(false);
const searchQuery = ref('');
const hideImported = ref(false);
const importSources = ref<WebSessionImportSourceSummary[]>([]);
const scanPhase = ref<ScanPhase>('complete');
const showPreviewModal = ref(false);
const previewingSourceId = ref('');
const previewSource = ref<WebSessionImportSourceSummary | null>(null);
const conversationViewerRef = ref<{
  goToPrevUserMessage: () => void;
  goToNextUserMessage: () => void;
  syncNavigationState?: () => void;
} | null>(null);
const conversationNavState = ref<ConversationViewerNavState>({
  currentUserPosition: 0,
  totalUserMessages: 0,
  hasPrev: false,
  hasNext: false,
});
const {
  conversation: currentConversation,
  messages: conversationMessages,
  windowState: conversationWindow,
  loading: previewLoading,
  loadingEarlier: previewLoadingEarlier,
  load: loadConversationWindow,
  loadEarlier: loadEarlierConversationWindow,
  reset: resetConversationWindow,
} = useAiConversationWindow(options => {
  const source = previewSource.value;
  if (source?.aiSessionId) {
    return aiSessionApi.conversationWindowByID(source.aiSessionId, options);
  }
  return aiSessionApi.conversationWindowBySessionID(source?.sessionId || '', options);
}, null);
let refreshTimer: ReturnType<typeof setTimeout> | null = null;

const filteredSources = computed(() => {
  const query = searchQuery.value.trim().toLowerCase();
  return importSources.value.filter(source => {
    if (hideImported.value && source.duplicate) {
      return false;
    }
    if (!query) {
      return true;
    }
    return (
      source.title?.toLowerCase().includes(query) ||
      source.sessionId.toLowerCase().includes(query) ||
      source.model?.toLowerCase().includes(query) ||
      source.filePath.toLowerCase().includes(query) ||
      source.existingSession?.title?.toLowerCase().includes(query)
    );
  });
});

const duplicateCount = computed(
  () => importSources.value.filter(source => source.duplicate).length
);
const importableCount = computed(() => countImportableWebSessionSources(importSources.value));

const previewSessionTitle = computed(() => {
  return (
    previewSource.value?.title || currentConversation.value?.title || t('terminal.untitledSession')
  );
});

const previewSessionInfo = computed<SessionInfo | null>(() => {
  if (!previewSource.value) {
    return null;
  }
  return {
    sessionId: previewSource.value.sessionId,
    type: previewSource.value.agent,
  };
});

watch(showModal, show => {
  if (show) {
    void loadSources(false);
    return;
  }
  clearRefreshTimer();
  searchQuery.value = '';
  hideImported.value = false;
  closePreview();
});

function clearRefreshTimer() {
  if (!refreshTimer) {
    return;
  }
  clearTimeout(refreshTimer);
  refreshTimer = null;
}

function handleClose() {
  showModal.value = false;
}

function closePreview() {
  showPreviewModal.value = false;
  previewingSourceId.value = '';
  previewSource.value = null;
  resetConversationWindow();
  conversationNavState.value = {
    currentUserPosition: 0,
    totalUserMessages: 0,
    hasPrev: false,
    hasNext: false,
  };
}

function updateConversationNavState(state: ConversationViewerNavState) {
  conversationNavState.value = state;
}

function formatSessionTime(source: WebSessionImportSourceSummary) {
  const raw = source.lastMessageAt || source.sessionStartedAt;
  if (!raw) {
    return '-';
  }
  const timestamp = Date.parse(raw);
  if (!Number.isFinite(timestamp)) {
    return raw;
  }
  return new Date(timestamp).toLocaleString();
}

function emitImportFromPreview() {
  const sessionId = previewSource.value?.sessionId || '';
  if (!sessionId || !previewSource.value?.importable || props.pendingSessionId) {
    return;
  }
  emit('import-session', {
    agent: previewSource.value.agent,
    sessionId,
  });
}

function openExistingSession(source: WebSessionImportSourceSummary) {
  if (!source.existingSession) {
    return;
  }
  emit('open-existing-session', source.existingSession);
}

async function openPreview(source: WebSessionImportSourceSummary) {
  previewSource.value = source;
  previewingSourceId.value = source.sessionId;
  showPreviewModal.value = true;

  try {
    if (previewSource.value?.sessionId !== source.sessionId) {
      return;
    }
    await loadConversationWindow(AI_CONVERSATION_WINDOW_LIMIT);
    await nextTick();
    conversationViewerRef.value?.syncNavigationState?.();
  } catch (error) {
    console.error('Failed to load conversation preview:', error);
    message.error(t('terminal.loadConversationFailed'));
  } finally {
    if (previewSource.value?.sessionId === source.sessionId) {
      previewingSourceId.value = '';
    }
  }
}

async function loadSources(isRefresh = false) {
  if (!props.projectId) {
    return;
  }

  clearRefreshTimer();
  if (!isRefresh) {
    loading.value = true;
  }

  try {
    const response = await http
      .Get<ItemResponse<ImportSourceList>>(
        `/projects/${props.projectId}/web-sessions/import-sources`,
        {
          cacheFor: 0,
        }
      )
      .send();

    const data = response?.item;
    if (!data) {
      importSources.value = [];
      scanPhase.value = 'complete';
      return;
    }

    importSources.value = normalizeWebSessionImportSources(data.items);
    scanPhase.value = data.scanPhase || 'complete';

    if (showModal.value && scanPhase.value !== 'complete') {
      refreshTimer = setTimeout(() => {
        void loadSources(true);
      }, 2000);
    }
  } catch (error) {
    console.error('Failed to load AI import sources:', error);
    if (!isRefresh) {
      message.error(t('common.loadFailed'));
    }
  } finally {
    if (!isRefresh) {
      loading.value = false;
    }
  }
}
</script>

<style scoped>
.web-session-import {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 240px;
}

.web-session-import__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.web-session-import__summary {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.web-session-import__scanning {
  border: 1px solid rgba(14, 116, 144, 0.16);
  background: rgba(6, 182, 212, 0.08);
  color: #0f766e;
  border-radius: 12px;
  padding: 10px 12px;
  font-size: 13px;
}

.web-session-import__list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 56vh;
  overflow: auto;
  padding-right: 2px;
}

.web-session-import__item {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 12px;
  border: 1px solid color-mix(in srgb, var(--n-border-color) 88%, transparent);
  background: color-mix(in srgb, var(--n-card-color) 88%, #f8fafc 12%);
  border-radius: 16px;
  padding: 14px 16px;
}

.web-session-import__copy {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.web-session-import__title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
}

.web-session-import__title {
  flex: 1 1 280px;
  min-width: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.4;
  color: var(--n-text-color-1);
}

.web-session-import__badges {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  flex-wrap: wrap;
  margin-left: auto;
  max-width: 100%;
}

.web-session-import__badges :deep(.n-tag) {
  display: inline-flex;
  align-items: center;
}

.web-session-import__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 12px;
  color: var(--n-text-color-3);
}

.web-session-import__path {
  font-size: 12px;
  line-height: 1.5;
  color: var(--n-text-color-2);
  word-break: break-all;
}

.web-session-import__duplicate {
  font-size: 12px;
  color: #b45309;
}

.web-session-import__actions {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.web-session-import__nav-indicator {
  min-width: 56px;
  text-align: center;
  font-size: 12px;
  color: var(--n-text-color-3);
}

:deep(.n-card-header) {
  align-items: center;
  gap: 12px;
}

:deep(.n-card-header__main) {
  min-width: 0;
  flex: 1;
}

.web-session-import__preview-title {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.web-session-import__preview-note {
  margin-bottom: 12px;
  border: 1px solid rgba(245, 158, 11, 0.24);
  background: rgba(245, 158, 11, 0.08);
  color: #92400e;
  border-radius: 12px;
  padding: 10px 12px;
  font-size: 13px;
}

@media (max-width: 720px) {
  .web-session-import__title-row {
    justify-content: flex-start;
  }

  .web-session-import__actions {
    justify-content: flex-start;
  }
}
</style>
