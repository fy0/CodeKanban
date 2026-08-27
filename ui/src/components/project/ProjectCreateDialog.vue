<template>
  <n-modal
    v-model:show="visible"
    preset="dialog"
    :title="t('project.createProject')"
    :positive-text="t('common.create')"
    :negative-text="t('common.cancel')"
    :loading="loading"
    @positive-click="handleCreate"
  >
    <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top">
      <n-form-item ref="nameFormItemRef" :label="t('project.projectName')" path="name">
        <n-input v-model:value="formData.name" :placeholder="t('project.namePlaceholder')" />
      </n-form-item>
      <n-form-item ref="pathFormItemRef" :label="t('project.projectDirectory')" path="path">
        <n-input-group>
          <n-input
            v-model:value="formData.path"
            :placeholder="t('project.pathPlaceholder')"
            @blur="handlePathBlur"
          />
          <n-button @click="handleOpenDirectoryPicker">
            <template #icon>
              <n-icon><FolderOpenOutline /></n-icon>
            </template>
          </n-button>
        </n-input-group>
        <template #feedback>
          <n-text depth="3">
            {{ t('project.pathHint') }}
          </n-text>
        </template>
      </n-form-item>
      <n-form-item :label="t('project.projectDescription')" path="description">
        <n-input
          v-model:value="formData.description"
          type="textarea"
          :rows="3"
          :placeholder="t('project.descriptionPlaceholder')"
        />
      </n-form-item>
      <n-form-item :label="t('project.hidePath')" path="hidePath">
        <n-space align="center">
          <n-switch v-model:value="formData.hidePath" />
          <n-text depth="3">{{ t('project.hidePathHint') }}</n-text>
        </n-space>
      </n-form-item>
    </n-form>
  </n-modal>

  <DirectoryPickerDialog
    v-model:show="showDirectoryPicker"
    :initial-path="pickerInitialPath"
    @confirm="handleDirectorySelected"
  />
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useMessage, type FormInst, type FormItemInst, type FormRules } from 'naive-ui';
import { FolderOpenOutline } from '@vicons/ionicons5';
import { useProjectStore } from '@/stores/project';
import type { Project } from '@/types/models';
import { useLocale } from '@/composables/useLocale';
import { http } from '@/api/http';
import DirectoryPickerDialog from '@/components/common/DirectoryPickerDialog.vue';
import { getErrorMessage } from '@/utils/errorHandler';

const { t } = useLocale();

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
  success: [project: Project];
}>();

const projectStore = useProjectStore();
const message = useMessage();

const visible = computed({
  get: () => props.show,
  set: value => emit('update:show', value),
});

const formRef = ref<FormInst | null>(null);
const nameFormItemRef = ref<FormItemInst | null>(null);
const pathFormItemRef = ref<FormItemInst | null>(null);
const loading = ref(false);
const showDirectoryPicker = ref(false);
const homeDir = ref('');
const lastAutoFilledName = ref('');
const formData = ref({
  name: '',
  path: '',
  description: '',
  hidePath: false,
});

// 目录选择器的初始路径：优先用已填写的路径，否则用 HOME 目录
const pickerInitialPath = computed(() => formData.value.path || homeDir.value);

function fillPathWithHomeIfEmpty() {
  if (!formData.value.path.trim() && homeDir.value) {
    formData.value.path = homeDir.value;
  }
}

function handleDirectorySelected(path: string) {
  formData.value.path = path;
  syncProjectNameFromPath(path);
  void syncProgrammaticValidation();
}

function handlePathBlur() {
  formData.value.path = formData.value.path.trim();
  syncProjectNameFromPath(formData.value.path);
  void syncProgrammaticValidation();
}

function extractDirectoryName(path: string) {
  const trimmedPath = path.trim();
  if (!trimmedPath) {
    return '';
  }

  const normalizedPath = trimmedPath.replace(/[\\/]+$/, '');
  if (!normalizedPath || /^[A-Za-z]:$/.test(normalizedPath)) {
    return '';
  }

  const segments = normalizedPath.split(/[\\/]/).filter(Boolean);
  return segments[segments.length - 1] ?? '';
}

function syncProjectNameFromPath(path: string) {
  const directoryName = extractDirectoryName(path);
  if (!directoryName) {
    return;
  }

  const currentName = formData.value.name.trim();
  if (!currentName || currentName === lastAutoFilledName.value) {
    formData.value.name = directoryName;
    lastAutoFilledName.value = directoryName;
  }
}

async function refreshFormItemValidation(field: 'name' | 'path') {
  await nextTick();

  const formItemRef = field === 'name' ? nameFormItemRef.value : pathFormItemRef.value;
  if (!formItemRef) {
    return;
  }

  formItemRef.restoreValidation();

  if (!formData.value[field].trim()) {
    return;
  }

  try {
    await formItemRef.validate({ trigger: 'input' });
  } catch {
    // Keep Naive UI's own error state when the field still fails validation.
  }
}

async function syncProgrammaticValidation() {
  await Promise.all([refreshFormItemValidation('path'), refreshFormItemValidation('name')]);
}

async function fetchHomeDir() {
  if (homeDir.value) {
    fillPathWithHomeIfEmpty();
    return homeDir.value;
  }

  try {
    const res = await http.Get<{ item?: { path: string } }>('/fs/home').send();
    if (res?.item?.path) {
      homeDir.value = res.item.path;
      fillPathWithHomeIfEmpty();
    }
  } catch (e) {
    console.error('Failed to get home directory:', e);
  }

  return homeDir.value;
}

async function handleOpenDirectoryPicker() {
  await fetchHomeDir();
  fillPathWithHomeIfEmpty();
  showDirectoryPicker.value = true;
}

// 获取 HOME 目录
void fetchHomeDir();

const rules: FormRules = {
  name: [
    { required: true, message: t('validation.projectNameRequired'), trigger: ['blur', 'input'] },
  ],
  path: [
    { required: true, message: t('validation.projectPathRequired'), trigger: ['blur', 'input'] },
  ],
};

watch(visible, newVal => {
  if (newVal) {
    fillPathWithHomeIfEmpty();
    void fetchHomeDir();
    void nextTick().then(() => formRef.value?.restoreValidation());
  } else {
    lastAutoFilledName.value = '';
    formData.value = { name: '', path: '', description: '', hidePath: false };
    void nextTick().then(() => formRef.value?.restoreValidation());
  }
});

async function handleCreate() {
  try {
    await formRef.value?.validate();
    loading.value = true;
    const project = await projectStore.createProject(formData.value);
    message.success(t('message.projectCreated'));
    visible.value = false;
    emit('success', project);
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
