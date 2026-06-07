export type CardTabIndicatorStyle = {
  transform: string;
  width: string;
  opacity: string;
};

export type EnsureActiveCardTabVisibleOptions = {
  padding?: number;
  behavior?: ScrollBehavior;
};

const HIDDEN_STYLE: CardTabIndicatorStyle = {
  transform: 'translateX(0px)',
  width: '0px',
  opacity: '0',
};
const DEFAULT_ACTIVE_TAB_SCROLL_PADDING = 16;

export function hiddenCardTabIndicatorStyle(): CardTabIndicatorStyle {
  return { ...HIDDEN_STYLE };
}

export function calculateCardTabIndicatorStyle(
  container: HTMLElement | null
): CardTabIndicatorStyle {
  if (!container) {
    return hiddenCardTabIndicatorStyle();
  }

  const wrapper = container.querySelector('.n-tabs-wrapper') as HTMLElement | null;
  if (!wrapper) {
    return hiddenCardTabIndicatorStyle();
  }

  const activeTabElement = wrapper.querySelector(
    '.n-tabs-tab.n-tabs-tab--active'
  ) as HTMLElement | null;
  if (!activeTabElement) {
    return hiddenCardTabIndicatorStyle();
  }

  const wrapperRect = wrapper.getBoundingClientRect();
  const activeRect = activeTabElement.getBoundingClientRect();
  const tabWidth = activeRect.width;

  let indicatorWidth: number;
  if (tabWidth > 150) {
    indicatorWidth = tabWidth * 0.35;
  } else if (tabWidth > 100) {
    indicatorWidth = tabWidth * 0.45;
  } else if (tabWidth > 60) {
    indicatorWidth = tabWidth * 0.6;
  } else {
    indicatorWidth = tabWidth * 0.75;
  }
  indicatorWidth = Math.max(20, Math.min(80, indicatorWidth));

  const scrollContainer = container.querySelector('.v-x-scroll') as HTMLElement | null;
  const scrollLeft = scrollContainer?.scrollLeft ?? 0;
  const offsetLeft =
    activeRect.left - wrapperRect.left - scrollLeft + (tabWidth - indicatorWidth) / 2;

  return {
    transform: `translateX(${offsetLeft}px)`,
    width: `${indicatorWidth}px`,
    opacity: '1',
  };
}

function resolveScrollableTarget(container: HTMLElement, target: number): number {
  if (container.scrollWidth > 0 && container.clientWidth > 0) {
    const maxScrollLeft = Math.max(0, container.scrollWidth - container.clientWidth);
    return Math.min(maxScrollLeft, Math.max(0, target));
  }
  return Math.max(0, target);
}

export function ensureActiveCardTabVisible(
  container: HTMLElement | null,
  options: EnsureActiveCardTabVisibleOptions = {}
): boolean {
  if (!container) {
    return false;
  }

  const scrollContainer = container.querySelector('.v-x-scroll') as HTMLElement | null;
  if (!scrollContainer) {
    return false;
  }

  const activeTabElement = container.querySelector(
    '.n-tabs-tab.n-tabs-tab--active'
  ) as HTMLElement | null;
  if (!activeTabElement) {
    return false;
  }

  const padding = Math.max(0, options.padding ?? DEFAULT_ACTIVE_TAB_SCROLL_PADDING);
  const scrollRect = scrollContainer.getBoundingClientRect();
  const activeRect = activeTabElement.getBoundingClientRect();
  const visibleLeft = scrollRect.left + padding;
  const visibleRight = scrollRect.right - padding;

  let delta = 0;
  if (activeRect.left < visibleLeft) {
    delta = activeRect.left - visibleLeft;
  } else if (activeRect.right > visibleRight) {
    delta = activeRect.right - visibleRight;
  }

  if (Math.abs(delta) < 1) {
    return false;
  }

  const currentScrollLeft = scrollContainer.scrollLeft;
  const nextScrollLeft = resolveScrollableTarget(scrollContainer, currentScrollLeft + delta);
  if (Math.abs(nextScrollLeft - currentScrollLeft) < 1) {
    return false;
  }

  if (typeof scrollContainer.scrollTo === 'function') {
    scrollContainer.scrollTo({
      left: nextScrollLeft,
      behavior: options.behavior ?? 'auto',
    });
  } else {
    scrollContainer.scrollLeft = nextScrollLeft;
  }
  return true;
}
