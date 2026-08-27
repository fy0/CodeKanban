import { computed, reactive, ref, watch } from 'vue';
import { defineStore } from 'pinia';
import Apis from '@/api';
import type { TerminalSession } from '@/types/models';

const TERMINAL_SESSION_POLL_MS = 5000;

function normalizeProjectIds(projectIds: string[]) {
  return Array.from(new Set(projectIds.map(projectId => projectId.trim()).filter(Boolean))).sort();
}

function normalizeSession(projectId: string, session: TerminalSession): TerminalSession {
  return {
    ...session,
    projectId: session.projectId || projectId,
  };
}

function arraysEqual(a: string[], b: string[]) {
  if (a.length !== b.length) {
    return false;
  }
  for (let index = 0; index < a.length; index += 1) {
    if (a[index] !== b[index]) {
      return false;
    }
  }
  return true;
}

export const useTerminalSessionSnapshotStore = defineStore('terminal-session-snapshot', () => {
  const scopes = reactive(new Map<string, string[]>());
  const sessionsByProject = ref<Map<string, TerminalSession[]>>(new Map());

  let refreshToken = 0;
  let pollTimer: number | null = null;
  let listenersBound = false;
  let refreshPromise: Promise<void> | null = null;

  const observedProjectIds = computed(() => {
    const ids = new Set<string>();
    scopes.forEach(projectIds => {
      projectIds.forEach(projectId => ids.add(projectId));
    });
    return Array.from(ids);
  });

  const sessionsById = computed(() => {
    const result = new Map<string, TerminalSession>();
    sessionsByProject.value.forEach(sessions => {
      sessions.forEach(session => {
        result.set(session.id, session);
      });
    });
    return result;
  });

  function stopPolling() {
    if (pollTimer != null && typeof window !== 'undefined') {
      window.clearTimeout(pollTimer);
      pollTimer = null;
    }
  }

  function schedulePolling(delay = TERMINAL_SESSION_POLL_MS) {
    stopPolling();
    if (!observedProjectIds.value.length || typeof window === 'undefined') {
      return;
    }
    pollTimer = window.setTimeout(async () => {
      pollTimer = null;
      await refresh('poll');
      schedulePolling();
    }, delay);
  }

  function handleVisibilityRefresh() {
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') {
      return;
    }
    void refresh('visibility');
    schedulePolling();
  }

  function bindWindowListeners() {
    if (listenersBound || typeof window === 'undefined') {
      return;
    }
    listenersBound = true;
    window.addEventListener('focus', handleVisibilityRefresh);
    window.addEventListener('online', handleVisibilityRefresh);
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', handleVisibilityRefresh);
    }
  }

  function unbindWindowListeners() {
    if (!listenersBound || typeof window === 'undefined') {
      return;
    }
    listenersBound = false;
    window.removeEventListener('focus', handleVisibilityRefresh);
    window.removeEventListener('online', handleVisibilityRefresh);
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', handleVisibilityRefresh);
    }
  }

  async function refresh(reason = 'manual') {
    if (refreshPromise) {
      return refreshPromise;
    }

    const projectIds = observedProjectIds.value;
    if (!projectIds.length) {
      sessionsByProject.value = new Map();
      return;
    }

    const token = ++refreshToken;
    const previous = sessionsByProject.value;

    refreshPromise = (async () => {
      const next = new Map<string, TerminalSession[]>();

      await Promise.all(
        projectIds.map(async projectId => {
          try {
            const response = await Apis.terminalSession
              .list({
                pathParams: { projectId },
                cacheFor: 0,
              })
              .send();
            const sessions = ((response?.items ?? []) as unknown as TerminalSession[]).map(
              session => normalizeSession(projectId, session)
            );
            next.set(projectId, sessions);
          } catch (error) {
            console.error('[Terminal Session Snapshot] Failed to refresh sessions', {
              projectId,
              reason,
              error,
            });
            next.set(projectId, previous.get(projectId) ?? []);
          }
        })
      );

      if (token === refreshToken) {
        sessionsByProject.value = next;
      }
    })().finally(() => {
      refreshPromise = null;
    });

    return refreshPromise;
  }

  function retainScope(scopeId: string, projectIds: string[]) {
    const normalizedScopeId = scopeId.trim();
    if (!normalizedScopeId) {
      return;
    }
    const normalizedProjectIds = normalizeProjectIds(projectIds);
    const current = scopes.get(normalizedScopeId) ?? [];
    if (arraysEqual(current, normalizedProjectIds)) {
      return;
    }
    scopes.set(normalizedScopeId, normalizedProjectIds);
  }

  function releaseScope(scopeId: string) {
    const normalizedScopeId = scopeId.trim();
    if (!normalizedScopeId) {
      return;
    }
    scopes.delete(normalizedScopeId);
  }

  watch(
    observedProjectIds,
    projectIds => {
      if (!projectIds.length) {
        stopPolling();
        unbindWindowListeners();
        sessionsByProject.value = new Map();
        return;
      }
      bindWindowListeners();
      void refresh('scope-change');
      schedulePolling();
    },
    { immediate: true }
  );

  return {
    observedProjectIds,
    sessionsByProject,
    sessionsById,
    retainScope,
    releaseScope,
    refresh,
  };
});
