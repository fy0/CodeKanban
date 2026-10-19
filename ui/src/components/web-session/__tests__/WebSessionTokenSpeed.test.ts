// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { describe, expect, it, vi } from 'vitest';
import WebSessionTokenSpeed from '../WebSessionTokenSpeed.vue';
import en from '@/i18n/locales/en-US';
import type { WebSessionTokenSpeed as TokenSpeed } from '@/types/models';

vi.mock('naive-ui', async () => {
  const { computed, defineComponent, h } = await import('vue');
  return {
    NPopover: defineComponent({
      setup(_, { slots }) {
        return () => h('div', [slots.trigger?.(), slots.default?.()]);
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
      tokensPerSecond: 50,
      estimated: false,
    })),
  };
}

function render(speed?: TokenSpeed) {
  return mount(WebSessionTokenSpeed, {
    props: { speed },
    global: {
      plugins: [createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': en } })],
    },
  });
}

describe('WebSessionTokenSpeed', () => {
  it('defaults to the recent average, switches dimensions and keeps history on a new run', async () => {
    const speed = makeSpeed();
    const wrapper = render(speed);
    expect(wrapper.get('.token-speed').attributes('aria-label')).toContain('Last 5 50 t/s');
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
    await wrapper.setProps({
      speed: { ...speed, version: 2, runId: 'next-run', sampleId: '', tokensPerSecond: null },
    });
    expect(wrapper.get('.token-speed').attributes('aria-label')).toContain(
      'Session average 40 t/s'
    );
    expect(wrapper.findAll('.token-speed-record')).toHaveLength(2);
    wrapper.unmount();
  });

  it('shows an empty state and supports older telemetry without statistics', () => {
    const wrapper = render();
    expect(wrapper.get('.token-speed').text()).toContain('-- t/s');
    expect(wrapper.text()).toContain('saved automatically');
    expect(wrapper.find('svg').exists()).toBe(false);
    wrapper.unmount();
  });
});
