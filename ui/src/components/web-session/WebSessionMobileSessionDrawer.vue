<template>
  <n-drawer
    class="mobile-session-drawer"
    placement="bottom"
    height="min(72dvh, 560px)"
    :show="show"
    @update:show="emit('update:show', $event)"
  >
    <n-drawer-content
      :title="t('webSession.switchSession')"
      closable
      :native-scrollbar="false"
      body-content-style="padding: 0;"
      footer-style="padding: 0;"
    >
      <div class="mobile-session-drawer-body">
        <div class="mobile-session-drawer-categories" role="tablist">
          <button
            type="button"
            class="mobile-session-drawer-category"
            :class="{ 'is-active': category === 'current' }"
            role="tab"
            :aria-selected="category === 'current'"
            @click="emit('select-category', 'current')"
          >
            <span>{{ t('webSession.currentSessions') }}</span>
            <span class="mobile-session-drawer-category-count">{{ currentCount }}</span>
          </button>
          <button
            type="button"
            class="mobile-session-drawer-category"
            :class="{ 'is-active': category === 'archived' }"
            role="tab"
            :aria-selected="category === 'archived'"
            @click="emit('select-category', 'archived')"
          >
            <span>{{ t('webSession.archivedSessions') }}</span>
            <span class="mobile-session-drawer-category-count">{{ archivedTotal }}</span>
          </button>
        </div>

        <n-virtual-list
          class="mobile-session-drawer-list"
          :items="items"
          :item-size="MOBILE_TAB_VIRTUAL_ITEM_SIZE"
          item-resizable
          key-field="key"
        >
          <template #default="{ item }">
            <div v-if="item.kind === 'date-group'" class="mobile-session-drawer-date-group">
              <span class="mobile-session-drawer-date-group-label">
                <span>{{ item.label }}</span>
                <span class="mobile-session-drawer-date-group-count">{{ item.count }}</span>
              </span>
              <button
                type="button"
                class="mobile-session-drawer-date-group-toggle"
                :title="sessionGroupToggleLabel(item.label, item.collapsed)"
                :aria-label="sessionGroupToggleLabel(item.label, item.collapsed)"
                :aria-expanded="!item.collapsed"
                @click.stop="emit('toggle-group', item.groupKey)"
              >
                <n-icon size="14" aria-hidden="true">
                  <ChevronForwardOutline v-if="item.collapsed" />
                  <ChevronDownOutline v-else />
                </n-icon>
              </button>
            </div>
            <div v-else-if="item.kind === 'session'" class="mobile-session-drawer-item-shell">
              <button
                type="button"
                class="mobile-session-drawer-item"
                :class="viewFor(item.session).rowClass"
                :title="viewFor(item.session).statusTooltip"
                @click="emit('select-session', item.session)"
              >
                <span class="mobile-session-drawer-agent-shell">
                  <span
                    class="mobile-session-drawer-agent-badge"
                    :class="viewFor(item.session).agentBadgeClass"
                  >
                    <span
                      class="mobile-session-drawer-agent-icon"
                      v-html="viewFor(item.session).assistantIcon"
                    ></span>
                  </span>
                  <span
                    v-if="viewFor(item.session).showStatusDot"
                    class="status-dot mobile-session-drawer-status-dot"
                    :class="viewFor(item.session).statusDotClass"
                  ></span>
                  <n-icon
                    v-if="viewFor(item.session).showWorkflowPlanBadge"
                    class="mobile-session-drawer-plan-badge"
                    :class="{ 'is-scheduled': viewFor(item.session).scheduledPlan }"
                    size="9"
                    aria-hidden="true"
                  >
                    <FlagIcon />
                  </n-icon>
                </span>
                <span class="mobile-session-drawer-item-title">{{ item.session.title }}</span>
                <n-icon
                  v-if="viewFor(item.session).showScheduledInput"
                  class="mobile-session-drawer-scheduled-marker"
                  size="11"
                  :title="t('webSession.scheduledBadge')"
                  :aria-label="t('webSession.scheduledBadge')"
                >
                  <TimeOutline />
                </n-icon>
                <span class="mobile-session-drawer-trailing">
                  <span
                    v-if="viewFor(item.session).projectBadge"
                    class="mobile-session-drawer-project-badge"
                    :style="{ '--badge-color': viewFor(item.session).projectBadge?.color }"
                    :title="viewFor(item.session).projectName"
                  >
                    {{ viewFor(item.session).projectBadge?.label }}
                  </span>
                  <time
                    class="mobile-session-drawer-time"
                    :datetime="item.session.updatedAt"
                    :title="viewFor(item.session).timeTitle"
                  >
                    {{ viewFor(item.session).timeLabel }}
                  </time>
                </span>
              </button>
            </div>
            <div v-else-if="item.kind === 'load-more'" class="mobile-session-drawer-row">
              <button
                type="button"
                class="mobile-session-drawer-load-more"
                :disabled="item.loading"
                @click="emit('load-more')"
              >
                {{ item.loading ? t('common.loading') : t('webSession.loadMoreArchived') }}
              </button>
            </div>
            <div v-else class="mobile-session-drawer-empty">
              {{ emptyLabel }}
            </div>
          </template>
        </n-virtual-list>
      </div>

      <template #footer>
        <div class="mobile-session-drawer-footer">
          <button
            type="button"
            class="mobile-session-drawer-scope"
            :class="{ 'is-current': currentProjectScope }"
            :title="scopeToggleTitle"
            :aria-label="scopeToggleTitle"
            :aria-pressed="currentProjectScope"
            @click="emit('toggle-scope')"
          >
            <span class="mobile-session-drawer-scope-icon" aria-hidden="true">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
                <rect
                  x="4.5"
                  y="5"
                  width="9"
                  height="6"
                  rx="1.5"
                  stroke="currentColor"
                  stroke-width="1.8"
                />
                <rect
                  x="10.5"
                  y="13"
                  width="9"
                  height="6"
                  rx="1.5"
                  stroke="currentColor"
                  stroke-width="1.8"
                  opacity="0.92"
                />
              </svg>
            </span>
            <span class="mobile-session-drawer-scope-label">{{ scopeLabel }}</span>
          </button>
          <button
            type="button"
            class="mobile-session-drawer-new-session"
            :title="t('webSession.newSession')"
            :aria-label="t('webSession.newSession')"
            @click="emit('new-session')"
          >
            <n-icon size="17" aria-hidden="true"><AddOutline /></n-icon>
            <span>{{ t('webSession.newSession') }}</span>
          </button>
        </div>
      </template>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import {
  AddOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  Flag as FlagIcon,
  TimeOutline,
} from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import type {
  MobileSessionDrawerView,
  MobileTabListDescriptor,
  SessionTab,
} from '@/components/web-session/webSessionPanelSession';

const MOBILE_TAB_VIRTUAL_ITEM_SIZE = 52;

const props = defineProps<{
  show: boolean;
  category: 'current' | 'archived';
  currentCount: number;
  archivedTotal: number;
  archivedLoading: boolean;
  items: MobileTabListDescriptor[];
  sessionViews: ReadonlyMap<string, MobileSessionDrawerView>;
  currentProjectScope: boolean;
  scopeToggleTitle: string;
}>();

const emit = defineEmits<{
  (event: 'update:show', show: boolean): void;
  (event: 'select-category', category: 'current' | 'archived'): void;
  (event: 'toggle-group', groupKey: string): void;
  (event: 'select-session', session: SessionTab): void;
  (event: 'load-more'): void;
  (event: 'toggle-scope'): void;
  (event: 'new-session'): void;
}>();

const { t } = useLocale();
const scopeLabel = computed(() =>
  props.currentProjectScope
    ? t('webSession.currentProjectSessions')
    : t('webSession.crossProjectSessions')
);
const emptyLabel = computed(() => {
  if (props.category === 'archived') {
    return props.archivedLoading ? t('common.loading') : t('webSession.archivedSessionsEmpty');
  }
  return t('webSession.currentSessionsEmpty');
});

function sessionGroupToggleLabel(groupLabel: string, collapsed: boolean) {
  return t(collapsed ? 'webSession.expandSessionGroup' : 'webSession.collapseSessionGroup', {
    group: groupLabel,
  });
}

function viewFor(session: SessionTab) {
  const view = props.sessionViews.get(session.id);
  if (!view) {
    throw new Error(`Missing mobile session drawer view for ${session.id}`);
  }
  return view;
}
</script>

<style scoped src="./styles/webSessionMobileSessionDrawer.css"></style>
