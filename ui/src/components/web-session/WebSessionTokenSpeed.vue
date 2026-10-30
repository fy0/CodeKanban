<template>
  <n-popover trigger="hover" placement="top-end" :show-arrow="false" :keep-alive-on-hover="true">
    <template #trigger>
      <button
        type="button"
        class="token-speed"
        :style="themeStyle"
        :aria-label="`${t('webSession.tokenSpeedDetails')}: ${metricLabel} ${label}`"
      >
        <Transition name="token-speed-update">
          <span :key="label" class="token-speed-value">{{ label }}</span>
        </Transition>
      </button>
    </template>
    <div class="token-speed-details" :style="themeStyle">
      <strong>{{ t('webSession.tokenSpeedDetails') }}</strong>
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
              <time :datetime="record.endedAt" :title="formatTime(record.endedAt, true)">{{
                formatTime(record.endedAt)
              }}</time>
              <small class="token-speed-model" :title="record.model || record.agent">{{
                record.model || record.agent || '—'
              }}</small>
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
  </n-popover>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { NPopover, useThemeVars } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import type { WebSessionTokenSpeed } from '@/types/models';
import {
  formatTokenSpeed,
  selectTokenSpeedMetric,
  type TokenSpeedMetric,
} from '@/utils/webSessionTokenSpeed';

const props = defineProps<{ speed?: WebSessionTokenSpeed | null }>();
const { t, locale } = useI18n();
const themeVars = useThemeVars();
const themeStyle = computed(() => ({
  '--token-speed-accent': themeVars.value.primaryColor,
  '--token-speed-secondary': themeVars.value.textColor2,
  '--token-speed-muted': themeVars.value.textColor3,
  '--token-speed-divider': themeVars.value.dividerColor,
}));
const selectedMetric = ref<TokenSpeedMetric>('recent5');
const selected = computed(() => selectTokenSpeedMetric(props.speed, selectedMetric.value));
const label = computed(() => formatTokenSpeed(selected.value.value));
const metricLabel = computed(() => t(`webSession.tokenSpeedMetric_${selected.value.metric}`));
const history = computed(() => props.speed?.history ?? []);
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
  flex: 0 0 auto;
  color: var(--token-speed-secondary);
  background: transparent;
  border: 0;
  border-radius: 4px;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
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
.token-speed-model {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.token-speed-record-value {
  flex-shrink: 0;
  align-items: flex-end;
}
</style>
