<template>
  <div
    class="project-browser"
    :class="{
      'project-browser--page': isPageMode,
      'project-browser--mobile-workspace': isMobileWorkspaceMode,
    }"
  >
    <n-page-header v-if="isPageMode">
      <template #title>
        <div class="title-wrapper">
          <n-icon size="24">
            <FolderOpenOutline />
          </n-icon>
          <span class="app-name-link" @click="handleAppNameClick">
            {{ appStore.appInfo.name }}
          </span>
          <n-popover v-if="updateInfo?.hasUpdate" trigger="hover" placement="bottom">
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
            <div style="max-width: 280px; cursor: pointer" @click="showUpdateModal = true">
              <div style="font-weight: 500; margin-bottom: 8px">
                {{ t('update.newVersionAvailable') }}
              </div>
              <div style="font-size: 13px; margin-bottom: 4px">
                {{ t('update.latestVersion') }}:
                <n-tag size="tiny" type="success">{{ updateInfo.latestVersion }}</n-tag>
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

    <div v-else class="mobile-workspace-header">
      <div class="mobile-workspace-header-main">
        <div class="mobile-workspace-title">
          <n-icon size="22">
            <FolderOpenOutline />
          </n-icon>
          <span>{{ t('nav.projects') }}</span>
        </div>
        <n-button type="primary" size="small" @click="showCreateDialog = true">
          <template #icon>
            <n-icon><AddOutline /></n-icon>
          </template>
          {{ t('project.addProject') }}
        </n-button>
      </div>
      <div class="mobile-workspace-header-actions">
        <LanguageSwitcher />
        <ThemeSwitcher :quick-toggle-on-click="false" trigger="click" />
        <n-button quaternary size="small" @click="goToSettings">
          <template #icon>
            <n-icon><SettingsOutline /></n-icon>
          </template>
          {{ t('nav.settingsShort') }}
        </n-button>
        <n-tag
          v-if="updateInfo?.hasUpdate"
          size="small"
          type="warning"
          :bordered="false"
          class="mobile-version-tag"
          @click="showUpdateModal = true"
        >
          v{{ appStore.appInfo.version }}
          <template #icon>
            <n-icon :component="ArrowUpCircleOutline" />
          </template>
        </n-tag>
        <n-tag v-else size="small" type="info" :bordered="false" class="mobile-version-tag">
          v{{ appStore.appInfo.version }}
        </n-tag>
      </div>
    </div>

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
            <n-button quaternary circle :disabled="!sortType" @click="toggleSortOrder">
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
            <div style="font-weight: 500; margin-bottom: 4px">
              {{ t('project.respectPriority') }}
            </div>
            <div style="font-size: 13px; color: var(--n-text-color-2)">
              {{ t('project.respectPriorityHint') }}
            </div>
          </div>
        </n-popover>
      </n-space>
    </div>

    <n-spin :show="projectStore.projectsLoading">
      <transition-group
        v-if="filteredAndSortedProjects.length > 0"
        name="project-list"
        tag="div"
        class="project-grid"
      >
        <div
          v-for="project in filteredAndSortedProjects"
          :key="project.id"
          class="project-grid-item"
        >
          <n-card
            class="project-card"
            :class="getProjectCardClass(project.id)"
            :aria-current="isCurrentProject(project.id) ? 'page' : undefined"
            @click="void goToProject(project.id)"
          >
            <template #header>
              <div class="project-card-header">
                <n-ellipsis style="max-width: 240px">
                  <span v-html="highlightText(project.name)"></span>
                </n-ellipsis>
                <div class="project-card-header-actions">
                  <n-icon
                    v-if="isCurrentProject(project.id)"
                    size="18"
                    color="var(--app-success, #18a058)"
                    class="current-project-icon"
                  >
                    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                      <path
                        fill="currentColor"
                        d="M9 16.17L4.83 12l-1.42 1.41L9 19L21 7l-1.41-1.41L9 16.17z"
                      />
                    </svg>
                  </n-icon>
                  <n-dropdown :options="getCardActions(project)" @select="onCardSelect">
                    <n-button text @click.stop>
                      <n-icon size="20"><EllipsisHorizontalOutline /></n-icon>
                    </n-button>
                  </n-dropdown>
                </div>
              </div>
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
                  :title="`${t('project.terminalCount')}: ${terminalCounts.get(project.id)}`"
                >
                  <template #icon>
                    <n-icon size="16"><TerminalOutline /></n-icon>
                  </template>
                  {{ terminalCounts.get(project.id) }}
                </n-tag>
                <n-tag
                  v-if="webSessionCounts.get(project.id) && webSessionCounts.get(project.id)! > 0"
                  size="small"
                  type="info"
                  :bordered="false"
                  :title="`${t('project.webSessionCount')}: ${webSessionCounts.get(project.id)}`"
                >
                  <template #icon>
                    <n-icon size="16"><ChatbubblesOutline /></n-icon>
                  </template>
                  {{ webSessionCounts.get(project.id) }}
                </n-tag>
                <n-tag
                  v-if="project.priority"
                  size="small"
                  :bordered="false"
                  :color="{
                    color: getPriorityTagColor(project.priority),
                    textColor: getReadableTextColor(getPriorityTagColor(project.priority)),
                  }"
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
        </div>
      </transition-group>
      <div v-else class="empty-container">
        <n-empty :description="searchQuery ? t('common.noData') : t('project.noProjects')" />
      </div>
    </n-spin>

    <ProjectCreateDialog v-model:show="showCreateDialog" @success="handleProjectCreated" />
    <ProjectEditDialog
      v-model:show="showEditDialog"
      :project="editingProject"
      @success="handleProjectUpdated"
    />

    <n-modal
      v-model:show="showUpdateModal"
      preset="card"
      style="width: 420px"
      :title="t('update.newVersionAvailable')"
    >
      <div style="margin-bottom: 16px">
        <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 12px">
          <span style="color: var(--n-text-color-3)">{{ t('update.currentVersion') }}:</span>
          <n-tag :bordered="false" size="small">{{ updateInfo?.currentVersion }}</n-tag>
        </div>
        <div style="display: flex; align-items: center; gap: 12px">
          <span style="color: var(--n-text-color-3)">{{ t('update.latestVersion') }}:</span>
          <n-tag type="success" :bordered="false" size="small">{{
            updateInfo?.latestVersion
          }}</n-tag>
        </div>
      </div>

      <n-alert type="info" :bordered="false" style="margin-bottom: 16px">
        <code style="user-select: all">npm install -g codekanban@latest</code>
      </n-alert>

      <template #footer>
        <n-space justify="end">
          <n-button @click="openUpdateUrl">{{ t('update.viewDetails') }}</n-button>
          <n-button type="primary" @click="copyUpdateCommand">{{
            t('update.copyCommand')
          }}</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useDialog, useMessage, type DropdownOption } from 'naive-ui';
import {
  AddOutline,
  ArrowDownOutline,
  ArrowUpCircleOutline,
  ArrowUpOutline,
  BookOutline,
  ChatbubblesOutline,
  EllipsisHorizontalOutline,
  FolderOpenOutline,
  FolderOutline,
  GitBranchOutline,
  SearchOutline,
  SettingsOutline,
  TerminalOutline,
} from '@vicons/ionicons5';

import Apis from '@/api';
import { useReq } from '@/api';
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue';
import ThemeSwitcher from '@/components/common/ThemeSwitcher.vue';
import ProjectCreateDialog from '@/components/project/ProjectCreateDialog.vue';
import ProjectEditDialog from '@/components/project/ProjectEditDialog.vue';
import {
  buildProjectBrowserProjectLocation,
  type ProjectBrowserMode,
} from '@/components/project/projectBrowserNavigation';
import { useAppClipboard } from '@/composables/useAppClipboard';
import { useLocale } from '@/composables/useLocale';
import { useAppStore } from '@/stores/app';
import type { ProjectPriority } from '@/stores/project';
import { useProjectStore } from '@/stores/project';
import { useTerminalStore } from '@/stores/terminal';
import { useWebSessionStore } from '@/stores/webSession';
import type { Project } from '@/types/models';
import { getReadableTextColor } from '@/utils/color';

type ProjectOption = DropdownOption & { project?: Project };
type SortType = 'name' | 'created' | 'updated' | 'accessed';

interface UpdateInfo {
  currentVersion: string;
  latestVersion: string;
  hasUpdate: boolean;
  updateUrl?: string;
  message?: string;
}

const props = withDefaults(
  defineProps<{
    mode?: ProjectBrowserMode;
    currentProjectId?: string;
  }>(),
  {
    mode: 'page',
    currentProjectId: '',
  }
);

const emit = defineEmits<{
  (event: 'mobile-project-select', payload: { projectId: string }): void;
}>();

const router = useRouter();
const projectStore = useProjectStore();
const terminalStore = useTerminalStore();
const webSessionStore = useWebSessionStore();
const appStore = useAppStore();
const { t } = useLocale();
const message = useMessage();
const { copyText } = useAppClipboard();
const dialog = useDialog();

const isPageMode = computed(() => props.mode === 'page');
const isMobileWorkspaceMode = computed(() => props.mode === 'mobile-workspace');

const showCreateDialog = ref(false);
const showEditDialog = ref(false);
const editingProject = ref<Project | null>(null);
const showUpdateModal = ref(false);
const updateInfo = ref<UpdateInfo | null>(null);
const searchQuery = ref('');
const sortType = ref<SortType>('accessed');
const sortOrder = ref<'asc' | 'desc'>('desc');
const respectPriority = ref(true);

const terminalCounts = terminalStore.terminalCounts;
const webSessionCounts = webSessionStore.sessionCounts;

const sortTypeOptions = computed(() => [
  { label: t('project.sortByAccessed'), value: 'accessed' },
  { label: t('project.sortByName'), value: 'name' },
  { label: t('project.sortByCreated'), value: 'created' },
  { label: t('project.sortByUpdated'), value: 'updated' },
]);

const { send: checkUpdate } = useReq(() => Apis.system.checkUpdate({}));
const { send: updatePriority } = useReq((projectId: string, priority: number | null) =>
  Apis.project.updatePriority({
    pathParams: { id: projectId },
    data: { priority },
  })
);

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

const copyUpdateCommand = async () => {
  const command = 'npm install -g codekanban@latest';
  await copyText(command, {
    failureMessage: t('terminal.copyFailed'),
    successMessage: t('update.commandCopied'),
    onSuccess: () => {
      showUpdateModal.value = false;
    },
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

function toggleSortOrder() {
  sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc';
}

function highlightText(text: string | null | undefined): string {
  if (!text) return '';
  if (!searchQuery.value) return text;

  const query = searchQuery.value.trim();
  if (!query) return text;

  const escapedQuery = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const regex = new RegExp(`(${escapedQuery})`, 'gi');

  return text.replace(regex, '<mark class="search-highlight">$1</mark>');
}

function getProjectTimestamp(value: string | null | undefined): number {
  if (!value) {
    return 0;
  }
  const timestamp = new Date(value).getTime();
  return Number.isFinite(timestamp) ? timestamp : 0;
}

const filteredAndSortedProjects = computed(() => {
  let projects = [...projectStore.projects];

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase();
    projects = projects.filter(project => {
      const nameMatch = project.name.toLowerCase().includes(query);
      const pathMatch = project.path.toLowerCase().includes(query);
      const descMatch = project.description?.toLowerCase().includes(query);
      return nameMatch || pathMatch || descMatch;
    });
  }

  projects.sort((a, b) => {
    if (respectPriority.value) {
      const priorityA = projectStore.getProjectPriority(a.id) ?? 0;
      const priorityB = projectStore.getProjectPriority(b.id) ?? 0;
      if (priorityA !== priorityB) {
        return priorityB - priorityA;
      }
    }

    let comparison = 0;
    switch (sortType.value) {
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
      default:
        comparison = getProjectTimestamp(a.lastAccessedAt) - getProjectTimestamp(b.lastAccessedAt);
        break;
    }

    if (comparison === 0) {
      comparison = getProjectTimestamp(a.createdAt) - getProjectTimestamp(b.createdAt);
    }

    return sortOrder.value === 'asc' ? comparison : -comparison;
  });

  return projects;
});

function isCurrentProject(projectId: string): boolean {
  return isMobileWorkspaceMode.value && projectId === props.currentProjectId;
}

function getProjectCardClass(projectId: string) {
  return {
    'is-current': isCurrentProject(projectId),
  };
}

function runMobileWorkspaceProjectSwitch(projectId: string) {
  projectStore.addRecentProject(projectId);
  emit('mobile-project-select', { projectId });
}

async function goToProject(id: string) {
  if (isMobileWorkspaceMode.value) {
    const normalizedProjectId = id.trim();
    if (!normalizedProjectId) {
      return;
    }
    runMobileWorkspaceProjectSwitch(normalizedProjectId);
    return;
  }

  const location = buildProjectBrowserProjectLocation({
    mode: props.mode,
    projectId: id,
    currentProjectId: props.currentProjectId,
  });

  if (!location) {
    return;
  }

  await router.push(location);
}

function goToSettings() {
  void router.push('/settings');
}

function goToGuide() {
  void router.push({ name: 'guide' });
}

function getCardActions(project: Project): DropdownOption[] {
  const isPinned = project.priority !== null && project.priority !== undefined;

  return [
    { label: t('project.openProject'), key: 'open', project } as ProjectOption,
    { label: t('common.edit'), key: 'edit', project } as ProjectOption,
    { type: 'divider', key: 'd1' },
    {
      label: isPinned ? t('project.unpinProject') : t('project.pinProject'),
      key: 'toggle-pin',
      project,
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
      ],
    },
    { type: 'divider', key: 'd2' },
    { label: t('common.delete'), key: 'delete', project } as ProjectOption,
  ];
}

async function handleSetPriority(projectId: string, priority: number | null) {
  try {
    const result = await updatePriority(projectId, priority);
    if (result?.item) {
      projectStore.updateProjectInList(result.item);
    }
  } catch (error) {
    console.error('Failed to update project priority:', error);
    message.error(t('message.operationFailed'));
  }
}

function handleAction(action: string, project: Project) {
  if (action === 'open') {
    void goToProject(project.id);
  } else if (action === 'edit') {
    openEditDialog(project);
  } else if (action === 'delete') {
    confirmDelete(project);
  } else if (action === 'toggle-pin') {
    const isPinned = project.priority !== null && project.priority !== undefined;
    void handleSetPriority(project.id, isPinned ? null : 5);
  } else if (action.startsWith('priority-')) {
    const priority = parseInt(action.split('-')[1], 10) as ProjectPriority;
    void handleSetPriority(project.id, priority);
  }
}

function onCardSelect(key: string | number, option: DropdownOption) {
  const project = (option as ProjectOption).project;
  if (!project) {
    return;
  }
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
        if (project.id === props.currentProjectId) {
          await router.push({ name: 'projects' });
        }
      } catch (error: unknown) {
        const messageText = error instanceof Error ? error.message : t('message.deleteFailed');
        message.error(messageText);
      }
    },
  });
}

async function handleProjectCreated(project?: Project) {
  if (project) {
    projectStore.updateProjectInList(project);
    await goToProject(project.id);
  }
}

async function handleProjectUpdated(project?: Project) {
  if (project) {
    projectStore.updateProjectInList(project);
  }
  if (isMobileWorkspaceMode.value && props.currentProjectId) {
    await projectStore.fetchProject(props.currentProjectId);
  }
}

function getPriorityTagColor(priority: number): string {
  const colorMap: Record<number, string> = {
    5: '#e74c3c',
    4: '#ff9800',
    3: '#ffc107',
    2: '#4caf50',
    1: '#2196f3',
  };
  return colorMap[priority] || '#999';
}

function getPriorityLabel(priority: number): string {
  return t('project.priorityLevel', { level: priority });
}

onMounted(() => {
  if (projectStore.projects.length === 0) {
    void projectStore.fetchProjects({ silent: true });
  }
  void terminalStore.loadTerminalCounts();
  void webSessionStore.loadSessionCounts();
  setTimeout(checkForUpdates, 2000);
});

watch(showEditDialog, value => {
  if (!value) {
    editingProject.value = null;
  }
});
</script>

<style scoped>
.project-browser {
  --project-browser-switch-duration: 0.28s;
  --project-browser-switch-ease: cubic-bezier(0.22, 1, 0.36, 1);
  max-width: 1400px;
  margin: 0 auto;
  color: var(--app-text-primary, var(--app-text-color, #333333));
}

.project-browser--page {
  padding: 24px;
}

.project-browser--mobile-workspace {
  padding: 16px;
  padding-bottom: calc(16px + var(--workspace-mobile-bottom-nav-space, 0px));
}

.mobile-workspace-header {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.mobile-workspace-header-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: nowrap;
}

.mobile-workspace-title {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
  font-size: 20px;
  font-weight: 600;
}

.mobile-workspace-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.mobile-version-tag {
  cursor: pointer;
}

.search-toolbar {
  display: flex;
  gap: 16px;
  align-items: center;
  margin-top: 16px;
  flex-wrap: wrap;
}

:deep(.search-highlight) {
  background-color: var(--app-accent-soft, rgba(59, 105, 169, 0.2));
  color: var(--app-text-primary, #1f1f1f);
  padding: 2px 4px;
  border-radius: 3px;
  font-weight: 500;
  transition: background-color 0.2s ease;
}

.project-list-move,
.project-list-enter-active,
.project-list-leave-active {
  transition:
    transform var(--project-browser-switch-duration) var(--project-browser-switch-ease),
    opacity 0.24s ease;
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

.project-grid-item {
  min-width: 0;
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
  transition:
    transform 0.22s var(--project-browser-switch-ease),
    box-shadow 0.22s var(--project-browser-switch-ease),
    border-color 0.22s var(--project-browser-switch-ease),
    background 0.22s var(--project-browser-switch-ease);
}

.project-card.is-current {
  border-color: color-mix(in srgb, var(--app-accent, #18a058) 45%, transparent);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--app-accent, #18a058) 20%, transparent);
  background:
    linear-gradient(
      135deg,
      color-mix(in srgb, var(--app-accent, #18a058) 9%, transparent) 0%,
      transparent 100%
    ),
    var(--app-surface, #fff);
}

.project-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.project-card-header-actions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.current-project-icon {
  flex-shrink: 0;
}

.path-text {
  margin-left: 8px;
}

@media (max-width: 767px) {
  .project-browser--page {
    padding: 16px;
  }

  .project-browser--page :deep(.n-page-header) {
    padding: 0;
  }

  .project-browser--page :deep(.n-page-header-content) {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
  }

  .title-wrapper {
    flex-wrap: wrap;
  }

  .search-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
  }

  .search-toolbar .n-input {
    max-width: none;
    width: 100%;
  }

  .search-toolbar .n-space {
    width: 100%;
    justify-content: space-between;
  }

  .project-grid {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .project-card :deep(.n-card-header) {
    padding: 12px;
  }

  .project-card :deep(.n-card__content) {
    padding: 12px;
    padding-top: 0;
  }

  .empty-container {
    min-height: 300px;
  }

  .mobile-workspace-header-main {
    align-items: center;
  }
}

@media (min-width: 768px) and (max-width: 1023px) {
  .project-browser--page {
    padding: 20px;
  }

  .project-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
