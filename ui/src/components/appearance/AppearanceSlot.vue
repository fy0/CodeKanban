<script setup lang="ts">
import { computed, defineComponent, h, onErrorCaptured, ref } from 'vue';
import { getAppearanceContributions, type AppearanceSlot } from '@/themes/registry';
const props = defineProps<{ name: AppearanceSlot }>();
const Boundary = defineComponent({
  setup(_, { slots }) {
    const failed = ref(false);
    onErrorCaptured(() => { failed.value = true; return false; });
    return () => failed.value ? h('span', { class: 'ck-plugin-error', role: 'status' }, 'Extension unavailable') : slots.default?.();
  },
});
const items = computed(() => getAppearanceContributions(props.name));
</script>

<template>
  <template v-if="items.length">
    <Boundary v-for="item in items" :key="item.id"><component :is="item.component" /></Boundary>
  </template>
  <slot v-else />
</template>
