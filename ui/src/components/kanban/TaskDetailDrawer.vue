<template>
  <n-drawer
    width="520"
    placement="right"
    :show="show"
    @update:show="emit('update:show', $event as boolean)"
    @after-leave="emit('closed')"
  >
    <n-drawer-content title="任务详情">
      <n-spin :show="detailLoading">
        <n-empty v-if="!task" description="请选择一个任务" />
        <div v-else class="task-detail">
          <n-form label-placement="top" :model="form">
            <n-form-item label="标题">
              <n-input v-model:value="form.title" />
            </n-form-item>

            <n-form-item label="描述">
              <n-input v-model:value="form.description" type="textarea" rows="5" placeholder="使用 Markdown 描述任务" />
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
              <n-date-picker
                v-model:formatted-value="form.dueDate"
                type="date"
                value-format="yyyy-MM-dd"
                clearable
              />
            </n-form-item>

            <n-form-item label="标签">
              <n-dynamic-tags v-model:value="form.tags" />
            </n-form-item>
          </n-form>

          <n-divider />

          <section>
            <div class="task-detail__section-header">
              <h3>评论</h3>
            </div>

            <n-space vertical size="small">
              <n-input
                v-model:value="newComment"
                type="textarea"
                rows="3"
                placeholder="输入评论内容"
              />
              <n-button type="primary" size="small" :loading="commentLoading" @click="handleCreateComment">
                发布评论
              </n-button>
            </n-space>

            <n-list v-if="comments.length" bordered style="margin-top: 12px">
              <n-list-item v-for="comment in comments" :key="comment.id">
                <n-space justify="space-between" align="center">
                  <div class="task-detail__comment">
                    <div class="content">{{ comment.content }}</div>
                    <n-text depth="3">{{ formatDate(comment.createdAt) }}</n-text>
                  </div>
                  <n-button quaternary type="error" size="tiny" @click="handleDeleteComment(comment.id)">
                    删除
                  </n-button>
                </n-space>
              </n-list-item>
            </n-list>
            <n-empty v-else description="还没有评论" />
          </section>
        </div>
      </n-spin>

      <template #footer>
        <n-space justify="space-between" style="width: 100%">
          <n-button tertiary @click="emit('update:show', false)">关闭</n-button>
          <n-space>
            <n-button type="error" tertiary :loading="deleteLoading" @click="confirmDelete">删除任务</n-button>
            <n-button type="primary" :loading="saveLoading" @click="handleSave">保存修改</n-button>
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
import { useTaskActions } from '@/composables/useTaskActions';
import { extractItem, extractItems } from '@/api/response';
import type { Task, TaskComment } from '@/types/models';

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
const { updateTask, bindWorktree, deleteTask, listComments, createComment, deleteCommentReq } = useTaskActions();
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
  })),
);

const priorityOptions = [
  { label: '普通', value: 0 },
  { label: '低', value: 1 },
  { label: '中', value: 2 },
  { label: '高', value: 3 },
];

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
  { immediate: true },
);

watch(
  () => props.taskId,
  id => {
    if (props.show && id) {
      loadComments(id);
    }
  },
  { immediate: true },
);

watch(
  () => props.show,
  value => {
    if (value && props.taskId) {
      loadComments(props.taskId);
    }
  },
);

async function loadComments(taskId: string) {
  detailLoading.value = true;
  try {
    const response = await listComments.send(taskId);
    const items = extractItems(response) as unknown as TaskComment[];
    taskStore.setComments(taskId, items);
  } catch (error: any) {
    message.error(error?.message ?? '加载评论失败');
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
    message.success('任务已更新');
  } catch (error: any) {
    message.error(error?.message ?? '保存失败');
  } finally {
    saveLoading.value = false;
  }
}

function confirmDelete() {
  if (!task.value) {
    return;
  }
  dialog.warning({
    title: '删除任务',
    content: '确定要删除该任务吗？操作不可恢复。',
    positiveText: '删除',
    negativeText: '取消',
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
    message.success('任务已删除');
  } catch (error: any) {
    message.error(error?.message ?? '删除失败');
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
    message.error(error?.message ?? '发表评论失败');
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
    message.error(error?.message ?? '删除评论失败');
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
}
</style>
