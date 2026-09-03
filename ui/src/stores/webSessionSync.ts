import {
  compareWebSessionRevisions,
  normalizeWebSessionRevision,
} from '@/utils/webSessionRevision';

export type WebSessionHydrationMode = 'catch-up' | 'snapshot';

export type WebSessionHydrationRequest = {
  sessionId: string;
  revision: string;
  reason: string;
  mode: WebSessionHydrationMode;
  signal: AbortSignal;
};

export type WebSessionHydrationRequestOptions = {
  mode?: WebSessionHydrationMode;
};

type PendingHydrationRequest = Omit<WebSessionHydrationRequest, 'signal'>;

type RevisionClock = 'observed' | 'applied' | 'hydrated';
type RevisionClocks = Record<RevisionClock, string>;
type TimerHandle = ReturnType<typeof globalThis.setTimeout>;

type WebSessionSyncOptions = {
  hydrate: (request: WebSessionHydrationRequest) => Promise<void>;
  onError?: (request: WebSessionHydrationRequest, error: unknown) => void;
  now?: () => number;
  setTimer?: (callback: () => void, delay: number) => TimerHandle;
  clearTimer?: (timer: TimerHandle) => void;
  retryBaseDelayMs?: number;
  trailingDelayMs?: number;
  /**
   * Kept as an alias for callers that used the original per-revision option.
   * The limit now applies to unresolved attempts across revisions as well.
   */
  maxAttemptsPerRevision?: number;
  maxAttemptsPerWindow?: number;
  attemptWindowMs?: number;
};

type HydrationAttempt = {
  revision: string;
  count: number;
  retryAfter: number;
  windowStartedAt: number;
};

type InFlightHydration = {
  promise: Promise<void>;
  controller: AbortController;
  revision: string;
};

type ScheduledHydration = {
  handle: TimerHandle;
  dueAt: number;
};

const DEFAULT_RETRY_BASE_DELAY_MS = 5_000;
const DEFAULT_TRAILING_DELAY_MS = 250;
const DEFAULT_MAX_ATTEMPTS_PER_WINDOW = 3;
const DEFAULT_ATTEMPT_WINDOW_MS = 30_000;

function finiteNonNegative(value: number | undefined, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(0, Math.trunc(value))
    : fallback;
}

function finitePositive(value: number | undefined, fallback: number) {
  return Math.max(1, finiteNonNegative(value, fallback));
}

export class WebSessionSync {
  private readonly revisions = new Map<string, RevisionClocks>();
  private readonly pending = new Map<string, PendingHydrationRequest>();
  private readonly inFlight = new Map<string, InFlightHydration>();
  private readonly attempts = new Map<string, HydrationAttempt>();
  private readonly scheduled = new Map<string, ScheduledHydration>();
  private readonly now: () => number;
  private readonly setTimer: (callback: () => void, delay: number) => TimerHandle;
  private readonly clearTimer: (timer: TimerHandle) => void;
  private readonly retryBaseDelayMs: number;
  private readonly trailingDelayMs: number;
  private readonly maxAttemptsPerWindow: number;
  private readonly attemptWindowMs: number;

  constructor(private readonly options: WebSessionSyncOptions) {
    this.now = options.now ?? Date.now;
    this.setTimer =
      options.setTimer ?? ((callback, delay) => globalThis.setTimeout(callback, delay));
    this.clearTimer = options.clearTimer ?? (timer => globalThis.clearTimeout(timer));
    this.retryBaseDelayMs = finiteNonNegative(
      options.retryBaseDelayMs,
      DEFAULT_RETRY_BASE_DELAY_MS
    );
    this.trailingDelayMs = finiteNonNegative(options.trailingDelayMs, DEFAULT_TRAILING_DELAY_MS);
    this.maxAttemptsPerWindow = finitePositive(
      options.maxAttemptsPerWindow ?? options.maxAttemptsPerRevision,
      DEFAULT_MAX_ATTEMPTS_PER_WINDOW
    );
    this.attemptWindowMs = finitePositive(options.attemptWindowMs, DEFAULT_ATTEMPT_WINDOW_MS);
  }

  observe(sessionId: string, value: unknown) {
    return this.advance(sessionId, 'observed', value);
  }

  markApplied(sessionId: string, value: unknown) {
    const revision = this.observe(sessionId, value);
    if (revision) {
      this.advance(sessionId, 'applied', revision);
    }
    return revision;
  }

  markHydrated(sessionId: string, value: unknown) {
    const revision = this.markApplied(sessionId, value);
    if (!revision) {
      return revision;
    }

    this.advance(sessionId, 'hydrated', revision);
    const pending = this.pending.get(sessionId);
    if (pending && compareWebSessionRevisions(pending.revision, revision) !== 1) {
      this.pending.delete(sessionId);
      this.clearScheduled(sessionId);
    }

    const attempt = this.attempts.get(sessionId);
    const remainingPending = this.pending.get(sessionId);
    if (remainingPending && compareWebSessionRevisions(remainingPending.revision, revision) === 1) {
      // The response made useful progress, but it did not cover the revision
      // that is still pending. Keep the unresolved-attempt budget so a stream
      // of one-revision-late snapshots cannot reset the circuit on every page.
      // Progress may use the short trailing delay; a genuinely stale response
      // keeps the exponential retry delay recorded for the attempt.
      if (attempt) {
        this.attempts.set(sessionId, {
          ...attempt,
          retryAfter: Math.min(attempt.retryAfter, this.now() + this.trailingDelayMs),
        });
      }
    } else if (
      attempt &&
      (!this.getObserved(sessionId) ||
        compareWebSessionRevisions(revision, this.getObserved(sessionId)) !== -1)
    ) {
      // A successful baseline resets the unresolved-attempt budget only after
      // it has caught up with every revision observed so far.
      this.attempts.delete(sessionId);
      this.clearScheduled(sessionId);
    }
    return revision;
  }

  getApplied(sessionId: string) {
    return this.revisions.get(sessionId)?.applied ?? '';
  }

  getObserved(sessionId: string) {
    return this.revisions.get(sessionId)?.observed ?? '';
  }

  getHydrated(sessionId: string) {
    return this.revisions.get(sessionId)?.hydrated ?? '';
  }

  getHydrationPromise(sessionId: string) {
    return this.inFlight.get(sessionId)?.promise ?? null;
  }

  hasPendingHydration(sessionId: string) {
    return (
      this.pending.has(sessionId) || this.inFlight.has(sessionId) || this.scheduled.has(sessionId)
    );
  }

  isSnapshotCurrent(sessionId: string, value: unknown) {
    const revision = normalizeWebSessionRevision(value);
    const hydrated = this.getHydrated(sessionId);
    return Boolean(revision && hydrated && compareWebSessionRevisions(hydrated, revision) !== -1);
  }

  shouldApply(sessionId: string, value: unknown) {
    const revision = normalizeWebSessionRevision(value);
    const applied = this.getApplied(sessionId);
    return !revision || !applied || compareWebSessionRevisions(revision, applied) !== -1;
  }

  requestHydration(
    sessionId: string,
    value: unknown,
    reason = '',
    options?: WebSessionHydrationRequestOptions
  ) {
    const normalizedSessionId = String(sessionId || '').trim();
    const incomingRevision = this.observe(normalizedSessionId, value);
    if (!normalizedSessionId || !incomingRevision) {
      return null;
    }

    // `observe` is monotonic, but returns the normalized value supplied by the
    // caller. A delayed/older notice must therefore hydrate the highest value
    // already observed for this session, rather than downgrading the pending
    // target and accidentally treating the real gap as resolved.
    const revision = this.getObserved(normalizedSessionId) || incomingRevision;

    if (this.isSnapshotCurrent(normalizedSessionId, revision)) {
      const hydrated = this.getHydrated(normalizedSessionId);
      const pending = this.pending.get(normalizedSessionId);
      if (pending && compareWebSessionRevisions(pending.revision, hydrated) !== 1) {
        this.pending.delete(normalizedSessionId);
        this.clearScheduled(normalizedSessionId);
      }
      if (!this.pending.has(normalizedSessionId)) {
        const observed = this.getObserved(normalizedSessionId);
        if (!observed || compareWebSessionRevisions(hydrated, observed) !== -1) {
          this.attempts.delete(normalizedSessionId);
          this.clearScheduled(normalizedSessionId);
        }
        const flight = this.inFlight.get(normalizedSessionId);
        if (flight && compareWebSessionRevisions(flight.revision, hydrated) !== 1) {
          this.abortFlight(normalizedSessionId, flight);
        }
      }
      return this.inFlight.get(normalizedSessionId)?.promise ?? null;
    }

    const mode: WebSessionHydrationMode = options?.mode ?? (reason ? 'snapshot' : 'catch-up');
    const nextRequest: PendingHydrationRequest = {
      sessionId: normalizedSessionId,
      revision,
      reason: String(reason || '').trim(),
      mode,
    };
    const pending = this.pending.get(normalizedSessionId);
    if (!pending) {
      this.pending.set(normalizedSessionId, nextRequest);
    } else {
      const comparison = compareWebSessionRevisions(revision, pending.revision);
      if (comparison === 1) {
        this.pending.set(normalizedSessionId, {
          ...nextRequest,
          // Once a caller has explicitly requested a snapshot, do not let a
          // later catch-up notice downgrade that recovery before it runs.
          mode: pending.mode === 'snapshot' || mode === 'snapshot' ? 'snapshot' : mode,
          reason: nextRequest.reason || pending.reason,
        });
      } else if (comparison === 0) {
        this.pending.set(normalizedSessionId, {
          ...pending,
          // A snapshot request is stronger than a catch-up request at the
          // same revision. Preserve any useful reason supplied by either path.
          mode: pending.mode === 'snapshot' || mode === 'snapshot' ? 'snapshot' : 'catch-up',
          reason: nextRequest.reason || pending.reason,
        });
      } else if (mode === 'snapshot' && pending.mode !== 'snapshot') {
        // A lower revision can still carry a stronger reset signal. Preserve
        // the current highest revision while upgrading its hydration mode.
        this.pending.set(normalizedSessionId, {
          ...pending,
          mode: 'snapshot',
          reason: nextRequest.reason || pending.reason,
        });
      }
    }

    const flight = this.inFlight.get(normalizedSessionId);
    if (flight) {
      return flight.promise;
    }

    const attempt = this.attempts.get(normalizedSessionId);
    if (attempt && compareWebSessionRevisions(revision, attempt.revision) === 1) {
      // A new revision arriving after an unresolved attempt is a trailing
      // request. Debouncing it is what prevents a hot event stream from
      // turning every revision into an immediate HTTP request.
      return this.scheduleTrailing(normalizedSessionId);
    }
    return this.startHydrationIfReady(normalizedSessionId);
  }

  cancelHydration(sessionId: string) {
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedSessionId) {
      return;
    }
    this.pending.delete(normalizedSessionId);
    this.attempts.delete(normalizedSessionId);
    this.clearScheduled(normalizedSessionId);
    const flight = this.inFlight.get(normalizedSessionId);
    if (flight) {
      this.abortFlight(normalizedSessionId, flight);
    }
  }

  clear(sessionId: string) {
    const normalizedSessionId = String(sessionId || '').trim();
    if (!normalizedSessionId) {
      return;
    }
    this.cancelHydration(normalizedSessionId);
    this.revisions.delete(normalizedSessionId);
  }

  private advance(sessionId: string, clock: RevisionClock, value: unknown) {
    const revision = normalizeWebSessionRevision(value);
    if (!sessionId || !revision) {
      return '';
    }
    let state = this.revisions.get(sessionId);
    if (!state) {
      state = { observed: '', applied: '', hydrated: '' };
      this.revisions.set(sessionId, state);
    }
    if (!state[clock] || compareWebSessionRevisions(revision, state[clock]) === 1) {
      state[clock] = revision;
    }
    return revision;
  }

  private startHydrationIfReady(sessionId: string): Promise<void> | null {
    const pending = this.pending.get(sessionId);
    if (!pending || this.inFlight.has(sessionId)) {
      return this.inFlight.get(sessionId)?.promise ?? null;
    }
    const delay = this.nextAttemptDelay(sessionId);
    if (delay == null || delay > 0) {
      return null;
    }

    this.clearScheduled(sessionId);
    const controller = new AbortController();
    const requested: WebSessionHydrationRequest = {
      ...pending,
      signal: controller.signal,
    };
    this.recordHydrationAttempt(sessionId, requested.revision);

    const flight = Promise.resolve()
      .then(() => this.options.hydrate(requested))
      .catch(error => {
        if (!controller.signal.aborted) {
          this.options.onError?.(requested, error);
        }
      })
      .finally(() => {
        if (this.inFlight.get(sessionId)?.promise !== flight) {
          return;
        }
        this.inFlight.delete(sessionId);
        const latest = this.pending.get(sessionId);
        if (!latest) {
          return;
        }
        if (this.isSnapshotCurrent(sessionId, latest.revision)) {
          this.pending.delete(sessionId);
          this.attempts.delete(sessionId);
          this.clearScheduled(sessionId);
          return;
        }
        // An unresolved response can be a rejected request, a stale snapshot,
        // or a malformed response that never called markHydrated. All of them
        // need the same bounded, backoff-controlled retry path. The attempt
        // circuit, rather than the revision comparison, is what guarantees
        // this cannot become an endless timer/request loop.
        this.scheduleTrailing(sessionId);
      });

    this.inFlight.set(sessionId, {
      promise: flight,
      controller,
      revision: requested.revision,
    });
    return flight;
  }

  private scheduleTrailing(sessionId: string): null {
    if (this.inFlight.has(sessionId) || !this.pending.has(sessionId)) {
      return null;
    }
    const budgetDelay = this.nextAttemptDelay(sessionId);
    if (budgetDelay == null) {
      // The unresolved-attempt circuit is intentionally closed. A later
      // notice may reopen it after the window expires, but no timer is kept
      // alive while the server continues to emit revisions.
      return null;
    }
    const delay = Math.max(this.trailingDelayMs, budgetDelay);
    const dueAt = this.now() + delay;
    const existing = this.scheduled.get(sessionId);
    if (existing && existing.dueAt <= dueAt) {
      return null;
    }
    if (existing) {
      this.clearTimer(existing.handle);
    }

    // A few embedders provide a synchronous timer double. Defer callback
    // bookkeeping until the handle has been assigned so that such a double
    // cannot observe an uninitialised `handle` (or leave a phantom schedule).
    const timerRef: { handle?: TimerHandle } = {};
    let timerReady = false;
    let callbackPending = false;
    const run = () => {
      if (!timerReady) {
        callbackPending = true;
        return;
      }
      if (this.scheduled.get(sessionId)?.handle !== timerRef.handle) {
        return;
      }
      this.scheduled.delete(sessionId);
      const remaining = this.nextAttemptDelay(sessionId);
      if (remaining == null) {
        return;
      }
      if (remaining > 0) {
        // A synchronous timer double has already fired before it returned a
        // handle. It cannot represent the passage of the retry delay; trying
        // to schedule again from here would recurse until the stack overflows.
        // Leave the pending request for a real timer or a later notice.
        if (callbackPending) {
          return;
        }
        this.scheduleTrailing(sessionId);
        return;
      }
      void this.startHydrationIfReady(sessionId);
    };
    timerRef.handle = this.setTimer(run, delay);
    timerReady = true;
    const scheduled: ScheduledHydration = { handle: timerRef.handle, dueAt };
    this.scheduled.set(sessionId, scheduled);
    if (callbackPending) {
      run();
    }
    return null;
  }

  private nextAttemptDelay(sessionId: string): number | null {
    const attempt = this.attempts.get(sessionId);
    if (!attempt) {
      return 0;
    }
    const now = this.now();
    if (now - attempt.windowStartedAt >= this.attemptWindowMs) {
      this.attempts.delete(sessionId);
      return 0;
    }
    if (attempt.count >= this.maxAttemptsPerWindow) {
      return null;
    }
    return Math.max(0, attempt.retryAfter - now);
  }

  private recordHydrationAttempt(sessionId: string, revision: string) {
    const now = this.now();
    const previous = this.attempts.get(sessionId);
    const activeWindow = previous && now - previous.windowStartedAt < this.attemptWindowMs;
    const count = activeWindow ? previous.count + 1 : 1;
    const delay = this.retryBaseDelayMs * 2 ** Math.min(Math.max(0, count - 1), 6);
    this.attempts.set(sessionId, {
      revision,
      count,
      retryAfter: now + delay,
      windowStartedAt: activeWindow ? previous.windowStartedAt : now,
    });
  }

  private clearScheduled(sessionId: string) {
    const scheduled = this.scheduled.get(sessionId);
    if (!scheduled) {
      return;
    }
    this.clearTimer(scheduled.handle);
    this.scheduled.delete(sessionId);
  }

  private abortFlight(sessionId: string, flight: InFlightHydration) {
    if (this.inFlight.get(sessionId) !== flight) {
      return;
    }
    this.inFlight.delete(sessionId);
    flight.controller.abort();
  }
}
