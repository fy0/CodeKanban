<template>
  <div class="project-workspace">
    <n-layout has-sider>
      <!-- 左侧最近项目侧边栏 -->
      <n-layout-sider bordered :width="240" :min-width="200" :max-width="400" resizable>
        <RecentProjects :current-project-id="currentProjectId" />
      </n-layout-sider>

      <n-layout has-sider>
        <!-- 右侧工作树侧边栏 -->
        <n-layout-sider
          bordered
          :width="320"
          :collapsed-width="0"
          show-trigger="arrow-circle"
        >
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useTitle } from '@vueuse/core';
import { useProjectStore } from '@/stores/project';
import WorktreeList from '@/components/worktree/WorktreeList.vue';
import KanbanBoard from '@/components/kanban/KanbanBoard.vue';
import RecentProjects from '@/components/project/RecentProjects.vue';
import TerminalPanel from '@/components/terminal/TerminalPanel.vue';
import type { Worktree } from '@/types/models';
import { APP_NAME } from '@/constants/app';

const route = useRoute();
const projectStore = useProjectStore();
const terminalPanelRef = ref<InstanceType<typeof TerminalPanel> | null>(null);

const currentProjectId = computed(() => (typeof route.params.id === 'string' ? route.params.id : ''));

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
  },
);

function handleOpenTerminal(worktree: Worktree) {
  terminalPanelRef.value?.createTerminal({
    worktreeId: worktree.id,
    workingDir: worktree.path,
    title: worktree.branchName,
  });
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
}
</style>
