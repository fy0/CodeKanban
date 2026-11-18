<template>
  <DefineCacheDetails>
    <div
      v-if="cache"
      :id="detailsId"
      :role="isMobile ? undefined : 'dialog'"
      :aria-label="t('webSession.promptCacheDetails')"
      class="prompt-cache-details"
      :style="themeStyle"
      @focusout="closeOnFocusExit"
      @keydown.esc="dismissDesktopDetails"
    >
      <div class="prompt-cache-heading">
        <strong v-if="!isMobile">{{ t('webSession.promptCacheDetails') }}</strong>
        <span class="prompt-cache-status" :class="`state-${state}`">{{ statusLabel }}</span>
      </div>
      <div class="prompt-cache-countdown">
        <strong>{{ countdownText }}</strong>
        <span>{{ countdownCaption }}</span>
      </div>
      <dl class="prompt-cache-stats">
        <div>
          <dt>{{ t('webSession.promptCacheLastHit') }}</dt>
          <dd>{{ cache.hitPercent === null ? '—' : `${cache.hitPercent}%` }}</dd>
        </div>
        <div>
          <dt>{{ t('webSession.promptCacheTokens') }}</dt>
          <dd>
            {{
              cache.hitPercent === null
                ? '—'
                : `${cache.cachedTokens.toLocaleString(locale)} / ${cache.inputTokens.toLocaleString(locale)}`
            }}
          </dd>
        </div>
        <div v-if="cache.observedAt !== null">
          <dt>{{ t('webSession.promptCacheObservedAt') }}</dt>
          <dd>{{ new Date(cache.observedAt).toLocaleString(locale) }}</dd>
        </div>
      </dl>
      <p v-if="isDecreased" class="prompt-cache-decrease-note" :class="`state-${state}`">
        {{
          t('webSession.promptCacheDecreaseHint', {
            previous: cache.previousCachedTokens?.toLocaleString(locale),
            cached: cache.cachedTokens.toLocaleString(locale),
            decrease: decreasePercent,
          })
        }}
      </p>
      <p v-else-if="isMiss" class="prompt-cache-miss-note">
        {{ t('webSession.promptCacheMissHint') }}
      </p>
      <p class="prompt-cache-note">{{ hint }}</p>
      <div v-if="renewal && renewal.availability !== 'disabled'" class="prompt-cache-renewal">
        <label class="prompt-cache-renewal-checkbox">
          <input
            type="checkbox"
            :checked="renewalActive"
            :disabled="renewal.availability !== 'ready' || renewalUpdating"
            :aria-describedby="`${detailsId}-renewal-note`"
            @change="toggleRenewal"
          />
          {{ t('webSession.promptCacheRenewalLabel') }}
        </label>
        <p :id="`${detailsId}-renewal-note`" class="prompt-cache-note">
          {{ t('webSession.promptCacheRenewalHint') }}
        </p>
        <div class="prompt-cache-renewal-controls">
          <span role="status">{{ renewalStatus }}</span>
          <button
            v-if="renewalActive"
            type="button"
            class="prompt-cache-renewal-action"
            :disabled="renewalUpdating"
            @click="emit('renewalAction', 'cycle')"
          >
            {{ t(renewalActionLabel) }}
          </button>
        </div>
        <p
          v-if="renewal.lastError && renewal.availability !== 'unsupported'"
          role="status"
          class="prompt-cache-renewal-error"
        >
          {{ t('webSession.promptCacheRenewalFailed', { category: renewal.lastError }) }}
        </p>
      </div>
    </div>
  </DefineCacheDetails>
  <n-popover
    v-if="cache"
    v-model:show="showPopover"
    trigger="hover"
    placement="top-end"
    :show-arrow="false"
    :keep-alive-on-hover="true"
    :disabled="isMobile"
  >
    <template #trigger>
      <button
        ref="ring"
        type="button"
        class="prompt-cache-ring"
        :class="[
          `state-${state}`,
          {
            'is-mobile': isMobile,
            'is-refreshing': animation === 'refresh',
            'is-interrupted': animation === 'miss',
          },
        ]"
        :style="themeStyle"
        :aria-label="
          [
            `${t('webSession.promptCacheDetails')}: ${statusLabel}`,
            countdownLabel,
            renewalStatus,
            renewalError,
          ]
            .filter(Boolean)
            .join(' · ')
        "
        aria-haspopup="dialog"
        :aria-expanded="isMobile ? showMobileDetails : showPopover"
        @click="openDetails"
        @focus="showPopover = !isMobile"
        @blur="closeOnFocusExit"
        @keydown.tab="focusRenewalControls"
        @keydown.esc="showPopover = false"
      >
        <svg
          :key="animationVersion"
          class="prompt-cache-ring-graphic"
          viewBox="0 0 32 32"
          aria-hidden="true"
        >
          <circle class="prompt-cache-track" cx="16" cy="16" r="12" />
          <circle
            class="prompt-cache-arc"
            cx="16"
            cy="16"
            r="12"
            pathLength="100"
            :stroke-dasharray="`${cache.remainingRatio * 100} 100`"
            transform="rotate(-90 16 16)"
          />
          <path
            v-if="cache.event === 'rebuilt'"
            class="prompt-cache-alert"
            d="M23 13a8 8 0 1 0 0 7m0-13v6h-6"
          />
          <path
            v-else-if="cache.event === 'suspected'"
            class="prompt-cache-alert"
            d="M12 12a4 4 0 1 1 6.4 3.2C16.8 16.3 16 17 16 19m0 4v.1"
          />
          <path v-else-if="!isMiss" class="prompt-cache-glyph" d="m17.5 9-6 8h4l-1 6 6-8h-4z" />
          <path v-else class="prompt-cache-alert" d="M16 11v6m0 3v.1" />
        </svg>
        <span v-if="renewalActive" class="prompt-cache-renewal-badge" aria-hidden="true">
          <svg v-if="renewal?.infinite" class="prompt-cache-renewal-loop" viewBox="0 0 16 16">
            <path d="M13 5.5a5.5 5.5 0 1 0 .2 4.5M13 2.5v3.4H9.6" />
          </svg>
          <template v-else>{{ renewal?.remaining }}</template>
        </span>
      </button>
    </template>
    <CacheDetails v-if="!isMobile" />
  </n-popover>
  <n-modal
    v-if="isMobile && cache"
    v-model:show="showMobileDetails"
    preset="card"
    :title="t('webSession.promptCacheDetails')"
    :style="{ width: 'min(380px, calc(100vw - 24px))' }"
    :content-style="{ maxHeight: 'calc(100dvh - 160px)', overflowY: 'auto' }"
  >
    <CacheDetails />
  </n-modal>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue';
import { createReusableTemplate } from '@vueuse/core';
import { NModal, NPopover, useThemeVars } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import { useResponsive } from '@/composables/useResponsive';
import type { WebSessionTokenSpeed } from '@/types/models';
import type { PromptCacheRenewalState } from '@/api/webSession';
import {
  estimateWebSessionPromptCache,
  formatPromptCacheCountdown,
  type PromptCacheSession,
} from './webSessionPromptCache';

const props = defineProps<{
  session: (PromptCacheSession & { id: string }) | null;
  now: number;
  speed?: WebSessionTokenSpeed | null;
  renewal?: PromptCacheRenewalState;
  renewalUpdating?: boolean;
}>();
const emit = defineEmits<{
  renewalDetails: [open: boolean];
  renewalAction: [action: 'cycle' | 'off'];
}>();
const { t, locale } = useI18n();
const { isMobile } = useResponsive();
const [DefineCacheDetails, CacheDetails] = createReusableTemplate();
const showPopover = ref(false);
const showMobileDetails = ref(false);
const ring = ref<HTMLButtonElement>();
const detailsId = useId();
const renewalActive = computed(() => Boolean(props.renewal?.infinite || props.renewal?.remaining));
const renewalStatus = computed(() => {
  const renewal = props.renewal;
  if (!renewal || renewal.availability === 'disabled') return '';
  if (renewal.availability === 'unsupported') return t('webSession.promptCacheRenewalUnsupported');
  if (renewal.availability === 'unavailable') return t('webSession.promptCacheRenewalUnavailable');
  if (renewal.availability === 'checking') return t('webSession.promptCacheRenewalChecking');
  if (renewal.infinite) return t('webSession.promptCacheRenewalInfinite');
  if (renewal.remaining)
    return t('webSession.promptCacheRenewalRemaining', { count: renewal.remaining });
  return t('webSession.promptCacheRenewalOff');
});
const renewalError = computed(() =>
  props.renewal?.lastError &&
  props.renewal.availability !== 'unsupported' &&
  props.renewal.availability !== 'disabled'
    ? t('webSession.promptCacheRenewalFailed', { category: props.renewal.lastError })
    : ''
);
const renewalActionLabel = computed(() => {
  if (props.renewal?.infinite) return 'webSession.promptCacheRenewalStop';
  if (props.renewal?.remaining === 3) return 'webSession.promptCacheRenewalMakeInfinite';
  return 'webSession.promptCacheRenewalAdd';
});
const cache = computed(() =>
  estimateWebSessionPromptCache(props.session, props.now, props.speed?.history)
);
const isMiss = computed(() => cache.value?.event === 'miss');
const isDecreased = computed(
  () => cache.value?.event === 'rebuilt' || cache.value?.event === 'suspected'
);
const decreasePercent = computed(() => {
  const previous = cache.value?.previousCachedTokens;
  if (!previous) return null;
  const decrease = ((previous - cache.value!.cachedTokens) / previous) * 100;
  return decrease < 1 ? '<1' : Math.round(decrease);
});
const state = computed(() =>
  isMiss.value ? 'miss' : isDecreased.value ? cache.value!.event : (cache.value?.state ?? 'unknown')
);
function formatExpiredLabel(expiredMs: number | null) {
  if (expiredMs == null || expiredMs < 60_000) {
    return t('webSession.promptCacheExpiredJustNow');
  }
  const minutes = Math.floor(expiredMs / 60_000);
  if (minutes < 60) {
    return t('webSession.promptCacheExpiredMinutes', { minutes });
  }
  const hours = Math.floor(minutes / 60);
  const value = hours < 24 ? hours : Math.floor(hours / 24);
  const unit: Intl.RelativeTimeFormatUnit = hours < 24 ? 'hour' : 'day';
  return new Intl.RelativeTimeFormat(locale.value, { numeric: 'always' }).format(-value, unit);
}
const countdownText = computed(() => {
  const value = cache.value;
  if (!value || value.remainingMs === null) return '—';
  if (value.remainingMs > 0) return `≈${formatPromptCacheCountdown(value.remainingMs)}`;
  return formatExpiredLabel(value.expiredMs);
});
const countdownCaption = computed(() =>
  cache.value?.remainingMs === 0
    ? t('webSession.promptCacheWindowExpired')
    : t('webSession.promptCacheWindowRemaining')
);
const countdownLabel = computed(() => {
  const value = cache.value;
  if (!value || value.remainingMs === null) return t('webSession.promptCacheUnknown');
  if (value.remainingMs > 0) {
    return t('webSession.promptCacheRemaining', {
      time: formatPromptCacheCountdown(value.remainingMs),
    });
  }
  return countdownText.value;
});
const statusLabel = computed(() => {
  const event = cache.value?.event;
  if (event === 'miss') return t('webSession.promptCacheMiss');
  if (event === 'rebuilt')
    return t('webSession.promptCacheRebuilt', { percent: cache.value?.hitPercent });
  if (event === 'suspected')
    return t('webSession.promptCacheSuspected', { percent: cache.value?.hitPercent });
  if (cache.value?.state === 'elapsed') return t('webSession.promptCacheUnconfirmed');
  if (cache.value?.hitPercent) return t('webSession.promptCacheHit');
  return t('webSession.promptCacheUnknown');
});
const hint = computed(() => {
  if (!cache.value?.observedAt) return t('webSession.promptCachePendingHint');
  if (!cache.value.windowMs) return t('webSession.promptCacheUnknownHint');
  if (cache.value.inputTokens < 1024) return t('webSession.promptCacheSmallHint');
  return t('webSession.promptCacheEstimateHint');
});
const themeVars = useThemeVars();
const themeStyle = computed(() => ({
  '--cache-accent': themeVars.value.successColor,
  '--cache-warning': themeVars.value.warningColor,
  '--cache-error': themeVars.value.errorColor,
  '--cache-muted': themeVars.value.textColor3,
  '--cache-divider': themeVars.value.dividerColor,
}));
const animation = ref('');
const animationVersion = ref(0);
let animationTimer: ReturnType<typeof setTimeout> | undefined;
watch(
  () => ({ sessionId: props.session?.id, cache: cache.value }),
  (next, previous) => {
    if (next.sessionId !== previous.sessionId) {
      showPopover.value = false;
      showMobileDetails.value = false;
      animation.value = '';
      clearTimeout(animationTimer);
      return;
    }
    const current = next.cache;
    const old = previous.cache;
    const newlyObservedMiss = current?.event && current.event !== 'hit' && !old?.event;
    if (
      !current?.observationKey ||
      !old?.observationKey ||
      current.observationKey === old.observationKey ||
      ((current.observedAt ?? 0) <= (old.observedAt ?? 0) && !newlyObservedMiss) ||
      (!current.event && !current.cachedTokens)
    )
      return;
    // The summary and durable record can describe the same request. Hydration
    // and delayed duplicate reports must not look like another cache refresh.
    if (
      old.observationKey.startsWith('snapshot:') &&
      !current.observationKey.startsWith('snapshot:') &&
      old.inputTokens === current.inputTokens &&
      old.cachedTokens === current.cachedTokens &&
      !newlyObservedMiss
    )
      return;
    animation.value = isMiss.value || current.event === 'suspected' ? 'miss' : 'refresh';
    animationVersion.value++;
    clearTimeout(animationTimer);
    animationTimer = setTimeout(() => {
      animation.value = '';
    }, 700);
  }
);
watch(isMobile, () => {
  showPopover.value = false;
  showMobileDetails.value = false;
});
watch(
  () => showPopover.value || showMobileDetails.value,
  open => emit('renewalDetails', open)
);
onBeforeUnmount(() => {
  clearTimeout(animationTimer);
  if (showPopover.value || showMobileDetails.value) emit('renewalDetails', false);
});
function toggleRenewal(event: Event) {
  if (!(event.target instanceof HTMLInputElement)) return;
  emit('renewalAction', event.target.checked ? 'cycle' : 'off');
  event.target.checked = renewalActive.value;
}
function closeOnFocusExit(event: FocusEvent) {
  const next = event.relatedTarget;
  if (next instanceof HTMLElement && document.getElementById(detailsId)?.contains(next)) return;
  showPopover.value = false;
}
function dismissDesktopDetails(event: KeyboardEvent) {
  if (isMobile.value) return;
  event.stopPropagation();
  ring.value?.focus();
  showPopover.value = false;
}
function focusRenewalControls(event: KeyboardEvent) {
  if (event.shiftKey || isMobile.value || !showPopover.value) return;
  const checkbox = document
    .getElementById(detailsId)
    ?.querySelector<HTMLInputElement>('input:not(:disabled)');
  if (checkbox) {
    event.preventDefault();
    checkbox.focus();
  }
}
function openDetails() {
  if (isMobile.value) showMobileDetails.value = true;
  else showPopover.value = true;
}
</script>

<style scoped>
.prompt-cache-ring {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 28px;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  color: var(--cache-muted);
  background: transparent;
  cursor: pointer;
}
.prompt-cache-renewal-badge {
  position: absolute;
  right: -5px;
  bottom: -3px;
  display: grid;
  place-items: center;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--cache-accent);
  color: #111;
  font-size: 9px;
  line-height: 1;
  pointer-events: none;
}
.prompt-cache-renewal-loop {
  width: 10px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
  animation: cache-renewal-spin 1.6s linear infinite;
}
@keyframes cache-renewal-spin {
  to {
    transform: rotate(360deg);
  }
}
.prompt-cache-renewal {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--cache-divider);
}
.prompt-cache-renewal-checkbox,
.prompt-cache-renewal-controls {
  display: flex;
  align-items: center;
  gap: 6px;
}
.prompt-cache-renewal-controls {
  flex-wrap: wrap;
  justify-content: space-between;
  margin-top: 6px;
  font-size: 12px;
}
.prompt-cache-renewal-action {
  border: 1px solid var(--cache-divider);
  border-radius: 4px;
  padding: 2px 6px;
  background: transparent;
  color: inherit;
}
.prompt-cache-renewal-error {
  color: var(--cache-error);
  font-size: 12px;
}
.prompt-cache-ring.state-estimated {
  color: var(--cache-accent);
}
.prompt-cache-ring.state-ending,
.prompt-cache-ring.state-suspected,
.prompt-cache-ring.state-miss {
  color: var(--cache-warning);
}
.prompt-cache-ring.state-rebuilt {
  color: var(--cache-error);
}
.prompt-cache-ring:focus-visible {
  outline: 2px solid currentColor;
  outline-offset: 2px;
}
.prompt-cache-ring-graphic {
  display: block;
  width: 24px;
  height: 24px;
  overflow: visible;
}
.prompt-cache-ring.is-mobile {
  flex-basis: 28px;
  width: 28px;
  height: 28px;
}
.prompt-cache-ring.is-mobile .prompt-cache-ring-graphic {
  width: 20px;
  height: 20px;
}
.prompt-cache-track,
.prompt-cache-arc {
  fill: none;
  stroke-width: 2.5;
}
.prompt-cache-track {
  stroke: var(--cache-divider);
}
.prompt-cache-arc {
  stroke: currentColor;
  stroke-linecap: round;
  transition: stroke-dasharray 500ms ease;
}
.prompt-cache-glyph {
  fill: currentColor;
}
.prompt-cache-alert {
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
}
.is-refreshing .prompt-cache-ring-graphic {
  animation: cache-refresh 650ms ease-out;
}
.is-interrupted .prompt-cache-ring-graphic {
  animation: cache-interrupted 650ms ease-out;
}
@keyframes cache-refresh {
  0% {
    transform: rotate(-110deg) scale(0.82);
    opacity: 0.4;
  }
  60% {
    transform: rotate(12deg) scale(1.1);
  }
  100% {
    transform: none;
    opacity: 1;
  }
}
@keyframes cache-interrupted {
  0%,
  100% {
    transform: rotate(0);
  }
  30% {
    transform: rotate(-22deg) scale(0.88);
    opacity: 0.4;
  }
  65% {
    transform: rotate(14deg) scale(1.12);
  }
}
.prompt-cache-details {
  width: min(320px, calc(100vw - 72px));
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.prompt-cache-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.prompt-cache-heading strong {
  font-size: 13px;
}
.prompt-cache-status {
  color: var(--cache-muted);
}
.prompt-cache-status.state-estimated {
  color: var(--cache-accent);
}
.prompt-cache-status.state-miss,
.prompt-cache-status.state-suspected,
.prompt-cache-status.state-ending {
  color: var(--cache-warning);
}
.prompt-cache-status.state-rebuilt,
.prompt-cache-decrease-note.state-rebuilt {
  color: var(--cache-error);
}
.prompt-cache-countdown {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 16px 0;
}
.prompt-cache-countdown strong {
  font-size: 28px;
  line-height: 1.2;
  font-weight: 600;
}
.prompt-cache-countdown span,
.prompt-cache-stats dt,
.prompt-cache-note {
  color: var(--cache-muted);
}
.prompt-cache-stats {
  display: grid;
  gap: 10px;
  margin: 0;
}
.prompt-cache-stats > div {
  display: flex;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 4px 12px;
}
.prompt-cache-stats dd {
  margin: 0;
}
.prompt-cache-note {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--cache-divider);
  line-height: 1.6;
}
.prompt-cache-miss-note,
.prompt-cache-decrease-note {
  margin: 12px 0 0;
  color: var(--cache-warning);
  line-height: 1.6;
}
@media (prefers-reduced-motion: reduce) {
  .prompt-cache-renewal-loop {
    animation: none;
  }
  .is-refreshing .prompt-cache-ring-graphic,
  .is-interrupted .prompt-cache-ring-graphic {
    animation: none;
  }
  .prompt-cache-arc {
    transition: none;
  }
}
</style>
