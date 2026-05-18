import { describe, expect, it } from 'vitest';

import {
  calculateCardTabIndicatorStyle,
  hiddenCardTabIndicatorStyle,
} from '@/utils/cardTabIndicator';

type MockElementOptions = {
  rect?: Partial<DOMRect>;
  scrollLeft?: number;
  className?: string;
  children?: MockElement[];
};

class MockElement {
  className: string;
  scrollLeft: number;
  private rect: DOMRect;
  private children: MockElement[];

  constructor(options: MockElementOptions = {}) {
    this.className = options.className ?? '';
    this.scrollLeft = options.scrollLeft ?? 0;
    this.rect = {
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
    } as DOMRect;
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
});
