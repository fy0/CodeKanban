<template>
  <div class="project-list-page">
    <n-page-header>
      <template #title>
        <div class="title-wrapper">
          <n-icon size="24">
            <FolderOpenOutline />
          </n-icon>
          <a
            href="https://github.com/fy0/CodeKanban"
            target="_blank"
            rel="noopener noreferrer"
            class="app-name-link"
          >
            {{ appStore.appInfo.name }}
          </a>
          <n-tag size="small" type="info" :bordered="false">
            v{{ appStore.appInfo.version }}
          </n-tag>
        </div>
      </template>
      <template #extra>
        <n-space>
          <n-button quaternary @click="goToSettings">
            <template #icon>
              <n-icon><SettingsOutline /></n-icon>
            </template>
            总设置
          </n-button>
          <n-button quaternary @click="goToGuide">
            <template #icon>
              <n-icon><BookOutline /></n-icon>
            </template>
            使用指引
          </n-button>
          <n-button type="primary" @click="showCreateDialog = true">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            添加项目
          </n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-spin :show="projectStore.loading">
      <div v-if="projectStore.projects.length > 0" class="project-grid">
        <n-card
          v-for="project in projectStore.projects"
          :key="project.id"
          hoverable
          class="project-card"
          @click="goToProject(project.id)"
        >
          <template #header>
            <n-space justify="space-between" align="center">
              <n-ellipsis style="max-width: 240px">
                {{ project.name }}
              </n-ellipsis>
              <n-dropdown :options="getCardActions(project)" @select="onCardSelect">
                <n-button text>
                  <n-icon size="20"><EllipsisHorizontalOutline /></n-icon>
                </n-button>
              </n-dropdown>
            </n-space>
          </template>

          <n-space vertical size="small">
            <n-text v-if="!project.hidePath" depth="3">
              <n-icon size="16"><FolderOutline /></n-icon>
              <span class="path-text">
                {{ project.path }}
              </span>
            </n-text>
            <n-text v-if="project.description" depth="3">
              {{ project.description }}
            </n-text>
            <n-divider style="margin: 8px 0" />
            <n-space size="small">
              <n-tag size="small" :bordered="false">
                <template #icon>
                  <n-icon size="16"><GitBranchOutline /></n-icon>
                </template>
                {{ project.defaultBranch || 'main' }}
              </n-tag>
              <n-tag
                v-if="terminalCounts.get(project.id) && terminalCounts.get(project.id)! > 0"
                size="small"
                type="success"
                :bordered="false"
              >
                <template #icon>
                  <n-icon size="16"><TerminalOutline /></n-icon>
                </template>
                {{ terminalCounts.get(project.id) }}
              </n-tag>
            </n-space>
          </n-space>
        </n-card>
      </div>
      <div v-else class="empty-container">
        <n-empty description="还没有任何项目，点击右上角创建一个吧" />
      </div>
    </n-spin>

    <ProjectCreateDialog v-model:show="showCreateDialog" @success="handleProjectCreated" />
    <ProjectEditDialog
      v-model:show="showEditDialog"
      :project="editingProject"
      @success="handleProjectUpdated"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue';
import { useRouter } from 'vue-router';
import { useDialog, useMessage, type DropdownOption } from 'naive-ui';
import { useTitle } from '@vueuse/core';
import {
  AddOutline,
  EllipsisHorizontalOutline,
  FolderOpenOutline,
  FolderOutline,
  GitBranchOutline,
  SettingsOutline,
  BookOutline,
  TerminalOutline,
} from '@vicons/ionicons5';
import ProjectCreateDialog from '@/components/project/ProjectCreateDialog.vue';
import ProjectEditDialog from '@/components/project/ProjectEditDialog.vue';
import { useProjectStore } from '@/stores/project';
import { useTerminalStore } from '@/stores/terminal';
import { useAppStore } from '@/stores/app';
import type { Project } from '@/types/models';

const appStore = useAppStore();

useTitle(`项目列表 - ${appStore.appInfo.name}`);

const router = useRouter();
const projectStore = useProjectStore();
const terminalStore = useTerminalStore();
const message = useMessage();
const dialog = useDialog();
const showCreateDialog = ref(false);
const showEditDialog = ref(false);
const editingProject = ref<Project | null>(null);

const terminalCounts = terminalStore.terminalCounts;

onMounted(() => {
  projectStore.fetchProjects();
  terminalStore.loadTerminalCounts();
});

watch(showEditDialog, value => {
  if (!value) {
    editingProject.value = null;
  }
});

function goToProject(id: string) {
  router.push({ name: 'project', params: { id } });
}

function goToSettings() {
  router.push({ name: 'settings' });
}

function goToGuide() {
  router.push({ name: 'guide' });
}

type ProjectOption = DropdownOption & { project: Project };

function getCardActions(project: Project): ProjectOption[] {
  return [
    { label: '打开', key: 'open', project } as ProjectOption,
    { label: '编辑', key: 'edit', project } as ProjectOption,
    { label: '删除', key: 'delete', project } as ProjectOption,
  ];
}

function handleAction(action: string, project: Project) {
  if (action === 'open') {
    goToProject(project.id);
  } else if (action === 'edit') {
    openEditDialog(project);
  } else if (action === 'delete') {
    confirmDelete(project);
  }
}

function onCardSelect(key: string | number, option: DropdownOption) {
  const project = (option as ProjectOption).project;
  handleAction(String(key), project);
}

function openEditDialog(project: Project) {
  editingProject.value = project;
  showEditDialog.value = true;
}

function confirmDelete(project: Project) {
  dialog.warning({
    title: '确认删除',
    content: `确定要删除项目 "${project.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await projectStore.deleteProject(project.id);
        message.success('项目已删除');
      } catch (error: any) {
        message.error(error?.message ?? '删除失败');
      }
    },
  });
}

async function handleProjectCreated(project?: Project) {
  await projectStore.fetchProjects();
  if (project) {
    goToProject(project.id);
  }
}

async function handleProjectUpdated() {
  await projectStore.fetchProjects();
}
</script>

<style scoped>
.project-list-page {
  padding: 24px;
  max-width: 1400px;
  margin: 0 auto;
}

.title-wrapper {
  display: flex;
  align-items: center;
  gap: 8px;
}

.app-name-link {
  color: inherit;
  text-decoration: none;
  transition: color 0.2s;
}

.app-name-link:hover {
  color: var(--n-primary-color);
}

.project-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
  margin-top: 24px;
}

.empty-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  margin-top: 24px;
}

.project-card {
  cursor: pointer;
  transition: transform 0.2s ease;
}

.project-card:hover {
  transform: translateY(-2px);
}

.path-text {
  margin-left: 8px;
}
</style>
