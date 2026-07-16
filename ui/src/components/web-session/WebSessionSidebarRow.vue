<template>
  <div
    class="session-sidebar-item session-sidebar-row"
    :class="[
      row.toneClass,
      {
        'has-workflow-plan-badge': row.hasWorkflowPlanBadge,
        'is-archiving': row.archiving,
        'is-archived': row.archived,
        'is-active': row.active,
      },
    ]"
    :style="{ '--session-sidebar-accent': row.accentColor }"
    :title="row.tooltip"
    @click="handleRowClick"
    @contextmenu.prevent.stop="handleContextMenu"
    @pointerdown="handlePointerDown"
    @pointermove="handlePointerMove"
    @pointerup="cancelLongPress"
    @pointercancel="cancelLongPress"
  >
    <button type="button" class="session-sidebar-main">
      <span class="session-sidebar-agent-wrap" aria-hidden="true">
        <span class="session-sidebar-agent-icon" v-html="row.iconHtml"></span>
        <n-icon v-if="row.hasWorkflowPlanBadge" class="session-sidebar-plan-flag" size="9">
          <FlagIcon />
        </n-icon>
      </span>
      <span class="session-sidebar-title-line">
        <span class="session-sidebar-item-title">{{ row.title }}</span>
        <span v-if="row.searchMatchLabel" class="session-sidebar-search-match">
          {{ row.searchMatchLabel }}
        </span>
      </span>
    </button>

    <div class="session-sidebar-actions">
      <span v-if="row.archiving" class="session-sidebar-spinner" aria-hidden="true"></span>
      <span
        v-if="row.projectBadge"
        class="project-index-badge session-project-badge"
        :class="{ 'is-single-project': row.singleProject }"
        :style="{ '--badge-color': row.projectBadge.color }"
      >
        {{ row.projectBadge.label }}
      </span>
      <span class="session-sidebar-trailing-slot">
        <span class="session-sidebar-activity-time" :title="row.activityTimeTitle">
          {{ row.activityTimeLabel }}
        </span>
        <button
          type="button"
          class="session-sidebar-menu-button"
          :title="row.moreActionsLabel"
          :aria-label="row.moreActionsLabel"
          @click.stop="handleMenuButtonClick"
        >
          <n-icon size="14">
            <EllipsisHorizontal />
          </n-icon>
        </button>
      </span>
    </div>

    <n-dropdown
      v-model:show="menuVisible"
      trigger="manual"
      placement="bottom-end"
      :x="menuX"
      :y="menuY"
      :options="actionOptions"
      @clickoutside="closeMenu"
      @select="handleMenuSelect"
    />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref } from 'vue';
import { NDropdown, NIcon, type DropdownOption } from 'naive-ui';
import { EllipsisHorizontal, Flag as FlagIcon } from '@vicons/ionicons5';
import type { WebSessionSidebarRowView } from './webSessionSidebarVirtualList';

defineProps<{
  row: WebSessionSidebarRowView;
  actionOptions: DropdownOption[];
}>();

const emit = defineEmits<{
  (event: 'select'): void;
  (event: 'action', key: string | number): void;
}>();

const LONG_PRESS_DELAY_MS = 500;
const LONG_PRESS_MOVE_THRESHOLD_PX = 8;
const menuVisible = ref(false);
const menuX = ref(0);
const menuY = ref(0);
let longPressTimer: number | null = null;
let longPressPointerId: number | null = null;
let longPressStartX = 0;
let longPressStartY = 0;
let suppressSelectUntil = 0;

function openMenuAt(x: number, y: number) {
  menuVisible.value = false;
  void nextTick(() => {
    menuX.value = x;
    menuY.value = y;
    menuVisible.value = true;
  });
}

function closeMenu() {
  menuVisible.value = false;
}

function handleMenuButtonClick(event: MouseEvent) {
  const button = event.currentTarget as HTMLButtonElement;
  const rect = button.getBoundingClientRect();
  openMenuAt(rect.right, rect.bottom);
}

function handleContextMenu(event: MouseEvent) {
  suppressSelectUntil = Date.now() + 800;
  openMenuAt(event.clientX, event.clientY);
}

function handleMenuSelect(key: string | number) {
  closeMenu();
  emit('action', key);
}

function handleRowClick() {
  if (Date.now() < suppressSelectUntil) {
    return;
  }
  emit('select');
}

function clearLongPressTimer() {
  if (longPressTimer !== null) {
    window.clearTimeout(longPressTimer);
    longPressTimer = null;
  }
  longPressPointerId = null;
}

function handlePointerDown(event: PointerEvent) {
  if (event.pointerType === 'mouse' || event.button !== 0 || !event.isPrimary) {
    return;
  }
  const target = event.target;
  if (target instanceof Element && target.closest('.session-sidebar-menu-button')) {
    return;
  }

  clearLongPressTimer();
  longPressPointerId = event.pointerId;
  longPressStartX = event.clientX;
  longPressStartY = event.clientY;
  longPressTimer = window.setTimeout(() => {
    longPressTimer = null;
    longPressPointerId = null;
    suppressSelectUntil = Date.now() + 800;
    openMenuAt(longPressStartX, longPressStartY);
  }, LONG_PRESS_DELAY_MS);
}

function handlePointerMove(event: PointerEvent) {
  if (event.pointerId !== longPressPointerId) {
    return;
  }
  if (
    Math.hypot(event.clientX - longPressStartX, event.clientY - longPressStartY) >
    LONG_PRESS_MOVE_THRESHOLD_PX
  ) {
    clearLongPressTimer();
  }
}

function cancelLongPress(event: PointerEvent) {
  if (event.pointerId === longPressPointerId) {
    clearLongPressTimer();
  }
}

onBeforeUnmount(clearLongPressTimer);
</script>

<style scoped>
.session-sidebar-item {
  --session-sidebar-outline: #4f8ff7;
  box-sizing: border-box;
  width: 100%;
  min-height: var(--session-sidebar-row-height, 34px);
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 0 4px 0 0;
  border: 1px solid color-mix(in srgb, var(--n-border-color) 48%, transparent);
  border-radius: 6px;
  background: var(--app-surface-color, #fff);
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.06);
  text-align: left;
  position: relative;
  cursor: pointer;
  -webkit-touch-callout: none;
  transition:
    background-color 0.18s ease,
    border-color 0.18s ease,
    box-shadow 0.18s ease;
}

.session-sidebar-item:hover {
  background: color-mix(in srgb, var(--session-sidebar-outline) 7%, var(--app-surface-color, #fff));
}

.session-sidebar-item.is-active {
  border-color: color-mix(in srgb, var(--session-sidebar-outline) 72%, #ffffff);
  background: color-mix(in srgb, var(--session-sidebar-outline) 8%, var(--app-surface-color, #fff));
  box-shadow:
    0 0 0 1px color-mix(in srgb, var(--session-sidebar-outline) 22%, transparent),
    0 1px 3px rgba(15, 23, 42, 0.06);
}

.session-sidebar-item:focus-within {
  border-color: color-mix(in srgb, var(--session-sidebar-outline) 72%, #ffffff);
  box-shadow:
    0 0 0 1px color-mix(in srgb, var(--session-sidebar-outline) 22%, transparent),
    0 1px 3px rgba(15, 23, 42, 0.06);
}

.session-sidebar-item.is-archived {
  opacity: 0.9;
}

.session-sidebar-item.is-archiving {
  cursor: wait;
}

.session-sidebar-plan-flag {
  position: absolute;
  z-index: 2;
  top: -7px;
  left: -6px;
  color: #6366f1;
  pointer-events: none;
}

.session-sidebar-plan-flag :deep(svg) {
  display: block;
}

.session-sidebar-main {
  flex: 1;
  min-width: 0;
  align-self: stretch;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 0 0 7px;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
  font: inherit;
}

.session-sidebar-main:focus {
  outline: none;
}

.session-sidebar-title-line {
  flex: 1;
  display: flex;
  align-items: center;
  min-width: 0;
  overflow: hidden;
}

.session-sidebar-agent-wrap {
  position: relative;
  width: var(--session-sidebar-leading-icon-size, 16px);
  height: var(--session-sidebar-leading-icon-size, 16px);
  flex: 0 0 var(--session-sidebar-leading-icon-size, 16px);
}

.session-sidebar-agent-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--session-sidebar-leading-icon-size, 16px);
  height: var(--session-sidebar-leading-icon-size, 16px);
  background: transparent;
  color: var(--n-primary-color);
}

.session-sidebar-agent-icon :deep(svg) {
  display: block;
}

.session-sidebar-item-title {
  min-width: 0;
  font-size: var(--session-sidebar-title-font-size, 12px);
  font-weight: 600;
  color: var(--app-text-color, var(--n-text-color-1, #111827));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.session-sidebar-search-match {
  flex: 0 0 auto;
  margin-left: 4px;
  color: var(--n-text-color-3);
  font-size: 10px;
  font-weight: 500;
  white-space: nowrap;
}

.session-sidebar-state-text {
  flex-shrink: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  font-weight: 500;
  color: var(--n-text-color-3);
}

.session-sidebar-actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 3px;
  min-width: 0;
  justify-content: flex-end;
}

@keyframes web-session-sidebar-spin {
  to {
    transform: rotate(360deg);
  }
}

.session-sidebar-spinner {
  width: 9px;
  height: 9px;
  flex-shrink: 0;
  border-radius: 50%;
  border: 1.75px solid color-mix(in srgb, var(--n-primary-color) 24%, transparent);
  border-top-color: var(--n-primary-color);
  animation: web-session-sidebar-spin 0.72s linear infinite;
}

.project-index-badge.session-project-badge {
  width: var(--session-sidebar-badge-size, 18px);
  height: var(--session-sidebar-badge-size, 18px);
  font-size: 9px;
  font-weight: 800;
  color: #ffffff;
  background: var(--badge-color, #3b82f6);
  background-image: none;
  border: 1px solid
    color-mix(in srgb, var(--badge-color, #3b82f6) 78%, var(--app-surface-color, #fff) 22%);
  margin-left: 0;
  box-shadow: 0 2px 6px color-mix(in srgb, var(--badge-color, #3b82f6) 22%, transparent);
}

.project-index-badge.session-project-badge.is-single-project {
  display: none;
}

.session-sidebar-trailing-slot {
  width: 38px;
  height: var(--session-sidebar-action-size, 22px);
  flex: 0 0 38px;
  display: grid;
  align-items: center;
  justify-items: end;
}

.session-sidebar-activity-time {
  grid-area: 1 / 1;
  padding-right: 2px;
  color: var(--n-text-color-3);
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
  pointer-events: none;
  transition: opacity 0.14s ease;
}

.session-sidebar-menu-button {
  grid-area: 1 / 1;
  width: var(--session-sidebar-action-size, 22px);
  height: var(--session-sidebar-action-size, 22px);
  padding: 0;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: color-mix(in srgb, var(--n-text-color-2) 84%, #475569);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  pointer-events: none;
  transition:
    opacity 0.14s ease,
    background-color 0.18s ease,
    color 0.18s ease;
}

.session-sidebar-item:hover .session-sidebar-activity-time,
.session-sidebar-item:focus-within .session-sidebar-activity-time {
  opacity: 0;
}

.session-sidebar-item:hover .session-sidebar-menu-button,
.session-sidebar-item:focus-within .session-sidebar-menu-button {
  opacity: 1;
  pointer-events: auto;
}

.session-sidebar-menu-button:hover {
  background: color-mix(in srgb, var(--n-border-color) 40%, transparent);
  color: var(--n-text-color-1);
}

.session-sidebar-menu-button :deep(svg) {
  display: block;
}

.session-sidebar-working {
  --session-sidebar-outline: #8b5cf6;
  background: color-mix(in srgb, #8b5cf6 9%, var(--app-surface-color, #fff));
}

.session-sidebar-approval {
  --session-sidebar-outline: #f79009;
  background: rgba(247, 144, 9, 0.12);
}

.session-sidebar-item.session-sidebar-approval.is-active,
.session-sidebar-item.session-sidebar-approval.is-active:hover {
  background: rgba(247, 144, 9, 0.18);
}

.session-sidebar-plan-approval {
  --session-sidebar-outline: var(--session-sidebar-accent, #0891b2);
  background: color-mix(
    in srgb,
    var(--web-session-plan-approval-bg, rgba(6, 182, 212, 0.14)) 82%,
    var(--app-surface-color, #fff)
  );
}

.session-sidebar-item.session-sidebar-plan-approval.is-active,
.session-sidebar-item.session-sidebar-plan-approval.is-active:hover {
  background: color-mix(
    in srgb,
    var(--web-session-plan-approval-bg, rgba(6, 182, 212, 0.14)) 100%,
    var(--app-surface-color, #fff)
  );
}

.session-sidebar-completion {
  --session-sidebar-outline: #10b981;
  background: color-mix(in srgb, #10b981 10%, var(--app-surface-color, #fff));
}

.session-sidebar-idle {
  background: var(--app-surface-color, #fff);
}

.session-sidebar-error {
  --session-sidebar-outline: #e5484d;
  background: color-mix(in srgb, #e5484d 9%, var(--app-surface-color, #fff));
}

.session-sidebar-item.session-sidebar-error:hover {
  background: color-mix(in srgb, #e5484d 11%, var(--app-surface-color, #fff));
}

.session-sidebar-item.session-sidebar-error.is-active,
.session-sidebar-item.session-sidebar-error.is-active:hover {
  background: color-mix(in srgb, #e5484d 14%, var(--app-surface-color, #fff));
}

@media (prefers-reduced-motion: reduce) {
  .session-sidebar-spinner {
    animation: none;
  }
}
</style>
