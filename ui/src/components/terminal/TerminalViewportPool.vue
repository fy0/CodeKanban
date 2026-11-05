<template>
  <TerminalViewport
    v-for="tab in mountedTabs"
    v-show="isActive && tab.id === activeId"
    :key="tab.id"
    :tab="tab"
    :emitter="emitter"
    :send="send"
    :should-auto-focus="shouldAutoFocus && isActive && tab.id === activeId"
    :is-mobile="isMobile"
    :history-replay-enabled="historyReplayEnabled"
  />
</template>

<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue';
import type EventEmitter from 'eventemitter3';
import type { TerminalTabState } from '@/stores/terminal';
import TerminalViewport from './TerminalViewport.vue';

const props = defineProps<{
  projectId: string;
  tabs: TerminalTabState[];
  activeId: string;
  isActive: boolean;
  emitter: EventEmitter;
  send: (sessionId: string, payload: { type: string; [key: string]: unknown }) => void;
  shouldAutoFocus?: boolean;
  isMobile?: boolean;
  historyReplayEnabled?: boolean;
}>();

// Bound recoverable renderers. Live terminals without server history must keep
// their visited viewport: evicting them would discard the only copy of the screen.
const MAX_MOUNTED_VIEWPORTS = 3;
const recentIds = shallowRef<string[]>([]);
const tabsById = computed(() => new Map(props.tabs.map(tab => [tab.id, tab])));
const evictableIds = computed(
  () =>
    new Set(
      props.tabs
        .filter(
          tab =>
            props.historyReplayEnabled ||
            (tab.renderMode === 'snapshot' && tab.connectionRenderMode === 'snapshot')
        )
        .map(tab => tab.id)
    )
);

watch(
  [() => props.projectId, () => props.activeId, () => props.isActive, tabsById, evictableIds],
  ([projectId, activeId, isActive, byId, evictable], previous) => {
    let next = previous && previous[0] !== projectId ? [] : recentIds.value;
    next = next.filter(id => byId.has(id));
    if (isActive && byId.has(activeId)) {
      next = [activeId, ...next.filter(id => id !== activeId)];
    }
    for (let index = next.length - 1; next.length > MAX_MOUNTED_VIEWPORTS && index >= 0; index--) {
      if (next[index] !== activeId && evictable.has(next[index]!)) next.splice(index, 1);
    }
    if (next.length !== recentIds.value.length || next.some((id, i) => id !== recentIds.value[i])) {
      recentIds.value = next;
    }
  },
  { immediate: true }
);

const mountedTabs = computed(() =>
  recentIds.value.map(id => tabsById.value.get(id)).filter((tab): tab is TerminalTabState => !!tab)
);
</script>
