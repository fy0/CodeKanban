// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { defineComponent, h, onMounted, onUnmounted, reactive } from 'vue';
import EventEmitter from 'eventemitter3';
import type { TerminalTabState } from '@/stores/terminal';
import TerminalViewportPool from '../TerminalViewportPool.vue';

const mounted = vi.fn();
const unmounted = vi.fn();
const Viewport = defineComponent({
  props: ['tab', 'shouldAutoFocus', 'emitter', 'send', 'isMobile'],
  setup(props) {
    onMounted(() => mounted(props.tab.id));
    onUnmounted(() => unmounted(props.tab.id));
    return () => h('div', { 'data-id': props.tab.id }, props.tab.title);
  },
});

let wrapper: VueWrapper;
function create(isActive = true, mode: 'live' | 'snapshot' = 'snapshot') {
  const tabs = reactive(
    Array.from(
      { length: 500 },
      (_, i) =>
        ({
          id: `t-${i}`,
          projectId: 'p1',
          title: `Terminal ${i}`,
          renderMode: mode,
          connectionRenderMode: mode,
        }) as TerminalTabState
    )
  );
  wrapper = mount(TerminalViewportPool, {
    props: {
      projectId: 'p1',
      tabs,
      activeId: 't-0',
      isActive,
      emitter: new EventEmitter(),
      send: vi.fn(),
      shouldAutoFocus: true,
    },
    global: { stubs: { TerminalViewport: Viewport } },
  });
  return tabs;
}

afterEach(() => {
  wrapper?.unmount();
  mounted.mockClear();
  unmounted.mockClear();
});

describe('terminal viewport working set', () => {
  it('creates no renderer for an unopened pane and only the selected renderer on first activation', async () => {
    create(false);
    expect(mounted).not.toHaveBeenCalled();
    await wrapper.setProps({ isActive: true });
    expect(mounted.mock.calls).toEqual([['t-0']]);
    expect(wrapper.findAllComponents(Viewport)).toHaveLength(1);
  });

  it('bounds the working set to three renderers across hundreds of tab selections', async () => {
    create();
    for (let i = 1; i < 100; i++) {
      await wrapper.setProps({ activeId: `t-${i}` });
      expect(wrapper.findAllComponents(Viewport).length).toBeLessThanOrEqual(3);
    }
    expect(wrapper.findAllComponents(Viewport).map(view => view.props('tab').id)).toEqual([
      't-99',
      't-98',
      't-97',
    ]);
    expect(unmounted).toHaveBeenCalledTimes(97);
    expect(
      wrapper.findAllComponents(Viewport).filter(view => view.props('shouldAutoFocus'))
    ).toHaveLength(1);
    await wrapper.setProps({ activeId: 't-98' });
    expect(mounted).toHaveBeenCalledTimes(100);
    await wrapper.setProps({ activeId: 't-0' });
    expect(mounted).toHaveBeenCalledTimes(101);
  });

  it('prunes closed sessions, handles empty tabs, and discards the old project working set', async () => {
    const tabs = create();
    await wrapper.setProps({ activeId: 't-1' });
    tabs.splice(0, 1);
    await wrapper.setProps({ activeId: 'empty-1' });
    expect(wrapper.findAllComponents(Viewport)).toHaveLength(1);
    expect((wrapper.findComponent(Viewport).element as HTMLElement).style.display).toBe('none');
    await wrapper.setProps({
      projectId: 'p2',
      tabs: [{ id: 'other', title: 'Other' } as TerminalTabState],
      activeId: 'other',
      isActive: false,
    });
    expect(wrapper.findAllComponents(Viewport)).toHaveLength(0);
    await wrapper.setProps({ isActive: true });
    expect(wrapper.findAllComponents(Viewport).map(view => view.props('tab').id)).toEqual([
      'other',
    ]);
  });

  it('preserves renderer identity when metadata changes or the pane is hidden', async () => {
    const tabs = create();
    tabs[0]!.title = 'Renamed';
    await wrapper.setProps({ isActive: false, activeId: 't-499' });
    expect(mounted).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain('Renamed');
    expect(wrapper.findComponent(Viewport).props('shouldAutoFocus')).toBe(false);
    await wrapper.setProps({ isActive: true });
    expect(mounted.mock.calls).toEqual([['t-0'], ['t-499']]);
  });

  it('keeps visited live screens when history replay is unavailable, then bounds the cache when enabled', async () => {
    create(true, 'live');
    for (let i = 1; i < 6; i++) await wrapper.setProps({ activeId: `t-${i}` });
    expect(wrapper.findAllComponents(Viewport)).toHaveLength(6);
    expect(unmounted).not.toHaveBeenCalled();
    await wrapper.setProps({ activeId: 't-0' });
    expect(mounted).toHaveBeenCalledTimes(6);
    await wrapper.setProps({ historyReplayEnabled: true });
    expect(wrapper.findAllComponents(Viewport)).toHaveLength(3);
    expect(wrapper.findAllComponents(Viewport).map(view => view.props('tab').id)).toEqual([
      't-0',
      't-5',
      't-4',
    ]);
  });

  it('does not evict a snapshot preference that actually fell back to a live stream', async () => {
    const tabs = create();
    tabs[0]!.connectionRenderMode = 'live';
    for (let i = 1; i < 6; i++) await wrapper.setProps({ activeId: `t-${i}` });
    expect(wrapper.findAllComponents(Viewport).map(view => view.props('tab').id)).toEqual([
      't-5',
      't-4',
      't-0',
    ]);
    expect(unmounted).not.toHaveBeenCalledWith('t-0');
  });
});
