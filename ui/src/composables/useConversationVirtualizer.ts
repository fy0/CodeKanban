import { computed, ref, watch, type Ref } from 'vue';
import { useResizeObserver } from '@vueuse/core';

export interface ConversationVirtualItem {
  key: string;
}

export interface VisibleConversationVirtualItem<T> {
  item: T;
  index: number;
  top: number;
  height: number;
}

interface UseConversationVirtualizerOptions<T extends ConversationVirtualItem> {
  items: Ref<T[]>;
  containerRef: Ref<HTMLElement | null>;
  estimateHeight: (item: T, index: number) => number;
  overscanPx?: number;
}

function clampIndex(index: number, maxIndex: number) {
  if (maxIndex < 0) {
    return 0;
  }
  return Math.max(0, Math.min(index, maxIndex));
}

function findIndexForOffset(offsets: number[], offset: number) {
  if (offsets.length <= 1) {
    return 0;
  }
  let low = 0;
  let high = offsets.length - 1;

  while (low < high) {
    const middle = Math.floor((low + high + 1) / 2);
    if (offsets[middle] <= offset) {
      low = middle;
    } else {
      high = middle - 1;
    }
  }

  return clampIndex(low, offsets.length - 2);
}

export function useConversationVirtualizer<T extends ConversationVirtualItem>({
  items,
  containerRef,
  estimateHeight,
  overscanPx = 640,
}: UseConversationVirtualizerOptions<T>) {
  const scrollTop = ref(0);
  const viewportHeight = ref(0);
  const measuredHeights = new Map<string, number>();
  const measuredVersion = ref(0);

  const itemIndexMap = computed(() => {
    return new Map(items.value.map((item, index) => [item.key, index]));
  });

  const heights = computed(() => {
    void measuredVersion.value;
    return items.value.map((item, index) => {
      return measuredHeights.get(item.key) ?? estimateHeight(item, index);
    });
  });

  const offsets = computed(() => {
    const nextOffsets = Array.from({ length: items.value.length + 1 }, () => 0);
    const currentHeights = heights.value;
    for (let index = 0; index < currentHeights.length; index += 1) {
      nextOffsets[index + 1] = nextOffsets[index] + currentHeights[index];
    }
    return nextOffsets;
  });

  const totalHeight = computed(() => {
    const currentOffsets = offsets.value;
    return currentOffsets[currentOffsets.length - 1] ?? 0;
  });

  const visibleRange = computed(() => {
    const itemCount = items.value.length;
    if (!itemCount) {
      return { start: 0, end: -1 };
    }

    if (viewportHeight.value <= 0) {
      const fallbackEnd = Math.min(itemCount - 1, 20);
      return { start: 0, end: fallbackEnd };
    }

    const currentOffsets = offsets.value;
    const start = findIndexForOffset(currentOffsets, Math.max(0, scrollTop.value - overscanPx));
    const end = findIndexForOffset(
      currentOffsets,
      Math.max(0, scrollTop.value + viewportHeight.value + overscanPx)
    );

    return {
      start,
      end: Math.max(start, end),
    };
  });

  const visibleItems = computed<VisibleConversationVirtualItem<T>[]>(() => {
    const { start, end } = visibleRange.value;
    if (end < start) {
      return [];
    }
    const currentOffsets = offsets.value;
    return items.value.slice(start, end + 1).map((item, relativeIndex) => {
      const index = start + relativeIndex;
      return {
        item,
        index,
        top: currentOffsets[index] ?? 0,
        height: heights.value[index] ?? estimateHeight(item, index),
      };
    });
  });

  const beforeHeight = computed(() => {
    const { start } = visibleRange.value;
    return offsets.value[start] ?? 0;
  });

  const afterHeight = computed(() => {
    const { end } = visibleRange.value;
    if (end < 0) {
      return 0;
    }
    return Math.max(0, totalHeight.value - (offsets.value[end + 1] ?? totalHeight.value));
  });

  function syncScrollPosition() {
    const container = containerRef.value;
    if (!container) {
      scrollTop.value = 0;
      return;
    }
    scrollTop.value = container.scrollTop;
  }

  function setMeasuredHeight(key: string, nextHeight: number) {
    if (!Number.isFinite(nextHeight)) {
      return false;
    }

    const index = itemIndexMap.value.get(key);
    if (index === undefined) {
      return false;
    }

    const currentHeights = heights.value;
    const previousHeight = currentHeights[index];
    const normalizedHeight = Math.max(1, Math.round(nextHeight));
    if (Math.abs(previousHeight - normalizedHeight) < 1) {
      return false;
    }

    const itemTop = offsets.value[index] ?? 0;
    const container = containerRef.value;
    measuredHeights.set(key, normalizedHeight);
    measuredVersion.value += 1;

    if (container && itemTop < container.scrollTop) {
      container.scrollTop += normalizedHeight - previousHeight;
      scrollTop.value = container.scrollTop;
    }

    return true;
  }

  function scrollToIndex(index: number, behavior: ScrollBehavior = 'auto') {
    const container = containerRef.value;
    if (!container || !items.value.length) {
      return;
    }
    const targetIndex = clampIndex(index, items.value.length - 1);
    const targetTop = offsets.value[targetIndex] ?? 0;
    container.scrollTo({
      top: targetTop,
      behavior,
    });
    scrollTop.value = container.scrollTop;
  }

  function scrollToKey(key: string, behavior: ScrollBehavior = 'auto') {
    const index = itemIndexMap.value.get(key);
    if (index === undefined) {
      return;
    }
    scrollToIndex(index, behavior);
  }

  useResizeObserver(containerRef, entries => {
    const entry = entries[0];
    if (!entry) {
      return;
    }
    viewportHeight.value = entry.contentRect.height;
    syncScrollPosition();
  });

  watch(
    () => items.value.length,
    () => {
      syncScrollPosition();
    },
    { immediate: true }
  );

  return {
    scrollTop,
    viewportHeight,
    totalHeight,
    beforeHeight,
    afterHeight,
    visibleItems,
    syncScrollPosition,
    setMeasuredHeight,
    scrollToIndex,
    scrollToKey,
  };
}
