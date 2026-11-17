// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import WebSessionTokenSpeed from '../WebSessionTokenSpeed.vue';
import en from '@/i18n/locales/en-US';
import zh from '@/i18n/locales/zh-CN';
import type { WebSessionTokenSpeed as TokenSpeed } from '@/types/models';

vi.mock('naive-ui', async () => {
  const { computed, defineComponent, h } = await import('vue');
  return {
    NPopover: defineComponent({
      name: 'NPopover',
      props: ['trigger', 'placement', 'showArrow', 'keepAliveOnHover', 'disabled'],
      setup(props, { slots }) {
        return () => h('div', [slots.trigger?.(), !props.disabled && slots.default?.()]);
      },
    }),
    NModal: defineComponent({
      name: 'NModal',
      props: ['show', 'preset', 'title', 'contentStyle'],
      emits: ['update:show'],
      setup(props, { slots }) {
        return () => (props.show ? h('div', { role: 'dialog' }, slots.default?.()) : null);
      },
    }),
    useThemeVars: () =>
      computed(() => ({
        primaryColor: '#18a058',
        textColor2: '#333',
        textColor3: '#777',
        dividerColor: '#ddd',
      })),
  };
});

function makeSpeed(): TokenSpeed {
  const summary = {
    tokensPerSecond: 50,
    outputTokens: 200,
    durationMs: 4000,
    sampleCount: 2,
    estimated: false,
  };
  return {
    epoch: 'process',
    version: 1,
    runId: 'run',
    sampleId: '2',
    tokensPerSecond: 100,
    estimated: true,
    generating: true,
    updatedAt: '',
    stats: {
      currentRun: { ...summary, tokensPerSecond: 60 },
      recent5: summary,
      recent20: summary,
      session: { ...summary, tokensPerSecond: 40 },
    },
    history: [1, 2].map(sampleId => ({
      runId: 'run',
      sampleId: String(sampleId),
      agent: 'codex',
      model: 'test-model',
      startedAt: '2026-10-19T00:00:00Z',
      endedAt: '2026-10-19T00:00:04Z',
      outputTokens: 200,
      durationMs: 4000,
      firstTokenMs: sampleId === 1 ? 1250 : null,
      tokensPerSecond: 50,
      estimated: false,
    })),
  };
}

function render(speed?: TokenSpeed, locale = 'en-US') {
  return mount(WebSessionTokenSpeed, {
    props: { speed },
    global: {
      plugins: [createI18n({ legacy: false, locale, messages: { 'en-US': en, 'zh-CN': zh } })],
    },
  });
}

describe('WebSessionTokenSpeed', () => {
  it('marks observed cache refreshes and interruptions inline, leaving unknown records unmarked', () => {
    const speed = makeSpeed();
    speed.history = [
      { ...speed.history![0]!, sampleId: '3', inputTokens: 20_000, cachedInputTokens: 0 },
      { ...speed.history![0]!, sampleId: '2', inputTokens: 20_000, cachedInputTokens: 18_000 },
      { ...speed.history![1]!, sampleId: '1' },
    ];
    const wrapper = render(speed, 'zh-CN');
    const rows = wrapper.findAll('.token-speed-record');
    expect(rows[0]!.get('.token-speed-cache-marker').classes()).toContain('cache-interrupted');
    expect(rows[0]!.get('.token-speed-cache-marker').attributes('title')).toContain('命中中断');
    expect(rows[1]!.get('.token-speed-cache-marker').text()).toBe('90%');
    expect(rows[1]!.get('.token-speed-cache-marker').attributes('title')).toContain(
      '18,000 / 20,000'
    );
    expect(rows[2]!.find('.token-speed-cache-marker').exists()).toBe(false);
    wrapper.unmount();
  });
  const originalWidth = window.innerWidth;
  afterEach(() => {
    window.innerWidth = originalWidth;
    window.dispatchEvent(new Event('resize'));
  });

  it('opens a dialog on mobile and preserves the selected metric when it closes', async () => {
    window.innerWidth = 390;
    const wrapper = render(makeSpeed(), 'zh-CN');
    expect(wrapper.getComponent({ name: 'NPopover' }).props('disabled')).toBe(true);
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    expect(wrapper.get('.token-speed').attributes('aria-haspopup')).toBe('dialog');
    await wrapper.get('.token-speed').trigger('click');
    expect(wrapper.get('.token-speed').attributes('aria-expanded')).toBe('true');
    expect(wrapper.get('[role="dialog"]').findAll('.token-speed-record')).toHaveLength(2);
    const sessionButton = wrapper
      .findAll('.token-speed-metric')
      .find(button => button.text().includes('会话均速'))!;
    await sessionButton.trigger('click');
    expect(wrapper.get('.token-speed').text()).toBe('40 t/s');
    wrapper.getComponent({ name: 'NModal' }).vm.$emit('update:show', false);
    await nextTick();
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    await wrapper.get('.token-speed').trigger('click');
    expect(wrapper.find('.token-speed-metric.selected').text()).toContain('会话均速');
    window.innerWidth = 1280;
    window.dispatchEvent(new Event('resize'));
    await nextTick();
    expect(wrapper.findComponent({ name: 'NModal' }).exists()).toBe(false);
    expect(wrapper.getComponent({ name: 'NPopover' }).props('disabled')).toBe(false);
    wrapper.unmount();
  });

  it('shows first token latency beside the model in seconds with one decimal', async () => {
    const speed = makeSpeed();
    const wrapper = render(speed, 'zh-CN');
    const records = wrapper.findAll('.token-speed-record');
    expect(records[0]!.get('.token-speed-record-meta').text()).toContain('test-model');
    expect(records[0]!.get('.token-speed-first-token').text()).toBe('首字 1.3 s');
    expect(records[1]!.get('.token-speed-first-token').text()).toBe('首字 -- s');
    await wrapper.setProps({
      speed: {
        ...speed,
        history: speed.history!.map(record => ({ ...record, firstTokenMs: 0 })),
      },
    });
    expect(wrapper.get('.token-speed-first-token').text()).toBe('首字 0.0 s');
    wrapper.unmount();
  });

  it('defaults to the recent average, switches dimensions and keeps history on a new run', async () => {
    const speed = makeSpeed();
    const wrapper = render(speed);
    expect(wrapper.get('.token-speed').attributes('aria-label')).toContain('Last 5 50 t/s');
    expect(wrapper.get('.token-speed').text()).toBe('50 t/s');
    expect(wrapper.getComponent({ name: 'NPopover' }).props()).toMatchObject({
      trigger: 'hover',
      keepAliveOnHover: true,
    });
    expect(wrapper.findAll('.token-speed-record')).toHaveLength(2);
    expect(wrapper.find('svg').attributes('aria-label')).toContain('last 20');
    const sessionButton = wrapper
      .findAll('.token-speed-metric')
      .find(button => button.text().includes('Session average'))!;
    await sessionButton.trigger('click');
    expect(sessionButton.attributes('aria-pressed')).toBe('true');
    expect(wrapper.get('.token-speed').attributes('aria-label')).toContain(
      'Session average 40 t/s'
    );
    expect(wrapper.get('.token-speed').text()).toBe('40 t/s');
    await wrapper.setProps({
      speed: { ...speed, version: 2, runId: 'next-run', sampleId: '', tokensPerSecond: null },
    });
    expect(wrapper.get('.token-speed').attributes('aria-label')).toContain(
      'Session average 40 t/s'
    );
    expect(wrapper.findAll('.token-speed-record')).toHaveLength(2);
    wrapper.unmount();
  });

  it('shows an empty state and supports older telemetry without statistics', async () => {
    const wrapper = render();
    expect(wrapper.get('.token-speed').text()).toContain('-- t/s');
    expect(wrapper.text()).toContain('saved automatically');
    expect(wrapper.find('svg').exists()).toBe(false);
    await wrapper.setProps({ speed: { ...makeSpeed(), stats: undefined } });
    expect(wrapper.get('.token-speed').text()).toBe('100 t/s');
    wrapper.unmount();
  });
});
