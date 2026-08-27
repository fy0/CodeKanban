export type AdaptiveGitRefreshMode = 'foreground' | 'background';

export interface AdaptiveGitRefreshResult {
  changed: boolean;
}

interface AdaptiveGitRefreshOptions {
  mode: AdaptiveGitRefreshMode;
  refresh: () => Promise<AdaptiveGitRefreshResult>;
  enabled?: () => boolean;
  onError?: (error: unknown) => void;
  now?: () => number;
  random?: () => number;
  isHidden?: () => boolean;
  setTimer?: (callback: () => void, delay: number) => ReturnType<typeof setTimeout>;
  clearTimer?: (timer: ReturnType<typeof setTimeout>) => void;
  slowThresholdMs?: number;
}

const REFRESH_INTERVALS: Record<AdaptiveGitRefreshMode, readonly number[]> = {
  foreground: [30_000, 60_000, 120_000],
  background: [60_000, 120_000, 300_000],
};

export class AdaptiveGitRefreshLoop {
  private readonly options: Required<
    Pick<
      AdaptiveGitRefreshOptions,
      'enabled' | 'now' | 'random' | 'isHidden' | 'setTimer' | 'clearTimer' | 'slowThresholdMs'
    >
  > &
    AdaptiveGitRefreshOptions;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private active = false;
  private running = false;
  private pending = false;
  private unchangedCount = 0;
  private pressureTier = 0;
  private listeningForVisibility = false;

  constructor(options: AdaptiveGitRefreshOptions) {
    this.options = {
      ...options,
      enabled: options.enabled ?? (() => true),
      now: options.now ?? Date.now,
      random: options.random ?? Math.random,
      isHidden:
        options.isHidden ??
        (() => typeof document !== 'undefined' && document.visibilityState === 'hidden'),
      setTimer: options.setTimer ?? ((callback, delay) => globalThis.setTimeout(callback, delay)),
      clearTimer: options.clearTimer ?? (timer => globalThis.clearTimeout(timer)),
      slowThresholdMs: options.slowThresholdMs ?? 2_000,
    };
  }

  start(options: { immediate?: boolean } = {}) {
    this.active = true;
    this.attachVisibilityListener();
    this.clearScheduledTimer();
    if (options.immediate) {
      this.trigger();
      return;
    }
    this.schedule();
  }

  stop() {
    this.active = false;
    this.pending = false;
    this.clearScheduledTimer();
    this.detachVisibilityListener();
  }

  reset() {
    this.unchangedCount = 0;
    this.pressureTier = 0;
    if (this.active && !this.running) {
      this.schedule();
    }
  }

  notifyMutation() {
    this.unchangedCount = 0;
    this.pressureTier = 0;
    this.trigger();
  }

  trigger() {
    if (!this.active || !this.options.enabled() || this.options.isHidden()) {
      return;
    }
    this.clearScheduledTimer();
    if (this.running) {
      this.pending = true;
      return;
    }
    void this.execute();
  }

  handleVisibilityChange = () => {
    if (!this.active) {
      return;
    }
    if (this.options.isHidden()) {
      this.clearScheduledTimer();
      return;
    }
    this.trigger();
  };

  private async execute() {
    this.running = true;
    const startedAt = this.options.now();
    let changed = false;
    let failed = false;
    try {
      const result = await this.options.refresh();
      changed = result.changed;
    } catch (error) {
      failed = true;
      this.options.onError?.(error);
    } finally {
      const duration = Math.max(0, this.options.now() - startedAt);
      if (changed) {
        this.unchangedCount = 0;
        this.pressureTier = 0;
      } else {
        this.unchangedCount += 1;
        if (failed || duration >= this.options.slowThresholdMs) {
          this.pressureTier = Math.min(2, this.pressureTier + 1);
        } else {
          this.pressureTier = Math.max(0, this.pressureTier - 1);
        }
      }
      this.running = false;

      if (!this.active) {
        this.pending = false;
      } else if (this.pending) {
        this.pending = false;
        this.trigger();
      } else {
        this.schedule();
      }
    }
  }

  private schedule() {
    this.clearScheduledTimer();
    if (!this.active || !this.options.enabled() || this.options.isHidden()) {
      return;
    }
    const stableTier = this.unchangedCount >= 5 ? 2 : this.unchangedCount >= 2 ? 1 : 0;
    const tier = Math.max(stableTier, this.pressureTier);
    const intervals = REFRESH_INTERVALS[this.options.mode];
    const interval = intervals[Math.min(tier, intervals.length - 1)];
    const jitter = 0.9 + Math.min(1, Math.max(0, this.options.random())) * 0.2;
    this.timer = this.options.setTimer(
      () => {
        this.timer = null;
        this.trigger();
      },
      Math.round(interval * jitter)
    );
  }

  private clearScheduledTimer() {
    if (this.timer === null) {
      return;
    }
    this.options.clearTimer(this.timer);
    this.timer = null;
  }

  private attachVisibilityListener() {
    if (this.listeningForVisibility || typeof document === 'undefined') {
      return;
    }
    document.addEventListener('visibilitychange', this.handleVisibilityChange);
    this.listeningForVisibility = true;
  }

  private detachVisibilityListener() {
    if (!this.listeningForVisibility || typeof document === 'undefined') {
      return;
    }
    document.removeEventListener('visibilitychange', this.handleVisibilityChange);
    this.listeningForVisibility = false;
  }
}
