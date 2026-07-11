<template>
  <button
    type="button"
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
    @click="emit('select')"
  >
    <div class="session-sidebar-main">
      <div class="session-sidebar-title-line">
        <span class="session-sidebar-agent-icon" v-html="row.iconHtml"></span>
        <span class="session-sidebar-item-title">{{ row.title }}</span>
        <span v-if="row.subtitle" class="session-sidebar-state-text"> · {{ row.subtitle }} </span>
      </div>
    </div>

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
      <span v-if="row.archived" class="session-archived-pill">{{ row.archivedLabel }}</span>
      <span
        v-else
        class="session-current-indicator"
        :class="{ 'is-hidden': !row.active }"
        :title="row.currentIndicatorTitle"
      >
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path>
          <circle cx="12" cy="12" r="3"></circle>
        </svg>
      </span>
    </div>
  </button>
</template>

<script setup lang="ts">
import type { WebSessionSidebarRowView } from './webSessionSidebarVirtualList';

defineProps<{
  row: WebSessionSidebarRowView;
}>();

const emit = defineEmits<{
  (event: 'select'): void;
}>();
</script>

<style scoped>
.session-sidebar-item {
  width: 100%;
  min-height: 34px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid color-mix(in srgb, var(--n-primary-color) 12%, var(--n-border-color));
  border-left: 4px solid var(--session-sidebar-accent, rgba(15, 23, 42, 0.08));
  background: var(--app-surface-color, #fff);
  text-align: left;
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    background-color 0.18s ease,
    box-shadow 0.18s ease;
}

.session-sidebar-item.has-workflow-plan-badge {
  position: relative;
  overflow: visible;
}

.session-sidebar-item.has-workflow-plan-badge::before,
.session-sidebar-item.has-workflow-plan-badge::after {
  content: '';
  position: absolute;
  top: 10px;
  left: -6px;
  z-index: 2;
  width: 18px;
  height: 2px;
  background: var(--session-sidebar-accent, #0ea5e9);
  transform-origin: center center;
  pointer-events: none;
}

.session-sidebar-item.has-workflow-plan-badge::before {
  transform: rotate(54deg);
}

.session-sidebar-item.has-workflow-plan-badge::after {
  transform: rotate(-54deg);
}

.session-sidebar-item:hover {
  box-shadow: 0 6px 16px rgba(15, 23, 42, 0.12);
}

.session-sidebar-item.is-active {
  border-color: color-mix(
    in srgb,
    var(--session-sidebar-accent, var(--n-primary-color)) 44%,
    var(--n-border-color)
  );
  background: linear-gradient(
    135deg,
    color-mix(
        in srgb,
        var(--session-sidebar-accent, var(--n-primary-color)) 14%,
        var(--app-surface-color, #fff)
      )
      0%,
    color-mix(
        in srgb,
        var(--session-sidebar-accent, var(--n-primary-color)) 6%,
        var(--app-surface-color, #fff)
      )
      100%
  );
  box-shadow: 0 6px 16px
    color-mix(in srgb, var(--session-sidebar-accent, var(--n-primary-color)) 20%, transparent);
}

.session-sidebar-item.is-archived {
  border-style: dashed;
}

.session-sidebar-item.is-archiving {
  cursor: wait;
}

.session-sidebar-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
}

.session-sidebar-title-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.session-sidebar-agent-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 999px;
  background: transparent;
  color: var(--n-primary-color);
  flex-shrink: 0;
}

.session-sidebar-agent-icon :deep(svg) {
  display: block;
}

.session-sidebar-item-title {
  min-width: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-color, var(--n-text-color-1, #111827));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
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
  gap: 6px;
}

@keyframes web-session-sidebar-spin {
  to {
    transform: rotate(360deg);
  }
}

.session-sidebar-spinner {
  width: 11px;
  height: 11px;
  flex-shrink: 0;
  border-radius: 50%;
  border: 1.75px solid color-mix(in srgb, var(--n-primary-color) 24%, transparent);
  border-top-color: var(--n-primary-color);
  animation: web-session-sidebar-spin 0.72s linear infinite;
}

.session-archived-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 38px;
  height: 18px;
  padding: 0 6px;
  border-radius: 999px;
  background: color-mix(in srgb, #94a3b8 16%, transparent);
  color: color-mix(in srgb, #334155 78%, var(--n-text-color-2));
  font-size: 10px;
  font-weight: 700;
}

.session-sidebar-item.is-active .session-archived-pill {
  background: color-mix(in srgb, var(--n-primary-color) 18%, rgba(255, 255, 255, 0.92));
  color: color-mix(in srgb, var(--n-primary-color) 88%, #ffffff 12%);
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--n-primary-color) 26%, transparent),
    0 1px 2px rgba(59, 130, 246, 0.14);
}

.project-index-badge.session-project-badge {
  width: 18px;
  height: 18px;
  font-size: 10px;
  color: #ffffff;
  background: var(--badge-color, #3b82f6);
  background-image: none;
  border: 1px solid
    color-mix(in srgb, var(--badge-color, #3b82f6) 78%, var(--app-surface-color, #fff) 22%);
  margin-left: 2px;
  box-shadow: none;
}

.project-index-badge.session-project-badge.is-single-project {
  visibility: hidden;
  pointer-events: none;
}

.session-current-indicator {
  width: 18px;
  height: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 0;
  border-radius: 50%;
  background: var(--n-primary-color);
  color: #ffffff;
  border: 1px solid
    color-mix(in srgb, var(--n-primary-color) 78%, var(--app-surface-color, #fff) 22%);
  box-shadow: none;
}

.session-current-indicator.is-hidden {
  opacity: 0;
  pointer-events: none;
}

.session-current-indicator svg {
  display: block;
}

.session-sidebar-working {
  background: color-mix(in srgb, #8b5cf6 8%, var(--app-surface-color, #fff));
}

.session-sidebar-approval {
  border-color: rgba(247, 144, 9, 0.44);
  background: rgba(247, 144, 9, 0.14);
}

.session-sidebar-item.session-sidebar-approval.is-active,
.session-sidebar-item.session-sidebar-approval.is-active:hover {
  border-color: rgba(247, 144, 9, 0.6);
  background: rgba(247, 144, 9, 0.22);
  box-shadow: none;
}

.session-sidebar-plan-approval {
  border-color: var(--web-session-plan-approval-border, rgba(6, 182, 212, 0.3));
  background: var(--web-session-plan-approval-bg, rgba(6, 182, 212, 0.14));
}

.session-sidebar-item.session-sidebar-plan-approval.is-active,
.session-sidebar-item.session-sidebar-plan-approval.is-active:hover {
  border-color: color-mix(
    in srgb,
    var(--web-session-plan-approval-accent-strong, #0e7490) 14%,
    var(--web-session-plan-approval-border, rgba(6, 182, 212, 0.3)) 86%
  );
  border-left-color: var(--web-session-plan-approval-accent, #0891b2);
  background: linear-gradient(
    135deg,
    color-mix(
        in srgb,
        var(--web-session-plan-approval-bg, rgba(6, 182, 212, 0.14)) 92%,
        var(--app-surface-color, #fff) 8%
      )
      0%,
    color-mix(
        in srgb,
        var(--web-session-plan-approval-bg, rgba(6, 182, 212, 0.14)) 76%,
        var(--app-surface-color, #fff) 24%
      )
      100%
  );
  box-shadow:
    inset 0 0 0 1px
      color-mix(in srgb, var(--web-session-plan-approval-accent, #0891b2) 16%, transparent),
    0 6px 18px color-mix(in srgb, var(--web-session-plan-approval-accent, #0891b2) 14%, transparent);
}

.session-sidebar-completion {
  background: color-mix(in srgb, #10b981 10%, var(--app-surface-color, #fff));
}

.session-sidebar-idle {
  background: color-mix(in srgb, #9ca3af 4%, var(--app-surface-color, #fff));
}

.session-sidebar-error {
  background: color-mix(in srgb, #f04438 8%, var(--app-surface-color, #fff));
}

@media (prefers-reduced-motion: reduce) {
  .session-sidebar-spinner {
    animation: none;
  }
}
</style>
