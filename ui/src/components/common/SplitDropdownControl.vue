<template>
  <div class="split-dropdown-control" :class="{ 'is-active': active, 'is-flat': flat }">
    <button
      type="button"
      class="split-dropdown-main"
      :title="title || undefined"
      :aria-label="ariaLabel || title || undefined"
      @click="$emit('main-click')"
    >
      <span v-if="$slots.prefix" class="split-dropdown-icon" aria-hidden="true">
        <slot name="prefix"></slot>
      </span>
      <span class="split-dropdown-label">{{ label }}</span>
    </button>
    <n-dropdown trigger="click" :placement="placement" :options="options" @select="handleSelect">
      <button
        type="button"
        class="split-dropdown-menu"
        :title="menuTitle || title || undefined"
        :aria-label="ariaLabel || menuTitle || title || undefined"
      >
        <slot name="caret">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M6 9l6 6 6-6"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </slot>
      </button>
    </n-dropdown>
  </div>
</template>

<script setup lang="ts">
import type { DropdownOption } from 'naive-ui';

withDefaults(
  defineProps<{
    label: string;
    options: DropdownOption[];
    placement?: string;
    title?: string;
    menuTitle?: string;
    ariaLabel?: string;
    active?: boolean;
    flat?: boolean;
  }>(),
  {
    placement: 'bottom-start',
    title: '',
    menuTitle: '',
    ariaLabel: '',
    active: false,
    flat: false,
  }
);

const emit = defineEmits<{
  (event: 'main-click'): void;
  (event: 'select', key: string | number): void;
}>();

function handleSelect(key: string | number) {
  emit('select', key);
}
</script>

<style scoped>
.split-dropdown-control {
  display: inline-flex;
  border-radius: 6px;
  gap: 0;
  padding: 0;
  border: 1px solid var(--kanban-notification-button-border, var(--app-border, rgba(0, 0, 0, 0.2)));
  background: var(--app-surface, var(--app-surface-color, var(--body-color, #ffffff)));
  color: var(--app-text-primary, inherit);
  box-shadow: none;
}

.split-dropdown-main,
.split-dropdown-menu {
  border: none;
  background: transparent;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font: inherit;
  color: inherit;
  height: 32px;
  font-size: 12px;
  font-weight: 500;
  transition: all 0.2s ease;
}

.split-dropdown-main {
  padding: 0 10px;
  justify-content: flex-start;
}

.split-dropdown-menu {
  padding: 0 8px;
  border-left: 1px solid var(--app-border, rgba(0, 0, 0, 0.08));
  justify-content: center;
}

.split-dropdown-control:hover {
  box-shadow: 0 4px 12px var(--app-shadow, rgba(15, 23, 42, 0.15));
}

.split-dropdown-control.is-flat:hover {
  box-shadow: none;
}

.split-dropdown-control.is-active {
  box-shadow: none;
}

.split-dropdown-control.is-active:hover {
  box-shadow: 0 4px 12px var(--app-shadow, rgba(15, 23, 42, 0.15));
}

.split-dropdown-main:focus-visible,
.split-dropdown-menu:focus-visible {
  outline: 2px solid var(--app-focus-ring, var(--n-primary-color));
  outline-offset: -2px;
  background: var(--app-accent-soft, color-mix(in srgb, var(--n-primary-color) 4%, transparent));
}

.split-dropdown-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.split-dropdown-label {
  display: inline-flex;
  align-items: center;
  white-space: nowrap;
}
</style>
