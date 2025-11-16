<template>
  <div class="general-settings-page">
    <n-page-header @back="handleBack">
      <template #title>
        <n-space align="center">
          <n-icon size="24">
            <SettingsOutline />
          </n-icon>
          <span>总设置</span>
        </n-space>
      </template>
      <template #extra>
        <n-space align="center">
          <n-button tertiary @click="handleResetTheme">
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
            恢复默认主题
          </n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-space vertical size="large">
      <n-card title="主题设置" size="huge">
        <n-form label-placement="left" label-width="120">
          <n-form-item label="主色调">
            <n-color-picker v-model:value="primaryColor" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item label="界面背景色">
            <n-color-picker v-model:value="bodyColor" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item label="卡片背景色">
            <n-color-picker v-model:value="surfaceColor" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
        </n-form>
      </n-card>


      <n-card title="项目与终端" size="huge">
        <n-form label-placement="left" label-width="160">
          <n-form-item label="最近项目数量">
            <n-space vertical size="small">
              <n-input-number v-model:value="recentProjectsLimitValue" :min="1" :max="20" :step="1" />
              <span class="form-tip">控制“最近项目”列表展示的数量</span>
            </n-space>
          </n-form-item>
          <n-form-item label="单项目终端上限">
            <n-space vertical size="small">
              <n-input-number v-model:value="terminalLimitValue" :min="1" :max="24" :step="1" />
              <span class="form-tip">限制每个项目可同时打开的终端标签数</span>
            </n-space>
          </n-form-item>
          <n-form-item label="终端关闭确认">
            <n-space vertical size="small">
              <n-switch v-model:value="confirmTerminalCloseValue" />
              <span class="form-tip">关闭终端前弹窗确认，避免误操作</span>
            </n-space>
          </n-form-item>
          <n-form-item label="终端快捷键">
            <n-space vertical size="small">
              <n-input
                :value="terminalShortcutValue"
                readonly
                :status="getShortcutStatus('terminal')"
                placeholder="按“记录新按键”后按下目标按键"
              >
                <template #suffix>
                  <span class="shortcut-hint">
                    {{ getShortcutHint('terminal') }}
                  </span>
                </template>
              </n-input>
              <n-space>
                <n-button size="small" @click="handleStartShortcutCapture('terminal')">
                  {{ isCapturing('terminal') ? '正在记录...' : '记录新按键' }}
                </n-button>
                <n-button
                  size="small"
                  text
                  :disabled="isTerminalShortcutDefault"
                  @click="handleResetShortcut('terminal')"
                >
                  恢复默认
                </n-button>
              </n-space>
              <span class="form-tip">用于展开/收起底部终端面板，默认 `</span>
            </n-space>
          </n-form-item>
          <n-form-item label="记事板快捷键">
            <n-space vertical size="small">
              <n-input
                :value="notepadShortcutValue"
                readonly
                :status="getShortcutStatus('notepad')"
                placeholder="按“记录新按键”后按下目标按键"
              >
                <template #suffix>
                  <span class="shortcut-hint">
                    {{ getShortcutHint('notepad') }}
                  </span>
                </template>
              </n-input>
              <n-space>
                <n-button size="small" @click="handleStartShortcutCapture('notepad')">
                  {{ isCapturing('notepad') ? '正在记录...' : '记录新按键' }}
                </n-button>
                <n-button
                  size="small"
                  text
                  :disabled="isNotepadShortcutDefault"
                  @click="handleResetShortcut('notepad')"
                >
                  恢复默认
                </n-button>
              </n-space>
              <span class="form-tip">用于展开/收起右侧记事板，默认 1</span>
            </n-space>
          </n-form-item>
          <n-form-item label="默认编辑器">
            <n-space vertical size="small">
              <n-select
                v-model:value="defaultEditorValue"
                :options="editorOptions"
                style="max-width: 240px"
              />
              <span class="form-tip">用于 Worktree 卡片的“打开编辑器”操作</span>
            </n-space>
          </n-form-item>
          <n-form-item v-if="showCustomEditorInput" label="自定义命令">
            <n-space vertical size="small">
              <n-input
                v-model:value="customEditorCommandValue"
                :placeholder="customCommandPlaceholder"
              />
              <span class="form-tip">
                使用 <code>{{ '{' }}{{ '{' }}path{{ '}' }}{{ '}' }}</code> 表示 Worktree 路径；若未包含占位符会在命令末尾追加路径。
              </span>
            </n-space>
          </n-form-item>
        </n-form>
      </n-card>

      <n-card title="实时预览" size="huge">
        <div class="preview-panel" :style="{ backgroundColor: surfaceColor }">
          <div class="preview-banner" :style="{ backgroundColor: primaryColor }">
            <n-space align="center" size="small">
              <n-icon size="24" color="#fff">
                <ColorPaletteOutline />
              </n-icon>
              <span>看看新主题的样子</span>
            </n-space>
          </div>
          <div class="preview-content">
            <n-space vertical size="medium">
              <n-button type="primary">主要按钮</n-button>
              <n-tag type="primary" :bordered="false">主色标签</n-tag>
              <n-alert type="info" title="提示">
                主题色会实时保存，所有页面会立即生效。
              </n-alert>
            </n-space>
          </div>
        </div>
      </n-card>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useTitle, useEventListener, useDebounceFn } from '@vueuse/core';
import { useMessage } from 'naive-ui';
import { ColorPaletteOutline, SettingsOutline, RefreshOutline } from '@vicons/ionicons5';
import {
  useSettingsStore,
  DEFAULT_TERMINAL_SHORTCUT,
  DEFAULT_NOTEPAD_SHORTCUT,
  type PanelShortcutSetting,
  type EditorPreference,
} from '@/stores/settings';
import { APP_NAME } from '@/constants/app';
import { DEFAULT_EDITOR, EDITOR_OPTIONS, isEditorPreference } from '@/constants/editor';

type ShortcutTarget = 'terminal' | 'notepad';

useTitle(`总设置 - ${APP_NAME}`);

const router = useRouter();
const message = useMessage();
const settingsStore = useSettingsStore();
const {
  theme,
  recentProjectsLimit,
  maxTerminalsPerProject,
  terminalShortcut,
  notepadShortcut,
  editorSettings,
  confirmBeforeTerminalClose,
} = storeToRefs(settingsStore);
const capturingTarget = ref<ShortcutTarget | null>(null);

const primaryColor = computed({
  get: () => theme.value.primaryColor,
  set: value => settingsStore.updateTheme({ primaryColor: value || '#3B69A9' }),
});

const bodyColor = computed({
  get: () => theme.value.bodyColor,
  set: value => settingsStore.updateTheme({ bodyColor: value || '#f7f8fa' }),
});

const surfaceColor = computed({
  get: () => theme.value.surfaceColor,
  set: value => settingsStore.updateTheme({ surfaceColor: value || '#ffffff' }),
});

const editorOptions = EDITOR_OPTIONS;
const customCommandPlaceholder = '例如：code --reuse-window {{path}}';

const defaultEditorValue = computed<EditorPreference>({
  get: () => editorSettings.value.defaultEditor,
  set: (value: EditorPreference | null) => {
    const normalized = value && isEditorPreference(value) ? value : DEFAULT_EDITOR;
    settingsStore.updateEditorSettings({ defaultEditor: normalized });
  },
});

const customEditorCommandValue = computed({
  get: () => editorSettings.value.customCommand,
  set: value => settingsStore.updateEditorSettings({ customCommand: value ?? '' }),
});

const showCustomEditorInput = computed(() => defaultEditorValue.value === 'custom');

// 使用本地 ref + 防抖来避免输入过程中立即删除项目
const recentProjectsLimitLocal = ref(recentProjectsLimit.value);
const debouncedUpdateRecentProjectsLimit = useDebounceFn((value: number) => {
  settingsStore.updateRecentProjectsLimit(value ?? 10);
}, 800);

const recentProjectsLimitValue = computed({
  get: () => recentProjectsLimitLocal.value,
  set: (value) => {
    recentProjectsLimitLocal.value = value ?? 10;
    debouncedUpdateRecentProjectsLimit(value ?? 10);
  },
});

// 单项目终端上限也应用相同的防抖机制
const terminalLimitLocal = ref(maxTerminalsPerProject.value);
const debouncedUpdateTerminalLimit = useDebounceFn((value: number) => {
  settingsStore.updateMaxTerminalsPerProject(value ?? 12);
}, 800);

const terminalLimitValue = computed({
  get: () => terminalLimitLocal.value,
  set: (value) => {
    terminalLimitLocal.value = value ?? 12;
    debouncedUpdateTerminalLimit(value ?? 12);
  },
});

const confirmTerminalCloseValue = computed({
  get: () => confirmBeforeTerminalClose.value,
  set: value => settingsStore.updateConfirmBeforeTerminalClose(value),
});

const terminalShortcutValue = computed(
  () => terminalShortcut.value.display || terminalShortcut.value.code,
);
const notepadShortcutValue = computed(() => notepadShortcut.value.display || notepadShortcut.value.code);
const isTerminalShortcutDefault = computed(
  () => terminalShortcut.value.code === DEFAULT_TERMINAL_SHORTCUT.code,
);
const isNotepadShortcutDefault = computed(() => notepadShortcut.value.code === DEFAULT_NOTEPAD_SHORTCUT.code);

function handleBack() {
  router.back();
}

function handleResetTheme() {
  settingsStore.resetTheme();
}

function handleStartShortcutCapture(target: ShortcutTarget) {
  if (capturingTarget.value === target) {
    return;
  }
  capturingTarget.value = target;
  message.info(`请按下新的${targetLabel(target)}快捷键（Esc 取消）`);
}

function handleResetShortcut(target: ShortcutTarget) {
  if (target === 'terminal') {
    settingsStore.resetTerminalShortcut();
  } else {
    settingsStore.resetNotepadShortcut();
  }
}

function isCapturing(target: ShortcutTarget) {
  return capturingTarget.value === target;
}

function getShortcutStatus(target: ShortcutTarget) {
  return isCapturing(target) ? 'warning' : undefined;
}

function getShortcutHint(target: ShortcutTarget) {
  return isCapturing(target) ? '等待输入… (Esc 取消)' : '单键、无 Ctrl/Alt';
}

function targetLabel(target: ShortcutTarget) {
  return target === 'terminal' ? '终端' : '记事板';
}

if (typeof window !== 'undefined') {
  useEventListener(window, 'keydown', event => {
    if (!capturingTarget.value) {
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      capturingTarget.value = null;
      return;
    }
    event.preventDefault();
    const shortcut = normalizeShortcutEvent(event);
    if (!shortcut) {
      message.warning('暂不支持该按键，请选择其他按键');
      return;
    }
    if (capturingTarget.value === 'terminal') {
      settingsStore.updateTerminalShortcut(shortcut);
    } else {
      settingsStore.updateNotepadShortcut(shortcut);
    }
    const target = capturingTarget.value;
    capturingTarget.value = null;
    message.success(`${targetLabel(target!)}快捷键已更新为 ${shortcut.display}`);
  });
}

function normalizeShortcutEvent(event: KeyboardEvent): PanelShortcutSetting | null {
  if (event.metaKey || event.ctrlKey || event.altKey) {
    return null;
  }
  const disallowedKeys = new Set(['Shift', 'CapsLock', 'Tab', 'Enter']);
  if (disallowedKeys.has(event.key)) {
    return null;
  }
  const code = event.code?.trim();
  if (!code) {
    return null;
  }
  const display = formatShortcutLabel(event);
  return {
    code,
    display,
  };
}

function formatShortcutLabel(event: KeyboardEvent) {
  if (event.key === ' ') {
    return 'Space';
  }
  if (event.key && event.key.length === 1) {
    return event.key;
  }
  return event.code;
}
</script>

<style scoped>
.general-settings-page {
  max-width: 960px;
  margin: 0 auto;
  padding: 24px 24px 48px 24px;
}

.preview-panel {
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--n-border-color);
}

.preview-banner {
  color: #fff;
  padding: 16px;
  font-size: 16px;
  font-weight: 600;
}

.preview-content {
  padding: 24px;
  background-color: rgba(255, 255, 255, 0.8);
}

.form-tip {
  font-size: 12px;
  color: var(--n-text-color-3, #8a8fa3);
}

.shortcut-hint {
  font-size: 12px;
  color: var(--n-text-color-3, #8a8fa3);
}
</style>
