import { defineStore, storeToRefs } from 'pinia';
import { computed, ref, watch } from 'vue';
import { projectApi, systemApi, worktreeApi } from '@/api/project';
import type { GitCapabilityResult, Project, Worktree } from '@/types/models';
import { useSettingsStore } from '@/stores/settings';
import type { EditorPreference } from '@/stores/settings';
import { gitOperationAvailable } from '@/utils/projectGitCapability';

const DEFAULT_MAX_RECENT_PROJECTS = 10;
const ACCESS_TOUCH_THROTTLE_MS = 60_000;

// 优先级类型定义：1-5级，数字越大优先级越高
export type ProjectPriority = 1 | 2 | 3 | 4 | 5;

export const useProjectStore = defineStore('project', () => {
  const projects = ref<Project[]>([]);
  const currentProject = ref<Project | null>(null);
  const worktrees = ref<Worktree[]>([]);
  const gitCapabilities = ref<GitCapabilityResult | null>(null);
  const projectsLoading = ref(false);
  const projectDetailLoading = ref(false);
  const loading = computed(() => projectsLoading.value || projectDetailLoading.value);
  const selectedWorktreeId = ref<string | null>(null);
  let projectsFlight: Promise<void> | null = null;
  let projectLoadGeneration = 0;
  const accessTouchedAt = new Map<string, number>();

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
    if (projectsFlight) {
      if (!options.silent) {
        projectsLoading.value = true;
      }
      try {
        await projectsFlight;
      } finally {
        if (!options.silent) {
          projectsLoading.value = false;
        }
      }
      return;
    }
    if (!options.silent) {
      projectsLoading.value = true;
    }
    projectsFlight = (async () => {
      const result = await projectApi.list();
      replaceProjectList(result.items);
    })();
    try {
      await projectsFlight;
    } finally {
      projectsFlight = null;
      if (!options.silent) {
        projectsLoading.value = false;
      }
    }
  }

  async function fetchProject(id: string) {
    const generation = ++projectLoadGeneration;
    projectDetailLoading.value = true;
    try {
      // Clear stale worktrees immediately when switching projects to avoid
      // temporarily showing the previous project's worktrees (which can lead to
      // actions using an outdated worktreeId).
      if (currentProject.value?.id !== id) {
        worktrees.value = [];
        gitCapabilities.value = null;
      }
      const [project, nextWorktrees] = await Promise.all([
        projectApi.get(id),
        worktreeApi.list(id),
      ]);
      if (generation !== projectLoadGeneration) {
        return;
      }
      currentProject.value = project;
      worktrees.value = nextWorktrees;
      selectedWorktreeId.value = null;
      void reconcileProjectWorktrees(id, generation);
    } finally {
      if (generation === projectLoadGeneration) {
        projectDetailLoading.value = false;
      }
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
      gitCapabilities.value = null;
      selectedWorktreeId.value = null;
    }
  }

  async function fetchWorktrees(projectId: string) {
    const nextWorktrees = await worktreeApi.list(projectId);
    if (currentProject.value?.id === projectId) {
      worktrees.value = nextWorktrees;
    }
    if (currentProject.value?.id === projectId) {
      await fetchGitCapabilities(projectId);
    }
  }

  async function reconcileProjectWorktrees(projectId: string, generation: number) {
    await fetchGitCapabilities(projectId);
    try {
      const synced = await worktreeApi.sync(projectId);
      if (generation !== projectLoadGeneration || currentProject.value?.id !== projectId) {
        return;
      }
      if (!Array.isArray(synced)) {
        return;
      }
      worktrees.value = synced;
      await fetchGitCapabilities(projectId);
    } catch (error) {
      console.warn('Failed to sync worktrees', error);
    }
  }

  async function fetchGitCapabilities(projectId: string) {
    try {
      const capabilities = await projectApi.gitCapabilities(projectId);
      if (currentProject.value?.id === projectId) {
        gitCapabilities.value = capabilities;
      }
      return capabilities;
    } catch (error) {
      if (currentProject.value?.id === projectId) {
        gitCapabilities.value = null;
      }
      console.warn('Failed to load Git capabilities', error);
      return null;
    }
  }

  async function refreshWorktreeCommitInfo(projectId: string) {
    if (
      currentProject.value?.id === projectId &&
      !gitOperationAvailable(gitCapabilities.value, 'status')
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

  async function refreshWorktreeStatus(worktreeId: string) {
    const updated = await worktreeApi.refreshStatus(worktreeId);
    if (currentProject.value?.id === updated.projectId) {
      updateWorktreeInList(worktreeId, updated);
    }
    return updated;
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
    const synced = await worktreeApi.sync(projectId);
    if (currentProject.value?.id === projectId) {
      worktrees.value = synced;
      await fetchGitCapabilities(projectId);
    }
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
    const now = Date.now();
    const lastTouchedAt = accessTouchedAt.get(projectId) ?? 0;
    if (now - lastTouchedAt < ACCESS_TOUCH_THROTTLE_MS) {
      return;
    }
    accessTouchedAt.set(projectId, now);
    void projectApi
      .markAccess(projectId)
      .then(project => {
        updateProjectInList(project);
      })
      .catch(error => {
        accessTouchedAt.delete(projectId);
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
    gitCapabilities,
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
    fetchGitCapabilities,
    refreshWorktreeCommitInfo,
    refreshWorktreeStatus,
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
