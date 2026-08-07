<template>
  <div class="kanban-board">
    <div class="board-header">
      <n-space justify="space-between" align="center">
        <div>
          <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 4px">
            <h2 style="margin: 0">{{ t('task.kanbanTitle') }}</h2>
            <n-button
              size="tiny"
              text
              :disabled="!projectId || boardLoading"
              :loading="boardLoading"
              @click="fetchTasks(currentProjectId)"
              style="font-size: 16px"
            >
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
              <RouterLink
                v-if="currentProjectId"
                :to="{ name: 'project', params: { id: currentProjectId } }"
              >
                {{ currentProjectName }}
              </RouterLink>
              <span v-else>{{ t('task.noProject') }}</span>
            </n-breadcrumb-item>
          </n-breadcrumb>
          <n-select
            style="width: 200px"
            size="small"
            :disabled="!projectId"
            v-model:value="worktreeFilterValue"
            :options="worktreeFilterOptions"
            :placeholder="t('task.allBranches')"
            clearable
            :consistent-menu-width="false"
          />
          <n-button
            size="small"
            type="primary"
            :disabled="!projectId"
            @click="openCreateDialog('todo')"
          >
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

        <!-- 移动端标签切换视图 -->
        <div v-else-if="isMobile" class="board-tabs-view">
          <n-tabs v-model:value="activeColumn" type="line" animated>
            <n-tab-pane
              v-for="column in columns"
              :key="column.key"
              :name="column.key"
              :tab="column.title"
            >
              <div class="mobile-column-container">
                <KanbanColumn
                  :title="column.title"
                  :status="column.key"
                  :tasks="filteredTasksByStatus[column.key] ?? []"
                  :show-add-button="projectId ? column.allowQuickAdd : false"
                  :add-disabled="!projectId"
                  :linked-terminals="linkedTerminals"
                  :is-mobile="true"
                  @task-moved="handleTaskMoved"
                  @task-clicked="handleTaskClicked"
                  @task-edit="handleTaskEdit"
                  @task-delete="handleTaskDeleteRequest"
                  @task-copy="handleTaskCopy"
                  @task-start-work="handleTaskStartWork"
                  @view-terminal="handleTaskViewTerminal"
                  @archive-click="handleArchiveClick"
                  @add-click="handleColumnQuickAdd(column.key)"
                />
              </div>
            </n-tab-pane>
          </n-tabs>
        </div>

        <!-- 桌面端网格视图 -->
        <div v-else class="board-columns">
          <KanbanColumn
            v-for="column in columns"
            :key="column.key"
            :title="column.title"
            :status="column.key"
            :tasks="filteredTasksByStatus[column.key] ?? []"
            :show-add-button="projectId ? column.allowQuickAdd : false"
            :add-disabled="!projectId"
            :linked-terminals="linkedTerminals"
            @task-moved="handleTaskMoved"
            @task-clicked="handleTaskClicked"
            @task-edit="handleTaskEdit"
            @task-delete="handleTaskDeleteRequest"
            @task-copy="handleTaskCopy"
            @task-start-work="handleTaskStartWork"
            @view-terminal="handleTaskViewTerminal"
            @archive-click="handleArchiveClick"
            @add-click="handleColumnQuickAdd(column.key)"
          />
        </div>
      </n-spin>
    </div>

    <TaskCreateDialog
      v-if="projectId"
      v-model:show="showCreateDialog"
      :project-id="projectId"
      :default-status="createTargetStatus"
      @created="handleTaskCreated"
    />

    <TaskDetailDrawer
      v-model:show="showDetailDrawer"
      :project-id="projectId"
      :task-id="taskStore.selectedTaskId"
      @closed="taskStore.selectTask(null)"
    />

    <TaskArchiveDialog
      v-model:show="showArchiveDialog"
      :tasks="archiveDialogTasks"
      @archived="handleTasksArchived"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { RouterLink } from 'vue-router';
import { NInput, useDialog, useMessage } from 'naive-ui';
import { AddOutline, RefreshOutline } from '@vicons/ionicons5';
import { storeToRefs } from 'pinia';
import { useAppClipboard } from '@/composables/useAppClipboard';
import { useResponsive } from '@/composables/useResponsive';
import KanbanColumn from './KanbanColumn.vue';
import TaskCreateDialog from './TaskCreateDialog.vue';
import TaskArchiveDialog from './TaskArchiveDialog.vue';
import TaskDetailDrawer from './TaskDetailDrawer.vue';
import { useSettingsStore } from '@/stores/settings';
import { useTaskStore } from '@/stores/task';
import { useTerminalStore, type TerminalCreateOptions } from '@/stores/terminal';
import { taskActions } from '@/composables/useTaskActions';
import { useProjectStore } from '@/stores/project';
import { useLocale } from '@/composables/useLocale';
import { extractItems, extractItem } from '@/api/response';
import type { Task } from '@/types/models';
import type { StartWorkAction } from '@/types/startWork';

const { t } = useLocale();
const { isMobile, isDesktop } = useResponsive();

// 移动端标签切换
const activeColumn = ref<Task['status']>('in_progress');

const props = defineProps<{
  projectId?: string;
}>();

type LinkedTerminalSummary = {
  sessionId: string;
  agentName: string;
  sessionTitle: string;
};

const taskStore = useTaskStore();
const projectStore = useProjectStore();
const terminalStore = useTerminalStore();
const settingsStore = useSettingsStore();
const { terminalQuickActions } = storeToRefs(settingsStore);
const message = useMessage();
const dialog = useDialog();
const { copyText, isSupported: clipboardSupported } = useAppClipboard();
const { listTasks, moveTask, deleteTask } = taskActions;

const showCreateDialog = ref(false);
const showDetailDrawer = ref(false);
const boardLoading = ref(false);
const deletingTaskId = ref<string | null>(null);
const showArchiveDialog = ref(false);
const archiveDialogTasks = ref<Task[]>([]);

type ColumnConfig = {
  key: Task['status'];
  title: string;
  allowQuickAdd?: boolean;
};

const columns = computed<ColumnConfig[]>(() => [
  { key: 'todo', title: t('task.status.todo'), allowQuickAdd: true },
  { key: 'in_progress', title: t('task.status.inProgress'), allowQuickAdd: true },
  { key: 'done', title: t('task.status.done') },
  { key: 'archived', title: t('task.status.archived') },
]);

const currentProjectId = computed(() => props.projectId ?? '');
const currentProjectName = computed(
  () => projectStore.currentProject?.name ?? t('task.unnamedProject')
);

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

const terminalTabs = computed(() => terminalStore.getTabs(currentProjectId.value));

const linkedTerminals = computed<Record<string, LinkedTerminalSummary>>(() => {
  const map: Record<string, LinkedTerminalSummary> = {};
  const sessions = terminalTabs.value ?? [];
  sessions.forEach(session => {
    if (!session.taskId) {
      return;
    }
    map[session.taskId] = {
      sessionId: session.id,
      agentName:
        session.aiAssistant?.displayName ||
        session.aiAssistant?.name ||
        session.aiAssistant?.type ||
        t('terminal.aiAssistantDetected'),
      sessionTitle: session.title,
    };
  });
  return map;
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
  { immediate: true }
);

onMounted(() => {
  terminalStore.emitter.on('task:view', handleTaskViewEvent);
});

onBeforeUnmount(() => {
  terminalStore.emitter.off('task:view', handleTaskViewEvent);
});

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

async function handleTaskMoved(event: {
  taskId: string;
  newStatus: Task['status'];
  newIndex: number;
  orderedTasks: Task[];
}) {
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
    orderIndex =
      prev && next ? (prev.orderIndex + next.orderIndex) / 2 : (prev?.orderIndex ?? 1000);
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
  await copyText(task.title, {
    failureMessage: t('task.copyTaskNameFailed'),
    successMessage: t('task.taskNameCopied'),
    unsupportedMessage: t('task.copyNotSupported'),
  });
}

function handleTaskCreated(task: Task) {
  taskStore.upsertTask(task);
}

function handleArchiveClick() {
  const doneTasks = filteredTasksByStatus.value['done'] ?? [];
  if (!doneTasks.length) {
    message.info(t('task.archiveEmptyHint'));
    return;
  }
  archiveDialogTasks.value = [...doneTasks];
  showArchiveDialog.value = true;
}

function handleTasksArchived(tasks: Task[]) {
  for (const task of tasks) {
    taskStore.upsertTask(task);
  }
}

function focusLinkedTerminal(task: Task): boolean {
  const session = terminalStore.getSessionByTask(task.id, currentProjectId.value);
  if (!session) {
    return false;
  }
  terminalStore.focusSession(currentProjectId.value, session.id);
  return true;
}

function handleTaskViewTerminal(task: Task) {
  if (!focusLinkedTerminal(task)) {
    message.warning(t('task.noLinkedTerminal'));
  } else {
    message.success(t('task.jumpToLinkedTerminal'));
  }
}

function normalizeTerminalEnter(value: string) {
  const trimmed = value.replace(/\s+$/, '');
  if (!trimmed) {
    return '';
  }
  if (trimmed.endsWith('\n') || trimmed.endsWith('\r')) {
    return trimmed;
  }
  return trimmed + '\r';
}

function resolveAgentCommand(agent: Exclude<StartWorkAction, 'terminal'>): string {
  const commandFor = (id: string) =>
    terminalQuickActions.value
      .find(action => action.id === id && action.enabled)
      ?.command?.trim() ?? '';
  if (agent === 'claude') {
    return commandFor('claude') || commandFor('ccr') || 'claude';
  }
  return commandFor('codex') || 'codex';
}

function sendWithRetry(sessionId: string, input: string): Promise<boolean> {
  return new Promise(resolve => {
    const payload = { type: 'input', data: input };
    const startAt = Date.now();
    const timeoutMs = 8000;
    if (terminalStore.send(sessionId, payload)) {
      resolve(true);
      return;
    }
    const timer = window.setInterval(() => {
      if (terminalStore.send(sessionId, payload)) {
        window.clearInterval(timer);
        resolve(true);
        return;
      }
      if (Date.now() - startAt > timeoutMs) {
        window.clearInterval(timer);
        resolve(false);
      }
    }, 200);
  });
}

function waitForTerminalReady(sessionId: string): Promise<void> {
  return new Promise(resolve => {
    const isReady = () => {
      const tab = terminalStore.getTabs(currentProjectId.value).find(t => t.id === sessionId);
      return tab?.clientStatus === 'ready';
    };

    if (isReady()) {
      resolve();
      return;
    }

    let timer = 0;

    const handleReady = (payload: any) => {
      if (payload?.type === 'ready') {
        cleanup();
        resolve();
      }
    };

    const cleanup = () => {
      terminalStore.emitter.off(sessionId, handleReady);
      if (timer) {
        window.clearTimeout(timer);
      }
    };

    terminalStore.emitter.on(sessionId, handleReady);

    timer = window.setTimeout(() => {
      cleanup();
      resolve();
    }, 8000);
  });
}

async function sendAgentCommandAndPrompt(
  sessionId: string,
  agent: Exclude<StartWorkAction, 'terminal'>,
  prompt: string
): Promise<boolean> {
  const command = resolveAgentCommand(agent);
  const commandInput = normalizeTerminalEnter(command);
  const promptInput = normalizeTerminalEnter(prompt);

  if (!commandInput || !promptInput) {
    return false;
  }

  await waitForTerminalReady(sessionId);
  const sentCommand = await sendWithRetry(sessionId, commandInput);
  if (!sentCommand) {
    return false;
  }
  await new Promise<void>(resolve => {
    window.setTimeout(() => resolve(), 500);
  });
  return await sendWithRetry(sessionId, promptInput);
}

function promptAgentPrompt(
  task: Task,
  agent: Exclude<StartWorkAction, 'terminal'>
): Promise<string | null> {
  return new Promise(resolve => {
    let settled = false;
    const resolveOnce = (value: string | null) => {
      if (settled) {
        return;
      }
      settled = true;
      resolve(value);
    };

    const valueRef = ref(task.title);
    const agentLabel =
      agent === 'claude' ? t('task.startWorkMenuClaude') : t('task.startWorkMenuCodex');

    dialog.create({
      title: t('task.startWorkPromptTitle', { agent: agentLabel }),
      content: () =>
        h(NInput, {
          value: valueRef.value,
          'onUpdate:value': (val: string) => (valueRef.value = val),
          type: 'textarea',
          placeholder: t('task.startWorkPromptPlaceholder'),
          autosize: { minRows: 4, maxRows: 12 },
        }),
      positiveText: t('task.startWorkPromptRun'),
      negativeText: t('common.cancel'),
      onPositiveClick: () => {
        const prompt = valueRef.value.trim();
        if (!prompt) {
          message.warning(t('task.startWorkPromptRequired'));
          return false;
        }
        resolveOnce(prompt);
        return true;
      },
      onNegativeClick: () => resolveOnce(null),
      onClose: () => resolveOnce(null),
    });
  });
}

async function handleTaskStartWork(task: Task, action: StartWorkAction = 'terminal') {
  try {
    if (action === 'terminal') {
      if (focusLinkedTerminal(task)) {
        message.success(t('task.jumpToLinkedTerminal'));
        return;
      }
    }

    if (!currentProjectId.value) {
      message.warning(t('task.noProject'));
      return;
    }

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

    const terminalOptions: TerminalCreateOptions = {
      worktreeId: targetWorktreeId!,
      title: task.title,
      workingDir: targetWorktree.path,
      taskId: task.id,
    };

    const createTaskTerminal = async (): Promise<string | undefined> => {
      const sessionId = await terminalStore.createSession(currentProjectId.value, terminalOptions);
      terminalStore.focusSession(currentProjectId.value, sessionId);
      return sessionId;
    };

    if (action !== 'terminal') {
      const prompt = await promptAgentPrompt(task, action);
      if (!prompt) {
        return;
      }

      const sessionId = await createTaskTerminal();
      if (!sessionId) {
        return;
      }

      const sent = await sendAgentCommandAndPrompt(sessionId, action, prompt);
      if (!sent) {
        message.error(t('task.startWorkFailed'));
        return;
      }
      message.success(
        t('task.agentStarted', {
          agent: action === 'claude' ? t('task.startWorkMenuClaude') : t('task.startWorkMenuCodex'),
        })
      );
    } else {
      const sessionId = await createTaskTerminal();
      if (!sessionId) {
        return;
      }
    }

    // 更新任务状态为"进行中"
    if (task.status !== 'in_progress') {
      const response = await moveTask.send(task.id, { status: 'in_progress' });
      const updated = extractItem(response) as unknown as Task | undefined;
      if (updated) {
        taskStore.upsertTask(updated);
      }
    }

    if (action === 'terminal') {
      message.success(t('task.terminalCreatedAndTaskUpdated'));
    }
  } catch (error: any) {
    message.error(error?.message ?? t('task.startWorkFailed'));
  }
}

function handleTaskViewEvent(event: { taskId?: string; projectId?: string }) {
  if (!event?.taskId) {
    return;
  }
  if (event.projectId && event.projectId !== currentProjectId.value) {
    return;
  }
  const task = taskStore.tasks.find(item => item.id === event.taskId);
  if (!task) {
    message.warning(t('task.taskNotFound'));
    return;
  }
  taskStore.selectTask(task.id);
  showDetailDrawer.value = true;
  const focused = focusLinkedTerminal(task);
  if (!focused) {
    message.warning(t('task.noLinkedTerminal'));
  }
  message.success(t('task.openedLinkedTaskPanel'));
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
  grid-template-columns: repeat(4, minmax(280px, 1fr));
  grid-template-rows: 100%;
  gap: 16px;
  height: calc(100vh - 160px);
  max-height: 100%;
  overflow: hidden;
}

/* 移动端标签视图样式 */
.board-tabs-view {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.board-tabs-view :deep(.n-tabs) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.board-tabs-view :deep(.n-tabs-nav) {
  position: sticky;
  top: 0;
  z-index: 10;
  background-color: var(--app-surface-color, #ffffff);
}

.board-tabs-view :deep(.n-tabs-pane-wrapper) {
  flex: 1;
  overflow: hidden;
}

.board-tabs-view :deep(.n-tab-pane) {
  height: 100%;
  padding: 0;
}

.mobile-column-container {
  height: calc(100vh - 240px);
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

/* 平板端响应式 */
@media (max-width: 1200px) and (min-width: 768px) {
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

/* 移动端头部适配 */
@media (max-width: 767px) {
  .board-header {
    padding: 12px 16px;
  }

  .board-header__actions {
    flex-wrap: wrap;
    gap: 8px;
  }

  .board-header__actions .n-breadcrumb {
    width: 100%;
    order: -1;
  }

  .board-header__actions .n-select {
    flex: 1;
    min-width: 120px;
  }

  .board-body {
    padding: 8px;
  }

  .mobile-column-container {
    height: calc(100vh - 280px);
  }
}
</style>
