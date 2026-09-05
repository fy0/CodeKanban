<script setup lang="ts">
defineProps<{
  text: string;
  label: string;
  time: string;
  timeTitle: string;
  expanded?: boolean;
}>();

defineEmits<{ toggle: [] }>();
</script>

<template>
  <div v-if="text.trim()" class="reasoning-summary">
    <button
      type="button"
      class="reasoning-summary-toggle"
      :aria-expanded="Boolean(expanded)"
      @click="$emit('toggle')"
    >
      <span aria-hidden="true">{{ expanded ? '▾' : '▸' }}</span>
      <span>{{ label }}</span>
      <span class="reasoning-summary-time" :title="timeTitle">{{ time }}</span>
    </button>
    <div v-if="expanded" class="reasoning-summary-body"><slot /></div>
  </div>
</template>

<style scoped>
.reasoning-summary {
  min-width: 0;
}

.reasoning-summary-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 100%;
  padding: 4px 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.reasoning-summary-time {
  font-size: 11px;
  opacity: 0.65;
}

.reasoning-summary-body {
  padding: 8px 0 0 16px;
  overflow-wrap: anywhere;
}
</style>
