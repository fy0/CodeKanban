<template>
  <div class="context-window-setting">
    <div
      class="context-window-setting__row"
      :title="pending ? t('webSession.contextWindowPending') : undefined"
    >
      <span>{{ t('webSession.contextWindowSetting') }}</span>
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
      <div>{{ actualLabel }}</div>
      <div v-if="pending">{{ t('webSession.contextWindowPending') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { NSelect } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import { contextWindowOptions } from './webSessionContextWindow';

const props = defineProps<{
  value: number;
  disabled?: boolean;
  details?: boolean;
  pending?: boolean;
  actualTokens?: number | null;
  running?: boolean;
}>();
const emit = defineEmits<{ 'update:value': [value: number] }>();
const { t } = useI18n();
const options = computed(() => contextWindowOptions(t('webSession.contextWindowDefault')));
const selectedLabel = computed(
  () => options.value.find(option => option.value === props.value)?.label
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
.context-window-setting__details {
  margin-top: 6px;
  font-size: 11px;
  color: var(--text-color-3, #888);
  line-height: 1.6;
}
</style>
