<template>
  <n-drawer
    width="520"
    placement="right"
    :show="show"
    :on-after-enter="handleAfterEnter"
    @update:show="emit('update:show', $event as boolean)"
    @after-leave="emit('closed')"
  >
    <n-drawer-content :title="t('task.taskDetail')">
      <n-spin :show="detailLoading">
        <n-empty v-if="!task" :description="t('task.pleaseSelectTask')" />
        <div v-else class="task-detail">
          <n-form label-placement="top" :model="form">
            <n-form-item :label="t('task.fieldTitle')">
              <n-input v-model:value="form.title" />
            </n-form-item>

            <n-form-item :label="t('task.fieldDescription')">
              <n-input
                v-model:value="form.description"
                type="textarea"
                rows="5"
                :placeholder="t('task.useMarkdown')"
              />
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
              <n-date-picker
                v-model:formatted-value="form.dueDate"
                type="date"
                value-format="yyyy-MM-dd"
                clearable
              />
            </n-form-item>

            <n-form-item :label="t('task.tags')">
              <n-dynamic-tags v-model:value="form.tags" />
            </n-form-item>
          </n-form>

          <n-divider />

          <section>
            <div class="task-detail__section-header">
              <h3>{{ t('task.comments') }}</h3>
            </div>

            <n-space vertical size="small">
              <n-input
                v-model:value="newComment"
                type="textarea"
                rows="3"
                :placeholder="t('task.commentPlaceholder')"
              />
              <n-button
                type="primary"
                size="small"
                :loading="commentLoading"
                @click="handleCreateComment"
              >
                {{ t('task.publishComment') }}
              </n-button>
            </n-space>

            <n-list v-if="comments.length" bordered style="margin-top: 12px">
              <n-list-item v-for="comment in comments" :key="comment.id">
                <n-space justify="space-between" align="center">
                  <div class="task-detail__comment">
                    <div class="content">{{ comment.content }}</div>
                    <n-text depth="3">{{ formatDate(comment.createdAt) }}</n-text>
                  </div>
                  <n-button
                    quaternary
                    type="error"
                    size="tiny"
                    @click="handleDeleteComment(comment.id)"
                  >
                    {{ t('task.deleteComment') }}
                  </n-button>
                </n-space>
              </n-list-item>
            </n-list>
            <n-empty v-else :description="t('task.noComments')" />
          </section>
        </div>
      </n-spin>

      <template #footer>
        <n-space justify="space-between" style="width: 100%">
          <n-button tertiary @click="emit('update:show', false)">{{ t('common.close') }}</n-button>
          <n-space>
            <n-button type="error" tertiary :loading="deleteLoading" @click="confirmDelete">{{
              t('task.deleteTask')
            }}</n-button>
            <n-button type="primary" :loading="saveLoading" @click="handleSave">{{
              t('task.saveChanges')
            }}</n-button>
          </n-space>
        </n-space>
      </template>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import dayjs from 'dayjs';
import { useTaskStore } from '@/stores/task';
import { useProjectStore } from '@/stores/project';
import { taskActions } from '@/composables/useTaskActions';
import { extractItem, extractItems } from '@/api/response';
import type { Task, TaskComment } from '@/types/models';
import { useLocale } from '@/composables/useLocale';

const { t } = useLocale();

const props = defineProps<{
  show: boolean;
  taskId?: string | null;
  projectId?: string;
}>();

const emit = defineEmits<{
  'update:show': [boolean];
  closed: [];
}>();

const taskStore = useTaskStore();
const projectStore = useProjectStore();
const {
  getTask,
  updateTask,
  bindWorktree,
  deleteTask,
  listComments,
  createComment,
  deleteCommentReq,
  invalidateTaskCache,
} = taskActions;
const message = useMessage();
const dialog = useDialog();

const form = ref({
  title: '',
  description: '',
  priority: 0,
  worktreeId: null as string | null,
  dueDate: null as string | null,
  tags: [] as string[],
});
const originalWorktreeId = ref<string | null>(null);
const newComment = ref('');

const detailLoading = ref(false);
const saveLoading = ref(false);
const deleteLoading = ref(false);
const commentLoading = ref(false);

const task = computed<Task | null>(() => {
  if (!props.taskId) {
    return null;
  }
  return taskStore.tasks.find(item => item.id === props.taskId) ?? null;
});

const comments = computed<TaskComment[]>(() => {
  if (!props.taskId) {
    return [];
  }
  return taskStore.commentsMap[props.taskId] ?? [];
});

const worktreeOptions = computed(() =>
  (projectStore.worktrees ?? []).map(worktree => ({
    label: worktree.branchName,
    value: worktree.id,
  }))
);

const priorityOptions = computed(() => [
  { label: t('task.priority.normal'), value: 0 },
  { label: t('task.priority.low'), value: 1 },
  { label: t('task.priority.medium'), value: 2 },
  { label: t('task.priority.high'), value: 3 },
]);

watch(
  () => task.value,
  value => {
    if (!value) {
      return;
    }
    form.value = {
      title: value.title,
      description: value.description ?? '',
      priority: value.priority,
      worktreeId: value.worktreeId ?? null,
      dueDate: value.dueDate ? dayjs(value.dueDate).format('YYYY-MM-DD') : null,
      tags: [...(value.tags ?? [])],
    };
    originalWorktreeId.value = value.worktreeId ?? null;
  },
  { immediate: true }
);

const handleAfterEnter = () => {
  void loadData();
};

async function loadData() {
  if (!props.show || !props.taskId) {
    return;
  }
  detailLoading.value = true;
  try {
    const [taskResponse, commentsResponse] = await Promise.all([
      getTask.send(props.taskId),
      listComments.send(props.taskId),
    ]);
    const freshTask = extractItem(taskResponse) as unknown as Task | undefined;
    if (freshTask) {
      taskStore.upsertTask(freshTask);
    }
    const items = extractItems(commentsResponse) as unknown as TaskComment[];
    taskStore.setComments(props.taskId, items);
  } catch (error: any) {
    message.error(error?.message ?? t('task.loadCommentsFailed'));
  } finally {
    detailLoading.value = false;
  }
}

async function handleSave() {
  if (!task.value) {
    return;
  }
  saveLoading.value = true;
  try {
    const payload = {
      title: form.value.title,
      description: form.value.description,
      priority: form.value.priority,
      tags: form.value.tags,
      dueDate: form.value.dueDate,
    };
    const response = await updateTask.send(task.value.id, payload);
    let updated = extractItem(response) as unknown as Task | undefined;

    if (form.value.worktreeId !== originalWorktreeId.value) {
      const bindResponse = await bindWorktree.send(task.value.id, form.value.worktreeId);
      updated = extractItem(bindResponse) as unknown as Task | undefined;
    }

    if (updated) {
      taskStore.upsertTask(updated);
      originalWorktreeId.value = updated.worktreeId ?? null;
    }
    // 更新后使缓存失效，确保其他地方获取最新数据
    invalidateTaskCache();
    message.success(t('task.taskUpdated'));
  } catch (error: any) {
    message.error(error?.message ?? t('task.saveFailed'));
  } finally {
    saveLoading.value = false;
  }
}

function confirmDelete() {
  if (!task.value) {
    return;
  }
  dialog.warning({
    title: t('task.deleteTask'),
    content: t('task.deleteConfirm'),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: handleDelete,
  });
}

async function handleDelete() {
  if (!task.value) {
    return;
  }
  deleteLoading.value = true;
  try {
    await deleteTask.send(task.value.id);
    taskStore.removeTask(task.value.id);
    emit('update:show', false);
    message.success(t('task.taskDeleted'));
  } catch (error: any) {
    message.error(error?.message ?? t('task.deleteTaskFailed'));
  } finally {
    deleteLoading.value = false;
  }
}

async function handleCreateComment() {
  if (!task.value || !newComment.value.trim()) {
    return;
  }
  commentLoading.value = true;
  try {
    const response = await createComment.send(task.value.id, newComment.value.trim());
    const comment = extractItem<TaskComment>(response);
    if (comment) {
      taskStore.appendComment(task.value.id, comment);
      newComment.value = '';
    }
  } catch (error: any) {
    message.error(error?.message ?? t('task.publishCommentFailed'));
  } finally {
    commentLoading.value = false;
  }
}

async function handleDeleteComment(commentId: string) {
  if (!task.value) {
    return;
  }
  try {
    await deleteCommentReq.send(commentId);
    taskStore.removeComment(task.value.id, commentId);
  } catch (error: any) {
    message.error(error?.message ?? t('task.deleteCommentFailed'));
  }
}

const formatDate = (value: string) => dayjs(value).format('YYYY-MM-DD HH:mm');
</script>

<style scoped>
.task-detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.task-detail__section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.task-detail__section-header h3 {
  margin: 0;
  font-size: 16px;
}

.task-detail__comment {
  display: flex;
  flex-direction: column;
}

.task-detail__comment .content {
  margin-bottom: 4px;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
