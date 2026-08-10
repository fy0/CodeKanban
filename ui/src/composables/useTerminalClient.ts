import { computed, onBeforeUnmount, watch, type Ref } from 'vue';
import { useTerminalStore, type TerminalCreateOptions } from '@/stores/terminal';
export type {
  BufferedTerminalMessage,
  ReplayBufferedMessagesResult,
  TerminalCreateOptions,
  ClientStatus,
  TerminalRemoteSnapshot,
  TerminalSerializedSnapshot,
  TerminalTabState,
  ServerMessage,
} from '@/stores/terminal';

export function useTerminalClient(projectIdRef: Ref<string>) {
  const store = useTerminalStore();

  const tabs = computed(() => store.getTabs(projectIdRef.value));

  const activeTabId = computed<string>({
    get: () => store.getActiveTabId(projectIdRef.value),
    set: value => {
      store.setActiveTab(projectIdRef.value, value);
    },
  });

  const hasSessions = computed(() => tabs.value.length > 0);

  watch(
    () => projectIdRef.value,
    (id, previousId) => {
      if (previousId && previousId !== id) {
        store.releaseProjectConnections(previousId);
      }
      if (!id) {
        return;
      }
      store.retainProjectConnections(id);
      store.prepareProject(id);
      void store.loadSessions(id);
    },
    { immediate: true }
  );

  onBeforeUnmount(() => {
    const id = projectIdRef.value;
    if (!id) {
      return;
    }
    store.releaseProjectConnections(id);
  });

  function reloadSessions() {
    const id = projectIdRef.value;
    if (!id) {
      return Promise.resolve();
    }
    store.prepareProject(id);
    return store.loadSessions(id);
  }

  return {
    tabs,
    activeTabId,
    hasSessions,
    emitter: store.emitter,
    reloadSessions,
    createSession(options: TerminalCreateOptions) {
      return store.createSession(projectIdRef.value, options);
    },
    renameSession(sessionId: string, title: string) {
      return store.renameSession(projectIdRef.value, sessionId, title);
    },
    closeSession(sessionId: string) {
      return store.closeSession(projectIdRef.value, sessionId);
    },
    setRenderMode(sessionId: string, mode: 'live' | 'snapshot' | null) {
      return store.setSessionRenderMode(projectIdRef.value, sessionId, mode);
    },
    setSnapshotInterval(sessionId: string, snapshotIntervalMs: number | null) {
      return store.setSessionSnapshotInterval(projectIdRef.value, sessionId, snapshotIntervalMs);
    },
    focusSession(sessionId: string) {
      return store.focusSession(projectIdRef.value, sessionId);
    },
    send: store.send,
    disconnectTab: store.disconnectTab,
    reorderTabs(fromIndex: number, toIndex: number) {
      store.reorderTabs(projectIdRef.value, fromIndex, toIndex);
    },
  };
}
