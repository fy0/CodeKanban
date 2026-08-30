<template>
  <n-modal
    :show="show"
    preset="card"
    class="scheduled-send-modal"
    :title="title"
    :bordered="false"
    :segmented="{ content: false, footer: false }"
    :mask-closable="!submitting"
    closable
    style="width: min(92vw, 520px)"
    @update:show="emit('update:show', $event)"
  >
    <div class="scheduled-send-modal-body">
      <div v-if="purpose === 'edit_message'" class="scheduled-send-section">
        <div class="scheduled-send-section-label">
          {{ t('webSession.scheduledEditContent') }}
        </div>
        <n-input
          :value="editText"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 8 }"
          :placeholder="t('webSession.inputPlaceholder')"
          @update:value="emit('update:edit-text', $event)"
        />
        <div v-if="editingAttachmentCount > 0" class="scheduled-send-selected">
          {{
            t('webSession.scheduledAttachmentsPreserved', {
              count: editingAttachmentCount,
            })
          }}
        </div>
      </div>

      <div class="scheduled-send-section">
        <div class="scheduled-send-section-label">
          {{ t('webSession.scheduleKindTitle') }}
        </div>
        <n-radio-group
          :value="scheduleKind"
          name="scheduled-schedule-kind"
          @update:value="emit('update:schedule-kind', $event)"
        >
          <n-radio-button value="at_time">
            {{ t('webSession.scheduleKindAtTime') }}
          </n-radio-button>
          <n-radio-button value="when_idle">
            {{ t('webSession.scheduleKindWhenIdle') }}
          </n-radio-button>
        </n-radio-group>
        <p v-if="scheduleKind === 'when_idle'" class="scheduled-send-condition-description">
          {{ t('webSession.scheduleWhenIdleDescription') }}
        </p>
      </div>

      <div v-if="scheduleKind === 'at_time'" class="scheduled-send-section">
        <div class="scheduled-send-section-label">
          {{ t('webSession.scheduleSendPresetTitle') }}
        </div>
        <div class="scheduled-send-presets">
          <button
            v-for="preset in presets"
            :key="preset.key"
            type="button"
            class="scheduled-send-preset"
            :class="{ 'is-selected': selectedPresetKey === preset.key }"
            @click="emit('select-preset', preset.timestamp)"
          >
            {{ preset.label }}
          </button>
        </div>
      </div>

      <div v-if="scheduleKind === 'at_time'" class="scheduled-send-section">
        <div class="scheduled-send-section-label">
          {{ t('webSession.scheduleSendCustomTime') }}
        </div>
        <n-date-picker
          :value="sendAt"
          type="datetime"
          clearable
          style="width: 100%"
          @update:value="emit('update:send-at', $event)"
        />
        <div v-if="selectedTimeLabel" class="scheduled-send-selected">
          {{ selectedTimeLabel }}
        </div>
      </div>

      <div class="scheduled-send-section">
        <div class="scheduled-send-section-label">
          {{ t('webSession.scheduledDependencyTitle') }}
        </div>
        <n-select
          :value="dependencyId || null"
          :options="dependencyOptions"
          clearable
          filterable
          :placeholder="t('webSession.scheduledDependencyNone')"
          @update:value="emit('update:dependency-id', $event ?? '')"
        />
        <p class="scheduled-send-condition-description">
          {{ t('webSession.scheduledDependencyDescription') }}
        </p>
      </div>

      <div
        v-if="purpose === 'message' || purpose === 'edit_message'"
        class="scheduled-send-section"
      >
        <div class="scheduled-send-section-label">
          {{ t('webSession.scheduleSendModeTitle') }}
        </div>
        <n-radio-group
          :value="mode"
          name="scheduled-send-mode"
          @update:value="emit('update:mode', $event)"
        >
          <div class="scheduled-send-mode-grid">
            <label class="scheduled-send-mode-option">
              <n-radio value="send" />
              <span>{{ t('webSession.scheduledModeSend') }}</span>
            </label>
            <label class="scheduled-send-mode-option">
              <n-radio value="queue" />
              <span>{{ t('webSession.scheduledModeQueue') }}</span>
            </label>
            <label class="scheduled-send-mode-option">
              <n-radio value="interrupt" />
              <span>{{ t('webSession.scheduledModeInterrupt') }}</span>
            </label>
          </div>
        </n-radio-group>
        <n-checkbox
          :checked="exitPlanMode"
          class="scheduled-send-exit-plan"
          @update:checked="emit('update:exit-plan-mode', $event)"
        >
          {{ t('webSession.scheduleExitPlanMode') }}
        </n-checkbox>
      </div>
    </div>

    <template #footer>
      <div class="scheduled-send-footer">
        <n-button secondary :disabled="submitting" @click="emit('update:show', false)">
          {{ t('common.cancel') }}
        </n-button>
        <n-button
          type="primary"
          :loading="submitting"
          :disabled="!canConfirm"
          @click="emit('confirm')"
        >
          {{ confirmLabel }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { useLocale } from '@/composables/useLocale';

type ScheduledSendMode = 'send' | 'interrupt' | 'queue';
type ScheduledSendPurpose = 'message' | 'execute_plan' | 'edit_message' | 'edit_plan';
type ScheduledScheduleKind = 'at_time' | 'when_idle';
type ScheduledSendPreset = {
  key: string;
  label: string;
  timestamp: number;
};
type ScheduledDependencyOption = {
  label: string;
  value: string;
  disabled?: boolean;
};

defineProps<{
  show: boolean;
  purpose: ScheduledSendPurpose;
  title: string;
  confirmLabel: string;
  submitting: boolean;
  editText: string;
  editingAttachmentCount: number;
  scheduleKind: ScheduledScheduleKind;
  presets: ScheduledSendPreset[];
  selectedPresetKey: string;
  sendAt: number | null;
  selectedTimeLabel: string;
  dependencyId: string;
  dependencyOptions: ScheduledDependencyOption[];
  mode: ScheduledSendMode;
  exitPlanMode: boolean;
  canConfirm: boolean;
}>();

const emit = defineEmits<{
  (event: 'update:show', show: boolean): void;
  (event: 'update:edit-text', text: string): void;
  (event: 'update:schedule-kind', kind: ScheduledScheduleKind): void;
  (event: 'update:send-at', timestamp: number | null): void;
  (event: 'update:dependency-id', dependencyId: string): void;
  (event: 'update:mode', mode: ScheduledSendMode): void;
  (event: 'update:exit-plan-mode', exitPlanMode: boolean): void;
  (event: 'select-preset', timestamp: number): void;
  (event: 'confirm'): void;
}>();

const { t } = useLocale();
</script>

<style scoped>
.scheduled-send-modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.scheduled-send-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.scheduled-send-section-label {
  font-size: 12px;
  font-weight: 700;
  color: var(--app-text-color, var(--n-text-color-1, #111827));
}

.scheduled-send-condition-description {
  margin: 0;
  max-width: 100%;
  color: var(--n-text-color-2);
  font-size: 12px;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.scheduled-send-exit-plan {
  margin-top: 2px;
}

.scheduled-send-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.scheduled-send-preset {
  border: 1px solid color-mix(in srgb, var(--n-border-color) 82%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, var(--app-surface-color, #fff) 96%, transparent);
  color: inherit;
  padding: 6px 12px;
  font-size: 12px;
  line-height: 1.2;
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    background-color 0.2s ease,
    color 0.2s ease;
}

.scheduled-send-preset.is-selected {
  border-color: color-mix(in srgb, var(--n-primary-color) 52%, transparent);
  background: color-mix(in srgb, var(--n-primary-color) 12%, transparent);
  color: var(--n-primary-color);
}

.scheduled-send-selected {
  font-size: 12px;
  color: var(--n-text-color-2);
}

.scheduled-send-mode-grid {
  display: grid;
  gap: 8px;
}

.scheduled-send-mode-option {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 10px;
  border: 1px solid color-mix(in srgb, var(--n-border-color) 82%, transparent);
  background: color-mix(in srgb, var(--app-surface-color, #fff) 97%, transparent);
}

.scheduled-send-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
