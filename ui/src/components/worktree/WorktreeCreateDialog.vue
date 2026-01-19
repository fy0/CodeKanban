<template>
  <n-modal
    v-model:show="visible"
    preset="dialog"
    :title="t('worktree.create')"
    :positive-text="t('common.create')"
    :negative-text="t('common.cancel')"
    :loading="loading"
    @positive-click="handleCreate"
  >
    <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top">
      <n-form-item :label="t('worktree.createLocation')" path="location">
        <n-radio-group v-model:value="formData.location">
          <n-space>
            <n-radio value="project">{{ t('worktree.locationProject') }}</n-radio>
            <n-radio value="global">{{ t('worktree.locationGlobal') }}</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>

      <n-form-item
        v-if="formData.location === 'global'"
        :label="t('worktree.globalBaseDirOverride')"
        path="globalBaseDirOverride"
      >
        <n-input
          v-model:value="formData.globalBaseDirOverride"
          :placeholder="t('worktree.globalBaseDirOverridePlaceholder')"
        />
      </n-form-item>
      <n-form-item :label="t('branch.branchName')" path="branchName">
        <n-input v-model:value="formData.branchName" :placeholder="t('branch.branchNamePlaceholder')" />
      </n-form-item>

      <n-form-item :label="t('branch.baseBranch')" path="baseBranch">
        <n-input v-model:value="formData.baseBranch" :placeholder="t('branch.baseBranchPlaceholder')" />
      </n-form-item>
    </n-form>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage, type FormInst, type FormRules } from 'naive-ui';
import { useProjectStore } from '@/stores/project';
import type { Worktree } from '@/types/models';
import { useLocale } from '@/composables/useLocale';

const { t } = useLocale();

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
  location: 'project' as 'project' | 'global',
  globalBaseDirOverride: '',
  branchName: '',
  baseBranch: '',
  createBranch: true,
});

const rules: FormRules = {
  branchName: [{ required: true, message: t('validation.branchNameRequired'), trigger: ['blur', 'input'] }],
};

/**
 * 判断路径是否看起来像绝对路径（跨平台）
 */
function looksLikeAbsPath(path: string) {
  const trimmed = path.trim();
  // Unix 风格：以 / 开头
  if (trimmed.startsWith('/')) {
    return true;
  }
  // Windows 风格：盘符 + 冒号 + 斜杠（如 C:\ 或 C:/）
  return /^[a-zA-Z]:[\\/]/.test(trimmed);
}

/**
 * 规范化路径：统一使用正斜杠，移除多余斜杠和尾部斜杠
 */
function normalizePath(path: string) {
  return path.replace(/\\/g, '/').replace(/\/+/g, '/').replace(/\/$/, '');
}

/**
 * 判断 worktree 基础路径是否为全局路径（不在项目目录内）
 */
function isGlobalWorktreeBasePath(projectPath: string, worktreeBasePath: string) {
  if (!looksLikeAbsPath(worktreeBasePath)) {
    return false;
  }
  const projectNorm = normalizePath(projectPath);
  const baseNorm = normalizePath(worktreeBasePath);
  return !baseNorm.startsWith(projectNorm + '/');
}

watch(visible, newVal => {
  if (newVal) {
    // 根据项目的 worktree 基础路径判断默认位置
    const current = projectStore.currentProject;
    if (current?.path && current.worktreeBasePath) {
      formData.value.location = isGlobalWorktreeBasePath(current.path, current.worktreeBasePath)
        ? 'global'
        : 'project';
    } else {
      formData.value.location = 'project';
    }

    // 使用项目默认分支作为基础分支
    formData.value.baseBranch =
      projectStore.currentProject?.defaultBranch ?? formData.value.baseBranch ?? 'main';
  } else {
    // 对话框关闭时重置表单
    formData.value = {
      location: 'project',
      globalBaseDirOverride: '',
      branchName: '',
      baseBranch: '',
      createBranch: true,
    };
  }
});

async function handleCreate() {
  if (!projectStore.currentProject) {
    message.error(t('project.selectProjectFirst'));
    return false;
  }

  try {
    await formRef.value?.validate();
    loading.value = true;
    const worktree = await projectStore.createWorktree(projectStore.currentProject.id, formData.value);
    // 先 emit success 事件，确保父组件能接收到
    emit('success', worktree);
    // 返回 true 让 Naive UI 自动关闭对话框
    return true;
  } catch (error: any) {
    message.error(error?.message ?? t('worktree.createFailed'));
    return false;
  } finally {
    loading.value = false;
  }
}
</script>
