<template>
  <div class="general-settings-page">
    <n-page-header @back="handleBack">
      <template #title>
        <n-space align="center" :wrap="false">
          <n-icon size="24" style="display: flex">
            <SettingsOutline />
          </n-icon>
          <span style="line-height: 24px">{{ t('settings.title') }}</span>
        </n-space>
      </template>
      <template #extra>
        <n-space align="center" class="settings-header-actions">
          <LanguageSwitcher :compact="isMobile" />
          <n-button tertiary class="settings-header-reset" @click="handleResetTheme">
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

    <!-- 主布局 -->
    <div class="settings-layout">
      <!-- 左侧导航 -->
      <aside class="settings-sidebar">
        <!-- 搜索框 -->
        <div class="settings-search-box">
          <n-input
            v-model:value="settingsSearchQuery"
            clearable
            :placeholder="t('settings.searchPlaceholder')"
          >
            <template #prefix>
              <n-icon size="16">
                <SearchOutline />
              </n-icon>
            </template>
          </n-input>
        </div>
        <nav class="settings-nav">
          <button
            v-for="card in settingsCards"
            :key="card.id"
            type="button"
            class="settings-nav-item"
            :class="{ 'is-active': activeSettingsSection === card.id }"
            @click="activeSettingsSection = card.id"
          >
            <span class="settings-nav-item__title">{{ card.title }}</span>
            <span v-if="card.dirty" class="settings-nav-item__dot"></span>
          </button>
        </nav>
      </aside>

      <!-- 右侧内容区 -->
      <main class="settings-main">
        <div class="settings-main-stack">
          <!-- 项目与工作区设置 -->
          <section
            v-show="isSettingsSectionVisible('project-workspace')"
            :ref="el => registerSettingsSectionRef('project-workspace', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('project-workspace')"
          >
            <n-card :title="t('settings.projectWorkspaceSettings')" size="huge">
              <n-form
                :label-placement="standardFormLabelPlacement"
                :label-width="standardFormLabelWidth"
              >
                <n-form-item :label="t('settings.pageTitle')" data-search-key="pageTitle">
                  <n-space vertical size="small" style="width: 100%">
                    <n-space :wrap="false" align="start">
                      <n-input
                        v-model:value="pageTitleInput"
                        clearable
                        :disabled="!pageTitleSettingsLoaded || pageTitleSettingsSaving"
                        :placeholder="t('settings.pageTitlePlaceholder')"
                        :status="pageTitleError ? 'error' : undefined"
                      />
                      <n-button
                        type="primary"
                        :loading="pageTitleSettingsSaving"
                        :disabled="!pageTitleSettingsLoaded || !pageTitleDirty || !!pageTitleError"
                        @click="handleSavePageTitle"
                      >
                        {{ t('common.save') }}
                      </n-button>
                    </n-space>
                    <span :class="pageTitleError ? 'form-error' : 'form-tip'">
                      {{ pageTitleError || t('settings.pageTitleTip') }}
                    </span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.recentProjectsLimit')"
                  data-search-key="recentProjectsLimit"
                >
                  <n-space vertical size="small">
                    <n-input-number
                      v-model:value="recentProjectsLimitValue"
                      :min="1"
                      :max="20"
                      :step="1"
                    />
                    <span class="form-tip">{{ t('settings.recentProjectsLimitTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.dailyTipEnabled')"
                  data-search-key="dailyTipEnabled"
                >
                  <n-space vertical size="small">
                    <n-switch
                      :value="dailyTipEnabledValue"
                      :loading="dailyTipSettingsSaving"
                      :disabled="!dailyTipSettingsLoaded || dailyTipSettingsSaving"
                      @update:value="handleDailyTipEnabledChange"
                    />
                    <n-button size="small" @click="handleShowRandomDailyTip">
                      {{ t('settings.dailyTipShowRandom') }}
                    </n-button>
                    <span class="form-tip">{{ t('settings.dailyTipEnabledTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.terminalShortcut')"
                  data-search-key="terminalShortcut"
                >
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
                        {{
                          isCapturing('terminal')
                            ? t('settings.recording')
                            : t('settings.recordNewKey')
                        }}
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
                <n-form-item
                  :label="t('settings.notepadShortcut')"
                  data-search-key="notepadShortcut"
                >
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
                        {{
                          isCapturing('notepad')
                            ? t('settings.recording')
                            : t('settings.recordNewKey')
                        }}
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
                <n-form-item :label="t('settings.defaultEditor')" data-search-key="defaultEditor">
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="defaultEditorValue"
                      :options="editorOptions"
                      style="max-width: 240px"
                    />
                    <span class="form-tip">{{ t('settings.defaultEditorTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  v-if="showCustomEditorInput"
                  :label="t('settings.customCommand')"
                  data-search-key="customCommand"
                >
                  <div class="settings-field-stack">
                    <n-input
                      v-model:value="customEditorCommandValue"
                      class="settings-command-input"
                      :placeholder="customCommandPlaceholder"
                    />
                    <span class="form-tip">
                      {{ customCommandTip }}
                    </span>
                  </div>
                </n-form-item>
              </n-form>
            </n-card>
          </section>

          <section
            v-show="isSettingsSectionVisible('terminal')"
            :ref="el => registerSettingsSectionRef('terminal', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('terminal')"
          >
            <n-card :title="t('settings.terminalSettings')" size="huge">
              <template #header-extra>
                <n-button
                  size="small"
                  :loading="developerSaving"
                  :disabled="!developerTerminalDirty || developerLoading"
                  @click="handleSaveDeveloperConfig"
                >
                  {{ t('common.save') }}
                </n-button>
              </template>
              <n-form
                :label-placement="standardFormLabelPlacement"
                :label-width="standardFormLabelWidth"
              >
                <n-form-item :label="t('settings.terminalLimit')" data-search-key="terminalLimit">
                  <n-space vertical size="small">
                    <n-input-number
                      v-model:value="terminalLimitValue"
                      :min="1"
                      :max="24"
                      :step="1"
                    />
                    <span class="form-tip">{{ t('settings.terminalLimitTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.confirmTerminalClose')"
                  data-search-key="confirmTerminalClose"
                >
                  <n-space vertical size="small">
                    <n-switch v-model:value="confirmTerminalCloseValue" />
                    <span class="form-tip">{{ t('settings.confirmTerminalCloseTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.sendResizeOnSwitch')"
                  data-search-key="sendResizeOnSwitch"
                >
                  <n-space vertical size="small">
                    <n-switch v-model:value="sendResizeOnSwitchValue" />
                    <span class="form-tip">{{ t('settings.sendResizeOnSwitchTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.terminalDefaultRenderMode')"
                  data-search-key="terminalDefaultRenderMode"
                >
                  <n-space vertical size="small">
                    <n-radio-group v-model:value="defaultTerminalRenderModeValue">
                      <n-space>
                        <n-radio value="live">{{ t('settings.terminalRenderModeLive') }}</n-radio>
                        <n-radio value="snapshot">
                          {{ t('settings.terminalRenderModeSnapshot') }}
                        </n-radio>
                      </n-space>
                    </n-radio-group>
                    <span class="form-tip">{{ t('settings.terminalDefaultRenderModeTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.terminalConnectionPolicy')"
                  data-search-key="terminalConnectionPolicy"
                >
                  <n-space vertical size="small">
                    <n-radio-group v-model:value="terminalConnectionPolicyValue">
                      <n-space vertical size="small">
                        <n-radio value="active-only">
                          {{ t('settings.terminalConnectionPolicyActiveOnly') }}
                        </n-radio>
                        <n-radio value="active-plus-mirror">
                          {{ t('settings.terminalConnectionPolicyActivePlusMirror') }}
                        </n-radio>
                      </n-space>
                    </n-radio-group>
                    <span class="form-tip">{{ t('settings.terminalConnectionPolicyTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.terminalDefaultSnapshotInterval')"
                  data-search-key="terminalDefaultSnapshotInterval"
                >
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="defaultTerminalSnapshotIntervalValue"
                      :options="snapshotIntervalOptions"
                      style="max-width: 180px"
                    />
                    <span class="form-tip">
                      {{ t('settings.terminalDefaultSnapshotIntervalTip') }}
                    </span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.inactiveTerminalSnapshotInterval')"
                  data-search-key="inactiveTerminalSnapshotInterval"
                >
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="inactiveTerminalSnapshotIntervalValue"
                      :options="inactiveSnapshotIntervalOptions"
                      style="max-width: 180px"
                      :disabled="terminalConnectionPolicyValue !== 'active-plus-mirror'"
                    />
                    <span class="form-tip">
                      {{ t('settings.inactiveTerminalSnapshotIntervalTip') }}
                    </span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.terminalSnapshotZlibCompression')"
                  data-search-key="terminalSnapshotZlibCompression"
                >
                  <n-space vertical size="small">
                    <n-switch v-model:value="defaultTerminalSnapshotZlibCompressionValue" />
                    <span class="form-tip">
                      {{ t('settings.terminalSnapshotZlibCompressionTip') }}
                    </span>
                  </n-space>
                </n-form-item>
                <n-form-item :label="t('settings.terminalShell')" data-search-key="terminalShell">
                  <div class="settings-field-stack">
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
                      <div class="settings-collapsible-field">
                        <n-input
                          v-model:value="customShellCommand"
                          class="settings-command-input settings-command-input--shell"
                          :placeholder="t('settings.customShellPlaceholder')"
                          :status="customShellStatus"
                          @blur="handleCustomShellBlur"
                        />
                      </div>
                    </n-collapse-transition>
                    <span class="form-tip">{{ t('settings.terminalShellTip') }}</span>
                    <span v-if="shellsData?.platform" class="form-tip">
                      {{ t('settings.currentPlatform') }}: {{ platformDisplayName }}
                    </span>
                  </div>
                </n-form-item>
                <n-form-item
                  :label="t('settings.terminalServerStateSnapshot')"
                  data-search-key="terminalServerStateSnapshot"
                >
                  <n-space vertical size="small">
                    <n-switch
                      v-model:value="developerForm.enableTerminalStateSnapshot"
                      :disabled="developerLoading"
                    />
                    <span class="form-tip">{{ t('settings.terminalServerStateSnapshotTip') }}</span>
                  </n-space>
                </n-form-item>
              </n-form>
            </n-card>

            <n-card :title="t('settings.terminalQuickActions')" size="huge">
              <n-form
                :label-placement="standardFormLabelPlacement"
                :label-width="standardFormLabelWidth"
              >
                <n-form-item
                  :label="t('settings.terminalQuickActionsList')"
                  data-search-key="terminalQuickActions"
                >
                  <n-space vertical size="small" style="width: 100%">
                    <n-dynamic-input
                      v-model:value="terminalQuickActionsLocal"
                      :on-create="createTerminalQuickAction"
                    >
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
                            <div class="terminal-quick-action-icon-grid">
                              <button
                                v-for="option in terminalQuickActionIconButtons"
                                :key="option.value"
                                type="button"
                                class="terminal-quick-action-icon-button"
                                :class="{ 'is-active': value.icon === option.value }"
                                :title="option.label"
                                :aria-pressed="value.icon === option.value"
                                @click="value.icon = option.value"
                              >
                                <span
                                  v-if="'svg' in option && option.svg"
                                  class="terminal-quick-action-svg"
                                  v-html="option.svg"
                                ></span>
                                <n-icon v-else :size="16">
                                  <component :is="option.icon" />
                                </n-icon>
                              </button>
                            </div>
                          </div>
                        </div>
                      </template>
                      <template #action="{ index, remove, create }">
                        <n-button-group size="small">
                          <n-button
                            quaternary
                            circle
                            @click="handleRemoveTerminalQuickAction(index, remove)"
                          >
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
          </section>

          <section
            v-show="isSettingsSectionVisible('session')"
            :ref="el => registerSettingsSectionRef('session', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('session')"
          >
            <n-card :title="t('settings.sessionSettings')" size="huge">
              <template #header-extra>
                <n-button
                  size="small"
                  :loading="developerSaving"
                  :disabled="!developerSessionDirty || developerLoading"
                  @click="handleSaveDeveloperConfig"
                >
                  {{ t('common.save') }}
                </n-button>
              </template>
              <n-form
                :label-placement="standardFormLabelPlacement"
                :label-width="standardFormLabelWidth"
              >
                <n-form-item
                  :label="t('settings.showWebSessionReasoning')"
                  data-search-key="showWebSessionReasoning"
                >
                  <n-space vertical size="small">
                    <n-switch v-model:value="showWebSessionReasoningValue" />
                    <span class="form-tip">{{ t('settings.showWebSessionReasoningTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionActivityDisplayMode')"
                  data-search-key="webSessionActivityDisplayMode"
                >
                  <n-space vertical size="small">
                    <n-radio-group v-model:value="webSessionActivityDisplayModeValue">
                      <n-space>
                        <n-radio value="default">{{ t('common.default') }}</n-radio>
                        <n-radio value="text">{{
                          t('settings.webSessionActivityDisplayModeText')
                        }}</n-radio>
                        <n-radio value="card">{{
                          t('settings.webSessionActivityDisplayModeCard')
                        }}</n-radio>
                      </n-space>
                    </n-radio-group>
                    <span class="form-tip">{{
                      t('settings.webSessionActivityDisplayModeTip')
                    }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionStreamingMarkdownThrottle')"
                  data-search-key="webSessionStreamingMarkdownThrottle"
                >
                  <n-space vertical size="small">
                    <n-radio-group v-model:value="webSessionStreamingMarkdownThrottleModeValue">
                      <n-space>
                        <n-radio value="default">{{ t('common.default') }}</n-radio>
                        <n-radio value="custom">{{ t('common.custom') }}</n-radio>
                      </n-space>
                    </n-radio-group>
                    <n-input-number
                      v-model:value="webSessionStreamingMarkdownThrottleCustomMsValue"
                      :min="1"
                      :step="10"
                      style="max-width: 180px"
                      :disabled="webSessionStreamingMarkdownThrottleModeValue !== 'custom'"
                    />
                    <span class="form-tip">
                      {{
                        t('settings.webSessionStreamingMarkdownThrottleTip', {
                          defaultMs: DEFAULT_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MS,
                        })
                      }}
                    </span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionAutoContinueScope')"
                  data-search-key="webSessionAutoContinueScope"
                >
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="webSessionAutoContinueScopeValue"
                      :options="webSessionAutoContinueScopeOptions"
                      style="max-width: 320px"
                    />
                    <span class="form-tip">{{ t('settings.webSessionAutoContinueScopeTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionAutoContinuePreset')"
                  data-search-key="webSessionAutoContinuePreset"
                >
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="webSessionAutoContinuePresetValue"
                      :options="webSessionAutoContinuePresetOptions"
                      style="max-width: 320px"
                    />
                    <span class="form-tip">{{
                      t('settings.webSessionAutoContinuePresetTip')
                    }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionAutoContinueMaxAttempts')"
                  data-search-key="webSessionAutoContinueMaxAttempts"
                >
                  <n-space vertical size="small">
                    <n-input-number
                      v-model:value="webSessionAutoContinueMaxAttemptsValue"
                      :min="0"
                      :max="100"
                      :step="1"
                      style="max-width: 180px"
                    />
                    <span class="form-tip">{{
                      t('settings.webSessionAutoContinueMaxAttemptsTip')
                    }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionAutoRetryDispatchPendingOnFailure')"
                  data-search-key="webSessionAutoRetryDispatchPendingOnFailure"
                >
                  <n-space vertical size="small">
                    <n-checkbox v-model:checked="webSessionAutoRetryDispatchPendingOnFailureValue">
                      {{ t('settings.webSessionAutoRetryDispatchPendingOnFailureEnabled') }}
                    </n-checkbox>
                    <span class="form-tip">{{
                      t('settings.webSessionAutoRetryDispatchPendingOnFailureTip')
                    }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionCodexDefaultModel')"
                  data-search-key="webSessionCodexDefaultModel"
                >
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="developerForm.webSessionCodexDefaultModel"
                      :options="webSessionCodexDefaultModelOptions"
                      :disabled="developerLoading"
                      filterable
                      tag
                      style="max-width: 320px"
                    />
                    <span class="form-tip">{{ t('settings.webSessionCodexDefaultModelTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionCodexDefaultReasoningEffort')"
                  data-search-key="webSessionCodexDefaultReasoningEffort"
                >
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="developerForm.webSessionCodexDefaultReasoningEffort"
                      :options="webSessionCodexDefaultReasoningEffortOptions"
                      :disabled="developerLoading"
                      style="max-width: 320px"
                    />
                    <span class="form-tip">{{
                      t('settings.webSessionCodexDefaultReasoningEffortTip')
                    }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionCodexDefaultPermissionLevel')"
                  data-search-key="webSessionCodexDefaultPermissionLevel"
                >
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="developerForm.webSessionCodexDefaultPermissionLevel"
                      :options="webSessionCodexDefaultPermissionLevelOptions"
                      :disabled="developerLoading"
                      style="max-width: 320px"
                    />
                    <span class="form-tip">{{
                      t('settings.webSessionCodexDefaultPermissionLevelTip')
                    }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item
                  :label="t('settings.webSessionCodexDefaultSyncMode')"
                  data-search-key="webSessionCodexDefaultSyncMode"
                >
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="developerForm.webSessionCodexDefaultSyncMode"
                      :options="webSessionSyncModeOptions"
                      :disabled="developerLoading"
                      style="max-width: 320px"
                    />
                    <span class="form-tip">{{
                      t('settings.webSessionCodexDefaultSyncModeTip')
                    }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item :label="t('settings.webSessionActiveCallTimeout')">
                  <n-space vertical size="small">
                    <n-radio-group
                      v-model:value="developerForm.webSessionActiveCallTimeout.enabledMode"
                      :disabled="developerLoading"
                    >
                      <n-space>
                        <n-radio value="default">{{ t('common.default') }}</n-radio>
                        <n-radio value="on">{{ t('common.yes') }}</n-radio>
                        <n-radio value="off">{{ t('common.no') }}</n-radio>
                      </n-space>
                    </n-radio-group>
                    <span class="form-tip">{{ t('settings.webSessionActiveCallTimeoutTip') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item :label="t('settings.webSessionActiveCallTimeoutSeconds')">
                  <n-space vertical size="small">
                    <n-radio-group
                      v-model:value="developerForm.webSessionActiveCallTimeout.timeoutMode"
                      :disabled="developerLoading"
                    >
                      <n-space>
                        <n-radio value="default">{{ t('common.default') }}</n-radio>
                        <n-radio value="custom">{{ t('common.custom') }}</n-radio>
                      </n-space>
                    </n-radio-group>
                    <n-input-number
                      v-if="developerUsesCustomActiveCallTimeout"
                      v-model:value="developerForm.webSessionActiveCallTimeout.customTimeoutSeconds"
                      :min="10"
                      :step="10"
                      :disabled="developerLoading"
                    />
                    <span class="form-tip">
                      {{
                        t('settings.webSessionActiveCallTimeoutSecondsTip', {
                          defaultSeconds: DEFAULT_ACTIVE_CALL_TIMEOUT_CUSTOM_SECONDS,
                        })
                      }}
                    </span>
                  </n-space>
                </n-form-item>
                <n-form-item :label="t('settings.webSessionActiveCallTimeoutCallKinds')">
                  <n-space vertical size="small">
                    <n-space>
                      <n-checkbox
                        v-model:checked="
                          developerForm.webSessionActiveCallTimeout.callKinds.useDefault
                        "
                        :disabled="developerLoading"
                      >
                        {{ t('settings.webSessionActiveCallTimeoutKindDefault') }}
                      </n-checkbox>
                    </n-space>
                    <n-space>
                      <n-checkbox
                        v-model:checked="developerForm.webSessionActiveCallTimeout.callKinds.mcp"
                        :disabled="
                          developerLoading ||
                          developerForm.webSessionActiveCallTimeout.callKinds.useDefault
                        "
                      >
                        {{ t('settings.webSessionActiveCallTimeoutKindMcp') }}
                      </n-checkbox>
                      <n-checkbox
                        v-model:checked="
                          developerForm.webSessionActiveCallTimeout.callKinds.command
                        "
                        :disabled="
                          developerLoading ||
                          developerForm.webSessionActiveCallTimeout.callKinds.useDefault
                        "
                      >
                        {{ t('settings.webSessionActiveCallTimeoutKindCommand') }}
                      </n-checkbox>
                      <n-checkbox
                        v-model:checked="developerForm.webSessionActiveCallTimeout.callKinds.tool"
                        :disabled="
                          developerLoading ||
                          developerForm.webSessionActiveCallTimeout.callKinds.useDefault
                        "
                      >
                        {{ t('settings.webSessionActiveCallTimeoutKindTool') }}
                      </n-checkbox>
                    </n-space>
                    <span class="form-tip">
                      {{ t('settings.webSessionActiveCallTimeoutCallKindsTip') }}
                    </span>
                  </n-space>
                </n-form-item>
                <n-form-item :label="t('settings.webSessionActiveCallTimeoutPrompt')">
                  <n-space vertical size="small" style="width: 100%">
                    <n-input
                      v-model:value="developerForm.webSessionActiveCallTimeout.promptTemplate"
                      type="textarea"
                      :autosize="{ minRows: 3, maxRows: 6 }"
                      :placeholder="DEFAULT_ACTIVE_CALL_TIMEOUT_PROMPT"
                      :disabled="developerLoading"
                    />
                    <span class="form-tip">
                      {{ t('settings.webSessionActiveCallTimeoutPromptTip') }}
                    </span>
                  </n-space>
                </n-form-item>
                <n-form-item :label="t('settings.webSessionQuickInputPinned')">
                  <n-space vertical size="small" style="width: 100%">
                    <n-dynamic-input
                      v-model:value="webSessionQuickInputPinnedLocal"
                      :on-create="createWebSessionQuickInputPinnedItem"
                    >
                      <template #default="{ value, index }">
                        <n-input
                          type="textarea"
                          class="web-session-quick-input-textarea"
                          :value="value"
                          :autosize="{ minRows: 2, maxRows: 4 }"
                          :placeholder="t('settings.webSessionQuickInputPinnedPlaceholder')"
                          @update:value="handleWebSessionQuickInputPinnedChange(index, $event)"
                        />
                      </template>
                      <template #action="{ index, remove, create }">
                        <n-button-group size="small">
                          <n-button quaternary circle @click="remove(index)">
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
                      <n-button
                        size="small"
                        type="primary"
                        :loading="webSessionQuickInputPinnedSaving"
                        :disabled="!webSessionQuickInputPinnedDirty"
                        @click="handleSaveWebSessionQuickInputPinned"
                      >
                        {{ t('common.save') }}
                      </n-button>
                      <n-button size="small" @click="handleResetWebSessionQuickInputPinned">
                        {{ t('settings.restoreDefault') }}
                      </n-button>
                    </n-space>
                    <span class="form-tip">{{ t('settings.webSessionQuickInputPinnedTip') }}</span>
                  </n-space>
                </n-form-item>
              </n-form>
            </n-card>
          </section>

          <section
            v-show="isSettingsSectionVisible('security')"
            :ref="el => registerSettingsSectionRef('security', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('security')"
          >
            <n-card :title="t('settings.securityTitle')" size="huge">
              <n-space vertical size="large">
                <n-alert
                  :type="authStore.enabled ? 'warning' : 'info'"
                  :bordered="false"
                  :show-icon="false"
                >
                  {{
                    authStore.enabled
                      ? t('settings.securityEnabledHint')
                      : t('settings.securityDisabledHint')
                  }}
                </n-alert>

                <n-alert
                  v-if="showSecurityAdminLoginPrompt"
                  type="info"
                  :bordered="false"
                  :show-icon="false"
                >
                  <n-space justify="space-between" align="center" :wrap="false" style="width: 100%">
                    <span>{{ t('settings.securityAdminLoginHint') }}</span>
                    <n-button
                      size="small"
                      type="primary"
                      secondary
                      @click="openSecurityAdminLoginDialog"
                    >
                      {{ t('settings.securityAdminLoginAction') }}
                    </n-button>
                  </n-space>
                </n-alert>

                <template v-if="!authStore.enabled">
                  <n-form
                    :label-placement="standardFormLabelPlacement"
                    :label-width="standardFormLabelWidth"
                  >
                    <n-form-item
                      :label="t('settings.securityNewPassword')"
                      data-search-key="securityNewPassword"
                    >
                      <n-input
                        v-model:value="enablePassword"
                        type="password"
                        show-password-on="click"
                        :placeholder="t('settings.securityPasswordPlaceholder')"
                      />
                    </n-form-item>
                    <n-form-item
                      :label="t('settings.securityConfirmPassword')"
                      data-search-key="securityConfirmPassword"
                    >
                      <n-input
                        v-model:value="enablePasswordConfirm"
                        type="password"
                        show-password-on="click"
                        :placeholder="t('settings.securityConfirmPasswordPlaceholder')"
                      />
                    </n-form-item>
                    <n-form-item
                      :label="actionFormItemLabel"
                      data-search-key="securityEnablePassword"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-button
                          type="primary"
                          :loading="authSaving"
                          :disabled="!enablePassword.trim() || !enablePasswordConfirm.trim()"
                          @click="handleEnablePasswordProtection"
                        >
                          {{ t('settings.securityEnableAction') }}
                        </n-button>
                        <span class="form-tip">{{ t('settings.securityAlgorithmHint') }}</span>
                      </n-space>
                    </n-form-item>
                  </n-form>
                </template>

                <template v-else>
                  <n-form
                    :label-placement="standardFormLabelPlacement"
                    :label-width="standardFormLabelWidth"
                  >
                    <n-form-item
                      :label="t('settings.securityCurrentPassword')"
                      data-search-key="securityCurrentPassword"
                    >
                      <n-input
                        v-model:value="currentPassword"
                        type="password"
                        show-password-on="click"
                        :placeholder="t('settings.securityCurrentPasswordPlaceholder')"
                        :disabled="securityManagementLocked || authSaving"
                      />
                    </n-form-item>
                    <n-form-item
                      :label="t('settings.securityNewPassword')"
                      data-search-key="securityChangePassword"
                    >
                      <n-input
                        v-model:value="newPassword"
                        type="password"
                        show-password-on="click"
                        :placeholder="t('settings.securityPasswordPlaceholder')"
                        :disabled="securityManagementLocked || authSaving"
                      />
                    </n-form-item>
                    <n-form-item
                      :label="t('settings.securityConfirmPassword')"
                      data-search-key="securityChangeConfirmPassword"
                    >
                      <n-input
                        v-model:value="newPasswordConfirm"
                        type="password"
                        show-password-on="click"
                        :placeholder="t('settings.securityConfirmPasswordPlaceholder')"
                        :disabled="securityManagementLocked || authSaving"
                      />
                    </n-form-item>
                    <n-form-item
                      :label="actionFormItemLabel"
                      data-search-key="securityChangePasswordButton"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-button
                          type="primary"
                          :loading="authSaving"
                          :disabled="
                            securityManagementLocked ||
                            !currentPassword.trim() ||
                            !newPassword.trim() ||
                            !newPasswordConfirm.trim()
                          "
                          @click="handleChangePasswordProtection"
                        >
                          {{ t('settings.securityChangeAction') }}
                        </n-button>
                        <span class="form-tip">{{ t('settings.securityRotateHint') }}</span>
                      </n-space>
                    </n-form-item>
                  </n-form>

                  <n-divider style="margin: 0" />

                  <n-form
                    :label-placement="standardFormLabelPlacement"
                    :label-width="standardFormLabelWidth"
                  >
                    <n-form-item
                      :label="t('settings.securityDisablePassword')"
                      data-search-key="securityDisablePassword"
                    >
                      <n-input
                        v-model:value="disablePassword"
                        type="password"
                        show-password-on="click"
                        :placeholder="t('settings.securityCurrentPasswordPlaceholder')"
                        :disabled="securityManagementLocked || authSaving"
                      />
                    </n-form-item>
                    <n-form-item
                      :label="actionFormItemLabel"
                      data-search-key="securityDisablePasswordButton"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-button
                          type="error"
                          ghost
                          :loading="authSaving"
                          :disabled="securityManagementLocked || !disablePassword.trim()"
                          @click="handleDisablePasswordProtection"
                        >
                          {{ t('settings.securityDisableAction') }}
                        </n-button>
                        <span class="form-tip">{{ t('settings.securityDisableHint') }}</span>
                      </n-space>
                    </n-form-item>
                  </n-form>
                </template>

                <n-divider style="margin: 0">{{
                  t('settings.securityAccessRulesTitle')
                }}</n-divider>
                <n-alert type="info" :bordered="false" :show-icon="false">
                  {{ t('settings.securityAccessRulesHint') }}
                </n-alert>
                <n-spin :show="authAccessLoading">
                  <n-form
                    :label-placement="standardFormLabelPlacement"
                    :label-width="standardFormLabelWidth"
                  >
                    <n-form-item
                      :label="t('settings.securityAccessRulesBypassIPs')"
                      data-search-key="securityAccessRulesBypassIPs"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-input
                          v-model:value="authAccessForm.bypassIPs"
                          type="textarea"
                          :autosize="{ minRows: 2, maxRows: 6 }"
                          :placeholder="t('settings.securityAccessRulesBypassIPsPlaceholder')"
                          :disabled="authAccessLoading || securityManagementLocked"
                        />
                        <span class="form-tip">{{
                          t('settings.securityAccessRulesBypassIPsTip')
                        }}</span>
                      </n-space>
                    </n-form-item>
                    <n-form-item
                      :label="t('settings.securityAccessRulesBypassDomains')"
                      data-search-key="securityAccessRulesBypassDomains"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-input
                          v-model:value="authAccessForm.bypassDomains"
                          type="textarea"
                          :autosize="{ minRows: 2, maxRows: 6 }"
                          :placeholder="t('settings.securityAccessRulesBypassDomainsPlaceholder')"
                          :disabled="authAccessLoading || securityManagementLocked"
                        />
                        <span class="form-tip">{{
                          t('settings.securityAccessRulesBypassDomainsTip')
                        }}</span>
                      </n-space>
                    </n-form-item>
                    <n-form-item
                      :label="t('settings.securityAccessRulesForceAuthIPs')"
                      data-search-key="securityAccessRulesForceAuthIPs"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-input
                          v-model:value="authAccessForm.forceAuthIPs"
                          type="textarea"
                          :autosize="{ minRows: 2, maxRows: 6 }"
                          :placeholder="t('settings.securityAccessRulesForceAuthIPsPlaceholder')"
                          :disabled="authAccessLoading || securityManagementLocked"
                        />
                        <span class="form-tip">{{
                          t('settings.securityAccessRulesForceAuthIPsTip')
                        }}</span>
                      </n-space>
                    </n-form-item>
                    <n-form-item
                      :label="t('settings.securityAccessRulesForceAuthDomains')"
                      data-search-key="securityAccessRulesForceAuthDomains"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-input
                          v-model:value="authAccessForm.forceAuthDomains"
                          type="textarea"
                          :autosize="{ minRows: 2, maxRows: 6 }"
                          :placeholder="
                            t('settings.securityAccessRulesForceAuthDomainsPlaceholder')
                          "
                          :disabled="authAccessLoading || securityManagementLocked"
                        />
                        <span class="form-tip">{{
                          t('settings.securityAccessRulesForceAuthDomainsTip')
                        }}</span>
                      </n-space>
                    </n-form-item>
                    <n-form-item
                      :label="t('settings.securityTrustedProxies')"
                      data-search-key="securityTrustedProxies"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-input
                          v-model:value="authAccessForm.trustedProxies"
                          type="textarea"
                          :autosize="{ minRows: 2, maxRows: 6 }"
                          :placeholder="t('settings.securityTrustedProxiesPlaceholder')"
                          :disabled="authAccessLoading || securityManagementLocked"
                        />
                        <span class="form-tip">{{ t('settings.securityTrustedProxiesTip') }}</span>
                      </n-space>
                    </n-form-item>
                    <n-form-item
                      :label="actionFormItemLabel"
                      data-search-key="securityAccessRulesSave"
                    >
                      <n-space vertical size="small" style="width: 100%">
                        <n-space>
                          <n-button
                            type="primary"
                            :loading="authAccessSaving"
                            :disabled="
                              authAccessLoading || securityManagementLocked || !authAccessDirty
                            "
                            @click="handleSaveAuthAccessConfig"
                          >
                            {{ t('common.save') }}
                          </n-button>
                          <n-button
                            :disabled="
                              authAccessLoading || securityManagementLocked || !authAccessDirty
                            "
                            @click="handleResetAuthAccessConfig"
                          >
                            {{ t('common.reset') }}
                          </n-button>
                        </n-space>
                        <span class="form-tip">{{ t('settings.securityAccessRulesSaveTip') }}</span>
                      </n-space>
                    </n-form-item>
                  </n-form>
                </n-spin>
              </n-space>
            </n-card>
          </section>

          <section
            v-show="isSettingsSectionVisible('developer')"
            :ref="el => registerSettingsSectionRef('developer', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('developer')"
          >
            <n-card :title="t('settings.developerOptions')" size="huge">
              <template #header-extra>
                <n-button
                  size="small"
                  :loading="developerSaving"
                  :disabled="!developerBehaviorDirty || developerLoading"
                  @click="handleSaveDeveloperConfig"
                >
                  {{ t('common.save') }}
                </n-button>
              </template>
              <n-spin :show="developerLoading">
                <n-form
                  :label-placement="standardFormLabelPlacement"
                  :label-width="standardFormLabelWidth"
                >
                  <n-form-item
                    :label="t('settings.developerScrollback')"
                    data-search-key="developerScrollback"
                  >
                    <n-space vertical size="small">
                      <n-switch
                        v-model:value="developerForm.enableTerminalScrollback"
                        :disabled="developerLoading"
                      />
                      <span class="form-tip">{{ t('settings.developerScrollbackTip') }}</span>
                    </n-space>
                  </n-form-item>
                </n-form>
              </n-spin>
            </n-card>
          </section>

          <section
            v-show="isSettingsSectionVisible('git')"
            :ref="el => registerSettingsSectionRef('git', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('git')"
          >
            <GitSettingsSection />
          </section>

          <section
            v-show="isSettingsSectionVisible('worktree')"
            :ref="el => registerSettingsSectionRef('worktree', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('worktree')"
          >
            <n-card :title="t('settings.worktreeSettings')" size="huge">
              <template #header-extra>
                <n-button
                  size="small"
                  :loading="worktreeSettingsSaving"
                  :disabled="
                    !worktreeSettingsDirty || worktreeSettingsLoading || !!globalBaseDirError
                  "
                  @click="handleSaveWorktreeSettings"
                >
                  {{ t('common.save') }}
                </n-button>
              </template>
              <n-spin :show="worktreeSettingsLoading">
                <n-form
                  :label-placement="standardFormLabelPlacement"
                  :label-width="standardFormLabelWidth"
                >
                  <n-form-item
                    :label="t('settings.worktreeGlobalBaseDir')"
                    :validation-status="globalBaseDirError ? 'error' : undefined"
                    :feedback="globalBaseDirError"
                    data-search-key="worktreeGlobalBaseDir"
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
                  <n-form-item
                    :label="t('settings.worktreeGlobalDirNamePattern')"
                    data-search-key="worktreeGlobalDirNamePattern"
                  >
                    <n-space vertical size="small" style="width: 100%">
                      <n-input v-model:value="worktreeSettingsForm.globalDirNamePattern" />
                      <span class="form-tip">{{
                        t('settings.worktreeGlobalDirNamePatternTip')
                      }}</span>
                    </n-space>
                  </n-form-item>
                </n-form>
              </n-spin>
            </n-card>
          </section>

          <!-- 主题设置 -->
          <section
            v-show="isSettingsSectionVisible('theme')"
            :ref="el => registerSettingsSectionRef('theme', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('theme')"
          >
            <n-card :title="t('settings.themeSettings')" size="huge">
              <n-form
                :label-placement="standardFormLabelPlacement"
                :label-width="themeFormLabelWidth"
              >
                <n-form-item :label="t('theme.presetTheme')" data-search-key="presetTheme">
                  <n-select
                    :value="currentPresetValue"
                    :options="presetOptions"
                    :disabled="followSystemValue"
                    style="max-width: 240px"
                    @update:value="handlePresetThemeChange"
                  />
                </n-form-item>
                <n-form-item :label="t('theme.followSystem')" data-search-key="followSystem">
                  <n-space vertical size="small">
                    <n-radio-group
                      :value="followSystemModeValue"
                      @update:value="handleFollowSystemModeChange"
                    >
                      <n-space>
                        <n-radio :value="-1">{{ t('common.default') }}</n-radio>
                        <n-radio :value="0">{{ t('common.no') }}</n-radio>
                        <n-radio :value="1">{{ t('common.yes') }}</n-radio>
                      </n-space>
                    </n-radio-group>
                    <span class="form-tip">{{ t('theme.followSystemHint') }}</span>
                  </n-space>
                </n-form-item>
                <n-form-item :label="t('settings.terminalTheme')" data-search-key="terminalTheme">
                  <n-space vertical size="small">
                    <n-select
                      v-model:value="terminalThemeValue"
                      :options="terminalThemeOptions"
                      style="max-width: 240px"
                    />
                    <span class="form-tip">{{ t('settings.terminalThemeTip') }}</span>
                  </n-space>
                </n-form-item>

                <n-divider style="margin: 16px 0">{{
                  t('settings.terminalFontSettings')
                }}</n-divider>

                <n-form-item
                  :label="t('settings.terminalFontFamily')"
                  data-search-key="terminalFontFamily"
                >
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

                <n-form-item
                  :label="t('settings.terminalFontSize')"
                  data-search-key="terminalFontSize"
                >
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

                <n-form-item
                  :label="t('settings.terminalFontWeight')"
                  data-search-key="terminalFontWeight"
                >
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

                <n-form-item
                  :label="t('settings.terminalLineHeight')"
                  data-search-key="terminalLineHeight"
                >
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

                <n-form-item
                  :label="t('settings.terminalLetterSpacing')"
                  data-search-key="terminalLetterSpacing"
                >
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

                <n-form-item
                  :label="t('settings.terminalWebGLRenderer')"
                  data-search-key="terminalWebGLRenderer"
                >
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

                <n-alert
                  v-if="hasCustomTheme"
                  type="info"
                  style="margin-bottom: 16px"
                  :bordered="false"
                >
                  {{ t('theme.customThemeHint') }}
                </n-alert>

                <n-form-item :label="t('settings.primaryColor')" data-search-key="primaryColor">
                  <n-color-picker
                    v-model:value="primaryColor"
                    :modes="['hex']"
                    :actions="['confirm']"
                  />
                </n-form-item>
                <n-form-item :label="t('settings.bodyColor')" data-search-key="bodyColor">
                  <n-color-picker
                    v-model:value="bodyColor"
                    :modes="['hex']"
                    :actions="['confirm']"
                  />
                </n-form-item>
                <n-form-item :label="t('settings.surfaceColor')" data-search-key="surfaceColor">
                  <n-color-picker
                    v-model:value="surfaceColor"
                    :modes="['hex']"
                    :actions="['confirm']"
                  />
                </n-form-item>
                <n-form-item :label="t('settings.textColor')" data-search-key="textColor">
                  <n-color-picker
                    v-model:value="textColor"
                    :modes="['hex']"
                    :actions="['confirm']"
                  />
                </n-form-item>

                <n-divider style="margin: 16px 0">{{ t('theme.terminalColors') }}</n-divider>

                <n-form-item :label="t('settings.terminalBg')" data-search-key="terminalBg">
                  <n-color-picker
                    v-model:value="terminalBg"
                    :modes="['hex']"
                    :actions="['confirm']"
                  />
                </n-form-item>
                <n-form-item :label="t('settings.terminalFg')" data-search-key="terminalFg">
                  <n-color-picker
                    v-model:value="terminalFg"
                    :modes="['hex']"
                    :actions="['confirm']"
                  />
                </n-form-item>
                <n-form-item :label="t('settings.terminalTabBg')" data-search-key="terminalTabBg">
                  <n-color-picker
                    v-model:value="terminalTabBg"
                    :modes="['hex']"
                    :actions="['confirm']"
                  />
                </n-form-item>
                <n-form-item
                  :label="t('settings.terminalTabActiveBg')"
                  data-search-key="terminalTabActiveBg"
                >
                  <n-color-picker
                    v-model:value="terminalTabActiveBg"
                    :modes="['hex']"
                    :actions="['confirm']"
                  />
                </n-form-item>
              </n-form>
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
          </section>

          <section
            v-show="isSettingsSectionVisible('maintenance')"
            :ref="el => registerSettingsSectionRef('maintenance', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('maintenance')"
          >
            <n-card :title="t('settings.dataMaintenanceTitle')" size="huge">
              <WebSessionHistoryCleanup />
            </n-card>
          </section>

          <section
            v-show="isSettingsSectionVisible('backup')"
            :ref="el => registerSettingsSectionRef('backup', el as HTMLElement | null)"
            class="settings-card-shell"
            :class="settingsCardShellClass('backup')"
          >
            <n-card :title="t('settings.backupTitle')" size="huge">
              <n-space vertical size="large">
                <n-alert type="info" :bordered="false" :show-icon="false">
                  {{ t('settings.backupDescription') }}
                </n-alert>
                <div class="settings-backup-group">
                  <div class="settings-backup-group__title">
                    {{ t('settings.backupExportGroupTitle') }}
                  </div>
                  <n-space vertical size="small">
                    <n-checkbox v-model:checked="settingsBackupExportOptions.includeServer">
                      {{ t('settings.backupOptionIncludeServer') }}
                    </n-checkbox>
                    <n-checkbox v-model:checked="settingsBackupExportOptions.includeClient">
                      {{ t('settings.backupOptionIncludeClient') }}
                    </n-checkbox>
                    <n-switch
                      v-model:value="settingsBackupExportOptions.includeQuickInputRecent"
                      :disabled="
                        !settingsBackupExportOptions.includeServer &&
                        !settingsBackupExportOptions.includeClient
                      "
                    >
                      <template #checked>{{ t('common.yes') }}</template>
                      <template #unchecked>{{ t('common.no') }}</template>
                    </n-switch>
                    <span class="form-tip">{{ t('settings.backupOptionIncludeRecent') }}</span>
                  </n-space>
                  <div class="settings-backup-subgroup">
                    <div class="settings-backup-subgroup__title">
                      {{ t('settings.backupAdvancedTitle') }}
                    </div>
                    <n-space vertical size="small">
                      <n-switch
                        v-model:value="settingsBackupExportOptions.includeSecurityAccess"
                        :disabled="!settingsBackupExportOptions.includeServer"
                      >
                        <template #checked>{{ t('common.yes') }}</template>
                        <template #unchecked>{{ t('common.no') }}</template>
                      </n-switch>
                      <span class="form-tip">{{
                        t('settings.backupOptionIncludeSecurityAccess')
                      }}</span>
                      <n-select
                        v-model:value="settingsBackupExportOptions.fileNameRule"
                        :options="settingsBackupFileNameRuleOptions"
                      />
                      <n-switch v-model:value="settingsBackupExportOptions.includeMetadata">
                        <template #checked>{{ t('common.yes') }}</template>
                        <template #unchecked>{{ t('common.no') }}</template>
                      </n-switch>
                      <span class="form-tip">{{ t('settings.backupOptionIncludeMetadata') }}</span>
                    </n-space>
                  </div>
                  <n-button
                    type="primary"
                    :loading="settingsBackupExporting"
                    :disabled="!settingsBackupCanExport"
                    @click="handleExportSettingsBackup"
                  >
                    {{ t('settings.backupExportAction') }}
                  </n-button>
                </div>
                <div class="settings-backup-group">
                  <div class="settings-backup-group__title">
                    {{ t('settings.backupImportGroupTitle') }}
                  </div>
                  <n-radio-group
                    v-model:value="settingsBackupImportMode"
                    name="settings-backup-mode"
                  >
                    <n-space vertical size="small">
                      <n-radio
                        v-for="option in settingsBackupImportModeOptions"
                        :key="option.value"
                        :value="option.value"
                      >
                        {{ option.label }}
                      </n-radio>
                    </n-space>
                  </n-radio-group>
                  <div class="settings-backup-subgroup">
                    <div class="settings-backup-subgroup__title">
                      {{ t('settings.backupAdvancedTitle') }}
                    </div>
                    <n-space vertical size="small">
                      <n-select
                        v-model:value="settingsBackupVersionWarningMode"
                        :options="settingsBackupVersionWarningModeOptions"
                      />
                      <n-switch v-model:value="settingsBackupAllowBreakingVersionContinue">
                        <template #checked>{{ t('common.yes') }}</template>
                        <template #unchecked>{{ t('common.no') }}</template>
                      </n-switch>
                      <span class="form-tip">
                        {{ t('settings.backupOptionAllowBreakingVersionContinue') }}
                      </span>
                      <span class="form-tip">{{ t('settings.backupImportStrategyTip') }}</span>
                    </n-space>
                  </div>
                  <n-button
                    :loading="settingsBackupImporting"
                    @click="handleChooseSettingsBackupFile"
                  >
                    {{ t('settings.backupImportAction') }}
                  </n-button>
                </div>
                <div v-if="settingsBackupSelectedFileName" class="settings-backup-file-name">
                  {{ t('settings.backupSelectedFile', { name: settingsBackupSelectedFileName }) }}
                </div>
                <span class="form-tip">{{ t('settings.backupVersionTip') }}</span>
                <input
                  ref="settingsBackupFileInputRef"
                  type="file"
                  accept="application/json,.json"
                  class="settings-hidden-file-input"
                  @change="handleSettingsBackupFileChange"
                />
              </n-space>
            </n-card>
          </section>
        </div>
      </main>
    </div>
    <n-modal
      v-model:show="showSecurityAdminLoginDialog"
      preset="card"
      style="width: min(92vw, 420px)"
      :title="t('settings.securityAdminLoginDialogTitle')"
      :mask-closable="!securityAdminLoginLoading"
    >
      <n-space vertical size="large">
        <span class="form-tip">{{ t('settings.securityAdminLoginDialogDescription') }}</span>
        <n-input
          v-model:value="securityAdminLoginPassword"
          type="password"
          show-password-on="click"
          :placeholder="t('settings.securityAdminLoginPasswordPlaceholder')"
          :disabled="securityAdminLoginLoading"
          @keyup.enter="handleSecurityAdminLogin"
        />
        <n-space justify="end">
          <n-button
            :disabled="securityAdminLoginLoading"
            @click="handleCloseSecurityAdminLoginDialog"
          >
            {{ t('common.cancel') }}
          </n-button>
          <n-button
            type="primary"
            :loading="securityAdminLoginLoading"
            :disabled="!securityAdminLoginPassword.trim()"
            @click="handleSecurityAdminLogin"
          >
            {{ t('settings.securityAdminLoginAction') }}
          </n-button>
        </n-space>
      </n-space>
    </n-modal>
    <DailyTipDialog
      v-if="activeDailyTip"
      v-model:show="showDailyTipDialog"
      :tip="activeDailyTip"
      :tip-index="activeDailyTipIndex"
      :total-tips="dailyTipCount"
      @next="handleShowAnotherDailyTip"
      @acknowledge="handleDailyTipClose"
      @disable="handleDailyTipDisable"
    />
    <n-modal
      v-model:show="showSettingsBackupPreviewDialog"
      preset="card"
      style="width: min(820px, calc(100vw - 32px))"
      :title="t('settings.backupPreviewTitle')"
    >
      <n-space vertical size="large">
        <n-grid :cols="2" :x-gap="16" :y-gap="12">
          <n-gi>
            <div class="settings-backup-meta-label">{{ t('settings.backupSchemaVersion') }}</div>
            <div class="settings-backup-meta-value">
              {{ settingsBackupPreview?.backupSchemaVersion ?? '-' }}
            </div>
          </n-gi>
          <n-gi>
            <div class="settings-backup-meta-label">{{ t('settings.backupCreatedBy') }}</div>
            <div class="settings-backup-meta-value">
              {{
                settingsBackupPreview?.sourceApp?.version
                  ? `${settingsBackupPreview?.sourceApp?.version} / ${settingsBackupPreview?.sourceApp?.channel || '-'}`
                  : '-'
              }}
            </div>
          </n-gi>
          <n-gi>
            <div class="settings-backup-meta-label">{{ t('settings.backupCurrentVersion') }}</div>
            <div class="settings-backup-meta-value">
              {{
                settingsBackupPreview?.currentApp?.version
                  ? `${settingsBackupPreview?.currentApp?.version} / ${settingsBackupPreview?.currentApp?.channel || '-'}`
                  : '-'
              }}
            </div>
          </n-gi>
          <n-gi>
            <div class="settings-backup-meta-label">{{ t('settings.backupImportStatus') }}</div>
            <div class="settings-backup-meta-value">
              {{
                settingsBackupPreview?.canImport
                  ? t('settings.backupImportAllowed')
                  : t('settings.backupImportBlocked')
              }}
            </div>
          </n-gi>
          <n-gi>
            <div class="settings-backup-meta-label">{{ t('settings.backupCreatedAt') }}</div>
            <div class="settings-backup-meta-value">
              {{ pendingSettingsBackup?.createdAt || '-' }}
            </div>
          </n-gi>
        </n-grid>

        <n-alert type="info" :bordered="false" :show-icon="false">
          {{ settingsBackupImportSummary }}
        </n-alert>

        <n-alert
          v-for="issue in settingsBackupPreviewErrors"
          :key="`backup-error-${issue.code}`"
          type="error"
          :bordered="false"
          :show-icon="false"
        >
          {{ issue.message }}
        </n-alert>
        <n-alert
          v-for="issue in settingsBackupPreviewWarnings"
          :key="`backup-warning-${issue.code}`"
          type="warning"
          :bordered="false"
          :show-icon="false"
        >
          {{ issue.message }}
        </n-alert>
        <n-alert
          v-if="
            settingsBackupHasBreakingVersionMismatch && !settingsBackupAllowBreakingVersionContinue
          "
          type="warning"
          :bordered="false"
          :show-icon="false"
        >
          {{ t('settings.backupBreakingVersionBlocked') }}
        </n-alert>
        <n-alert
          v-if="
            settingsBackupVersionWarningMode === 'strict' && settingsBackupHasAnyVersionMismatch
          "
          type="warning"
          :bordered="false"
          :show-icon="false"
        >
          {{ t('settings.backupStrictVersionBlocked') }}
        </n-alert>

        <div>
          <div class="settings-backup-section-title">{{ t('settings.backupSectionsTitle') }}</div>
          <span class="form-tip">{{ t('settings.backupUncheckedTip') }}</span>
          <div class="settings-backup-section-list">
            <div
              v-for="section in settingsBackupPreviewSections"
              :key="section.key"
              class="settings-backup-section-item"
            >
              <n-checkbox
                :checked="settingsBackupSelectedSectionKeySet.has(section.key)"
                @update:checked="handleToggleSettingsBackupSection(section.key, $event)"
              >
                <span class="settings-backup-section-item__title">{{ section.label }}</span>
              </n-checkbox>
              <div class="settings-backup-section-item__meta">
                {{ section.target }} · {{ section.action }}
              </div>
              <div v-if="section.changedKeys?.length" class="settings-backup-section-item__keys">
                {{ section.changedKeys.join(', ') }}
              </div>
            </div>
          </div>
        </div>

        <n-space justify="end">
          <n-button @click="closeSettingsBackupPreviewDialog">
            {{ t('common.cancel') }}
          </n-button>
          <n-button
            v-if="settingsBackupImportMode !== 'preview-only'"
            type="primary"
            :loading="settingsBackupImporting"
            :disabled="!settingsBackupCanConfirmImport"
            @click="handleConfirmSettingsBackupImport"
          >
            {{ t('settings.backupImportConfirm') }}
          </n-button>
        </n-space>
      </n-space>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useEventListener, useDebounceFn, useStorage } from '@vueuse/core';
import { useDialog, useMessage } from 'naive-ui';
import {
  ChatbubblesOutline,
  CodeOutline,
  ColorPaletteOutline,
  LogoGithub,
  NavigateOutline,
  RefreshOutline,
  RocketOutline,
  SearchOutline,
  SettingsOutline,
  TerminalOutline,
  PlayOutline,
  Add,
  Remove,
} from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import { useResponsive } from '@/composables/useResponsive';
import { useAuthStore, type AuthAccessConfig } from '@/stores/auth';
import { useDeveloperConfigStore } from '@/stores/developerConfig';
import { sanitizeSettingsSectionId, type SettingsSectionId } from '@/stores/settingsUi';
import { getAssistantIconByType } from '@/utils/assistantIcon';
import {
  useSettingsStore,
  DEFAULT_TERMINAL_SHORTCUT,
  DEFAULT_NOTEPAD_SHORTCUT,
  DEFAULT_TERMINAL_QUICK_ACTIONS,
  DEFAULT_WEB_SESSION_QUICK_INPUT_PINNED,
  TERMINAL_FONT_OPTIONS,
  FONT_WEIGHT_OPTIONS,
  type PanelShortcutSetting,
  type EditorPreference,
  type FontWeight,
  type TerminalQuickAction,
  type TerminalQuickActionIcon,
  type WebSessionAutoContinuePreset,
  type WebSessionAutoContinueScope,
  type WebSessionStreamingMarkdownThrottleMode,
  DEFAULT_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MS,
  type FollowSystemThemeSetting,
} from '@/stores/settings';
import type { WebSessionActivityDisplayMode } from '@/constants/webSessionActivityDisplayMode';
import { DEFAULT_EDITOR, EDITOR_OPTIONS, isEditorPreference } from '@/constants/editor';
import {
  DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS,
  TERMINAL_SNAPSHOT_INTERVAL_OPTIONS,
  formatTerminalSnapshotInterval,
  type TerminalRenderMode,
} from '@/constants/terminalRenderMode';
import {
  DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS,
  type TerminalConnectionPolicy,
} from '@/constants/terminalConnectionPolicy';
import { useThemeOptions, useTerminalThemeOptions } from '@/composables/useThemeOptions';
import {
  lightenColor,
  darkenColor,
  ensureHexWithHash,
  isDarkHex,
  getReadableTextColor,
} from '@/utils/color';
import {
  createThemeMaintenanceWarningController,
  createThemeSelectionController,
} from '@/utils/themeMaintenanceWarning';
import Apis from '@/api';
import { http } from '@/api/http';
import { webSessionApi } from '@/api/webSession';
import { useReq, useInit } from '@/api/composable';
import DailyTipDialog from '@/components/common/DailyTipDialog.vue';
import WebSessionHistoryCleanup from '@/components/settings/WebSessionHistoryCleanup.vue';
import GitSettingsSection from '@/components/settings/GitSettingsSection.vue';
import type {
  DeveloperConfig,
  AvailableShellsResponse,
  WebSessionCodexDefaultReasoningEffort,
  WebSessionCodexModelInfo,
  WebSessionReasoningEffort,
  WorktreeConfig,
} from '@/types/models';
import {
  CODEX_MODEL_OPTIONS,
  defaultModelForAgent,
  resolveCodexReasoningEfforts,
} from '@/components/web-session/webSessionModelOptions';
import { GENERIC_CODEX_REASONING_EFFORTS } from '@/constants/webSessionDefaults';
import {
  DEFAULT_ACTIVE_CALL_TIMEOUT_CALL_KINDS,
  DEFAULT_ACTIVE_CALL_TIMEOUT_CUSTOM_SECONDS,
  DEFAULT_ACTIVE_CALL_TIMEOUT_PROMPT,
  applyDeveloperConfig,
  sanitizeDeveloperConfig,
} from '@/utils/developerConfig';
import {
  getDailyTips,
  selectAnotherRandomDailyTipIndex,
  selectRandomDailyTipIndex,
  type DailyTipDefinition,
} from '@/utils/dailyTips';
import {
  buildImportableSettingsBackup,
  buildSettingsBackupFile,
  downloadSettingsBackupFile,
  exportServerSettingsBackup,
  formatSettingsBackupFileName,
  hasSettingsBackupContent,
  importSettingsBackup,
  parseSettingsBackupJSON,
  previewSettingsBackup,
  type SettingsBackupFile,
  type SettingsBackupExportOptions,
  type SettingsBackupImportMode,
  type SettingsBackupPreviewIssue,
  type SettingsBackupPreviewResult,
  type SettingsBackupPreviewSection,
  type SettingsBackupVersionWarningMode,
} from '@/utils/settingsBackup';

type ShortcutTarget = 'terminal' | 'notepad';

const SHELL_AUTO_VALUE = '__auto__';
const SHELL_CUSTOM_VALUE = '__custom__';
const DEFAULT_AUTH_PROXY_HEADER = 'X-Forwarded-For';
const DEFAULT_SETTINGS_BACKUP_EXPORT_OPTIONS: SettingsBackupExportOptions = {
  includeServer: true,
  includeClient: true,
  includeSecurityAccess: false,
  includeQuickInputRecent: false,
  fileNameRule: 'app-version-datetime',
  includeMetadata: true,
};
const SETTINGS_BACKUP_BREAKING_WARNING_CODES = new Set(['source_app_breaking_version_differs']);
const SETTINGS_BACKUP_VERSION_WARNING_CODES = new Set([
  'source_app_version_differs',
  'source_app_breaking_version_differs',
  'source_app_channel_differs',
]);

type ItemResponse<T> = {
  item?: T;
};

interface SettingsCardDefinition {
  id: SettingsSectionId;
  title: string;
  description: string;
  searchTerms: string[];
  dirty?: boolean;
  matchCount?: number;
}

const { t, locale } = useLocale();
const { isMobile } = useResponsive();

const route = useRoute();
const router = useRouter();
const message = useMessage();
const dialog = useDialog();
const authStore = useAuthStore();
const settingsStore = useSettingsStore();
const developerConfigStore = useDeveloperConfigStore();
const { loading: developerLoading, saving: developerSaving } = storeToRefs(developerConfigStore);
const {
  pageTitle,
  pageTitleSettingsLoaded,
  pageTitleSettingsSaving,
  theme,
  currentPresetId,
  followSystemThemeSetting,
  followSystemTheme,
  customTheme,
  recentProjectsLimit,
  maxTerminalsPerProject,
  dailyTipEnabled,
  dailyTipSettingsLoaded,
  dailyTipSettingsSaving,
  terminalShortcut,
  notepadShortcut,
  webSessionQuickInput,
  terminalQuickActions,
  editorSettings,
  confirmBeforeTerminalClose,
  showWebSessionReasoning,
  webSessionActivityDisplayMode,
  webSessionAutoContinueScope,
  webSessionAutoContinuePreset,
  webSessionAutoContinueMaxAttempts,
  webSessionAutoRetryDispatchPendingOnFailure,
  webSessionStreamingMarkdownThrottleMode,
  webSessionStreamingMarkdownThrottleCustomMs,
  terminalThemeId,
  terminalFont,
  terminalWebGLRenderer,
  defaultTerminalRenderMode,
  defaultTerminalSnapshotIntervalMs,
  defaultTerminalSnapshotZlibCompression,
  terminalConnectionPolicy,
  inactiveTerminalSnapshotIntervalMs,
} = storeToRefs(settingsStore);

const initialRouteSection = sanitizeSettingsSectionId(
  typeof route.query.section === 'string' ? route.query.section : undefined
);
const localSettingsSection = ref<SettingsSectionId>(initialRouteSection);
const localSettingsSearchQuery = ref(typeof route.query.q === 'string' ? route.query.q : '');
const highlightedSettingsSection = ref<SettingsSectionId | null>(null);
const settingsSectionRefs = new Map<SettingsSectionId, HTMLElement>();
let highlightResetTimer: ReturnType<typeof setTimeout> | null = null;

const capturingTarget = ref<ShortcutTarget | null>(null);
const themeWarningController = createThemeMaintenanceWarningController({
  t,
  warning: options => dialog.warning(options),
});
const themeSelectionController = createThemeSelectionController({
  getCurrentPresetId: () => currentPresetId.value,
  isFollowSystemTheme: () => followSystemTheme.value,
  selectPreset: presetId => settingsStore.selectPreset(presetId),
  toggleFollowSystemTheme: enabled => settingsStore.toggleFollowSystemTheme(enabled),
  confirmPresetThemeChange: themeWarningController.confirmPresetThemeChange,
  confirmFollowSystemEnable: themeWarningController.confirmFollowSystemEnable,
});
const settingsBackupExporting = ref(false);
const settingsBackupImporting = ref(false);
const settingsBackupFileInputRef = ref<HTMLInputElement | null>(null);
const settingsBackupSelectedFileName = ref('');
const pendingSettingsBackup = ref<SettingsBackupFile | null>(null);
const settingsBackupPreview = ref<SettingsBackupPreviewResult | null>(null);
const settingsBackupExportOptions = reactive<SettingsBackupExportOptions>({
  ...DEFAULT_SETTINGS_BACKUP_EXPORT_OPTIONS,
});
const settingsBackupImportMode = ref<SettingsBackupImportMode>('preview-confirm');
const settingsBackupVersionWarningMode = ref<SettingsBackupVersionWarningMode>('standard');
const settingsBackupAllowBreakingVersionContinue = ref(false);
const settingsBackupSelectedSectionKeys = ref<string[]>([]);
const showSettingsBackupPreviewDialog = ref(false);
const authSaving = ref(false);
const authAccessLoading = ref(false);
const authAccessSaving = ref(false);
const enablePassword = ref('');
const enablePasswordConfirm = ref('');
const currentPassword = ref('');
const newPassword = ref('');
const newPasswordConfirm = ref('');
const disablePassword = ref('');
const authAccessForm = reactive({
  bypassIPs: '',
  bypassDomains: '',
  forceAuthIPs: '',
  forceAuthDomains: '',
  trustedProxies: '',
});
const authAccessOriginal = ref<AuthAccessConfig | null>(null);
const showSecurityAdminLoginDialog = ref(false);
const securityAdminLoginPassword = ref('');
const securityAdminLoginLoading = ref(false);
const showDailyTipDialog = ref(false);
const activeDailyTipIndex = ref(0);
const settingsBackupPreviewWarnings = computed<SettingsBackupPreviewIssue[]>(
  () => settingsBackupPreview.value?.warnings ?? []
);
const settingsBackupPreviewErrors = computed<SettingsBackupPreviewIssue[]>(
  () => settingsBackupPreview.value?.errors ?? []
);
const settingsBackupPreviewSections = computed<SettingsBackupPreviewSection[]>(
  () => settingsBackupPreview.value?.sections ?? []
);
const settingsBackupSelectedSectionKeySet = computed(
  () => new Set(settingsBackupSelectedSectionKeys.value)
);
const settingsBackupCanExport = computed(
  () => settingsBackupExportOptions.includeServer || settingsBackupExportOptions.includeClient
);
const settingsBackupSelectedSectionCount = computed(
  () => settingsBackupSelectedSectionKeys.value.length
);
const settingsBackupFilteredImport = computed<SettingsBackupFile | null>(() => {
  if (!pendingSettingsBackup.value) {
    return null;
  }
  return buildImportableSettingsBackup(
    pendingSettingsBackup.value,
    settingsBackupSelectedSectionKeys.value
  );
});
const settingsBackupHasFilteredImportContent = computed(() =>
  hasSettingsBackupContent(settingsBackupFilteredImport.value)
);
const settingsBackupHasBreakingVersionMismatch = computed(() =>
  settingsBackupPreviewWarnings.value.some(issue =>
    SETTINGS_BACKUP_BREAKING_WARNING_CODES.has(issue.code)
  )
);
const settingsBackupHasAnyVersionMismatch = computed(() =>
  settingsBackupPreviewWarnings.value.some(issue =>
    SETTINGS_BACKUP_VERSION_WARNING_CODES.has(issue.code)
  )
);
const settingsBackupBlockedByVersionPolicy = computed(() => {
  if (
    settingsBackupHasBreakingVersionMismatch.value &&
    !settingsBackupAllowBreakingVersionContinue.value
  ) {
    return true;
  }
  return (
    settingsBackupVersionWarningMode.value === 'strict' && settingsBackupHasAnyVersionMismatch.value
  );
});
const settingsBackupCanConfirmImport = computed(
  () =>
    Boolean(settingsBackupPreview.value?.canImport) &&
    settingsBackupHasFilteredImportContent.value &&
    !settingsBackupBlockedByVersionPolicy.value
);
const settingsBackupImportSummary = computed(() =>
  t('settings.backupPreviewSummary', { count: settingsBackupSelectedSectionCount.value })
);
const settingsBackupFileNameRuleOptions = computed(() => [
  { label: t('settings.backupFileNameRuleVersionDate'), value: 'app-version-date' },
  { label: t('settings.backupFileNameRuleVersionDateTime'), value: 'app-version-datetime' },
  {
    label: t('settings.backupFileNameRuleChannelVersionDateTime'),
    value: 'channel-app-version-datetime',
  },
]);
const settingsBackupImportModeOptions = computed(() => [
  { label: t('settings.backupImportModePreviewOnly'), value: 'preview-only' },
  { label: t('settings.backupImportModePreviewConfirm'), value: 'preview-confirm' },
  { label: t('settings.backupImportModeDirect'), value: 'direct-import' },
]);
const settingsBackupVersionWarningModeOptions = computed(() => [
  { label: t('settings.backupVersionWarningModeStandard'), value: 'standard' },
  { label: t('settings.backupVersionWarningModeStrict'), value: 'strict' },
]);

watch(
  () => settingsBackupExportOptions.includeServer,
  includeServer => {
    if (!includeServer) {
      settingsBackupExportOptions.includeSecurityAccess = false;
    }
  }
);

watch(showSettingsBackupPreviewDialog, show => {
  if (!show && !settingsBackupImporting.value && pendingSettingsBackup.value) {
    resetSettingsBackupPreviewState();
  }
});

// 使用 composable 获取主题和终端配色选项
const presetOptions = useThemeOptions();
const terminalThemeOptions = useTerminalThemeOptions();
const dailyTips = computed(() => getDailyTips(locale.value));
const dailyTipCount = computed(() => dailyTips.value.length);
const activeDailyTip = computed<DailyTipDefinition | null>(() => {
  if (dailyTips.value.length === 0) {
    return null;
  }
  return dailyTips.value[activeDailyTipIndex.value] ?? dailyTips.value[0] ?? null;
});

// 当前预设 ID
const currentPresetValue = computed(() => currentPresetId.value);

// 跟随系统主题
const followSystemValue = computed(() => followSystemTheme.value);
const followSystemModeValue = computed(() => followSystemThemeSetting.value);

// 是否有自定义主题
const hasCustomTheme = computed(() => customTheme.value !== null);
const standardFormLabelPlacement = computed<'left' | 'top'>(() =>
  isMobile.value ? 'top' : 'left'
);
const standardFormLabelWidth = computed<number | string>(() => (isMobile.value ? 'auto' : 160));
const themeFormLabelWidth = computed<number | string>(() => (isMobile.value ? 'auto' : 140));
const actionFormItemLabel = computed(() => (isMobile.value ? undefined : '\u00a0'));
const securityManagementLocked = computed(() => !authStore.canManageSecurity);
const showSecurityAdminLoginPrompt = computed(() => authStore.enabled && !authStore.authenticated);

const activeSettingsSection = computed<SettingsSectionId>({
  get: () => localSettingsSection.value,
  set: value => {
    localSettingsSection.value = value;
  },
});

const settingsSearchQuery = computed({
  get: () => localSettingsSearchQuery.value,
  set: value => {
    localSettingsSearchQuery.value = value ?? '';
  },
});

function syncPageSettingsRouteState() {
  const routeSection = typeof route.query.section === 'string' ? route.query.section : '';
  const routeQuery = typeof route.query.q === 'string' ? route.query.q : '';
  const nextSection = activeSettingsSection.value;
  const nextQuery = settingsSearchQuery.value.trim();

  if (routeSection === nextSection && routeQuery === nextQuery) {
    return;
  }

  const nextRouteQuery = {
    ...route.query,
    section: nextSection,
    q: nextQuery || undefined,
  };

  void router.replace({ query: nextRouteQuery });
}

watch(
  () => route.query.section,
  value => {
    localSettingsSection.value = sanitizeSettingsSectionId(
      typeof value === 'string' ? value : undefined
    );
  }
);

watch(
  () => route.query.q,
  value => {
    localSettingsSearchQuery.value = typeof value === 'string' ? value : '';
  }
);

watch([activeSettingsSection, settingsSearchQuery], () => {
  syncPageSettingsRouteState();
});

function registerSettingsSectionRef(section: SettingsSectionId, element: HTMLElement | null) {
  if (element) {
    element.dataset.sectionId = section;
    settingsSectionRefs.set(section, element);
    return;
  }
  settingsSectionRefs.delete(section);
}

function clearSettingsSectionHighlight() {
  if (highlightResetTimer) {
    clearTimeout(highlightResetTimer);
    highlightResetTimer = null;
  }
  highlightedSettingsSection.value = null;
}

async function scrollToSettingsSection(section: SettingsSectionId, highlight = false) {
  await nextTick();
  const element = settingsSectionRefs.get(section);
  if (element) {
    element.scrollIntoView({
      block: 'start',
      behavior: isMobile.value ? 'auto' : 'smooth',
    });
  }
  if (!highlight) {
    return;
  }
  highlightedSettingsSection.value = section;
  if (highlightResetTimer) {
    clearTimeout(highlightResetTimer);
  }
  highlightResetTimer = setTimeout(() => {
    highlightedSettingsSection.value = null;
    highlightResetTimer = null;
  }, 2400);
}

async function handlePresetThemeChange(value: string) {
  await themeSelectionController.selectPresetWithConfirmation(value);
}

async function handleFollowSystemModeChange(value: FollowSystemThemeSetting) {
  if (value === 1) {
    await themeSelectionController.toggleFollowSystemThemeWithConfirmation(true);
    return;
  }
  settingsStore.setFollowSystemThemeSetting(value);
}

const developerForm = reactive<DeveloperConfig>(sanitizeDeveloperConfig());
const developerOriginal = ref<DeveloperConfig | null>(null);
const codexModelCatalog = ref<WebSessionCodexModelInfo[]>([]);
const developerUsesCustomActiveCallTimeout = computed(
  () => developerForm.webSessionActiveCallTimeout.timeoutMode === 'custom'
);
const webSessionSyncModeOptions = computed(() => [
  { label: t('settings.webSessionCodexDefaultSyncModeOption'), value: 'default' },
  { label: t('settings.webSessionSyncModeFast'), value: 'fast' },
  { label: t('settings.webSessionSyncModeDeep'), value: 'deep' },
]);
const webSessionCodexDefaultModelOptions = computed(() => [
  { label: t('settings.webSessionCodexDefaultModelOption'), value: 'default' },
  ...CODEX_MODEL_OPTIONS.map(option => ({
    label: option.menuLabel || option.label,
    value: option.value,
  })),
]);

function reasoningEffortLabel(effort: WebSessionCodexDefaultReasoningEffort) {
  if (effort === 'default') {
    return t('settings.webSessionCodexDefaultReasoningEffortOption');
  }
  if (effort === 'model_default') {
    return t('settings.webSessionCodexModelDefaultReasoningEffort');
  }
  const labels: Record<Exclude<WebSessionReasoningEffort, 'default'>, string> = {
    none: 'Off',
    minimal: 'Minimal',
    low: 'Low',
    medium: 'Mid',
    high: 'High',
    xhigh: 'Xhigh',
    max: 'Max',
    ultra: 'Ultra',
  };
  return labels[effort];
}

function supportedDefaultReasoningEfforts(model: string): WebSessionCodexDefaultReasoningEffort[] {
  const effectiveModel = defaultModelForAgent('codex', model);
  const supported = resolveCodexReasoningEfforts(effectiveModel, codexModelCatalog.value);
  const explicitEfforts =
    supported ?? GENERIC_CODEX_REASONING_EFFORTS.filter(value => value !== 'default');
  return ['default', 'model_default', ...explicitEfforts];
}

const webSessionCodexDefaultReasoningEffortOptions = computed(() =>
  supportedDefaultReasoningEfforts(developerForm.webSessionCodexDefaultModel).map(value => ({
    label: reasoningEffortLabel(value),
    value,
  }))
);
const webSessionCodexDefaultPermissionLevelOptions = computed(() => [
  { label: t('settings.webSessionCodexDefaultPermissionOption'), value: 'default' },
  { label: t('settings.webSessionCodexStandardPermission'), value: 'standard' },
  { label: t('webSession.permissionElevated'), value: 'elevated' },
  { label: t('webSession.permissionYolo'), value: 'yolo' },
]);
const developerBehaviorDirty = computed(() => {
  if (!developerOriginal.value) {
    return false;
  }
  return (
    developerForm.enableTerminalScrollback !== developerOriginal.value.enableTerminalScrollback
  );
});
const developerSessionDirty = computed(() => {
  if (!developerOriginal.value) {
    return false;
  }
  return (
    developerForm.webSessionCodexDefaultModel !==
      developerOriginal.value.webSessionCodexDefaultModel ||
    developerForm.webSessionCodexDefaultReasoningEffort !==
      developerOriginal.value.webSessionCodexDefaultReasoningEffort ||
    developerForm.webSessionCodexDefaultPermissionLevel !==
      developerOriginal.value.webSessionCodexDefaultPermissionLevel ||
    developerForm.webSessionCodexDefaultSyncMode !==
      developerOriginal.value.webSessionCodexDefaultSyncMode ||
    JSON.stringify(developerForm.webSessionActiveCallTimeout) !==
      JSON.stringify(developerOriginal.value.webSessionActiveCallTimeout)
  );
});
const developerTerminalDirty = computed(() => {
  if (!developerOriginal.value) {
    return false;
  }
  return (
    developerForm.enableTerminalStateSnapshot !==
    developerOriginal.value.enableTerminalStateSnapshot
  );
});

watch(
  () => developerConfigStore.config,
  config => {
    if (
      developerBehaviorDirty.value ||
      developerSessionDirty.value ||
      developerTerminalDirty.value
    ) {
      return;
    }
    const next = sanitizeDeveloperConfig(config);
    applyDeveloperConfig(developerForm, next);
    developerOriginal.value = sanitizeDeveloperConfig(next);
  },
  { deep: true }
);

async function loadDeveloperConfig(force = true) {
  try {
    const next = await developerConfigStore.load(force);
    applyDeveloperConfig(developerForm, next);
    developerOriginal.value = sanitizeDeveloperConfig(next);
  } catch (error) {
    console.error('Failed to load developer config:', error);
    developerOriginal.value = sanitizeDeveloperConfig(developerForm);
  }
}

async function handleSaveDeveloperConfig() {
  try {
    const payload = sanitizeDeveloperConfig(developerForm);
    const saved = await developerConfigStore.update(payload);
    applyDeveloperConfig(developerForm, saved);
    developerOriginal.value = sanitizeDeveloperConfig(saved);
    message.success(t('common.saveSuccess'));
  } catch (error) {
    console.error('Failed to save developer config:', error);
    message.error(t('common.saveFailed'));
  }
}

async function loadCodexModelCatalog() {
  try {
    const config = await webSessionApi.runtimeConfig();
    codexModelCatalog.value = config.models ?? [];
  } catch {
    codexModelCatalog.value = [];
  }
}

watch(
  [() => developerForm.webSessionCodexDefaultModel, () => codexModelCatalog.value],
  ([model]) => {
    const supported = supportedDefaultReasoningEfforts(model);
    if (!supported.includes(developerForm.webSessionCodexDefaultReasoningEffort)) {
      developerForm.webSessionCodexDefaultReasoningEffort = 'default';
    }
  },
  { deep: true }
);

watch(
  () => developerForm.webSessionActiveCallTimeout.callKinds.useDefault,
  useDefault => {
    if (!useDefault) {
      return;
    }
    developerForm.webSessionActiveCallTimeout.callKinds.mcp =
      DEFAULT_ACTIVE_CALL_TIMEOUT_CALL_KINDS.mcp;
    developerForm.webSessionActiveCallTimeout.callKinds.command =
      DEFAULT_ACTIVE_CALL_TIMEOUT_CALL_KINDS.command;
    developerForm.webSessionActiveCallTimeout.callKinds.tool =
      DEFAULT_ACTIVE_CALL_TIMEOUT_CALL_KINDS.tool;
  }
);

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
    worktreeSettingsForm.globalDirNamePattern !==
      worktreeSettingsOriginal.value.globalDirNamePattern
  );
});

const { send: fetchWorktreeSettings, loading: worktreeSettingsLoading } = useReq(() =>
  http.Get<ItemResponse<WorktreeConfig>>('/system/worktree-settings')
);

const { send: updateWorktreeSettings, loading: worktreeSettingsSaving } = useReq(
  (config: WorktreeConfig) =>
    http.Post<ItemResponse<WorktreeConfig>>('/system/worktree-settings/update', config)
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

const { send: fetchShells, loading: shellsLoading } = useReq(() =>
  http.Get<ItemResponse<AvailableShellsResponse>>('/system/terminal-shells')
);

const { send: updateShell } = useReq((shell: string) =>
  http.Post('/system/terminal-shells/update', { shell })
);

const { send: validateShell } = useReq((shell: string) =>
  http.Post<{ valid: boolean; message?: string }>('/system/terminal-shells/validate', { shell })
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

function normalizeWebSessionQuickInputPinnedItems(items: string[]) {
  const normalized: string[] = [];
  const seen = new Set<string>();

  for (const item of items) {
    const trimmed = item.trim();
    if (!trimmed || seen.has(trimmed)) {
      continue;
    }
    normalized.push(trimmed);
    seen.add(trimmed);
  }

  return normalized;
}

function stringArraysEqual(left: string[], right: string[]) {
  if (left.length !== right.length) {
    return false;
  }
  return left.every((item, index) => item === right[index]);
}

function normalizeMultilineEntries(value: string) {
  const seen = new Set<string>();
  const entries: string[] = [];
  for (const raw of value.split(/\r?\n/)) {
    const trimmed = raw.trim();
    if (!trimmed || seen.has(trimmed)) {
      continue;
    }
    seen.add(trimmed);
    entries.push(trimmed);
  }
  return entries;
}

function joinAuthAccessEntries(items?: string[]) {
  return (items ?? []).join('\n');
}

function buildAuthAccessConfigFromForm(): AuthAccessConfig {
  return {
    accessRules: {
      bypassIPs: normalizeMultilineEntries(authAccessForm.bypassIPs),
      bypassDomains: normalizeMultilineEntries(authAccessForm.bypassDomains),
      forceAuthIPs: normalizeMultilineEntries(authAccessForm.forceAuthIPs),
      forceAuthDomains: normalizeMultilineEntries(authAccessForm.forceAuthDomains),
    },
    proxyHeader: DEFAULT_AUTH_PROXY_HEADER,
    trustedProxies: normalizeMultilineEntries(authAccessForm.trustedProxies),
  };
}

function applyAuthAccessConfig(config: AuthAccessConfig) {
  authAccessForm.bypassIPs = joinAuthAccessEntries(config.accessRules.bypassIPs);
  authAccessForm.bypassDomains = joinAuthAccessEntries(config.accessRules.bypassDomains);
  authAccessForm.forceAuthIPs = joinAuthAccessEntries(config.accessRules.forceAuthIPs);
  authAccessForm.forceAuthDomains = joinAuthAccessEntries(config.accessRules.forceAuthDomains);
  authAccessForm.trustedProxies = joinAuthAccessEntries(config.trustedProxies);
}

function authAccessConfigsEqual(left: AuthAccessConfig, right: AuthAccessConfig) {
  return (
    left.proxyHeader === right.proxyHeader &&
    stringArraysEqual(left.trustedProxies, right.trustedProxies) &&
    stringArraysEqual(left.accessRules.bypassIPs, right.accessRules.bypassIPs) &&
    stringArraysEqual(left.accessRules.bypassDomains, right.accessRules.bypassDomains) &&
    stringArraysEqual(left.accessRules.forceAuthIPs, right.accessRules.forceAuthIPs) &&
    stringArraysEqual(left.accessRules.forceAuthDomains, right.accessRules.forceAuthDomains)
  );
}

const authAccessDirty = computed(() => {
  if (!authAccessOriginal.value) {
    return false;
  }
  return !authAccessConfigsEqual(buildAuthAccessConfigFromForm(), authAccessOriginal.value);
});

useInit(() => {
  loadDeveloperConfig();
  void loadCodexModelCatalog();
  loadWorktreeSettings();
  loadShellsConfig();
  loadAuthAccessConfig();
  void settingsStore.loadPageTitleSettings();
  void settingsStore.loadDailyTipSettings();
  void settingsStore.loadWebSessionQuickInput();
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
    case 'windows':
      return 'Windows';
    case 'darwin':
      return 'macOS';
    case 'linux':
      return 'Linux';
    default:
      return platform || '';
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

async function loadAuthAccessConfig() {
  authAccessLoading.value = true;
  try {
    const config = await authStore.fetchAccessConfig();
    authAccessOriginal.value = config;
    applyAuthAccessConfig(config);
  } catch (error) {
    console.error('Failed to load auth access config:', error);
    message.error(error instanceof Error ? error.message : t('common.loadFailed'));
  } finally {
    authAccessLoading.value = false;
  }
}

function handleResetAuthAccessConfig() {
  if (!authAccessOriginal.value) {
    return;
  }
  applyAuthAccessConfig(authAccessOriginal.value);
}

function openSecurityAdminLoginDialog() {
  securityAdminLoginPassword.value = '';
  showSecurityAdminLoginDialog.value = true;
}

function handleCloseSecurityAdminLoginDialog() {
  if (securityAdminLoginLoading.value) {
    return;
  }
  securityAdminLoginPassword.value = '';
  showSecurityAdminLoginDialog.value = false;
}

function ensureSecurityManagementAccess() {
  if (authStore.canManageSecurity) {
    return true;
  }
  message.warning(t('settings.securityAdminLoginHint'));
  openSecurityAdminLoginDialog();
  return false;
}

function resetAuthFormFields() {
  enablePassword.value = '';
  enablePasswordConfirm.value = '';
  currentPassword.value = '';
  newPassword.value = '';
  newPasswordConfirm.value = '';
  disablePassword.value = '';
}

async function handleSecurityAdminLogin() {
  if (!securityAdminLoginPassword.value.trim()) {
    message.error(t('auth.passwordRequired'));
    return;
  }

  securityAdminLoginLoading.value = true;
  try {
    await authStore.loginWithPassword(securityAdminLoginPassword.value);
    securityAdminLoginPassword.value = '';
    showSecurityAdminLoginDialog.value = false;
    message.success(t('settings.securityAdminLoginSuccess'));
  } catch (error) {
    console.error('Failed to authenticate administrator session:', error);
    message.error(error instanceof Error ? error.message : t('auth.loginFailed'));
  } finally {
    securityAdminLoginLoading.value = false;
  }
}

async function handleEnablePasswordProtection() {
  if (!enablePassword.value.trim() || !enablePasswordConfirm.value.trim()) {
    message.error(t('auth.passwordRequired'));
    return;
  }
  if (enablePassword.value !== enablePasswordConfirm.value) {
    message.error(t('auth.passwordMismatch'));
    return;
  }

  authSaving.value = true;
  try {
    await authStore.enablePasswordProtection(enablePassword.value);
    resetAuthFormFields();
    message.success(t('settings.securityEnableSuccess'));
  } catch (error) {
    console.error('Failed to enable password protection:', error);
    message.error(error instanceof Error ? error.message : t('common.saveFailed'));
  } finally {
    authSaving.value = false;
  }
}

async function handleChangePasswordProtection() {
  if (!ensureSecurityManagementAccess()) {
    return;
  }
  if (
    !currentPassword.value.trim() ||
    !newPassword.value.trim() ||
    !newPasswordConfirm.value.trim()
  ) {
    message.error(t('auth.passwordRequired'));
    return;
  }
  if (newPassword.value !== newPasswordConfirm.value) {
    message.error(t('auth.passwordMismatch'));
    return;
  }

  authSaving.value = true;
  try {
    await authStore.changePasswordProtection(currentPassword.value, newPassword.value);
    resetAuthFormFields();
    message.success(t('settings.securityChangeSuccess'));
  } catch (error) {
    console.error('Failed to change password protection:', error);
    message.error(error instanceof Error ? error.message : t('common.saveFailed'));
  } finally {
    authSaving.value = false;
  }
}

async function handleDisablePasswordProtection() {
  if (!ensureSecurityManagementAccess()) {
    return;
  }
  if (!disablePassword.value.trim()) {
    message.error(t('auth.passwordRequired'));
    return;
  }

  authSaving.value = true;
  try {
    await authStore.disablePasswordProtection(disablePassword.value);
    resetAuthFormFields();
    message.success(t('settings.securityDisableSuccess'));
  } catch (error) {
    console.error('Failed to disable password protection:', error);
    message.error(error instanceof Error ? error.message : t('common.saveFailed'));
  } finally {
    authSaving.value = false;
  }
}

async function handleSaveAuthAccessConfig() {
  if (!ensureSecurityManagementAccess()) {
    return;
  }
  authAccessSaving.value = true;
  try {
    const saved = await authStore.updateAccessConfig(buildAuthAccessConfigFromForm());
    authAccessOriginal.value = saved;
    applyAuthAccessConfig(saved);
    await authStore.refreshStatus();
    if (authStore.enabled && !authStore.canAccessProtectedContent) {
      await router.replace({
        name: 'login',
        query: {
          redirect: route.fullPath || '/settings?section=security',
        },
      });
      return;
    }
    message.success(t('settings.securityAccessRulesSaveSuccess'));
  } catch (error) {
    console.error('Failed to save auth access config:', error);
    message.error(error instanceof Error ? error.message : t('common.saveFailed'));
  } finally {
    authAccessSaving.value = false;
  }
}

function resetSettingsBackupFileInput() {
  if (settingsBackupFileInputRef.value) {
    settingsBackupFileInputRef.value.value = '';
  }
}

function handleChooseSettingsBackupFile() {
  resetSettingsBackupFileInput();
  settingsBackupFileInputRef.value?.click();
}

function resetSettingsBackupPreviewState(options?: { keepSelectedFileName?: boolean }) {
  if (!options?.keepSelectedFileName) {
    settingsBackupSelectedFileName.value = '';
  }
  pendingSettingsBackup.value = null;
  settingsBackupPreview.value = null;
  settingsBackupSelectedSectionKeys.value = [];
}

function closeSettingsBackupPreviewDialog() {
  if (settingsBackupImporting.value) {
    return;
  }
  showSettingsBackupPreviewDialog.value = false;
  resetSettingsBackupPreviewState();
}

function handleToggleSettingsBackupSection(key: string, checked: boolean) {
  const selected = new Set(settingsBackupSelectedSectionKeys.value);
  if (checked) {
    selected.add(key);
  } else {
    selected.delete(key);
  }
  settingsBackupSelectedSectionKeys.value = Array.from(selected);
}

function canAutoImportSettingsBackup(preview: SettingsBackupPreviewResult) {
  if (settingsBackupImportMode.value !== 'direct-import' || !preview.canImport) {
    return false;
  }
  const warnings = preview.warnings ?? [];
  if (
    warnings.some(issue => SETTINGS_BACKUP_BREAKING_WARNING_CODES.has(issue.code)) &&
    !settingsBackupAllowBreakingVersionContinue.value
  ) {
    return false;
  }
  if (
    settingsBackupVersionWarningMode.value === 'strict' &&
    warnings.some(issue => SETTINGS_BACKUP_VERSION_WARNING_CODES.has(issue.code))
  ) {
    return false;
  }
  return warnings.length === 0;
}

async function refreshSettingsAfterBackupImport(importedBackup: SettingsBackupFile) {
  const tasks: Array<Promise<unknown>> = [];
  const server = importedBackup.payload.server;

  if (server?.developer) {
    tasks.push(loadDeveloperConfig());
  }
  if (server?.worktree) {
    tasks.push(loadWorktreeSettings());
  }
  if (server?.terminalShell) {
    tasks.push(loadShellsConfig());
  }
  if (server?.authAccess) {
    tasks.push(loadAuthAccessConfig());
  }
  if (server?.dailyTip) {
    tasks.push(settingsStore.loadDailyTipSettings(true));
  }
  if (typeof server?.pageTitle === 'string') {
    tasks.push(settingsStore.loadPageTitleSettings(true));
  }
  if (server?.webSessionQuickInput?.pinned || server?.webSessionQuickInput?.recent) {
    tasks.push(settingsStore.loadWebSessionQuickInput(true));
  }

  if (tasks.length > 0) {
    await Promise.all(tasks);
  }
}

async function executeSettingsBackupImport(backup: SettingsBackupFile) {
  await importSettingsBackup(backup);

  const clientPayload = backup.payload.client;
  if (clientPayload) {
    settingsStore.importClientBackup(clientPayload);
  }

  await refreshSettingsAfterBackupImport(backup);
}

async function handleExportSettingsBackup() {
  if (!settingsBackupCanExport.value) {
    message.error(t('settings.backupNothingSelected'));
    return;
  }
  settingsBackupExporting.value = true;
  try {
    const serverBackup = await exportServerSettingsBackup();
    const backup = buildSettingsBackupFile({
      serverBackup,
      clientPayload: settingsStore.exportClientBackup(locale.value, {
        includeQuickInputRecent: settingsBackupExportOptions.includeQuickInputRecent,
      }),
      exportOptions: settingsBackupExportOptions,
    });
    if (!hasSettingsBackupContent(backup)) {
      message.error(t('settings.backupNothingSelected'));
      return;
    }
    downloadSettingsBackupFile(
      backup,
      formatSettingsBackupFileName({
        appInfo: serverBackup.sourceApp,
        rule: settingsBackupExportOptions.fileNameRule,
        createdAt: backup.createdAt,
      })
    );
    message.success(t('settings.backupExportSuccess'));
  } catch (error) {
    console.error('Failed to export settings backup:', error);
    message.error(error instanceof Error ? error.message : t('settings.backupExportFailed'));
  } finally {
    settingsBackupExporting.value = false;
  }
}

async function handleSettingsBackupFileChange(event: Event) {
  const input = event.target as HTMLInputElement | null;
  const file = input?.files?.[0];
  if (!file) {
    return;
  }

  settingsBackupImporting.value = true;
  settingsBackupSelectedFileName.value = file.name;
  try {
    const text = await file.text();
    const backup = parseSettingsBackupJSON(text);
    const preview = await previewSettingsBackup(backup);
    pendingSettingsBackup.value = backup;
    settingsBackupPreview.value = preview;
    settingsBackupSelectedSectionKeys.value = (preview.sections ?? []).map(section => section.key);

    if (canAutoImportSettingsBackup(preview)) {
      const importableBackup = buildImportableSettingsBackup(
        backup,
        settingsBackupSelectedSectionKeys.value
      );
      if (!hasSettingsBackupContent(importableBackup)) {
        message.error(t('settings.backupNothingSelected'));
        showSettingsBackupPreviewDialog.value = true;
        return;
      }
      await executeSettingsBackupImport(importableBackup);
      resetSettingsBackupPreviewState();
      message.success(t('settings.backupImportSuccess'));
      return;
    }

    showSettingsBackupPreviewDialog.value = true;
  } catch (error) {
    console.error('Failed to parse or preview settings backup:', error);
    resetSettingsBackupPreviewState();
    message.error(error instanceof Error ? error.message : t('settings.backupImportFailed'));
  } finally {
    settingsBackupImporting.value = false;
    resetSettingsBackupFileInput();
  }
}

async function handleConfirmSettingsBackupImport() {
  if (!settingsBackupFilteredImport.value || !settingsBackupCanConfirmImport.value) {
    return;
  }

  settingsBackupImporting.value = true;
  try {
    await executeSettingsBackupImport(settingsBackupFilteredImport.value);
    showSettingsBackupPreviewDialog.value = false;
    resetSettingsBackupPreviewState();
    message.success(t('settings.backupImportSuccess'));
  } catch (error) {
    console.error('Failed to import settings backup:', error);
    message.error(error instanceof Error ? error.message : t('settings.backupImportFailed'));
  } finally {
    settingsBackupImporting.value = false;
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

const previewPanelStyle = computed(() => {
  const primaryHex = ensureHexWithHash(primaryColor.value || '#3B69A9', '#3B69A9');
  const surfaceHex = ensureHexWithHash(surfaceColor.value || '#ffffff', '#ffffff');
  const surfaceIsDark = isDarkHex(surfaceHex);
  const contentBg = surfaceIsDark ? lightenColor(surfaceHex, 0.08) : darkenColor(surfaceHex, 0.04);
  return {
    '--preview-panel-bg': surfaceHex,
    '--preview-banner-bg': primaryHex,
    '--preview-banner-text': getReadableTextColor(primaryHex),
    '--preview-content-bg': contentBg,
    '--preview-content-text': fallbackTextColor.value,
  };
});

const editorOptions = EDITOR_OPTIONS;
const customCommandPathToken = '${path}';
const customCommandTip = computed(() =>
  t('settings.customCommandTip', { path: customCommandPathToken })
);
const customCommandPlaceholder = computed(() =>
  t('settings.customCommandPlaceholder', { path: customCommandPathToken })
);

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
  set: value => {
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
  set: value => {
    terminalLimitLocal.value = value ?? 12;
    debouncedUpdateTerminalLimit(value ?? 12);
  },
});

const dailyTipEnabledValue = computed(() => dailyTipEnabled.value);
const pageTitleInput = ref(pageTitle.value);
const pageTitleOriginal = ref(pageTitle.value);
const pageTitleError = computed(() => {
  const value = pageTitleInput.value.trim();
  if (/[\u0000-\u001f\u007f-\u009f]/u.test(value)) {
    return t('settings.pageTitleControlCharacterError');
  }
  if (Array.from(value).length > 64) {
    return t('settings.pageTitleTooLongError', { max: 64 });
  }
  return '';
});
const pageTitleDirty = computed(() => pageTitleInput.value !== pageTitleOriginal.value);

watch(pageTitle, next => {
  if (!pageTitleDirty.value) {
    pageTitleInput.value = next;
  }
  pageTitleOriginal.value = next;
});

async function handleSavePageTitle() {
  if (!pageTitleDirty.value || pageTitleError.value) {
    return;
  }
  try {
    const savedTitle = await settingsStore.updatePageTitle(pageTitleInput.value);
    pageTitleInput.value = savedTitle;
    pageTitleOriginal.value = savedTitle;
    message.success(t('common.saveSuccess'));
  } catch (error) {
    console.error('Failed to save page title settings:', error);
    message.error(t('common.saveFailed'));
  }
}

async function handleDailyTipEnabledChange(value: boolean) {
  try {
    await settingsStore.updateDailyTipEnabled(value);
  } catch (error) {
    console.error('Failed to save daily tip settings:', error);
    message.error(t('common.saveFailed'));
  }
}

function handleShowRandomDailyTip() {
  if (dailyTips.value.length === 0) {
    return;
  }
  activeDailyTipIndex.value = selectRandomDailyTipIndex(Math.random(), dailyTips.value.length);
  showDailyTipDialog.value = true;
}

function handleDailyTipClose() {
  showDailyTipDialog.value = false;
}

function handleShowAnotherDailyTip() {
  activeDailyTipIndex.value = selectAnotherRandomDailyTipIndex(
    activeDailyTipIndex.value,
    Math.random(),
    dailyTips.value.length
  );
}

function handleDailyTipDisable() {
  dialog.warning({
    title: t('dailyTip.disableConfirmTitle'),
    content: t('dailyTip.disableConfirmContent'),
    positiveText: t('dailyTip.disableForever'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await settingsStore.updateDailyTipEnabled(false);
      } catch (error) {
        console.error('Failed to save daily tip settings:', error);
        message.error(t('common.saveFailed'));
        return false;
      }
      showDailyTipDialog.value = false;
      return true;
    },
  });
}

const confirmTerminalCloseValue = computed({
  get: () => confirmBeforeTerminalClose.value,
  set: value => settingsStore.updateConfirmBeforeTerminalClose(value),
});

const snapshotIntervalOptions = computed(() => [
  {
    label: `${t('common.default')} (${formatTerminalSnapshotInterval(DEFAULT_TERMINAL_SNAPSHOT_INTERVAL_MS)})`,
    value: null,
  },
  ...TERMINAL_SNAPSHOT_INTERVAL_OPTIONS.map(interval => ({
    label: formatTerminalSnapshotInterval(interval),
    value: interval,
  })),
]);

const inactiveSnapshotIntervalOptions = computed(() =>
  TERMINAL_SNAPSHOT_INTERVAL_OPTIONS.map(interval => ({
    label: formatTerminalSnapshotInterval(interval),
    value: interval,
  }))
);

const defaultTerminalRenderModeValue = computed({
  get: () => defaultTerminalRenderMode.value,
  set: (value: TerminalRenderMode) => settingsStore.updateDefaultTerminalRenderMode(value),
});

const defaultTerminalSnapshotIntervalValue = computed({
  get: () => defaultTerminalSnapshotIntervalMs.value,
  set: (value: number | null) => settingsStore.updateDefaultTerminalSnapshotIntervalMs(value),
});

const defaultTerminalSnapshotZlibCompressionValue = computed({
  get: () => defaultTerminalSnapshotZlibCompression.value,
  set: (value: boolean) => settingsStore.updateDefaultTerminalSnapshotZlibCompression(value),
});

const showWebSessionReasoningValue = computed({
  get: () => showWebSessionReasoning.value,
  set: value => settingsStore.updateShowWebSessionReasoning(value),
});

const webSessionActivityDisplayModeValue = computed({
  get: () => webSessionActivityDisplayMode.value,
  set: (value: WebSessionActivityDisplayMode) =>
    settingsStore.updateWebSessionActivityDisplayMode(value),
});

const webSessionStreamingMarkdownThrottleModeValue = computed({
  get: () => webSessionStreamingMarkdownThrottleMode.value,
  set: (value: WebSessionStreamingMarkdownThrottleMode) =>
    settingsStore.updateWebSessionStreamingMarkdownThrottleMode(value),
});

const webSessionStreamingMarkdownThrottleCustomMsValue = computed({
  get: () => webSessionStreamingMarkdownThrottleCustomMs.value,
  set: (value: number | null) =>
    settingsStore.updateWebSessionStreamingMarkdownThrottleCustomMs(
      value ?? DEFAULT_WEB_SESSION_STREAMING_MARKDOWN_THROTTLE_MS
    ),
});

const webSessionAutoContinueScopeOptions = computed(() => [
  {
    label: t('settings.webSessionAutoContinueScopeNetworkOnly'),
    value: 'network_only',
  },
  {
    label: t('settings.webSessionAutoContinueScopeNetworkAndRateLimit'),
    value: 'network_and_rate_limit',
  },
  {
    label: t('settings.webSessionAutoContinueScopeAllFailures'),
    value: 'all_failures',
  },
]);

const webSessionAutoContinueScopeValue = computed({
  get: () => webSessionAutoContinueScope.value,
  set: (value: WebSessionAutoContinueScope) =>
    settingsStore.updateWebSessionAutoContinueScope(value),
});

const webSessionAutoContinuePresetOptions = computed(() => [
  {
    label: t('settings.webSessionAutoContinuePresetGentleStop'),
    value: 'gentle_stop',
  },
  {
    label: t('settings.webSessionAutoContinuePresetAggressiveStop'),
    value: 'aggressive_stop',
  },
  {
    label: t('settings.webSessionAutoContinuePresetSustain60s'),
    value: 'sustain_60s',
  },
]);

const webSessionAutoContinuePresetValue = computed({
  get: () => webSessionAutoContinuePreset.value,
  set: (value: WebSessionAutoContinuePreset) =>
    settingsStore.updateWebSessionAutoContinuePreset(value),
});

const webSessionAutoContinueMaxAttemptsValue = computed({
  get: () => webSessionAutoContinueMaxAttempts.value,
  set: (value: number | null) => settingsStore.updateWebSessionAutoContinueMaxAttempts(value),
});

const webSessionAutoRetryDispatchPendingOnFailureValue = computed({
  get: () => webSessionAutoRetryDispatchPendingOnFailure.value,
  set: (value: boolean) => settingsStore.updateWebSessionAutoRetryDispatchPendingOnFailure(value),
});

const terminalConnectionPolicyValue = computed({
  get: () => terminalConnectionPolicy.value,
  set: (value: TerminalConnectionPolicy) => settingsStore.updateTerminalConnectionPolicy(value),
});

const inactiveTerminalSnapshotIntervalValue = computed({
  get: () =>
    inactiveTerminalSnapshotIntervalMs.value ?? DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS,
  set: (value: number | null) =>
    settingsStore.updateInactiveTerminalSnapshotIntervalMs(
      value ?? DEFAULT_INACTIVE_TERMINAL_SNAPSHOT_INTERVAL_MS
    ),
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
  const agentOptions: {
    label: string;
    value: TerminalQuickActionIcon;
    svg?: string;
    icon?: any;
  }[] = [
    {
      label: t('settings.terminalQuickActionIconClaude'),
      value: 'claude',
      svg: normalizeSvgSize(getAssistantIconByType('claude-code'), 16),
    },
    {
      label: t('settings.terminalQuickActionIconCodex'),
      value: 'codex',
      svg: normalizeSvgSize(getAssistantIconByType('codex'), 16),
    },
    {
      label: t('settings.terminalQuickActionIconQwen'),
      value: 'qwen',
      svg: normalizeSvgSize(getAssistantIconByType('qwen-code'), 16),
    },
    {
      label: t('settings.terminalQuickActionIconGemini'),
      value: 'gemini',
      svg: normalizeSvgSize(getAssistantIconByType('gemini'), 16),
    },
    { label: t('settings.terminalQuickActionIconCursor'), value: 'cursor', icon: NavigateOutline },
    { label: t('settings.terminalQuickActionIconCopilot'), value: 'copilot', icon: LogoGithub },
  ];

  const genericOptions: { label: string; value: TerminalQuickActionIcon; icon: any }[] = [
    {
      label: t('settings.terminalQuickActionIconTerminal'),
      value: 'terminal',
      icon: TerminalOutline,
    },
    { label: t('settings.terminalQuickActionIconChat'), value: 'chat', icon: ChatbubblesOutline },
    { label: t('settings.terminalQuickActionIconCode'), value: 'code', icon: CodeOutline },
    { label: t('settings.terminalQuickActionIconRocket'), value: 'rocket', icon: RocketOutline },
    { label: t('settings.terminalQuickActionIconPlay'), value: 'play', icon: PlayOutline },
  ];

  const options = [...agentOptions, ...genericOptions];
  return options;
});

const terminalQuickActionsLocal = ref<TerminalQuickAction[]>(
  terminalQuickActions.value.map(item => ({ ...item }))
);
const webSessionQuickInputPinnedLocal = ref<string[]>([...webSessionQuickInput.value.pinned]);
const webSessionQuickInputPinnedOriginal = ref<string[]>([...webSessionQuickInput.value.pinned]);
const webSessionQuickInputPinnedSaving = ref(false);
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
  { deep: true }
);

const webSessionQuickInputPinnedDirty = computed(
  () =>
    !stringArraysEqual(
      normalizeWebSessionQuickInputPinnedItems(webSessionQuickInputPinnedLocal.value),
      normalizeWebSessionQuickInputPinnedItems(webSessionQuickInputPinnedOriginal.value)
    )
);

watch(
  () => webSessionQuickInput.value.pinned,
  next => {
    if (webSessionQuickInputPinnedDirty.value || webSessionQuickInputPinnedSaving.value) {
      return;
    }
    webSessionQuickInputPinnedOriginal.value = [...next];
    webSessionQuickInputPinnedLocal.value = [...next];
  },
  { deep: true }
);

watch(
  terminalQuickActionsLocal,
  next => {
    if (syncingTerminalQuickActions) {
      return;
    }
    debouncedUpdateTerminalQuickActions(next.map(item => ({ ...item })));
  },
  { deep: true }
);

function createWebSessionQuickInputPinnedItem() {
  return '';
}

function handleWebSessionQuickInputPinnedChange(index: number, value: string) {
  webSessionQuickInputPinnedLocal.value = webSessionQuickInputPinnedLocal.value.map((item, i) =>
    i === index ? value : item
  );
}

async function handleSaveWebSessionQuickInputPinned() {
  if (!webSessionQuickInputPinnedDirty.value) {
    return;
  }
  webSessionQuickInputPinnedSaving.value = true;
  try {
    const next = await settingsStore.saveWebSessionQuickInputPinned(
      webSessionQuickInputPinnedLocal.value
    );
    webSessionQuickInputPinnedOriginal.value = [...next.pinned];
    webSessionQuickInputPinnedLocal.value = [...next.pinned];
    message.success(t('common.saveSuccess'));
  } catch (error) {
    console.error('Failed to save web session quick input settings:', error);
    message.error(t('common.saveFailed'));
  } finally {
    webSessionQuickInputPinnedSaving.value = false;
  }
}

function handleResetWebSessionQuickInputPinned() {
  webSessionQuickInputPinnedLocal.value = [...DEFAULT_WEB_SESSION_QUICK_INPUT_PINNED];
}

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

const allSettingsCards = computed<SettingsCardDefinition[]>(() => {
  const cards: SettingsCardDefinition[] = [
    {
      id: 'project-workspace',
      title: t('settings.projectWorkspaceSettings'),
      description: t('settings.defaultEditor'),
      dirty: pageTitleDirty.value,
      searchTerms: [
        t('settings.pageTitle'),
        t('settings.pageTitleTip'),
        t('settings.recentProjectsLimit'),
        t('settings.dailyTipEnabled'),
        t('settings.terminalShortcut'),
        t('settings.notepadShortcut'),
        t('settings.defaultEditor'),
        t('settings.customCommand'),
      ],
    },
    {
      id: 'terminal',
      title: t('settings.terminalSettings'),
      description: t('settings.terminalDefaultRenderMode'),
      dirty: developerTerminalDirty.value,
      searchTerms: [
        t('settings.terminalLimit'),
        t('settings.confirmTerminalClose'),
        t('settings.sendResizeOnSwitch'),
        t('settings.terminalDefaultRenderMode'),
        t('settings.terminalConnectionPolicy'),
        t('settings.terminalDefaultSnapshotInterval'),
        t('settings.inactiveTerminalSnapshotInterval'),
        t('settings.terminalSnapshotZlibCompression'),
        t('settings.terminalShell'),
        t('settings.terminalServerStateSnapshot'),
        t('settings.terminalQuickActions'),
        t('settings.terminalQuickActionsList'),
        t('settings.terminalQuickActionNamePlaceholder'),
        t('settings.terminalQuickActionCommandPlaceholder'),
      ],
    },
    {
      id: 'session',
      title: t('settings.sessionSettings'),
      description: t('settings.webSessionStreamingMarkdownThrottle'),
      dirty: webSessionQuickInputPinnedDirty.value || developerSessionDirty.value,
      searchTerms: [
        t('settings.showWebSessionReasoning'),
        t('settings.webSessionActivityDisplayMode'),
        t('settings.webSessionActivityDisplayModeText'),
        t('settings.webSessionActivityDisplayModeCard'),
        t('settings.webSessionStreamingMarkdownThrottle'),
        t('settings.webSessionAutoContinueScope'),
        t('settings.webSessionAutoContinuePreset'),
        t('settings.webSessionAutoContinueMaxAttempts'),
        t('settings.webSessionAutoRetryDispatchPendingOnFailure'),
        t('settings.webSessionQuickInputPinned'),
        t('settings.webSessionCodexDefaultModel'),
        t('settings.webSessionCodexDefaultReasoningEffort'),
        t('settings.webSessionCodexDefaultPermissionLevel'),
        t('settings.webSessionCodexDefaultSyncMode'),
        t('settings.webSessionActiveCallTimeout'),
        t('settings.webSessionActiveCallTimeoutSeconds'),
        t('settings.webSessionActiveCallTimeoutCallKinds'),
        t('settings.webSessionActiveCallTimeoutPrompt'),
      ],
    },
    {
      id: 'security',
      title: t('settings.securityTitle'),
      description: t('settings.securityNewPassword'),
      dirty: authAccessDirty.value,
      searchTerms: [
        t('settings.securityCurrentPassword'),
        t('settings.securityNewPassword'),
        t('settings.securityConfirmPassword'),
        t('settings.securityEnableAction'),
        t('settings.securityDisableAction'),
        t('settings.securityAccessRulesTitle'),
        t('settings.securityAccessRulesBypassIPs'),
        t('settings.securityAccessRulesBypassDomains'),
        t('settings.securityAccessRulesForceAuthIPs'),
        t('settings.securityAccessRulesForceAuthDomains'),
        t('settings.securityTrustedProxies'),
        t('settings.securityAdminLoginAction'),
      ],
    },
    {
      id: 'developer',
      title: t('settings.developerOptions'),
      description: t('settings.developerScrollback'),
      dirty: developerBehaviorDirty.value,
      searchTerms: [t('settings.developerScrollback')],
    },
    {
      id: 'git',
      title: t('settings.gitSettings'),
      description: t('settings.gitSettingsDescription'),
      searchTerms: [
        t('settings.gitReadEngine'),
        t('settings.gitWriteEngine'),
        t('settings.gitExecutable'),
        t('settings.gitEngineBuiltin'),
        t('settings.gitEngineSystem'),
      ],
    },
    {
      id: 'worktree',
      title: t('settings.worktreeSettings'),
      description: t('settings.worktreeGlobalBaseDir'),
      dirty: worktreeSettingsDirty.value,
      searchTerms: [
        t('settings.worktreeGlobalBaseDir'),
        t('settings.worktreeGlobalDirNamePattern'),
      ],
    },
    {
      id: 'theme',
      title: t('settings.themeSettings'),
      description: t('settings.terminalFontSettings'),
      searchTerms: [
        t('theme.presetTheme'),
        t('theme.followSystem'),
        t('settings.terminalTheme'),
        t('settings.terminalFontFamily'),
        t('settings.terminalFontSize'),
        t('settings.terminalFontWeight'),
        t('settings.terminalLineHeight'),
        t('settings.terminalLetterSpacing'),
        t('settings.terminalWebGLRenderer'),
        t('settings.primaryColor'),
        t('settings.bodyColor'),
        t('settings.surfaceColor'),
        t('settings.textColor'),
        t('settings.terminalBg'),
        t('settings.terminalFg'),
        t('settings.terminalTabBg'),
        t('settings.terminalTabActiveBg'),
        t('settings.realtimePreview'),
        t('settings.previewTheme'),
        t('settings.sampleCard'),
      ],
    },
    {
      id: 'maintenance',
      title: t('settings.dataMaintenanceTitle'),
      description: t('settings.historyCleanupDescription'),
      searchTerms: [
        t('settings.historyCleanupTitle'),
        t('settings.historyCleanupAction'),
        t('settings.historyCleanupScopeProjects'),
      ],
    },
    {
      id: 'backup',
      title: t('settings.backupTitle'),
      description: t('settings.backupDescription'),
      searchTerms: [
        t('settings.backupExportAction'),
        t('settings.backupImportAction'),
        t('settings.backupPreviewTitle'),
        t('settings.backupSchemaVersion'),
      ],
    },
  ];

  return cards;
});

const settingsCards = computed<SettingsCardDefinition[]>(() => {
  const cards = allSettingsCards.value;
  const query = normalizeSearchText(settingsSearchQuery.value);
  if (!query) {
    return cards;
  }

  // 搜索时返回匹配的卡片，并添加 matchCount
  return cards
    .map(card => {
      const matches = [card.title, card.description, ...card.searchTerms].filter(term =>
        normalizeSearchText(term).includes(query)
      );
      return {
        ...card,
        matchCount: matches.length,
      };
    })
    .filter(card => card.matchCount > 0);
});

// 当搜索结果变化时，自动切换到第一个匹配的卡片
watch(
  () => settingsCards.value,
  newCards => {
    const query = normalizeSearchText(settingsSearchQuery.value);
    if (!query) {
      return;
    }
    const firstMatch = newCards[0];
    if (firstMatch && firstMatch.id !== activeSettingsSection.value) {
      activeSettingsSection.value = firstMatch.id;
      // 滚动到该设置区域
      nextTick(() => {
        const element = settingsSectionRefs.get(firstMatch.id);
        if (element) {
          element.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }
      });
    }
  }
);

const hasSearchQuery = computed(() => settingsSearchQuery.value.trim().length > 0);

// 搜索相关的函数
function highlightMatchText(text: string, query: string): string {
  const normalizedQuery = query.trim();
  if (!normalizedQuery) {
    return text;
  }
  const leadingWhitespace = text.match(/^\s+/)?.[0] ?? '';
  const content = text.slice(leadingWhitespace.length);
  const regex = new RegExp(`(${normalizedQuery.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
  return `${leadingWhitespace}${content.replace(
    regex,
    '<mark class="search-highlight">$1</mark>'
  )}`;
}

function normalizeSearchText(value: string) {
  return value.replace(/\s+/g, ' ').trim().toLowerCase();
}

function updateElementHighlight(element: Element | null, query: string) {
  if (!element) {
    return;
  }

  const savedText = element.getAttribute('data-original-text');
  const originalText = savedText ?? element.textContent ?? '';
  if (!savedText) {
    element.setAttribute('data-original-text', originalText);
  }

  if (!query) {
    element.textContent = originalText;
    return;
  }

  if (normalizeSearchText(originalText).includes(query)) {
    element.innerHTML = highlightMatchText(originalText, query);
  } else {
    element.textContent = originalText;
  }
}

function updateSearchHighlights() {
  const query = normalizeSearchText(settingsSearchQuery.value);
  const allFormItems = document.querySelectorAll('.general-settings-page [data-search-key]');

  allFormItems.forEach(formItem => {
    const searchKey = formItem.getAttribute('data-search-key');
    if (!searchKey) return;

    // 查找标签元素 - NaiveUI 的 n-form-item 标签在 .n-form-item-blank 之前
    // 尝试多种选择器
    let labelElement = null as Element | null;

    // 方法1: 直接查找 .n-form-item-label
    labelElement = formItem.querySelector('.n-form-item-label');

    // 方法2: 如果找不到，尝试查找包含标签的 span 元素
    if (!labelElement) {
      const children = Array.from(formItem.children);
      for (const child of children) {
        if (
          child.classList.contains('n-form-item-label') ||
          (child.querySelector && child.querySelector('.n-form-item-label'))
        ) {
          labelElement = child.classList.contains('n-form-item-label')
            ? child
            : child.querySelector('.n-form-item-label');
          break;
        }
      }
    }

    // 方法3: 如果还是找不到，尝试查找第一个包含文本的子元素
    if (!labelElement) {
      const allChildren = formItem.querySelectorAll('*');
      for (const child of allChildren) {
        if (child.classList.contains('n-form-item-label')) {
          labelElement = child;
          break;
        }
      }
    }

    if (!labelElement) return;

    // 获取原始文本（保存到 data 属性中避免重复处理）
    const savedText = labelElement.getAttribute('data-original-text');
    const originalText = savedText || labelElement.textContent || '';
    if (!savedText) {
      labelElement.setAttribute('data-original-text', originalText);
    }

    const labelText = originalText;
    const normalizedSearchKey = normalizeSearchText(searchKey);
    const formText = normalizeSearchText(formItem.textContent || '');
    const sectionElement = formItem.closest('.settings-card-shell') as HTMLElement | null;
    const sectionId = sectionElement?.dataset.sectionId as SettingsSectionId | undefined;
    const sectionMeta = sectionId
      ? allSettingsCards.value.find(card => card.id === sectionId)
      : undefined;
    const sectionText = sectionMeta
      ? normalizeSearchText(
          [sectionMeta.title, sectionMeta.description, ...sectionMeta.searchTerms].join(' ')
        )
      : '';

    const isVisible =
      !query ||
      normalizedSearchKey.includes(query) ||
      formText.includes(query) ||
      sectionText.includes(query);

    // 控制可见性
    if (isVisible) {
      formItem.classList.remove('form-item-hidden');
    } else {
      formItem.classList.add('form-item-hidden');
    }

    // 更新高亮
    updateElementHighlight(labelElement, query);

    const tipElements = formItem.querySelectorAll('.form-tip');
    tipElements.forEach(tip => {
      updateElementHighlight(tip, query);
    });
  });
}

function scheduleUpdateSearchHighlights() {
  nextTick(() => {
    setTimeout(() => {
      updateSearchHighlights();
    }, 50);
  });
}

// 监听搜索查询变化
watch(settingsSearchQuery, () => {
  scheduleUpdateSearchHighlights();
});

watch(activeSettingsSection, () => {
  scheduleUpdateSearchHighlights();
});

onMounted(() => {
  settingsSectionRefs.forEach((element, section) => {
    element.dataset.sectionId = section;
  });
  setTimeout(() => {
    updateSearchHighlights();
  }, 100);
});

function isSettingsSectionVisible(section: SettingsSectionId) {
  return activeSettingsSection.value === section;
}

function settingsCardShellClass(section: SettingsSectionId) {
  return {
    'is-highlighted': highlightedSettingsSection.value === section,
  };
}

function handleResetFontFamily() {
  settingsStore.updateTerminalFont({ fontFamily: '' });
}

const terminalShortcutValue = computed(
  () => terminalShortcut.value.display || terminalShortcut.value.code
);
const notepadShortcutValue = computed(
  () => notepadShortcut.value.display || notepadShortcut.value.code
);
const isTerminalShortcutDefault = computed(
  () => terminalShortcut.value.code === DEFAULT_TERMINAL_SHORTCUT.code
);
const isNotepadShortcutDefault = computed(
  () => notepadShortcut.value.code === DEFAULT_NOTEPAD_SHORTCUT.code
);

function handleBack() {
  clearSettingsSectionHighlight();
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
/* ========================================
   设置界面样式 - 左侧边栏 + 主内容区
   ======================================== */

/* 页面容器 */
.general-settings-page {
  max-width: 1400px;
  margin: 0 auto;
  padding: 24px 32px 48px;
}

/* 主布局 - 左侧边栏 + 右侧内容 */
.settings-layout {
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 40px;
  align-items: start;
}

/* 左侧导航 */
.settings-sidebar {
  position: sticky;
  top: 32px;
}

/* 搜索框 */
.settings-search-box {
  margin-bottom: 12px;
}

.settings-search-box :deep(.n-input) {
  height: 40px;
}

.settings-search-box :deep(.n-input__input-el) {
  height: 38px;
  font-size: 14px;
}

.settings-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.settings-nav-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  padding: 11px 16px;
  border: none;
  border-radius: 10px;
  background: transparent;
  color: var(--n-text-color-2);
  font-size: 14px;
  font-weight: 500;
  text-align: left;
  cursor: pointer;
  transition:
    background-color 0.15s ease,
    color 0.15s ease,
    transform 0.15s ease;
}

.settings-nav-item:hover {
  background-color: var(--n-color-hover);
  color: var(--n-text-color-1);
  transform: translateX(2px);
}

.settings-nav-item.is-active {
  background-color: var(--n-primary-color);
  color: #ffffff;
  transform: translateX(0);
}

.settings-nav-item__title {
  flex: 1;
}

.settings-nav-item__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: #ffffff;
  flex-shrink: 0;
}

/* 右侧内容区 */
.settings-main {
  min-width: 0;
  max-width: 800px;
}

.settings-main-stack {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* 设置卡片容器 */
.settings-card-shell {
  display: flex;
  flex-direction: column;
  gap: 24px;
  scroll-margin-top: 20px;
}

.settings-card-shell.is-highlighted :deep(.n-card) {
  box-shadow: 0 0 0 2px var(--n-primary-color);
}

/* 表单样式 */
.form-tip {
  font-size: 13px;
  color: var(--n-text-color-3);
  margin-top: 6px;
  line-height: 1.5;
}

.form-error {
  font-size: 13px;
  color: var(--n-error-color, #d03050);
  margin-top: 6px;
  line-height: 1.5;
}

:deep(.n-form-item) {
  margin-bottom: 24px;
  align-items: flex-start;
}

:deep(.n-form-item:last-child) {
  margin-bottom: 0;
}

:deep(.n-form-item-label) {
  display: flex;
  align-items: flex-start;
  font-weight: 500;
  color: var(--n-text-color-1);
  font-size: 14px;
  padding-top: 6px;
}

:deep(.n-form-item-blank) {
  width: 100%;
  min-width: 0;
}

/* 统一输入框样式 */
:deep(.n-input),
:deep(.n-input-number),
:deep(.n-select) {
  border-radius: 8px;
}

:deep(.n-button) {
  border-radius: 8px;
}

/* 卡片样式 */
:deep(.n-card) {
  border: 1px solid var(--n-border-color);
  border-radius: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  background-color: var(--app-surface-color, var(--n-card-color));
}

:deep(.n-card-header) {
  padding: 18px 24px;
  border-bottom: 1px solid var(--n-border-color);
}

:deep(.n-card__content) {
  padding: 28px 24px;
}

/* 页面头部 */
:deep(.n-page-header) {
  padding-bottom: 20px;
}

:deep(.n-page-header .n-page-header-header) {
  align-items: center;
}

:deep(.n-page-header__title) {
  font-size: 20px;
  font-weight: 600;
}

/* 预览面板 */
.preview-panel {
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid var(--n-border-color);
  background-color: var(--preview-panel-bg, var(--app-surface-color));
}

.preview-banner {
  background-color: var(--preview-banner-bg, var(--n-primary-color));
  color: var(--preview-banner-text, var(--kanban-terminal-fg));
  padding: 12px;
  font-size: 14px;
  font-weight: 600;
}

.preview-content {
  padding: 16px;
  background-color: var(--preview-content-bg, var(--app-surface-color));
  color: var(--preview-content-text, var(--kanban-terminal-fg));
}

/* 工具类 */
.settings-field-stack {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  min-width: 0;
}

.settings-command-input {
  width: 100%;
  max-width: 560px;
}

.settings-command-input--shell {
  max-width: 320px;
}

.settings-hidden-file-input {
  display: none;
}

.settings-backup-file-name {
  font-size: 13px;
  color: var(--n-text-color-2);
}

.settings-backup-group,
.settings-backup-subgroup {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.settings-backup-subgroup {
  padding: 4px 0 0 12px;
}

.settings-backup-group__title,
.settings-backup-subgroup__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--n-text-color-1);
}

.settings-backup-meta-label {
  font-size: 12px;
  color: var(--n-text-color-3);
  margin-bottom: 4px;
}

.settings-backup-meta-value {
  font-size: 14px;
  color: var(--n-text-color-1);
  font-weight: 500;
}

.settings-backup-section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--n-text-color-1);
  margin-bottom: 12px;
}

.settings-backup-section-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.settings-backup-section-item {
  border: 1px solid var(--n-border-color);
  border-radius: 10px;
  padding: 12px 14px;
  background: var(--app-surface-color, var(--n-card-color));
}

.settings-backup-section-item__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--n-text-color-1);
}

.settings-backup-section-item__meta,
.settings-backup-section-item__keys {
  margin-top: 4px;
  font-size: 12px;
  color: var(--n-text-color-3);
  word-break: break-word;
}

.settings-collapsible-field {
  width: 100%;
  margin-top: 8px;
}

.shortcut-hint {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.unit-label {
  font-size: 12px;
  color: var(--n-text-color-3);
  margin-left: 4px;
}

/* 终端快捷操作相关 */
.terminal-quick-action-item {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.web-session-quick-input-textarea {
  width: 100%;
}

.terminal-quick-action-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.terminal-quick-action-row-inputs {
  gap: 16px;
}

.terminal-quick-action-row-icons {
  width: 100%;
  flex-wrap: wrap;
}

.terminal-quick-action-input {
  flex: 1;
  min-width: 180px;
}

.terminal-quick-action-icon-grid {
  display: flex;
  width: 100%;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-start;
}

.terminal-quick-action-icon-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 42px;
  width: 42px;
  height: 32px;
  padding: 0;
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  background-color: var(--n-color, transparent);
  color: var(--n-text-color-2);
  cursor: pointer;
  transition:
    border-color 0.15s ease,
    background-color 0.15s ease,
    color 0.15s ease;
}

.terminal-quick-action-icon-button:hover {
  border-color: var(--n-primary-color-hover, var(--n-primary-color));
  color: var(--n-primary-color-hover, var(--n-primary-color));
}

.terminal-quick-action-icon-button.is-active {
  border-color: var(--n-primary-color);
  background-color: color-mix(in srgb, var(--n-primary-color) 10%, transparent);
  color: var(--n-primary-color);
}

.terminal-quick-action-icon-button :deep(.n-icon) {
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
    padding: 10px 12px 12px;
    max-width: 100%;
    height: 100dvh;
    overflow-x: hidden;
    overflow-y: hidden;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
  }

  /* 移动端：侧边栏变为顶部横向滚动导航 */
  .settings-layout {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-width: 0;
    min-height: 0;
    flex: 1 1 auto;
    overflow: hidden;
  }

  .settings-sidebar {
    position: static;
    min-width: 0;
    flex: 0 0 auto;
  }

  .settings-nav {
    flex-direction: row;
    flex-wrap: nowrap;
    overflow-x: auto;
    overflow-y: hidden;
    padding-bottom: 6px;
    gap: 8px;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
    touch-action: pan-x;
    overscroll-behavior-x: contain;
  }

  .settings-nav::-webkit-scrollbar {
    display: none;
  }

  .settings-search-box {
    margin-bottom: 8px;
  }

  .settings-nav-item {
    width: auto;
    flex: 0 0 auto;
    min-width: max-content;
    padding: 8px 12px;
    white-space: nowrap;
  }

  .settings-main {
    max-width: none;
    min-width: 0;
    min-height: 0;
    flex: 1 1 auto;
    overflow-y: auto;
    overflow-x: hidden;
    padding-bottom: 16px;
    -webkit-overflow-scrolling: touch;
    overscroll-behavior-y: contain;
  }

  .settings-main-stack,
  .settings-card-shell {
    width: 100%;
    gap: 16px;
    min-width: 0;
  }

  /* 表单标签置顶 */
  .general-settings-page :deep(.n-page-header),
  .general-settings-page :deep(.n-page-header-wrapper),
  .general-settings-page :deep(.n-page-header-header),
  .general-settings-page :deep(.n-page-header-content),
  .general-settings-page :deep(.n-page-header-header__main),
  .general-settings-page :deep(.n-page-header-header__title),
  .general-settings-page :deep(.n-page-header-header__extra) {
    min-width: 0;
  }

  .general-settings-page :deep(.n-page-header-header) {
    flex-wrap: wrap;
    gap: 10px;
  }

  .general-settings-page :deep(.n-page-header-header__extra) {
    width: 100%;
  }

  .general-settings-page :deep(.n-page-header-header__extra .n-space) {
    width: 100%;
    justify-content: flex-start;
    flex-wrap: wrap !important;
  }

  .general-settings-page :deep(.n-page-header) {
    flex: 0 0 auto;
    margin-bottom: 8px;
    padding-bottom: 8px;
  }

  .settings-header-actions {
    gap: 8px !important;
  }

  .general-settings-page :deep(.settings-header-actions .n-button) {
    min-width: 36px;
  }

  .general-settings-page :deep(.settings-header-reset .n-button__content) {
    display: none;
  }

  .general-settings-page :deep(.settings-header-reset .n-base-icon) {
    margin-right: 0;
  }

  .general-settings-page :deep(.n-form-item) {
    margin-bottom: 18px;
    min-width: 0;
  }

  .general-settings-page :deep(.n-form-item-label) {
    text-align: left;
    padding-bottom: 8px;
    padding-top: 0;
  }

  .general-settings-page :deep(.n-form-item-blank) {
    width: 100%;
    min-width: 0;
  }

  .general-settings-page :deep(.n-form),
  .general-settings-page :deep(.n-card),
  .general-settings-page :deep(.n-card__content),
  .general-settings-page :deep(.n-form-item-feedback-wrapper) {
    min-width: 0;
  }

  .general-settings-page :deep(.n-form-item .n-space) {
    width: 100%;
    min-width: 0;
    flex-wrap: wrap !important;
  }

  .general-settings-page :deep(.n-form-item .n-space-item) {
    min-width: 0;
    max-width: 100%;
  }

  .general-settings-page :deep(.n-input-number),
  .general-settings-page :deep(.n-select),
  .general-settings-page :deep(.n-input) {
    max-width: none !important;
    width: 100%;
  }

  .general-settings-page :deep(.n-input-number .n-input__input-el) {
    text-align: left;
  }

  .general-settings-page :deep(.n-button-group) {
    display: flex;
    flex-wrap: wrap;
  }

  .general-settings-page :deep(.n-slider) {
    width: 100% !important;
    min-width: 0;
  }

  /* 卡片内容紧凑 */
  .general-settings-page :deep(.n-card-header) {
    padding: 12px 16px;
  }

  .general-settings-page :deep(.n-card__content) {
    padding: 16px;
  }

  /* 预览面板 */
  .preview-content {
    padding: 12px;
  }

  .preview-banner {
    padding: 10px;
    font-size: 13px;
  }

  .settings-command-input,
  .settings-command-input--shell {
    max-width: none;
  }

  .terminal-quick-action-row-inputs,
  .terminal-quick-action-row-icons {
    flex-direction: column;
    align-items: stretch;
  }

  .terminal-quick-action-input {
    min-width: 0;
    width: 100%;
  }

  .general-settings-page :deep(.n-dynamic-input-item) {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
  }

  .general-settings-page :deep(.n-dynamic-input-item__action) {
    width: 100%;
    margin: 0 !important;
    justify-content: flex-end;
  }

  .terminal-quick-action-icon-grid {
    gap: 6px;
  }
}

/* 平板端响应式 */
@media (min-width: 768px) and (max-width: 1023px) {
  .general-settings-page {
    padding: 20px;
  }

  .settings-layout {
    grid-template-columns: 200px 1fr;
    gap: 24px;
  }

  .settings-main {
    max-width: none;
  }
}

/* ========================================
   搜索高亮样式
   ======================================== */
:deep(.search-highlight) {
  background-color: var(--n-primary-color-hover, #ffec3d);
  color: inherit;
  padding: 1px 2px;
  border-radius: 2px;
  font-weight: 600;
}

.form-item-hidden {
  display: none !important;
}
</style>
