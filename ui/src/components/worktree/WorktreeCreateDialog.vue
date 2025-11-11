<template>
  <n-modal
    v-model:show="visible"
    preset="dialog"
    title="创建 Worktree"
    positive-text="创建"
    negative-text="取消"
    :loading="loading"
    @positive-click="handleCreate"
  >
    <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top">
      <n-form-item label="分支名称" path="branchName">
        <n-input v-model:value="formData.branchName" placeholder="feature/new-feature" />
      </n-form-item>

      <n-form-item label="基础分支" path="baseBranch">
        <n-input v-model:value="formData.baseBranch" placeholder="main" />
      </n-form-item>
    </n-form>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage, type FormInst, type FormRules } from 'naive-ui';
import { useProjectStore } from '@/stores/project';
import type { Worktree } from '@/types/models';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
  success: [worktree: Worktree];
}>();

const projectStore = useProjectStore();
const message = useMessage();

const visible = computed({
  get: () => props.show,
  set: value => emit('update:show', value),
});

const formRef = ref<FormInst | null>(null);
const loading = ref(false);
const formData = ref({
  branchName: '',
  baseBranch: '',
  createBranch: true,
});

const rules: FormRules = {
  branchName: [{ required: true, message: '请输入分支名称', trigger: ['blur', 'input'] }],
};

watch(visible, newVal => {
  if (newVal) {
    formData.value.baseBranch =
      projectStore.currentProject?.defaultBranch ?? formData.value.baseBranch ?? 'main';
  } else {
    formData.value = { branchName: '', baseBranch: '', createBranch: true };
  }
});

async function handleCreate() {
  if (!projectStore.currentProject) {
    message.error('请先选择项目');
    return false;
  }

  try {
    await formRef.value?.validate();
    loading.value = true;
    const worktree = await projectStore.createWorktree(projectStore.currentProject.id, formData.value);
    visible.value = false;
    emit('success', worktree);
  } catch (error: any) {
    message.error(error?.message ?? '创建失败');
    return false;
  } finally {
    loading.value = false;
  }

  return true;
}
</script>
