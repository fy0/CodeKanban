<template>
  <div class="project-workspace">
    <n-layout has-sider>
      <!-- 左侧最近项目侧边栏 -->
      <n-layout-sider bordered :width="240" :min-width="200" :max-width="400" resizable>
        <RecentProjects
          :current-project-id="currentProjectId"
          @edit-current="openProjectEditDialog"
          @toggle-terminal="toggleTerminalPanel"
        />
      </n-layout-sider>

      <n-layout has-sider>
        <!-- 右侧工作树侧边栏 -->
        <n-layout-sider bordered :width="320" :collapsed-width="0" show-trigger="arrow-circle">
          <WorktreeList @open-terminal="handleOpenTerminal" />
        </n-layout-sider>

        <n-layout-content>
          <!-- 主内容区 -->
          <div class="workspace-content">
            <KanbanBoard :project-id="currentProjectId" />
          </div>
        </n-layout-content>
      </n-layout>
    </n-layout>

    <!-- 悬浮终端面板 -->
    <TerminalPanel ref="terminalPanelRef" :project-id="currentProjectId" />
    <ProjectEditDialog
      v-model:show="showEditDialog"
      :project="projectStore.currentProject"
      @success="handleProjectUpdated"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, provide, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useTitle } from '@vueuse/core';
import { useProjectStore } from '@/stores/project';
import WorktreeList from '@/components/worktree/WorktreeList.vue';
import KanbanBoard from '@/components/kanban/KanbanBoard.vue';
import RecentProjects from '@/components/project/RecentProjects.vue';
import TerminalPanel from '@/components/terminal/TerminalPanel.vue';
import ProjectEditDialog from '@/components/project/ProjectEditDialog.vue';
import type { Worktree } from '@/types/models';
import { APP_NAME } from '@/constants/app';

const route = useRoute();
const projectStore = useProjectStore();
const terminalPanelRef = ref<InstanceType<typeof TerminalPanel> | null>(null);
const showEditDialog = ref(false);

// 提供终端面板引用给子组件
provide('terminalPanelRef', terminalPanelRef);

const currentProjectId = computed(() =>
  typeof route.params.id === 'string' ? route.params.id : ''
);

const pageTitle = computed(() => {
  const projectName = projectStore.currentProject?.name;
  return projectName ? `${projectName} - ${APP_NAME}` : APP_NAME;
});

useTitle(pageTitle);

const loadProject = (id: string) => {
  if (!id) {
    return;
  }
  projectStore.fetchProject(id);
  projectStore.addRecentProject(id);
};

onMounted(() => {
  if (currentProjectId.value) {
    loadProject(currentProjectId.value);
  }
});

watch(
  () => route.params.id,
  newId => {
    if (typeof newId === 'string') {
      loadProject(newId);
    }
  }
);

// 监听路由变化，当从分支管理等页面返回到项目工作区时刷新 worktrees
watch(
  () => route.name,
  (newName, oldName) => {
    // 当从分支管理页面返回到项目工作区时，重新加载 worktrees
    // 这样可以确保在分支管理页面创建的新 worktree 能够立即显示
    if (newName === 'project' && oldName === 'project-branches' && currentProjectId.value) {
      projectStore.fetchWorktrees(currentProjectId.value);
    }
  }
);

watch(
  currentProjectId,
  newId => {
    if (!newId) {
      return;
    }
    nextTick(() => {
      void terminalPanelRef.value?.reloadSessions();
    });
  },
  { immediate: true }
);

function handleOpenTerminal(worktree: Worktree) {
  terminalPanelRef.value?.createTerminal({
    worktreeId: worktree.id,
    workingDir: worktree.path,
    title: worktree.branchName,
  });
}

function openProjectEditDialog() {
  showEditDialog.value = true;
}

async function handleProjectUpdated() {
  if (currentProjectId.value) {
    await projectStore.fetchProject(currentProjectId.value);
  }
}

function toggleTerminalPanel() {
  terminalPanelRef.value?.toggleExpanded();
}
</script>

<style scoped>
.project-workspace {
  height: 100vh;
}

.workspace-content {
  padding: 24px;
  height: 100vh;
  overflow-y: auto;
  background-color: var(--app-surface-color, #ffffff);
}
</style>
