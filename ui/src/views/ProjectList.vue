<template>
  <div class="project-list-page">
    <n-page-header>
      <template #title>
        <div class="title-wrapper">
          <n-icon size="24">
            <FolderOpenOutline />
          </n-icon>
          <span
            class="app-name-link"
            @click="handleAppNameClick"
          >
            {{ appStore.appInfo.name }}
          </span>
          <n-popover
            v-if="updateInfo?.hasUpdate"
            trigger="hover"
            placement="bottom"
          >
            <template #trigger>
              <n-tag
                size="small"
                type="warning"
                :bordered="false"
                style="cursor: pointer"
                @click="showUpdateModal = true"
              >
                v{{ appStore.appInfo.version }}
                <template #icon>
                  <n-icon :component="ArrowUpCircleOutline" />
                </template>
              </n-tag>
            </template>
            <div
              style="max-width: 280px; cursor: pointer"
              @click="showUpdateModal = true"
            >
              <div style="font-weight: 500; margin-bottom: 8px">{{ t('update.newVersionAvailable') }}</div>
              <div style="font-size: 13px; margin-bottom: 4px">
                {{ t('update.latestVersion') }}: <n-tag size="tiny" type="success">{{ updateInfo.latestVersion }}</n-tag>
              </div>
              <div style="font-size: 12px; color: var(--n-text-color-3)">
                {{ t('update.clickToView') }}
              </div>
            </div>
          </n-popover>
          <n-tag v-else size="small" type="info" :bordered="false">
            v{{ appStore.appInfo.version }}
          </n-tag>
        </div>
      </template>
      <template #extra>
        <n-space align="center">
          <LanguageSwitcher />
          <ThemeSwitcher />
          <n-button quaternary size="small" @click="goToSettings">
            <template #icon>
              <n-icon><SettingsOutline /></n-icon>
            </template>
            {{ t('nav.settings') }}
          </n-button>
          <n-button quaternary size="small" @click="goToGuide">
            <template #icon>
              <n-icon><BookOutline /></n-icon>
            </template>
            {{ t('nav.guide') }}
          </n-button>
          <n-button type="primary" size="small" @click="showCreateDialog = true">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            {{ t('project.addProject') }}
          </n-button>
        </n-space>
      </template>
    </n-page-header>

    <!-- 搜索和排序工具栏 -->
    <div class="search-toolbar">
      <n-input
        v-model:value="searchQuery"
        :placeholder="t('project.searchPlaceholder')"
        clearable
        style="max-width: 400px; flex: 1; min-width: 200px"
      >
        <template #prefix>
          <n-icon><SearchOutline /></n-icon>
        </template>
      </n-input>
      <n-space align="center" :wrap="false">
        <n-select
          v-model:value="sortType"
          :options="sortTypeOptions"
          :placeholder="t('project.sortBy')"
          style="width: 150px"
        />
        <n-tooltip :disabled="!sortType">
          <template #trigger>
            <n-button
              quaternary
              circle
              :disabled="!sortType"
              @click="toggleSortOrder"
            >
              <template #icon>
                <n-icon size="20">
                  <ArrowDownOutline v-if="sortOrder === 'desc'" />
                  <ArrowUpOutline v-else />
                </n-icon>
              </template>
            </n-button>
          </template>
          {{ sortOrder === 'desc' ? t('project.descending') : t('project.ascending') }}
        </n-tooltip>
        <n-popover trigger="hover" placement="bottom">
          <template #trigger>
            <n-checkbox v-model:checked="respectPriority" />
          </template>
          <div style="max-width: 300px">
            <div style="font-weight: 500; margin-bottom: 4px">{{ t('project.respectPriority') }}</div>
            <div style="font-size: 13px; color: var(--n-text-color-2)">{{ t('project.respectPriorityHint') }}</div>
          </div>
        </n-popover>
      </n-space>
    </div>

    <n-spin :show="projectStore.loading">
      <transition-group
        v-if="filteredAndSortedProjects.length > 0"
        name="project-list"
        tag="div"
        class="project-grid"
      >
        <n-card
          v-for="project in filteredAndSortedProjects"
          :key="project.id"
          hoverable
          class="project-card"
          :class="{ 'has-notifications': hasProjectNotifications(project.id) }"
          @click="goToProject(project.id)"
        >
          <template #header>
            <n-space justify="space-between" align="center">
              <n-ellipsis style="max-width: 240px">
                <span v-html="highlightText(project.name)"></span>
              </n-ellipsis>
              <n-dropdown :options="getCardActions(project)" @select="onCardSelect">
                <n-button text @click.stop>
                  <n-icon size="20"><EllipsisHorizontalOutline /></n-icon>
                </n-button>
              </n-dropdown>
            </n-space>
          </template>

          <n-space vertical size="small">
            <n-text v-if="!project.hidePath" depth="3">
              <n-icon size="16"><FolderOutline /></n-icon>
              <span class="path-text" v-html="highlightText(project.path)"></span>
            </n-text>
            <n-text v-if="project.description" depth="3">
              <span v-html="highlightText(project.description)"></span>
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
              <n-tag
                v-if="project.priority"
                size="small"
                :bordered="false"
                :color="{ color: getPriorityTagColor(project.priority), textColor: '#fff' }"
              >
                <template #icon>
                  <n-icon size="16">
                    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                      <path
                        fill="currentColor"
                        d="M16,12V4H17V2H7V4H8V12L6,14V16H11.2V22H12.8V16H18V14L16,12Z"
                      />
                    </svg>
                  </n-icon>
                </template>
                {{ getPriorityLabel(project.priority) }}
              </n-tag>
            </n-space>
          </n-space>
        </n-card>
      </transition-group>
      <div v-else class="empty-container">
        <n-empty
          :description="searchQuery ? t('common.noData') : t('project.noProjects')"
        />
      </div>
    </n-spin>

    <ProjectCreateDialog v-model:show="showCreateDialog" @success="handleProjectCreated" />
    <ProjectEditDialog
      v-model:show="showEditDialog"
      :project="editingProject"
      @success="handleProjectUpdated"
    />

    <!-- 更新提示模态框 -->
    <n-modal v-model:show="showUpdateModal" preset="card" style="width: 420px" :title="t('update.newVersionAvailable')">
      <div style="margin-bottom: 16px">
        <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 12px">
          <span style="color: var(--n-text-color-3)">{{ t('update.currentVersion') }}:</span>
          <n-tag :bordered="false" size="small">{{ updateInfo?.currentVersion }}</n-tag>
        </div>
        <div style="display: flex; align-items: center; gap: 12px">
          <span style="color: var(--n-text-color-3)">{{ t('update.latestVersion') }}:</span>
          <n-tag type="success" :bordered="false" size="small">{{ updateInfo?.latestVersion }}</n-tag>
        </div>
      </div>

      <n-alert type="info" :bordered="false" style="margin-bottom: 16px">
        <code style="user-select: all">npm install -g codekanban@latest</code>
      </n-alert>

      <template #footer>
        <n-space justify="end">
          <n-button @click="openUpdateUrl">{{ t('update.viewDetails') }}</n-button>
          <n-button type="primary" @click="copyUpdateCommand">{{ t('update.copyCommand') }}</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from 'vue';
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
  SearchOutline,
  ArrowDownOutline,
  ArrowUpOutline,
  ArrowUpCircleOutline,
} from '@vicons/ionicons5';
import ProjectCreateDialog from '@/components/project/ProjectCreateDialog.vue';
import ProjectEditDialog from '@/components/project/ProjectEditDialog.vue';
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue';
import ThemeSwitcher from '@/components/common/ThemeSwitcher.vue';
import { useProjectStore } from '@/stores/project';
import { useTerminalStore } from '@/stores/terminal';
import { useAppStore } from '@/stores/app';
import { useLocale } from '@/composables/useLocale';
import type { Project } from '@/types/models';
import Apis from '@/api';
import { useReq } from '@/api';
import type { ProjectPriority } from '@/stores/project';

const appStore = useAppStore();
const { t } = useLocale();

useTitle(`${t('project.title')} - ${appStore.appInfo.name}`);

const router = useRouter();
const projectStore = useProjectStore();
const terminalStore = useTerminalStore();
const message = useMessage();
const dialog = useDialog();
const showCreateDialog = ref(false);
const showEditDialog = ref(false);
const editingProject = ref<Project | null>(null);
const showUpdateModal = ref(false);

// 更新检查
interface UpdateInfo {
  currentVersion: string;
  latestVersion: string;
  hasUpdate: boolean;
  updateUrl?: string;
  message?: string;
}
const updateInfo = ref<UpdateInfo | null>(null);

const { send: checkUpdate } = useReq(() => Apis.system.checkUpdate({}));

const checkForUpdates = async () => {
  try {
    const result = await checkUpdate();
    if (result) {
      updateInfo.value = result;
    }
  } catch (error) {
    console.error('Failed to check for updates:', error);
  }
};

const copyUpdateCommand = () => {
  const command = 'npm install -g codekanban@latest';
  navigator.clipboard.writeText(command).then(() => {
    message.success(t('update.commandCopied'));
    showUpdateModal.value = false;
  });
};

const openUpdateUrl = () => {
  if (updateInfo.value?.updateUrl) {
    window.open(updateInfo.value.updateUrl, '_blank');
  }
};

const handleAppNameClick = () => {
  dialog.info({
    title: t('nav.visitProjectConfirm'),
    content: t('nav.visitProjectMessage'),
    positiveText: t('nav.visitNow'),
    negativeText: t('common.cancel'),
    onPositiveClick: () => {
      window.open('https://github.com/fy0/CodeKanban', '_blank', 'noopener,noreferrer');
    },
  });
};

const terminalCounts = terminalStore.terminalCounts;

// Track projects with unviewed notifications (completion or approval needed)
const projectNotifications = ref<Map<string, number>>(new Map());

// Handle AI completion notification
function handleAICompletionForProject(event: any) {
  const { projectId } = event;
  if (projectId) {
    const currentCount = projectNotifications.value.get(projectId) || 0;
    projectNotifications.value.set(projectId, currentCount + 1);
  }
}

// Handle AI approval needed notification
function handleAIApprovalForProject(event: any) {
  const { projectId } = event;
  if (projectId) {
    const currentCount = projectNotifications.value.get(projectId) || 0;
    projectNotifications.value.set(projectId, currentCount + 1);
  }
}

// Clear notifications for a project
function clearProjectNotifications(projectId: string) {
  projectNotifications.value.delete(projectId);
}

// Check if a project has notifications
function hasProjectNotifications(projectId: string): boolean {
  return (projectNotifications.value.get(projectId) || 0) > 0;
}

// 使用 useReq 定义优先级更新请求
const { send: updatePriority, loading: priorityLoading } = useReq(
  (projectId: string, priority: number | null) => Apis.project.updatePriority({
    pathParams: { id: projectId },
    data: { priority }
  })
);

// 搜索和排序
const searchQuery = ref('');
const sortType = ref<'name' | 'created' | 'updated' | 'accessed'>('accessed'); // 默认按访问时间
const sortOrder = ref<'asc' | 'desc'>('desc'); // 默认降序
const respectPriority = ref(true); // 默认尊重优先级

type SortType = 'name' | 'created' | 'updated' | 'accessed';

const sortTypeOptions = computed(() => [
  { label: t('project.sortByAccessed'), value: 'accessed' },
  { label: t('project.sortByName'), value: 'name' },
  { label: t('project.sortByCreated'), value: 'created' },
  { label: t('project.sortByUpdated'), value: 'updated' },
]);

// 切换排序顺序
function toggleSortOrder() {
  sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc';
}

// 高亮搜索关键字
function highlightText(text: string | null | undefined): string {
  if (!text) return '';
  if (!searchQuery.value) return text;

  const query = searchQuery.value.trim();
  if (!query) return text;

  // 转义正则表达式特殊字符
  const escapedQuery = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const regex = new RegExp(`(${escapedQuery})`, 'gi');

  return text.replace(regex, '<mark class="search-highlight">$1</mark>');
}

// 获取项目访问顺序索引（用于访问时间排序）
function getAccessIndex(projectId: string): number {
  const recentIds = projectStore.recentProjects.map(p => p.id);
  const index = recentIds.indexOf(projectId);
  // 如果项目在最近访问列表中，返回其索引
  // 如果不在列表中，返回一个很大的数（表示从未访问或很久未访问）
  return index >= 0 ? index : Number.MAX_SAFE_INTEGER;
}

// 过滤和排序后的项目列表
const filteredAndSortedProjects = computed(() => {
  let projects = [...projectStore.projects];

  // 过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase();
    projects = projects.filter(project => {
      const nameMatch = project.name.toLowerCase().includes(query);
      const pathMatch = project.path.toLowerCase().includes(query);
      const descMatch = project.description?.toLowerCase().includes(query);
      return nameMatch || pathMatch || descMatch;
    });
  }

  // 排序
  projects.sort((a, b) => {
    // 如果尊重优先级，先按优先级排序
    if (respectPriority.value) {
      const priorityA = projectStore.getProjectPriority(a.id) ?? 0;
      const priorityB = projectStore.getProjectPriority(b.id) ?? 0;

      // 优先级高的排在前面（数字大的优先级高）
      if (priorityA !== priorityB) {
        return priorityB - priorityA;
      }
    }

    // 优先级相同时，按选定的排序方式排序
    if (sortType.value) {
      let comparison = 0;

      switch (sortType.value as SortType) {
        case 'name':
          comparison = a.name.localeCompare(b.name);
          break;
        case 'created':
          comparison = new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
          break;
        case 'updated':
          comparison = new Date(a.updatedAt).getTime() - new Date(b.updatedAt).getTime();
          break;
        case 'accessed':
          // 访问时间排序：索引越小表示越近访问，所以用 b - a（降序时最近的在前）
          comparison = getAccessIndex(b.id) - getAccessIndex(a.id);
          break;
      }

      return sortOrder.value === 'asc' ? comparison : -comparison;
    }

    return 0;
  });

  return projects;
});

onMounted(() => {
  projectStore.fetchProjects();
  terminalStore.loadTerminalCounts();
  // 延迟检查更新，避免阻塞页面加载
  setTimeout(checkForUpdates, 2000);

  // Listen for AI notifications
  terminalStore.emitter.on('ai:completed', handleAICompletionForProject);
  terminalStore.emitter.on('ai:approval-needed', handleAIApprovalForProject);
});

onUnmounted(() => {
  // Clean up event listeners
  terminalStore.emitter.off('ai:completed', handleAICompletionForProject);
  terminalStore.emitter.off('ai:approval-needed', handleAIApprovalForProject);
});

watch(showEditDialog, value => {
  if (!value) {
    editingProject.value = null;
  }
});

function goToProject(id: string) {
  // Clear notifications for this project when user navigates to it
  clearProjectNotifications(id);
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
  const isPinned = project.priority !== null && project.priority !== undefined;

  return [
    { label: t('project.openProject'), key: 'open', project } as ProjectOption,
    { label: t('common.edit'), key: 'edit', project } as ProjectOption,
    { type: 'divider', key: 'd1' } as any,
    {
      label: isPinned ? t('project.unpinProject') : t('project.pinProject'),
      key: 'toggle-pin',
      project
    } as ProjectOption,
    {
      label: t('project.setPriority'),
      key: 'priority',
      children: [
        { label: t('project.priority5'), key: 'priority-5', project } as ProjectOption,
        { label: t('project.priority4'), key: 'priority-4', project } as ProjectOption,
        { label: t('project.priority3'), key: 'priority-3', project } as ProjectOption,
        { label: t('project.priority2'), key: 'priority-2', project } as ProjectOption,
        { label: t('project.priority1'), key: 'priority-1', project } as ProjectOption,
      ]
    } as any,
    { type: 'divider', key: 'd2' } as any,
    { label: t('common.delete'), key: 'delete', project } as ProjectOption,
  ];
}

// 处理优先级更新的辅助函数
async function handleSetPriority(projectId: string, priority: number | null) {
  try {
    const result = await updatePriority(projectId, priority);
    // Apis 返回的结果包含 item 字段
    if (result?.item) {
      // 更新 Store 中的状态
      projectStore.updateProjectInList(result.item);
    }
  } catch (error) {
    console.error('Failed to update project priority:', error);
    message.error(t('message.operationFailed'));
  }
}

function handleAction(action: string, project: Project) {
  if (action === 'open') {
    goToProject(project.id);
  } else if (action === 'edit') {
    openEditDialog(project);
  } else if (action === 'delete') {
    confirmDelete(project);
  } else if (action === 'toggle-pin') {
    const isPinned = project.priority !== null && project.priority !== undefined;
    handleSetPriority(project.id, isPinned ? null : 5);
  } else if (action.startsWith('priority-')) {
    const priority = parseInt(action.split('-')[1]) as ProjectPriority;
    handleSetPriority(project.id, priority);
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
    title: t('project.deleteProject'),
    content: `${t('project.deleteConfirm')}: "${project.name}"?`,
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await projectStore.deleteProject(project.id);
        message.success(t('message.deleteSuccess'));
      } catch (error: any) {
        message.error(error?.message ?? t('message.deleteFailed'));
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

function getPriorityTagColor(priority: number): string {
  const colorMap: Record<number, string> = {
    5: '#e74c3c', // 红色 - 最高优先级
    4: '#ff9800', // 橙色
    3: '#ffc107', // 黄色
    2: '#4caf50', // 绿色
    1: '#2196f3', // 蓝色 - 最低优先级
  };
  return colorMap[priority] || '#999';
}

function getPriorityLabel(priority: number): string {
  return t('project.priorityLevel', { level: priority });
}
</script>

<style scoped>
.project-list-page {
  padding: 24px;
  max-width: 1400px;
  margin: 0 auto;
}

.search-toolbar {
  display: flex;
  gap: 16px;
  align-items: center;
  margin-top: 16px;
  flex-wrap: wrap;
}

/* 搜索关键字高亮 */
:deep(.search-highlight) {
  background-color: color-mix(in srgb, var(--n-primary-color, #3b69a9) 20%, transparent);
  color: var(--kanban-terminal-fg, var(--n-text-color-1, #1f1f1f));
  padding: 2px 4px;
  border-radius: 3px;
  font-weight: 500;
  transition: background-color 0.2s ease;
}

/* 项目卡片过渡动画 */
.project-list-move,
.project-list-enter-active,
.project-list-leave-active {
  transition: all 0.5s cubic-bezier(0.55, 0, 0.1, 1);
}

.project-list-enter-from {
  opacity: 0;
  transform: scale(0.8) translateY(30px);
}

.project-list-leave-to {
  opacity: 0;
  transform: scale(0.8) translateY(-30px);
}

.project-list-leave-active {
  position: absolute;
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
  cursor: pointer;
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
  transition: all 0.3s ease;
}

.project-card:hover {
  transform: translateY(-2px);
}

.project-card.has-notifications {
  background: linear-gradient(135deg, rgba(18, 183, 106, 0.08) 0%, rgba(18, 183, 106, 0.02) 100%);
  border-left: 3px solid #12b76a;
  animation: notificationPulse 2s ease-in-out infinite;
}

@keyframes notificationPulse {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(18, 183, 106, 0);
  }
  50% {
    box-shadow: 0 0 20px 0 rgba(18, 183, 106, 0.3);
  }
}

.path-text {
  margin-left: 8px;
}
</style>
