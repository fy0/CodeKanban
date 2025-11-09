<template>
  <div class="worktree-list">
    <div class="list-header">
      <n-space justify="space-between" align="center">
        <h3>Worktrees</h3>
        <n-space>
          <n-button
            text
            @click="handleRefreshAll"
            :loading="refreshing"
            :disabled="!projectStore.currentProject || !gitFeaturesAvailable"
          >
            <template #icon>
              <n-icon><RefreshOutline /></n-icon>
            </template>
          </n-button>
          <n-button
            type="primary"
            size="small"
            @click="showCreateDialog = true"
            :disabled="!projectStore.currentProject || !gitFeaturesAvailable"
          >
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            新建
          </n-button>
        </n-space>
      </n-space>
    </div>

    <n-alert
      v-if="showGitWarning"
      type="warning"
      class="git-warning"
      title="Worktree 功能不可用"
      :show-icon="false"
    >
      当前项目目录不是 Git 仓库，无法创建或刷新 Worktree；仍可使用任务等其他功能。
    </n-alert>

    <n-scrollbar style="max-height: calc(100vh - 80px)">
      <div v-if="projectStore.worktrees.length" class="worktree-items">
        <WorktreeCard
          v-for="worktree in projectStore.worktrees"
          :key="worktree.id"
          :worktree="worktree"
          :selected="projectStore.selectedWorktreeId === worktree.id"
          @select="handleSelectWorktree"
          @refresh="handleRefresh"
          @delete="confirmDelete"
          @open-explorer="handleOpenExplorer"
          @open-terminal="handleOpenTerminal"
        />
      </div>
      <n-empty v-else description="暂无 Worktree" class="worktree-empty" />
    </n-scrollbar>

    <WorktreeCreateDialog
      v-if="projectStore.currentProject"
      v-model:show="showCreateDialog"
      @success="handleWorktreeCreated"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import { AddOutline, RefreshOutline } from '@vicons/ionicons5';
import WorktreeCard from './WorktreeCard.vue';
import WorktreeCreateDialog from './WorktreeCreateDialog.vue';
import { useProjectStore } from '@/stores/project';
import type { Worktree } from '@/types/models';

const emit = defineEmits<{
  'open-terminal': [payload: Worktree];
}>();

const projectStore = useProjectStore();
const message = useMessage();
const dialog = useDialog();

const showCreateDialog = ref(false);
const refreshing = ref(false);

const hasMainWorktree = computed(() => projectStore.worktrees.some(worktree => worktree.isMain));
const gitFeaturesAvailable = computed(() => {
  if (!projectStore.currentProject) {
    return false;
  }
  if (projectStore.loading) {
    return true;
  }
  return hasMainWorktree.value;
});
const showGitWarning = computed(
  () => Boolean(projectStore.currentProject) && !projectStore.loading && !hasMainWorktree.value
);

watch(gitFeaturesAvailable, enabled => {
  if (!enabled) {
    showCreateDialog.value = false;
  }
});

async function handleRefreshAll() {
  if (!projectStore.currentProject) {
    return;
  }
  if (!gitFeaturesAvailable.value) {
    message.warning('当前项目不是 Git 仓库，无法刷新 Worktree 状态');
    return;
  }
  refreshing.value = true;
  try {
    await projectStore.refreshAllWorktrees(projectStore.currentProject.id);
    message.success('已刷新所有 Worktree 状态');
  } catch (error: any) {
    message.error(error?.message ?? '刷新失败');
  } finally {
    refreshing.value = false;
  }
}

async function handleRefresh(id: string) {
  try {
    await projectStore.refreshWorktreeStatus(id);
    message.success('状态已刷新');
  } catch (error: any) {
    message.error(error?.message ?? '刷新失败');
  }
}

function confirmDelete(worktree: Worktree) {
  dialog.warning({
    title: '确认删除',
    content: `确定要删除 worktree "${worktree.branchName}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await projectStore.deleteWorktree(worktree.id);
        message.success('Worktree 已删除');
      } catch (error: any) {
        message.error(error?.message ?? '删除失败');
      }
    },
  });
}

async function handleOpenExplorer(path: string) {
  try {
    await projectStore.openInExplorer(path);
  } catch (error: any) {
    message.error(error?.message ?? '打开文件管理器失败');
  }
}

function handleOpenTerminal(worktree: Worktree) {
  emit('open-terminal', worktree);
}

async function handleWorktreeCreated(worktree?: Worktree) {
  if (!projectStore.currentProject || !gitFeaturesAvailable.value) {
    return;
  }
  await projectStore.fetchWorktrees(projectStore.currentProject.id);
  if (worktree) {
    message.success(`Worktree ${worktree.branchName} 创建成功`);
  }
}

function handleSelectWorktree(worktreeId: string) {
  if (projectStore.selectedWorktreeId === worktreeId) {
    projectStore.setSelectedWorktree(null);
  } else {
    projectStore.setSelectedWorktree(worktreeId);
  }
}
</script>

<style scoped>
.worktree-list {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.list-header {
  padding: 16px;
  border-bottom: 1px solid var(--n-border-color);
}

.worktree-items {
  padding: 8px;
}

h3 {
  margin: 0;
}

.git-warning {
  margin: 8px 16px;
}

.worktree-empty {
  margin: 16px;
}
</style>
