<template>
  <n-modal
    :show="show"
    preset="card"
    class="message-edit-modal"
    :title="t('webSession.editUserMessageTitle')"
    :bordered="false"
    :segmented="{ content: false, footer: false }"
    :mask-closable="!submitting"
    closable
    style="width: min(92vw, 620px)"
    @update:show="emit('update:show', $event)"
  >
    <div class="message-edit-modal-body">
      <div class="message-edit-hint">{{ t('webSession.editUserMessageHint') }}</div>
      <n-input
        :value="text"
        type="textarea"
        :autosize="{ minRows: 5, maxRows: 14 }"
        :placeholder="t('webSession.inputPlaceholder')"
        :disabled="submitting"
        autofocus
        @update:value="emit('update:text', $event)"
      />
      <div v-if="attachments.length > 0" class="message-edit-attachments">
        <div>
          {{
            t('webSession.editUserMessageAttachmentsPreserved', {
              count: attachments.length,
            })
          }}
        </div>
        <div class="attachment-row">
          <span v-for="attachment in attachments" :key="attachment.id" class="attachment-pill">
            {{ attachment.name }}
          </span>
        </div>
      </div>
      <n-alert type="warning" :show-icon="true" :bordered="false">
        {{ t('webSession.editUserMessageWorkspaceWarning') }}
      </n-alert>
    </div>
    <template #footer>
      <div class="message-edit-modal-footer">
        <n-button secondary :disabled="submitting" @click="emit('update:show', false)">
          {{ t('common.cancel') }}
        </n-button>
        <n-button
          type="primary"
          :loading="submitting"
          :disabled="!canSubmit"
          @click="emit('confirm')"
        >
          {{ t('webSession.editUserMessageSubmit') }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { useLocale } from '@/composables/useLocale';

type WebSessionMessageEditAttachment = {
  id: string;
  name: string;
};

defineProps<{
  show: boolean;
  text: string;
  submitting: boolean;
  canSubmit: boolean;
  attachments: WebSessionMessageEditAttachment[];
}>();

const emit = defineEmits<{
  (event: 'update:show', show: boolean): void;
  (event: 'update:text', text: string): void;
  (event: 'confirm'): void;
}>();

const { t } = useLocale();
</script>

<style scoped>
.message-edit-modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.message-edit-hint,
.message-edit-attachments {
  color: var(--n-text-color-3);
  font-size: 12px;
  line-height: 1.5;
}

.message-edit-attachments {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.attachment-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.attachment-pill {
  max-width: 100%;
  padding: 4px 8px;
  border-radius: 999px;
  background: var(--app-surface-sunken, #f1f3f5);
  color: var(--n-text-color-2);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.message-edit-modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
