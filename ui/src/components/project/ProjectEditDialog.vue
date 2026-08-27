<template>
  <n-modal
    v-model:show="visible"
    preset="dialog"
    :title="t('project.editProject')"
    :positive-text="t('common.save')"
    :negative-text="t('common.cancel')"
    :loading="loading"
    @positive-click="handleUpdate"
  >
    <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top">
      <n-form-item :label="t('project.projectName')" path="name">
        <n-input v-model:value="formData.name" :placeholder="t('project.namePlaceholder')" />
      </n-form-item>
      <n-form-item :label="t('project.projectDescription')" path="description">
        <n-input
          v-model:value="formData.description"
          type="textarea"
          :rows="3"
          :placeholder="t('project.descriptionPlaceholder')"
        />
      </n-form-item>
      <n-form-item :label="t('project.projectPath')">
        <n-input :value="props.project?.path ?? ''" disabled />
      </n-form-item>
      <n-form-item :label="t('project.hidePath')" path="hidePath">
        <n-space align="center">
          <n-switch v-model:value="formData.hidePath" />
          <n-text depth="3">{{ t('project.hidePathHint') }}</n-text>
        </n-space>
      </n-form-item>
      <n-divider />
      <div class="pi-access-section">
        <div class="pi-access-section__header">
          <span class="pi-access-section__label">{{ t('project.piAccess') }}</span>
          <n-tag
            v-if="piTrustStatus"
            size="small"
            :type="piTrustStatus.trusted ? 'success' : 'default'"
          >
            {{
              piTrustStatus.trusted ? t('project.piAccessTrusted') : t('project.piAccessNotTrusted')
            }}
          </n-tag>
        </div>
        <n-spin :show="piTrustLoading" size="small">
          <div class="pi-access-section__body">
            <n-text
              v-if="piTrustStatus?.trustedPath && !piTrustStatus.trusted"
              depth="3"
              class="pi-access-section__renewal"
            >
              {{ t('project.piAccessNeedsRenewal') }}
            </n-text>
            <n-button
              v-if="piTrustStatus?.trusted"
              attr-type="button"
              size="small"
              type="error"
              secondary
              :loading="piTrustMutating"
              @click="confirmRevokePiTrust"
            >
              {{ t('project.piTrustRevoke') }}
            </n-button>
            <n-button
              v-else-if="piTrustStatus"
              attr-type="button"
              size="small"
              type="warning"
              secondary
              :disabled="piTrustMutating"
              @click="showPiTrustDialog = true"
            >
              {{ t('project.piTrustConfirm') }}
            </n-button>
          </div>
        </n-spin>
      </div>
    </n-form>
  </n-modal>
  <PiProjectTrustDialog
    v-if="props.project"
    v-model:show="showPiTrustDialog"
    :project-id="props.project.id"
    :project-path="piTrustStatus?.projectPath || props.project.path"
    @trusted="handlePiTrusted"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useDialog, useMessage, type FormInst, type FormRules } from 'naive-ui';
import { projectApi } from '@/api/project';
import PiProjectTrustDialog from '@/components/project/PiProjectTrustDialog.vue';
import { useProjectStore } from '@/stores/project';
import type { Project, ProjectAgentTrustStatus } from '@/types/models';
import { useLocale } from '@/composables/useLocale';
import { getErrorMessage } from '@/utils/errorHandler';

const { t } = useLocale();

const props = defineProps<{
  show: boolean;
  project: Project | null;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
  success: [project: Project];
}>();

const projectStore = useProjectStore();
const dialog = useDialog();
const message = useMessage();

const visible = computed({
  get: () => props.show,
  set: value => emit('update:show', value),
});

const formRef = ref<FormInst | null>(null);
const loading = ref(false);
const piTrustLoading = ref(false);
const piTrustMutating = ref(false);
const piTrustStatus = ref<ProjectAgentTrustStatus | null>(null);
const showPiTrustDialog = ref(false);
const formData = ref({
  name: '',
  description: '',
  hidePath: false,
});

const rules: FormRules = {
  name: [
    { required: true, message: t('validation.projectNameRequired'), trigger: ['blur', 'input'] },
  ],
};

function syncFormWithProject(project: Project | null) {
  if (!project) {
    formData.value = { name: '', description: '', hidePath: false };
    return;
  }
  formData.value = {
    name: project.name,
    description: project.description ?? '',
    hidePath: project.hidePath ?? false,
  };
}

watch(
  () => props.project,
  project => {
    if (!visible.value) {
      return;
    }
    syncFormWithProject(project);
    piTrustStatus.value = null;
    void loadPiTrust(project?.id || '');
  },
  { immediate: true }
);

watch(visible, value => {
  if (value) {
    syncFormWithProject(props.project);
    piTrustStatus.value = null;
    void loadPiTrust(props.project?.id || '');
  } else {
    showPiTrustDialog.value = false;
  }
});

async function loadPiTrust(projectId: string) {
  if (!projectId) {
    piTrustStatus.value = null;
    piTrustLoading.value = false;
    return;
  }
  piTrustLoading.value = true;
  try {
    const status = await projectApi.getPiTrust(projectId);
    if (props.project?.id === projectId) {
      piTrustStatus.value = status;
    }
  } catch {
    if (props.project?.id === projectId) {
      piTrustStatus.value = null;
      message.error(t('project.piTrustLoadFailed'));
    }
  } finally {
    if (props.project?.id === projectId) {
      piTrustLoading.value = false;
    }
  }
}

function handlePiTrusted(status: ProjectAgentTrustStatus) {
  if (status.projectId === props.project?.id) {
    piTrustStatus.value = status;
  }
}

function confirmRevokePiTrust() {
  const projectId = props.project?.id || '';
  if (!projectId || piTrustMutating.value) {
    return;
  }
  dialog.warning({
    title: t('project.piTrustRevokeTitle'),
    content: t('project.piTrustRevokeConfirm'),
    positiveText: t('project.piTrustRevoke'),
    negativeText: t('common.cancel'),
    async onPositiveClick() {
      piTrustMutating.value = true;
      try {
        const status = await projectApi.revokePiTrust(projectId);
        if (props.project?.id === projectId) {
          piTrustStatus.value = status;
        }
        message.success(t('project.piTrustRevoked'));
      } catch {
        message.error(t('project.piTrustRevokeFailed'));
        return false;
      } finally {
        piTrustMutating.value = false;
      }
      return true;
    },
  });
}

async function handleUpdate() {
  if (!props.project) {
    message.warning(t('project.selectProjectToEdit'));
    return false;
  }
  try {
    await formRef.value?.validate();
    loading.value = true;
    const project = await projectStore.updateProject(props.project.id, {
      name: formData.value.name.trim(),
      description: formData.value.description.trim(),
      hidePath: formData.value.hidePath,
    });
    message.success(t('message.projectUpdated'));
    emit('success', project);
    visible.value = false;
  } catch (error) {
    const errorMessage = getErrorMessage(error);
    if (errorMessage) {
      message.error(errorMessage);
    }
    return false;
  } finally {
    loading.value = false;
  }
  return true;
}
</script>

<style scoped>
.pi-access-section {
  display: grid;
  gap: 10px;
}

.pi-access-section__header,
.pi-access-section__body {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.pi-access-section__label {
  font-weight: 500;
}

.pi-access-section__renewal {
  flex: 1;
  min-width: 0;
  line-height: 1.45;
}
</style>
