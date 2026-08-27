<template>
  <n-modal
    :show="show"
    preset="card"
    class="command-execution-detail-modal"
    :title="title"
    :bordered="false"
    :segmented="{ content: false, footer: false }"
    :mask-closable="true"
    closable
    style="width: min(92vw, 960px)"
    @update:show="emit('update:show', $event)"
  >
    <div v-if="detail" class="command-execution-detail-summary">
      {{
        t('webSession.compactToolDetailCount', {
          kind: kindLabel,
          count: detail.count,
        })
      }}
    </div>
    <div v-if="loading" class="command-execution-detail-loading">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="items.length > 0" class="command-execution-detail-list">
      <details
        v-for="(detailItem, index) in items"
        :key="`${detailItem.toolId}:${detailItem.timestamp}`"
        class="command-execution-detail-item"
        :open="index === 0"
      >
        <summary class="command-execution-detail-item-summary">
          <span class="command-execution-detail-item-label">
            {{ index === 0 ? t('webSession.compactToolLatest') : `#${items.length - index}` }}
          </span>
          <span class="command-execution-detail-item-command">
            {{ detailItem.summary || detailItem.command || t('webSession.compactToolNoSummary') }}
          </span>
          <span class="tool-state-badge" :class="`state-${detailItem.status}`">
            <span class="tool-state-dot"></span>
            {{ toolStateLabel(detailItem.status) }}
          </span>
          <span
            class="command-execution-detail-item-time"
            :title="formatDetailDateTime(detailItem)"
          >
            {{ formatDetailTime(detailItem) }}
          </span>
        </summary>

        <div class="command-execution-detail-item-body">
          <div class="tool-section">
            <div class="tool-section-label">{{ t('webSession.compactToolSummary') }}</div>
            <pre class="tool-code">{{
              detailItem.summary || detailItem.command || t('webSession.compactToolNoSummary')
            }}</pre>
          </div>
          <div v-if="showInput(detailItem)" class="tool-section">
            <div class="tool-section-label">{{ t('webSession.toolInput') }}</div>
            <pre class="tool-code">{{ stringifyValue(detailItem.input) }}</pre>
          </div>
          <div v-if="detailItem.output" class="tool-section">
            <div class="tool-section-label">{{ t('webSession.toolOutput') }}</div>
            <pre class="tool-code">{{ detailItem.output }}</pre>
          </div>
        </div>
      </details>
    </div>
    <div v-else class="command-execution-detail-empty">
      {{ t('webSession.compactToolEmpty') }}
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useLocale } from '@/composables/useLocale';
import type {
  WebSessionCommandExecutionGroupDetail,
  WebSessionCommandExecutionGroupItem,
} from '@/api/webSession';
import {
  formatWebSessionDateTime,
  formatWebSessionTimestamp,
} from '@/components/web-session/webSessionTimeFormat';

const props = defineProps<{
  show: boolean;
  loading: boolean;
  detail: WebSessionCommandExecutionGroupDetail | null;
  title: string;
  kindLabel: string;
}>();

const emit = defineEmits<{
  (event: 'update:show', show: boolean): void;
}>();

const { locale, t } = useLocale();
const items = computed(() => {
  if (!props.detail) {
    return [];
  }
  return [...props.detail.items].sort((left, right) => {
    const leftTime = Date.parse(left.completedAt || left.startedAt || left.timestamp || '') || 0;
    const rightTime =
      Date.parse(right.completedAt || right.startedAt || right.timestamp || '') || 0;
    return rightTime - leftTime;
  });
});

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return undefined;
  }
  return value as Record<string, unknown>;
}

function showInput(item: WebSessionCommandExecutionGroupItem) {
  const input = asRecord(item.input);
  if (!input) {
    return Boolean(item.input);
  }
  const keys = Object.keys(input);
  if (item.kind === 'command_execution') {
    return !(keys.length === 1 && keys[0] === 'command');
  }
  return keys.length > 0;
}

function stringifyValue(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  try {
    const serialized = JSON.stringify(value, null, 2);
    return typeof serialized === 'string' ? serialized : String(value ?? '');
  } catch {
    return String(value ?? '');
  }
}

function detailTimestamp(item: WebSessionCommandExecutionGroupItem) {
  return Date.parse(item.completedAt || item.startedAt || item.timestamp || '');
}

function formatDetailTime(item: WebSessionCommandExecutionGroupItem) {
  const timestamp = detailTimestamp(item);
  return Number.isFinite(timestamp) ? formatWebSessionTimestamp(timestamp, locale.value) : '';
}

function formatDetailDateTime(item: WebSessionCommandExecutionGroupItem) {
  const timestamp = detailTimestamp(item);
  return Number.isFinite(timestamp) ? formatWebSessionDateTime(timestamp, locale.value) : '';
}

function toolStateLabel(status: WebSessionCommandExecutionGroupItem['status']) {
  if (status === 'done') {
    return t('webSession.toolDone');
  }
  if (status === 'error') {
    return t('webSession.toolError');
  }
  return t('webSession.toolRunning');
}
</script>

<style scoped>
.command-execution-detail-summary {
  margin-bottom: 12px;
  font-size: 12px;
  color: var(--n-text-color-3);
}

.command-execution-detail-loading,
.command-execution-detail-empty {
  padding: 16px 4px 8px;
  font-size: 13px;
  color: var(--n-text-color-3);
}

.command-execution-detail-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.command-execution-detail-item {
  border: 1px solid color-mix(in srgb, var(--n-primary-color) 12%, var(--n-border-color));
  border-radius: 12px;
  background: color-mix(in srgb, var(--app-surface-color, #fff) 96%, var(--n-primary-color) 4%);
  overflow: hidden;
}

.command-execution-detail-item[open] {
  border-color: color-mix(in srgb, var(--n-primary-color) 22%, var(--n-border-color));
}

.command-execution-detail-item-summary {
  list-style: none;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  cursor: pointer;
}

.command-execution-detail-item-summary::-webkit-details-marker {
  display: none;
}

.command-execution-detail-item-label {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 56px;
  padding: 4px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--n-primary-color) 10%, transparent);
  color: var(--n-primary-color);
  font-size: 11px;
  font-weight: 700;
}

.command-execution-detail-item-command {
  min-width: 0;
  overflow: hidden;
  color: var(--app-text-color, var(--n-text-color-1, #111827));
  font-family: 'SFMono-Regular', 'JetBrains Mono', 'Consolas', monospace;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.command-execution-detail-item-time {
  font-size: 11px;
  color: var(--n-text-color-3);
}

.command-execution-detail-item-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 0 14px 14px;
}

.tool-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tool-section-label {
  color: var(--n-text-color-3);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

.tool-code {
  max-height: 360px;
  margin: 0;
  overflow: auto;
  padding: 12px;
  border-radius: 8px;
  background: var(--app-surface-sunken, #f6f8fa);
  color: var(--app-text-color, var(--n-text-color-1, #111827));
  font-family: 'SFMono-Regular', 'JetBrains Mono', 'Consolas', monospace;
  font-size: 12px;
  line-height: 1.55;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.tool-state-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--n-text-color-3);
  font-size: 11px;
  white-space: nowrap;
}

.tool-state-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}

.tool-state-badge.state-running {
  color: var(--n-warning-color, #f79009);
}

.tool-state-badge.state-done {
  color: var(--n-success-color, #12b76a);
}

.tool-state-badge.state-error {
  color: var(--n-error-color, #f04438);
}

@media (max-width: 720px) {
  .command-execution-detail-item-summary {
    grid-template-columns: 1fr auto;
    align-items: start;
  }

  .command-execution-detail-item-label {
    grid-column: 1 / -1;
    width: fit-content;
  }

  .command-execution-detail-item-command {
    white-space: normal;
    word-break: break-word;
  }

  .command-execution-detail-item-time {
    justify-self: end;
  }
}
</style>
