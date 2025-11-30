import { defineStore, storeToRefs } from 'pinia';
import { computed, ref, watch } from 'vue';
import { projectApi, systemApi, worktreeApi } from '@/api/project';
import type { Project, Worktree } from '@/types/models';
import { useSettingsStore } from '@/stores/settings';
import type { EditorPreference } from '@/stores/settings';

const RECENT_PROJECTS_KEY = 'recent_projects';
const DEFAULT_MAX_RECENT_PROJECTS = 10;

// 优先级类型定义：1-5级，数字越大优先级越高
export type ProjectPriority = 1 | 2 | 3 | 4 | 5;

export const useProjectStore = defineStore('project', () => {
  const projects = ref<Project[]>([]);
  const currentProject = ref<Project | null>(null);
  const worktrees = ref<Worktree[]>([]);
  const loading = ref(false);
  const recentProjectIds = ref<string[]>(loadRecentProjectIds());
  const selectedWorktreeId = ref<string | null>(null);

  const hasProjects = computed(() => projects.value.length > 0);

  const settingsStore = useSettingsStore();
  const { recentProjectsLimit } = storeToRefs(settingsStore);
  const resolvedRecentLimit = computed(() => Math.max(recentProjectsLimit.value || DEFAULT_MAX_RECENT_PROJECTS, 1));

  watch(
    resolvedRecentLimit,
    limit => {
      enforceRecentLimit(limit);
    },
    { immediate: true },
  );

  const selectedWorktree = computed(() => {
    if (!selectedWorktreeId.value) {
      return null;
    }
    return worktrees.value.find(worktree => worktree.id === selectedWorktreeId.value) ?? null;
  });

  const recentProjects = computed(() => {
    const projectList = recentProjectIds.value
      .map(id => projects.value.find(p => p.id === id))
      .filter((p): p is Project => p !== undefined);

    // 按照优先级排序：优先级高的在前，没有优先级的保持原顺序在后
    return projectList.sort((a, b) => {
      const priorityA = a.priority;
      const priorityB = b.priority;

      // 如果两个都有优先级，按优先级降序排列
      if (priorityA && priorityB) {
        return priorityB - priorityA;
      }

      // 如果只有A有优先级，A排在前面
      if (priorityA && !priorityB) {
        return -1;
      }

      // 如果只有B有优先级，B排在前面
      if (!priorityA && priorityB) {
        return 1;
      }

      // 两个都没有优先级，保持原顺序（通过在原数组中的索引）
      const indexA = recentProjectIds.value.indexOf(a.id);
      const indexB = recentProjectIds.value.indexOf(b.id);
      return indexA - indexB;
    });
  });

  watch(worktrees, list => {
    if (selectedWorktreeId.value && !list.some(worktree => worktree.id === selectedWorktreeId.value)) {
      selectedWorktreeId.value = null;
    }
  });

  async function fetchProjects() {
    loading.value = true;
    try {
      const result = await projectApi.list();
      projects.value = result.items;
    } finally {
      loading.value = false;
    }
  }

  async function fetchProject(id: string) {
    loading.value = true;
    try {
      currentProject.value = await projectApi.get(id);
      selectedWorktreeId.value = null;
      await fetchWorktrees(id);
    } finally {
      loading.value = false;
    }
  }

  async function createProject(
    payload: { name: string; path: string; description?: string; hidePath: boolean },
  ) {
    const project = await projectApi.create(payload);
    projects.value.push(project);
    return project;
  }

  async function updateProject(
    id: string,
    payload: { name: string; description?: string; hidePath: boolean },
  ) {
    const project = await projectApi.update(id, payload);
    const index = projects.value.findIndex(item => item.id === id);
    if (index !== -1) {
      projects.value.splice(index, 1, project);
    }
    if (currentProject.value?.id === id) {
      currentProject.value = project;
    }
    return project;
  }

  async function deleteProject(id: string) {
    await projectApi.delete(id);
    projects.value = projects.value.filter(project => project.id !== id);
    if (currentProject.value?.id === id) {
      currentProject.value = null;
      worktrees.value = [];
      selectedWorktreeId.value = null;
    }
  }

  async function fetchWorktrees(projectId: string) {
    worktrees.value = await worktreeApi.list(projectId);
    refreshWorktreeCommitInfo(projectId);
  }

  async function refreshWorktreeCommitInfo(projectId: string) {
    try {
      const refreshed = await worktreeApi.refreshCommitInfo(projectId);
      if (currentProject.value?.id === projectId) {
        worktrees.value = refreshed;
      }
    } catch (error) {
      console.warn('Failed to refresh worktree commit info', error);
    }
  }

  async function createWorktree(
    projectId: string,
    payload: { branchName: string; baseBranch?: string; createBranch?: boolean },
  ) {
    const worktree = await worktreeApi.create(projectId, payload);
    // 创建成功后立即刷新列表，确保 UI 能及时更新
    await fetchWorktrees(projectId);
    return worktree;
  }

  async function deleteWorktree(id: string, force = false, deleteBranch = true) {
    await worktreeApi.delete(id, force, deleteBranch);
    worktrees.value = worktrees.value.filter(worktree => worktree.id !== id);
  }

  function updateWorktreeInList(id: string, updated: Worktree) {
    const index = worktrees.value.findIndex(worktree => worktree.id === id);
    if (index !== -1) {
      worktrees.value.splice(index, 1, updated);
    }
  }

  async function syncWorktrees(projectId: string) {
    await worktreeApi.sync(projectId);
    await fetchWorktrees(projectId);
  }

  async function openInExplorer(path: string) {
    await systemApi.openExplorer(path);
  }

  async function openInEditor(path: string, editor: EditorPreference, customCommand?: string) {
    await systemApi.openEditor({
      path,
      editor,
      customCommand,
    });
  }

  function setSelectedWorktree(worktreeId: string | null) {
    selectedWorktreeId.value = worktreeId;
  }

  function addRecentProject(projectId: string) {
    const index = recentProjectIds.value.indexOf(projectId);
    if (index > -1) {
      recentProjectIds.value.splice(index, 1);
    }
    recentProjectIds.value.unshift(projectId);
    enforceRecentLimit(resolvedRecentLimit.value);
    saveRecentProjectIds(recentProjectIds.value);
  }

  function enforceRecentLimit(limit: number) {
    const normalizedLimit = Math.max(Math.floor(limit ?? DEFAULT_MAX_RECENT_PROJECTS), 1);
    if (recentProjectIds.value.length > normalizedLimit) {
      recentProjectIds.value = recentProjectIds.value.slice(0, normalizedLimit);
      saveRecentProjectIds(recentProjectIds.value);
    }
  }

  function removeRecentProject(projectId: string) {
    const index = recentProjectIds.value.indexOf(projectId);
    if (index > -1) {
      recentProjectIds.value.splice(index, 1);
      saveRecentProjectIds(recentProjectIds.value);
    }
  }

  function getProjectPriority(projectId: string): ProjectPriority | null {
    const project = projects.value.find(p => p.id === projectId);
    return (project?.priority as ProjectPriority | null) ?? null;
  }

  function updateProjectInList(updatedProject: Project) {
    // 更新项目列表中的项目
    const index = projects.value.findIndex(p => p.id === updatedProject.id);
    if (index !== -1) {
      projects.value[index] = updatedProject;
    }

    // 如果是当前项目，也更新当前项目
    if (currentProject.value?.id === updatedProject.id) {
      currentProject.value = updatedProject;
    }
  }

  return {
    projects,
    currentProject,
    worktrees,
    selectedWorktree,
    selectedWorktreeId,
    loading,
    hasProjects,
    recentProjects,
    fetchProjects,
    fetchProject,
    createProject,
    updateProject,
    deleteProject,
    fetchWorktrees,
    refreshWorktreeCommitInfo,
    createWorktree,
    deleteWorktree,
    updateWorktreeInList,
    syncWorktrees,
    openInExplorer,
    openInEditor,
    addRecentProject,
    removeRecentProject,
    getProjectPriority,
    updateProjectInList,
    setSelectedWorktree,
  };
});

function loadRecentProjectIds(): string[] {
  try {
    const stored = localStorage.getItem(RECENT_PROJECTS_KEY);
    return stored ? JSON.parse(stored) : [];
  } catch {
    return [];
  }
}

function saveRecentProjectIds(ids: string[]) {
  try {
    localStorage.setItem(RECENT_PROJECTS_KEY, JSON.stringify(ids));
  } catch (error) {
    console.error('Failed to save recent projects:', error);
  }
}
