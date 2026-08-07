<template>
  <div class="kanban-column" :class="{ 'is-mobile': isMobile }">
    <div class="column-header">
      <div class="column-header__title">
        <h3>{{ title }}</h3>
        <n-button
          v-if="showAddButton"
          circle
          size="tiny"
          quaternary
          :disabled="addDisabled"
          @click="emit('add-click')"
        >
          <n-icon size="16">
            <AddOutline />
          </n-icon>
        </n-button>
        <n-tooltip v-if="status === 'done'" trigger="hover" placement="bottom">
          <template #trigger>
            <n-button
              circle
              size="tiny"
              quaternary
              :disabled="tasks.length === 0"
              @click="emit('archive-click')"
            >
              <n-icon size="16">
                <ArchiveOutline />
              </n-icon>
            </n-button>
          </template>
          {{ t('task.status.archived') }}
        </n-tooltip>
      </div>
      <n-badge :value="tasks.length" :max="99" />
    </div>

    <div class="column-body">
      <draggable
        v-model="localTasks"
        class="task-list"
        item-key="id"
        :animation="200"
        :group="{ name: 'kanban-tasks', pull: true, put: true }"
        :delay="isMobile ? 200 : 0"
        :delay-on-touch-only="true"
        :touch-start-threshold="10"
        @change="handleChange"
      >
        <template #item="{ element }">
          <TaskCard
            :task="element"
            :linked-terminal="linkedTerminals ? linkedTerminals[element.id] : undefined"
            @click="emit('task-clicked', element)"
            @edit="emit('task-edit', element)"
            @delete="emit('task-delete', element)"
            @copy="emit('task-copy', element)"
            @start-work="action => emit('task-start-work', element, action)"
            @view-terminal="emit('view-terminal', element)"
          />
        </template>
      </draggable>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { AddOutline, ArchiveOutline } from '@vicons/ionicons5';
import draggable from 'vuedraggable';
import TaskCard from './TaskCard.vue';
import type { Task } from '@/types/models';
import type { StartWorkAction } from '@/types/startWork';
import { useLocale } from '@/composables/useLocale';

type LinkedTerminalSummary = {
  sessionId: string;
  agentName: string;
  sessionTitle: string;
};

const props = defineProps<{
  title: string;
  status: Task['status'];
  tasks: Task[];
  showAddButton?: boolean;
  addDisabled?: boolean;
  linkedTerminals?: Record<string, LinkedTerminalSummary>;
  isMobile?: boolean;
}>();

const emit = defineEmits<{
  'task-moved': [
    { taskId: string; newStatus: Task['status']; newIndex: number; orderedTasks: Task[] },
  ];
  'task-clicked': [Task];
  'task-edit': [Task];
  'task-delete': [Task];
  'task-copy': [Task];
  'task-start-work': [Task, StartWorkAction];
  'view-terminal': [Task];
  'add-click': [];
  'archive-click': [];
}>();

const localTasks = ref<Task[]>([]);
const { t } = useLocale();

watch(
  () => props.tasks,
  value => {
    localTasks.value = [...value];
  },
  { immediate: true, deep: true }
);

function handleChange(event: any) {
  if (event?.added) {
    emit('task-moved', {
      taskId: event.added.element.id,
      newStatus: props.status,
      newIndex: event.added.newIndex,
      orderedTasks: [...localTasks.value],
    });
    return;
  }
  if (event?.moved) {
    emit('task-moved', {
      taskId: event.moved.element.id,
      newStatus: props.status,
      newIndex: event.moved.newIndex,
      orderedTasks: [...localTasks.value],
    });
  }
}
</script>

<style scoped>
.kanban-column {
  display: flex;
  flex-direction: column;
  background-color: var(--kanban-board-bg, var(--app-body-color, #f5f5f5));
  border-radius: 8px;
  border: 1px solid var(--n-border-color, #e0e0e0);
  height: 100%;
  overflow: hidden;
}

.column-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: var(--kanban-border, 1px solid var(--n-border-color));
  flex-shrink: 0;
}

.column-header__title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.column-header h3 {
  margin: 0;
  font-size: 16px;
}

.column-body {
  flex: 1;
  padding: 12px;
  overflow-y: auto;
  overflow-x: hidden;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
  min-height: 100%;
}

/* 移动端样式 */
.kanban-column.is-mobile {
  border: none;
  border-radius: 0;
  background-color: transparent;
}

.kanban-column.is-mobile .column-header {
  display: none; /* 移动端隐藏列标题，因为标签已显示 */
}

.kanban-column.is-mobile .column-body {
  padding: 8px 0;
}

.kanban-column.is-mobile .task-list {
  gap: 8px;
}
</style>
