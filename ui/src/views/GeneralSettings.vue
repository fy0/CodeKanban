<template>
  <div class="general-settings-page">
    <n-page-header @back="handleBack">
      <template #title>
        <n-space align="center" :wrap="false">
          <n-icon size="24" style="display: flex;">
            <SettingsOutline />
          </n-icon>
          <span style="line-height: 24px;">{{ t('settings.title') }}</span>
        </n-space>
      </template>
      <template #extra>
        <n-space align="center">
          <LanguageSwitcher />
          <n-button tertiary @click="handleResetTheme">
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
            {{ t('settings.resetTheme') }}
          </n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-space vertical size="large">
      <n-card :title="t('settings.themeSettings')" size="huge">
        <n-form label-placement="left" label-width="120">
          <n-form-item :label="t('theme.presetTheme')">
            <n-select
              v-model:value="currentPresetValue"
              :options="presetOptions"
              :disabled="followSystemValue"
              style="max-width: 240px"
            />
          </n-form-item>
          <n-form-item :label="t('theme.followSystem')">
            <n-space vertical size="small">
              <n-switch v-model:value="followSystemValue" />
              <span class="form-tip">{{ t('theme.customThemeHint') }}</span>
            </n-space>
          </n-form-item>

          <n-divider style="margin: 16px 0">{{ t('theme.customTheme') }}</n-divider>

          <n-alert v-if="hasCustomTheme" type="info" style="margin-bottom: 16px" :bordered="false">
            {{ t('theme.customThemeHint') }}
          </n-alert>

          <n-form-item :label="t('settings.primaryColor')">
            <n-color-picker v-model:value="primaryColor" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.bodyColor')">
            <n-color-picker v-model:value="bodyColor" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.surfaceColor')">
            <n-color-picker v-model:value="surfaceColor" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
        </n-form>
      </n-card>


      <n-card :title="t('settings.projectAndTerminal')" size="huge">
        <n-form label-placement="left" label-width="160">
          <n-form-item :label="t('settings.recentProjectsLimit')">
            <n-space vertical size="small">
              <n-input-number v-model:value="recentProjectsLimitValue" :min="1" :max="20" :step="1" />
              <span class="form-tip">{{ t('settings.recentProjectsLimitTip') }}</span>
            </n-space>
          </n-form-item>
          <n-form-item :label="t('settings.terminalLimit')">
            <n-space vertical size="small">
              <n-input-number v-model:value="terminalLimitValue" :min="1" :max="24" :step="1" />
              <span class="form-tip">{{ t('settings.terminalLimitTip') }}</span>
            </n-space>
          </n-form-item>
          <n-form-item :label="t('settings.confirmTerminalClose')">
            <n-space vertical size="small">
              <n-switch v-model:value="confirmTerminalCloseValue" />
              <span class="form-tip">{{ t('settings.confirmTerminalCloseTip') }}</span>
            </n-space>
          </n-form-item>
          <n-form-item :label="t('settings.terminalTheme')">
            <n-space vertical size="small">
              <n-select
                v-model:value="terminalThemeValue"
                :options="terminalThemeOptions"
                style="max-width: 240px"
              />
              <span class="form-tip">{{ t('settings.terminalThemeTip') }}</span>
            </n-space>
          </n-form-item>
          <n-form-item :label="t('settings.terminalShortcut')">
            <n-space vertical size="small">
              <n-input
                :value="terminalShortcutValue"
                readonly
                :status="getShortcutStatus('terminal')"
                :placeholder="t('settings.recordNewKey')"
              >
                <template #suffix>
                  <span class="shortcut-hint">
                    {{ getShortcutHint('terminal') }}
                  </span>
                </template>
              </n-input>
              <n-space>
                <n-button size="small" @click="handleStartShortcutCapture('terminal')">
                  {{ isCapturing('terminal') ? t('settings.recording') : t('settings.recordNewKey') }}
                </n-button>
                <n-button
                  size="small"
                  text
                  :disabled="isTerminalShortcutDefault"
                  @click="handleResetShortcut('terminal')"
                >
                  {{ t('settings.restoreDefault') }}
                </n-button>
              </n-space>
              <span class="form-tip">{{ t('settings.terminalShortcutTip') }}</span>
            </n-space>
          </n-form-item>
          <n-form-item :label="t('settings.notepadShortcut')">
            <n-space vertical size="small">
              <n-input
                :value="notepadShortcutValue"
                readonly
                :status="getShortcutStatus('notepad')"
                :placeholder="t('settings.recordNewKey')"
              >
                <template #suffix>
                  <span class="shortcut-hint">
                    {{ getShortcutHint('notepad') }}
                  </span>
                </template>
              </n-input>
              <n-space>
                <n-button size="small" @click="handleStartShortcutCapture('notepad')">
                  {{ isCapturing('notepad') ? t('settings.recording') : t('settings.recordNewKey') }}
                </n-button>
                <n-button
                  size="small"
                  text
                  :disabled="isNotepadShortcutDefault"
                  @click="handleResetShortcut('notepad')"
                >
                  {{ t('settings.restoreDefault') }}
                </n-button>
              </n-space>
              <span class="form-tip">{{ t('settings.notepadShortcutTip') }}</span>
            </n-space>
          </n-form-item>
          <n-form-item :label="t('settings.defaultEditor')">
            <n-space vertical size="small">
              <n-select
                v-model:value="defaultEditorValue"
                :options="editorOptions"
                style="max-width: 240px"
              />
              <span class="form-tip">{{ t('settings.defaultEditorTip') }}</span>
            </n-space>
          </n-form-item>
          <n-form-item v-if="showCustomEditorInput" :label="t('settings.customCommand')">
            <n-space vertical size="small">
              <n-input
                v-model:value="customEditorCommandValue"
                :placeholder="customCommandPlaceholder"
              />
              <span class="form-tip">
                {{ t('settings.customCommandTip') }}
              </span>
            </n-space>
          </n-form-item>
        </n-form>
      </n-card>

      <n-card :title="t('settings.aiAssistantStatusTracking')" size="huge">
        <template #header-extra>
          <n-button
            size="small"
            :loading="saveLoading"
            :disabled="!aiStatusDirty"
            @click="handleSaveAIStatus"
          >
            {{ t('common.save') }}
          </n-button>
        </template>
        <n-spin :show="aiStatusLoading">
          <n-form label-placement="left" label-width="160">
            <n-form-item :label="t('settings.aiAssistantClaudeCode')">
              <n-space align="center">
                <n-switch v-model:value="aiStatusForm.claudeCode" />
                <span class="form-tip">{{ t('settings.aiStatusClaudeSupport') }}</span>
              </n-space>
            </n-form-item>
            <n-form-item :label="t('settings.aiAssistantQwenCode')">
              <n-space align="center">
                <n-switch v-model:value="aiStatusForm.qwenCode" />
                <span class="form-tip">{{ t('settings.aiStatusQwenSupport') }}</span>
              </n-space>
            </n-form-item>
            <n-form-item>
              <template #label>
                {{ t('settings.aiAssistantCodex') }}
                <n-tag size="small" type="warning" :bordered="false" style="margin-left: 4px;">
                  {{ t('settings.aiStatusHasIssues') }}
                </n-tag>
              </template>
              <n-switch v-model:value="aiStatusForm.codex" />
            </n-form-item>
          </n-form>
          <span class="form-tip">{{ t('settings.aiAssistantStatusTrackingTip') }}</span>
        </n-spin>
      </n-card>

      <n-card :title="t('settings.realtimePreview')" size="huge">
        <div class="preview-panel" :style="previewPanelStyle">
          <div class="preview-banner">
            <n-space align="center" size="small">
              <n-icon size="24">
                <ColorPaletteOutline />
              </n-icon>
              <span>{{ t('settings.previewTheme') }}</span>
            </n-space>
          </div>
          <div class="preview-content">
            <n-space vertical size="medium">
              <n-button type="primary">{{ t('common.save') }}</n-button>
              <n-tag type="primary" :bordered="false">{{ t('settings.sampleCard') }}</n-tag>
              <n-alert type="info" :title="t('common.info')">
                {{ t('settings.sampleCardContent') }}
              </n-alert>
            </n-space>
          </div>
        </div>
      </n-card>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, reactive } from 'vue';
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useTitle, useEventListener, useDebounceFn } from '@vueuse/core';
import { useMessage } from 'naive-ui';
import { ColorPaletteOutline, SettingsOutline, RefreshOutline } from '@vicons/ionicons5';
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue';
import { useLocale } from '@/composables/useLocale';
import {
  useSettingsStore,
  DEFAULT_TERMINAL_SHORTCUT,
  DEFAULT_NOTEPAD_SHORTCUT,
  type PanelShortcutSetting,
  type EditorPreference,
} from '@/stores/settings';
import { APP_NAME } from '@/constants/app';
import { DEFAULT_EDITOR, EDITOR_OPTIONS, isEditorPreference } from '@/constants/editor';
import { useThemeOptions, useTerminalThemeOptions } from '@/composables/useThemeOptions';
import { lightenColor, darkenColor, ensureHexWithHash, isDarkHex, getReadableTextColor } from '@/utils/color';
import Apis from '@/api';
import { useReq, useInit } from '@/api/composable';
import type { AIAssistantStatusConfig } from '@/types/models';

type ShortcutTarget = 'terminal' | 'notepad';

const { t, locale } = useLocale();

useTitle(`${t('settings.title')} - ${APP_NAME}`);

const router = useRouter();
const message = useMessage();
const settingsStore = useSettingsStore();
const {
  theme,
  currentPresetId,
  followSystemTheme,
  customTheme,
  recentProjectsLimit,
  maxTerminalsPerProject,
  terminalShortcut,
  notepadShortcut,
  editorSettings,
  confirmBeforeTerminalClose,
  terminalThemeId,
} = storeToRefs(settingsStore);
const capturingTarget = ref<ShortcutTarget | null>(null);

// 使用 composable 获取主题和终端配色选项
const presetOptions = useThemeOptions();
const terminalThemeOptions = useTerminalThemeOptions();

// 当前预设 ID
const currentPresetValue = computed({
  get: () => currentPresetId.value,
  set: (value: string) => {
    settingsStore.selectPreset(value);
  },
});

// 跟随系统主题
const followSystemValue = computed({
  get: () => followSystemTheme.value,
  set: (value: boolean) => {
    settingsStore.toggleFollowSystemTheme(value);
  },
});

// 是否有自定义主题
const hasCustomTheme = computed(() => customTheme.value !== null);

// AI Assistant Status Tracking
const aiStatusForm = reactive<AIAssistantStatusConfig>({
  claudeCode: true,
  codex: false,
  qwenCode: true,
  gemini: false,
  cursor: false,
  copilot: false,
});
const aiStatusOriginal = ref<AIAssistantStatusConfig | null>(null);
const aiStatusDirty = computed(() => {
  if (!aiStatusOriginal.value) return false;
  return (
    aiStatusForm.claudeCode !== aiStatusOriginal.value.claudeCode ||
    aiStatusForm.codex !== aiStatusOriginal.value.codex ||
    aiStatusForm.qwenCode !== aiStatusOriginal.value.qwenCode
  );
});

const { send: fetchAIStatus, loading: aiStatusLoading } = useReq(
  () => Apis.system.aiAssistantStatusGet()
);

const { send: updateAIStatus, loading: saveLoading } = useReq(
  (config: AIAssistantStatusConfig) => Apis.system.aiAssistantStatusUpdate({ data: config })
);

async function loadAIStatus() {
  try {
    const resp = await fetchAIStatus();
    const config = resp?.item;
    if (config) {
      Object.assign(aiStatusForm, config);
      aiStatusOriginal.value = { ...config };
    }
  } catch (error) {
    console.error('Failed to load AI status config:', error);
  }
}

async function handleSaveAIStatus() {
  try {
    await updateAIStatus({ ...aiStatusForm });
    aiStatusOriginal.value = { ...aiStatusForm };
    message.success(t('common.saveSuccess'));
  } catch (error) {
    console.error('Failed to save AI status config:', error);
    message.error(t('common.saveFailed'));
  }
}

useInit(() => {
  loadAIStatus();
});
const primaryColor = computed({
  get: () => theme.value.primaryColor,
  set: value => {
    settingsStore.applyCustomTheme({ primaryColor: value || '#3B69A9' });
  },
});

const bodyColor = computed({
  get: () => theme.value.bodyColor,
  set: value => {
    settingsStore.applyCustomTheme({ bodyColor: value || '#f7f8fa' });
  },
});

const surfaceColor = computed({
  get: () => theme.value.surfaceColor,
  set: value => {
    settingsStore.applyCustomTheme({ surfaceColor: value || '#ffffff' });
  },
});

const fallbackTextColor = computed(() => {
  if (theme.value.textColor) {
    return theme.value.textColor;
  }
  const bodyHex = ensureHexWithHash(theme.value.bodyColor || '#ffffff');
  return getReadableTextColor(bodyHex);
});

const previewPanelStyle = computed(() => {
  const primaryHex = ensureHexWithHash(primaryColor.value || '#3B69A9', '#3B69A9');
  const surfaceHex = ensureHexWithHash(surfaceColor.value || '#ffffff', '#ffffff');
  const surfaceIsDark = isDarkHex(surfaceHex);
  const contentBg = surfaceIsDark
    ? lightenColor(surfaceHex, 0.08)
    : darkenColor(surfaceHex, 0.04);
  return {
    '--preview-panel-bg': surfaceHex,
    '--preview-banner-bg': primaryHex,
    '--preview-banner-text': getReadableTextColor(primaryHex),
    '--preview-content-bg': contentBg,
    '--preview-content-text': fallbackTextColor.value,
  };
});

const editorOptions = EDITOR_OPTIONS;
const customCommandPlaceholder = computed(() => t('settings.customCommandPlaceholder'));

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

const terminalThemeValue = computed({
  get: () => terminalThemeId.value,
  set: (value: string) => settingsStore.updateTerminalTheme(value),
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
  message.info(t('settings.pressNewShortcut', { target: targetLabel(target) }));
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
  return isCapturing(target) ? t('settings.waitingForInput') : t('settings.singleKeyNoModifier');
}

function targetLabel(target: ShortcutTarget) {
  return target === 'terminal' ? t('settings.terminal') : t('settings.notepad');
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
      message.warning(t('settings.keyNotSupported'));
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

:deep(.n-page-header) {
  padding-bottom: 16px;
}

:deep(.n-page-header .n-page-header-header) {
  align-items: center;
}

.preview-panel {
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--n-border-color);
  background-color: var(--preview-panel-bg, var(--app-surface-color, #fff));
}

.preview-banner {
  background-color: var(--preview-banner-bg, var(--n-primary-color, #3b69a9));
  color: var(--preview-banner-text, var(--kanban-terminal-fg, #fff));
  padding: 16px;
  font-size: 16px;
  font-weight: 600;
}

.preview-content {
  padding: 24px;
  background-color: var(--preview-content-bg, var(--app-surface-color, #fff));
  color: var(--preview-content-text, var(--kanban-terminal-fg, #1f1f1f));
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
