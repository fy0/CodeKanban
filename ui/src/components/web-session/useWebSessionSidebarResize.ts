import {
  computed,
  nextTick,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue';
import { useStorage } from '@vueuse/core';

const SIDEBAR_WIDTH_STORAGE_KEY = 'workspace-web-session-sidebar-width';
const STATUS_TEXT_THRESHOLD = 280;
const MIN_SIDEBAR_WIDTH = 200;
const MAX_SIDEBAR_WIDTH = 400;
const DEFAULT_SIDEBAR_WIDTH = 200;
const MIN_MAIN_WIDTH = 420;

export type WebSessionSidebarResizeOptions = {
  visible: MaybeRefOrGetter<boolean>;
  getRootElement: () => HTMLElement | null;
};

export function useWebSessionSidebarResize({
  visible,
  getRootElement,
}: WebSessionSidebarResizeOptions) {
  const containerWidth = ref(0);
  const resizing = ref(false);
  const persistedWidth = useStorage<number>(SIDEBAR_WIDTH_STORAGE_KEY, DEFAULT_SIDEBAR_WIDTH);
  let resizeObserver: ResizeObserver | null = null;

  function clamp(min: number, value: number, max: number) {
    return Math.max(min, Math.min(max, value));
  }

  const maxWidth = computed(() => {
    if (!containerWidth.value) {
      return MAX_SIDEBAR_WIDTH;
    }
    const maxAllowed = Math.max(MIN_SIDEBAR_WIDTH, containerWidth.value - MIN_MAIN_WIDTH);
    return Math.min(MAX_SIDEBAR_WIDTH, maxAllowed);
  });
  const width = computed(() => {
    if (!containerWidth.value) {
      return DEFAULT_SIDEBAR_WIDTH;
    }
    return clamp(MIN_SIDEBAR_WIDTH, Math.round(persistedWidth.value), Math.round(maxWidth.value));
  });
  const showStatusText = computed(() => width.value >= STATUS_TEXT_THRESHOLD);

  function updateContainerWidth() {
    const parent = getRootElement()?.parentElement;
    containerWidth.value = parent?.getBoundingClientRect().width ?? 0;
  }

  function setupResizeObserver() {
    resizeObserver?.disconnect();
    resizeObserver = null;
    const parent = getRootElement()?.parentElement;
    if (!parent || typeof ResizeObserver === 'undefined') {
      updateContainerWidth();
      return;
    }
    resizeObserver = new ResizeObserver(updateContainerWidth);
    resizeObserver.observe(parent);
    updateContainerWidth();
  }

  function resetWidth() {
    persistedWidth.value = DEFAULT_SIDEBAR_WIDTH;
  }

  function startResize(event: MouseEvent) {
    if (!containerWidth.value) {
      return;
    }
    event.preventDefault();
    resizing.value = true;
    const startX = event.clientX;
    const startWidth = width.value;

    function onMouseMove(moveEvent: MouseEvent) {
      const delta = startX - moveEvent.clientX;
      persistedWidth.value = Math.round(
        clamp(MIN_SIDEBAR_WIDTH, startWidth + delta, maxWidth.value)
      );
    }

    function onMouseUp() {
      resizing.value = false;
      document.removeEventListener('mousemove', onMouseMove);
      document.removeEventListener('mouseup', onMouseUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    }

    document.addEventListener('mousemove', onMouseMove);
    document.addEventListener('mouseup', onMouseUp);
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }

  watch(containerWidth, () => {
    if (!toValue(visible)) {
      return;
    }
    persistedWidth.value = clamp(MIN_SIDEBAR_WIDTH, persistedWidth.value, maxWidth.value);
  });

  watch(
    () => toValue(visible),
    isVisible => {
      if (!isVisible) {
        resizeObserver?.disconnect();
        resizeObserver = null;
        containerWidth.value = 0;
        return;
      }
      void nextTick(setupResizeObserver);
    },
    { immediate: true }
  );

  onScopeDispose(() => {
    resizeObserver?.disconnect();
    resizeObserver = null;
  });

  return {
    resizing,
    width,
    showStatusText,
    resetWidth,
    startResize,
  };
}
