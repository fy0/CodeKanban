<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    :title="t('project.piTrustTitle')"
    class="pi-project-trust-dialog"
    style="width: min(92vw, 520px)"
    :mask-closable="!loading"
    :closable="!loading"
  >
    <n-alert type="warning" :show-icon="true" :bordered="false">
      {{ t('project.piTrustWarning') }}
    </n-alert>
    <div class="pi-project-trust-dialog__details">
      <p>{{ t('project.piTrustResources') }}</p>
      <p>{{ t('project.piTrustScope') }}</p>
      <code class="pi-project-trust-dialog__path">{{ projectPath }}</code>
    </div>
    <template #footer>
      <n-space justify="end">
        <n-button :disabled="loading" @click="visible = false">
          {{ t('common.cancel') }}
        </n-button>
        <n-button type="warning" :loading="loading" @click="confirmTrust">
          {{ t('project.piTrustConfirm') }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useMessage } from 'naive-ui';
import { projectApi } from '@/api/project';
import { useLocale } from '@/composables/useLocale';
import type { ProjectAgentTrustStatus } from '@/types/models';

const props = defineProps<{
  show: boolean;
  projectId: string;
  projectPath: string;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
  trusted: [status: ProjectAgentTrustStatus];
}>();

const { t } = useLocale();
const message = useMessage();
const loading = ref(false);
const visible = computed({
  get: () => props.show,
  set: value => emit('update:show', value),
});

async function confirmTrust() {
  if (!props.projectId || loading.value) {
    return;
  }
  loading.value = true;
  try {
    const status = await projectApi.trustForPi(props.projectId);
    emit('trusted', status);
    visible.value = false;
    message.success(t('project.piTrustGranted'));
  } catch (error) {
    const text = error instanceof Error ? error.message : t('project.piTrustFailed');
    message.error(text);
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.pi-project-trust-dialog__details {
  display: grid;
  gap: 10px;
  margin-top: 16px;
  color: var(--n-text-color-2);
  line-height: 1.55;
}

.pi-project-trust-dialog__details p {
  margin: 0;
}

.pi-project-trust-dialog__path {
  display: block;
  overflow-wrap: anywhere;
  padding: 8px 10px;
  border: 1px solid var(--n-border-color);
  border-radius: 4px;
  color: var(--n-text-color-1);
  background: var(--n-color-embedded);
}
</style>
