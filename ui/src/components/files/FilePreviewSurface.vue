<template>
  <div class="file-preview-shell" :class="{ 'is-mobile': mobile }">
    <div v-if="showHeader" class="file-preview-header">
      <div class="file-preview-heading">
        <n-button
          v-if="showBackButton"
          text
          size="small"
          class="file-preview-back"
          @click="emit('close')"
        >
          {{ backLabel }}
        </n-button>
        <div class="file-preview-title-row">
          <div v-if="headerTitle" class="file-preview-title">{{ headerTitle }}</div>
          <div v-if="showDiffToggle" class="file-preview-mode-toggle" role="tablist">
            <button
              type="button"
              class="file-preview-mode-button"
              :class="{ 'is-active': previewMode === 'diff' }"
              @click="emit('mode-change', 'diff')"
            >
              {{ diffLabel }}
            </button>
            <button
              type="button"
              class="file-preview-mode-button"
              :class="{ 'is-active': previewMode === 'file' }"
              @click="emit('mode-change', 'file')"
            >
              {{ fileLabel }}
            </button>
          </div>
        </div>
        <div v-if="headerMeta" class="file-preview-meta">{{ headerMeta }}</div>
      </div>
      <div v-if="previewResult" class="file-preview-actions">
        <n-button tertiary size="small" @click="emit('download')">
          {{ downloadLabel }}
        </n-button>
      </div>
    </div>

    <div v-if="previewMode === 'diff' && diffLoading" class="file-preview-empty">
      <n-spin size="small" />
    </div>
    <div v-else-if="previewMode === 'diff' && diffError" class="file-preview-empty">
      <n-alert type="error" :show-icon="false">{{ diffError }}</n-alert>
    </div>
    <template v-else-if="previewMode === 'diff'">
      <div
        v-if="diffResult?.available"
        class="file-preview-content"
        :class="{ 'is-diff-content': previewMode === 'diff' }"
      >
        <div class="file-preview-diff chat-markdown" v-html="renderedDiff"></div>
      </div>
      <div v-else class="file-preview-empty">
        <n-alert type="info" :show-icon="false">{{ diffUnavailableText }}</n-alert>
      </div>
    </template>
    <div v-else-if="previewLoading" class="file-preview-empty">
      <n-spin size="small" />
    </div>
    <div v-else-if="previewError" class="file-preview-empty">
      <n-alert type="error" :show-icon="false">{{ previewError }}</n-alert>
    </div>
    <template v-else-if="previewResult">
      <div class="file-preview-content">
        <img
          v-if="previewResult.previewKind === 'image'"
          :src="previewResult.inlineUrl"
          :alt="previewResult.entry.name"
          class="file-preview-image"
          @click="emit('image-preview')"
        />
        <div
          v-else-if="previewResult.previewKind === 'markdown'"
          class="file-preview-markdown chat-markdown"
          v-html="renderedMarkdown"
        ></div>
        <pre v-else-if="previewResult.previewKind === 'text'" class="file-preview-text">{{
          previewResult.textContent
        }}</pre>
        <iframe
          v-else-if="previewResult.previewKind === 'pdf'"
          :src="previewResult.inlineUrl"
          class="file-preview-frame"
          :title="previewResult.entry.name"
        ></iframe>
        <audio
          v-else-if="previewResult.previewKind === 'audio'"
          :src="previewResult.inlineUrl"
          controls
          class="file-preview-media"
        ></audio>
        <video
          v-else-if="previewResult.previewKind === 'video'"
          :src="previewResult.inlineUrl"
          controls
          class="file-preview-media"
        ></video>
        <pre
          v-else-if="previewResult.previewKind === 'binary' && previewFallbackText"
          class="file-preview-text"
          >{{ previewFallbackText }}</pre
        >
        <div v-else class="file-preview-binary">
          {{ binaryPreviewHint }}
        </div>
      </div>

      <div v-if="previewResult.truncated" class="file-preview-truncated">
        {{ previewTruncatedLabel }}
      </div>
    </template>
    <div v-else class="file-preview-empty">
      {{ emptyLabel }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { FilePreviewMode } from '@/components/files/fileManagerDiff';
import type { FileManagerDiffResult, FileManagerPreviewResult } from '@/types/fileManager';

const props = withDefaults(
  defineProps<{
    previewResult: FileManagerPreviewResult | null;
    previewLoading: boolean;
    previewError: string;
    previewFallbackText: string;
    renderedMarkdown: string;
    previewMeta: string;
    emptyLabel: string;
    binaryPreviewHint: string;
    previewTruncatedLabel: string;
    downloadLabel: string;
    previewMode: FilePreviewMode;
    showDiffToggle: boolean;
    fileLabel: string;
    diffLabel: string;
    diffResult: FileManagerDiffResult | null;
    diffLoading: boolean;
    diffError: string;
    renderedDiff: string;
    diffUnavailableText: string;
    backLabel?: string;
    fallbackTitle?: string;
    mobile?: boolean;
    showBackButton?: boolean;
  }>(),
  {
    backLabel: '',
    fallbackTitle: '',
    mobile: false,
    showBackButton: false,
  }
);

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'download'): void;
  (e: 'image-preview'): void;
  (e: 'mode-change', value: FilePreviewMode): void;
}>();

const showHeader = computed(
  () =>
    props.showBackButton ||
    Boolean(props.previewResult) ||
    props.showDiffToggle ||
    Boolean(props.fallbackTitle)
);
const headerTitle = computed(() => props.previewResult?.entry.name || props.fallbackTitle);
const headerMeta = computed(() =>
  props.previewResult || props.fallbackTitle ? props.previewMeta : ''
);
</script>

<style scoped>
.file-preview-shell {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  background: var(--app-surface);
  color: var(--app-text-primary);
}

.file-preview-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px;
  border-bottom: 1px solid var(--app-border);
  background: color-mix(in srgb, var(--app-surface) 86%, var(--app-canvas) 14%);
}

.file-preview-shell.is-mobile .file-preview-header {
  position: sticky;
  top: 0;
  z-index: 1;
  padding-top: calc(8px + env(safe-area-inset-top, 0px));
}

.file-preview-heading {
  flex: 1;
  min-width: 0;
}

.file-preview-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.file-preview-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.file-preview-mode-toggle {
  display: inline-flex;
  align-items: center;
  padding: 3px;
  border-radius: 999px;
  background: var(--app-accent-soft);
  gap: 4px;
}

.file-preview-mode-button {
  border: none;
  border-radius: 999px;
  background: transparent;
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  padding: 6px 10px;
  cursor: pointer;
  transition:
    background 120ms ease,
    color 120ms ease;
}

.file-preview-mode-button.is-active {
  background: var(--app-surface-raised);
  color: var(--app-info);
  box-shadow: 0 6px 16px color-mix(in srgb, var(--app-shadow) 65%, transparent);
}

.file-preview-mode-button:focus-visible {
  outline: 2px solid var(--app-focus-ring);
  outline-offset: 1px;
}

.file-preview-back {
  margin: -6px 0 8px -10px;
}

.file-preview-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 15px;
  font-weight: 700;
}

.file-preview-meta {
  margin-top: 4px;
  color: var(--app-text-muted);
  font-size: 12px;
}

.file-preview-content {
  flex: 1;
  min-height: 0;
  min-width: 0;
  overflow: auto;
  -webkit-overflow-scrolling: touch;
  padding: 16px;
  scrollbar-width: thin;
  scrollbar-color: color-mix(in srgb, var(--app-accent) 42%, transparent) transparent;
}

.file-preview-content::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.file-preview-content::-webkit-scrollbar-track {
  background: transparent;
}

.file-preview-content::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: color-mix(in srgb, var(--app-accent) 32%, transparent);
}

.file-preview-content::-webkit-scrollbar-thumb:hover {
  background: color-mix(in srgb, var(--app-accent) 48%, transparent);
}

.file-preview-image,
.file-preview-frame,
.file-preview-media {
  width: 100%;
  border-radius: 14px;
  background: var(--app-surface-sunken);
}

.file-preview-image {
  display: block;
  object-fit: contain;
  cursor: zoom-in;
}

.file-preview-frame {
  min-height: 420px;
  border: none;
}

.file-preview-shell.is-mobile .file-preview-frame {
  min-height: 62vh;
}

.file-preview-text {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
}

.file-preview-markdown {
  font-size: 14px;
}

.file-preview-diff {
  width: 100%;
  min-width: 0;
}

.file-preview-diff :deep(pre.markdown-code-block) {
  margin: 0;
  max-width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
}

.file-preview-diff :deep(pre.markdown-code-block code.hljs) {
  width: max-content;
  min-width: 100%;
  white-space: pre;
  word-break: normal;
  overflow-wrap: normal;
}

.file-preview-binary,
.file-preview-truncated {
  color: var(--app-text-secondary);
  font-size: 13px;
}

.file-preview-truncated {
  padding: 0 16px 16px;
}

.file-preview-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 220px;
  padding: 16px;
  color: var(--app-text-secondary);
  text-align: center;
}

@media (max-width: 820px) {
  .file-preview-header {
    padding: 7px 10px 6px;
    gap: 8px;
  }

  .file-preview-shell.is-mobile .file-preview-heading {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }

  .file-preview-shell.is-mobile .file-preview-back {
    margin: 0;
    min-width: auto;
    min-height: 24px;
    padding: 0 4px;
    align-self: flex-start;
  }

  .file-preview-shell.is-mobile .file-preview-title-row {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  .file-preview-shell.is-mobile .file-preview-title {
    font-size: 13px;
    line-height: 1.25;
  }

  .file-preview-shell.is-mobile .file-preview-meta {
    margin-top: 0;
    font-size: 11px;
    line-height: 1.2;
  }

  .file-preview-shell.is-mobile .file-preview-actions {
    flex-shrink: 0;
    align-self: flex-start;
  }

  .file-preview-shell.is-mobile .file-preview-mode-toggle {
    margin-left: auto;
    padding: 2px;
    gap: 2px;
    flex-shrink: 0;
  }

  .file-preview-shell.is-mobile .file-preview-mode-button {
    padding: 4px 8px;
    font-size: 11px;
  }

  .file-preview-shell.is-mobile .file-preview-actions :deep(.n-button) {
    min-height: 26px;
    padding: 0 10px;
  }

  .file-preview-content {
    padding: 8px 10px calc(10px + env(safe-area-inset-bottom, 0px));
  }

  .file-preview-shell.is-mobile .file-preview-content {
    padding: 6px 8px calc(8px + env(safe-area-inset-bottom, 0px));
  }

  .file-preview-shell.is-mobile .file-preview-diff :deep(pre.markdown-code-block) {
    margin: 0;
    border-left: none;
    border-right: none;
    border-radius: 0;
    box-shadow: none;
    width: 100%;
    min-width: 100%;
    max-width: 100%;
    overflow-x: auto;
  }

  .file-preview-shell.is-mobile .file-preview-diff :deep(pre.markdown-code-block code.hljs) {
    width: max-content;
    min-width: 100%;
    padding: 6px 8px 8px;
    font-size: 12px;
  }

  .file-preview-shell.is-mobile
    .file-preview-diff
    :deep(pre.markdown-code-block[data-language]::before) {
    padding: 6px 8px 0;
  }
}
</style>
