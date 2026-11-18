// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import WebSessionPromptCache from '../WebSessionPromptCache.vue';
import zh from '@/i18n/locales/zh-CN';
import type { WebSessionTokenSpeed, WebSessionTokenSpeedRecord } from '@/types/models';

vi.mock('naive-ui', async () => {
  const { computed, defineComponent, h } = await import('vue');
  return {
    NPopover: defineComponent({
      name: 'NPopover',
      props: ['show', 'disabled', 'trigger', 'keepAliveOnHover'],
      setup(props, { slots }) {
        return () => h('div', [slots.trigger?.(), !props.disabled && slots.default?.()]);
      },
    }),
    NModal: defineComponent({
      name: 'NModal',
      props: ['show'],
      setup(props, { slots }) {
        return () => (props.show ? h('div', { role: 'dialog' }, slots.default?.()) : null);
      },
    }),
    useThemeVars: () =>
      computed(() => ({
        successColor: '#18a058',
        warningColor: '#a60',
        errorColor: '#d03050',
        textColor3: '#777',
        dividerColor: '#ddd',
      })),
  };
});
const observedAt = Date.parse('2026-11-18T00:00:00Z');
const session = {
  id: 'session',
  agent: 'codex' as const,
  model: 'gpt-6.1-sol',
  contextEstimateMode: 'latest_token_count' as const,
  contextEstimate: {
    inputTokens: 20_000,
    cachedInputTokens: 18_000,
    outputTokens: 100,
    usedTokens: 20_100,
  },
  latestTokenCountUpdatedAt: new Date(observedAt).toISOString(),
};
const record: WebSessionTokenSpeedRecord = {
  runId: 'run',
  sampleId: '1',
  agent: 'codex',
  model: session.model,
  startedAt: new Date(observedAt - 1000).toISOString(),
  endedAt: session.latestTokenCountUpdatedAt,
  inputTokens: 20_000,
  cachedInputTokens: 18_000,
  outputTokens: 100,
  durationMs: 1000,
  tokensPerSecond: 100,
  estimated: false,
};
function speed(history: WebSessionTokenSpeedRecord[]): WebSessionTokenSpeed {
  return {
    epoch: 'epoch',
    version: 1,
    runId: 'run',
    sampleId: '1',
    tokensPerSecond: 100,
    generating: false,
    estimated: false,
    updatedAt: session.latestTokenCountUpdatedAt,
    history,
  };
}
function render() {
  return mount(WebSessionPromptCache, {
    props: { session, now: observedAt + 60_000, speed: speed([record]) },
    global: {
      plugins: [createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } })],
    },
  });
}
afterEach(() => {
  vi.useRealTimers();
  window.innerWidth = 1280;
  window.dispatchEvent(new Event('resize'));
});

describe('WebSessionPromptCache', () => {
  it('keeps cache status separate from renewal modes and never queues on open', async () => {
    const wrapper = render();
    expect(wrapper.find('.prompt-cache-renewal').exists()).toBe(false);
    await wrapper.setProps({ renewal: { availability: 'ready', remaining: 0, infinite: false } });
    await wrapper.get('.prompt-cache-ring').trigger('click');
    expect(wrapper.emitted('renewalAction')).toBeUndefined();
    expect(wrapper.get('.prompt-cache-renewal').text()).toContain('自动续期已关闭');
    expect(wrapper.find('.prompt-cache-renewal-badge').exists()).toBe(false);
    const checkbox = wrapper.get('input[type="checkbox"]');
    await checkbox.setValue(true);
    expect(wrapper.emitted('renewalAction')).toEqual([['cycle']]);
    for (const remaining of [1, 2, 3]) {
      await wrapper.setProps({ renewal: { availability: 'ready', remaining, infinite: false } });
      expect(wrapper.get('.prompt-cache-renewal-badge').text()).toBe(String(remaining));
      await wrapper.get('.prompt-cache-renewal-action').trigger('click');
    }
    expect(wrapper.emitted('renewalAction')).toEqual([['cycle'], ['cycle'], ['cycle'], ['cycle']]);
    await wrapper.setProps({ renewal: { availability: 'ready', remaining: 0, infinite: true } });
    expect(wrapper.find('.prompt-cache-renewal-loop').exists()).toBe(true);
    expect(wrapper.get('.prompt-cache-ring').attributes('aria-label')).toContain(
      '无限续期正在运行'
    );
    expect(wrapper.get('.prompt-cache-stats').text()).toContain('90%');
    await wrapper.get('.prompt-cache-renewal-action').trigger('click');
    expect(wrapper.emitted('renewalAction')?.at(-1)).toEqual(['cycle']);
    await checkbox.setValue(false);
    expect(wrapper.emitted('renewalAction')?.at(-1)).toEqual(['off']);
    await wrapper.setProps({
      renewal: { availability: 'unsupported', remaining: 0, infinite: false },
    });
    expect(checkbox.attributes('disabled')).toBeDefined();
    expect(wrapper.get('.prompt-cache-renewal').text()).toContain('当前 Codex 不支持缓存续期');
    await wrapper.setProps({
      renewal: {
        availability: 'ready',
        remaining: 0,
        infinite: false,
        lastError: 'sample_timeout',
      },
    });
    expect(wrapper.find('.prompt-cache-renewal-badge').exists()).toBe(false);
    expect(wrapper.get('.prompt-cache-renewal-error').text()).toContain('最近一次续期失败');
    wrapper.unmount();
  });

  it('provides its own ring and details with a countdown, without animating hydration or clock ticks', async () => {
    const wrapper = render();
    expect(wrapper.get('.prompt-cache-ring').attributes('aria-label')).toContain('≈29:00');
    expect(wrapper.get('.prompt-cache-arc').attributes('stroke-dasharray')).toContain('96.');
    expect(wrapper.get('.prompt-cache-stats').text()).toContain('90%');
    expect(wrapper.getComponent({ name: 'NPopover' }).props()).toMatchObject({
      trigger: 'hover',
      keepAliveOnHover: true,
    });
    expect(wrapper.find('.is-refreshing').exists()).toBe(false);
    await wrapper.setProps({ now: observedAt + 61_000 });
    expect(wrapper.get('.prompt-cache-countdown').text()).toContain('≈28:59');
    expect(wrapper.find('.is-refreshing').exists()).toBe(false);
    await wrapper.setProps({ now: observedAt + 31 * 60_000 });
    expect(wrapper.get('.prompt-cache-countdown').text()).toContain('已过期 1 分钟');
    expect(wrapper.get('.prompt-cache-countdown').text()).toContain('估算保留窗口已过期');
    expect(wrapper.get('.prompt-cache-status').text()).toBe('缓存待确认');
    await wrapper.setProps({ now: observedAt + 91 * 60_000 });
    expect(wrapper.get('.prompt-cache-countdown').text()).toContain('1小时前');
    expect(wrapper.find('.is-interrupted').exists()).toBe(false);
    wrapper.unmount();
  });

  it('animates new requests and retains a complete rebuild across reloads', async () => {
    vi.useFakeTimers();
    const wrapper = render();
    const next = {
      ...record,
      sampleId: '2',
      endedAt: new Date(observedAt + 120_000).toISOString(),
    };
    await wrapper.setProps({ now: observedAt + 120_000, speed: speed([next, record]) });
    expect(wrapper.get('.prompt-cache-ring').classes()).toContain('is-refreshing');
    await vi.advanceTimersByTimeAsync(701);
    await wrapper.setProps({ now: observedAt + 121_000, speed: speed([next, record]) });
    expect(wrapper.find('.is-refreshing').exists()).toBe(false);
    const miss = {
      ...next,
      sampleId: '3',
      cachedInputTokens: 0,
      endedAt: new Date(observedAt + 180_000).toISOString(),
    };
    await wrapper.setProps({ now: observedAt + 180_000, speed: speed([miss, next, record]) });
    expect(wrapper.get('.prompt-cache-ring').classes()).toContain('state-rebuilt');
    expect(wrapper.get('.prompt-cache-status').text()).toBe('缓存重铸 0%');
    await wrapper.setProps({ session: { ...session, id: 'other' } });
    expect(wrapper.find('.is-interrupted').exists()).toBe(false);
    expect(wrapper.get('.prompt-cache-status').text()).toBe('缓存重铸 0%');
    wrapper.unmount();
  });

  it('opens an independent mobile panel and closes it on a session switch', async () => {
    window.innerWidth = 390;
    const wrapper = render();
    expect(wrapper.getComponent({ name: 'NPopover' }).props('disabled')).toBe(true);
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    await wrapper.get('.prompt-cache-ring').trigger('click');
    expect(wrapper.get('[role="dialog"]').text()).toContain('≈29:00');
    expect(wrapper.get('.prompt-cache-ring').attributes('aria-expanded')).toBe('true');
    await wrapper.setProps({ session: { ...session, id: 'other' } });
    await nextTick();
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    wrapper.unmount();
  });
  it('uses separate ring states for rebuilds and suspected rebuilds', async () => {
    const wrapper = render();
    const partial = {
      ...record,
      sampleId: '2',
      cachedInputTokens: 1000,
      endedAt: new Date(observedAt + 120_000).toISOString(),
    };
    await wrapper.setProps({ now: observedAt + 120_000, speed: speed([partial, record]) });
    expect(wrapper.get('.prompt-cache-ring').classes()).toContain('state-rebuilt');
    expect(wrapper.get('.prompt-cache-status').text()).toBe('缓存重铸 5%');
    expect(wrapper.get('.prompt-cache-stats').text()).toContain('1,000 / 20,000');
    expect(wrapper.get('.prompt-cache-decrease-note').text()).toContain('18,000 降至 1,000');
    expect(wrapper.get('.prompt-cache-alert').attributes('d')).toContain('a8');
    await wrapper.setProps({ session: { ...session, id: 'reloaded' } });
    expect(wrapper.get('.prompt-cache-status').text()).toBe('缓存重铸 5%');
    await wrapper.setProps({ speed: speed([{ ...partial, cachedInputTokens: 17_000 }, record]) });
    expect(wrapper.get('.prompt-cache-ring').classes()).toContain('state-suspected');
    expect(wrapper.get('.prompt-cache-status').text()).toBe('疑似重铸 85%');
    expect(wrapper.get('.prompt-cache-decrease-note').text()).toContain('18,000 降至 17,000');
    wrapper.unmount();
  });
});
