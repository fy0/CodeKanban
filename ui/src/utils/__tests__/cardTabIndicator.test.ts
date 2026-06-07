import { describe, expect, it } from 'vitest';

import {
  calculateCardTabIndicatorStyle,
  ensureActiveCardTabVisible,
  hiddenCardTabIndicatorStyle,
} from '@/utils/cardTabIndicator';

type MockElementOptions = {
  rect?: Partial<DOMRect>;
  scrollLeft?: number;
  scrollWidth?: number;
  clientWidth?: number;
  className?: string;
  children?: MockElement[];
};

class MockElement {
  className: string;
  scrollLeft: number;
  scrollWidth: number;
  clientWidth: number;
  scrollToCalls: ScrollToOptions[];
  private rect: DOMRect;
  private children: MockElement[];

  constructor(options: MockElementOptions = {}) {
    this.className = options.className ?? '';
    this.scrollLeft = options.scrollLeft ?? 0;
    this.scrollWidth = options.scrollWidth ?? 0;
    this.clientWidth = options.clientWidth ?? 0;
    this.scrollToCalls = [];
    const rect = {
      x: 0,
      y: 0,
      left: 0,
      right: 0,
      top: 0,
      bottom: 0,
      width: 0,
      height: 0,
      toJSON: () => ({}),
      ...options.rect,
    };
    if (options.rect?.right === undefined && typeof rect.left === 'number') {
      rect.right = rect.left + rect.width;
    }
    this.rect = rect as DOMRect;
    this.children = options.children ?? [];
  }

  querySelector(selector: string): MockElement | null {
    const normalizedSelector = selector
      .split('.')
      .map(part => part.trim())
      .filter(Boolean);

    return this.findChild(element => {
      const classes = element.className.split(/\s+/).filter(Boolean);
      return normalizedSelector.every(className => classes.includes(className));
    });
  }

  getBoundingClientRect(): DOMRect {
    return this.rect;
  }

  scrollTo(options: ScrollToOptions): void {
    this.scrollToCalls.push(options);
    if (typeof options.left === 'number') {
      this.scrollLeft = options.left;
    }
  }

  private findChild(predicate: (element: MockElement) => boolean): MockElement | null {
    for (const child of this.children) {
      if (predicate(child)) {
        return child;
      }
      const nested = child.findChild(predicate);
      if (nested) {
        return nested;
      }
    }
    return null;
  }
}

describe('cardTabIndicator', () => {
  it('returns a hidden style when required tab elements are missing', () => {
    expect(calculateCardTabIndicatorStyle(null)).toEqual(hiddenCardTabIndicatorStyle());
    expect(calculateCardTabIndicatorStyle(new MockElement() as unknown as HTMLElement)).toEqual(
      hiddenCardTabIndicatorStyle()
    );
  });

  it('positions the indicator under the active tab with scroll offset applied', () => {
    const activeTab = new MockElement({
      className: 'n-tabs-tab n-tabs-tab--active',
      rect: { left: 220, width: 120 },
    });
    const wrapper = new MockElement({
      className: 'n-tabs-wrapper',
      rect: { left: 40, width: 480 },
      children: [activeTab],
    });
    const scrollContainer = new MockElement({
      className: 'v-x-scroll',
      scrollLeft: 30,
    });
    const container = new MockElement({
      children: [wrapper, scrollContainer],
    });

    expect(calculateCardTabIndicatorStyle(container as unknown as HTMLElement)).toEqual({
      transform: 'translateX(183px)',
      width: '54px',
      opacity: '1',
    });
  });

  it('clamps indicator width for wide active tabs', () => {
    const activeTab = new MockElement({
      className: 'n-tabs-tab n-tabs-tab--active',
      rect: { left: 160, width: 260 },
    });
    const wrapper = new MockElement({
      className: 'n-tabs-wrapper',
      rect: { left: 100 },
      children: [activeTab],
    });
    const container = new MockElement({
      children: [wrapper],
    });

    expect(calculateCardTabIndicatorStyle(container as unknown as HTMLElement)).toEqual({
      transform: 'translateX(150px)',
      width: '80px',
      opacity: '1',
    });
  });

  it('does not scroll when active tab is already visible', () => {
    const activeTab = new MockElement({
      className: 'n-tabs-tab n-tabs-tab--active',
      rect: { left: 96, right: 180, width: 84 },
    });
    const wrapper = new MockElement({
      className: 'n-tabs-wrapper',
      children: [activeTab],
    });
    const scrollContainer = new MockElement({
      className: 'v-x-scroll',
      scrollLeft: 120,
      scrollWidth: 800,
      clientWidth: 320,
      rect: { left: 0, right: 320, width: 320 },
      children: [wrapper],
    });
    const container = new MockElement({
      children: [scrollContainer],
    });

    expect(ensureActiveCardTabVisible(container as unknown as HTMLElement)).toBe(false);
    expect(scrollContainer.scrollLeft).toBe(120);
    expect(scrollContainer.scrollToCalls).toHaveLength(0);
  });

  it('scrolls right when active tab overflows past the visible area', () => {
    const activeTab = new MockElement({
      className: 'n-tabs-tab n-tabs-tab--active',
      rect: { left: 260, right: 380, width: 120 },
    });
    const wrapper = new MockElement({
      className: 'n-tabs-wrapper',
      children: [activeTab],
    });
    const scrollContainer = new MockElement({
      className: 'v-x-scroll',
      scrollLeft: 100,
      scrollWidth: 1000,
      clientWidth: 300,
      rect: { left: 0, right: 300, width: 300 },
      children: [wrapper],
    });
    const container = new MockElement({
      children: [scrollContainer],
    });

    expect(ensureActiveCardTabVisible(container as unknown as HTMLElement)).toBe(true);
    expect(scrollContainer.scrollLeft).toBe(196);
    expect(scrollContainer.scrollToCalls).toEqual([{ left: 196, behavior: 'auto' }]);
  });

  it('scrolls left when active tab is before the visible area', () => {
    const activeTab = new MockElement({
      className: 'n-tabs-tab n-tabs-tab--active',
      rect: { left: -24, right: 80, width: 104 },
    });
    const wrapper = new MockElement({
      className: 'n-tabs-wrapper',
      children: [activeTab],
    });
    const scrollContainer = new MockElement({
      className: 'v-x-scroll',
      scrollLeft: 200,
      scrollWidth: 1000,
      clientWidth: 300,
      rect: { left: 0, right: 300, width: 300 },
      children: [wrapper],
    });
    const container = new MockElement({
      children: [scrollContainer],
    });

    expect(ensureActiveCardTabVisible(container as unknown as HTMLElement)).toBe(true);
    expect(scrollContainer.scrollLeft).toBe(160);
    expect(scrollContainer.scrollToCalls).toEqual([{ left: 160, behavior: 'auto' }]);
  });

  it('keeps scroll unchanged when active tab elements are missing', () => {
    const scrollContainer = new MockElement({
      className: 'v-x-scroll',
      scrollLeft: 80,
      scrollWidth: 600,
      clientWidth: 240,
      rect: { left: 0, right: 240, width: 240 },
    });
    const container = new MockElement({
      children: [scrollContainer],
    });

    expect(ensureActiveCardTabVisible(null)).toBe(false);
    expect(ensureActiveCardTabVisible(container as unknown as HTMLElement)).toBe(false);
    expect(scrollContainer.scrollLeft).toBe(80);
    expect(scrollContainer.scrollToCalls).toHaveLength(0);
  });
});
