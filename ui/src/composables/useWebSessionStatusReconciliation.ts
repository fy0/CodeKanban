import { onBeforeUnmount, onMounted, toValue, watch, type MaybeRefOrGetter } from 'vue';

import { useWebSessionStore } from '@/stores/webSession';

const WEB_SESSION_RECONCILE_MIN_DELAY_MS = 25_000;
const WEB_SESSION_RECONCILE_MAX_DELAY_MS = 30_000;

type TimerHandle = ReturnType<typeof globalThis.setTimeout>;

export interface WebSessionStatusReconcileLoopOptions {
  enabled: () => boolean;
  reconcile: () => Promise<unknown>;
  onError?: (error: unknown) => void;
  isHidden?: () => boolean;
  random?: () => number;
  setTimer?: (callback: () => void, delay: number) => TimerHandle;
  clearTimer?: (timer: TimerHandle) => void;
  minDelayMs?: number;
  maxDelayMs?: number;
}

export class WebSessionStatusReconcileLoop {
  private readonly enabled: () => boolean;
  private readonly reconcile: () => Promise<unknown>;
  private readonly onError?: (error: unknown) => void;
  private readonly isHidden: () => boolean;
  private readonly random: () => number;
  private readonly setTimer: (callback: () => void, delay: number) => TimerHandle;
  private readonly clearTimer: (timer: TimerHandle) => void;
  private readonly minDelayMs: number;
  private readonly maxDelayMs: number;
  private timer: TimerHandle | null = null;
  private active = false;
  private running = false;
  private failureReported = false;

  constructor(options: WebSessionStatusReconcileLoopOptions) {
    this.enabled = options.enabled;
    this.reconcile = options.reconcile;
    this.onError = options.onError;
    this.isHidden =
      options.isHidden ??
      (() => typeof document !== 'undefined' && document.visibilityState === 'hidden');
    this.random = options.random ?? Math.random;
    this.setTimer =
      options.setTimer ?? ((callback, delay) => globalThis.setTimeout(callback, delay));
    this.clearTimer = options.clearTimer ?? (timer => globalThis.clearTimeout(timer));
    this.minDelayMs = Math.max(0, options.minDelayMs ?? WEB_SESSION_RECONCILE_MIN_DELAY_MS);
    this.maxDelayMs = Math.max(
      this.minDelayMs,
      options.maxDelayMs ?? WEB_SESSION_RECONCILE_MAX_DELAY_MS
    );
  }

  start(options: { immediate?: boolean } = {}) {
    this.active = true;
    if (options.immediate) {
      this.trigger();
      return;
    }
    if (!this.running && this.timer == null) {
      this.schedule();
    }
  }

  stop() {
    this.active = false;
    this.failureReported = false;
    this.clearScheduledTimer();
  }

  trigger() {
    if (!this.active || !this.enabled() || this.isHidden()) {
      return;
    }
    this.clearScheduledTimer();
    if (this.running) {
      return;
    }
    void this.execute();
  }

  handleVisibilityChange() {
    if (!this.active) {
      return;
    }
    if (this.isHidden()) {
      this.clearScheduledTimer();
      return;
    }
    this.trigger();
  }

  private async execute() {
    this.running = true;
    try {
      await this.reconcile();
      this.failureReported = false;
    } catch (error) {
      if (!this.failureReported) {
        this.onError?.(error);
        this.failureReported = true;
      }
    } finally {
      this.running = false;
      this.schedule();
    }
  }

  private schedule() {
    this.clearScheduledTimer();
    if (!this.active || !this.enabled() || this.isHidden()) {
      return;
    }
    const random = Math.min(1, Math.max(0, this.random()));
    const delay = Math.round(this.minDelayMs + (this.maxDelayMs - this.minDelayMs) * random);
    this.timer = this.setTimer(() => {
      this.timer = null;
      this.trigger();
    }, delay);
  }

  private clearScheduledTimer() {
    if (this.timer == null) {
      return;
    }
    this.clearTimer(this.timer);
    this.timer = null;
  }
}

export function useWebSessionStatusReconciliation(enabled: MaybeRefOrGetter<boolean> = true) {
  const webSessionStore = useWebSessionStore();
  const shouldRun = () => toValue(enabled) && webSessionStore.hasReconcilePrioritySessions;
  const loop = new WebSessionStatusReconcileLoop({
    enabled: shouldRun,
    reconcile: () => webSessionStore.reconcileRecentSessions(),
    onError: error => {
      console.warn('[Web Session] Failed to reconcile active session summaries', error);
    },
  });
  let mounted = false;

  watch(shouldRun, value => {
    if (!mounted) {
      return;
    }
    if (value) {
      loop.start();
    } else {
      loop.stop();
    }
  });

  const handleVisibilityChange = () => loop.handleVisibilityChange();
  const handleRecoverySignal = () => loop.trigger();

  onMounted(() => {
    mounted = true;
    if (typeof window !== 'undefined') {
      window.addEventListener('focus', handleRecoverySignal);
      window.addEventListener('pageshow', handleRecoverySignal);
    }
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', handleVisibilityChange);
    }
    if (shouldRun()) {
      loop.start();
    }
  });

  onBeforeUnmount(() => {
    mounted = false;
    loop.stop();
    if (typeof window !== 'undefined') {
      window.removeEventListener('focus', handleRecoverySignal);
      window.removeEventListener('pageshow', handleRecoverySignal);
    }
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    }
  });
}
