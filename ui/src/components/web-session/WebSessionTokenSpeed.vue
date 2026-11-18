<template>
  <DefineSpeedDetails>
    <div class="token-speed-details" :class="{ 'is-mobile': isMobile }" :style="themeStyle">
      <strong v-if="!isMobile">{{ t('webSession.tokenSpeedDetails') }}</strong>
      <div class="token-speed-metrics">
        <button
          v-for="metric in metrics"
          :key="metric.key"
          type="button"
          class="token-speed-metric"
          :class="{ selected: selectedMetric === metric.key }"
          :aria-pressed="selectedMetric === metric.key"
          @click="selectedMetric = metric.key"
        >
          <span>{{ t(`webSession.tokenSpeedMetric_${metric.key}`) }}</span>
          <strong>
            <Transition name="token-speed-update">
              <span :key="formatTokenSpeed(metric.value)" class="token-speed-value">{{
                formatTokenSpeed(metric.value)
              }}</span>
            </Transition>
          </strong>
          <small v-if="metric.summary">{{
            t('webSession.tokenSpeedSampleCount', { count: metric.summary.sampleCount })
          }}</small>
        </button>
      </div>
      <p class="token-speed-note">{{ t('webSession.tokenSpeedMethod') }}</p>
      <template v-if="history.length">
        <div class="token-speed-history-title">
          <strong>{{ t('webSession.tokenSpeedHistory') }}</strong>
          <small>{{ t('webSession.tokenSpeedOldToNew') }}</small>
        </div>
        <svg
          v-if="history.length > 1"
          class="token-speed-trend"
          viewBox="0 0 360 52"
          preserveAspectRatio="none"
          role="img"
          :aria-label="t('webSession.tokenSpeedTrend')"
        >
          <polyline
            :points="trendPoints"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            vector-effect="non-scaling-stroke"
          />
        </svg>
        <div class="token-speed-history">
          <div
            v-for="record in history"
            :key="`${record.runId}:${record.sampleId}`"
            class="token-speed-record"
          >
            <div class="token-speed-record-info">
              <div class="token-speed-record-time">
                <time :datetime="record.endedAt" :title="formatTime(record.endedAt, true)">{{
                  formatTime(record.endedAt)
                }}</time>
                <span
                  v-if="cacheMarker(record)"
                  class="token-speed-cache-marker"
                  :class="`cache-${cacheMarker(record)!.event}`"
                  :title="cacheMarker(record)!.hint"
                  :aria-label="cacheMarker(record)!.hint"
                >
                  <svg viewBox="0 0 16 16" aria-hidden="true">
                    <path
                      v-if="cacheMarker(record)!.event === 'rebuilt'"
                      d="M13 6a5 5 0 1 0 0 4M13 2v4H9"
                    />
                    <path
                      v-else-if="cacheMarker(record)!.event === 'hit'"
                      d="M3 8h10m-4-4 4 4-4 4"
                    />
                    <path
                      v-else-if="cacheMarker(record)!.event === 'suspected'"
                      d="M5.5 5.5a2.5 2.5 0 1 1 4 2c-1 .7-1.5 1-1.5 2.5m0 3v.1"
                    />
                    <path v-else d="M8 3v6m0 3v.1" />
                  </svg>
                  <span>{{ cacheMarker(record)!.label }} {{ cacheMarker(record)!.percent }}%</span>
                </span>
              </div>
              <div class="token-speed-record-meta">
                <small class="token-speed-model" :title="record.model || record.agent">{{
                  record.model || record.agent || '—'
                }}</small>
                <small
                  class="token-speed-first-token"
                  :title="t('webSession.tokenSpeedFirstTokenHint')"
                  >{{ t('webSession.tokenSpeedFirstToken') }}
                  {{ record.firstTokenMs == null ? '--' : (record.firstTokenMs / 1000).toFixed(1) }}
                  s</small
                >
              </div>
            </div>
            <div class="token-speed-record-info token-speed-record-value">
              <strong>{{ formatTokenSpeed(record) }}</strong>
              <small
                >{{ record.estimated ? '≈' : ''
                }}{{ Math.round(record.outputTokens).toLocaleString() }} tokens ·
                {{ (record.durationMs / 1000).toFixed(1) }} s</small
              >
            </div>
          </div>
        </div>
      </template>
      <p v-else class="token-speed-note">{{ t('webSession.tokenSpeedNoHistory') }}</p>
    </div>
  </DefineSpeedDetails>
  <n-popover
    trigger="hover"
    placement="top-end"
    :show-arrow="false"
    :keep-alive-on-hover="true"
    :disabled="isMobile"
  >
    <template #trigger>
      <button
        type="button"
        class="token-speed"
        :style="themeStyle"
        :aria-label="`${t('webSession.tokenSpeedDetails')}: ${metricLabel} ${label}`"
        :aria-haspopup="isMobile ? 'dialog' : undefined"
        :aria-expanded="isMobile ? showMobileDetails : undefined"
        @click="showMobileDetails = isMobile"
      >
        <Transition name="token-speed-update">
          <span :key="label" class="token-speed-value">{{ label }}</span>
        </Transition>
      </button>
    </template>
    <SpeedDetails v-if="!isMobile" />
  </n-popover>
  <n-modal
    v-if="isMobile"
    v-model:show="showMobileDetails"
    preset="card"
    class="token-speed-modal"
    :title="t('webSession.tokenSpeedDetails')"
    :style="{ width: 'min(480px, calc(100vw - 24px))' }"
    :content-style="{ maxHeight: 'calc(100dvh - 160px)', overflowY: 'auto' }"
  >
    <SpeedDetails />
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { createReusableTemplate } from '@vueuse/core';
import { NModal, NPopover, useThemeVars } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import { useResponsive } from '@/composables/useResponsive';
import type { WebSessionTokenSpeed, WebSessionTokenSpeedRecord } from '@/types/models';
import { promptCacheRecordEvent } from './webSessionPromptCache';
import {
  formatTokenSpeed,
  selectTokenSpeedMetric,
  type TokenSpeedMetric,
} from '@/utils/webSessionTokenSpeed';

const props = defineProps<{ speed?: WebSessionTokenSpeed | null }>();
const { t, locale } = useI18n();
const { isMobile } = useResponsive();
const [DefineSpeedDetails, SpeedDetails] = createReusableTemplate();
const showMobileDetails = ref(false);
watch(isMobile, mobile => {
  if (!mobile) showMobileDetails.value = false;
});
const themeVars = useThemeVars();
const themeStyle = computed(() => ({
  '--token-speed-accent': themeVars.value.primaryColor,
  '--token-speed-secondary': themeVars.value.textColor2,
  '--token-speed-muted': themeVars.value.textColor3,
  '--token-speed-divider': themeVars.value.dividerColor,
  '--token-speed-warning': themeVars.value.warningColor,
  '--token-speed-error': themeVars.value.errorColor,
}));
const selectedMetric = ref<TokenSpeedMetric>('recent5');
const selected = computed(() => selectTokenSpeedMetric(props.speed, selectedMetric.value));
const label = computed(() => formatTokenSpeed(selected.value.value));
const metricLabel = computed(() => t(`webSession.tokenSpeedMetric_${selected.value.metric}`));
const history = computed(() => props.speed?.history ?? []);
const cacheMarkers = computed(
  () =>
    new Map(
      history.value.map((record, index) => {
        const previous = history.value[index + 1];
        const event = promptCacheRecordEvent(record, previous);
        if (!event) return [`${record.runId}:${record.sampleId}`, null];
        const cached = Math.min(record.inputTokens ?? 0, record.cachedInputTokens ?? 0);
        const percent = Math.round((cached / (record.inputTokens || 1)) * 100);
        const detail = t(
          cached > 0 ? 'webSession.promptCacheRecordHit' : 'webSession.promptCacheRecordMiss',
          {
            cached: cached.toLocaleString(locale.value),
            input: record.inputTokens?.toLocaleString(locale.value),
          }
        );
        const label = t(`webSession.promptCacheRecordLabel_${event}`);
        let hint = `${label} ${percent}%. ${detail}`;
        if (event === 'rebuilt' || event === 'suspected') {
          const previousCached = Math.min(previous!.inputTokens!, previous!.cachedInputTokens!);
          const decrease = ((previousCached - cached) / previousCached) * 100;
          hint += ` ${t('webSession.promptCacheDecreaseHint', {
            previous: previousCached.toLocaleString(locale.value),
            cached: cached.toLocaleString(locale.value),
            decrease: decrease < 1 ? '<1' : Math.round(decrease),
          })}`;
        }
        return [
          `${record.runId}:${record.sampleId}`,
          {
            event,
            percent,
            label,
            hint,
          },
        ];
      })
    )
);
function cacheMarker(record: WebSessionTokenSpeedRecord) {
  return cacheMarkers.value.get(`${record.runId}:${record.sampleId}`);
}
const metrics = computed(() =>
  (['latest', 'currentRun', 'recent5', 'recent20', 'session'] as const).map(key => ({
    key,
    value: key === 'latest' ? props.speed : props.speed?.stats?.[key],
    summary: key === 'latest' ? null : props.speed?.stats?.[key],
  }))
);
const trendPoints = computed(() => {
  const records = [...history.value].reverse();
  const max = Math.max(1, ...records.map(record => record.tokensPerSecond));
  return records
    .map(
      (record, i) =>
        `${4 + (i * 352) / Math.max(1, records.length - 1)},${48 - (record.tokensPerSecond / max) * 44}`
    )
    .join(' ');
});
function formatTime(value: string, full = false) {
  return new Date(value).toLocaleString(
    locale.value,
    full
      ? { dateStyle: 'medium', timeStyle: 'medium' }
      : { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }
  );
}
</script>

<style scoped>
.token-speed {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  flex: 0 0 auto;
  color: var(--token-speed-secondary);
  background: transparent;
  border: 0;
  border-radius: 4px;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  line-height: 16px;
  text-align: end;
  white-space: nowrap;
  padding: 4px 2px;
  cursor: pointer;
}
.token-speed:hover,
.token-speed:focus-visible {
  color: var(--token-speed-accent);
}
.token-speed-value {
  display: inline-block;
  white-space: nowrap;
}
.token-speed-update-enter-active {
  animation: token-speed-update 220ms cubic-bezier(0.16, 1, 0.3, 1);
}
.token-speed-update-leave-active {
  display: none;
}
@keyframes token-speed-update {
  from {
    opacity: 0.45;
    transform: translateY(0.25em);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
@media (prefers-reduced-motion: reduce) {
  .token-speed-update-enter-active {
    animation: none;
  }
}
.token-speed-details {
  width: min(380px, calc(100vw - 48px));
  max-height: calc(100dvh - 100px);
  overflow-y: auto;
  overscroll-behavior: contain;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.token-speed-details.is-mobile {
  width: 100%;
  max-height: none;
  overflow: visible;
}
.token-speed-details.is-mobile .token-speed-history {
  max-height: none;
  overflow: visible;
}
.token-speed-metrics {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
  margin-top: 10px;
}
.token-speed-metric {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 3px;
  padding: 8px 10px;
  background: transparent;
  color: inherit;
  border: 1px solid var(--token-speed-divider);
  border-radius: 6px;
  cursor: pointer;
  text-align: start;
  font: inherit;
}
.token-speed-metric:first-child {
  grid-column: 1 / -1;
}
.token-speed-metric.selected {
  border-color: var(--token-speed-accent);
  color: var(--token-speed-accent);
}
.token-speed-metric strong {
  font-size: 15px;
}
.token-speed-note,
small {
  color: var(--token-speed-muted);
}
.token-speed-note {
  margin: 10px 0;
  line-height: 1.6;
}
.token-speed-history-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.token-speed-trend {
  display: block;
  width: 100%;
  height: 52px;
  margin: 8px 0;
  color: var(--token-speed-accent);
}
.token-speed-history {
  max-height: 220px;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.token-speed-record {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 7px 0;
  border-bottom: 1px solid var(--token-speed-divider);
}
.token-speed-record-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.token-speed-record-time {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  line-height: 16px;
}
.token-speed-record-time time {
  text-align: end;
  white-space: nowrap;
}
.token-speed-cache-marker {
  display: inline-flex;
  align-items: center;
  justify-content: flex-start;
  gap: 2px;
  color: var(--token-speed-accent);
  font-size: 10px;
  text-align: start;
  white-space: nowrap;
}
.token-speed-cache-marker svg {
  flex: none;
  width: 11px;
  height: 11px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.token-speed-cache-marker.cache-miss,
.token-speed-cache-marker.cache-suspected {
  color: var(--token-speed-warning);
}
.token-speed-cache-marker.cache-suspected {
  border-bottom: 1px dashed currentColor;
}
.token-speed-cache-marker.cache-rebuilt {
  color: var(--token-speed-error);
  background: color-mix(in srgb, var(--token-speed-error) 10%, transparent);
  border-radius: 3px;
  padding: 0 3px;
}
.token-speed-model {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.token-speed-record-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.token-speed-first-token {
  flex-shrink: 0;
  white-space: nowrap;
}
.token-speed-record-value {
  flex-shrink: 0;
  align-items: flex-end;
}
</style>
