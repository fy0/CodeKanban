<template>
  <n-modal
    :show="show"
    preset="card"
    class="local-file-modal"
    :title="t('webSession.localFileTitle')"
    :bordered="false"
    :segmented="{ content: false, footer: false }"
    :mask-closable="!action"
    :closable="!action"
    style="width: min(92vw, 560px)"
    @update:show="emit('update:show', $event)"
  >
    <div v-if="target" class="local-file-modal-body">
      <div class="local-file-name">{{ target.name }}</div>
      <code class="local-file-path">{{ target.path }}</code>
    </div>
    <template #footer>
      <div class="local-file-modal-footer">
        <n-button secondary :disabled="Boolean(action)" @click="emit('update:show', false)">
          {{ t('common.close') }}
        </n-button>
        <n-button
          secondary
          :loading="action === 'open-location'"
          :disabled="Boolean(action)"
          @click="emit('open-location')"
        >
          <template #icon>
            <n-icon><FolderOpenOutline /></n-icon>
          </template>
          {{ t('webSession.localFileOpenLocation') }}
        </n-button>
        <n-button
          secondary
          :loading="action === 'open-file-view'"
          :disabled="Boolean(action)"
          @click="emit('open-file-view')"
        >
          <template #icon>
            <n-icon><DocumentTextOutline /></n-icon>
          </template>
          {{ t('webSession.localFileOpenInFileView') }}
        </n-button>
        <n-button
          type="primary"
          :loading="action === 'download'"
          :disabled="Boolean(action)"
          @click="emit('download')"
        >
          <template #icon>
            <n-icon><DownloadOutline /></n-icon>
          </template>
          {{ t('webSession.localFileDownload') }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { DocumentTextOutline, DownloadOutline, FolderOpenOutline } from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';

type WebSessionLocalFileDialogTarget = {
  name: string;
  path: string;
};

type WebSessionLocalFileAction = '' | 'download' | 'open-file-view' | 'open-location';

defineProps<{
  show: boolean;
  target: WebSessionLocalFileDialogTarget | null;
  action: WebSessionLocalFileAction;
}>();

const emit = defineEmits<{
  (event: 'update:show', show: boolean): void;
  (event: 'open-file-view'): void;
  (event: 'open-location'): void;
  (event: 'download'): void;
}>();

const { t } = useLocale();
</script>

<style scoped>
.local-file-modal-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.local-file-name {
  min-width: 0;
  color: var(--n-text-color-1);
  font-size: 14px;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.local-file-path {
  display: block;
  max-height: 132px;
  overflow: auto;
  padding: 10px 12px;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
  background: color-mix(in srgb, var(--app-surface-color, #fff) 92%, var(--n-border-color));
  color: var(--n-text-color-2);
  font-size: 12px;
  line-height: 1.5;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.local-file-modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: nowrap;
}

.local-file-modal-footer :deep(.n-button) {
  min-width: 92px;
}

@media (max-width: 480px) {
  .local-file-modal-footer {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .local-file-modal-footer :deep(.n-button) {
    width: 100%;
    min-width: 0;
  }

  .local-file-modal-footer :deep(.n-button:first-child) {
    grid-column: 1 / -1;
  }

  .local-file-modal-footer :deep(.n-button:last-child) {
    grid-column: 1 / -1;
  }
}
</style>
