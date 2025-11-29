<template>
  <n-modal
    preset="card"
    class="task-create-dialog"
    :title="t('task.newTask')"
    :show="show"
    @update:show="emit('update:show', $event as boolean)"
    :style="dialogStyle"
    :card-style="dialogCardStyle"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-width="80">
      <n-form-item :label="t('task.fieldTitle')" path="title">
        <n-input v-model:value="form.title" :placeholder="t('task.titlePlaceholder')" />
      </n-form-item>

      <n-form-item :label="t('task.fieldDescription')">
        <n-input v-model:value="form.description" type="textarea" rows="4" :placeholder="t('task.descriptionPlaceholder')" />
      </n-form-item>

      <n-form-item :label="t('task.fieldPriority')">
        <n-select v-model:value="form.priority" :options="priorityOptions" />
      </n-form-item>

      <n-form-item :label="t('task.relatedBranch')">
        <n-select
          v-model:value="form.worktreeId"
          :options="worktreeOptions"
          :placeholder="t('task.optional')"
          clearable
        />
      </n-form-item>

      <n-form-item :label="t('task.dueDate')">
        <n-date-picker v-model:formatted-value="form.dueDate" value-format="yyyy-MM-dd" type="date" clearable />
      </n-form-item>

      <n-form-item :label="t('task.tags')">
        <n-dynamic-tags v-model:value="form.tags" />
      </n-form-item>
    </n-form>

    <template #footer>
      <n-space justify="end">
        <n-button @click="emit('update:show', false)">{{ t('common.cancel') }}</n-button>
        <n-button type="primary" :loading="createLoading" @click="handleSubmit">{{ t('common.create') }}</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, watch, type CSSProperties } from 'vue';
import { useMessage, type FormInst, type FormRules } from 'naive-ui';
import { useProjectStore } from '@/stores/project';
import { taskActions } from '@/composables/useTaskActions';
import { extractItem } from '@/api/response';
import type { Task } from '@/types/models';
import { useLocale } from '@/composables/useLocale';

const { t } = useLocale();

const props = defineProps<{
  show: boolean;
  projectId: string;
  defaultStatus?: Task['status'];
}>();

const emit = defineEmits<{
  'update:show': [boolean];
  created: [Task];
}>();

const projectStore = useProjectStore();
const { createTask } = taskActions;
const message = useMessage();
const resolvedStatus = computed<Task['status']>(() => props.defaultStatus ?? 'todo');

const formRef = ref<FormInst | null>(null);
const form = ref({
  title: '',
  description: '',
  priority: 0,
  worktreeId: null as string | null,
  dueDate: null as string | null,
  tags: [] as string[],
});

const rules: FormRules = {
  title: [{ required: true, message: t('validation.taskTitleRequired'), trigger: 'blur' }],
};

const priorityOptions = [
  { label: t('task.priority.normal'), value: 0 },
  { label: t('task.priority.low'), value: 1 },
  { label: t('task.priority.medium'), value: 2 },
  { label: t('task.priority.high'), value: 3 },
];

const worktreeOptions = computed(() =>
  (projectStore.worktrees ?? []).map(worktree => ({
    label: worktree.branchName,
    value: worktree.id,
  })),
);

const createLoading = createTask.loading;

watch(
  () => props.show,
  value => {
    if (!value) {
      resetForm();
    }
  },
);

function resetForm() {
  form.value = {
    title: '',
    description: '',
    priority: 0,
    worktreeId: null,
    dueDate: null,
    tags: [],
  };
}

function validate() {
  return formRef.value?.validate();
}

async function handleSubmit() {
  try {
    await validate();
  } catch {
    return;
  }

  if (!props.projectId) {
    message.error(t('task.missingProjectId'));
    return;
  }

  try {
    const response = await createTask.send(props.projectId, {
      title: form.value.title,
      description: form.value.description,
      status: resolvedStatus.value,
      priority: form.value.priority,
      worktreeId: form.value.worktreeId,
      dueDate: form.value.dueDate,
      tags: form.value.tags,
    });
    const task = extractItem(response) as unknown as Task | undefined;
    if (task) {
      emit('created', task);
      message.success(t('message.taskCreated'));
      emit('update:show', false);
      resetForm();
    }
  } catch (error: any) {
    message.error(error?.message ?? t('message.taskCreateFailed'));
  }
}

const dialogStyle: CSSProperties = {
  width: 'min(90vw, 800px)',
  maxWidth: '800px',
};

const dialogCardStyle: CSSProperties = {
  backgroundColor: 'transparent',
  boxShadow: 'none',
};
</script>
