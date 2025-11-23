<template>
  <div class="kanban-board">
    <div class="board-header">
      <n-space justify="space-between" align="center">
        <div>
          <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 4px;">
            <h2 style="margin: 0;">{{ t('task.kanbanTitle') }}</h2>
            <n-button size="tiny" text :disabled="!projectId || boardLoading" :loading="boardLoading"
              @click="fetchTasks(currentProjectId)" style="font-size: 16px;">
              <template #icon>
                <n-icon size="16">
                  <RefreshOutline />
                </n-icon>
              </template>
            </n-button>
          </div>
          <n-text depth="3">{{ t('task.dragToReorder') }}</n-text>
        </div>
        <div class="board-header__actions">
          <n-breadcrumb separator="/">
            <n-breadcrumb-item>
              <RouterLink to="/">{{ t('project.title') }}</RouterLink>
            </n-breadcrumb-item>
            <n-breadcrumb-item>
              <RouterLink v-if="currentProjectId" :to="{ name: 'project', params: { id: currentProjectId } }">
                {{ currentProjectName }}
              </RouterLink>
              <span v-else>{{ t('task.noProject') }}</span>
            </n-breadcrumb-item>
          </n-breadcrumb>
          <n-select style="width: 200px" size="small" :disabled="!projectId" v-model:value="worktreeFilterValue"
            :options="worktreeFilterOptions" :placeholder="t('task.allBranches')" clearable :consistent-menu-width="false" />
          <n-button size="small" type="primary" :disabled="!projectId" @click="openCreateDialog('todo')">
            <template #icon>
              <n-icon>
                <AddOutline />
              </n-icon>
            </template>
            {{ t('task.newTask') }}
          </n-button>
        </div>
      </n-space>
    </div>

    <div class="board-body">
      <n-spin :show="boardLoading">
        <n-empty v-if="!projectId" :description="t('task.noProject')" />
        <div v-else class="board-columns">
          <KanbanColumn v-for="column in columns" :key="column.key" :title="column.title" :status="column.key"
            :tasks="filteredTasksByStatus[column.key] ?? []" :show-add-button="projectId ? column.allowQuickAdd : false"
            :add-disabled="!projectId" @task-moved="handleTaskMoved" @task-clicked="handleTaskClicked"
            @task-edit="handleTaskEdit" @task-delete="handleTaskDeleteRequest" @task-copy="handleTaskCopy"
            @task-start-work="handleTaskStartWork" @add-click="handleColumnQuickAdd(column.key)" />
        </div>
      </n-spin>
    </div>

    <TaskCreateDialog v-if="projectId" v-model:show="showCreateDialog" :project-id="projectId"
      :default-status="createTargetStatus" @created="handleTaskCreated" />

    <TaskDetailDrawer v-model:show="showDetailDrawer" :project-id="projectId" :task-id="taskStore.selectedTaskId"
      @closed="taskStore.selectTask(null)" />
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch, type Ref } from 'vue';
import { RouterLink } from 'vue-router';
import { useClipboard } from '@vueuse/core';
import { useDialog, useMessage } from 'naive-ui';
import { AddOutline, RefreshOutline } from '@vicons/ionicons5';
import KanbanColumn from './KanbanColumn.vue';
import TaskCreateDialog from './TaskCreateDialog.vue';
import TaskDetailDrawer from './TaskDetailDrawer.vue';
import { useTaskStore } from '@/stores/task';
import { useTaskActions } from '@/composables/useTaskActions';
import { useProjectStore } from '@/stores/project';
import { useLocale } from '@/composables/useLocale';
import { extractItems, extractItem } from '@/api/response';
import type { Task } from '@/types/models';
import type TerminalPanel from '@/components/terminal/TerminalPanel.vue';

const { t } = useLocale();

const props = defineProps<{
  projectId?: string;
}>();

const taskStore = useTaskStore();
const projectStore = useProjectStore();
const message = useMessage();
const dialog = useDialog();
const { copy: copyTaskTitle, isSupported: clipboardSupported } = useClipboard();
const { listTasks, moveTask, deleteTask } = useTaskActions();

// 注入终端面板引用
const terminalPanelRef = inject<Ref<InstanceType<typeof TerminalPanel> | null>>('terminalPanelRef');

const showCreateDialog = ref(false);
const showDetailDrawer = ref(false);
const boardLoading = ref(false);
const deletingTaskId = ref<string | null>(null);

type ColumnConfig = {
  key: Task['status'];
  title: string;
  allowQuickAdd?: boolean;
};

const columns = computed<ColumnConfig[]>(() => [
  { key: 'todo', title: t('task.status.todo'), allowQuickAdd: true },
  { key: 'in_progress', title: t('task.status.inProgress'), allowQuickAdd: true },
  { key: 'done', title: t('task.status.done') },
]);

const currentProjectId = computed(() => props.projectId ?? '');
const currentProjectName = computed(() => projectStore.currentProject?.name ?? t('task.unnamedProject'));

const createTargetStatus = ref<Task['status']>('todo');

const ALL_WORKTREES_OPTION = '__all__';

const worktreeFilterValue = computed<string | null>({
  get: () => projectStore.selectedWorktreeId ?? ALL_WORKTREES_OPTION,
  set: value => {
    if (!value || value === ALL_WORKTREES_OPTION) {
      projectStore.setSelectedWorktree(null);
    } else {
      projectStore.setSelectedWorktree(value);
    }
  },
});

const worktreeFilterOptions = computed(() => {
  const options = (projectStore.worktrees ?? []).map(worktree => ({
    label: worktree.branchName,
    value: worktree.id,
  }));
  return [{ label: t('task.allBranches'), value: ALL_WORKTREES_OPTION }, ...options];
});

const filteredTasksByStatus = computed(() => {
  const selectedId = projectStore.selectedWorktreeId;
  const base = taskStore.tasksByStatus;
  if (!selectedId) {
    return base;
  }
  const buckets: Record<string, Task[]> = {};
  Object.keys(base).forEach(status => {
    buckets[status] = base[status].filter(task => task.worktreeId === selectedId);
  });
  return buckets;
});

watch(
  () => currentProjectId.value,
  id => {
    if (id) {
      fetchTasks(id);
    } else {
      taskStore.setTasks([]);
    }
  },
  { immediate: true },
);

async function fetchTasks(projectId: string) {
  boardLoading.value = true;
  try {
    const response = await listTasks.send(projectId);
    const items = extractItems(response) as unknown as Task[];
    taskStore.setTasks(items);
  } catch (error: any) {
    message.error(error?.message ?? t('task.loadTasksFailed'));
  } finally {
    boardLoading.value = false;
  }
}

async function handleTaskMoved(event: { taskId: string; newStatus: Task['status']; newIndex: number; orderedTasks: Task[] }) {
  const { taskId, newStatus, newIndex, orderedTasks } = event;
  const siblings = orderedTasks;
  let orderIndex = 1000;

  if (siblings.length <= 1) {
    orderIndex = 1000;
  } else if (newIndex <= 0) {
    const next = siblings[1];
    orderIndex = next ? next.orderIndex / 2 : 500;
  } else if (newIndex >= siblings.length - 1) {
    const prev = siblings[newIndex - 1] ?? siblings[siblings.length - 2];
    orderIndex = prev.orderIndex + 1000;
  } else {
    const prev = siblings[newIndex - 1];
    const next = siblings[newIndex + 1];
    orderIndex = prev && next ? (prev.orderIndex + next.orderIndex) / 2 : prev?.orderIndex ?? 1000;
  }

  try {
    const response = await moveTask.send(taskId, { status: newStatus, orderIndex });
    const updated = extractItem(response) as unknown as Task | undefined;
    if (updated) {
      taskStore.upsertTask(updated);
    }
  } catch (error: any) {
    message.error(error?.message ?? t('task.moveTaskFailed'));
    fetchTasks(currentProjectId.value);
  }
}

function handleTaskClicked(task: Task) {
  taskStore.selectTask(task.id);
  showDetailDrawer.value = true;
}

function handleTaskEdit(task: Task) {
  handleTaskClicked(task);
}

function openCreateDialog(status: Task['status'] = 'todo') {
  createTargetStatus.value = status;
  showCreateDialog.value = true;
}

function handleColumnQuickAdd(status: Task['status']) {
  if (!props.projectId) {
    return;
  }
  openCreateDialog(status);
}

function handleTaskDeleteRequest(task: Task) {
  dialog.warning({
    title: t('task.deleteTaskTitle'),
    content: t('task.deleteTaskConfirm', { title: task.title }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: () => performTaskDelete(task),
  });
}

async function performTaskDelete(task: Task) {
  if (deletingTaskId.value) {
    return;
  }
  deletingTaskId.value = task.id;
  try {
    await deleteTask.send(task.id);
    taskStore.removeTask(task.id);
    message.success(t('task.taskDeleted'));
  } catch (error: any) {
    message.error(error?.message ?? t('task.deleteTaskFailed'));
  } finally {
    deletingTaskId.value = null;
  }
}

async function handleTaskCopy(task: Task) {
  try {
    if (!clipboardSupported.value) {
      throw new Error(t('task.copyNotSupported'));
    }
    await copyTaskTitle(task.title);
    message.success(t('task.taskNameCopied'));
  } catch (error: any) {
    message.error(error?.message ?? t('task.copyTaskNameFailed'));
  }
}

function handleTaskCreated(task: Task) {
  taskStore.upsertTask(task);
}

async function handleTaskStartWork(task: Task) {
  try {
    // 确定要使用的worktree
    let targetWorktreeId = task.worktreeId;
    let targetWorktree = targetWorktreeId
      ? projectStore.worktrees.find(w => w.id === targetWorktreeId)
      : null;

    // 如果任务没有关联分支，或者关联的分支不存在，使用主分支
    if (!targetWorktree) {
      targetWorktree = projectStore.worktrees.find(w => w.isMain);
      if (!targetWorktree) {
        message.error(t('task.noAvailableWorktree'));
        return;
      }
      targetWorktreeId = targetWorktree.id;
    }

    // 使用终端面板创建终端会话（会自动展开终端面板）
    if (terminalPanelRef?.value) {
      terminalPanelRef.value.createTerminal({
        worktreeId: targetWorktreeId!,
        title: task.title,
        workingDir: targetWorktree.path,
      });
    }

    // 更新任务状态为"进行中"
    if (task.status !== 'in_progress') {
      const response = await moveTask.send(task.id, { status: 'in_progress' });
      const updated = extractItem(response) as unknown as Task | undefined;
      if (updated) {
        taskStore.upsertTask(updated);
      }
    }

    message.success(t('task.terminalCreatedAndTaskUpdated'));
  } catch (error: any) {
    message.error(error?.message ?? t('task.startWorkFailed'));
  }
}
</script>

<style scoped>
.kanban-board {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: var(--app-surface-color, #ffffff);
}

.board-header {
  padding: 16px 24px;
  border-bottom: var(--kanban-border, 1px solid var(--n-border-color));
}

.board-header h2 {
  margin: 0 0 4px;
}

.board-header__actions {
  display: flex;
  align-items: center;
  gap: 16px;
}

.board-body {
  flex: 1;
  padding: 16px;
  overflow: hidden;
  min-height: 0;
}

.board-columns {
  display: grid;
  grid-template-columns: repeat(3, minmax(280px, 1fr));
  grid-template-rows: 100%;
  gap: 16px;
  height: calc(100vh - 160px);
  max-height: 100%;
  overflow: hidden;
}

@media (max-width: 1200px) {
  .board-body {
    overflow-y: auto;
  }

  .board-columns {
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    grid-template-rows: auto;
    height: auto;
    min-height: 100%;
  }
}
</style>
