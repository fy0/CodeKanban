<template>
  <aside ref="rootRef" class="docked-terminal-sidebar">
    <div class="terminal-resizer" :class="{ 'is-dragging': isResizing }" @mousedown="startResize">
      <div class="resizer-handle"></div>
    </div>

    <section
      class="terminal-sidebar-panel"
      :style="{ width: `${effectiveSidebarWidthPx}px` }"
      :aria-label="t('terminal.title')"
    >
      <header class="terminal-sidebar-header">
        <div class="terminal-sidebar-heading">
          <n-icon size="15"><TerminalOutline /></n-icon>
          <span>{{ t('terminal.title') }}</span>
          <span class="terminal-sidebar-count">{{ rows.length }}</span>
        </div>
        <n-tooltip placement="bottom" :delay="250">
          <template #trigger>
            <button type="button" class="terminal-sidebar-reset" @click="resetWidth">
              <n-icon size="14"><RefreshOutline /></n-icon>
            </button>
          </template>
          {{ t('common.reset') }}
        </n-tooltip>
      </header>

      <div class="terminal-sidebar-list">
        <TerminalSidebarRow
          v-for="row in rows"
          :key="row.id"
          :row="row"
          :action-options="actionOptions"
          @select="selectRow(row.id)"
          @action="handleRowAction(row.id, $event)"
        />
        <div v-if="!rows.length" class="terminal-sidebar-empty">
          {{ t('terminal.emptyGuideTitle') }}
        </div>
      </div>
    </section>
  </aside>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useStorage } from '@vueuse/core';
import { NIcon, NInput, NTooltip, type DropdownOption } from 'naive-ui';
import {
  ChevronForwardOutline,
  CreateOutline,
  CopyOutline,
  RefreshOutline,
  TerminalOutline,
} from '@vicons/ionicons5';
import { useDialog, useMessage } from 'naive-ui';
import { useLocale } from '@/composables/useLocale';
import { useSettingsStore } from '@/stores/settings';
import { useTerminalStore, type TerminalTabState } from '@/stores/terminal';
import { storeToRefs } from 'pinia';
import { getAssistantIconByType } from '@/utils/assistantIcon';
import {
  formatWebSessionDateTime,
  formatWebSessionSidebarTime,
} from '@/components/web-session/webSessionTimeFormat';
import TerminalSidebarRow, {
  type TerminalSidebarRowView,
} from '@/components/terminal/TerminalSidebarRow.vue';

const props = defineProps<{ projectId: string }>();
const { t, locale } = useLocale();
const message = useMessage();
const dialog = useDialog();
const settingsStore = useSettingsStore();
const { confirmBeforeTerminalClose } = storeToRefs(settingsStore);
const terminalStore = useTerminalStore();
const { setActiveTab, getActiveTabId, focusSession, renameSession, closeSession, createSession } =
  terminalStore;
const tabs = computed(() => terminalStore.getTabs(props.projectId));

const MIN_SIDEBAR_WIDTH = 176;
const MAX_SIDEBAR_WIDTH = 420;
const DEFAULT_SIDEBAR_WIDTH = 252;
const MIN_TERMINAL_MAIN_WIDTH = 420;
const sidebarWidthPx = useStorage('workspace-terminal-sidebar-width', DEFAULT_SIDEBAR_WIDTH);
const rootRef = ref<HTMLElement | null>(null);
const containerWidth = ref(0);
const isResizing = ref(false);

function clamp(min: number, value: number, max: number) {
  return Math.max(min, Math.min(max, value));
}

function updateContainerWidth() {
  const parent = rootRef.value?.parentElement;
  containerWidth.value = parent?.getBoundingClientRect().width ?? 0;
}

const maxWidthByContainer = computed(() => {
  if (!containerWidth.value) {
    return MAX_SIDEBAR_WIDTH;
  }
  return Math.min(
    MAX_SIDEBAR_WIDTH,
    Math.max(MIN_SIDEBAR_WIDTH, containerWidth.value - MIN_TERMINAL_MAIN_WIDTH)
  );
});

const effectiveSidebarWidthPx = computed(() =>
  clamp(MIN_SIDEBAR_WIDTH, Math.round(sidebarWidthPx.value), Math.round(maxWidthByContainer.value))
);

const actionOptions = computed<DropdownOption[]>(() => [
  {
    label: t('terminal.rename'),
    key: 'rename',
    icon: () => hIcon(CreateOutline),
  },
  {
    label: t('terminal.duplicateTab'),
    key: 'duplicate',
    icon: () => hIcon(CopyOutline),
  },
  {
    type: 'divider',
    key: 'divider',
  },
  {
    label: t('terminal.closeTerminal'),
    key: 'close',
    icon: () => hIcon(ChevronForwardOutline),
  },
]);

function hIcon(icon: unknown) {
  return h(NIcon, null, { default: () => h(icon as any) });
}

const rows = computed<TerminalSidebarRowView[]>(() =>
  tabs.value.map(tab => {
    const timestamp = parseTimestamp(tab.lastActive || tab.createdAt);
    const agentName = getAgentName(tab);
    return {
      id: tab.id,
      title: tab.title || t('terminal.defaultTerminalTitle'),
      agentName,
      iconHtml: getAssistantIconByType(
        tab.aiAssistant?.detected ? tab.aiAssistant.type : undefined
      ),
      active: tab.id === getActiveTabId(props.projectId),
      tooltip: [tab.title, agentName, tab.workingDir].filter(Boolean).join(' · '),
      activityTimeLabel: formatWebSessionSidebarTime(timestamp),
      activityTimeTitle: formatWebSessionDateTime(timestamp, locale.value),
    };
  })
);

function parseTimestamp(value?: string) {
  if (!value) {
    return 0;
  }
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function getAgentName(tab: TerminalTabState) {
  return tab.aiAssistant?.detected
    ? tab.aiAssistant.displayName || tab.aiAssistant.name || tab.aiAssistant.type
    : t('terminal.title');
}

function selectRow(sessionId: string) {
  setActiveTab(props.projectId, sessionId);
  focusSession(props.projectId, sessionId);
}

function promptRename(tab: TerminalTabState) {
  const inputValue = ref(tab.title);
  dialog.create({
    title: t('terminal.renameTitle'),
    content: () =>
      h(NInput, {
        value: inputValue.value,
        'onUpdate:value': (value: string) => {
          inputValue.value = value;
        },
        maxlength: 64,
        autofocus: true,
        placeholder: t('terminal.renamePlaceholder'),
      }),
    positiveText: t('terminal.save'),
    negativeText: t('common.cancel'),
    showIcon: false,
    onPositiveClick: async () => {
      const title = inputValue.value.trim();
      if (!title) {
        message.warning(t('terminal.emptyName'));
        return false;
      }
      if (title === tab.title) {
        return true;
      }
      try {
        await renameSession(props.projectId, tab.id, title);
        message.success(t('terminal.renameSuccess'));
        return true;
      } catch (error: any) {
        message.error(error?.message ?? t('terminal.renameFailed'));
        return false;
      }
    },
  });
}

async function handleRowAction(sessionId: string, key: string | number) {
  const tab = tabs.value.find(item => item.id === sessionId);
  if (!tab) {
    return;
  }
  if (key === 'rename') {
    promptRename(tab);
    return;
  }
  if (key === 'duplicate') {
    try {
      await createSession(props.projectId, {
        worktreeId: tab.worktreeId,
        workingDir: tab.workingDir,
        title: `${tab.title}${t('terminal.duplicateSuffix')}`,
        rows: tab.rows,
        cols: tab.cols,
        insertAfterSessionId: tab.id,
      });
      message.success(t('terminal.duplicateSuccess'));
    } catch (error: any) {
      message.error(error?.message ?? t('terminal.duplicateFailed'));
    }
    return;
  }
  if (key === 'close') {
    const close = () => closeSession(props.projectId, tab.id);
    if (confirmBeforeTerminalClose.value) {
      dialog.warning({
        title: t('terminal.confirmCloseTitle'),
        content: t('terminal.confirmCloseContent', { title: tab.title }),
        positiveText: t('terminal.confirmCloseButton'),
        negativeText: t('common.cancel'),
        onPositiveClick: close,
      });
    } else {
      await close();
    }
  }
}

function resetWidth() {
  sidebarWidthPx.value = DEFAULT_SIDEBAR_WIDTH;
}

function startResize(event: MouseEvent) {
  if (!containerWidth.value) {
    return;
  }
  event.preventDefault();
  isResizing.value = true;
  const startX = event.clientX;
  const startWidth = effectiveSidebarWidthPx.value;
  const onMouseMove = (moveEvent: MouseEvent) => {
    sidebarWidthPx.value = Math.round(
      clamp(MIN_SIDEBAR_WIDTH, startWidth + startX - moveEvent.clientX, maxWidthByContainer.value)
    );
  };
  const onMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  };
  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', onMouseUp);
  document.body.style.cursor = 'col-resize';
  document.body.style.userSelect = 'none';
}

let resizeObserver: ResizeObserver | null = null;
onMounted(() => {
  const parent = rootRef.value?.parentElement;
  if (parent && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(updateContainerWidth);
    resizeObserver.observe(parent);
  }
  updateContainerWidth();
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
});

watch(
  () => containerWidth.value,
  () => {
    sidebarWidthPx.value = clamp(
      MIN_SIDEBAR_WIDTH,
      sidebarWidthPx.value,
      maxWidthByContainer.value
    );
  }
);
</script>

<style scoped>
.docked-terminal-sidebar {
  display: flex;
  min-height: 0;
  flex: 0 0 auto;
}

.terminal-sidebar-panel {
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  border-left: 1px solid var(--n-border-color);
  background: var(--app-surface-color, var(--n-card-color, #fff));
}

.terminal-sidebar-header {
  min-height: 38px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 10px;
  border-bottom: 1px solid color-mix(in srgb, var(--n-border-color) 70%, transparent);
}

.terminal-sidebar-heading {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--n-text-color-2);
  font-size: 12px;
  font-weight: 700;
}

.terminal-sidebar-count {
  min-width: 18px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 5px;
  border-radius: 9px;
  background: color-mix(in srgb, var(--n-primary-color) 12%, transparent);
  color: var(--n-primary-color);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}

.terminal-sidebar-reset {
  width: 24px;
  height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--n-text-color-3);
  cursor: pointer;
}

.terminal-sidebar-reset:hover {
  background: color-mix(in srgb, var(--n-border-color) 45%, transparent);
  color: var(--n-text-color-1);
}

.terminal-sidebar-list {
  min-height: 0;
  flex: 1;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding: 7px;
}

.terminal-sidebar-empty {
  min-height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--n-text-color-3);
  font-size: 12px;
}

.terminal-resizer {
  flex: 0 0 6px;
  width: 6px;
  margin: 0 -3px;
  position: relative;
  z-index: 2;
  cursor: col-resize;
}

.resizer-handle {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 2px;
  height: 24px;
  border-radius: 1px;
  background: transparent;
  opacity: 0;
  transform: translate(-50%, -50%);
  transition:
    background-color 0.15s ease,
    height 0.15s ease,
    opacity 0.15s ease;
}

.terminal-resizer:hover .resizer-handle {
  height: 40px;
  background: var(--n-border-color, #d0d0d0);
  opacity: 1;
}

.terminal-resizer.is-dragging .resizer-handle {
  height: 60px;
  background: var(--n-primary-color, #3b82f6);
  opacity: 1;
}
</style>
