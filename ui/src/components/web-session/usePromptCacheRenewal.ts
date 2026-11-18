import { computed, ref, watch, type Ref } from 'vue';
import { useIntervalFn } from '@vueuse/core';
import { webSessionApi, type PromptCacheRenewalState } from '@/api/webSession';
import type { WebSessionSummary } from '@/types/models';

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

  async function refresh(action?: 'cycle' | 'off') {
    const value = session.value;
    const key = sessionKey.value;
    if (!value || !key || updating.value || (!action && fetching)) return;
    const request = ++latestRequest;
    if (action) updating.value = true;
    fetching = true;
    try {
      const response = await webSessionApi.promptCacheRenewal(value.projectId, value.id, action);
      if (request === latestRequest && key === sessionKey.value) state.value = response;
    } catch {
      if (action && request === latestRequest && state.value) {
        state.value = { ...state.value, lastError: 'connection_closed' };
      }
    } finally {
      if (request === latestRequest) {
        fetching = false;
        updating.value = false;
      }
    }
  }

  const { pause, resume } = useIntervalFn(() => void refresh(), 2000, { immediate: false });
  watch(
    sessionKey,
    () => {
      latestRequest++;
      fetching = false;
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

  return { state, updating, setDetailsOpen, setMode: (action: 'cycle' | 'off') => refresh(action) };
}
