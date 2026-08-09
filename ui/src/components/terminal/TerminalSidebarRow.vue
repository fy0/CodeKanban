<template>
  <div
    class="terminal-sidebar-item"
    :class="{ 'is-active': row.active }"
    :title="row.tooltip"
    @click="handleRowClick"
    @contextmenu.prevent.stop="handleContextMenu"
  >
    <button type="button" class="terminal-sidebar-main">
      <span
        class="terminal-sidebar-agent-icon"
        :style="row.iconColor ? { color: row.iconColor } : undefined"
        aria-hidden="true"
        v-html="row.iconHtml"
      ></span>
      <span class="terminal-sidebar-title-line">
        <span class="terminal-sidebar-title">{{ row.title }}</span>
        <span class="terminal-sidebar-agent-name">{{ row.agentName }}</span>
      </span>
    </button>

    <span
      v-if="row.projectBadge"
      class="terminal-sidebar-project-badge"
      :style="{
        background: row.projectBadge.color,
        color: getReadableTextColor(row.projectBadge.color),
      }"
      :title="row.projectName"
      >{{ row.projectBadge.label }}</span
    >
    <span class="terminal-sidebar-trailing-slot">
      <span class="terminal-sidebar-activity-time" :title="row.activityTimeTitle">
        {{ row.activityTimeLabel }}
      </span>
      <button
        type="button"
        class="terminal-sidebar-menu-button"
        :title="moreActionsLabel"
        :aria-label="moreActionsLabel"
        @click.stop="handleMenuButtonClick"
      >
        <n-icon size="14">
          <EllipsisHorizontal />
        </n-icon>
      </button>
    </span>

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
import { EllipsisHorizontal } from '@vicons/ionicons5';
import { useLocale } from '@/composables/useLocale';
import { getReadableTextColor } from '@/utils/color';

export interface TerminalSidebarRowView {
  id: string;
  /** 所属项目 id（跨项目「全部终端」视图：选中据此跳转/操作对应项目） */
  projectId: string;
  title: string;
  agentName: string;
  iconHtml: string;
  /** 图标品牌色；未识别/无品牌色时留空，使用默认中性色 */
  iconColor?: string;
  /** 跨项目视图下显示的项目徽章；当前项目行可不传 */
  projectBadge?: { label: string; color: string } | null;
  /** 项目名称（徽章 title） */
  projectName?: string;
  active: boolean;
  tooltip: string;
  activityTimeLabel: string;
  activityTimeTitle: string;
}

defineProps<{
  row: TerminalSidebarRowView;
  actionOptions: DropdownOption[];
}>();

const emit = defineEmits<{
  (event: 'select'): void;
  (event: 'action', key: string | number): void;
}>();

const { t } = useLocale();
const moreActionsLabel = t('common.moreActions');
const menuVisible = ref(false);
const menuX = ref(0);
const menuY = ref(0);
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

onBeforeUnmount(closeMenu);
</script>

<style scoped>
.terminal-sidebar-item {
  --terminal-sidebar-outline: var(--app-info, #4f8ff7);
  box-sizing: border-box;
  width: 100%;
  min-height: 34px;
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 0 4px 0 0;
  border: 1px solid color-mix(in srgb, var(--app-border, #e0e0e0) 48%, transparent);
  border-radius: 6px;
  background: var(--app-surface, var(--app-surface-color, #fff));
  box-shadow: 0 1px 3px
    color-mix(in srgb, var(--app-shadow, rgba(15, 23, 42, 0.12)) 50%, transparent);
  text-align: left;
  position: relative;
  cursor: pointer;
  transition:
    background-color 0.18s ease,
    border-color 0.18s ease,
    box-shadow 0.18s ease;
}

.terminal-sidebar-item:hover {
  background: color-mix(
    in srgb,
    var(--terminal-sidebar-outline) 7%,
    var(--app-surface, var(--app-surface-color, #fff))
  );
}

.terminal-sidebar-item.is-active {
  border-color: color-mix(
    in srgb,
    var(--terminal-sidebar-outline) 72%,
    var(--app-border-strong, #ffffff)
  );
  background: color-mix(
    in srgb,
    var(--terminal-sidebar-outline) 8%,
    var(--app-surface, var(--app-surface-color, #fff))
  );
  box-shadow:
    0 0 0 1px color-mix(in srgb, var(--terminal-sidebar-outline) 22%, transparent),
    0 1px 3px color-mix(in srgb, var(--app-shadow, rgba(15, 23, 42, 0.12)) 50%, transparent);
}

.terminal-sidebar-item:focus-within {
  border-color: var(--app-focus-ring, var(--terminal-sidebar-outline));
  box-shadow:
    0 0 0 1px
      color-mix(in srgb, var(--app-focus-ring, var(--terminal-sidebar-outline)) 28%, transparent),
    0 1px 3px color-mix(in srgb, var(--app-shadow, rgba(15, 23, 42, 0.12)) 50%, transparent);
}

.terminal-sidebar-main {
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

.terminal-sidebar-main:focus {
  outline: none;
}

.terminal-sidebar-agent-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  color: var(--app-text-muted, var(--n-text-color-3));
}

.terminal-sidebar-agent-icon :deep(svg) {
  display: block;
  width: 16px;
  height: 16px;
}

.terminal-sidebar-project-badge {
  flex: 0 0 auto;
  min-width: 16px;
  height: 16px;
  padding: 0 3px;
  box-sizing: border-box;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--app-accent-contrast, #fff);
  font-size: 9px;
  font-weight: 700;
  line-height: 1;
}

.terminal-sidebar-title-line {
  min-width: 0;
  flex: 1;
  display: flex;
  align-items: baseline;
  gap: 5px;
  overflow: hidden;
}

.terminal-sidebar-title,
.terminal-sidebar-agent-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.terminal-sidebar-title {
  color: var(--app-text-primary, var(--app-text-color, #111827));
  font-size: 12px;
  font-weight: 600;
}

.terminal-sidebar-agent-name {
  flex-shrink: 1;
  color: var(--app-text-muted, var(--n-text-color-3));
  font-size: 10px;
  font-weight: 500;
}

.terminal-sidebar-trailing-slot {
  width: 38px;
  height: 22px;
  flex: 0 0 38px;
  display: grid;
  align-items: center;
  justify-items: end;
}

.terminal-sidebar-activity-time,
.terminal-sidebar-menu-button {
  grid-area: 1 / 1;
}

.terminal-sidebar-activity-time {
  padding-right: 2px;
  color: var(--app-text-muted, var(--n-text-color-3));
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
  pointer-events: none;
  transition: opacity 0.14s ease;
}

.terminal-sidebar-menu-button {
  width: 22px;
  height: 22px;
  padding: 0;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--app-text-secondary, var(--n-text-color-2));
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

.terminal-sidebar-item:hover .terminal-sidebar-activity-time,
.terminal-sidebar-item:focus-within .terminal-sidebar-activity-time {
  opacity: 0;
}

.terminal-sidebar-item:hover .terminal-sidebar-menu-button,
.terminal-sidebar-item:focus-within .terminal-sidebar-menu-button {
  opacity: 1;
  pointer-events: auto;
}

.terminal-sidebar-menu-button:hover {
  background: var(--app-surface-hover, color-mix(in srgb, var(--n-border-color) 40%, transparent));
  color: var(--app-text-primary, var(--n-text-color-1));
}

.terminal-sidebar-menu-button :deep(svg) {
  display: block;
}
</style>
