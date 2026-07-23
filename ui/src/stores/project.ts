import { defineStore, storeToRefs } from 'pinia';
import { computed, ref, watch } from 'vue';
import { projectApi, systemApi, worktreeApi } from '@/api/project';
import type { Project, Worktree } from '@/types/models';
import { useSettingsStore } from '@/stores/settings';
import type { EditorPreference } from '@/stores/settings';
import { projectSupportsGit } from '@/utils/projectGitCapability';

const DEFAULT_MAX_RECENT_PROJECTS = 10;

// 优先级类型定义：1-5级，数字越大优先级越高
export type ProjectPriority = 1 | 2 | 3 | 4 | 5;

export const useProjectStore = defineStore('project', () => {
  const projects = ref<Project[]>([]);
  const currentProject = ref<Project | null>(null);
  const worktrees = ref<Worktree[]>([]);
  const projectsLoading = ref(false);
  const projectDetailLoading = ref(false);
  const loading = computed(() => projectsLoading.value || projectDetailLoading.value);
  const selectedWorktreeId = ref<string | null>(null);

  const hasProjects = computed(() => projects.value.length > 0);

  const settingsStore = useSettingsStore();
  const { recentProjectsLimit } = storeToRefs(settingsStore);
  const resolvedRecentLimit = computed(() =>
    Math.max(recentProjectsLimit.value || DEFAULT_MAX_RECENT_PROJECTS, 1)
  );

  const selectedWorktree = computed(() => {
    if (!selectedWorktreeId.value) {
      return null;
    }
    return worktrees.value.find(worktree => worktree.id === selectedWorktreeId.value) ?? null;
  });

  const recentProjects = computed(() =>
    [...projects.value]
      .filter(project => Boolean(project.lastAccessedAt))
      .sort(compareRecentProjects)
      .slice(0, resolvedRecentLimit.value)
  );

  watch(worktrees, list => {
    if (
      selectedWorktreeId.value &&
      !list.some(worktree => worktree.id === selectedWorktreeId.value)
    ) {
      selectedWorktreeId.value = null;
    }
  });

  async function fetchProjects(options: { silent?: boolean } = {}) {
    if (!options.silent) {
      projectsLoading.value = true;
    }
    try {
      const result = await projectApi.list();
      replaceProjectList(result.items);
    } finally {
      if (!options.silent) {
        projectsLoading.value = false;
      }
    }
  }

  async function fetchProject(id: string) {
    projectDetailLoading.value = true;
    try {
      // Clear stale worktrees immediately when switching projects to avoid
      // temporarily showing the previous project's worktrees (which can lead to
      // actions using an outdated worktreeId).
      if (currentProject.value?.id !== id) {
        worktrees.value = [];
      }
      currentProject.value = await projectApi.get(id);
      selectedWorktreeId.value = null;
      await fetchWorktrees(id);
    } finally {
      projectDetailLoading.value = false;
    }
  }

  async function createProject(payload: {
    name: string;
    path: string;
    description?: string;
    hidePath: boolean;
  }) {
    const project = await projectApi.create(payload);
    updateProjectInList(project);
    return project;
  }

  async function updateProject(
    id: string,
    payload: { name: string; description?: string; hidePath: boolean }
  ) {
    const project = await projectApi.update(id, payload);
    updateProjectInList(project);
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
    if (
      currentProject.value?.id === projectId &&
      projectSupportsGit(currentProject.value, worktrees.value)
    ) {
      void refreshWorktreeCommitInfo(projectId);
    }
  }

  async function refreshWorktreeCommitInfo(projectId: string) {
    if (
      currentProject.value?.id === projectId &&
      !projectSupportsGit(currentProject.value, worktrees.value)
    ) {
      return;
    }

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
    payload: {
      branchName: string;
      baseBranch?: string;
      createBranch?: boolean;
      location?: 'project' | 'global';
      globalBaseDirOverride?: string;
    }
  ) {
    const worktree = await worktreeApi.create(projectId, payload);
    // 创建成功后立即刷新列表，确保 UI 能及时更新
    await fetchWorktrees(projectId);
    await fetchProject(projectId);
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
    void projectApi
      .markAccess(projectId)
      .then(project => {
        updateProjectInList(project);
      })
      .catch(error => {
        console.warn('Failed to record project access:', error);
        void fetchProjects({ silent: true });
      });
  }

  function removeRecentProject(projectId: string) {
    void projectApi
      .clearAccess(projectId)
      .then(project => {
        updateProjectInList(project);
      })
      .catch(error => {
        console.warn('Failed to clear project access:', error);
        void fetchProjects({ silent: true });
      });
  }

  function getProjectPriority(projectId: string): ProjectPriority | null {
    const project = projects.value.find(p => p.id === projectId);
    return (project?.priority as ProjectPriority | null) ?? null;
  }

  function updateProjectInList(updatedProject: Project) {
    const index = projects.value.findIndex(project => project.id === updatedProject.id);
    if (index !== -1) {
      projects.value.splice(index, 1, updatedProject);
    } else {
      projects.value.push(updatedProject);
    }

    if (currentProject.value?.id === updatedProject.id) {
      currentProject.value = updatedProject;
    }
  }

  function replaceProjectList(nextProjects: Project[]) {
    projects.value = [...nextProjects];
    if (currentProject.value) {
      const nextCurrentProject = projects.value.find(
        project => project.id === currentProject.value?.id
      );
      if (nextCurrentProject) {
        currentProject.value = nextCurrentProject;
      }
    }
  }

  return {
    projects,
    currentProject,
    worktrees,
    selectedWorktree,
    selectedWorktreeId,
    projectsLoading,
    projectDetailLoading,
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
    replaceProjectList,
    setSelectedWorktree,
  };
});

function compareRecentProjects(left: Project, right: Project) {
  const priorityComparison = compareProjectPriority(left, right);
  if (priorityComparison !== 0) {
    return priorityComparison;
  }

  const accessComparison = getTimestamp(right.lastAccessedAt) - getTimestamp(left.lastAccessedAt);
  if (accessComparison !== 0) {
    return accessComparison;
  }

  return getTimestamp(right.createdAt) - getTimestamp(left.createdAt);
}

function compareProjectPriority(left: Project, right: Project) {
  const leftPriority = left.priority ?? 0;
  const rightPriority = right.priority ?? 0;
  return rightPriority - leftPriority;
}

function getTimestamp(value: string | null | undefined) {
  if (!value) {
    return 0;
  }
  const timestamp = new Date(value).getTime();
  return Number.isFinite(timestamp) ? timestamp : 0;
}
