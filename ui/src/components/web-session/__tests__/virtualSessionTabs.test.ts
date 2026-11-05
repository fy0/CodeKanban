// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { nextTick } from 'vue';
import VirtualSessionTabs from '../VirtualSessionTabs.vue';
import {
  getSessionTabDropIndex,
  getSessionTabRange,
  getSessionTabScrollLeft,
} from '../virtualSessionTabs';

const { observe } = vi.hoisted(() => ({ observe: vi.fn() }));
vi.mock('@vueuse/core', () => ({ useResizeObserver: observe }));
let wrapper: VueWrapper;
const items = Array.from({ length: 500 }, (_, index) => ({
  id: `session-${index}`,
  title: `Session ${index}`,
}));

async function create(activeId = items[0]!.id) {
  wrapper = mount(VirtualSessionTabs, {
    props: {
      items,
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
  Object.defineProperty(viewport, 'clientWidth', { value: 400, configurable: true });
  Object.defineProperty(viewport, 'scrollWidth', {
    get: () => items.length * 200,
    configurable: true,
  });
  vi.spyOn(viewport, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 400, 40));
  observe.mock.calls.at(-1)![1]([{ contentRect: { width: 400 } }]);
  await nextTick();
  return viewport;
}

beforeEach(() => observe.mockClear());
afterEach(() => {
  wrapper?.unmount();
  vi.restoreAllMocks();
});

describe('fixed-width virtual session tabs', () => {
  it('bounds rendered tabs by viewport and overscan even with 500 sessions', async () => {
    const viewport = await create();
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(4);
    viewport.scrollLeft = 4000;
    await wrapper.get('[role="tablist"]').trigger('scroll');
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(6);
    expect(wrapper.get('[role="tab"]').attributes('data-session-id')).toBe('session-18');
  });

  it('reveals an offscreen active session on mount and after project replacement', async () => {
    const viewport = await create('session-499');
    expect(viewport.scrollLeft).toBe(99600);
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
    viewport.scrollLeft = 4000;
    await wrapper.get('[role="tablist"]').trigger('scroll');
    await wrapper.get('[data-session-id="session-20"]').trigger('dragstart');
    viewport.scrollLeft = 10000;
    await wrapper.get('[role="tablist"]').trigger('scroll');
    expect(wrapper.find('[data-session-id="session-20"]').exists()).toBe(true);
    expect(wrapper.findAll('[role="tab"]').length).toBeLessThanOrEqual(7);
    await wrapper.get('[role="tablist"]').trigger('drop', { clientX: 350 });
    expect(wrapper.emitted('reorder')).toEqual([[{ oldIndex: 20, newIndex: 51 }]]);
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
});
