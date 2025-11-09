<template>
  <n-modal
    preset="card"
    class="task-create-dialog"
    title="新建任务"
    :show="show"
    @update:show="emit('update:show', $event as boolean)"
    :style="dialogStyle"
    :card-style="dialogCardStyle"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-width="80">
      <n-form-item label="标题" path="title">
        <n-input v-model:value="form.title" placeholder="请输入任务标题" />
      </n-form-item>

      <n-form-item label="描述">
        <n-input v-model:value="form.description" type="textarea" rows="4" placeholder="任务描述" />
      </n-form-item>

      <n-form-item label="优先级">
        <n-select v-model:value="form.priority" :options="priorityOptions" />
      </n-form-item>

      <n-form-item label="关联分支">
        <n-select
          v-model:value="form.worktreeId"
          :options="worktreeOptions"
          placeholder="可选"
          clearable
        />
      </n-form-item>

      <n-form-item label="截止日期">
        <n-date-picker v-model:formatted-value="form.dueDate" value-format="yyyy-MM-dd" type="date" clearable />
      </n-form-item>

      <n-form-item label="标签">
        <n-dynamic-tags v-model:value="form.tags" />
      </n-form-item>
    </n-form>

    <template #footer>
      <n-space justify="end">
        <n-button @click="emit('update:show', false)">取消</n-button>
        <n-button type="primary" :loading="createLoading" @click="handleSubmit">创建</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, watch, type CSSProperties } from 'vue';
import { useMessage, type FormInst, type FormRules } from 'naive-ui';
import { useProjectStore } from '@/stores/project';
import { useTaskActions } from '@/composables/useTaskActions';
import { extractItem } from '@/api/response';
import type { Task } from '@/types/models';

const props = defineProps<{
  show: boolean;
  projectId: string;
}>();

const emit = defineEmits<{
  'update:show': [boolean];
  created: [Task];
}>();

const projectStore = useProjectStore();
const { createTask } = useTaskActions();
const message = useMessage();

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
  title: [{ required: true, message: '请输入任务标题', trigger: 'blur' }],
};

const priorityOptions = [
  { label: '普通', value: 0 },
  { label: '低', value: 1 },
  { label: '中', value: 2 },
  { label: '高', value: 3 },
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
    message.error('缺少项目 ID');
    return;
  }

  try {
    const response = await createTask.send(props.projectId, {
      title: form.value.title,
      description: form.value.description,
      priority: form.value.priority,
      worktreeId: form.value.worktreeId,
      dueDate: form.value.dueDate,
      tags: form.value.tags,
    });
    const task = extractItem(response) as unknown as Task | undefined;
    if (task) {
      emit('created', task);
      message.success('任务创建成功');
      emit('update:show', false);
      resetForm();
    }
  } catch (error: any) {
    message.error(error?.message ?? '创建任务失败');
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
