<template>
  <div class="kanban-column">
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
        @change="handleChange"
      >
        <template #item="{ element }">
          <TaskCard
            :task="element"
            @click="emit('task-clicked', element)"
            @edit="emit('task-edit', element)"
            @delete="emit('task-delete', element)"
            @copy="emit('task-copy', element)"
            @start-work="emit('task-start-work', element)"
          />
        </template>
      </draggable>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { AddOutline } from '@vicons/ionicons5';
import draggable from 'vuedraggable';
import TaskCard from './TaskCard.vue';
import type { Task } from '@/types/models';

const props = defineProps<{
  title: string;
  status: Task['status'];
  tasks: Task[];
  showAddButton?: boolean;
  addDisabled?: boolean;
}>();

const emit = defineEmits<{
  'task-moved': [{ taskId: string; newStatus: Task['status']; newIndex: number; orderedTasks: Task[] }];
  'task-clicked': [Task];
  'task-edit': [Task];
  'task-delete': [Task];
  'task-copy': [Task];
  'task-start-work': [Task];
  'add-click': [];
}>();

const localTasks = ref<Task[]>([]);

watch(
  () => props.tasks,
  value => {
    localTasks.value = [...value];
  },
  { immediate: true, deep: true },
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
  background-color: var(--app-surface-color, var(--n-color, #f5f5f5));
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
  border-bottom: 1px solid var(--n-border-color);
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
</style>
