<template>
  <div class="recent-projects">
    <div class="recent-projects-header">
      <n-space justify="space-between" align="center" style="width: 100%">
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
        <n-space>
          <n-button text :disabled="!currentProject" @click="emit('editCurrent')">
            <template #icon>
              <n-icon size="20">
                <CreateOutline />
              </n-icon>
            </template>
            编辑
          </n-button>
          <n-button text @click="handleGoToSettings">
            <template #icon>
              <n-icon size="20">
                <SettingsOutline />
              </n-icon>
            </template>
            设置
          </n-button>
        </n-space>
      </n-space>
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
            <div class="project-name-row">
              <n-tag
                v-if="terminalCounts.get(project.id) && terminalCounts.get(project.id)! > 0"
                size="small"
                type="success"
                :bordered="false"
                class="terminal-tag"
                :class="{ clickable: project.id === currentProjectId }"
                @click.stop="handleTerminalTagClick(project.id)"
              >
                <template #icon>
                  <n-icon size="14"><TerminalOutline /></n-icon>
                </template>
                {{ terminalCounts.get(project.id) }}
              </n-tag>
              <n-text class="project-name" strong>{{ project.name }}</n-text>
            </div>
            <n-text v-if="!project.hidePath" class="project-path" depth="3">
              {{ project.path }}
            </n-text>
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
    <div class="version-info-container">
      <a
        class="version-info"
        href="https://github.com/fy0/CodeKanban"
        target="_blank"
        rel="noopener noreferrer"
      >
        <img src="/favicon.svg" alt="CodeKanban" class="app-logo" />
        <n-text strong style="font-size: 13px">{{ appStore.appInfo.name }}</n-text>
        <n-text depth="3" style="font-size: 11px">v{{ appStore.appInfo.version }}</n-text>
      </a>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useProjectStore } from '@/stores/project';
import { useTerminalStore } from '@/stores/terminal';
import { useAppStore } from '@/stores/app';
import { CreateOutline, SettingsOutline, TerminalOutline } from '@vicons/ionicons5';

const emit = defineEmits<{ editCurrent: []; toggleTerminal: [] }>();
const props = defineProps<{
  currentProjectId: string;
}>();

const router = useRouter();
const projectStore = useProjectStore();
const terminalStore = useTerminalStore();
const appStore = useAppStore();

const loading = computed(() => projectStore.loading);
const currentProject = computed(() => projectStore.currentProject);
const recentProjects = computed(() => projectStore.recentProjects);
const terminalCounts = terminalStore.terminalCounts;

const handleSelectProject = (projectId: string) => {
  if (projectId !== props.currentProjectId) {
    router.push({ name: 'project', params: { id: projectId } });
  }
};

const handleTerminalTagClick = (projectId: string) => {
  if (projectId === props.currentProjectId) {
    // 只有当前激活的项目的终端图标才能切换终端面板
    emit('toggleTerminal');
  }
};

const handleBackToList = () => {
  router.push({ name: 'projects' });
};

const handleGoToSettings = () => {
  router.push({ name: 'settings' });
};

onMounted(() => {
  if (projectStore.projects.length === 0) {
    projectStore.fetchProjects();
  }
  terminalStore.loadTerminalCounts();
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

.project-name-row {
  min-width: 0;
  overflow: hidden;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: nowrap;
}

.project-name {
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}

.project-path {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.terminal-tag {
  flex-shrink: 0;
  font-size: 12px;
  line-height: 1;
  transition: opacity 0.2s, transform 0.2s;
}

.terminal-tag.clickable {
  cursor: pointer;
}

.terminal-tag.clickable:hover {
  opacity: 0.8;
  transform: scale(1.05);
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

.version-info-container {
  padding: 12px 16px;
  border-top: 1px solid var(--n-border-color);
  background-color: var(--n-color-target);
  display: flex;
  align-items: center;
}

.version-info {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  text-decoration: none;
  color: inherit;
  transition: background-color 0.2s;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 4px;
  margin: -4px -8px;
}

.version-info:hover {
  background-color: var(--n-item-color-hover);
}

.version-info :deep(.n-text) {
  line-height: 1;
  display: flex;
  align-items: center;
}

.app-logo {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}
</style>
