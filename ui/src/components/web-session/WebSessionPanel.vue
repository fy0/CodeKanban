<template>
  <div class="web-session-panel" :style="webSessionStyleVars">
    <WebSessionCompletionNotifier />
    <WebSessionApprovalNotifier />
    <aside
      v-if="webSessionDevMode"
      class="web-session-dev-panel"
      :class="{ 'has-cyber-policy-warning': showCyberPolicyWarning }"
      aria-label="DEV"
    >
      <span class="web-session-dev-title">DEV</span>
      <label class="web-session-dev-control">
        <span>{{ t('webSession.devCyberPolicyWarning') }}</span>
        <n-switch v-model:value="devCyberPolicyWarning" size="small" :disabled="!currentSession" />
      </label>
    </aside>
    <PiProjectTrustDialog
      v-if="props.projectId"
      v-model:show="showPiTrustDialog"
      :project-id="props.projectId"
      :project-path="piTrustProjectPath"
      @trusted="handlePiProjectTrusted"
    />
    <WebSessionImportDialog
      v-if="props.projectId"
      v-model:show="showImportDialog"
      :project-id="props.projectId"
      :pending-session-id="importingCodexSessionId"
      @import-session="handleImportCodexSession"
      @open-existing-session="handleOpenImportedCodexSession"
    />
    <WebSessionTreeDrawer
      v-if="currentRealSession && canOpenPiTree"
      v-model:show="showPiTreeDrawer"
      :project-id="props.projectId"
      :session-id="currentRealSession.id"
      :can-mutate="canMutatePiTree"
      @navigated="handlePiTreeNavigated"
      @created="handlePiTreeCreated"
    />

    <WebSessionMobileSessionDrawer
      v-if="isMobile"
      :show="showMobileTabSelector"
      :category="mobileSessionCategory"
      :current-count="mobileCurrentSessions.length"
      :archived-total="mobileArchivedMeta.total"
      :archived-loading="mobileArchivedMeta.loading"
      :items="mobileTabDescriptors"
      :session-views="mobileSessionDrawerViews"
      :current-project-scope="sidebarScope === 'current'"
      :scope-toggle-title="sidebarScopeToggleTitle"
      @update:show="handleMobileSessionSelectorVisibilityChange"
      @select-category="setMobileSessionCategory"
      @toggle-group="toggleSessionGroup"
      @select-session="handleMobileTabSessionSelect"
      @load-more="loadMoreMobileArchivedSessions"
      @toggle-scope="toggleSidebarScope"
      @new-session="handleMobileTabNewSession"
    />

    <div class="panel-main">
      <div class="panel-body">
        <div class="panel-content">
          <div
            v-if="isMobile"
            class="mobile-project-context-bar"
            :aria-label="mobileProjectContextLabel"
          >
            <div class="mobile-project-context-main">
              <span class="mobile-project-context-icon" aria-hidden="true">
                <n-icon size="17"><FolderOpenOutline /></n-icon>
              </span>
              <span class="mobile-project-context-copy">
                <span class="mobile-project-context-name">{{ mobileProjectName }}</span>
                <span v-if="mobileProjectBranch" class="mobile-project-context-branch">
                  <n-icon size="11" aria-hidden="true"><GitBranchOutline /></n-icon>
                  <span>{{ mobileProjectBranch }}</span>
                </span>
              </span>
            </div>

            <div class="mobile-project-context-actions">
              <n-tooltip
                v-if="showMobileChangesSummaryBadge"
                :disabled="!mobileChangesSummaryIncomplete"
              >
                <template #trigger>
                  <span
                    class="mobile-changes-summary-badge"
                    :title="mobileChangesSummaryLabel"
                    :aria-label="mobileChangesSummaryLabel"
                  >
                    <span class="changes-summary-count">{{
                      mobileChangesSummaryDisplay.count
                    }}</span>
                    <template v-if="mobileChangesSummaryIncomplete">
                      <n-icon :size="12" class="changes-summary-warning">
                        <WarningOutline />
                      </n-icon>
                    </template>
                    <template v-else>
                      <span class="changes-summary-separator">,</span>
                      <span class="changes-summary-add">{{
                        mobileChangesSummaryDisplay.additions
                      }}</span>
                      <span class="changes-summary-separator">,</span>
                      <span class="changes-summary-del">{{
                        mobileChangesSummaryDisplay.deletions
                      }}</span>
                      <n-icon
                        v-if="mobileChangesSummaryLoading"
                        :size="11"
                        class="changes-summary-loading"
                      >
                        <SyncOutline />
                      </n-icon>
                    </template>
                  </span>
                </template>
                {{ mobileChangesSummaryStatusText }}
              </n-tooltip>

              <n-dropdown
                trigger="click"
                placement="bottom-end"
                :options="mobileProjectSwitchOptions"
                @select="handleMobileProjectSwitchSelect"
                @update:show="handleMobileProjectSwitchMenuShow"
              >
                <n-button
                  text
                  size="small"
                  class="mobile-project-switch"
                  :title="t('webSession.switchProject')"
                  :aria-label="t('webSession.switchProject')"
                >
                  <template #icon>
                    <span
                      v-if="currentMobileProjectBadge"
                      class="mobile-project-switch-badge"
                      :style="{ background: currentMobileProjectBadge.color }"
                    >
                      {{ currentMobileProjectBadge.label }}
                    </span>
                  </template>
                  <n-icon size="14"><ChevronDownOutline /></n-icon>
                </n-button>
              </n-dropdown>
            </div>
          </div>

          <div class="panel-header">
            <div
              v-if="isMobile && (sessions.length > 0 || archivedPreviewSession)"
              class="mobile-tab-selector"
            >
              <button
                type="button"
                class="mobile-tab-trigger"
                :title="currentSession ? getSessionStatusTooltip(currentSession) : undefined"
                @click.stop="handleMobileTabTriggerClick"
              >
                <span class="mobile-tab-trigger-main">
                  <span class="mobile-tab-title">{{ activeSessionTitle }}</span>
                  <span
                    v-if="activeSessionStatusLabel"
                    class="ai-status-pill mobile-tab-trigger-status"
                    :class="`state-${activeSessionAttentionStateClass}`"
                  >
                    <span class="mobile-tab-trigger-status-text">
                      {{ activeSessionStatusLabel }}
                    </span>
                  </span>
                  <n-icon
                    v-if="activeSessionHasWorkflowPlanBadge"
                    class="mobile-tab-trigger-plan-badge"
                    :class="{ 'is-scheduled': activeSessionHasScheduledPlanExecution }"
                    size="12"
                    aria-hidden="true"
                  >
                    <FlagIcon />
                  </n-icon>
                </span>
                <n-icon class="mobile-tab-arrow" :class="{ 'is-open': showMobileTabSelector }">
                  <ChevronDownOutline />
                </n-icon>
              </button>
            </div>

            <div v-else-if="sessions.length > 0" ref="tabsContainerRef" class="tabs-container">
              <n-tabs
                :value="activeTabSessionId"
                type="card"
                closable
                size="small"
                :theme-overrides="tabsThemeOverrides"
                @update:value="handleSessionSelect"
                @close="handleArchiveSession"
              >
                <n-tab-pane
                  v-for="session in sessions"
                  :key="session.id"
                  :name="session.id"
                  display-directive="show:lazy"
                  :tab-props="createTabProps(session)"
                >
                  <template #tab>
                    <span class="tab-label" :title="session.title">
                      <n-icon
                        v-if="shouldShowSessionWorkflowPlanBadge(session)"
                        class="tab-workflow-plan-flag"
                        :class="{ 'is-scheduled': hasScheduledPlanExecution(session) }"
                        size="12"
                        aria-hidden="true"
                      >
                        <FlagIcon />
                      </n-icon>
                      <span
                        v-if="shouldShowSessionStatusDot(session)"
                        class="status-dot"
                        :class="getSessionStatusDotClass(session)"
                      ></span>
                      <span class="tab-title" :style="tabTitleStyle">{{ session.title }}</span>
                      <span
                        v-if="isSessionArchiving(session.id)"
                        class="tab-action-spinner"
                        aria-hidden="true"
                      ></span>
                      <span
                        class="ai-status-pill"
                        :class="[
                          `state-${getSessionPillStateClass(session)}`,
                          getSessionPillSizeClass(),
                        ]"
                        :title="getSessionStatusTooltip(session)"
                      >
                        <span
                          class="ai-status-icon"
                          v-html="getSessionAssistantIcon(session)"
                        ></span>
                        <span class="ai-status-text">{{ getSessionStatusLabel(session) }}</span>
                        <span class="ai-status-emoji">{{ getSessionStatusEmoji(session) }}</span>
                      </span>
                    </span>
                  </template>
                </n-tab-pane>
              </n-tabs>
              <div class="active-tab-indicator" :style="activeTabIndicatorStyle"></div>
            </div>

            <div v-else-if="!archivedPreviewSession" class="empty-tabs-label">
              {{ emptyStateTitle }}
            </div>

            <n-dropdown
              trigger="manual"
              placement="bottom-start"
              :show="!!contextMenuSession"
              :options="contextMenuOptions"
              :x="contextMenuX"
              :y="contextMenuY"
              @select="handleContextMenuSelect"
              @clickoutside="contextMenuSession = null"
            />

            <div class="header-actions">
              <n-dropdown
                v-if="isMobile"
                class="mobile-header-action-menu"
                trigger="click"
                placement="bottom-end"
                :options="mobileActionMenuOptions"
                @select="handleMobileActionMenuSelect"
              >
                <n-button
                  secondary
                  size="small"
                  class="new-session-button"
                  :title="t('common.moreActions')"
                  :aria-label="t('common.moreActions')"
                >
                  <template #icon>
                    <n-icon><AddOutline /></n-icon>
                  </template>
                </n-button>
              </n-dropdown>
              <n-tooltip v-else trigger="hover" placement="bottom" :delay="100">
                <template #trigger>
                  <n-button
                    text
                    size="small"
                    class="desktop-header-icon-button"
                    :title="t('webSession.newSession')"
                    :aria-label="t('webSession.newSession')"
                    @click="handleStartDraftSession()"
                  >
                    <template #icon>
                      <n-icon><AddOutline /></n-icon>
                    </template>
                  </n-button>
                </template>
                {{ t('webSession.newSession') }}
              </n-tooltip>
              <n-tooltip v-if="!isMobile" trigger="hover" placement="bottom" :delay="100">
                <template #trigger>
                  <n-button
                    text
                    size="small"
                    class="desktop-header-icon-button"
                    :title="t('webSession.importCodexSession')"
                    :aria-label="t('webSession.importCodexSession')"
                    @click="openImportDialog()"
                  >
                    <template #icon>
                      <n-icon><TimeOutline /></n-icon>
                    </template>
                  </n-button>
                </template>
                {{ t('webSession.importCodexSession') }}
              </n-tooltip>
              <n-tooltip
                v-if="!isMobile && canOpenPiTree"
                trigger="hover"
                placement="bottom"
                :delay="100"
              >
                <template #trigger>
                  <n-button
                    text
                    size="small"
                    class="desktop-header-icon-button"
                    :title="t('webSession.treeOpen')"
                    :aria-label="t('webSession.treeOpen')"
                    @click="showPiTreeDrawer = true"
                  >
                    <template #icon>
                      <n-icon><GitNetworkOutline /></n-icon>
                    </template>
                  </n-button>
                </template>
                {{ t('webSession.treeOpen') }}
              </n-tooltip>
            </div>
          </div>

          <div v-if="currentSession" class="timeline-shell">
            <div
              ref="timelineScrollRef"
              class="timeline-scroll"
              @click.capture="handleTimelineLinkClick"
              @scroll="handleTimelineScroll"
              @wheel.passive="handleTimelineWheel"
              @pointerdown.passive="handleTimelinePointerDown"
              @touchstart.passive="handleTimelineTouchStart"
              @touchmove.passive="handleTimelineTouchMove"
              @touchend.passive="handleTimelineTouchEnd"
              @touchcancel.passive="handleTimelineTouchEnd"
            >
              <div ref="timelineListRef" class="timeline-list">
                <div
                  class="timeline-agent-toolbar"
                  :class="{
                    'is-search-open': timelineSearchOpen,
                  }"
                >
                  <template v-if="timelineSearchOpen">
                    <n-input
                      ref="timelineSearchInputRef"
                      v-model:value="timelineSearchQuery"
                      class="timeline-search-input"
                      size="small"
                      clearable
                      :placeholder="t('webSession.conversationSearchPlaceholder')"
                      :aria-label="t('webSession.conversationSearchPlaceholder')"
                      @keydown.esc="closeTimelineSearch"
                    >
                      <template #prefix>
                        <n-icon size="15" aria-hidden="true"><SearchOutline /></n-icon>
                      </template>
                    </n-input>
                    <n-popover trigger="click" placement="bottom-end">
                      <template #trigger>
                        <n-button
                          quaternary
                          circle
                          size="small"
                          :type="timelineSearchHasNonDefaultFilters ? 'primary' : 'default'"
                          :title="t('webSession.conversationSearchFilters')"
                          :aria-label="t('webSession.conversationSearchFilters')"
                        >
                          <template #icon>
                            <n-icon><FunnelOutline /></n-icon>
                          </template>
                        </n-button>
                      </template>
                      <div class="timeline-search-filter-popover">
                        <n-checkbox v-model:checked="timelineSearchFilters.user">
                          {{ t('webSession.conversationSearchUser') }}
                        </n-checkbox>
                        <n-checkbox v-model:checked="timelineSearchFilters.assistant">
                          {{ t('webSession.conversationSearchAssistant') }}
                        </n-checkbox>
                        <n-checkbox v-model:checked="timelineSearchFilters.tools">
                          {{ t('webSession.conversationSearchTools') }}
                        </n-checkbox>
                        <n-checkbox v-model:checked="timelineSearchFilters.system">
                          {{ t('webSession.conversationSearchSystem') }}
                        </n-checkbox>
                      </div>
                    </n-popover>
                    <n-popover
                      v-if="timelineSearchQuery.trim() && timelineSearchMatches.length > 0"
                      trigger="click"
                      placement="bottom-end"
                      :show-arrow="false"
                    >
                      <template #trigger>
                        <button
                          type="button"
                          class="timeline-search-count timeline-search-count-trigger"
                          :title="t('webSession.conversationSearchJump')"
                          :aria-label="`${t('webSession.conversationSearchJump')}: ${timelineSearchResultLabel}`"
                          aria-live="polite"
                        >
                          {{ timelineSearchResultLabel }}
                          <n-icon size="12" aria-hidden="true"><ChevronDownOutline /></n-icon>
                        </button>
                      </template>
                      <div class="timeline-search-jump-popover">
                        <div class="timeline-search-jump-title">
                          {{ t('webSession.conversationSearchJump') }}
                        </div>
                        <n-pagination
                          :page="timelineSearchCurrentIndex + 1"
                          :page-count="timelineSearchMatches.length"
                          :simple="true"
                          size="small"
                          @update:page="handleTimelineSearchPageChange"
                        />
                      </div>
                    </n-popover>
                    <span
                      v-else-if="timelineSearchQuery.trim()"
                      class="timeline-search-count"
                      aria-live="polite"
                    >
                      {{ timelineSearchResultLabel }}
                    </span>
                    <n-tooltip trigger="hover" placement="bottom" :delay="100">
                      <template #trigger>
                        <n-button
                          quaternary
                          circle
                          size="small"
                          :disabled="!timelineSearchHasPrevious"
                          :title="t('webSession.conversationSearchPrevious')"
                          :aria-label="t('webSession.conversationSearchPrevious')"
                          @click="void navigateTimelineSearch('previous')"
                        >
                          <template #icon>
                            <n-icon><ChevronUpOutline /></n-icon>
                          </template>
                        </n-button>
                      </template>
                      {{ t('webSession.conversationSearchPrevious') }}
                    </n-tooltip>
                    <n-tooltip trigger="hover" placement="bottom" :delay="100">
                      <template #trigger>
                        <n-button
                          quaternary
                          circle
                          size="small"
                          :disabled="!timelineSearchHasNext"
                          :title="t('webSession.conversationSearchNext')"
                          :aria-label="t('webSession.conversationSearchNext')"
                          @click="void navigateTimelineSearch('next')"
                        >
                          <template #icon>
                            <n-icon><ChevronDownOutline /></n-icon>
                          </template>
                        </n-button>
                      </template>
                      {{ t('webSession.conversationSearchNext') }}
                    </n-tooltip>
                    <n-button
                      quaternary
                      circle
                      size="small"
                      :title="t('webSession.conversationSearchClose')"
                      :aria-label="t('webSession.conversationSearchClose')"
                      @click="closeTimelineSearch"
                    >
                      <template #icon>
                        <n-icon><CloseOutline /></n-icon>
                      </template>
                    </n-button>
                  </template>

                  <span
                    v-if="!timelineSearchOpen"
                    class="timeline-navigation-reveal-zone"
                    :class="{
                      'is-expanded': timelineNavigationControlsExpanded,
                      'is-mobile': isMobile,
                    }"
                    @mouseenter="handleTimelineNavigationPointerEnter"
                    @mouseleave="handleTimelineNavigationPointerLeave"
                    @focusin="handleTimelineNavigationFocusIn"
                    @focusout="handleTimelineNavigationFocusOut"
                  >
                    <transition name="timeline-navigation-reveal">
                      <span
                        v-if="timelineNavigationControlsExpanded"
                        class="timeline-navigation-controls"
                      >
                        <n-popover
                          trigger="manual"
                          placement="bottom"
                          :show="timelineStartConfirmationArmed"
                          :show-arrow="true"
                          :disabled="!timelineStartConfirmationArmed"
                        >
                          <template #trigger>
                            <span class="timeline-navigation-trigger">
                              <n-tooltip trigger="hover" placement="bottom" :delay="100">
                                <template #trigger>
                                  <n-button
                                    quaternary
                                    circle
                                    size="small"
                                    :class="[
                                      'timeline-navigation-button',
                                      { 'is-confirm-armed': timelineStartConfirmationArmed },
                                    ]"
                                    :loading="timelineNavigationPending === 'start'"
                                    :disabled="timelineNavigationBusy || !currentRealSession"
                                    :title="t('webSession.timelineJumpToStart')"
                                    :aria-label="t('webSession.timelineJumpToStart')"
                                    @click="void handleTimelineStartClick()"
                                  >
                                    <template #icon>
                                      <n-icon><ArrowUpOutline /></n-icon>
                                    </template>
                                  </n-button>
                                </template>
                                {{ t('webSession.timelineJumpToStart') }}
                              </n-tooltip>
                            </span>
                          </template>
                          <div
                            class="timeline-start-confirm-popover-card"
                            role="status"
                            aria-live="polite"
                          >
                            <div class="timeline-start-confirm-title">
                              {{ t('webSession.timelineJumpToStartConfirmTitle') }}
                            </div>
                            <div class="timeline-start-confirm-body">
                              {{ t('webSession.timelineJumpToStartConfirmBody') }}
                            </div>
                          </div>
                        </n-popover>
                        <n-tooltip trigger="hover" placement="bottom" :delay="100">
                          <template #trigger>
                            <n-button
                              quaternary
                              circle
                              size="small"
                              class="timeline-navigation-button"
                              :loading="timelineNavigationPending === 'previous'"
                              :disabled="
                                timelineNavigationBusy || !timelineViewportNavigation.previous
                              "
                              :title="t('terminal.prevUserMessage')"
                              :aria-label="t('terminal.prevUserMessage')"
                              @click="void navigateTimelineViewportUserMessage('previous')"
                            >
                              <template #icon>
                                <n-icon><ChevronUpOutline /></n-icon>
                              </template>
                            </n-button>
                          </template>
                          {{ t('terminal.prevUserMessage') }}
                        </n-tooltip>
                        <n-tooltip trigger="hover" placement="bottom" :delay="100">
                          <template #trigger>
                            <n-button
                              quaternary
                              circle
                              size="small"
                              class="timeline-navigation-button"
                              :loading="timelineNavigationPending === 'next'"
                              :disabled="timelineNavigationBusy || !timelineViewportNavigation.next"
                              :title="t('terminal.nextUserMessage')"
                              :aria-label="t('terminal.nextUserMessage')"
                              @click="void navigateTimelineViewportUserMessage('next')"
                            >
                              <template #icon>
                                <n-icon><ChevronDownOutline /></n-icon>
                              </template>
                            </n-button>
                          </template>
                          {{ t('terminal.nextUserMessage') }}
                        </n-tooltip>
                        <n-tooltip trigger="hover" placement="bottom" :delay="100">
                          <template #trigger>
                            <n-button
                              quaternary
                              circle
                              size="small"
                              class="timeline-navigation-button"
                              :loading="timelineNavigationPending === 'end'"
                              :disabled="timelineNavigationBusy || !currentRealSession"
                              :title="t('webSession.timelineJumpToEnd')"
                              :aria-label="t('webSession.timelineJumpToEnd')"
                              @click="void jumpToTimelineEnd()"
                            >
                              <template #icon>
                                <n-icon><ArrowDownOutline /></n-icon>
                              </template>
                            </n-button>
                          </template>
                          {{ t('webSession.timelineJumpToEnd') }}
                        </n-tooltip>
                      </span>
                    </transition>
                    <button
                      v-if="!timelineNavigationControlsExpanded"
                      type="button"
                      class="timeline-navigation-activation-zone"
                      :aria-label="t('webSession.timelineNavigationReveal')"
                      :aria-expanded="timelineNavigationControlsExpanded"
                      @click="handleTimelineNavigationActivation"
                    ></button>
                  </span>
                  <n-tooltip
                    v-if="!timelineSearchOpen"
                    trigger="hover"
                    placement="bottom"
                    :delay="100"
                  >
                    <template #trigger>
                      <n-button
                        quaternary
                        circle
                        size="small"
                        :disabled="!currentSession"
                        :title="t('webSession.conversationSearchOpen')"
                        :aria-label="t('webSession.conversationSearchOpen')"
                        @click="openTimelineSearch"
                      >
                        <template #icon>
                          <n-icon><SearchOutline /></n-icon>
                        </template>
                      </n-button>
                    </template>
                    {{ t('webSession.conversationSearchOpen') }}
                  </n-tooltip>
                </div>
                <div v-if="historyMeta.loading" class="history-loading">
                  {{
                    currentRealSession?.syncState === 'syncing'
                      ? t('webSession.syncLoading')
                      : t('common.loading')
                  }}
                </div>

                <div
                  v-if="
                    visibleBlocks.length === 0 &&
                    filteredTimelineBlocks.length === 0 &&
                    !historyMeta.loading &&
                    currentRealSession?.syncState !== 'syncing'
                  "
                  class="timeline-intro"
                >
                  <span class="timeline-intro-badge">
                    {{ getAgentDisplayName(currentSession.agent) }}
                  </span>
                  <div class="timeline-intro-title">{{ t('webSession.readyTitle') }}</div>
                  <div class="timeline-intro-text">{{ t('webSession.readyDescription') }}</div>
                </div>

                <div
                  v-for="item in visibleBlocks"
                  :key="item.key"
                  :ref="element => setTimelineBlockRef(element, item)"
                  class="timeline-item"
                  :class="`kind-${item.kind}`"
                  :data-timeline-key="item.key"
                  :data-timeline-order-index="item.orderIndex"
                >
                  <div v-if="!shouldHideTimelineMeta(item)" class="item-meta">
                    <span v-if="item.kind === 'user'" class="user-message-navigation">
                      <n-tooltip
                        v-if="canEditTimelineUserMessage(item)"
                        trigger="hover"
                        placement="top"
                        :delay="100"
                      >
                        <template #trigger>
                          <n-button
                            quaternary
                            circle
                            size="small"
                            class="user-message-navigation-button user-message-edit-button"
                            :disabled="isRunActive"
                            :title="timelineUserMessageEditTitle"
                            :aria-label="timelineUserMessageEditTitle"
                            @click.stop="openTimelineUserMessageEdit(item)"
                          >
                            <template #icon>
                              <n-icon><CreateOutline /></n-icon>
                            </template>
                          </n-button>
                        </template>
                        {{ timelineUserMessageEditTitle }}
                      </n-tooltip>
                      <n-tooltip trigger="hover" placement="top" :delay="100">
                        <template #trigger>
                          <n-button
                            quaternary
                            circle
                            size="small"
                            class="user-message-navigation-button"
                            :loading="isTimelineUserMessageNavigationLoading(item, 'previous')"
                            :disabled="!canNavigateTimelineUserMessage(item, 'previous')"
                            :title="t('terminal.prevUserMessage')"
                            :aria-label="t('terminal.prevUserMessage')"
                            @click.stop="navigateTimelineUserMessage(item, 'previous')"
                          >
                            <template #icon>
                              <n-icon><ChevronUpOutline /></n-icon>
                            </template>
                          </n-button>
                        </template>
                        {{ t('terminal.prevUserMessage') }}
                      </n-tooltip>
                      <n-tooltip trigger="hover" placement="top" :delay="100">
                        <template #trigger>
                          <n-button
                            quaternary
                            circle
                            size="small"
                            class="user-message-navigation-button"
                            :loading="isTimelineUserMessageNavigationLoading(item, 'next')"
                            :disabled="!canNavigateTimelineUserMessage(item, 'next')"
                            :title="t('terminal.nextUserMessage')"
                            :aria-label="t('terminal.nextUserMessage')"
                            @click.stop="navigateTimelineUserMessage(item, 'next')"
                          >
                            <template #icon>
                              <n-icon><ChevronDownOutline /></n-icon>
                            </template>
                          </n-button>
                        </template>
                        {{ t('terminal.nextUserMessage') }}
                      </n-tooltip>
                    </span>
                    <span
                      class="item-role"
                      :class="{
                        'timeline-search-role-highlight': isTimelineSearchBlockMatch(item),
                        'timeline-search-role-highlight-active': isTimelineSearchBlockActive(item),
                      }"
                    >
                      {{ timelineRoleLabel(item) }}
                    </span>
                    <span class="item-time" :title="formatDateTime(item.timestamp)">{{
                      formatTime(item.timestamp)
                    }}</span>
                    <span v-if="item.runDurationMs != null" class="item-duration">
                      ·
                      {{
                        t('webSession.runDuration', {
                          duration: formatWorkDuration(item.runDurationMs),
                        })
                      }}
                    </span>
                  </div>

                  <div
                    v-if="item.kind === 'tool' && item.tool && isPlanTool(item.tool)"
                    class="timeline-tool-shell plan-tool-shell"
                  >
                    <div
                      class="tool-card timeline-tool-card is-plan-tool is-static-plan-tool"
                      :class="{
                        'is-raw-capable': shouldShowPlanRawToggle(item),
                        'is-raw-active': isTimelineRawBlockActive(item, 'plan'),
                      }"
                      :data-raw-toggle-card="
                        shouldShowPlanRawToggle(item)
                          ? getTimelineRawModeKey(item, 'plan')
                          : undefined
                      "
                      :tabindex="shouldShowPlanRawToggle(item) ? 0 : undefined"
                      @click="activateTimelineRawBlock(item, 'plan')"
                      @focusin="activateTimelineRawBlock(item, 'plan')"
                      @keydown.enter.self.prevent="activateTimelineRawBlock(item, 'plan')"
                      @keydown.space.self.prevent="activateTimelineRawBlock(item, 'plan')"
                    >
                      <button
                        v-if="shouldShowTimelineRawToggle(item, 'plan')"
                        type="button"
                        class="timeline-display-toggle timeline-display-toggle--copy"
                        title="copy"
                        @click.stop="copyTimelineBlock(item, 'plan')"
                      >
                        copy
                      </button>
                      <button
                        v-if="shouldShowTimelineRawToggle(item, 'plan')"
                        type="button"
                        class="timeline-display-toggle"
                        :class="{ 'is-active': isBlockRawMode(item, 'plan') }"
                        :title="t('terminal.rawMode')"
                        @click.stop="toggleBlockRawMode(item, 'plan')"
                      >
                        raw
                      </button>
                      <div class="tool-body plan-tool-body">
                        <div class="plan-tool-header">
                          <span
                            class="plan-tool-badge"
                            :class="{
                              'timeline-search-role-highlight': isTimelineSearchBlockMatch(item),
                              'timeline-search-role-highlight-active':
                                isTimelineSearchBlockActive(item),
                            }"
                          >
                            {{ t('webSession.planCardBadge') }}
                          </span>
                          <span class="plan-tool-caption">{{
                            t('webSession.planCardCaption')
                          }}</span>
                        </div>
                        <div v-if="item.tool.output" class="plan-tool-content">
                          <pre
                            v-if="isBlockRawMode(item, 'plan')"
                            class="timeline-raw-text plan-tool-content--raw"
                          ><code
                            v-html="
                              renderHighlightedPlainText(item.tool.output, timelineSearchQuery)
                            "
                          ></code></pre>
                          <div
                            v-else
                            class="chat-markdown"
                            v-html="
                              renderMarkdown(
                                getPlanToolMarkdownText(item),
                                getPlanToolMarkdownRenderOptions(item)
                              )
                            "
                          ></div>
                        </div>
                        <div v-if="showPlanActions(item.tool.id)" class="plan-tool-actions">
                          <div class="plan-tool-action-row">
                            <n-dropdown
                              trigger="manual"
                              placement="bottom-end"
                              :show="showPlanQuickActions"
                              :options="planQuickActionOptions"
                              :x="planQuickActionsX"
                              :y="planQuickActionsY"
                              @select="handlePlanQuickActionSelect"
                              @clickoutside="closePlanQuickActions"
                            />
                            <div class="plan-tool-action-split">
                              <n-button
                                size="small"
                                type="primary"
                                class="plan-tool-action-primary"
                                :loading="isSubmittingPlanExecution"
                                :disabled="isSubmittingMessage"
                                @click="handlePlanCardImplement"
                              >
                                {{ t('webSession.planActionImplement') }}
                              </n-button>
                              <button
                                type="button"
                                class="plan-tool-action-menu-btn"
                                :title="t('webSession.planActionMenu')"
                                :aria-label="t('webSession.planActionMenu')"
                                :aria-expanded="showPlanQuickActions"
                                :disabled="isSubmittingMessage"
                                @click="handlePlanQuickActionTriggerClick"
                              >
                                <n-icon size="14"><ChevronDownOutline /></n-icon>
                              </button>
                            </div>
                            <n-button
                              size="small"
                              secondary
                              class="plan-tool-action-secondary"
                              :disabled="isSubmittingMessage"
                              @click="handlePlanCardCancel"
                            >
                              {{ t('webSession.planActionCancel') }}
                            </n-button>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div
                    v-else-if="shouldRenderActivityDisplayRow(item)"
                    class="timeline-activity-display-shell"
                    :class="`mode-${effectiveWebSessionActivityDisplayMode}`"
                  >
                    <button
                      type="button"
                      class="timeline-activity-display-row"
                      :class="[
                        `mode-${effectiveWebSessionActivityDisplayMode}`,
                        item.tool ? `state-${item.tool.status}` : '',
                      ]"
                      :title="getActivityDisplayTitle(item)"
                      @click="handleActivityDisplayClick(item)"
                    >
                      <span class="activity-display-main">
                        <span
                          class="activity-display-label"
                          :class="{
                            'timeline-search-role-highlight': isTimelineSearchBlockMatch(item),
                            'timeline-search-role-highlight-active':
                              isTimelineSearchBlockActive(item),
                          }"
                        >
                          {{ activityDisplayLabel(item) }}
                        </span>
                        <span class="activity-display-time" :title="formatDateTime(item.timestamp)">
                          {{ formatTime(item.timestamp) }}
                        </span>
                        <span
                          v-if="getActivityDisplayCount(item) > 1"
                          class="activity-display-count"
                        >
                          x{{ getActivityDisplayCount(item) }}
                        </span>
                        <span class="activity-display-summary">
                          {{ activityDisplaySummary(item) }}
                        </span>
                      </span>
                      <span
                        v-if="item.tool && effectiveWebSessionActivityDisplayMode === 'card'"
                        class="tool-state-badge activity-display-state"
                        :class="`state-${item.tool.status}`"
                      >
                        <span class="tool-state-dot"></span>
                        {{ toolStateLabel(item.tool) }}
                      </span>
                    </button>
                    <div
                      v-if="item.tool && !isCompactTool(item.tool) && isToolExpanded(item.tool.id)"
                      class="activity-display-expanded tool-body"
                    >
                      <div v-if="item.tool.input" class="tool-section">
                        <div class="tool-section-label">{{ t('webSession.toolInput') }}</div>
                        <pre class="tool-code">{{ stringifyValue(item.tool.input) }}</pre>
                      </div>
                      <div v-if="item.tool.output" class="tool-section">
                        <div class="tool-section-label">{{ t('webSession.toolOutput') }}</div>
                        <pre class="tool-code">{{ item.tool.output }}</pre>
                      </div>
                      <div
                        v-else-if="shouldShowToolPendingPlaceholder(item.tool)"
                        class="tool-section"
                      >
                        <div class="tool-section-label">{{ t('webSession.toolOutput') }}</div>
                        <pre class="tool-code">{{ t('common.loading') }}</pre>
                      </div>
                    </div>
                  </div>

                  <div v-else-if="item.kind === 'tool' && item.tool" class="timeline-tool-shell">
                    <div
                      v-if="isCompactTool(item.tool)"
                      class="tool-card timeline-tool-card command-tool-card"
                      :class="`state-${item.tool.status}`"
                    >
                      <button
                        type="button"
                        class="command-tool-button"
                        @click="openCommandExecutionDetail(item)"
                      >
                        <span class="command-tool-copy">
                          <span class="command-tool-topline">
                            <span
                              class="command-tool-label"
                              :class="{
                                'timeline-search-role-highlight': isTimelineSearchBlockMatch(item),
                                'timeline-search-role-highlight-active':
                                  isTimelineSearchBlockActive(item),
                              }"
                            >
                              {{ compactToolLabel(item.tool) }}
                            </span>
                            <span
                              v-if="getCompactToolCount(item.tool) > 1"
                              class="command-tool-count"
                            >
                              x{{ getCompactToolCount(item.tool) }}
                            </span>
                            <span class="command-tool-time" :title="formatDateTime(item.timestamp)">
                              {{ formatTime(item.timestamp) }}
                            </span>
                          </span>
                          <span
                            class="command-tool-command"
                            :title="getCompactToolSummary(item.tool)"
                          >
                            {{ getCompactToolDisplaySummary(item.tool) }}
                          </span>
                        </span>
                        <span class="tool-state-badge" :class="`state-${item.tool.status}`">
                          <span class="tool-state-dot"></span>
                          {{ toolStateLabel(item.tool) }}
                        </span>
                      </button>
                    </div>

                    <div
                      v-else
                      class="tool-card timeline-tool-card"
                      :class="toolCardClass(item.tool)"
                    >
                      <button
                        type="button"
                        class="tool-header"
                        @click="toggleToolExpanded(item.tool)"
                      >
                        <span class="tool-header-main">
                          <span class="tool-header-leading">
                            <span
                              class="tool-kind"
                              :class="{
                                'timeline-search-role-highlight': isTimelineSearchBlockMatch(item),
                                'timeline-search-role-highlight-active':
                                  isTimelineSearchBlockActive(item),
                              }"
                            >
                              {{ toolKindLabel(item.tool) }}
                            </span>
                            <span class="tool-name">{{ item.tool.name }}</span>
                          </span>
                          <span class="tool-state-badge" :class="`state-${item.tool.status}`">
                            <span class="tool-state-dot"></span>
                            {{ toolStateLabel(item.tool) }}
                          </span>
                        </span>
                        <span v-if="formatToolPreview(item.tool)" class="tool-preview">{{
                          formatToolPreview(item.tool)
                        }}</span>
                      </button>
                      <div v-if="isToolExpanded(item.tool.id)" class="tool-body">
                        <div v-if="isImageViewTool(item.tool)" class="tool-section">
                          <div class="tool-section-label">
                            {{ t('webSession.imageViewPreview') }}
                          </div>
                          <div class="image-view-preview-card">
                            <div class="image-view-preview-meta">
                              <span class="image-view-preview-name">
                                <n-icon size="14"><ImageOutline /></n-icon>
                                <span>{{ getImageViewDisplayName(item.tool) }}</span>
                              </span>
                              <span
                                v-if="getImageViewDisplayPath(item.tool)"
                                class="image-view-preview-path"
                                :title="getImageViewDisplayPath(item.tool)"
                              >
                                {{ getImageViewDisplayPath(item.tool) }}
                              </span>
                            </div>
                            <div class="image-view-preview-frame">
                              <div
                                v-if="getImageViewPreviewState(item.tool) !== 'ready'"
                                class="image-view-preview-status"
                                :class="{
                                  'is-error': getImageViewPreviewState(item.tool) === 'error',
                                }"
                              >
                                {{
                                  getImageViewPreviewState(item.tool) === 'error'
                                    ? t('webSession.imageViewLoadFailed')
                                    : t('webSession.imageViewLoading')
                                }}
                              </div>
                              <img
                                v-if="
                                  getImageViewPreviewSrc(item.tool) &&
                                  getImageViewPreviewState(item.tool) !== 'error'
                                "
                                :src="getImageViewPreviewSrc(item.tool)"
                                :alt="getImageViewDisplayName(item.tool)"
                                class="image-view-preview-image"
                                :class="{
                                  'is-ready': getImageViewPreviewState(item.tool) === 'ready',
                                }"
                                loading="lazy"
                                @load="handleImageViewPreviewLoad(item.tool.id)"
                                @error="handleImageViewPreviewError(item.tool.id)"
                              />
                            </div>
                          </div>
                        </div>
                        <div v-if="item.tool.input" class="tool-section">
                          <div class="tool-section-label">{{ t('webSession.toolInput') }}</div>
                          <pre class="tool-code">{{ stringifyValue(item.tool.input) }}</pre>
                        </div>
                        <div v-if="item.tool.output" class="tool-section">
                          <div class="tool-section-label">{{ t('webSession.toolOutput') }}</div>
                          <pre class="tool-code">{{ item.tool.output }}</pre>
                        </div>
                        <div
                          v-else-if="shouldShowToolPendingPlaceholder(item.tool)"
                          class="tool-section"
                        >
                          <div class="tool-section-label">{{ t('webSession.toolOutput') }}</div>
                          <pre class="tool-code">{{ t('common.loading') }}</pre>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div
                    v-else-if="item.kind === 'system' && item.detail"
                    class="timeline-history-card-shell"
                  >
                    <div
                      class="approval-card history-interaction-card"
                      :class="historyInteractionCardClass(item)"
                    >
                      <div class="approval-card-header">
                        <span
                          class="approval-badge"
                          :class="[
                            historyInteractionBadgeClass(item),
                            {
                              'timeline-search-role-highlight': isTimelineSearchBlockMatch(item),
                              'timeline-search-role-highlight-active':
                                isTimelineSearchBlockActive(item),
                            },
                          ]"
                        >
                          {{ historyInteractionTitle(item) }}
                        </span>
                        <span class="approval-time" :title="formatDateTime(item.timestamp)">
                          {{ formatTime(item.timestamp) }}
                        </span>
                      </div>

                      <div
                        v-if="historyInteractionPrompt(item)"
                        class="approval-prompt history-interaction-prompt"
                      >
                        {{ historyInteractionPrompt(item) }}
                      </div>

                      <div
                        v-if="item.detail.questions?.length"
                        class="history-question-list user-input-card"
                      >
                        <div
                          v-for="question in item.detail.questions"
                          :key="`${item.id}:${question.id}`"
                          class="user-input-question history-question-card"
                        >
                          <div class="user-input-question-header">
                            {{ historyQuestionTitle(question) }}
                          </div>
                          <div
                            v-if="
                              question.header &&
                              question.question &&
                              question.header !== question.question
                            "
                            class="user-input-question-copy"
                          >
                            {{ question.question }}
                          </div>
                          <div v-if="question.options.length > 0" class="history-option-list">
                            <div
                              v-for="option in question.options"
                              :key="`${question.id}:${option.label}`"
                              class="history-option-row"
                            >
                              <div class="history-option-label">{{ option.label }}</div>
                              <div v-if="option.description" class="history-option-description">
                                {{ option.description }}
                              </div>
                            </div>
                          </div>
                          <div
                            v-if="question.isOther || question.options.length === 0"
                            class="history-question-note"
                          >
                            {{
                              question.isSecret
                                ? t('webSession.historySecretInput')
                                : t('webSession.historyFreeformInput')
                            }}
                          </div>
                        </div>
                      </div>

                      <div
                        v-if="item.detail.answers?.length"
                        class="history-answer-list user-input-card"
                      >
                        <div
                          v-for="answer in item.detail.answers"
                          :key="`${item.id}:${answer.id}`"
                          class="user-input-question history-answer-card"
                        >
                          <div class="user-input-question-header">{{ answer.label }}</div>
                          <div class="history-answer-values">
                            <span
                              v-for="value in formatHistoryAnswerValues(answer)"
                              :key="`${answer.id}:${value}`"
                              class="history-answer-chip"
                            >
                              {{ value }}
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div v-else class="timeline-message-row">
                    <n-tooltip
                      v-if="shouldShowTimelineUserMessageDeliveryIndicator(item)"
                      trigger="hover"
                      placement="left"
                      :delay="100"
                    >
                      <template #trigger>
                        <button
                          type="button"
                          class="user-message-failure-indicator"
                          :class="{ 'is-loading': isRetryingTimelineUserMessage(item) }"
                          :aria-label="failedTimelineUserMessageActionLabel(item)"
                          :aria-busy="isRetryingTimelineUserMessage(item)"
                          @click.stop="handleRetryTimelineUserMessage(item)"
                        >
                          <n-icon size="23">
                            <RefreshOutline v-if="isRetryingTimelineUserMessage(item)" />
                            <AlertCircleOutline v-else />
                          </n-icon>
                        </button>
                      </template>
                      {{ failedTimelineUserMessageActionLabel(item) }}
                    </n-tooltip>

                    <div
                      class="item-bubble"
                      :class="[
                        item.level ? `level-${item.level}` : undefined,
                        item.itemType ? `type-${item.itemType}` : undefined,
                        {
                          'is-raw-capable': shouldShowMessageRawToggle(item),
                          'is-raw-active': isTimelineRawBlockActive(item, 'message'),
                        },
                      ]"
                      :data-raw-toggle-card="
                        shouldShowMessageRawToggle(item)
                          ? getTimelineRawModeKey(item, 'message')
                          : undefined
                      "
                      :tabindex="shouldShowMessageRawToggle(item) ? 0 : undefined"
                      @mouseenter="handleMessageBubbleMouseEnter(item)"
                      @mouseleave="handleMessageBubbleMouseLeave(item)"
                      @click="handleMessageBubbleClick(item)"
                      @focusin="activateTimelineRawBlock(item, 'message')"
                      @focusout="handleMessageBubbleFocusOut(item, $event)"
                      @keydown.enter.self.prevent="activateTimelineRawBlock(item, 'message')"
                      @keydown.space.self.prevent="activateTimelineRawBlock(item, 'message')"
                    >
                      <button
                        v-if="shouldShowTimelineRawToggle(item, 'message')"
                        type="button"
                        class="timeline-display-toggle timeline-display-toggle--copy"
                        title="copy"
                        @click.stop="copyTimelineBlock(item, 'message')"
                      >
                        copy
                      </button>
                      <button
                        v-if="shouldShowTimelineRawToggle(item, 'message')"
                        type="button"
                        class="timeline-display-toggle"
                        :class="{ 'is-active': isBlockRawMode(item, 'message') }"
                        :title="t('terminal.rawMode')"
                        @click.stop="toggleBlockRawMode(item, 'message')"
                      >
                        raw
                      </button>
                      <pre
                        v-if="shouldShowMessageRawToggle(item) && isBlockRawMode(item, 'message')"
                        class="item-text item-text--raw timeline-raw-text"
                      ><code v-html="renderHighlightedPlainText(item.text, timelineSearchQuery)"></code></pre>
                      <div
                        v-else-if="getDisplayBlockText(item)"
                        class="item-text chat-markdown"
                        v-html="
                          renderMarkdown(
                            getMessageMarkdownText(item),
                            getMessageMarkdownRenderOptions(item)
                          )
                        "
                      ></div>
                      <div v-if="item.attachments.length > 0" class="attachment-row">
                        <span
                          v-for="attachment in item.attachments"
                          :key="attachment.id"
                          class="attachment-pill"
                        >
                          <n-popover
                            v-if="shouldUseAttachmentHoverPreview(attachment)"
                            trigger="hover"
                            placement="bottom-start"
                            :delay="120"
                          >
                            <template #trigger>
                              <button
                                type="button"
                                class="attachment-preview-trigger"
                                :title="attachment.name"
                                @click="openAttachmentPreview(attachment)"
                              >
                                <span class="attachment-preview-trigger-text">{{
                                  attachment.name
                                }}</span>
                              </button>
                            </template>
                            <div class="attachment-hover-preview">
                              <img
                                :src="getAttachmentPreviewUrl(attachment.id)"
                                :alt="attachment.name"
                                class="attachment-hover-image"
                                loading="lazy"
                              />
                            </div>
                          </n-popover>
                          <button
                            v-else-if="shouldUseAttachmentModalPreview(attachment)"
                            type="button"
                            class="attachment-preview-trigger"
                            :title="attachment.name"
                            @click="openAttachmentPreview(attachment)"
                          >
                            <span class="attachment-preview-trigger-text">{{
                              attachment.name
                            }}</span>
                          </button>
                          <button
                            v-else
                            type="button"
                            class="attachment-preview-trigger is-static"
                            :title="attachment.name"
                          >
                            <span class="attachment-preview-trigger-text">{{
                              attachment.name
                            }}</span>
                          </button>
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div v-if="showRuntimeStrip" class="runtime-strip">
                  <button
                    type="button"
                    class="live-card"
                    :class="`phase-${displayLiveState.phase}`"
                    :aria-label="liveCardAriaLabel"
                    @click="handleLiveCardClick"
                  >
                    <div class="live-card-main">
                      <span class="live-orb"></span>
                      <div class="live-copy">
                        <div class="live-title">{{ liveStateLabel }}</div>
                        <div
                          class="live-detail"
                          :class="{ 'is-placeholder': !liveStateDetail }"
                          :title="liveStateDetailTitle"
                        >
                          {{ liveStateSecondaryText }}
                        </div>
                      </div>
                    </div>
                    <div class="live-meta">
                      <span v-if="liveStateWorking" class="live-activity" aria-hidden="true">
                        <span class="live-activity-bar"></span>
                        <span class="live-activity-bar"></span>
                        <span class="live-activity-bar"></span>
                      </span>
                      <n-tooltip placement="top-end" :delay="120">
                        <template #trigger>
                          <span class="live-time">{{ getLiveTimeText(displayLiveState) }}</span>
                        </template>
                        <div class="live-time-tooltip">
                          <div
                            v-for="item in getLiveTimeTooltipItems(displayLiveState)"
                            :key="item.key"
                            class="live-time-tooltip-row"
                          >
                            <span class="live-time-tooltip-label">{{ item.label }}</span>
                            <span class="live-time-tooltip-value">{{ item.value }}</span>
                          </div>
                        </div>
                      </n-tooltip>
                    </div>
                  </button>

                  <div
                    v-if="pendingApproval"
                    class="approval-card"
                    :class="{ 'is-stale': pendingApproval.stale || !pendingApproval.actionable }"
                  >
                    <div class="approval-card-header">
                      <span class="approval-badge">{{ t('webSession.approvalTitle') }}</span>
                      <span
                        class="approval-time"
                        :title="formatDateTime(pendingApproval.requestedAt)"
                        >{{ formatTime(pendingApproval.requestedAt) }}</span
                      >
                    </div>
                    <div class="approval-prompt">
                      {{ pendingApproval.prompt || t('webSession.approvalPromptFallback') }}
                    </div>
                    <pre v-if="pendingApproval.command" class="approval-command">{{
                      pendingApproval.command
                    }}</pre>
                    <div
                      v-if="pendingApproval.stale || !pendingApproval.actionable"
                      class="approval-note"
                    >
                      {{
                        pendingApproval.recoveryMessage ||
                        t('webSession.approvalDetailsUnavailable')
                      }}
                    </div>
                    <div class="approval-actions">
                      <n-button
                        size="small"
                        type="primary"
                        :disabled="pendingApproval.stale || !pendingApproval.actionable"
                        @click="handleApproval('approve')"
                      >
                        {{ t('webSession.approvalApprove') }}
                      </n-button>
                      <n-button
                        size="small"
                        secondary
                        :disabled="pendingApproval.stale || !pendingApproval.actionable"
                        @click="handleApproval('reject')"
                      >
                        {{ t('webSession.approvalReject') }}
                      </n-button>
                      <n-button size="small" tertiary @click="handleAbortCurrent">
                        {{ t('webSession.stop') }}
                      </n-button>
                    </div>
                  </div>

                  <div v-else-if="approvalDetailsMissing" class="approval-card is-stale">
                    <div class="approval-card-header">
                      <span class="approval-badge">{{ t('webSession.approvalTitle') }}</span>
                    </div>
                    <div class="approval-prompt">
                      {{
                        approvalDetailsLoading
                          ? t('webSession.approvalDetailsLoading')
                          : t('webSession.approvalDetailsUnavailable')
                      }}
                    </div>
                    <div class="approval-actions">
                      <n-button
                        size="small"
                        secondary
                        :loading="approvalDetailsLoading"
                        @click="handleRecoverApprovalDetails"
                      >
                        <template #icon
                          ><n-icon><RefreshOutline /></n-icon
                        ></template>
                        {{ t('webSession.approvalDetailsRefresh') }}
                      </n-button>
                      <n-button size="small" tertiary @click="handleAbortCurrent">
                        {{ t('webSession.stop') }}
                      </n-button>
                    </div>
                  </div>

                  <div
                    v-else-if="pendingUserInput && !inlinePlanChoice"
                    :key="pendingUserInput.itemId"
                    class="approval-card user-input-card"
                    :class="{ 'is-stale': pendingUserInput.stale }"
                  >
                    <div class="approval-card-header">
                      <span class="approval-badge">{{ t('webSession.userInputTitle') }}</span>
                      <span
                        class="approval-time"
                        :title="formatDateTime(pendingUserInput.requestedAt)"
                        >{{ formatTime(pendingUserInput.requestedAt) }}</span
                      >
                    </div>
                    <div class="approval-prompt">
                      {{ pendingUserInput.prompt || t('webSession.userInputPromptFallback') }}
                    </div>
                    <div v-if="pendingUserInput.stale" class="approval-note">
                      {{ pendingUserInput.recoveryMessage || t('webSession.recoveredRuntimeHint') }}
                    </div>
                    <div
                      v-for="question in pendingUserInput.questions"
                      :key="question.id"
                      v-memo="userInputQuestionMemoDeps(question)"
                      class="user-input-question"
                    >
                      <div class="user-input-question-header">
                        {{ question.header || question.question }}
                      </div>
                      <div
                        v-if="
                          question.header &&
                          question.question &&
                          question.header !== question.question
                        "
                        class="user-input-question-copy"
                      >
                        {{ question.question }}
                      </div>
                      <n-checkbox-group
                        v-if="question.options.length > 0 && question.multiSelect"
                        v-model:value="userInputSelections[question.id]"
                        :disabled="isUserInputInteractionDisabled"
                        class="user-input-options"
                      >
                        <div
                          v-for="option in question.options"
                          :key="`${question.id}:${option.label}`"
                          :class="{
                            'is-selected': userInputSelections[question.id]?.includes(option.label),
                            'is-disabled': isUserInputInteractionDisabled,
                          }"
                          class="user-input-option"
                        >
                          <n-checkbox :value="option.label">
                            <span class="user-input-option-label">{{ option.label }}</span>
                          </n-checkbox>
                          <div v-if="option.description" class="user-input-option-description">
                            {{ option.description }}
                          </div>
                        </div>
                      </n-checkbox-group>
                      <n-radio-group
                        v-else-if="question.options.length > 0"
                        :value="userInputSelections[question.id]?.[0] || null"
                        :disabled="isUserInputInteractionDisabled"
                        class="user-input-options"
                        @update:value="handleUserInputSingleSelect(question.id, $event)"
                      >
                        <div
                          v-for="option in question.options"
                          :key="`${question.id}:${option.label}`"
                          :class="{
                            'is-selected': userInputSelections[question.id]?.includes(option.label),
                            'is-disabled': isUserInputInteractionDisabled,
                          }"
                          class="user-input-option"
                        >
                          <n-radio :value="option.label">
                            <span class="user-input-option-label">{{ option.label }}</span>
                          </n-radio>
                          <div v-if="option.description" class="user-input-option-description">
                            {{ option.description }}
                          </div>
                        </div>
                      </n-radio-group>
                      <n-input
                        v-if="question.isOther || question.options.length === 0"
                        v-model:value="userInputDrafts[question.id]"
                        :type="question.isSecret ? 'password' : 'text'"
                        size="small"
                        :disabled="isUserInputInteractionDisabled"
                        :show-password-on="question.isSecret ? 'mousedown' : undefined"
                        :placeholder="userInputPlaceholder(question)"
                        @keydown="handleUserInputEnter"
                      />
                    </div>
                    <div class="approval-actions">
                      <n-button
                        size="small"
                        type="primary"
                        :loading="isSubmittingUserInput"
                        :disabled="isUserInputInteractionDisabled"
                        @click="handleUserInputSubmit"
                      >
                        {{ t('webSession.userInputSubmit') }}
                      </n-button>
                      <n-button
                        size="small"
                        tertiary
                        :disabled="isUserInputInteractionDisabled"
                        @click="handleAbortCurrent"
                      >
                        {{ t('webSession.stop') }}
                      </n-button>
                    </div>
                    <div
                      v-if="showUserInputSubmitSlowHint"
                      class="approval-note"
                      aria-live="polite"
                    >
                      {{ t('webSession.userInputSubmitSlow') }}
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <n-alert
              v-if="showCyberPolicyWarning"
              class="cyber-policy-alert"
              type="warning"
              :bordered="false"
              :theme-overrides="cyberPolicyAlertThemeOverrides"
              closable
              @close="dismissCyberPolicyWarning"
            >
              {{ t('webSession.cyberPolicyFlagged') }}
            </n-alert>
          </div>

          <div v-else-if="!currentSession" class="empty-state">
            <n-empty :description="emptyStateDescription" />
          </div>

          <div
            v-if="isGoalCardVisible"
            class="goal-card"
            :class="currentSessionGoal ? `status-${currentSessionGoal.status}` : 'status-empty'"
          >
            <div class="goal-card-header">
              <div class="goal-card-title-row">
                <span class="goal-badge">Goal</span>
                <span v-if="currentSessionGoal" class="goal-status-badge">
                  {{ currentSessionGoal.status }}
                </span>
              </div>
              <div class="goal-card-actions">
                <n-button
                  size="small"
                  tertiary
                  :disabled="isCurrentSessionGoalModeBlocked"
                  @click="handleGoalCompose"
                >
                  Edit
                </n-button>
                <n-button
                  v-if="currentSessionGoal?.status === 'active'"
                  size="small"
                  tertiary
                  :disabled="isCurrentSessionGoalModeBlocked"
                  @click="handleGoalPause"
                >
                  Pause
                </n-button>
                <n-button
                  v-else-if="currentSessionGoal"
                  size="small"
                  tertiary
                  :disabled="isCurrentSessionGoalModeBlocked"
                  @click="handleGoalResume"
                >
                  Resume
                </n-button>
                <n-button
                  v-if="currentSessionGoal"
                  size="small"
                  tertiary
                  :disabled="isCurrentSessionGoalModeBlocked"
                  @click="handleGoalClear"
                >
                  Clear
                </n-button>
              </div>
            </div>
            <div v-if="currentSessionGoal" class="goal-card-body">
              <div class="goal-objective">{{ currentSessionGoal.objective }}</div>
              <div class="goal-meta-row">
                <span>Used {{ currentSessionGoal.tokensUsed }} tokens</span>
                <span v-if="currentSessionGoal.tokenBudget != null">
                  Budget {{ currentSessionGoal.tokenBudget }}
                </span>
                <span>{{ formatGoalDuration(currentSessionGoal.timeUsedSeconds) }}</span>
                <span :title="formatIsoDateTime(currentSessionGoal.updatedAt)">
                  {{ formatIsoTime(currentSessionGoal.updatedAt) }}
                </span>
              </div>
            </div>
            <div v-else class="goal-empty">
              {{ 'Persistent Codex thread goal is not set for this session.' }}
            </div>
            <div v-if="isCurrentSessionGoalModeBlocked" class="goal-empty">
              {{ goalModeUnavailableMessage() }}
            </div>
          </div>

          <div
            class="composer"
            :class="{
              'is-drag-over': isComposerDragOver,
              'is-mobile': isMobile,
              'is-mobile-focused': isMobileComposerFocused,
              'is-mobile-settings-expanded': isMobile && isMobileComposerSettingsExpanded,
            }"
            @paste.capture="handleComposerPaste"
            @dragenter="handleComposerDragEnter"
            @dragover="handleComposerDragOver"
            @dragleave="handleComposerDragLeave"
            @drop="handleComposerDrop"
          >
            <input
              ref="fileInputRef"
              type="file"
              accept="image/*"
              multiple
              class="hidden-file-input"
              @change="handleFileChange"
            />

            <div
              v-if="isMobile"
              class="composer-mobile-toolbar"
              :class="{
                'is-collapsed': isMobileComposerCollapsed,
                'is-settings-expanded': isMobileComposerSettingsExpanded,
              }"
            >
              <div
                v-if="!isMobileComposerCollapsed"
                class="composer-mobile-summary"
                :class="{ 'is-expanded': isMobileComposerSettingsExpanded }"
              >
                <button
                  type="button"
                  class="composer-mobile-toggle"
                  :class="{ 'is-compact': isMobileComposerSettingsExpanded }"
                  :aria-expanded="isMobileComposerSettingsExpanded"
                  :title="mobileComposerSettingsToggleLabel"
                  :aria-label="mobileComposerSettingsToggleLabel"
                  @click="toggleMobileComposerSettingsExpanded"
                >
                  <span
                    v-if="!isMobileComposerSettingsExpanded"
                    class="composer-mobile-toggle-copy"
                  >
                    <span class="composer-mobile-toggle-chips">
                      <span
                        v-for="token in mobileComposerSummaryTokens"
                        :key="token.key"
                        class="composer-mobile-toggle-chip"
                      >
                        {{ token.label }}
                      </span>
                    </span>
                  </span>
                  <n-icon
                    class="composer-mobile-toggle-arrow"
                    :class="{ 'is-open': isMobileComposerSettingsExpanded }"
                  >
                    <ChevronDownOutline />
                  </n-icon>
                </button>
              </div>

              <div
                v-if="isMobileComposerCollapsed && mobileComposerPendingSummary.length > 0"
                class="composer-mobile-pending-summary"
              >
                <span
                  v-for="item in mobileComposerPendingSummary"
                  :key="item.kind"
                  class="composer-mobile-pending-chip"
                  :class="`mode-${item.kind}`"
                >
                  {{ item.label }}
                </span>
              </div>

              <button
                type="button"
                class="composer-mobile-panel-toggle"
                :class="{ 'is-collapsed': isMobileComposerCollapsed }"
                :aria-expanded="!isMobileComposerCollapsed"
                :title="mobileComposerPanelToggleLabel"
                :aria-label="mobileComposerPanelToggleLabel"
                @click="toggleMobileComposerCollapsed"
              >
                <n-icon
                  class="composer-mobile-panel-toggle-arrow"
                  :class="{ 'is-collapsed': isMobileComposerCollapsed }"
                >
                  <ChevronDownOutline />
                </n-icon>
              </button>
            </div>

            <template v-if="!isMobile || !isMobileComposerCollapsed">
              <div
                v-if="!isMobile || isMobileComposerSettingsExpanded"
                class="composer-config"
                :class="{ 'is-mobile': isMobile }"
              >
                <div class="composer-config-row">
                  <n-dropdown
                    :trigger="isMobile ? 'click' : 'hover'"
                    placement="top-start"
                    :options="agentDropdownOptions"
                    :render-label="renderAgentDropdownLabel"
                    @select="handleAgentDropdownSelect"
                  >
                    <button
                      type="button"
                      class="composer-agent-trigger"
                      :title="selectedAgentTitle"
                      :aria-label="selectedAgentTitle"
                    >
                      <span class="composer-agent-trigger-icon" v-html="selectedAgentIcon"></span>
                      <n-icon class="composer-agent-trigger-arrow">
                        <ChevronDownOutline />
                      </n-icon>
                    </button>
                  </n-dropdown>
                  <n-select
                    :show="showModelSelector"
                    v-model:value="selectedModel"
                    @update:show="handleModelSelectorShowChange"
                    @mouseenter="handleComposerSelectorPointerEnter('model')"
                    @mouseleave="handleComposerSelectorPointerLeave('model')"
                    class="composer-select model-select"
                    :style="modelSelectStyle"
                    :menu-props="modelSelectMenuProps"
                    :render-option="renderModelOption"
                    size="small"
                    :options="modelOptions"
                  />
                  <n-select
                    v-if="selectedAgent === 'claude'"
                    v-model:value="selectedClaudeRuntime"
                    class="composer-select claude-runtime-select"
                    size="small"
                    :menu-props="claudeRuntimeSelectMenuProps"
                    :render-option="renderModelOption"
                    :options="claudeRuntimeOptions"
                  />
                  <n-select
                    v-if="selectedAgent === 'codex' || selectedAgent === 'pi'"
                    :show="isMobile ? undefined : showReasoningSelector"
                    v-model:value="selectedReasoningEffort"
                    @update:show="handleReasoningSelectorShowChange"
                    @mouseenter="handleComposerSelectorPointerEnter('reasoning')"
                    @mouseleave="handleComposerSelectorPointerLeave('reasoning')"
                    class="composer-select reasoning-select"
                    size="small"
                    :menu-props="reasoningSelectMenuProps"
                    :options="reasoningEffortOptions"
                  />
                  <div class="composer-mode-row">
                    <n-button-group class="composer-mode-switch">
                      <n-button
                        size="small"
                        :type="selectedWorkflowMode === 'default' ? 'primary' : 'default'"
                        @click="setWorkflowMode('default')"
                      >
                        {{ t('webSession.workflowDefault') }}
                      </n-button>
                      <n-button
                        size="small"
                        :type="selectedWorkflowMode === 'plan' ? 'primary' : 'default'"
                        @click="setWorkflowMode('plan')"
                      >
                        {{ t('webSession.workflowPlan') }}
                      </n-button>
                    </n-button-group>
                    <n-select
                      v-model:value="selectedPermissionLevel"
                      class="composer-select permission-select"
                      size="small"
                      :disabled="selectedAgent === 'pi'"
                      :options="permissionLevelOptions"
                    />
                  </div>
                  <div v-if="currentSession" class="composer-path" :title="currentSession.cwd">
                    {{ currentSession.cwd }}
                  </div>
                  <div class="composer-settings">
                    <button
                      v-if="selectedAgent === 'codex'"
                      type="button"
                      class="composer-settings-trigger"
                      :class="{
                        'has-active-settings': isGoalCardVisible,
                        'is-active': isGoalCardVisible,
                        'has-goal': Boolean(currentRealSession?.goal),
                      }"
                      :title="
                        selectedAgent === 'codex'
                          ? isCurrentDraftCodexSession
                            ? 'Insert /goal'
                            : 'Toggle goal card'
                          : 'Goal is only available for Codex'
                      "
                      :aria-label="
                        selectedAgent === 'codex'
                          ? isCurrentDraftCodexSession
                            ? 'Insert /goal'
                            : 'Toggle goal card'
                          : 'Goal is only available for Codex'
                      "
                      :disabled="selectedAgent !== 'codex' || !currentSession"
                      @click="toggleGoalCard"
                    >
                      <n-icon size="18"><IconGoal /></n-icon>
                    </button>
                    <n-popover
                      v-if="hasKnownSubAgents"
                      trigger="click"
                      placement="top-end"
                      :show-arrow="true"
                    >
                      <template #trigger>
                        <button
                          type="button"
                          class="composer-sub-agent-trigger"
                          :class="{ 'is-idle': !hasActiveSubAgents }"
                          :title="subAgentTriggerTitle"
                          :aria-label="subAgentTriggerTitle"
                        >
                          <span
                            v-if="hasActiveSubAgents"
                            class="composer-sub-agent-trigger-pulse"
                          ></span>
                          <span class="composer-sub-agent-trigger-count">{{
                            hasActiveSubAgents ? activeSubAgentCount : knownSubAgents.length
                          }}</span>
                        </button>
                      </template>
                      <div class="live-sub-agent-popover">
                        <div class="live-sub-agent-popover-title">
                          {{
                            t('webSession.subAgentPopoverTitle', {
                              count: knownSubAgents.length,
                              active: activeSubAgentCount,
                            })
                          }}
                        </div>
                        <div class="live-sub-agent-list">
                          <button
                            v-for="agent in knownSubAgents"
                            :key="agent.id"
                            type="button"
                            class="live-sub-agent-item"
                            :class="{
                              'is-active':
                                agent.status === 'pending_init' || agent.status === 'running',
                            }"
                            :aria-label="`${t('webSession.subAgentLocate')}: ${agent.title}`"
                            @click="locateSubAgent(agent)"
                          >
                            <span class="live-sub-agent-dot"></span>
                            <div class="live-sub-agent-copy">
                              <div class="live-sub-agent-title" :title="agent.title">
                                {{ agent.title }}
                                <span class="live-sub-agent-status">{{
                                  subAgentStatusLabel(agent.status)
                                }}</span>
                              </div>
                              <div
                                class="live-sub-agent-summary"
                                :title="
                                  subAgentLatestActivitySummary(agent) ||
                                  subAgentFallbackSummary(agent)
                                "
                              >
                                {{
                                  subAgentLatestActivitySummary(agent) ||
                                  subAgentFallbackSummary(agent)
                                }}
                              </div>
                            </div>
                          </button>
                        </div>
                      </div>
                    </n-popover>
                    <n-popover
                      trigger="click"
                      placement="top-end"
                      :show-arrow="true"
                      @update:show="handleComposerSettingsPopoverShow"
                    >
                      <template #trigger>
                        <button
                          type="button"
                          class="composer-settings-trigger"
                          :class="{ 'has-active-settings': composerSettingsHasActiveItems }"
                          :title="t('webSession.composerSettings')"
                          :aria-label="t('webSession.composerSettings')"
                        >
                          <n-icon size="16"><SettingsOutline /></n-icon>
                        </button>
                      </template>
                      <div class="composer-settings-popover-card">
                        <div class="composer-settings-popover-title">
                          {{ t('webSession.composerSettings') }}
                        </div>
                        <n-checkbox
                          v-model:checked="webSessionAutoContinueEnabledValue"
                          size="small"
                        >
                          {{ t('webSession.infiniteRetry') }}
                        </n-checkbox>
                        <n-checkbox
                          v-model:checked="webSessionAutoRetryDispatchPendingOnFailureValue"
                          size="small"
                          :disabled="!currentSessionAutoRetryEnabled"
                        >
                          {{ t('webSession.autoRetryDispatchPendingOnFailure') }}
                        </n-checkbox>
                        <div class="composer-settings-popover-tip">
                          {{ t('webSession.autoRetryDispatchPendingOnFailureTip') }}
                        </div>
                        <div
                          v-if="autoRetryRateLimitNotice"
                          class="composer-settings-popover-tip is-warning"
                        >
                          {{ t('webSession.autoRetryRateLimitNotice') }}
                        </div>
                        <n-checkbox
                          v-model:checked="webSessionActiveCallTimeoutEnabledValue"
                          size="small"
                          :disabled="!canConfigureActiveCallTimeout"
                        >
                          {{ activeCallTimeoutCheckboxLabel }}
                        </n-checkbox>
                        <div class="composer-settings-popover-tip">
                          {{ activeCallTimeoutPopoverTip }}
                        </div>
                        <div
                          v-if="canForceTerminateCodexAppServer"
                          class="composer-settings-danger-zone"
                        >
                          <button
                            type="button"
                            class="composer-settings-danger-button"
                            :disabled="forceTerminateAppServerLoading"
                            @click="confirmForceTerminateCodexAppServer"
                          >
                            <n-icon size="15" aria-hidden="true"><WarningOutline /></n-icon>
                            <span>{{ t('webSession.forceTerminateAppServer') }}</span>
                          </button>
                        </div>
                      </div>
                    </n-popover>
                  </div>
                </div>
              </div>

              <div v-if="draftAttachments.length > 0" class="draft-attachments">
                <span
                  v-for="(attachment, index) in draftAttachments"
                  :key="attachment.id"
                  class="draft-attachment-pill"
                >
                  <n-popover
                    v-if="shouldUseAttachmentHoverPreview(attachment)"
                    trigger="hover"
                    placement="bottom-start"
                    :delay="120"
                  >
                    <template #trigger>
                      <button
                        type="button"
                        class="attachment-preview-trigger"
                        :title="draftAttachmentDisplayName(attachment, index)"
                        @click="openDraftAttachmentPreview(attachment, index)"
                      >
                        <span class="attachment-preview-trigger-text">{{
                          draftAttachmentDisplayName(attachment, index)
                        }}</span>
                      </button>
                    </template>
                    <div class="attachment-hover-preview">
                      <img
                        :src="getAttachmentPreviewUrl(attachment.id)"
                        :alt="attachment.name"
                        class="attachment-hover-image"
                        loading="lazy"
                      />
                    </div>
                  </n-popover>
                  <button
                    v-else-if="shouldUseAttachmentModalPreview(attachment)"
                    type="button"
                    class="attachment-preview-trigger"
                    :title="draftAttachmentDisplayName(attachment, index)"
                    @click="openDraftAttachmentPreview(attachment, index)"
                  >
                    <span class="attachment-preview-trigger-text">{{
                      draftAttachmentDisplayName(attachment, index)
                    }}</span>
                  </button>
                  <button
                    v-else
                    type="button"
                    class="attachment-preview-trigger is-static"
                    :title="draftAttachmentDisplayName(attachment, index)"
                  >
                    <span class="attachment-preview-trigger-text">{{
                      draftAttachmentDisplayName(attachment, index)
                    }}</span>
                  </button>
                  <button
                    type="button"
                    class="draft-attachment-remove"
                    @click="removeAttachment(attachment.id)"
                  >
                    ×
                  </button>
                </span>
              </div>

              <div v-if="pendingInputs.length > 0" class="pending-inputs">
                <div class="pending-input-section-list">
                  <div
                    v-for="(item, index) in pendingInputs"
                    :key="item.id"
                    class="pending-input-chip"
                  >
                    <n-popover
                      trigger="click"
                      placement="bottom-start"
                      :show="pendingInputPopoverId === item.id"
                      :show-arrow="false"
                      display-directive="show"
                      @update:show="handlePendingInputPopoverUpdate(item.id, $event)"
                    >
                      <template #trigger>
                        <button type="button" class="pending-input-trigger">
                          <span class="pending-input-badge" :class="`mode-${item.mode}`">
                            {{
                              item.mode === 'redirect'
                                ? t('webSession.pendingRedirect')
                                : t('webSession.pendingQueue')
                            }}
                          </span>
                          <span
                            v-if="pendingInputTimingLabel(item)"
                            class="pending-input-timing"
                            :class="{
                              'is-paused': item.paused,
                              'is-error': Boolean(item.lastError),
                            }"
                          >
                            {{ pendingInputTimingLabel(item) }}
                          </span>
                          <span class="pending-input-preview">
                            {{ pendingInputPreview(item) }}
                          </span>
                        </button>
                      </template>
                      <div class="pending-input-popover-card">
                        <div class="pending-input-popover-header">
                          <span class="pending-input-badge" :class="`mode-${item.mode}`">
                            {{
                              item.mode === 'redirect'
                                ? t('webSession.pendingRedirect')
                                : t('webSession.pendingQueue')
                            }}
                          </span>
                          <span class="pending-input-position">
                            {{ t('webSession.pendingPosition', { index: index + 1 }) }}
                          </span>
                          <span
                            v-if="pendingInputTimingLabel(item)"
                            class="pending-input-timing"
                            :class="{
                              'is-paused': item.paused,
                              'is-error': Boolean(item.lastError),
                            }"
                          >
                            {{ pendingInputTimingLabel(item) }}
                          </span>
                          <span
                            v-if="item.attachmentIds.length > 0"
                            class="pending-input-attachments"
                          >
                            {{
                              t('webSession.pendingAttachmentsCount', {
                                count: item.attachmentIds.length,
                              })
                            }}
                          </span>
                        </div>
                        <div v-if="isEditingPendingInput(item.id)" class="pending-input-editor">
                          <n-input
                            v-model:value="pendingEditText"
                            type="textarea"
                            size="small"
                            :autosize="pendingInputEditorAutosize"
                          />
                          <div class="pending-input-editor-actions">
                            <n-button
                              size="tiny"
                              tertiary
                              :disabled="pendingEditActionId === item.id"
                              @click="cancelPendingEdit"
                            >
                              {{ t('common.cancel') }}
                            </n-button>
                            <n-button
                              size="tiny"
                              type="primary"
                              :disabled="!pendingEditCanSave"
                              :loading="pendingEditActionId === item.id"
                              @click="handlePendingEditSave(item.id)"
                            >
                              {{ t('common.save') }}
                            </n-button>
                          </div>
                        </div>
                        <template v-else>
                          <div class="pending-input-popover-text">
                            {{ pendingInputPreview(item) }}
                          </div>
                          <div v-if="item.lastError" class="pending-input-error">
                            <span>{{ item.lastError }}</span>
                            <small v-if="item.attemptCount">
                              {{
                                t('webSession.pendingSteerAttempts', { count: item.attemptCount })
                              }}
                            </small>
                            <code v-if="item.lastErrorCode">{{ item.lastErrorCode }}</code>
                          </div>
                          <div v-if="item.nativeQueued" class="pending-input-native-status">
                            {{ t('webSession.pendingNativeQueued') }}
                          </div>
                          <div
                            v-else-if="item.status === 'persisting'"
                            class="pending-input-native-status"
                          >
                            {{ t('webSession.pendingSteerPersisting') }}
                          </div>
                          <div v-else class="pending-input-popover-actions">
                            <button
                              type="button"
                              class="pending-input-action"
                              :disabled="index === 0 || pendingEditActionId === item.id"
                              @click="handleMovePendingInputToAbsoluteIndex(item, index - 1)"
                            >
                              {{ t('webSession.pendingMoveUp') }}
                            </button>
                            <button
                              type="button"
                              class="pending-input-action"
                              :disabled="
                                index >= pendingInputs.length - 1 || pendingEditActionId === item.id
                              "
                              @click="handleMovePendingInputToAbsoluteIndex(item, index + 1)"
                            >
                              {{ t('webSession.pendingMoveDown') }}
                            </button>
                            <button
                              type="button"
                              class="pending-input-action"
                              :disabled="pendingEditActionId === item.id"
                              @click="handleTogglePendingPriority(item)"
                            >
                              {{
                                item.mode === 'redirect'
                                  ? t('webSession.pendingDemoteToQueue')
                                  : t('webSession.pendingPromoteToRedirect')
                              }}
                            </button>
                            <button
                              v-if="item.paused"
                              type="button"
                              class="pending-input-action"
                              :disabled="pendingEditActionId === item.id"
                              @click="handleResumePendingInput(item.id)"
                            >
                              {{ t('webSession.pendingResume') }}
                            </button>
                            <button
                              v-if="item.text.trim()"
                              type="button"
                              class="pending-input-action"
                              :disabled="pendingEditActionId === item.id"
                              @click="startPendingEdit(item)"
                            >
                              {{ t('common.edit') }}
                            </button>
                          </div>
                        </template>
                      </div>
                    </n-popover>
                    <button
                      v-if="!item.nativeQueued && item.status !== 'persisting'"
                      type="button"
                      class="pending-input-remove"
                      :disabled="pendingEditActionId === item.id"
                      @click.stop="handleRemovePendingInput(item.id)"
                    >
                      ×
                    </button>
                  </div>
                </div>
              </div>

              <div v-if="scheduledInputs.length > 0" class="scheduled-inputs">
                <div
                  v-for="item in scheduledInputs"
                  :key="item.id"
                  class="scheduled-input-item"
                  :class="`state-${item.status}`"
                >
                  <n-popover
                    trigger="manual"
                    placement="bottom-start"
                    :show="activeScheduledInputPopoverId === item.id"
                    :show-arrow="false"
                    content-style="padding: 6px 8px;"
                    @clickoutside="closeScheduledInputPopover"
                  >
                    <template #trigger>
                      <button
                        type="button"
                        class="scheduled-input-trigger"
                        :aria-expanded="activeScheduledInputPopoverId === item.id"
                        :aria-label="t('webSession.scheduledDetails')"
                        :aria-busy="scheduledInputActionId === item.id"
                        @click="toggleScheduledInputPopover(item.id)"
                      >
                        <span class="scheduled-input-badge" :class="`state-${item.status}`">
                          {{
                            item.status === 'expired'
                              ? t('webSession.scheduledExpiredBadge')
                              : item.status === 'failed'
                                ? t('webSession.scheduledFailedBadge')
                                : t('webSession.scheduledBadge')
                          }}
                        </span>
                        <span class="scheduled-input-mode">
                          {{
                            item.action === 'execute_plan'
                              ? t('webSession.scheduledPlanMode')
                              : scheduledModeLabel(item.mode)
                          }}
                        </span>
                        <span class="scheduled-input-time" :title="scheduledInputTimeTitle(item)">
                          {{ scheduledInputTimeLabel(item) }}
                        </span>
                        <span class="scheduled-input-preview">{{
                          scheduledInputPreview(item)
                        }}</span>
                      </button>
                    </template>
                    <div class="scheduled-input-popover-card">
                      <div class="scheduled-input-popover-header">
                        <span class="scheduled-input-badge" :class="`state-${item.status}`">
                          {{
                            item.status === 'expired'
                              ? t('webSession.scheduledExpiredBadge')
                              : item.status === 'failed'
                                ? t('webSession.scheduledFailedBadge')
                                : t('webSession.scheduledBadge')
                          }}
                        </span>
                        <span class="scheduled-input-mode">
                          {{
                            item.action === 'execute_plan'
                              ? t('webSession.scheduledPlanMode')
                              : scheduledModeLabel(item.mode)
                          }}
                        </span>
                      </div>
                      <div class="scheduled-input-detail-row">
                        <span>
                          {{
                            item.scheduleKind === 'when_idle'
                              ? t('webSession.scheduledConditionLabel')
                              : t('webSession.scheduledForLabel')
                          }}
                        </span>
                        <strong>
                          {{
                            item.scheduleKind === 'when_idle'
                              ? scheduledIdleStatusLabel(item)
                              : formatDateTime(item.scheduledFor ?? item.createdAt)
                          }}
                        </strong>
                      </div>
                      <div
                        v-if="item.action === 'message' && item.exitPlanMode"
                        class="scheduled-input-detail-row"
                      >
                        <span>{{ t('webSession.scheduledTriggerActionLabel') }}</span>
                        <strong>{{ t('webSession.scheduleExitPlanMode') }}</strong>
                      </div>
                      <div class="scheduled-input-popover-text">
                        {{ scheduledInputDetailText(item) }}
                      </div>
                      <div v-if="item.attachmentIds.length > 0" class="scheduled-input-attachments">
                        {{
                          t('webSession.pendingAttachmentsCount', {
                            count: item.attachmentIds.length,
                          })
                        }}
                      </div>
                      <div
                        v-if="item.status === 'failed' || item.status === 'expired'"
                        class="scheduled-input-error"
                      >
                        <span>{{ t('webSession.scheduledFailureReason') }}</span>
                        <strong>{{ scheduledFailureReason(item) }}</strong>
                      </div>
                      <div
                        v-else-if="item.scheduleKind === 'when_idle' && item.conditionError"
                        class="scheduled-input-error"
                      >
                        <span>{{ t('webSession.scheduledConditionError') }}</span>
                        <strong>{{ item.conditionError }}</strong>
                      </div>
                      <div class="scheduled-input-popover-actions">
                        <template v-if="item.status !== 'expired'">
                          <button
                            type="button"
                            class="scheduled-input-action is-primary"
                            :disabled="Boolean(scheduledInputActionId)"
                            @click="handleDispatchScheduledInputNow(item)"
                          >
                            {{ scheduledImmediateActionLabel(item) }}
                          </button>
                          <button
                            type="button"
                            class="scheduled-input-action"
                            :disabled="Boolean(scheduledInputActionId)"
                            @click="openScheduledInputEditDialog(item)"
                          >
                            {{ scheduledEditActionLabel(item) }}
                          </button>
                        </template>
                        <button
                          type="button"
                          class="scheduled-input-action is-danger"
                          :disabled="Boolean(scheduledInputActionId)"
                          @click="handleRemoveScheduledInput(item.id)"
                        >
                          {{ scheduledRemoveActionLabel(item) }}
                        </button>
                      </div>
                    </div>
                  </n-popover>
                  <button
                    type="button"
                    class="scheduled-input-remove"
                    :title="scheduledRemoveActionLabel(item)"
                    :aria-label="scheduledRemoveActionLabel(item)"
                    :disabled="Boolean(scheduledInputActionId)"
                    @click.stop="handleRemoveScheduledInput(item.id)"
                  >
                    ×
                  </button>
                </div>
              </div>

              <div class="composer-input-shell" :class="{ 'is-mobile': isMobile }">
                <WebSessionComposerEditor
                  :key="composerEditorKey"
                  ref="composerInputRef"
                  v-model="composerText"
                  class="composer-input"
                  :placeholder="composerPlaceholder"
                  :min-rows="composerMinRows"
                  :max-rows="composerMaxRows"
                  :compact="isMobile"
                  :skills="codexSkills"
                  :goal-enabled="selectedAgent === 'codex'"
                  @focus="handleComposerFocus"
                  @blur="handleComposerBlur"
                  @submit="handleComposerSubmitShortcut"
                />
              </div>

              <div class="composer-footer" :class="{ 'is-mobile': isMobile }">
                <div v-if="isMobile" class="composer-footer-left composer-footer-left-mobile">
                  <n-popover
                    v-model:show="showQuickInputPopover"
                    trigger="manual"
                    placement="top-start"
                    content-style="padding: 2px 0;"
                    @clickoutside="handleMobileQuickInputClickOutside"
                    @update:show="handleQuickInputPopoverVisibilityChange"
                  >
                    <template #trigger>
                      <button
                        type="button"
                        class="composer-icon-btn composer-icon-btn-mobile"
                        :title="quickInputButtonTitle"
                        :aria-label="quickInputButtonTitle"
                        @pointerdown.stop.prevent="handleMobileQuickInputTrigger"
                        @click.stop.prevent
                        @keydown.enter.stop.prevent="handleMobileQuickInputTrigger"
                        @keydown.space.stop.prevent="handleMobileQuickInputTrigger"
                      >
                        <n-icon size="18"><FlashOutline /></n-icon>
                      </button>
                    </template>
                    <div class="quick-input-popover-card">
                      <div class="quick-input-popover-header">
                        <div class="quick-input-search-row">
                          <n-input
                            v-model:value="quickInputSearch"
                            size="small"
                            clearable
                            :placeholder="t('webSession.quickInputSearchPlaceholder')"
                            @mousedown.stop
                            @touchstart.stop
                            @click.stop
                          >
                            <template #prefix>
                              <n-icon size="14"><SearchOutline /></n-icon>
                            </template>
                          </n-input>
                          <n-radio-group
                            v-model:value="quickInputScope"
                            size="small"
                            class="quick-input-scope"
                            @mousedown.stop
                            @touchstart.stop
                            @click.stop
                          >
                            <n-radio-button value="global">
                              {{ t('webSession.quickInputScopeGlobal') }}
                            </n-radio-button>
                            <n-radio-button value="project" :disabled="!props.projectId">
                              {{ t('webSession.quickInputScopeProject') }}
                            </n-radio-button>
                          </n-radio-group>
                        </div>
                      </div>
                      <div v-if="quickInputItems.length === 0" class="quick-input-empty">
                        {{ quickInputEmptyLabel }}
                      </div>
                      <div v-else class="quick-input-scroll">
                        <div class="quick-input-item-list">
                          <button
                            v-for="text in quickInputVisibleItems"
                            :key="text"
                            type="button"
                            class="quick-input-item"
                            :class="{ 'is-selected': isQuickInputSelected(text) }"
                            @click="handleQuickInputApply(text)"
                          >
                            <span class="quick-input-item-text">{{ text }}</span>
                          </button>
                        </div>
                      </div>
                      <div class="quick-input-footer">
                        <n-checkbox
                          v-model:checked="quickInputDirectSendEnabled"
                          size="small"
                          @mousedown.stop
                          @touchstart.stop
                          @click.stop
                        >
                          {{ t('webSession.quickInputDirectSend') }}
                        </n-checkbox>
                        <n-pagination
                          v-if="quickInputPageCount > 1"
                          v-model:page="quickInputPage"
                          class="quick-input-pagination"
                          :page-count="quickInputPageCount"
                          :page-size="WEB_SESSION_QUICK_INPUT_PAGE_SIZE"
                          :simple="true"
                          size="small"
                        />
                      </div>
                    </div>
                  </n-popover>
                  <button
                    type="button"
                    class="composer-icon-btn composer-icon-btn-mobile composer-icon-btn-mobile-secondary"
                    :title="t('webSession.skills')"
                    :aria-label="t('webSession.skills')"
                    @pointerdown.stop.prevent="handleMobileSkillTrigger"
                    @click.stop.prevent
                    @keydown.enter.stop.prevent="handleMobileSkillTrigger"
                    @keydown.space.stop.prevent="handleMobileSkillTrigger"
                  >
                    <n-icon size="18"><SparklesOutline /></n-icon>
                  </button>
                  <button
                    type="button"
                    class="composer-icon-btn composer-icon-btn-mobile composer-icon-btn-mobile-secondary"
                    :title="t('webSession.attachImage')"
                    :aria-label="t('webSession.attachImage')"
                    @pointerdown.stop.prevent="handleMobileAttachmentTrigger"
                    @click.stop.prevent
                    @keydown.enter.stop.prevent="handleMobileAttachmentTrigger"
                    @keydown.space.stop.prevent="handleMobileAttachmentTrigger"
                  >
                    <n-icon size="18"><ImageOutline /></n-icon>
                  </button>
                </div>
                <div v-if="!isMobile" class="composer-footer-left">
                  <n-popover
                    v-model:show="showQuickInputPopover"
                    trigger="click"
                    placement="top-start"
                    content-style="padding: 2px 0;"
                    @update:show="handleQuickInputPopoverVisibilityChange"
                  >
                    <template #trigger>
                      <button
                        type="button"
                        class="composer-icon-btn"
                        :title="quickInputButtonTitle"
                        :aria-label="quickInputButtonTitle"
                      >
                        <n-icon size="14"><FlashOutline /></n-icon>
                      </button>
                    </template>
                    <div class="quick-input-popover-card">
                      <div class="quick-input-popover-header">
                        <div class="quick-input-search-row">
                          <n-input
                            v-model:value="quickInputSearch"
                            size="small"
                            clearable
                            :placeholder="t('webSession.quickInputSearchPlaceholder')"
                            @mousedown.stop
                            @touchstart.stop
                            @click.stop
                          >
                            <template #prefix>
                              <n-icon size="14"><SearchOutline /></n-icon>
                            </template>
                          </n-input>
                          <n-radio-group
                            v-model:value="quickInputScope"
                            size="small"
                            class="quick-input-scope"
                            @mousedown.stop
                            @touchstart.stop
                            @click.stop
                          >
                            <n-radio-button value="global">
                              {{ t('webSession.quickInputScopeGlobal') }}
                            </n-radio-button>
                            <n-radio-button value="project" :disabled="!props.projectId">
                              {{ t('webSession.quickInputScopeProject') }}
                            </n-radio-button>
                          </n-radio-group>
                        </div>
                      </div>
                      <div v-if="quickInputItems.length === 0" class="quick-input-empty">
                        {{ quickInputEmptyLabel }}
                      </div>
                      <div v-else class="quick-input-scroll">
                        <div class="quick-input-item-list">
                          <button
                            v-for="text in quickInputVisibleItems"
                            :key="text"
                            type="button"
                            class="quick-input-item"
                            :class="{ 'is-selected': isQuickInputSelected(text) }"
                            @click="handleQuickInputApply(text)"
                          >
                            <span class="quick-input-item-text">{{ text }}</span>
                          </button>
                        </div>
                      </div>
                      <div class="quick-input-footer">
                        <n-checkbox
                          v-model:checked="quickInputDirectSendEnabled"
                          size="small"
                          @mousedown.stop
                          @touchstart.stop
                          @click.stop
                        >
                          {{ t('webSession.quickInputDirectSend') }}
                        </n-checkbox>
                        <n-pagination
                          v-if="quickInputPageCount > 1"
                          v-model:page="quickInputPage"
                          class="quick-input-pagination"
                          :page-count="quickInputPageCount"
                          :page-size="WEB_SESSION_QUICK_INPUT_PAGE_SIZE"
                          :simple="true"
                          size="small"
                        />
                      </div>
                    </div>
                  </n-popover>
                  <n-popover
                    v-model:show="showSkillBrowser"
                    trigger="click"
                    placement="top-start"
                    content-style="padding: 0;"
                    @update:show="handleSkillBrowserVisibilityChange"
                  >
                    <template #trigger>
                      <button
                        type="button"
                        class="composer-icon-btn"
                        :title="t('webSession.skills')"
                        :aria-label="t('webSession.skills')"
                      >
                        <n-icon size="14"><SparklesOutline /></n-icon>
                      </button>
                    </template>
                    <WebSessionSkillCatalogPanel
                      :skills="codexSkills"
                      :loading="codexSkillsLoading"
                      @select-token="handleSkillTokenInsert"
                      @select-template="handleSkillTemplateInsert"
                    />
                  </n-popover>
                  <button type="button" class="composer-icon-btn" @click="openFilePicker">
                    <n-icon size="14"><ImageOutline /></n-icon>
                  </button>
                  <span class="composer-hint">{{ composerHint }}</span>
                </div>

                <div class="composer-footer-right">
                  <n-dropdown
                    trigger="manual"
                    placement="top-end"
                    :show="showSendQuickActions"
                    :options="sendQuickActionOptions"
                    :x="sendQuickActionsX"
                    :y="sendQuickActionsY"
                    @select="handleSendQuickActionSelect"
                    @clickoutside="closeSendQuickActions"
                  />
                  <n-popover
                    v-if="contextUsageIndicator"
                    v-model:show="showContextUsagePopover"
                    trigger="hover"
                    placement="top-end"
                    :show-arrow="false"
                    :keep-alive-on-hover="true"
                    @update:show="handleContextUsagePopoverVisibilityChange"
                  >
                    <template #trigger>
                      <button
                        type="button"
                        class="composer-context-pill"
                        :class="`state-${contextUsageIndicator.state}`"
                        :aria-label="contextUsageIndicator.title"
                      >
                        {{ contextUsageIndicator.label }}
                      </button>
                    </template>
                    <div class="context-usage-popover">
                      <div class="context-usage-popover__header">
                        <span class="context-usage-popover__title">{{
                          contextUsageIndicator.title
                        }}</span>
                        <span
                          class="context-usage-popover__percent"
                          :class="`state-${contextUsageIndicator.state}`"
                        >
                          {{ contextUsageIndicator.label }}
                        </span>
                      </div>
                      <template v-if="contextUsageIndicator.available">
                        <div
                          class="context-usage-stats"
                          :class="{
                            'has-compact-marker': contextUsageIndicator.showCompactMarker,
                          }"
                        >
                          <div class="context-usage-stat">
                            <span class="context-usage-stat__label">{{
                              t('webSession.contextUsageCurrent')
                            }}</span>
                            <button
                              type="button"
                              class="context-usage-number"
                              :title="contextNumberTitle('used')"
                              @click="toggleContextNumber('used')"
                            >
                              {{
                                formatContextTokenCount(contextUsageIndicator.usedTokens, 'used')
                              }}
                            </button>
                          </div>
                          <div class="context-usage-stat">
                            <span class="context-usage-stat__label">{{
                              t('webSession.contextUsageWindowLabel')
                            }}</span>
                            <button
                              type="button"
                              class="context-usage-number"
                              :title="contextNumberTitle('window')"
                              @click="toggleContextNumber('window')"
                            >
                              {{
                                formatContextTokenCount(
                                  contextUsageIndicator.contextWindowTokens,
                                  'window'
                                )
                              }}
                            </button>
                          </div>
                          <div
                            v-if="contextUsageIndicator.showCompactMarker"
                            class="context-usage-stat"
                          >
                            <span class="context-usage-stat__label">{{
                              t('webSession.contextUsageCompactLabel')
                            }}</span>
                            <button
                              type="button"
                              class="context-usage-number"
                              :title="contextNumberTitle('compact')"
                              @click="toggleContextNumber('compact')"
                            >
                              {{
                                formatContextTokenCount(
                                  contextUsageIndicator.compactLimitTokens,
                                  'compact'
                                )
                              }}
                            </button>
                          </div>
                        </div>
                        <div class="context-usage-axis" aria-hidden="true">
                          <span
                            class="context-usage-axis__fill"
                            :style="{
                              width: `${contextUsageIndicator.usedPercent}%`,
                            }"
                          ></span>
                          <span
                            class="context-usage-axis__marker context-usage-axis__marker--current"
                            :style="{
                              left: `${contextUsageIndicator.usedPercent}%`,
                            }"
                          ></span>
                          <span
                            v-if="contextUsageIndicator.showCompactMarker"
                            class="context-usage-axis__marker context-usage-axis__marker--compact"
                            :style="{
                              left: `${contextUsageIndicator.compactPercent}%`,
                            }"
                          ></span>
                        </div>
                        <div class="context-usage-axis-labels">
                          <span>{{ t('webSession.contextUsageCurrentMarker') }}</span>
                          <span>{{ t('webSession.contextUsageWindowMarker') }}</span>
                        </div>
                      </template>
                      <div v-else class="context-usage-unavailable">
                        {{ t('webSession.contextUsageUnavailableDescription') }}
                      </div>

                      <div
                        v-if="contextUsageIndicator.available || contextUsageIndicator.hasUsage"
                        class="context-usage-total-stats"
                      >
                        <div class="context-usage-stat">
                          <span class="context-usage-stat__label">
                            {{ t('webSession.contextUsageCumulativeNonCached') }}
                          </span>
                          <span class="context-usage-total-value">
                            {{
                              formatWebSessionTokenCount(
                                contextUsageIndicator.cumulativeNonCachedTokens
                              )
                            }}
                            tokens
                          </span>
                        </div>
                        <div class="context-usage-stat">
                          <span class="context-usage-stat__label">
                            {{ t('webSession.contextUsageTotalUsage') }}
                          </span>
                          <span class="context-usage-total-value">
                            {{ formatWebSessionTokenCount(contextUsageIndicator.totalTokens) }}
                            tokens
                          </span>
                        </div>
                      </div>

                      <div class="context-usage-divider"></div>
                      <div class="work-timing-popover">
                        <div class="work-timing-popover__header">
                          <span class="work-timing-popover__title">
                            <n-icon size="15"><TimeOutline /></n-icon>
                            {{ t('webSession.workTimingTitle') }}
                          </span>
                          <span class="work-timing-popover__value">{{
                            formatWorkDuration(currentWorkTimingDurationMs)
                          }}</span>
                        </div>
                        <div class="work-timing-popover__status">
                          <n-spin v-if="workTimingCalculationLoading" :size="13" />
                          <span v-else>{{ workTimingStatusLabel }}</span>
                        </div>
                        <div
                          v-if="currentWorkRunDurationMs != null"
                          class="work-timing-popover__current"
                        >
                          {{
                            t('webSession.workTimingCurrentRun', {
                              duration: formatWorkDuration(currentWorkRunDurationMs),
                            })
                          }}
                        </div>
                      </div>
                    </div>
                  </n-popover>
                  <n-button
                    v-if="isRunActive"
                    secondary
                    type="warning"
                    class="composer-stop-btn"
                    @click="handleAbortCurrent"
                  >
                    {{ t('webSession.stop') }}
                  </n-button>
                  <template v-if="isRunActive">
                    <n-button
                      secondary
                      class="composer-queue-btn"
                      :loading="isSubmittingQueuedMessage"
                      :disabled="!canStageDuringRun"
                      @click="handlePreinput('queue')"
                    >
                      {{ t('webSession.preinputQueue') }}
                    </n-button>
                    <div class="composer-send-action-group">
                      <n-button
                        type="primary"
                        class="composer-send-btn"
                        :loading="isSubmittingRedirectedMessage"
                        :disabled="!canStageDuringRun"
                        @pointerdown="handlePrimarySendPointerDown"
                        @pointermove="handlePrimarySendPointerMove"
                        @pointerup="handlePrimarySendPointerUp"
                        @pointercancel="handlePrimarySendPointerCancel"
                        @click="handlePrimarySendButtonClick"
                      >
                        {{ t('webSession.preinputRedirect') }}
                      </n-button>
                      <button
                        type="button"
                        class="composer-send-menu-btn"
                        :title="t('webSession.sendQuickActions')"
                        :aria-label="t('webSession.sendQuickActions')"
                        :disabled="!canStageDuringRun"
                        @click="handleSendQuickActionTriggerClick"
                      >
                        <n-icon size="14"><ChevronDownOutline /></n-icon>
                      </button>
                    </div>
                  </template>
                  <n-popover
                    v-else
                    trigger="manual"
                    placement="top-end"
                    :show="showSendConflictWarning"
                    :show-arrow="true"
                    :disabled="!showSendConflictWarning"
                  >
                    <template #trigger>
                      <div
                        class="composer-send-action-group"
                        :class="{ 'is-confirm-armed': isSendConflictConfirmationArmed }"
                      >
                        <n-button
                          type="primary"
                          :class="[
                            'composer-send-btn',
                            { 'is-confirm-armed': isSendConflictConfirmationArmed },
                          ]"
                          :loading="isSubmittingMessage && !isSubmittingPlanExecution"
                          :disabled="!canSend"
                          @pointerdown="handlePrimarySendPointerDown"
                          @pointermove="handlePrimarySendPointerMove"
                          @pointerup="handlePrimarySendPointerUp"
                          @pointercancel="handlePrimarySendPointerCancel"
                          @click="handlePrimarySendButtonClick"
                        >
                          {{
                            isSendConflictConfirmationArmed
                              ? t('webSession.sendEmphatic')
                              : t('webSession.send')
                          }}
                        </n-button>
                        <button
                          type="button"
                          class="composer-send-menu-btn"
                          :title="t('webSession.sendQuickActions')"
                          :aria-label="t('webSession.sendQuickActions')"
                          :disabled="!canSend"
                          @click="handleSendQuickActionTriggerClick"
                        >
                          <n-icon size="14"><ChevronDownOutline /></n-icon>
                        </button>
                      </div>
                    </template>
                    <div
                      class="composer-send-confirm-popover-card"
                      role="status"
                      aria-live="polite"
                    >
                      <div class="composer-send-confirm-title">
                        {{ t('webSession.sendConflictWarningTitle') }}
                      </div>
                      <div class="composer-send-confirm-body">
                        {{ sendConflictWarningBody }}
                      </div>
                    </div>
                  </n-popover>
                </div>
              </div>
              <TransferProgressDialog
                v-if="composerTransferCard"
                :message="composerTransferCard.message"
                :detail="composerTransferCard.detail"
                :progress="composerTransferCard.progress"
                :tone="composerTransferCard.tone"
                :card-style="composerTransferDialogStyle"
              />
            </template>
          </div>
        </div>

        <WebSessionSidebar
          v-if="showCrossProjectSidebar"
          ref="sidebarRootRef"
          :resizing="isSidebarResizing"
          :width="effectiveSidebarWidthPx"
          :scope-label="sidebarScopeLabel"
          :scope-options="sidebarScopeOptions"
          :scope-aria-label="sidebarScopeAriaLabel"
          :visible-session-count="sidebarVisibleSessionCount"
          :search-query="sidebarSearchQuery"
          :search-archived="sidebarSearchArchived"
          :search-body="sidebarSearchBody"
          :search-progress-visible="sidebarSearchProgressVisible"
          :search-progress-percentage="sidebarSearchProgressPercentage"
          :empty="sidebarIsEmpty"
          :search-error="sidebarSearchError"
          :no-search-results="sidebarHasNoSearchResults"
          :search-active="normalizedSidebarSearchQuery.length > 0"
          :items="sidebarVirtualItems"
          :get-action-options="buildSidebarSessionActionOptions"
          @start-resize="startSidebarResize"
          @toggle-scope="toggleSidebarScope"
          @select-scope="handleSidebarScopeSelect"
          @reset-width="resetSidebarWidth"
          @update:search-query="sidebarSearchQuery = $event"
          @update:search-archived="sidebarSearchArchived = $event"
          @update:search-body="sidebarSearchBody = $event"
          @toggle-group="toggleSessionGroup"
          @select-session="handleSidebarVirtualSessionSelect"
          @session-action="handleSidebarSessionActionSelect"
          @load-more="handleLoadMoreArchived"
        />
      </div>
    </div>
    <WebSessionScheduledSendDialog
      :show="showScheduledSendDialog"
      :purpose="scheduledSendPurpose"
      :title="scheduledDialogTitle"
      :confirm-label="scheduledDialogConfirmLabel"
      :submitting="scheduledSendSubmitting"
      :edit-text="scheduledEditText"
      :editing-attachment-count="scheduledEditingInput?.attachmentIds.length ?? 0"
      :schedule-kind="scheduledScheduleKind"
      :presets="scheduledSendPresetOptions"
      :selected-preset-key="selectedScheduledSendPresetKey"
      :send-at="scheduledSendAt"
      :selected-time-label="scheduledDialogSelectedTimeLabel"
      :mode="scheduledSendMode"
      :exit-plan-mode="scheduledExitPlanMode"
      :can-confirm="canConfirmScheduledSend"
      @update:show="handleScheduledSendDialogVisibilityChange"
      @update:edit-text="scheduledEditText = $event"
      @update:schedule-kind="scheduledScheduleKind = $event"
      @update:send-at="scheduledSendAt = $event"
      @update:mode="scheduledSendMode = $event"
      @update:exit-plan-mode="scheduledExitPlanMode = $event"
      @select-preset="handleScheduledSendPresetSelect"
      @confirm="handleConfirmScheduledSend"
    />
    <WebSessionLocalFileDialog
      :show="showLocalFileDialog"
      :target="localFileDialogTarget"
      :action="localFileAction"
      @update:show="handleLocalFileDialogVisibilityChange"
      @open-location="handleOpenLocalFileLocation"
      @download="handleDownloadLocalFile"
    />
    <WebSessionMessageEditDialog
      :show="showMessageEditDialog"
      :text="messageEditText"
      :submitting="messageEditSubmitting"
      :can-submit="messageEditCanSubmit"
      :attachments="editingUserMessage?.block.attachments ?? []"
      @update:show="handleMessageEditDialogVisibilityChange"
      @update:text="messageEditText = $event"
      @confirm="handleConfirmMessageEdit"
    />
    <WebSessionAttachmentPreviewDialog
      :show="showAttachmentPreview"
      :preview="activeAttachmentPreview"
      @update:show="handleAttachmentPreviewVisibilityChange"
    />
    <WebSessionCommandExecutionDetailDialog
      :show="showCommandExecutionDetail"
      :loading="loadingCommandExecutionDetail"
      :detail="activeCommandExecutionDetail"
      :title="commandExecutionDetailTitle"
      :kind-label="commandExecutionDetailKindLabel"
      @update:show="handleCommandExecutionDetailVisibilityChange"
    />
    <n-modal
      :show="isMobile && showSkillBrowser"
      preset="card"
      class="skill-browser-modal"
      :title="t('webSession.skills')"
      :bordered="false"
      :segmented="{ content: false, footer: false }"
      :mask-closable="false"
      closable
      style="width: min(92vw, 520px)"
      @mask-click="handleMobileSkillMaskClick"
      @update:show="handleSkillBrowserVisibilityChange"
    >
      <WebSessionSkillCatalogPanel
        :skills="codexSkills"
        :loading="codexSkillsLoading"
        @select-token="handleSkillTokenInsert"
        @select-template="handleSkillTemplateInsert"
      />
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import {
  type Component,
  type CSSProperties,
  type VNode,
  computed,
  h,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  watch,
  type HTMLAttributes,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useDebounceFn, useEventListener, useResizeObserver, useStorage } from '@vueuse/core';
import { storeToRefs } from 'pinia';
import {
  NCheckbox,
  NIcon,
  NInput,
  useDialog,
  useMessage,
  type DialogReactive,
  type DropdownOption,
} from 'naive-ui';
import {
  AddOutline,
  AlertCircleOutline,
  ArchiveOutline,
  ArrowDownOutline,
  ArrowUpOutline,
  ChevronDownOutline,
  ChevronUpOutline,
  CloseOutline,
  CopyOutline,
  CreateOutline,
  FlashOutline,
  Flag as FlagIcon,
  FolderOpenOutline,
  FunnelOutline,
  GitNetworkOutline,
  GitBranchOutline,
  GridOutline,
  ImageOutline,
  RefreshCircleOutline,
  RefreshOutline,
  SettingsOutline,
  SearchOutline,
  SparklesOutline,
  SyncOutline,
  TimeOutline,
  TrashOutline,
  WarningOutline,
} from '@vicons/ionicons5';
import Sortable, { type SortableEvent } from 'sortablejs';
import { useAppClipboard } from '@/composables/useAppClipboard';
import { useLocale } from '@/composables/useLocale';
import { useMobileKeyboard } from '@/composables/useMobileKeyboard';
import { useResponsive } from '@/composables/useResponsive';
import { projectApi, systemApi } from '@/api/project';
import { useProjectStore } from '@/stores/project';
import { useSettingsStore } from '@/stores/settings';
import { useDeveloperConfigStore } from '@/stores/developerConfig';
import {
  isWebSessionMessageDeliveryError,
  useWebSessionStore,
  type WebSessionBlock,
  type WebSessionDraftState,
  type WebSessionHistoryPage,
  type WebSessionHistoryAnswerEntry,
  type WebSessionLiveState,
  type WebSessionPendingInput,
  type WebSessionScheduledInput,
  type WebSessionSubAgent,
  type WebSessionSubAgentStatus,
  type WebSessionUserInputOption,
  type WebSessionUserInputQuestion,
} from '@/stores/webSession';
import type {
  CodexSkillSummary,
  DeveloperConfig,
  ProjectAgentTrustStatus,
  WebSessionAgent,
  WebSessionContextWindowSource,
  WebSessionRuntimeConfig,
  WebSessionReasoningEffort,
  WebSessionSummary,
} from '@/types/models';
import {
  calculateCardTabIndicatorStyle,
  ensureActiveCardTabVisible,
  hiddenCardTabIndicatorStyle,
} from '@/utils/cardTabIndicator';
import {
  isWebSessionActivityDisplayToolKind,
  resolveWebSessionActivityDisplayMode,
  shouldUseWebSessionActivityDisplayMode,
} from '@/constants/webSessionActivityDisplayMode';
import { getAssistantIconByType } from '@/utils/assistantIcon';
import { isDarkHex } from '@/utils/color';
import { renderHighlightedPlainText, renderMarkdown } from '@/utils/markdown';
import {
  buildImagePlaceholder,
  buildImageViewPreviewUrl,
  insertImagePlaceholdersAtCursor,
  resolveImageAttachmentDisplayName,
  stripImagePlaceholdersFromText,
} from '@/utils/webSessionImages';
import { urlBase } from '@/api';
import {
  webSessionApi,
  type WebSessionCommandExecutionGroupDetail,
  type WebSessionCommandExecutionGroupItem,
  type SessionSearchChunkResult,
  type WebSessionPiTreeMutationResult,
} from '@/api/webSession';
import { createLongPressTracker } from '@/utils/longPress';
import TransferProgressDialog from '@/components/common/TransferProgressDialog.vue';
import IconGoal from '@/components/icons/IconGoal.vue';
import WebSessionApprovalNotifier from '@/components/web-session/WebSessionApprovalNotifier.vue';
import WebSessionAttachmentPreviewDialog from '@/components/web-session/WebSessionAttachmentPreviewDialog.vue';
import WebSessionCommandExecutionDetailDialog from '@/components/web-session/WebSessionCommandExecutionDetailDialog.vue';
import WebSessionComposerEditor from '@/components/web-session/WebSessionComposerEditor.vue';
import WebSessionCompletionNotifier from '@/components/web-session/WebSessionCompletionNotifier.vue';
import PiProjectTrustDialog from '@/components/project/PiProjectTrustDialog.vue';
import { isPiProjectTrusted } from '@/components/project/piProjectTrust';
import WebSessionImportDialog from '@/components/web-session/WebSessionImportDialog.vue';
import WebSessionLocalFileDialog from '@/components/web-session/WebSessionLocalFileDialog.vue';
import WebSessionMessageEditDialog from '@/components/web-session/WebSessionMessageEditDialog.vue';
import WebSessionMobileSessionDrawer from '@/components/web-session/WebSessionMobileSessionDrawer.vue';
import WebSessionScheduledSendDialog from '@/components/web-session/WebSessionScheduledSendDialog.vue';
import WebSessionSidebar from '@/components/web-session/WebSessionSidebar.vue';
import { useWebSessionSidebarResize } from '@/components/web-session/useWebSessionSidebarResize';
import WebSessionSkillCatalogPanel from '@/components/web-session/WebSessionSkillCatalogPanel.vue';
import WebSessionTreeDrawer from '@/components/web-session/WebSessionTreeDrawer.vue';
import type { WebSessionComposerEditorExposed } from '@/components/web-session/webSessionComposerEditor';
import {
  isWebSessionDevMode,
  shouldShowCyberPolicyWarning,
} from '@/components/web-session/webSessionDevMode';
import { resolveWebSessionAgentCapability } from '@/components/web-session/webSessionAgentCapabilities';
import {
  buildWebSessionComposerPastePlan,
  getImageFilesFromTransfer,
  mergeClipboardImageFiles,
  readClipboardImageFiles,
  renderWebSessionComposerPastePlan,
  type WebSessionComposerPastePlan,
} from '@/components/web-session/webSessionComposerPaste';
import { resolveWebSessionMobileContextWorktree } from '@/components/web-session/webSessionMobileProjectContext';
import { useWebSessionMobileProjectSwitch } from '@/components/web-session/useWebSessionMobileProjectSwitch';
import { isFailedWebSessionUserMessage } from '@/components/web-session/webSessionMessageFailure';
import { useWebSessionMobileChangesSummary } from '@/components/web-session/useWebSessionMobileChangesSummary';
import {
  insertCodexSkillTokenAtCursor,
  replaceTextSelection,
} from '@/components/web-session/webSessionCodexSkills';
import {
  CLAUDE_RUNTIME_OPTIONS,
  CLAUDE_MODEL_OPTIONS,
  CODEX_ADDITIONAL_MODEL_OPTIONS,
  CODEX_MODEL_OPTIONS,
  CODEX_PRIMARY_MODEL_OPTIONS,
  CUSTOM_MODEL_VALUE,
  MORE_MODELS_VALUE,
  defaultModelForAgent as resolveDefaultModelForAgent,
  defaultPermissionLevelForAgent as resolveDefaultPermissionLevelForAgent,
  defaultReasoningEffortForAgent as resolveDefaultReasoningEffortForAgent,
  resolveCodexReasoningEfforts,
  resolvePiModelOptionGroups,
  resolvePiModelOptions,
  resolvePiReasoningEfforts,
  type WebSessionClaudeRuntimeOption,
  type WebSessionModelOptionGroup,
} from '@/components/web-session/webSessionModelOptions';
import {
  buildTimelineRawModeKey,
  pruneActiveTimelineRawBlockKey,
  resolveActivatedTimelineRawBlockKey,
  shouldClearActiveTimelineRawBlockKey,
  shouldShowTimelineRawToggle as shouldShowTimelineRawToggleForBlock,
  toggleExclusiveTimelineRawBlock,
  type TimelineRawSurface,
} from '@/components/web-session/webSessionRawToggle';
import { resolveWebSessionAttachmentPreviewMode } from '@/components/web-session/webSessionAttachmentPreview';
import { projectWebSessionVisibleTimelineBlocks } from '@/components/web-session/webSessionCompactTimeline';
import {
  findLatestSubAgentActivityBlock,
  isTransportRetryActivityText,
  subAgentActivitySummary,
} from '@/components/web-session/webSessionSubAgentActivity';
import { resolveWebSessionTimelineSubAgent } from '@/components/web-session/webSessionTimelineRole';
import { useWebSessionConversationSearch } from '@/components/web-session/useWebSessionConversationSearch';
import { useWebSessionLocalFileNavigation } from '@/components/web-session/useWebSessionLocalFileNavigation';
import { createWebSessionToolPresentation } from '@/components/web-session/webSessionToolPresentation';
import { createWebSessionStreamingMarkdownController } from '@/components/web-session/webSessionStreamingMarkdown';
import {
  filterWebSessionQuickInputItems,
  paginateWebSessionQuickInputItems,
  WEB_SESSION_QUICK_INPUT_PAGE_SIZE,
  type WebSessionQuickInputScope,
} from '@/components/web-session/webSessionQuickInput';
import {
  createWebSessionMobileComposerScrollState,
  createWebSessionTimelineFollowState,
  resolveWebSessionMobileComposerBottomScrollAction,
  resolveWebSessionMobileComposerScrollState,
  resolveWebSessionTimelineFollowState,
  shouldApplyWebSessionTimelineAutoScroll,
  type WebSessionMobileComposerScrollState,
  type WebSessionTimelineScrollMetrics,
} from '@/components/web-session/webSessionTimelineScroll';
import {
  findClosestWebSessionTimelineAnchor,
  forgetWebSessionTimelinePosition,
  getWebSessionTimelinePosition,
  loadWebSessionTimelinePositionState,
  persistWebSessionTimelinePositionState,
  rememberWebSessionTimelinePosition,
  resolveWebSessionTimelineAnchor,
  resolveWebSessionTimelineRestoreScrollTop,
  type WebSessionTimelinePosition,
} from '@/components/web-session/webSessionTimelinePosition';
import {
  canNavigateWebSessionUserMessage,
  findViewportAdjacentWebSessionUserMessageKey,
  resolveWebSessionTimelineStartConfirmation,
  resolveWebSessionUserMessageTarget,
  type WebSessionTimelineStartConfirmationState,
  type WebSessionUserMessageNavigationDirection,
} from '@/components/web-session/webSessionUserMessageNavigation';
import {
  getWebSessionSidebarTone,
  getWebSessionTabTone,
} from '@/components/web-session/sessionVisualState';
import {
  beginWebSessionSubmit,
  buildWebSessionSubmitOwnerId,
  endWebSessionSubmit,
  getWebSessionSubmitEntry,
  isWebSessionSubmitting,
  resolveOptimisticWebSessionLiveState,
  shouldShowWebSessionExecuteFeedback,
  transferWebSessionSubmit,
  type WebSessionSubmitKind,
  type WebSessionSubmitState,
} from '@/components/web-session/webSessionSubmitState';
import {
  buildWebSessionUserInputSubmitOwnerId,
  hasMissingWebSessionUserInputAnswers,
  scheduleWebSessionUserInputSlowHint,
} from '@/components/web-session/webSessionUserInputSubmit';
import {
  buildWebSessionUserInputQuestionMemoDeps,
  buildWebSessionUserInputDraftSyncKey,
  buildWebSessionUserInputDraftStorageKey,
  reconcileWebSessionUserInputLocalState,
} from '@/components/web-session/webSessionUserInputDraftSync';
import { pickLatestWebSessionPendingInputEditDraft } from '@/components/web-session/webSessionPendingInputEdit';
import {
  buildWebSessionSendConfirmationSignature,
  findWebSessionSendConflicts,
  resolveWebSessionSendConfirmation,
  type WebSessionSendConfirmationState,
} from '@/components/web-session/webSessionSendGuard';
import {
  resolveWebSessionDisplayState,
  resolveWebSessionSidebarSortTimestamp,
  type WebSessionDisplayState,
} from '@/components/web-session/webSessionSessionState';
import { shouldShowAutoRetryRateLimitNotice } from '@/components/web-session/webSessionAutoRetryNotice';
import {
  canMutateWebSessionPiTree,
  canOpenWebSessionPiTree,
} from '@/components/web-session/webSessionTree';
import {
  formatWebSessionDateTime,
  formatWebSessionSidebarTime,
  formatWebSessionTimestamp,
} from '@/components/web-session/webSessionTimeFormat';
import {
  formatElapsedDuration,
  resolveWebSessionLiveTimeCopy,
  type WebSessionLiveTimeTooltipItem,
} from '@/components/web-session/webSessionLiveTime';
import {
  calculateBillableTokenUsage,
  calculateCodexRemainingContext,
  calculateTotalTokenUsage,
} from '@/components/web-session/webSessionContextUsage';
import { formatWebSessionTokenCount } from '@/components/web-session/webSessionContextDisplay';
import { resolveCopyableAgentSessionId } from '@/components/web-session/webSessionSessionId';
import {
  buildOrderedTabSessions,
  resolveNextWebSessionTabAfterClose,
  resolveActiveTabSessionId,
  resolveUnderlyingTabSessionId,
  sortMobileCurrentSessions,
} from '@/components/web-session/webSessionTabOrder';
import {
  buildWebSessionMobileTabDescriptors,
  type MobileSessionCategory,
} from '@/components/web-session/webSessionMobileTabOptions';
import { buildWebSessionMobilePendingSummary } from '@/components/web-session/webSessionMobilePendingSummary';
import {
  resolveWebSessionMobileSelectionAction,
  type WebSessionMobileSelectionAction,
} from '@/components/web-session/webSessionMobileSelection';
import {
  collapseProjectDraftTabs,
  resolveStartDraftSessionDecision,
} from '@/components/web-session/webSessionDraftTabs';
import {
  createWebSessionProjectInitializationGate,
  resolveWebSessionDraftProjectPresentation,
} from '@/components/web-session/webSessionProjectScope';
import {
  normalizeWebSessionSidebarScope,
  resolveWebSessionSidebarProjectIds,
  resolveWebSessionSidebarToggleScope,
  type WebSessionSidebarScope,
} from '@/components/web-session/webSessionSidebarScope';
import {
  buildWebSessionSidebarVirtualItems,
  groupWebSessionSidebarEntriesByDate,
  groupWebSessionItemsByDate,
  resolveWebSessionSidebarCollapsedKeys,
  type WebSessionSidebarRowView,
  type WebSessionSidebarSessionEntry,
  type WebSessionSidebarVirtualItem,
} from '@/components/web-session/webSessionSidebarVirtualList';
import {
  mergeWebSessionSearchMatchSources,
  mergeWebSessionSidebarSearchPage,
  normalizeWebSessionSidebarSearchQuery,
  resolveWebSessionSidebarSearchMatchSources,
} from '@/components/web-session/webSessionSidebarSearch';
import type { CrossProjectSessionItem } from '@/components/web-session/webSessionSidebarView';
import {
  isArchivedPreviewSession,
  isDraftSession,
  type ArchivedPreviewSessionTab,
  type DraftSessionTab,
  type MobileSessionDrawerView,
  type MobileTabListDescriptor,
  type SessionTab,
} from '@/components/web-session/webSessionPanelSession';
import { normalizeWebSessionSyncState } from '@/utils/webSessionSyncState';
import { createWebSessionSnapshotLoadController } from '@/utils/webSessionSnapshotLoadController';
import { createWebSessionCatchUpScheduler } from '@/components/web-session/webSessionCatchUpScheduler';
import { buildProjectBadgeMap, type ProjectBadge } from '@/utils/projectBadge';
import { buildWorkspaceRouteQuery, inferWorkspaceRouteTab } from '@/utils/workspaceRoute';
import {
  buildWebSessionProjectLocation,
  buildWebSessionRouteQuery,
  getWebSessionRouteSessionId,
  isWebSessionRouteQuerySynced,
  resolveWebSessionDeepLinkTarget,
  shouldPreserveWebSessionRouteSessionId,
} from '@/utils/webSessionRoute';
import { selectMostRecentWebSession } from '@/utils/webSessionRecency';

const MAX_TAB_TITLE_WIDTH = 160;
const TAB_LABEL_EXTRA_SPACE = 40;
const TABS_CONTAINER_STATIC_OFFSET = 220;
const TABS_CONTAINER_MIN_OFFSET = 140;
const SHARED_WIDTH_HIDE_THRESHOLD = 860;
const WEB_SESSION_CATCH_UP_DEBOUNCE_MS = 150;
const WEB_SESSION_CATCH_UP_SETTLE_MS = 180;
const WEB_SESSION_TIMELINE_POSITION_DEBOUNCE_MS = 180;
const WEB_SESSION_TIMELINE_POSITION_MAX_WAIT_MS = 600;
const WEB_SESSION_TIMELINE_RESTORE_HISTORY_LIMIT = 120;
const WEB_SESSION_TIMELINE_EDGE_HISTORY_LIMIT = 80;
const WEB_SESSION_TIMELINE_START_CONFIRM_TTL_MS = 5000;
const WEB_SESSION_TIMELINE_NAVIGATION_VISIBLE_MS = 5000;
const DRAFT_SESSION_STORAGE_KEY = 'workspace-web-session-draft-tabs';
const ACTIVE_DRAFT_SESSION_STORAGE_KEY = 'workspace-web-session-active-draft';
const TAB_ORDER_STORAGE_KEY = 'workspace-web-session-tab-order';
const TAB_VISIT_MRU_STORAGE_KEY = 'workspace-web-session-tab-visit-mru-v1';
const SIDEBAR_SCOPE_STORAGE_KEY = 'workspace-web-session-sidebar-scope';
const SIDEBAR_SEARCH_ARCHIVED_STORAGE_KEY = 'workspace-web-session-sidebar-search-archived';
const SIDEBAR_SEARCH_BODY_STORAGE_KEY = 'workspace-web-session-sidebar-search-body';
const CYBER_POLICY_DISMISSALS_STORAGE_KEY = 'workspace-web-session-cyber-policy-dismissals';
const SIDEBAR_SEARCH_SCAN_LIMIT = 50;
const MOBILE_COMPOSER_COLLAPSED_STORAGE_KEY = 'workspace-web-session-mobile-composer-collapsed';
const LIVE_TIME_TICK_MS = 1000;
const DEFAULT_ACTIVE_CALL_TIMEOUT_SECONDS = 120;
const WEB_SESSION_SEND_CONFIRM_TTL_MS = 5000;
const MOBILE_COMPOSER_OVERLAY_OPEN_GUARD_MS = 180;
const STREAMING_MARKDOWN_RENDER_OPTIONS = Object.freeze({
  disableCodeHighlight: true,
});

const props = withDefaults(
  defineProps<{
    projectId: string;
    showSidebar?: boolean;
    isActive?: boolean;
  }>(),
  {
    showSidebar: true,
    isActive: true,
  }
);

const emit = defineEmits<{
  (event: 'mobile-composer-focus-change', focused: boolean): void;
  (event: 'request-mobile-view', view: 'webSession' | 'changes'): void;
}>();

const liveStateClockMs = ref(Date.now());
const collapsedSessionGroupKeys = ref<ReadonlySet<string>>(new Set());
let liveStateClockTimer: number | null = null;
const sidebarCalendarDayStartMs = computed(() => {
  const date = new Date(liveStateClockMs.value);
  date.setHours(0, 0, 0, 0);
  return date.getTime();
});

type SidebarSearchState = {
  items: WebSessionSummary[];
  scanned: number;
  total: number;
  loading: boolean;
  done: boolean;
  error: boolean;
};

function createSidebarSearchState(): SidebarSearchState {
  return {
    items: [],
    scanned: 0,
    total: 0,
    loading: false,
    done: false,
    error: false,
  };
}

type MobileTabSelectorSource = 'header' | 'bottom-nav';

type InlinePlanChoiceOption = {
  label: string;
  isExecute: boolean;
};

type InlinePlanChoice = {
  questionId: string;
  prompt: string;
  options: InlinePlanChoiceOption[];
};

type CommandExecutionDetailItem = WebSessionCommandExecutionGroupItem;
type CommandExecutionDetail = WebSessionCommandExecutionGroupDetail;

type ImageViewPreviewState = 'loading' | 'ready' | 'error';

type ScheduledSendMode = 'send' | 'interrupt' | 'queue';
type ScheduledSendPurpose = 'message' | 'execute_plan' | 'edit_message' | 'edit_plan';
type ScheduledScheduleKind = 'at_time' | 'when_idle';

type ScheduledSendPresetOption = {
  key: string;
  label: string;
  timestamp: number;
};

type ScheduledPlanDialogTarget = {
  sessionId: string;
  planItemId: string;
  pendingItemId?: string;
  questionId?: string;
  executeOptionLabel?: string;
};

function isAbortLikeError(error: unknown) {
  return Boolean(
    error &&
      typeof error === 'object' &&
      'name' in error &&
      String((error as { name?: unknown }).name || '') === 'AbortError'
  );
}

const webSessionStore = useWebSessionStore();
const projectStore = useProjectStore();
const settingsStore = useSettingsStore();
const route = useRoute();
const router = useRouter();
const dialog = useDialog();
const message = useMessage();
const { locale, t } = useLocale();
const { copyText } = useAppClipboard();
const { isMobile } = useResponsive();
const timelineMarkdownRenderOptions = computed(() => ({
  enableCodeBlockCopy: true,
  codeBlockCopyLabel: 'copy',
  enableLinkCopy: true,
  linkCopyLabel: t('common.copyLink'),
}));
const streamingTimelineMarkdownRenderOptions = computed(() => ({
  ...STREAMING_MARKDOWN_RENDER_OPTIONS,
  enableCodeBlockCopy: true,
  codeBlockCopyLabel: 'copy',
  enableLinkCopy: true,
  linkCopyLabel: t('common.copyLink'),
}));
const effectiveWebSessionActivityDisplayMode = computed(() =>
  resolveWebSessionActivityDisplayMode(webSessionActivityDisplayMode.value)
);

const {
  activeTheme,
  confirmBeforeTerminalClose,
  showWebSessionReasoning,
  webSessionActivityDisplayMode,
  webSessionStreamingMarkdownThrottleMs,
  webSessionQuickInput,
  webSessionQuickInputDirectSend,
} = storeToRefs(settingsStore);

const globalActiveCallTimeoutEnabled = ref(true);
const globalActiveCallTimeoutSeconds = ref(DEFAULT_ACTIVE_CALL_TIMEOUT_SECONDS);
const developerConfigStore = useDeveloperConfigStore();
const { config: developerConfig } = storeToRefs(developerConfigStore);
const webSessionAutoContinueScope = computed(
  () => developerConfig.value.webSessionAutoRetryDefaults.scope
);
const webSessionAutoContinuePreset = computed(
  () => developerConfig.value.webSessionAutoRetryDefaults.preset
);
const webSessionAutoContinueMaxAttempts = computed(
  () => developerConfig.value.webSessionAutoRetryDefaults.maxAttempts
);
const webSessionAutoRetryDispatchPendingOnFailure = computed(
  () => developerConfig.value.webSessionAutoRetryDefaults.dispatchPendingOnFailure
);

watch(
  developerConfig,
  config => {
    globalActiveCallTimeoutEnabled.value = resolveGlobalActiveCallTimeoutEnabled(config);
    globalActiveCallTimeoutSeconds.value = resolveGlobalActiveCallTimeoutSeconds(config);
  },
  { deep: true, immediate: true }
);

function resolveGlobalActiveCallTimeoutEnabled(config?: DeveloperConfig | null) {
  const mode = config?.webSessionActiveCallTimeout?.enabledMode;
  if (mode === 'off') {
    return false;
  }
  return true;
}

function resolveGlobalActiveCallTimeoutSeconds(config?: DeveloperConfig | null) {
  const timeoutConfig = config?.webSessionActiveCallTimeout;
  if (timeoutConfig?.timeoutMode !== 'custom') {
    return DEFAULT_ACTIVE_CALL_TIMEOUT_SECONDS;
  }
  const seconds = Number(timeoutConfig.customTimeoutSeconds);
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return DEFAULT_ACTIVE_CALL_TIMEOUT_SECONDS;
  }
  return Math.max(10, Math.trunc(seconds));
}

function formatActiveCallTimeoutDuration(seconds: number) {
  const normalizedSeconds = Math.max(1, Math.trunc(seconds));
  return t('webSession.activeCallTimeoutDurationSeconds', { seconds: normalizedSeconds });
}

async function loadComposerDeveloperConfig(force = false) {
  try {
    const config = await developerConfigStore.load(force);
    globalActiveCallTimeoutEnabled.value = resolveGlobalActiveCallTimeoutEnabled(config);
    globalActiveCallTimeoutSeconds.value = resolveGlobalActiveCallTimeoutSeconds(config);
    return true;
  } catch (error) {
    console.error('[Web Session] Failed to load developer config for composer settings', error);
    return false;
  }
}

function resolveInheritedActiveCallTimeoutEnabled(
  source: { agent?: WebSessionAgent; activeCallTimeoutEnabled?: boolean | null } | null | undefined,
  agent: WebSessionAgent
) {
  if (agent !== 'codex') {
    return false;
  }
  if (typeof source?.activeCallTimeoutEnabled === 'boolean') {
    return source.activeCallTimeoutEnabled;
  }
  return globalActiveCallTimeoutEnabled.value;
}
const persistedDraftSessionsByProject = useStorage<Record<string, DraftSessionTab[]>>(
  DRAFT_SESSION_STORAGE_KEY,
  {}
);
const persistedActiveDraftSessionIdByProject = useStorage<Record<string, string>>(
  ACTIVE_DRAFT_SESSION_STORAGE_KEY,
  {}
);
const persistedTabOrderByProject = useStorage<Record<string, string[]>>(TAB_ORDER_STORAGE_KEY, {});
const persistedTabVisitMruByProject = useStorage<Record<string, string[]>>(
  TAB_VISIT_MRU_STORAGE_KEY,
  {}
);
const dismissedCyberPolicyWarnings = useStorage<Record<string, boolean>>(
  CYBER_POLICY_DISMISSALS_STORAGE_KEY,
  {}
);
const routeWebSessionId = computed(() => getWebSessionRouteSessionId(route.query));
const routeWorkspaceTab = computed(() => inferWorkspaceRouteTab(route.query));
const webSessionDevMode = computed(() => isWebSessionDevMode(route.query));

const tabsContainerRef = ref<HTMLElement | null>(null);
const timelineScrollRef = ref<HTMLDivElement | null>(null);
const timelineListRef = ref<HTMLDivElement | null>(null);
const timelineUserMessageElements = new Map<string, HTMLElement>();
const timelineBlockElements = new Map<string, HTMLElement>();
type TimelineNavigationAction = 'start' | 'end' | 'previous' | 'next';
type TimelineEdgeWindow = WebSessionHistoryPage & { sessionId: string };
const timelineEdgeWindow = shallowRef<TimelineEdgeWindow | null>(null);
const timelineNavigationPending = ref<TimelineNavigationAction | null>(null);
const timelineNavigationBusy = computed(() => timelineNavigationPending.value !== null);
const timelineViewportNavigation = ref({ previous: false, next: false });
const timelineNavigationControlsExpanded = ref(false);
const timelineStartConfirmationState = ref<WebSessionTimelineStartConfirmationState | null>(null);
let timelineNavigationRequestVersion = 0;
const userMessageNavigationPending = ref<{
  key: string;
  direction: WebSessionUserMessageNavigationDirection;
} | null>(null);
const mobileComposerScrollState = ref<WebSessionMobileComposerScrollState | null>(null);
const fileInputRef = ref<HTMLInputElement | null>(null);
const composerInputRef = ref<WebSessionComposerEditorExposed | null>(null);
const composerEditorResetVersion = ref(0);
const sidebarRootRef = ref<InstanceType<typeof WebSessionSidebar> | null>(null);
const autoFollowBottom = ref(true);
const showJumpToBottom = ref(false);
const devCyberPolicySessionId = ref('');
const lastTimelineScrollTop = ref(0);
let timelineScrollSyncVersion = 0;
const pendingTimelinePositionRestore = ref<{
  projectId: string;
  sessionId: string;
  position: WebSessionTimelinePosition;
  generation: number;
} | null>(null);
const expandedTools = ref<Record<string, boolean>>({});
const imageViewPreviewSrcByToolId = ref<Record<string, string>>({});
const imageViewPreviewStateByToolId = ref<Record<string, ImageViewPreviewState>>({});
const showMobileTabSelector = ref(false);
const mobileTabSelectorSource = ref<MobileTabSelectorSource>('header');
const showQuickInputPopover = ref(false);
const showContextUsagePopover = ref(false);
const workTimingCalculationLoading = ref(false);
const workTimingCalculationError = ref('');
const workTimingBusyRetryPending = ref(false);
const workTimingBusyRetryConsumed = ref(false);
type ContextNumberKey = 'used' | 'window' | 'compact';
const contextExactNumbers = ref<Record<ContextNumberKey, boolean>>({
  used: false,
  window: false,
  compact: false,
});
const quickInputScope = ref<WebSessionQuickInputScope>(props.projectId ? 'project' : 'global');
const quickInputSearch = ref('');
const quickInputPage = ref(1);
const showSkillBrowser = ref(false);
const showImportDialog = ref(false);
const showPiTrustDialog = ref(false);
const showPiTreeDrawer = ref(false);
const piTrustServerProjectPath = ref('');
const pendingPiAgentSelection = ref(false);
const contextMenuSession = ref<SessionTab | null>(null);
const contextMenuX = ref(0);
const contextMenuY = ref(0);
const showSendQuickActions = ref(false);
const sendQuickActionsX = ref(0);
const sendQuickActionsY = ref(0);
const sendQuickActionAnchor = shallowRef<HTMLElement | null>(null);
const showPlanQuickActions = ref(false);
const planQuickActionsX = ref(0);
const planQuickActionsY = ref(0);
const planQuickActionAnchor = shallowRef<HTMLElement | null>(null);
const showScheduledSendDialog = ref(false);
const showMessageEditDialog = ref(false);
const editingUserMessage = ref<{
  projectId: string;
  sessionId: string;
  block: WebSessionBlock;
} | null>(null);
const messageEditText = ref('');
const messageEditSubmitting = ref(false);
const scheduledSendPurpose = ref<ScheduledSendPurpose>('message');
const scheduledPlanDialogTarget = ref<ScheduledPlanDialogTarget | null>(null);
const scheduledEditingInput = ref<WebSessionScheduledInput | null>(null);
const scheduledEditText = ref('');
const activeScheduledInputPopoverId = ref('');
const scheduledInputActionId = ref('');
const scheduledSendAt = ref<number | null>(null);
const scheduledSendMode = ref<ScheduledSendMode>('send');
const scheduledExitPlanMode = ref(false);
const scheduledScheduleKind = ref<ScheduledScheduleKind>('at_time');
const scheduledSendPresetOptions = ref<ScheduledSendPresetOption[]>([]);
const scheduledSendSubmitting = ref(false);
const activeTabIndicatorStyle = ref(hiddenCardTabIndicatorStyle());
const tabsContainerWidth = ref(0);
const tabTitleMaxWidth = ref(MAX_TAB_TITLE_WIDTH);
const isComposerDragOver = ref(false);
const isMobileComposerCollapsed = useStorage(MOBILE_COMPOSER_COLLAPSED_STORAGE_KEY, false);
const isMobileComposerSettingsExpanded = ref(false);
const isMobileComposerFocused = ref(false);
const isComposerFocused = ref(false);
const isMobileKeyboardResizeFrozen = ref(false);
const showAttachmentPreview = ref(false);
const activeAttachmentPreview = ref<{
  id: string;
  name: string;
  url: string;
} | null>(null);
const runtimeConfig = ref<WebSessionRuntimeConfig | null>(null);
const codexRuntimeConfig = runtimeConfig;
const codexRuntimeConfigReady = ref(false);
let runtimeConfigRetryTimer: ReturnType<typeof setTimeout> | null = null;
let runtimeConfigRetryDelayMs = 500;
let runtimeConfigLoadGeneration = 0;
const codexSkills = ref<CodexSkillSummary[]>([]);
const codexSkillsLoading = ref(false);
const codexSkillsLoaded = ref(false);
const showCommandExecutionDetail = ref(false);
const loadingCommandExecutionDetail = ref(false);
const activeCommandExecutionDetail = ref<CommandExecutionDetail | null>(null);
const activeCommandExecutionGroupId = ref('');
const dismissedPlanActions = ref<Record<string, boolean>>({});
const rawTimelineBlocks = ref<Record<string, boolean>>({});
const activeRawTimelineBlockKey = ref('');
const streamingMarkdownTextByKey = ref<Record<string, string>>({});
const userInputSelections = ref<Record<string, string[]>>({});
const userInputDrafts = ref<Record<string, string>>({});
let activeUserInputDraftStorageKey = '';
const submitStateBySessionId = ref<WebSessionSubmitState>({});
const userInputSubmitStateByOwnerId = ref<WebSessionSubmitState>({});
const userInputSlowStateByOwnerId = ref<WebSessionSubmitState>({});
const archiveStateBySessionId = ref<WebSessionSubmitState>({});
const importingCodexSessionId = ref('');
const sendConfirmationState = ref<WebSessionSendConfirmationState | null>(null);
const liveCardContinuePending = ref(false);
const retryingUserMessageKey = ref('');
const optimisticUnreadClearedVersionBySession = ref<Record<string, number>>({});
const webSessionCatchUpActive = ref(false);
const isProjectSessionInitializing = ref(false);
const pendingRouteActivationSessionId = ref('');
const frozenBlocks = ref<WebSessionBlock[] | null>(null);

const mobileKeyboard = useMobileKeyboard({
  enabled: () => Boolean(isMobile.value),
  onDismissed: () => {
    if (!props.isActive || !currentSession.value) {
      return;
    }
    scheduleScrollToBottom();
  },
  onStateChange: state => {
    isMobileKeyboardResizeFrozen.value = state.isResizeFrozen;
    emitMobileComposerChromeHidden(
      Boolean(isMobile.value && props.isActive && state.isResizeFrozen)
    );
  },
});
const pendingHistoryAnchor = ref<{
  sessionId: string;
  previousHeight: number;
  previousTop: number;
} | null>(null);
const tabDragSortable = shallowRef<Sortable | null>(null);
const timelinePositionStorage = resolveTimelinePositionStorage();
let timelinePositionState = loadWebSessionTimelinePositionState(timelinePositionStorage);
let timelinePositionRestoreGeneration = 0;
let timelinePositionRestoreRunningGeneration = 0;
let composerDragDepth = 0;
let webSessionCatchUpTimer: number | null = null;
let webSessionCatchUpToken = 0;
const webSessionCatchUpScheduler = createWebSessionCatchUpScheduler(reason => {
  void refreshWebSessionCatchUp(reason);
}, WEB_SESSION_CATCH_UP_DEBOUNCE_MS);
let sendConfirmationTimer: number | null = null;
let timelineNavigationControlsTimer: number | null = null;
let timelineStartConfirmationTimer: number | null = null;
let lastEmittedMobileComposerChromeHidden = false;
let mobileTimelineTouchY: number | null = null;
let scheduledSendLongPressAnchor: HTMLElement | null = null;
const loadedSidebarProjectIds = new Set<string>();
const streamingMarkdownController = createWebSessionStreamingMarkdownController({
  delayMs: webSessionStreamingMarkdownThrottleMs.value,
  onStateChange: state => {
    streamingMarkdownTextByKey.value = state;
  },
});
const persistedSidebarScope = useStorage<string>(SIDEBAR_SCOPE_STORAGE_KEY, 'all');
const sidebarSearchQuery = ref('');
const sidebarSearchArchived = useStorage<boolean>(SIDEBAR_SEARCH_ARCHIVED_STORAGE_KEY, false);
const sidebarSearchBody = useStorage<boolean>(SIDEBAR_SEARCH_BODY_STORAGE_KEY, true);
const sidebarSearchState = ref<SidebarSearchState>(createSidebarSearchState());
let sidebarSearchRequestVersion = 0;
let sidebarSearchAbortController: AbortController | null = null;
const composerTransferErrorMessage = ref('');
const composerTransferErrorDetail = ref('');
let composerTransferErrorTimer: number | null = null;
const composerPastePendingBySession = ref<Record<string, number>>({});
const composerPasteQueues = new Map<string, Promise<void>>();
let cancelUserInputSlowHint: (() => void) | null = null;
let activeUserInputSlowHintOwnerId = '';
let mobileQuickInputOpenedAt = 0;
let mobileSkillBrowserOpenedAt = 0;
const realSessionSnapshotLoadController = createWebSessionSnapshotLoadController();
const projectSessionInitializationGate = createWebSessionProjectInitializationGate();

const IMAGE_ATTACHMENT_NAME_PATTERN = /\.(png|jpe?g|gif|webp|bmp|svg|tiff?)$/i;

const draftAgent = ref<WebSessionAgent>('codex');
const draftClaudeRuntime = ref<WebSessionClaudeRuntimeOption>('claude');
const draftModel = ref(defaultModelForAgent('codex'));
const draftReasoningEffort = ref<WebSessionReasoningEffort>(
  defaultReasoningEffortForAgent('codex')
);
const draftWorkflowMode = ref<'default' | 'plan'>('default');
const draftPermissionLevel = ref<'default' | 'elevated' | 'yolo'>(
  defaultPermissionLevelForAgent('codex')
);
const draftSessions = ref<DraftSessionTab[]>([]);
const activeDraftSessionId = ref('');
const activeArchivedPreviewId = ref('');
const archivedPreviewSession = ref<ArchivedPreviewSessionTab | null>(null);
const tabOrderIds = ref<string[]>([]);
const tabMruIds = ref<string[]>([]);

const realSessions = computed<SessionTab[]>(() =>
  webSessionStore.getSessions(props.projectId).map(session => ({
    ...session,
    isDraft: false as const,
  }))
);
const nonArchivedVisibleSessions = computed<SessionTab[]>(() => [
  ...realSessions.value,
  ...draftSessions.value,
]);
const allVisibleSessions = computed<SessionTab[]>(() => [
  ...sessions.value,
  ...(archivedPreviewSession.value ? [archivedPreviewSession.value] : []),
]);
const visibleSessionById = computed(() => {
  const map = new Map<string, SessionTab>();
  allVisibleSessions.value.forEach(session => {
    map.set(session.id, session);
  });
  return map;
});
const sessions = computed<SessionTab[]>((): SessionTab[] =>
  buildOrderedTabSessions(
    normalizeTabOrderIds(
      tabOrderIds.value,
      nonArchivedVisibleSessions.value.map((session: SessionTab) => session.id)
    ),
    nonArchivedVisibleSessions.value
  )
);
const currentSession = computed<SessionTab | null>(() => {
  if (
    activeArchivedPreviewId.value &&
    archivedPreviewSession.value?.id === activeArchivedPreviewId.value
  ) {
    return archivedPreviewSession.value;
  }
  if (activeDraftSessionId.value) {
    return draftSessions.value.find(session => session.id === activeDraftSessionId.value) ?? null;
  }
  const activeRealId = webSessionStore.getActiveSessionId(props.projectId);
  return realSessions.value.find(session => session.id === activeRealId) ?? null;
});

const mobileProject = computed(() => {
  if (projectStore.currentProject?.id === props.projectId) {
    return projectStore.currentProject;
  }
  return (
    projectStore.projects.find(project => project.id === props.projectId) ??
    projectStore.recentProjects.find(project => project.id === props.projectId) ??
    null
  );
});
const mobileProjectName = computed(
  () => mobileProject.value?.name?.trim() || t('webSession.mobileProjectFallback')
);
const mobileContextWorktree = computed(() =>
  resolveWebSessionMobileContextWorktree(projectStore.worktrees, {
    projectId: props.projectId,
    sessionWorktreeId: currentSession.value?.worktreeId,
    selectedWorktreeId: projectStore.selectedWorktreeId,
  })
);
const mobileProjectBranch = computed(
  () =>
    mobileContextWorktree.value?.branchName?.trim() ||
    mobileProject.value?.defaultBranch?.trim() ||
    ''
);
const mobileProjectContextLabel = computed(() =>
  t('webSession.mobileProjectContext', {
    project: mobileProjectName.value,
    branch: mobileProjectBranch.value || t('webSession.mobileProjectBranchUnknown'),
  })
);
const {
  display: mobileChangesSummaryDisplay,
  loading: mobileChangesSummaryLoading,
  incomplete: mobileChangesSummaryIncomplete,
  statusText: mobileChangesSummaryStatusText,
  visible: showMobileChangesSummaryBadge,
  label: mobileChangesSummaryLabel,
} = useWebSessionMobileChangesSummary({
  projectId: () => props.projectId,
  isActive: () => props.isActive,
  isMobile,
  translate: t,
});
const {
  badges: mobileProjectSwitchBadges,
  currentBadge: currentMobileProjectBadge,
  filteredProjects: filteredMobileProjectSwitchProjects,
  search: mobileProjectSwitchSearch,
} = useWebSessionMobileProjectSwitch({
  projectId: () => props.projectId,
  getProjectName,
});

function isCurrentVisibleSession(sessionId: string) {
  return Boolean(sessionId && currentSession.value?.id === sessionId);
}

const devCyberPolicyWarning = computed({
  get: () =>
    Boolean(currentSession.value?.id && devCyberPolicySessionId.value === currentSession.value.id),
  set: enabled => {
    devCyberPolicySessionId.value = enabled ? (currentSession.value?.id ?? '') : '';
  },
});
const currentRealSession = computed<WebSessionSummary | null>(() => {
  const session = currentSession.value;
  return session && !isDraftSession(session) ? session : null;
});
const {
  show: showLocalFileDialog,
  target: localFileDialogTarget,
  action: localFileAction,
  clear: clearLocalFileDialog,
  handleVisibilityChange: handleLocalFileDialogVisibilityChange,
  openLocation: handleOpenLocalFileLocation,
  download: handleDownloadLocalFile,
  handleTimelineLinkClick,
} = useWebSessionLocalFileNavigation({
  currentSession: currentRealSession,
  fallbackProjectId: () => props.projectId,
});
const workTimingSessionBusy = computed(() => {
  const session = currentRealSession.value;
  return Boolean(
    session?.workTiming?.currentRun ||
      session?.status === 'running' ||
      session?.status === 'waiting_approval' ||
      session?.status === 'aborting'
  );
});
watch(
  () => [currentRealSession.value?.id, workTimingSessionBusy.value] as const,
  ([sessionId, isBusy], previous) => {
    if (sessionId !== previous?.[0]) {
      showContextUsagePopover.value = false;
      workTimingBusyRetryPending.value = false;
      workTimingBusyRetryConsumed.value = false;
      workTimingCalculationError.value = '';
      return;
    }
    if (
      showContextUsagePopover.value &&
      workTimingBusyRetryPending.value &&
      workTimingBusyRetryConsumed.value === false &&
      previous?.[1] === true &&
      isBusy === false
    ) {
      workTimingBusyRetryConsumed.value = true;
      workTimingBusyRetryPending.value = false;
      void calculateWorkTimingOnPopoverOpen();
    }
  }
);
const currentCyberPolicyWarningDismissed = computed(() => {
  const sessionId = currentRealSession.value?.id;
  return Boolean(sessionId && dismissedCyberPolicyWarnings.value[sessionId] === true);
});
const showCyberPolicyWarning = computed(() =>
  shouldShowCyberPolicyWarning({
    sessionFlagged: currentRealSession.value?.cyberPolicyFlagged,
    sessionDismissed: currentCyberPolicyWarningDismissed.value,
    devMode: webSessionDevMode.value,
    simulatedWarning: devCyberPolicyWarning.value,
  })
);
const cyberPolicyAlertThemeOverrides = computed(() => {
  const dark = isDarkHex(activeTheme.value.bodyColor || '#ffffff');
  return {
    borderRadius: '0',
    padding: '8px 14px',
    iconSize: '16px',
    iconMargin: '10px 7px 0 14px',
    closeSize: '18px',
    closeMargin: '9px 12px 0 0',
    colorWarning: dark ? '#332b1f' : '#fff1d6',
    contentTextColorWarning: dark ? '#f4e3c3' : '#5c4326',
    iconColorWarning: dark ? '#f5b74f' : '#e88700',
  };
});

function dismissCyberPolicyWarning() {
  const session = currentRealSession.value;
  if (session?.cyberPolicyFlagged) {
    dismissedCyberPolicyWarnings.value = {
      ...dismissedCyberPolicyWarnings.value,
      [session.id]: true,
    };
  }
  devCyberPolicyWarning.value = false;
}

watch(webSessionDevMode, enabled => {
  if (!enabled) {
    devCyberPolicySessionId.value = '';
  }
});
watch(
  () =>
    [
      currentRealSession.value?.id ?? '',
      currentRealSession.value?.cyberPolicyFlagged === true,
    ] as const,
  ([sessionId, flagged]) => {
    if (!sessionId || flagged || dismissedCyberPolicyWarnings.value[sessionId] !== true) {
      return;
    }
    const nextDismissals = { ...dismissedCyberPolicyWarnings.value };
    delete nextDismissals[sessionId];
    dismissedCyberPolicyWarnings.value = nextDismissals;
  },
  { immediate: true }
);
const runtimeCapabilityFor = (agent: WebSessionAgent) =>
  resolveWebSessionAgentCapability(runtimeConfig.value, agent);
const runtimeCodexCapability = computed(() => runtimeCapabilityFor('codex'));
const runtimeClaudeCapability = computed(() => runtimeCapabilityFor('claude'));
const runtimePiCapability = computed(() => runtimeCapabilityFor('pi'));
const piUnavailableReason = computed(() => {
  const config = runtimeConfig.value;
  switch (config?.piDiagnostics) {
    case 'not_installed':
      return t('webSession.piNotInstalled');
    case 'version_unknown':
      return t('webSession.piVersionUnknown');
    case 'version_too_old':
      return t('webSession.piVersionTooOld', {
        current: config.piVersion || '-',
        required: config.piMinVersion || '0.84.1',
      });
    case 'rpc_start_failed':
      return t('webSession.piRpcStartFailed');
    case 'rpc_protocol_incompatible':
      return t('webSession.piRpcIncompatible');
    case 'rpc_timeout':
      return t('webSession.piRpcTimeout');
    default:
      return t('webSession.composerHintPiUnavailable');
  }
});
const piUnavailableAgentLabel = computed(() => {
  const diagnostic = runtimeConfig.value?.piDiagnostics;
  if (diagnostic === 'not_installed') return t('webSession.piNotInstalledAgentLabel');
  if (diagnostic === 'version_too_old') return t('webSession.piUpgradeAgentLabel');
  return t('webSession.piUnavailableAgentLabel');
});
const runtimeHasCodex = computed(() => runtimeCodexCapability.value.supportsWebSession);
const runtimeHasClaudeCode = computed(() => runtimeClaudeCapability.value.supportsWebSession);
const runtimeCodexVersion = computed(() => codexRuntimeConfig.value?.codexVersion?.trim() || '');
const runtimeMultiAgentV2MinVersion = computed(
  () =>
    codexRuntimeConfig.value?.multiAgentV2MinCodexVersion?.trim() ||
    codexRuntimeConfig.value?.webSessionMinCodexVersion?.trim() ||
    '0.146.0'
);
const runtimeSupportsMultiAgentV2 = computed(() => {
  const config = codexRuntimeConfig.value;
  if (typeof config?.supportsMultiAgentV2 === 'boolean') {
    return config.supportsMultiAgentV2;
  }
  // Older servers used supportsWebSession for the V2 capability.
  return config?.supportsWebSession === true;
});
const isCodexCompatibilityMode = computed(
  () =>
    codexRuntimeConfig.value !== null && runtimeHasCodex.value && !runtimeSupportsMultiAgentV2.value
);
const runtimeGoalModeMinVersion = computed(
  () => codexRuntimeConfig.value?.goalModeMinCodexVersion?.trim() || '0.133.0'
);
const runtimeSupportsGoalMode = computed(() => runtimeCodexCapability.value.supportsGoal);
const isMessageCapabilityBlocked = computed(() => {
  if (selectedAgent.value === 'pi') {
    return !runtimePiCapability.value.supportsWebSession;
  }
  if (!runtimeConfig.value) {
    return false;
  }
  if (selectedAgent.value === 'codex') {
    return !runtimeHasCodex.value;
  }
  if (selectedAgent.value === 'claude') {
    return !runtimeHasClaudeCode.value;
  }
  return !runtimePiCapability.value.supportsWebSession;
});
const isCurrentSessionGoalModeBlocked = computed(() => {
  const session = currentSession.value;
  if (!session || session.agent !== 'codex') {
    return false;
  }
  if (!codexRuntimeConfig.value) {
    return false;
  }
  return !runtimeSupportsGoalMode.value;
});
const currentSessionGoal = computed(() => currentRealSession.value?.goal ?? null);
const isCurrentDraftCodexSession = computed(() => {
  const session = currentSession.value;
  return Boolean(session && session.agent === 'codex' && isDraftSession(session));
});
const isGoalCardVisible = computed(() => {
  const session = currentSession.value;
  return Boolean(
    session && session.agent === 'codex' && !isDraftSession(session) && showGoalCard.value
  );
});
const showGoalCard = ref(false);

function formatGoalDuration(totalSeconds: number) {
  const seconds = Math.max(0, Number(totalSeconds || 0));
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  if (minutes < 60) {
    return remainder > 0 ? `${minutes}m ${remainder}s` : `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  const restMinutes = minutes % 60;
  return restMinutes > 0 ? `${hours}h ${restMinutes}m` : `${hours}h`;
}
const sendGuardProjectId = computed(() => currentRealSession.value?.projectId || props.projectId);
const currentDraftSessionId = computed(() => currentSession.value?.id ?? '');
const composerEditorKey = computed(
  () => `${currentDraftSessionId.value}:${composerEditorResetVersion.value}`
);
const currentSessionAutoRetryEnabled = computed(() =>
  Boolean(currentSession.value?.autoRetryEnabled)
);
const currentSessionAutoRetryDispatchPendingOnFailure = computed(() =>
  Boolean(currentSession.value?.autoRetryDispatchPendingOnFailure)
);

const canConfigureActiveCallTimeout = computed(() => currentSession.value?.agent === 'codex');
const canForceTerminateCodexAppServer = computed(() => currentRealSession.value?.agent === 'codex');
const forceTerminateAppServerLoading = ref(false);
const currentSessionActiveCallTimeoutEnabled = computed(() => {
  const session = currentSession.value;
  if (!session || session.agent !== 'codex') {
    return false;
  }
  if (typeof session.activeCallTimeoutEnabled === 'boolean') {
    return session.activeCallTimeoutEnabled;
  }
  return globalActiveCallTimeoutEnabled.value;
});
const activeCallTimeoutDurationLabel = computed(() =>
  formatActiveCallTimeoutDuration(globalActiveCallTimeoutSeconds.value)
);
const activeCallTimeoutCheckboxLabel = computed(() =>
  t('webSession.autoInterruptLongCall', { duration: activeCallTimeoutDurationLabel.value })
);
const activeCallTimeoutPopoverTip = computed(() =>
  canConfigureActiveCallTimeout.value
    ? t('webSession.autoInterruptLongCallTip')
    : t('webSession.autoInterruptLongCallUnavailableTip')
);
const composerSettingsHasActiveItems = computed(
  () => currentSessionAutoRetryEnabled.value || currentSessionActiveCallTimeoutEnabled.value
);
const autoRetryRateLimitNotice = computed(() =>
  shouldShowAutoRetryRateLimitNotice(
    currentRealSession.value,
    displayLiveState.value.phase === 'error' ? displayLiveState.value.errorMessage : ''
  )
);
const webSessionAutoContinueEnabledValue = computed({
  get: () => currentSessionAutoRetryEnabled.value,
  set: value => {
    const next = value === true;
    const session = currentSession.value;
    if (!session) {
      return;
    }
    if (isDraftSession(session)) {
      updateActiveDraftSession(current => ({
        ...current,
        autoRetryEnabled: next,
        autoRetryPolicyMode: current.autoRetryPolicyMode === 'custom' ? 'custom' : 'default',
        autoRetryScope:
          current.autoRetryPolicyMode === 'custom'
            ? current.autoRetryScope
            : webSessionAutoContinueScope.value,
        autoRetryPreset:
          current.autoRetryPolicyMode === 'custom'
            ? current.autoRetryPreset
            : webSessionAutoContinuePreset.value,
        autoRetryMaxAttempts:
          current.autoRetryPolicyMode === 'custom'
            ? current.autoRetryMaxAttempts
            : webSessionAutoContinueMaxAttempts.value,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      void webSessionStore
        .updateAutoRetry(currentRealSession.value.id, {
          enabled: next,
          policyMode: currentRealSession.value.autoRetryPolicyMode,
          scope:
            currentRealSession.value.autoRetryPolicyMode === 'custom'
              ? currentRealSession.value.autoRetryScope
              : webSessionAutoContinueScope.value,
          preset:
            currentRealSession.value.autoRetryPolicyMode === 'custom'
              ? currentRealSession.value.autoRetryPreset
              : webSessionAutoContinuePreset.value,
          maxAttempts:
            currentRealSession.value.autoRetryPolicyMode === 'custom'
              ? currentRealSession.value.autoRetryMaxAttempts
              : webSessionAutoContinueMaxAttempts.value,
        })
        .catch(error => {
          message.error(error instanceof Error ? error.message : t('common.error'));
        });
    }
  },
});

const webSessionAutoRetryDispatchPendingOnFailureValue = computed({
  get: () => currentSessionAutoRetryDispatchPendingOnFailure.value,
  set: value => {
    const next = value === true;
    const session = currentSession.value;
    if (!session) {
      return;
    }
    if (isDraftSession(session)) {
      updateActiveDraftSession(current => ({
        ...current,
        autoRetryDispatchPendingOnFailure: next,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      void webSessionStore
        .updateAutoRetryDispatchPendingOnFailure(currentRealSession.value.id, next)
        .catch(error => {
          message.error(error instanceof Error ? error.message : t('common.error'));
        });
    }
  },
});

const webSessionActiveCallTimeoutEnabledValue = computed({
  get: () => currentSessionActiveCallTimeoutEnabled.value,
  set: value => {
    const next = value === true;
    const session = currentSession.value;
    if (!session || session.agent !== 'codex') {
      return;
    }
    if (isDraftSession(session)) {
      updateActiveDraftSession(current => ({
        ...current,
        activeCallTimeoutEnabled: next,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      void webSessionStore
        .updateActiveCallTimeout(currentRealSession.value.id, next)
        .catch(error => {
          message.error(error instanceof Error ? error.message : t('common.error'));
        });
    }
  },
});

function handleComposerSettingsPopoverShow(show: boolean) {
  if (show) {
    void loadComposerDeveloperConfig();
  }
}

function confirmForceTerminateCodexAppServer() {
  const session = currentRealSession.value;
  if (!session || session.agent !== 'codex' || forceTerminateAppServerLoading.value) {
    return;
  }
  dialog.warning({
    title: t('webSession.forceTerminateAppServerTitle'),
    content: t('webSession.forceTerminateAppServerConfirm'),
    positiveText: t('webSession.forceTerminateAppServer'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      forceTerminateAppServerLoading.value = true;
      try {
        const result = await webSessionApi.terminateCodexAppServer(session.projectId, session.id);
        message.success(
          t('webSession.forceTerminateAppServerSuccess', { pid: result.processRootPid || '-' })
        );
      } catch (error) {
        message.error(
          t('webSession.forceTerminateAppServerFailed', {
            error: error instanceof Error ? error.message : t('common.error'),
          })
        );
        return false;
      } finally {
        forceTerminateAppServerLoading.value = false;
      }
      return true;
    },
  });
}
const composerText = computed({
  get: () => webSessionStore.getDraft(props.projectId, currentDraftSessionId.value).text,
  set: value => {
    const sessionId = currentDraftSessionId.value;
    if (!sessionId) {
      return;
    }
    webSessionStore.setDraftText(props.projectId, sessionId, value);
  },
});

function clearComposerDraftAfterSubmit(sessionId: string, projectId = props.projectId) {
  const restoreFocus = isComposerFocused.value;
  webSessionStore.clearDraft(projectId, sessionId);
  if (projectId === props.projectId && sessionId === currentDraftSessionId.value) {
    composerEditorResetVersion.value += 1;
    if (restoreFocus) {
      nextTick(() => composerInputRef.value?.focus());
    }
  }
}

function restoreComposerDraftAfterFailedSubmit(
  sessionId: string,
  draft: WebSessionDraftState,
  projectId = props.projectId
) {
  const restoreFocus = isComposerFocused.value;
  webSessionStore.restoreDraft(projectId, sessionId, draft);
  if (projectId === props.projectId && sessionId === currentDraftSessionId.value) {
    composerEditorResetVersion.value += 1;
    if (restoreFocus) {
      nextTick(() => composerInputRef.value?.focus());
    }
  }
}

async function handleGoalPause() {
  if (!currentRealSession.value) {
    return;
  }
  if (!(await ensureGoalModeAvailable())) {
    return;
  }
  try {
    await webSessionStore.pauseGoal(currentRealSession.value.id);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleGoalResume() {
  if (!currentRealSession.value) {
    return;
  }
  if (!(await ensureGoalModeAvailable())) {
    return;
  }
  try {
    await webSessionStore.resumeGoal(currentRealSession.value.id);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleGoalClear() {
  if (!currentRealSession.value) {
    return;
  }
  if (!(await ensureGoalModeAvailable())) {
    return;
  }
  try {
    await webSessionStore.clearGoal(currentRealSession.value.id);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

function parseComposerGoalCommand(raw: string) {
  const text = String(raw || '').trim();
  const match = text.match(/^\/goal(?:\s+(.+))?$/s);
  if (!match) {
    return null;
  }
  return {
    objective: String(match[1] || '').trim(),
  };
}

async function handleGoalCompose() {
  if (!(await ensureGoalModeAvailable())) {
    return;
  }
  const existing = currentRealSession.value?.goal?.objective?.trim() ?? '';
  const nextValue = existing ? `/goal ${existing}` : '/goal ';
  const sessionId = currentDraftSessionId.value;
  if (!sessionId) {
    return;
  }
  setComposerTextAndSelection(nextValue, nextValue.length);
  composerInputRef.value?.focus();
}

async function toggleGoalCard() {
  if (isCurrentDraftCodexSession.value) {
    showGoalCard.value = false;
    await handleGoalCompose();
    return;
  }
  showGoalCard.value = !showGoalCard.value;
  if (showGoalCard.value && currentRealSession.value?.agent === 'codex') {
    if (!(await ensureGoalModeAvailable())) {
      showGoalCard.value = false;
      return;
    }
    try {
      await webSessionStore.refreshGoal(currentRealSession.value.id);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('common.error'));
    }
  }
}
const liveBlocks = computed(() =>
  currentRealSession.value ? webSessionStore.getTimelineBlocks(currentRealSession.value.id) : []
);
const blocks = computed(() =>
  webSessionCatchUpActive.value ? (frozenBlocks.value ?? []) : liveBlocks.value
);
const timelineBlocks = computed(() => {
  const edgeWindow = timelineEdgeWindow.value;
  return edgeWindow && edgeWindow.sessionId === currentRealSession.value?.id
    ? edgeWindow.items
    : blocks.value;
});
function isFailedTimelineUserMessage(item: WebSessionBlock) {
  return isFailedWebSessionUserMessage(item);
}

function isRetryingTimelineUserMessage(item: WebSessionBlock) {
  return retryingUserMessageKey.value === item.key;
}

function shouldShowTimelineUserMessageDeliveryIndicator(item: WebSessionBlock) {
  return isFailedTimelineUserMessage(item) || isRetryingTimelineUserMessage(item);
}

function failedTimelineUserMessageActionLabel(item: WebSessionBlock) {
  if (isRetryingTimelineUserMessage(item)) {
    return t('webSession.userMessageRetrying');
  }
  if (
    retryingUserMessageKey.value ||
    isRunActive.value ||
    (currentRealSession.value &&
      isWebSessionSubmitting(submitStateBySessionId.value, currentRealSession.value.id))
  ) {
    return t('webSession.userMessageRetryBusy');
  }
  return t('webSession.userMessageFailed');
}

function cloneBlockForFreeze(block: WebSessionBlock): WebSessionBlock {
  return {
    ...block,
    attachments: block.attachments.map(attachment => ({ ...attachment })),
    tool: block.tool
      ? {
          ...block.tool,
          meta: block.tool.meta ? { ...block.tool.meta } : undefined,
          commandGroup: block.tool.commandGroup ? { ...block.tool.commandGroup } : undefined,
        }
      : undefined,
    detail: block.detail
      ? {
          ...block.detail,
          questions: block.detail.questions?.map(question => ({
            ...question,
            options: question.options.map(option => ({ ...option })),
          })),
          answers: block.detail.answers?.map(answer => ({
            ...answer,
            values: [...answer.values],
          })),
        }
      : undefined,
  };
}

function snapshotBlocksForFreeze() {
  frozenBlocks.value = liveBlocks.value.map(cloneBlockForFreeze);
}

function clearWebSessionCatchUpTimer() {
  if (webSessionCatchUpTimer != null) {
    window.clearTimeout(webSessionCatchUpTimer);
    webSessionCatchUpTimer = null;
  }
}

function stopWebSessionCatchUp(reason: string) {
  clearWebSessionCatchUpTimer();
  webSessionCatchUpScheduler.cancel();
  if (!webSessionCatchUpActive.value && !frozenBlocks.value) {
    return;
  }
  webSessionCatchUpActive.value = false;
  frozenBlocks.value = null;
  webSessionCatchUpToken += 1;
  console.debug('[Web Session Catch-Up] settled', {
    sessionId: currentRealSession.value?.id,
    reason,
  });
}

function isDocumentVisible() {
  return typeof document === 'undefined' || document.visibilityState === 'visible';
}

function beginWebSessionCatchUp(reason: string) {
  if (!currentRealSession.value) {
    return;
  }
  if (!webSessionCatchUpActive.value) {
    snapshotBlocksForFreeze();
    webSessionCatchUpActive.value = true;
  }
  clearWebSessionCatchUpTimer();
  console.debug('[Web Session Catch-Up] start', {
    sessionId: currentRealSession.value.id,
    reason,
  });
}

async function refreshWebSessionCatchUp(reason: string) {
  const session = currentRealSession.value;
  const sessionId = session?.id;
  if (!sessionId) {
    stopWebSessionCatchUp(`${reason}-no-session`);
    return;
  }

  beginWebSessionCatchUp(reason);
  const token = ++webSessionCatchUpToken;
  const sessionIsArchivedPreview = isArchivedPreviewSession(currentSession.value);
  const isCurrentCatchUp = () =>
    token === webSessionCatchUpToken && isCurrentVisibleSession(sessionId);

  try {
    let serverRevision = session.revision;
    if (session?.projectId && !session.archivedAt) {
      await webSessionStore.reconcileRecentSessions();
      if (!isCurrentCatchUp()) {
        return;
      }
      serverRevision =
        webSessionStore.getSessions(session.projectId).find(item => item.id === sessionId)
          ?.revision ?? serverRevision;
    }
    let snapshot = null;
    if (!webSessionStore.isSessionSnapshotCurrent(sessionId, serverRevision)) {
      if (!isCurrentCatchUp()) {
        return;
      }
      snapshot = await webSessionStore.loadSessionSnapshot(session.projectId, sessionId, {
        rememberActive: false,
        preserveArchivedPosition: sessionIsArchivedPreview,
        conditional: true,
      });
    }
    if (!isCurrentCatchUp()) {
      return;
    }
    if (sessionIsArchivedPreview && snapshot?.session) {
      archivedPreviewSession.value = {
        ...snapshot.session,
        isArchivedPreview: true,
      };
    }
    syncArchivedPreviewSessionSummary(sessionId);
  } catch (error) {
    console.warn('[Web Session Catch-Up] Failed to refresh session snapshot', {
      sessionId,
      reason,
      error,
    });
  }

  if (token !== webSessionCatchUpToken) {
    return;
  }

  clearWebSessionCatchUpTimer();
  webSessionCatchUpTimer = window.setTimeout(() => {
    if (token !== webSessionCatchUpToken) {
      return;
    }
    stopWebSessionCatchUp(reason);
    nextTick(() => {
      const container = timelineScrollRef.value;
      if (!container) {
        return;
      }
      if (autoFollowBottom.value) {
        syncScrollToBottom();
      } else {
        updateBottomState(container);
      }
      markSessionViewed(sessionId);
    });
  }, WEB_SESSION_CATCH_UP_SETTLE_MS);
}

function scheduleWebSessionCatchUp(reason: string) {
  if (!currentRealSession.value?.id) {
    stopWebSessionCatchUp(`${reason}-no-session`);
    return;
  }
  beginWebSessionCatchUp(reason);
  webSessionCatchUpScheduler.schedule(reason);
}

function handleWebSessionDocumentVisibilityChange() {
  if (!isDocumentVisible()) {
    beginWebSessionCatchUp('document-hidden');
    return;
  }
  refreshTabHeaderLayout();
  void loadCodexRuntimeConfig();
  scheduleWebSessionCatchUp('document-visible');
}

function handleWebSessionWindowFocus() {
  if (!isDocumentVisible()) {
    return;
  }
  refreshTabHeaderLayout();
  void loadComposerDeveloperConfig(true);
  void loadCodexRuntimeConfig();
  scheduleWebSessionCatchUp('window-focus');
}

function handleWebSessionWindowPageShow() {
  if (!isDocumentVisible()) {
    return;
  }
  refreshTabHeaderLayout();
  void loadComposerDeveloperConfig(true);
  void loadCodexRuntimeConfig();
  scheduleWebSessionCatchUp('window-pageshow');
}

function isReasoningBlock(block: WebSessionBlock) {
  if (!block.tool) {
    return false;
  }
  return (
    normalizeToolKindValue(block.tool.kind) === 'reasoning' ||
    normalizeToolKindValue(String(block.tool.meta?.kind ?? '')) === 'reasoning'
  );
}

function hasReasoningContent(block: WebSessionBlock) {
  if (!isReasoningBlock(block)) {
    return false;
  }
  return Boolean(block.tool?.output?.trim());
}

function isActivityDisplayBlock(block: WebSessionBlock) {
  if (block.kind !== 'tool' || !block.tool) {
    return false;
  }
  if (isInteractiveDynamicTool(block.tool)) {
    return false;
  }
  return isWebSessionActivityDisplayToolKind(
    block.tool.kind || String(block.tool.meta?.kind ?? '')
  );
}

function shouldRenderActivityDisplayRow(block: WebSessionBlock) {
  return (
    shouldUseWebSessionActivityDisplayMode(webSessionActivityDisplayMode.value) &&
    isActivityDisplayBlock(block)
  );
}

function shouldShowToolPendingPlaceholder(tool: NonNullable<WebSessionBlock['tool']>) {
  if (tool.status !== 'running') {
    return false;
  }
  if (isCompactTool(tool)) {
    return !getCompactToolSummary(tool).trim();
  }
  const hasOutput = typeof tool.output === 'string' && tool.output.trim().length > 0;
  if (hasOutput) {
    return false;
  }
  if (tool.input == null) {
    return true;
  }
  return stringifyValue(tool.input).trim().length === 0;
}
function normalizeChoiceText(value: string) {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/\s+/g, ' ');
}
function isPlanTool(tool?: {
  name: string;
  kind?: string;
  meta?: Record<string, unknown> | undefined;
}) {
  if (!tool) {
    return false;
  }
  const meta = tool.meta ?? {};
  const candidates: string[] = [
    tool.name,
    tool.kind ?? '',
    typeof meta.kind === 'string' ? meta.kind : '',
    typeof meta.title === 'string' ? meta.title : '',
  ];
  return candidates.some(value => normalizeChoiceText(value) === 'plan');
}
function shouldShowMessageRawToggle(block: WebSessionBlock) {
  if (block.kind !== 'user' && block.kind !== 'assistant') {
    return false;
  }
  return Boolean(block.text?.trim());
}
function getDisplayBlockText(block: WebSessionBlock) {
  if (!block.text) {
    return '';
  }
  return stripImagePlaceholdersFromText(block.text, block.attachments.length);
}
function shouldShowPlanRawToggle(block: WebSessionBlock) {
  return Boolean(
    block.kind === 'tool' && block.tool && isPlanTool(block.tool) && block.tool.output?.trim()
  );
}
type StreamingMarkdownSurface = 'message' | 'plan';

function buildStreamingMarkdownKey(block: WebSessionBlock, surface: StreamingMarkdownSurface) {
  return `${block.key}:${surface}`;
}

function isStreamingMessageMarkdownBlock(block: WebSessionBlock) {
  return block.kind === 'assistant' && liveState.value.running && block.done !== true;
}

function isStreamingPlanMarkdownBlock(block: WebSessionBlock) {
  return Boolean(
    block.kind === 'tool' && block.tool && isPlanTool(block.tool) && block.tool.status === 'running'
  );
}

function getStreamingMarkdownText(block: WebSessionBlock, surface: StreamingMarkdownSurface) {
  if (surface === 'plan') {
    return block.tool?.output ?? '';
  }
  return getDisplayBlockText(block);
}

function getEffectiveStreamingMarkdownText(
  block: WebSessionBlock,
  surface: StreamingMarkdownSurface
) {
  const fallback = getStreamingMarkdownText(block, surface);
  return streamingMarkdownTextByKey.value[buildStreamingMarkdownKey(block, surface)] ?? fallback;
}

function getMessageMarkdownText(block: WebSessionBlock) {
  if (!isStreamingMessageMarkdownBlock(block)) {
    return getDisplayBlockText(block);
  }
  return getEffectiveStreamingMarkdownText(block, 'message');
}

function getMessageMarkdownRenderOptions(block: WebSessionBlock) {
  const options = isStreamingMessageMarkdownBlock(block)
    ? streamingTimelineMarkdownRenderOptions.value
    : timelineMarkdownRenderOptions.value;
  const query = timelineSearchQuery.value.trim();
  return query ? { ...options, textHighlightQuery: query } : options;
}

function getPlanToolMarkdownText(block: WebSessionBlock) {
  if (!isStreamingPlanMarkdownBlock(block)) {
    return block.tool?.output ?? '';
  }
  return getEffectiveStreamingMarkdownText(block, 'plan');
}

function getPlanToolMarkdownRenderOptions(block: WebSessionBlock) {
  const options = isStreamingPlanMarkdownBlock(block)
    ? streamingTimelineMarkdownRenderOptions.value
    : timelineMarkdownRenderOptions.value;
  const query = timelineSearchQuery.value.trim();
  return query ? { ...options, textHighlightQuery: query } : options;
}

function getTimelineRawModeKey(block: WebSessionBlock, surface: TimelineRawSurface) {
  return buildTimelineRawModeKey({
    sessionId: currentSession.value?.id,
    surface,
    blockKey: block.key,
  });
}
function isTimelineRawBlockActive(block: WebSessionBlock, surface: TimelineRawSurface) {
  return activeRawTimelineBlockKey.value === getTimelineRawModeKey(block, surface);
}
function activateTimelineRawBlock(block: WebSessionBlock, surface: TimelineRawSurface) {
  const rawCapable =
    surface === 'message' ? shouldShowMessageRawToggle(block) : shouldShowPlanRawToggle(block);
  activeRawTimelineBlockKey.value = resolveActivatedTimelineRawBlockKey(
    rawCapable,
    getTimelineRawModeKey(block, surface)
  );
}
function deactivateTimelineRawBlock(block: WebSessionBlock, surface: TimelineRawSurface) {
  const key = getTimelineRawModeKey(block, surface);
  if (activeRawTimelineBlockKey.value === key) {
    activeRawTimelineBlockKey.value = '';
  }
}
function handleMessageBubbleMouseEnter(block: WebSessionBlock) {
  if (isMobile.value || !shouldShowMessageRawToggle(block)) {
    return;
  }
  activateTimelineRawBlock(block, 'message');
}
function handleMessageBubbleMouseLeave(block: WebSessionBlock) {
  if (isMobile.value || !shouldShowMessageRawToggle(block)) {
    return;
  }
  deactivateTimelineRawBlock(block, 'message');
}
function handleMessageBubbleClick(block: WebSessionBlock) {
  if (!isMobile.value || !shouldShowMessageRawToggle(block)) {
    return;
  }
  activateTimelineRawBlock(block, 'message');
}
function handleMessageBubbleFocusOut(block: WebSessionBlock, event: FocusEvent) {
  if (!shouldShowMessageRawToggle(block)) {
    return;
  }
  const currentTarget = event.currentTarget;
  const relatedTarget = event.relatedTarget;
  if (
    currentTarget instanceof Element &&
    relatedTarget instanceof Node &&
    currentTarget.contains(relatedTarget)
  ) {
    return;
  }
  deactivateTimelineRawBlock(block, 'message');
}
function shouldShowTimelineRawToggle(block: WebSessionBlock, surface: TimelineRawSurface) {
  const rawCapable =
    surface === 'message' ? shouldShowMessageRawToggle(block) : shouldShowPlanRawToggle(block);
  return shouldShowTimelineRawToggleForBlock({
    activeKey: activeRawTimelineBlockKey.value,
    rawKey: getTimelineRawModeKey(block, surface),
    rawCapable,
    rawMode: isBlockRawMode(block, surface),
  });
}
function isBlockRawMode(block: WebSessionBlock, surface: TimelineRawSurface) {
  return !!rawTimelineBlocks.value[getTimelineRawModeKey(block, surface)];
}
function toggleBlockRawMode(block: WebSessionBlock, surface: TimelineRawSurface) {
  const key = getTimelineRawModeKey(block, surface);
  rawTimelineBlocks.value = toggleExclusiveTimelineRawBlock(rawTimelineBlocks.value, key);
}
function getTimelineBlockCopyText(block: WebSessionBlock, surface: TimelineRawSurface) {
  if (surface === 'plan') {
    return block.tool?.output ?? '';
  }
  return block.text;
}
async function copyTimelineBlock(block: WebSessionBlock, surface: TimelineRawSurface) {
  const content = getTimelineBlockCopyText(block, surface);
  await copyText(content, {
    failureMessage: t('terminal.copyFailed'),
    successMessage: t('common.copySuccess'),
  });
}
function isExecutePlanOption(option: WebSessionUserInputOption) {
  const text = normalizeChoiceText(`${option.label} ${option.description}`);
  const mentionsPlan = /计划|plan/.test(text);
  const mentionsExecute = /开始|执行|实现|实施|继续|start|execute|implement|proceed/.test(text);
  const mentionsCancel = /取消|暂不|稍后|later|cancel|dismiss|hold/.test(text);
  return mentionsExecute && (mentionsPlan || !mentionsCancel);
}
function isCancelPlanOption(option: WebSessionUserInputOption) {
  const text = normalizeChoiceText(`${option.label} ${option.description}`);
  return /取消|暂不|稍后|稍后再说|later|cancel|dismiss|hold|keep planning|stay in plan/.test(text);
}
function isPlanChoiceQuestion(question?: WebSessionUserInputQuestion) {
  if (!question || question.options.length !== 2) {
    return false;
  }
  const hasExecute = question.options.some(isExecutePlanOption);
  const hasCancel = question.options.some(isCancelPlanOption);
  return hasExecute && hasCancel;
}
function isPlanChoiceRequestBlock(block: WebSessionBlock) {
  return (
    block.kind === 'system' &&
    block.detail?.type === 'user_input_request' &&
    isPlanChoiceQuestion(block.detail.questions?.[0])
  );
}

const knownSubAgents = computed<WebSessionSubAgent[]>(() =>
  currentRealSession.value ? webSessionStore.getSubAgents(currentRealSession.value.id) : []
);
const subAgentByThreadId = computed(
  () => new Map(knownSubAgents.value.map(agent => [agent.id, agent] as const))
);
const hasKnownSubAgents = computed(() => knownSubAgents.value.length > 0);

function subAgentStatusLabel(status: WebSessionSubAgentStatus) {
  return t(`webSession.subAgentStatus.${status}`);
}

function timelineSubAgent(item: WebSessionBlock) {
  return resolveWebSessionTimelineSubAgent(
    item.sourceThreadId,
    currentRealSession.value?.nativeSessionId,
    subAgentByThreadId.value
  );
}

function shouldRenderToolBlockInTimeline(block: WebSessionBlock) {
  if (block.kind !== 'tool' || !block.tool) {
    return true;
  }
  if (isInteractiveDynamicTool(block.tool)) {
    return false;
  }
  if (isReasoningBlock(block)) {
    return hasReasoningContent(block) || shouldShowToolPendingPlaceholder(block.tool);
  }
  const activeToolGroupId = liveState.value.tool?.groupId || '';
  const activeToolId = liveState.value.tool?.id || '';
  const blockGroupId = block.tool.commandGroup?.id || '';
  if (
    liveState.value.running &&
    block.tool.status === 'running' &&
    ((activeToolGroupId && blockGroupId === activeToolGroupId) ||
      (activeToolId && block.tool.id === activeToolId))
  ) {
    return shouldShowToolPendingPlaceholder(block.tool);
  }
  return true;
}

const filteredTimelineBlocks = computed(() =>
  timelineBlocks.value.filter(block => {
    if (
      !showWebSessionReasoning.value &&
      isReasoningBlock(block) &&
      currentSession.value?.agent !== 'codex'
    ) {
      return false;
    }
    if (isPlanChoiceRequestBlock(block)) {
      return false;
    }
    if (!shouldRenderToolBlockInTimeline(block)) {
      return false;
    }
    return true;
  })
);
const visibleBlocks = computed(() =>
  projectWebSessionVisibleTimelineBlocks(filteredTimelineBlocks.value)
);
const {
  inputRef: timelineSearchInputRef,
  openState: timelineSearchOpen,
  query: timelineSearchQuery,
  filters: timelineSearchFilters,
  currentIndex: timelineSearchCurrentIndex,
  matches: timelineSearchMatches,
  hasPrevious: timelineSearchHasPrevious,
  hasNext: timelineSearchHasNext,
  hasNonDefaultFilters: timelineSearchHasNonDefaultFilters,
  resultLabel: timelineSearchResultLabel,
  normalizedQuery: normalizedTimelineSearchQuery,
  open: openTimelineSearch,
  close: closeTimelineSearch,
  resetForSessionChange: resetTimelineSearchForSessionChange,
  scheduleRemoteSearch: scheduleTimelineSearchRequest,
  navigate: navigateTimelineSearch,
  selectPage: handleTimelineSearchPageChange,
  isBlockMatch: isTimelineSearchBlockMatch,
  isBlockActive: isTimelineSearchBlockActive,
} = useWebSessionConversationSearch({
  currentSession: currentRealSession,
  visibleBlocks,
  allBlocks: blocks,
  isActive: () => props.isActive,
  translate: t,
  onOpen: () => {
    hideTimelineNavigationControls();
    if (getCurrentTimelineEdgeWindow()) {
      resetTimelineEdgeWindow();
    }
  },
  loadEarlierHistory: loadEarlierTimelineSearchHistory,
  scrollToBlock: scrollToTimelineBlock,
});
const visibleRawTimelineBlockKeys = computed(() => {
  const keys: string[] = [];
  visibleBlocks.value.forEach(block => {
    if (shouldShowMessageRawToggle(block)) {
      keys.push(getTimelineRawModeKey(block, 'message'));
    }
    if (shouldShowPlanRawToggle(block)) {
      keys.push(getTimelineRawModeKey(block, 'plan'));
    }
  });
  return keys;
});
const latestPlanBlock = computed(() => {
  for (let index = blocks.value.length - 1; index >= 0; index -= 1) {
    const block = blocks.value[index];
    if (block?.kind === 'tool' && block.tool && isPlanTool(block.tool)) {
      return block;
    }
  }
  return null;
});
const latestPlanToolId = computed(() => latestPlanBlock.value?.tool?.id ?? '');
const latestPlanItemId = computed(() => latestPlanBlock.value?.id ?? '');
const hasUserMessageAfterLatestPlan = computed(() => {
  const planToolId = latestPlanToolId.value;
  if (!planToolId) {
    return false;
  }
  const planIndex = blocks.value.findIndex(
    block => block.kind === 'tool' && block.tool?.id === planToolId
  );
  if (planIndex < 0) {
    return false;
  }
  return blocks.value.slice(planIndex + 1).some(block => block.kind === 'user');
});
const liveState = computed(() =>
  currentRealSession.value
    ? webSessionStore.getLiveState(currentRealSession.value.id)
    : ({ phase: 'idle', running: false, updatedAt: Date.now() } as WebSessionLiveState)
);
const streamingMarkdownTargets = computed(() =>
  visibleBlocks.value.flatMap(block => {
    const targets: Array<{ key: string; text: string }> = [];
    if (isStreamingMessageMarkdownBlock(block)) {
      const text = getDisplayBlockText(block);
      if (text) {
        targets.push({
          key: buildStreamingMarkdownKey(block, 'message'),
          text,
        });
      }
    }
    if (isStreamingPlanMarkdownBlock(block)) {
      const text = block.tool?.output ?? '';
      if (text) {
        targets.push({
          key: buildStreamingMarkdownKey(block, 'plan'),
          text,
        });
      }
    }
    return targets;
  })
);
const pendingApproval = computed(() =>
  currentRealSession.value ? webSessionStore.getPendingApproval(currentRealSession.value.id) : null
);
const approvalRecoveryKey = ref('');
const approvalRecoveryStatus = ref<'idle' | 'loading' | 'unavailable'>('idle');
const approvalDetailsMissing = computed(
  () => liveState.value.phase === 'waiting_approval' && !pendingApproval.value
);
const approvalDetailsLoading = computed(
  () => approvalDetailsMissing.value && approvalRecoveryStatus.value !== 'unavailable'
);
const pendingUserInput = computed(() =>
  currentRealSession.value ? webSessionStore.getPendingUserInput(currentRealSession.value.id) : null
);

function currentApprovalRecoveryKey() {
  const session = currentRealSession.value;
  if (!session) {
    return '';
  }
  return `${session.id}:${session.assistantStateUpdatedAt || liveState.value.updatedAt}`;
}

async function recoverApprovalDetails(force = false) {
  const session = currentRealSession.value;
  if (!session || liveState.value.phase !== 'waiting_approval' || pendingApproval.value) {
    return;
  }
  const recoveryKey = currentApprovalRecoveryKey();
  if (
    !force &&
    approvalRecoveryKey.value === recoveryKey &&
    approvalRecoveryStatus.value !== 'idle'
  ) {
    return;
  }
  approvalRecoveryKey.value = recoveryKey;
  approvalRecoveryStatus.value = 'loading';
  try {
    await webSessionStore.loadSessionSnapshot(session.projectId, session.id, {
      rememberActive: false,
    });
    if (approvalRecoveryKey.value !== recoveryKey) {
      return;
    }
    approvalRecoveryStatus.value = webSessionStore.getPendingApproval(session.id)
      ? 'idle'
      : 'unavailable';
  } catch {
    if (approvalRecoveryKey.value === recoveryKey) {
      approvalRecoveryStatus.value = 'unavailable';
    }
  }
}

function handleRecoverApprovalDetails() {
  void recoverApprovalDetails(true);
}

watch(
  () => [
    currentRealSession.value?.id ?? '',
    currentRealSession.value?.assistantStateUpdatedAt ?? '',
    liveState.value.phase,
    pendingApproval.value?.itemId ?? '',
  ],
  () => {
    if (approvalDetailsMissing.value) {
      void recoverApprovalDetails();
      return;
    }
    approvalRecoveryKey.value = '';
    approvalRecoveryStatus.value = 'idle';
  },
  { immediate: true }
);

function getRuntimeSwitchNoticeKey() {
  if (!currentRealSession.value) {
    return '';
  }
  if (pendingApproval.value || liveState.value.phase === 'waiting_approval') {
    return 'webSession.runtimeSwitchNoticeApproval';
  }
  if (pendingUserInput.value || liveState.value.running) {
    return 'webSession.runtimeSwitchNoticeNextMessage';
  }
  return '';
}

function showRuntimeSwitchNotice(noticeKey: string) {
  if (!noticeKey) {
    return;
  }
  message.info(t(noticeKey));
}

const pendingUserInputSyncKey = computed(() =>
  buildWebSessionUserInputDraftSyncKey(currentRealSession.value?.id, pendingUserInput.value)
);
const pendingUserInputDraftStorageKey = computed(() =>
  buildWebSessionUserInputDraftStorageKey(currentRealSession.value?.id, pendingUserInput.value)
);
const currentUserInputSubmitOwnerId = computed(() =>
  currentRealSession.value && pendingUserInput.value
    ? buildWebSessionUserInputSubmitOwnerId(
        currentRealSession.value.id,
        pendingUserInput.value.itemId
      )
    : ''
);
const isSubmittingUserInput = computed(() =>
  isWebSessionSubmitting(userInputSubmitStateByOwnerId.value, currentUserInputSubmitOwnerId.value)
);
const isUserInputSubmitSlow = computed(() =>
  isWebSessionSubmitting(userInputSlowStateByOwnerId.value, currentUserInputSubmitOwnerId.value)
);
const isUserInputInteractionDisabled = computed(
  () => Boolean(pendingUserInput.value?.stale) || isSubmittingUserInput.value
);
const showUserInputSubmitSlowHint = computed(
  () => isSubmittingUserInput.value && isUserInputSubmitSlow.value
);

function persistActiveUserInputDraft() {
  if (!activeUserInputDraftStorageKey) {
    return;
  }
  webSessionStore.setPendingUserInputDraft(activeUserInputDraftStorageKey, {
    selections: userInputSelections.value,
    drafts: userInputDrafts.value,
  });
}

function clearUserInputDraftStorage(key: string) {
  const normalizedKey = String(key || '').trim();
  if (!normalizedKey) {
    return;
  }
  webSessionStore.clearPendingUserInputDraft(normalizedKey);
  if (activeUserInputDraftStorageKey === normalizedKey) {
    activeUserInputDraftStorageKey = '';
  }
}
const inlinePlanChoice = computed<InlinePlanChoice | null>(() => {
  const request = pendingUserInput.value;
  if (!request || request.stale || !latestPlanToolId.value) {
    return null;
  }
  const question = request.questions[0];
  if (request.questions.length !== 1 || !isPlanChoiceQuestion(question)) {
    return null;
  }
  return {
    questionId: question.id,
    prompt: request.prompt?.trim() || question.question?.trim() || question.header?.trim() || '',
    options: question.options.map(option => ({
      label: option.label,
      isExecute: isExecutePlanOption(option),
    })),
  };
});
const isPlanWaitingApprovalState = computed(
  () =>
    liveState.value.phase === 'waiting_plan_approval' &&
    !pendingApproval.value &&
    Boolean(latestPlanToolId.value) &&
    !hasUserMessageAfterLatestPlan.value
);
const currentSubmitEntry = computed(() =>
  getWebSessionSubmitEntry(submitStateBySessionId.value, currentDraftSessionId.value)
);
const currentSubmitShowsExecuteFeedback = computed(() =>
  shouldShowWebSessionExecuteFeedback(currentSubmitEntry.value)
);
const isOptimisticExecuteFeedbackActive = computed(
  () => currentSubmitShowsExecuteFeedback.value && !liveState.value.running
);
const displayLiveState = computed(() =>
  resolveOptimisticWebSessionLiveState(liveState.value, currentSubmitEntry.value)
);
const activeSubAgents = computed(() => displayLiveState.value.activeSubAgents ?? []);
const activeSubAgentCount = computed(
  () => displayLiveState.value.activeSubAgentCount ?? activeSubAgents.value.length
);
const hasActiveSubAgents = computed(() => activeSubAgentCount.value > 0);
const subAgentTriggerTitle = computed(() =>
  hasActiveSubAgents.value
    ? t('webSession.liveSubAgentCount', { count: activeSubAgentCount.value })
    : t('webSession.subAgentKnownCount', { count: knownSubAgents.value.length })
);
const isSubmittingPlanExecution = computed(() => currentSubmitEntry.value?.kind === 'execute_plan');
const showRuntimeStrip = computed(() => {
  if (pendingApproval.value || pendingUserInput.value) {
    return true;
  }
  if (isPlanWaitingApprovalState.value && !isOptimisticExecuteFeedbackActive.value) {
    return false;
  }
  if (displayLiveState.value.phase === 'idle') {
    return false;
  }
  if (
    displayLiveState.value.phase === 'done' &&
    latestPlanToolId.value &&
    !hasUserMessageAfterLatestPlan.value
  ) {
    return false;
  }
  return true;
});
const hasRecoveredRuntimeRequest = computed(() =>
  Boolean(pendingApproval.value?.stale || pendingUserInput.value?.stale)
);
const recoveredRuntimeHint = computed(
  () =>
    pendingApproval.value?.recoveryMessage ||
    pendingUserInput.value?.recoveryMessage ||
    t('webSession.recoveredRuntimeHint')
);
const historyMeta = computed(() =>
  currentRealSession.value
    ? webSessionStore.getHistoryMeta(currentRealSession.value.id)
    : { hasMore: false, beforeCursor: '', total: 0, loading: false }
);

function getCurrentTimelineEdgeWindow() {
  const edgeWindow = timelineEdgeWindow.value;
  return edgeWindow?.sessionId === currentRealSession.value?.id ? edgeWindow : null;
}

const timelineStartConfirmationArmed = computed(
  () =>
    Boolean(currentRealSession.value?.id) &&
    timelineStartConfirmationState.value?.sessionId === currentRealSession.value?.id
);

function clearTimelineNavigationControlsTimer() {
  if (timelineNavigationControlsTimer != null) {
    window.clearTimeout(timelineNavigationControlsTimer);
    timelineNavigationControlsTimer = null;
  }
}

function hideTimelineNavigationControls() {
  clearTimelineNavigationControlsTimer();
  timelineNavigationControlsExpanded.value = false;
  clearTimelineStartConfirmation();
}

function scheduleTimelineNavigationControlsHide() {
  clearTimelineNavigationControlsTimer();
  if (!isMobile.value || !timelineNavigationControlsExpanded.value) {
    return;
  }
  timelineNavigationControlsTimer = window.setTimeout(() => {
    timelineNavigationControlsTimer = null;
    if (timelineNavigationBusy.value) {
      scheduleTimelineNavigationControlsHide();
      return;
    }
    hideTimelineNavigationControls();
  }, WEB_SESSION_TIMELINE_NAVIGATION_VISIBLE_MS);
}

function showTimelineNavigationControls() {
  if (timelineSearchOpen.value) {
    return;
  }
  timelineNavigationControlsExpanded.value = true;
  scheduleTimelineNavigationControlsHide();
}

function keepTimelineNavigationControlsVisible() {
  if (isMobile.value) {
    showTimelineNavigationControls();
  }
}

function handleTimelineNavigationPointerEnter() {
  if (!isMobile.value) {
    showTimelineNavigationControls();
  }
}

function handleTimelineNavigationPointerLeave() {
  if (!isMobile.value) {
    hideTimelineNavigationControls();
  }
}

function handleTimelineNavigationFocusIn() {
  showTimelineNavigationControls();
}

function handleTimelineNavigationFocusOut(event: FocusEvent) {
  const container = event.currentTarget as HTMLElement | null;
  const nextTarget = event.relatedTarget;
  if (
    !isMobile.value &&
    container &&
    (!(nextTarget instanceof Node) || !container.contains(nextTarget))
  ) {
    hideTimelineNavigationControls();
  }
}

function handleTimelineNavigationActivation() {
  showTimelineNavigationControls();
}

function clearTimelineStartConfirmationTimer() {
  if (timelineStartConfirmationTimer != null) {
    window.clearTimeout(timelineStartConfirmationTimer);
    timelineStartConfirmationTimer = null;
  }
}

function setTimelineStartConfirmationState(
  nextState: WebSessionTimelineStartConfirmationState | null
) {
  clearTimelineStartConfirmationTimer();
  timelineStartConfirmationState.value = nextState;
  if (!nextState) {
    return;
  }
  const delay = Math.max(0, nextState.expiresAt - Date.now());
  timelineStartConfirmationTimer = window.setTimeout(() => {
    timelineStartConfirmationTimer = null;
    if (timelineStartConfirmationState.value?.sessionId === nextState.sessionId) {
      timelineStartConfirmationState.value = null;
    }
  }, delay);
}

function clearTimelineStartConfirmation() {
  setTimelineStartConfirmationState(null);
}

function resetTimelineEdgeWindow() {
  timelineNavigationRequestVersion += 1;
  timelineEdgeWindow.value = null;
  timelineNavigationPending.value = null;
  hideTimelineNavigationControls();
  void nextTick(refreshTimelineViewportNavigation);
}

function mergeTimelineEdgeWindowItems(
  current: readonly WebSessionBlock[],
  incoming: readonly WebSessionBlock[]
) {
  const byId = new Map(current.map(item => [item.id, item]));
  incoming.forEach(item => byId.set(item.id, item));
  return Array.from(byId.values()).sort((left, right) => left.orderIndex - right.orderIndex);
}

function isTimelineNavigationRequestCurrent(version: number, sessionId: string) {
  return version === timelineNavigationRequestVersion && currentRealSession.value?.id === sessionId;
}

async function handleTimelineStartClick() {
  const session = currentRealSession.value;
  if (!session || timelineNavigationBusy.value) {
    return;
  }
  keepTimelineNavigationControlsVisible();
  const confirmation = resolveWebSessionTimelineStartConfirmation({
    sessionId: session.id,
    currentState: timelineStartConfirmationState.value,
    now: Date.now(),
    ttlMs: WEB_SESSION_TIMELINE_START_CONFIRM_TTL_MS,
  });
  setTimelineStartConfirmationState(confirmation.nextState);
  if (confirmation.shouldProceed) {
    await jumpToTimelineStart();
  }
}

async function jumpToTimelineStart() {
  const session = currentRealSession.value;
  if (!session || timelineNavigationBusy.value) {
    return;
  }
  const requestVersion = ++timelineNavigationRequestVersion;
  timelineNavigationPending.value = 'start';
  pendingHistoryAnchor.value = null;
  cancelTimelinePositionRestore();
  invalidateTimelineScrollSync();
  autoFollowBottom.value = false;
  showJumpToBottom.value = true;
  try {
    const page = await webSessionStore.fetchHistoryWindow(session.id, {
      afterCursor: '0',
      limit: WEB_SESSION_TIMELINE_EDGE_HISTORY_LIMIT,
    });
    if (!isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      return;
    }
    timelineEdgeWindow.value = { ...page, sessionId: session.id };
    forgetTimelinePosition(session.projectId, session.id);
    await waitForTimelinePositionLayout();
    if (!isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      return;
    }
    const container = timelineScrollRef.value;
    if (container) {
      container.scrollTop = 0;
      lastTimelineScrollTop.value = 0;
      resetBottomState(container, false);
      resetMobileComposerScrollState(container);
    }
  } catch (error) {
    if (isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      message.error(error instanceof Error ? error.message : t('common.error'));
    }
  } finally {
    if (isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      timelineNavigationPending.value = null;
      refreshTimelineViewportNavigation();
    }
  }
}

async function jumpToTimelineEnd() {
  const session = currentRealSession.value;
  if (!session || timelineNavigationBusy.value) {
    return;
  }
  keepTimelineNavigationControlsVisible();
  clearTimelineStartConfirmation();
  const requestVersion = ++timelineNavigationRequestVersion;
  timelineNavigationPending.value = 'end';
  pendingHistoryAnchor.value = null;
  cancelTimelinePositionRestore();
  invalidateTimelineScrollSync();
  try {
    const snapshot = await webSessionStore.loadSessionSnapshot(session.projectId, session.id, {
      rememberActive: false,
      preserveArchivedPosition: Boolean(session.archivedAt),
      limit: WEB_SESSION_TIMELINE_EDGE_HISTORY_LIMIT,
    });
    if (!isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      return;
    }
    if (isArchivedPreviewSession(currentSession.value) && snapshot?.session) {
      archivedPreviewSession.value = {
        ...snapshot.session,
        isArchivedPreview: true,
      };
      syncArchivedPreviewSessionSummary(session.id);
    }
    timelineEdgeWindow.value = null;
    autoFollowBottom.value = true;
    showJumpToBottom.value = false;
    await waitForTimelinePositionLayout();
    if (isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      syncScrollToBottom();
    }
  } catch (error) {
    if (isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      message.error(error instanceof Error ? error.message : t('common.error'));
    }
  } finally {
    if (isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      timelineNavigationPending.value = null;
      refreshTimelineViewportNavigation();
    }
  }
}

async function loadEarlierTimelineSearchHistory(sessionId: string) {
  const meta = webSessionStore.getHistoryMeta(sessionId);
  if (meta.loading || !meta.hasMore || !meta.beforeCursor) {
    return false;
  }
  const container = timelineScrollRef.value;
  if (container) {
    pendingHistoryAnchor.value = {
      sessionId,
      previousHeight: container.scrollHeight,
      previousTop: container.scrollTop,
    };
  }
  const previousCursor = meta.beforeCursor;
  await webSessionStore.loadMoreHistory(sessionId, 100);
  await nextTick();
  restoreHistoryAnchor();
  const nextMeta = webSessionStore.getHistoryMeta(sessionId);
  return nextMeta.beforeCursor !== previousCursor || nextMeta.hasMore !== meta.hasMore;
}
function setTimelineBlockRef(element: unknown, block: WebSessionBlock) {
  if (element instanceof HTMLElement) {
    timelineBlockElements.set(block.key, element);
    if (block.kind === 'user') {
      timelineUserMessageElements.set(block.key, element);
    }
    return;
  }
  timelineBlockElements.delete(block.key);
  timelineUserMessageElements.delete(block.key);
}

function readTimelineViewportUserMessageCandidates() {
  const visibleUserKeys = new Set(
    visibleBlocks.value.filter(block => block.kind === 'user').map(block => block.key)
  );
  return Array.from(timelineUserMessageElements.entries()).flatMap(([key, element]) => {
    if (!visibleUserKeys.has(key) || !element.isConnected) {
      return [];
    }
    return [{ key, top: element.getBoundingClientRect().top }];
  });
}

function findTimelineViewportUserMessageTarget(
  direction: WebSessionUserMessageNavigationDirection
) {
  const container = timelineScrollRef.value;
  if (!container) {
    return null;
  }
  return findViewportAdjacentWebSessionUserMessageKey(
    readTimelineViewportUserMessageCandidates(),
    container.getBoundingClientRect().top,
    direction
  );
}

function canLoadLaterTimelineEdgeHistory() {
  const edgeWindow = getCurrentTimelineEdgeWindow();
  return Boolean(edgeWindow?.hasLater && edgeWindow.afterCursor);
}

function refreshTimelineViewportNavigation() {
  if (!currentRealSession.value || !timelineScrollRef.value) {
    timelineViewportNavigation.value = { previous: false, next: false };
    return;
  }
  const edgeWindow = getCurrentTimelineEdgeWindow();
  timelineViewportNavigation.value = {
    previous: Boolean(
      findTimelineViewportUserMessageTarget('previous') ||
        (!edgeWindow && canLoadEarlierUserMessageHistory())
    ),
    next: Boolean(
      findTimelineViewportUserMessageTarget('next') ||
        (edgeWindow && canLoadLaterTimelineEdgeHistory())
    ),
  };
}

async function loadLaterTimelineEdgeHistory() {
  const edgeWindow = getCurrentTimelineEdgeWindow();
  if (!edgeWindow?.hasLater || !edgeWindow.afterCursor) {
    return;
  }
  const cursor = edgeWindow.afterCursor;
  const page = await webSessionStore.fetchHistoryWindow(edgeWindow.sessionId, {
    afterCursor: cursor,
    limit: WEB_SESSION_TIMELINE_EDGE_HISTORY_LIMIT,
  });
  const current = getCurrentTimelineEdgeWindow();
  if (!current || current.sessionId !== edgeWindow.sessionId || current.afterCursor !== cursor) {
    return;
  }
  timelineEdgeWindow.value = {
    ...page,
    sessionId: edgeWindow.sessionId,
    items: mergeTimelineEdgeWindowItems(current.items, page.items),
  };
  await nextTick();
}

async function navigateTimelineViewportUserMessage(
  direction: WebSessionUserMessageNavigationDirection
) {
  const session = currentRealSession.value;
  if (!session || timelineNavigationBusy.value) {
    return;
  }
  keepTimelineNavigationControlsVisible();
  clearTimelineStartConfirmation();
  const requestVersion = ++timelineNavigationRequestVersion;
  timelineNavigationPending.value = direction;
  try {
    while (isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      const targetKey = findTimelineViewportUserMessageTarget(direction);
      if (targetKey) {
        scrollToTimelineUserMessage(targetKey);
        return;
      }

      const previousState = getUserMessageHistoryLoadStateKey();
      if (direction === 'previous' && !getCurrentTimelineEdgeWindow()) {
        if (!canLoadEarlierUserMessageHistory()) {
          return;
        }
        await loadEarlierHistoryForUserMessageNavigation();
      } else if (direction === 'next' && canLoadLaterTimelineEdgeHistory()) {
        await loadLaterTimelineEdgeHistory();
      } else {
        return;
      }
      if (!isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
        return;
      }
      if (getUserMessageHistoryLoadStateKey() === previousState) {
        return;
      }
      await nextTick();
    }
  } catch (error) {
    if (isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      pendingHistoryAnchor.value = null;
      message.error(error instanceof Error ? error.message : t('common.error'));
    }
  } finally {
    if (isTimelineNavigationRequestCurrent(requestVersion, session.id)) {
      timelineNavigationPending.value = null;
      refreshTimelineViewportNavigation();
    }
  }
}

function scrollToTimelineBlock(targetKey: string) {
  const container = timelineScrollRef.value;
  const element = timelineBlockElements.get(targetKey);
  if (!container || !element) {
    return;
  }
  const containerRect = container.getBoundingClientRect();
  const elementRect = element.getBoundingClientRect();
  const targetTop = container.scrollTop + (elementRect.top - containerRect.top) - 12;
  invalidateTimelineScrollSync();
  autoFollowBottom.value = false;
  showJumpToBottom.value = true;
  lastTimelineScrollTop.value = container.scrollTop;
  container.scrollTo({
    top: Math.max(0, targetTop),
    behavior: 'smooth',
  });
}

async function locateSubAgent(agent: WebSessionSubAgent) {
  await nextTick();
  const target = findLatestSubAgentActivityBlock(visibleBlocks.value, agent.id);
  if (target) {
    scrollToTimelineBlock(target.key);
    return;
  }
  message.info(t('webSession.subAgentNoTimelineActivity'));
}

function subAgentLatestActivitySummary(agent: WebSessionSubAgent) {
  const target = findLatestSubAgentActivityBlock(timelineBlocks.value, agent.id);
  if (target) {
    return subAgentActivitySummary(target);
  }
  return isTransportRetryActivityText(agent.summary) ? '' : agent.summary;
}

function subAgentFallbackSummary(agent: WebSessionSubAgent) {
  return agent.status === 'pending_init' || agent.status === 'running'
    ? t('webSession.liveSubAgentNoSummary')
    : t('webSession.subAgentNoSummary');
}

const timelineUserMessageEditTitle = computed(() =>
  isRunActive.value ? t('webSession.editUserMessageRunning') : t('webSession.editUserMessage')
);
const messageEditCanSubmit = computed(() => {
  const editing = editingUserMessage.value;
  if (!editing || messageEditSubmitting.value) {
    return false;
  }
  return messageEditText.value.trim().length > 0 || editing.block.attachments.length > 0;
});

function canEditTimelineUserMessage(block: WebSessionBlock) {
  const session = currentRealSession.value;
  return Boolean(
    block.kind === 'user' &&
      !block.deliveryState &&
      session?.agent === 'codex' &&
      session.nativeSessionId
  );
}

function openTimelineUserMessageEdit(block: WebSessionBlock) {
  const session = currentRealSession.value;
  if (!session || !canEditTimelineUserMessage(block) || isRunActive.value) {
    return;
  }
  editingUserMessage.value = {
    projectId: session.projectId,
    sessionId: session.id,
    block,
  };
  messageEditText.value = block.text;
  showMessageEditDialog.value = true;
}

function handleMessageEditDialogVisibilityChange(show: boolean) {
  if (!show && messageEditSubmitting.value) {
    return;
  }
  showMessageEditDialog.value = show;
  if (!show) {
    editingUserMessage.value = null;
    messageEditText.value = '';
  }
}

async function handleConfirmMessageEdit() {
  const editing = editingUserMessage.value;
  if (!editing || !messageEditCanSubmit.value) {
    return;
  }
  messageEditSubmitting.value = true;
  try {
    const snapshot = await webSessionStore.editUserMessage(
      editing.projectId,
      editing.sessionId,
      editing.block.id,
      messageEditText.value
    );
    const branch = snapshot.session;
    if (!branch) {
      throw new Error(t('common.error'));
    }
    showMessageEditDialog.value = false;
    editingUserMessage.value = null;
    messageEditText.value = '';
    if (editing.projectId !== props.projectId) {
      projectStore.addRecentProject(editing.projectId);
      await router.push(buildProjectRouteLocation(editing.projectId, branch.id));
    } else {
      clearArchivedPreviewSession();
      activeArchivedPreviewId.value = '';
      await nextTick();
      insertTabAfter(branch.id, editing.sessionId);
      await activateTabById(branch.id, { connectReal: false });
      await syncWebSessionRouteSessionId(branch.id);
      autoFollowBottom.value = true;
      scrollToBottom(true);
    }
    message.success(t('webSession.editUserMessageSuccess'));
  } catch (error) {
    message.error(formatSessionInteractionError(error));
  } finally {
    messageEditSubmitting.value = false;
  }
}

function canLoadEarlierUserMessageHistory() {
  return Boolean(
    currentRealSession.value &&
      !getCurrentTimelineEdgeWindow() &&
      historyMeta.value.hasMore &&
      historyMeta.value.beforeCursor &&
      !historyMeta.value.loading &&
      !pendingHistoryAnchor.value
  );
}

function isTimelineUserMessageNavigationLoading(
  block: WebSessionBlock,
  direction: WebSessionUserMessageNavigationDirection
) {
  return (
    userMessageNavigationPending.value?.key === block.key &&
    userMessageNavigationPending.value.direction === direction
  );
}

function canNavigateTimelineUserMessage(
  block: WebSessionBlock,
  direction: WebSessionUserMessageNavigationDirection
) {
  if (
    userMessageNavigationPending.value ||
    timelineNavigationBusy.value ||
    historyMeta.value.loading
  ) {
    return false;
  }
  return canNavigateWebSessionUserMessage({
    blocks: visibleBlocks.value,
    currentKey: block.key,
    direction,
    canLoadEarlier: canLoadEarlierUserMessageHistory(),
    canLoadLater: canLoadLaterTimelineEdgeHistory(),
  });
}

function getUserMessageHistoryLoadStateKey() {
  const edgeWindow = getCurrentTimelineEdgeWindow();
  if (edgeWindow) {
    return [
      'edge',
      edgeWindow.afterCursor,
      edgeWindow.hasLater ? 'later' : 'end',
      edgeWindow.items.length,
      edgeWindow.items[edgeWindow.items.length - 1]?.key ?? '',
    ].join(':');
  }
  const firstBlockKey = blocks.value[0]?.key ?? '';
  return [
    historyMeta.value.beforeCursor,
    historyMeta.value.hasMore ? 'more' : 'end',
    blocks.value.length,
    firstBlockKey,
  ].join(':');
}

async function loadEarlierHistoryForUserMessageNavigation() {
  const session = currentRealSession.value;
  const container = timelineScrollRef.value;
  if (!session || !container) {
    return;
  }
  pendingHistoryAnchor.value = {
    sessionId: session.id,
    previousHeight: container.scrollHeight,
    previousTop: container.scrollTop,
  };
  await webSessionStore.loadMoreHistory(session.id);
  await nextTick();
  restoreHistoryAnchor();
}

async function loadLaterHistoryForUserMessageNavigation() {
  await loadLaterTimelineEdgeHistory();
}

function scrollToTimelineUserMessage(targetKey: string) {
  const container = timelineScrollRef.value;
  const element = timelineUserMessageElements.get(targetKey);
  if (!container || !element) {
    return;
  }
  const containerRect = container.getBoundingClientRect();
  const elementRect = element.getBoundingClientRect();
  const targetTop = container.scrollTop + (elementRect.top - containerRect.top);
  invalidateTimelineScrollSync();
  autoFollowBottom.value = false;
  showJumpToBottom.value = true;
  lastTimelineScrollTop.value = container.scrollTop;
  container.scrollTo({
    top: Math.max(0, targetTop),
    behavior: 'smooth',
  });
}

async function navigateTimelineUserMessage(
  block: WebSessionBlock,
  direction: WebSessionUserMessageNavigationDirection
) {
  if (!canNavigateTimelineUserMessage(block, direction)) {
    return;
  }
  userMessageNavigationPending.value = { key: block.key, direction };
  try {
    const targetKey = await resolveWebSessionUserMessageTarget({
      currentKey: block.key,
      direction,
      getBlocks: () => visibleBlocks.value,
      canLoadEarlier: canLoadEarlierUserMessageHistory,
      canLoadLater: canLoadLaterTimelineEdgeHistory,
      getLoadStateKey: getUserMessageHistoryLoadStateKey,
      loadEarlier: loadEarlierHistoryForUserMessageNavigation,
      loadLater: loadLaterHistoryForUserMessageNavigation,
    });
    if (!targetKey) {
      return;
    }
    await nextTick();
    scrollToTimelineUserMessage(targetKey);
  } catch (error) {
    pendingHistoryAnchor.value = null;
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    userMessageNavigationPending.value = null;
  }
}

const draftAttachments = computed(() =>
  webSessionStore.getDraftAttachments(props.projectId, currentDraftSessionId.value)
);
const sendConflictSessions = computed(() => {
  const projectId = sendGuardProjectId.value;
  if (!projectId) {
    return [];
  }
  return findWebSessionSendConflicts({
    currentSessionId: currentRealSession.value?.id ?? '',
    sessions: webSessionStore.getSessions(projectId).map(session => ({
      id: session.id,
      title: session.title,
      workflowMode: session.workflowMode,
      livePhase: webSessionStore.getLiveState(session.id).phase,
    })),
  });
});
const draftAttachmentUpload = computed(() =>
  webSessionStore.getDraftAttachmentUpload(props.projectId, currentDraftSessionId.value)
);
const isComposerPastePending = computed(
  () => (composerPastePendingBySession.value[currentDraftSessionId.value] ?? 0) > 0
);
const isDraftAttachmentUploading = computed(
  () => Boolean(draftAttachmentUpload.value) || isComposerPastePending.value
);
const composerTransferCard = computed(() => {
  if (draftAttachmentUpload.value) {
    const upload = draftAttachmentUpload.value;
    return {
      tone: 'progress' as const,
      message:
        upload.totalFiles > 1
          ? t('webSession.attachmentUploadingBatch', {
              current: upload.currentFileIndex,
              total: upload.totalFiles,
            })
          : t('webSession.attachmentUploading'),
      detail: '',
      progress: upload.percent ?? 0,
    };
  }
  if (isComposerPastePending.value) {
    return {
      tone: 'progress' as const,
      message: t('webSession.attachmentUploading'),
      detail: '',
      progress: 0,
    };
  }
  if (composerTransferErrorMessage.value) {
    return {
      tone: 'error' as const,
      message: composerTransferErrorMessage.value,
      detail: '',
      progress: null,
    };
  }
  return null;
});
const composerTransferDialogStyle = computed(() => {
  return {
    '--terminal-transfer-card-bg': 'var(--app-surface-raised, #fff)',
    '--terminal-transfer-card-fg': 'var(--app-text-primary, #333)',
    '--terminal-transfer-card-border': 'var(--app-border, #e0e0e0)',
    '--terminal-transfer-card-track': 'var(--app-surface-sunken, #f2f3f5)',
  } as CSSProperties;
});
function draftAttachmentDisplayName(attachment: { name: string }, index: number) {
  return resolveImageAttachmentDisplayName(attachment.name, index + 1);
}
function openDraftAttachmentPreview(
  attachment: { id: string; name: string; mime?: string },
  index: number
) {
  openAttachmentPreview({
    ...attachment,
    name: draftAttachmentDisplayName(attachment, index),
  });
}
const pendingInputs = computed(() =>
  currentRealSession.value ? webSessionStore.getPendingInputs(currentRealSession.value.id) : []
);
const localPendingInputs = computed(() =>
  pendingInputs.value.filter(item => !item.nativeQueued && item.status !== 'persisting')
);
const pendingInputEditorAutosize = { minRows: 2, maxRows: 3 };
const pendingInputPopoverId = ref('');
const pendingEditingId = ref('');
const pendingEditText = ref('');
const pendingEditActionId = ref('');
const pendingEditCanSave = computed(() => pendingEditText.value.trim().length > 0);
const scheduledInputs = computed(() =>
  currentRealSession.value ? webSessionStore.getScheduledInputs(currentRealSession.value.id) : []
);
const activeScheduledPlanTargetIds = computed(
  () =>
    new Set(
      scheduledInputs.value
        .filter(item => item.action === 'execute_plan' && item.status === 'scheduled')
        .map(item => item.targetId)
        .filter(Boolean)
    )
);
const currentScheduledPlanTarget = computed<ScheduledPlanDialogTarget | null>(() => {
  const sessionId = currentRealSession.value?.id ?? '';
  const planItemId = latestPlanItemId.value;
  if (!sessionId || !planItemId) {
    return null;
  }
  const executeOption = inlinePlanChoice.value?.options.find(option => option.isExecute);
  return {
    sessionId,
    planItemId,
    ...(inlinePlanChoice.value?.questionId && executeOption
      ? {
          pendingItemId: pendingUserInput.value?.itemId,
          questionId: inlinePlanChoice.value.questionId,
          executeOptionLabel: executeOption.label,
        }
      : {}),
  };
});
const currentSessionLatestEventSeq = computed(() =>
  currentRealSession.value ? webSessionStore.getLatestEventSeq(currentRealSession.value.id) : 0
);
const currentComposerSubmitKind = computed(() => resolveComposerSubmitKind());
const sendConfirmationSignature = computed(() =>
  buildWebSessionSendConfirmationSignature({
    ownerId: currentDraftSessionId.value,
    text: composerText.value,
    attachmentIds: draftAttachments.value.map(item => item.id),
    conflictSessionIds: sendConflictSessions.value.map(session => session.id),
  })
);
const planImplementConfirmationSignature = computed(() =>
  buildWebSessionSendConfirmationSignature({
    ownerId: currentRealSession.value?.id ?? '',
    text: '__implement_plan__',
    attachmentIds: [],
    conflictSessionIds: sendConflictSessions.value.map(session => session.id),
  })
);
const isSendConflictConfirmationArmed = computed(
  () =>
    currentComposerSubmitKind.value === 'execute_send' &&
    sendConflictSessions.value.length > 0 &&
    Boolean(
      sendConfirmationState.value &&
        sendConfirmationState.value.signature === sendConfirmationSignature.value
    )
);
const isSubmittingMessage = computed(() =>
  isWebSessionSubmitting(submitStateBySessionId.value, currentDraftSessionId.value)
);
const isSubmittingRedirectedMessage = computed(
  () => currentSubmitEntry.value?.kind === 'redirect_message'
);
const isSubmittingQueuedMessage = computed(
  () => currentSubmitEntry.value?.kind === 'queue_message'
);
const isRunActive = computed(() => liveState.value.running);
const canOpenPiTree = computed(() => {
  const session = currentRealSession.value;
  return canOpenWebSessionPiTree({
    archived: Boolean(session?.archivedAt),
    agent: session?.agent,
    supportsTree: runtimePiCapability.value.supportsTree,
    nativeSessionId: session?.nativeSessionId ?? undefined,
    threadPath: session?.threadPath ?? undefined,
  });
});
const canMutatePiTree = computed(() =>
  canMutateWebSessionPiTree({
    canOpen: canOpenPiTree.value,
    running: isRunActive.value,
    pendingCount: pendingInputs.value.length,
  })
);
const hasDraftContent = computed(
  () => composerText.value.trim().length > 0 || draftAttachments.value.length > 0
);
const canSend = computed(
  () =>
    !isMessageCapabilityBlocked.value &&
    !isRunActive.value &&
    !isSubmittingMessage.value &&
    hasDraftContent.value &&
    !isDraftAttachmentUploading.value
);
const canStageDuringRun = computed(
  () =>
    !isMessageCapabilityBlocked.value &&
    isRunActive.value &&
    !isSubmittingMessage.value &&
    hasDraftContent.value &&
    !isDraftAttachmentUploading.value
);
const canOpenSendQuickActions = computed(() =>
  isRunActive.value ? canStageDuringRun.value : canSend.value
);
const sendQuickActionOptions = computed<DropdownOption[]>(() => {
  const options: DropdownOption[] = [];
  if (isRunActive.value) {
    options.push(
      {
        key: 'redirect',
        label: t('webSession.preinputRedirect'),
      },
      {
        key: 'queue',
        label: t('webSession.preinputQueue'),
      }
    );
  } else {
    options.push({
      key: 'send',
      label: isSendConflictConfirmationArmed.value
        ? t('webSession.sendEmphatic')
        : t('webSession.send'),
    });
  }
  options.push({
    key: 'schedule',
    label: t('webSession.scheduleSend'),
  });
  return options;
});
const planQuickActionOptions = computed<DropdownOption[]>(() => [
  {
    key: 'schedule-plan',
    label: t('webSession.planActionSchedule'),
  },
]);
const isScheduledDialogPlan = computed(
  () => scheduledSendPurpose.value === 'execute_plan' || scheduledSendPurpose.value === 'edit_plan'
);
const isScheduledDialogEdit = computed(
  () => scheduledSendPurpose.value === 'edit_message' || scheduledSendPurpose.value === 'edit_plan'
);
const scheduledDialogTitle = computed(() => {
  switch (scheduledSendPurpose.value) {
    case 'execute_plan':
      return t('webSession.planScheduleTitle');
    case 'edit_plan':
      return t('webSession.scheduledPlanEditTitle');
    case 'edit_message':
      return t('webSession.scheduledEditTitle');
    default:
      return t('webSession.scheduleSend');
  }
});
const scheduledDialogConfirmLabel = computed(() => {
  switch (scheduledSendPurpose.value) {
    case 'execute_plan':
      return t('webSession.planScheduleConfirm');
    case 'edit_plan':
      return t('webSession.scheduledPlanEditConfirm');
    case 'edit_message':
      return t('webSession.scheduledEditConfirm');
    default:
      return t('webSession.scheduleSendConfirm');
  }
});
const selectedScheduledSendPresetKey = computed(
  () =>
    scheduledSendPresetOptions.value.find(option => option.timestamp === scheduledSendAt.value)
      ?.key ?? ''
);
const scheduledSendSelectedTimeLabel = computed(() =>
  scheduledSendAt.value ? formatWebSessionDateTime(scheduledSendAt.value, locale.value) : ''
);
const scheduledDialogSelectedTimeLabel = computed(() =>
  isScheduledDialogPlan.value
    ? t('webSession.planScheduleSelectedTime', { time: scheduledSendSelectedTimeLabel.value })
    : t('webSession.scheduleSendSelectedTime', { time: scheduledSendSelectedTimeLabel.value })
);
const canConfirmScheduledSend = computed(() => {
  const requiresScheduledTime = scheduledScheduleKind.value === 'at_time';
  if (
    isMessageCapabilityBlocked.value ||
    (requiresScheduledTime &&
      (!Number.isFinite(scheduledSendAt.value) || Number(scheduledSendAt.value) <= Date.now())) ||
    scheduledSendSubmitting.value
  ) {
    return false;
  }
  if (isScheduledDialogEdit.value) {
    const editing = scheduledEditingInput.value;
    const current = editing
      ? scheduledInputs.value.find(item => item.id === editing.id)
      : undefined;
    if (!current || (current.status !== 'scheduled' && current.status !== 'failed')) {
      return false;
    }
    if (scheduledSendPurpose.value === 'edit_plan') {
      return current.action === 'execute_plan';
    }
    return (
      current.action === 'message' &&
      (scheduledEditText.value.trim().length > 0 || current.attachmentIds.length > 0)
    );
  }
  if (scheduledSendPurpose.value === 'execute_plan') {
    const target = scheduledPlanDialogTarget.value;
    return Boolean(
      target &&
        target.sessionId === currentRealSession.value?.id &&
        target.planItemId === latestPlanItemId.value &&
        !activeScheduledPlanTargetIds.value.has(target.planItemId)
    );
  }
  return hasDraftContent.value && !isDraftAttachmentUploading.value;
});
const composerMinRows = computed(() => (isMobile.value ? 1 : 3));
const composerMaxRows = computed(() => (isMobile.value ? 8 : 10));
const composerPlaceholder = computed(() =>
  isMobile.value
    ? locale.value === 'zh-CN'
      ? '输入消息'
      : 'Type a message'
    : t('webSession.inputPlaceholder')
);
const composerHint = computed(() => {
  if (codexRuntimeConfig.value && selectedAgent.value === 'codex' && !runtimeHasCodex.value) {
    return t('webSession.composerHintCodexMissing');
  }
  if (codexRuntimeConfig.value && selectedAgent.value === 'claude' && !runtimeHasClaudeCode.value) {
    return t('webSession.composerHintClaudeMissing');
  }
  if (selectedAgent.value === 'pi' && !runtimePiCapability.value.supportsWebSession) {
    return piUnavailableReason.value;
  }
  if (isDraftAttachmentUploading.value) {
    return t('webSession.composerHintUploading');
  }
  if (hasRecoveredRuntimeRequest.value) {
    return t('webSession.composerHintRecovered');
  }
  if (
    pendingApproval.value ||
    liveState.value.phase === 'waiting_approval' ||
    liveState.value.phase === 'waiting_plan_approval'
  ) {
    return t('webSession.composerHintApproval');
  }
  if (pendingUserInput.value) {
    return t('webSession.composerHintUserInput');
  }
  if (liveState.value.running) {
    return t('webSession.composerHintRunning');
  }
  return t('webSession.composerHintIdle');
});
const quickInputPinnedItems = computed(() => webSessionQuickInput.value.pinned);
const quickInputRecentItems = computed(() => {
  const pinned = new Set(quickInputPinnedItems.value);
  const recent =
    quickInputScope.value === 'project' && props.projectId
      ? (webSessionQuickInput.value.recentByProject[props.projectId] ?? [])
      : webSessionQuickInput.value.recent;
  return recent.filter(text => !pinned.has(text));
});
const hasQuickInputOptions = computed(
  () => quickInputPinnedItems.value.length > 0 || quickInputRecentItems.value.length > 0
);
const quickInputButtonTitle = computed(() =>
  hasQuickInputOptions.value ? t('webSession.quickInput') : t('webSession.quickInputUnavailable')
);
const quickInputDirectSendEnabled = computed({
  get: () => webSessionQuickInputDirectSend.value,
  set: value => {
    settingsStore.updateWebSessionQuickInputDirectSend(value === true);
  },
});
const quickInputAllItems = computed(() => [
  ...quickInputPinnedItems.value,
  ...quickInputRecentItems.value,
]);
const quickInputItems = computed(() => {
  return filterWebSessionQuickInputItems(quickInputAllItems.value, quickInputSearch.value);
});
const quickInputPageCount = computed(() =>
  Math.max(1, Math.ceil(quickInputItems.value.length / WEB_SESSION_QUICK_INPUT_PAGE_SIZE))
);
const quickInputVisibleItems = computed(() => {
  return paginateWebSessionQuickInputItems(
    quickInputItems.value,
    quickInputPage.value,
    WEB_SESSION_QUICK_INPUT_PAGE_SIZE
  );
});
const quickInputEmptyLabel = computed(() =>
  quickInputSearch.value.trim()
    ? t('webSession.quickInputSearchEmpty')
    : t('webSession.quickInputEmpty')
);
watch([quickInputScope, quickInputSearch, () => props.projectId], () => {
  quickInputPage.value = 1;
});
watch(quickInputPageCount, pageCount => {
  quickInputPage.value = Math.min(quickInputPage.value, pageCount);
});
const normalizedComposerText = computed(() => composerText.value.trim());
const selectedAgentLabel = computed(
  () =>
    agentOptions.find(option => option.value === selectedAgent.value)?.label ?? selectedAgent.value
);
const selectedModelLabel = computed(
  () => getKnownModelLabel(selectedModel.value) || t('common.default')
);
const selectedWorkflowModeLabel = computed(() =>
  selectedWorkflowMode.value === 'plan'
    ? t('webSession.workflowPlan')
    : t('webSession.workflowDefault')
);
const mobileComposerPanelToggleLabel = computed(() =>
  isMobileComposerCollapsed.value
    ? t('webSession.composerPanelExpand')
    : t('webSession.composerPanelCollapse')
);
const mobileComposerSettingsToggleLabel = computed(() =>
  isMobileComposerSettingsExpanded.value
    ? t('webSession.composerSettingsCollapse')
    : t('webSession.composerSettingsExpand')
);
const mobileComposerSummaryTokens = computed(() => [
  { key: 'agent', label: selectedAgentLabel.value },
  { key: 'model', label: selectedModelLabel.value },
  { key: 'workflow', label: selectedWorkflowModeLabel.value },
]);
const mobileComposerPendingSummary = computed(() => {
  return buildWebSessionMobilePendingSummary(pendingInputs.value, scheduledInputs.value).map(
    item => ({
      ...item,
      label: t(
        item.kind === 'redirect'
          ? 'webSession.pendingRedirectCount'
          : item.kind === 'queue'
            ? 'webSession.pendingQueueCount'
            : 'webSession.scheduledCount',
        { count: item.count }
      ),
    })
  );
});

type ContextUsageIndicator = {
  state: 'idle' | 'active' | 'warning' | 'unavailable';
  label: string;
  title: string;
  available: boolean;
  hasUsage: boolean;
  cumulativeNonCachedTokens: number;
  totalTokens: number;
  usedTokens: number;
  contextWindowTokens: number;
  compactLimitTokens: number;
  usedPercent: number;
  compactPercent: number;
  showCompactMarker: boolean;
};

const contextUsageIndicator = computed<ContextUsageIndicator | null>(() => {
  const session = currentSession.value;
  if (!session) {
    return null;
  }
  if (session.agent === 'codex' && isDraftSession(session) && !codexRuntimeConfigReady.value) {
    return null;
  }

  const inputTokens = Math.max(0, Number(session.usage.inputTokens || 0));
  const cachedInputTokens = Math.max(0, Number(session.usage.cachedInputTokens || 0));
  const outputTokens = Math.max(0, Number(session.usage.outputTokens || 0));
  const cumulativeNonCachedTokens = calculateBillableTokenUsage(
    inputTokens,
    cachedInputTokens,
    outputTokens
  );
  const totalTokens = calculateTotalTokenUsage(inputTokens, outputTokens);
  const hasUsage = inputTokens > 0 || cachedInputTokens > 0 || outputTokens > 0;
  const unavailable = {
    state: 'unavailable' as const,
    label: t('webSession.contextUsageLabelUnavailable'),
    title: t('webSession.contextUsageUnavailableTitle'),
    available: false,
    hasUsage,
    cumulativeNonCachedTokens,
    totalTokens,
    usedTokens: 0,
    contextWindowTokens: 0,
    compactLimitTokens: 0,
    usedPercent: 0,
    compactPercent: 0,
    showCompactMarker: false,
  };
  const runtimeConfig = codexRuntimeConfig.value;
  const sessionSource =
    session.contextWindowSource === 'session_usage' ||
    session.contextWindowSource === 'config' ||
    session.contextWindowSource === 'model_catalog' ||
    session.contextWindowSource === 'default' ||
    session.contextWindowSource === 'unavailable'
      ? session.contextWindowSource
      : ('unavailable' as WebSessionContextWindowSource);
  const sessionWindowTokens =
    typeof session.contextWindowTokens === 'number' &&
    Number.isFinite(session.contextWindowTokens) &&
    session.contextWindowTokens > 0
      ? session.contextWindowTokens
      : null;
  const runtimeConfigMatchesModel =
    session.agent === 'codex' &&
    typeof runtimeConfig?.model === 'string' &&
    runtimeConfig.model.trim() !== '' &&
    runtimeConfig.model.trim().toLowerCase() === session.model.trim().toLowerCase();
  const runtimeWindowTokens =
    runtimeConfigMatchesModel &&
    typeof runtimeConfig?.contextWindowTokens === 'number' &&
    Number.isFinite(runtimeConfig.contextWindowTokens) &&
    runtimeConfig.contextWindowTokens > 0
      ? runtimeConfig.contextWindowTokens
      : null;
  const canUseSessionWindow =
    sessionWindowTokens !== null &&
    (sessionSource === 'session_usage' ||
      sessionSource === 'config' ||
      sessionSource === 'model_catalog');
  const contextWindowTokens = canUseSessionWindow ? sessionWindowTokens : runtimeWindowTokens;
  const runtimeCompactLimitTokens =
    runtimeConfigMatchesModel &&
    typeof runtimeConfig?.compactLimitTokens === 'number' &&
    Number.isFinite(runtimeConfig.compactLimitTokens) &&
    runtimeConfig.compactLimitTokens > 0
      ? runtimeConfig.compactLimitTokens
      : null;
  const compactLimitTokens =
    sessionSource === 'session_usage'
      ? contextWindowTokens
      : (runtimeCompactLimitTokens ?? contextWindowTokens);
  if (
    (session.agent !== 'codex' && session.agent !== 'claude') ||
    !contextWindowTokens ||
    !compactLimitTokens
  ) {
    return unavailable;
  }

  const usedTokens = Math.max(0, Number(session.contextEstimate.usedTokens || 0));
  const { remainingPercent } = calculateCodexRemainingContext({
    compactLimitTokens,
    usedTokens,
    baselineTokens: session.agent === 'claude' ? 0 : undefined,
  });
  const usedPercent = Math.max(
    0,
    Math.min(100, Math.round((usedTokens / contextWindowTokens) * 100))
  );
  const compactPercent = Math.max(
    0,
    Math.min(100, Math.round((compactLimitTokens / contextWindowTokens) * 100))
  );
  const showCompactMarker =
    compactLimitTokens > 0 &&
    compactLimitTokens < contextWindowTokens &&
    Math.abs(contextWindowTokens - compactLimitTokens) >= 1;

  return {
    state: remainingPercent <= 10 ? 'warning' : remainingPercent <= 25 ? 'active' : 'idle',
    label: t('webSession.contextUsageLabel', { percent: remainingPercent }),
    title: t(
      session.agent === 'claude'
        ? 'webSession.contextUsageTitleClaude'
        : 'webSession.contextUsageTitle'
    ),
    available: true,
    hasUsage,
    cumulativeNonCachedTokens,
    totalTokens,
    usedTokens,
    contextWindowTokens,
    compactLimitTokens,
    usedPercent,
    compactPercent,
    showCompactMarker,
  };
});

function formatContextTokenCount(value: number, key: ContextNumberKey) {
  return formatWebSessionTokenCount(value, contextExactNumbers.value[key]);
}

function toggleContextNumber(key: ContextNumberKey) {
  contextExactNumbers.value = {
    ...contextExactNumbers.value,
    [key]: !contextExactNumbers.value[key],
  };
}

function contextNumberTitle(key: ContextNumberKey) {
  return contextExactNumbers.value[key]
    ? t('webSession.contextUsageShowCompact')
    : t('webSession.contextUsageShowExact');
}

const currentWorkRunDurationMs = computed<number | null>(() => {
  const currentRun = currentRealSession.value?.workTiming?.currentRun;
  const startedAt = Date.parse(currentRun?.startedAt ?? '');
  if (!currentRun || !Number.isFinite(startedAt) || startedAt <= 0) {
    return null;
  }
  const pausedAt = Date.parse(currentRun.pausedAt ?? '');
  const end =
    Number.isFinite(pausedAt) && pausedAt > 0
      ? Math.min(liveStateClockMs.value, pausedAt)
      : liveStateClockMs.value;
  return Math.max(0, end - startedAt - Math.max(0, Number(currentRun.pausedDurationMs) || 0));
});

const currentWorkTimingDurationMs = computed(() => {
  const completed = Math.max(
    0,
    Number(currentRealSession.value?.workTiming?.completedDurationMs ?? 0) || 0
  );
  return completed + (currentWorkRunDurationMs.value ?? 0);
});

const workTimingStatusLabel = computed(() => {
  if (workTimingCalculationError.value) {
    return t('webSession.workTimingCalculationFailed');
  }
  if (workTimingCalculationLoading.value) {
    return t('webSession.workTimingCalculating');
  }
  const state = currentRealSession.value?.workTiming?.backfillState ?? 'pending';
  if (state === 'partial') {
    return t('webSession.workTimingRecordedPartial');
  }
  if (state === 'unavailable') {
    return t('webSession.workTimingUnavailable');
  }
  if (state === 'failed') {
    return t('webSession.workTimingCalculationFailed');
  }
  if (state === 'pending') {
    return t('webSession.workTimingPending');
  }
  return t('webSession.workTimingComplete');
});

function formatWorkDuration(valueMs: number) {
  return formatElapsedDuration(0, Math.max(0, Number(valueMs) || 0));
}

async function calculateWorkTimingOnPopoverOpen() {
  const session = currentRealSession.value;
  if (
    !session ||
    isDraftSession(session) ||
    workTimingCalculationLoading.value ||
    (session.workTiming?.backfillState !== 'pending' &&
      session.workTiming?.backfillState !== 'failed')
  ) {
    return;
  }
  workTimingCalculationLoading.value = true;
  workTimingCalculationError.value = '';
  let retryAfterBusyResponse = false;
  try {
    const result = await webSessionStore.calculateSessionWorkTiming(session.projectId, session.id);
    if (result.status === 'busy') {
      if (
        showContextUsagePopover.value &&
        workTimingSessionBusy.value === false &&
        workTimingBusyRetryConsumed.value === false
      ) {
        workTimingBusyRetryConsumed.value = true;
        retryAfterBusyResponse = true;
      } else {
        workTimingBusyRetryPending.value = true;
      }
      return;
    }
    if (result.status === 'failed') {
      workTimingCalculationError.value = result.error || 'failed';
    }
  } catch (error) {
    workTimingCalculationError.value = error instanceof Error ? error.message : String(error);
  } finally {
    workTimingCalculationLoading.value = false;
    if (retryAfterBusyResponse && showContextUsagePopover.value) {
      void calculateWorkTimingOnPopoverOpen();
    }
  }
}

function handleContextUsagePopoverVisibilityChange(show: boolean) {
  showContextUsagePopover.value = show;
  if (!show) {
    workTimingBusyRetryPending.value = false;
    workTimingBusyRetryConsumed.value = false;
    return;
  }
  workTimingBusyRetryConsumed.value = false;
  void calculateWorkTimingOnPopoverOpen();
}

function setMobileComposerFocusState(focused: boolean) {
  if (isMobileComposerFocused.value === focused) {
    return;
  }
  isMobileComposerFocused.value = focused;
}

function emitMobileComposerChromeHidden(hidden: boolean) {
  if (lastEmittedMobileComposerChromeHidden === hidden) {
    return;
  }
  lastEmittedMobileComposerChromeHidden = hidden;
  emit('mobile-composer-focus-change', hidden);
}

function ensureMobileComposerVisible() {
  if (!isMobile.value) {
    return;
  }
  isMobileComposerCollapsed.value = false;
}

function resetMobileComposerScrollState(container = timelineScrollRef.value) {
  mobileComposerScrollState.value = container
    ? createWebSessionMobileComposerScrollState(readTimelineScrollMetrics(container))
    : null;
}

function collapseMobileComposerPanel() {
  if (!isMobile.value) {
    return;
  }
  isMobileComposerCollapsed.value = true;
  showQuickInputPopover.value = false;
  isMobileComposerSettingsExpanded.value = false;
  mobileKeyboard.setFocused(false);
  setMobileComposerFocusState(false);
}

function expandMobileComposerPanel() {
  if (!isMobile.value) {
    return;
  }
  ensureMobileComposerVisible();
}

function toggleMobileComposerCollapsed() {
  if (!isMobile.value) {
    return;
  }
  const nextCollapsed = !isMobileComposerCollapsed.value;
  if (nextCollapsed) {
    collapseMobileComposerPanel();
  } else {
    expandMobileComposerPanel();
  }
  resetMobileComposerScrollState();
}

function toggleMobileComposerSettingsExpanded() {
  if (!isMobile.value) {
    return;
  }
  ensureMobileComposerVisible();
  isMobileComposerSettingsExpanded.value = !isMobileComposerSettingsExpanded.value;
}

function prepareQuickInputPopover() {
  quickInputScope.value = props.projectId ? 'project' : 'global';
  quickInputSearch.value = '';
  quickInputPage.value = 1;
}

function handleQuickInputPopoverVisibilityChange(show: boolean) {
  if (show) {
    prepareQuickInputPopover();
  }
  showQuickInputPopover.value = show;
}

function handleMobileQuickInputClickOutside() {
  if (Date.now() - mobileQuickInputOpenedAt < MOBILE_COMPOSER_OVERLAY_OPEN_GUARD_MS) {
    return;
  }
  showQuickInputPopover.value = false;
}

function handleMobileQuickInputTrigger() {
  if (!isMobile.value) {
    return;
  }
  const nextShow = !showQuickInputPopover.value;
  if (nextShow) {
    prepareQuickInputPopover();
  }
  showQuickInputPopover.value = nextShow;
  if (nextShow) {
    mobileQuickInputOpenedAt = Date.now();
  }
}

function handleMobileAttachmentTrigger() {
  if (!isMobile.value) {
    return;
  }
  openFilePicker();
}

function handleMobileSkillTrigger() {
  if (!isMobile.value) {
    return;
  }
  handleSkillBrowserVisibilityChange(!showSkillBrowser.value);
}

function handleMobileSkillMaskClick() {
  if (Date.now() - mobileSkillBrowserOpenedAt < MOBILE_COMPOSER_OVERLAY_OPEN_GUARD_MS) {
    return;
  }
  handleSkillBrowserVisibilityChange(false);
}

async function ensureCodexSkillsLoaded(force = false) {
  if (codexSkillsLoading.value) {
    return;
  }
  if (!force && codexSkillsLoaded.value) {
    return;
  }

  codexSkillsLoading.value = true;
  try {
    codexSkills.value = await systemApi.listCodexSkills();
  } catch (error) {
    console.warn('[Web Session] Failed to load Codex skills', error);
  } finally {
    codexSkillsLoading.value = false;
    codexSkillsLoaded.value = true;
  }
}

function handleSkillBrowserVisibilityChange(nextShow: boolean) {
  showSkillBrowser.value = nextShow;
  if (nextShow) {
    if (isMobile.value) {
      mobileSkillBrowserOpenedAt = Date.now();
    }
    showQuickInputPopover.value = false;
    void ensureCodexSkillsLoaded();
  }
}

function clearComposerTransferError() {
  if (composerTransferErrorTimer != null) {
    window.clearTimeout(composerTransferErrorTimer);
    composerTransferErrorTimer = null;
  }
  composerTransferErrorMessage.value = '';
  composerTransferErrorDetail.value = '';
}

function beginSessionSubmit(ownerId: string, kind: WebSessionSubmitKind) {
  submitStateBySessionId.value = beginWebSessionSubmit(submitStateBySessionId.value, ownerId, {
    kind,
  });
}

function endSessionSubmit(ownerId: string) {
  submitStateBySessionId.value = endWebSessionSubmit(submitStateBySessionId.value, ownerId);
}

function transferSessionSubmit(fromOwnerId: string, toOwnerId: string) {
  submitStateBySessionId.value = transferWebSessionSubmit(
    submitStateBySessionId.value,
    fromOwnerId,
    toOwnerId
  );
}

function clearUserInputSlowHintTimer(ownerId = '') {
  const normalizedOwnerId = buildWebSessionSubmitOwnerId(ownerId);
  if (
    normalizedOwnerId &&
    activeUserInputSlowHintOwnerId &&
    activeUserInputSlowHintOwnerId !== normalizedOwnerId
  ) {
    return;
  }
  if (cancelUserInputSlowHint) {
    cancelUserInputSlowHint();
    cancelUserInputSlowHint = null;
  }
  activeUserInputSlowHintOwnerId = '';
}

function beginUserInputSubmit(ownerId: string) {
  const normalizedOwnerId = buildWebSessionSubmitOwnerId(ownerId);
  if (!normalizedOwnerId) {
    return;
  }
  userInputSubmitStateByOwnerId.value = beginWebSessionSubmit(
    userInputSubmitStateByOwnerId.value,
    normalizedOwnerId
  );
  userInputSlowStateByOwnerId.value = endWebSessionSubmit(
    userInputSlowStateByOwnerId.value,
    normalizedOwnerId
  );
  clearUserInputSlowHintTimer();
  activeUserInputSlowHintOwnerId = normalizedOwnerId;
  cancelUserInputSlowHint = scheduleWebSessionUserInputSlowHint(normalizedOwnerId, slowOwnerId => {
    cancelUserInputSlowHint = null;
    activeUserInputSlowHintOwnerId = '';
    userInputSlowStateByOwnerId.value = beginWebSessionSubmit(
      userInputSlowStateByOwnerId.value,
      slowOwnerId
    );
  });
}

function endUserInputSubmit(ownerId: string) {
  const normalizedOwnerId = buildWebSessionSubmitOwnerId(ownerId);
  if (!normalizedOwnerId) {
    return;
  }
  userInputSubmitStateByOwnerId.value = endWebSessionSubmit(
    userInputSubmitStateByOwnerId.value,
    normalizedOwnerId
  );
  clearUserInputSlowHintTimer(normalizedOwnerId);
  userInputSlowStateByOwnerId.value = endWebSessionSubmit(
    userInputSlowStateByOwnerId.value,
    normalizedOwnerId
  );
}

function clearSendConflictConfirmationTimer() {
  if (sendConfirmationTimer != null) {
    window.clearTimeout(sendConfirmationTimer);
    sendConfirmationTimer = null;
  }
}

function setSendConflictConfirmationState(nextState: WebSessionSendConfirmationState | null) {
  clearSendConflictConfirmationTimer();
  sendConfirmationState.value = nextState;
  if (!nextState) {
    return;
  }
  const delay = Math.max(0, nextState.expiresAt - Date.now());
  sendConfirmationTimer = window.setTimeout(() => {
    sendConfirmationTimer = null;
    if (sendConfirmationState.value?.signature === nextState.signature) {
      sendConfirmationState.value = null;
    }
  }, delay);
}

function clearSendConflictConfirmation() {
  setSendConflictConfirmationState(null);
}

function closeSendQuickActions() {
  showSendQuickActions.value = false;
  sendQuickActionAnchor.value = null;
}

function closePlanQuickActions() {
  showPlanQuickActions.value = false;
  planQuickActionAnchor.value = null;
}

function openSendQuickActionsFromElement(anchorEl: HTMLElement) {
  if (!canOpenSendQuickActions.value) {
    return;
  }
  const rect = anchorEl.getBoundingClientRect();
  sendQuickActionAnchor.value = anchorEl;
  sendQuickActionsX.value = Math.round(rect.right);
  sendQuickActionsY.value = Math.round(rect.top);
  showSendQuickActions.value = true;
}

function buildScheduledSendPresetOptions(now = Date.now()): ScheduledSendPresetOption[] {
  return [
    {
      key: '5m',
      label: t('webSession.schedulePreset5Minutes'),
      timestamp: now + 5 * 60_000,
    },
    {
      key: '10m',
      label: t('webSession.schedulePreset10Minutes'),
      timestamp: now + 10 * 60_000,
    },
    {
      key: '30m',
      label: t('webSession.schedulePreset30Minutes'),
      timestamp: now + 30 * 60_000,
    },
    {
      key: '1h',
      label: t('webSession.schedulePreset1Hour'),
      timestamp: now + 60 * 60_000,
    },
  ];
}

function openScheduledSendDialog(
  purpose: ScheduledSendPurpose = 'message',
  planTarget: ScheduledPlanDialogTarget | null = null
) {
  const presets = buildScheduledSendPresetOptions();
  scheduledSendPurpose.value = purpose;
  scheduledPlanDialogTarget.value = purpose === 'execute_plan' ? planTarget : null;
  scheduledEditingInput.value = null;
  scheduledEditText.value = '';
  scheduledSendPresetOptions.value = presets;
  scheduledSendAt.value =
    presets.find(option => option.key === '5m')?.timestamp ??
    presets[0]?.timestamp ??
    Date.now() + 5 * 60_000;
  scheduledSendMode.value = 'send';
  scheduledExitPlanMode.value = false;
  scheduledScheduleKind.value = 'at_time';
  scheduledSendSubmitting.value = false;
  showScheduledSendDialog.value = true;
  closeSendQuickActions();
  closePlanQuickActions();
}

function openScheduledInputEditDialog(item: WebSessionScheduledInput) {
  if (
    !currentRealSession.value ||
    item.status === 'expired' ||
    scheduledInputActionId.value === item.id
  ) {
    return;
  }
  const presets = buildScheduledSendPresetOptions();
  const fallbackTime =
    presets.find(option => option.key === '5m')?.timestamp ?? Date.now() + 5 * 60_000;
  scheduledSendPurpose.value = item.action === 'execute_plan' ? 'edit_plan' : 'edit_message';
  scheduledPlanDialogTarget.value = null;
  scheduledEditingInput.value = item;
  scheduledEditText.value = item.text;
  scheduledSendPresetOptions.value = presets;
  scheduledSendAt.value =
    item.scheduledFor != null && item.scheduledFor > Date.now() ? item.scheduledFor : fallbackTime;
  scheduledSendMode.value = item.mode;
  scheduledExitPlanMode.value = item.exitPlanMode;
  scheduledScheduleKind.value = item.scheduleKind;
  scheduledSendSubmitting.value = false;
  activeScheduledInputPopoverId.value = '';
  showScheduledSendDialog.value = true;
}

function handleScheduledSendDialogVisibilityChange(show: boolean) {
  showScheduledSendDialog.value = show;
  if (!show) {
    scheduledSendSubmitting.value = false;
    scheduledSendPurpose.value = 'message';
    scheduledPlanDialogTarget.value = null;
    scheduledEditingInput.value = null;
    scheduledEditText.value = '';
    scheduledExitPlanMode.value = false;
    scheduledScheduleKind.value = 'at_time';
  }
}

function handleScheduledSendPresetSelect(timestamp: number) {
  scheduledSendAt.value = timestamp;
}

function toggleScheduledInputPopover(inputId: string) {
  activeScheduledInputPopoverId.value =
    activeScheduledInputPopoverId.value === inputId ? '' : inputId;
}

function closeScheduledInputPopover() {
  activeScheduledInputPopoverId.value = '';
}

const sendQuickActionLongPress = createLongPressTracker({
  onLongPress: () => {
    if (!scheduledSendLongPressAnchor) {
      return;
    }
    openSendQuickActionsFromElement(scheduledSendLongPressAnchor);
  },
});

function handlePrimarySendPointerDown(event: PointerEvent) {
  if (event.button !== 0 || !canOpenSendQuickActions.value) {
    return;
  }
  scheduledSendLongPressAnchor = event.currentTarget as HTMLElement | null;
  sendQuickActionLongPress.pointerDown(event.pointerId, {
    clientX: event.clientX,
    clientY: event.clientY,
  });
}

function handlePrimarySendPointerMove(event: PointerEvent) {
  sendQuickActionLongPress.pointerMove(event.pointerId, {
    clientX: event.clientX,
    clientY: event.clientY,
  });
}

function handlePrimarySendPointerUp(event: PointerEvent) {
  sendQuickActionLongPress.pointerUp(event.pointerId);
}

function handlePrimarySendPointerCancel(event: PointerEvent) {
  sendQuickActionLongPress.pointerCancel(event.pointerId);
}

function handleSendQuickActionTriggerClick(event: MouseEvent) {
  const anchorEl = event.currentTarget as HTMLElement | null;
  if (!anchorEl || !canOpenSendQuickActions.value) {
    return;
  }
  if (showSendQuickActions.value && sendQuickActionAnchor.value === anchorEl) {
    closeSendQuickActions();
    return;
  }
  openSendQuickActionsFromElement(anchorEl);
}

function handlePlanQuickActionTriggerClick(event: MouseEvent) {
  const anchorEl = event.currentTarget as HTMLElement | null;
  if (!anchorEl || isSubmittingMessage.value) {
    return;
  }
  if (showPlanQuickActions.value && planQuickActionAnchor.value === anchorEl) {
    closePlanQuickActions();
    return;
  }
  const rect = anchorEl.getBoundingClientRect();
  planQuickActionAnchor.value = anchorEl;
  planQuickActionsX.value = Math.round(rect.right);
  planQuickActionsY.value = Math.round(rect.bottom);
  showPlanQuickActions.value = true;
}

function handlePlanQuickActionSelect(key: string | number) {
  const action = String(key || '').trim();
  closePlanQuickActions();
  if (action === 'schedule-plan') {
    const target = currentScheduledPlanTarget.value;
    if (target && !activeScheduledPlanTargetIds.value.has(target.planItemId)) {
      openScheduledSendDialog('execute_plan', target);
    }
  }
}

async function handlePrimarySendButtonClick() {
  if (sendQuickActionLongPress.consumeClick()) {
    return;
  }
  closeSendQuickActions();
  if (isRunActive.value) {
    await handlePreinput('redirect');
    return;
  }
  await handleSubmit();
}

async function handleSendQuickActionSelect(key: string | number) {
  const action = String(key || '').trim();
  closeSendQuickActions();
  switch (action) {
    case 'send':
      await handleSubmit();
      return;
    case 'redirect':
      await handlePreinput('redirect');
      return;
    case 'queue':
      await handlePreinput('queue');
      return;
    case 'schedule':
      openScheduledSendDialog();
      return;
    default:
      return;
  }
}

function showComposerTransferError(detail?: string) {
  clearComposerTransferError();
  composerTransferErrorMessage.value = t('webSession.attachmentUploadFailed');
  composerTransferErrorDetail.value = String(detail || '').trim();
  composerTransferErrorTimer = window.setTimeout(() => {
    composerTransferErrorTimer = null;
    clearComposerTransferError();
  }, 900);
}

async function handleQuickInputApply(text: string) {
  showQuickInputPopover.value = false;
  const applied = await applyQuickInputText(text);
  if (!applied || !quickInputDirectSendEnabled.value) {
    return;
  }
  await triggerPrimaryComposerAction();
}

function recordSubmittedPrompt(text: string, projectId?: string) {
  settingsStore.recordWebSessionRecentInput(text, projectId);
  void settingsStore.syncWebSessionQuickInputToServer();
}

function isQuickInputSelected(text: string) {
  return normalizedComposerText.value.length > 0 && normalizedComposerText.value === text.trim();
}

function resolveComposerSubmitKind(): WebSessionSubmitKind {
  const workflowMode = currentSession.value?.workflowMode ?? draftWorkflowMode.value;
  return workflowMode === 'plan' ? 'plan_message' : 'execute_send';
}

const liveStateLabel = computed(() => {
  if (hasRecoveredRuntimeRequest.value) {
    return t('webSession.liveRecovered');
  }
  switch (displayLiveState.value.phase) {
    case 'starting':
      return t('webSession.liveStarting');
    case 'thinking':
      return t('webSession.liveThinking');
    case 'retrying':
      if (displayLiveState.value.retry?.attempt && displayLiveState.value.retry?.maxAttempts) {
        return t('webSession.liveRetryingProgress', {
          attempt: displayLiveState.value.retry.attempt,
          max: displayLiveState.value.retry.maxAttempts,
        });
      }
      return t('webSession.liveRetrying');
    case 'tool':
      if (isCompactToolKind(displayLiveState.value.tool?.kind)) {
        const count = Math.max(1, Number(displayLiveState.value.tool?.count ?? 1) || 1);
        const label = compactToolLabel(displayLiveState.value.tool);
        const toolLabel = count > 1 ? `${label} x${count}` : label;
        return t('webSession.liveTool', { tool: toolLabel });
      }
      return t('webSession.liveTool', { tool: displayLiveState.value.tool?.name || 'Tool' });
    case 'waiting_approval':
    case 'waiting_plan_approval':
      return t('webSession.liveWaitingApproval');
    case 'waiting_input':
      return t('webSession.liveWaitingInput');
    case 'done':
      return t('webSession.liveDone');
    case 'error':
      return t('webSession.liveError');
    default:
      return t('webSession.liveIdle');
  }
});
const liveStateDetail = computed(() => {
  if (hasRecoveredRuntimeRequest.value) {
    return recoveredRuntimeHint.value;
  }
  if (isOptimisticExecuteFeedbackActive.value) {
    return '';
  }
  if (pendingApproval.value?.prompt) {
    return pendingApproval.value.prompt;
  }
  if (
    displayLiveState.value.phase === 'waiting_approval' ||
    displayLiveState.value.phase === 'waiting_plan_approval'
  ) {
    return t('webSession.liveWaitingApprovalDetail');
  }
  if (pendingUserInput.value?.prompt) {
    return pendingUserInput.value.prompt;
  }
  if (displayLiveState.value.phase === 'retrying' && displayLiveState.value.retry?.remoteUrl) {
    return displayLiveState.value.retry.remoteUrl.trim();
  }
  if (displayLiveState.value.phase === 'retrying' && displayLiveState.value.retry?.message) {
    const message = displayLiveState.value.retry.message.trim();
    if (message && message !== liveStateLabel.value) {
      return message;
    }
  }
  if (displayLiveState.value.phase === 'tool' && displayLiveState.value.tool?.summary) {
    return displayLiveState.value.tool.summary;
  }
  if (displayLiveState.value.phase === 'tool' && displayLiveState.value.tool?.kind) {
    return displayLiveState.value.tool.kind;
  }
  if (displayLiveState.value.phase === 'error' && displayLiveState.value.errorMessage) {
    return displayLiveState.value.errorMessage;
  }
  return '';
});
const liveStateDetailTitle = computed(() => {
  if (displayLiveState.value.phase !== 'retrying') {
    return undefined;
  }
  return displayLiveState.value.retry?.remoteUrl?.trim() || undefined;
});
const liveStateSecondaryText = computed(() => {
  if (liveStateDetail.value) {
    return liveStateDetail.value;
  }
  switch (displayLiveState.value.phase) {
    case 'starting':
      return t('webSession.liveStartingDetail');
    case 'thinking':
      return t('webSession.liveThinkingDetail');
    case 'retrying':
      return t('webSession.liveRetryingDetail');
    case 'tool':
      return compactToolLabel(displayLiveState.value.tool);
    case 'waiting_approval':
    case 'waiting_plan_approval':
      return t('webSession.liveWaitingApprovalDetail');
    case 'waiting_input':
      return t('webSession.liveWaitingInputDetail');
    case 'done':
      return t('webSession.liveDoneDetail');
    case 'error':
      return t('webSession.liveErrorDetail');
    default:
      return t('webSession.liveIdleDetail');
  }
});
const liveStateWorking = computed(() =>
  ['starting', 'thinking', 'retrying', 'tool'].includes(displayLiveState.value.phase)
);
const shouldAutoContinueOnLiveCardClick = computed(
  () =>
    liveState.value.phase === 'error' &&
    Boolean(currentRealSession.value) &&
    !liveCardContinuePending.value
);
const liveCardAriaLabel = computed(() =>
  shouldAutoContinueOnLiveCardClick.value ? 'continue' : t('webSession.jumpToBottom')
);
const underlyingTabSessionId = computed(() =>
  resolveUnderlyingTabSessionId({
    activeDraftSessionId: activeDraftSessionId.value,
    activeRealSessionId: webSessionStore.getActiveSessionId(props.projectId),
  })
);
const activeTabSessionId = computed(() =>
  resolveActiveTabSessionId({
    activeArchivedPreviewId: activeArchivedPreviewId.value,
    activeDraftSessionId: activeDraftSessionId.value,
    activeRealSessionId: webSessionStore.getActiveSessionId(props.projectId),
  })
);
const activeSessionId = computed(() => currentSession.value?.id ?? '');
const emptyStateTitle = computed(() => t('webSession.draftTitle'));
const emptyStateDescription = computed(() => t('webSession.draftDescription'));
const activeSessionTitle = computed(() => currentSession.value?.title ?? emptyStateTitle.value);
const activeSessionStatusLabel = computed(() =>
  currentSession.value ? getSessionStatusLabel(currentSession.value) : ''
);
const activeSessionAttentionStateClass = computed(() =>
  currentSession.value ? getSessionAttentionStateClass(currentSession.value) : 'unknown'
);
const activeSessionHasWorkflowPlanBadge = computed(() =>
  shouldShowSessionWorkflowPlanBadge(currentSession.value)
);
const activeSessionHasScheduledPlanExecution = computed(() =>
  hasScheduledPlanExecution(currentSession.value)
);
const sidebarScope = computed<WebSessionSidebarScope>({
  get: () => normalizeWebSessionSidebarScope(persistedSidebarScope.value),
  set: value => {
    persistedSidebarScope.value = normalizeWebSessionSidebarScope(value);
  },
});
const showCrossProjectSidebar = computed(() => !isMobile.value && props.showSidebar);
const {
  resizing: isSidebarResizing,
  width: effectiveSidebarWidthPx,
  showStatusText: showSidebarStatusText,
  resetWidth: resetSidebarWidth,
  startResize: startSidebarResize,
} = useWebSessionSidebarResize({
  visible: showCrossProjectSidebar,
  getRootElement: () => sidebarRootRef.value?.rootElement ?? null,
});
const normalizedSidebarSearchQuery = computed(() =>
  normalizeWebSessionSidebarSearchQuery(sidebarSearchQuery.value)
);
const archivedSidebarSearchActive = computed(
  () => sidebarSearchArchived.value && normalizedSidebarSearchQuery.value.length > 0
);
const sidebarScopeOptions = computed<DropdownOption[]>(() => [
  {
    key: 'all',
    label: t('webSession.sidebarScopeAll'),
  },
  {
    key: 'current',
    label: t('webSession.sidebarScopeCurrentProject'),
  },
]);
const sidebarScopeLabel = computed(() =>
  sidebarScope.value === 'current'
    ? t('webSession.sidebarScopeCurrentProject')
    : t('webSession.sidebarScopeAll')
);
const sidebarScopeAriaLabel = computed(() =>
  t('webSession.sidebarScopeAria', { scope: sidebarScopeLabel.value })
);
const sidebarScopeToggleLabel = computed(() =>
  resolveWebSessionSidebarToggleScope(sidebarScope.value) === 'current'
    ? t('webSession.sidebarScopeCurrentProject')
    : t('webSession.sidebarScopeAll')
);
const sidebarScopeToggleTitle = computed(() =>
  t('webSession.sidebarScopeToggle', {
    current: sidebarScopeLabel.value,
    next: sidebarScopeToggleLabel.value,
  })
);
const mobileSessionCategory = ref<MobileSessionCategory>('current');
const mobileCurrentSessions = computed<SessionTab[]>(() => {
  if (sidebarScope.value !== 'all') {
    return sortMobileCurrentSessions(sessions.value, session =>
      resolveWebSessionSidebarSortTimestamp(session)
    );
  }

  const draftSessions = sessions.value.filter(isDraftSession);
  const crossProjectCurrentSessions = crossProjectSessions.value.map(
    item => item.session as SessionTab
  );
  return sortMobileCurrentSessions([...draftSessions, ...crossProjectCurrentSessions], session =>
    resolveWebSessionSidebarSortTimestamp(session)
  );
});
const mobileArchivedProjectIds = computed(() => sidebarVisibleProjectIds.value);
const mobileArchivedScopeKey = computed(() => mobileArchivedProjectIds.value.join('|'));
const mobileArchivedMeta = computed(() => baseArchivedSidebarMeta.value);
const mobileArchivedSessions = computed<SessionTab[]>(() =>
  baseCrossProjectArchivedSessions.value.map(item => item.session as SessionTab)
);
const mobileCurrentSessionGroups = computed(() =>
  groupWebSessionItemsByDate({
    items: mobileCurrentSessions.value,
    getTimestamp: session => resolveWebSessionSidebarSortTimestamp(session),
    labels: {
      today: t('webSession.sessionGroupToday'),
      yesterday: t('webSession.sessionGroupYesterday'),
      lastSevenDays: t('webSession.sessionGroupLastSevenDays'),
      earlier: t('webSession.sessionGroupEarlier'),
    },
    now: liveStateClockMs.value,
  }).map(group => ({
    key: group.key,
    label: group.label,
    sessions: group.items,
  }))
);
const mobileVisibleSessions = computed<SessionTab[]>(() =>
  mobileSessionCategory.value === 'archived'
    ? mobileArchivedSessions.value
    : mobileCurrentSessions.value
);
const mobileProjectBadgeById = computed(() => {
  const ids = new Set(sidebarProjectIdsToLoad.value.filter(Boolean));
  const ordered: string[] = [];
  projectStore.projects.forEach(project => {
    if (project.id && ids.has(project.id) && !ordered.includes(project.id)) {
      ordered.push(project.id);
    }
  });
  projectStore.recentProjects.forEach(project => {
    if (project.id && ids.has(project.id) && !ordered.includes(project.id)) {
      ordered.push(project.id);
    }
  });
  sidebarProjectIdsToLoad.value.forEach(projectId => {
    if (projectId && !ordered.includes(projectId)) {
      ordered.push(projectId);
    }
  });
  return buildProjectBadgeMap(ordered, getProjectName);
});
watch(
  pendingUserInputSyncKey,
  syncKey => {
    persistActiveUserInputDraft();
    const request = pendingUserInput.value;
    const storageKey = pendingUserInputDraftStorageKey.value;
    activeUserInputDraftStorageKey = storageKey;
    if (!syncKey || !request || !storageKey) {
      activeUserInputDraftStorageKey = '';
      userInputSelections.value = {};
      userInputDrafts.value = {};
      return;
    }
    const storedState = webSessionStore.getPendingUserInputDraft(storageKey);
    const nextState = reconcileWebSessionUserInputLocalState(request.questions, {
      selections: storedState?.selections ?? {},
      drafts: storedState?.drafts ?? {},
    });
    userInputSelections.value = nextState.selections;
    userInputDrafts.value = nextState.drafts;
  },
  { immediate: true }
);

watch(
  [userInputSelections, userInputDrafts],
  () => {
    persistActiveUserInputDraft();
  },
  { deep: true }
);

watch(currentUserInputSubmitOwnerId, (nextOwnerId, previousOwnerId) => {
  if (previousOwnerId && previousOwnerId !== nextOwnerId) {
    endUserInputSubmit(previousOwnerId);
  }
});

const mobileTabDescriptors = computed<MobileTabListDescriptor[]>(() =>
  buildWebSessionMobileTabDescriptors({
    section: mobileSessionCategory.value,
    sessions: mobileVisibleSessions.value,
    sessionGroups:
      mobileSessionCategory.value === 'current' ? mobileCurrentSessionGroups.value : undefined,
    collapsedGroupKeys: collapsedSessionGroupKeys.value,
    hasArchivedLoadMore: mobileArchivedMeta.value.hasMore,
    isArchivedLoading: mobileArchivedMeta.value.loading,
  }).filter((descriptor): descriptor is MobileTabListDescriptor => descriptor.kind !== 'header')
);

const mobileSessionDrawerViews = computed<ReadonlyMap<string, MobileSessionDrawerView>>(() => {
  const views = new Map<string, MobileSessionDrawerView>();
  const calendarDayStart = new Date(sidebarCalendarDayStartMs.value);
  const includeDate = mobileSessionCategory.value !== 'archived';

  mobileVisibleSessions.value.forEach(session => {
    const displayState = getSessionDisplayState(session);
    const projectId = session.projectId || props.projectId;
    const projectBadge = projectId ? (mobileProjectBadgeById.value.get(projectId) ?? null) : null;

    views.set(session.id, {
      rowClass: {
        'is-selected': session.id === activeSessionId.value,
        'is-approval': displayState.hasUnviewedApproval,
        'is-completion': !displayState.hasUnviewedApproval && displayState.hasUnviewedCompletion,
      },
      statusTooltip: getSessionStatusTooltip(session),
      agentBadgeClass:
        !isDraftSession(session) && session.status === 'err'
          ? 'state-error'
          : `state-${displayState.attentionStateClass}`,
      assistantIcon: getSessionAssistantIcon(session),
      showStatusDot: displayState.showStatusDot && Boolean(displayState.statusDotClass),
      statusDotClass: displayState.statusDotClass ?? undefined,
      showWorkflowPlanBadge: shouldShowSessionWorkflowPlanBadge(session),
      scheduledPlan: hasScheduledPlanExecution(session),
      showScheduledInput: shouldHighlightScheduledInputSession(session),
      projectBadge,
      projectName: projectId ? getProjectName(projectId) : '',
      timeLabel: formatWebSessionSidebarTime(
        resolveWebSessionSidebarSortTimestamp(session),
        calendarDayStart,
        includeDate
      ),
      timeTitle: formatWebSessionDateTime(
        resolveWebSessionSidebarSortTimestamp(session),
        locale.value
      ),
    });
  });

  return views;
});

function toggleSessionGroup(groupKey: string) {
  const nextCollapsedKeys = new Set(collapsedSessionGroupKeys.value);
  if (nextCollapsedKeys.has(groupKey)) {
    nextCollapsedKeys.delete(groupKey);
  } else {
    nextCollapsedKeys.add(groupKey);
  }
  collapsedSessionGroupKeys.value = nextCollapsedKeys;
}

async function setMobileSessionCategory(section: 'current' | 'archived') {
  mobileSessionCategory.value = section;
  if (section !== 'archived') {
    return;
  }
  if (
    mobileArchivedMeta.value.loading ||
    !mobileArchivedScopeKey.value ||
    webSessionStore.hasArchivedScope(mobileArchivedProjectIds.value)
  ) {
    return;
  }
  try {
    await ensureArchivedScopeLoaded(mobileArchivedProjectIds.value, 20);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function loadMoreMobileArchivedSessions() {
  if (
    !mobileArchivedScopeKey.value ||
    mobileArchivedMeta.value.loading ||
    !mobileArchivedMeta.value.hasMore
  ) {
    return;
  }
  try {
    await webSessionStore.loadArchivedSessions(mobileArchivedProjectIds.value, {
      limit: 20,
    });
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

function syncMobileSessionCategoryToCurrentSession() {
  mobileSessionCategory.value = isArchivedPreviewSession(currentSession.value)
    ? 'archived'
    : 'current';
}

function closeMobileSessionSelector() {
  showMobileTabSelector.value = false;
}

function openMobileSessionSelectorFromElement(
  _anchorEl: HTMLElement,
  source: MobileTabSelectorSource
) {
  if (!isMobile.value) {
    return;
  }
  syncMobileSessionCategoryToCurrentSession();
  if (mobileSessionCategory.value === 'archived') {
    void setMobileSessionCategory('archived');
  }
  mobileTabSelectorSource.value = source;
  showMobileTabSelector.value = true;
}

function handleMobileTabTriggerClick() {
  if (showMobileTabSelector.value && mobileTabSelectorSource.value === 'header') {
    closeMobileSessionSelector();
    return;
  }
  syncMobileSessionCategoryToCurrentSession();
  if (mobileSessionCategory.value === 'archived') {
    void setMobileSessionCategory('archived');
  }
  mobileTabSelectorSource.value = 'header';
  showMobileTabSelector.value = true;
}

function handleMobileSessionSelectorVisibilityChange(show: boolean) {
  if (show) {
    syncMobileSessionCategoryToCurrentSession();
    if (mobileSessionCategory.value === 'archived') {
      void setMobileSessionCategory('archived');
    }
  }
  showMobileTabSelector.value = show;
}

function requestMobileViewForBottomNavSelector() {
  if (mobileTabSelectorSource.value !== 'bottom-nav') {
    return;
  }
  emit('request-mobile-view', 'webSession');
}

function renderDropdownIcon(icon: Component) {
  return () => h(NIcon, null, { default: () => h(icon) });
}

function renderMobileProjectOptionBadge(badge: ProjectBadge) {
  return () =>
    h(
      'span',
      {
        class: 'mobile-project-option-badge',
        style: { background: badge.color },
      },
      badge.label
    );
}

const mobileProjectSwitchOptions = computed<DropdownOption[]>(() => [
  {
    type: 'render',
    key: '__search__',
    render: () =>
      h(
        'div',
        {
          class: 'mobile-project-switch-search',
          style: {
            boxSizing: 'border-box',
            width: '180px',
            padding: '7px 10px 6px',
          },
          onClick: (event: MouseEvent) => event.stopPropagation(),
          onKeydown: (event: KeyboardEvent) => {
            if (event.key !== 'Escape') {
              event.stopPropagation();
            }
          },
        },
        [
          h(
            'div',
            {
              style: {
                margin: '0 2px 6px',
                color: 'var(--n-text-color-3)',
                fontSize: '12px',
                fontWeight: '500',
                lineHeight: '1.4',
                userSelect: 'none',
              },
            },
            t('webSession.switchProject')
          ),
          h(
            NInput,
            {
              value: mobileProjectSwitchSearch.value,
              size: 'small',
              clearable: true,
              autofocus: true,
              placeholder: t('terminal.projectSearchPlaceholder'),
              'aria-label': t('terminal.projectSearchPlaceholder'),
              'onUpdate:value': (value: string) => {
                mobileProjectSwitchSearch.value = value;
              },
            },
            {
              prefix: () =>
                h(NIcon, { size: 14, 'aria-hidden': true }, { default: () => h(SearchOutline) }),
            }
          ),
        ]
      ),
  },
  ...(filteredMobileProjectSwitchProjects.value.length === 0
    ? [
        {
          type: 'render' as const,
          key: '__empty__',
          render: () =>
            h(
              'div',
              {
                style: {
                  boxSizing: 'border-box',
                  width: '180px',
                  padding: '8px 12px',
                  color: 'var(--n-text-color-3)',
                  fontSize: '12px',
                  textAlign: 'center',
                },
              },
              t('common.noData')
            ),
        },
      ]
    : filteredMobileProjectSwitchProjects.value.map(project => {
        const badge = mobileProjectSwitchBadges.value.get(project.id);
        return {
          label: project.name?.trim() || project.id,
          key: project.id,
          disabled: project.id === props.projectId,
          icon: badge ? renderMobileProjectOptionBadge(badge) : undefined,
        } satisfies DropdownOption;
      })),
  {
    type: 'divider',
    key: '__project_list_divider__',
  },
  {
    label: t('terminal.openProjectList'),
    key: '__open_project_list__',
    icon: renderDropdownIcon(GridOutline),
  },
]);

function handleMobileProjectSwitchMenuShow(show: boolean) {
  if (!show) {
    mobileProjectSwitchSearch.value = '';
  }
}

function handleMobileProjectSwitchSelect(key: string | number) {
  if (typeof key !== 'string') {
    return;
  }
  if (key === '__open_project_list__') {
    void router.push({ name: 'projects' });
    return;
  }
  if (key.startsWith('__') || key === props.projectId) {
    return;
  }
  projectStore.addRecentProject(key);
  void router.push(buildProjectRouteLocation(key));
}

function buildSessionActionOptions(session: SessionTab | null): DropdownOption[] {
  const copyableSessionId = resolveCopyableAgentSessionId(
    session,
    Boolean(session && isDraftSession(session))
  );
  const canClaudeSync =
    !!session &&
    !isDraftSession(session) &&
    session.agent === 'claude' &&
    (Boolean(session.nativeSessionId) || Boolean(session.threadPath));
  const canCodexSync =
    !!session &&
    !isDraftSession(session) &&
    session.agent === 'codex' &&
    Boolean(session.nativeSessionId);
  const canPiTree =
    !!session &&
    !isDraftSession(session) &&
    !session.archivedAt &&
    session.agent === 'pi' &&
    runtimePiCapability.value.supportsTree &&
    Boolean(session.nativeSessionId && session.threadPath);

  const options: DropdownOption[] = [
    {
      label: t('webSession.newSession'),
      key: 'new',
      icon: renderDropdownIcon(AddOutline),
    },
    {
      label: t('webSession.importCodexSession'),
      key: 'import',
      icon: renderDropdownIcon(TimeOutline),
    },
    {
      label: t('common.edit'),
      key: 'rename',
      icon: renderDropdownIcon(CreateOutline),
      disabled: !session || isDraftSession(session),
    },
    {
      label: t('webSession.archiveAction'),
      key: 'archive',
      icon: renderDropdownIcon(ArchiveOutline),
      disabled:
        !session ||
        isDraftSession(session) ||
        isArchivedPreviewSession(session) ||
        isSessionArchiving(session.id),
    },
    {
      label: t('common.delete'),
      key: 'delete',
      icon: renderDropdownIcon(TrashOutline),
      disabled: !session,
    },
  ];

  if (copyableSessionId) {
    options.splice(2, 0, {
      label: t('terminal.copyAISessionId'),
      key: 'copy-session-id',
      icon: renderDropdownIcon(CopyOutline),
    });
  }

  if (canPiTree) {
    options.splice(2, 0, {
      label: t('webSession.treeOpen'),
      key: 'tree',
      icon: renderDropdownIcon(GitNetworkOutline),
    });
  }

  if (canClaudeSync || canCodexSync) {
    options.splice(2, 0, {
      label:
        session?.agent === 'claude'
          ? t('webSession.syncSessionAction')
          : t('webSession.syncFromTerminal'),
      key: 'sync',
      icon: renderDropdownIcon(RefreshOutline),
      disabled: session?.agent === 'claude' ? !canClaudeSync : !canCodexSync,
    });
  }

  if (canCodexSync) {
    options.splice(3, 0, {
      label: t('webSession.deepSyncFromTerminal'),
      key: 'deep-sync',
      icon: renderDropdownIcon(RefreshCircleOutline),
      disabled: !canCodexSync,
    });
  }

  return options;
}

function buildSidebarSessionActionOptions(session: WebSessionSummary): DropdownOption[] {
  const options: DropdownOption[] = [
    {
      label: t('common.edit'),
      key: 'rename',
      icon: renderDropdownIcon(CreateOutline),
    },
  ];

  options.push(
    session.archivedAt
      ? {
          label: t('webSession.unarchiveAction'),
          key: 'unarchive',
          icon: renderDropdownIcon(ArchiveOutline),
        }
      : {
          label: t('webSession.archiveAction'),
          key: 'archive',
          icon: renderDropdownIcon(ArchiveOutline),
          disabled: isSessionArchiving(session.id),
        },
    {
      label: t('common.delete'),
      key: 'delete',
      icon: renderDropdownIcon(TrashOutline),
    }
  );

  return options;
}

const contextMenuOptions = computed<DropdownOption[]>(() =>
  buildSessionActionOptions(contextMenuSession.value)
);

const mobileActionMenuOptions = computed<DropdownOption[]>(() =>
  buildSessionActionOptions(currentSession.value)
);

async function handleSessionActionSelect(action: string, session: SessionTab | null) {
  if (action === 'new') {
    await handleStartDraftSession();
    return;
  }
  if (action === 'import') {
    openImportDialog();
    return;
  }
  if (!session) {
    return;
  }
  if (action === 'tree') {
    showPiTreeDrawer.value = true;
    return;
  }
  if (action === 'copy-session-id') {
    const sessionId = resolveCopyableAgentSessionId(session, isDraftSession(session));
    if (!sessionId) {
      return;
    }
    await copyText(sessionId, {
      failureMessage: t('terminal.copyFailed'),
      successMessage: t('terminal.aiSessionIdCopied'),
    });
    return;
  }
  if (action === 'rename') {
    await handleRenameSession(session.id);
    return;
  }
  if (action === 'sync') {
    confirmSyncSession(session, 'fast');
    return;
  }
  if (action === 'deep-sync') {
    confirmSyncSession(session, 'deep');
    return;
  }
  if (action === 'archive') {
    handleArchiveSession(session.id);
    return;
  }
  if (action === 'delete') {
    dialog.warning({
      title: t('common.delete'),
      content: t('webSession.deleteConfirm', { title: session.title }),
      positiveText: t('common.delete'),
      negativeText: t('common.cancel'),
      onPositiveClick: async () => performDeleteSession(session.id),
    });
  }
}

async function handleMobileActionMenuSelect(key: string | number) {
  await handleSessionActionSelect(String(key), currentSession.value);
}

async function handlePiTreeNavigated(result: {
  projectId: string;
  sessionId: string;
  editorText: string;
}) {
  const sourceProjectId = result.projectId;
  const sourceSessionId = result.sessionId;
  if (!sourceProjectId || !sourceSessionId) {
    return;
  }
  webSessionStore.setDraftText(sourceProjectId, sourceSessionId, result.editorText);
  const sourceStillActive = () =>
    props.projectId === sourceProjectId && currentRealSession.value?.id === sourceSessionId;
  try {
    await webSessionStore.loadSessionSnapshot(sourceProjectId, sourceSessionId, {
      rememberActive: sourceStillActive(),
    });
    if (sourceStillActive()) {
      composerEditorResetVersion.value += 1;
    }
  } catch (error) {
    if (sourceStillActive()) {
      message.error(error instanceof Error ? error.message : t('webSession.treeReloadFailed'));
    }
  }
}

async function handlePiTreeCreated(event: {
  projectId: string;
  sessionId: string;
  result: WebSessionPiTreeMutationResult;
  activate: boolean;
}) {
  const { projectId: sourceProjectId, sessionId: sourceSessionId, result, activate } = event;
  const target = result.session;
  if (!target?.id || target.projectId !== sourceProjectId) {
    if (props.projectId === sourceProjectId && currentRealSession.value?.id === sourceSessionId) {
      message.error(t('webSession.treeCreateInvalid'));
    }
    return;
  }
  const targetSessionId = target.id;
  const sourceStillActive = () =>
    props.projectId === sourceProjectId && currentRealSession.value?.id === sourceSessionId;
  webSessionStore.setDraftText(sourceProjectId, targetSessionId, result.editorText ?? '');
  if (!activate || !sourceStillActive()) {
    return;
  }
  try {
    await webSessionStore.loadSessions(sourceProjectId, true);
    if (!sourceStillActive()) {
      return;
    }
    const activated = await activateTabById(targetSessionId);
    if (!activated) {
      throw new Error(t('webSession.treeCreateInvalid'));
    }
    composerEditorResetVersion.value += 1;
    scrollToBottom(true);
  } catch (error) {
    if (sourceStillActive()) {
      message.error(error instanceof Error ? error.message : t('webSession.treeCreateInvalid'));
    }
  }
}

const tabsThemeOverrides = computed(() => {
  const theme = activeTheme.value;
  const tabBg = theme.terminalTabBg || theme.bodyColor;
  const tabActiveBg = theme.terminalTabActiveBg || theme.surfaceColor;
  return {
    tabColor: tabBg,
    tabColorSegment: tabActiveBg,
  };
});
const approvalColors = computed(() => {
  const theme = activeTheme.value;
  const isDarkTheme = isDarkHex(theme.bodyColor || '#ffffff');
  const accent = theme.approvalColor || (isDarkTheme ? '#cca700' : '#f79009');
  return {
    bg: `color-mix(in srgb, ${accent} ${isDarkTheme ? 18 : 14}%, transparent)`,
    border: `color-mix(in srgb, ${accent} ${isDarkTheme ? 40 : 30}%, transparent)`,
    accent,
    accentStrong: accent,
    glow: `color-mix(in srgb, ${accent} ${isDarkTheme ? 24 : 16}%, transparent)`,
  };
});
const approvalTabColors = computed(() => {
  const theme = activeTheme.value;
  const isDarkTheme = isDarkHex(theme.bodyColor || '#ffffff');
  if (isDarkTheme) {
    return {
      bg: 'var(--web-session-approval-bg)',
      border: 'var(--web-session-approval-border)',
      activeBg:
        'color-mix(in srgb, var(--web-session-approval-bg, rgba(247, 144, 9, 0.16)) 78%, var(--app-surface-color, #fff) 22%)',
      activeBorder:
        'color-mix(in srgb, var(--web-session-approval-border, rgba(247, 144, 9, 0.42)) 88%, transparent 12%)',
    };
  }
  return {
    bg: 'rgba(247, 144, 9, 0.14)',
    border: 'rgba(247, 144, 9, 0.44)',
    activeBg: 'rgba(247, 144, 9, 0.22)',
    activeBorder: 'rgba(247, 144, 9, 0.6)',
  };
});
const planApprovalColors = computed(() => {
  const theme = activeTheme.value;
  const isDarkTheme = isDarkHex(theme.bodyColor || '#ffffff');
  const accent = theme.planApprovalColor || (isDarkTheme ? '#4ec9b0' : '#0891b2');
  return {
    bg: `color-mix(in srgb, ${accent} ${isDarkTheme ? 18 : 14}%, transparent)`,
    border: `color-mix(in srgb, ${accent} ${isDarkTheme ? 40 : 30}%, transparent)`,
    accent,
    accentStrong: accent,
    glow: `color-mix(in srgb, ${accent} ${isDarkTheme ? 24 : 16}%, transparent)`,
  };
});
const webSessionStyleVars = computed(
  () =>
    ({
      '--web-session-mobile-composer-inset':
        isMobile.value && isMobileKeyboardResizeFrozen.value
          ? '0px'
          : 'var(--workspace-mobile-websession-inset, 0px)',
      '--web-session-approval-bg': approvalColors.value.bg,
      '--web-session-approval-border': approvalColors.value.border,
      '--web-session-approval-accent': approvalColors.value.accent,
      '--web-session-approval-accent-strong': approvalColors.value.accentStrong,
      '--web-session-approval-glow': approvalColors.value.glow,
      '--web-session-approval-tab-bg': approvalTabColors.value.bg,
      '--web-session-approval-tab-border': approvalTabColors.value.border,
      '--web-session-approval-tab-active-bg': approvalTabColors.value.activeBg,
      '--web-session-approval-tab-active-border': approvalTabColors.value.activeBorder,
      '--web-session-plan-approval-bg': planApprovalColors.value.bg,
      '--web-session-plan-approval-border': planApprovalColors.value.border,
      '--web-session-plan-approval-accent': planApprovalColors.value.accent,
      '--web-session-plan-approval-accent-strong': planApprovalColors.value.accentStrong,
      '--web-session-plan-approval-glow': planApprovalColors.value.glow,
    }) as CSSProperties
);
const tabTitleStyle = computed(() => ({
  maxWidth: `${tabTitleMaxWidth.value}px`,
}));
const timelineContentVersion = computed(() =>
  visibleBlocks.value
    .map(block => {
      const toolGroupCount = block.tool?.commandGroup?.count ?? 0;
      const groupItemsLength = Array.isArray(block.payload?.groupItems)
        ? block.payload.groupItems.length
        : 0;
      const toolVersion = block.tool
        ? `${block.tool.id}:${block.tool.status}:${String(block.tool.output ?? '').length}:${toolGroupCount}:${groupItemsLength}`
        : '';
      return `${block.key}:${block.kind}:${block.text.length}:${block.attachments.length}:${toolVersion}:${block.done ? 1 : 0}`;
    })
    .join('|')
);
const sidebarProjectIdsToLoad = computed(() => {
  const ids = new Set<string>();
  if (props.projectId) {
    ids.add(props.projectId);
  }
  projectStore.recentProjects.forEach(project => {
    if (project.id) {
      ids.add(project.id);
    }
  });
  projectStore.projects.forEach(project => {
    if (project.id) {
      ids.add(project.id);
    }
  });
  return Array.from(ids);
});
const piTrustProjectPath = computed(() => {
  if (piTrustServerProjectPath.value) {
    return piTrustServerProjectPath.value;
  }
  if (projectStore.currentProject?.id === props.projectId) {
    return projectStore.currentProject.path || '';
  }
  return projectStore.projects.find(project => project.id === props.projectId)?.path || '';
});
const sidebarVisibleProjectIds = computed(() =>
  resolveWebSessionSidebarProjectIds({
    scope: sidebarScope.value,
    currentProjectId: props.projectId,
    allProjectIds: sidebarProjectIdsToLoad.value,
  })
);

function parseTimestamp(value?: string | null) {
  if (!value) {
    return 0;
  }
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function resolveDraftProjectPresentation(
  agent: WebSessionAgent,
  worktreeId?: string | null,
  projectId = props.projectId
) {
  return resolveWebSessionDraftProjectPresentation({
    projectId,
    agent,
    worktreeId,
    currentProject: projectStore.currentProject,
    projects: projectStore.projects,
    worktrees: projectStore.worktrees,
    worktreesReady:
      projectStore.currentProject?.id === projectId && !projectStore.projectDetailLoading,
  });
}

function fallbackDraftTitle(agent: WebSessionAgent, projectId = props.projectId) {
  return resolveDraftProjectPresentation(agent, null, projectId).title;
}

function normalizeDraftSession(
  session: Partial<DraftSessionTab>,
  index: number,
  projectId: string
): DraftSessionTab | null {
  const id = String(session.id || '').trim();
  if (!id) {
    return null;
  }
  const agent: WebSessionAgent =
    session.agent === 'claude' || session.agent === 'pi' ? session.agent : 'codex';
  const requestedWorktreeId =
    typeof session.worktreeId === 'string' ? session.worktreeId || null : null;
  const presentation = resolveDraftProjectPresentation(agent, requestedWorktreeId, projectId);
  const nowIso = new Date().toISOString();
  return {
    id,
    projectId,
    worktreeId: presentation.worktreeId,
    orderIndex: Number.MAX_SAFE_INTEGER - index,
    agent,
    claudeRuntime: session.claudeRuntime === 'ccr' ? 'ccr' : 'claude',
    title: presentation.title,
    model:
      typeof session.model === 'string' && session.model.trim()
        ? session.model.trim()
        : defaultModelForAgent(agent),
    reasoningEffort:
      session.reasoningEffort === 'default' ||
      session.reasoningEffort === 'none' ||
      session.reasoningEffort === 'minimal' ||
      session.reasoningEffort === 'low' ||
      session.reasoningEffort === 'medium' ||
      session.reasoningEffort === 'high' ||
      session.reasoningEffort === 'xhigh' ||
      session.reasoningEffort === 'max' ||
      session.reasoningEffort === 'ultra'
        ? session.reasoningEffort
        : defaultReasoningEffortForAgent(agent),
    workflowMode: session.workflowMode === 'plan' ? 'plan' : 'default',
    permissionLevel:
      session.permissionLevel === 'default' ||
      session.permissionLevel === 'elevated' ||
      session.permissionLevel === 'yolo'
        ? session.permissionLevel
        : defaultPermissionLevelForAgent(agent),
    activeCallTimeoutEnabled: resolveInheritedActiveCallTimeoutEnabled(session, agent),
    autoRetryEnabled: session.autoRetryEnabled === true,
    autoRetryPolicyMode: session.autoRetryPolicyMode === 'custom' ? 'custom' : 'default',
    autoRetryScope:
      session.autoRetryPolicyMode === 'custom' &&
      (session.autoRetryScope === 'network_and_rate_limit' ||
        session.autoRetryScope === 'all_failures')
        ? session.autoRetryScope
        : webSessionAutoContinueScope.value,
    autoRetryPreset:
      session.autoRetryPolicyMode === 'custom' &&
      (session.autoRetryPreset === 'aggressive_stop' || session.autoRetryPreset === 'sustain_60s')
        ? session.autoRetryPreset
        : webSessionAutoContinuePreset.value,
    autoRetryMaxAttempts:
      session.autoRetryPolicyMode === 'custom' && typeof session.autoRetryMaxAttempts === 'number'
        ? session.autoRetryMaxAttempts
        : webSessionAutoContinueMaxAttempts.value,
    autoRetryDispatchPendingOnFailure:
      typeof session.autoRetryDispatchPendingOnFailure === 'boolean'
        ? session.autoRetryDispatchPendingOnFailure
        : webSessionAutoRetryDispatchPendingOnFailure.value,
    cwd: presentation.cwd,
    nativeSessionId: null,
    status: 'idle',
    hasUnread: false,
    archivedAt: null,
    activityAt:
      typeof session.activityAt === 'string' && session.activityAt.trim()
        ? session.activityAt
        : nowIso,
    lastMessageAt: null,
    sourceKind: typeof session.sourceKind === 'string' ? session.sourceKind : 'codex_app_server',
    syncState: normalizeWebSessionSyncState(session.syncState),
    sourceCreatedAt: null,
    sourceUpdatedAt: null,
    lastSyncedAt: null,
    threadPath: null,
    threadPreview: null,
    turnCount: 0,
    itemCount: 0,
    syncError: null,
    createdAt:
      typeof session.createdAt === 'string' && session.createdAt.trim()
        ? session.createdAt
        : nowIso,
    updatedAt:
      typeof session.updatedAt === 'string' && session.updatedAt.trim()
        ? session.updatedAt
        : nowIso,
    usage: {
      inputTokens: 0,
      cachedInputTokens: 0,
      outputTokens: 0,
      cost: 0,
    },
    contextEstimate: {
      inputTokens: 0,
      cachedInputTokens: 0,
      outputTokens: 0,
      usedTokens: 0,
    },
    contextEstimateMode: 'cumulative_total',
    lastContextCompactionAt: null,
    contextWindowTokens: null,
    contextWindowSource: 'unavailable',
    isDraft: true,
  };
}

function loadPersistedDraftSessions(projectId: string) {
  const stored = persistedDraftSessionsByProject.value[projectId];
  if (!Array.isArray(stored) || stored.length === 0) {
    return [];
  }
  const seen = new Set<string>();
  return stored
    .map((session, index) => normalizeDraftSession(session, index, projectId))
    .filter((session): session is DraftSessionTab => {
      if (!session || seen.has(session.id)) {
        return false;
      }
      seen.add(session.id);
      return true;
    });
}

function normalizeSessionIdList(value: unknown) {
  if (!Array.isArray(value) || value.length === 0) {
    return [];
  }
  const seen = new Set<string>();
  return value
    .map(item => String(item || '').trim())
    .filter(sessionId => {
      if (!sessionId || seen.has(sessionId)) {
        return false;
      }
      seen.add(sessionId);
      return true;
    });
}

function loadPersistedTabOrderIds(projectId: string) {
  return normalizeSessionIdList(persistedTabOrderByProject.value[projectId]);
}

function loadPersistedTabMruIds(projectId: string) {
  return normalizeSessionIdList(persistedTabVisitMruByProject.value[projectId]);
}

function getVisibleTabIds(): string[] {
  return nonArchivedVisibleSessions.value.map((session: SessionTab) => session.id);
}

function normalizeTabOrderIds(
  orderIds: string[],
  visibleIds: string[] = getVisibleTabIds()
): string[] {
  const visibleSet = new Set(visibleIds);
  const realIds = realSessions.value
    .map(session => session.id)
    .filter(sessionId => visibleSet.has(sessionId));
  const draftIds = draftSessions.value
    .map(session => session.id)
    .filter(sessionId => visibleSet.has(sessionId));
  const realIndexById = new Map(realIds.map((sessionId, index) => [sessionId, index]));
  const draftSet = new Set(draftIds);
  const draftSlots = Array.from({ length: realIds.length + 1 }, () => [] as string[]);
  const seenDraftIds = new Set<string>();
  let currentSlot = 0;

  normalizeSessionIdList(orderIds).forEach(sessionId => {
    const realIndex = realIndexById.get(sessionId);
    if (realIndex != null) {
      currentSlot = realIndex + 1;
      return;
    }
    if (!draftSet.has(sessionId) || seenDraftIds.has(sessionId)) {
      return;
    }
    draftSlots[currentSlot].push(sessionId);
    seenDraftIds.add(sessionId);
  });

  draftIds.forEach(sessionId => {
    if (!seenDraftIds.has(sessionId)) {
      draftSlots[realIds.length].push(sessionId);
      seenDraftIds.add(sessionId);
    }
  });

  const next = [...draftSlots[0]];
  realIds.forEach((sessionId, index) => {
    next.push(sessionId, ...draftSlots[index + 1]);
  });
  return next;
}

function normalizeTabMruIds(mruIds: string[], visibleIds: string[] = getVisibleTabIds()): string[] {
  const visibleSet = new Set(visibleIds);
  const next: string[] = [];

  normalizeSessionIdList(mruIds).forEach(sessionId => {
    if (!visibleSet.has(sessionId) || next.includes(sessionId)) {
      return;
    }
    next.push(sessionId);
  });

  return next;
}

function persistTabNavigationState(
  projectId: string,
  nextOrderIds = tabOrderIds.value,
  nextMruIds = tabMruIds.value,
  visibleIds = getVisibleTabIds()
) {
  if (!projectId) {
    return;
  }

  const normalizedOrderIds = normalizeTabOrderIds(nextOrderIds, visibleIds);
  const normalizedMruIds = normalizeTabMruIds(nextMruIds, visibleIds);
  const hasPersistableDraft = normalizedOrderIds.some(sessionId => {
    const session = visibleSessionById.value.get(sessionId);
    return session && isDraftSession(session);
  });
  const persistableIds = hasPersistableDraft
    ? normalizedOrderIds.filter(sessionId => {
        const session = visibleSessionById.value.get(sessionId);
        return session && !isArchivedPreviewSession(session);
      })
    : [];
  const persistableMruIds = normalizedMruIds.filter(sessionId => {
    const session = visibleSessionById.value.get(sessionId);
    return session && !isArchivedPreviewSession(session);
  });

  persistedTabOrderByProject.value = persistableIds.length
    ? {
        ...persistedTabOrderByProject.value,
        [projectId]: persistableIds,
      }
    : Object.fromEntries(
        Object.entries(persistedTabOrderByProject.value).filter(([key]) => key !== projectId)
      );

  persistedTabVisitMruByProject.value = persistableMruIds.length
    ? {
        ...persistedTabVisitMruByProject.value,
        [projectId]: persistableMruIds,
      }
    : Object.fromEntries(
        Object.entries(persistedTabVisitMruByProject.value).filter(([key]) => key !== projectId)
      );
}

function replaceTabNavigationState(
  nextOrderIds: string[],
  nextMruIds: string[],
  projectId = props.projectId,
  visibleIds = getVisibleTabIds()
) {
  const normalizedOrderIds = normalizeTabOrderIds(nextOrderIds, visibleIds);
  const normalizedMruIds = normalizeTabMruIds(nextMruIds, visibleIds);
  tabOrderIds.value = normalizedOrderIds;
  tabMruIds.value = normalizedMruIds;
  persistTabNavigationState(projectId, normalizedOrderIds, normalizedMruIds, visibleIds);
}

function syncTabNavigationState(
  projectId = props.projectId,
  options?: { orderIds?: string[]; mruIds?: string[]; visibleIds?: string[] }
) {
  const visibleIds = options?.visibleIds ?? getVisibleTabIds();
  replaceTabNavigationState(
    options?.orderIds ?? tabOrderIds.value,
    options?.mruIds ?? tabMruIds.value,
    projectId,
    visibleIds
  );
}

function rememberTabVisit(sessionId: string, projectId = props.projectId) {
  const normalizedSessionId = String(sessionId || '').trim();
  if (!normalizedSessionId) {
    return;
  }
  const visibleIds = getVisibleTabIds();
  if (!visibleIds.includes(normalizedSessionId)) {
    return;
  }
  replaceTabNavigationState(
    tabOrderIds.value,
    [normalizedSessionId, ...tabMruIds.value.filter(id => id !== normalizedSessionId)],
    projectId,
    visibleIds
  );
}

function insertTabAfter(
  sessionId: string,
  afterId = underlyingTabSessionId.value,
  projectId = props.projectId
) {
  const visibleIds = getVisibleTabIds();
  if (!visibleIds.includes(sessionId)) {
    return;
  }
  if (afterId === sessionId) {
    return;
  }
  const nextOrderIds = normalizeTabOrderIds(
    tabOrderIds.value.filter(id => id !== sessionId),
    visibleIds.filter(id => id !== sessionId)
  );
  let insertIndex = nextOrderIds.length;
  if (afterId) {
    const anchorIndex = nextOrderIds.indexOf(afterId);
    insertIndex = anchorIndex >= 0 ? anchorIndex + 1 : nextOrderIds.length;
  }
  nextOrderIds.splice(insertIndex, 0, sessionId);
  replaceTabNavigationState(
    nextOrderIds,
    [sessionId, ...tabMruIds.value.filter(id => id !== sessionId)],
    projectId,
    visibleIds
  );
}

function replaceTabIdInNavigationState(
  fromId: string,
  toId: string,
  projectId = props.projectId,
  visibleIds = Array.from(new Set([...getVisibleTabIds().filter(id => id !== fromId), toId]))
) {
  const nextOrderIds = tabOrderIds.value.map(sessionId =>
    sessionId === fromId ? toId : sessionId
  );
  const nextMruIds = tabMruIds.value.map(sessionId => (sessionId === fromId ? toId : sessionId));
  replaceTabNavigationState(nextOrderIds, nextMruIds, projectId, visibleIds);
}

function persistDraftSessionState(
  projectId: string,
  nextDrafts = draftSessions.value,
  nextActiveDraftId = activeDraftSessionId.value
) {
  if (!projectId) {
    return;
  }
  const normalizedDrafts = nextDrafts
    .map((session, index) => normalizeDraftSession(session, index, projectId))
    .filter((session): session is DraftSessionTab => Boolean(session));
  persistedDraftSessionsByProject.value = normalizedDrafts.length
    ? {
        ...persistedDraftSessionsByProject.value,
        [projectId]: normalizedDrafts,
      }
    : Object.fromEntries(
        Object.entries(persistedDraftSessionsByProject.value).filter(([key]) => key !== projectId)
      );
  const normalizedActiveDraftId = normalizedDrafts.some(session => session.id === nextActiveDraftId)
    ? nextActiveDraftId
    : '';
  persistedActiveDraftSessionIdByProject.value = normalizedActiveDraftId
    ? {
        ...persistedActiveDraftSessionIdByProject.value,
        [projectId]: normalizedActiveDraftId,
      }
    : Object.fromEntries(
        Object.entries(persistedActiveDraftSessionIdByProject.value).filter(
          ([key]) => key !== projectId
        )
      );
}

function replaceDraftSessionState(
  nextDrafts: DraftSessionTab[],
  nextActiveDraftId: string,
  projectId = props.projectId
) {
  draftSessions.value = nextDrafts;
  activeDraftSessionId.value = nextActiveDraftId;
  if (nextActiveDraftId) {
    activeArchivedPreviewId.value = '';
  }
  persistDraftSessionState(projectId, nextDrafts, nextActiveDraftId);
}

function reconcileDraftSessionProjectScope(projectId = props.projectId) {
  if (!projectId || draftSessions.value.length === 0) {
    return;
  }

  let changed = false;
  const nextDrafts = draftSessions.value.map(draft => {
    if (draft.projectId !== projectId) {
      return draft;
    }
    const presentation = resolveDraftProjectPresentation(draft.agent, draft.worktreeId, projectId);
    if (
      draft.title === presentation.title &&
      draft.cwd === presentation.cwd &&
      draft.worktreeId === presentation.worktreeId
    ) {
      return draft;
    }
    changed = true;
    return {
      ...draft,
      title: presentation.title,
      cwd: presentation.cwd,
      worktreeId: presentation.worktreeId,
    };
  });

  if (changed) {
    replaceDraftSessionState(nextDrafts, activeDraftSessionId.value, projectId);
  }
}

function resolveCurrentProjectWorktreeId(...worktreeIds: Array<string | null | undefined>) {
  for (const worktreeId of worktreeIds) {
    const normalizedWorktreeId = String(worktreeId || '').trim();
    if (!normalizedWorktreeId) {
      continue;
    }
    const worktree = projectStore.worktrees.find(
      item => item.id === normalizedWorktreeId && item.projectId === props.projectId
    );
    if (worktree) {
      return worktree.id;
    }
  }
  return null;
}

function resolveCreateSessionWorktreeId(source: SessionTab | null) {
  const worktreeId = isDraftSession(source)
    ? resolveCurrentProjectWorktreeId(source.worktreeId, projectStore.selectedWorktreeId)
    : resolveCurrentProjectWorktreeId(projectStore.selectedWorktreeId, source?.worktreeId);
  return worktreeId ?? undefined;
}

function resolveDraftContext(worktreeId?: string | null) {
  const agent = currentSession.value?.agent ?? draftAgent.value;
  const presentation = resolveDraftProjectPresentation(
    agent,
    resolveCurrentProjectWorktreeId(worktreeId)
  );
  return {
    worktreeId: presentation.worktreeId,
    cwd: presentation.cwd,
  };
}

function buildDraftTitle(agent: WebSessionAgent) {
  const baseTitle = fallbackDraftTitle(agent);
  const samePrefixCount = draftSessions.value.filter(
    session => session.title === baseTitle || session.title.startsWith(`${baseTitle} `)
  ).length;
  return samePrefixCount > 0 ? `${baseTitle} ${samePrefixCount + 1}` : baseTitle;
}

function updateDraftSession(draftId: string, updater: (draft: DraftSessionTab) => DraftSessionTab) {
  replaceDraftSessionState(
    draftSessions.value.map(session => (session.id === draftId ? updater(session) : session)),
    activeDraftSessionId.value
  );
}

function updateActiveDraftSession(updater: (draft: DraftSessionTab) => DraftSessionTab) {
  if (!activeDraftSessionId.value) {
    return;
  }
  updateDraftSession(activeDraftSessionId.value, updater);
}

function createDraftSession(forceAgent?: WebSessionAgent) {
  const anchorId = underlyingTabSessionId.value;
  const source = currentSession.value;
  const nextAgent = forceAgent ?? source?.agent ?? draftAgent.value;
  const context = resolveDraftContext(
    source?.worktreeId ?? projectStore.selectedWorktreeId ?? null
  );
  const nowIso = new Date().toISOString();
  const draft: DraftSessionTab = {
    id: `draft_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
    projectId: props.projectId,
    worktreeId: context.worktreeId,
    orderIndex: Number.MAX_SAFE_INTEGER - draftSessions.value.length,
    agent: nextAgent,
    claudeRuntime: source?.claudeRuntime === 'ccr' ? 'ccr' : draftClaudeRuntime.value,
    title: buildDraftTitle(nextAgent),
    model: defaultModelForAgent(nextAgent),
    reasoningEffort: defaultReasoningEffortForAgent(nextAgent),
    workflowMode: source?.workflowMode || draftWorkflowMode.value,
    permissionLevel: defaultPermissionLevelForAgent(nextAgent),
    activeCallTimeoutEnabled: resolveInheritedActiveCallTimeoutEnabled(source, nextAgent),
    autoRetryEnabled: source?.autoRetryEnabled === true,
    autoRetryPolicyMode:
      source?.autoRetryEnabled === true && source.autoRetryPolicyMode === 'custom'
        ? 'custom'
        : 'default',
    autoRetryScope:
      source?.autoRetryEnabled === true && source.autoRetryPolicyMode === 'custom'
        ? source.autoRetryScope
        : webSessionAutoContinueScope.value,
    autoRetryPreset:
      source?.autoRetryEnabled === true && source.autoRetryPolicyMode === 'custom'
        ? source.autoRetryPreset
        : webSessionAutoContinuePreset.value,
    autoRetryMaxAttempts:
      source?.autoRetryEnabled === true &&
      source.autoRetryPolicyMode === 'custom' &&
      typeof source.autoRetryMaxAttempts === 'number'
        ? source.autoRetryMaxAttempts
        : webSessionAutoContinueMaxAttempts.value,
    autoRetryDispatchPendingOnFailure:
      typeof source?.autoRetryDispatchPendingOnFailure === 'boolean'
        ? source.autoRetryDispatchPendingOnFailure
        : webSessionAutoRetryDispatchPendingOnFailure.value,
    cwd: context.cwd,
    nativeSessionId: null,
    status: 'idle',
    hasUnread: false,
    archivedAt: null,
    activityAt: nowIso,
    lastMessageAt: null,
    sourceKind:
      nextAgent === 'codex'
        ? 'codex_app_server'
        : nextAgent === 'pi'
          ? 'pi_rpc'
          : 'claude_stream_json',
    syncState: 'missing',
    sourceCreatedAt: null,
    sourceUpdatedAt: null,
    lastSyncedAt: null,
    threadPath: null,
    threadPreview: null,
    turnCount: 0,
    itemCount: 0,
    syncError: null,
    createdAt: nowIso,
    updatedAt: nowIso,
    usage: {
      inputTokens: 0,
      cachedInputTokens: 0,
      outputTokens: 0,
      cost: 0,
    },
    contextEstimate: {
      inputTokens: 0,
      cachedInputTokens: 0,
      outputTokens: 0,
      usedTokens: 0,
    },
    contextEstimateMode: 'cumulative_total',
    lastContextCompactionAt: null,
    contextWindowTokens: null,
    contextWindowSource: 'unavailable',
    isDraft: true,
  };
  replaceDraftSessionState([...draftSessions.value, draft], draft.id);
  insertTabAfter(draft.id, anchorId);
  webSessionStore.setActiveSession(props.projectId, '');
  return draft;
}

function ensureDefaultDraftSession() {
  if (
    realSessions.value.length > 0 ||
    draftSessions.value.length > 0 ||
    archivedPreviewSession.value
  ) {
    return;
  }
  createDraftSession();
}

function clearArchivedPreviewSession() {
  if (activeArchivedPreviewId.value === archivedPreviewSession.value?.id) {
    activeArchivedPreviewId.value = '';
  }
  archivedPreviewSession.value = null;
}

function syncArchivedPreviewSessionSummary(sessionId: string) {
  if (!archivedPreviewSession.value || archivedPreviewSession.value.id !== sessionId) {
    return;
  }
  const latest =
    webSessionStore
      .getArchivedSessions(mobileArchivedProjectIds.value)
      .find(item => item.id === sessionId) ??
    webSessionStore
      .getArchivedSessions(sidebarVisibleProjectIds.value)
      .find(item => item.id === sessionId) ??
    archivedPreviewSession.value;
  archivedPreviewSession.value = {
    ...latest,
    isArchivedPreview: true,
  };
}

function resolveNextTabAfterClose(sessionId: string) {
  const nextVisibleIds = getVisibleTabIds().filter(id => id !== sessionId);
  const nextOrderIds = normalizeTabOrderIds(
    tabOrderIds.value.filter(id => id !== sessionId),
    nextVisibleIds
  );
  const fallbackId = resolveNextWebSessionTabAfterClose({
    closingSessionId: sessionId,
    sessions: nonArchivedVisibleSessions.value,
    mruIds: tabMruIds.value,
  });
  if (fallbackId) {
    return fallbackId;
  }
  const currentOrderIds = normalizeTabOrderIds(tabOrderIds.value, getVisibleTabIds());
  const closedIndex = currentOrderIds.indexOf(sessionId);
  if (closedIndex < 0) {
    return nextOrderIds[0] ?? '';
  }
  return nextOrderIds[closedIndex] ?? nextOrderIds[closedIndex - 1] ?? nextOrderIds[0] ?? '';
}

async function activateTabById(
  sessionId: string,
  options?: { connectReal?: boolean; routeDriven?: boolean }
) {
  const session = visibleSessionById.value.get(sessionId);
  if (!session) {
    return false;
  }
  if (!options?.routeDriven) {
    pendingRouteActivationSessionId.value = '';
  }

  if (isDraftSession(session)) {
    realSessionSnapshotLoadController.cancel();
    replaceDraftSessionState(draftSessions.value, session.id);
    activeArchivedPreviewId.value = '';
    webSessionStore.setActiveSession(props.projectId, '');
    rememberTabVisit(session.id);
    return true;
  } else if (isArchivedPreviewSession(session)) {
    realSessionSnapshotLoadController.cancel();
    activeArchivedPreviewId.value = session.id;
    return true;
  } else {
    replaceDraftSessionState(draftSessions.value, '');
    activeArchivedPreviewId.value = '';
    rememberTabVisit(session.id);
    if (options?.connectReal === false) {
      realSessionSnapshotLoadController.cancel();
      webSessionStore.setActiveSession(props.projectId, session.id);
      return true;
    } else {
      return await connectVisibleRealSession(props.projectId, session.id);
    }
  }
}

function buildProjectRouteLocation(projectId: string, sessionId = '') {
  return (
    buildWebSessionProjectLocation({
      projectId,
      sessionId,
      query: route.query,
    }) ?? {
      name: 'project' as const,
      params: { id: projectId },
      query: buildWebSessionRouteQuery(buildWorkspaceRouteQuery(route.query, 'web'), sessionId),
    }
  );
}

async function syncWebSessionRouteSessionId(sessionId = '') {
  if (isWebSessionRouteQuerySynced(route.query, sessionId)) {
    return;
  }
  await router.replace({
    query: buildWebSessionRouteQuery(route.query, sessionId),
  });
}

async function openArchivedPreviewSession(
  session: WebSessionSummary,
  options?: { snapshotLoaded?: boolean; routeDriven?: boolean }
) {
  if (!options?.routeDriven) {
    pendingRouteActivationSessionId.value = '';
  }
  const previousPreviewId = archivedPreviewSession.value?.id ?? '';
  if (previousPreviewId && previousPreviewId !== session.id) {
    clearArchivedPreviewSession();
  }
  archivedPreviewSession.value = {
    ...session,
    isArchivedPreview: true,
  };
  activeArchivedPreviewId.value = session.id;
  if (!options?.snapshotLoaded) {
    const snapshot = await webSessionStore.loadSessionSnapshot(session.projectId, session.id, {
      rememberActive: false,
      preserveArchivedPosition: true,
    });
    if (snapshot?.session) {
      archivedPreviewSession.value = {
        ...snapshot.session,
        isArchivedPreview: true,
      };
    }
  }
  syncArchivedPreviewSessionSummary(session.id);
}

async function connectVisibleRealSession(projectId: string, sessionId: string) {
  if (!projectId || !sessionId) {
    return false;
  }
  const snapshotLoad = realSessionSnapshotLoadController.begin();
  activeArchivedPreviewId.value = '';
  webSessionStore.setActiveSession(projectId, sessionId);
  try {
    await webSessionStore.loadSessionSnapshot(projectId, sessionId, {
      rememberActive: false,
      signal: snapshotLoad.signal,
    });
    return realSessionSnapshotLoadController.isCurrent(snapshotLoad);
  } catch (error) {
    if (isAbortLikeError(error) || !realSessionSnapshotLoadController.isCurrent(snapshotLoad)) {
      return false;
    }
    throw error;
  } finally {
    realSessionSnapshotLoadController.release(snapshotLoad);
  }
}

async function activateSessionFromRoute(
  projectId: string,
  requestedSessionId: string,
  options?: {
    loadedSessions?: WebSessionSummary[];
    showError?: boolean;
  }
) {
  const routeTarget = resolveWebSessionDeepLinkTarget({
    currentProjectId: projectId,
    requestedSessionId,
    loadedSessions: options?.loadedSessions ?? realSessions.value,
  });

  if (routeTarget.action === 'none') {
    return false;
  }

  if (routeTarget.action === 'activate-loaded') {
    const handled = await activateTabById(routeTarget.sessionId, { routeDriven: true });
    if (handled) {
      pendingRouteActivationSessionId.value = '';
    }
    return handled;
  }

  if (routeTarget.action !== 'load-snapshot') {
    return false;
  }

  const snapshotLoad = realSessionSnapshotLoadController.begin();
  try {
    const snapshot = await webSessionStore.loadSessionSnapshot(projectId, routeTarget.sessionId, {
      rememberActive: false,
      signal: snapshotLoad.signal,
      preserveArchivedPosition: true,
    });
    if (!realSessionSnapshotLoadController.isCurrent(snapshotLoad)) {
      return false;
    }
    const snapshotTarget = resolveWebSessionDeepLinkTarget({
      currentProjectId: projectId,
      requestedSessionId: routeTarget.sessionId,
      snapshotSession: snapshot?.session ?? null,
    });

    if (snapshotTarget.action === 'activate-real') {
      const handled = await activateTabById(snapshotTarget.sessionId, {
        connectReal: false,
        routeDriven: true,
      });
      if (handled) {
        pendingRouteActivationSessionId.value = '';
      }
      return handled;
    }

    if (snapshotTarget.action === 'open-archived-preview' && snapshot?.session) {
      await openArchivedPreviewSession(snapshot.session, {
        snapshotLoaded: true,
        routeDriven: true,
      });
      pendingRouteActivationSessionId.value = '';
      return true;
    }

    pendingRouteActivationSessionId.value = '';
    await syncWebSessionRouteSessionId('');
  } catch (error) {
    if (isAbortLikeError(error) || !realSessionSnapshotLoadController.isCurrent(snapshotLoad)) {
      return false;
    }
    pendingRouteActivationSessionId.value = '';
    await syncWebSessionRouteSessionId('');
    if (options?.showError !== false) {
      message.error(error instanceof Error ? error.message : t('common.error'));
    }
  } finally {
    realSessionSnapshotLoadController.release(snapshotLoad);
  }

  return false;
}

function removeDraftSessionRecord(sessionId: string, options?: { preserveDraftState?: boolean }) {
  const removedActive = activeDraftSessionId.value === sessionId;
  const nextActiveDraftId = removedActive ? '' : activeDraftSessionId.value;
  replaceDraftSessionState(
    draftSessions.value.filter(session => session.id !== sessionId),
    nextActiveDraftId
  );
  if (!options?.preserveDraftState) {
    webSessionStore.clearDraft(props.projectId, sessionId);
  }
}

async function closeTabById(
  sessionId: string,
  closer: () => Promise<void> | void,
  options?: { syncNavigationOnly?: boolean }
) {
  const wasActive = activeSessionId.value === sessionId;
  const fallbackTabId = wasActive ? resolveNextTabAfterClose(sessionId) : '';

  await closer();

  syncTabNavigationState();

  if (wasActive) {
    if (fallbackTabId && (await activateTabById(fallbackTabId))) {
      return;
    }
    ensureDefaultDraftSession();
    if (activeSessionId.value) {
      rememberTabVisit(activeSessionId.value);
    }
    return;
  }

  if (options?.syncNavigationOnly) {
    syncTabNavigationState();
  }
}

function markSessionViewed(sessionId?: string) {
  const normalizedSessionId = String(sessionId || '').trim();
  if (!props.isActive || !normalizedSessionId) {
    return;
  }
  const session = visibleSessionById.value.get(normalizedSessionId);
  if (session && !isDraftSession(session)) {
    optimisticUnreadClearedVersionBySession.value = {
      ...optimisticUnreadClearedVersionBySession.value,
      [normalizedSessionId]: getSessionUnreadVersion(session),
    };
  }
  webSessionStore.emitter.emit('web-session:viewed', {
    sessionId: normalizedSessionId,
  });
}

function getSessionUnreadVersion(session: WebSessionSummary) {
  return parseTimestamp(
    session.statusUpdatedAt ||
      session.assistantStateUpdatedAt ||
      session.updatedAt ||
      session.activityAt ||
      session.lastMessageAt ||
      session.createdAt
  );
}

function hasSessionUnread(session: (typeof sessions.value)[number]) {
  if (isDraftSession(session)) {
    return false;
  }
  if (!session.hasUnread) {
    return false;
  }
  const optimisticClearedVersion = optimisticUnreadClearedVersionBySession.value[session.id] ?? 0;
  return getSessionUnreadVersion(session) > optimisticClearedVersion;
}

function getProjectName(projectId: string) {
  if (!projectId) {
    return '';
  }
  if (projectStore.currentProject?.id === projectId && projectStore.currentProject.name) {
    return projectStore.currentProject.name;
  }
  return (
    projectStore.projects.find(project => project.id === projectId)?.name ||
    projectStore.recentProjects.find(project => project.id === projectId)?.name ||
    projectId
  );
}

function buildSidebarProjectOrder(items: Array<Pick<CrossProjectSessionItem, 'projectId'>>) {
  const presentProjectIds = new Set(items.map(item => item.projectId).filter(Boolean));
  const projectIds: string[] = [];
  projectStore.projects.forEach(project => {
    if (project.id && presentProjectIds.has(project.id)) {
      projectIds.push(project.id);
    }
  });
  items.forEach(item => {
    if (item.projectId && !projectIds.includes(item.projectId)) {
      projectIds.push(item.projectId);
    }
  });
  return projectIds;
}

function withProjectBadges(
  items: CrossProjectSessionItem[],
  projectIds = buildSidebarProjectOrder(items)
) {
  const projectBadge = buildProjectBadgeMap(projectIds, getProjectName);

  return items.map(item => ({
    ...item,
    // 当前项目不需要徽章，只有跨项目行显示项目标记。
    projectBadge: item.projectId === props.projectId ? undefined : projectBadge.get(item.projectId),
  }));
}

function sortCrossProjectSessionItems(items: CrossProjectSessionItem[]) {
  const projectIds = buildSidebarProjectOrder(items);
  const sorted = [...items].sort((left, right) => {
    const rightTimestamp = resolveWebSessionSidebarSortTimestamp(right.session);
    const leftTimestamp = resolveWebSessionSidebarSortTimestamp(left.session);
    if (rightTimestamp !== leftTimestamp) {
      return rightTimestamp - leftTimestamp;
    }
    if (left.session.orderIndex !== right.session.orderIndex) {
      return left.session.orderIndex - right.session.orderIndex;
    }
    return left.session.id.localeCompare(right.session.id);
  });
  return withProjectBadges(sorted, projectIds);
}

function mergeSidebarSearchResults(
  localItems: CrossProjectSessionItem[],
  remoteSessions: WebSessionSummary[],
  archived: boolean,
  query: string,
  includeBody: boolean
) {
  const localByID = new Map(localItems.map(item => [item.session.id, item]));
  const merged: CrossProjectSessionItem[] = localItems.flatMap(item => {
    const searchMatchSources = resolveWebSessionSidebarSearchMatchSources(
      item.session,
      query,
      includeBody
    );
    if (searchMatchSources.length === 0) {
      return [];
    }
    return [
      {
        ...item,
        session: {
          ...item.session,
          searchMatchSources,
        },
      },
    ];
  });
  const mergedIndexByID = new Map(merged.map((item, index) => [item.session.id, index]));

  remoteSessions.forEach(session => {
    if (Boolean(session.archivedAt) !== archived) {
      return;
    }
    const existingIndex = mergedIndexByID.get(session.id);
    if (existingIndex !== undefined) {
      const existingItem = merged[existingIndex];
      merged[existingIndex] = {
        ...existingItem,
        session: {
          ...existingItem.session,
          searchMatchSources: mergeWebSessionSearchMatchSources(
            existingItem.session.searchMatchSources,
            session.searchMatchSources
          ),
        },
      };
      return;
    }
    const localItem = localByID.get(session.id);
    if (localItem) {
      merged.push({
        ...localItem,
        session: {
          ...localItem.session,
          searchMatchSources: mergeWebSessionSearchMatchSources(
            resolveWebSessionSidebarSearchMatchSources(localItem.session, query, includeBody),
            session.searchMatchSources
          ),
        },
      });
    } else {
      merged.push({
        session,
        projectId: session.projectId,
        projectName: getProjectName(session.projectId),
        isCurrent: archived
          ? activeArchivedPreviewId.value === session.id
          : session.projectId === props.projectId && session.id === activeSessionId.value,
      });
    }
    mergedIndexByID.set(session.id, merged.length - 1);
  });
  return sortCrossProjectSessionItems(merged);
}

const crossProjectSessions = computed<CrossProjectSessionItem[]>(() => {
  const rawItems: CrossProjectSessionItem[] = [];
  sidebarVisibleProjectIds.value.forEach(projectId => {
    webSessionStore.getSessions(projectId).forEach(session => {
      rawItems.push({
        session,
        projectId,
        projectName: getProjectName(projectId),
        isCurrent: projectId === props.projectId && session.id === activeSessionId.value,
      });
    });
  });
  return sortCrossProjectSessionItems(rawItems);
});

const filteredCrossProjectSessions = computed(() => {
  const query = normalizedSidebarSearchQuery.value;
  if (!query) {
    return crossProjectSessions.value;
  }
  return mergeSidebarSearchResults(
    crossProjectSessions.value,
    sidebarSearchState.value.items,
    false,
    query,
    sidebarSearchBody.value
  );
});

const baseCrossProjectArchivedSessions = computed<CrossProjectSessionItem[]>(() => {
  const items = webSessionStore
    .getArchivedSessions(sidebarVisibleProjectIds.value)
    .map(session => ({
      session,
      projectId: session.projectId,
      projectName: getProjectName(session.projectId),
      isCurrent: activeArchivedPreviewId.value === session.id,
    }));
  return withProjectBadges(items);
});

const searchedCrossProjectArchivedSessions = computed<CrossProjectSessionItem[]>(() => {
  return mergeSidebarSearchResults(
    baseCrossProjectArchivedSessions.value,
    sidebarSearchState.value.items,
    true,
    normalizedSidebarSearchQuery.value,
    sidebarSearchBody.value
  );
});

const showArchivedSidebarSection = computed(
  () => normalizedSidebarSearchQuery.value.length === 0 || sidebarSearchArchived.value
);

const crossProjectArchivedSessions = computed(() =>
  !showArchivedSidebarSection.value
    ? []
    : archivedSidebarSearchActive.value
      ? searchedCrossProjectArchivedSessions.value
      : baseCrossProjectArchivedSessions.value
);

const baseArchivedSidebarMeta = computed(() =>
  webSessionStore.getArchivedMeta(sidebarVisibleProjectIds.value)
);

const archivedSidebarMeta = computed(() => {
  if (!showArchivedSidebarSection.value) {
    return {
      scopeKey: '',
      total: 0,
      offset: 0,
      hasMore: false,
      loading: false,
    };
  }
  if (!archivedSidebarSearchActive.value) {
    return baseArchivedSidebarMeta.value;
  }
  const state = sidebarSearchState.value;
  const archivedTotal = searchedCrossProjectArchivedSessions.value.length;
  return {
    scopeKey: 'sidebar-search',
    total: archivedTotal,
    offset: archivedTotal,
    hasMore: false,
    loading: state.loading,
  };
});

function cancelSidebarSearchRequest() {
  sidebarSearchRequestVersion += 1;
  sidebarSearchAbortController?.abort();
  sidebarSearchAbortController = null;
}

function clearSidebarSearchState(loading = false) {
  cancelSidebarSearchRequest();
  sidebarSearchState.value = {
    ...createSidebarSearchState(),
    loading,
  };
}

async function loadSidebarSearch() {
  const query = normalizedSidebarSearchQuery.value;
  const projectIds = [...sidebarVisibleProjectIds.value];
  if (!query || projectIds.length === 0) {
    clearSidebarSearchState();
    return;
  }

  cancelSidebarSearchRequest();
  const scopeKey = projectIds.join('|');
  const includeArchived = sidebarSearchArchived.value;
  const includeBody = sidebarSearchBody.value;
  const requestVersion = ++sidebarSearchRequestVersion;
  const abortController = new AbortController();
  sidebarSearchAbortController = abortController;
  sidebarSearchState.value = {
    ...createSidebarSearchState(),
    loading: true,
  };

  try {
    let cursor = '';
    while (true) {
      const result: SessionSearchChunkResult = await webSessionApi.search(
        {
          projectIds,
          query,
          includeArchived,
          includeBody,
          cursor,
          scanLimit: SIDEBAR_SEARCH_SCAN_LIMIT,
        },
        { signal: abortController.signal }
      );
      if (
        requestVersion !== sidebarSearchRequestVersion ||
        query !== normalizedSidebarSearchQuery.value ||
        scopeKey !== sidebarVisibleProjectIds.value.join('|') ||
        includeArchived !== sidebarSearchArchived.value ||
        includeBody !== sidebarSearchBody.value
      ) {
        return;
      }

      const done = result.done || !result.nextCursor;
      sidebarSearchState.value = {
        items: mergeWebSessionSidebarSearchPage(sidebarSearchState.value.items, result.items),
        scanned: sidebarSearchState.value.scanned + result.scanned,
        total: result.total,
        loading: !done,
        done,
        error: false,
      };
      if (done) {
        return;
      }
      cursor = result.nextCursor ?? '';
    }
  } catch (error) {
    if (requestVersion !== sidebarSearchRequestVersion || isAbortLikeError(error)) {
      return;
    }
    sidebarSearchState.value = {
      ...sidebarSearchState.value,
      loading: false,
      done: true,
      error: true,
    };
    console.error('[Web Session] Failed to search sidebar sessions', error);
  } finally {
    if (sidebarSearchAbortController === abortController) {
      sidebarSearchAbortController = null;
    }
  }
}

const scheduleSidebarSearch = useDebounceFn(() => {
  void loadSidebarSearch();
}, 300);

async function ensureArchivedScopeLoaded(projectIds: string[], limit = 20) {
  if (!projectIds.length || webSessionStore.hasArchivedScope(projectIds)) {
    return;
  }
  await webSessionStore.loadArchivedSessions(projectIds, {
    reset: true,
    limit,
  });
}

const isSingleSidebarProject = computed(() => {
  const ids = new Set(
    [...crossProjectSessions.value, ...crossProjectArchivedSessions.value]
      .map(item => item.projectId)
      .filter(Boolean)
  );
  return ids.size <= 1;
});

function clamp(min: number, value: number, max: number) {
  return Math.max(min, Math.min(max, value));
}

const currentSidebarEntries = computed<WebSessionSidebarSessionEntry<CrossProjectSessionItem>[]>(
  () =>
    filteredCrossProjectSessions.value.map(item => ({
      source: item,
      row: buildSidebarSessionRow(item, false),
    }))
);
const archivedSidebarEntries = computed<WebSessionSidebarSessionEntry<CrossProjectSessionItem>[]>(
  () =>
    crossProjectArchivedSessions.value.map(item => ({
      source: item,
      row: buildSidebarSessionRow(item, true),
    }))
);
const currentSidebarSections = computed(() =>
  groupWebSessionSidebarEntriesByDate({
    entries: currentSidebarEntries.value,
    getTimestamp: entry => resolveWebSessionSidebarSortTimestamp(entry.source.session),
    labels: {
      today: t('webSession.sessionGroupToday'),
      yesterday: t('webSession.sessionGroupYesterday'),
      lastSevenDays: t('webSession.sessionGroupLastSevenDays'),
      earlier: t('webSession.sessionGroupEarlier'),
    },
    now: liveStateClockMs.value,
  })
);
const sidebarIsEmpty = computed(
  () =>
    normalizedSidebarSearchQuery.value.length === 0 &&
    crossProjectSessions.value.length === 0 &&
    baseCrossProjectArchivedSessions.value.length === 0 &&
    baseArchivedSidebarMeta.value.total === 0 &&
    !baseArchivedSidebarMeta.value.loading
);
const sidebarHasNoSearchResults = computed(
  () =>
    normalizedSidebarSearchQuery.value.length > 0 &&
    sidebarSearchState.value.done &&
    !sidebarSearchState.value.error &&
    filteredCrossProjectSessions.value.length === 0 &&
    crossProjectArchivedSessions.value.length === 0
);
const sidebarVisibleSessionCount = computed(
  () => filteredCrossProjectSessions.value.length + archivedSidebarMeta.value.total
);
const sidebarSearchError = computed(
  () => normalizedSidebarSearchQuery.value.length > 0 && sidebarSearchState.value.error
);
const sidebarSearchProgressVisible = computed(
  () => normalizedSidebarSearchQuery.value.length > 0 && sidebarSearchState.value.loading
);
const sidebarSearchProgressPercentage = computed(() => {
  const { scanned, total } = sidebarSearchState.value;
  if (total <= 0) {
    return 0;
  }
  return Math.max(0, Math.min(100, Math.round((scanned / total) * 100)));
});
const archivedSidebarEmptyLabel = computed(() => {
  if (!archivedSidebarSearchActive.value) {
    return t('webSession.archivedSessionsEmpty');
  }
  return sidebarSearchState.value.error
    ? t('webSession.sidebarArchivedSearchFailed')
    : t('webSession.sidebarArchivedSearchNoResults');
});
const sidebarVirtualItems = computed<WebSessionSidebarVirtualItem<CrossProjectSessionItem>[]>(() =>
  buildWebSessionSidebarVirtualItems({
    currentSections: currentSidebarSections.value,
    collapsedSectionKeys: resolveWebSessionSidebarCollapsedKeys({
      collapsedSectionKeys: collapsedSessionGroupKeys.value,
      searchActive: normalizedSidebarSearchQuery.value.length > 0,
    }),
    showArchived: showArchivedSidebarSection.value,
    archived: archivedSidebarEntries.value,
    archivedLabel: t('webSession.sessionGroupArchived'),
    archivedEmptyLabel: archivedSidebarEmptyLabel.value,
    archivedLoadingLabel: t('common.loading'),
    archivedTotal: archivedSidebarMeta.value.total,
    archivedLoading: archivedSidebarMeta.value.loading,
    archivedHasMore: archivedSidebarMeta.value.hasMore,
    loadMoreLabel: archivedSidebarMeta.value.loading
      ? t('common.loading')
      : t('webSession.loadMoreArchived'),
  })
);

const agentOptions: Array<{ label: string; value: WebSessionAgent }> = [
  { label: 'Codex', value: 'codex' },
  { label: 'Claude', value: 'claude' },
  { label: 'Pi', value: 'pi' },
];
const showAdditionalCodexModels = ref(false);
const showModelSelector = ref(false);
const showReasoningSelector = ref(false);
const keepAdditionalCodexModelsForNextOpen = ref(false);
type ComposerHoverSelector = 'model' | 'reasoning';
const COMPOSER_SELECTOR_HOVER_CLOSE_DELAY = 120;
const composerSelectorHoverCloseTimers: Record<ComposerHoverSelector, number | null> = {
  model: null,
  reasoning: null,
};
const MODEL_SELECT_MIN_WIDTH = 66;
const MODEL_SELECT_MAX_WIDTH = 112;
const MODEL_SELECT_BASE_WIDTH = 46;
const MODEL_SELECT_CHAR_WIDTH = 6.5;
const modelSelectMenuProps = {
  class: 'web-session-model-select-menu',
  style: { minWidth: '132px', maxWidth: '180px' },
  onMouseenter: () => handleComposerSelectorPointerEnter('model'),
  onMouseleave: () => handleComposerSelectorPointerLeave('model'),
};
const reasoningSelectMenuProps = {
  class: 'web-session-reasoning-select-menu',
  onMouseenter: () => handleComposerSelectorPointerEnter('reasoning'),
  onMouseleave: () => handleComposerSelectorPointerLeave('reasoning'),
};
const claudeRuntimeSelectMenuProps = {
  class: 'web-session-claude-runtime-select-menu',
  style: { minWidth: '172px' },
};

function renderAgentDropdownLabel(option: DropdownOption) {
  const value = String(option.key ?? option.value ?? '');
  const label = String(option.label ?? value);
  return h('span', { class: 'composer-agent-option' }, [
    h('span', {
      class: 'composer-agent-option-icon',
      innerHTML: getAgentIcon(value),
    }),
    h('span', { class: 'composer-agent-option-label' }, label),
  ]);
}

const agentDropdownOptions = computed<DropdownOption[]>(() =>
  agentOptions.map(option => ({
    label:
      option.value === 'codex' && isCodexCompatibilityMode.value
        ? t('webSession.codexCompatibilityAgentLabel')
        : option.value === 'pi' && !runtimePiCapability.value.supportsWebSession
          ? piUnavailableAgentLabel.value
          : option.label,
    key: option.value,
    value: option.value,
    disabled:
      agentSwitchDisabled.value ||
      (option.value === 'pi'
        ? !runtimePiCapability.value.supportsWebSession
        : runtimeConfig.value !== null && !runtimeCapabilityFor(option.value).supportsWebSession),
  }))
);

const agentSwitchDisabled = computed(() => Boolean(currentSession.value?.nativeSessionId));

function getAgentIcon(agent: WebSessionAgent | string) {
  if (agent === 'claude') {
    return getAssistantIconByType('claude-code');
  }
  return getAssistantIconByType(agent === 'pi' ? 'pi' : 'codex');
}

function getAgentDisplayName(agent: WebSessionAgent) {
  return agent === 'claude' ? 'Claude Code' : agent === 'pi' ? 'Pi' : 'Codex';
}

const selectedAgentTitle = computed(() =>
  t('webSession.agentSelectorTitle', { agent: selectedAgentLabel.value })
);
const selectedAgentIcon = computed(() => getAgentIcon(selectedAgent.value));

async function handleAgentDropdownSelect(key: string | number) {
  if (agentSwitchDisabled.value) {
    return;
  }
  const next = String(key);
  if (next !== 'claude' && next !== 'codex' && next !== 'pi') {
    return;
  }
  if (next === 'pi' && !(await ensurePiProjectTrust(true))) {
    return;
  }
  selectedAgent.value = next;
}

async function ensurePiProjectTrust(forAgentSelection = false) {
  const projectId = props.projectId;
  if (!projectId) {
    return false;
  }
  try {
    const status = await projectApi.getPiTrust(projectId);
    if (projectId !== props.projectId) {
      return false;
    }
    piTrustServerProjectPath.value = status.projectPath || '';
    if (isPiProjectTrusted(status, projectId)) {
      return true;
    }
    pendingPiAgentSelection.value = forAgentSelection;
    showPiTrustDialog.value = true;
    return false;
  } catch (error) {
    console.error('Failed to load Pi project access:', error);
    message.error(t('project.piTrustLoadFailed'));
    return false;
  }
}

function handlePiProjectTrusted(status: ProjectAgentTrustStatus) {
  if (!isPiProjectTrusted(status, props.projectId)) {
    pendingPiAgentSelection.value = false;
    return;
  }
  piTrustServerProjectPath.value = status.projectPath || '';
  if (pendingPiAgentSelection.value && !agentSwitchDisabled.value) {
    selectedAgent.value = 'pi';
  }
  pendingPiAgentSelection.value = false;
}

function getKnownModelLabel(value?: string | null) {
  const normalizedModel = String(value || '').trim();
  if (!normalizedModel) {
    return '';
  }
  return (
    [
      ...CLAUDE_MODEL_OPTIONS,
      ...CODEX_MODEL_OPTIONS,
      ...resolvePiModelOptions(runtimeConfig.value?.piModels ?? []),
    ].find(option => option.value === normalizedModel)?.label ?? normalizedModel
  );
}

function renderModelOption(info: {
  node: VNode;
  option: { type?: unknown; label?: unknown; menuLabel?: unknown };
  selected: boolean;
}) {
  if (info.option.type === 'group') {
    return info.node;
  }
  return h('div', info.node.props ?? {}, [
    h(
      'div',
      {
        class: 'n-base-select-option__content',
      },
      String(info.option.menuLabel ?? info.option.label ?? '')
    ),
    info.selected
      ? h(
          'div',
          {
            class: 'n-base-select-option__check',
          },
          '✓'
        )
      : null,
  ]);
}

function withCurrentModelOption(
  options: Array<{ label: string; value: string }>,
  currentModel?: string | null
) {
  const normalizedModel = String(currentModel || '').trim();
  if (!normalizedModel) {
    return options;
  }
  if (options.some(option => option.value === normalizedModel)) {
    return options;
  }
  return [
    ...options,
    {
      label: getKnownModelLabel(normalizedModel),
      value: normalizedModel,
    },
  ];
}

function withCurrentPiModelOption(
  groups: WebSessionModelOptionGroup[],
  currentModel?: string | null
): WebSessionModelOptionGroup[] {
  const normalizedModel = String(currentModel || '').trim();
  if (
    !normalizedModel ||
    groups.some(group => group.children.some(option => option.value === normalizedModel))
  ) {
    return groups;
  }
  const separator = normalizedModel.indexOf('/');
  const provider = separator > 0 ? normalizedModel.slice(0, separator) : t('common.current');
  const currentOption = {
    label: getKnownModelLabel(normalizedModel),
    value: normalizedModel,
  };
  const matchingGroup = groups.find(group => group.label === provider);
  if (!matchingGroup) {
    return [
      ...groups,
      {
        type: 'group',
        key: `pi-provider-current-${provider}`,
        label: provider,
        children: [currentOption],
      },
    ];
  }
  return groups.map(group =>
    group === matchingGroup ? { ...group, children: [...group.children, currentOption] } : group
  );
}

function getModelLabelVisualLength(label: string) {
  return Array.from(label).reduce(
    (total, char) => (/[\u4e00-\u9fff]/.test(char) ? total + 2 : total + 1),
    0
  );
}

function resolveModelSelectWidth(label: string) {
  const visualLength = Math.max(1, getModelLabelVisualLength(label.trim()));
  const width = Math.round(MODEL_SELECT_BASE_WIDTH + visualLength * MODEL_SELECT_CHAR_WIDTH);
  return clamp(MODEL_SELECT_MIN_WIDTH, width, MODEL_SELECT_MAX_WIDTH);
}

function defaultModelForAgent(agent: WebSessionAgent) {
  return resolveDefaultModelForAgent(agent, developerConfig.value.webSessionCodexDefaultModel);
}

function defaultReasoningEffortForAgent(agent: WebSessionAgent): WebSessionReasoningEffort {
  return resolveDefaultReasoningEffortForAgent(
    agent,
    developerConfig.value.webSessionCodexDefaultReasoningEffort
  );
}

function defaultPermissionLevelForAgent(agent: WebSessionAgent): 'default' | 'elevated' | 'yolo' {
  return resolveDefaultPermissionLevelForAgent(
    agent,
    developerConfig.value.webSessionCodexDefaultPermissionLevel
  );
}

function reasoningEffortLabel(effort: WebSessionReasoningEffort) {
  switch (effort) {
    case 'default':
      return t('common.default');
    case 'none':
      return 'Off';
    case 'minimal':
      return 'Minimal';
    case 'low':
      return 'Low';
    case 'medium':
      return 'Mid';
    case 'high':
      return 'High';
    case 'xhigh':
      return 'Xhigh';
    case 'max':
      return 'Max';
    case 'ultra':
      return 'Ultra';
  }
}

function supportedCodexReasoningEfforts(model: string) {
  return resolveCodexReasoningEfforts(model, codexRuntimeConfig.value?.models ?? []);
}

function reasoningEffortForModel(
  model: string,
  currentEffort: WebSessionReasoningEffort
): WebSessionReasoningEffort {
  const supported = supportedCodexReasoningEfforts(model);
  if (!supported || currentEffort === 'default' || supported.includes(currentEffort)) {
    return currentEffort;
  }
  return 'default';
}

function withCurrentReasoningEffortOption(
  options: Array<{ label: string; value: string }>,
  currentEffort?: string | null
) {
  const normalizedEffort = String(currentEffort || '')
    .trim()
    .toLowerCase();
  if (!normalizedEffort) {
    return options;
  }
  if (options.some(option => option.value === normalizedEffort)) {
    return options;
  }
  return [
    ...options,
    {
      label: `${normalizedEffort} (Current)`,
      value: normalizedEffort,
    },
  ];
}

function handleModelSelectorShowChange(show: boolean) {
  if (show && !keepAdditionalCodexModelsForNextOpen.value) {
    showAdditionalCodexModels.value = false;
  }
  if (show) {
    keepAdditionalCodexModelsForNextOpen.value = false;
  }
  showModelSelector.value = show;
}

function handleReasoningSelectorShowChange(show: boolean) {
  if (!isMobile.value) {
    showReasoningSelector.value = show;
  }
}

function clearComposerSelectorHoverCloseTimer(selector: ComposerHoverSelector) {
  const timer = composerSelectorHoverCloseTimers[selector];
  if (timer !== null) {
    window.clearTimeout(timer);
    composerSelectorHoverCloseTimers[selector] = null;
  }
}

function setComposerSelectorShow(selector: ComposerHoverSelector, show: boolean) {
  if (selector === 'model') {
    handleModelSelectorShowChange(show);
    return;
  }
  showReasoningSelector.value = show;
}

function handleComposerSelectorPointerEnter(selector: ComposerHoverSelector) {
  if (isMobile.value) {
    return;
  }
  clearComposerSelectorHoverCloseTimer(selector);
  setComposerSelectorShow(selector, true);
}

function handleComposerSelectorPointerLeave(selector: ComposerHoverSelector) {
  if (isMobile.value) {
    return;
  }
  clearComposerSelectorHoverCloseTimer(selector);
  composerSelectorHoverCloseTimers[selector] = window.setTimeout(() => {
    composerSelectorHoverCloseTimers[selector] = null;
    setComposerSelectorShow(selector, false);
  }, COMPOSER_SELECTOR_HOVER_CLOSE_DELAY);
}

const selectedModelDisplayLabel = computed(() => getKnownModelLabel(selectedModel.value));
const modelSelectStyle = computed<CSSProperties>(() => ({
  width: `${resolveModelSelectWidth(selectedModelDisplayLabel.value)}px`,
}));
const claudeRuntimeOptions = computed(() =>
  CLAUDE_RUNTIME_OPTIONS.map(option => ({
    ...option,
    label: option.label,
  }))
);

const piModelOptionGroups = computed(() =>
  resolvePiModelOptionGroups(runtimeConfig.value?.piModels ?? [])
);

const modelOptions = computed(() => {
  const activeModel = currentSession.value?.model ?? draftModel.value;
  if (selectedAgent.value === 'claude') {
    return [
      ...withCurrentModelOption(CLAUDE_MODEL_OPTIONS, activeModel),
      { label: t('webSession.customModel'), value: CUSTOM_MODEL_VALUE },
    ];
  }
  if (selectedAgent.value === 'pi') {
    return withCurrentPiModelOption(piModelOptionGroups.value, activeModel);
  }
  if (!showAdditionalCodexModels.value) {
    return [
      ...withCurrentModelOption(CODEX_PRIMARY_MODEL_OPTIONS, activeModel),
      { label: t('webSession.customModel'), value: CUSTOM_MODEL_VALUE },
      { label: t('webSession.moreModels'), value: MORE_MODELS_VALUE },
    ];
  }
  const primaryOptions = withCurrentModelOption(CODEX_PRIMARY_MODEL_OPTIONS, activeModel);
  const additionalOptions = CODEX_ADDITIONAL_MODEL_OPTIONS.filter(
    option => !primaryOptions.some(primary => primary.value === option.value)
  );
  return [
    ...primaryOptions,
    { label: t('webSession.customModel'), value: CUSTOM_MODEL_VALUE },
    ...additionalOptions,
  ];
});

const reasoningEffortOptions = computed(() => {
  if (selectedAgent.value === 'pi') {
    const values = resolvePiReasoningEfforts(
      runtimeConfig.value?.piModels ?? [],
      selectedModel.value
    );
    return values.map(value => ({ label: reasoningEffortLabel(value), value }));
  }
  const supported =
    selectedAgent.value === 'codex' ? supportedCodexReasoningEfforts(selectedModel.value) : null;
  if (supported) {
    return ['default', ...supported].map(value => ({
      label: reasoningEffortLabel(value as WebSessionReasoningEffort),
      value,
    }));
  }
  const options: Array<{ label: string; value: WebSessionReasoningEffort }> = [
    'default',
    'none',
    'low',
    'medium',
    'high',
    'xhigh',
  ].map(value => ({
    label: reasoningEffortLabel(value as WebSessionReasoningEffort),
    value: value as WebSessionReasoningEffort,
  }));
  const activeEffort = currentSession.value?.reasoningEffort ?? draftReasoningEffort.value;
  return withCurrentReasoningEffortOption(options, activeEffort);
});

const selectedAgent = computed({
  get: () => currentSession.value?.agent ?? draftAgent.value,
  set: value => {
    const next = value as WebSessionAgent;
    const firstPiModel = runtimeConfig.value?.piModels?.[0];
    const nextModel =
      next === 'pi' && firstPiModel
        ? `${firstPiModel.provider}/${firstPiModel.id}`
        : defaultModelForAgent(next);
    const nextReasoningEffort = defaultReasoningEffortForAgent(next);
    const nextPermissionLevel = defaultPermissionLevelForAgent(next);
    draftAgent.value = next;
    if (next === 'codex') {
      draftClaudeRuntime.value = 'claude';
    }
    draftModel.value = nextModel;
    draftReasoningEffort.value = nextReasoningEffort;
    draftPermissionLevel.value = nextPermissionLevel;
    if (isDraftSession(currentSession.value)) {
      updateActiveDraftSession(current => ({
        ...current,
        agent: next,
        claudeRuntime:
          next === 'claude'
            ? current.claudeRuntime === 'ccr'
              ? 'ccr'
              : draftClaudeRuntime.value
            : 'claude',
        model: nextModel,
        reasoningEffort: nextReasoningEffort,
        permissionLevel: nextPermissionLevel,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      void webSessionStore.updateAgent(currentRealSession.value.id, next).catch(error => {
        message.error(error instanceof Error ? error.message : t('common.error'));
      });
    }
  },
});

const selectedModel = computed({
  get: () => currentSession.value?.model ?? draftModel.value,
  set: value => {
    const next = String(value);
    if (next === MORE_MODELS_VALUE) {
      showAdditionalCodexModels.value = true;
      keepAdditionalCodexModelsForNextOpen.value = true;
      nextTick(() => {
        handleModelSelectorShowChange(true);
      });
      return;
    }
    if (next === CUSTOM_MODEL_VALUE) {
      openCustomModelDialog();
      return;
    }
    const currentEffort = currentSession.value?.reasoningEffort ?? draftReasoningEffort.value;
    const nextEffort =
      selectedAgent.value === 'codex'
        ? reasoningEffortForModel(next, currentEffort)
        : currentEffort;
    draftModel.value = next;
    draftReasoningEffort.value = nextEffort;
    if (isDraftSession(currentSession.value)) {
      updateActiveDraftSession(current => ({
        ...current,
        model: next,
        reasoningEffort: nextEffort,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      const noticeKey = getRuntimeSwitchNoticeKey();
      const sessionID = currentRealSession.value.id;
      void (async () => {
        await webSessionStore.updateModel(sessionID, next);
        if (nextEffort !== currentEffort) {
          await webSessionStore.updateReasoningEffort(sessionID, nextEffort);
        }
      })()
        .then(() => showRuntimeSwitchNotice(noticeKey))
        .catch(error => {
          message.error(error instanceof Error ? error.message : t('common.error'));
        });
    }
  },
});

const selectedClaudeRuntime = computed<WebSessionClaudeRuntimeOption>({
  get: () => currentSession.value?.claudeRuntime ?? draftClaudeRuntime.value,
  set: value => {
    const next: WebSessionClaudeRuntimeOption = value === 'ccr' ? 'ccr' : 'claude';
    draftClaudeRuntime.value = next;
    if (isDraftSession(currentSession.value)) {
      updateActiveDraftSession(current => ({
        ...current,
        claudeRuntime: next,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      const noticeKey = getRuntimeSwitchNoticeKey();
      void webSessionStore
        .updateClaudeRuntime(currentRealSession.value.id, next)
        .then(() => showRuntimeSwitchNotice(noticeKey))
        .catch(error => {
          message.error(error instanceof Error ? error.message : t('common.error'));
        });
    }
  },
});

const selectedReasoningEffort = computed<WebSessionReasoningEffort>({
  get: () => currentSession.value?.reasoningEffort ?? draftReasoningEffort.value,
  set: value => {
    const next = value as WebSessionReasoningEffort;
    draftReasoningEffort.value = next;
    if (isDraftSession(currentSession.value)) {
      updateActiveDraftSession(current => ({
        ...current,
        reasoningEffort: next,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      const noticeKey = getRuntimeSwitchNoticeKey();
      void webSessionStore
        .updateReasoningEffort(currentRealSession.value.id, next)
        .then(() => showRuntimeSwitchNotice(noticeKey))
        .catch(error => {
          message.error(error instanceof Error ? error.message : t('common.error'));
        });
    }
  },
});

const permissionLevelOptions = computed(() => {
  if (selectedAgent.value === 'pi') {
    return [{ label: t('webSession.permissionUnrestricted'), value: 'elevated' }];
  }
  return [
    ...(selectedAgent.value === 'claude'
      ? []
      : [{ label: t('webSession.permissionDefault'), value: 'default' }]),
    { label: t('webSession.permissionElevated'), value: 'elevated' },
    { label: t('webSession.permissionYolo'), value: 'yolo' },
  ];
});

const selectedWorkflowMode = computed<'default' | 'plan'>({
  get: () => currentSession.value?.workflowMode ?? draftWorkflowMode.value,
  set: value => {
    const next = value as 'default' | 'plan';
    draftWorkflowMode.value = next;
    if (isDraftSession(currentSession.value)) {
      updateActiveDraftSession(current => ({
        ...current,
        workflowMode: next,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      const noticeKey = getRuntimeSwitchNoticeKey();
      void webSessionStore
        .updateWorkflowMode(currentRealSession.value.id, next)
        .then(() => showRuntimeSwitchNotice(noticeKey))
        .catch(error => {
          message.error(error instanceof Error ? error.message : t('common.error'));
        });
    }
  },
});

const selectedPermissionLevel = computed<'default' | 'elevated' | 'yolo'>({
  get: () => {
    const value = currentSession.value?.permissionLevel ?? draftPermissionLevel.value;
    if (selectedAgent.value === 'claude' && value === 'default') {
      return 'elevated';
    }
    return value;
  },
  set: value => {
    const next =
      selectedAgent.value === 'claude' && value === 'default'
        ? 'elevated'
        : (value as 'default' | 'elevated' | 'yolo');
    draftPermissionLevel.value = next;
    if (isDraftSession(currentSession.value)) {
      updateActiveDraftSession(current => ({
        ...current,
        permissionLevel: next,
        updatedAt: new Date().toISOString(),
      }));
      return;
    }
    if (currentRealSession.value) {
      const noticeKey = getRuntimeSwitchNoticeKey();
      void webSessionStore
        .updatePermissionLevel(currentRealSession.value.id, next)
        .then(() => showRuntimeSwitchNotice(noticeKey))
        .catch(error => {
          message.error(error instanceof Error ? error.message : t('common.error'));
        });
    }
  },
});

const refreshTabSortable = useDebounceFn(() => {
  nextTick(() => {
    setupTabSorting();
  });
}, 100);

let tabScrollContainer: HTMLElement | null = null;

function refreshTabHeaderLayout() {
  if (isMobile.value) {
    cleanupTabScrollListener();
    destroyTabSorting();
    activeTabIndicatorStyle.value = hiddenCardTabIndicatorStyle();
    return;
  }

  nextTick(() => {
    recalcTabTitleWidth();
    setupTabScrollListener();
    refreshTabSortable();
    updateActiveTabIndicator();
    syncActiveTabIntoView();
  });
}

function setWorkflowMode(mode: 'default' | 'plan') {
  draftWorkflowMode.value = mode;
  const session = currentSession.value;
  if (!session) {
    return;
  }
  if (isDraftSession(session)) {
    updateActiveDraftSession(current => ({
      ...current,
      workflowMode: mode,
      updatedAt: new Date().toISOString(),
    }));
    return;
  }
  const noticeKey = getRuntimeSwitchNoticeKey();
  void webSessionStore
    .updateWorkflowMode(session.id, mode)
    .then(() => showRuntimeSwitchNotice(noticeKey))
    .catch(error => {
      message.error(error instanceof Error ? error.message : t('common.error'));
    });
}

function openCustomModelDialog() {
  const inputValue = ref((currentSession.value?.model ?? draftModel.value).trim());
  dialog.create({
    title: t('webSession.customModelTitle'),
    content: () =>
      h(NInput, {
        value: inputValue.value,
        'onUpdate:value': (value: string) => {
          inputValue.value = value;
        },
        maxlength: 128,
        autofocus: true,
        placeholder: t('webSession.customModelPlaceholder'),
      }),
    positiveText: t('common.save'),
    negativeText: t('common.cancel'),
    showIcon: false,
    maskClosable: false,
    closeOnEsc: true,
    onPositiveClick: async () => {
      const nextModel = inputValue.value.trim();
      if (!nextModel) {
        message.warning(t('webSession.customModelEmpty'));
        return false;
      }
      const currentEffort = currentSession.value?.reasoningEffort ?? draftReasoningEffort.value;
      const nextEffort =
        selectedAgent.value === 'codex'
          ? reasoningEffortForModel(nextModel, currentEffort)
          : currentEffort;
      draftModel.value = nextModel;
      draftReasoningEffort.value = nextEffort;
      if (!currentSession.value) {
        return true;
      }
      if (isDraftSession(currentSession.value)) {
        updateActiveDraftSession(current => ({
          ...current,
          model: nextModel,
          reasoningEffort: nextEffort,
          updatedAt: new Date().toISOString(),
        }));
        return true;
      }
      try {
        const sessionID = currentSession.value.id;
        await webSessionStore.updateModel(sessionID, nextModel);
        if (nextEffort !== currentEffort) {
          await webSessionStore.updateReasoningEffort(sessionID, nextEffort);
        }
        return true;
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('common.error'));
        return false;
      }
    },
  });
}

function formatTime(timestamp: number) {
  return formatWebSessionTimestamp(timestamp, locale.value);
}

function formatDateTime(timestamp: number) {
  return formatWebSessionDateTime(timestamp, locale.value);
}

function formatIsoTime(value?: string | null) {
  const timestamp = Date.parse(typeof value === 'string' ? value : '');
  return Number.isFinite(timestamp) ? formatTime(timestamp) : '';
}

function formatIsoDateTime(value?: string | null) {
  const timestamp = Date.parse(typeof value === 'string' ? value : '');
  return Number.isFinite(timestamp) ? formatDateTime(timestamp) : '';
}

function getLiveElapsedText(state: WebSessionLiveState) {
  return resolveWebSessionLiveTimeCopy({
    state,
    blocks: blocks.value,
    session: currentRealSession.value,
    pendingApproval: pendingApproval.value,
    pendingUserInput: pendingUserInput.value,
    now: liveStateClockMs.value,
    labels: {
      startedAt: t('webSession.liveTooltipStartedAt'),
      elapsed: t('webSession.liveTooltipElapsed'),
      sinceLastActivity: t('webSession.liveTooltipSinceLastActivity'),
    },
    formatTime,
    formatDateTime,
  }).timeText;
}

function getLiveTimeText(state: WebSessionLiveState) {
  const elapsed = getLiveElapsedText(state);
  if (elapsed) {
    return elapsed;
  }
  return formatTime(state.updatedAt);
}

function getLiveTimeTooltipItems(state: WebSessionLiveState): WebSessionLiveTimeTooltipItem[] {
  return resolveWebSessionLiveTimeCopy({
    state,
    blocks: blocks.value,
    session: currentRealSession.value,
    pendingApproval: pendingApproval.value,
    pendingUserInput: pendingUserInput.value,
    now: liveStateClockMs.value,
    labels: {
      startedAt: t('webSession.liveTooltipStartedAt'),
      elapsed: t('webSession.liveTooltipElapsed'),
      sinceLastActivity: t('webSession.liveTooltipSinceLastActivity'),
    },
    formatTime,
    formatDateTime,
  }).tooltipItems;
}

const {
  stringifyValue,
  asRecord,
  extractToolWorkingDirectory,
  getImageViewToolData,
  isImageViewTool,
  getImageViewDisplayName,
  getImageViewDisplayPath,
  normalizeToolKindValue,
  isCompactToolKind,
  compactToolLabel,
  isCompactTool,
  isInteractiveDynamicTool,
  toolCardClass,
  getCompactToolSummary,
  getCompactToolDisplaySummary,
  toolKindLabel,
  formatToolPreview,
} = createWebSessionToolPresentation({
  translate: t,
  shouldShowPending: shouldShowToolPendingPlaceholder,
});
function getImageViewPreviewSrc(tool?: NonNullable<WebSessionBlock['tool']>) {
  if (!tool) {
    return '';
  }
  return imageViewPreviewSrcByToolId.value[tool.id] ?? '';
}

function getImageViewPreviewState(tool?: NonNullable<WebSessionBlock['tool']>) {
  if (!tool) {
    return 'loading' as const;
  }
  return imageViewPreviewStateByToolId.value[tool.id] ?? 'loading';
}

function ensureImageViewPreview(tool: NonNullable<WebSessionBlock['tool']>) {
  if (imageViewPreviewSrcByToolId.value[tool.id]) {
    if (imageViewPreviewStateByToolId.value[tool.id] === 'error') {
      imageViewPreviewStateByToolId.value = {
        ...imageViewPreviewStateByToolId.value,
        [tool.id]: 'loading',
      };
    }
    return;
  }

  const data = getImageViewToolData(tool);
  if (!data) {
    return;
  }

  const previewSrc = buildImageViewPreviewUrl(data.path, {
    cwd: data.cwd || extractToolWorkingDirectory(tool.input) || currentRealSession.value?.cwd,
  });
  if (!previewSrc) {
    return;
  }

  imageViewPreviewSrcByToolId.value = {
    ...imageViewPreviewSrcByToolId.value,
    [tool.id]: previewSrc,
  };
  imageViewPreviewStateByToolId.value = {
    ...imageViewPreviewStateByToolId.value,
    [tool.id]: 'loading',
  };
}

function handleImageViewPreviewLoad(toolId: string) {
  imageViewPreviewStateByToolId.value = {
    ...imageViewPreviewStateByToolId.value,
    [toolId]: 'ready',
  };
}

function handleImageViewPreviewError(toolId: string) {
  imageViewPreviewStateByToolId.value = {
    ...imageViewPreviewStateByToolId.value,
    [toolId]: 'error',
  };
}

function getCompactToolCount(tool: NonNullable<WebSessionBlock['tool']>) {
  return Math.max(1, Number(tool.commandGroup?.count ?? 1) || 1);
}

function activityDisplayLabel(block: WebSessionBlock) {
  if (!block.tool) {
    return timelineRoleLabel(block);
  }
  if (isReasoningBlock(block)) {
    return t('webSession.toolReasoning');
  }
  return compactToolLabel(block.tool);
}

function activityDisplaySummary(block: WebSessionBlock) {
  if (!block.tool) {
    return '';
  }
  if (isReasoningBlock(block)) {
    const output = String(block.tool.output ?? '')
      .replace(/\s+/g, ' ')
      .trim();
    if (output) {
      return output.slice(0, 160);
    }
  }
  if (isCompactTool(block.tool)) {
    return getCompactToolDisplaySummary(block.tool);
  }
  return formatToolPreview(block.tool) || t('webSession.compactToolNoSummary');
}

function getActivityDisplayCount(block: WebSessionBlock) {
  return block.tool ? getCompactToolCount(block.tool) : 1;
}

function getActivityDisplayTitle(block: WebSessionBlock) {
  const parts = [
    activityDisplayLabel(block),
    formatTime(block.timestamp),
    activityDisplaySummary(block),
  ]
    .map(value => String(value ?? '').trim())
    .filter(Boolean);
  return parts.join(' · ');
}

function handleActivityDisplayClick(block: WebSessionBlock) {
  if (!block.tool) {
    return;
  }
  if (isCompactTool(block.tool)) {
    void openCommandExecutionDetail(block);
    return;
  }
  toggleToolExpanded(block.tool);
}

function shouldHideTimelineMeta(item: WebSessionBlock) {
  if (!Number.isFinite(item.timestamp) || item.timestamp <= 0) {
    return true;
  }
  if (timelineSubAgent(item)) {
    return false;
  }
  return item.kind === 'tool' && item.tool ? isCompactTool(item.tool) : false;
}

function canPreviewAttachment(attachment: { name: string; mime?: string }) {
  const normalizedMime = attachment.mime?.trim().toLowerCase();
  if (normalizedMime) {
    return normalizedMime.startsWith('image/');
  }
  return IMAGE_ATTACHMENT_NAME_PATTERN.test(attachment.name);
}

function attachmentPreviewMode(attachment: { name: string; mime?: string }) {
  return resolveWebSessionAttachmentPreviewMode({
    previewable: canPreviewAttachment(attachment),
    isMobile: isMobile.value,
  });
}

function shouldUseAttachmentHoverPreview(attachment: { name: string; mime?: string }) {
  return attachmentPreviewMode(attachment) === 'popover';
}

function shouldUseAttachmentModalPreview(attachment: { name: string; mime?: string }) {
  return attachmentPreviewMode(attachment) === 'modal';
}

function getAttachmentPreviewUrl(attachmentID: string) {
  const normalizedID = String(attachmentID || '').trim();
  if (!normalizedID) {
    return '';
  }
  const path = `/api/v1/web-sessions/attachments/${encodeURIComponent(normalizedID)}`;
  return urlBase ? new URL(path, urlBase).toString() : path;
}

function openAttachmentPreview(attachment: { id: string; name: string; mime?: string }) {
  if (!canPreviewAttachment(attachment)) {
    return;
  }
  activeAttachmentPreview.value = {
    id: attachment.id,
    name: attachment.name,
    url: getAttachmentPreviewUrl(attachment.id),
  };
  showAttachmentPreview.value = true;
}

function handleAttachmentPreviewVisibilityChange(show: boolean) {
  showAttachmentPreview.value = show;
  if (!show) {
    activeAttachmentPreview.value = null;
  }
}

const commandExecutionDetailTitle = computed(() =>
  activeCommandExecutionDetail.value
    ? t('webSession.compactToolDetailTitleWithCount', {
        kind: compactToolLabel(activeCommandExecutionDetail.value),
        count: activeCommandExecutionDetail.value.count,
      })
    : t('webSession.compactToolDetailTitle')
);

const commandExecutionDetailKindLabel = computed(() =>
  activeCommandExecutionDetail.value ? compactToolLabel(activeCommandExecutionDetail.value) : ''
);
function buildLocalCommandExecutionDetail(block: WebSessionBlock): CommandExecutionDetail | null {
  if (!block.tool) {
    return null;
  }
  const payload = asRecord(block.payload);
  const rawItems = Array.isArray(payload?.groupItems) ? payload?.groupItems : null;
  if (!rawItems || rawItems.length === 0) {
    return null;
  }
  const items: CommandExecutionDetailItem[] = [];
  rawItems.forEach(item => {
    const record = asRecord(item);
    if (!record) {
      return;
    }
    items.push({
      toolId: String(record.toolId ?? ''),
      kind: String(record.kind ?? ''),
      title: String(record.title ?? ''),
      summary: String(record.summary ?? ''),
      command: String(record.command ?? ''),
      input: record.input,
      output: typeof record.output === 'string' ? record.output : undefined,
      status:
        record.status === 'running' || record.status === 'error' || record.status === 'done'
          ? record.status
          : 'done',
      timestamp: typeof record.timestamp === 'string' ? record.timestamp : '',
      startedAt: typeof record.startedAt === 'string' ? record.startedAt : undefined,
      completedAt: typeof record.completedAt === 'string' ? record.completedAt : undefined,
    });
  });
  if (items.length === 0) {
    return null;
  }
  const groupId = block.tool.commandGroup?.id || block.tool.id;
  return {
    groupId,
    kind: block.tool.kind ?? '',
    title: block.tool.name,
    summary: getCompactToolSummary(block.tool),
    count: Math.max(
      items.length,
      Number(block.tool.commandGroup?.count ?? items.length) || items.length
    ),
    firstSeq: Number(block.tool.commandGroup?.firstSeq ?? 0),
    lastSeq: Number(block.tool.commandGroup?.lastSeq ?? 0),
    status: block.tool.status,
    latestToolId: block.tool.commandGroup?.latestToolId || block.tool.id,
    items,
  };
}

function isExpandableCommandGroup(block: WebSessionBlock) {
  const group = block.tool?.commandGroup;
  return Boolean(group && (group.compacted || group.count > 1));
}

async function openCommandExecutionDetail(block: WebSessionBlock) {
  if (!currentRealSession.value) {
    return;
  }
  const tool = block.tool;
  if (!tool) {
    return;
  }
  const groupId = tool.commandGroup?.id || tool.id;
  if (!groupId) {
    return;
  }

  activeCommandExecutionGroupId.value = groupId;
  showCommandExecutionDetail.value = true;
  loadingCommandExecutionDetail.value = true;
  const requestSessionId = currentRealSession.value.id;
  const requestGroupId = groupId;

  if (!isExpandableCommandGroup(block)) {
    const localDetail = buildLocalCommandExecutionDetail(block);
    if (localDetail) {
      activeCommandExecutionDetail.value = localDetail;
      loadingCommandExecutionDetail.value = false;
      return;
    }
  }

  try {
    const detail = await webSessionStore.loadCommandGroupDetail(requestSessionId, groupId);
    if (
      currentRealSession.value?.id === requestSessionId &&
      activeCommandExecutionGroupId.value === requestGroupId
    ) {
      activeCommandExecutionDetail.value = detail;
    }
  } catch (error) {
    if (
      currentRealSession.value?.id === requestSessionId &&
      activeCommandExecutionGroupId.value === requestGroupId
    ) {
      activeCommandExecutionDetail.value = null;
    }
    message.error(
      error instanceof Error && error.message
        ? error.message
        : t('webSession.compactToolLoadFailed')
    );
  } finally {
    if (
      currentRealSession.value?.id === requestSessionId &&
      activeCommandExecutionGroupId.value === requestGroupId
    ) {
      loadingCommandExecutionDetail.value = false;
    }
  }
}

function handleCommandExecutionDetailVisibilityChange(show: boolean) {
  showCommandExecutionDetail.value = show;
  if (!show) {
    activeCommandExecutionDetail.value = null;
    activeCommandExecutionGroupId.value = '';
    loadingCommandExecutionDetail.value = false;
  }
}

function isToolExpanded(toolId: string) {
  return Boolean(expandedTools.value[toolId]);
}

function toggleToolExpanded(tool: NonNullable<WebSessionBlock['tool']>) {
  const nextExpanded = !expandedTools.value[tool.id];
  if (nextExpanded && isImageViewTool(tool)) {
    ensureImageViewPreview(tool);
  }

  expandedTools.value = {
    ...expandedTools.value,
    [tool.id]: nextExpanded,
  };
}

function showPlanActions(toolId: string) {
  return Boolean(
    currentRealSession.value &&
      latestPlanToolId.value === toolId &&
      !activeScheduledPlanTargetIds.value.has(latestPlanItemId.value) &&
      (!liveState.value.running || inlinePlanChoice.value) &&
      !dismissedPlanActions.value[toolId] &&
      !hasUserMessageAfterLatestPlan.value
  );
}

function setPlanActionsDismissed(toolId: string, dismissed: boolean) {
  if (!toolId) {
    return;
  }
  dismissedPlanActions.value = {
    ...dismissedPlanActions.value,
    [toolId]: dismissed,
  };
}

function beginSessionArchive(sessionId: string) {
  archiveStateBySessionId.value = beginWebSessionSubmit(archiveStateBySessionId.value, sessionId);
}

function endSessionArchive(sessionId: string) {
  archiveStateBySessionId.value = endWebSessionSubmit(archiveStateBySessionId.value, sessionId);
}

function isSessionArchiving(sessionId: string) {
  return isWebSessionSubmitting(archiveStateBySessionId.value, sessionId);
}

function toolStateLabel(tool: { status: 'running' | 'done' | 'error' }) {
  if (tool.status === 'done') {
    return t('webSession.toolDone');
  }
  if (tool.status === 'error') {
    return t('webSession.toolError');
  }
  return t('webSession.toolRunning');
}

function timelineRoleLabel(item: WebSessionBlock) {
  const agent = timelineSubAgent(item);
  if (agent) {
    return t('webSession.timelineAgentRole', { name: agent.title });
  }
  if (item.kind === 'user') {
    return t('terminal.user');
  }
  if (item.kind === 'assistant') {
    return t('terminal.assistant');
  }
  if (item.kind === 'tool') {
    return item.tool?.name || t('webSession.toolKindDefault');
  }
  return t('common.info');
}

function historyInteractionTitle(item: WebSessionBlock) {
  switch (item.detail?.type) {
    case 'approval_request':
      return t('webSession.approvalTitle');
    case 'approval_response':
      return item.detail.action === 'reject'
        ? t('webSession.historyApprovalRejected')
        : t('webSession.historyApprovalApproved');
    case 'user_input_request':
      return t('webSession.userInputTitle');
    case 'user_input_response':
      return t('webSession.historyUserInputSubmitted');
    default:
      return t('common.info');
  }
}

function historyInteractionPrompt(item: WebSessionBlock) {
  if (item.detail?.type === 'user_input_request' && item.detail.questions?.length) {
    return '';
  }
  if (item.detail?.type === 'user_input_response' && item.detail.answers?.length) {
    return '';
  }
  return item.detail?.prompt?.trim() || item.text?.trim() || '';
}

function historyInteractionBadgeClass(item: WebSessionBlock) {
  switch (item.detail?.type) {
    case 'approval_request':
      return 'state-approval-request';
    case 'approval_response':
      return item.detail.action === 'reject' ? 'state-approval-reject' : 'state-approval-approve';
    case 'user_input_request':
      return 'state-user-input-request';
    case 'user_input_response':
      return 'state-user-input-response';
    default:
      return '';
  }
}

function historyInteractionCardClass(item: WebSessionBlock) {
  switch (item.detail?.type) {
    case 'approval_request':
      return 'type-approval-request';
    case 'approval_response':
      return item.detail.action === 'reject' ? 'type-approval-reject' : 'type-approval-approve';
    case 'user_input_request':
      return 'type-user-input-request';
    case 'user_input_response':
      return 'type-user-input-response';
    default:
      return '';
  }
}

function historyQuestionTitle(question: WebSessionUserInputQuestion) {
  return (
    question.header?.trim() || question.question?.trim() || t('webSession.historyQuestionLabel')
  );
}

function formatHistoryAnswerValues(answer: WebSessionHistoryAnswerEntry) {
  if (answer.masked) {
    return answer.values.map(() => t('webSession.historyMaskedAnswer'));
  }
  return answer.values;
}

async function initializeProjectSessions(projectId: string) {
  if (!projectId) {
    return;
  }
  const initialization = projectSessionInitializationGate.begin(projectId);
  const isCurrentInitialization = () =>
    projectSessionInitializationGate.isCurrent(initialization, props.projectId);
  if (!isCurrentInitialization()) {
    return;
  }
  isProjectSessionInitializing.value = true;
  realSessionSnapshotLoadController.cancel();
  try {
    await loadComposerDeveloperConfig();
    if (!isCurrentInitialization()) {
      return;
    }
    clearArchivedPreviewSession();
    activeArchivedPreviewId.value = '';
    tabOrderIds.value = loadPersistedTabOrderIds(projectId);
    tabMruIds.value = loadPersistedTabMruIds(projectId);
    const restoredDraftState = collapseProjectDraftTabs({
      drafts: loadPersistedDraftSessions(projectId),
      activeDraftId: persistedActiveDraftSessionIdByProject.value[projectId] ?? '',
      orderIds: tabOrderIds.value,
      mruIds: tabMruIds.value,
    });
    restoredDraftState.removedDraftIds.forEach(draftId => {
      webSessionStore.clearDraft(projectId, draftId);
    });
    tabOrderIds.value = restoredDraftState.orderIds;
    tabMruIds.value = restoredDraftState.mruIds;
    const restoredDrafts = restoredDraftState.drafts;
    const activeDraftId = restoredDraftState.activeDraftId;
    replaceDraftSessionState(restoredDrafts, activeDraftId, projectId);
    const loadedSessions = await webSessionStore.loadSessions(projectId);
    if (!isCurrentInitialization()) {
      return;
    }
    syncTabNavigationState(projectId, {
      orderIds: tabOrderIds.value,
      mruIds: tabMruIds.value,
    });
    await webSessionStore.openEventStream();
    if (!isCurrentInitialization()) {
      return;
    }
    const routeSessionId = routeWebSessionId.value;
    if (routeSessionId) {
      pendingRouteActivationSessionId.value = routeSessionId;
      const handled = await activateSessionFromRoute(projectId, routeSessionId, {
        loadedSessions,
      });
      if (!isCurrentInitialization()) {
        return;
      }
      if (handled) {
        return;
      }
    }
    if (activeDraftId) {
      await activateTabById(activeDraftId, { connectReal: false });
      return;
    }
    const rememberedSessionId = webSessionStore.getActiveSessionId(projectId);
    const targetSessionId =
      loadedSessions.find(session => session.id === rememberedSessionId)?.id ??
      selectMostRecentWebSession(loadedSessions)?.id;
    if (targetSessionId) {
      try {
        await activateTabById(targetSessionId);
      } catch (error) {
        if (isCurrentInitialization()) {
          console.warn('[Web Session] Failed to initialize current session', {
            projectId,
            sessionId: targetSessionId,
            error,
          });
        }
      }
      return;
    }
    if (restoredDrafts.length > 0) {
      const fallbackDraftId =
        tabMruIds.value.find(sessionId =>
          restoredDrafts.some(session => session.id === sessionId)
        ) ??
        restoredDrafts[restoredDrafts.length - 1]?.id ??
        '';
      if (fallbackDraftId) {
        await activateTabById(fallbackDraftId, { connectReal: false });
      }
      return;
    }
    ensureDefaultDraftSession();
  } finally {
    if (isCurrentInitialization()) {
      isProjectSessionInitializing.value = false;
    }
  }
}

async function handleSessionSelect(sessionId: string) {
  if (!sessionId) {
    return;
  }
  closeMobileSessionSelector();
  if (sessionId === activeSessionId.value) {
    pendingRouteActivationSessionId.value = '';
    const session = currentSession.value;
    void syncWebSessionRouteSessionId(
      session && !isDraftSession(session) && session.projectId === props.projectId ? session.id : ''
    ).catch(error => {
      console.error('[Web Session] Failed to sync route session id', error);
    });
    rememberTabVisit(sessionId);
    scrollToBottom(true);
    return;
  }
  try {
    await activateTabById(sessionId);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleSidebarSessionSelect(item: CrossProjectSessionItem) {
  const sessionId = item.session.id;
  if (!sessionId) {
    return;
  }
  try {
    if (item.projectId === props.projectId && sessionId === activeSessionId.value) {
      pendingRouteActivationSessionId.value = '';
      void syncWebSessionRouteSessionId(sessionId).catch(error => {
        console.error('[Web Session] Failed to sync route session id', error);
      });
      scrollToBottom(true);
      return;
    }
    if (item.projectId !== props.projectId) {
      webSessionStore.setActiveSession(item.projectId, sessionId);
      projectStore.addRecentProject(item.projectId);
      await router.push(buildProjectRouteLocation(item.projectId, sessionId));
      return;
    }
    await activateTabById(sessionId);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleArchivedSidebarSessionSelect(item: CrossProjectSessionItem) {
  if (!item.session.id) {
    return;
  }
  try {
    if (item.projectId !== props.projectId) {
      projectStore.addRecentProject(item.projectId);
      await router.push(buildProjectRouteLocation(item.projectId, item.session.id));
      return;
    }
    if (archivedPreviewSession.value?.id === item.session.id) {
      pendingRouteActivationSessionId.value = '';
      void syncWebSessionRouteSessionId(item.session.id).catch(error => {
        console.error('[Web Session] Failed to sync route session id', error);
      });
      activeArchivedPreviewId.value = item.session.id;
      scrollToBottom(true);
      return;
    }
    await openArchivedPreviewSession(item.session);
  } catch (error) {
    clearArchivedPreviewSession();
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleSidebarVirtualSessionSelect(
  item: WebSessionSidebarVirtualItem<CrossProjectSessionItem>
) {
  if (item.type !== 'session') {
    return;
  }
  if (item.entry.row.archived) {
    await handleArchivedSidebarSessionSelect(item.entry.source);
    return;
  }
  await handleSidebarSessionSelect(item.entry.source);
}

async function handleLoadMoreArchived() {
  try {
    await webSessionStore.loadArchivedSessions(sidebarVisibleProjectIds.value, {
      limit: 20,
    });
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleSidebarSessionActionSelect(
  key: string | number,
  item: CrossProjectSessionItem
) {
  const action = String(key);
  if (action === 'rename') {
    openSessionRenameDialog(item.session);
    return;
  }
  if (action === 'archive') {
    handleSidebarArchiveSession(item);
    return;
  }
  if (action === 'unarchive') {
    await handleSidebarUnarchiveSession(item);
    return;
  }
  if (action === 'delete') {
    confirmSidebarDeleteSession(item);
  }
}

function handleSidebarArchiveSession(item: CrossProjectSessionItem) {
  if (isSessionArchiving(item.session.id)) {
    return;
  }
  if (confirmBeforeTerminalClose.value) {
    let archiveConfirmDialog: DialogReactive | null = null;
    archiveConfirmDialog = dialog.warning({
      title: t('webSession.confirmCloseTitle'),
      content: () =>
        h('div', { class: 'web-session-close-confirm' }, [
          h('div', { class: 'web-session-close-confirm__message' }, [
            t('webSession.confirmCloseContent', { title: item.session.title }),
          ]),
        ]),
      positiveText: t('webSession.confirmCloseButton'),
      negativeText: t('common.cancel'),
      onPositiveClick: async () => {
        if (archiveConfirmDialog?.loading) {
          return false;
        }
        if (archiveConfirmDialog) {
          archiveConfirmDialog.loading = true;
        }
        try {
          return await performSidebarArchiveSession(item);
        } finally {
          if (archiveConfirmDialog) {
            archiveConfirmDialog.loading = false;
          }
        }
      },
    });
    return;
  }

  void performSidebarArchiveSession(item);
}

async function performSidebarArchiveSession(item: CrossProjectSessionItem): Promise<boolean> {
  if (isSessionArchiving(item.session.id)) {
    return false;
  }
  beginSessionArchive(item.session.id);
  try {
    const visibleSession = visibleSessionById.value.get(item.session.id);
    if (visibleSession && visibleSession.projectId === item.projectId) {
      await closeTabById(item.session.id, async () => {
        await webSessionStore.archiveSession(item.projectId, item.session.id);
      });
    } else {
      await webSessionStore.archiveSession(item.projectId, item.session.id);
    }
    await refreshArchivedSidebar();
    return true;
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
    return false;
  } finally {
    endSessionArchive(item.session.id);
  }
}

async function handleSidebarUnarchiveSession(item: CrossProjectSessionItem) {
  try {
    const restored = await webSessionStore.unarchiveSession(item.projectId, item.session.id);
    await refreshArchivedSidebar();
    if (archivedPreviewSession.value?.id === item.session.id) {
      clearArchivedPreviewSession();
      if (restored.projectId === props.projectId) {
        await activateTabById(restored.id);
      }
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

function confirmSidebarDeleteSession(item: CrossProjectSessionItem) {
  dialog.warning({
    title: t('common.delete'),
    content: t('webSession.deleteConfirm', { title: item.session.title }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => performSidebarDeleteSession(item),
  });
}

async function performSidebarDeleteSession(item: CrossProjectSessionItem): Promise<boolean> {
  const visibleSession = visibleSessionById.value.get(item.session.id);
  if (visibleSession && visibleSession.projectId === item.projectId) {
    return performDeleteSession(item.session.id);
  }
  try {
    if (archivedPreviewSession.value?.id === item.session.id) {
      clearArchivedPreviewSession();
    }
    await webSessionStore.deleteSession(item.projectId, item.session.id);
    await refreshArchivedSidebar();
    return true;
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
    return false;
  }
}

function handleSidebarScopeSelect(key: string | number) {
  sidebarScope.value = normalizeWebSessionSidebarScope(key);
}

function toggleSidebarScope() {
  sidebarScope.value = resolveWebSessionSidebarToggleScope(sidebarScope.value);
}

function openImportDialog() {
  closeMobileSessionSelector();
  contextMenuSession.value = null;
  showImportDialog.value = true;
}

function promptSyncExistingImportedSession(session: WebSessionSummary) {
  dialog.warning({
    title: t('webSession.importCodexSessionReuseTitle'),
    content: t('webSession.importCodexSessionReuseContent', {
      title: session.title,
    }),
    positiveText: t('webSession.syncFromTerminal'),
    negativeText: t('webSession.importCodexSessionReuseSkip'),
    onPositiveClick: async () => handleSyncSession(session.id, 'fast', false),
  });
}

async function handleOpenImportedCodexSession(session: WebSessionSummary) {
  try {
    showImportDialog.value = false;

    let target = session;
    if (session.archivedAt) {
      target = await webSessionStore.unarchiveSession(session.projectId, session.id);
      await refreshArchivedSidebar();
      if (archivedPreviewSession.value?.id === session.id) {
        clearArchivedPreviewSession();
        activeArchivedPreviewId.value = '';
      }
    }

    await activateTabById(target.id);
    scrollToBottom(true);
    if (target.agent === 'codex') {
      promptSyncExistingImportedSession(target);
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleImportCodexSession(source: { agent: 'codex' | 'pi'; sessionId: string }) {
  const { agent, sessionId } = source;
  if (!props.projectId || !sessionId || importingCodexSessionId.value) {
    return;
  }
  if (agent === 'pi' && !(await ensurePiProjectTrust())) {
    return;
  }

  importingCodexSessionId.value = sessionId;
  try {
    const result = await webSessionStore.importSession(props.projectId, sessionId, 'fast', agent);

    if (result.reused) {
      await refreshArchivedSidebar();
      if (archivedPreviewSession.value?.id === result.session.id) {
        clearArchivedPreviewSession();
        activeArchivedPreviewId.value = '';
      }
    }

    showImportDialog.value = false;
    await activateTabById(result.session.id, { connectReal: false });
    scrollToBottom(true);
    message.success(t('webSession.importCodexSessionSuccess'));

    if (result.reused && !result.synced && result.session.agent === 'codex') {
      promptSyncExistingImportedSession(result.session);
    }
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('webSession.importCodexSessionFailed')
    );
  } finally {
    importingCodexSessionId.value = '';
  }
}

async function handleCreateSession(
  forceAgent?: WebSessionAgent,
  options: { onCreated?: (session: WebSessionSummary) => void } = {}
) {
  try {
    const projectId = props.projectId;
    const source = currentSession.value;
    const sourceSessionId = source?.id ?? '';
    const agent = forceAgent ?? source?.agent ?? selectedAgent.value;
    if (!(await ensureMessageCapabilityAvailable(agent))) {
      return undefined;
    }
    if (agent === 'codex') {
      maybeNotifyCodexCompatibilityMode();
    }
    const worktreeId = resolveCreateSessionWorktreeId(source);
    const session = await webSessionStore.createSession(
      projectId,
      {
        worktreeId,
        agent,
        claudeRuntime:
          agent === 'claude'
            ? source?.claudeRuntime === 'ccr'
              ? 'ccr'
              : draftClaudeRuntime.value
            : 'claude',
        model: source?.model || draftModel.value || defaultModelForAgent(agent),
        reasoningEffort:
          source?.reasoningEffort ||
          (agent === 'codex'
            ? selectedReasoningEffort.value
            : defaultReasoningEffortForAgent(agent)),
        workflowMode: source?.workflowMode || draftWorkflowMode.value,
        permissionLevel:
          (source?.permissionLevel === 'default' && agent === 'claude'
            ? 'elevated'
            : source?.permissionLevel) || draftPermissionLevel.value,
        activeCallTimeoutEnabled: resolveInheritedActiveCallTimeoutEnabled(source, agent),
        autoRetryEnabled: source?.autoRetryEnabled === true,
        autoRetryPolicyMode: source?.autoRetryPolicyMode === 'custom' ? 'custom' : 'default',
        autoRetryScope:
          source?.autoRetryEnabled === true && source.autoRetryPolicyMode === 'custom'
            ? source.autoRetryScope
            : webSessionAutoContinueScope.value,
        autoRetryPreset:
          source?.autoRetryEnabled === true && source.autoRetryPolicyMode === 'custom'
            ? source.autoRetryPreset
            : webSessionAutoContinuePreset.value,
        autoRetryMaxAttempts:
          source?.autoRetryEnabled === true &&
          source.autoRetryPolicyMode === 'custom' &&
          typeof source.autoRetryMaxAttempts === 'number'
            ? source.autoRetryMaxAttempts
            : webSessionAutoContinueMaxAttempts.value,
        autoRetryDispatchPendingOnFailure:
          typeof source?.autoRetryDispatchPendingOnFailure === 'boolean'
            ? source.autoRetryDispatchPendingOnFailure
            : webSessionAutoRetryDispatchPendingOnFailure.value,
      },
      {
        rememberActive: false,
      }
    );
    const shouldActivateCreatedSession = sourceSessionId
      ? isCurrentVisibleSession(sourceSessionId)
      : !currentSession.value;
    if (isDraftSession(source)) {
      webSessionStore.moveDraft(projectId, source.id, session.id);
      replaceTabIdInNavigationState(source.id, session.id);
      removeDraftSessionRecord(source.id, {
        preserveDraftState: true,
      });
    }
    options.onCreated?.(session);
    if (shouldActivateCreatedSession) {
      draftAgent.value = session.agent;
      draftClaudeRuntime.value = session.claudeRuntime === 'ccr' ? 'ccr' : 'claude';
      draftModel.value = session.model;
      draftReasoningEffort.value =
        session.reasoningEffort || defaultReasoningEffortForAgent(session.agent);
      draftWorkflowMode.value = session.workflowMode;
      draftPermissionLevel.value = session.permissionLevel;
      await activateTabById(session.id, { connectReal: false });
      scrollToBottom(true);
    }
    return session;
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
    return null;
  }
}

async function handleStartDraftSession(forceAgent?: WebSessionAgent) {
  await loadComposerDeveloperConfig();
  const anchorId = underlyingTabSessionId.value;
  const decision = resolveStartDraftSessionDecision(draftSessions.value, {
    activeDraftId: activeDraftSessionId.value,
    mruIds: tabMruIds.value,
  });
  if (decision.kind === 'reuse') {
    insertTabAfter(decision.draft.id, anchorId);
    await activateTabById(decision.draft.id, { connectReal: false });
    closeMobileSessionSelector();
    contextMenuSession.value = null;
    expandedTools.value = {};
    autoFollowBottom.value = true;
    scrollToBottom(true);
    updateActiveTabIndicator();
    syncActiveTabIntoView();
    focusComposer();
    if (decision.shouldNotifyExistingDraft) {
      message.info(t('webSession.existingDraftSessionNotice'));
    }
    return;
  }
  const draft = createDraftSession(forceAgent);
  draftAgent.value = draft.agent;
  draftClaudeRuntime.value = draft.claudeRuntime === 'ccr' ? 'ccr' : 'claude';
  draftModel.value = draft.model || defaultModelForAgent(draft.agent);
  draftReasoningEffort.value = draft.reasoningEffort || defaultReasoningEffortForAgent(draft.agent);
  draftWorkflowMode.value = draft.workflowMode;
  draftPermissionLevel.value = draft.permissionLevel;
  closeMobileSessionSelector();
  contextMenuSession.value = null;
  expandedTools.value = {};
  autoFollowBottom.value = true;
  scrollToBottom(true);
  updateActiveTabIndicator();
  syncActiveTabIntoView();
  focusComposer();
}

async function handleRenameSession(sessionId: string) {
  const session = visibleSessionById.value.get(sessionId);
  if (!session || isDraftSession(session)) {
    return;
  }

  openSessionRenameDialog(session);
}

function openSessionRenameDialog(session: WebSessionSummary | SessionTab) {
  if (isDraftSession(session)) {
    return;
  }

  const inputValue = ref(session.title);
  dialog.create({
    title: t('webSession.renameTitle'),
    content: () =>
      h(NInput, {
        value: inputValue.value,
        'onUpdate:value': (value: string) => {
          inputValue.value = value;
        },
        maxlength: 64,
        autofocus: true,
        placeholder: t('webSession.renamePlaceholder'),
      }),
    positiveText: t('common.save'),
    negativeText: t('common.cancel'),
    showIcon: false,
    maskClosable: false,
    closeOnEsc: true,
    onPositiveClick: async () => {
      const nextTitle = inputValue.value.trim();
      if (!nextTitle) {
        message.warning(t('webSession.emptyName'));
        return false;
      }
      if (nextTitle === session.title) {
        return true;
      }
      try {
        await webSessionStore.renameSession(session.projectId, session.id, nextTitle);
        if (isArchivedPreviewSession(session) && archivedPreviewSession.value?.id === session.id) {
          archivedPreviewSession.value = {
            ...archivedPreviewSession.value,
            title: nextTitle,
          };
        }
        if (session.archivedAt) {
          await refreshArchivedSidebar();
        }
        message.success(t('webSession.renameSuccess'));
        return true;
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('webSession.renameFailed'));
        return false;
      }
    },
  });
}

async function refreshArchivedSidebar() {
  await webSessionStore.loadArchivedSessions(sidebarVisibleProjectIds.value, {
    reset: true,
    limit: 20,
  });
  if (normalizedSidebarSearchQuery.value) {
    await loadSidebarSearch();
  }
}

function handleArchiveSession(sessionId: string) {
  if (isSessionArchiving(sessionId)) {
    return;
  }
  const session = visibleSessionById.value.get(sessionId);
  if (!session) {
    return;
  }

  if (isDraftSession(session)) {
    void closeTabById(sessionId, () => {
      removeDraftSessionRecord(sessionId);
    });
    return;
  }
  if (isArchivedPreviewSession(session)) {
    void closeTabById(sessionId, () => {
      clearArchivedPreviewSession();
    });
    return;
  }

  if (confirmBeforeTerminalClose.value) {
    let archiveConfirmDialog: DialogReactive | null = null;
    archiveConfirmDialog = dialog.warning({
      title: t('webSession.confirmCloseTitle'),
      content: () =>
        h('div', { class: 'web-session-close-confirm' }, [
          h('div', { class: 'web-session-close-confirm__message' }, [
            t('webSession.confirmCloseContent', { title: session.title }),
          ]),
        ]),
      positiveText: t('webSession.confirmCloseButton'),
      negativeText: t('common.cancel'),
      onPositiveClick: async () => {
        if (archiveConfirmDialog?.loading) {
          return false;
        }
        if (archiveConfirmDialog) {
          archiveConfirmDialog.loading = true;
        }
        try {
          return await performArchiveSession(session);
        } finally {
          if (archiveConfirmDialog) {
            archiveConfirmDialog.loading = false;
          }
        }
      },
    });
    return;
  }

  void performArchiveSession(session);
}

async function performArchiveSession(session: WebSessionSummary): Promise<boolean> {
  if (isSessionArchiving(session.id)) {
    return false;
  }
  beginSessionArchive(session.id);
  try {
    await closeTabById(session.id, async () => {
      await webSessionStore.archiveSession(session.projectId, session.id);
    });
    await refreshArchivedSidebar();
    return true;
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
    return false;
  } finally {
    endSessionArchive(session.id);
  }
}

async function performDeleteSession(sessionId: string): Promise<boolean> {
  const session = visibleSessionById.value.get(sessionId);
  if (!session) {
    return false;
  }
  try {
    await closeTabById(sessionId, async () => {
      if (isArchivedPreviewSession(session)) {
        clearArchivedPreviewSession();
        return;
      }
      if (isDraftSession(session)) {
        removeDraftSessionRecord(sessionId);
        return;
      }
      await webSessionStore.deleteSession(session.projectId, sessionId);
    });
    if (!isDraftSession(session) && !isArchivedPreviewSession(session)) {
      await refreshArchivedSidebar();
    }
    forgetTimelinePosition(session.projectId, sessionId);
    return true;
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
    return false;
  }
}

function openFilePicker() {
  showQuickInputPopover.value = false;
  showSkillBrowser.value = false;
  fileInputRef.value?.click();
}

function hasFileTransfer(dataTransfer: DataTransfer | null) {
  if (!dataTransfer) {
    return false;
  }

  if (Array.from(dataTransfer.items || []).some(item => item.kind === 'file')) {
    return true;
  }

  return (
    Array.from(dataTransfer.files || []).length > 0 ||
    Array.from(dataTransfer.types || []).includes('Files')
  );
}

function resetComposerDragState() {
  composerDragDepth = 0;
  isComposerDragOver.value = false;
}

async function uploadComposerImages(files: File[]) {
  const sessionId = currentDraftSessionId.value;
  if (!sessionId) {
    return;
  }
  clearComposerTransferError();
  const result = await webSessionStore.uploadAttachments(props.projectId, sessionId, files);
  if (result.attachments.length > 0) {
    insertUploadedImagePlaceholders(result.attachments.length);
  }
  if (result.errors.length === 0) {
    return;
  }

  const detail = result.errors[0]?.message || '';
  showComposerTransferError(detail);
  result.errors.forEach(error => {
    const errorMessage = error.fileName ? `${error.fileName}: ${error.message}` : error.message;
    message.error(errorMessage || t('common.error'));
  });
}

async function handleFileChange(event: Event) {
  const target = event.target as HTMLInputElement | null;
  const files = Array.from(target?.files ?? []).filter(file => file.type.startsWith('image/'));
  if (files.length === 0) {
    return;
  }
  try {
    await uploadComposerImages(files);
  } finally {
    if (target) {
      target.value = '';
    }
  }
}

function confirmRemoteImageDownload(count: number) {
  return new Promise<boolean>(resolve => {
    let settled = false;
    const settle = (value: boolean) => {
      if (settled) {
        return;
      }
      settled = true;
      resolve(value);
    };

    dialog.warning({
      title: t('webSession.remoteImageDownloadTitle'),
      content: t('webSession.remoteImageDownloadPrompt', { count }),
      positiveText: t('common.yes'),
      negativeText: t('common.no'),
      closable: false,
      closeOnEsc: false,
      maskClosable: false,
      onPositiveClick: () => settle(true),
      onNegativeClick: () => settle(false),
    });
  });
}

async function prepareComposerPaste(options: {
  sessionId: string;
  html: string;
  plainText: string;
  eventImageFiles: File[];
  clipboardImageFiles: Promise<File[]>;
  baseText: string;
  selection: { start: number; end: number };
}) {
  const clipboardImageFiles = await options.clipboardImageFiles;
  const plan = buildWebSessionComposerPastePlan({
    failureMarker: t('webSession.pastedImageFailedMarker'),
    html: options.html,
    imageFiles: mergeClipboardImageFiles(options.eventImageFiles, clipboardImageFiles),
    plainText: options.plainText,
  });
  if (!plan) {
    insertComposerPasteText(
      options.sessionId,
      options.plainText,
      options.baseText,
      options.selection
    );
    return;
  }

  const downloadRemoteImages =
    plan.remoteImages.length > 0
      ? await confirmRemoteImageDownload(plan.remoteImages.length)
      : false;
  if (plan.images.length === 0 && plan.unavailableImages.length === 0 && !downloadRemoteImages) {
    insertComposerPasteText(
      options.sessionId,
      renderWebSessionComposerPastePlan(plan),
      options.baseText,
      options.selection
    );
    return;
  }

  enqueueComposerPaste(
    options.sessionId,
    plan,
    options.baseText,
    options.selection,
    downloadRemoteImages
  );
}

function handleComposerPaste(event: ClipboardEvent) {
  const sessionId = currentDraftSessionId.value;
  const clipboardData = event.clipboardData;
  if (!sessionId || !clipboardData) {
    return;
  }

  const html = clipboardData.getData('text/html');
  const plainText = clipboardData.getData('text/plain');
  const eventImageFiles = getImageFilesFromTransfer(clipboardData);
  const initialPlan = buildWebSessionComposerPastePlan({
    failureMarker: t('webSession.pastedImageFailedMarker'),
    html,
    imageFiles: eventImageFiles,
    plainText,
  });
  if (!initialPlan) {
    return;
  }

  const clipboardImageFiles =
    initialPlan.unavailableImageCount > 0
      ? readClipboardImageFiles()
      : Promise.resolve([] as File[]);
  event.preventDefault();
  const selection = getComposerSelectionRange();
  const baseText = composerText.value;
  void prepareComposerPaste({
    sessionId,
    html,
    plainText,
    eventImageFiles,
    clipboardImageFiles,
    baseText,
    selection,
  });
}

function handleComposerDragEnter(event: DragEvent) {
  if (!hasFileTransfer(event.dataTransfer)) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();
  composerDragDepth += 1;
  isComposerDragOver.value = true;
}

function handleComposerDragOver(event: DragEvent) {
  if (!hasFileTransfer(event.dataTransfer)) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'copy';
  }
  isComposerDragOver.value = true;
}

function handleComposerDragLeave(event: DragEvent) {
  if (!isComposerDragOver.value) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();
  composerDragDepth = Math.max(0, composerDragDepth - 1);
  if (composerDragDepth === 0) {
    isComposerDragOver.value = false;
  }
}

async function handleComposerDrop(event: DragEvent) {
  if (!hasFileTransfer(event.dataTransfer)) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();
  const files = getImageFilesFromTransfer(event.dataTransfer);
  resetComposerDragState();
  if (files.length === 0) {
    return;
  }

  await uploadComposerImages(files);
}

function removeAttachment(attachmentId: string) {
  const sessionId = currentDraftSessionId.value;
  if (!sessionId) {
    return;
  }
  webSessionStore.removeDraftAttachment(props.projectId, sessionId, attachmentId);
}

function focusComposer() {
  ensureMobileComposerVisible();
  nextTick(() => {
    composerInputRef.value?.focus();
  });
}

function getComposerSelectionRange() {
  return (
    composerInputRef.value?.getSelectionRange() ?? {
      start: composerText.value.length,
      end: composerText.value.length,
    }
  );
}

function setComposerTextAndSelection(text: string, cursor: number) {
  const sessionId = currentDraftSessionId.value;
  if (!sessionId) {
    return false;
  }

  webSessionStore.setDraftText(props.projectId, sessionId, text);
  ensureMobileComposerVisible();
  nextTick(() => {
    composerInputRef.value?.focus();
    composerInputRef.value?.setSelectionRange(cursor, cursor);
  });
  return true;
}

function updateComposerPastePending(sessionId: string, delta: number) {
  const next = { ...composerPastePendingBySession.value };
  const count = Math.max(0, (next[sessionId] ?? 0) + delta);
  if (count > 0) {
    next[sessionId] = count;
  } else {
    delete next[sessionId];
  }
  composerPastePendingBySession.value = next;
}

function insertComposerPasteText(
  sessionId: string,
  pastedText: string,
  baseText: string,
  fallbackSelection: { start: number; end: number }
) {
  if (!pastedText) {
    return;
  }

  const draft = webSessionStore.getDraft(props.projectId, sessionId);
  const isCurrentDraft = currentDraftSessionId.value === sessionId;
  const selection = isCurrentDraft
    ? getComposerSelectionRange()
    : draft.text === baseText
      ? fallbackSelection
      : { start: draft.text.length, end: draft.text.length };
  const next = replaceTextSelection(draft.text, selection.start, selection.end, pastedText);
  webSessionStore.setDraftText(props.projectId, sessionId, next.text);

  if (isCurrentDraft) {
    ensureMobileComposerVisible();
    nextTick(() => {
      composerInputRef.value?.focus();
      composerInputRef.value?.setSelectionRange(next.cursor, next.cursor);
    });
  }
}

async function processComposerPaste(
  sessionId: string,
  plan: WebSessionComposerPastePlan,
  baseText: string,
  fallbackSelection: { start: number; end: number },
  downloadRemoteImages: boolean
) {
  const replacements: string[] = [];
  const remoteReplacements: string[] = [];
  const unavailableReplacements: string[] = [];
  if (currentDraftSessionId.value === sessionId) {
    clearComposerTransferError();
  }

  for (const segment of plan.segments) {
    if (segment.type === 'image' && replacements[segment.imageIndex] == null) {
      const file = plan.images[segment.imageIndex];
      try {
        const attachment = await webSessionStore.uploadAttachment(props.projectId, sessionId, file);
        const attachmentIndex = webSessionStore
          .getDraftAttachments(props.projectId, sessionId)
          .findIndex(item => item.id === attachment.id);
        replacements[segment.imageIndex] =
          attachmentIndex >= 0 ? buildImagePlaceholder(attachmentIndex + 1) : plan.failureMarker;
      } catch (error) {
        const detail = error instanceof Error ? error.message : t('common.error');
        replacements[segment.imageIndex] = plan.failureMarker;
        if (currentDraftSessionId.value === sessionId) {
          showComposerTransferError(detail);
        }
        message.error(file.name ? `${file.name}: ${detail}` : detail);
      }
      continue;
    }

    if (
      segment.type === 'unavailable-image' &&
      unavailableReplacements[segment.unavailableImageIndex] == null
    ) {
      const source = plan.unavailableImages[segment.unavailableImageIndex];
      try {
        const attachment = await webSessionStore.importClipboardAttachment(
          props.projectId,
          sessionId,
          source
        );
        const attachmentIndex = webSessionStore
          .getDraftAttachments(props.projectId, sessionId)
          .findIndex(item => item.id === attachment.id);
        unavailableReplacements[segment.unavailableImageIndex] =
          attachmentIndex >= 0 ? buildImagePlaceholder(attachmentIndex + 1) : plan.failureMarker;
      } catch (error) {
        const detail = error instanceof Error ? error.message : t('common.error');
        unavailableReplacements[segment.unavailableImageIndex] = plan.failureMarker;
        if (currentDraftSessionId.value === sessionId) {
          showComposerTransferError(detail);
        }
        message.error(detail);
      }
      continue;
    }

    if (segment.type !== 'remote-image' || remoteReplacements[segment.remoteImageIndex] != null) {
      continue;
    }
    const url = plan.remoteImages[segment.remoteImageIndex];
    if (!downloadRemoteImages) {
      remoteReplacements[segment.remoteImageIndex] = url;
      continue;
    }
    try {
      const attachment = await webSessionStore.importRemoteAttachment(
        props.projectId,
        sessionId,
        url
      );
      const attachmentIndex = webSessionStore
        .getDraftAttachments(props.projectId, sessionId)
        .findIndex(item => item.id === attachment.id);
      remoteReplacements[segment.remoteImageIndex] =
        attachmentIndex >= 0 ? buildImagePlaceholder(attachmentIndex + 1) : url;
    } catch {
      remoteReplacements[segment.remoteImageIndex] = url;
      message.warning(t('webSession.remoteImageDownloadFailed'));
    }
  }

  insertComposerPasteText(
    sessionId,
    renderWebSessionComposerPastePlan(
      plan,
      replacements,
      remoteReplacements,
      unavailableReplacements
    ),
    baseText,
    fallbackSelection
  );
}

function enqueueComposerPaste(
  sessionId: string,
  plan: WebSessionComposerPastePlan,
  baseText: string,
  fallbackSelection: { start: number; end: number },
  downloadRemoteImages: boolean
) {
  const queueKey = `${props.projectId}:${sessionId}`;
  const previousTask = composerPasteQueues.get(queueKey) ?? Promise.resolve();
  updateComposerPastePending(sessionId, 1);

  const task = previousTask
    .catch(() => undefined)
    .then(() =>
      processComposerPaste(sessionId, plan, baseText, fallbackSelection, downloadRemoteImages)
    )
    .catch(error => {
      const detail = error instanceof Error ? error.message : t('common.error');
      message.error(detail);
    })
    .finally(() => {
      updateComposerPastePending(sessionId, -1);
      if (composerPasteQueues.get(queueKey) === task) {
        composerPasteQueues.delete(queueKey);
      }
    });

  composerPasteQueues.set(queueKey, task);
}

async function applyQuickInputText(text: string) {
  const sessionId = currentDraftSessionId.value;
  if (!sessionId) {
    return false;
  }

  return setComposerTextAndSelection(text, text.length);
}

function insertUploadedImagePlaceholders(uploadedCount: number) {
  const sessionId = currentDraftSessionId.value;
  if (!sessionId || uploadedCount <= 0) {
    return;
  }

  const attachmentCount = webSessionStore.getDraftAttachments(props.projectId, sessionId).length;
  const firstIndex = attachmentCount - uploadedCount + 1;
  if (firstIndex <= 0) {
    return;
  }

  const placeholders = Array.from({ length: uploadedCount }, (_, index) =>
    buildImagePlaceholder(firstIndex + index)
  );
  const selection = getComposerSelectionRange();
  const nextComposer = insertImagePlaceholdersAtCursor(
    composerText.value,
    selection.start,
    selection.end,
    placeholders
  );

  setComposerTextAndSelection(nextComposer.text, nextComposer.cursor);
}

function handleSkillTokenInsert(skill: CodexSkillSummary) {
  const sessionId = currentDraftSessionId.value;
  if (!sessionId) {
    return;
  }

  const selection = getComposerSelectionRange();
  const nextComposer = insertCodexSkillTokenAtCursor(
    composerText.value,
    selection.start,
    selection.end,
    skill.name
  );
  setComposerTextAndSelection(nextComposer.text, nextComposer.cursor);
  showSkillBrowser.value = false;
}

function handleSkillTemplateInsert(skill: CodexSkillSummary) {
  const sessionId = currentDraftSessionId.value;
  if (!sessionId || !skill.defaultPrompt) {
    return;
  }

  const selection = getComposerSelectionRange();
  const nextComposer = replaceTextSelection(
    composerText.value,
    selection.start,
    selection.end,
    skill.defaultPrompt
  );
  setComposerTextAndSelection(nextComposer.text, nextComposer.cursor);
  showSkillBrowser.value = false;
}

async function prepareSessionForSend(
  session: WebSessionSummary,
  options: { shouldActivate?: () => boolean } = {}
) {
  if (!session.archivedAt) {
    return {
      session,
      navigateProjectId: '',
    };
  }

  const restored = await webSessionStore.unarchiveSession(session.projectId, session.id);
  await refreshArchivedSidebar();
  if (archivedPreviewSession.value?.id === session.id) {
    clearArchivedPreviewSession();
  }
  const shouldActivate = options.shouldActivate?.() ?? true;
  if (shouldActivate) {
    if (restored.projectId === props.projectId) {
      await activateTabById(restored.id);
    } else {
      webSessionStore.setActiveSession(restored.projectId, restored.id);
    }
  }

  return {
    session: restored,
    navigateProjectId: restored.projectId !== props.projectId ? restored.projectId : '',
  };
}

async function handleRetryTimelineUserMessage(item: WebSessionBlock) {
  const initialSession = currentRealSession.value;
  if (!initialSession || !isFailedTimelineUserMessage(item) || retryingUserMessageKey.value) {
    return;
  }
  if (
    isRunActive.value ||
    isWebSessionSubmitting(submitStateBySessionId.value, initialSession.id)
  ) {
    message.info(t('webSession.userMessageRetryBusy'));
    return;
  }

  const attachmentIds = item.attachments.map(attachment => attachment.id).filter(Boolean);
  if (!item.text.trim() && attachmentIds.length === 0) {
    return;
  }

  const submitKind: WebSessionSubmitKind =
    initialSession.workflowMode === 'plan' ? 'plan_message' : 'execute_send';
  if (
    submitKind === 'execute_send' &&
    !ensureSendConflictConfirmed(
      buildWebSessionSendConfirmationSignature({
        ownerId: initialSession.id,
        text: item.text,
        attachmentIds,
        conflictSessionIds: sendConflictSessions.value.map(session => session.id),
      }),
      { notifyOnBlock: true }
    )
  ) {
    return;
  }

  const sourceSessionId = initialSession.id;
  retryingUserMessageKey.value = item.key;
  beginSessionSubmit(sourceSessionId, submitKind);
  try {
    const prepared = await prepareSessionForSend(initialSession, {
      shouldActivate: () => isCurrentVisibleSession(sourceSessionId),
    });
    if (!(await ensureMessageCapabilityAvailable(prepared.session.agent))) {
      return;
    }
    await webSessionStore.sendMessage(prepared.session.id, item.text, attachmentIds, undefined, {
      outgoingMessageId: item.id,
      attachments: item.attachments,
    });
    recordSubmittedPrompt(item.text, prepared.session.projectId || props.projectId);
    if (prepared.navigateProjectId && isCurrentVisibleSession(prepared.session.id)) {
      projectStore.addRecentProject(prepared.navigateProjectId);
      await router.push(buildProjectRouteLocation(prepared.navigateProjectId, prepared.session.id));
    }
    if (isCurrentVisibleSession(prepared.session.id)) {
      autoFollowBottom.value = true;
      scrollToBottom(true);
    }
    message.success(t('webSession.userMessageRetrySuccess'));
  } catch (error) {
    message.error(
      t('webSession.userMessageRetryFailed', {
        reason: formatSessionInteractionError(error),
      })
    );
  } finally {
    endSessionSubmit(sourceSessionId);
    if (retryingUserMessageKey.value === item.key) {
      retryingUserMessageKey.value = '';
    }
  }
}

async function continueErroredSession(session: WebSessionSummary) {
  const sourceSessionId = session.id;
  const prepared = await prepareSessionForSend(session, {
    shouldActivate: () => isCurrentVisibleSession(sourceSessionId),
  });
  if (!(await ensureMessageCapabilityAvailable(prepared.session.agent))) {
    return;
  }
  await webSessionStore.sendMessage(prepared.session.id, 'continue', []);
  if (prepared.navigateProjectId && isCurrentVisibleSession(prepared.session.id)) {
    projectStore.addRecentProject(prepared.navigateProjectId);
    await router.push(buildProjectRouteLocation(prepared.navigateProjectId, prepared.session.id));
  }
  if (isCurrentVisibleSession(prepared.session.id)) {
    autoFollowBottom.value = true;
    scrollToBottom(true);
  }
}

async function handleSubmit() {
  const submitProjectId = props.projectId;
  const initialSubmitOwnerId = currentDraftSessionId.value;
  const initialSession = currentSession.value;
  const initialRealSession = currentRealSession.value;
  const draft = webSessionStore.getDraft(submitProjectId, initialSubmitOwnerId);
  const draftText = draft.text;
  const attachments = [...draft.attachments];
  if (
    !initialSubmitOwnerId ||
    isWebSessionSubmitting(submitStateBySessionId.value, initialSubmitOwnerId) ||
    isRunActive.value ||
    isDraftAttachmentUploading.value ||
    (draftText.trim().length === 0 && attachments.length === 0)
  ) {
    return;
  }
  const submitAgent = initialRealSession?.agent ?? selectedAgent.value;
  const submitKind = resolveComposerSubmitKind();
  const goalCommand = parseComposerGoalCommand(draftText);
  const isPiCompactCommand = submitAgent === 'pi' && draftText.trim() === '/compact';
  const shouldStageOutgoingMessage = !goalCommand && !isPiCompactCommand;
  if (submitKind === 'execute_send') {
    if (!ensureSendConflictConfirmed(sendConfirmationSignature.value)) {
      return;
    }
  } else {
    clearSendConflictConfirmation();
  }
  let submitOwnerId = initialSubmitOwnerId;
  let draftSessionId = initialSubmitOwnerId;
  let submissionSucceeded = false;
  let messageRetainedForRetry = false;
  let outgoingMessageId = '';
  let outgoingMessageSessionId = '';
  const stageComposerMessage = (session: WebSessionSummary | null) => {
    if (!shouldStageOutgoingMessage || !session || outgoingMessageId) {
      return;
    }
    outgoingMessageSessionId = session.id;
    outgoingMessageId = webSessionStore.stageOutgoingMessage(
      session.id,
      draftText,
      attachments.map(item => item.id),
      { attachments }
    );
  };
  beginSessionSubmit(submitOwnerId, submitKind);
  clearComposerDraftAfterSubmit(draftSessionId, submitProjectId);
  stageComposerMessage(initialRealSession);
  try {
    if (submitAgent === 'pi' && !(await ensurePiProjectTrust())) {
      return;
    }
    if (isPiCompactCommand) {
      if (!initialRealSession || isDraftSession(initialSession)) {
        throw new Error(t('webSession.piCompactRequiresSession'));
      }
      if (attachments.length > 0) {
        throw new Error(t('webSession.compactAttachmentsUnsupported'));
      }
      if (!runtimeCapabilityFor('pi').supportsCompaction) {
        throw new Error(t('webSession.piCompactUnavailable'));
      }
      await webSessionStore.compactSession(initialRealSession.id);
      submissionSucceeded = true;
      settingsStore.recordWebSessionRecentInput(draftText);
      void settingsStore.syncWebSessionQuickInputToServer();
      return;
    }
    let session = initialRealSession;
    if (!session || isDraftSession(initialSession)) {
      const created = await handleCreateSession(undefined, {
        onCreated: createdSession => {
          if (isDraftSession(initialSession)) {
            draftSessionId = createdSession.id;
          }
          if (createdSession.id !== submitOwnerId) {
            transferSessionSubmit(submitOwnerId, createdSession.id);
            submitOwnerId = createdSession.id;
          }
        },
      });
      if (!created) {
        return;
      }
      session = created;
    }
    if (!session) {
      return;
    }
    stageComposerMessage(session);
    const sessionBeforePreparation = session;
    const prepared = await prepareSessionForSend(session, {
      shouldActivate: () => isCurrentVisibleSession(sessionBeforePreparation.id),
    });
    session = prepared.session;
    if (session.id !== submitOwnerId) {
      transferSessionSubmit(submitOwnerId, session.id);
      submitOwnerId = session.id;
    }
    if (goalCommand) {
      if (!goalCommand.objective) {
        await handleGoalCompose();
        return;
      }
      if (attachments.length > 0) {
        throw new Error('/goal does not accept attachments');
      }
      if (!(await ensureGoalModeAvailable())) {
        return;
      }
      if (session.agent !== 'codex') {
        throw new Error('Goal is only available for Codex');
      }
      if (session.nativeSessionId) {
        await webSessionStore.setGoal(session.id, goalCommand.objective, 'active');
      } else {
        await webSessionStore.bootstrapGoal(session.id, goalCommand.objective, 'active');
      }
      submissionSucceeded = true;
      recordSubmittedPrompt(draftText, session.projectId || submitProjectId);
      message.success('Goal updated');
      return;
    }
    if (!(await ensureMessageCapabilityAvailable(session.agent))) {
      return;
    }
    await webSessionStore.sendMessage(
      session.id,
      draftText,
      attachments.map(item => item.id),
      undefined,
      { outgoingMessageId, attachments }
    );
    submissionSucceeded = true;
    recordSubmittedPrompt(draftText, session.projectId || submitProjectId);
    const isCurrentSubmissionSession = isCurrentVisibleSession(session.id);
    if (prepared.navigateProjectId && isCurrentSubmissionSession) {
      projectStore.addRecentProject(prepared.navigateProjectId);
      await router.push(buildProjectRouteLocation(prepared.navigateProjectId, session.id));
    }
    if (isCurrentSubmissionSession) {
      autoFollowBottom.value = true;
      isMobileComposerSettingsExpanded.value = false;
      scrollToBottom(true);
    }
  } catch (error) {
    messageRetainedForRetry = isWebSessionMessageDeliveryError(error);
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    if (!submissionSucceeded && !messageRetainedForRetry) {
      if (outgoingMessageId && outgoingMessageSessionId) {
        webSessionStore.discardOutgoingMessage(outgoingMessageSessionId, outgoingMessageId);
      }
      restoreComposerDraftAfterFailedSubmit(draftSessionId, draft, submitProjectId);
    }
    endSessionSubmit(submitOwnerId);
  }
}

async function handleConfirmScheduledSend() {
  if (isScheduledDialogEdit.value) {
    await handleConfirmScheduledInputUpdate();
    return;
  }
  if (scheduledSendPurpose.value === 'execute_plan') {
    await handleConfirmScheduledPlanExecution();
    return;
  }
  const submitProjectId = props.projectId;
  const initialSubmitOwnerId = currentDraftSessionId.value;
  const initialSession = currentSession.value;
  const draft = webSessionStore.getDraft(submitProjectId, initialSubmitOwnerId);
  const draftText = draft.text;
  const attachments = [...draft.attachments];
  const executeAt = Number(scheduledSendAt.value);
  const requiresScheduledTime = scheduledScheduleKind.value === 'at_time';
  const scheduleKind = scheduledScheduleKind.value;
  const sendMode = scheduledSendMode.value;
  const submitAgent =
    initialSession && !isDraftSession(initialSession) ? initialSession.agent : selectedAgent.value;
  if (
    !initialSubmitOwnerId ||
    scheduledSendSubmitting.value ||
    isDraftAttachmentUploading.value ||
    (draftText.trim().length === 0 && attachments.length === 0) ||
    (requiresScheduledTime && (!Number.isFinite(executeAt) || executeAt <= Date.now()))
  ) {
    return;
  }
  if (submitAgent === 'pi' && !(await ensurePiProjectTrust())) {
    return;
  }
  scheduledSendSubmitting.value = true;
  try {
    let session = initialSession && !isDraftSession(initialSession) ? initialSession : null;
    let draftSessionId = initialSubmitOwnerId;
    if (!session || isDraftSession(initialSession)) {
      const created = await handleCreateSession();
      if (!created) {
        return;
      }
      session = created;
      if (isDraftSession(initialSession)) {
        draftSessionId = session.id;
      }
    }
    if (!session) {
      return;
    }
    const sessionBeforePreparation = session;
    const prepared = await prepareSessionForSend(session, {
      shouldActivate: () => isCurrentVisibleSession(sessionBeforePreparation.id),
    });
    session = prepared.session;
    if (!(await ensureMessageCapabilityAvailable(session.agent))) {
      return;
    }
    await webSessionStore.scheduleMessage(
      session.id,
      draftText,
      attachments.map(item => item.id),
      scheduleKind === 'when_idle'
        ? { scheduleKind: 'when_idle' }
        : { scheduleKind: 'at_time', scheduledFor: executeAt },
      sendMode,
      { exitPlanMode: scheduledExitPlanMode.value }
    );
    recordSubmittedPrompt(draftText, session.projectId || submitProjectId);
    clearComposerDraftAfterSubmit(draftSessionId, submitProjectId);
    const isCurrentSubmissionSession = isCurrentVisibleSession(session.id);
    if (prepared.navigateProjectId && isCurrentSubmissionSession) {
      projectStore.addRecentProject(prepared.navigateProjectId);
      await router.push(buildProjectRouteLocation(prepared.navigateProjectId, session.id));
    }
    if (isCurrentSubmissionSession) {
      isMobileComposerSettingsExpanded.value = false;
    }
    handleScheduledSendDialogVisibilityChange(false);
    message.success(t('webSession.scheduleSendCreated'));
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    scheduledSendSubmitting.value = false;
  }
}

async function handleConfirmScheduledPlanExecution() {
  const target = scheduledPlanDialogTarget.value;
  const executeAt = Number(scheduledSendAt.value);
  const requiresScheduledTime = scheduledScheduleKind.value === 'at_time';
  if (
    !target ||
    target.sessionId !== currentRealSession.value?.id ||
    target.planItemId !== latestPlanItemId.value ||
    activeScheduledPlanTargetIds.value.has(target.planItemId) ||
    scheduledSendSubmitting.value ||
    (requiresScheduledTime && (!Number.isFinite(executeAt) || executeAt <= Date.now()))
  ) {
    return;
  }

  scheduledSendSubmitting.value = true;
  try {
    const current = currentRealSession.value;
    if (!current || !(await ensureMessageCapabilityAvailable(current.agent))) {
      return;
    }
    const prepared = await prepareSessionForSend(current, {
      shouldActivate: () => isCurrentVisibleSession(current.id),
    });
    if (prepared.session.id !== target.sessionId) {
      return;
    }
    await webSessionStore.schedulePlanExecution(
      prepared.session.id,
      scheduledScheduleKind.value === 'when_idle'
        ? { scheduleKind: 'when_idle' }
        : { scheduleKind: 'at_time', scheduledFor: executeAt },
      {
        planItemId: target.planItemId,
        pendingItemId: target.pendingItemId,
        questionId: target.questionId,
        executeOptionLabel: target.executeOptionLabel,
      }
    );
    if (prepared.navigateProjectId && isCurrentVisibleSession(prepared.session.id)) {
      projectStore.addRecentProject(prepared.navigateProjectId);
      await router.push(buildProjectRouteLocation(prepared.navigateProjectId, prepared.session.id));
    }
    handleScheduledSendDialogVisibilityChange(false);
    message.success(t('webSession.planScheduleCreated'));
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    scheduledSendSubmitting.value = false;
  }
}

async function handleConfirmScheduledInputUpdate() {
  const editing = scheduledEditingInput.value;
  const executeAt = Number(scheduledSendAt.value);
  const currentSession = currentRealSession.value;
  const current = editing ? scheduledInputs.value.find(item => item.id === editing.id) : undefined;
  const scheduleKind = scheduledScheduleKind.value;
  const requiresScheduledTime = scheduleKind === 'at_time';
  if (
    !currentSession ||
    !current ||
    (current.status !== 'scheduled' && current.status !== 'failed') ||
    scheduledSendSubmitting.value ||
    (requiresScheduledTime && (!Number.isFinite(executeAt) || executeAt <= Date.now()))
  ) {
    return;
  }
  if (
    current.action === 'message' &&
    scheduledEditText.value.trim().length === 0 &&
    current.attachmentIds.length === 0
  ) {
    return;
  }

  scheduledSendSubmitting.value = true;
  try {
    if (!(await ensureMessageCapabilityAvailable(currentSession.agent))) {
      return;
    }
    await webSessionStore.updateScheduledInput(currentSession.id, current.id, {
      scheduleKind,
      scheduledFor: requiresScheduledTime ? executeAt : null,
      ...(current.action === 'message'
        ? {
            text: scheduledEditText.value,
            mode: scheduledSendMode.value,
            exitPlanMode: scheduledExitPlanMode.value,
          }
        : {}),
    });
    handleScheduledSendDialogVisibilityChange(false);
    message.success(t('webSession.scheduledUpdated'));
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    scheduledSendSubmitting.value = false;
  }
}

async function handlePreinput(mode: 'redirect' | 'queue') {
  const submitProjectId = props.projectId;
  const session = currentRealSession.value;
  const draftSessionId = currentDraftSessionId.value;
  const draft = webSessionStore.getDraft(submitProjectId, draftSessionId);
  const draftText = draft.text;
  const attachments = [...draft.attachments];
  if (
    !session ||
    !draftSessionId ||
    isWebSessionSubmitting(submitStateBySessionId.value, draftSessionId) ||
    isDraftAttachmentUploading.value ||
    (draftText.trim().length === 0 && attachments.length === 0)
  ) {
    return;
  }
  let submissionSucceeded = false;
  beginSessionSubmit(draftSessionId, mode === 'redirect' ? 'redirect_message' : 'queue_message');
  clearComposerDraftAfterSubmit(draftSessionId, submitProjectId);
  try {
    if (!(await ensureMessageCapabilityAvailable(session.agent, { refresh: false }))) {
      return;
    }
    await webSessionStore.sendMessage(
      session.id,
      draftText,
      attachments.map(item => item.id),
      mode,
      { attachments }
    );
    submissionSucceeded = true;
    recordSubmittedPrompt(draftText, session.projectId || submitProjectId);
    if (isCurrentVisibleSession(session.id)) {
      isMobileComposerSettingsExpanded.value = false;
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    if (!submissionSucceeded) {
      restoreComposerDraftAfterFailedSubmit(draftSessionId, draft, submitProjectId);
    }
    endSessionSubmit(draftSessionId);
  }
}

async function triggerPrimaryComposerAction() {
  if (isDraftAttachmentUploading.value) {
    return;
  }
  if (isRunActive.value) {
    if (canStageDuringRun.value) {
      await handlePreinput('redirect');
    }
    return;
  }
  if (canSend.value) {
    await handleSubmit();
  }
}

function handleComposerFocus() {
  isComposerFocused.value = true;
  if (!isMobile.value) {
    return;
  }
  ensureMobileComposerVisible();
  mobileKeyboard.setFocused(true);
  setMobileComposerFocusState(true);
}

function handleComposerBlur() {
  isComposerFocused.value = false;
  if (!isMobile.value) {
    return;
  }
  mobileKeyboard.setFocused(false);
  setMobileComposerFocusState(false);
}

function handleComposerSubmitShortcut() {
  if (isDraftAttachmentUploading.value || !hasDraftContent.value) {
    return;
  }
  void triggerPrimaryComposerAction();
}

function getSendConflictSessionTitle(title: string) {
  const normalized = String(title || '').trim();
  return normalized || t('terminal.untitledSession');
}

function formatSendConflictSessionList(
  sessions: Array<{
    title: string;
  }>
) {
  const titles = sessions.slice(0, 2).map(session => getSendConflictSessionTitle(session.title));
  if (sessions.length <= 2) {
    return titles.join(locale.value === 'zh-CN' ? '、' : ', ');
  }
  return t('webSession.sendConflictListOverflow', {
    first: titles[0],
    second: titles[1],
    remaining: sessions.length - 2,
  });
}

function buildSendConflictWarningBody(
  sessions: Array<{
    title: string;
  }>
) {
  if (sessions.length === 0) {
    return '';
  }
  const formatted = formatSendConflictSessionList(sessions);
  if (sessions.length === 1) {
    return t('webSession.sendConflictWarningBodySingle', { sessions: formatted });
  }
  return t('webSession.sendConflictWarningBodyMultiple', {
    count: sessions.length,
    sessions: formatted,
  });
}

function ensureSendConflictConfirmed(
  signature: string,
  options?: {
    notifyOnBlock?: boolean;
  }
) {
  const confirmation = resolveWebSessionSendConfirmation({
    conflicts: sendConflictSessions.value,
    currentState: sendConfirmationState.value,
    signature,
    now: Date.now(),
    ttlMs: WEB_SESSION_SEND_CONFIRM_TTL_MS,
  });
  setSendConflictConfirmationState(confirmation.nextState);
  if (!confirmation.shouldProceed && options?.notifyOnBlock) {
    const warningBody = buildSendConflictWarningBody(sendConflictSessions.value);
    if (warningBody) {
      message.warning(warningBody);
    }
  }
  return confirmation.shouldProceed;
}

const showSendConflictWarning = computed(
  () => isSendConflictConfirmationArmed.value && sendConflictSessions.value.length > 0
);
const sendConflictWarningBody = computed(() =>
  showSendConflictWarning.value ? buildSendConflictWarningBody(sendConflictSessions.value) : ''
);

function handleUserInputEnter(event: KeyboardEvent) {
  if (event.key !== 'Enter') {
    return;
  }
  if (event.shiftKey || event.ctrlKey || event.altKey || event.metaKey) {
    return;
  }
  if (event.isComposing || event.keyCode === 229) {
    return;
  }
  event.preventDefault();
  event.stopPropagation();
  if (isSubmittingUserInput.value) {
    return;
  }
  void handleUserInputSubmit();
}

function handleUserInputSingleSelect(questionId: string, value: string | null) {
  const normalizedQuestionId = String(questionId || '').trim();
  if (!normalizedQuestionId) {
    return;
  }
  const normalizedValue = String(value || '').trim();
  userInputSelections.value = {
    ...userInputSelections.value,
    [normalizedQuestionId]: normalizedValue ? [normalizedValue] : [],
  };
}

function pendingInputPreview(item: WebSessionPendingInput) {
  const text = item.text.trim();
  if (text) {
    return text.length > 72 ? `${text.slice(0, 72)}...` : text;
  }
  return t('webSession.pendingAttachments', { count: item.attachmentIds.length });
}

function pendingInputTimingLabel(item: WebSessionPendingInput) {
  const remainingSeconds =
    item.readyAt == null
      ? 0
      : Math.max(0, Math.ceil((item.readyAt - liveStateClockMs.value) / 1000));
  if (item.status === 'failed') {
    return t('webSession.pendingSteerFailed');
  }
  if (item.status === 'persisting') {
    return remainingSeconds > 0
      ? t('webSession.pendingSteerPersistRetryCountdown', { seconds: remainingSeconds })
      : t('webSession.pendingSteerPersisting');
  }
  if (item.status === 'retrying') {
    return remainingSeconds > 0
      ? t('webSession.pendingSteerRetryCountdown', { seconds: remainingSeconds })
      : t('webSession.pendingSteerRetrying');
  }
  if (item.paused) {
    return t('webSession.pendingPaused');
  }
  if (item.mode !== 'redirect' || item.readyAt == null) {
    return '';
  }
  return remainingSeconds > 0
    ? t('webSession.pendingSteerCountdown', { seconds: remainingSeconds })
    : t('webSession.pendingSteering');
}

function isEditingPendingInput(pendingId: string) {
  return pendingEditingId.value === pendingId;
}

function handlePendingInputPopoverUpdate(pendingId: string, show: boolean) {
  if (show) {
    pendingInputPopoverId.value = pendingId;
  } else if (pendingInputPopoverId.value === pendingId) {
    pendingInputPopoverId.value = '';
  }
}

function persistPendingEditDraft() {
  const session = currentRealSession.value;
  const pendingId = pendingEditingId.value;
  if (!session || !pendingId) {
    return;
  }
  webSessionStore.setPendingInputEditDraft(
    session.projectId,
    session.id,
    pendingId,
    pendingEditText.value
  );
}

function clearPendingEditState(options: { clearDraft?: boolean } = {}) {
  const session = currentRealSession.value;
  const pendingId = pendingEditingId.value;
  if (options.clearDraft !== false && session && pendingId) {
    webSessionStore.clearPendingInputEditDraft(session.projectId, session.id, pendingId);
  }
  pendingEditingId.value = '';
  pendingEditText.value = '';
  pendingEditActionId.value = '';
  if (pendingInputPopoverId.value === pendingId) {
    pendingInputPopoverId.value = '';
  }
}

function restorePendingEditForCurrentSession() {
  const session = currentRealSession.value;
  if (!session || pendingEditingId.value) {
    return;
  }

  const pendingItems = pendingInputs.value;
  const pendingIds = pendingItems.map(item => item.id);
  if (pendingIds.length > 0) {
    const storedDrafts = webSessionStore.getPendingInputEditDrafts(session.projectId, session.id);
    const pendingIdSet = new Set(pendingIds);
    Object.keys(storedDrafts)
      .filter(pendingId => !pendingIdSet.has(pendingId))
      .forEach(pendingId => {
        webSessionStore.clearPendingInputEditDraft(session.projectId, session.id, pendingId);
      });

    const latest = pickLatestWebSessionPendingInputEditDraft(
      webSessionStore.getPendingInputEditDrafts(session.projectId, session.id),
      pendingItems.filter(item => item.paused && !item.nativeQueued).map(item => item.id)
    );
    if (!latest) {
      return;
    }
    pendingEditingId.value = latest.pendingId;
    pendingEditText.value = latest.draft.text;
    pendingInputPopoverId.value = latest.pendingId;
  }
}

watch([pendingEditingId, pendingEditText], () => {
  persistPendingEditDraft();
});

async function startPendingEdit(item: WebSessionPendingInput) {
  const session = currentRealSession.value;
  if (!session || item.nativeQueued || item.status === 'persisting' || pendingEditActionId.value) {
    return;
  }
  pendingEditActionId.value = item.id;
  try {
    if (!item.paused) {
      await webSessionStore.pausePendingInput(session.id, item.id);
    }
    if (
      currentRealSession.value?.id !== session.id ||
      !pendingInputs.value.some(entry => entry.id === item.id)
    ) {
      return;
    }
    const storedDraft = webSessionStore.getPendingInputEditDraft(
      session.projectId,
      session.id,
      item.id
    );
    pendingEditingId.value = item.id;
    pendingEditText.value = storedDraft?.text ?? item.text;
    pendingInputPopoverId.value = item.id;
    webSessionStore.setPendingInputEditDraft(
      session.projectId,
      session.id,
      item.id,
      pendingEditText.value
    );
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    pendingEditActionId.value = '';
  }
}

async function cancelPendingEdit() {
  const session = currentRealSession.value;
  const pendingId = pendingEditingId.value;
  if (!session || !pendingId || pendingEditActionId.value) {
    return;
  }
  pendingEditActionId.value = pendingId;
  try {
    await webSessionStore.resumePendingInput(session.id, pendingId);
    webSessionStore.clearPendingInputEditDraft(session.projectId, session.id, pendingId);
    if (currentRealSession.value?.id === session.id && pendingEditingId.value === pendingId) {
      clearPendingEditState({ clearDraft: false });
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    pendingEditActionId.value = '';
  }
}

async function handleResumePendingInput(pendingId: string) {
  const session = currentRealSession.value;
  if (!session || pendingEditActionId.value) {
    return;
  }
  pendingEditActionId.value = pendingId;
  try {
    await webSessionStore.resumePendingInput(session.id, pendingId);
    webSessionStore.clearPendingInputEditDraft(session.projectId, session.id, pendingId);
    if (currentRealSession.value?.id === session.id && pendingEditingId.value === pendingId) {
      clearPendingEditState({ clearDraft: false });
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    pendingEditActionId.value = '';
  }
}

async function handlePendingEditSave(pendingId: string) {
  const session = currentRealSession.value;
  if (!session || !pendingEditCanSave.value || pendingEditActionId.value) {
    return;
  }
  pendingEditActionId.value = pendingId;
  try {
    await webSessionStore.updatePendingInput(session.id, pendingId, pendingEditText.value);
    webSessionStore.clearPendingInputEditDraft(session.projectId, session.id, pendingId);
    if (currentRealSession.value?.id === session.id && pendingEditingId.value === pendingId) {
      clearPendingEditState({ clearDraft: false });
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    pendingEditActionId.value = '';
  }
}

async function handleMovePendingInput(
  item: WebSessionPendingInput,
  mode: WebSessionPendingInput['mode'],
  index: number
) {
  if (!currentRealSession.value || item.nativeQueued || item.status === 'persisting') {
    return;
  }
  try {
    await webSessionStore.reorderPendingInput(currentRealSession.value.id, item.id, mode, index);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleMovePendingInputToAbsoluteIndex(
  item: WebSessionPendingInput,
  absoluteIndex: number
) {
  const currentItems = localPendingInputs.value;
  const clampedIndex = Math.max(0, Math.min(absoluteIndex, currentItems.length - 1));
  const targetItem = currentItems[clampedIndex];
  if (!targetItem) {
    return;
  }

  const targetMode = targetItem.mode;
  const partitionItems = currentItems.filter(entry => entry.mode === targetMode);
  const partitionIndex = partitionItems.findIndex(entry => entry.id === targetItem.id);
  const nextIndex = partitionIndex < 0 ? partitionItems.length : partitionIndex;
  await handleMovePendingInput(item, targetMode, nextIndex);
}

async function handleTogglePendingPriority(item: WebSessionPendingInput) {
  if (!currentRealSession.value || item.nativeQueued || item.status === 'persisting') {
    return;
  }
  const targetMode = item.mode === 'redirect' ? 'queue' : 'redirect';
  const targetIndex = localPendingInputs.value.filter(entry => entry.mode === targetMode).length;
  try {
    await webSessionStore.reorderPendingInput(
      currentRealSession.value.id,
      item.id,
      targetMode,
      targetIndex
    );
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

function scheduledModeLabel(mode: WebSessionScheduledInput['mode']) {
  switch (mode) {
    case 'interrupt':
      return t('webSession.scheduledModeInterrupt');
    case 'queue':
      return t('webSession.scheduledModeQueue');
    default:
      return t('webSession.scheduledModeSend');
  }
}

const scheduledIdleConfirmationWindowMs = 20_000;

function scheduledIdleStatusLabel(item: WebSessionScheduledInput) {
  if (item.blockingReasons.length > 0) {
    return item.blockingReasons
      .map(reason => {
        switch (reason) {
          case 'git_dirty':
            return t('webSession.scheduledIdleBlockedGitDirty');
          case 'git_unavailable':
            return t('webSession.scheduledIdleBlockedGitUnavailable');
          case 'non_plan_session_active':
            return t('webSession.scheduledIdleBlockedSession');
        }
      })
      .join(locale.value === 'zh-CN' ? '、' : ', ');
  }
  if (item.idleSince != null) {
    const remainingSeconds = Math.max(
      0,
      Math.ceil(
        (item.idleSince + scheduledIdleConfirmationWindowMs - liveStateClockMs.value) / 1000
      )
    );
    return remainingSeconds > 0
      ? t('webSession.scheduledIdleStabilizing', { seconds: remainingSeconds })
      : t('webSession.scheduledIdleStarting');
  }
  return t('webSession.scheduledIdleChecking');
}

function scheduledInputTimeLabel(item: WebSessionScheduledInput) {
  if (item.scheduleKind === 'when_idle') {
    if (item.idleSince != null && item.blockingReasons.length === 0) {
      const remainingSeconds = Math.max(
        0,
        Math.ceil(
          (item.idleSince + scheduledIdleConfirmationWindowMs - liveStateClockMs.value) / 1000
        )
      );
      return remainingSeconds > 0
        ? t('webSession.scheduledIdleCountdownShort', { seconds: remainingSeconds })
        : t('webSession.scheduledIdleStartingShort');
    }
    return t('webSession.scheduleKindWhenIdle');
  }
  return formatTime(item.scheduledFor ?? item.createdAt);
}

function scheduledInputTimeTitle(item: WebSessionScheduledInput) {
  return item.scheduleKind === 'when_idle'
    ? scheduledIdleStatusLabel(item)
    : formatDateTime(item.scheduledFor ?? item.createdAt);
}

function scheduledInputPreview(item: WebSessionScheduledInput) {
  if (item.action === 'execute_plan') {
    return t('webSession.planActionImplement');
  }
  const text = item.text.trim();
  if (text) {
    return text.length > 72 ? `${text.slice(0, 72)}...` : text;
  }
  return t('webSession.pendingAttachments', { count: item.attachmentIds.length });
}

function scheduledInputDetailText(item: WebSessionScheduledInput) {
  if (item.action === 'execute_plan') {
    return t('webSession.planActionImplement');
  }
  return (
    item.text.trim() || t('webSession.pendingAttachments', { count: item.attachmentIds.length })
  );
}

function scheduledImmediateActionLabel(item: WebSessionScheduledInput) {
  if (item.action === 'execute_plan') {
    return item.status === 'failed'
      ? t('webSession.scheduledRetryPlanNow')
      : t('webSession.scheduledImplementNow');
  }
  return item.status === 'failed'
    ? t('webSession.scheduledRetryNow')
    : t('webSession.scheduledSendNow');
}

function scheduledEditActionLabel(item: WebSessionScheduledInput) {
  if (item.status === 'failed') {
    return t('webSession.scheduledReschedule');
  }
  return item.action === 'execute_plan'
    ? t('webSession.scheduledChangeSchedule')
    : t('common.edit');
}

function scheduledRemoveActionLabel(item: WebSessionScheduledInput) {
  return item.status === 'scheduled'
    ? t('webSession.scheduledCancel')
    : t('webSession.scheduledRemove');
}

function scheduledFailureReason(item: WebSessionScheduledInput) {
  return item.lastError || t('webSession.scheduledFailureUnknown');
}

async function performDispatchScheduledInputNow(item: WebSessionScheduledInput) {
  const session = currentRealSession.value;
  if (!session || scheduledInputActionId.value) {
    return;
  }
  scheduledInputActionId.value = item.id;
  closeScheduledInputPopover();
  try {
    if (!(await ensureMessageCapabilityAvailable(session.agent))) {
      return;
    }
    await webSessionStore.dispatchScheduledInputNow(session.id, item.id);
    message.success(
      item.action === 'execute_plan'
        ? t('webSession.scheduledPlanStarted')
        : t('webSession.scheduledSentNow')
    );
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    scheduledInputActionId.value = '';
  }
}

function handleDispatchScheduledInputNow(item: WebSessionScheduledInput) {
  if (item.action === 'message' && item.mode === 'interrupt' && isRunActive.value) {
    closeScheduledInputPopover();
    dialog.warning({
      title: t('webSession.scheduledInterruptNowTitle'),
      content: t('webSession.scheduledInterruptNowBody'),
      positiveText: t('webSession.scheduledInterruptNowConfirm'),
      negativeText: t('common.cancel'),
      onPositiveClick: () => performDispatchScheduledInputNow(item),
    });
    return;
  }
  void performDispatchScheduledInputNow(item);
}

async function handleRemovePendingInput(pendingId: string) {
  const session = currentRealSession.value;
  if (
    !session ||
    pendingInputs.value.some(
      item => item.id === pendingId && (item.nativeQueued || item.status === 'persisting')
    )
  ) {
    return;
  }
  try {
    await webSessionStore.removePendingInput(session.id, pendingId);
    webSessionStore.clearPendingInputEditDraft(session.projectId, session.id, pendingId);
    if (currentRealSession.value?.id === session.id && pendingEditingId.value === pendingId) {
      clearPendingEditState({ clearDraft: false });
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

async function handleRemoveScheduledInput(inputId: string) {
  if (!currentRealSession.value || scheduledInputActionId.value) {
    return;
  }
  scheduledInputActionId.value = inputId;
  closeScheduledInputPopover();
  try {
    await webSessionStore.removeScheduledInput(currentRealSession.value.id, inputId);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  } finally {
    scheduledInputActionId.value = '';
  }
}

function userInputPlaceholder(question: WebSessionUserInputQuestion) {
  if (question.options.length === 0) {
    return t('webSession.userInputAnswerPlaceholder');
  }
  if (question.isOther) {
    return t('webSession.userInputOtherPlaceholder');
  }
  return t('webSession.userInputAnswerPlaceholder');
}

function userInputQuestionMemoDeps(question: WebSessionUserInputQuestion) {
  return buildWebSessionUserInputQuestionMemoDeps({
    requestKey: pendingUserInputSyncKey.value,
    question,
    selections: userInputSelections.value[question.id],
    draft: userInputDrafts.value[question.id],
    disabled: isUserInputInteractionDisabled.value,
    placeholder: userInputPlaceholder(question),
  });
}

function buildUserInputAnswers() {
  const request = pendingUserInput.value;
  if (!request) {
    return null;
  }
  const answers: Record<string, string[]> = {};
  for (const question of request.questions) {
    const values = [...(userInputSelections.value[question.id] ?? [])];
    const freeform = (userInputDrafts.value[question.id] ?? '').trim();
    if (question.options.length === 0) {
      if (freeform) {
        answers[question.id] = [freeform];
      }
      continue;
    }
    if (question.isOther && freeform) {
      values.push(freeform);
    }
    if (values.length > 0) {
      answers[question.id] = values;
    }
  }
  return answers;
}

function formatSessionInteractionError(error: unknown) {
  const rawMessage = error instanceof Error ? error.message.trim() : '';
  if (rawMessage.includes('session is not running')) {
    return t('webSession.recoveredActionExpired');
  }
  return rawMessage || t('common.error');
}

async function refreshRuntimeCapabilities() {
  await loadCodexRuntimeConfig(true);
  return codexRuntimeConfig.value;
}

function goalModeUnavailableMessage() {
  if (runtimeCodexVersion.value) {
    return t('webSession.goalModeUnavailableWithCurrent', {
      requiredVersion: runtimeGoalModeMinVersion.value,
      currentVersion: runtimeCodexVersion.value,
    });
  }
  return t('webSession.goalModeUnavailable', {
    version: runtimeGoalModeMinVersion.value,
  });
}

function codexCompatibilityModeMessage() {
  if (runtimeCodexVersion.value) {
    return t('webSession.codexCompatibilityModeWithCurrent', {
      requiredVersion: runtimeMultiAgentV2MinVersion.value,
      currentVersion: runtimeCodexVersion.value,
    });
  }
  return t('webSession.codexCompatibilityMode', {
    requiredVersion: runtimeMultiAgentV2MinVersion.value,
  });
}

function maybeNotifyCodexCompatibilityMode() {
  if (!isCodexCompatibilityMode.value) {
    return;
  }
  message.warning(codexCompatibilityModeMessage());
}

async function ensureMessageCapabilityAvailable(
  agent: WebSessionAgent,
  options: { refresh?: boolean } = {}
) {
  const config = options.refresh === false ? codexRuntimeConfig.value : await refreshRuntimeCapabilities();
  if (!config) {
    if (agent === 'pi') {
      message.warning(piUnavailableReason.value);
      return false;
    }
    return true;
  }
  const capability = resolveWebSessionAgentCapability(config, agent);
  if (capability.supportsWebSession) {
    if (agent === 'pi') {
      return ensurePiProjectTrust(false);
    }
    return true;
  }
  if (agent === 'codex') {
    message.warning(t('webSession.codexNotInstalled'));
  } else if (agent === 'claude') {
    message.warning(t('webSession.claudeCodeNotInstalled'));
  } else {
    message.warning(piUnavailableReason.value);
  }
  return false;
}

async function ensureGoalModeAvailable() {
  const config = await refreshRuntimeCapabilities();
  if (!config) {
    return true;
  }
  if (config.hasCodex !== true) {
    message.warning(t('webSession.codexNotInstalled'));
    return false;
  }
  if (config.supportsGoalMode === true) {
    return true;
  }
  message.warning(goalModeUnavailableMessage());
  return false;
}

function findInlinePlanChoiceOption(mode: 'execute' | 'plan') {
  if (!inlinePlanChoice.value) {
    return null;
  }
  return (
    inlinePlanChoice.value.options.find(option => option.isExecute === (mode === 'execute')) ?? null
  );
}

async function answerInlinePlanChoice(mode: 'execute' | 'plan') {
  if (!currentRealSession.value || !pendingUserInput.value || !inlinePlanChoice.value) {
    return false;
  }
  const draftStorageKey = activeUserInputDraftStorageKey || pendingUserInputDraftStorageKey.value;
  const option = findInlinePlanChoiceOption(mode);
  if (!option || !inlinePlanChoice.value.questionId) {
    return false;
  }
  await webSessionStore.answerUserInput(
    currentRealSession.value.id,
    pendingUserInput.value.itemId,
    {
      [inlinePlanChoice.value.questionId]: [option.label],
    }
  );
  clearUserInputDraftStorage(draftStorageKey);
  userInputSelections.value = {};
  userInputDrafts.value = {};
  return true;
}

async function handlePlanCardImplement() {
  closePlanQuickActions();
  if (!currentRealSession.value || isSubmittingMessage.value) {
    return;
  }
  if (
    !ensureSendConflictConfirmed(planImplementConfirmationSignature.value, { notifyOnBlock: true })
  ) {
    return;
  }
  let submitOwnerId = currentRealSession.value.id;
  beginSessionSubmit(submitOwnerId, 'execute_plan');
  try {
    const sourceSession = currentRealSession.value;
    if (!sourceSession) {
      return;
    }
    const prepared = await prepareSessionForSend(sourceSession, {
      shouldActivate: () => isCurrentVisibleSession(sourceSession.id),
    });
    const targetSession = prepared.session;
    if (targetSession.id !== submitOwnerId) {
      transferSessionSubmit(submitOwnerId, targetSession.id);
      submitOwnerId = targetSession.id;
    }

    if (targetSession.workflowMode === 'plan') {
      await webSessionStore.updateWorkflowMode(targetSession.id, 'default');
    }

    const answered = await answerInlinePlanChoice('execute');
    if (!answered) {
      if (!(await ensureMessageCapabilityAvailable(targetSession.agent))) {
        return;
      }
      await webSessionStore.sendMessage(targetSession.id, 'Implement the plan.', []);
    }

    if (prepared.navigateProjectId && isCurrentVisibleSession(targetSession.id)) {
      projectStore.addRecentProject(prepared.navigateProjectId);
      await router.push(buildProjectRouteLocation(prepared.navigateProjectId, targetSession.id));
    }
    if (isCurrentVisibleSession(targetSession.id)) {
      autoFollowBottom.value = true;
      scrollToBottom(true);
    }
  } catch (error) {
    message.error(formatSessionInteractionError(error));
  } finally {
    endSessionSubmit(submitOwnerId);
  }
}

async function handlePlanCardCancel() {
  closePlanQuickActions();
  const toolId = latestPlanToolId.value;
  setPlanActionsDismissed(toolId, true);
  focusComposer();
}

async function handleUserInputSubmit() {
  if (!currentRealSession.value || !pendingUserInput.value || isSubmittingUserInput.value) {
    return;
  }
  const sessionId = currentRealSession.value.id;
  const request = pendingUserInput.value;
  if (request.stale) {
    message.info(request.recoveryMessage || t('webSession.recoveredActionExpired'));
    return;
  }
  const answers = buildUserInputAnswers();
  if (!answers) {
    return;
  }
  if (hasMissingWebSessionUserInputAnswers(request.questions, answers)) {
    message.warning(t('webSession.userInputAnswerRequired'));
    return;
  }
  const submitOwnerId = buildWebSessionUserInputSubmitOwnerId(sessionId, request.itemId);
  if (!submitOwnerId) {
    return;
  }
  const draftStorageKey = activeUserInputDraftStorageKey || pendingUserInputDraftStorageKey.value;
  beginUserInputSubmit(submitOwnerId);
  let answered = false;
  try {
    await webSessionStore.answerUserInput(sessionId, request.itemId, answers);
    answered = true;
    clearUserInputDraftStorage(draftStorageKey);
    userInputSelections.value = {};
    userInputDrafts.value = {};
  } catch (error) {
    message.error(formatSessionInteractionError(error));
  } finally {
    if (!answered || currentUserInputSubmitOwnerId.value !== submitOwnerId) {
      endUserInputSubmit(submitOwnerId);
    }
  }
}

async function handleApproval(action: 'approve' | 'reject') {
  if (!currentRealSession.value || !pendingApproval.value) {
    return;
  }
  if (pendingApproval.value.stale) {
    message.info(pendingApproval.value.recoveryMessage || t('webSession.recoveredActionExpired'));
    return;
  }
  try {
    if (action === 'approve') {
      await webSessionStore.approveSession(currentRealSession.value.id);
      return;
    }
    await webSessionStore.rejectSession(currentRealSession.value.id);
  } catch (error) {
    message.error(formatSessionInteractionError(error));
  }
}

async function handleAbortCurrent() {
  if (!currentRealSession.value) {
    return;
  }
  try {
    await webSessionStore.abortSession(currentRealSession.value.id);
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

function resolveTimelinePositionStorage(): Storage | null {
  if (typeof window === 'undefined') {
    return null;
  }
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function persistTimelinePositionStateNow() {
  persistWebSessionTimelinePositionState(timelinePositionState, timelinePositionStorage);
}

const scheduleTimelinePositionPersist = useDebounceFn(
  persistTimelinePositionStateNow,
  WEB_SESSION_TIMELINE_POSITION_DEBOUNCE_MS,
  { maxWait: WEB_SESSION_TIMELINE_POSITION_MAX_WAIT_MS }
);

function readRenderedTimelineAnchor(container: HTMLDivElement) {
  const containerRect = container.getBoundingClientRect();
  const elements = Array.from(
    container.querySelectorAll<HTMLElement>('.timeline-item[data-timeline-key]')
  );
  let low = 0;
  let high = elements.length - 1;
  let firstVisibleIndex = -1;
  while (low <= high) {
    const middle = Math.floor((low + high) / 2);
    const rect = elements[middle].getBoundingClientRect();
    if (rect.bottom > containerRect.top) {
      firstVisibleIndex = middle;
      high = middle - 1;
    } else {
      low = middle + 1;
    }
  }
  const element = firstVisibleIndex >= 0 ? elements[firstVisibleIndex] : null;
  if (!element) {
    return null;
  }
  const key = element.dataset.timelineKey ?? '';
  const orderIndex = Number(element.dataset.timelineOrderIndex);
  const rect = element.getBoundingClientRect();
  const candidates =
    key && Number.isFinite(orderIndex)
      ? [{ key, orderIndex, top: rect.top, bottom: rect.bottom }]
      : [];
  const anchor = resolveWebSessionTimelineAnchor(
    candidates,
    containerRect.top,
    containerRect.bottom
  );
  if (!anchor) {
    return null;
  }
  return {
    key: anchor.key,
    orderIndex: anchor.orderIndex,
    offsetPx: anchor.top - containerRect.top,
  };
}

function captureTimelinePosition(projectId: string, sessionId: string, persistImmediately = false) {
  const container = timelineScrollRef.value;
  if (!projectId || !sessionId || !container || getCurrentTimelineEdgeWindow()) {
    return false;
  }
  const metrics = readTimelineScrollMetrics(container);
  const followState = createWebSessionTimelineFollowState(metrics, autoFollowBottom.value);
  const anchor = readRenderedTimelineAnchor(container);
  timelinePositionState = rememberWebSessionTimelinePosition(
    timelinePositionState,
    projectId,
    sessionId,
    {
      anchorKey: anchor?.key ?? '',
      anchorOrderIndex: anchor?.orderIndex ?? null,
      anchorOffsetPx: anchor?.offsetPx ?? 0,
      scrollTop: Math.max(0, container.scrollTop),
      followBottom: followState.autoFollowBottom,
      updatedAt: Date.now(),
    }
  );
  if (persistImmediately) {
    persistTimelinePositionStateNow();
  } else {
    void scheduleTimelinePositionPersist();
  }
  return true;
}

const scheduleTimelinePositionCapture = useDebounceFn(
  () => {
    if (!props.isActive || pendingTimelinePositionRestore.value) {
      return;
    }
    captureTimelinePosition(props.projectId, currentSession.value?.id ?? '');
  },
  WEB_SESSION_TIMELINE_POSITION_DEBOUNCE_MS,
  { maxWait: WEB_SESSION_TIMELINE_POSITION_MAX_WAIT_MS }
);

function forgetTimelinePosition(projectId: string, sessionId: string) {
  timelinePositionState = forgetWebSessionTimelinePosition(
    timelinePositionState,
    projectId,
    sessionId
  );
  persistTimelinePositionStateNow();
}

function cancelTimelinePositionRestore() {
  timelinePositionRestoreGeneration += 1;
  pendingTimelinePositionRestore.value = null;
}

function invalidateTimelineScrollSync() {
  timelineScrollSyncVersion += 1;
}

function cancelTimelinePositionRestoreForUserInteraction(container: HTMLDivElement) {
  invalidateTimelineScrollSync();
  if (!pendingTimelinePositionRestore.value) {
    return;
  }
  cancelTimelinePositionRestore();
  resetBottomState(container, false);
  resetMobileComposerScrollState(container);
  void scheduleTimelinePositionCapture();
}

function isTimelinePositionRestoreCurrent(
  generation: number,
  projectId: string,
  sessionId: string
) {
  return (
    pendingTimelinePositionRestore.value?.generation === generation &&
    timelinePositionRestoreGeneration === generation &&
    props.projectId === projectId &&
    currentSession.value?.id === sessionId &&
    props.isActive
  );
}

async function waitForTimelinePositionLayout() {
  await nextTick();
  if (typeof window === 'undefined' || typeof window.requestAnimationFrame !== 'function') {
    return;
  }
  await new Promise<void>(resolve => {
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => resolve());
    });
  });
}

function getTimelineRestoreCandidates() {
  return visibleBlocks.value.flatMap(block => {
    const element = timelineBlockElements.get(block.key);
    if (!element) {
      return [];
    }
    return [{ key: block.key, orderIndex: block.orderIndex, element }];
  });
}

function applyTimelinePositionRestore(
  container: HTMLDivElement,
  position: WebSessionTimelinePosition,
  element?: HTMLElement
) {
  if (element) {
    const containerRect = container.getBoundingClientRect();
    const elementRect = element.getBoundingClientRect();
    container.scrollTop = resolveWebSessionTimelineRestoreScrollTop(
      container.scrollTop,
      elementRect.top - containerRect.top,
      position.anchorOffsetPx,
      Math.max(0, container.scrollHeight - container.clientHeight)
    );
  } else {
    container.scrollTop = Math.min(
      Math.max(0, container.scrollHeight - container.clientHeight),
      Math.max(0, position.scrollTop)
    );
  }
  resetBottomState(container, false);
  resetMobileComposerScrollState(container);
}

async function restorePendingTimelinePosition() {
  const pending = pendingTimelinePositionRestore.value;
  if (
    !pending ||
    timelinePositionRestoreRunningGeneration === pending.generation ||
    !props.isActive
  ) {
    return;
  }
  const { generation, projectId, sessionId, position } = pending;
  timelinePositionRestoreRunningGeneration = generation;
  try {
    await waitForTimelinePositionLayout();
    while (isTimelinePositionRestoreCurrent(generation, projectId, sessionId)) {
      const container = timelineScrollRef.value;
      if (!container || container.clientHeight <= 0) {
        return;
      }
      const meta = currentRealSession.value
        ? webSessionStore.getHistoryMeta(sessionId)
        : { hasMore: false, beforeCursor: '', total: 0, loading: false };
      if (meta.loading) {
        return;
      }

      const candidates = getTimelineRestoreCandidates();
      const exact = position.anchorKey
        ? candidates.find(candidate => candidate.key === position.anchorKey)
        : undefined;
      if (exact) {
        applyTimelinePositionRestore(container, position, exact.element);
        pendingTimelinePositionRestore.value = null;
        void scheduleTimelinePositionCapture();
        ensureTimelineHistoryFilled();
        return;
      }

      const minimumOrderIndex = candidates.reduce(
        (minimum, candidate) => Math.min(minimum, candidate.orderIndex),
        Number.POSITIVE_INFINITY
      );
      const needsEarlierHistory =
        position.anchorOrderIndex != null &&
        (!Number.isFinite(minimumOrderIndex) || minimumOrderIndex > position.anchorOrderIndex);
      if (needsEarlierHistory && currentRealSession.value && meta.hasMore && meta.beforeCursor) {
        const previousCursor = meta.beforeCursor;
        try {
          await webSessionStore.loadMoreHistory(
            sessionId,
            WEB_SESSION_TIMELINE_RESTORE_HISTORY_LIMIT
          );
        } catch (error) {
          console.warn('[Web Session] Failed to restore timeline history', {
            sessionId,
            error,
          });
          applyTimelinePositionRestore(container, position);
          pendingTimelinePositionRestore.value = null;
          void scheduleTimelinePositionCapture();
          return;
        }
        if (!isTimelinePositionRestoreCurrent(generation, projectId, sessionId)) {
          return;
        }
        const nextMeta = webSessionStore.getHistoryMeta(sessionId);
        await waitForTimelinePositionLayout();
        if (
          nextMeta.beforeCursor === previousCursor &&
          nextMeta.hasMore === meta.hasMore &&
          getTimelineRestoreCandidates().length === candidates.length
        ) {
          const latestContainer = timelineScrollRef.value;
          if (latestContainer) {
            applyTimelinePositionRestore(latestContainer, position);
          }
          pendingTimelinePositionRestore.value = null;
          void scheduleTimelinePositionCapture();
          return;
        }
        continue;
      }

      const closest = findClosestWebSessionTimelineAnchor(
        candidates,
        position.anchorKey,
        position.anchorOrderIndex
      );
      applyTimelinePositionRestore(container, position, closest?.element);
      pendingTimelinePositionRestore.value = null;
      void scheduleTimelinePositionCapture();
      ensureTimelineHistoryFilled();
      return;
    }
  } finally {
    if (timelinePositionRestoreRunningGeneration === generation) {
      timelinePositionRestoreRunningGeneration = 0;
    }
  }
}

function beginTimelinePositionRestore(projectId: string, sessionId: string) {
  cancelTimelinePositionRestore();
  invalidateTimelineScrollSync();
  if (!projectId || !sessionId) {
    return;
  }
  const position = getWebSessionTimelinePosition(timelinePositionState, projectId, sessionId);
  if (!position || position.followBottom) {
    autoFollowBottom.value = true;
    showJumpToBottom.value = false;
    if (props.isActive) {
      scrollToBottom(true);
    }
    return;
  }

  const generation = timelinePositionRestoreGeneration;
  autoFollowBottom.value = false;
  showJumpToBottom.value = true;
  lastTimelineScrollTop.value = position.scrollTop;
  pendingTimelinePositionRestore.value = {
    projectId,
    sessionId,
    position,
    generation,
  };
  void restorePendingTimelinePosition();
}

function handleTimelinePositionPageHide() {
  if (!pendingTimelinePositionRestore.value) {
    captureTimelinePosition(props.projectId, currentSession.value?.id ?? '', true);
  }
  persistTimelinePositionStateNow();
}

function syncScrollToBottom() {
  const container = timelineScrollRef.value;
  if (!container) {
    return;
  }
  container.scrollTop = Math.max(0, container.scrollHeight - container.clientHeight);
  autoFollowBottom.value = true;
  showJumpToBottom.value = false;
  lastTimelineScrollTop.value = container.scrollTop;
  resetMobileComposerScrollState(container);
  if (!pendingTimelinePositionRestore.value) {
    void scheduleTimelinePositionCapture();
  }
}

function scheduleScrollToBottom(force = false) {
  const scheduledVersion = ++timelineScrollSyncVersion;
  nextTick(() => {
    const run = () => {
      const container = timelineScrollRef.value;
      if (
        !container ||
        !shouldApplyWebSessionTimelineAutoScroll(
          scheduledVersion,
          timelineScrollSyncVersion,
          force,
          autoFollowBottom.value
        )
      ) {
        return;
      }
      if (force || autoFollowBottom.value) {
        syncScrollToBottom();
      } else {
        updateBottomState(container);
      }
    };

    if (typeof window === 'undefined' || typeof window.requestAnimationFrame !== 'function') {
      run();
      return;
    }

    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(run);
    });
  });
}

function scrollToBottom(force = false) {
  if (!force && !autoFollowBottom.value) {
    return;
  }
  if (force && getCurrentTimelineEdgeWindow()) {
    resetTimelineEdgeWindow();
  }
  scheduleScrollToBottom(force);
}

async function handleLiveCardClick() {
  if (shouldAutoContinueOnLiveCardClick.value && currentRealSession.value) {
    liveCardContinuePending.value = true;
    try {
      await continueErroredSession(currentRealSession.value);
      return;
    } catch (error) {
      message.error(formatSessionInteractionError(error));
    } finally {
      liveCardContinuePending.value = false;
    }
  }
  scrollToBottom(true);
}

function readTimelineScrollMetrics(container: HTMLDivElement): WebSessionTimelineScrollMetrics {
  return {
    scrollTop: container.scrollTop,
    scrollHeight: container.scrollHeight,
    clientHeight: container.clientHeight,
  };
}

function applyTimelineFollowState(state: {
  autoFollowBottom: boolean;
  showJumpToBottom: boolean;
  lastScrollTop: number;
}) {
  autoFollowBottom.value = state.autoFollowBottom;
  showJumpToBottom.value = state.showJumpToBottom;
  lastTimelineScrollTop.value = state.lastScrollTop;
}

function updateBottomState(container: HTMLDivElement) {
  applyTimelineFollowState(
    resolveWebSessionTimelineFollowState(
      {
        autoFollowBottom: autoFollowBottom.value,
        showJumpToBottom: showJumpToBottom.value,
        lastScrollTop: lastTimelineScrollTop.value,
      },
      readTimelineScrollMetrics(container)
    )
  );
}

function resetBottomState(container: HTMLDivElement, autoFollow = autoFollowBottom.value) {
  applyTimelineFollowState(
    createWebSessionTimelineFollowState(readTimelineScrollMetrics(container), autoFollow)
  );
}

function restoreHistoryAnchor() {
  const anchor = pendingHistoryAnchor.value;
  const container = timelineScrollRef.value;
  if (!anchor || !container || currentSession.value?.id !== anchor.sessionId) {
    return false;
  }
  container.scrollTop = anchor.previousTop + (container.scrollHeight - anchor.previousHeight);
  pendingHistoryAnchor.value = null;
  resetBottomState(container);
  return true;
}

function handleMobileTimelineScrollForComposer(container: HTMLDivElement) {
  if (!isMobile.value) {
    mobileComposerScrollState.value = null;
    return;
  }

  const metrics = readTimelineScrollMetrics(container);
  const previous =
    mobileComposerScrollState.value ?? createWebSessionMobileComposerScrollState(metrics);
  const resolved = resolveWebSessionMobileComposerScrollState(previous, metrics);
  mobileComposerScrollState.value = resolved.state;

  if (resolved.action === 'collapse' && !isMobileComposerCollapsed.value) {
    collapseMobileComposerPanel();
  }
}

function handleMobileTimelineBottomScroll(container: HTMLDivElement, scrollDownDelta: number) {
  if (!isMobile.value || !isMobileComposerCollapsed.value) {
    return;
  }
  const action = resolveWebSessionMobileComposerBottomScrollAction(
    readTimelineScrollMetrics(container),
    scrollDownDelta
  );
  if (action !== 'expand') {
    return;
  }
  expandMobileComposerPanel();
  resetMobileComposerScrollState(container);
}

function handleTimelineWheel(event: WheelEvent) {
  const container = event.currentTarget as HTMLDivElement | null;
  if (!container) {
    return;
  }
  cancelTimelinePositionRestoreForUserInteraction(container);
  handleMobileTimelineBottomScroll(container, event.deltaY);
}

function handleTimelinePointerDown(event: PointerEvent) {
  const container = event.currentTarget as HTMLDivElement | null;
  if (container) {
    cancelTimelinePositionRestoreForUserInteraction(container);
  }
}

function handleTimelineTouchStart(event: TouchEvent) {
  const container = event.currentTarget as HTMLDivElement | null;
  if (container) {
    cancelTimelinePositionRestoreForUserInteraction(container);
  }
  mobileTimelineTouchY = event.touches[0]?.clientY ?? null;
}

function handleTimelineTouchMove(event: TouchEvent) {
  const container = event.currentTarget as HTMLDivElement | null;
  const touchY = event.touches[0]?.clientY;
  if (!container || typeof touchY !== 'number' || mobileTimelineTouchY == null) {
    mobileTimelineTouchY = touchY ?? null;
    return;
  }
  const scrollDownDelta = mobileTimelineTouchY - touchY;
  mobileTimelineTouchY = touchY;
  handleMobileTimelineBottomScroll(container, scrollDownDelta);
}

function handleTimelineTouchEnd() {
  mobileTimelineTouchY = null;
}

function handleTimelineScroll(event: Event) {
  const container = event.currentTarget as HTMLDivElement | null;
  if (!container) {
    return;
  }
  if (!props.isActive) {
    return;
  }
  if (pendingTimelinePositionRestore.value) {
    cancelTimelinePositionRestoreForUserInteraction(container);
    return;
  }
  const nearTop = container.scrollTop < 120;
  const wasAutoFollowingBottom = autoFollowBottom.value;
  updateBottomState(container);
  if (wasAutoFollowingBottom && !autoFollowBottom.value) {
    invalidateTimelineScrollSync();
  }
  handleMobileTimelineScrollForComposer(container);
  refreshTimelineViewportNavigation();
  void scheduleTimelinePositionCapture();
  if (
    nearTop &&
    !getCurrentTimelineEdgeWindow() &&
    !pendingHistoryAnchor.value &&
    currentRealSession.value &&
    historyMeta.value.hasMore &&
    !historyMeta.value.loading
  ) {
    pendingHistoryAnchor.value = {
      sessionId: currentRealSession.value.id,
      previousHeight: container.scrollHeight,
      previousTop: container.scrollTop,
    };
    void webSessionStore.loadMoreHistory(currentRealSession.value.id).catch(error => {
      pendingHistoryAnchor.value = null;
      console.error('[Web Session] Failed to load more history', error);
    });
  }
}

function ensureTimelineHistoryFilled() {
  const container = timelineScrollRef.value;
  if (
    !container ||
    !currentRealSession.value ||
    getCurrentTimelineEdgeWindow() ||
    pendingHistoryAnchor.value ||
    historyMeta.value.loading ||
    !historyMeta.value.hasMore
  ) {
    return;
  }
  const lacksScrollableOverflow = container.scrollHeight <= container.clientHeight + 24;
  if (!lacksScrollableOverflow) {
    return;
  }
  void webSessionStore.loadMoreHistory(currentRealSession.value.id).catch(error => {
    console.error('[Web Session] Failed to auto-fill history', error);
  });
}

function recalcTabTitleWidth(explicitWidth?: number) {
  if (typeof explicitWidth === 'number') {
    tabsContainerWidth.value = explicitWidth;
  }
  const containerWidth =
    typeof explicitWidth === 'number' ? explicitWidth : tabsContainerWidth.value;
  if (!containerWidth) {
    tabTitleMaxWidth.value = MAX_TAB_TITLE_WIDTH;
    return;
  }
  const sessionCount = Math.max(sessions.value.length, 1);
  let activeOffset = TABS_CONTAINER_STATIC_OFFSET;
  if (containerWidth - activeOffset < SHARED_WIDTH_HIDE_THRESHOLD) {
    activeOffset = TABS_CONTAINER_MIN_OFFSET;
  }
  const availableWidth = Math.max(containerWidth - activeOffset, 0);
  const rawWidth = availableWidth / sessionCount - TAB_LABEL_EXTRA_SPACE;
  tabTitleMaxWidth.value = Math.round(Math.min(MAX_TAB_TITLE_WIDTH, Math.max(56, rawWidth)));
}

function updateActiveTabIndicator() {
  nextTick(() => {
    activeTabIndicatorStyle.value =
      !isMobile.value && activeTabSessionId.value
        ? calculateCardTabIndicatorStyle(tabsContainerRef.value)
        : hiddenCardTabIndicatorStyle();
  });
}

function syncActiveTabIntoView() {
  if (isMobile.value || !activeTabSessionId.value) {
    return;
  }

  nextTick(() => {
    if (isMobile.value || !activeTabSessionId.value) {
      return;
    }
    if (ensureActiveCardTabVisible(tabsContainerRef.value)) {
      updateActiveTabIndicator();
    }
  });
}

function setupTabScrollListener() {
  cleanupTabScrollListener();
  nextTick(() => {
    if (isMobile.value) {
      return;
    }
    const container = tabsContainerRef.value;
    if (!container) {
      return;
    }
    const scrollContainer = container.querySelector('.v-x-scroll') as HTMLElement | null;
    if (scrollContainer) {
      tabScrollContainer = scrollContainer;
      scrollContainer.addEventListener('scroll', updateActiveTabIndicator);
    }
  });
}

function cleanupTabScrollListener() {
  if (tabScrollContainer) {
    tabScrollContainer.removeEventListener('scroll', updateActiveTabIndicator);
    tabScrollContainer = null;
  }
}

function shouldShowSessionWorkflowPlanBadge(
  session: Pick<WebSessionSummary, 'workflowMode'> | null | undefined
) {
  return session?.workflowMode === 'plan';
}

function hasScheduledPlanExecution(
  session: Pick<WebSessionSummary, 'hasScheduledPlanExecution'> | null | undefined
) {
  return session?.hasScheduledPlanExecution === true;
}

function shouldHighlightScheduledInputSession(
  session: Pick<WebSessionSummary, 'id'> | null | undefined
) {
  return Boolean(
    session &&
      webSessionStore.getScheduledInputs(session.id).some(item => item.status === 'scheduled')
  );
}

function createTabProps(session: (typeof sessions.value)[number]): HTMLAttributes {
  const isActive = activeTabSessionId.value === session.id;
  const theme = activeTheme.value;
  const hideHeaderBorder = theme.terminalHeaderBorder === false;
  const props: HTMLAttributes = {
    onContextmenu: (event: MouseEvent) => handleTabContextMenu(event, session),
  };
  const classes: string[] = [];

  if (isSessionArchiving(session.id)) {
    classes.push('is-archiving');
  }
  if (isArchivedPreviewSession(session)) {
    classes.push('is-tab-drag-locked');
  }

  if (usesSessionPlanApprovalTone(session)) {
    classes.push('has-unviewed-plan-approval');
    if (isActive && hideHeaderBorder) {
      props.style = {
        borderBottom: 'none',
      };
    }
  } else if (usesSessionApprovalTone(session)) {
    classes.push('has-unviewed-approval');
    if (isActive && hideHeaderBorder) {
      props.style = {
        borderBottom: 'none',
      };
    }
  } else if (usesSessionCompletionTone(session)) {
    classes.push('has-unviewed-completion');
    if (isActive && hideHeaderBorder) {
      props.style = {
        borderBottom: 'none',
      };
    }
  } else if (isActive) {
    props.style = {
      backgroundColor: theme.terminalTabActiveBg || theme.surfaceColor,
      ...(hideHeaderBorder ? { borderBottom: 'none' } : {}),
    };
  } else {
    props.style = {
      backgroundColor: theme.terminalTabBg || theme.bodyColor,
    };
  }

  if (classes.length > 0) {
    props.class = classes.join(' ');
  }
  if (shouldShowSessionWorkflowPlanBadge(session)) {
    props.class = [props.class, 'has-workflow-plan-badge'].filter(Boolean).join(' ');
  }
  return props;
}

function getSessionDisplayState(session: SessionTab): WebSessionDisplayState {
  return resolveWebSessionDisplayState({
    isDraft: isDraftSession(session),
    hasUnread: hasSessionUnread(session),
    status: session.status,
    syncState: session.syncState,
    livePhase: isDraftSession(session) ? null : webSessionStore.getLiveState(session.id).phase,
    assistantState: session.assistantState,
  });
}

function getSessionLabelState(session: (typeof sessions.value)[number]) {
  return getSessionDisplayState(session).assistantStateClass;
}

function getSessionVisualInput(session: (typeof sessions.value)[number]) {
  if (isDraftSession(session)) {
    return null;
  }
  return {
    phase: webSessionStore.getLiveState(session.id).phase,
    hasUnread: hasSessionUnread(session),
    status: session.status,
  } as const;
}

function getSessionStatusLabel(session: (typeof sessions.value)[number]) {
  const labelKey = getSessionDisplayState(session).statusLabelKey;
  return labelKey ? t(labelKey) : '';
}

function getSessionPillStateClass(session: (typeof sessions.value)[number]) {
  return getSessionDisplayState(session).pillStateClass;
}

function getSessionAttentionStateClass(session: (typeof sessions.value)[number]) {
  return getSessionDisplayState(session).attentionStateClass;
}

function getSessionStatusEmoji(session: (typeof sessions.value)[number]) {
  return getSessionDisplayState(session).statusEmoji;
}

function getSessionAssistantIcon(session: (typeof sessions.value)[number]) {
  return getAgentIcon(session.agent);
}

function getSessionStatusTooltip(session: (typeof sessions.value)[number]) {
  const label = getSessionStatusLabel(session);
  const agentName = getAgentDisplayName(session.agent);
  if (isDraftSession(session)) {
    return agentName;
  }
  const suffix = session.syncState === 'error' && session.syncError ? session.syncError : '';
  const base = label ? `${agentName} · ${label}` : agentName;
  return suffix ? `${base} · ${suffix}` : base;
}

function getSessionHoverTimeText(
  session: Pick<WebSessionSummary, 'updatedAt' | 'lastMessageAt' | 'createdAt'> | null | undefined
) {
  if (!session) {
    return '';
  }
  const timestamp = parseTimestamp(session.updatedAt || session.lastMessageAt || session.createdAt);
  return timestamp > 0 ? formatDateTime(timestamp) : '';
}

function joinSessionHoverParts(parts: Array<string | null | undefined>) {
  return parts
    .map(part => String(part ?? '').trim())
    .filter(Boolean)
    .join(' · ');
}

function getSidebarSessionAccentColor(tone: ReturnType<typeof getWebSessionSidebarTone>): string {
  switch (tone) {
    case 'working':
      return '#8b5cf6';
    case 'approval':
      return approvalColors.value.accent;
    case 'plan_approval':
      return planApprovalColors.value.accent;
    case 'completion':
      return '#10b981';
    case 'idle':
      return '#9ca3af';
    case 'error':
      return '#f04438';
    default:
      return 'rgba(15, 23, 42, 0.08)';
  }
}

function getSidebarSessionToneClass(tone: ReturnType<typeof getWebSessionSidebarTone>): string {
  switch (tone) {
    case 'working':
      return 'session-sidebar-working';
    case 'approval':
      return 'session-sidebar-approval';
    case 'plan_approval':
      return 'session-sidebar-plan-approval';
    case 'completion':
      return 'session-sidebar-completion';
    case 'idle':
      return 'session-sidebar-idle';
    case 'error':
      return 'session-sidebar-error';
    default:
      return '';
  }
}

function buildSidebarSessionRow(
  item: CrossProjectSessionItem,
  archived: boolean
): WebSessionSidebarRowView {
  const session = item.session;
  const activityTimestamp = resolveWebSessionSidebarSortTimestamp(session);
  const phase = webSessionStore.getLiveState(session.id).phase;
  const hasUnread = hasSessionUnread(session);
  const displayState = resolveWebSessionDisplayState({
    isDraft: false,
    hasUnread,
    status: session.status,
    syncState: session.syncState,
    livePhase: phase,
    assistantState: session.assistantState,
  });
  const subtitle =
    showSidebarStatusText.value && displayState.statusLabelKey
      ? t(displayState.statusLabelKey)
      : '';
  const tone = getWebSessionSidebarTone({
    phase,
    hasUnread,
    status: session.status,
  });
  const searchMatchLabels = (session.searchMatchSources ?? []).map(source =>
    source === 'title'
      ? t('webSession.sidebarSearchMatchTitle')
      : t('webSession.sidebarSearchMatchBody')
  );

  return {
    key: `${archived ? 'archived' : 'current'}:${item.projectId}:${session.id}`,
    sessionId: session.id,
    title: session.title,
    searchMatchLabel: searchMatchLabels.length > 0 ? `[${searchMatchLabels.join(',')}]` : '',
    iconHtml: getAgentIcon(session.agent),
    subtitle,
    tooltip: joinSessionHoverParts([
      item.projectName,
      session.title,
      subtitle,
      getSessionHoverTimeText(session),
    ]),
    accentColor: getSidebarSessionAccentColor(tone),
    toneClass: getSidebarSessionToneClass(tone),
    active: item.isCurrent,
    archived,
    archiving: !archived && isSessionArchiving(session.id),
    hasWorkflowPlanBadge: shouldShowSessionWorkflowPlanBadge(session),
    hasScheduledPlanExecution: hasScheduledPlanExecution(session),
    hasScheduledInput: shouldHighlightScheduledInputSession(session),
    scheduledInputTitle: t('webSession.scheduledBadge'),
    singleProject: isSingleSidebarProject.value,
    projectBadge: item.projectBadge,
    currentIndicatorTitle: t('terminal.currentActiveSession'),
    activityTimeLabel: formatWebSessionSidebarTime(
      activityTimestamp,
      new Date(sidebarCalendarDayStartMs.value),
      !archived
    ),
    activityTimeTitle: formatWebSessionDateTime(activityTimestamp, locale.value),
    moreActionsLabel: t('common.moreActions'),
  };
}

function getSessionPillSizeClass() {
  const width = tabTitleMaxWidth.value;
  if (width < 60) {
    return 'pill-size-icon-only';
  }
  if (width < 90) {
    return 'pill-size-icon-emoji';
  }
  return 'pill-size-full';
}

function shouldShowSessionStatusDot(session: (typeof sessions.value)[number]) {
  return getSessionDisplayState(session).showStatusDot;
}

function getSessionStatusDotClass(session: (typeof sessions.value)[number]) {
  return getSessionDisplayState(session).statusDotClass ?? session.status;
}

function getSessionTabTone(session: (typeof sessions.value)[number]) {
  const visualInput = getSessionVisualInput(session);
  return visualInput ? getWebSessionTabTone(visualInput) : 'default';
}

function usesSessionApprovalTone(session: (typeof sessions.value)[number]) {
  return getSessionTabTone(session) === 'approval';
}

function usesSessionPlanApprovalTone(session: (typeof sessions.value)[number]) {
  return getSessionTabTone(session) === 'plan_approval';
}

function usesSessionCompletionTone(session: (typeof sessions.value)[number]) {
  return getSessionTabTone(session) === 'completion';
}

function handleTabContextMenu(event: MouseEvent, session: (typeof sessions.value)[number]) {
  event.preventDefault();
  event.stopPropagation();
  contextMenuSession.value = session;
  contextMenuX.value = event.clientX;
  contextMenuY.value = event.clientY;
}

async function handleContextMenuSelect(key: string | number) {
  const session = contextMenuSession.value;
  contextMenuSession.value = null;
  await handleSessionActionSelect(String(key), session);
}

function syncModeLabel(mode: 'fast' | 'deep') {
  return mode === 'deep'
    ? t('settings.webSessionSyncModeDeep')
    : t('settings.webSessionSyncModeFast');
}

function confirmSyncSession(session: WebSessionSummary, mode: 'fast' | 'deep') {
  const clearExisting = ref(false);
  const isClaude = session.agent === 'claude';
  dialog.warning({
    title: t('webSession.syncConfirmTitle'),
    content: () =>
      h('div', { class: 'web-session-close-confirm' }, [
        h('div', { class: 'web-session-close-confirm__message' }, [
          isClaude
            ? t('webSession.syncConfirmContentClaude')
            : t('webSession.syncConfirmContent', { mode: syncModeLabel(mode) }),
        ]),
        h(
          'div',
          { class: 'web-session-close-confirm__checkbox' },
          h(
            NCheckbox,
            {
              checked: clearExisting.value,
              'onUpdate:checked': (value: boolean) => {
                clearExisting.value = value;
              },
            },
            { default: () => t('webSession.syncClearExisting') }
          )
        ),
      ]),
    positiveText: isClaude
      ? t('webSession.syncSessionAction')
      : mode === 'deep'
        ? t('webSession.deepSyncFromTerminal')
        : t('webSession.syncFromTerminal'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => handleSyncSessionSummary(session, mode, clearExisting.value),
  });
}

async function handleSyncSession(
  sessionId: string,
  mode: 'fast' | 'deep' = 'fast',
  clearExisting = false
) {
  const session = visibleSessionById.value.get(sessionId);
  if (!session) {
    return;
  }
  await handleSyncSessionSummary(session, mode, clearExisting);
}

async function handleSyncSessionSummary(
  session: WebSessionSummary,
  mode: 'fast' | 'deep' = 'fast',
  clearExisting = false
) {
  try {
    await webSessionStore.syncSession(session.projectId, session.id, mode, clearExisting, {
      rememberActive: !session.archivedAt,
    });
    syncArchivedPreviewSessionSummary(session.id);
    message.success(
      mode === 'deep' ? t('webSession.deepSyncSuccess') : t('webSession.syncSuccess')
    );
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('webSession.syncFailed'));
  }
}

async function performMobileSessionSelection(
  session: SessionTab,
  action: WebSessionMobileSelectionAction = resolveWebSessionMobileSelectionAction({
    currentProjectId: props.projectId,
    activeArchivedPreviewId: activeArchivedPreviewId.value,
    target: session,
  })
) {
  if (action.type === 'none') {
    return;
  }
  if (action.type === 'select-local') {
    await handleSessionSelect(action.sessionId);
    return;
  }
  if (action.type === 'focus-archived-preview') {
    activeArchivedPreviewId.value = action.sessionId;
    scrollToBottom(true);
    return;
  }
  try {
    if (action.type === 'open-archived-preview') {
      await openArchivedPreviewSession(session);
      return;
    }
    if (action.type === 'navigate-project') {
      if (!session.archivedAt) {
        webSessionStore.setActiveSession(action.projectId, action.sessionId);
      }
      projectStore.addRecentProject(action.projectId);
      await router.push(buildProjectRouteLocation(action.projectId, action.sessionId));
    }
  } catch (error) {
    if (action.type === 'open-archived-preview') {
      clearArchivedPreviewSession();
    }
    message.error(error instanceof Error ? error.message : t('common.error'));
  }
}

function handleMobileTabNewSession() {
  requestMobileViewForBottomNavSelector();
  closeMobileSessionSelector();
  void handleStartDraftSession();
}

function handleMobileTabSessionSelect(session: SessionTab) {
  requestMobileViewForBottomNavSelector();
  closeMobileSessionSelector();
  void performMobileSessionSelection(session);
}

function setupTabSorting() {
  if (isMobile.value) {
    destroyTabSorting();
    return;
  }
  const container = tabsContainerRef.value;
  if (!container || sessions.value.length <= 1) {
    destroyTabSorting();
    return;
  }
  const wrapper = container.querySelector('.n-tabs-wrapper') as HTMLElement | null;
  if (!wrapper) {
    destroyTabSorting();
    return;
  }
  if (tabDragSortable.value) {
    if (tabDragSortable.value.el === wrapper) {
      tabDragSortable.value.option('disabled', sessions.value.length <= 1);
      return;
    }
    destroyTabSorting();
  }
  tabDragSortable.value = Sortable.create(wrapper, {
    animation: 150,
    direction: 'horizontal',
    draggable: '.n-tabs-tab-wrapper',
    handle: '.n-tabs-tab:not(.is-tab-drag-locked)',
    filter: '.n-tabs-tab__close',
    preventOnFilter: false,
    ghostClass: 'web-session-tab-ghost',
    chosenClass: 'web-session-tab-chosen',
    dragClass: 'web-session-tab-dragging',
    onEnd: handleTabDragEnd,
  });
  tabDragSortable.value.option('disabled', sessions.value.length <= 1);
}

function destroyTabSorting() {
  if (tabDragSortable.value) {
    tabDragSortable.value.destroy();
    tabDragSortable.value = null;
  }
}

function handleTabDragEnd(event: SortableEvent) {
  const fromIndex = event.oldDraggableIndex ?? event.oldIndex ?? -1;
  const toIndex = event.newDraggableIndex ?? event.newIndex ?? -1;
  if (fromIndex === -1 || toIndex === -1 || fromIndex === toIndex) {
    return;
  }
  const previousOrderIds = [...tabOrderIds.value];
  const previousMruIds = [...tabMruIds.value];
  const reorderedSessions = [...sessions.value];
  const [movingSession] = reorderedSessions.splice(fromIndex, 1);
  if (!movingSession) {
    return;
  }
  reorderedSessions.splice(toIndex, 0, movingSession);
  replaceTabNavigationState(
    reorderedSessions.map(session => session.id),
    previousMruIds
  );
  if (isArchivedPreviewSession(movingSession)) {
    replaceTabNavigationState(previousOrderIds, previousMruIds);
    nextTick(() => {
      updateActiveTabIndicator();
      syncActiveTabIntoView();
    });
    return;
  }
  if (isDraftSession(movingSession)) {
    nextTick(() => {
      updateActiveTabIndicator();
      syncActiveTabIntoView();
    });
    return;
  }
  const reorderedRealSessions = reorderedSessions.filter(
    session => !isDraftSession(session) && !isArchivedPreviewSession(session)
  );
  const realIndex = reorderedRealSessions.findIndex(session => session.id === movingSession.id);
  const previousRealSessionId = reorderedRealSessions[realIndex - 1]?.id ?? '';
  const nextRealSessionId = reorderedRealSessions[realIndex + 1]?.id ?? '';
  void webSessionStore
    .moveSession(props.projectId, movingSession.id, previousRealSessionId, nextRealSessionId)
    .catch(error => {
      replaceTabNavigationState(previousOrderIds, previousMruIds);
      message.error(error instanceof Error ? error.message : t('common.error'));
    });
  nextTick(() => {
    updateActiveTabIndicator();
    syncActiveTabIntoView();
  });
}

async function loadCodexRuntimeConfig(force = false) {
  const generation = ++runtimeConfigLoadGeneration;
  if (force) {
    clearRuntimeConfigRetry();
    runtimeConfigRetryDelayMs = 500;
  }
  try {
    const config = await webSessionApi.runtimeConfig({ force });
    if (generation !== runtimeConfigLoadGeneration) {
      return;
    }
    codexRuntimeConfig.value = config;
    if (!force && config.capabilitiesRefreshing) {
      scheduleRuntimeConfigRetry();
    } else {
      clearRuntimeConfigRetry();
      runtimeConfigRetryDelayMs = 500;
    }
  } catch (error) {
    if (generation !== runtimeConfigLoadGeneration) {
      return;
    }
    codexRuntimeConfig.value = null;
    console.warn('[Web Session] Failed to load runtime config', error);
  } finally {
    if (generation === runtimeConfigLoadGeneration) {
      codexRuntimeConfigReady.value = true;
    }
  }
}

function scheduleRuntimeConfigRetry() {
  clearRuntimeConfigRetry();
  runtimeConfigRetryTimer = window.setTimeout(() => {
    runtimeConfigRetryTimer = null;
    void loadCodexRuntimeConfig();
  }, runtimeConfigRetryDelayMs);
  runtimeConfigRetryDelayMs = Math.min(runtimeConfigRetryDelayMs * 2, 5_000);
}

function clearRuntimeConfigRetry() {
  if (runtimeConfigRetryTimer != null) {
    window.clearTimeout(runtimeConfigRetryTimer);
    runtimeConfigRetryTimer = null;
  }
}

watch(
  () => props.projectId,
  projectId => {
    clearSendConflictConfirmation();
    piTrustServerProjectPath.value = '';
    pendingPiAgentSelection.value = false;
    showPiTrustDialog.value = false;
    if (projectId) {
      void initializeProjectSessions(projectId);
    } else {
      projectSessionInitializationGate.invalidate();
      isProjectSessionInitializing.value = false;
    }
  },
  { immediate: true }
);

watch(
  () => {
    const listedProject = projectStore.projects.find(project => project.id === props.projectId);
    return [
      props.projectId,
      projectStore.currentProject?.id ?? '',
      projectStore.currentProject?.name ?? '',
      projectStore.currentProject?.path ?? '',
      listedProject?.name ?? '',
      listedProject?.path ?? '',
      projectStore.projectDetailLoading,
      projectStore.worktrees
        .map(worktree => `${worktree.projectId}:${worktree.id}:${worktree.path}`)
        .join('|'),
    ] as const;
  },
  () => {
    reconcileDraftSessionProjectScope();
  },
  { immediate: true }
);

watch(
  () => routeWebSessionId.value,
  sessionId => {
    if (!sessionId) {
      pendingRouteActivationSessionId.value = '';
      return;
    }
    if (!props.projectId) {
      return;
    }
    if (isProjectSessionInitializing.value) {
      pendingRouteActivationSessionId.value = sessionId;
      return;
    }
    const session = currentSession.value;
    if (
      session &&
      !isDraftSession(session) &&
      session.id === sessionId &&
      session.projectId === props.projectId
    ) {
      pendingRouteActivationSessionId.value = '';
      return;
    }
    pendingRouteActivationSessionId.value = sessionId;
    void activateSessionFromRoute(props.projectId, sessionId).catch(error => {
      console.error('[Web Session] Failed to activate session from route', error);
    });
  }
);

watch([sendConfirmationSignature, planImplementConfirmationSignature], signatures => {
  if (!sendConfirmationState.value) {
    return;
  }
  const activeSignatures = signatures.filter(Boolean);
  if (!activeSignatures.includes(sendConfirmationState.value.signature)) {
    clearSendConflictConfirmation();
  }
});

watch([canOpenSendQuickActions, isRunActive, () => currentRealSession.value?.id ?? ''], values => {
  if (!showSendQuickActions.value) {
    return;
  }
  if (!values[0]) {
    closeSendQuickActions();
  }
});

watch(
  [isSubmittingMessage, () => currentRealSession.value?.id ?? '', latestPlanItemId],
  ([submitting, sessionId, planItemId]) => {
    if (showPlanQuickActions.value && (submitting || !showPlanActions(latestPlanToolId.value))) {
      closePlanQuickActions();
    }
    const target = scheduledPlanDialogTarget.value;
    if (
      showScheduledSendDialog.value &&
      scheduledSendPurpose.value === 'execute_plan' &&
      target &&
      (target.sessionId !== sessionId || target.planItemId !== planItemId)
    ) {
      handleScheduledSendDialogVisibilityChange(false);
    }
  }
);

watch(
  () => currentRealSession.value?.id ?? '',
  (sessionId, previousSessionId) => {
    if (sessionId === previousSessionId) {
      return;
    }
    resetTimelineEdgeWindow();
    clearPendingEditState({ clearDraft: false });
    clearLocalFileDialog();
    closeScheduledInputPopover();
    if (showScheduledSendDialog.value && isScheduledDialogEdit.value) {
      handleScheduledSendDialogVisibilityChange(false);
    }
    nextTick(restorePendingEditForCurrentSession);
  }
);

watch(
  () =>
    pendingInputs.value
      .map(
        item =>
          `${item.id}:${item.paused ? 'paused' : 'active'}:${item.nativeQueued ? 'native' : ''}`
      )
      .join('|'),
  () => {
    if (
      pendingEditingId.value &&
      !pendingInputs.value.some(item => item.id === pendingEditingId.value)
    ) {
      clearPendingEditState();
    }
    restorePendingEditForCurrentSession();
    const session = currentRealSession.value;
    if (!session || pendingInputs.value.length === 0) {
      return;
    }
    const pendingIdSet = new Set(pendingInputs.value.map(item => item.id));
    Object.keys(webSessionStore.getPendingInputEditDrafts(session.projectId, session.id))
      .filter(pendingId => !pendingIdSet.has(pendingId))
      .forEach(pendingId => {
        webSessionStore.clearPendingInputEditDraft(session.projectId, session.id, pendingId);
      });
  },
  { immediate: true }
);

watch(
  () => scheduledInputs.value.map(item => `${item.id}:${item.status}`).join('|'),
  () => {
    if (
      activeScheduledInputPopoverId.value &&
      !scheduledInputs.value.some(item => item.id === activeScheduledInputPopoverId.value)
    ) {
      closeScheduledInputPopover();
    }
    const editing = scheduledEditingInput.value;
    const current = editing
      ? scheduledInputs.value.find(item => item.id === editing.id)
      : undefined;
    if (
      showScheduledSendDialog.value &&
      isScheduledDialogEdit.value &&
      (!current || (current.status !== 'scheduled' && current.status !== 'failed'))
    ) {
      handleScheduledSendDialogVisibilityChange(false);
    }
  }
);

watch(
  () => sessions.value.map(session => session.id).join('|'),
  () => {
    if (isProjectSessionInitializing.value) {
      return;
    }
    syncTabNavigationState();
  }
);

watch(
  () => activeTabSessionId.value,
  sessionId => {
    if (
      !sessionId ||
      isProjectSessionInitializing.value ||
      !sessions.value.some(session => session.id === sessionId) ||
      tabMruIds.value[0] === sessionId
    ) {
      return;
    }
    rememberTabVisit(sessionId);
  }
);

watch(
  sidebarVisibleProjectIds,
  projectIds => {
    projectIds.forEach(projectId => {
      if (!projectId || loadedSidebarProjectIds.has(projectId)) {
        return;
      }
      loadedSidebarProjectIds.add(projectId);
      void webSessionStore.loadSessions(projectId).catch(error => {
        loadedSidebarProjectIds.delete(projectId);
        console.error('[Web Session] Failed to preload sidebar sessions', projectId, error);
      });
    });
  },
  { immediate: true }
);

watch(
  [
    normalizedSidebarSearchQuery,
    sidebarSearchArchived,
    sidebarSearchBody,
    () => sidebarVisibleProjectIds.value.join('|'),
  ],
  () => {
    const searchActive = normalizedSidebarSearchQuery.value.length > 0;
    clearSidebarSearchState(searchActive);
    if (searchActive) {
      scheduleSidebarSearch();
    }
  }
);

watch(
  sidebarVisibleProjectIds,
  projectIds => {
    void ensureArchivedScopeLoaded(projectIds, 20).catch(error => {
      console.error('[Web Session] Failed to preload archived sidebar sessions', error);
    });
  },
  { immediate: true }
);

watch(
  [() => props.projectId, () => currentSession.value?.id ?? ''],
  ([projectId, sessionId], previous) => {
    const [previousProjectId, previousSessionId] = previous ?? ['', ''];
    if (
      previousProjectId &&
      previousSessionId &&
      (previousProjectId !== projectId || previousSessionId !== sessionId)
    ) {
      captureTimelinePosition(previousProjectId, previousSessionId, true);
    }
    cancelTimelinePositionRestore();
    stopWebSessionCatchUp('session-change');
    streamingMarkdownController.clear();
    pendingHistoryAnchor.value = null;
    userMessageNavigationPending.value = null;
    timelineUserMessageElements.clear();
    timelineBlockElements.clear();
    resetTimelineSearchForSessionChange();
    handleCommandExecutionDetailVisibilityChange(false);
    rawTimelineBlocks.value = {};
    activeRawTimelineBlockKey.value = '';
    syncMobileSessionCategoryToCurrentSession();
    if (!sessionId) {
      closeMobileSessionSelector();
      return;
    }
    const session = currentSession.value;
    if (!session) {
      return;
    }
    draftAgent.value = session.agent;
    draftClaudeRuntime.value = session.claudeRuntime === 'ccr' ? 'ccr' : 'claude';
    draftModel.value = session.model || defaultModelForAgent(session.agent);
    draftReasoningEffort.value =
      session.reasoningEffort || defaultReasoningEffortForAgent(session.agent);
    draftWorkflowMode.value = session.workflowMode;
    draftPermissionLevel.value = session.permissionLevel;
    if (timelineSearchOpen.value && normalizedTimelineSearchQuery.value) {
      scheduleTimelineSearchRequest();
    }
    expandedTools.value = {};
    beginTimelinePositionRestore(projectId, sessionId);
    updateActiveTabIndicator();
    syncActiveTabIntoView();
    if (!isDraftSession(session)) {
      markSessionViewed(session.id);
    }
  },
  { immediate: true }
);

watch(
  [() => props.isActive, () => currentRealSession.value?.id ?? ''],
  ([isActive, sessionId]) => {
    webSessionStore.setEventSessionFocus(isActive ? sessionId : '');
  },
  { immediate: true }
);

watch(
  visibleRawTimelineBlockKeys,
  keys => {
    activeRawTimelineBlockKey.value = pruneActiveTimelineRawBlockKey(
      activeRawTimelineBlockKey.value,
      keys
    );
  },
  { immediate: true }
);

watch(
  streamingMarkdownTargets,
  targets => {
    streamingMarkdownController.sync(targets);
  },
  { immediate: true, deep: true }
);

watch(
  () => webSessionStreamingMarkdownThrottleMs.value,
  value => {
    streamingMarkdownController.setDelayMs(value);
  },
  { immediate: true }
);

useEventListener(typeof document !== 'undefined' ? document : undefined, 'pointerdown', event => {
  const target = event.target;
  const clickedInsideRawCard =
    target instanceof Element && Boolean(target.closest('[data-raw-toggle-card]'));
  if (shouldClearActiveTimelineRawBlockKey(activeRawTimelineBlockKey.value, clickedInsideRawCard)) {
    activeRawTimelineBlockKey.value = '';
  }
});

watch(
  [() => currentSession.value, routeWorkspaceTab, routeWebSessionId],
  ([session, workspaceTab]) => {
    const sessionIsDraft = Boolean(session && isDraftSession(session));
    if (
      shouldPreserveWebSessionRouteSessionId({
        workspaceTab,
        pendingRouteSessionId: pendingRouteActivationSessionId.value,
        currentProjectId: props.projectId,
        currentSessionId: session?.id,
        currentSessionProjectId: session && !sessionIsDraft ? session.projectId : '',
        currentSessionIsDraft: sessionIsDraft,
      })
    ) {
      return;
    }
    const nextRouteSessionId =
      workspaceTab === 'web' && session && !sessionIsDraft && session.projectId === props.projectId
        ? session.id
        : '';
    void syncWebSessionRouteSessionId(nextRouteSessionId).catch(error => {
      console.error('[Web Session] Failed to sync route session id', error);
    });
  },
  { immediate: true }
);

watch(
  () => props.isActive,
  active => {
    if (!active) {
      return;
    }
    markSessionViewed(currentRealSession.value?.id);
  },
  { immediate: true }
);

watch(currentSessionLatestEventSeq, () => {
  markSessionViewed(currentRealSession.value?.id);
});

watch(
  [
    () => webSessionAutoContinueScope.value,
    () => webSessionAutoContinuePreset.value,
    () => webSessionAutoContinueMaxAttempts.value,
  ],
  ([scope, preset, maxAttempts]) => {
    if (
      !isDraftSession(currentSession.value) ||
      currentSession.value.autoRetryPolicyMode === 'custom'
    ) {
      return;
    }
    if (
      currentSession.value.autoRetryScope === scope &&
      currentSession.value.autoRetryPreset === preset &&
      currentSession.value.autoRetryMaxAttempts === maxAttempts
    ) {
      return;
    }
    updateActiveDraftSession(current => ({
      ...current,
      autoRetryScope: scope,
      autoRetryPreset: preset,
      autoRetryMaxAttempts: maxAttempts,
      updatedAt: new Date().toISOString(),
    }));
  }
);

watch(
  () =>
    sessions.value
      .map(
        session =>
          `${session.id}:${session.orderIndex}:${session.status}:${session.hasUnread}:${getSessionLabelState(session)}:${getSessionPillStateClass(session)}`
      )
      .join('|'),
  () => {
    nextTick(() => {
      recalcTabTitleWidth();
      updateActiveTabIndicator();
      setupTabScrollListener();
      syncActiveTabIntoView();
      if (isMobile.value) {
        destroyTabSorting();
      } else {
        refreshTabSortable();
      }
    });
  },
  { immediate: true }
);

watch(
  () => isMobile.value,
  mobile => {
    if (mobile) {
      closeMobileSessionSelector();
      cleanupTabScrollListener();
      destroyTabSorting();
      activeTabIndicatorStyle.value = hiddenCardTabIndicatorStyle();
      return;
    }
    nextTick(() => {
      setupTabScrollListener();
      refreshTabSortable();
      updateActiveTabIndicator();
      syncActiveTabIntoView();
    });
  },
  { immediate: true }
);

watch(timelineContentVersion, async () => {
  await nextTick();
  refreshTimelineViewportNavigation();
  if (pendingTimelinePositionRestore.value) {
    void restorePendingTimelinePosition();
    return;
  }
  if (restoreHistoryAnchor()) {
    markSessionViewed(currentRealSession.value?.id);
    ensureTimelineHistoryFilled();
    refreshTimelineViewportNavigation();
    return;
  }
  const container = timelineScrollRef.value;
  if (!container) {
    return;
  }
  if (autoFollowBottom.value) {
    scheduleScrollToBottom();
  } else {
    updateBottomState(container);
  }
  markSessionViewed(currentRealSession.value?.id);
  ensureTimelineHistoryFilled();
  refreshTimelineViewportNavigation();
});

watch(
  () =>
    currentRealSession.value
      ? `${currentRealSession.value.id}:${historyMeta.value.loading ? 1 : 0}:${historyMeta.value.beforeCursor}`
      : '',
  () => {
    if (pendingTimelinePositionRestore.value && !historyMeta.value.loading) {
      void restorePendingTimelinePosition();
    }
  }
);

watch(currentDraftSessionId, () => {
  clearComposerTransferError();
  clearSendConflictConfirmation();
  showQuickInputPopover.value = false;
});

watch(
  () => currentSession.value?.id,
  (sessionId, previousSessionId) => {
    showPiTreeDrawer.value = false;
    showQuickInputPopover.value = false;
    resetMobileComposerScrollState();
    if (isMobile.value) {
      isMobileComposerSettingsExpanded.value = false;
    }
    if (previousSessionId && previousSessionId !== sessionId) {
      webSessionStore.trimInactiveSessionEvents(sessionId || '');
    }
  }
);

watch(
  () => isMobile.value,
  mobile => {
    hideTimelineNavigationControls();
    isMobileComposerSettingsExpanded.value = false;
    resetMobileComposerScrollState();
    if (!mobile) {
      mobileKeyboard.reset();
      setMobileComposerFocusState(false);
    }
  },
  { immediate: true }
);

watch(
  () => props.isActive,
  active => {
    if (!active) {
      if (!pendingTimelinePositionRestore.value) {
        captureTimelinePosition(props.projectId, currentSession.value?.id ?? '', true);
      }
      webSessionStore.trimInactiveSessionEvents(currentRealSession.value?.id || '');
      mobileKeyboard.reset();
      setMobileComposerFocusState(false);
      return;
    }
    refreshTabHeaderLayout();
    if (currentSession.value?.id) {
      beginTimelinePositionRestore(props.projectId, currentSession.value.id);
    }
    if (!isDocumentVisible() || !currentRealSession.value?.id) {
      return;
    }
    scheduleWebSessionCatchUp('panel-active');
  }
);

watch(
  () => webSessionStore.eventRecoveryVersion,
  version => {
    if (version <= 0 || !props.isActive || !isDocumentVisible()) {
      return;
    }
    scheduleWebSessionCatchUp('event-stream-recovered');
  }
);

useResizeObserver(timelineListRef, () => {
  if (!currentSession.value) {
    return;
  }
  if (pendingTimelinePositionRestore.value) {
    void restorePendingTimelinePosition();
    return;
  }
  scheduleScrollToBottom();
});

useResizeObserver(timelineScrollRef, entries => {
  const container = entries[0]?.target as HTMLDivElement | undefined;
  if (!container || !currentSession.value) {
    return;
  }
  if (pendingTimelinePositionRestore.value) {
    void restorePendingTimelinePosition();
    return;
  }
  if (autoFollowBottom.value) {
    scheduleScrollToBottom();
  } else {
    updateBottomState(container);
  }
});

watch(
  () => selectedAgent.value,
  value => {
    if (!draftModel.value || (value === 'claude' && draftModel.value.startsWith('gpt-'))) {
      draftModel.value = defaultModelForAgent(value);
    }
    if (value === 'codex' && !draftModel.value.startsWith('gpt-')) {
      draftModel.value = defaultModelForAgent(value);
    }
  }
);

useResizeObserver(tabsContainerRef, entries => {
  const entry = entries[0];
  if (!entry) {
    return;
  }
  const width = entry.contentRect.width;
  if (width !== tabsContainerWidth.value) {
    recalcTabTitleWidth(width);
    updateActiveTabIndicator();
    syncActiveTabIntoView();
  }
});

onMounted(() => {
  liveStateClockTimer = window.setInterval(() => {
    liveStateClockMs.value = Date.now();
  }, LIVE_TIME_TICK_MS);
  void settingsStore.loadWebSessionQuickInput();
  void loadComposerDeveloperConfig();
  void loadCodexRuntimeConfig();
  void ensureCodexSkillsLoaded();
  if (projectStore.projects.length === 0) {
    void projectStore.fetchProjects({ silent: true }).catch(error => {
      console.error('[Web Session] Failed to preload projects', error);
    });
  }
  window.addEventListener('focus', handleWebSessionWindowFocus);
  window.addEventListener('pageshow', handleWebSessionWindowPageShow);
  window.addEventListener('pagehide', handleTimelinePositionPageHide);
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleWebSessionDocumentVisibilityChange);
  }
  nextTick(() => {
    recalcTabTitleWidth();
    setupTabScrollListener();
    updateActiveTabIndicator();
    syncActiveTabIntoView();
    if (currentSession.value) {
      beginTimelinePositionRestore(props.projectId, currentSession.value.id);
    }
  });
});

onBeforeUnmount(() => {
  clearRuntimeConfigRetry();
  clearComposerSelectorHoverCloseTimer('model');
  clearComposerSelectorHoverCloseTimer('reasoning');
  if (!pendingTimelinePositionRestore.value) {
    captureTimelinePosition(props.projectId, currentSession.value?.id ?? '', true);
  }
  persistTimelinePositionStateNow();
  cancelTimelinePositionRestore();
  cancelSidebarSearchRequest();
  hideTimelineNavigationControls();
  persistPendingEditDraft();
  persistActiveUserInputDraft();
  realSessionSnapshotLoadController.cancel();
  streamingMarkdownController.clear();
  timelineUserMessageElements.clear();
  timelineBlockElements.clear();
  userMessageNavigationPending.value = null;
  if (liveStateClockTimer != null) {
    window.clearInterval(liveStateClockTimer);
    liveStateClockTimer = null;
  }
  clearUserInputSlowHintTimer();
  userInputSubmitStateByOwnerId.value = {};
  userInputSlowStateByOwnerId.value = {};
  mobileKeyboard.reset();
  setMobileComposerFocusState(false);
  emitMobileComposerChromeHidden(false);
  clearComposerTransferError();
  clearSendConflictConfirmation();
  closeSendQuickActions();
  sendQuickActionLongPress.pointerCancel();
  stopWebSessionCatchUp('unmount');
  resetComposerDragState();
  cleanupTabScrollListener();
  destroyTabSorting();
  window.removeEventListener('focus', handleWebSessionWindowFocus);
  window.removeEventListener('pageshow', handleWebSessionWindowPageShow);
  window.removeEventListener('pagehide', handleTimelinePositionPageHide);
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleWebSessionDocumentVisibilityChange);
  }
});

defineExpose({
  closeMobileSessionSelector,
  openMobileSessionSelectorFromElement,
});
</script>

<style scoped src="./styles/webSessionPanelLayout.css"></style>
<style scoped src="./styles/webSessionPanelTimeline.css"></style>
<style scoped src="./styles/webSessionPanelComposer.css"></style>
<style scoped src="./styles/webSessionPanelResponsive.css"></style>
