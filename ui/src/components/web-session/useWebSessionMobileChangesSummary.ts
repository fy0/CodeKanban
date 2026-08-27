import { computed, onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter } from 'vue';
import { useStorage } from '@vueuse/core';
import { fileManagerApi } from '@/api/fileManager';
import { useProjectStore } from '@/stores/project';
import {
  chooseGitChangesScope,
  formatGitChangesBadgeDelta,
  GIT_CHANGES_IGNORE_UNTRACKED_DEFAULT,
  GIT_CHANGES_IGNORE_UNTRACKED_STORAGE_KEY,
  shouldLoadGitChangesStats,
  shouldShowGitChangesBadge,
  type GitChangesBadgeSummary,
} from '@/components/changes/gitChangesSummary';
import { createGitChangesLoadController } from '@/components/changes/gitChangesLoadController';
import { gitOperationAvailable } from '@/utils/projectGitCapability';

const REFRESH_INTERVAL_MS = 10_000;
const STATS_TIMEOUT_MS = 5_000;

type Translate = (key: string) => string;

export type WebSessionMobileChangesSummaryOptions = {
  projectId: MaybeRefOrGetter<string>;
  isActive: MaybeRefOrGetter<boolean>;
  isMobile: MaybeRefOrGetter<boolean>;
  translate: Translate;
};

export function useWebSessionMobileChangesSummary({
  projectId,
  isActive,
  isMobile,
  translate,
}: WebSessionMobileChangesSummaryOptions) {
  const projectStore = useProjectStore();
  const ignoreUntracked = useStorage<boolean>(
    GIT_CHANGES_IGNORE_UNTRACKED_STORAGE_KEY,
    GIT_CHANGES_IGNORE_UNTRACKED_DEFAULT
  );
  const summary = ref<GitChangesBadgeSummary | null>(null);
  const scopeId = ref('');
  const scopeController = createGitChangesLoadController();
  const loadController = createGitChangesLoadController();
  let refreshTimer: number | null = null;

  const canLoad = computed(() => {
    const currentProjectId = toValue(projectId);
    return (
      toValue(isMobile) &&
      toValue(isActive) &&
      Boolean(currentProjectId) &&
      projectStore.currentProject?.id === currentProjectId &&
      !projectStore.projectDetailLoading &&
      gitOperationAvailable(projectStore.gitCapabilities, 'status', projectStore.selectedWorktreeId)
    );
  });

  const display = computed(() => {
    const current = summary.value ?? {
      count: 0,
      additions: 0,
      deletions: 0,
    };
    return {
      count: current.count,
      additions: formatGitChangesBadgeDelta('+', current.additions),
      deletions: formatGitChangesBadgeDelta('-', current.deletions),
    };
  });
  const loading = computed(() => summary.value?.state === 'loading');
  const incomplete = computed(
    () => summary.value?.state === 'partial' || summary.value?.state === 'timedOut'
  );
  const statusText = computed(() =>
    translate(
      summary.value?.state === 'timedOut' ? 'gitChanges.statsTimedOut' : 'gitChanges.statsPartial'
    )
  );
  const visible = computed(() => canLoad.value && shouldShowGitChangesBadge(summary.value));
  const label = computed(() => {
    const current = display.value;
    return `${translate('nav.changes')} ${current.count},${current.additions},${current.deletions}`;
  });

  function stopTimer() {
    if (refreshTimer != null && typeof window !== 'undefined') {
      window.clearInterval(refreshTimer);
    }
    refreshTimer = null;
  }

  function reset() {
    summary.value = null;
    scopeId.value = '';
  }

  async function resolveScope() {
    if (scopeId.value) {
      return scopeId.value;
    }
    const currentProjectId = toValue(projectId);
    const loadHandle = scopeController.begin();
    try {
      const scopes = await fileManagerApi.listScopes(currentProjectId, {
        signal: loadHandle.signal,
      });
      if (!scopeController.isCurrent(loadHandle)) {
        return '';
      }
      const scope = chooseGitChangesScope(scopes, {
        preferredWorktreeId: projectStore.selectedWorktreeId,
      });
      scopeId.value = scope?.id ?? '';
      return scopeId.value;
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        return '';
      }
      throw error;
    } finally {
      scopeController.release(loadHandle);
    }
  }

  async function load() {
    if (
      !canLoad.value ||
      (typeof document !== 'undefined' && document.visibilityState === 'hidden')
    ) {
      return;
    }
    const currentScopeId = await resolveScope().catch(() => '');
    const currentProjectId = toValue(projectId);
    if (!currentScopeId || !canLoad.value) {
      return;
    }
    const loadHandle = loadController.begin();
    const previousSummary = summary.value;
    try {
      const fastSummary = await fileManagerApi.changesSummary(currentProjectId, currentScopeId, {
        includeUntracked: !ignoreUntracked.value,
        withStats: false,
        signal: loadHandle.signal,
      });
      if (!loadController.isCurrent(loadHandle)) {
        return;
      }
      if (fastSummary.count <= 0) {
        summary.value = {
          count: 0,
          additions: 0,
          deletions: 0,
          state: 'complete',
          changeToken: fastSummary.changeToken,
          scopeId: currentScopeId,
        };
        return;
      }
      if (!shouldLoadGitChangesStats(previousSummary, currentScopeId, fastSummary.changeToken)) {
        return;
      }
      const retainedSummary =
        previousSummary?.scopeId === currentScopeId && previousSummary.state === 'complete'
          ? previousSummary
          : null;
      summary.value = {
        count: retainedSummary?.count ?? fastSummary.count,
        additions: retainedSummary?.additions ?? null,
        deletions: retainedSummary?.deletions ?? null,
        state: 'loading',
        changeToken: fastSummary.changeToken,
        scopeId: currentScopeId,
      };
      const statsSummary = await fileManagerApi.changesSummary(currentProjectId, currentScopeId, {
        includeUntracked: !ignoreUntracked.value,
        withStats: true,
        timeoutMs: STATS_TIMEOUT_MS,
        signal: loadHandle.signal,
      });
      if (!loadController.isCurrent(loadHandle)) {
        return;
      }
      summary.value = {
        count: statsSummary.count > 0 ? statsSummary.count : fastSummary.count,
        additions: statsSummary.statsComplete ? (statsSummary.additions ?? 0) : null,
        deletions: statsSummary.statsComplete ? (statsSummary.deletions ?? 0) : null,
        state: statsSummary.statsComplete
          ? 'complete'
          : statsSummary.statsTimedOut
            ? 'timedOut'
            : 'partial',
        changeToken: statsSummary.changeToken,
        scopeId: currentScopeId,
      };
    } catch (error) {
      if (
        loadController.isCurrent(loadHandle) &&
        !(error instanceof Error && error.name === 'AbortError')
      ) {
        summary.value = previousSummary;
      }
    } finally {
      loadController.release(loadHandle);
    }
  }

  function startTimer() {
    stopTimer();
    if (typeof window === 'undefined' || !canLoad.value) {
      return;
    }
    refreshTimer = window.setInterval(() => {
      void load();
    }, REFRESH_INTERVAL_MS);
  }

  async function initialize() {
    if (!canLoad.value) {
      return;
    }
    await load();
    startTimer();
  }

  watch(
    () =>
      [
        toValue(projectId),
        toValue(isActive),
        toValue(isMobile),
        projectStore.currentProject?.id,
        projectStore.projectDetailLoading,
        projectStore.selectedWorktreeId,
        gitOperationAvailable(
          projectStore.gitCapabilities,
          'status',
          projectStore.selectedWorktreeId
        ),
      ] as const,
    () => {
      stopTimer();
      scopeController.cancel();
      loadController.cancel();
      reset();
      void initialize();
    },
    { immediate: true }
  );

  watch(ignoreUntracked, () => {
    loadController.cancel();
    void load();
  });

  onScopeDispose(() => {
    stopTimer();
    scopeController.cancel();
    loadController.cancel();
  });

  return {
    display,
    loading,
    incomplete,
    statusText,
    visible,
    label,
    refresh: load,
  };
}
