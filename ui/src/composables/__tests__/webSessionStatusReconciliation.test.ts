// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { defineComponent, h, nextTick, ref } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  useWebSessionStatusReconciliation,
  WebSessionStatusReconcileLoop,
} from '@/composables/useWebSessionStatusReconciliation';

const { storeMock } = vi.hoisted(() => ({
  storeMock: {
    hasReconcilePrioritySessions: false,
    reconcileRecentSessions: vi.fn(),
  },
}));

vi.mock('@/stores/webSession', () => ({
  useWebSessionStore: () => storeMock,
}));

describe('WebSessionStatusReconcileLoop', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    storeMock.hasReconcilePrioritySessions = false;
    storeMock.reconcileRecentSessions.mockReset();
    storeMock.reconcileRecentSessions.mockResolvedValue({ items: [], missingIds: [] });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it('reconciles within 30 seconds and stops when disabled', async () => {
    const reconcile = vi.fn().mockResolvedValue(undefined);
    const loop = new WebSessionStatusReconcileLoop({
      enabled: () => true,
      reconcile,
      random: () => 1,
    });

    loop.start();
    await vi.advanceTimersByTimeAsync(29_999);
    expect(reconcile).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    expect(reconcile).toHaveBeenCalledOnce();

    loop.stop();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(reconcile).toHaveBeenCalledOnce();
  });

  it('pauses while hidden and reconciles immediately when visibility returns', async () => {
    let hidden = true;
    const reconcile = vi.fn().mockResolvedValue(undefined);
    const loop = new WebSessionStatusReconcileLoop({
      enabled: () => true,
      reconcile,
      isHidden: () => hidden,
      random: () => 1,
    });

    loop.start();
    await vi.advanceTimersByTimeAsync(2 * 60 * 60 * 1000);
    expect(reconcile).not.toHaveBeenCalled();

    hidden = false;
    loop.handleVisibilityChange();
    await vi.advanceTimersByTimeAsync(0);
    expect(reconcile).toHaveBeenCalledOnce();
  });

  it('does not overlap requests and retries after a failure', async () => {
    let resolveFirst!: () => void;
    const firstRequest = new Promise<void>(resolve => {
      resolveFirst = resolve;
    });
    const reconcile = vi
      .fn()
      .mockReturnValueOnce(firstRequest)
      .mockRejectedValueOnce(new Error('temporary failure'))
      .mockResolvedValue(undefined);
    const onError = vi.fn();
    const loop = new WebSessionStatusReconcileLoop({
      enabled: () => true,
      reconcile,
      onError,
      random: () => 1,
    });

    loop.start({ immediate: true });
    loop.trigger();
    expect(reconcile).toHaveBeenCalledOnce();

    resolveFirst();
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(reconcile).toHaveBeenCalledTimes(2);
    expect(onError).toHaveBeenCalledOnce();

    await vi.advanceTimersByTimeAsync(30_000);
    expect(reconcile).toHaveBeenCalledTimes(3);
  });

  it('wires visibility and focus recovery once for the application lifetime', async () => {
    storeMock.hasReconcilePrioritySessions = true;
    const enabled = ref(true);
    let visibilityState: DocumentVisibilityState = 'hidden';
    vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visibilityState);
    const component = defineComponent({
      setup() {
        useWebSessionStatusReconciliation(enabled);
        return () => h('div');
      },
    });
    const wrapper = mount(component);

    await vi.advanceTimersByTimeAsync(2 * 60 * 60 * 1000);
    expect(storeMock.reconcileRecentSessions).not.toHaveBeenCalled();

    visibilityState = 'visible';
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(0);
    expect(storeMock.reconcileRecentSessions).toHaveBeenCalledOnce();

    enabled.value = false;
    await nextTick();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(storeMock.reconcileRecentSessions).toHaveBeenCalledOnce();

    enabled.value = true;
    await nextTick();
    window.dispatchEvent(new Event('focus'));
    await vi.advanceTimersByTimeAsync(0);
    expect(storeMock.reconcileRecentSessions).toHaveBeenCalledTimes(2);

    wrapper.unmount();
    window.dispatchEvent(new Event('pageshow'));
    await vi.advanceTimersByTimeAsync(0);
    expect(storeMock.reconcileRecentSessions).toHaveBeenCalledTimes(2);
  });
});
