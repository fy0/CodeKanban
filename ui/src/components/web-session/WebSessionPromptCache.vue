<template>
  <DefineCacheDetails>
    <div v-if="cache" class="prompt-cache-details" :style="themeStyle">
      <div class="prompt-cache-heading">
        <strong v-if="!isMobile">{{ t('webSession.promptCacheDetails') }}</strong>
        <span class="prompt-cache-status" :class="`state-${state}`">{{ statusLabel }}</span>
      </div>
      <div class="prompt-cache-countdown">
        <strong>{{
          cache.remainingMs === null ? '—' : `≈${formatPromptCacheCountdown(cache.remainingMs)}`
        }}</strong>
        <span>{{ t('webSession.promptCacheWindowRemaining') }}</span>
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
      <p v-if="isMiss" class="prompt-cache-miss-note">{{ t('webSession.promptCacheMissHint') }}</p>
      <p class="prompt-cache-note">{{ hint }}</p>
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
        type="button"
        class="prompt-cache-ring"
        :class="[
          `state-${state}`,
          { 'is-refreshing': animation === 'refresh', 'is-interrupted': animation === 'miss' },
        ]"
        :style="themeStyle"
        :aria-label="`${t('webSession.promptCacheDetails')}: ${statusLabel} · ${countdownLabel}`"
        :aria-haspopup="isMobile ? 'dialog' : 'true'"
        :aria-expanded="isMobile ? showMobileDetails : showPopover"
        @click="openDetails"
        @focus="showPopover = !isMobile"
        @blur="showPopover = false"
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
          <path v-if="!isMiss" class="prompt-cache-glyph" d="m17.5 9-6 8h4l-1 6 6-8h-4z" />
          <path v-else class="prompt-cache-alert" d="M16 11v6m0 3v.1" />
        </svg>
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
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { createReusableTemplate } from '@vueuse/core';
import { NModal, NPopover, useThemeVars } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import { useResponsive } from '@/composables/useResponsive';
import type { WebSessionTokenSpeed } from '@/types/models';
import {
  estimateWebSessionPromptCache,
  formatPromptCacheCountdown,
  type PromptCacheSession,
} from './webSessionPromptCache';

const props = defineProps<{
  session: (PromptCacheSession & { id: string }) | null;
  now: number;
  speed?: WebSessionTokenSpeed | null;
}>();
const { t, locale } = useI18n();
const { isMobile } = useResponsive();
const [DefineCacheDetails, CacheDetails] = createReusableTemplate();
const showPopover = ref(false);
const showMobileDetails = ref(false);
const cache = computed(() =>
  estimateWebSessionPromptCache(props.session, props.now, props.speed?.history)
);
const isMiss = computed(
  () => cache.value?.event === 'miss' || cache.value?.event === 'interrupted'
);
const state = computed(() => (isMiss.value ? 'miss' : (cache.value?.state ?? 'unknown')));
const countdownLabel = computed(() =>
  cache.value?.remainingMs != null && cache.value.remainingMs > 0
    ? t('webSession.promptCacheRemaining', {
        time: formatPromptCacheCountdown(cache.value.remainingMs),
      })
    : t(
        cache.value?.state === 'elapsed'
          ? 'webSession.promptCacheUnconfirmed'
          : 'webSession.promptCacheUnknown'
      )
);
const statusLabel = computed(() => {
  const event = cache.value?.event;
  if (event === 'interrupted') return t('webSession.promptCacheInterrupted');
  if (event === 'miss') return t('webSession.promptCacheMiss');
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
    const newlyObservedMiss = current?.event && current.event !== 'refreshed' && !old?.event;
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
    animation.value = isMiss.value ? 'miss' : 'refresh';
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
onBeforeUnmount(() => clearTimeout(animationTimer));
function openDetails() {
  if (isMobile.value) showMobileDetails.value = true;
  else showPopover.value = true;
}
</script>

<style scoped>
.prompt-cache-ring {
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
.prompt-cache-ring.state-estimated {
  color: var(--cache-accent);
}
.prompt-cache-ring.state-ending,
.prompt-cache-ring.state-miss {
  color: var(--cache-warning);
}
.prompt-cache-ring:focus-visible {
  outline: 2px solid currentColor;
  outline-offset: 2px;
}
.prompt-cache-ring-graphic {
  width: 28px;
  height: 28px;
  overflow: visible;
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
.prompt-cache-status.state-ending {
  color: var(--cache-warning);
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
.prompt-cache-miss-note {
  margin: 12px 0 0;
  color: var(--cache-warning);
  line-height: 1.6;
}
@media (prefers-reduced-motion: reduce) {
  .is-refreshing .prompt-cache-ring-graphic,
  .is-interrupted .prompt-cache-ring-graphic {
    animation: none;
  }
  .prompt-cache-arc {
    transition: none;
  }
}
</style>
