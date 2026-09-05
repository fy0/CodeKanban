<template>
  <div class="context-window-setting">
    <div
      class="context-window-setting__row"
      :title="pending ? t('webSession.contextWindowPending') : undefined"
    >
      <span class="context-window-setting__label">
        {{ t('webSession.contextWindowSetting') }}
        <n-tooltip trigger="hover">
          <template #trigger>
            <button
              type="button"
              class="context-window-setting__help"
              :aria-label="t('webSession.contextWindowUpgradeHint')"
            >
              ?
            </button>
          </template>
          <div class="context-window-setting__help-text">
            {{ t('webSession.contextWindowReportedHint') }}
            {{ t('webSession.contextWindowUpgradeHint') }}
          </div>
        </n-tooltip>
      </span>
      <n-select
        :value="value"
        :options="options"
        :disabled="disabled"
        size="tiny"
        :consistent-menu-width="false"
        :to="details ? false : undefined"
        :aria-label="t('webSession.contextWindowSetting')"
        @update:value="emit('update:value', $event)"
      />
    </div>
    <div v-if="details" class="context-window-setting__details">
      <div>{{ t('webSession.contextWindowCurrentSetting') }}: {{ selectedLabel }}</div>
      <div v-if="requestedValue != null">
        {{ t('webSession.contextWindowRequested') }}: {{ requestedLabel }}
      </div>
      <div>{{ actualLabel }}</div>
      <div>{{ t('webSession.contextWindowReportedHint') }}</div>
      <div>{{ t('webSession.contextWindowUpgradeHint') }}</div>
      <div v-if="pending">{{ t('webSession.contextWindowPending') }}</div>
      <div v-if="metadataFallback" class="context-window-setting__warning">
        {{ t('webSession.contextWindowMetadataFallback') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { NSelect, NTooltip } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import { contextWindowOptions } from './webSessionContextWindow';

const props = defineProps<{
  value: number;
  disabled?: boolean;
  details?: boolean;
  pending?: boolean;
  actualTokens?: number | null;
  running?: boolean;
  requestedValue?: number | null;
  metadataFallback?: boolean;
}>();
const emit = defineEmits<{ 'update:value': [value: number] }>();
const { t } = useI18n();
const options = computed(() => contextWindowOptions(t('webSession.contextWindowDefault')));
const selectedLabel = computed(
  () => options.value.find(option => option.value === props.value)?.label
);
const requestedLabel = computed(
  () => options.value.find(option => option.value === props.requestedValue)?.label
);
const actualLabel = computed(() =>
  props.actualTokens
    ? `${t(props.running ? 'webSession.contextWindowActual' : 'webSession.contextWindowLastActual')}: ${props.actualTokens.toLocaleString()} tokens`
    : t('webSession.contextWindowUnconfirmed')
);
</script>

<style scoped>
.context-window-setting__row {
  display: flex;
  align-items: center;
  gap: 12px;
  white-space: nowrap;
}
.context-window-setting__row :deep(.n-select) {
  width: 90px;
  margin-left: auto;
}
.context-window-setting__label {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.context-window-setting__help {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  padding: 0;
  border: 1px solid currentColor;
  border-radius: 50%;
  background: transparent;
  color: var(--text-color-3, #888);
  font: inherit;
  font-size: 10px;
  line-height: 1;
  cursor: help;
}
.context-window-setting__help-text {
  max-width: 260px;
  white-space: normal;
}
.context-window-setting__details {
  margin-top: 6px;
  font-size: 11px;
  color: var(--text-color-3, #888);
  line-height: 1.6;
  max-width: 260px;
}
.context-window-setting__warning {
  color: var(--n-warning-color, #b7791f);
}
</style>
