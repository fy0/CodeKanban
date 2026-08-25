import {
  compareWebSessionRevisions,
  normalizeWebSessionRevision,
} from '@/utils/webSessionRevision';

export type WebSessionHydrationRequest = {
  sessionId: string;
  revision: string;
  reason: string;
};

type RevisionClock = 'observed' | 'applied' | 'hydrated';
type RevisionClocks = Record<RevisionClock, string>;

type WebSessionSyncOptions = {
  hydrate: (request: WebSessionHydrationRequest) => Promise<void>;
  onError?: (request: WebSessionHydrationRequest, error: unknown) => void;
};

export class WebSessionSync {
  private readonly revisions = new Map<string, RevisionClocks>();
  private readonly pending = new Map<string, WebSessionHydrationRequest>();
  private readonly inFlight = new Map<string, Promise<void>>();

  constructor(private readonly options: WebSessionSyncOptions) {}

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
    if (revision) {
      this.advance(sessionId, 'hydrated', revision);
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

  requestHydration(sessionId: string, value: unknown, reason = '') {
    const revision = this.observe(sessionId, value);
    if (!sessionId || !revision) {
      return null;
    }
    if (this.isSnapshotCurrent(sessionId, revision)) {
      this.pending.delete(sessionId);
      return null;
    }

    const pending = this.pending.get(sessionId);
    if (!pending || compareWebSessionRevisions(revision, pending.revision) === 1) {
      this.pending.set(sessionId, { sessionId, revision, reason });
    }
    return this.inFlight.get(sessionId) ?? this.startHydration(sessionId);
  }

  clear(sessionId: string) {
    this.revisions.delete(sessionId);
    this.pending.delete(sessionId);
    this.inFlight.delete(sessionId);
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

  private startHydration(sessionId: string) {
    const requested = this.pending.get(sessionId);
    if (!requested) {
      return null;
    }

    const flight = Promise.resolve()
      .then(() => this.options.hydrate(requested))
      .catch(error => this.options.onError?.(requested, error))
      .finally(() => {
        if (this.inFlight.get(sessionId) !== flight) {
          return;
        }
        this.inFlight.delete(sessionId);
        const latest = this.pending.get(sessionId);
        if (!latest) {
          return;
        }
        if (this.isSnapshotCurrent(sessionId, latest.revision)) {
          this.pending.delete(sessionId);
        } else if (compareWebSessionRevisions(latest.revision, requested.revision) === 1) {
          this.startHydration(sessionId);
        }
      });
    this.inFlight.set(sessionId, flight);
    return flight;
  }
}
