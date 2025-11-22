<template>
  <div
    class="notepad-container"
    :class="{ collapsed: isCollapsed }"
    :style="notepadStyle"
    @pointerdown.capture="handlePanelPointerDown"
  >
    <!-- 调整大小的拖拽条 -->
    <div class="resize-handle" @mousedown="startResize"></div>

    <!-- 折叠按钮 -->
    <div class="notepad-toggle" :title="shortcutTooltip" @click="toggleCollapse">
      <n-icon size="18">
        <component :is="isCollapsed ? ChevronLeftIcon : ChevronRightIcon" />
      </n-icon>
    </div>

    <!-- 记事板面板 -->
    <div v-if="!isCollapsed" class="notepad-panel">
      <div class="notepad-header">
        <!-- 全局/项目切换 -->
        <div class="scope-switcher">
          <n-button-group size="tiny">
            <n-button
              :type="currentScope === 'global' ? 'primary' : 'default'"
              @click="switchScope('global')"
            >
              {{ t('notepad.global') }}
            </n-button>
            <n-button
              :type="currentScope === 'project' ? 'primary' : 'default'"
              :disabled="!currentProjectId"
              @click="switchScope('project')"
            >
              {{ t('notepad.project') }}
            </n-button>
          </n-button-group>
        </div>

        <!-- 标签页 -->
        <div class="tab-bar">
          <Draggable
            v-model="tabs"
            item-key="id"
            tag="div"
            class="tab-list"
            :animation="150"
            ghost-class="tab-ghost"
            drag-class="tab-dragging"
            @end="handleTabDragEnd"
          >
            <template #item="{ element: tab }">
              <div
                class="tab-item"
                :class="{ active: tab.id === activeTabId }"
                @click="setActiveTab(tab.id)"
                @dblclick="handleRenameTab(tab)"
                @contextmenu.prevent="handleRenameTab(tab)"
              >
                <span class="tab-label-text">{{ tab.name }}</span>
                <button
                  v-if="tabs.length > 1"
                  class="tab-close-btn"
                  :title="t('notepad.closeTab')"
                  @click.stop="handleCloseTab(tab.id)"
                >
                  <n-icon size="12">
                    <component :is="CloseIcon" />
                  </n-icon>
                </button>
              </div>
            </template>
          </Draggable>
          <n-button size="tiny" quaternary circle @click="handleAddTab">
            <template #icon>
              <n-icon><component :is="AddIcon" /></n-icon>
            </template>
          </n-button>
        </div>
      </div>

      <div class="notepad-content">
        <n-input
          v-if="activeTab"
          v-model:value="activeTab.content"
          type="textarea"
          :placeholder="t('notepad.placeholder')"
          :autosize="{ minRows: 8, maxRows: 25 }"
          @update:value="handleContentChange"
        />
        <div v-if="activeTab && saveStatusText" class="save-status">
          <n-spin v-if="isSavingNow || isWaitingToSave" :size="12" :stroke-width="12" />
          <span>{{ saveStatusText }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, h } from 'vue';
import { useRoute } from 'vue-router';
import { NButton, NButtonGroup, NIcon, NInput, NSpin, useDialog, useMessage } from 'naive-ui';
import { useEventListener } from '@vueuse/core';
import { storeToRefs } from 'pinia';
import {
  ChevronBackOutline as ChevronLeftIcon,
  ChevronForwardOutline as ChevronRightIcon,
  Add as AddIcon,
  CloseOutline as CloseIcon,
} from '@vicons/ionicons5';
import Draggable from 'vuedraggable';
import { notepadApi } from '@/api/notepad';
import type { NotePad } from '@/types/models';
import { debounce } from '@/utils/debounce';
import { useSettingsStore } from '@/stores/settings';
import { usePanelStack } from '@/composables/usePanelStack';
import { useLocale } from '@/composables/useLocale';

const { t } = useLocale();
const message = useMessage();
const dialog = useDialog();
const route = useRoute();
const settingsStore = useSettingsStore();
const { notepadShortcut } = storeToRefs(settingsStore);

const getInitialCollapsedState = (): boolean => {
  const stored = localStorage.getItem('notepad-collapsed');
  return stored ? JSON.parse(stored) : true;
};

const getInitialWidth = (): number => {
  const stored = localStorage.getItem('notepad-width');
  return stored ? parseInt(stored, 10) : 280;
};

const isCollapsed = ref(getInitialCollapsedState());
const panelWidth = ref(getInitialWidth());
const { zIndex: notepadPanelZIndex, bringToFront: bringNotepadToFront } = usePanelStack('notepad-panel');
const notepadStyle = computed(() => ({
  width: `${panelWidth.value}px`,
  zIndex: notepadPanelZIndex.value,
}));
const tabs = ref<NotePad[]>([]);
const activeTabId = ref<string>('');
const currentScope = ref<'global' | 'project'>('global');
const pendingSave = ref(false);
const activeSaveRequests = ref(0);
const isSavingNow = computed(() => activeSaveRequests.value > 0);
const isWaitingToSave = computed(() => pendingSave.value && !isSavingNow.value);
const saveStatusText = computed(() => {
  if (isSavingNow.value) {
    return t('notepad.saving');
  }
  if (isWaitingToSave.value) {
    return t('notepad.willSave');
  }
  return '';
});
const ORDER_GAP = 1000;

// 获取当前项目ID
const currentProjectId = computed(() => {
  const id = route.params.id as string | undefined;
  return id || null;
});

const activeTab = computed(() => {
  return tabs.value.find((tab) => tab.id === activeTabId.value);
});

const setActiveTab = (tabId?: string | null) => {
  activeTabId.value = tabId ?? '';
};

const toggleCollapse = () => {
  isCollapsed.value = !isCollapsed.value;
  localStorage.setItem('notepad-collapsed', JSON.stringify(isCollapsed.value));
  if (!isCollapsed.value) {
    bringNotepadToFront();
  }
};

const handlePanelPointerDown = () => {
  bringNotepadToFront();
};
const shortcutTooltip = computed(() => t('notepad.shortcutLabel', { shortcut: notepadShortcut.value.display || notepadShortcut.value.code }));
const shortcutCode = computed(() => notepadShortcut.value.code);

if (typeof window !== 'undefined') {
  useEventListener(window, 'keydown', event => {
    if (event.defaultPrevented || event.repeat) {
      return;
    }
    if (!isNotepadShortcut(event)) {
      return;
    }
    const activeElement = (typeof document !== 'undefined' ? document.activeElement : null) as HTMLElement | null;
    if (isNotepadElement(activeElement) || isEditableElement(activeElement)) {
      return;
    }
    event.preventDefault();
    toggleCollapse();
  });
}

function isNotepadShortcut(event: KeyboardEvent) {
  if (event.metaKey || event.ctrlKey || event.altKey) {
    return false;
  }
  return event.code === shortcutCode.value;
}

function isNotepadElement(element: HTMLElement | null) {
  if (!element) {
    return false;
  }
  return Boolean(element.closest('.notepad-container'));
}

function isEditableElement(element: HTMLElement | null) {
  if (!element) {
    return false;
  }
  if (element.isContentEditable) {
    return true;
  }
  const tagName = element.tagName;
  if (tagName === 'INPUT' || tagName === 'TEXTAREA') {
    const input = element as HTMLInputElement | HTMLTextAreaElement;
    return !input.readOnly && !input.disabled;
  }
  return false;
}

// 调整大小功能
const isResizing = ref(false);

const startResize = (e: MouseEvent) => {
  isResizing.value = true;
  const startX = e.clientX;
  const startWidth = panelWidth.value;

  const handleMouseMove = (moveEvent: MouseEvent) => {
    if (!isResizing.value) return;
    const deltaX = startX - moveEvent.clientX;
    const newWidth = Math.max(200, Math.min(600, startWidth + deltaX));
    panelWidth.value = newWidth;
    localStorage.setItem('notepad-width', newWidth.toString());
  };

  const handleMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
  };

  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
};

// 切换作用域
const switchScope = async (scope: 'global' | 'project') => {
  if (scope === 'project' && !currentProjectId.value) return;
  currentScope.value = scope;
  await loadTabs();
};

const loadTabs = async () => {
  try {
    // 先清空当前激活的tab，避免切换作用域时显示旧数据
    activeTabId.value = '';

    const projectId =
      currentScope.value === 'project' && currentProjectId.value ? currentProjectId.value : undefined;
    const data = await notepadApi.list(projectId);
    tabs.value = [...data].sort((a, b) => a.orderIndex - b.orderIndex);

    // 如果没有标签，创建一个默认标签
    if (tabs.value.length === 0) {
      await handleAddTab();
    } else {
      // 激活第一个标签
      setActiveTab(tabs.value[0].id);
    }
  } catch (error) {
    message.error(t('notepad.loadFailed'));
    console.error(error);
  }
};

const handleAddTab = async () => {
  try {
    const projectId =
      currentScope.value === 'project' && currentProjectId.value ? currentProjectId.value : undefined;
    const createData: { projectId?: string; name: string; content: string } = {
      name: t('notepad.newTab'),
      content: '',
    };
    // 只有当 projectId 有值时才添加到请求中
    if (projectId) {
      createData.projectId = projectId;
    }
    const newTab = await notepadApi.create(createData);
    tabs.value.push(newTab);
    setActiveTab(newTab.id);
  } catch (error) {
    message.error(t('notepad.createFailed'));
    console.error(error);
  }
};

const handleCloseTab = async (tabId: string) => {
  try {
    await notepadApi.delete(tabId);
    const index = tabs.value.findIndex((tab) => tab.id === tabId);
    if (index > -1) {
      tabs.value.splice(index, 1);
    }
    if (activeTabId.value === tabId) {
      setActiveTab(tabs.value[0]?.id);
    }
  } catch (error) {
    message.error(t('notepad.deleteFailed'));
    console.error(error);
  }
};

const handleRenameTab = (tab: NotePad) => {
  let inputValue = tab.name;

  dialog.create({
    title: t('notepad.renameTitle'),
    content: () =>
      h(NInput, {
        defaultValue: tab.name,
        placeholder: t('notepad.renamePlaceholder'),
        onUpdateValue: (value: string) => {
          inputValue = value;
        },
      }),
    positiveText: t('common.confirm'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      const newName = inputValue.trim();
      if (newName && newName !== tab.name) {
        try {
          const updated = await notepadApi.update(tab.id, { name: newName });
          const index = tabs.value.findIndex((t) => t.id === tab.id);
          if (index > -1) {
            tabs.value[index] = updated;
          }
        } catch (error) {
          message.error(t('notepad.renameFailed'));
          console.error(error);
        }
      }
    },
  });
};

type DragEndEvent = {
  oldIndex?: number | null;
  newIndex?: number | null;
};

const calculateOrderIndex = (prev?: NotePad, next?: NotePad) => {
  if (prev && next) {
    if (prev.orderIndex === next.orderIndex) {
      return prev.orderIndex + ORDER_GAP / 2;
    }
    return (prev.orderIndex + next.orderIndex) / 2;
  }
  if (!prev && next) {
    return next.orderIndex - ORDER_GAP;
  }
  if (prev && !next) {
    return prev.orderIndex + ORDER_GAP;
  }
  return 0;
};

const revertTabOrder = (oldIndex: number, newIndex: number) => {
  if (oldIndex === newIndex) {
    return;
  }
  const [movedTab] = tabs.value.splice(newIndex, 1);
  if (movedTab) {
    tabs.value.splice(oldIndex, 0, movedTab);
  }
};

const handleTabDragEnd = async ({ oldIndex, newIndex }: DragEndEvent) => {
  if (
    oldIndex === undefined ||
    newIndex === undefined ||
    oldIndex === null ||
    newIndex === null ||
    oldIndex === newIndex
  ) {
    return;
  }

  const movedTab = tabs.value[newIndex];
  if (!movedTab) {
    return;
  }

  const prevTab = tabs.value[newIndex - 1];
  const nextTab = tabs.value[newIndex + 1];
  const previousOrderIndex = movedTab.orderIndex;
  const newOrderIndex = calculateOrderIndex(prevTab, nextTab);

  if (newOrderIndex === previousOrderIndex) {
    return;
  }

  movedTab.orderIndex = newOrderIndex;
  try {
    const updated = await notepadApi.move(movedTab.id, newOrderIndex);
    const targetIndex = tabs.value.findIndex((tab) => tab.id === updated.id);
    if (targetIndex > -1) {
      tabs.value[targetIndex] = updated;
    }
  } catch (error) {
    message.error(t('notepad.moveFailed'));
    movedTab.orderIndex = previousOrderIndex;
    revertTabOrder(oldIndex, newIndex);
    console.error(error);
  }
};

const saveContent = debounce(async (tabId: string, content: string) => {
  pendingSave.value = false;
  activeSaveRequests.value += 1;
  try {
    await notepadApi.update(tabId, { content });
  } catch (error) {
    message.error(t('notepad.saveFailed'));
    console.error(error);
  } finally {
    activeSaveRequests.value = Math.max(0, activeSaveRequests.value - 1);
  }
}, 3000);

const handleContentChange = (value: string) => {
  if (activeTab.value) {
    pendingSave.value = true;
    saveContent(activeTab.value.id, value);
  }
};

// 监听路由变化，如果当前是项目笔记且项目改变，重新加载
watch(
  () => currentProjectId.value,
  () => {
    if (currentScope.value === 'project') {
      loadTabs();
    }
  },
);

// 监听路由变化，回到项目列表页时自动折叠
watch(
  () => route.name,
  (newRouteName) => {
    if (newRouteName === 'projects') {
      isCollapsed.value = true;
      localStorage.setItem('notepad-collapsed', JSON.stringify(true));
    }
  },
);

onMounted(() => {
  loadTabs();
});
</script>

<style scoped>
.notepad-container {
  position: fixed;
  top: 80px;
  right: 0;
  bottom: 40px;
  background: var(--app-body-color, #fafafa);
  border-left: 1px solid var(--n-border-color, #e0e0e0);
  box-shadow: -2px 0 8px var(--n-box-shadow-color, rgba(0, 0, 0, 0.08));
  transition: transform 0.3s ease;
  display: flex;
  flex-direction: column;
}

.notepad-container.collapsed {
  transform: translateX(100%);
}

.resize-handle {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  cursor: ew-resize;
  background: transparent;
  transition: background-color 0.2s;
}

.resize-handle:hover {
  background: var(--n-color-target, var(--n-primary-color, #1890ff));
}

.notepad-toggle {
  position: absolute;
  left: -28px;
  top: 50%;
  transform: translateY(-50%);
  width: 28px;
  height: 56px;
  background: var(--app-surface-color, #fafafa);
  border: 1px solid var(--n-border-color, #e0e0e0);
  border-right: none;
  border-radius: 6px 0 0 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color 0.2s;
  box-shadow: -2px 0 4px var(--n-box-shadow-color, rgba(0, 0, 0, 0.05));
}

.notepad-toggle:hover {
  background: var(--app-body-color, #f0f0f0);
}

.notepad-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--app-surface-color, #ffffff);
}

.notepad-header {
  padding: 8px;
  border-bottom: 1px solid var(--n-border-color, #e0e0e0);
  background: var(--app-body-color, #fafafa);
}

.scope-switcher {
  margin-bottom: 8px;
  display: flex;
  justify-content: center;
}

.notepad-content {
  flex: 1;
  padding: 8px;
  overflow-y: auto;
  background: var(--app-surface-color, #ffffff);
}

.tab-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tab-list {
  display: flex;
  flex: 1;
  gap: 6px;
  overflow-x: auto;
  padding-bottom: 2px;
}

.tab-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border: 1px solid var(--n-border-color, #dedede);
  border-radius: 4px;
  font-size: 12px;
  background: var(--app-surface-color, #ffffff);
  cursor: grab;
  user-select: none;
  transition: border-color 0.2s, background-color 0.2s;
}

.tab-item.active {
  border-color: var(--n-primary-color, #1890ff);
  color: var(--n-primary-color, #1890ff);
  background: var(--n-primary-color-suppl, rgba(24, 144, 255, 0.1));
}

.tab-item:active {
  cursor: grabbing;
}

.tab-label-text {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tab-close-btn {
  border: none;
  background: transparent;
  padding: 0;
  display: flex;
  align-items: center;
  color: inherit;
  cursor: pointer;
}

.tab-close-btn:hover {
  color: var(--n-color-error, #ff4d4f);
}

.tab-ghost {
  opacity: 0.4;
}

.tab-dragging {
  cursor: grabbing;
}

.save-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--n-text-color-3, #8c8c8c);
  margin-top: 6px;
}

:deep(.n-input__textarea-el) {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace;
  font-size: 12px;
  line-height: 1.6;
  background: var(--app-surface-color, #ffffff);
}
</style>
