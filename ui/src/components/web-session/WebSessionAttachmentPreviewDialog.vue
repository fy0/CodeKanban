<template>
  <n-modal
    :show="show"
    preset="card"
    class="attachment-preview-modal"
    :title="preview?.name"
    :bordered="false"
    :segmented="{ content: false, footer: false }"
    :mask-closable="true"
    closable
    style="width: min(92vw, 960px)"
    @update:show="emit('update:show', $event)"
  >
    <div v-if="preview" class="attachment-preview-modal-body">
      <img :src="preview.url" :alt="preview.name" class="attachment-preview-modal-image" />
    </div>
  </n-modal>
</template>

<script setup lang="ts">
type WebSessionAttachmentPreview = {
  name: string;
  url: string;
};

defineProps<{
  show: boolean;
  preview: WebSessionAttachmentPreview | null;
}>();

const emit = defineEmits<{
  (event: 'update:show', show: boolean): void;
}>();
</script>

<style scoped>
.attachment-preview-modal-body {
  display: flex;
  align-items: center;
  justify-content: center;
  max-height: calc(88vh - 96px);
}

.attachment-preview-modal-image {
  display: block;
  max-width: 100%;
  max-height: calc(88vh - 96px);
  border-radius: 12px;
  object-fit: contain;
}
</style>
