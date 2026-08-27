<template>
  <div
    class="transfer-progress-dialog"
    :class="{ 'is-error': tone === 'error' }"
    :style="cardStyle"
  >
    <span class="transfer-progress-message">{{ message }}</span>
    <span v-if="detail" class="transfer-progress-detail">{{ detail }}</span>
    <div v-if="normalizedProgress !== null" class="transfer-progress-track">
      <div class="transfer-progress-fill" :style="progressStyle"></div>
    </div>
    <span v-if="normalizedProgress !== null" class="transfer-progress-percent">
      {{ normalizedProgress }}%
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from 'vue';

const props = withDefaults(
  defineProps<{
    message: string;
    detail?: string;
    progress?: number | null;
    tone?: 'progress' | 'error';
    cardStyle?: CSSProperties;
  }>(),
  {
    detail: '',
    progress: null,
    tone: 'progress',
    cardStyle: undefined,
  }
);

const normalizedProgress = computed(() => {
  if (typeof props.progress !== 'number' || !Number.isFinite(props.progress)) {
    return props.progress === null ? null : null;
  }
  return Math.max(0, Math.min(100, Math.round(props.progress)));
});

const progressStyle = computed(() => ({
  width: `${normalizedProgress.value ?? 0}%`,
}));
</script>

<style scoped>
.transfer-progress-dialog {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  z-index: 12;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  min-width: 220px;
  max-width: min(320px, calc(100% - 32px));
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid var(--terminal-transfer-card-border, rgba(255, 255, 255, 0.14));
  background: var(--terminal-transfer-card-bg, rgba(15, 17, 26, 0.92));
  color: var(--terminal-transfer-card-fg, var(--kanban-terminal-fg, #f6f8ff));
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.3);
  backdrop-filter: blur(10px);
  pointer-events: none;
  text-align: center;
}

.transfer-progress-dialog.is-error {
  border-color: rgba(255, 117, 117, 0.35);
}

.transfer-progress-message {
  font-size: 13px;
  line-height: 1.4;
}

.transfer-progress-detail {
  max-width: 100%;
  margin-top: -2px;
  font-size: 12px;
  line-height: 1.35;
  opacity: 0.78;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.transfer-progress-track {
  width: 100%;
  height: 6px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--terminal-transfer-card-track, rgba(255, 255, 255, 0.12));
}

.transfer-progress-fill {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, rgba(112, 211, 255, 0.95), rgba(116, 170, 156, 0.95));
  transition: width 120ms ease-out;
}

.transfer-progress-dialog.is-error .transfer-progress-fill {
  background: linear-gradient(90deg, rgba(255, 131, 131, 0.95), rgba(255, 180, 117, 0.95));
}

.transfer-progress-percent {
  font-size: 12px;
  opacity: 0.8;
}
</style>
