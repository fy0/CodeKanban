<template>
  <div class="recent-projects">
    <div class="recent-projects-header">
      <n-button text @click="handleBackToList">
        <template #icon>
          <n-icon size="20">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
              <path
                fill="currentColor"
                d="M20 11H7.83l5.59-5.59L12 4l-8 8l8 8l1.41-1.41L7.83 13H20v-2z"
              />
            </svg>
          </n-icon>
        </template>
        返回列表
      </n-button>
    </div>
    <div v-if="recentProjects.length === 0" class="empty-state">
      <n-text depth="3">{{ loading ? '加载中...' : '暂无最近项目' }}</n-text>
    </div>
    <div v-else class="projects-list">
      <TransitionGroup name="project-list" tag="div">
        <div
          v-for="project in recentProjects"
          :key="project.id"
          class="project-item"
          :class="{ active: project.id === currentProjectId }"
          @click="handleSelectProject(project.id)"
        >
          <div class="project-info">
            <n-text class="project-name" strong>{{ project.name }}</n-text>
            <n-text class="project-path" depth="3">{{ project.path }}</n-text>
          </div>
          <n-icon v-if="project.id === currentProjectId" size="18" color="#18a058">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
              <path
                fill="currentColor"
                d="M9 16.17L4.83 12l-1.42 1.41L9 19L21 7l-1.41-1.41L9 16.17z"
              />
            </svg>
          </n-icon>
        </div>
      </TransitionGroup>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useProjectStore } from '@/stores/project';

const props = defineProps<{
  currentProjectId: string;
}>();

const router = useRouter();
const projectStore = useProjectStore();

const loading = computed(() => projectStore.loading);
const recentProjects = computed(() => projectStore.recentProjects);

const handleSelectProject = (projectId: string) => {
  if (projectId !== props.currentProjectId) {
    router.push({ name: 'project', params: { id: projectId } });
  }
};

const handleBackToList = () => {
  router.push({ name: 'projects' });
};

onMounted(() => {
  if (projectStore.projects.length === 0) {
    projectStore.fetchProjects();
  }
});
</script>

<style scoped>
.recent-projects {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--n-color);
}

.recent-projects-header {
  padding: 16px;
  border-bottom: 1px solid var(--n-border-color);
}

.empty-state {
  padding: 32px 16px;
  text-align: center;
}

.projects-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
}

.project-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  cursor: pointer;
  transition: background-color 0.2s;
  border-left: 3px solid transparent;
}

.project-item:hover {
  background-color: var(--n-item-color-hover);
}

.project-item.active {
  background-color: var(--n-item-color-active);
  border-left-color: var(--n-primary-color);
}

.project-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.project-name {
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project-path {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 过渡动画 */
.project-list-move,
.project-list-enter-active,
.project-list-leave-active {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.project-list-enter-from {
  opacity: 0;
  transform: translateX(-20px);
}

.project-list-leave-to {
  opacity: 0;
  transform: translateX(20px);
}

.project-list-leave-active {
  position: absolute;
  width: 100%;
}
</style>
