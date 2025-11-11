<template>
  <div class="kanban-column">
    <div class="column-header">
      <h3>{{ title }}</h3>
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
          <TaskCard :task="element" @click="emit('task-clicked', element)" />
        </template>
      </draggable>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import draggable from 'vuedraggable';
import TaskCard from './TaskCard.vue';
import type { Task } from '@/types/models';

const props = defineProps<{
  title: string;
  status: Task['status'];
  tasks: Task[];
}>();

const emit = defineEmits<{
  'task-moved': [{ taskId: string; newStatus: Task['status']; newIndex: number; orderedTasks: Task[] }];
  'task-clicked': [Task];
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
  height: 100%;
  background-color: #f5f5f5;
  border-radius: 8px;
  border: 1px solid #e0e0e0;
}

.column-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--n-border-color);
}

.column-header h3 {
  margin: 0;
  font-size: 16px;
}

.column-body {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.task-list {
  min-height: 80px;
}
</style>
