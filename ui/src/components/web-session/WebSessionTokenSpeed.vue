<template>
  <span class="token-speed" :title="description" :aria-label="`${description}: ${label}`">{{
    label
  }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { WebSessionTokenSpeed } from '@/types/models';
import { formatTokenSpeed } from '@/utils/webSessionTokenSpeed';

const props = defineProps<{ speed?: WebSessionTokenSpeed | null }>();
const { t } = useI18n();
const label = computed(() => formatTokenSpeed(props.speed));
const description = computed(() =>
  t(
    props.speed?.tokensPerSecond == null
      ? 'webSession.tokenSpeedUnavailable'
      : props.speed.estimated
        ? 'webSession.tokenSpeedEstimated'
        : 'webSession.tokenSpeedCalibrated'
  )
);
</script>

<style scoped>
.token-speed {
  display: inline-block;
  flex: 0 0 auto;
  color: var(--n-text-color-2);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  padding: 4px 2px;
}
</style>
