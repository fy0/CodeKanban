import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { WebSessionSnapshot } from '@/api/webSession';
import type { WebSessionSummary } from '@/types/models';
import {
  isWebSessionMessageDeliveryError,
  useWebSessionStore,
  webSessionRuntimePerformance,
} from '@/stores/webSession';

const {
  listMock,
  reconcileMock,
  queryArchivedMock,
  snapshotMock,
  catchUpMock,
  historyMock,
  syncMock,
  deleteMock,
  commandGroupDetailMock,
} = vi.hoisted(() => ({
  listMock: vi.fn(),
  reconcileMock: vi.fn(),
  queryArchivedMock: vi.fn(),
  snapshotMock: vi.fn(),
  catchUpMock: vi.fn(),
  historyMock: vi.fn(),
  syncMock: vi.fn(),
  deleteMock: vi.fn(),
  commandGroupDetailMock: vi.fn(),
}));

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    list: listMock,
    reconcile: reconcileMock,
    queryArchived: queryArchivedMock,
    snapshot: snapshotMock,
    catchUp: catchUpMock,
    history: historyMock,
    sync: syncMock,
    delete: deleteMock,
    commandGroupDetail: commandGroupDetailMock,
  },
}));

vi.mock('@/utils/ws', () => ({
  resolveWsUrl: (path: string) => path,
}));

function createStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
    removeItem(key: string) {
      store.delete(key);
    },
    clear() {
      store.clear();
    },
  };
}

function makeSession(overrides: Partial<WebSessionSummary> = {}): WebSessionSummary {
  return {
    id: 'session-1',
    revision: '1',
    attentionRevision: '0',
    historyEpoch: '1',
    eventCursor: '0:9223372036854775807',
    projectId: 'project-1',
    worktreeId: null,
    orderIndex: 1000,
    agent: 'codex',
    backend: 'codex_app_server',
    title: 'Codex Session',
    model: 'gpt-5.4',
    reasoningEffort: 'medium',
    workflowMode: 'default',
    permissionLevel: 'elevated',
    cwd: '/tmp/project',
    nativeSessionId: 'native-1',
    status: 'running',
    assistantState: 'waiting_input',
    hasUnread: false,
    archivedAt: null,
    activityAt: '2026-04-09T10:00:00.000Z',
    statusUpdatedAt: '2026-04-09T10:00:00.000Z',
    lastMessageAt: '2026-04-09T10:00:00.000Z',
    assistantStateUpdatedAt: '2026-04-09T10:00:00.000Z',
    sourceKind: 'codex_app_server',
    syncState: 'fresh',
    lastSyncMode: 'fast',
    sourceCreatedAt: '2026-04-09T09:00:00.000Z',
    sourceUpdatedAt: '2026-04-09T10:00:00.000Z',
    lastSyncedAt: '2026-04-09T10:00:00.000Z',
    threadPath: '/tmp/session.jsonl',
    threadPreview: 'preview',
    turnCount: 1,
    itemCount: 1,
    syncError: null,
    createdAt: '2026-04-09T09:00:00.000Z',
    updatedAt: '2026-04-09T10:00:00.000Z',
    usage: {
      inputTokens: 1,
      cachedInputTokens: 0,
      outputTokens: 1,
      cost: 0,
    },
    contextEstimate: {
      inputTokens: 1,
      cachedInputTokens: 0,
      outputTokens: 1,
      usedTokens: 2,
    },
    contextEstimateMode: 'cumulative_total',
    lastContextCompactionAt: null,
    contextWindowTokens: null,
    contextWindowSource: 'default',
    ...overrides,
  };
}

function toMillis(value?: string | null) {
  const parsed = Date.parse(value ?? '');
  return Number.isFinite(parsed) ? parsed : Date.now();
}

function toWireSession(session: WebSessionSummary) {
  return {
    id: session.id,
    rev: session.revision,
    ar: session.attentionRevision,
    he: session.historyEpoch,
    ec: session.eventCursor,
    pid: session.projectId,
    wid: session.worktreeId,
    oi: session.orderIndex,
    ag: session.agent,
    be: session.backend,
    md: session.model,
    re: session.reasoningEffort,
    wm: session.workflowMode,
    pl: session.permissionLevel,
    ttl: session.title,
    cwd: session.cwd,
    nsid: session.nativeSessionId,
    cpf: session.cyberPolicyFlagged === true,
    spe: session.hasScheduledPlanExecution === true,
    st: session.status,
    ast: session.assistantState ?? undefined,
    unr: session.hasUnread,
    aa: session.archivedAt ? toMillis(session.archivedAt) : null,
    act: toMillis(session.activityAt),
    sta: session.statusUpdatedAt ? toMillis(session.statusUpdatedAt) : null,
    ca: toMillis(session.createdAt),
    lu: toMillis(session.updatedAt),
    lma: session.lastMessageAt ? toMillis(session.lastMessageAt) : null,
    asu: session.assistantStateUpdatedAt ? toMillis(session.assistantStateUpdatedAt) : null,
    sk: session.sourceKind,
    ss: session.syncState,
    lsm: session.lastSyncMode ?? undefined,
    sca: session.sourceCreatedAt ? toMillis(session.sourceCreatedAt) : null,
    sua: session.sourceUpdatedAt ? toMillis(session.sourceUpdatedAt) : null,
    lsa: session.lastSyncedAt ? toMillis(session.lastSyncedAt) : null,
    tp: session.threadPath,
    tpv: session.threadPreview,
    tc: session.turnCount,
    ic: session.itemCount,
    se: session.syncError,
    usa: {
      in: session.usage.inputTokens,
      cin: session.usage.cachedInputTokens,
      out: session.usage.outputTokens,
    },
    cea: {
      in: session.contextEstimate.inputTokens,
      cin: session.contextEstimate.cachedInputTokens,
      out: session.contextEstimate.outputTokens,
      usd: session.contextEstimate.usedTokens,
    },
    cem: session.contextEstimateMode,
    lcca: session.lastContextCompactionAt ? toMillis(session.lastContextCompactionAt) : null,
    cost: session.usage.cost,
    cwt: session.contextWindowTokens,
    cws: session.contextWindowSource,
  };
}

function makeWireHistoryItem(index: number, overrides: Record<string, unknown> = {}) {
  return {
    id: `history-${index}`,
    oi: index,
    kd: 'system',
    tp: 'note',
    txt: `history ${index}`,
    ts2: Date.parse('2026-04-09T10:00:00.000Z') + index,
    ...overrides,
  };
}

class FakeWebSocket {
  static OPEN = 1;
  static instances: FakeWebSocket[] = [];
  static revision = 1;

  url: string;
  readyState = 0;
  sent: unknown[] = [];
  onopen: ((event: unknown) => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: ((event: unknown) => void) | null = null;
  onclose: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
    queueMicrotask(() => {
      this.readyState = FakeWebSocket.OPEN;
      this.onopen?.({});
    });
  }

  send(payload: string) {
    this.sent.push(JSON.parse(payload));
  }

  dispatch(frame: unknown) {
    const record = frame && typeof frame === 'object' ? (frame as Record<string, unknown>) : null;
    const revisionedFrame =
      record && record.k !== 'hb' && record.k !== 'err'
        ? {
            ...record,
            rev: record.rev ?? String(++FakeWebSocket.revision),
          }
        : frame;
    this.onmessage?.({
      data: JSON.stringify(revisionedFrame),
    });
  }

  close() {
    this.readyState = 3;
    this.onclose?.();
  }
}

function findSocket(url: string) {
  return FakeWebSocket.instances.find(instance => instance.url === url) ?? null;
}

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
}

async function waitForCommandCount(count: number) {
  let socket = findSocket('/api/v1/web-sessions/ws');
  for (let attempt = 0; attempt < 10 && (socket?.sent.length ?? 0) < count; attempt += 1) {
    await flushMicrotasks();
    socket = findSocket('/api/v1/web-sessions/ws');
  }
  return socket;
}

describe('webSession loading behavior', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const localStorage = createStorageMock();
    const sessionStorage = createStorageMock();
    vi.stubGlobal('localStorage', localStorage);
    vi.stubGlobal('sessionStorage', sessionStorage);
    vi.stubGlobal('window', {
      localStorage,
      sessionStorage,
      location: {
        protocol: 'http:',
        host: 'localhost:5173',
      },
      setTimeout,
      clearTimeout,
      setInterval,
      clearInterval,
    });
    vi.stubGlobal('WebSocket', FakeWebSocket);
    FakeWebSocket.instances = [];
    FakeWebSocket.revision = 1;
    listMock.mockReset();
    reconcileMock.mockReset();
    reconcileMock.mockResolvedValue({ items: [], missingIds: [] });
    queryArchivedMock.mockReset();
    snapshotMock.mockReset();
    catchUpMock.mockReset();
    historyMock.mockReset();
    syncMock.mockReset();
    deleteMock.mockReset();
    commandGroupDetailMock.mockReset();
    webSessionRuntimePerformance.reset();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('singleflights concurrent forced lists and session snapshots', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-singleflight', revision: '2' });
    let resolveList!: (sessions: WebSessionSummary[]) => void;
    const listPromise = new Promise<WebSessionSummary[]>(resolve => {
      resolveList = resolve;
    });
    listMock.mockReturnValue(listPromise);

    const firstList = store.loadSessions(session.projectId, true);
    const secondList = store.loadSessions(session.projectId, true);
    expect(listMock).toHaveBeenCalledTimes(1);
    resolveList([session]);
    await Promise.all([firstList, secondList]);

    let resolveSnapshot!: (snapshot: {
      revision: string;
      session: WebSessionSummary;
      history: { items: unknown[]; hasMore: boolean; total: number };
    }) => void;
    const snapshotPromise = new Promise<{
      revision: string;
      session: WebSessionSummary;
      history: { items: unknown[]; hasMore: boolean; total: number };
    }>(resolve => {
      resolveSnapshot = resolve;
    });
    snapshotMock.mockReturnValue(snapshotPromise);

    const firstSnapshot = store.loadSessionSnapshot(session.projectId, session.id);
    const secondSnapshot = store.loadSessionSnapshot(session.projectId, session.id);
    expect(snapshotMock).toHaveBeenCalledTimes(1);
    resolveSnapshot({
      revision: '2',
      session,
      history: { items: [], hasMore: false, total: 0 },
    });
    await Promise.all([firstSnapshot, secondSnapshot]);
  });

  it('distinguishes an uninitialized timeline from a successfully loaded empty timeline', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-empty-hydration', status: 'done', itemCount: 0 });
    let resolveSnapshot!: (snapshot: WebSessionSnapshot) => void;
    snapshotMock.mockReturnValue(
      new Promise<WebSessionSnapshot>(resolve => {
        resolveSnapshot = resolve;
      })
    );

    expect(store.getHistoryMeta(session.id)).toMatchObject({
      hydrationState: 'uninitialized',
      loading: false,
    });
    const request = store.loadSessionSnapshot(session.projectId, session.id);
    expect(store.getHistoryMeta(session.id)).toMatchObject({
      hydrationState: 'loading',
      loading: true,
    });

    resolveSnapshot({
      revision: session.revision,
      historyEpoch: '1',
      eventCursor: '0:9223372036854775807',
      session,
      history: { items: [], hasMore: false, total: 0 },
    });
    await request;

    expect(store.getHistoryMeta(session.id)).toMatchObject({
      hydrationState: 'ready',
      hydrationError: '',
      loading: false,
      total: 0,
    });
  });

  it('exposes an initial timeline hydration failure for an explicit retry', async () => {
    const store = useWebSessionStore();
    const error = new Error('snapshot unavailable');
    snapshotMock.mockRejectedValue(error);

    await expect(
      store.loadSessionSnapshot('project-hydration-error', 'session-hydration-error')
    ).rejects.toThrow('snapshot unavailable');
    expect(store.getHistoryMeta('session-hydration-error')).toMatchObject({
      hydrationState: 'error',
      hydrationError: 'snapshot unavailable',
      loading: false,
    });
  });

  it('does not attach a full snapshot consumer to a conditional flight', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-flight-profile', revision: '1' });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValueOnce({
      revision: '1',
      historyEpoch: '1',
      eventCursor: '0:9223372036854775807',
      session,
      history: { items: [], hasMore: false, total: 0 },
    });
    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    let resolveConditional!: (snapshot: WebSessionSnapshot) => void;
    snapshotMock.mockImplementationOnce(
      () =>
        new Promise<WebSessionSnapshot>(resolve => {
          resolveConditional = resolve;
        })
    );
    const conditional = store.loadSessionSnapshot(session.projectId, session.id, {
      conditional: true,
      skipTrailing: true,
    });
    await vi.waitFor(() => expect(snapshotMock).toHaveBeenCalledTimes(2));

    snapshotMock.mockResolvedValueOnce({
      revision: '1',
      session,
      history: { items: [], hasMore: false, total: 0 },
    });
    const full = store.loadSessionSnapshot(session.projectId, session.id, {
      skipTrailing: true,
    });
    await vi.waitFor(() => expect(snapshotMock).toHaveBeenCalledTimes(3));
    expect(snapshotMock.mock.calls[1]?.[2]).toMatchObject({ knownRevision: '1' });
    expect(snapshotMock.mock.calls[2]?.[2]).not.toHaveProperty('knownRevision');

    resolveConditional({ revision: '1', unchanged: true });
    await Promise.all([conditional, full]);
  });

  it('starts a fresh flight after the last snapshot consumer aborts', async () => {
    const store = useWebSessionStore();
    let resolveFirst!: (snapshot: WebSessionSnapshot) => void;
    snapshotMock.mockImplementationOnce(
      () =>
        new Promise<WebSessionSnapshot>(resolve => {
          resolveFirst = resolve;
        })
    );
    const firstController = new AbortController();
    const first = store.loadSessionSnapshot('project-flight-abort', 'session-flight-abort', {
      signal: firstController.signal,
      skipTrailing: true,
    });
    await vi.waitFor(() => expect(snapshotMock).toHaveBeenCalledOnce());

    firstController.abort();
    await expect(first).rejects.toMatchObject({ name: 'AbortError' });

    snapshotMock.mockResolvedValueOnce({ revision: '1', unchanged: true });
    const second = store.loadSessionSnapshot('project-flight-abort', 'session-flight-abort', {
      skipTrailing: true,
    });
    await vi.waitFor(() => expect(snapshotMock).toHaveBeenCalledTimes(2));
    resolveFirst({ revision: '1', unchanged: true });
    await second;
  });

  it('settles a consumer when session removal aborts a transport that ignores abort', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-transport-ignores-abort' });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockImplementationOnce(() => new Promise<WebSessionSnapshot>(() => undefined));
    deleteMock.mockResolvedValue(undefined);

    await store.loadSessions(session.projectId);
    const request = store.loadSessionSnapshot(session.projectId, session.id);
    await vi.waitFor(() => expect(snapshotMock).toHaveBeenCalledOnce());

    await store.deleteSession(session.projectId, session.id);

    await expect(request).resolves.toBeNull();
  });

  it('tracks Codex app-server runtime across snapshots and websocket events', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-app-server', revision: '2' });
    listMock.mockResolvedValue([session]);
    snapshotMock
      .mockResolvedValueOnce({
        revision: '2',
        historyEpoch: '1',
        eventCursor: '0:9223372036854775807',
        session,
        history: { items: [], hasMore: false, total: 0 },
        codexAppServer: {
          state: 'starting',
          runId: 'run-1',
          canTerminate: false,
        },
      })
      .mockResolvedValueOnce({
        revision: '4',
        historyEpoch: '1',
        eventCursor: '0:9223372036854775807',
        unchanged: true,
        codexAppServer: {
          state: 'inactive',
          canTerminate: false,
        },
      });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    expect(store.getCodexAppServerRuntime(session.id)).toEqual({
      state: 'starting',
      runId: 'run-1',
      canTerminate: false,
    });

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      rev: '3',
      ts: Date.now(),
      op: 'app_server',
      cas: { st: 'active', rid: 'run-1', pid: 4242, ct: true },
    });
    expect(store.getCodexAppServerRuntime(session.id)).toEqual({
      state: 'active',
      runId: 'run-1',
      processRootPid: 4242,
      canTerminate: true,
    });

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      rev: '4',
      ts: Date.now(),
      op: 'app_server',
      cas: { st: 'draining', rid: 'run-1', pid: 4242, ct: true },
    });
    expect(store.getCodexAppServerRuntime(session.id).state).toBe('draining');

    await store.loadSessionSnapshot(session.projectId, session.id, { conditional: true });
    expect(store.getCodexAppServerRuntime(session.id)).toEqual({
      state: 'inactive',
      canTerminate: false,
    });
  });

  it('does not let a stale unchanged snapshot roll back app-server runtime', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-app-server-race', revision: '2' });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValueOnce({
      revision: '2',
      historyEpoch: '1',
      eventCursor: '0:9223372036854775807',
      session,
      history: { items: [], hasMore: false, total: 0 },
      codexAppServer: {
        state: 'active',
        runId: 'run-1',
        processRootPid: 4242,
        canTerminate: true,
      },
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();

    let resolveSnapshot!: (snapshot: WebSessionSnapshot) => void;
    snapshotMock.mockReturnValueOnce(
      new Promise<WebSessionSnapshot>(resolve => {
        resolveSnapshot = resolve;
      })
    );
    const staleSnapshot = store.loadSessionSnapshot(session.projectId, session.id, {
      conditional: true,
      skipTrailing: true,
    });
    await flushMicrotasks();

    findSocket('/api/v1/web-sessions/events')?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      rev: '3',
      ts: Date.now(),
      op: 'app_server',
      cas: { st: 'inactive', ct: false },
    });
    resolveSnapshot({
      revision: '2',
      historyEpoch: '1',
      eventCursor: '0:9223372036854775807',
      unchanged: true,
      codexAppServer: {
        state: 'active',
        runId: 'run-1',
        processRootPid: 4242,
        canTerminate: true,
      },
    });
    await staleSnapshot;

    expect(store.getCodexAppServerRuntime(session.id)).toEqual({
      state: 'inactive',
      canTerminate: false,
    });
  });

  it('does not start a snapshot when its caller is already aborted', async () => {
    const store = useWebSessionStore();
    const controller = new AbortController();
    controller.abort();

    await expect(
      store.loadSessionSnapshot('project-aborted', 'session-aborted', {
        signal: controller.signal,
      })
    ).rejects.toMatchObject({ name: 'AbortError' });
    expect(snapshotMock).not.toHaveBeenCalled();
  });

  it('ignores a snapshot that resolves after its caller is aborted', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-late-abort', revision: '1', itemCount: 1 });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValueOnce({
      revision: '1',
      session,
      history: {
        items: [makeWireHistoryItem(1, { txt: 'baseline' })],
        hasMore: false,
        total: 1,
      },
    });
    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    let resolveLate!: (snapshot: WebSessionSnapshot) => void;
    snapshotMock.mockReturnValueOnce(
      new Promise<WebSessionSnapshot>(resolve => {
        resolveLate = resolve;
      })
    );
    const controller = new AbortController();
    const request = store.loadSessionSnapshot(session.projectId, session.id, {
      signal: controller.signal,
      skipTrailing: true,
    });
    await vi.waitFor(() => expect(snapshotMock).toHaveBeenCalledTimes(2));

    controller.abort();
    resolveLate({
      revision: '2',
      session: { ...session, revision: '2', itemCount: 99 },
      history: {
        items: [makeWireHistoryItem(1, { txt: 'late state' })],
        hasMore: false,
        total: 1,
      },
    });

    await expect(request).rejects.toMatchObject({ name: 'AbortError' });
    expect(store.getSessions(session.projectId)[0]).toMatchObject({
      id: session.id,
      revision: '1',
      itemCount: 1,
    });
    expect(store.getBlocks(session.id).map(block => block.text)).toEqual(['baseline']);
  });

  it('does not resurrect a session from a snapshot that finishes after deletion', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-delete-race', revision: '1' });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    let resolveLate!: (snapshot: WebSessionSnapshot) => void;
    snapshotMock.mockReturnValueOnce(
      new Promise<WebSessionSnapshot>(resolve => {
        resolveLate = resolve;
      })
    );
    const request = store.loadSessionSnapshot(session.projectId, session.id);
    await vi.waitFor(() => expect(snapshotMock).toHaveBeenCalledOnce());

    await store.deleteSession(session.projectId, session.id);
    resolveLate({
      revision: '2',
      session: { ...session, revision: '2', itemCount: 2 },
      history: {
        items: [makeWireHistoryItem(1, { txt: 'stale resurrection' })],
        hasMore: false,
        total: 1,
      },
    });

    await expect(request).resolves.toBeNull();
    expect(store.getSessions(session.projectId)).toEqual([]);
    expect(store.getBlocks(session.id)).toEqual([]);
  });

  it('rejects stale HTTP snapshots and uses knownRevision for unchanged probes', async () => {
    const store = useWebSessionStore();
    const newest = makeSession({ id: 'session-revision-guard', revision: '5', itemCount: 1 });
    const stale = makeSession({
      ...newest,
      revision: '4',
      itemCount: 0,
      updatedAt: '2026-04-09T09:00:00.000Z',
    });
    listMock.mockResolvedValue([newest]);
    snapshotMock
      .mockResolvedValueOnce({
        revision: '5',
        session: newest,
        history: {
          items: [makeWireHistoryItem(1, { txt: 'newest state' })],
          hasMore: false,
          total: 1,
        },
      })
      .mockResolvedValueOnce({
        revision: '4',
        session: stale,
        history: { items: [], hasMore: false, total: 0 },
      })
      .mockResolvedValueOnce({
        revision: '5',
        unchanged: true,
      });

    await store.loadSessions(newest.projectId);
    await store.loadSessionSnapshot(newest.projectId, newest.id);
    await store.loadSessionSnapshot(newest.projectId, newest.id);
    expect(store.getBlocks(newest.id).map(block => block.text)).toEqual(['newest state']);

    await store.loadSessionSnapshot(newest.projectId, newest.id, { conditional: true });
    expect(snapshotMock).toHaveBeenLastCalledWith(
      newest.projectId,
      newest.id,
      expect.objectContaining({ knownRevision: '5' })
    );
    expect(store.isSessionSnapshotCurrent(newest.id, '5')).toBe(true);
  });

  it('merges incremental catch-up items from the hydrated event cursor', async () => {
    const store = useWebSessionStore();
    const baseline = makeSession({
      id: 'session-catch-up',
      revision: '5',
      eventCursor: '5:9223372036854775807',
      itemCount: 1,
    });
    const caughtUp = makeSession({
      ...baseline,
      revision: '7',
      eventCursor: '7:9223372036854775807',
      itemCount: 2,
    });
    listMock.mockResolvedValue([baseline]);
    snapshotMock.mockResolvedValue({
      revision: '5',
      historyEpoch: '1',
      eventCursor: '5:9223372036854775807',
      session: baseline,
      history: {
        items: [makeWireHistoryItem(1, { id: 'assistant-1', txt: 'draft', es: 5 })],
        hasMore: false,
        total: 1,
      },
    });
    catchUpMock.mockResolvedValue({
      revision: '7',
      historyEpoch: '1',
      nextEventCursor: '7:9223372036854775807',
      targetEventCursor: '7:9223372036854775807',
      hasMore: false,
      resetRequired: false,
      session: caughtUp,
      items: [
        makeWireHistoryItem(1, { id: 'assistant-1', txt: 'complete', es: 6 }),
        makeWireHistoryItem(2, { id: 'assistant-2', txt: 'next', es: 7 }),
      ],
      total: 2,
      pendingEpoch: 'process-1',
      pendingVersion: 0,
      pendingInputs: [],
      scheduledInputs: [],
      subAgents: [],
    });

    await store.loadSessions(baseline.projectId);
    await store.loadSessionSnapshot(baseline.projectId, baseline.id);
    await store.catchUpSession(baseline.projectId, baseline.id);

    expect(catchUpMock).toHaveBeenCalledWith(
      baseline.projectId,
      baseline.id,
      expect.objectContaining({
        afterEventCursor: '5:9223372036854775807',
        historyEpoch: '1',
        limit: 80,
      })
    );
    expect(store.getBlocks(baseline.id).map(item => item.text)).toEqual(['complete', 'next']);
    expect(snapshotMock).toHaveBeenCalledTimes(1);
  });

  it('does not let a stale catch-up response overwrite a newer realtime item', async () => {
    const store = useWebSessionStore();
    const baseline = makeSession({
      id: 'session-catch-up-race',
      revision: '5',
      eventCursor: '5:9223372036854775807',
      itemCount: 1,
    });
    const realtime = makeSession({
      ...baseline,
      revision: '7',
      eventCursor: '7:9223372036854775807',
    });
    let resolveCatchUp!: (value: unknown) => void;
    const catchUpPromise = new Promise(resolve => {
      resolveCatchUp = resolve;
    });
    listMock.mockResolvedValue([baseline]);
    snapshotMock.mockResolvedValue({
      revision: '5',
      historyEpoch: '1',
      eventCursor: '5:9223372036854775807',
      session: baseline,
      history: {
        items: [makeWireHistoryItem(1, { id: 'assistant-1', txt: 'draft', es: 5 })],
        hasMore: false,
        total: 1,
      },
    });
    catchUpMock.mockReturnValueOnce(catchUpPromise).mockResolvedValueOnce({
      revision: '7',
      historyEpoch: '1',
      nextEventCursor: '7:9223372036854775807',
      targetEventCursor: '7:9223372036854775807',
      hasMore: false,
      resetRequired: false,
      session: realtime,
      items: [],
      total: 1,
      pendingEpoch: 'process-1',
      pendingVersion: 0,
      pendingInputs: [],
      scheduledInputs: [],
      subAgents: [],
    });

    await store.loadSessions(baseline.projectId);
    await store.loadSessionSnapshot(baseline.projectId, baseline.id);
    store.setActiveSession(baseline.projectId, baseline.id);
    await store.openEventStream();

    const catchUp = store.catchUpSession(baseline.projectId, baseline.id);
    await vi.waitFor(() => expect(catchUpMock).toHaveBeenCalledTimes(1));
    findSocket('/api/v1/web-sessions/events')?.dispatch({
      v: 1,
      k: 'evt',
      sid: baseline.id,
      rev: '7',
      ts: Date.now(),
      op: 'hist_item',
      s: toWireSession(realtime),
      i: makeWireHistoryItem(1, {
        id: 'assistant-1',
        kd: 'assistant',
        tp: 'agent_message',
        txt: 'realtime',
        es: 7,
      }),
    });
    resolveCatchUp({
      revision: '6',
      historyEpoch: '1',
      nextEventCursor: '6:9223372036854775807',
      targetEventCursor: '6:9223372036854775807',
      hasMore: false,
      resetRequired: false,
      session: { ...baseline, revision: '6', eventCursor: '6:9223372036854775807' },
      items: [
        makeWireHistoryItem(1, {
          id: 'assistant-1',
          kd: 'assistant',
          tp: 'agent_message',
          txt: 'stale catch-up',
          es: 6,
        }),
      ],
      total: 1,
      pendingEpoch: 'process-1',
      pendingVersion: 0,
      pendingInputs: [],
      scheduledInputs: [],
      subAgents: [],
    });
    await catchUp;

    expect(store.getBlocks(baseline.id)[0]?.text).toBe('realtime');
    expect(catchUpMock).toHaveBeenCalledTimes(2);
    expect(catchUpMock).toHaveBeenLastCalledWith(
      baseline.projectId,
      baseline.id,
      expect.objectContaining({ afterEventCursor: '7:1', historyEpoch: '1' })
    );
  });

  it('falls back to a snapshot when catch-up reports a new history epoch', async () => {
    const store = useWebSessionStore();
    const baseline = makeSession({
      id: 'session-catch-up-reset',
      revision: '5',
      historyEpoch: '1',
      eventCursor: '5:9223372036854775807',
    });
    const replaced = makeSession({
      ...baseline,
      revision: '8',
      historyEpoch: '2',
      eventCursor: '8:9223372036854775807',
    });
    listMock.mockResolvedValue([baseline]);
    snapshotMock
      .mockResolvedValueOnce({
        revision: '5',
        historyEpoch: '1',
        eventCursor: '5:9223372036854775807',
        session: baseline,
        history: {
          items: [makeWireHistoryItem(1, { id: 'old-item', txt: 'old' })],
          hasMore: false,
          total: 1,
        },
      })
      .mockResolvedValueOnce({
        revision: '8',
        historyEpoch: '2',
        eventCursor: '8:9223372036854775807',
        session: replaced,
        history: {
          items: [makeWireHistoryItem(1, { id: 'new-item', txt: 'rebuilt' })],
          hasMore: false,
          total: 1,
        },
      });
    catchUpMock.mockResolvedValue({
      revision: '8',
      historyEpoch: '2',
      nextEventCursor: '8:9223372036854775807',
      targetEventCursor: '8:9223372036854775807',
      hasMore: false,
      resetRequired: true,
      session: replaced,
      items: [],
      total: 1,
      pendingEpoch: 'process-1',
      pendingVersion: 0,
      pendingInputs: [],
      scheduledInputs: [],
      subAgents: [],
    });

    await store.loadSessions(baseline.projectId);
    await store.loadSessionSnapshot(baseline.projectId, baseline.id);
    await store.catchUpSession(baseline.projectId, baseline.id);

    expect(snapshotMock).toHaveBeenCalledTimes(2);
    expect(store.getBlocks(baseline.id).map(item => item.text)).toEqual(['rebuilt']);
  });

  it('falls back once when catch-up returns a hasMore page without cursor progress', async () => {
    const store = useWebSessionStore();
    const baseline = makeSession({
      id: 'session-catch-up-stalled',
      revision: '5',
      historyEpoch: '1',
      eventCursor: '5:9223372036854775807',
    });
    const rebuilt = makeSession({
      ...baseline,
      revision: '6',
      eventCursor: '6:9223372036854775807',
    });
    listMock.mockResolvedValue([baseline]);
    snapshotMock
      .mockResolvedValueOnce({
        revision: '5',
        historyEpoch: '1',
        eventCursor: '5:9223372036854775807',
        session: baseline,
        history: {
          items: [makeWireHistoryItem(1, { id: 'old-item', txt: 'old' })],
          hasMore: false,
          total: 1,
        },
      })
      .mockResolvedValueOnce({
        revision: '6',
        historyEpoch: '1',
        eventCursor: '6:9223372036854775807',
        session: rebuilt,
        history: {
          items: [makeWireHistoryItem(1, { id: 'new-item', txt: 'rebuilt' })],
          hasMore: false,
          total: 1,
        },
      });
    catchUpMock.mockResolvedValue({
      revision: '5',
      historyEpoch: '1',
      nextEventCursor: '5:9223372036854775807',
      targetEventCursor: '5:9223372036854775807',
      hasMore: true,
      resetRequired: false,
      session: baseline,
      items: [],
      total: 1,
      pendingEpoch: 'process-1',
      pendingVersion: 0,
      pendingInputs: [],
      scheduledInputs: [],
      subAgents: [],
    });

    await store.loadSessions(baseline.projectId);
    await store.loadSessionSnapshot(baseline.projectId, baseline.id);
    await store.catchUpSession(baseline.projectId, baseline.id);

    expect(catchUpMock).toHaveBeenCalledOnce();
    expect(snapshotMock).toHaveBeenCalledTimes(2);
    expect(store.getBlocks(baseline.id).map(item => item.text)).toEqual(['rebuilt']);
  });

  it('marks unread state with the attention revision instead of content revision', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-mark-read',
      revision: '5',
      attentionRevision: '3',
      hasUnread: true,
    });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    const request = store.markSessionRead(session.id);
    const commandSocket = await waitForCommandCount(1);
    const sent = commandSocket?.sent[0] as
      | { rid: string; sid: string; op: string; p: Record<string, unknown> }
      | undefined;
    expect(sent).toMatchObject({
      sid: session.id,
      op: 'mark_read',
      p: { ar: '3' },
    });
    if (!sent) {
      throw new Error('mark_read command was not sent');
    }
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: sent.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'mark_read',
      ok: 1,
      p: { hasUnread: false, attentionRevision: '4' },
    });
    await request;

    expect(store.getSessions(session.projectId)[0]).toMatchObject({
      revision: '5',
      attentionRevision: '4',
      hasUnread: false,
    });
  });

  it('keeps attention state monotonic across stale summaries', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-attention-race',
      revision: '5',
      attentionRevision: '3',
      hasUnread: true,
    });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    const request = store.markSessionRead(session.id);
    const commandSocket = await waitForCommandCount(1);
    const sent = commandSocket?.sent[0] as { rid?: string } | undefined;
    if (!sent?.rid) {
      throw new Error('mark_read command was not sent');
    }
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: sent.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'mark_read',
      ok: 1,
      p: { hasUnread: false, attentionRevision: '4' },
    });
    await request;

    reconcileMock.mockResolvedValue({
      items: [
        makeSession({
          ...session,
          revision: '6',
          attentionRevision: '3',
          hasUnread: true,
          status: 'done',
          assistantState: null,
          statusUpdatedAt: '2026-04-09T10:05:00.000Z',
        }),
      ],
      missingIds: [],
    });
    await store.reconcileRecentSessions();

    expect(store.getSessions(session.projectId)[0]).toMatchObject({
      revision: '6',
      status: 'done',
      attentionRevision: '4',
      hasUnread: false,
    });

    await store.openEventStream();
    findSocket('/api/v1/web-sessions/events')?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      rev: '7',
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...session,
        revision: '7',
        attentionRevision: '5',
        hasUnread: true,
        status: 'done',
        assistantState: null,
      }),
    });

    expect(store.getSessions(session.projectId)[0]).toMatchObject({
      revision: '7',
      attentionRevision: '5',
      hasUnread: true,
    });
  });

  it('invalidates cleaned histories without removing session summaries', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-history-cleanup',
      revision: '8',
      status: 'done',
      assistantState: null,
      itemCount: 1,
      turnCount: 1,
    });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      revision: '8',
      session,
      history: {
        items: [makeWireHistoryItem(1, { txt: 'cached history' })],
        hasMore: false,
        total: 1,
      },
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    expect(store.getBlocks(session.id)).toHaveLength(1);

    store.invalidateCleanedHistories([session.id]);

    expect(store.getBlocks(session.id)).toEqual([]);
    expect(store.getSessions(session.projectId)).toHaveLength(1);
    expect(store.getSessions(session.projectId)[0]).toMatchObject({
      id: session.id,
      revision: undefined,
      syncState: 'missing',
      lastSyncMode: null,
      lastSyncedAt: null,
      itemCount: 0,
      turnCount: 0,
    });
  });

  it('drops websocket events older than the applied session revision', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-stale-event', revision: '5', itemCount: 1 });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      revision: '5',
      session,
      history: {
        items: [makeWireHistoryItem(1, { txt: 'baseline' })],
        hasMore: false,
        total: 1,
      },
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      rev: '6',
      ts: Date.now(),
      op: 'hist_item',
      i: makeWireHistoryItem(2, { txt: 'new event' }),
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      rev: '5',
      ts: Date.now(),
      op: 'hist_item',
      i: makeWireHistoryItem(3, { txt: 'stale event' }),
    });

    expect(store.getBlocks(session.id).map(block => block.text)).toEqual(['baseline', 'new event']);
  });

  it('singleflights resync notifications through conditional HTTP hydration', async () => {
    const store = useWebSessionStore();
    const baseline = makeSession({
      id: 'session-resync',
      revision: '5',
      itemCount: 1,
    });
    const hydrated = makeSession({
      ...baseline,
      revision: '6',
      itemCount: 2,
      updatedAt: '2026-04-09T10:00:02.000Z',
    });
    listMock.mockResolvedValue([baseline]);
    snapshotMock
      .mockResolvedValueOnce({
        revision: '5',
        session: baseline,
        history: {
          items: [makeWireHistoryItem(1, { txt: 'baseline' })],
          hasMore: false,
          total: 1,
        },
      })
      .mockResolvedValue({
        revision: '6',
        session: hydrated,
        history: {
          items: [
            makeWireHistoryItem(1, { txt: 'baseline' }),
            makeWireHistoryItem(2, { txt: 'hydrated' }),
          ],
          hasMore: false,
          total: 2,
        },
      });

    await store.loadSessions(baseline.projectId);
    await store.loadSessionSnapshot(baseline.projectId, baseline.id);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');

    const resyncFrame = {
      v: 1,
      k: 'evt',
      sid: baseline.id,
      rev: '6',
      ts: Date.now(),
      op: 'resync_required',
      p: { reason: 'history_reconciled' },
    };
    eventSocket?.dispatch(resyncFrame);
    await flushMicrotasks();
    expect(snapshotMock).toHaveBeenCalledTimes(1);

    store.setEventSessionFocus(baseline.id);
    eventSocket?.dispatch(resyncFrame);
    eventSocket?.dispatch(resyncFrame);

    await vi.waitFor(() =>
      expect(store.getBlocks(baseline.id).map(block => block.text)).toEqual([
        'baseline',
        'hydrated',
      ])
    );
    expect(snapshotMock).toHaveBeenCalledTimes(2);
    expect(snapshotMock).toHaveBeenLastCalledWith(
      baseline.projectId,
      baseline.id,
      expect.objectContaining({
        limit: 80,
        signal: expect.any(AbortSignal),
      })
    );
    eventSocket?.dispatch(resyncFrame);
    await flushMicrotasks();
    expect(snapshotMock).toHaveBeenCalledTimes(2);
  });

  it('aborts resync hydration and ignores later notices after event focus is cleared', async () => {
    const store = useWebSessionStore();
    const baseline = makeSession({
      id: 'session-resync-focus-cleared',
      revision: '5',
      itemCount: 1,
    });
    listMock.mockResolvedValue([baseline]);
    snapshotMock.mockResolvedValueOnce({
      revision: '5',
      session: baseline,
      history: {
        items: [makeWireHistoryItem(1, { txt: 'baseline' })],
        hasMore: false,
        total: 1,
      },
    });
    snapshotMock.mockImplementationOnce(
      (
        _projectId: string,
        _sessionId: string,
        options: { signal: AbortSignal }
      ): Promise<WebSessionSnapshot> =>
        new Promise((_resolve, reject) => {
          options.signal.addEventListener(
            'abort',
            () => {
              const error = new Error('aborted');
              error.name = 'AbortError';
              reject(error);
            },
            { once: true }
          );
        })
    );

    await store.loadSessions(baseline.projectId);
    await store.loadSessionSnapshot(baseline.projectId, baseline.id);
    await store.openEventStream();
    store.setEventSessionFocus(baseline.id);
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    const resyncFrame = {
      v: 1,
      k: 'evt',
      sid: baseline.id,
      rev: '6',
      ts: Date.now(),
      op: 'resync_required',
      p: { reason: 'history_reconciled' },
    };

    eventSocket?.dispatch(resyncFrame);
    await vi.waitFor(() => expect(snapshotMock).toHaveBeenCalledTimes(2));
    const hydrationSignal = snapshotMock.mock.calls[1]?.[2]?.signal as AbortSignal | undefined;
    expect(hydrationSignal?.aborted).toBe(false);

    store.setEventSessionFocus('');
    await vi.waitFor(() => expect(hydrationSignal?.aborted).toBe(true));
    eventSocket?.dispatch(resyncFrame);
    await flushMicrotasks();
    expect(snapshotMock).toHaveBeenCalledTimes(2);
  });

  it('bounds snapshot requests when resync notices keep advancing ahead of stale HTTP data', async () => {
    vi.useFakeTimers();
    window.setTimeout = setTimeout;
    window.clearTimeout = clearTimeout;
    window.setInterval = setInterval;
    window.clearInterval = clearInterval;

    try {
      const store = useWebSessionStore();
      const baseline = makeSession({
        id: 'session-resync-hot-stream',
        revision: '5',
        itemCount: 1,
      });
      listMock.mockResolvedValue([baseline]);
      snapshotMock.mockResolvedValue({
        revision: '5',
        historyEpoch: '1',
        eventCursor: '5:9223372036854775807',
        session: baseline,
        history: {
          items: [makeWireHistoryItem(1, { txt: 'baseline' })],
          hasMore: false,
          total: 1,
        },
      });

      await store.loadSessions(baseline.projectId);
      await store.loadSessionSnapshot(baseline.projectId, baseline.id);
      await store.openEventStream();
      store.setEventSessionFocus(baseline.id);
      const eventSocket = findSocket('/api/v1/web-sessions/events');
      for (let revision = 6; revision <= 40; revision += 1) {
        eventSocket?.dispatch({
          v: 1,
          k: 'evt',
          sid: baseline.id,
          rev: String(revision),
          ts: Date.now(),
          op: 'resync_required',
          p: { reason: 'history_reconciled' },
        });
      }

      await flushMicrotasks();
      await vi.advanceTimersByTimeAsync(20_000);
      await flushMicrotasks();

      // One initial load plus at most the three attempts in the unresolved
      // circuit. Newer notices must not create one request per revision.
      expect(snapshotMock.mock.calls.length).toBeLessThanOrEqual(4);
    } finally {
      vi.useRealTimers();
    }
  });

  it('loads archived session snapshots over HTTP without replacing the active session', async () => {
    const store = useWebSessionStore();
    const currentSession = makeSession({
      id: 'session-current',
      title: 'Current Session',
    });
    const archivedSession = makeSession({
      id: 'session-archived',
      title: 'Archived Session',
      archivedAt: '2026-04-09T11:00:00.000Z',
      status: 'done',
      syncState: 'fresh',
      itemCount: 1,
    });

    listMock.mockResolvedValue([currentSession]);
    queryArchivedMock.mockResolvedValue({
      items: [archivedSession],
      total: 1,
      hasMore: false,
      nextOffset: 1,
    });
    snapshotMock.mockResolvedValue({
      session: archivedSession,
      history: {
        items: [
          {
            id: 'history-archived',
            oi: 1,
            kd: 'assistant',
            tp: 'message',
            txt: 'Recovered archived history',
            ts2: Date.parse('2026-04-09T11:05:00.000Z'),
          },
        ],
        hasMore: false,
        total: 1,
      },
    });

    await store.loadSessions(currentSession.projectId);
    await store.loadArchivedSessions([currentSession.projectId], {
      reset: true,
      limit: 20,
    });
    store.setActiveSession(currentSession.projectId, currentSession.id);

    await store.loadSessionSnapshot(archivedSession.projectId, archivedSession.id, {
      rememberActive: false,
    });

    expect(snapshotMock).toHaveBeenCalledWith(
      archivedSession.projectId,
      archivedSession.id,
      expect.objectContaining({
        limit: 80,
        signal: expect.any(AbortSignal),
      })
    );
    expect(store.getActiveSessionId(currentSession.projectId)).toBe(currentSession.id);
    expect(store.getBlocks(archivedSession.id)).toHaveLength(1);
  });

  it('applies goal_get ack data without waiting for or fetching a snapshot', async () => {
    const store = useWebSessionStore();
    const updatedAt = '2026-04-09T10:00:00.000Z';
    const session = makeSession({
      id: 'session-goal-refresh',
      revision: '7',
      goal: {
        threadId: 'native-1',
        objective: 'Keep the objective',
        status: 'active',
        tokenBudget: null,
        tokensUsed: 10,
        timeUsedSeconds: 20,
        createdAt: updatedAt,
        updatedAt,
      },
    });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    const refresh = store.refreshGoal(session.id);
    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await flushMicrotasks();
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }
    const sent = commandSocket?.sent[0] as { rid: string; op: string } | undefined;
    expect(sent?.op).toBe('goal_get');
    if (!sent) {
      throw new Error('goal_get command was not sent');
    }
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: sent.rid,
      sid: session.id,
      rev: '8',
      ts: Date.now(),
      op: 'goal_get',
      ok: 1,
      p: {
        goal: {
          tid: 'native-1',
          obj: 'Keep the objective',
          st: 'active',
          tu: 42,
          tsu: 30,
          ca: Date.parse(updatedAt),
          ua: Date.parse(updatedAt),
        },
      },
    });

    await refresh;
    expect(snapshotMock).not.toHaveBeenCalled();
    expect(store.getSessions(session.projectId)[0]?.goal).toMatchObject({
      updatedAt,
      tokensUsed: 42,
      timeUsedSeconds: 30,
    });
  });

  it('sends goal_bootstrap for draft codex goal starts', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-goal-bootstrap',
      nativeSessionId: null,
      status: 'idle',
      assistantState: null,
      goal: null,
    });

    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId);

    const socket = findSocket('/api/v1/web-sessions/ws');
    expect(socket).toBeNull();

    const promise = store.bootstrapGoal(session.id, 'Start from the goal immediately', 'active');
    await flushMicrotasks();

    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    expect(commandSocket).not.toBeNull();
    const sent = commandSocket!.sent[0] as {
      op: string;
      rid: string;
      sid: string;
      p: Record<string, unknown>;
    };
    expect(sent.op).toBe('goal_bootstrap');
    expect(sent.sid).toBe(session.id);
    expect(sent.p).toEqual({
      obj: 'Start from the goal immediately',
      st: 'active',
    });

    commandSocket!.dispatch({
      v: 1,
      k: 'ack',
      rid: sent.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'goal_bootstrap',
      ok: 1,
    });
    commandSocket!.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'session',
      s: {
        ...toWireSession({
          ...session,
          nativeSessionId: 'native-bootstrapped',
          status: 'running',
          assistantState: 'working',
          goal: {
            threadId: 'native-bootstrapped',
            objective: 'Start from the goal immediately',
            status: 'active',
            tokenBudget: null,
            tokensUsed: 0,
            timeUsedSeconds: 0,
            createdAt: '2026-04-09T10:00:00.000Z',
            updatedAt: '2026-04-09T10:00:00.000Z',
          },
        }),
        goal: {
          tid: 'native-bootstrapped',
          obj: 'Start from the goal immediately',
          st: 'active',
          tu: 0,
          tsu: 0,
          ca: Date.parse('2026-04-09T10:00:00.000Z'),
          ua: Date.parse('2026-04-09T10:00:00.000Z'),
        },
      },
    });

    await promise;

    const updated = store.getSessions(session.projectId).find(item => item.id === session.id);
    expect(updated?.nativeSessionId).toBe('native-bootstrapped');
    expect(updated?.goal?.objective).toBe('Start from the goal immediately');
  });

  it('passes abort signals through snapshot loads triggered by tab activation', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-signal',
      title: 'Signal Session',
    });
    const controller = new AbortController();

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id, {
      signal: controller.signal,
    });

    expect(snapshotMock).toHaveBeenCalledWith(
      session.projectId,
      session.id,
      expect.objectContaining({
        limit: 80,
        signal: expect.any(AbortSignal),
      })
    );
  });

  it('loads older history pages over HTTP and merges them into the session timeline', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-history',
      status: 'done',
      itemCount: 3,
      syncState: 'fresh',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          {
            id: 'history-2',
            oi: 2,
            kd: 'assistant',
            tp: 'message',
            txt: 'second',
            ts2: Date.parse('2026-04-09T10:02:00.000Z'),
          },
          {
            id: 'history-3',
            oi: 3,
            kd: 'assistant',
            tp: 'message',
            txt: 'third',
            ts2: Date.parse('2026-04-09T10:03:00.000Z'),
          },
        ],
        hasMore: true,
        beforeCursor: '2',
        total: 3,
      },
    });
    historyMock.mockResolvedValue({
      items: [
        {
          id: 'history-1',
          oi: 1,
          kd: 'user',
          tp: 'message',
          txt: 'first',
          ts2: Date.parse('2026-04-09T10:01:00.000Z'),
        },
      ],
      hasMore: false,
      beforeCursor: '',
      total: 3,
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.loadMoreHistory(session.id, 80);

    expect(historyMock).toHaveBeenCalledWith(session.projectId, session.id, {
      beforeCursor: '2',
      limit: 80,
    });
    expect(store.getBlocks(session.id).map(item => item.orderIndex)).toEqual([1, 2, 3]);
    expect(store.getHistoryMeta(session.id)).toMatchObject({
      hasMore: false,
      beforeCursor: '',
      total: 3,
      loading: false,
    });
  });

  it('fetches a chronological preview window without replacing the realtime timeline', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-history-start',
      status: 'done',
      itemCount: 3,
      syncState: 'fresh',
    });

    listMock.mockResolvedValue([session]);
    historyMock.mockResolvedValue({
      items: [
        {
          id: 'history-1',
          oi: 1,
          kd: 'user',
          tp: 'message',
          txt: 'first',
          ts2: Date.parse('2026-04-09T10:01:00.000Z'),
        },
        {
          id: 'history-2',
          oi: 2,
          kd: 'assistant',
          tp: 'message',
          txt: 'second',
          ts2: Date.parse('2026-04-09T10:02:00.000Z'),
        },
      ],
      hasMore: false,
      hasLater: true,
      afterCursor: '2',
      total: 3,
    });

    await store.loadSessions(session.projectId);
    const page = await store.fetchHistoryWindow(session.id, {
      afterCursor: '0',
      limit: 80,
    });

    expect(historyMock).toHaveBeenCalledWith(session.projectId, session.id, {
      afterCursor: '0',
      limit: 80,
    });
    expect(page.items.map(item => item.orderIndex)).toEqual([1, 2]);
    expect(page).toMatchObject({ hasLater: true, afterCursor: '2', total: 3 });
    expect(store.getBlocks(session.id)).toEqual([]);
  });

  it('restores pending inputs from snapshot responses', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-pending',
      status: 'running',
      assistantState: 'working',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [
        {
          id: 'pending-1',
          mode: 'redirect',
          text: 'Steer follow-up',
          attachmentIds: ['attachment-1'],
          readyAt: '2026-04-09T10:01:05.000Z',
          paused: true,
          status: 'failed',
          attemptCount: 2,
          lastError: 'active turn cannot be steered',
          lastErrorCode: 'activeTurnNotSteerable',
          createdAt: '2026-04-09T10:01:00.000Z',
        },
        {
          id: 'pi-native-1',
          mode: 'queue',
          text: 'Accepted by Pi',
          attachmentIds: [],
          paused: false,
          nativeQueued: true,
          createdAt: '2026-04-09T10:01:01.000Z',
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getPendingInputs(session.id)).toEqual([
      {
        id: 'pending-1',
        mode: 'redirect',
        text: 'Steer follow-up',
        attachmentIds: ['attachment-1'],
        readyAt: Date.parse('2026-04-09T10:01:05.000Z'),
        paused: true,
        status: 'failed',
        attemptCount: 2,
        lastError: 'active turn cannot be steered',
        lastErrorCode: 'activeTurnNotSteerable',
        createdAt: Date.parse('2026-04-09T10:01:00.000Z'),
      },
      {
        id: 'pi-native-1',
        mode: 'queue',
        text: 'Accepted by Pi',
        attachmentIds: [],
        readyAt: null,
        paused: false,
        nativeQueued: true,
        createdAt: Date.parse('2026-04-09T10:01:01.000Z'),
      },
    ]);
  });

  it('restores scheduled inputs from snapshot responses', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-scheduled',
      status: 'idle',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [],
      scheduledInputs: [
        {
          id: 'scheduled-1',
          mode: 'redirect',
          status: 'scheduled',
          text: 'Send later',
          attachmentIds: ['attachment-7'],
          scheduledFor: '2026-04-09T10:05:00.000Z',
          createdAt: '2026-04-09T10:01:00.000Z',
          updatedAt: '2026-04-09T10:01:00.000Z',
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getScheduledInputs(session.id)).toEqual([
      {
        id: 'scheduled-1',
        dependsOnId: '',
        dependencyStatus: 'none',
        action: 'message',
        targetId: '',
        mode: 'interrupt',
        exitPlanMode: false,
        status: 'scheduled',
        lastError: '',
        text: 'Send later',
        attachmentIds: ['attachment-7'],
        scheduleKind: 'at_time',
        scheduledFor: Date.parse('2026-04-09T10:05:00.000Z'),
        idleSince: null,
        blockingReasons: [],
        conditionError: '',
        createdAt: Date.parse('2026-04-09T10:01:00.000Z'),
        updatedAt: Date.parse('2026-04-09T10:01:00.000Z'),
        sentAt: null,
        canceledAt: null,
      },
    ]);
    expect(store.getSessions(session.projectId)[0]?.hasScheduledPlanExecution).toBe(false);
  });

  it('removes pending inputs via the backend command channel and pending events', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-pending-remove',
      status: 'running',
      assistantState: 'working',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [
        {
          id: 'pending-1',
          mode: 'queue',
          text: 'Queued follow-up',
          attachmentIds: [],
          createdAt: '2026-04-09T10:01:00.000Z',
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();

    const removePromise = store.removePendingInput(session.id, 'pending-1');
    for (let attempt = 0; attempt < 5; attempt += 1) {
      const socket = findSocket('/api/v1/web-sessions/ws');
      if (socket?.sent.length) {
        break;
      }
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
    }

    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(commandSocket).not.toBeNull();
    expect(eventSocket).not.toBeNull();

    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'pending_del',
      p: {
        id: 'pending-1',
      },
    });

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    eventSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_del',
      ok: 1,
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'pending',
      pi: [],
    });

    await removePromise;
    expect(store.getPendingInputs(session.id)).toEqual([]);
  });

  it('treats a stale pending delete as idempotent without hydrating runtime state', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-pending-stale',
      status: 'running',
      assistantState: 'working',
    });
    const snapshotWithPending = (id: string, text: string) => ({
      session,
      history: { items: [], hasMore: false, total: 0 },
      pendingInputs: [
        {
          id,
          mode: 'queue',
          text,
          attachmentIds: [],
          createdAt: '2026-04-09T10:01:00.000Z',
        },
      ],
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValueOnce(snapshotWithPending('pending-delete', 'Delete me'));

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    const dispatchMissingPendingError = async (operation: string) => {
      let commandSocket = findSocket('/api/v1/web-sessions/ws');
      for (
        let attempt = 0;
        attempt < 5 &&
        (commandSocket?.sent.at(-1) as { op?: string } | undefined)?.op !== operation;
        attempt += 1
      ) {
        await Promise.resolve();
        await new Promise(resolve => setTimeout(resolve, 0));
        commandSocket = findSocket('/api/v1/web-sessions/ws');
      }
      const requestId = String(
        (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
      );
      commandSocket?.dispatch({
        v: 1,
        k: 'err',
        rid: requestId,
        sid: session.id,
        ts: Date.now(),
        code: 'not_found',
        msg: 'pending input not found',
      });
    };

    const deletePromise = store.removePendingInput(session.id, 'pending-delete');
    await dispatchMissingPendingError('pending_del');
    await expect(deletePromise).resolves.toBeUndefined();
    expect(store.getPendingInputs(session.id)).toEqual([]);
    expect(store.lastError).toBeNull();
    expect(snapshotMock).toHaveBeenCalledTimes(1);
  });

  it('keeps non-stale pending command failures visible', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-pending-real-error' });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    const removePromise = store.removePendingInput(session.id, 'pending-1');
    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }
    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'err',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      code: 'internal',
      msg: 'database unavailable',
    });

    await expect(removePromise).rejects.toThrow('database unavailable');
    expect(store.lastError).toBe('database unavailable');
    expect(snapshotMock).not.toHaveBeenCalled();
  });

  it('updates pending inputs via the backend command channel and pending events', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-pending-update',
      status: 'running',
      assistantState: 'working',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [
        {
          id: 'pending-1',
          mode: 'queue',
          text: 'Queued follow-up',
          attachmentIds: [],
          createdAt: '2026-04-09T10:01:00.000Z',
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();

    const updatePromise = store.updatePendingInput(session.id, 'pending-1', 'Updated follow-up');

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'pending_update',
      p: {
        id: 'pending-1',
        txt: 'Updated follow-up',
        paused: true,
      },
    });

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_update',
      ok: 1,
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'pending',
      pi: [
        {
          id: 'pending-1',
          m: 'queue',
          txt: 'Updated follow-up',
          atts: [],
          ca: Date.parse('2026-04-09T10:01:00.000Z'),
        },
      ],
    });

    await updatePromise;
    expect(store.getPendingInputs(session.id)[0]).toMatchObject({
      text: 'Updated follow-up',
      mode: 'queue',
      localStaged: true,
      paused: false,
    });
    expect(store.getPendingInputs(session.id)[0]?.readyAt).toBeGreaterThan(Date.now());
  });

  it('pauses server input immediately and resumes it after a local undo window', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.parse('2026-04-09T10:02:00.000Z'));
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-pending-pause',
      status: 'running',
      assistantState: 'working',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [
        {
          id: 'pending-1',
          mode: 'redirect',
          text: 'Steer follow-up',
          attachmentIds: [],
          readyAt: '2026-04-09T10:01:05.000Z',
          paused: false,
          createdAt: '2026-04-09T10:01:00.000Z',
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();

    const pausePromise = store.pausePendingInput(session.id, 'pending-1');
    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await flushMicrotasks();
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(commandSocket?.sent.at(-1)).toMatchObject({
      op: 'pending_update',
      p: { id: 'pending-1', paused: true },
    });
    const pauseRequestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: pauseRequestId,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_update',
      ok: 1,
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'pending',
      pi: [
        {
          id: 'pending-1',
          m: 'redirect',
          txt: 'Steer follow-up',
          ps: true,
          ca: Date.parse('2026-04-09T10:01:00.000Z'),
        },
      ],
    });
    await pausePromise;
    expect(store.getPendingInputs(session.id)[0]).toMatchObject({
      paused: true,
      readyAt: null,
    });

    const commandCountBeforeResume = commandSocket?.sent.length ?? 0;
    await store.resumePendingInput(session.id, 'pending-1');
    expect(commandSocket?.sent).toHaveLength(commandCountBeforeResume);
    expect(store.getPendingInputs(session.id)[0]).toMatchObject({
      paused: false,
      readyAt: Date.parse('2026-04-09T10:02:05.000Z'),
      localStaged: true,
    });

    await vi.advanceTimersByTimeAsync(5_000);
    await flushMicrotasks();
    expect(commandSocket?.sent.at(-1)).toMatchObject({
      op: 'pending_update',
      p: { id: 'pending-1', paused: false },
    });
    const resumeRequestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: resumeRequestId,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_update',
      ok: 1,
      pe: 'process-1',
      pv: 3,
      pi: [
        {
          id: 'pending-1',
          m: 'redirect',
          txt: 'Steer follow-up',
          ps: false,
          ca: Date.parse('2026-04-09T10:01:00.000Z'),
        },
      ],
    });
    await flushMicrotasks();
    expect(store.getPendingInputs(session.id)[0]).toMatchObject({
      paused: false,
      readyAt: null,
    });
  });

  it('reorders pending inputs via the backend command channel and pending events', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-pending-reorder',
      status: 'running',
      assistantState: 'working',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [
        {
          id: 'pending-1',
          mode: 'queue',
          text: 'First queued follow-up',
          attachmentIds: [],
          createdAt: '2026-04-09T10:01:00.000Z',
        },
        {
          id: 'pending-2',
          mode: 'queue',
          text: 'Second queued follow-up',
          attachmentIds: [],
          createdAt: '2026-04-09T10:02:00.000Z',
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();

    const reorderPromise = store.reorderPendingInput(session.id, 'pending-2', 'queue', 0);

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'pending_reorder',
      p: {
        id: 'pending-2',
        mode: 'queue',
        idx: 0,
      },
    });

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_reorder',
      ok: 1,
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'pending',
      pi: [
        {
          id: 'pending-2',
          m: 'queue',
          txt: 'Second queued follow-up',
          atts: [],
          ca: Date.parse('2026-04-09T10:02:00.000Z'),
        },
        {
          id: 'pending-1',
          m: 'queue',
          txt: 'First queued follow-up',
          atts: [],
          ca: Date.parse('2026-04-09T10:01:00.000Z'),
        },
      ],
    });

    await reorderPromise;
    expect(store.getPendingInputs(session.id).map(item => item.id)).toEqual([
      'pending-2',
      'pending-1',
    ]);
  });

  it('clears pending inputs via the backend command channel and pending events', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-pending-clear',
      status: 'running',
      assistantState: 'working',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [
        {
          id: 'pending-1',
          mode: 'queue',
          text: 'Queued follow-up',
          attachmentIds: [],
          createdAt: '2026-04-09T10:01:00.000Z',
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();

    const clearPromise = store.clearPendingInputs(session.id);

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'pending_clear',
      p: {},
    });

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_clear',
      ok: 1,
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'pending',
      pi: [],
    });

    await clearPromise;
    expect(store.getPendingInputs(session.id)).toEqual([]);
  });

  it('sends queued input immediately and applies the pending snapshot from its ack', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-pending-optimistic',
      status: 'running',
      assistantState: 'working',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      revision: '2',
      session: makeSession({
        ...session,
        revision: '2',
        status: 'running',
        assistantState: 'working',
      }),
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [],
    });

    await store.loadSessions(session.projectId);

    const sendPromise = store.sendMessage(session.id, 'Optimistic queued follow-up', [], 'queue');

    expect(store.getPendingInputs(session.id)).toEqual([]);

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    expect(commandSocket).not.toBeNull();
    const command = commandSocket?.sent.at(-1) as
      | { rid?: string; p?: { pid?: string } }
      | undefined;
    expect(command).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'send',
      p: {
        txt: 'Optimistic queued follow-up',
        atts: [],
        mode: 'queue',
        pid: expect.any(String),
      },
    });

    const requestId = String(command?.rid ?? '');
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'send',
      ok: 1,
      pe: 'process-1',
      pv: 1,
      pi: [
        {
          id: command?.p?.pid,
          m: 'queue',
          txt: 'Optimistic queued follow-up',
          ca: Date.now(),
        },
      ],
    });

    await sendPromise;
    expect(snapshotMock).not.toHaveBeenCalled();
    expect(store.getPendingInputs(session.id)).toMatchObject([
      {
        id: command?.p?.pid,
        mode: 'queue',
        text: 'Optimistic queued follow-up',
      },
    ]);
  });

  it('keeps redirects in session storage for five seconds before sending', async () => {
    vi.useFakeTimers();
    const startedAt = Date.parse('2026-08-29T06:00:00.000Z');
    vi.setSystemTime(startedAt);
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-local-redirect',
      status: 'running',
      assistantState: 'working',
    });
    const attachment = {
      id: 'attachment-1',
      name: 'scope.png',
      mime: 'image/png',
      size: 128,
      path: '/tmp/scope.png',
      createdAt: '2026-08-29T05:59:00.000Z',
    };
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    await store.sendMessage(session.id, 'Redirect after undo', [attachment.id], 'redirect', {
      attachments: [attachment],
    });

    const staged = store.getPendingInputs(session.id)[0];
    expect(staged).toMatchObject({
      mode: 'redirect',
      text: 'Redirect after undo',
      readyAt: startedAt + 5_000,
      localStaged: true,
    });
    expect(findSocket('/api/v1/web-sessions/ws')).toBeNull();
    const stored = JSON.parse(
      globalThis.sessionStorage.getItem('kanban-web-session-staged-pending-inputs') ?? '{}'
    );
    expect(stored[session.id][0]).toMatchObject({
      id: staged?.id,
      attachments: [{ id: attachment.id, name: attachment.name }],
    });

    await vi.advanceTimersByTimeAsync(4_999);
    expect(findSocket('/api/v1/web-sessions/ws')).toBeNull();
    await vi.advanceTimersByTimeAsync(1);
    await flushMicrotasks();

    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    const command = commandSocket?.sent.at(-1) as { rid?: string; p?: { pid?: string } };
    expect(command).toMatchObject({
      op: 'send',
      sid: session.id,
      p: {
        txt: 'Redirect after undo',
        atts: [attachment.id],
        mode: 'redirect',
        pid: staged?.id,
      },
    });
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: command.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'send',
      ok: 1,
      pe: 'process-1',
      pv: 1,
      pi: [],
    });
    await flushMicrotasks();
    expect(store.getPendingInputs(session.id)).toEqual([]);
  });

  it('sends a staged redirect immediately when it is changed to queue', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.parse('2026-08-29T06:00:00.000Z'));
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-staged-redirect-to-queue',
      status: 'running',
      assistantState: 'working',
    });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    await store.sendMessage(session.id, 'Queue this now', [], 'redirect');
    const pendingId = store.getPendingInputs(session.id)[0]?.id ?? '';
    const reorderPromise = store.reorderPendingInput(session.id, pendingId, 'queue', 0);
    const commandSocket = await waitForCommandCount(1);
    const command = commandSocket?.sent[0] as { rid?: string } | undefined;
    expect(command).toMatchObject({
      op: 'send',
      sid: session.id,
      p: {
        txt: 'Queue this now',
        mode: 'queue',
        pid: pendingId,
      },
    });

    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: command?.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'send',
      ok: 1,
      pe: 'process-queue',
      pv: 1,
      pi: [{ id: pendingId, m: 'queue', txt: 'Queue this now', ca: Date.now() }],
    });
    await reorderPromise;

    const queued = store.getPendingInputs(session.id)[0];
    expect(queued).toMatchObject({ id: pendingId, mode: 'queue' });
    expect(queued).not.toHaveProperty('localStaged');
    expect(globalThis.sessionStorage.getItem('kanban-web-session-staged-pending-inputs')).toBe(
      null
    );
  });

  it('restores a paused server queue item when a staged promotion is changed back to queue', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.parse('2026-08-29T06:00:00.000Z'));
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-staged-promotion-to-queue',
      status: 'running',
      assistantState: 'working',
    });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      revision: '2',
      pendingEpoch: 'process-restore',
      pendingVersion: 0,
      session,
      history: { items: [], hasMore: false, total: 0 },
      pendingInputs: [
        {
          id: 'pending-restore',
          mode: 'queue',
          text: 'Restore me',
          attachmentIds: [],
          paused: false,
          createdAt: '2026-08-29T05:59:00.000Z',
        },
      ],
    });
    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    const promotePromise = store.reorderPendingInput(session.id, 'pending-restore', 'redirect', 0);
    const commandSocket = await waitForCommandCount(1);
    const pauseCommand = commandSocket?.sent[0] as { rid?: string } | undefined;
    expect(pauseCommand).toMatchObject({
      op: 'pending_update',
      p: { id: 'pending-restore', paused: true },
    });
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: pauseCommand?.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_update',
      ok: 1,
      pe: 'process-restore',
      pv: 1,
      pi: [{ id: 'pending-restore', m: 'queue', txt: 'Restore me', ps: true, ca: Date.now() }],
    });
    await promotePromise;
    expect(store.getPendingInputs(session.id)[0]).toMatchObject({
      id: 'pending-restore',
      mode: 'redirect',
      localStaged: true,
    });

    const restorePromise = store.reorderPendingInput(session.id, 'pending-restore', 'queue', 0);
    await waitForCommandCount(2);
    const reorderCommand = commandSocket?.sent[1] as { rid?: string } | undefined;
    expect(reorderCommand).toMatchObject({
      op: 'pending_reorder',
      p: { id: 'pending-restore', mode: 'queue', idx: 0 },
    });
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: reorderCommand?.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_reorder',
      ok: 1,
      pe: 'process-restore',
      pv: 2,
      pi: [{ id: 'pending-restore', m: 'queue', txt: 'Restore me', ps: true, ca: Date.now() }],
    });

    await waitForCommandCount(3);
    const resumeCommand = commandSocket?.sent[2] as { rid?: string } | undefined;
    expect(resumeCommand).toMatchObject({
      op: 'pending_update',
      p: { id: 'pending-restore', paused: false },
    });
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: resumeCommand?.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'pending_update',
      ok: 1,
      pe: 'process-restore',
      pv: 3,
      pi: [{ id: 'pending-restore', m: 'queue', txt: 'Restore me', ca: Date.now() }],
    });
    await restorePromise;

    const restored = store.getPendingInputs(session.id)[0];
    expect(restored).toMatchObject({ id: 'pending-restore', mode: 'queue', paused: false });
    expect(restored).not.toHaveProperty('localStaged');
  });

  it('restarts a restored staged redirect with a fresh five-second window', async () => {
    vi.useFakeTimers();
    const startedAt = Date.parse('2026-08-29T06:00:00.000Z');
    vi.setSystemTime(startedAt);
    const session = makeSession({ id: 'session-restored-redirect' });
    listMock.mockResolvedValue([session]);
    const firstStore = useWebSessionStore();
    await firstStore.loadSessions(session.projectId);
    await firstStore.sendMessage(session.id, 'Wait after refresh', [], 'redirect');
    await vi.advanceTimersByTimeAsync(4_000);

    vi.clearAllTimers();
    setActivePinia(createPinia());
    const restoredAt = Date.now();
    const restoredStore = useWebSessionStore();
    expect(restoredStore.getPendingInputs(session.id)[0]).toMatchObject({
      text: 'Wait after refresh',
      readyAt: restoredAt + 5_000,
      localStaged: true,
    });

    await vi.advanceTimersByTimeAsync(4_999);
    expect(findSocket('/api/v1/web-sessions/ws')).toBeNull();
    await vi.advanceTimersByTimeAsync(1);
    const commandSocket = await waitForCommandCount(1);
    expect(commandSocket?.sent[0]).toMatchObject({
      op: 'send',
      sid: session.id,
      p: { mode: 'redirect' },
    });
  });

  it('dispatches a restored resume before the server baseline is hydrated', async () => {
    vi.useFakeTimers();
    const startedAt = Date.parse('2026-08-29T06:00:00.000Z');
    vi.setSystemTime(startedAt);
    const sessionId = 'session-restored-resume';
    globalThis.sessionStorage.setItem(
      'kanban-web-session-staged-pending-inputs',
      JSON.stringify({
        [sessionId]: [
          {
            id: 'pending-restored-resume',
            mode: 'redirect',
            text: 'Resume after refresh',
            attachmentIds: [],
            readyAt: startedAt + 1_000,
            paused: false,
            createdAt: startedAt - 1_000,
            localStaged: true,
            action: 'resume',
            attachments: [],
          },
        ],
      })
    );
    useWebSessionStore();

    await vi.advanceTimersByTimeAsync(5_000);
    const commandSocket = await waitForCommandCount(1);
    expect(commandSocket?.sent[0]).toMatchObject({
      op: 'pending_update',
      sid: sessionId,
      p: { id: 'pending-restored-resume', paused: false },
    });
  });

  it('keeps a failed staged redirect paused without automatic retries', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.parse('2026-08-29T06:00:00.000Z'));
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-failed-staged-redirect' });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.sendMessage(session.id, 'Fail once', [], 'redirect');

    await vi.advanceTimersByTimeAsync(5_000);
    const commandSocket = await waitForCommandCount(1);
    const command = commandSocket?.sent[0] as { rid?: string } | undefined;
    commandSocket?.dispatch({
      v: 1,
      k: 'err',
      rid: command?.rid,
      sid: session.id,
      ts: Date.now(),
      op: 'send',
      code: 'unavailable',
      msg: 'runtime unavailable',
    });
    await flushMicrotasks();

    expect(store.getPendingInputs(session.id)[0]).toMatchObject({
      mode: 'redirect',
      paused: true,
      readyAt: null,
      status: 'failed',
      attemptCount: 1,
      lastError: 'runtime unavailable',
      lastErrorCode: 'unavailable',
    });
    await vi.advanceTimersByTimeAsync(60_000);
    expect(commandSocket?.sent).toHaveLength(1);
  });

  it('continues a staged redirect countdown after switching sessions', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.parse('2026-08-29T06:00:00.000Z'));
    const store = useWebSessionStore();
    const first = makeSession({ id: 'session-countdown-first' });
    const second = makeSession({ id: 'session-countdown-second' });
    listMock.mockResolvedValue([first, second]);
    await store.loadSessions(first.projectId);
    store.setActiveSession(first.projectId, first.id);
    await store.sendMessage(first.id, 'Keep counting', [], 'redirect');

    await vi.advanceTimersByTimeAsync(2_500);
    store.setActiveSession(first.projectId, second.id);
    await vi.advanceTimersByTimeAsync(2_500);
    const commandSocket = await waitForCommandCount(1);
    expect(commandSocket?.sent[0]).toMatchObject({
      op: 'send',
      sid: first.id,
      p: { txt: 'Keep counting', mode: 'redirect' },
    });
  });

  it('applies pending clocks independently from durable revisions', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-pending-clock', revision: '9' });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'pending',
      pe: 'process-a',
      pv: 2,
      pi: [{ id: 'pending-a', m: 'queue', txt: 'A', ca: Date.now() }],
      rev: '1',
    });
    expect(store.getPendingInputs(session.id).map(item => item.id)).toEqual(['pending-a']);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'pending',
      pe: 'process-a',
      pv: 2,
      pi: [],
      rev: '99',
    });
    expect(store.getPendingInputs(session.id).map(item => item.id)).toEqual(['pending-a']);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'pending',
      pe: 'process-b',
      pv: 0,
      pi: [],
      rev: '1',
    });
    expect(store.getPendingInputs(session.id)).toEqual([]);
  });

  it('stores scheduled inputs from schedule_send acknowledgements', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-schedule-command',
      status: 'idle',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId);

    const scheduledAt = Date.parse('2026-04-09T10:08:00.000Z');
    const schedulePromise = store.scheduleMessage(
      session.id,
      'Later message',
      [],
      scheduledAt,
      'interrupt'
    );

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'schedule_send',
      p: {
        txt: 'Later message',
        atts: [],
        mode: 'interrupt',
        sk: 'at_time',
        at: scheduledAt,
      },
    });

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'schedule_send',
      ok: 1,
      p: {
        id: 'scheduled-ack-1',
        m: 'interrupt',
        st: 'scheduled',
        txt: 'Later message',
        atts: [],
        sf: scheduledAt,
        ca: scheduledAt - 60_000,
        ua: scheduledAt - 60_000,
      },
    });

    const created = await schedulePromise;
    expect(created).toMatchObject({
      id: 'scheduled-ack-1',
      action: 'message',
      targetId: '',
      mode: 'interrupt',
      status: 'scheduled',
    });
    expect(store.getScheduledInputs(session.id)).toEqual([
      {
        id: 'scheduled-ack-1',
        dependsOnId: '',
        dependencyStatus: 'none',
        action: 'message',
        targetId: '',
        mode: 'interrupt',
        exitPlanMode: false,
        status: 'scheduled',
        lastError: '',
        text: 'Later message',
        attachmentIds: [],
        scheduleKind: 'at_time',
        scheduledFor: scheduledAt,
        idleSince: null,
        blockingReasons: [],
        conditionError: '',
        createdAt: scheduledAt - 60_000,
        updatedAt: scheduledAt - 60_000,
        sentAt: null,
        canceledAt: null,
      },
    ]);
  });

  it('stores when-idle messages without a scheduled timestamp', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-schedule-message-idle',
      status: 'idle',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    const schedulePromise = store.scheduleMessage(
      session.id,
      'Send when idle',
      [],
      { scheduleKind: 'when_idle' },
      'queue'
    );

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    const sent = commandSocket?.sent.at(-1) as
      | { rid?: string; p?: Record<string, unknown> }
      | undefined;
    expect(sent).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'schedule_send',
      p: {
        txt: 'Send when idle',
        atts: [],
        mode: 'queue',
        sk: 'when_idle',
      },
    });
    expect(sent?.p).not.toHaveProperty('at');

    const createdAt = Date.parse('2026-04-09T10:00:00.000Z');
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: String(sent?.rid ?? ''),
      sid: session.id,
      ts: Date.now(),
      op: 'schedule_send',
      ok: 1,
      p: {
        id: 'scheduled-message-idle',
        a: 'message',
        m: 'queue',
        sk: 'when_idle',
        st: 'scheduled',
        txt: 'Send when idle',
        atts: [],
        br: ['git_dirty'],
        ce: '',
        ca: createdAt,
        ua: createdAt,
      },
    });

    await expect(schedulePromise).resolves.toMatchObject({
      id: 'scheduled-message-idle',
      action: 'message',
      scheduleKind: 'when_idle',
      scheduledFor: null,
      blockingReasons: ['git_dirty'],
    });
    expect(store.getScheduledInputs(session.id)[0]).toMatchObject({
      id: 'scheduled-message-idle',
      action: 'message',
      mode: 'queue',
      scheduleKind: 'when_idle',
      scheduledFor: null,
    });
  });

  it('stores scheduled plan executions with their bound plan target', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-schedule-plan-command',
      status: 'done',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    const scheduledAt = Date.parse('2026-04-09T10:18:00.000Z');
    const schedulePromise = store.schedulePlanExecution(session.id, scheduledAt, {
      planItemId: 'plan-item-1',
      pendingItemId: 'plan-choice-1',
      questionId: 'direction',
      executeOptionLabel: 'Implement plan',
    });

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'schedule_plan',
      p: {
        pid: 'plan-item-1',
        iid: 'plan-choice-1',
        qid: 'direction',
        opt: 'Implement plan',
        sk: 'at_time',
        at: scheduledAt,
      },
    });

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'schedule_plan',
      ok: 1,
      p: {
        id: 'scheduled-plan-1',
        a: 'execute_plan',
        tid: 'plan-item-1',
        m: 'send',
        st: 'scheduled',
        txt: 'Implement the plan.',
        sf: scheduledAt,
        ca: scheduledAt - 60_000,
        ua: scheduledAt - 60_000,
      },
    });

    await expect(schedulePromise).resolves.toMatchObject({
      id: 'scheduled-plan-1',
      action: 'execute_plan',
      targetId: 'plan-item-1',
      status: 'scheduled',
    });
    expect(store.getScheduledInputs(session.id)[0]).toMatchObject({
      action: 'execute_plan',
      targetId: 'plan-item-1',
      text: 'Implement the plan.',
      scheduleKind: 'at_time',
    });
    expect(store.getSessions(session.projectId)[0]?.hasScheduledPlanExecution).toBe(true);
  });

  it('stores when-idle plan schedules without a scheduled timestamp', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-schedule-plan-idle',
      status: 'done',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    const schedulePromise = store.schedulePlanExecution(
      session.id,
      { scheduleKind: 'when_idle' },
      { planItemId: 'plan-item-idle' }
    );

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    const sent = commandSocket?.sent.at(-1) as
      | { rid?: string; p?: Record<string, unknown> }
      | undefined;
    expect(sent).toMatchObject({
      p: {
        pid: 'plan-item-idle',
        sk: 'when_idle',
      },
    });
    expect(sent?.p).not.toHaveProperty('at');

    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: String(sent?.rid ?? ''),
      sid: session.id,
      ts: Date.now(),
      op: 'schedule_plan',
      ok: 1,
      p: {
        id: 'scheduled-plan-idle',
        a: 'execute_plan',
        tid: 'plan-item-idle',
        m: 'send',
        sk: 'when_idle',
        st: 'scheduled',
        txt: 'Implement the plan.',
        br: ['git_dirty'],
        ce: '',
        ca: Date.parse('2026-04-09T10:00:00.000Z'),
        ua: Date.parse('2026-04-09T10:00:00.000Z'),
      },
    });

    await expect(schedulePromise).resolves.toMatchObject({
      id: 'scheduled-plan-idle',
      scheduleKind: 'when_idle',
      scheduledFor: null,
      blockingReasons: ['git_dirty'],
    });
    expect(store.getSessions(session.projectId)[0]?.hasScheduledPlanExecution).toBe(true);
  });

  it('updates and immediately dispatches scheduled inputs through commands', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-scheduled-manage',
      status: 'idle',
      assistantState: null,
    });
    const originalAt = Date.parse('2026-04-09T10:20:00.000Z');
    const updatedAt = Date.parse('2026-04-09T10:40:00.000Z');
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: { items: [], hasMore: false, total: 0 },
      pendingInputs: [],
      scheduledInputs: [
        {
          id: 'scheduled-manage-1',
          action: 'message',
          mode: 'send',
          status: 'failed',
          lastError: 'temporary failure',
          text: 'Original text',
          attachmentIds: ['attachment-1'],
          scheduledFor: originalAt,
          createdAt: originalAt - 60_000,
          updatedAt: originalAt,
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    const updatePromise = store.updateScheduledInput(session.id, 'scheduled-manage-1', {
      scheduledFor: updatedAt,
      text: 'Updated text',
      mode: 'queue',
    });
    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }
    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'scheduled_update',
      p: {
        id: 'scheduled-manage-1',
        at: updatedAt,
        txt: 'Updated text',
        mode: 'queue',
      },
    });
    const updateRequestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: updateRequestId,
      sid: session.id,
      ts: Date.now(),
      op: 'scheduled_update',
      ok: 1,
      p: {
        id: 'scheduled-manage-1',
        a: 'message',
        m: 'queue',
        st: 'scheduled',
        txt: 'Updated text',
        atts: ['attachment-1'],
        sf: updatedAt,
        ca: originalAt - 60_000,
        ua: updatedAt - 60_000,
      },
    });

    await expect(updatePromise).resolves.toMatchObject({
      id: 'scheduled-manage-1',
      status: 'scheduled',
      lastError: '',
      text: 'Updated text',
      mode: 'queue',
      scheduledFor: updatedAt,
    });

    const dispatchPromise = store.dispatchScheduledInputNow(session.id, 'scheduled-manage-1');
    for (
      let attempt = 0;
      attempt < 5 &&
      (commandSocket?.sent.at(-1) as { op?: string } | undefined)?.op !== 'scheduled_now';
      attempt += 1
    ) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
    }
    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'scheduled_now',
      p: { id: 'scheduled-manage-1' },
    });
    const dispatchRequestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: dispatchRequestId,
      sid: session.id,
      ts: Date.now(),
      op: 'scheduled_now',
      ok: 1,
      p: { id: 'scheduled-manage-1' },
    });

    await dispatchPromise;
    expect(store.getScheduledInputs(session.id)).toEqual([]);
  });

  it('updates and removes scheduled inputs through scheduled events and commands', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-scheduled-events',
      status: 'idle',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.openEventStream();

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'scheduled',
      si: [
        {
          id: 'scheduled-evt-active',
          a: 'execute_plan',
          tid: 'plan-item-active',
          m: 'send',
          st: 'scheduled',
          txt: 'Implement the plan.',
          atts: [],
          sk: 'when_idle',
          ca: Date.parse('2026-04-09T10:00:00.000Z'),
          ua: Date.parse('2026-04-09T10:00:00.000Z'),
        },
      ],
    });
    expect(store.getSessions(session.projectId)[0]?.hasScheduledPlanExecution).toBe(true);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'scheduled',
      si: [
        {
          id: 'scheduled-evt-1',
          a: 'execute_plan',
          tid: 'plan-item-expired',
          m: 'send',
          st: 'expired',
          err: 'scheduled plan is no longer available',
          txt: 'Implement the plan.',
          atts: [],
          sf: Date.parse('2026-04-09T10:09:00.000Z'),
          ca: Date.parse('2026-04-09T10:01:00.000Z'),
          ua: Date.parse('2026-04-09T10:02:00.000Z'),
        },
      ],
    });

    expect(store.getScheduledInputs(session.id)).toEqual([
      {
        id: 'scheduled-evt-1',
        dependsOnId: '',
        dependencyStatus: 'none',
        action: 'execute_plan',
        targetId: 'plan-item-expired',
        mode: 'send',
        exitPlanMode: false,
        status: 'expired',
        lastError: 'scheduled plan is no longer available',
        text: 'Implement the plan.',
        attachmentIds: [],
        scheduleKind: 'at_time',
        scheduledFor: Date.parse('2026-04-09T10:09:00.000Z'),
        idleSince: null,
        blockingReasons: [],
        conditionError: '',
        createdAt: Date.parse('2026-04-09T10:01:00.000Z'),
        updatedAt: Date.parse('2026-04-09T10:02:00.000Z'),
        sentAt: null,
        canceledAt: null,
      },
    ]);
    expect(store.getSessions(session.projectId)[0]?.hasScheduledPlanExecution).toBe(false);

    const removePromise = store.removeScheduledInput(session.id, 'scheduled-evt-1');
    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    expect(commandSocket?.sent.at(-1)).toMatchObject({
      k: 'cmd',
      sid: session.id,
      op: 'scheduled_del',
      p: {
        id: 'scheduled-evt-1',
      },
    });

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'scheduled_del',
      ok: 1,
      p: {
        id: 'scheduled-evt-1',
      },
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'scheduled',
      si: [],
    });

    await removePromise;
    expect(store.getScheduledInputs(session.id)).toEqual([]);
    expect(store.getSessions(session.projectId)[0]?.hasScheduledPlanExecution).toBe(false);
  });

  it('hydrates first sends from incremental events without HTTP snapshots', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-command-hydration',
      status: 'idle',
      assistantState: null,
      itemCount: 0,
      turnCount: 0,
      updatedAt: '2026-04-09T10:00:00.000Z',
      lastMessageAt: null,
    });
    const runningSession = makeSession({
      ...session,
      status: 'running',
      assistantState: 'working',
      itemCount: 1,
      turnCount: 1,
      updatedAt: '2026-04-09T10:00:02.000Z',
      lastMessageAt: '2026-04-09T10:00:02.000Z',
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    const sendPromise = store.sendMessage(session.id, 'hello', []);

    expect(store.getBlocks(session.id)).toEqual([]);
    expect(store.getTimelineBlocks(session.id)).toMatchObject([
      {
        kind: 'user',
        text: 'hello',
        deliveryState: 'sending',
      },
    ]);

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    expect(commandSocket).not.toBeNull();

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'send',
      ok: 1,
    });
    commandSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      s: toWireSession(runningSession),
      i: {
        id: 'history-live-send',
        oi: 1,
        kd: 'user',
        tp: 'user_message',
        txt: 'hello',
        ts2: Date.parse('2026-04-09T10:00:01.000Z'),
      },
    });

    const sendResult = await sendPromise;

    expect(snapshotMock).not.toHaveBeenCalled();
    expect(store.getBlocks(session.id)).toHaveLength(1);
    expect(store.getBlocks(session.id)[0]?.text).toBe('hello');
    expect(store.getTimelineBlocks(session.id)).toHaveLength(1);
    expect(store.getTimelineBlocks(session.id)[0]?.deliveryState).toBeUndefined();
    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'starting',
      running: true,
    });
    expect(sendResult).toMatchObject({ accepted: true, runtimeObserved: true });
  });

  it('reports an accepted send when no authoritative runtime activity arrives in time', async () => {
    vi.useFakeTimers();
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-delayed-runtime',
      status: 'done',
      assistantState: null,
      itemCount: 0,
      turnCount: 0,
    });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({ revision: '2', unchanged: true });
    await store.loadSessions(session.projectId);

    const sendPromise = store.sendMessage(session.id, 'start slowly', []);
    await flushMicrotasks();
    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    const command = commandSocket?.sent.at(-1) as { rid?: string } | undefined;
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: String(command?.rid ?? ''),
      sid: session.id,
      ts: Date.now(),
      op: 'send',
      ok: 1,
    });

    await vi.advanceTimersByTimeAsync(3_000);
    await expect(sendPromise).resolves.toMatchObject({
      accepted: true,
      runtimeObserved: false,
      revision: '2',
    });
  });

  it('sends a fresh-context handoff through the current web session', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-fresh-context',
      status: 'done',
      assistantState: 'waiting_plan_approval',
      workflowMode: 'plan',
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.openEventStream();

    const sendPromise = store.sendMessage(
      session.id,
      'Implement the existing plan in a fresh context.',
      [],
      undefined,
      { freshContext: true }
    );
    const commandSocket = await waitForCommandCount(1);
    const command = commandSocket?.sent.at(-1) as
      | { rid?: string; op?: string; sid?: string; p?: Record<string, unknown> }
      | undefined;

    expect(command).toMatchObject({
      op: 'fresh_send',
      sid: session.id,
      p: {
        txt: 'Implement the existing plan in a fresh context.',
        atts: [],
      },
    });
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: String(command?.rid ?? ''),
      sid: session.id,
      ts: Date.now(),
      op: 'fresh_send',
      ok: 1,
    });
    findSocket('/api/v1/web-sessions/events')?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...session,
        status: 'running',
        assistantState: 'working',
        updatedAt: '2026-04-09T10:00:01.000Z',
      }),
    });

    await sendPromise;
    expect(snapshotMock).not.toHaveBeenCalled();
    expect(store.getSessions(session.projectId)).toHaveLength(1);
    expect(store.getTimelineBlocks(session.id)).toMatchObject([
      {
        kind: 'user',
        text: 'Implement the existing plan in a fresh context.',
        deliveryState: 'accepted',
      },
    ]);
  });

  it('keeps a pre-ACK failure retryable and reconciles the same bubble after retry', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-message-delivery-retry',
      status: 'idle',
      assistantState: null,
      itemCount: 0,
      turnCount: 0,
      lastMessageAt: null,
    });
    const runningSession = makeSession({
      ...session,
      status: 'idle',
      assistantState: 'error',
      itemCount: 2,
      turnCount: 1,
      updatedAt: '2026-04-09T10:00:03.000Z',
      lastMessageAt: '2026-04-09T10:00:03.000Z',
    });
    const attachments = [
      {
        id: 'attachment-1',
        name: 'evidence.png',
        mime: 'image/png',
        size: 128,
        path: '/tmp/evidence.png',
      },
    ];

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');

    const firstSend = store.sendMessage(session.id, 'inspect this', ['attachment-1'], undefined, {
      attachments,
    });
    const firstLocalMessage = store.getTimelineBlocks(session.id)[0]!;
    expect(firstLocalMessage).toMatchObject({
      text: 'inspect this',
      deliveryState: 'sending',
      attachments: [{ id: 'attachment-1', name: 'evidence.png' }],
    });

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await flushMicrotasks();
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }
    const firstRequestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'err',
      rid: firstRequestId,
      sid: session.id,
      op: 'send',
      ts: Date.now(),
      code: 'unavailable',
      msg: 'APP server did not accept the message',
    });

    const deliveryError = await firstSend.catch(error => error);
    expect(isWebSessionMessageDeliveryError(deliveryError)).toBe(true);
    expect(store.getBlocks(session.id)).toEqual([]);
    expect(store.getTimelineBlocks(session.id)).toMatchObject([
      {
        id: firstLocalMessage.id,
        deliveryState: 'failed',
      },
    ]);

    const retrySend = store.sendMessage(
      session.id,
      firstLocalMessage.text,
      firstLocalMessage.attachments.map(attachment => attachment.id),
      undefined,
      {
        outgoingMessageId: firstLocalMessage.id,
        attachments: firstLocalMessage.attachments,
      }
    );
    expect(store.getTimelineBlocks(session.id)).toMatchObject([
      {
        id: firstLocalMessage.id,
        deliveryState: 'sending',
      },
    ]);

    for (let attempt = 0; attempt < 5 && (commandSocket?.sent.length ?? 0) < 2; attempt += 1) {
      await flushMicrotasks();
    }
    const retryRequestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: retryRequestId,
      sid: session.id,
      ts: Date.now(),
      op: 'send',
      ok: 1,
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      s: toWireSession(runningSession),
      i: {
        id: 'history-retried-user-message',
        oi: 1,
        kd: 'user',
        tp: 'user_message',
        txt: 'inspect this',
        atts: attachments,
        ts2: Date.parse('2026-04-09T10:00:01.000Z'),
      },
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: {
        id: 'history-agent-run-failed',
        oi: 2,
        kd: 'system',
        tp: 'run_fail',
        out: 'failed',
        txt: '429 Too Many Requests',
        ts2: Date.parse('2026-04-09T10:00:02.000Z'),
      },
    });

    await retrySend;

    const timeline = store.getTimelineBlocks(session.id);
    const userMessages = timeline.filter(block => block.kind === 'user');
    expect(userMessages.map(block => block.id)).toEqual(['history-retried-user-message']);
    expect(userMessages[0]?.deliveryState).toBeUndefined();
    expect(timeline.some(block => block.id === firstLocalMessage.id)).toBe(false);
    expect(timeline.some(block => block.itemType === 'run_fail')).toBe(true);
  });

  it('trims inactive session history to a recent retained window', async () => {
    const store = useWebSessionStore();
    const active = makeSession({ id: 'session-active' });
    const inactive = makeSession({ id: 'session-inactive', orderIndex: 900, title: 'Inactive' });

    listMock.mockResolvedValue([active, inactive]);
    snapshotMock.mockImplementation(async (_projectId: string, sessionId: string) => ({
      session: sessionId === active.id ? active : inactive,
      history: {
        items: Array.from({ length: 420 }, (_, index) => ({
          id: `${sessionId}-${index + 1}`,
          oi: index + 1,
          kd: 'assistant',
          tp: 'message',
          txt: `message ${index + 1}`,
          ts2: Date.parse('2026-04-09T10:00:00.000Z') + index,
        })),
        hasMore: false,
        beforeCursor: '',
        total: 420,
      },
      pendingInputs: [],
      scheduledInputs: [],
    }));

    await store.loadSessions(active.projectId);
    await store.loadSessionSnapshot(active.projectId, active.id);
    await store.loadSessionSnapshot(inactive.projectId, inactive.id);

    expect(store.getBlocks(inactive.id)).toHaveLength(420);
    store.getLiveState(inactive.id);
    webSessionRuntimePerformance.reset();

    store.trimInactiveSessionEvents(active.id);

    const trimmed = store.getBlocks(inactive.id);
    expect(trimmed).toHaveLength(160);
    expect(trimmed[0]?.orderIndex).toBe(261);
    expect(store.getHistoryMeta(inactive.id)).toMatchObject({
      hasMore: true,
      beforeCursor: '261',
    });
    store.getPendingApproval(inactive.id);
    expect(webSessionRuntimePerformance.snapshot()).toMatchObject({
      fullDerivations: 1,
      scannedBlocks: 160,
    });

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: inactive.id,
      ts: Date.now(),
      op: 'hist_item',
      i: {
        id: `${inactive.id}-420`,
        oi: 420,
        kd: 'assistant',
        tp: 'message',
        txt: 'updated after trim',
        ts2: Date.parse('2026-04-09T10:00:00.000Z') + 419,
      },
    });
    expect(store.getBlocks(inactive.id)).toHaveLength(160);
    expect(store.getBlocks(inactive.id).at(-1)?.text).toBe('updated after trim');
    expect(webSessionRuntimePerformance.snapshot().eventSorts).toBe(0);
  });

  it('falls back to HTTP snapshots when a send only receives an ack', async () => {
    vi.useFakeTimers();
    window.setTimeout = setTimeout;
    window.clearTimeout = clearTimeout;
    window.setInterval = setInterval;
    window.clearInterval = clearInterval;

    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-ack-only',
      status: 'idle',
      assistantState: null,
      itemCount: 0,
      turnCount: 0,
      updatedAt: '2026-04-09T10:00:00.000Z',
      lastMessageAt: null,
    });
    const hydratedSession = makeSession({
      ...session,
      status: 'running',
      assistantState: 'working',
      itemCount: 1,
      turnCount: 1,
      updatedAt: '2026-04-09T10:00:03.000Z',
      lastMessageAt: '2026-04-09T10:00:03.000Z',
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      revision: '2',
      session: hydratedSession,
      history: {
        items: [
          {
            id: 'history-snapshot-send',
            oi: 1,
            kd: 'user',
            tp: 'user_message',
            txt: 'hello from snapshot',
            ts2: Date.parse('2026-04-09T10:00:02.000Z'),
          },
        ],
        hasMore: false,
        total: 1,
      },
      pendingInputs: [],
    });

    await store.loadSessions(session.projectId);

    const sendPromise = store.sendMessage(session.id, 'hello from snapshot', []);

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    expect(commandSocket).not.toBeNull();

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'send',
      ok: 1,
    });

    await vi.advanceTimersByTimeAsync(500);
    await sendPromise;

    expect(snapshotMock).toHaveBeenCalledWith(
      session.projectId,
      session.id,
      expect.objectContaining({
        limit: 80,
        signal: expect.any(AbortSignal),
      })
    );
    expect(store.getBlocks(session.id)).toHaveLength(1);
    expect(store.getBlocks(session.id)[0]?.text).toBe('hello from snapshot');
  });

  it('keeps native compaction pending until a revisioned event observes the run start', async () => {
    const store = useWebSessionStore();
    const idleSession = makeSession({
      id: 'session-compact-hydration',
      agent: 'pi',
      sourceKind: 'pi_rpc',
      status: 'idle',
      assistantState: null,
      updatedAt: '2026-04-09T10:00:00.000Z',
    });
    const runningSession = makeSession({
      ...idleSession,
      status: 'running',
      assistantState: 'working',
      updatedAt: '2026-04-09T10:00:03.000Z',
    });

    listMock.mockResolvedValue([idleSession]);
    await store.loadSessions(idleSession.projectId);

    const compactPromise = store.compactSession(idleSession.id);
    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      await new Promise(resolve => setTimeout(resolve, 0));
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    expect(commandSocket?.sent.at(-1)).toMatchObject({
      sid: idleSession.id,
      op: 'compact',
      p: {},
    });
    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: idleSession.id,
      ts: Date.now(),
      op: 'compact',
      ok: 1,
    });
    commandSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: idleSession.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(runningSession),
    });

    await compactPromise;
    expect(snapshotMock).not.toHaveBeenCalled();
    expect(store.getLiveState(idleSession.id)).toMatchObject({
      running: true,
      phase: 'starting',
    });
  });

  it('keeps abort pending until a revisioned event observes the session stop', async () => {
    vi.useFakeTimers();
    window.setTimeout = setTimeout;
    window.clearTimeout = clearTimeout;
    window.setInterval = setInterval;
    window.clearInterval = clearInterval;

    const store = useWebSessionStore();
    const runningSession = makeSession({
      id: 'session-abort-hydration',
      status: 'running',
      assistantState: 'working',
      itemCount: 1,
      turnCount: 1,
      updatedAt: '2026-04-09T10:00:00.000Z',
    });
    const stoppedSession = makeSession({
      ...runningSession,
      status: 'idle',
      assistantState: null,
      updatedAt: '2026-04-09T10:00:03.000Z',
    });

    listMock.mockResolvedValue([runningSession]);
    snapshotMock.mockResolvedValue({
      session: runningSession,
      history: {
        items: [],
        hasMore: false,
        total: 0,
      },
      pendingInputs: [],
    });

    await store.loadSessions(runningSession.projectId);

    const abortPromise = store.abortSession(runningSession.id);

    let commandSocket = findSocket('/api/v1/web-sessions/ws');
    for (let attempt = 0; attempt < 5 && !commandSocket?.sent.length; attempt += 1) {
      await Promise.resolve();
      commandSocket = findSocket('/api/v1/web-sessions/ws');
    }

    expect(commandSocket).not.toBeNull();

    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: runningSession.id,
      ts: Date.now(),
      op: 'abort',
      ok: 1,
    });

    commandSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(stoppedSession),
    });

    await vi.advanceTimersByTimeAsync(200);
    await abortPromise;

    expect(snapshotMock).not.toHaveBeenCalled();
    expect(store.getLiveState(runningSession.id)).toMatchObject({
      running: false,
      phase: 'idle',
    });
  });

  it('keeps completion notifications driven by the dedicated event stream websocket', async () => {
    const store = useWebSessionStore();
    const runningSession = makeSession({
      id: 'session-running',
      status: 'running',
      assistantState: null,
    });
    const doneSession = makeSession({
      ...runningSession,
      status: 'done',
      assistantState: null,
      updatedAt: '2026-04-09T10:05:00.000Z',
      lastMessageAt: '2026-04-09T10:05:00.000Z',
    });
    const handleCompleted = vi.fn();

    listMock.mockResolvedValue([runningSession]);
    await store.loadSessions(runningSession.projectId);
    store.emitter.on('ai:completed', handleCompleted);

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(doneSession),
    });

    expect(handleCompleted).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: runningSession.id,
        projectId: runningSession.projectId,
      })
    );

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(runningSession),
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(doneSession),
    });

    expect(handleCompleted).toHaveBeenCalledTimes(1);
    store.emitter.off('ai:completed', handleCompleted);
  });

  it('derives active sub-agent count and summaries from running tool blocks', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-sub-agents',
      status: 'running',
      assistantState: 'working',
      itemCount: 3,
      turnCount: 1,
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          {
            id: 'history-user',
            oi: 1,
            kd: 'user',
            tp: 'user_message',
            txt: 'delegate work',
            ts2: Date.parse('2026-04-09T10:00:00.000Z'),
          },
          {
            id: 'history-sub-agent-1',
            oi: 2,
            kd: 'tool',
            tp: 'sub_agent_tool_call',
            ts2: Date.parse('2026-04-09T10:00:01.000Z'),
            tl: {
              id: 'sub-agent-1',
              name: 'Sub Agent',
              kind: 'collabAgentToolCall',
              st: 'running',
              in: {
                title: 'Research agent',
                prompt: 'Inspect current sub-agent support',
              },
              meta: {
                kind: 'sub_agent_tool_call',
                title: 'Research agent',
                subtitle: 'Inspect current sub-agent support',
              },
            },
          },
          {
            id: 'history-sub-agent-2',
            oi: 3,
            kd: 'tool',
            tp: 'sub_agent_tool_call',
            ts2: Date.parse('2026-04-09T10:00:02.000Z'),
            tl: {
              id: 'sub-agent-2',
              name: 'Sub Agent',
              kind: 'sub_agent_tool_call',
              st: 'running',
              in: {
                title: 'Patch agent',
                task: 'Update timeout filtering',
              },
              meta: {
                kind: 'sub_agent_tool_call',
                title: 'Patch agent',
                subtitle: 'Update timeout filtering',
              },
            },
          },
        ],
        hasMore: false,
        total: 3,
      },
      pendingInputs: [],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getLiveState(session.id)).toMatchObject({
      running: true,
      activeSubAgentCount: 2,
      activeSubAgents: [
        {
          id: 'sub-agent-1',
          title: 'Research agent',
          summary: 'Inspect current sub-agent support',
        },
        {
          id: 'sub-agent-2',
          title: 'Patch agent',
          summary: 'Update timeout filtering',
        },
      ],
    });
  });

  it('expands one running wait block into multiple active sub agents using receiverThreadIds', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-sub-agent-wait',
      status: 'running',
      assistantState: 'working',
      itemCount: 7,
      turnCount: 1,
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          {
            id: 'history-user',
            oi: 1,
            kd: 'user',
            tp: 'user_message',
            txt: 'start three agents',
            ts2: Date.parse('2026-04-09T10:00:00.000Z'),
          },
          {
            id: 'spawn-1',
            oi: 2,
            kd: 'tool',
            tp: 'sub_agent_tool_call',
            ts2: Date.parse('2026-04-09T10:00:01.000Z'),
            tl: {
              id: 'call-1',
              name: 'Sub Agent',
              kind: 'collabAgentToolCall',
              st: 'done',
              in: {
                prompt: 'Wait at least 35 seconds',
                receiverThreadIds: ['agent-1'],
              },
              out: JSON.stringify({
                receiverThreadIds: ['agent-1'],
                agentsStates: {
                  'agent-1': { message: null, status: 'pendingInit' },
                },
              }),
              meta: {
                kind: 'sub_agent_tool_call',
                title: 'Sub Agent',
                subtitle: 'Wait at least 35 seconds',
              },
            },
          },
          {
            id: 'spawn-2',
            oi: 3,
            kd: 'tool',
            tp: 'sub_agent_tool_call',
            ts2: Date.parse('2026-04-09T10:00:02.000Z'),
            tl: {
              id: 'call-2',
              name: 'Sub Agent',
              kind: 'collabAgentToolCall',
              st: 'done',
              in: {
                prompt: 'Wait at least 36 seconds',
                receiverThreadIds: ['agent-2'],
              },
              out: JSON.stringify({
                receiverThreadIds: ['agent-2'],
                agentsStates: {
                  'agent-2': { message: null, status: 'pendingInit' },
                },
              }),
              meta: {
                kind: 'sub_agent_tool_call',
                title: 'Sub Agent',
                subtitle: 'Wait at least 36 seconds',
              },
            },
          },
          {
            id: 'spawn-3',
            oi: 4,
            kd: 'tool',
            tp: 'sub_agent_tool_call',
            ts2: Date.parse('2026-04-09T10:00:03.000Z'),
            tl: {
              id: 'call-3',
              name: 'Sub Agent',
              kind: 'collabAgentToolCall',
              st: 'done',
              in: {
                prompt: 'Wait at least 37 seconds',
                receiverThreadIds: ['agent-3'],
              },
              out: JSON.stringify({
                receiverThreadIds: ['agent-3'],
                agentsStates: {
                  'agent-3': { message: null, status: 'pendingInit' },
                },
              }),
              meta: {
                kind: 'sub_agent_tool_call',
                title: 'Sub Agent',
                subtitle: 'Wait at least 37 seconds',
              },
            },
          },
          {
            id: 'assistant-names',
            oi: 4.5,
            kd: 'assistant',
            tp: 'agent_message',
            txt: '已启动 Locke、Singer、Halley。现在对 3 个 agent 分别等待，最长 90 秒。',
            ts2: Date.parse('2026-04-09T10:00:03.500Z'),
            dn: true,
          },
          {
            id: 'wait-running',
            oi: 5,
            kd: 'tool',
            tp: 'sub_agent_tool_call',
            ts2: Date.parse('2026-04-09T10:00:04.000Z'),
            tl: {
              id: 'wait-call',
              name: 'Sub Agent',
              kind: 'collabAgentToolCall',
              st: 'running',
              in: {
                receiverThreadIds: ['agent-1', 'agent-2', 'agent-3'],
                agentsStates: {},
              },
              meta: {
                kind: 'sub_agent_tool_call',
                title: 'Sub Agent',
                subtitle: '',
              },
            },
          },
          {
            id: 'wait-timeout',
            oi: 6,
            kd: 'tool',
            tp: 'sub_agent_tool_call',
            ts2: Date.parse('2026-04-09T10:00:05.000Z'),
            tl: {
              id: 'wait-timeout-call',
              name: 'Sub Agent',
              kind: 'collabAgentToolCall',
              st: 'done',
              in: {
                receiverThreadIds: ['agent-1', 'agent-2', 'agent-3'],
                agentsStates: {},
              },
              out: JSON.stringify({
                receiverThreadIds: [],
                agentsStates: {},
                timedOut: true,
              }),
              meta: {
                kind: 'sub_agent_tool_call',
                title: 'Sub Agent',
                subtitle: '',
              },
            },
          },
        ],
        hasMore: false,
        total: 5,
      },
      pendingInputs: [],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getLiveState(session.id)).toMatchObject({
      running: true,
      activeSubAgentCount: 3,
      activeSubAgents: [
        {
          id: 'agent-1',
          title: 'Locke',
          summary: 'Wait at least 35 seconds',
        },
        {
          id: 'agent-2',
          title: 'Singer',
          summary: 'Wait at least 36 seconds',
        },
        {
          id: 'agent-3',
          title: 'Halley',
          summary: 'Wait at least 37 seconds',
        },
      ],
    });
  });

  it('uses the authoritative sub-agent registry and ignores child tools for root live state', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-authoritative-sub-agents',
      nativeSessionId: 'thread-root',
      status: 'running',
      assistantState: 'working',
      itemCount: 2,
      turnCount: 1,
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          {
            id: 'child-command',
            sourceThreadId: 'thread-child-running',
            sourceTurnId: 'turn-child',
            orderIndex: 1,
            kind: 'tool',
            itemType: 'command_execution',
            text: '',
            timestamp: '2026-04-09T10:00:01.000Z',
            attachments: [],
            tool: {
              id: 'child-command',
              name: 'CommandExecution',
              kind: 'command_execution',
              status: 'running',
              input: { command: 'sleep 30' },
            },
          },
        ],
        hasMore: false,
        total: 1,
      },
      pendingInputs: [],
      subAgents: [
        {
          threadId: 'thread-root',
          nickname: 'Root',
          status: 'running',
          currentTurnId: 'turn-root',
          summary: 'Main session must not be counted as a sub-agent',
        },
        {
          threadId: 'thread-child-running',
          nickname: 'Atlas',
          role: 'worker',
          status: 'running',
          currentTurnId: 'turn-child',
          summary: 'Inspecting the repository',
        },
        {
          threadId: 'thread-child-idle',
          nickname: 'Kepler',
          role: 'worker',
          status: 'running',
          summary: 'Finished an earlier turn and remains reusable',
        },
        {
          threadId: 'thread-child-done',
          nickname: 'Nova',
          role: 'reviewer',
          status: 'completed',
          summary: 'Review complete',
        },
      ],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getSubAgents(session.id)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'thread-child-running',
          title: 'Atlas [worker]',
          status: 'running',
        }),
        expect.objectContaining({
          id: 'thread-child-done',
          title: 'Nova [reviewer]',
          status: 'completed',
        }),
        expect.objectContaining({
          id: 'thread-child-idle',
          title: 'Kepler [worker]',
          status: 'idle',
        }),
      ])
    );
    expect(store.getSubAgents(session.id)).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ id: 'thread-root' })])
    );
    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'starting',
      activeSubAgentCount: 1,
      activeSubAgents: [{ id: 'thread-child-running', title: 'Atlas [worker]' }],
    });
    expect(store.getLiveState(session.id).tool).toBeUndefined();
    expect(store.getBlocks(session.id)[0]?.sourceThreadId).toBe('thread-child-running');
  });

  it('suppresses completion notifications while pending inputs remain queued', async () => {
    const store = useWebSessionStore();
    const runningSession = makeSession({
      id: 'session-running-pending',
      status: 'running',
      assistantState: null,
    });
    const doneSession = makeSession({
      ...runningSession,
      status: 'done',
      assistantState: null,
      updatedAt: '2026-04-09T10:05:00.000Z',
      lastMessageAt: '2026-04-09T10:05:00.000Z',
    });
    const handleCompleted = vi.fn();

    listMock.mockResolvedValue([runningSession]);
    await store.loadSessions(runningSession.projectId);
    store.emitter.on('ai:completed', handleCompleted);

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'pending',
      pi: [
        {
          id: 'pending-1',
          m: 'queue',
          txt: 'queued follow-up',
          atts: [],
          ca: Date.parse('2026-04-09T10:04:59.000Z'),
        },
      ],
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(doneSession),
    });

    expect(handleCompleted).not.toHaveBeenCalled();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'pending',
      pi: [],
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(runningSession),
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: runningSession.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...doneSession,
        updatedAt: '2026-04-09T10:06:00.000Z',
        lastMessageAt: '2026-04-09T10:06:00.000Z',
      }),
    });

    expect(handleCompleted).toHaveBeenCalledTimes(1);
    store.emitter.off('ai:completed', handleCompleted);
  });

  it('keeps realtime user message attachments on incoming history items', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-live-attachments',
      status: 'running',
      assistantState: null,
      itemCount: 0,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.openEventStream();

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: {
        id: 'history-live-1',
        oi: 1,
        kd: 'user',
        tp: 'user_message',
        txt: 'hello [Image #1]',
        ts2: Date.parse('2026-04-09T10:01:00.000Z'),
        atts: [
          {
            id: 'att-live-1',
            name: 'image.png',
            mime: 'image/png',
            sz: 42,
          },
        ],
      },
    });

    const blocks = store.getBlocks(session.id);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.attachments).toEqual([
      expect.objectContaining({
        id: 'att-live-1',
        name: 'image.png',
        mime: 'image/png',
        size: 42,
      }),
    ]);
  });

  it('clears retrying state when a newer assistant update arrives on the same history item', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-retry-assistant-recovered',
      status: 'running',
      assistantState: 'working',
      itemCount: 2,
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          {
            id: 'assistant-1',
            oi: 1,
            kd: 'assistant',
            tp: 'agent_message',
            txt: 'Draft reply',
            ts2: Date.parse('2026-04-09T10:00:00.000Z'),
            obs: Date.parse('2026-04-09T10:00:00.000Z'),
            dn: false,
          },
          {
            id: 'retry-1',
            oi: 2,
            kd: 'system',
            tp: 'note',
            txt: 'Reconnecting... 1/5',
            ts2: Date.parse('2026-04-09T10:00:10.000Z'),
            obs: Date.parse('2026-04-09T10:00:10.000Z'),
            lvl: 'warn',
            pl: {
              code: 'transport_retrying',
              txt: 'Reconnecting... 1/5',
              remoteUrl: 'https://proxy.example.test/v1',
              attempt: 1,
              maxAttempts: 5,
            },
          },
        ],
        hasMore: false,
        total: 2,
      },
      pendingInputs: [],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'retrying',
      running: true,
      retry: expect.objectContaining({
        remoteUrl: 'https://proxy.example.test/v1',
        attempt: 1,
        maxAttempts: 5,
      }),
    });

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: {
        id: 'assistant-1',
        oi: 1,
        kd: 'assistant',
        tp: 'agent_message',
        txt: 'Draft reply recovered',
        ts2: Date.parse('2026-04-09T10:00:00.000Z'),
        obs: Date.parse('2026-04-09T10:00:20.000Z'),
        dn: false,
      },
    });

    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'thinking',
      running: true,
    });
    expect(store.getLiveState(session.id).retry).toBeUndefined();
  });

  it('clears retrying state when a newer tool update arrives on the same history item', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-retry-tool-recovered',
      status: 'running',
      assistantState: 'working',
      itemCount: 2,
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          {
            id: 'tool-1',
            oi: 1,
            kd: 'tool',
            tp: 'command_execution',
            ts2: Date.parse('2026-04-09T10:00:00.000Z'),
            obs: Date.parse('2026-04-09T10:00:00.000Z'),
            tl: {
              id: 'tool-1',
              name: 'Shell',
              kind: 'command_execution',
              st: 'running',
              out: '',
            },
          },
          {
            id: 'retry-2',
            oi: 2,
            kd: 'system',
            tp: 'note',
            txt: 'Reconnecting... 1/5',
            ts2: Date.parse('2026-04-09T10:00:10.000Z'),
            obs: Date.parse('2026-04-09T10:00:10.000Z'),
            lvl: 'warn',
            pl: {
              code: 'transport_retrying',
              txt: 'Reconnecting... 1/5',
              attempt: 1,
              maxAttempts: 5,
            },
          },
        ],
        hasMore: false,
        total: 2,
      },
      pendingInputs: [],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'retrying',
      running: true,
    });

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: {
        id: 'tool-1',
        oi: 1,
        kd: 'tool',
        tp: 'command_execution',
        ts2: Date.parse('2026-04-09T10:00:00.000Z'),
        obs: Date.parse('2026-04-09T10:00:21.000Z'),
        tl: {
          id: 'tool-1',
          name: 'Shell',
          kind: 'command_execution',
          st: 'running',
          out: 'Recovered output',
        },
      },
    });

    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'tool',
      running: true,
      tool: expect.objectContaining({
        id: 'tool-1',
      }),
    });
    expect(store.getLiveState(session.id).retry).toBeUndefined();
  });

  it('keeps retrying state when no newer runtime progress has arrived', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-retry-still-active',
      status: 'running',
      assistantState: 'working',
      itemCount: 2,
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          {
            id: 'assistant-2',
            oi: 1,
            kd: 'assistant',
            tp: 'agent_message',
            txt: 'Still running',
            ts2: Date.parse('2026-04-09T10:00:00.000Z'),
            obs: Date.parse('2026-04-09T10:00:00.000Z'),
            dn: false,
          },
          {
            id: 'retry-3',
            oi: 2,
            kd: 'system',
            tp: 'note',
            txt: 'Reconnecting... 2/5',
            ts2: Date.parse('2026-04-09T10:00:10.000Z'),
            obs: Date.parse('2026-04-09T10:00:10.000Z'),
            lvl: 'warn',
            pl: {
              code: 'transport_retrying',
              txt: 'Reconnecting... 2/5',
              attempt: 2,
              maxAttempts: 5,
            },
          },
        ],
        hasMore: false,
        total: 2,
      },
      pendingInputs: [],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'retrying',
      running: true,
      retry: expect.objectContaining({
        attempt: 2,
        maxAttempts: 5,
      }),
    });
  });

  it('clears retrying state when a newer working session summary follows hidden reasoning', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-retry-reasoning-recovered',
      status: 'running',
      assistantState: 'working',
      itemCount: 2,
    });

    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          {
            id: 'assistant-reasoning-recovery',
            oi: 1,
            kd: 'assistant',
            tp: 'agent_message',
            txt: 'Working before reconnect',
            ts2: Date.parse('2026-04-09T10:00:00.000Z'),
            obs: Date.parse('2026-04-09T10:00:00.000Z'),
            dn: false,
          },
          {
            id: 'retry-reasoning-recovery',
            oi: 2,
            kd: 'system',
            tp: 'note',
            txt: 'Reconnecting... 1/5',
            ts2: Date.parse('2026-04-09T10:00:10.000Z'),
            obs: Date.parse('2026-04-09T10:00:10.000Z'),
            lvl: 'warn',
            pl: {
              code: 'transport_retrying',
              txt: 'Reconnecting... 1/5',
              attempt: 1,
              maxAttempts: 5,
            },
          },
        ],
        hasMore: false,
        total: 2,
      },
      pendingInputs: [],
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'retrying',
      running: true,
    });

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...session,
        assistantState: 'working',
        assistantStateUpdatedAt: '2026-04-09T10:00:20.000Z',
        updatedAt: '2026-04-09T10:00:20.000Z',
      }),
    });

    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'thinking',
      running: true,
    });
    expect(store.getLiveState(session.id).retry).toBeUndefined();
  });

  it('replies to websocket heartbeat pings on the event stream', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-heartbeat',
      status: 'running',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.openEventStream();

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    eventSocket?.dispatch({
      v: 1,
      k: 'hb',
      ts: Date.now(),
      op: 'ping',
    });

    expect(eventSocket?.sent).toEqual([
      expect.objectContaining({
        k: 'hb',
        op: 'pong',
      }),
    ]);
  });

  it('sends the focused session id over the event websocket', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-focus',
      status: 'idle',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.openEventStream();

    store.setEventSessionFocus(session.id);

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();
    expect(eventSocket?.sent.at(-1)).toMatchObject({
      k: 'hb',
      op: 'focus',
      sid: session.id,
    });
  });

  it('forces a reconnect when the event stream stops receiving heartbeats', async () => {
    vi.useFakeTimers();
    window.setTimeout = setTimeout;
    window.clearTimeout = clearTimeout;
    window.setInterval = setInterval;
    window.clearInterval = clearInterval;

    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-watchdog',
      status: 'running',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    const openPromise = store.openEventStream();
    await Promise.resolve();
    await openPromise;

    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(store.connectionState).toBe('open');

    await vi.advanceTimersByTimeAsync(40001);
    expect(store.connectionState).toBe('closed');

    await vi.advanceTimersByTimeAsync(1200);
    await flushMicrotasks();

    expect(FakeWebSocket.instances).toHaveLength(2);
    expect(store.connectionState).toBe('open');
    expect(store.eventRecoveryVersion).toBe(1);
    expect(store.eventLastDisconnectReason).toBeNull();
  });

  it('keeps retrying the event stream until a reconnect attempt succeeds', async () => {
    vi.useFakeTimers();
    window.setTimeout = setTimeout;
    window.clearTimeout = clearTimeout;
    window.setInterval = setInterval;
    window.clearInterval = clearInterval;

    let eventConnectAttempt = 0;

    class FlakyEventWebSocket {
      static OPEN = 1;
      static instances: FlakyEventWebSocket[] = [];

      url: string;
      readyState = 0;
      sent: unknown[] = [];
      onopen: ((event: unknown) => void) | null = null;
      onmessage: ((event: { data: string }) => void) | null = null;
      onerror: ((event: unknown) => void) | null = null;
      onclose: (() => void) | null = null;

      constructor(url: string) {
        this.url = url;
        FlakyEventWebSocket.instances.push(this);
        const isEventStream = url === '/api/v1/web-sessions/events';
        if (isEventStream) {
          eventConnectAttempt += 1;
        }
        const attempt = eventConnectAttempt;
        queueMicrotask(() => {
          if (!isEventStream || attempt === 1 || attempt >= 4) {
            this.readyState = FlakyEventWebSocket.OPEN;
            this.onopen?.({});
            return;
          }
          this.readyState = 3;
          this.onclose?.();
        });
      }

      send(payload: string) {
        this.sent.push(JSON.parse(payload));
      }

      dispatch(frame: unknown) {
        this.onmessage?.({
          data: JSON.stringify(frame),
        });
      }

      close() {
        this.readyState = 3;
        this.onclose?.();
      }
    }

    vi.stubGlobal('WebSocket', FlakyEventWebSocket);

    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-persistent-retry',
      status: 'running',
      assistantState: null,
    });

    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);
    await store.openEventStream();

    expect(FlakyEventWebSocket.instances).toHaveLength(1);
    expect(store.connectionState).toBe('open');

    FlakyEventWebSocket.instances[0]?.close();
    expect(store.connectionState).toBe('closed');

    await vi.advanceTimersByTimeAsync(1199);
    expect(FlakyEventWebSocket.instances).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(1);
    await flushMicrotasks();
    expect(FlakyEventWebSocket.instances).toHaveLength(2);
    expect(store.connectionState).toBe('closed');

    await vi.advanceTimersByTimeAsync(2399);
    expect(FlakyEventWebSocket.instances).toHaveLength(2);

    await vi.advanceTimersByTimeAsync(1);
    await flushMicrotasks();
    expect(FlakyEventWebSocket.instances).toHaveLength(3);
    expect(store.connectionState).toBe('closed');

    await vi.advanceTimersByTimeAsync(4799);
    expect(FlakyEventWebSocket.instances).toHaveLength(3);

    await vi.advanceTimersByTimeAsync(1);
    await flushMicrotasks();
    expect(FlakyEventWebSocket.instances).toHaveLength(4);
    expect(store.connectionState).toBe('open');
    expect(store.eventRecoveryVersion).toBe(1);
    expect(store.eventLastDisconnectReason).toBeNull();
  });

  it('keeps unrelated project collections stable during an optimistic session update', async () => {
    const store = useWebSessionStore();
    const targetSession = makeSession({
      id: 'session-target',
      projectId: 'project-1',
      workflowMode: 'default',
    });
    const unrelatedSession = makeSession({
      id: 'session-unrelated',
      projectId: 'project-2',
    });
    listMock.mockImplementation(async (projectId: string) =>
      projectId === 'project-1' ? [targetSession] : [unrelatedSession]
    );

    await store.loadSessions('project-1');
    await store.loadSessions('project-2');
    const unrelatedCollection = store.getSessions('project-2');

    const updatePromise = store.updateWorkflowMode(targetSession.id, 'plan');
    for (let attempt = 0; attempt < 5; attempt += 1) {
      await flushMicrotasks();
      if (findSocket('/api/v1/web-sessions/ws')?.sent.length) {
        break;
      }
    }

    expect(store.getSessions('project-1')[0]?.workflowMode).toBe('plan');
    expect(store.getSessions('project-2')).toBe(unrelatedCollection);

    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    const requestId = String(
      (commandSocket?.sent.at(-1) as { rid?: string } | undefined)?.rid ?? ''
    );
    commandSocket?.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: targetSession.id,
      ts: Date.now(),
      op: 'set_wm',
      ok: 1,
    });
    await updatePromise;
  });

  it('shares one runtime projection across live, approval, and user-input reads', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-runtime-cache',
      status: 'running',
      assistantState: 'working',
    });
    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    expect(eventSocket).not.toBeNull();

    webSessionRuntimePerformance.reset();
    store.getLiveState(session.id);
    store.getPendingApproval(session.id);
    store.getPendingUserInput(session.id);
    store.getLiveState(session.id);
    expect(webSessionRuntimePerformance.snapshot()).toMatchObject({
      fullDerivations: 1,
      scannedBlocks: 0,
    });

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_page',
      h: {
        its: [makeWireHistoryItem(1)],
        hm: false,
        tot: 1,
      },
    });
    store.getPendingApproval(session.id);
    store.getLiveState(session.id);
    store.getPendingUserInput(session.id);
    expect(webSessionRuntimePerformance.snapshot()).toMatchObject({
      fullDerivations: 2,
      scannedBlocks: 1,
    });

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...session,
        title: 'Replaced summary object',
        updatedAt: '2026-04-09T10:01:00.000Z',
      }),
    });
    store.getLiveState(session.id);
    store.getPendingApproval(session.id);
    store.getPendingUserInput(session.id);
    expect(webSessionRuntimePerformance.snapshot()).toMatchObject({
      fullDerivations: 3,
      scannedBlocks: 2,
    });
  });

  it('uses the event index for realtime updates and only sorts unordered inputs', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-indexed-history',
      status: 'running',
      assistantState: 'working',
      itemCount: 2,
    });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          makeWireHistoryItem(1, {
            id: 'user-1',
            kd: 'user',
            tp: 'user_message',
            txt: 'start',
          }),
          makeWireHistoryItem(2, {
            id: 'assistant-1',
            kd: 'assistant',
            tp: 'agent_message',
            txt: 'draft',
            dn: false,
          }),
        ],
        hasMore: true,
        beforeCursor: '1',
        total: 2,
      },
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    store.getLiveState(session.id);
    const previousBlocks = store.getBlocks(session.id);
    const unchangedUserBlock = previousBlocks[0];

    webSessionRuntimePerformance.reset();
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: makeWireHistoryItem(2, {
        id: 'assistant-1',
        kd: 'assistant',
        tp: 'agent_message',
        txt: 'draft completed',
        obs: Date.parse('2026-04-09T10:01:00.000Z'),
        dn: false,
      }),
    });

    const updatedBlocks = store.getBlocks(session.id);
    expect(updatedBlocks).not.toBe(previousBlocks);
    expect(updatedBlocks[0]).toBe(unchangedUserBlock);
    expect(updatedBlocks.map(block => block.id)).toEqual(['user-1', 'assistant-1']);
    expect(updatedBlocks[1]?.text).toBe('draft completed');
    expect(webSessionRuntimePerformance.snapshot()).toEqual({
      fullDerivations: 0,
      incrementalDerivations: 1,
      scannedBlocks: 1,
      eventSorts: 0,
    });

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: makeWireHistoryItem(3, {
        id: 'assistant-2',
        kd: 'assistant',
        tp: 'agent_message',
        txt: 'monotonic append',
        dn: true,
      }),
    });
    expect(store.getBlocks(session.id).map(block => block.orderIndex)).toEqual([1, 2, 3]);
    expect(webSessionRuntimePerformance.snapshot().eventSorts).toBe(0);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: makeWireHistoryItem(1.5, {
        id: 'system-out-of-order',
        txt: 'inserted out of order',
      }),
    });
    expect(store.getBlocks(session.id).map(block => block.orderIndex)).toEqual([1, 1.5, 2, 3]);
    expect(webSessionRuntimePerformance.snapshot().eventSorts).toBe(1);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: makeWireHistoryItem(3, {
        id: 'assistant-2',
        kd: 'assistant',
        tp: 'agent_message',
        txt: 'updated after index rebuild',
        dn: true,
      }),
    });
    expect(store.getBlocks(session.id).filter(block => block.id === 'assistant-2')).toHaveLength(1);
    expect(store.getBlocks(session.id).at(-1)?.text).toBe('updated after index rebuild');
    expect(webSessionRuntimePerformance.snapshot().eventSorts).toBe(1);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_page',
      h: {
        its: [makeWireHistoryItem(0, { id: 'historical-0' })],
        hm: false,
        tot: 5,
      },
    });
    expect(store.getBlocks(session.id).map(block => block.orderIndex)).toEqual([0, 1, 1.5, 2, 3]);
    expect(webSessionRuntimePerformance.snapshot().eventSorts).toBe(2);
  });

  it('invalidates runtime caches and event indexes across snapshots, archive moves, and delete', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-runtime-reset',
      status: 'running',
      assistantState: 'working',
    });
    listMock.mockResolvedValue([session]);
    snapshotMock
      .mockResolvedValueOnce({
        session,
        history: {
          items: [makeWireHistoryItem(1, { id: 'old-item' })],
          hasMore: false,
          total: 1,
        },
      })
      .mockResolvedValueOnce({
        session: { ...session, updatedAt: '2026-04-09T10:02:00.000Z' },
        history: {
          items: [
            makeWireHistoryItem(2, {
              id: 'new-item',
              kd: 'assistant',
              tp: 'agent_message',
              txt: 'snapshot replacement',
            }),
          ],
          hasMore: false,
          total: 1,
        },
      });
    deleteMock.mockResolvedValue(undefined);

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    store.getLiveState(session.id);
    webSessionRuntimePerformance.reset();

    await store.loadSessionSnapshot(session.projectId, session.id);
    expect(store.getBlocks(session.id).map(block => block.id)).toEqual(['new-item']);
    store.getPendingApproval(session.id);
    expect(webSessionRuntimePerformance.snapshot()).toMatchObject({
      fullDerivations: 1,
      scannedBlocks: 1,
      eventSorts: 1,
    });

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...session,
        archivedAt: '2026-04-09T10:03:00.000Z',
        updatedAt: '2026-04-09T10:03:00.000Z',
      }),
    });
    expect(webSessionRuntimePerformance.snapshot().fullDerivations).toBe(2);

    await store.deleteSession(session.projectId, session.id);
    expect(store.getBlocks(session.id)).toEqual([]);
    store.getLiveState(session.id);
    store.getPendingApproval(session.id);
    expect(webSessionRuntimePerformance.snapshot().fullDerivations).toBe(4);

    listMock.mockResolvedValue([{ ...session, updatedAt: '2026-04-09T10:04:00.000Z' }]);
    await store.loadSessions(session.projectId, true);
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: makeWireHistoryItem(1, {
        id: 'new-item',
        kd: 'assistant',
        tp: 'agent_message',
        txt: 'fresh after delete',
      }),
    });
    expect(store.getBlocks(session.id)).toHaveLength(1);
    expect(store.getBlocks(session.id)[0]?.text).toBe('fresh after delete');
  });

  it('preserves pending recovery and run failure semantics in the unified projection', async () => {
    const store = useWebSessionStore();
    const approvalSession = makeSession({
      id: 'session-approval-recovery',
      status: 'running',
      assistantState: 'waiting_approval',
    });
    const inputSession = makeSession({
      id: 'session-input-recovery',
      status: 'running',
      assistantState: 'waiting_input',
    });
    listMock.mockResolvedValue([approvalSession, inputSession]);
    snapshotMock.mockImplementation(async (_projectId: string, sessionId: string) => ({
      session: sessionId === approvalSession.id ? approvalSession : inputSession,
      history: {
        items:
          sessionId === approvalSession.id
            ? [
                makeWireHistoryItem(1, {
                  id: 'approval-request',
                  tp: 'approval_req',
                  dt: { type: 'approval_request', prompt: 'Allow command?' },
                }),
                makeWireHistoryItem(2, {
                  id: 'approval-restart',
                  tp: 'run_abort',
                  pl: { reason: 'process_restart', msg: 'Restarted while waiting' },
                }),
              ]
            : [
                makeWireHistoryItem(1, {
                  id: 'input-request',
                  siid: 'input-source-id',
                  tp: 'user_input_request',
                  dt: {
                    type: 'user_input_request',
                    prompt: 'Choose a target',
                    questions: [],
                  },
                }),
                makeWireHistoryItem(2, {
                  id: 'input-restart',
                  tp: 'run_abort',
                  pl: { reason: 'process_restart', msg: 'Restarted before input' },
                }),
              ],
        hasMore: false,
        total: 2,
      },
    }));

    await store.loadSessions(approvalSession.projectId);
    await store.loadSessionSnapshot(approvalSession.projectId, approvalSession.id);
    await store.loadSessionSnapshot(inputSession.projectId, inputSession.id);

    expect(store.getPendingApproval(approvalSession.id)).toMatchObject({
      id: 'approval-request',
      stale: true,
      recoveryReason: 'process_restart',
      recoveryMessage: 'Restarted while waiting',
    });
    expect(store.getLiveState(approvalSession.id)).toMatchObject({
      phase: 'waiting_approval',
      approval: { id: 'approval-request', stale: true },
    });
    expect(store.getPendingUserInput(inputSession.id)).toMatchObject({
      id: 'input-request',
      itemId: 'input-source-id',
      stale: true,
      recoveryReason: 'process_restart',
      recoveryMessage: 'Restarted before input',
    });

    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    const failedSession = {
      ...inputSession,
      status: 'err' as const,
      assistantState: null,
      updatedAt: '2026-04-09T10:05:00.000Z',
    };
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: inputSession.id,
      ts: Date.now(),
      op: 'hist_item',
      s: toWireSession(failedSession),
      i: makeWireHistoryItem(3, {
        id: 'run-failure',
        tp: 'run_fail',
        txt: 'Runtime failed',
      }),
    });
    expect(store.getPendingUserInput(inputSession.id)).toBeNull();
    expect(store.getLiveState(inputSession.id)).toMatchObject({
      phase: 'error',
      running: false,
      errorMessage: 'Runtime failed',
    });
  });

  it('restores an actionable approval from snapshot when approval history is missing', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-snapshot-approval',
      status: 'running',
      assistantState: 'waiting_approval',
    });
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: [
          makeWireHistoryItem(1, {
            kd: 'user',
            tp: 'user_message',
            txt: 'An earlier user message',
          }),
        ],
        hasMore: false,
        total: 1,
      },
      pendingApproval: {
        itemId: 'command-1',
        kind: 'command_approval',
        prompt: 'Approve this command?',
        command: 'rm -r /tmp/example',
        requestedAt: '2026-04-09T10:00:01.000Z',
        actionable: true,
      },
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);

    expect(store.getPendingApproval(session.id)).toMatchObject({
      id: 'command-1',
      itemId: 'command-1',
      kind: 'command_approval',
      prompt: 'Approve this command?',
      command: 'rm -r /tmp/example',
      actionable: true,
      stale: false,
    });
    expect(store.getLiveState(session.id)).toMatchObject({
      phase: 'waiting_approval',
      approval: { itemId: 'command-1' },
    });

    await store.openEventStream();
    findSocket('/api/v1/web-sessions/events')?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: makeWireHistoryItem(2, {
        id: 'approval-response',
        tp: 'approval_res',
        dt: { type: 'approval_response', action: 'approve' },
      }),
    });
    expect(store.getPendingApproval(session.id)).toBeNull();
  });

  it('does not emit an empty approval notification from waiting status alone', async () => {
    const store = useWebSessionStore();
    const workingSession = makeSession({
      id: 'session-missing-approval-details',
      status: 'running',
      assistantState: 'working',
    });
    const handleApproval = vi.fn();
    listMock.mockResolvedValue([workingSession]);

    await store.loadSessions(workingSession.projectId);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    store.getLiveState(workingSession.id);
    store.emitter.on('ai:approval-needed', handleApproval);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: workingSession.id,
      rev: '2',
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...workingSession,
        revision: '2',
        assistantState: 'waiting_approval',
        assistantStateUpdatedAt: '2026-04-09T10:02:00.000Z',
      }),
    });

    expect(store.getPendingApproval(workingSession.id)).toBeNull();
    expect(store.getLiveState(workingSession.id).phase).toBe('waiting_approval');
    expect(handleApproval).not.toHaveBeenCalled();
  });

  it('derives one next projection per history frame without repeating approval notifications', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-single-next-projection',
      status: 'running',
      assistantState: 'waiting_approval',
    });
    const handleApproval = vi.fn();
    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    store.getLiveState(session.id);
    store.emitter.on('ai:approval-needed', handleApproval);
    webSessionRuntimePerformance.reset();

    const approvalItem = makeWireHistoryItem(1, {
      id: 'approval-single-frame',
      tp: 'approval_req',
      dt: { type: 'approval_request', prompt: 'Allow this operation?' },
    });
    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: approvalItem,
    });
    expect(webSessionRuntimePerformance.snapshot()).toEqual({
      fullDerivations: 0,
      incrementalDerivations: 1,
      scannedBlocks: 1,
      eventSorts: 0,
    });
    expect(handleApproval).toHaveBeenCalledTimes(1);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'hist_item',
      i: approvalItem,
    });
    expect(webSessionRuntimePerformance.snapshot()).toMatchObject({
      fullDerivations: 0,
      incrementalDerivations: 2,
      scannedBlocks: 2,
      eventSorts: 0,
    });
    expect(handleApproval).toHaveBeenCalledTimes(1);
  });

  it('does not repeat plan approval notifications for delayed plan updates', async () => {
    const store = useWebSessionStore();
    const workingSession = makeSession({
      id: 'session-delayed-plan-approval',
      status: 'running',
      assistantState: 'working',
      workflowMode: 'plan',
    });
    const planApprovalSession = makeSession({
      ...workingSession,
      revision: '2',
      assistantState: 'waiting_plan_approval',
      assistantStateUpdatedAt: '2026-04-09T10:02:00.000Z',
      updatedAt: '2026-04-09T10:02:00.000Z',
    });
    const handleApproval = vi.fn();
    listMock.mockResolvedValue([workingSession]);

    await store.loadSessions(workingSession.projectId);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    store.getLiveState(workingSession.id);
    store.emitter.on('ai:approval-needed', handleApproval);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: workingSession.id,
      rev: '2',
      ts: Date.now(),
      op: 'session',
      s: toWireSession(planApprovalSession),
    });
    expect(handleApproval).toHaveBeenCalledTimes(1);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: workingSession.id,
      rev: '3',
      ts: Date.now(),
      op: 'scheduled',
      si: [
        {
          id: 'scheduled-plan-1',
          a: 'execute_plan',
          tid: 'plan-item-1',
          m: 'send',
          st: 'scheduled',
          txt: 'Implement the plan.',
          atts: [],
          sf: Date.parse('2026-04-09T10:10:00.000Z'),
          ca: Date.parse('2026-04-09T10:02:00.000Z'),
          ua: Date.parse('2026-04-09T10:02:00.000Z'),
        },
      ],
    });
    expect(handleApproval).toHaveBeenCalledTimes(1);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: workingSession.id,
      rev: '4',
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...planApprovalSession,
        revision: '4',
        workflowMode: 'default',
        updatedAt: '2026-04-09T10:03:00.000Z',
      }),
    });
    expect(handleApproval).toHaveBeenCalledTimes(1);

    eventSocket?.dispatch({
      v: 1,
      k: 'evt',
      sid: workingSession.id,
      rev: '5',
      ts: Date.now(),
      op: 'session',
      s: toWireSession({
        ...planApprovalSession,
        revision: '5',
        workflowMode: 'default',
        assistantStateUpdatedAt: '2026-04-09T10:04:00.000Z',
        updatedAt: '2026-04-09T10:04:00.000Z',
      }),
    });
    expect(handleApproval).toHaveBeenCalledTimes(2);
  });

  it('singleflights and caches stable command group details per session', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ id: 'session-command-group' });
    listMock.mockResolvedValue([session]);
    await store.loadSessions(session.projectId);

    let resolveDetail!: (detail: {
      groupId: string;
      kind: string;
      title: string;
      summary: string;
      count: number;
      firstSeq: number;
      lastSeq: number;
      status: 'done';
      items: [];
    }) => void;
    const detailPromise = new Promise<Parameters<typeof resolveDetail>[0]>(resolve => {
      resolveDetail = resolve;
    });
    commandGroupDetailMock.mockReturnValueOnce(detailPromise);

    const first = store.loadCommandGroupDetail(session.id, 'group-1');
    const second = store.loadCommandGroupDetail(session.id, 'group-1');
    expect(commandGroupDetailMock).toHaveBeenCalledTimes(1);

    resolveDetail({
      groupId: 'group-1',
      kind: 'command_execution',
      title: 'CommandExecution',
      summary: 'git status',
      count: 2,
      firstSeq: 1,
      lastSeq: 2,
      status: 'done',
      items: [],
    });
    const [firstDetail, secondDetail] = await Promise.all([first, second]);
    expect(firstDetail).toBe(secondDetail);

    await store.loadCommandGroupDetail(session.id, 'group-1');
    expect(commandGroupDetailMock).toHaveBeenCalledTimes(1);
  });

  it('updates a 500-block assistant tail 1000 times without full rescans or event sorts', async () => {
    const store = useWebSessionStore();
    const session = makeSession({
      id: 'session-runtime-performance',
      status: 'running',
      assistantState: 'working',
      itemCount: 500,
    });
    const historyItems = Array.from({ length: 499 }, (_, index) => makeWireHistoryItem(index + 1));
    historyItems.push(
      makeWireHistoryItem(500, {
        id: 'assistant-stream',
        kd: 'assistant',
        tp: 'agent_message',
        txt: 'chunk 0',
        dn: false,
      })
    );
    listMock.mockResolvedValue([session]);
    snapshotMock.mockResolvedValue({
      session,
      history: {
        items: historyItems,
        hasMore: false,
        total: 500,
      },
    });

    await store.loadSessions(session.projectId);
    await store.loadSessionSnapshot(session.projectId, session.id);
    await store.openEventStream();
    const eventSocket = findSocket('/api/v1/web-sessions/events');
    const handleWorking = vi.fn();
    const handleApproval = vi.fn();
    const handleCompleted = vi.fn();
    store.emitter.on('ai:working', handleWorking);
    store.emitter.on('ai:approval-needed', handleApproval);
    store.emitter.on('ai:completed', handleCompleted);
    store.getLiveState(session.id);
    webSessionRuntimePerformance.reset();

    for (let index = 0; index < 1000; index += 1) {
      eventSocket?.dispatch({
        v: 1,
        k: 'evt',
        sid: session.id,
        ts: Date.now(),
        op: 'hist_item',
        i: makeWireHistoryItem(500, {
          id: 'assistant-stream',
          kd: 'assistant',
          tp: 'agent_message',
          txt: `chunk ${index + 1}`,
          obs: Date.parse('2026-04-09T10:01:00.000Z') + index,
          dn: false,
        }),
      });
    }

    expect(store.getBlocks(session.id)).toHaveLength(500);
    expect(store.getBlocks(session.id).at(-1)?.text).toBe('chunk 1000');
    expect(webSessionRuntimePerformance.snapshot()).toEqual({
      fullDerivations: 0,
      incrementalDerivations: 1000,
      scannedBlocks: 1000,
      eventSorts: 0,
    });
    expect(handleWorking).not.toHaveBeenCalled();
    expect(handleApproval).not.toHaveBeenCalled();
    expect(handleCompleted).not.toHaveBeenCalled();
  });
});
