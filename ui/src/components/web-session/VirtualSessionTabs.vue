<template>
  <div class="virtual-session-tabs">
    <div class="n-tabs">
      <div class="n-tabs-nav n-tabs-nav--card-type">
        <div
          ref="viewport"
          class="virtual-tab-viewport v-x-scroll"
          role="tablist"
          @scroll="readScroll"
          @wheel="handleWheel"
          @dragover="handleDragOver"
          @drop="handleDrop"
          @dragleave="handleDragLeave"
        >
          <div
            class="n-tabs-wrapper virtual-tab-track"
            :style="{ width: `${items.length * tabWidth}px` }"
          >
            <div
              v-for="entry in visibleItems"
              :key="entry.item.id"
              class="n-tabs-tab-wrapper virtual-tab-wrapper"
              :style="{
                width: `${tabWidth}px`,
                transform: `translateX(${entry.index * tabWidth}px)`,
              }"
            >
              <div
                v-bind="tabProps(entry.item)"
                class="n-tabs-tab"
                :class="{
                  'n-tabs-tab--active': entry.item.id === activeId,
                  'is-dragging': entry.item.id === draggedId,
                }"
                role="tab"
                :aria-selected="entry.item.id === activeId"
                :aria-setsize="items.length"
                :aria-posinset="entry.index + 1"
                :aria-label="entry.item.title"
                :tabindex="entry.item.id === (activeId || items[0]?.id) ? 0 : -1"
                :data-session-id="entry.item.id"
                :draggable="items.length > 1 && !isLocked?.(entry.item)"
                @click="emit('select', entry.item.id)"
                @keydown="handleKeydown($event, entry.index)"
                @dragstart="handleDragStart($event, entry.item)"
                @dragend="stopDrag"
              >
                <slot :item="entry.item" />
                <button
                  type="button"
                  class="n-tabs-tab__close"
                  :aria-label="`${closeLabel} ${entry.item.title}`"
                  tabindex="-1"
                  draggable="false"
                  @click.stop="emit('close', entry.item.id)"
                  @mousedown.stop
                >
                  <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
                    <path
                      d="m3 3 6 6m0-6-6 6"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="1.4"
                    />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts" generic="T extends { id: string; title: string }">
import { computed, nextTick, onBeforeUnmount, ref, watch, type HTMLAttributes } from 'vue';
import { useResizeObserver } from '@vueuse/core';
import {
  getSessionTabDropIndex,
  getSessionTabRange,
  getSessionTabScrollLeft,
  getSessionTabWidth,
} from './virtualSessionTabs';

const props = defineProps<{
  items: T[];
  activeId: string;
  closeLabel: string;
  tabProps: (item: T) => HTMLAttributes;
  isLocked?: (item: T) => boolean;
}>();
const emit = defineEmits<{
  select: [id: string];
  close: [id: string];
  reorder: [event: { oldIndex: number; newIndex: number }];
}>();
const viewport = ref<HTMLElement | null>(null);
const scrollLeft = ref(0);
const viewportWidth = ref(0);
const draggedId = ref('');
let dragX = 0;
let dragFrame: number | null = null;
const activeIndex = computed(() => props.items.findIndex(item => item.id === props.activeId));
// Clamp once per viewport/count change; large lists keep a stable width without measuring tabs.
const tabWidth = computed(() => getSessionTabWidth(props.items.length, viewportWidth.value));
const range = computed(() =>
  getSessionTabRange(props.items.length, scrollLeft.value, viewportWidth.value, tabWidth.value)
);
const visibleItems = computed(() => {
  const entries = props.items
    .slice(range.value.start, range.value.end)
    .map((item, offset) => ({ item, index: range.value.start + offset }));
  // Keep the native drag source mounted while autoscroll advances the virtual window.
  if (draggedId.value) {
    const index = props.items.findIndex(item => item.id === draggedId.value);
    if (index >= 0 && !entries.some(entry => entry.index === index))
      entries.push({ item: props.items[index]!, index });
  }
  return entries;
});

function readScroll() {
  scrollLeft.value = viewport.value?.scrollLeft ?? 0;
}

function scrollToActive() {
  const element = viewport.value;
  if (!element || !element.clientWidth || activeIndex.value < 0 || draggedId.value) return;
  const nextScrollLeft = getSessionTabScrollLeft(
    activeIndex.value,
    element.scrollLeft,
    element.clientWidth,
    tabWidth.value
  );
  element.scrollLeft = Math.min(
    Math.max(0, props.items.length * tabWidth.value - element.clientWidth),
    nextScrollLeft
  );
  readScroll();
}

useResizeObserver(viewport, ([entry]) => {
  if (!entry) return;
  viewportWidth.value = entry.contentRect.width;
});
watch(
  [() => props.activeId, () => props.items.map(item => item.id).join('\0')],
  () => {
    if (draggedId.value && !props.items.some(item => item.id === draggedId.value)) stopDrag();
    scrollToActive();
  },
  { flush: 'post' }
);
watch(viewportWidth, scrollToActive, { flush: 'post' });

function handleWheel(event: WheelEvent) {
  if (event.ctrlKey || event.shiftKey || Math.abs(event.deltaX) > Math.abs(event.deltaY)) return;
  const element = viewport.value;
  if (!element || element.scrollWidth <= element.clientWidth) return;
  event.preventDefault();
  element.scrollLeft +=
    event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? element.clientWidth : 1);
  readScroll();
}

async function handleKeydown(event: KeyboardEvent, index: number) {
  if (event.target !== event.currentTarget) return;
  let next = index;
  switch (event.key) {
    case 'ArrowLeft':
      next = Math.max(0, index - 1);
      break;
    case 'ArrowRight':
      next = Math.min(props.items.length - 1, index + 1);
      break;
    case 'Home':
      next = 0;
      break;
    case 'End':
      next = props.items.length - 1;
      break;
    case 'Delete':
      event.preventDefault();
      emit('close', props.items[index]!.id);
      return;
    case 'Enter':
    case ' ':
      event.preventDefault();
      emit('select', props.items[index]!.id);
      return;
    default:
      return;
  }
  event.preventDefault();
  const item = props.items[next];
  if (!item) return;
  emit('select', item.id);
  await nextTick();
  scrollToActive();
  await nextTick();
  const tab = [...(viewport.value?.querySelectorAll<HTMLElement>('[role="tab"]') ?? [])].find(
    element => element.dataset.sessionId === item.id
  );
  tab?.focus({ preventScroll: true });
}

function handleDragStart(event: DragEvent, item: T) {
  if (props.isLocked?.(item) || (event.target as Element).closest('.n-tabs-tab__close')) {
    event.preventDefault();
    return;
  }
  draggedId.value = item.id;
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('application/x-codekanban-session-tab', item.id);
  }
}

function autoscroll() {
  const element = viewport.value;
  if (!element || !draggedId.value) {
    dragFrame = null;
    return;
  }
  const rect = element.getBoundingClientRect();
  const speed = dragX < rect.left + 40 ? -12 : dragX > rect.right - 40 ? 12 : 0;
  if (speed) {
    element.scrollLeft += speed;
    readScroll();
  }
  dragFrame = requestAnimationFrame(autoscroll);
}

function handleDragOver(event: DragEvent) {
  if (!draggedId.value) return;
  event.preventDefault();
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
  dragX = event.clientX;
  if (dragFrame === null) dragFrame = requestAnimationFrame(autoscroll);
}

function handleDragLeave(event: DragEvent) {
  if (viewport.value?.contains(event.relatedTarget as Node | null)) return;
  if (dragFrame !== null) cancelAnimationFrame(dragFrame);
  dragFrame = null;
}

function handleDrop(event: DragEvent) {
  const element = viewport.value;
  if (!element || !draggedId.value) return;
  event.preventDefault();
  const oldIndex = props.items.findIndex(item => item.id === draggedId.value);
  const newIndex = getSessionTabDropIndex(
    oldIndex,
    element.scrollLeft + event.clientX - element.getBoundingClientRect().left,
    props.items.length,
    tabWidth.value
  );
  stopDrag();
  if (oldIndex >= 0 && oldIndex !== newIndex) emit('reorder', { oldIndex, newIndex });
}

function stopDrag() {
  draggedId.value = '';
  if (dragFrame !== null) cancelAnimationFrame(dragFrame);
  dragFrame = null;
}

onBeforeUnmount(stopDrag);
defineExpose({ scrollToActive });
</script>

<style scoped>
.virtual-session-tabs {
  min-width: 0;
  overflow: hidden;
}
.virtual-tab-viewport {
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: none;
  overscroll-behavior-x: contain;
}
.virtual-tab-viewport::-webkit-scrollbar {
  display: none;
  width: 0;
  height: 0;
}
.virtual-tab-track {
  position: relative;
  height: 36px;
}
.virtual-tab-wrapper {
  position: absolute;
  top: 0;
  left: 0;
  padding-right: 4px;
  box-sizing: border-box;
}
.n-tabs-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 4px 8px;
  box-sizing: border-box;
  border: 1px solid;
  border-radius: 4px 4px 0 0;
  font-size: 13px;
  white-space: nowrap;
}
.n-tabs-tab--active::after {
  content: '';
  position: absolute;
  bottom: 5px;
  left: 38%;
  width: 24%;
  height: 2px;
  border-radius: 1px;
  background: var(--n-primary-color, var(--app-primary-color, #18a058));
}
.n-tabs-tab:focus-visible {
  outline: 2px solid var(--n-primary-color, #18a058);
  outline-offset: -2px;
}
.n-tabs-tab.is-dragging {
  opacity: 0.45;
}
.n-tabs-tab__close {
  flex: 0 0 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 18px;
  margin-left: auto;
  padding: 0;
  border: 0;
  border-radius: 3px;
  color: inherit;
  background: transparent;
  cursor: pointer;
}
.n-tabs-tab__close:hover {
  background: color-mix(in srgb, currentColor 12%, transparent);
}
</style>
