<template>
  <n-card class="task-card" size="small" @click="emit('click')">
    <div class="task-card__header">
      <div class="task-card__title">
        <n-ellipsis :line-clamp="2">
          {{ task.title }}
        </n-ellipsis>
        <n-tag v-if="priorityLabel" size="tiny" :type="priorityType" :bordered="false">
          {{ priorityLabel }}
        </n-tag>
      </div>
      <div class="task-card__actions">
        <n-tooltip trigger="hover" placement="bottom">
          <template #trigger>
            <n-button quaternary circle size="tiny" type="primary" @click.stop="handleStartWork">
              <n-icon size="14">
                <PlayCircleOutline />
              </n-icon>
            </n-button>
          </template>
          {{ t('task.startWorkTooltip') }}
        </n-tooltip>
        <n-tooltip trigger="hover" placement="bottom">
          <template #trigger>
            <n-button quaternary circle size="tiny" @click.stop="handleCopy">
              <n-icon size="14">
                <CopyOutline />
              </n-icon>
            </n-button>
          </template>
          {{ t('task.copyTaskName') }}
        </n-tooltip>
        <n-tooltip trigger="hover" placement="bottom">
          <template #trigger>
            <n-button quaternary circle size="tiny" @click.stop="handleEdit">
              <n-icon size="14">
                <CreateOutline />
              </n-icon>
            </n-button>
          </template>
          {{ t('task.quickEdit') }}
        </n-tooltip>
        <n-tooltip trigger="hover" placement="bottom">
          <template #trigger>
            <n-button quaternary circle size="tiny" @click.stop="handleDelete">
              <n-icon size="14">
                <TrashOutline />
              </n-icon>
            </n-button>
          </template>
          {{ t('task.quickDelete') }}
        </n-tooltip>
      </div>
    </div>

    <div class="task-card__meta">
      <n-space size="small" wrap>
        <n-tag v-if="displayBranchName" size="tiny" :bordered="false">
          {{ displayBranchName }}
        </n-tag>
        <n-tag v-if="task.dueDate" size="tiny" :type="isOverdue ? 'error' : 'default'" :bordered="false">
          {{ t('task.duePrefix') }} {{ formatDate(task.dueDate) }}
        </n-tag>
      </n-space>
    </div>

    <n-space v-if="task.tags?.length" size="small" wrap>
      <n-tag v-for="tag in task.tags" :key="tag" size="tiny" :bordered="false">
        {{ tag }}
      </n-tag>
    </n-space>
  </n-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import dayjs from 'dayjs';
import { CopyOutline, CreateOutline, TrashOutline, PlayCircleOutline } from '@vicons/ionicons5';
import type { Task } from '@/types/models';
import { useLocale } from '@/composables/useLocale';

const { t } = useLocale();

const props = defineProps<{
  task: Task;
}>();

const emit = defineEmits<{
  click: [];
  edit: [];
  delete: [];
  copy: [];
  'start-work': [];
}>();

const priorityMap = computed((): Record<number, { label: string; type: 'default' | 'info' | 'warning' | 'error' }> => ({
  0: { label: '', type: 'default' as const },
  1: { label: t('task.priority.low'), type: 'info' as const },
  2: { label: t('task.priority.medium'), type: 'warning' as const },
  3: { label: t('task.priority.high'), type: 'error' as const },
}));

const priorityType = computed(() => priorityMap.value[props.task.priority]?.type ?? 'default');
const priorityLabel = computed(() => priorityMap.value[props.task.priority]?.label ?? '');

// 优先显示 branchName，如果没有再尝试从 worktree 中获取
const displayBranchName = computed(() => {
  return props.task.branchName || props.task.worktree?.branchName || '';
});

const isOverdue = computed(() => {
  if (!props.task.dueDate) {
    return false;
  }
  return dayjs(props.task.dueDate).isBefore(dayjs(), 'day');
});

const formatDate = (value: string) => dayjs(value).format('MM-DD');

const handleEdit = () => emit('edit');
const handleDelete = () => emit('delete');
const handleCopy = () => emit('copy');
const handleStartWork = () => emit('start-work');
</script>

<style scoped>
.task-card {
  cursor: pointer;
  transition: all 0.2s ease;
}

.task-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 16px var(--n-box-shadow-color, rgba(0, 0, 0, 0.12));
}

.task-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

/* 当空间较窄时，标题和按钮分成两行 */
@media (max-width: 1200px) {
  .task-card__header {
    flex-direction: column;
    align-items: stretch;
  }

  .task-card__actions {
    justify-content: flex-end;
  }
}

.task-card__title {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
}

.task-card__actions {
  display: flex;
  align-items: center;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.2s ease;
  pointer-events: none;
  flex-shrink: 0;
}

.task-card:hover .task-card__actions {
  opacity: 1;
  pointer-events: auto;
}

.task-card__meta {
  margin-bottom: 8px;
}
</style>
