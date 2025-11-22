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
          {{ t('common.backToList') }}
        </n-button>
        <n-space>
          <ThemeSwitcher />
          <n-button text :disabled="!currentProject" @click="emit('editCurrent')">
            <template #icon>
              <n-icon size="20">
                <CreateOutline />
              </n-icon>
            </template>
            {{ t('common.edit') }}
          </n-button>
          <n-button text @click="handleGoToSettings">
            <template #icon>
              <n-icon size="20">
                <SettingsOutline />
              </n-icon>
            </template>
            {{ t('nav.settings') }}
          </n-button>
        </n-space>
      </n-space>
    </div>
    <div v-if="recentProjects.length === 0" class="empty-state">
      <n-text depth="3">{{ loading ? t('common.loading') : t('common.noRecentProjects') }}</n-text>
    </div>
    <div v-else class="projects-list">
      <TransitionGroup name="project-list" tag="div">
        <div
          v-for="project in recentProjects"
          :key="project.id"
          class="project-item"
          :class="{ active: project.id === currentProjectId }"
          @click="handleSelectProject(project.id)"
          @contextmenu="handleContextMenu($event, project.id)"
        >
          <n-icon
            v-if="projectStore.getProjectPriority(project.id)"
            size="12"
            :color="getPriorityColor(projectStore.getProjectPriority(project.id)!)"
            class="pin-icon-corner"
            :title="t('project.unpinProject')"
            @click.stop="handleUnpinProject(project.id)"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
              <path
                fill="currentColor"
                d="M16,12V4H17V2H7V4H8V12L6,14V16H11.2V22H12.8V16H18V14L16,12Z"
              />
            </svg>
          </n-icon>
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
    <n-dropdown
      placement="bottom-start"
      trigger="manual"
      :x="contextMenu.x"
      :y="contextMenu.y"
      :options="contextMenuOptions"
      :show="contextMenu.show"
      :on-clickoutside="handleClickOutside"
      @select="handleContextMenuSelect"
    />
    <div class="version-info-container">
      <a
        class="version-info"
        href="https://github.com/fy0/CodeKanban"
        target="_blank"
        rel="noopener noreferrer"
      >
        <img src="/favicon.svg" alt="CodeKanban" class="app-logo" />
        <n-text strong style="font-size: 13px">{{ appStore.appInfo.name }}</n-text>
        <n-popover v-if="updateInfo?.hasUpdate" trigger="hover" placement="top">
          <template #trigger>
            <n-text
              type="warning"
              style="font-size: 11px; cursor: pointer; display: flex; align-items: center; gap: 2px"
              @click.prevent="showUpdateModal = true"
            >
              v{{ appStore.appInfo.version }}
              <n-icon :size="12" :component="ArrowUpCircleOutline" />
            </n-text>
          </template>
          <div style="font-size: 12px">
            {{ t('update.newVersionAvailable') }}: {{ updateInfo.latestVersion }}
          </div>
        </n-popover>
        <n-text v-else depth="3" style="font-size: 11px">v{{ appStore.appInfo.version }}</n-text>
      </a>
    </div>

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
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useDialog, useMessage } from 'naive-ui';
import { useProjectStore } from '@/stores/project';
import { useTerminalStore } from '@/stores/terminal';
import { useAppStore } from '@/stores/app';
import { CreateOutline, SettingsOutline, TerminalOutline, ArrowUpCircleOutline } from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import ThemeSwitcher from '@/components/common/ThemeSwitcher.vue';
import type { ProjectPriority } from '@/stores/project';
import type { DropdownOption } from 'naive-ui';
import Apis from '@/api';
import { useReq } from '@/api';

const { t } = useLocale();
const dialog = useDialog();
const message = useMessage();

// 更新检查
interface UpdateInfo {
  currentVersion: string;
  latestVersion: string;
  hasUpdate: boolean;
  updateUrl?: string;
}
const updateInfo = ref<UpdateInfo | null>(null);
const showUpdateModal = ref(false);

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

interface ContextMenuState {
  show: boolean;
  x: number;
  y: number;
  projectId: string | null;
}

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

const contextMenu = ref<ContextMenuState>({
  show: false,
  x: 0,
  y: 0,
  projectId: null,
});

// 使用 useReq 定义优先级更新请求
const { send: updatePriority, loading: priorityLoading } = useReq(
  (projectId: string, priority: number | null) => Apis.project.updatePriority({
    pathParams: { id: projectId },
    data: { priority }
  })
);

const handleSelectProject = (projectId: string) => {
  if (projectId !== props.currentProjectId) {
    router.push({ name: 'project', params: { id: projectId } });
  }
};

const handleContextMenu = (e: MouseEvent, projectId: string) => {
  e.preventDefault();
  contextMenu.value = {
    show: false,
    x: e.clientX,
    y: e.clientY,
    projectId,
  };
  // 使用 nextTick 确保在 DOM 更新后显示菜单
  setTimeout(() => {
    contextMenu.value.show = true;
  }, 0);
};

const handleClickOutside = () => {
  contextMenu.value.show = false;
};

const contextMenuOptions = computed<DropdownOption[]>(() => {
  if (!contextMenu.value.projectId) return [];

  const projectId = contextMenu.value.projectId;
  const currentPriority = projectStore.getProjectPriority(projectId);
  const isPinned = currentPriority !== null;
  const hasTerminals = terminalCounts.get(projectId) && terminalCounts.get(projectId)! > 0;

  return [
    {
      label: t('project.edit'),
      key: 'edit',
    },
    {
      type: 'divider',
      key: 'd1',
    },
    {
      label: isPinned ? t('project.unpinProject') : t('project.pinProject'),
      key: 'toggle-pin',
    },
    {
      label: t('project.setPriority'),
      key: 'priority',
      children: [
        {
          label: t('project.priority5'),
          key: 'priority-5',
        },
        {
          label: t('project.priority4'),
          key: 'priority-4',
        },
        {
          label: t('project.priority3'),
          key: 'priority-3',
        },
        {
          label: t('project.priority2'),
          key: 'priority-2',
        },
        {
          label: t('project.priority1'),
          key: 'priority-1',
        },
      ],
    },
    {
      type: 'divider',
      key: 'd2',
    },
    {
      label: t('project.closeAllTerminals'),
      key: 'close-all-terminals',
      disabled: !hasTerminals,
    },
    {
      type: 'divider',
      key: 'd3',
    },
    {
      label: t('project.removeFromRecent'),
      key: 'remove',
    },
  ];
});

// 处理优先级更新的辅助函数
const handleSetPriority = async (projectId: string, priority: number | null) => {
  try {
    const result = await updatePriority(projectId, priority);

    // Apis 返回的结果包含 item 字段
    if (result?.item) {
      // 更新 Store 中的状态（Store 只负责存储，不调用 API）
      projectStore.updateProjectInList(result.item);
    }
  } catch (error) {
    console.error('Failed to update project priority:', error);
  }
};

const handleContextMenuSelect = async (key: string) => {
  const projectId = contextMenu.value.projectId;
  if (!projectId) return;

  contextMenu.value.show = false;

  switch (key) {
    case 'edit':
      if (projectId === props.currentProjectId) {
        emit('editCurrent');
      } else {
        // 如果不是当前项目，先切换到该项目
        router.push({ name: 'project', params: { id: projectId } }).then(() => {
          emit('editCurrent');
        });
      }
      break;
    case 'toggle-pin':
      if (projectStore.getProjectPriority(projectId)) {
        await handleSetPriority(projectId, null);
      } else {
        await handleSetPriority(projectId, 5);
      }
      break;
    case 'priority-5':
      await handleSetPriority(projectId, 5);
      break;
    case 'priority-4':
      await handleSetPriority(projectId, 4);
      break;
    case 'priority-3':
      await handleSetPriority(projectId, 3);
      break;
    case 'priority-2':
      await handleSetPriority(projectId, 2);
      break;
    case 'priority-1':
      await handleSetPriority(projectId, 1);
      break;
    case 'close-all-terminals':
      {
        const terminalCount = terminalCounts.get(projectId) || 0;
        const project = projectStore.projects.find(p => p.id === projectId);
        dialog.warning({
          title: t('project.closeAllTerminals'),
          content: t('project.closeAllTerminalsConfirm', {
            count: terminalCount,
            name: project?.name || '',
          }),
          positiveText: t('common.confirm'),
          negativeText: t('common.cancel'),
          onPositiveClick: async () => {
            try {
              await terminalStore.closeAllSessions(projectId);
            } catch (error) {
              console.error('Failed to close all terminals:', error);
            }
          },
        });
      }
      break;
    case 'remove':
      projectStore.removeRecentProject(projectId);
      break;
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

const getPriorityColor = (priority: ProjectPriority): string => {
  const colorMap: Record<ProjectPriority, string> = {
    5: '#e74c3c', // 红色 - 最高优先级
    4: '#ff9800', // 橙色
    3: '#ffc107', // 黄色
    2: '#4caf50', // 绿色
    1: '#2196f3', // 蓝色 - 最低优先级
  };
  return colorMap[priority];
};

const handleUnpinProject = async (projectId: string) => {
  await handleSetPriority(projectId, null);
};

onMounted(() => {
  if (projectStore.projects.length === 0) {
    projectStore.fetchProjects();
  }
  terminalStore.loadTerminalCounts();
  // 延迟检查更新
  setTimeout(checkForUpdates, 2000);
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
  position: relative;
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

.pin-icon-corner {
  position: absolute;
  top: 4px;
  left: 4px;
  z-index: 1;
  pointer-events: auto;
  opacity: 0.85;
  filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.15));
  cursor: pointer;
  transition: opacity 0.2s, transform 0.2s;
}

.pin-icon-corner:hover {
  opacity: 1;
  transform: scale(1.2);
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
