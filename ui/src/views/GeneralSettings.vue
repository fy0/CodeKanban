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
      <!-- 项目与终端设置 -->
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
          <n-form-item :label="t('settings.sendResizeOnSwitch')">
            <n-space vertical size="small">
              <n-switch v-model:value="sendResizeOnSwitchValue" />
              <span class="form-tip">{{ t('settings.sendResizeOnSwitchTip') }}</span>
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
          <n-form-item :label="t('settings.terminalShell')">
            <n-space vertical size="small">
              <n-spin :show="shellsLoading" size="small">
                <n-select
                  v-model:value="selectedShellValue"
                  :options="shellSelectOptions"
                  :loading="shellsLoading"
                  style="max-width: 320px"
                  :disabled="shellsLoading"
                />
              </n-spin>
              <n-collapse-transition :show="showCustomShellInput">
                <n-input
                  v-model:value="customShellCommand"
                  :placeholder="t('settings.customShellPlaceholder')"
                  :status="customShellStatus"
                  style="max-width: 320px; margin-top: 8px;"
                  @blur="handleCustomShellBlur"
                />
              </n-collapse-transition>
              <span class="form-tip">{{ t('settings.terminalShellTip') }}</span>
              <span v-if="shellsData?.platform" class="form-tip">
                {{ t('settings.currentPlatform') }}: {{ platformDisplayName }}
              </span>
            </n-space>
          </n-form-item>
        </n-form>
      </n-card>

      <n-card :title="t('settings.terminalQuickActions')" size="huge">
        <n-form label-placement="left" label-width="160">
          <n-form-item :label="t('settings.terminalQuickActionsList')">
            <n-space vertical size="small" style="width: 100%">
              <n-dynamic-input v-model:value="terminalQuickActionsLocal" :on-create="createTerminalQuickAction">
                <template #default="{ value }">
                  <div class="terminal-quick-action-item">
                    <div class="terminal-quick-action-row terminal-quick-action-row-switches">
                      <n-space align="center" size="small" wrap>
                        <n-switch v-model:value="value.enabled" />
                        <n-tooltip trigger="hover" placement="top" :delay="80">
                          <template #trigger>
                            <n-checkbox v-model:checked="value.stacked">
                              {{ t('settings.terminalQuickActionStackLabel') }}
                            </n-checkbox>
                          </template>
                          {{ t('settings.terminalQuickActionStackTip') }}
                        </n-tooltip>
                      </n-space>
                    </div>
                    <div class="terminal-quick-action-row terminal-quick-action-row-inputs">
                      <n-input
                        v-model:value="value.name"
                        class="terminal-quick-action-input"
                        :placeholder="t('settings.terminalQuickActionNamePlaceholder')"
                      />
                      <n-input
                        v-model:value="value.command"
                        class="terminal-quick-action-input"
                        :placeholder="t('settings.terminalQuickActionCommandPlaceholder')"
                      />
                    </div>
                    <div class="terminal-quick-action-row terminal-quick-action-row-icons">
                      <n-radio-group v-model:value="value.icon" size="small">
                        <n-radio-button
                          v-for="option in terminalQuickActionIconButtons"
                          :key="option.value"
                          :value="option.value"
                          :title="option.label"
                        >
                          <span v-if="'svg' in option && option.svg" class="terminal-quick-action-svg" v-html="option.svg"></span>
                          <n-icon v-else :size="16">
                            <component :is="option.icon" />
                          </n-icon>
                        </n-radio-button>
                      </n-radio-group>
                    </div>
                  </div>
                </template>
                <template #action="{ index, remove, create }">
                  <n-button-group size="small">
                    <n-button quaternary circle @click="handleRemoveTerminalQuickAction(index, remove)">
                      <template #icon>
                        <n-icon>
                          <Remove />
                        </n-icon>
                      </template>
                    </n-button>
                    <n-button quaternary circle @click="create(index)">
                      <template #icon>
                        <n-icon>
                          <Add />
                        </n-icon>
                      </template>
                    </n-button>
                  </n-button-group>
                </template>
              </n-dynamic-input>
              <n-space>
                <n-button size="small" @click="handleResetTerminalQuickActions">
                  {{ t('settings.restoreDefault') }}
                </n-button>
              </n-space>
              <span class="form-tip">{{ t('settings.terminalQuickActionsTip') }}</span>
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
            <n-form-item :label="t('settings.aiAssistantCodex')">
              <n-space align="center">
                <n-switch v-model:value="aiStatusForm.codex" />
                <span class="form-tip">{{ t('settings.aiStatusCodexSupport') }}</span>
              </n-space>
            </n-form-item>
            <n-form-item :label="t('settings.aiAssistantQwenCode')">
              <n-space align="center">
                <n-switch v-model:value="aiStatusForm.qwenCode" />
                <span class="form-tip">{{ t('settings.aiStatusQwenSupport') }}</span>
              </n-space>
            </n-form-item>
          </n-form>
          <span class="form-tip">{{ t('settings.aiAssistantStatusTrackingTip') }}</span>
        </n-spin>
      </n-card>

      <n-card :title="t('settings.developerOptions')" size="huge">
        <template #header-extra>
          <n-button
            size="small"
            :loading="developerSaving"
            :disabled="!developerDirty || developerLoading"
            @click="handleSaveDeveloperConfig"
          >
            {{ t('common.save') }}
          </n-button>
        </template>
        <n-spin :show="developerLoading">
          <n-form label-placement="left" label-width="160">
            <n-form-item :label="t('settings.developerScrollback')">
              <n-space vertical size="small">
                <n-switch
                  v-model:value="developerForm.enableTerminalScrollback"
                  :disabled="developerLoading"
                />
                <span class="form-tip">{{ t('settings.developerScrollbackTip') }}</span>
              </n-space>
            </n-form-item>
            <n-form-item :label="t('settings.renameSessionTitleEachCommand')">
              <n-space vertical size="small">
                <n-switch
                  v-model:value="developerForm.renameSessionTitleEachCommand"
                  :disabled="developerLoading"
                />
                <span class="form-tip">{{ t('settings.renameSessionTitleEachCommandTip') }}</span>
              </n-space>
            </n-form-item>
            <n-form-item :label="t('settings.autoCreateTaskOnStartWork')">
              <n-space vertical size="small">
                <n-switch
                  v-model:value="developerForm.autoCreateTaskOnStartWork"
                  :disabled="developerLoading"
                />
                <span class="form-tip">{{ t('settings.autoCreateTaskOnStartWorkTip') }}</span>
              </n-space>
            </n-form-item>
          </n-form>
        </n-spin>
      </n-card>

      <n-card :title="t('settings.worktreeSettings')" size="huge">
        <template #header-extra>
          <n-button
            size="small"
            :loading="worktreeSettingsSaving"
            :disabled="!worktreeSettingsDirty || worktreeSettingsLoading || !!globalBaseDirError"
            @click="handleSaveWorktreeSettings"
          >
            {{ t('common.save') }}
          </n-button>
        </template>
        <n-spin :show="worktreeSettingsLoading">
          <n-form label-placement="left" label-width="160">
            <n-form-item
              :label="t('settings.worktreeGlobalBaseDir')"
              :validation-status="globalBaseDirError ? 'error' : undefined"
              :feedback="globalBaseDirError"
            >
              <n-space vertical size="small" style="width: 100%">
                <n-input
                  v-model:value="worktreeSettingsForm.globalBaseDir"
                  :placeholder="t('settings.worktreeGlobalBaseDirPlaceholder')"
                  :status="globalBaseDirError ? 'error' : undefined"
                  @blur="validateGlobalBaseDir"
                  @input="validateGlobalBaseDir"
                />
                <span class="form-tip">{{ t('settings.worktreeGlobalBaseDirTip') }}</span>
              </n-space>
            </n-form-item>
            <n-form-item :label="t('settings.worktreeGlobalDirNamePattern')">
              <n-space vertical size="small" style="width: 100%">
                <n-input v-model:value="worktreeSettingsForm.globalDirNamePattern" />
                <span class="form-tip">{{ t('settings.worktreeGlobalDirNamePatternTip') }}</span>
              </n-space>
            </n-form-item>
          </n-form>
        </n-spin>
      </n-card>

      <!-- 主题设置 -->
      <n-card :title="t('settings.themeSettings')" size="huge">
        <n-form label-placement="left" label-width="140">
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
              <span class="form-tip">{{ t('theme.followSystemHint') }}</span>
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

          <n-divider style="margin: 16px 0">{{ t('settings.terminalFontSettings') }}</n-divider>

          <n-form-item :label="t('settings.terminalFontFamily')">
            <n-space vertical size="small">
              <n-space>
                <n-select
                  v-model:value="terminalFontFamilyValue"
                  :options="fontFamilyOptions"
                  style="width: 200px"
                  filterable
                  tag
                  :placeholder="t('settings.terminalFontFamilyPlaceholder')"
                />
                <n-button
                  size="small"
                  text
                  :disabled="!terminalFontFamilyValue"
                  @click="handleResetFontFamily"
                >
                  {{ t('settings.restoreDefault') }}
                </n-button>
              </n-space>
              <span class="form-tip">{{ t('settings.terminalFontFamilyTip') }}</span>
            </n-space>
          </n-form-item>

          <n-form-item :label="t('settings.terminalFontSize')">
            <n-space vertical size="small">
              <n-space align="center">
                <n-slider
                  v-model:value="terminalFontSizeValue"
                  :min="8"
                  :max="32"
                  :step="1"
                  style="width: 180px"
                />
                <n-input-number
                  v-model:value="terminalFontSizeValue"
                  :min="8"
                  :max="32"
                  :step="1"
                  size="small"
                  style="width: 80px"
                />
                <span class="unit-label">px</span>
              </n-space>
              <span class="form-tip">{{ t('settings.terminalFontSizeTip') }}</span>
            </n-space>
          </n-form-item>

          <n-form-item :label="t('settings.terminalFontWeight')">
            <n-space vertical size="small">
              <n-space>
                <n-select
                  v-model:value="terminalFontWeightValue"
                  :options="fontWeightOptions"
                  style="width: 160px"
                />
                <n-select
                  v-model:value="terminalFontWeightBoldValue"
                  :options="fontWeightOptions"
                  style="width: 160px"
                />
              </n-space>
              <span class="form-tip">{{ t('settings.terminalFontWeightTip') }}</span>
            </n-space>
          </n-form-item>

          <n-form-item :label="t('settings.terminalLineHeight')">
            <n-space vertical size="small">
              <n-space align="center">
                <n-slider
                  v-model:value="terminalLineHeightValue"
                  :min="1.0"
                  :max="2.0"
                  :step="0.1"
                  style="width: 180px"
                />
                <n-input-number
                  v-model:value="terminalLineHeightValue"
                  :min="1.0"
                  :max="2.0"
                  :step="0.1"
                  size="small"
                  style="width: 80px"
                />
              </n-space>
              <span class="form-tip">{{ t('settings.terminalLineHeightTip') }}</span>
            </n-space>
          </n-form-item>

          <n-form-item :label="t('settings.terminalLetterSpacing')">
            <n-space vertical size="small">
              <n-space align="center">
                <n-slider
                  v-model:value="terminalLetterSpacingValue"
                  :min="-2"
                  :max="5"
                  :step="0.5"
                  style="width: 180px"
                />
                <n-input-number
                  v-model:value="terminalLetterSpacingValue"
                  :min="-2"
                  :max="5"
                  :step="0.5"
                  size="small"
                  style="width: 80px"
                />
                <span class="unit-label">px</span>
              </n-space>
              <span class="form-tip">{{ t('settings.terminalLetterSpacingTip') }}</span>
            </n-space>
          </n-form-item>

          <n-form-item :label="t('settings.terminalWebGLRenderer')">
            <n-space vertical size="small">
              <n-radio-group v-model:value="terminalWebGLRendererValue">
                <n-space>
                  <n-radio value="auto">{{ t('settings.webglAuto') }}</n-radio>
                  <n-radio value="force">{{ t('settings.webglForce') }}</n-radio>
                  <n-radio value="disable">{{ t('settings.webglDisable') }}</n-radio>
                </n-space>
              </n-radio-group>
              <span class="form-tip">{{ webglRendererTip }}</span>
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
          <n-form-item :label="t('settings.textColor')">
            <n-color-picker v-model:value="textColor" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>

          <n-divider style="margin: 16px 0">{{ t('theme.terminalColors') }}</n-divider>

          <n-form-item :label="t('settings.terminalBg')">
            <n-color-picker v-model:value="terminalBg" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.terminalFg')">
            <n-color-picker v-model:value="terminalFg" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.terminalTabBg')">
            <n-color-picker v-model:value="terminalTabBg" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.terminalTabActiveBg')">
            <n-color-picker v-model:value="terminalTabActiveBg" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>

          <n-divider style="margin: 16px 0">{{ t('theme.statusColors') }}</n-divider>

          <n-form-item :label="t('settings.terminalTabCompletionBg')">
            <n-color-picker v-model:value="terminalTabCompletionBg" :modes="['hex', 'rgb']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.terminalTabCompletionBorder')">
            <n-color-picker v-model:value="terminalTabCompletionBorder" :modes="['hex', 'rgb']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.terminalTabApprovalBg')">
            <n-color-picker v-model:value="terminalTabApprovalBg" :modes="['hex', 'rgb']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.terminalTabApprovalBorder')">
            <n-color-picker v-model:value="terminalTabApprovalBorder" :modes="['hex', 'rgb']" :actions="['confirm']" />
          </n-form-item>

          <n-divider style="margin: 16px 0">{{ t('theme.floatingButtonColors') }}</n-divider>

          <n-form-item :label="t('settings.terminalFloatingButtonBg')">
            <n-color-picker v-model:value="terminalFloatingButtonBg" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
          <n-form-item :label="t('settings.terminalFloatingButtonFg')">
            <n-color-picker v-model:value="terminalFloatingButtonFg" :modes="['hex']" :actions="['confirm']" />
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 实时预览 -->
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
import { computed, ref, reactive, watch } from 'vue';
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useTitle, useEventListener, useDebounceFn, useStorage } from '@vueuse/core';
import { useDialog, useMessage } from 'naive-ui';
import {
  ChatbubblesOutline,
  CodeOutline,
  ColorPaletteOutline,
  LogoGithub,
  LogoGoogle,
  NavigateOutline,
  RefreshOutline,
  RocketOutline,
  SettingsOutline,
  SparklesOutline,
  TerminalOutline,
  PlayOutline,
  Add,
  Remove,
} from '@vicons/ionicons5';
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue';
import { useLocale } from '@/composables/useLocale';
import { getAssistantIconByType } from '@/utils/assistantIcon';
import {
  useSettingsStore,
  DEFAULT_TERMINAL_SHORTCUT,
  DEFAULT_NOTEPAD_SHORTCUT,
  DEFAULT_TERMINAL_QUICK_ACTIONS,
  TERMINAL_FONT_OPTIONS,
  FONT_WEIGHT_OPTIONS,
  type PanelShortcutSetting,
  type EditorPreference,
  type FontWeight,
  type TerminalQuickAction,
  type TerminalQuickActionIcon,
} from '@/stores/settings';
import { APP_NAME } from '@/constants/app';
import { DEFAULT_EDITOR, EDITOR_OPTIONS, isEditorPreference } from '@/constants/editor';
import { useThemeOptions, useTerminalThemeOptions } from '@/composables/useThemeOptions';
import { lightenColor, darkenColor, ensureHexWithHash, isDarkHex, getReadableTextColor } from '@/utils/color';
import Apis from '@/api';
import { http } from '@/api/http';
import { useReq, useInit } from '@/api/composable';
import type { AIAssistantStatusConfig, DeveloperConfig, AvailableShellsResponse, WorktreeConfig } from '@/types/models';

type ShortcutTarget = 'terminal' | 'notepad';

const SHELL_AUTO_VALUE = '__auto__';
const SHELL_CUSTOM_VALUE = '__custom__';

type ItemResponse<T> = {
  item?: T;
};

const { t, locale } = useLocale();

useTitle(`${t('settings.title')} - ${APP_NAME}`);

const router = useRouter();
const message = useMessage();
const dialog = useDialog();
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
  terminalQuickActions,
  editorSettings,
  confirmBeforeTerminalClose,
  terminalThemeId,
  terminalFont,
  terminalWebGLRenderer,
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
    aiStatusForm.qwenCode !== aiStatusOriginal.value.qwenCode ||
    aiStatusForm.gemini !== aiStatusOriginal.value.gemini ||
    aiStatusForm.cursor !== aiStatusOriginal.value.cursor ||
    aiStatusForm.copilot !== aiStatusOriginal.value.copilot
  );
});

const { send: fetchAIStatus, loading: aiStatusLoading } = useReq(
  () => Apis.system.aiAssistantStatusGet()
);

const { send: updateAIStatus, loading: saveLoading } = useReq(
  (config: AIAssistantStatusConfig) => Apis.system.aiAssistantStatusUpdate({ data: config })
);

const developerForm = reactive<DeveloperConfig>({
  enableTerminalScrollback: false,
  renameSessionTitleEachCommand: false,
  autoCreateTaskOnStartWork: true,
});
const developerOriginal = ref<DeveloperConfig | null>(null);
const developerDirty = computed(() => {
  if (!developerOriginal.value) {
    return false;
  }
  return (
    developerForm.enableTerminalScrollback !== developerOriginal.value.enableTerminalScrollback
    || developerForm.renameSessionTitleEachCommand !== developerOriginal.value.renameSessionTitleEachCommand
    || developerForm.autoCreateTaskOnStartWork !== developerOriginal.value.autoCreateTaskOnStartWork
  );
});

const { send: fetchDeveloperConfig, loading: developerLoading } = useReq(
  () => http.Get<ItemResponse<DeveloperConfig>>('/system/developer-config')
);

const { send: updateDeveloperConfig, loading: developerSaving } = useReq(
  (config: DeveloperConfig) => http.Post('/system/developer-config/update', config)
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

async function loadDeveloperConfig() {
  try {
    const resp = await fetchDeveloperConfig();
    const config = resp?.item;
    if (config !== undefined && config !== null) {
      developerForm.enableTerminalScrollback = config.enableTerminalScrollback ?? false;
      developerForm.renameSessionTitleEachCommand = config.renameSessionTitleEachCommand ?? false;
      developerForm.autoCreateTaskOnStartWork = config.autoCreateTaskOnStartWork ?? true;
      developerOriginal.value = { ...developerForm };
    } else {
      // 如果后端没有返回配置，使用默认值并标记为已加载
      developerOriginal.value = { ...developerForm };
    }
  } catch (error) {
    console.error('Failed to load developer config:', error);
    // 即使失败，也设置 original 值，避免按钮一直禁用
    developerOriginal.value = { ...developerForm };
  }
}

async function handleSaveDeveloperConfig() {
  try {
    await updateDeveloperConfig({ ...developerForm });
    developerOriginal.value = { ...developerForm };
    message.success(t('common.saveSuccess'));
  } catch (error) {
    console.error('Failed to save developer config:', error);
    message.error(t('common.saveFailed'));
  }
}

// Worktree 全局设置
const worktreeSettingsForm = reactive<WorktreeConfig>({
  globalBaseDir: '',
  globalDirNamePattern: '{projectName}-{branch}',
});
const worktreeSettingsOriginal = ref<WorktreeConfig | null>(null);
const globalBaseDirError = ref('');

/**
 * 判断路径是否看起来像绝对路径（跨平台）
 */
function looksLikeAbsPath(path: string) {
  const trimmed = path.trim();
  // Unix 风格：以 / 开头
  if (trimmed.startsWith('/')) {
    return true;
  }
  // Windows 风格：盘符 + 冒号 + 斜杠
  return /^[a-zA-Z]:[\\/]/.test(trimmed);
}

/**
 * 验证全局基础目录路径
 */
function validateGlobalBaseDir() {
  const val = worktreeSettingsForm.globalBaseDir.trim();
  if (val === '') {
    globalBaseDirError.value = '';
    return true;
  }
  if (!looksLikeAbsPath(val)) {
    globalBaseDirError.value = t('validation.mustBeAbsolutePath');
    return false;
  }
  globalBaseDirError.value = '';
  return true;
}

// 检测表单是否有改动
const worktreeSettingsDirty = computed(() => {
  if (!worktreeSettingsOriginal.value) {
    return false;
  }
  return (
    worktreeSettingsForm.globalBaseDir !== worktreeSettingsOriginal.value.globalBaseDir ||
    worktreeSettingsForm.globalDirNamePattern !== worktreeSettingsOriginal.value.globalDirNamePattern
  );
});

const { send: fetchWorktreeSettings, loading: worktreeSettingsLoading } = useReq(
  () => http.Get<ItemResponse<WorktreeConfig>>('/system/worktree-settings')
);

const { send: updateWorktreeSettings, loading: worktreeSettingsSaving } = useReq(
  (config: WorktreeConfig) => http.Post<ItemResponse<WorktreeConfig>>('/system/worktree-settings/update', config)
);

/**
 * 加载 Worktree 全局设置
 */
async function loadWorktreeSettings() {
  try {
    const resp = await fetchWorktreeSettings();
    const config = resp?.item;
    if (config) {
      worktreeSettingsForm.globalBaseDir = config.globalBaseDir ?? '';
      worktreeSettingsForm.globalDirNamePattern =
        config.globalDirNamePattern ?? worktreeSettingsForm.globalDirNamePattern;
      worktreeSettingsOriginal.value = { ...worktreeSettingsForm };
    } else {
      worktreeSettingsOriginal.value = { ...worktreeSettingsForm };
    }
  } catch (error) {
    console.error('Failed to load worktree settings:', error);
    worktreeSettingsOriginal.value = { ...worktreeSettingsForm };
  }
}

/**
 * 保存 Worktree 全局设置
 */
async function handleSaveWorktreeSettings() {
  try {
    await updateWorktreeSettings({ ...worktreeSettingsForm });
    worktreeSettingsOriginal.value = { ...worktreeSettingsForm };
    message.success(t('common.saveSuccess'));
  } catch (error) {
    console.error('Failed to save worktree settings:', error);
    message.error(t('common.saveFailed'));
  }
}

// Terminal Shell Settings
const shellsData = ref<AvailableShellsResponse | null>(null);
const selectedShellId = ref<string>(SHELL_AUTO_VALUE);
const customShellCommand = ref('');
const customShellValid = ref(true);

const { send: fetchShells, loading: shellsLoading } = useReq(
  () => http.Get<ItemResponse<AvailableShellsResponse>>('/system/terminal-shells')
);

const { send: updateShell } = useReq(
  (shell: string) => http.Post('/system/terminal-shells/update', { shell })
);

const { send: validateShell } = useReq(
  (shell: string) => http.Post<{ valid: boolean; message?: string }>('/system/terminal-shells/validate', { shell })
);

async function loadShellsConfig() {
  try {
    const resp = await fetchShells();
    const data = resp?.item;
    if (data) {
      shellsData.value = data;
      // Determine selected shell ID based on current config
      if (!data.currentShell || data.currentShell === '') {
        selectedShellId.value = SHELL_AUTO_VALUE;
      } else {
        const matchedOption = data.options.find(opt => opt.command === data.currentShell);
        if (matchedOption) {
          selectedShellId.value = matchedOption.id;
        } else {
          selectedShellId.value = SHELL_CUSTOM_VALUE;
          customShellCommand.value = data.currentShell;
        }
      }
    }
  } catch (error) {
    console.error('Failed to load shell config:', error);
  }
}

useInit(() => {
  loadAIStatus();
  loadDeveloperConfig();
  loadWorktreeSettings();
  loadShellsConfig();
});

const shellSelectOptions = computed(() => {
  const options: Array<{ label: string; value: string; disabled?: boolean }> = [];

  // Auto option
  options.push({
    label: t('settings.shellAuto'),
    value: SHELL_AUTO_VALUE,
  });

  // Platform-specific options
  if (shellsData.value?.options) {
    for (const opt of shellsData.value.options) {
      let label = opt.available
        ? `${opt.name} - ${opt.description}`
        : `${opt.name} (${t('settings.shellNotInstalled')})`;

      // Add warning if present (translate using i18n key)
      if (opt.warning) {
        const warningText = t(`settings.${opt.warning}`);
        label += ` ⚠️ ${warningText}`;
      }

      options.push({
        label,
        value: opt.id,
        disabled: !opt.available,
      });
    }
  }

  // Custom option
  if (shellsData.value?.customAllowed) {
    options.push({
      label: t('settings.shellCustom'),
      value: SHELL_CUSTOM_VALUE,
    });
  }

  return options;
});

const showCustomShellInput = computed(() => selectedShellId.value === SHELL_CUSTOM_VALUE);

const customShellStatus = computed(() => {
  if (!customShellCommand.value) return undefined;
  return customShellValid.value ? undefined : 'error';
});

const platformDisplayName = computed(() => {
  const platform = shellsData.value?.platform;
  switch (platform) {
    case 'windows': return 'Windows';
    case 'darwin': return 'macOS';
    case 'linux': return 'Linux';
    default: return platform || '';
  }
});

const selectedShellValue = computed({
  get: () => selectedShellId.value,
  set: async (value: string) => {
    selectedShellId.value = value;

    if (value === SHELL_AUTO_VALUE) {
      // Save empty string for auto
      await saveShellConfig('');
    } else if (value === SHELL_CUSTOM_VALUE) {
      // Don't save yet, wait for custom input
      customShellCommand.value = '';
      customShellValid.value = true;
    } else {
      // Find the command for this shell ID
      const opt = shellsData.value?.options.find(o => o.id === value);
      if (opt) {
        await saveShellConfig(opt.command);
      }
    }
  },
});

async function handleCustomShellBlur() {
  if (!customShellCommand.value.trim()) {
    customShellValid.value = true;
    return;
  }

  try {
    const resp = await validateShell(customShellCommand.value);
    customShellValid.value = resp?.valid ?? false;
    if (customShellValid.value) {
      await saveShellConfig(customShellCommand.value);
    } else {
      message.error(resp?.message || t('settings.shellInvalid'));
    }
  } catch (error) {
    console.error('Failed to validate shell:', error);
    customShellValid.value = false;
  }
}

async function saveShellConfig(shell: string) {
  try {
    await updateShell(shell);
    message.success(t('settings.shellSaveSuccess'));
  } catch (error) {
    console.error('Failed to save shell config:', error);
    message.error(t('common.saveFailed'));
  }
}

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

const textColor = computed({
  get: () => theme.value.textColor || fallbackTextColor.value,
  set: value => {
    settingsStore.applyCustomTheme({ textColor: value || '#333333' });
  },
});

const terminalBg = computed({
  get: () => theme.value.terminalBg,
  set: value => {
    settingsStore.applyCustomTheme({ terminalBg: value || '#0c0c0c' });
  },
});

const terminalFg = computed({
  get: () => theme.value.terminalFg,
  set: value => {
    settingsStore.applyCustomTheme({ terminalFg: value || '#cccccc' });
  },
});

const terminalTabBg = computed({
  get: () => theme.value.terminalTabBg || theme.value.surfaceColor,
  set: value => {
    settingsStore.applyCustomTheme({ terminalTabBg: value || '#F1F1F1' });
  },
});

const terminalTabActiveBg = computed({
  get: () => theme.value.terminalTabActiveBg || theme.value.terminalBg,
  set: value => {
    settingsStore.applyCustomTheme({ terminalTabActiveBg: value || '#FFFFFF' });
  },
});

const terminalTabCompletionBg = computed({
  get: () => theme.value.terminalTabCompletionBg || 'rgba(16, 185, 129, 0.25)',
  set: value => {
    settingsStore.applyCustomTheme({ terminalTabCompletionBg: value || 'rgba(16, 185, 129, 0.25)' });
  },
});

const terminalTabCompletionBorder = computed({
  get: () => theme.value.terminalTabCompletionBorder || 'rgba(16, 185, 129, 0.5)',
  set: value => {
    settingsStore.applyCustomTheme({ terminalTabCompletionBorder: value || 'rgba(16, 185, 129, 0.5)' });
  },
});

const terminalTabApprovalBg = computed({
  get: () => theme.value.terminalTabApprovalBg || 'rgba(247, 144, 9, 0.25)',
  set: value => {
    settingsStore.applyCustomTheme({ terminalTabApprovalBg: value || 'rgba(247, 144, 9, 0.25)' });
  },
});

const terminalTabApprovalBorder = computed({
  get: () => theme.value.terminalTabApprovalBorder || 'rgba(247, 144, 9, 0.5)',
  set: value => {
    settingsStore.applyCustomTheme({ terminalTabApprovalBorder: value || 'rgba(247, 144, 9, 0.5)' });
  },
});

const terminalFloatingButtonBg = computed({
  get: () => theme.value.terminalFloatingButtonBg || '#1a1a1a',
  set: value => {
    settingsStore.applyCustomTheme({ terminalFloatingButtonBg: value || '#1a1a1a' });
  },
});

const terminalFloatingButtonFg = computed({
  get: () => theme.value.terminalFloatingButtonFg || '#ffffff',
  set: value => {
    settingsStore.applyCustomTheme({ terminalFloatingButtonFg: value || '#ffffff' });
  },
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

// 切换终端时发送 resize 指令（与 TerminalPanel.vue 共享同一个 localStorage key）
const sendResizeOnSwitchValue = useStorage('terminal-send-resize-on-switch', true);

function normalizeSvgSize(svg: string, sizePx: number) {
  const size = `${sizePx}px`;
  return svg
    .replace(/width:\s*12px;\s*height:\s*12px;/g, `width: ${size}; height: ${size};`)
    .replace(/width="12px"/g, `width="${size}"`)
    .replace(/height="12px"/g, `height="${size}"`);
}

const terminalQuickActionIconButtons = computed(() => {
  const agentOptions: { label: string; value: TerminalQuickActionIcon; svg?: string; icon?: any }[] = [
    { label: t('settings.terminalQuickActionIconClaude'), value: 'claude', svg: normalizeSvgSize(getAssistantIconByType('claude-code'), 16) },
    { label: t('settings.terminalQuickActionIconCodex'), value: 'codex', svg: normalizeSvgSize(getAssistantIconByType('codex'), 16) },
    { label: t('settings.terminalQuickActionIconQwen'), value: 'qwen', svg: normalizeSvgSize(getAssistantIconByType('qwen-code'), 16) },
    { label: t('settings.terminalQuickActionIconGemini'), value: 'gemini', svg: normalizeSvgSize(getAssistantIconByType('gemini'), 16) },
    { label: t('settings.terminalQuickActionIconCursor'), value: 'cursor', icon: NavigateOutline },
    { label: t('settings.terminalQuickActionIconCopilot'), value: 'copilot', icon: LogoGithub },
  ];

  const genericOptions: { label: string; value: TerminalQuickActionIcon; icon: any }[] = [
    { label: t('settings.terminalQuickActionIconTerminal'), value: 'terminal', icon: TerminalOutline },
    { label: t('settings.terminalQuickActionIconChat'), value: 'chat', icon: ChatbubblesOutline },
    { label: t('settings.terminalQuickActionIconCode'), value: 'code', icon: CodeOutline },
    { label: t('settings.terminalQuickActionIconRocket'), value: 'rocket', icon: RocketOutline },
    { label: t('settings.terminalQuickActionIconPlay'), value: 'play', icon: PlayOutline },
  ];

  const options = [...agentOptions, ...genericOptions];
  return options;
});

const terminalQuickActionsLocal = ref<TerminalQuickAction[]>(terminalQuickActions.value.map(item => ({ ...item })));
let syncingTerminalQuickActions = false;
const debouncedUpdateTerminalQuickActions = useDebounceFn((actions: TerminalQuickAction[]) => {
  settingsStore.updateTerminalQuickActions(actions);
}, 300);

watch(
  terminalQuickActions,
  next => {
    syncingTerminalQuickActions = true;
    terminalQuickActionsLocal.value = next.map(item => ({ ...item }));
    setTimeout(() => {
      syncingTerminalQuickActions = false;
    }, 0);
  },
  { deep: true },
);

watch(
  terminalQuickActionsLocal,
  next => {
    if (syncingTerminalQuickActions) {
      return;
    }
    debouncedUpdateTerminalQuickActions(next.map(item => ({ ...item })));
  },
  { deep: true },
);

function createTerminalQuickAction(): TerminalQuickAction {
  return {
    id: `custom-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    name: '',
    command: '',
    icon: 'terminal',
    enabled: true,
    stacked: false,
  };
}

function handleResetTerminalQuickActions() {
  settingsStore.updateTerminalQuickActions(DEFAULT_TERMINAL_QUICK_ACTIONS);
}

function handleRemoveTerminalQuickAction(index: number, remove: (index: number) => void) {
  dialog.warning({
    title: t('common.confirm'),
    content: t('settings.terminalQuickActionRemoveConfirm'),
    positiveText: t('common.confirm'),
    negativeText: t('common.cancel'),
    onPositiveClick: () => {
      remove(index);
    },
  });
}

const terminalThemeValue = computed({
  get: () => terminalThemeId.value,
  set: (value: string) => settingsStore.updateTerminalTheme(value),
});

// 终端字体设置
const fontFamilyOptions = computed(() => {
  const options = TERMINAL_FONT_OPTIONS.map(opt => ({
    label: opt.value === '' ? t('settings.terminalFontDefault') : opt.label,
    value: opt.value,
  }));
  // 添加自定义输入提示
  options.push({
    label: `── ${t('settings.terminalFontCustomHint')} ──`,
    value: '__custom_hint__',
    disabled: true,
  } as any);
  return options;
});

const terminalFontFamilyValue = computed({
  get: () => terminalFont.value.fontFamily,
  set: (value: string) => settingsStore.updateTerminalFont({ fontFamily: value ?? '' }),
});

const terminalFontSizeValue = computed({
  get: () => terminalFont.value.fontSize,
  set: (value: number) => settingsStore.updateTerminalFont({ fontSize: value ?? 14 }),
});

const fontWeightOptions = FONT_WEIGHT_OPTIONS.map(opt => ({
  label: opt.label,
  value: opt.value,
}));

const terminalFontWeightValue = computed({
  get: () => terminalFont.value.fontWeight,
  set: (value: FontWeight) => settingsStore.updateTerminalFont({ fontWeight: value ?? 'normal' }),
});

const terminalFontWeightBoldValue = computed({
  get: () => terminalFont.value.fontWeightBold,
  set: (value: FontWeight) => settingsStore.updateTerminalFont({ fontWeightBold: value ?? 'bold' }),
});

const terminalLineHeightValue = computed({
  get: () => terminalFont.value.lineHeight,
  set: (value: number) => settingsStore.updateTerminalFont({ lineHeight: value ?? 1.1 }),
});

const terminalLetterSpacingValue = computed({
  get: () => terminalFont.value.letterSpacing,
  set: (value: number) => settingsStore.updateTerminalFont({ letterSpacing: value ?? 0 }),
});

const terminalWebGLRendererValue = computed({
  get: () => terminalWebGLRenderer.value,
  set: (value: 'auto' | 'force' | 'disable') => settingsStore.updateTerminalWebGLRenderer(value),
});

const webglRendererTip = computed(() => {
  switch (terminalWebGLRendererValue.value) {
    case 'force':
      return t('settings.webglForceTip');
    case 'disable':
      return t('settings.webglDisableTip');
    default:
      return t('settings.webglAutoTip');
  }
});

function handleResetFontFamily() {
  settingsStore.updateTerminalFont({ fontFamily: '' });
}

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

.unit-label {
  font-size: 12px;
  color: var(--n-text-color-3, #8a8fa3);
  margin-left: 4px;
}

.terminal-quick-action-item {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.terminal-quick-action-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.terminal-quick-action-row-inputs {
  gap: 12px;
}

.terminal-quick-action-row-icons {
  flex-wrap: wrap;
}

.terminal-quick-action-input {
  flex: 1;
  min-width: 180px;
}

.terminal-quick-action-row-icons :deep(.n-radio-group) {
  line-height: 1;
}

.terminal-quick-action-row-icons :deep(.n-radio-button__content) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
}

.terminal-quick-action-row-icons :deep(.n-icon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  line-height: 1;
}

.terminal-quick-action-svg {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  line-height: 1;
}

.terminal-quick-action-svg :deep(svg) {
  display: block;
  width: 16px;
  height: 16px;
}

/* ========================================
   移动端响应式样式
   ======================================== */
@media (max-width: 767px) {
  .general-settings-page {
    padding: 16px;
    padding-bottom: 32px;
  }

  /* 表单标签置顶 */
  .general-settings-page :deep(.n-form-item) {
    --n-label-width: auto !important;
  }

  .general-settings-page :deep(.n-form-item-label) {
    text-align: left;
    padding-bottom: 8px;
  }

  .general-settings-page :deep(.n-form-item-blank) {
    min-width: 0;
  }

  /* 卡片内容紧凑 */
  .general-settings-page :deep(.n-card) {
    margin-bottom: 12px;
  }

  .general-settings-page :deep(.n-card-header) {
    padding: 12px 16px;
  }

  .general-settings-page :deep(.n-card__content) {
    padding: 12px 16px;
  }

  /* 预览面板 */
  .preview-content {
    padding: 16px;
  }

  .preview-banner {
    padding: 12px;
    font-size: 14px;
  }

  /* 表单项间距 */
  .general-settings-page :deep(.n-form-item) {
    margin-bottom: 16px;
  }

  .terminal-quick-action-row-inputs,
  .terminal-quick-action-row-icons {
    flex-direction: column;
  }
}

/* 平板端响应式 */
@media (min-width: 768px) and (max-width: 1023px) {
  .general-settings-page {
    padding: 20px;
    max-width: 100%;
  }
}
</style>
