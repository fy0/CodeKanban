// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { nextTick } from 'vue';
import VirtualSessionTabs from '../VirtualSessionTabs.vue';
import {
  SESSION_TAB_MIN_WIDTH,
  getSessionTabDropIndex,
  getSessionTabRange,
  getSessionTabScrollLeft,
  getSessionTabWidth,
} from '../virtualSessionTabs';

const { observe } = vi.hoisted(() => ({ observe: vi.fn() }));
vi.mock('@vueuse/core', () => ({ useResizeObserver: observe }));
let wrapper: VueWrapper;
const items = Array.from({ length: 500 }, (_, index) => ({
  id: `session-${index}`,
  title: `Session ${index}`,
}));

async function resize(viewport: HTMLElement, width: number) {
  Object.defineProperty(viewport, 'clientWidth', { value: width, configurable: true });
  observe.mock.calls.at(-1)![1]([{ contentRect: { width } }]);
  await nextTick();
}

async function create(activeId = items[0]!.id, sessionItems = items, width = 400) {
  wrapper = mount(VirtualSessionTabs, {
    props: {
      items: sessionItems,
      activeId,
      closeLabel: 'Close',
      tabProps: () => ({}),
      onSelect: (id: string) => {
        void wrapper.setProps({ activeId: id });
      },
    },
    slots: { default: '<span>{{ params.item.title }}</span>' },
    attachTo: document.body,
  });
  const viewport = wrapper.get('[role="tablist"]').element as HTMLElement;
  Object.defineProperty(viewport, 'scrollWidth', {
    get: () => parseFloat((wrapper.get('.virtual-tab-track').element as HTMLElement).style.width),
    configurable: true,
  });
  vi.spyOn(viewport, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 400, 40));
  await resize(viewport, width);
  return viewport;
}

beforeEach(() => observe.mockClear());
afterEach(() => {
  wrapper?.unmount();
  vi.restoreAllMocks();
});

describe('adaptive virtual session tabs', () => {
  it('adapts a small list to the viewport and count, then keeps large lists at the minimum width', async () => {
    const viewport = await create('session-0', items.slice(0, 3), 900);
    const widths = () =>
      wrapper.findAll('.virtual-tab-wrapper').map(tab => (tab.element as HTMLElement).style.width);
    expect(widths()).toEqual(['200px', '200px', '200px']);

    await resize(viewport, 500);
    expect(widths()).toEqual(['166px', '166px', '166px']);
    expect((wrapper.get('.virtual-tab-track').element as HTMLElement).style.width).toBe('498px');
    expect(
      (wrapper.findAll('.virtual-tab-wrapper')[2]!.element as HTMLElement).style.transform
    ).toBe('translateX(332px)');

    await wrapper.setProps({ items: items.slice(0, 4) });
    expect(widths().every(width => width === '140px')).toBe(true);
    await wrapper.setProps({ items });
    expect(widths().every(width => width === '140px')).toBe(true);
    await resize(viewport, 1200);
    expect(widths().every(width => width === '140px')).toBe(true);
    expect(widths().length).toBeLessThanOrEqual(11);
  });

  it('keeps the active tab visible after the viewport or tab count changes its width', async () => {
    const viewport = await create('session-2', items.slice(0, 3), 600);
    await resize(viewport, 400);
    expect(viewport.scrollLeft).toBe(20);
    await wrapper.setProps({ items: items.slice(0, 2), activeId: 'session-1' });
    expect(viewport.scrollLeft).toBe(0);
  });

  it('reorders using the current width while a small list is adapting', async () => {
    await create('session-0', items.slice(0, 3), 500);
    await wrapper.get('[data-session-id="session-0"]').trigger('dragstart');
    await wrapper.get('[role="tablist"]').trigger('drop', { clientX: 450 });
    expect(wrapper.emitted('reorder')).toEqual([[{ oldIndex: 0, newIndex: 2 }]]);
  });

  it('bounds rendered tabs by viewport and overscan even with 500 sessions', async () => {
    const viewport = await create();
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(5);
    viewport.scrollLeft = 2800;
    await wrapper.get('[role="tablist"]').trigger('scroll');
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(7);
    expect(wrapper.get('[role="tab"]').attributes('data-session-id')).toBe('session-18');
  });

  it('reveals an offscreen active session on mount and after project replacement', async () => {
    const viewport = await create('session-499');
    expect(viewport.scrollLeft).toBe(69600);
    expect(wrapper.get('[aria-selected="true"]').attributes('data-session-id')).toBe('session-499');
    await wrapper.setProps({
      items: [{ id: 'new-project', title: 'New project' }],
      activeId: 'new-project',
    });
    expect(viewport.scrollLeft).toBe(0);
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1);
  });

  it('navigates to offscreen tabs by keyboard and focuses the rendered target', async () => {
    await create();
    await wrapper.get('[role="tab"]').trigger('keydown', { key: 'End' });
    await nextTick();
    await nextTick();
    expect(wrapper.emitted('select')?.at(-1)).toEqual(['session-499']);
    expect(document.activeElement?.getAttribute('data-session-id')).toBe('session-499');
    await wrapper.get('[aria-selected="true"]').trigger('keydown', { key: 'Home' });
    await nextTick();
    expect(wrapper.get('[aria-selected="true"]').attributes('data-session-id')).toBe('session-0');
  });

  it('closes the requested session without selecting it', async () => {
    await create();
    await wrapper.get('.n-tabs-tab__close').trigger('click');
    expect(wrapper.emitted('close')).toEqual([['session-0']]);
    expect(wrapper.emitted('select')).toBeUndefined();
  });

  it('uses global indexes when dragging across virtual windows and retains the drag source', async () => {
    const viewport = await create('session-20');
    viewport.scrollLeft = 2800;
    await wrapper.get('[role="tablist"]').trigger('scroll');
    await wrapper.get('[data-session-id="session-20"]').trigger('dragstart');
    viewport.scrollLeft = 7000;
    await wrapper.get('[role="tablist"]').trigger('scroll');
    expect(wrapper.find('[data-session-id="session-20"]').exists()).toBe(true);
    expect(wrapper.findAll('[role="tab"]').length).toBeLessThanOrEqual(8);
    await wrapper.get('[role="tablist"]').trigger('drop', { clientX: 350 });
    expect(wrapper.emitted('reorder')).toEqual([[{ oldIndex: 20, newIndex: 52 }]]);
  });

  it('does not begin a reorder for locked tabs or external drops', async () => {
    await create();
    await wrapper.setProps({ isLocked: () => true });
    await wrapper.get('[role="tab"]').trigger('dragstart');
    await wrapper.get('[role="tablist"]').trigger('drop', { clientX: 350 });
    expect(wrapper.emitted('reorder')).toBeUndefined();
  });
});

describe('virtual tab geometry', () => {
  it.each([
    [0, 1000, 200],
    [3, 0, 200],
    [3, 900, 200],
    [3, 500, 166],
    [4, 500, 140],
    [50, 500, 140],
    [500, 1600, 140],
  ])('uses a bounded tab width for %i sessions in %ipx', (count, viewportWidth, expected) => {
    expect(getSessionTabWidth(count, viewportWidth)).toBe(expected);
  });

  it('clamps the range after a large project is replaced by an empty or short list', () => {
    expect(getSessionTabRange(0, 100000, 400)).toEqual({ start: 0, end: 0 });
    expect(getSessionTabRange(3, 100000, 400)).toEqual({ start: 2, end: 3 });
  });

  it('avoids scrolling a visible tab and reveals one outside either edge', () => {
    expect(getSessionTabScrollLeft(21, 4000, 400)).toBe(4000);
    expect(getSessionTabScrollLeft(19, 4000, 400)).toBe(3800);
    expect(getSessionTabScrollLeft(22, 4000, 400)).toBe(4200);
  });

  it('adjusts insertion indexes when moving in either direction or dropping at the ends', () => {
    expect(getSessionTabDropIndex(20, 4350, 500)).toBe(21);
    expect(getSessionTabDropIndex(20, 4010, 500)).toBe(20);
    expect(getSessionTabDropIndex(20, -20, 500)).toBe(0);
    expect(getSessionTabDropIndex(20, 200000, 500)).toBe(499);
  });

  it('uses the same bounded width for virtual ranges, active scrolling, and drop indexes', () => {
    expect(getSessionTabRange(500, 2800, 400, SESSION_TAB_MIN_WIDTH)).toEqual({
      start: 18,
      end: 25,
    });
    expect(getSessionTabScrollLeft(19, 2800, 400, SESSION_TAB_MIN_WIDTH)).toBe(2660);
    expect(getSessionTabScrollLeft(22, 2800, 400, SESSION_TAB_MIN_WIDTH)).toBe(2820);
    expect(getSessionTabDropIndex(20, 7350, 500, SESSION_TAB_MIN_WIDTH)).toBe(52);
  });
});
