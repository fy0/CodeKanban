<template>
  <div class="recent-projects" :class="{ 'is-compact': isCompact }">
    <div class="recent-projects-header" :class="{ 'is-compact': isCompact }">
      <template v-if="!isCompact">
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
          <n-space align="center" :wrap="false" size="small">
            <ThemeSwitcher />
            <n-popover trigger="hover" placement="bottom">
              <template #trigger>
                <n-button
                  quaternary
                  circle
                  size="small"
                  :disabled="!currentProject"
                  @click="emit('editCurrent')"
                >
                  <template #icon>
                    <n-icon size="18">
                      <CreateOutline />
                    </n-icon>
                  </template>
                </n-button>
              </template>
              {{ t('common.edit') }}
            </n-popover>
            <n-popover trigger="hover" placement="bottom">
              <template #trigger>
                <n-button quaternary circle size="small" @click="handleGoToSettings">
                  <template #icon>
                    <n-icon size="18">
                      <SettingsOutline />
                    </n-icon>
                  </template>
                </n-button>
              </template>
              {{ t('nav.settings') }}
            </n-popover>
          </n-space>
        </n-space>
      </template>
      <template v-else>
        <div class="recent-projects-compact-actions">
          <n-popover trigger="hover" placement="right">
            <template #trigger>
              <n-button quaternary circle class="compact-header-button" @click="handleBackToList">
                <template #icon>
                  <n-icon size="18">
                    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                      <path
                        fill="currentColor"
                        d="M20 11H7.83l5.59-5.59L12 4l-8 8l8 8l1.41-1.41L7.83 13H20v-2z"
                      />
                    </svg>
                  </n-icon>
                </template>
              </n-button>
            </template>
            {{ t('common.backToList') }}
          </n-popover>
        </div>
      </template>
    </div>

    <div v-if="recentProjects.length === 0" class="empty-state">
      <n-text depth="3">{{ loading ? t('common.loading') : t('common.noRecentProjects') }}</n-text>
    </div>

    <n-scrollbar v-else class="projects-scrollbar">
      <div class="projects-list" :class="{ 'is-compact': isCompact }">
        <TransitionGroup name="project-list" tag="div">
          <div v-for="project in recentProjects" :key="project.id" class="project-list-row">
            <n-popover
              trigger="hover"
              placement="right-start"
              :disabled="
                isMobile === true || (!isCompact && !isProjectSummaryPopoverEnabled(project.id))
              "
              :content-style="compactPopoverContentStyle"
            >
              <template #trigger>
                <div class="project-item-popover-trigger">
                  <div
                    class="project-item"
                    :class="{ active: project.id === currentProjectId, 'is-compact': isCompact }"
                    :title="isCompact ? project.name : undefined"
                    :aria-current="project.id === currentProjectId ? 'page' : undefined"
                    @click="handleSelectProject(project.id)"
                    @contextmenu="handleContextMenu($event, project.id)"
                  >
                    <n-icon
                      v-if="projectStore.getProjectPriority(project.id)"
                      size="12"
                      :color="getPriorityColor(projectStore.getProjectPriority(project.id)!)"
                      class="pin-icon-corner"
                      :class="{ 'is-compact': isCompact }"
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

                    <template v-if="isCompact">
                      <div class="project-compact-avatar">
                        {{ getProjectMonogram(project.name) }}
                      </div>
                      <div
                        v-if="getProjectSessionBadge(project.id)"
                        class="project-compact-counts"
                        :title="formatProjectBadgeLabel(getProjectSessionBadge(project.id))"
                      >
                        <template v-if="isCombinedProjectSessionBadge(project.id)">
                          <span class="project-compact-count is-terminal">{{
                            getCombinedTerminalCount(project.id)
                          }}</span>
                          <span class="project-compact-count is-web-session">{{
                            getCombinedWebSessionCount(project.id)
                          }}</span>
                        </template>
                        <template v-else>
                          <span
                            class="project-compact-count"
                            :class="getCompactSingleCountClass(project.id)"
                            >{{ getSingleProjectSessionCount(project.id) }}</span
                          >
                        </template>
                      </div>
                    </template>

                    <template v-else>
                      <div class="project-info">
                        <div class="project-name-row">
                          <n-tag
                            v-if="getProjectSessionBadge(project.id)"
                            size="small"
                            :type="getProjectSessionBadgeType(project.id)"
                            :bordered="false"
                            class="terminal-tag"
                            :class="{
                              'terminal-tag--combined': isCombinedProjectSessionBadge(project.id),
                              clickable:
                                getProjectSessionBadge(project.id)?.kind === 'terminal' &&
                                project.id === currentProjectId,
                            }"
                            :title="formatProjectBadgeLabel(getProjectSessionBadge(project.id))"
                            @click.stop="
                              handleProjectBadgeClick(
                                project.id,
                                getProjectSessionBadge(project.id)
                              )
                            "
                          >
                            <template #icon>
                              <n-icon
                                v-if="isCombinedProjectSessionBadge(project.id)"
                                size="14"
                                class="terminal-tag-combined-icon"
                                :class="getCombinedProjectSessionActiveIconClass()"
                              >
                                <component :is="getCombinedProjectSessionActiveIcon()" />
                              </n-icon>
                              <n-icon v-else size="14">
                                <component
                                  :is="
                                    getProjectSessionBadge(project.id)?.kind === 'terminal'
                                      ? TerminalOutline
                                      : ChatbubblesOutline
                                  "
                                />
                              </n-icon>
                            </template>
                            <span
                              v-if="isCombinedProjectSessionBadge(project.id)"
                              class="terminal-tag-combined-counts"
                            >
                              <span
                                class="terminal-tag-combined-count terminal-tag-combined-count--terminal"
                              >
                                {{ getCombinedTerminalCount(project.id) }}
                              </span>
                              <span class="terminal-tag-combined-separator" aria-hidden="true"
                                >·</span
                              >
                              <span
                                class="terminal-tag-combined-count terminal-tag-combined-count--web"
                              >
                                {{ getCombinedWebSessionCount(project.id) }}
                              </span>
                            </span>
                            <template v-else>
                              {{ getSingleProjectSessionCount(project.id) }}
                            </template>
                          </n-tag>
                          <n-text class="project-name" strong>{{ project.name }}</n-text>
                        </div>
                        <n-text v-if="!project.hidePath" class="project-path" depth="3">
                          {{ project.path }}
                        </n-text>
                      </div>
                      <Transition name="project-check">
                        <n-icon v-if="project.id === currentProjectId" size="18" color="#18a058">
                          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                            <path
                              fill="currentColor"
                              d="M9 16.17L4.83 12l-1.42 1.41L9 19L21 7l-1.41-1.41L9 16.17z"
                            />
                          </svg>
                        </n-icon>
                      </Transition>
                    </template>
                  </div>
                </div>
              </template>

              <ProjectAiStatusSummaryCard :project-id="project.id" compact />
            </n-popover>
          </div>
        </TransitionGroup>
      </div>
    </n-scrollbar>
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

    <div class="version-info-container" :class="{ 'is-compact': isCompact }">
      <a
        class="version-info"
        :class="{ 'is-compact': isCompact }"
        href="https://github.com/fy0/CodeKanban"
        target="_blank"
        rel="noopener noreferrer"
        :title="brandTitle"
        @click.prevent="handleAppNameClick"
      >
        <span class="app-logo-shell" :class="{ 'is-compact': isCompact }">
          <img src="/favicon.svg" alt="CodeKanban" class="app-logo" />
        </span>
        <template v-if="!isCompact">
          <span class="version-info-meta">
            <n-text strong class="version-app-name">{{ appStore.appInfo.name }}</n-text>
            <span class="version-info-secondary">
              <n-popover v-if="updateInfo?.hasUpdate" trigger="hover" placement="top">
                <template #trigger>
                  <n-text
                    type="warning"
                    class="version-text version-text--update"
                    @click.prevent.stop="showUpdateModal = true"
                  >
                    v{{ displayAppVersion }}
                    <n-icon :size="12" :component="ArrowUpCircleOutline" />
                  </n-text>
                </template>
                <div class="version-update-popover">
                  {{ t('update.newVersionAvailable') }}:
                  {{ formatDisplayVersion(updateInfo.latestVersion) }}
                </div>
              </n-popover>
              <n-text v-else depth="3" class="version-text version-text--plain">
                v{{ displayAppVersion }}
              </n-text>
            </span>
          </span>
        </template>
        <n-popover v-else-if="updateInfo?.hasUpdate" trigger="hover" placement="right">
          <template #trigger>
            <n-text
              type="warning"
              class="version-text version-text--update"
              :class="{ 'is-compact': isCompact }"
              @click.prevent.stop="showUpdateModal = true"
            >
              <n-icon :size="10" :component="ArrowUpCircleOutline" />
            </n-text>
          </template>
          <div class="version-update-popover">
            {{ t('update.newVersionAvailable') }}:
            {{ formatDisplayVersion(updateInfo.latestVersion) }}
          </div>
        </n-popover>
      </a>
    </div>

    <n-modal
      v-model:show="showUpdateModal"
      preset="card"
      style="width: 420px"
      :title="t('update.newVersionAvailable')"
    >
      <div style="margin-bottom: 16px">
        <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 12px">
          <span style="color: var(--n-text-color-3)">{{ t('update.currentVersion') }}:</span>
          <n-tag :bordered="false" size="small">{{
            formatDisplayVersion(updateInfo?.currentVersion)
          }}</n-tag>
        </div>
        <div style="display: flex; align-items: center; gap: 12px">
          <span style="color: var(--n-text-color-3)">{{ t('update.latestVersion') }}:</span>
          <n-tag type="success" :bordered="false" size="small">{{
            formatDisplayVersion(updateInfo?.latestVersion)
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
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useStorage } from '@vueuse/core';
import { useDialog } from 'naive-ui';
import { useProjectStore } from '@/stores/project';
import { useTerminalStore } from '@/stores/terminal';
import { useWebSessionStore } from '@/stores/webSession';
import { useAppStore } from '@/stores/app';
import {
  CreateOutline,
  SettingsOutline,
  TerminalOutline,
  ChatbubblesOutline,
  ArrowUpCircleOutline,
} from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import ThemeSwitcher from '@/components/common/ThemeSwitcher.vue';
import ProjectAiStatusSummaryCard from '@/components/project/ProjectAiStatusSummaryCard.vue';
import {
  resolvePreferredProjectSessionKind,
  resolveProjectSessionBadge,
  type ProjectSessionBadge,
} from '@/utils/projectSessionBadge';
import { useAppClipboard } from '@/composables/useAppClipboard';
import { formatVersionForDisplay } from '@/utils/versionDisplay';
import { DEFAULT_DESKTOP_WORKSPACE_ROUTE_TAB } from '@/utils/workspaceRoute';
import type { ProjectPriority } from '@/stores/project';
import type { DropdownOption } from 'naive-ui';
import Apis from '@/api';
import { useReq } from '@/api';

const { t } = useLocale();
const dialog = useDialog();
const { copyText } = useAppClipboard();

interface UpdateInfo {
  currentVersion: string;
  latestVersion: string;
  hasUpdate: boolean;
  updateUrl?: string;
}

interface ContextMenuState {
  show: boolean;
  x: number;
  y: number;
  projectId: string | null;
}

type MobileView =
  | 'kanban'
  | 'terminal'
  | 'webSession'
  | 'files'
  | 'changes'
  | 'projects'
  | 'notifications';
type WorkspaceTab = 'kanban' | 'terminal' | 'web' | 'changes' | 'files';

const emit = defineEmits<{ editCurrent: []; showTerminal: [] }>();
const props = defineProps<{
  currentProjectId: string;
  isMobile?: boolean;
  compact?: boolean;
}>();

const MOBILE_ACTIVE_VIEW_STORAGE_KEY = 'workspace-mobile-active-view-by-project';
const WORKSPACE_ACTIVE_TAB_STORAGE_KEY = 'workspace-active-tab';

const route = useRoute();
const router = useRouter();
const projectStore = useProjectStore();
const terminalStore = useTerminalStore();
const webSessionStore = useWebSessionStore();
const appStore = useAppStore();

const updateInfo = ref<UpdateInfo | null>(null);
const showUpdateModal = ref(false);
const contextMenu = ref<ContextMenuState>({
  show: false,
  x: 0,
  y: 0,
  projectId: null,
});

const { send: checkUpdate } = useReq(() => Apis.system.checkUpdate({}));
const { send: updatePriority } = useReq((projectId: string, priority: number | null) =>
  Apis.project.updatePriority({
    pathParams: { id: projectId },
    data: { priority },
  })
);

const loading = computed(() => projectStore.loading);
const currentProject = computed(() => projectStore.currentProject);
const recentProjects = computed(() => projectStore.recentProjects);
const topProjectId = computed(() => recentProjects.value[0]?.id ?? '');
const isCompact = computed(() => props.compact === true);
const displayAppVersion = computed(() => formatVersionForDisplay(appStore.appInfo.version));
const brandTitle = computed(() => `${appStore.appInfo.name} v${displayAppVersion.value}`);
const terminalCounts = terminalStore.terminalCounts;
const webSessionCounts = webSessionStore.sessionCounts;
const compactPopoverContentStyle = 'padding: 4px 6px;';
const storedMobileViews = useStorage<Record<string, MobileView>>(
  MOBILE_ACTIVE_VIEW_STORAGE_KEY,
  {}
);
const storedWorkspaceTab = useStorage<WorkspaceTab>(
  WORKSPACE_ACTIVE_TAB_STORAGE_KEY,
  DEFAULT_DESKTOP_WORKSPACE_ROUTE_TAB
);

const preferredSessionKind = computed(() =>
  resolvePreferredProjectSessionKind({
    isMobile: Boolean(props.isMobile),
    isProjectWorkspace: !props.isMobile && route.name === 'project',
    mobileActiveView: props.currentProjectId ? storedMobileViews.value[props.currentProjectId] : '',
    workspaceActiveTab: storedWorkspaceTab.value,
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
    window.open(updateInfo.value.updateUrl, '_blank', 'noopener,noreferrer');
  }
};

const formatDisplayVersion = (version?: string | null) =>
  version ? formatVersionForDisplay(version) : '';
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

const handleSelectProject = (projectId: string) => {
  if (projectId !== props.currentProjectId) {
    router.push({ name: 'project', params: { id: projectId } });
  }
};

const isProjectSummaryPopoverEnabled = (projectId: string) =>
  projectId === props.currentProjectId && projectId === topProjectId.value;

const handleContextMenu = (event: MouseEvent, projectId: string) => {
  event.preventDefault();
  contextMenu.value = {
    show: false,
    x: event.clientX,
    y: event.clientY,
    projectId,
  };

  setTimeout(() => {
    contextMenu.value.show = true;
  }, 0);
};

const handleClickOutside = () => {
  contextMenu.value.show = false;
};

const contextMenuOptions = computed<DropdownOption[]>(() => {
  if (!contextMenu.value.projectId) {
    return [];
  }

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

const handleSetPriority = async (projectId: string, priority: number | null) => {
  try {
    const result = await updatePriority(projectId, priority);
    if (result?.item) {
      projectStore.updateProjectInList(result.item);
    }
  } catch (error) {
    console.error('Failed to update project priority:', error);
  }
};

const handleContextMenuSelect = async (key: string) => {
  const projectId = contextMenu.value.projectId;
  if (!projectId) {
    return;
  }

  contextMenu.value.show = false;

  switch (key) {
    case 'edit':
      if (projectId === props.currentProjectId) {
        emit('editCurrent');
      } else {
        router.push({ name: 'project', params: { id: projectId } }).then(() => {
          emit('editCurrent');
        });
      }
      break;
    case 'toggle-pin':
      await handleSetPriority(projectId, projectStore.getProjectPriority(projectId) ? null : 5);
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
    case 'close-all-terminals': {
      const terminalCount = terminalCounts.get(projectId) || 0;
      const project = projectStore.projects.find(item => item.id === projectId);
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
      break;
    }
    case 'remove':
      projectStore.removeRecentProject(projectId);
      break;
  }
};

const handleTerminalTagClick = (projectId: string) => {
  if (projectId === props.currentProjectId) {
    emit('showTerminal');
  }
};

const getProjectSessionBadge = (projectId: string) =>
  resolveProjectSessionBadge({
    terminalCount: terminalCounts.get(projectId) ?? 0,
    webSessionCount: webSessionCounts.get(projectId) ?? 0,
    preferredKind: preferredSessionKind.value,
  });

const isCombinedProjectSessionBadge = (projectId: string) =>
  getProjectSessionBadge(projectId)?.kind === 'combined';

const getCombinedTerminalCount = (projectId: string) => {
  const badge = getProjectSessionBadge(projectId);
  return badge?.kind === 'combined' ? badge.terminalCount : 0;
};

const getCombinedWebSessionCount = (projectId: string) => {
  const badge = getProjectSessionBadge(projectId);
  return badge?.kind === 'combined' ? badge.webSessionCount : 0;
};

const getCombinedProjectSessionActiveIcon = () =>
  preferredSessionKind.value === 'webSession' ? ChatbubblesOutline : TerminalOutline;

const getCombinedProjectSessionActiveIconClass = () =>
  preferredSessionKind.value === 'webSession' ? 'is-web-session' : 'is-terminal';

const getSingleProjectSessionCount = (projectId: string) => {
  const badge = getProjectSessionBadge(projectId);
  return badge && badge.kind !== 'combined' ? badge.count : 0;
};

const getCompactSingleCountClass = (projectId: string) => {
  const badge = getProjectSessionBadge(projectId);

  if (!badge || badge.kind === 'combined') {
    return '';
  }

  return badge.kind === 'terminal' ? 'is-terminal' : 'is-web-session';
};

const getProjectSessionBadgeType = (projectId: string) => {
  const badge = getProjectSessionBadge(projectId);
  if (!badge || badge.kind === 'combined') {
    return undefined;
  }
  return badge.kind === 'terminal' ? 'success' : 'info';
};

const formatProjectBadgeLabel = (badge: ProjectSessionBadge) => {
  if (!badge) {
    return '';
  }
  if (badge.kind === 'combined') {
    return `${t('project.terminalCount')}: ${badge.terminalCount} · ${t('project.webSessionCount')}: ${badge.webSessionCount}`;
  }
  return badge.kind === 'terminal'
    ? `${t('project.terminalCount')}: ${badge.count}`
    : `${t('project.webSessionCount')}: ${badge.count}`;
};

const handleProjectBadgeClick = (projectId: string, badge: ProjectSessionBadge) => {
  if (badge?.kind === 'terminal') {
    handleTerminalTagClick(projectId);
  }
};

const handleBackToList = () => {
  router.push({ name: 'projects' });
};

const handleGoToSettings = () => {
  void router.push('/settings');
};

const getProjectMonogram = (name: string) => {
  const trimmed = name.trim();
  if (!trimmed) {
    return '?';
  }

  const segments = trimmed.split(/\s+/).filter(Boolean);
  if (segments.length >= 2) {
    return `${segments[0][0]}${segments[1][0]}`.toUpperCase();
  }

  return trimmed.slice(0, 1).toUpperCase();
};

const getPriorityColor = (priority: ProjectPriority): string => {
  const colorMap: Record<ProjectPriority, string> = {
    5: '#e74c3c',
    4: '#ff9800',
    3: '#ffc107',
    2: '#4caf50',
    1: '#2196f3',
  };
  return colorMap[priority];
};

const handleUnpinProject = async (projectId: string) => {
  await handleSetPriority(projectId, null);
};

onMounted(() => {
  if (projectStore.projects.length === 0) {
    void projectStore.fetchProjects({ silent: true });
  }
  terminalStore.loadTerminalCounts();
  webSessionStore.loadSessionCounts();
  setTimeout(checkForUpdates, 2000);
});
</script>

<style scoped>
.recent-projects {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--n-color);
  overflow: hidden;
}

.recent-projects-header {
  --recent-projects-header-height: 64px;
  padding: 16px;
  min-height: var(--recent-projects-header-height);
  box-sizing: border-box;
  border-bottom: 1px solid var(--n-border-color);
}

.recent-projects-header.is-compact {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: calc(var(--recent-projects-header-height) - 4px);
  padding: 0 10px;
}

.recent-projects-compact-actions {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.compact-header-button {
  width: 32px;
  min-width: 32px;
  height: 32px;
  border-radius: 12px;
}

.empty-state {
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px 16px;
  text-align: center;
}

.projects-scrollbar {
  flex: 1;
  min-height: 0;
}

.projects-scrollbar :deep(.n-scrollbar-container) {
  overflow-x: hidden !important;
}

.projects-scrollbar :deep(.n-scrollbar-content) {
  min-width: 0;
  width: 100%;
  display: block;
}

.projects-scrollbar :deep(.n-scrollbar-rail.n-scrollbar-rail--vertical) {
  right: 4px;
}

.projects-list {
  min-height: 0;
  width: 100%;
  box-sizing: border-box;
  padding: 10px 0 14px;
  overflow-x: hidden;
  position: relative;
  scrollbar-gutter: stable;
}

.projects-list.is-compact {
  padding: 12px 0 14px;
}

.projects-list.is-compact .project-item-popover-trigger {
  display: flex;
  justify-content: center;
  width: 100%;
  box-sizing: border-box;
}

.project-list-row {
  display: block;
}

.project-item-popover-trigger {
  display: block;
}

.project-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-width: 0;
  padding: 12px 16px;
  cursor: pointer;
  border-left: 3px solid transparent;
  transition:
    background-color 0.2s,
    border-left-color 0.2s;
}

.project-item:hover {
  background-color: var(--n-item-color-hover, rgba(0, 0, 0, 0.04));
}

.project-item.active {
  background-color: transparent;
  border-left-color: var(--n-primary-color);
}

.project-item.is-compact {
  width: 64px;
  height: 40px;
  min-height: 40px;
  justify-content: flex-start;
  align-items: center;
  margin: 0 auto 12px;
  gap: 6px;
  padding: 0 6px;
  border-left: none;
  border: none;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  overflow: visible;
}

.project-item.is-compact:hover {
  background: transparent;
}

.project-item.is-compact.active {
  background: transparent;
}

.project-compact-avatar {
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  border: 1px solid #cfd6df;
  background: #ffffff;
  color: #334155;
  font-size: 17px;
  font-weight: 500;
  letter-spacing: 0;
  text-transform: uppercase;
  box-shadow: none;
}

.project-item.is-compact.active .project-compact-avatar {
  border-color: #374151;
  color: #111827;
}

.project-compact-counts {
  width: 20px;
  min-width: 20px;
  flex: 0 0 20px;
  margin-left: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
}

.project-compact-count {
  display: inline-flex;
  width: 100%;
  min-height: 16px;
  align-items: center;
  justify-content: center;
  padding: 0 2px;
  border-radius: 3px;
  background: #e5e7eb;
  text-align: center;
  color: #334155;
  font-size: 10px;
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: -0.01em;
  font-variant-numeric: tabular-nums;
}

.project-item.is-compact.active .project-compact-count {
  background: #d1d5db;
  color: #1f2937;
}

.project-item.is-compact.active .project-compact-count.is-terminal {
  background: #dcfce7;
  color: #166534;
}

.project-item.is-compact.active .project-compact-count.is-web-session {
  background: #dbeafe;
  color: #1d4ed8;
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
  transition:
    opacity 0.2s,
    transform 0.2s;
}

.terminal-tag--combined {
  --terminal-tag-terminal-color: #18a058;
  --terminal-tag-terminal-bg: #eaf8e3;
  --terminal-tag-web-color: #2080f0;
  --terminal-tag-web-bg: #e7edf5;
  --n-padding: 0 6px 0 5px;
  color: rgba(15, 23, 42, 0.92);
  background: linear-gradient(
    90deg,
    color-mix(in srgb, var(--terminal-tag-terminal-bg) 94%, white 6%) 0%,
    color-mix(in srgb, var(--terminal-tag-terminal-bg) 78%, var(--terminal-tag-web-bg) 22%) 44%,
    color-mix(in srgb, var(--terminal-tag-web-bg) 78%, var(--terminal-tag-terminal-bg) 22%) 56%,
    color-mix(in srgb, var(--terminal-tag-web-bg) 94%, white 6%) 100%
  ) !important;
}

.terminal-tag--combined :deep(.n-tag__content) {
  display: inline-flex;
  align-items: center;
}

.terminal-tag--combined :deep(.n-tag__icon) {
  margin-right: 3px;
}

.terminal-tag-combined-icon {
  color: var(--terminal-tag-terminal-color);
}

.terminal-tag-combined-icon.is-web-session {
  color: var(--terminal-tag-web-color);
}

.terminal-tag-combined-counts {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.01em;
  font-variant-numeric: tabular-nums;
}

.terminal-tag-combined-count {
  display: inline-block;
}

.terminal-tag-combined-count--terminal {
  color: var(--terminal-tag-terminal-color);
}

.terminal-tag-combined-separator {
  display: inline-block;
  margin: 0 1px;
  color: rgba(15, 23, 42, 0.48);
}

.terminal-tag-combined-count--web {
  color: var(--terminal-tag-web-color);
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
  transition:
    opacity 0.2s,
    transform 0.2s;
}

.pin-icon-corner.is-compact {
  top: 6px;
  left: auto;
  right: 6px;
}

.pin-icon-corner:hover {
  opacity: 1;
  transform: scale(1.2);
}

.project-list-move,
.project-list-enter-active,
.project-list-leave-active {
  transition: transform 0.4s cubic-bezier(0.22, 1, 0.36, 1);
}

.project-list-enter-from {
  transform: translateY(8px);
}

.project-list-leave-to {
  transform: translateY(-8px);
}

.project-list-leave-active {
  position: absolute;
  left: 0;
  right: 0;
}

.project-check-enter-active,
.project-check-leave-active {
  transition:
    opacity 0.18s var(--project-switch-ease),
    transform 0.18s var(--project-switch-ease);
}

.project-check-enter-from,
.project-check-leave-to {
  opacity: 0;
  transform: translateX(-6px) scale(0.9);
}

.version-info-container {
  flex-shrink: 0;
  padding: 12px 16px;
  border-top: 1px solid var(--n-border-color);
  background-color: var(--n-color-target);
  display: flex;
  align-items: center;
  container-type: inline-size;
}

.version-info-container.is-compact {
  padding: 12px 10px 14px;
  justify-content: center;
}

.version-info {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  column-gap: 8px;
  min-width: 0;
  text-decoration: none;
  color: inherit;
  background: transparent;
  border: none;
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease,
    box-shadow 0.2s ease,
    transform 0.2s ease;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 4px;
  margin: -4px -8px;
  text-align: left;
}

.version-info.is-compact {
  display: inline-flex;
  position: relative;
  justify-content: center;
  width: 24px;
  min-width: 24px;
  height: 24px;
  margin: 0;
  padding: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

.version-info:hover {
  background-color: var(--n-item-color-hover);
}

.version-info.is-compact:hover {
  background: transparent;
  box-shadow: none;
}

.version-info.is-compact:active {
  transform: none;
}

.version-info:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--n-primary-color, #2080f0) 45%, transparent 55%);
  outline-offset: 2px;
}

.app-logo-shell {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.version-info-meta {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.version-info-secondary {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.version-app-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}

.app-logo-shell.is-compact {
  width: 18px;
  height: 18px;
  background: transparent;
  box-shadow: none;
}

.version-info :deep(.n-text) {
  line-height: 1;
  display: flex;
  align-items: center;
}

.version-text--update {
  font-size: 11px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 2px;
}

.version-text--plain {
  font-size: 11px;
}

.version-update-popover {
  font-size: 12px;
}

.version-text--update.is-compact {
  position: absolute;
  right: -3px;
  bottom: -2px;
  width: 16px;
  height: 16px;
  justify-content: center;
  font-size: 0;
  gap: 0;
  color: #ffffff;
  background: linear-gradient(135deg, #f59e0b 0%, #f97316 100%);
  border: 2px solid var(--app-surface-color, #ffffff);
  border-radius: 999px;
  box-shadow: 0 4px 10px rgba(249, 115, 22, 0.28);
}

.app-logo {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
  transition: opacity 0.2s ease;
}

@container (max-width: 188px) {
  .version-info:not(.is-compact) {
    align-items: flex-start;
  }

  .version-info-meta {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    align-items: start;
    gap: 2px;
  }

  .version-info-secondary {
    min-width: 0;
    justify-content: flex-start;
  }
}

@media (prefers-reduced-motion: reduce) {
  .version-info,
  .app-logo {
    transition: none;
  }

  .version-info.is-compact:hover,
  .version-info.is-compact:active,
  .version-info.is-compact:hover .app-logo {
    transform: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .project-item,
  .terminal-tag,
  .pin-icon-corner,
  .version-info,
  .project-list-move,
  .project-list-enter-active,
  .project-list-leave-active,
  .project-check-enter-active,
  .project-check-leave-active {
    transition-duration: 0.01ms !important;
    animation: none !important;
  }

  .project-check-enter-from,
  .project-check-leave-to {
    transform: none !important;
  }
}
</style>
