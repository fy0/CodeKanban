import { computed, ref, watch, type Ref } from 'vue';
import { useIntervalFn } from '@vueuse/core';
import { webSessionApi, type PromptCacheRenewalState } from '@/api/webSession';
import type { WebSessionSummary } from '@/types/models';

type RenewalAction = 'cycle' | 'off';

export function usePromptCacheRenewal(session: Ref<WebSessionSummary | null>) {
  const state = ref<PromptCacheRenewalState>();
  const updating = ref(false);
  const detailsOpen = ref(false);
  const sessionKey = computed(() => {
    const value = session.value;
    return value?.agent === 'codex' ? `${value.projectId}/${value.id}` : '';
  });
  let latestRequest = 0;
  let fetching = false;
  let pendingActions: RenewalAction[] = [];

  async function refresh() {
    const value = session.value;
    const key = sessionKey.value;
    if (!value || !key || updating.value || fetching) return;
    const request = ++latestRequest;
    fetching = true;
    try {
      const response = await webSessionApi.promptCacheRenewal(value.projectId, value.id);
      if (request === latestRequest && key === sessionKey.value) state.value = response;
    } catch {
      return;
    } finally {
      if (request === latestRequest) fetching = false;
    }
  }

  async function setMode(action: RenewalAction) {
    const value = session.value;
    const key = sessionKey.value;
    if (!value || !key) return;
    const actions = pendingActions;
    if (action === 'off') actions.length = 0;
    actions.push(action);
    if (updating.value) return;
    updating.value = true;
    fetching = false;
    const request = ++latestRequest;
    const isCurrent = () => request === latestRequest && key === sessionKey.value;
    try {
      while (actions.length && isCurrent()) {
        const nextAction = actions.shift()!;
        try {
          const response = await webSessionApi.promptCacheRenewal(
            value.projectId,
            value.id,
            nextAction
          );
          if (isCurrent()) state.value = response;
        } catch {
          if (isCurrent() && state.value) {
            state.value = { ...state.value, lastError: 'connection_closed' };
          }
        }
      }
    } finally {
      if (request === latestRequest) updating.value = false;
    }
  }

  const { pause, resume } = useIntervalFn(() => void refresh(), 2000, { immediate: false });
  watch(
    sessionKey,
    () => {
      latestRequest++;
      fetching = false;
      pendingActions = [];
      updating.value = false;
      state.value = undefined;
      void refresh();
    },
    { immediate: true }
  );
  watch(
    () =>
      Boolean(
        sessionKey.value &&
          (state.value?.infinite ||
            state.value?.remaining ||
            (detailsOpen.value &&
              state.value?.availability !== 'disabled' &&
              state.value?.availability !== 'unsupported'))
      ),
    shouldPoll => (shouldPoll ? resume() : pause())
  );

  function setDetailsOpen(open: boolean) {
    detailsOpen.value = open;
    if (open) void refresh();
  }

  return { state, updating, setDetailsOpen, setMode };
}
