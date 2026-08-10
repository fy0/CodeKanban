<template>
  <div class="work-timing-backfill-entry">
    <div class="work-timing-backfill-entry__copy">
      <div class="work-timing-backfill-entry__title">
        {{ t('settings.workTimingBackfillTitle') }}
      </div>
      <div class="form-tip">{{ t('settings.workTimingBackfillDescription') }}</div>
      <div v-if="status" class="work-timing-backfill-entry__status">
        {{
          t('settings.workTimingBackfillRemaining', {
            count: status.remainingSessionCount,
          })
        }}
      </div>
    </div>

    <div class="work-timing-backfill-entry__actions">
      <label class="work-timing-backfill-entry__batch">
        <span>{{ t('settings.workTimingBackfillBatchSize') }}</span>
        <n-input-number
          v-model:value="batchSize"
          :min="1"
          :max="500"
          :step="1"
          :disabled="running"
          :aria-label="t('settings.workTimingBackfillBatchSize')"
        />
      </label>
      <n-button
        type="primary"
        secondary
        :loading="running"
        :disabled="running || status?.remainingSessionCount === 0"
        @click="runBatch"
      >
        <template #icon>
          <n-icon><RefreshOutline /></n-icon>
        </template>
        {{
          running ? t('settings.workTimingBackfillRunning') : t('settings.workTimingBackfillAction')
        }}
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useMessage } from 'naive-ui';
import { RefreshOutline } from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import { webSessionApi, type WebSessionWorkTimingBackfillStatus } from '@/api/webSession';

const { t } = useLocale();
const message = useMessage();
const batchSize = ref<number | null>(50);
const running = ref(false);
const status = ref<WebSessionWorkTimingBackfillStatus | null>(null);

async function loadStatus() {
  try {
    status.value = await webSessionApi.workTimingBackfillStatus();
  } catch (error) {
    console.error('Failed to load web session work timing status:', error);
  }
}

async function runBatch() {
  if (running.value) {
    return;
  }
  running.value = true;
  try {
    const result = await webSessionApi.runWorkTimingBackfill(
      Math.max(1, Math.min(500, Math.trunc(Number(batchSize.value ?? 50))))
    );
    status.value = result;
    message.success(
      t('settings.workTimingBackfillSuccess', {
        attempted: result.attemptedSessionCount,
        calculated: result.calculatedSessionCount,
        remaining: result.remainingSessionCount,
      })
    );
  } catch (error) {
    console.error('Failed to calculate web session work timing:', error);
    message.error(t('settings.workTimingBackfillFailed'));
  } finally {
    running.value = false;
  }
}

onMounted(() => {
  void loadStatus();
});
</script>

<style scoped>
.work-timing-backfill-entry {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.work-timing-backfill-entry__copy {
  min-width: 0;
}

.work-timing-backfill-entry__title {
  margin-bottom: 4px;
  font-size: 14px;
  font-weight: 600;
}

.work-timing-backfill-entry__status {
  margin-top: 6px;
  color: var(--n-text-color-2);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.work-timing-backfill-entry__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  flex: 0 0 auto;
}

.work-timing-backfill-entry__batch {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--n-text-color-3);
  font-size: 12px;
  white-space: nowrap;
}

.work-timing-backfill-entry__actions :deep(.n-input-number) {
  width: 96px;
}

@media (max-width: 640px) {
  .work-timing-backfill-entry {
    align-items: flex-start;
    flex-direction: column;
  }

  .work-timing-backfill-entry__actions {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
