<template>
  <div
    class="notepad-container"
    :class="{ collapsed: isCollapsed }"
    :style="{ width: `${panelWidth}px` }"
  >
    <!-- 调整大小的拖拽条 -->
    <div class="resize-handle" @mousedown="startResize"></div>

    <!-- 折叠按钮 -->
    <div class="notepad-toggle" @click="toggleCollapse">
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
              全局
            </n-button>
            <n-button
              :type="currentScope === 'project' ? 'primary' : 'default'"
              :disabled="!currentProjectId"
              @click="switchScope('project')"
            >
              项目
            </n-button>
          </n-button-group>
        </div>

        <!-- 标签页 -->
        <n-tabs
          v-model:value="activeTabId"
          type="card"
          closable
          size="small"
          @close="handleCloseTab"
        >
          <n-tab-pane
            v-for="tab in sortedTabs"
            :key="tab.id"
            :name="tab.id"
            :tab="tab.name"
            :closable="sortedTabs.length > 1"
          >
            <template #tab>
              <div class="tab-label" @dblclick="handleRenameTab(tab)">
                {{ tab.name }}
              </div>
            </template>
          </n-tab-pane>
          <template #suffix>
            <n-button size="tiny" quaternary circle @click="handleAddTab">
              <template #icon>
                <n-icon><component :is="AddIcon" /></n-icon>
              </template>
            </n-button>
          </template>
        </n-tabs>
      </div>

      <div class="notepad-content">
        <n-input
          v-if="activeTab"
          v-model:value="activeTab.content"
          type="textarea"
          placeholder="在这里记录你的想法..."
          :autosize="{ minRows: 8, maxRows: 25 }"
          @update:value="handleContentChange"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, h } from 'vue';
import { useRoute } from 'vue-router';
import {
  NTabs,
  NTabPane,
  NButton,
  NButtonGroup,
  NIcon,
  NInput,
  useDialog,
  useMessage,
} from 'naive-ui';
import {
  ChevronBackOutline as ChevronLeftIcon,
  ChevronForwardOutline as ChevronRightIcon,
  Add as AddIcon,
} from '@vicons/ionicons5';
import { notepadApi } from '@/api/notepad';
import type { NotePad } from '@/types/models';
import { debounce } from '@/utils/debounce';

const message = useMessage();
const dialog = useDialog();
const route = useRoute();

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
const tabs = ref<NotePad[]>([]);
const activeTabId = ref<string>('');
const currentScope = ref<'global' | 'project'>('global');

// 获取当前项目ID
const currentProjectId = computed(() => {
  const id = route.params.id as string | undefined;
  return id || null;
});

const sortedTabs = computed(() => {
  return [...tabs.value].sort((a, b) => a.orderIndex - b.orderIndex);
});

const activeTab = computed(() => {
  return tabs.value.find((tab) => tab.id === activeTabId.value);
});

const toggleCollapse = () => {
  isCollapsed.value = !isCollapsed.value;
  localStorage.setItem('notepad-collapsed', JSON.stringify(isCollapsed.value));
};

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
    const projectId = currentScope.value === 'project' ? currentProjectId.value! : undefined;
    const data = await notepadApi.list(projectId);
    tabs.value = data;

    // 如果没有标签，创建一个默认标签
    if (tabs.value.length === 0) {
      await handleAddTab();
    } else if (!activeTabId.value || !tabs.value.find((t) => t.id === activeTabId.value)) {
      activeTabId.value = tabs.value[0].id;
    }
  } catch (error) {
    message.error('加载记事板失败');
    console.error(error);
  }
};

const handleAddTab = async () => {
  try {
    const projectId = currentScope.value === 'project' ? currentProjectId.value! : undefined;
    const createData: { projectId?: string; name: string; content: string } = {
      name: '新标签',
      content: '',
    };
    // 只有当 projectId 有值时才添加到请求中
    if (projectId) {
      createData.projectId = projectId;
    }
    const newTab = await notepadApi.create(createData);
    tabs.value.push(newTab);
    activeTabId.value = newTab.id;
  } catch (error) {
    message.error('创建标签失败');
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
    if (activeTabId.value === tabId && tabs.value.length > 0) {
      activeTabId.value = tabs.value[0].id;
    }
  } catch (error) {
    message.error('删除标签失败');
    console.error(error);
  }
};

const handleRenameTab = (tab: NotePad) => {
  let inputValue = tab.name;

  dialog.create({
    title: '重命名标签',
    content: () =>
      h(NInput, {
        defaultValue: tab.name,
        placeholder: '请输入标签名称',
        onUpdateValue: (value: string) => {
          inputValue = value;
        },
      }),
    positiveText: '确定',
    negativeText: '取消',
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
          message.error('重命名失败');
          console.error(error);
        }
      }
    },
  });
};

const saveContent = debounce(async (tabId: string, content: string) => {
  try {
    await notepadApi.update(tabId, { content });
  } catch (error) {
    message.error('保存失败');
    console.error(error);
  }
}, 1000);

const handleContentChange = (value: string) => {
  if (activeTab.value) {
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
  background: #fafafa;
  border-left: 1px solid #e0e0e0;
  box-shadow: -2px 0 8px rgba(0, 0, 0, 0.08);
  z-index: 999;
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
  background: #1890ff;
}

.notepad-toggle {
  position: absolute;
  left: -28px;
  top: 50%;
  transform: translateY(-50%);
  width: 28px;
  height: 56px;
  background: #fafafa;
  border: 1px solid #e0e0e0;
  border-right: none;
  border-radius: 6px 0 0 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color 0.2s;
  box-shadow: -2px 0 4px rgba(0, 0, 0, 0.05);
}

.notepad-toggle:hover {
  background: #f0f0f0;
}

.notepad-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #ffffff;
}

.notepad-header {
  padding: 8px;
  border-bottom: 1px solid #e0e0e0;
  background: #fafafa;
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
  background: #ffffff;
}

.tab-label {
  padding: 2px 6px;
  user-select: none;
  font-size: 12px;
}

:deep(.n-input__textarea-el) {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace;
  font-size: 12px;
  line-height: 1.6;
  background: #ffffff;
}

:deep(.n-tabs .n-tabs-tab) {
  font-size: 12px;
  padding: 4px 8px;
}

:deep(.n-tabs-nav) {
  font-size: 12px;
}
</style>
