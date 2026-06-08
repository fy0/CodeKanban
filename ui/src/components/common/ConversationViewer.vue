<template>
  <div class="conversation-viewer">
    <n-spin :show="loading" class="conversation-content-wrap">
      <div
        v-if="renderMessages.length > 0"
        ref="conversationContainerRef"
        class="conversation-container"
        @click.capture="handleMarkdownLinkClick"
        @scroll="handleConversationScroll"
      >
        <div
          v-if="showLoadEarlierIndicator"
          class="conversation-history-banner"
          :class="{ 'is-loading': loadingEarlier }"
        >
          <span>{{
            loadingEarlier ? t('common.loading') : t('terminal.loadingEarlierMessages')
          }}</span>
        </div>

        <div
          v-if="windowBeforeHeight > 0"
          class="conversation-spacer"
          :style="{ height: `${windowBeforeHeight}px` }"
        ></div>

        <div
          v-if="useVirtualizedView && virtualBeforeHeight > 0"
          class="conversation-spacer"
          :style="{ height: `${virtualBeforeHeight}px` }"
        ></div>

        <div
          v-for="item in visibleRenderMessages"
          :key="item.renderKey"
          :ref="el => setMessageRef(el, item)"
          class="message-shell"
        >
          <div
            class="message-item"
            :class="[
              item.message.role,
              {
                'message-item--active-user':
                  item.message.role === 'user' && item.key === activeUserMessageKey,
              },
            ]"
          >
            <div class="message-header">
              <span class="message-role">{{
                item.message.role === 'user' ? t('terminal.user') : t('terminal.assistant')
              }}</span>
              <div class="message-header-meta">
                <div v-if="item.message.role === 'assistant'" class="message-actions">
                  <n-button
                    size="tiny"
                    quaternary
                    :loading="isAssistantActionLoading(item.message)"
                    @click="copyMessageContent(item)"
                  >
                    copy
                  </n-button>
                  <n-button
                    size="tiny"
                    quaternary
                    :type="isRawMode(item.key) ? 'primary' : 'default'"
                    :loading="isAssistantActionLoading(item.message)"
                    @click="toggleRawMode(item)"
                  >
                    {{ t('terminal.rawMode') }}
                  </n-button>
                </div>
                <span v-if="item.message.timestamp" class="message-time">{{
                  formatTime(item.message.timestamp)
                }}</span>
              </div>
            </div>

            <pre v-if="isRawMode(item.key) && item.rawContent" class="message-raw"><code>{{
              item.rawContent
            }}</code></pre>
            <div
              v-else-if="item.renderedContent"
              class="message-content chat-markdown"
              v-html="renderMarkdown(item.renderedContent, markdownRenderOptions)"
            ></div>

            <div v-if="item.attachments.length" class="message-attachments">
              <n-popover
                v-for="attachment in item.attachments"
                :key="attachment.id"
                trigger="hover"
                placement="bottom-start"
                :flip="true"
                @update:show="
                  (visible: boolean) => handleAttachmentPreviewToggle(attachment, visible)
                "
              >
                <template #trigger>
                  <button
                    type="button"
                    class="image-attachment-chip"
                    :class="{
                      'image-attachment-chip--previewable': attachment.previewable,
                      'image-attachment-chip--disabled': !attachment.previewable,
                    }"
                    @mouseenter="primeImagePreview(attachment)"
                    @focus="primeImagePreview(attachment)"
                    @click="handleAttachmentClick(item, attachment)"
                  >
                    <n-icon :size="14"><ImageOutline /></n-icon>
                    <span>{{ attachment.label }}</span>
                  </button>
                </template>
                <div class="attachment-popover">
                  <template v-if="!attachment.previewable">
                    <span class="attachment-preview-hint">{{
                      getAttachmentUnavailableReason(attachment)
                    }}</span>
                  </template>
                  <template v-else>
                    <div
                      v-if="getImagePreviewState(attachment.id).status === 'loaded'"
                      class="attachment-preview-loaded"
                    >
                      <img
                        :src="getImagePreviewState(attachment.id).objectUrl"
                        :alt="attachment.label"
                        class="attachment-preview-image"
                      />
                    </div>
                    <div
                      v-else-if="getImagePreviewState(attachment.id).status === 'error'"
                      class="attachment-preview-error"
                    >
                      {{ t('terminal.imagePreviewFailed') }}
                    </div>
                    <div v-else class="attachment-preview-loading">
                      <n-spin size="small" />
                    </div>
                  </template>
                </div>
              </n-popover>
            </div>

            <div v-if="item.hasToolControls" class="tool-result-controls">
              <n-button
                size="tiny"
                quaternary
                :loading="!!toolResultLoading[item.message.toolUseId || '']"
                @click.stop="toggleToolResult(item.message)"
              >
                {{
                  isExpanded(item.message.toolUseId || '')
                    ? t('terminal.collapseToolResult')
                    : t('terminal.expandToolResult')
                }}
              </n-button>
            </div>
          </div>
        </div>

        <div
          v-if="useVirtualizedView && virtualAfterHeight > 0"
          class="conversation-spacer"
          :style="{ height: `${virtualAfterHeight}px` }"
        ></div>
      </div>
      <n-empty v-else-if="!loading" :description="emptyText" class="conversation-empty" />
    </n-spin>

    <div class="conversation-toolbar">
      <div v-if="sessionInfo" class="session-info">
        <n-tag
          v-if="sessionInfo.type"
          size="small"
          :type="sessionInfo.type === 'claude_code' ? 'info' : 'success'"
        >
          <template #icon>
            <n-icon size="12">
              <svg
                v-if="sessionInfo.type === 'claude_code'"
                viewBox="0 0 24 24"
                fill="currentColor"
              >
                <path
                  d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"
                />
              </svg>
              <LogoGithub v-else />
            </n-icon>
          </template>
          {{ sessionInfo.type === 'claude_code' ? 'Claude Code' : 'Codex' }}
        </n-tag>
        <code class="session-id-code">{{ sessionInfo.sessionId }}</code>
        <n-button size="tiny" quaternary @click="copySessionId">
          <template #icon>
            <n-icon size="12"><CopyOutline /></n-icon>
          </template>
        </n-button>
      </div>
      <div class="toolbar-right">
        <n-tooltip v-if="refreshEnabled">
          <template #trigger>
            <n-button size="tiny" quaternary :loading="refreshing" @click="emit('refresh')">
              <template #icon>
                <n-icon size="14"><RefreshOutline /></n-icon>
              </template>
            </n-button>
          </template>
          {{ t('terminal.refreshConversation') }}
        </n-tooltip>
        <n-checkbox v-model:checked="showUserOnly" size="small">
          {{ t('terminal.showUserMessagesOnly') }}
        </n-checkbox>
      </div>
    </div>

    <n-modal v-model:show="imagePreviewVisible" preset="card" style="width: 72vw; max-width: 960px">
      <template #header>
        <div class="image-preview-header">
          <span>{{ currentPreviewAttachment?.label || t('terminal.imagePreview') }}</span>
          <span v-if="imagePreviewImages.length > 1" class="image-preview-counter">
            {{ imagePreviewIndex + 1 }} / {{ imagePreviewImages.length }}
          </span>
        </div>
      </template>
      <template #header-extra>
        <n-space :size="6" align="center">
          <n-button
            quaternary
            circle
            size="small"
            :disabled="imagePreviewIndex <= 0"
            @click="goToPreviewImage(-1)"
          >
            {{ '<' }}
          </n-button>
          <n-button
            quaternary
            circle
            size="small"
            :disabled="imagePreviewIndex >= imagePreviewImages.length - 1"
            @click="goToPreviewImage(1)"
          >
            {{ '>' }}
          </n-button>
        </n-space>
      </template>
      <div class="image-preview-body">
        <div
          v-if="
            currentPreviewAttachment &&
            getImagePreviewState(currentPreviewAttachment.id).status === 'loaded'
          "
          class="image-preview-loaded"
        >
          <img
            :src="getImagePreviewState(currentPreviewAttachment.id).objectUrl"
            :alt="currentPreviewAttachment.label"
            class="image-preview-full"
          />
        </div>
        <div
          v-else-if="
            currentPreviewAttachment &&
            getImagePreviewState(currentPreviewAttachment.id).status === 'error'
          "
          class="image-preview-placeholder"
        >
          {{ t('terminal.imagePreviewFailed') }}
        </div>
        <div v-else class="image-preview-placeholder">
          <n-spin size="large" />
        </div>
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { useTimeAgo } from '@vueuse/core';
import { useDialog, useMessage } from 'naive-ui';
import { CopyOutline, ImageOutline, LogoGithub, RefreshOutline } from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import { useAppClipboard } from '@/composables/useAppClipboard';
import { useConversationVirtualizer } from '@/composables/useConversationVirtualizer';
import { renderMarkdown } from '@/utils/markdown';
import {
  getClickedMarkdownCodeCopyText,
  getClickedMarkdownLink,
  getClickedMarkdownLinkCopyHref,
  resolveNavigableHref,
} from '@/utils/messageLinkNavigation';
import {
  estimateConversationMessageHeight,
  recordConversationMessageHeight,
} from '@/utils/conversationHeightCache';

export interface ConversationImageAttachment {
  id: string;
  label: string;
  previewable: boolean;
  previewUrl?: string;
  mimeType?: string;
}

export interface ConversationMessage {
  role: 'user' | 'assistant';
  content: string;
  timestamp?: string;
  kind?: string;
  toolUseId?: string;
  hasMore?: boolean;
  full?: string;
  images?: ConversationImageAttachment[];
}

export interface SessionInfo {
  sessionId: string;
  type?: 'claude_code' | 'codex' | string;
}

export interface ConversationViewerNavState {
  currentUserPosition: number;
  totalUserMessages: number;
  hasPrev: boolean;
  hasNext: boolean;
}

export interface ConversationWindowState {
  totalMessages: number;
  totalUserMessages: number;
  userMessagesBeforeWindow: number;
  windowStart: number;
  hasMoreBefore: boolean;
}

interface DisplayMessageItem {
  key: string;
  sourceIndex: number;
  message: ConversationMessage;
}

interface RenderMessageItem extends DisplayMessageItem {
  renderKey: string;
  heightVariant: string;
  renderedContent: string;
  rawContent: string;
  attachments: DisplayAttachment[];
  imageCount: number;
  hasToolControls: boolean;
  estimatedHeight: number;
}

interface ImagePreviewState {
  status: 'idle' | 'loading' | 'loaded' | 'error';
  objectUrl?: string;
}

interface DisplayAttachment extends ConversationImageAttachment {
  source: 'parsed' | 'placeholder';
}

const DIRECT_RENDER_LIMIT = 120;
const MIN_MEASURED_MESSAGE_HEIGHT = 72;

const props = withDefaults(
  defineProps<{
    messages: ConversationMessage[];
    loading?: boolean;
    refreshing?: boolean;
    refreshEnabled?: boolean;
    sessionInfo?: SessionInfo | null;
    emptyText?: string;
    useRelativeTime?: boolean;
    loadToolResult?: ((toolUseId: string) => Promise<string | null | undefined>) | null;
    conversationWindow?: ConversationWindowState | null;
    loadingEarlier?: boolean;
    loadEarlier?: (() => Promise<unknown> | void) | null;
  }>(),
  {
    loading: false,
    refreshing: false,
    refreshEnabled: false,
    sessionInfo: null,
    emptyText: '',
    useRelativeTime: true,
    loadToolResult: null,
    conversationWindow: null,
    loadingEarlier: false,
    loadEarlier: null,
  }
);

const emit = defineEmits<{
  (e: 'refresh'): void;
  (e: 'nav-state-change', state: ConversationViewerNavState): void;
}>();

const { t } = useLocale();
const dialog = useDialog();
const message = useMessage();
const { copyText } = useAppClipboard();

const showUserOnly = ref(false);
const conversationContainerRef = ref<HTMLElement | null>(null);
const expandedToolResults = ref<Record<string, boolean>>({});
const toolResultLoading = ref<Record<string, boolean>>({});
const rawMode = ref<Record<string, boolean>>({});
const activeUserMessageKey = ref<string | null>(null);
const previewCache = ref<Record<string, ImagePreviewState>>({});
const imagePreviewVisible = ref(false);
const imagePreviewImages = ref<ConversationImageAttachment[]>([]);
const imagePreviewIndex = ref(0);

const toolResultRequests = new Map<string, Promise<string>>();
const imagePreviewRequests = new Map<string, Promise<void>>();
const messageRefs = new Map<string, HTMLElement>();
const renderKeyToElement = new Map<string, HTMLElement>();
const observedRenderKeyByElement = new WeakMap<HTMLElement, string>();
const pendingHistoryAnchor = ref<{ previousTop: number; previousHeight: number } | null>(null);

let navigationFrame = 0;
let messageResizeObserver: ResizeObserver | null = null;

const imagePlaceholderPattern = /\[Image #(\d+)\]/g;

const displayMessages = computed<DisplayMessageItem[]>(() => {
  return props.messages
    .map((msg, index) => ({
      key: `msg-${index}`,
      sourceIndex: index,
      message: msg,
    }))
    .filter(item => !showUserOnly.value || item.message.role === 'user');
});

const emptyText = computed(() => props.emptyText || t('terminal.noMessages'));
const conversationWindow = computed<ConversationWindowState>(() => {
  return (
    props.conversationWindow ?? {
      totalMessages: props.messages.length,
      totalUserMessages: userMessageKeys.value.length,
      userMessagesBeforeWindow: 0,
      windowStart: 0,
      hasMoreBefore: false,
    }
  );
});
const markdownRenderOptions = computed(() => ({
  enableCodeBlockCopy: true,
  codeBlockCopyLabel: 'copy',
  enableLinkCopy: true,
  linkCopyLabel: t('common.copyLink'),
}));

const userMessageKeys = computed(() => {
  return displayMessages.value.filter(item => item.message.role === 'user').map(item => item.key);
});

const userMessageIndexMap = computed(() => {
  return new Map(userMessageKeys.value.map((key, index) => [key, index]));
});

const averageMeasuredHeight = computed(() => {
  if (renderMessages.value.length === 0) {
    return 160;
  }
  const total = renderMessages.value.reduce((sum, item) => sum + item.estimatedHeight, 0);
  return Math.max(MIN_MEASURED_MESSAGE_HEIGHT, Math.round(total / renderMessages.value.length));
});

const windowBeforeHeight = computed(() => {
  const start = Math.max(0, conversationWindow.value.windowStart);
  return start * averageMeasuredHeight.value;
});

const showLoadEarlierIndicator = computed(() => {
  return conversationWindow.value.hasMoreBefore || props.loadingEarlier;
});

function isToolResult(msg: ConversationMessage) {
  return msg.kind === 'tool_result' && !!msg.toolUseId;
}

function messageHasToolControls(msg: ConversationMessage) {
  return isToolResult(msg) && !!msg.toolUseId && !!(msg.hasMore || msg.full);
}

function isExpanded(toolUseId: string) {
  return !!expandedToolResults.value[toolUseId];
}

function isRawMode(key: string) {
  return !!rawMode.value[key];
}

function getRenderedContent(msg: ConversationMessage) {
  return isToolResult(msg) && msg.toolUseId && isExpanded(msg.toolUseId) && msg.full
    ? msg.full
    : msg.content;
}

function getSynchronousRawContent(msg: ConversationMessage) {
  return msg.full || msg.content || '';
}

function getInlinePlaceholderAttachments(item: DisplayMessageItem): DisplayAttachment[] {
  const placeholders = Array.from(item.message.content.matchAll(imagePlaceholderPattern));
  return placeholders.map((match, index) => ({
    id: `${item.key}-placeholder-${index}`,
    label: `Image #${match[1]}`,
    previewable: false,
    source: 'placeholder',
  }));
}

function getAttachmentIdentity(label: string) {
  const normalized = label.trim();
  const match = normalized.match(/^\[?Image #(\d+)\]?$/i);
  if (match) {
    return `image-${match[1]}`;
  }
  return normalized.toLowerCase();
}

function getAttachmentUnavailableReason(attachment: DisplayAttachment) {
  if (attachment.source === 'placeholder') {
    return t('terminal.imagePreviewUnavailablePlaceholder');
  }
  return t('terminal.imagePreviewUnavailable');
}

function getDisplayAttachments(item: DisplayMessageItem): DisplayAttachment[] {
  const dedupedAttachments: DisplayAttachment[] = [];
  const seenIdentities = new Set<string>();

  for (const attachment of item.message.images ?? []) {
    const identity = getAttachmentIdentity(attachment.label);
    if (seenIdentities.has(identity)) {
      continue;
    }
    seenIdentities.add(identity);
    dedupedAttachments.push({
      ...attachment,
      source: 'parsed' as const,
    });
  }

  const placeholders = getInlinePlaceholderAttachments(item).filter(placeholder => {
    return !seenIdentities.has(getAttachmentIdentity(placeholder.label));
  });
  return [...dedupedAttachments, ...placeholders];
}

function getHeightVariant(item: DisplayMessageItem) {
  if (isRawMode(item.key)) {
    return 'raw';
  }
  if (isToolResult(item.message)) {
    return item.message.toolUseId && isExpanded(item.message.toolUseId)
      ? 'tool-expanded'
      : 'tool-collapsed';
  }
  return 'markdown';
}

const renderMessages = computed<RenderMessageItem[]>(() => {
  const sessionId = props.sessionInfo?.sessionId;
  return displayMessages.value.map(item => {
    const renderedContent = getRenderedContent(item.message);
    const rawContent = getSynchronousRawContent(item.message);
    const attachments = getDisplayAttachments(item);
    const heightVariant = getHeightVariant(item);
    const hasToolControls = messageHasToolControls(item.message);
    const estimateContent = heightVariant === 'raw' ? rawContent : renderedContent;

    return {
      ...item,
      renderKey: `${item.key}:${heightVariant}`,
      heightVariant,
      renderedContent,
      rawContent,
      attachments,
      imageCount: attachments.length,
      hasToolControls,
      estimatedHeight: estimateConversationMessageHeight({
        sessionId,
        sourceIndex: item.sourceIndex,
        variant: heightVariant,
        role: item.message.role,
        kind: item.message.kind,
        content: estimateContent,
        imageCount: attachments.length,
        hasToolControls,
      }),
    };
  });
});

const renderMessageByRenderKey = computed(() => {
  return new Map(renderMessages.value.map(item => [item.renderKey, item]));
});

const renderMessageIndexMap = computed(() => {
  return new Map(renderMessages.value.map((item, index) => [item.key, index]));
});

const useVirtualizedView = computed(() => renderMessages.value.length > DIRECT_RENDER_LIMIT);

const virtualizer = useConversationVirtualizer<RenderMessageItem>({
  items: renderMessages,
  containerRef: conversationContainerRef,
  estimateHeight: item => item.estimatedHeight,
  overscanPx: 720,
});

const virtualBeforeHeight = computed(() => {
  return useVirtualizedView.value ? virtualizer.beforeHeight.value : 0;
});

const virtualAfterHeight = computed(() => {
  return useVirtualizedView.value ? virtualizer.afterHeight.value : 0;
});

const visibleRenderMessages = computed(() => {
  return useVirtualizedView.value
    ? virtualizer.visibleItems.value.map(entry => entry.item)
    : renderMessages.value;
});

const navState = computed<ConversationViewerNavState>(() => {
  const visibleUserCount = userMessageKeys.value.length;
  const totalUserMessages = Math.max(
    conversationWindow.value.totalUserMessages,
    conversationWindow.value.userMessagesBeforeWindow + visibleUserCount
  );
  if (totalUserMessages === 0) {
    return {
      currentUserPosition: 0,
      totalUserMessages: 0,
      hasPrev: false,
      hasNext: false,
    };
  }

  const currentIndex = activeUserMessageKey.value
    ? (userMessageIndexMap.value.get(activeUserMessageKey.value) ?? 0)
    : 0;
  const currentUserPosition = Math.min(
    totalUserMessages,
    conversationWindow.value.userMessagesBeforeWindow + currentIndex + 1
  );

  return {
    currentUserPosition,
    totalUserMessages,
    hasPrev: currentUserPosition > 1,
    hasNext: currentUserPosition < totalUserMessages,
  };
});

const currentPreviewAttachment = computed(() => {
  return imagePreviewImages.value[imagePreviewIndex.value] ?? null;
});

watch(
  navState,
  value => {
    emit('nav-state-change', value);
  },
  { immediate: true, deep: true }
);

watch(
  renderMessages,
  async value => {
    if (value.length === 0) {
      activeUserMessageKey.value = null;
      emit('nav-state-change', {
        currentUserPosition: 0,
        totalUserMessages: 0,
        hasPrev: false,
        hasNext: false,
      });
      return;
    }
    await nextTick();
    if (restoreHistoryAnchor()) {
      virtualizer.syncScrollPosition();
      scheduleNavigationSync();
      return;
    }
    virtualizer.syncScrollPosition();
    scheduleNavigationSync();
  },
  { deep: true }
);

watch(
  () => props.messages,
  async () => {
    if (pendingHistoryAnchor.value) {
      return;
    }
    await nextTick();
    resetScrollToPrimaryMessage();
  },
  { immediate: true }
);

watch(
  () => props.sessionInfo?.sessionId,
  () => {
    rawMode.value = {};
    expandedToolResults.value = {};
    toolResultLoading.value = {};
    activeUserMessageKey.value = null;
    clearMessageElements();
    cleanupPreviewCache();
    pendingHistoryAnchor.value = null;
    imagePreviewVisible.value = false;
    imagePreviewImages.value = [];
    imagePreviewIndex.value = 0;
  },
  { immediate: true }
);

watch(showUserOnly, async () => {
  await nextTick();
  if (activeUserMessageKey.value && userMessageIndexMap.value.has(activeUserMessageKey.value)) {
    scheduleNavigationSync();
    return;
  }
  const firstKey = userMessageKeys.value[0];
  if (firstKey) {
    scrollToUserMessageByKey(firstKey);
    return;
  }
  conversationContainerRef.value?.scrollTo({ top: 0, behavior: 'auto' });
  activeUserMessageKey.value = null;
});

function ensureMessageResizeObserver() {
  if (
    messageResizeObserver ||
    typeof window === 'undefined' ||
    typeof ResizeObserver === 'undefined'
  ) {
    return;
  }
  messageResizeObserver = new ResizeObserver(entries => {
    for (const entry of entries) {
      const target = entry.target;
      if (!(target instanceof HTMLElement)) {
        continue;
      }
      const renderKey = observedRenderKeyByElement.get(target);
      if (!renderKey) {
        continue;
      }
      const item = renderMessageByRenderKey.value.get(renderKey);
      if (!item) {
        continue;
      }
      const measuredHeight = Math.ceil(
        entry.borderBoxSize?.[0]?.blockSize ?? entry.contentRect.height ?? 0
      );
      if (!Number.isFinite(measuredHeight) || measuredHeight < MIN_MEASURED_MESSAGE_HEIGHT) {
        continue;
      }

      virtualizer.setMeasuredHeight(renderKey, measuredHeight);
      recordConversationMessageHeight({
        sessionId: props.sessionInfo?.sessionId,
        sourceIndex: item.sourceIndex,
        variant: item.heightVariant,
        role: item.message.role,
        kind: item.message.kind,
        content: item.heightVariant === 'raw' ? item.rawContent : item.renderedContent,
        imageCount: item.imageCount,
        hasToolControls: item.hasToolControls,
        height: measuredHeight,
      });
    }
    scheduleNavigationSync();
  });
}

function clearMessageElements() {
  if (messageResizeObserver) {
    renderKeyToElement.forEach(element => {
      messageResizeObserver?.unobserve(element);
    });
  }
  renderKeyToElement.clear();
  messageRefs.clear();
}

function setMessageRef(el: unknown, item: RenderMessageItem) {
  ensureMessageResizeObserver();

  const previous = renderKeyToElement.get(item.renderKey);
  if (previous && previous !== el) {
    messageResizeObserver?.unobserve(previous);
    renderKeyToElement.delete(item.renderKey);
    if (messageRefs.get(item.key) === previous) {
      messageRefs.delete(item.key);
    }
  }

  if (!(el instanceof HTMLElement)) {
    return;
  }

  renderKeyToElement.set(item.renderKey, el);
  messageRefs.set(item.key, el);
  observedRenderKeyByElement.set(el, item.renderKey);
  messageResizeObserver?.unobserve(el);
  messageResizeObserver?.observe(el);
}

function getScrollContainer() {
  return conversationContainerRef.value;
}

function scheduleNavigationSync() {
  if (navigationFrame) {
    cancelAnimationFrame(navigationFrame);
  }
  navigationFrame = requestAnimationFrame(() => {
    navigationFrame = 0;
    syncActiveUserMessage();
  });
}

function syncActiveUserMessage() {
  const userKeys = userMessageKeys.value;
  if (userKeys.length === 0) {
    activeUserMessageKey.value = null;
    return;
  }

  const container = getScrollContainer();
  if (!container) {
    activeUserMessageKey.value = userKeys[0];
    return;
  }

  const containerTop = container.getBoundingClientRect().top;
  let closestAbove: { key: string; offset: number } | null = null;
  let closestBelow: { key: string; offset: number } | null = null;

  for (const key of userKeys) {
    const element = messageRefs.get(key);
    if (!element) {
      continue;
    }
    const offset = element.getBoundingClientRect().top - containerTop;
    if (offset <= 16) {
      if (!closestAbove || offset > closestAbove.offset) {
        closestAbove = { key, offset };
      }
      continue;
    }
    if (!closestBelow || offset < closestBelow.offset) {
      closestBelow = { key, offset };
    }
  }

  activeUserMessageKey.value = closestAbove?.key ?? closestBelow?.key ?? userKeys[0];
}

function handleConversationScroll() {
  virtualizer.syncScrollPosition();
  scheduleNavigationSync();
  maybeLoadEarlierHistory();
}

function restoreHistoryAnchor() {
  const anchor = pendingHistoryAnchor.value;
  const container = conversationContainerRef.value;
  if (!anchor || !container) {
    return false;
  }
  container.scrollTop = anchor.previousTop + (container.scrollHeight - anchor.previousHeight);
  pendingHistoryAnchor.value = null;
  return true;
}

function maybeLoadEarlierHistory() {
  const container = conversationContainerRef.value;
  if (
    !container ||
    !props.loadEarlier ||
    props.loadingEarlier ||
    !conversationWindow.value.hasMoreBefore
  ) {
    return;
  }
  if (container.scrollTop > 120) {
    return;
  }
  pendingHistoryAnchor.value = {
    previousTop: container.scrollTop,
    previousHeight: container.scrollHeight,
  };
  void Promise.resolve(props.loadEarlier()).catch(() => {
    pendingHistoryAnchor.value = null;
  });
}

function handleMarkdownLinkClick(event: MouseEvent) {
  if (event.defaultPrevented || typeof window === 'undefined') {
    return;
  }

  const codeText = getClickedMarkdownCodeCopyText(event.target, event.currentTarget);
  if (codeText) {
    event.preventDefault();
    event.stopPropagation();
    void copyText(codeText, {
      failureMessage: t('terminal.copyFailed'),
      successMessage: t('common.copySuccess'),
    });
    return;
  }

  const copyHref = getClickedMarkdownLinkCopyHref(event.target, event.currentTarget);
  if (copyHref) {
    event.preventDefault();
    event.stopPropagation();
    void copyText(copyHref, {
      failureMessage: t('terminal.copyFailed'),
      successMessage: t('common.linkCopied'),
    });
    return;
  }

  const anchor = getClickedMarkdownLink(event.target, event.currentTarget);
  if (!anchor) {
    return;
  }

  event.preventDefault();
  const href = resolveNavigableHref(anchor.getAttribute('href') ?? '', window.location.href);
  if (!href) {
    message.warning(t('common.invalidLink'));
    return;
  }

  dialog.warning({
    title: t('common.openLinkTitle'),
    content: () =>
      h('div', { class: 'conversation-link-confirm' }, [
        h('div', { class: 'conversation-link-confirm__message' }, [t('common.openLinkMessage')]),
        h('code', { class: 'conversation-link-confirm__href' }, href),
      ]),
    positiveText: t('common.openInNewTab'),
    negativeText: t('common.cancel'),
    onPositiveClick: () => {
      window.open(href, '_blank', 'noopener,noreferrer');
      message.success(t('common.openLinkSuccess'));
    },
  });
}

async function resetScrollToPrimaryMessage() {
  const container = conversationContainerRef.value;
  if (container) {
    container.scrollTo({ top: 0, behavior: 'auto' });
  }
  virtualizer.syncScrollPosition();

  const firstUserKey = userMessageKeys.value[0];
  if (!firstUserKey) {
    activeUserMessageKey.value = null;
    scheduleNavigationSync();
    return;
  }

  await nextTick();
  scrollToUserMessageByKey(firstUserKey);
}

function scrollToUserMessageByKey(targetKey: string, behavior: ScrollBehavior = 'auto') {
  if (!targetKey) {
    return;
  }

  const container = conversationContainerRef.value;
  const element = messageRefs.get(targetKey);
  if (container && element) {
    const containerRect = container.getBoundingClientRect();
    const elementRect = element.getBoundingClientRect();
    const targetTop = container.scrollTop + (elementRect.top - containerRect.top);
    container.scrollTo({
      top: Math.max(0, targetTop),
      behavior,
    });
    virtualizer.syncScrollPosition();
  } else {
    const targetIndex = renderMessageIndexMap.value.get(targetKey);
    if (targetIndex !== undefined) {
      virtualizer.scrollToIndex(targetIndex, behavior);
    }
  }

  activeUserMessageKey.value = targetKey;
  void nextTick().then(() => {
    scheduleNavigationSync();
  });
}

async function goToPrevUserMessage() {
  const currentIndex = activeUserMessageKey.value
    ? (userMessageIndexMap.value.get(activeUserMessageKey.value) ?? 0)
    : 0;
  if (currentIndex <= 0) {
    if (conversationWindow.value.hasMoreBefore && props.loadEarlier && !props.loadingEarlier) {
      const container = conversationContainerRef.value;
      if (container) {
        pendingHistoryAnchor.value = {
          previousTop: container.scrollTop,
          previousHeight: container.scrollHeight,
        };
      }
      await Promise.resolve(props.loadEarlier()).catch(() => {
        pendingHistoryAnchor.value = null;
      });
      await nextTick();
      const nextIndex = Math.max(0, userMessageKeys.value.length - 1);
      const nextKey = userMessageKeys.value[nextIndex];
      if (nextKey) {
        scrollToUserMessageByKey(nextKey, 'smooth');
      }
    }
    return;
  }
  scrollToUserMessageByKey(userMessageKeys.value[currentIndex - 1], 'smooth');
}

function goToNextUserMessage() {
  const currentIndex = activeUserMessageKey.value
    ? (userMessageIndexMap.value.get(activeUserMessageKey.value) ?? 0)
    : 0;
  if (currentIndex >= userMessageKeys.value.length - 1) {
    return;
  }
  scrollToUserMessageByKey(userMessageKeys.value[currentIndex + 1], 'smooth');
}

defineExpose({
  goToPrevUserMessage,
  goToNextUserMessage,
  syncNavigationState: scheduleNavigationSync,
});

function formatTime(timestamp: string) {
  if (props.useRelativeTime) {
    return useTimeAgo(new Date(timestamp)).value;
  }
  return new Date(timestamp).toLocaleString();
}

function isAssistantActionLoading(msg: ConversationMessage) {
  return !!(msg.toolUseId && toolResultLoading.value[msg.toolUseId]);
}

async function ensureToolResultLoaded(msg: ConversationMessage) {
  if (!isToolResult(msg) || !msg.toolUseId) {
    return msg.full || msg.content || '';
  }
  if (msg.full) {
    return msg.full;
  }
  if (!msg.hasMore || !props.loadToolResult) {
    return msg.content;
  }
  const existing = toolResultRequests.get(msg.toolUseId);
  if (existing) {
    return existing;
  }

  toolResultLoading.value = { ...toolResultLoading.value, [msg.toolUseId]: true };
  const request = Promise.resolve(props.loadToolResult(msg.toolUseId))
    .then(content => content || msg.full || msg.content || '')
    .finally(() => {
      toolResultRequests.delete(msg.toolUseId as string);
      const next = { ...toolResultLoading.value };
      delete next[msg.toolUseId as string];
      toolResultLoading.value = next;
      scheduleNavigationSync();
    });

  toolResultRequests.set(msg.toolUseId, request);
  return request;
}

async function toggleToolResult(msg: ConversationMessage) {
  const toolUseId = msg.toolUseId;
  if (!toolUseId) {
    return;
  }
  const nextValue = !isExpanded(toolUseId);
  expandedToolResults.value = { ...expandedToolResults.value, [toolUseId]: nextValue };
  if (nextValue) {
    await ensureToolResultLoaded(msg);
  }
}

async function toggleRawMode(item: RenderMessageItem) {
  const nextValue = !isRawMode(item.key);
  rawMode.value = { ...rawMode.value, [item.key]: nextValue };
  if (nextValue) {
    await ensureToolResultLoaded(item.message);
  }
}

async function copyMessageContent(item: RenderMessageItem) {
  const content = (await ensureToolResultLoaded(item.message)) || item.rawContent;
  await copyText(content, {
    failureMessage: t('terminal.copyFailed'),
    successMessage: t('common.copySuccess'),
  });
}

function getImagePreviewState(attachmentId: string): ImagePreviewState {
  return previewCache.value[attachmentId] || { status: 'idle' };
}

async function primeImagePreview(attachment: ConversationImageAttachment) {
  if (!attachment.previewable || !attachment.previewUrl) {
    return;
  }
  const existingState = getImagePreviewState(attachment.id);
  if (existingState.status === 'loaded' || existingState.status === 'loading') {
    return;
  }
  const pending = imagePreviewRequests.get(attachment.id);
  if (pending) {
    return pending;
  }

  previewCache.value = {
    ...previewCache.value,
    [attachment.id]: { status: 'loading' },
  };

  const request = fetch(attachment.previewUrl)
    .then(async response => {
      if (!response.ok) {
        throw new Error('preview-request-failed');
      }
      const blob = await response.blob();
      const objectUrl = URL.createObjectURL(blob);
      const prevEntry = previewCache.value[attachment.id];
      if (prevEntry?.objectUrl) {
        URL.revokeObjectURL(prevEntry.objectUrl);
      }
      previewCache.value = {
        ...previewCache.value,
        [attachment.id]: {
          status: 'loaded',
          objectUrl,
        },
      };
    })
    .catch(() => {
      previewCache.value = {
        ...previewCache.value,
        [attachment.id]: { status: 'error' },
      };
    })
    .finally(() => {
      imagePreviewRequests.delete(attachment.id);
    });

  imagePreviewRequests.set(attachment.id, request);
  return request;
}

function handleAttachmentPreviewToggle(attachment: ConversationImageAttachment, visible: boolean) {
  if (visible) {
    void primeImagePreview(attachment);
  }
}

function handleAttachmentClick(item: RenderMessageItem, attachment: DisplayAttachment) {
  if (!attachment.previewable) {
    message.info(getAttachmentUnavailableReason(attachment));
    return;
  }
  void openImagePreview(item.attachments, attachment.id);
}

async function openImagePreview(images: ConversationImageAttachment[], attachmentId: string) {
  const previewableImages = images.filter(image => image.previewable);
  const targetIndex = previewableImages.findIndex(image => image.id === attachmentId);
  if (targetIndex < 0) {
    return;
  }
  imagePreviewImages.value = previewableImages;
  imagePreviewIndex.value = targetIndex;
  imagePreviewVisible.value = true;
  await primeImagePreview(previewableImages[targetIndex]);
}

function goToPreviewImage(direction: number) {
  const nextIndex = imagePreviewIndex.value + direction;
  if (nextIndex < 0 || nextIndex >= imagePreviewImages.value.length) {
    return;
  }
  imagePreviewIndex.value = nextIndex;
  void primeImagePreview(imagePreviewImages.value[nextIndex]);
}

function cleanupPreviewCache() {
  for (const entry of Object.values(previewCache.value)) {
    if (entry.objectUrl) {
      URL.revokeObjectURL(entry.objectUrl);
    }
  }
  previewCache.value = {};
  imagePreviewRequests.clear();
}

async function copySessionId() {
  if (!props.sessionInfo?.sessionId) {
    return;
  }
  await copyText(props.sessionInfo.sessionId, {
    failureMessage: t('terminal.copyFailed'),
    successMessage: t('terminal.aiSessionIdCopied'),
  });
}

onBeforeUnmount(() => {
  if (navigationFrame) {
    cancelAnimationFrame(navigationFrame);
  }
  clearMessageElements();
  messageResizeObserver?.disconnect();
  cleanupPreviewCache();
});
</script>

<style scoped>
.conversation-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.conversation-content-wrap {
  flex: 1;
  min-height: 0;
}

.conversation-container {
  height: 60vh;
  max-height: 60vh;
  min-height: 240px;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 18px 16px 12px 8px;
}

.conversation-history-banner {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  justify-content: center;
  margin-bottom: 10px;
  pointer-events: none;
}

.conversation-history-banner span {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--n-color-embedded) 92%, white 8%);
  color: var(--n-text-color-3);
  font-size: 12px;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

.conversation-history-banner.is-loading span {
  color: var(--n-primary-color);
}

.conversation-link-confirm {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.conversation-link-confirm__message {
  line-height: 1.5;
}

.conversation-link-confirm__href {
  display: block;
  max-width: 100%;
  padding: 8px 10px;
  border-radius: 8px;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  background: var(--n-code-color);
}

.conversation-spacer {
  width: 100%;
  pointer-events: none;
}

.conversation-empty {
  height: 240px;
}

.message-shell {
  box-sizing: border-box;
  padding-bottom: 16px;
}

.message-item {
  padding: 12px 16px;
  border-radius: 8px;
  background: var(--n-color-embedded);
  transition:
    box-shadow 0.2s ease,
    border-color 0.2s ease;
}

.message-item.user {
  border-left: 3px solid var(--n-primary-color);
}

.message-item.assistant {
  border-left: 3px solid var(--n-success-color);
}

.message-item--active-user {
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--n-primary-color) 45%, transparent);
}

.message-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 8px;
}

.message-role {
  font-weight: 600;
  font-size: 13px;
}

.message-item.user .message-role {
  color: var(--n-primary-color);
}

.message-item.assistant .message-role {
  color: var(--n-success-color);
}

.message-header-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.message-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.message-time {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.message-content {
  min-width: 0;
}

.message-raw {
  margin: 0;
  padding: 12px;
  border-radius: 6px;
  background: var(--n-code-color);
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.6;
}

.message-attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
}

.image-attachment-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid color-mix(in srgb, var(--n-primary-color) 35%, transparent);
  background: color-mix(in srgb, var(--n-primary-color) 10%, transparent);
  color: var(--n-primary-color);
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.image-attachment-chip:hover,
.image-attachment-chip:focus-visible {
  border-color: var(--n-primary-color);
  background: color-mix(in srgb, var(--n-primary-color) 14%, transparent);
}

.image-attachment-chip--disabled {
  cursor: default;
  opacity: 0.7;
}

.attachment-popover {
  width: 180px;
  min-height: 120px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.attachment-preview-loading,
.attachment-preview-error,
.attachment-preview-hint {
  color: var(--n-text-color-3);
  font-size: 12px;
  text-align: center;
}

.attachment-preview-image {
  display: block;
  max-width: 160px;
  max-height: 120px;
  border-radius: 8px;
}

.tool-result-controls {
  margin-top: 8px;
}

.conversation-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--n-border-color);
  margin-top: 8px;
}

.session-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.session-id-code {
  font-size: 12px;
  font-family: monospace;
  background: var(--n-color-embedded);
  padding: 2px 8px;
  border-radius: 4px;
  color: var(--n-text-color-2);
  user-select: all;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.image-preview-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.image-preview-counter {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.image-preview-body {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 360px;
}

.image-preview-loaded {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
}

.image-preview-full {
  max-width: 100%;
  max-height: 70vh;
  border-radius: 10px;
}

.image-preview-placeholder {
  min-height: 240px;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--n-text-color-3);
}
</style>
