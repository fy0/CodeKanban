<template>
  <div ref="rootElement" class="session-sidebar-shell">
    <div
      class="terminal-resizer"
      :class="{ 'is-dragging': resizing }"
      @mousedown="emit('start-resize', $event)"
    >
      <div class="resizer-handle"></div>
    </div>

    <aside class="session-sidebar" :style="{ width: `${width}px` }">
      <div class="session-sidebar-header">
        <div class="session-sidebar-title-wrap">
          <SplitDropdownControl
            class="session-sidebar-scope-control"
            :label="scopeLabel"
            :options="scopeOptions"
            :title="scopeAriaLabel"
            :menu-title="scopeAriaLabel"
            :aria-label="scopeAriaLabel"
            flat
            @main-click="emit('toggle-scope')"
            @select="emit('select-scope', $event)"
          >
            <template #prefix>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
                <path
                  d="M4 7h16M4 12h10M4 17h8"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            </template>
          </SplitDropdownControl>
        </div>
        <div class="session-sidebar-header-actions">
          <span class="session-sidebar-count">{{ visibleSessionCount }}</span>
          <n-tooltip placement="bottom" :delay="250">
            <template #trigger>
              <button
                type="button"
                class="session-sidebar-reset"
                :aria-label="t('common.reset')"
                @click="emit('reset-width')"
              >
                <n-icon size="14"><RefreshOutline /></n-icon>
              </button>
            </template>
            {{ t('common.reset') }}
          </n-tooltip>
        </div>
      </div>

      <div class="session-sidebar-search-row">
        <n-input
          :value="searchQuery"
          class="session-sidebar-search-input"
          size="small"
          clearable
          :placeholder="t('webSession.sidebarSearchPlaceholder')"
          :aria-label="t('webSession.sidebarSearchPlaceholder')"
          @update:value="emit('update:search-query', $event)"
        >
          <template #prefix>
            <n-icon size="15" aria-hidden="true"><SearchOutline /></n-icon>
          </template>
        </n-input>
        <n-popover trigger="click" placement="bottom-end" :show-arrow="false">
          <template #trigger>
            <n-button
              secondary
              size="small"
              class="session-sidebar-filter-button"
              :class="{ 'is-active': searchArchived || !searchBody }"
              :title="t('webSession.sidebarSearchFilter')"
              :aria-label="t('webSession.sidebarSearchFilter')"
              :aria-pressed="searchArchived || !searchBody"
            >
              <template #icon>
                <n-icon size="16"><FunnelOutline /></n-icon>
              </template>
            </n-button>
          </template>
          <div class="session-sidebar-filter-popover">
            <n-checkbox :checked="searchBody" @update:checked="emit('update:search-body', $event)">
              {{ t('webSession.sidebarSearchBody') }}
            </n-checkbox>
            <n-checkbox
              :checked="searchArchived"
              @update:checked="emit('update:search-archived', $event)"
            >
              {{ t('webSession.sidebarSearchArchived') }}
            </n-checkbox>
          </div>
        </n-popover>
      </div>

      <div
        v-if="searchProgressVisible"
        class="session-sidebar-search-progress"
        role="progressbar"
        :aria-label="t('webSession.sidebarSearchProgress')"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-valuenow="searchProgressPercentage"
      >
        <span :style="{ width: `${searchProgressPercentage}%` }"></span>
      </div>

      <div v-if="empty" class="session-sidebar-empty">
        {{ t('webSession.emptyTitle') }}
      </div>
      <div v-else-if="searchError" class="session-sidebar-empty">
        {{ t('webSession.sidebarSearchFailed') }}
      </div>
      <div v-else-if="noSearchResults" class="session-sidebar-empty">
        {{ t('webSession.sidebarSearchNoResults') }}
      </div>

      <n-virtual-list
        v-else
        class="session-sidebar-list"
        :items="items"
        :item-size="WEB_SESSION_SIDEBAR_VIRTUAL_ITEM_SIZE"
        item-resizable
        key-field="key"
      >
        <template #default="{ item }">
          <div
            v-if="item.type === 'section'"
            class="session-sidebar-virtual-section"
            :class="{ 'is-separated': item.separated }"
          >
            <span class="session-sidebar-section-label">
              <span>{{ item.label }}</span>
              <span>({{ item.count }})</span>
            </span>
            <button
              v-if="!searchActive"
              type="button"
              class="session-sidebar-section-toggle"
              :title="sessionGroupToggleLabel(item.label, item.collapsed)"
              :aria-label="sessionGroupToggleLabel(item.label, item.collapsed)"
              :aria-expanded="!item.collapsed"
              @click.stop="emit('toggle-group', item.sectionKey)"
            >
              <n-icon size="13" aria-hidden="true">
                <ChevronForwardOutline v-if="item.collapsed" />
                <ChevronDownOutline v-else />
              </n-icon>
            </button>
          </div>
          <div v-else-if="item.type === 'empty'" class="session-sidebar-virtual-row">
            <div class="session-sidebar-section-empty">{{ item.label }}</div>
          </div>
          <div v-else-if="item.type === 'session'" class="session-sidebar-virtual-row">
            <WebSessionSidebarRow
              :row="item.entry.row"
              :action-options="getActionOptions(item.entry.source.session)"
              @select="emit('select-session', item)"
              @action="emit('session-action', $event, item.entry.source)"
            />
          </div>
          <div v-else class="session-sidebar-virtual-row">
            <button
              type="button"
              class="session-sidebar-load-more"
              :disabled="item.disabled"
              @click="emit('load-more')"
            >
              {{ item.label }}
            </button>
          </div>
        </template>
      </n-virtual-list>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import type { DropdownOption } from 'naive-ui';
import {
  ChevronDownOutline,
  ChevronForwardOutline,
  FunnelOutline,
  RefreshOutline,
  SearchOutline,
} from '@vicons/ionicons5';
import SplitDropdownControl from '@/components/common/SplitDropdownControl.vue';
import WebSessionSidebarRow from '@/components/web-session/WebSessionSidebarRow.vue';
import {
  WEB_SESSION_SIDEBAR_VIRTUAL_ITEM_SIZE,
  type WebSessionSidebarVirtualItem,
} from '@/components/web-session/webSessionSidebarVirtualList';
import type { CrossProjectSessionItem } from '@/components/web-session/webSessionSidebarView';
import { useLocale } from '@/composables/useLocale';
import type { WebSessionSummary } from '@/types/models';

defineProps<{
  resizing: boolean;
  width: number;
  scopeLabel: string;
  scopeOptions: DropdownOption[];
  scopeAriaLabel: string;
  visibleSessionCount: number;
  searchQuery: string;
  searchArchived: boolean;
  searchBody: boolean;
  searchProgressVisible: boolean;
  searchProgressPercentage: number;
  empty: boolean;
  searchError: boolean;
  noSearchResults: boolean;
  searchActive: boolean;
  items: WebSessionSidebarVirtualItem<CrossProjectSessionItem>[];
  getActionOptions: (session: WebSessionSummary) => DropdownOption[];
}>();

const emit = defineEmits<{
  (event: 'start-resize', mouseEvent: MouseEvent): void;
  (event: 'toggle-scope'): void;
  (event: 'select-scope', key: string | number): void;
  (event: 'reset-width'): void;
  (event: 'update:search-query', query: string): void;
  (event: 'update:search-archived', archived: boolean): void;
  (event: 'update:search-body', includeBody: boolean): void;
  (event: 'toggle-group', groupKey: string): void;
  (event: 'select-session', item: WebSessionSidebarVirtualItem<CrossProjectSessionItem>): void;
  (event: 'session-action', key: string | number, item: CrossProjectSessionItem): void;
  (event: 'load-more'): void;
}>();

const rootElement = ref<HTMLElement | null>(null);
const { t } = useLocale();

function sessionGroupToggleLabel(label: string, collapsed: boolean) {
  return collapsed
    ? t('webSession.sessionGroupExpand', { label })
    : t('webSession.sessionGroupCollapse', { label });
}

defineExpose({ rootElement });
</script>

<style scoped src="./styles/webSessionSidebar.css"></style>
