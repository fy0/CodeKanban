import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { WebSessionSummary } from '@/types/models';
import { useWebSessionStore } from '@/stores/webSession';

const { listMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
}));

vi.mock('@/api/webSession', () => ({
  webSessionApi: {
    list: listMock,
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
    projectId: 'project-1',
    worktreeId: null,
    orderIndex: 1000,
    agent: 'codex',
    title: 'Codex Session',
    model: 'gpt-5.4',
    reasoningEffort: 'medium',
    workflowMode: 'default',
    permissionLevel: 'elevated',
    activeCallTimeoutEnabled: false,
    autoRetryEnabled: false,
    autoRetryPolicyMode: 'default',
    autoRetryScope: 'network_only',
    autoRetryPreset: 'gentle_stop',
    autoRetryDispatchPendingOnFailure: false,
    cwd: '/tmp/project',
    nativeSessionId: 'native-1',
    status: 'running',
    assistantState: 'working',
    hasUnread: false,
    archivedAt: null,
    activityAt: '2026-04-10T10:00:00.000Z',
    statusUpdatedAt: '2026-04-10T10:00:00.000Z',
    lastMessageAt: '2026-04-10T10:00:00.000Z',
    assistantStateUpdatedAt: '2026-04-10T10:00:00.000Z',
    sourceKind: 'codex_app_server',
    syncState: 'fresh',
    lastSyncMode: 'fast',
    sourceCreatedAt: '2026-04-10T09:00:00.000Z',
    sourceUpdatedAt: '2026-04-10T10:00:00.000Z',
    lastSyncedAt: '2026-04-10T10:00:00.000Z',
    threadPath: '/tmp/session.jsonl',
    threadPreview: 'preview',
    turnCount: 1,
    itemCount: 1,
    syncError: null,
    createdAt: '2026-04-10T09:00:00.000Z',
    updatedAt: '2026-04-10T10:00:00.000Z',
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
    pid: session.projectId,
    wid: session.worktreeId,
    oi: session.orderIndex,
    ag: session.agent,
    md: session.model,
    re: session.reasoningEffort,
    wm: session.workflowMode,
    pl: session.permissionLevel,
    acte: session.activeCallTimeoutEnabled === true,
    ae: session.autoRetryEnabled,
    arpm: session.autoRetryPolicyMode,
    ars: session.autoRetryScope,
    arp: session.autoRetryPreset,
    aram: session.autoRetryMaxAttempts,
    ardpf: session.autoRetryDispatchPendingOnFailure,
    ttl: session.title,
    cwd: session.cwd,
    nsid: session.nativeSessionId,
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

class FakeWebSocket {
  static OPEN = 1;
  static instances: FakeWebSocket[] = [];

  url: string;
  readyState = 0;
  sent: Array<Record<string, unknown>> = [];
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
    this.sent.push(JSON.parse(payload) as Record<string, unknown>);
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

function findSocket(url: string) {
  return FakeWebSocket.instances.find(instance => instance.url === url) ?? null;
}

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
}

describe('webSession auto retry optimistic updates', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const localStorage = createStorageMock();
    vi.stubGlobal('localStorage', localStorage);
    vi.stubGlobal('window', {
      localStorage,
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
    listMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps the optimistic auto retry toggle when an older session summary arrives first', async () => {
    const store = useWebSessionStore();
    const session = makeSession();
    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId, true);
    await store.openEventStream();
    await flushMicrotasks();

    const updatePromise = store.updateAutoRetry(session.id, {
      enabled: true,
      scope: 'network_only',
      preset: 'gentle_stop',
    });
    await flushMicrotasks();

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    if (!eventSocket || !commandSocket) {
      throw new Error('expected both event and command sockets to be connected');
    }

    const optimisticSession = store.getSessions(session.projectId)[0];
    expect(optimisticSession?.autoRetryEnabled).toBe(true);
    expect(optimisticSession?.autoRetryPolicyMode).toBe('custom');
    expect(commandSocket.sent[0]).toMatchObject({
      op: 'set_ar',
      sid: session.id,
      p: {
        ae: true,
        arpm: 'custom',
        ars: 'network_only',
        arp: 'gentle_stop',
      },
    });

    eventSocket.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'status',
      s: toWireSession(
        makeSession({
          updatedAt: '2026-04-10T09:59:59.000Z',
        })
      ),
    });

    const afterStaleFrame = store.getSessions(session.projectId)[0];
    expect(afterStaleFrame?.autoRetryEnabled).toBe(true);
    expect(afterStaleFrame?.autoRetryScope).toBe('network_only');
    expect(afterStaleFrame?.autoRetryPreset).toBe('gentle_stop');

    const requestId = String(commandSocket.sent[0]?.rid ?? '');
    commandSocket.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'set_ar',
    });
    await updatePromise;

    eventSocket.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'status',
      s: toWireSession(
        makeSession({
          autoRetryEnabled: true,
          autoRetryPolicyMode: 'custom',
          updatedAt: '2026-04-10T10:00:02.000Z',
        })
      ),
    });

    const confirmedSession = store.getSessions(session.projectId)[0];
    expect(confirmedSession?.autoRetryEnabled).toBe(true);
    expect(confirmedSession?.autoRetryScope).toBe('network_only');
    expect(confirmedSession?.autoRetryPreset).toBe('gentle_stop');
  });

  it('keeps the optimistic auto retry toggle after ack until a newer summary confirms it', async () => {
    const store = useWebSessionStore();
    const session = makeSession();
    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId, true);
    await store.openEventStream();
    await flushMicrotasks();

    const updatePromise = store.updateAutoRetry(session.id, {
      enabled: true,
      scope: 'network_only',
      preset: 'gentle_stop',
    });
    await flushMicrotasks();

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    if (!eventSocket || !commandSocket) {
      throw new Error('expected both event and command sockets to be connected');
    }

    const requestId = String(commandSocket.sent[0]?.rid ?? '');
    commandSocket.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'set_ar',
    });
    await updatePromise;

    eventSocket.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(
        makeSession({
          updatedAt: '2026-04-10T10:00:01.000Z',
        })
      ),
    });

    const stillOptimisticSession = store.getSessions(session.projectId)[0];
    expect(stillOptimisticSession?.autoRetryEnabled).toBe(true);
    expect(stillOptimisticSession?.autoRetryScope).toBe('network_only');
    expect(stillOptimisticSession?.autoRetryPreset).toBe('gentle_stop');

    eventSocket.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(
        makeSession({
          autoRetryEnabled: true,
          autoRetryPolicyMode: 'custom',
          updatedAt: '2026-04-10T10:00:02.000Z',
        })
      ),
    });

    const confirmedSession = store.getSessions(session.projectId)[0];
    expect(confirmedSession?.autoRetryEnabled).toBe(true);
    expect(confirmedSession?.autoRetryScope).toBe('network_only');
    expect(confirmedSession?.autoRetryPreset).toBe('gentle_stop');
  });

  it('toggles a default policy without sending browser-owned policy values', async () => {
    const store = useWebSessionStore();
    const session = makeSession();
    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId, true);
    await store.openEventStream();
    await flushMicrotasks();

    const updatePromise = store.updateAutoRetry(session.id, {
      enabled: true,
      policyMode: 'default',
    });
    await flushMicrotasks();

    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    if (!commandSocket) {
      throw new Error('expected the command socket to be connected');
    }
    expect(commandSocket.sent[0]).toMatchObject({
      op: 'set_ar',
      sid: session.id,
      p: { ae: true, arpm: 'default' },
    });
    expect(commandSocket.sent[0]?.p).not.toHaveProperty('ars');
    expect(commandSocket.sent[0]?.p).not.toHaveProperty('arp');
    expect(commandSocket.sent[0]?.p).not.toHaveProperty('aram');

    const requestId = String(commandSocket.sent[0]?.rid ?? '');
    commandSocket.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'set_ar',
    });
    await updatePromise;
  });

  it('keeps the optimistic retry failure dispatch toggle until a newer summary confirms it', async () => {
    const store = useWebSessionStore();
    const session = makeSession();
    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId, true);
    await store.openEventStream();
    await flushMicrotasks();

    const updatePromise = store.updateAutoRetryDispatchPendingOnFailure(session.id, true);
    await flushMicrotasks();

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    if (!eventSocket || !commandSocket) {
      throw new Error('expected both event and command sockets to be connected');
    }

    expect(store.getSessions(session.projectId)[0]?.autoRetryDispatchPendingOnFailure).toBe(true);
    expect(commandSocket.sent[0]).toMatchObject({
      op: 'set_ardpf',
      sid: session.id,
      p: { ardpf: true },
    });

    eventSocket.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'status',
      s: toWireSession(
        makeSession({
          updatedAt: '2026-04-10T09:59:59.000Z',
        })
      ),
    });
    expect(store.getSessions(session.projectId)[0]?.autoRetryDispatchPendingOnFailure).toBe(true);

    const requestId = String(commandSocket.sent[0]?.rid ?? '');
    commandSocket.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'set_ardpf',
    });
    await updatePromise;

    eventSocket.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'session',
      s: toWireSession(
        makeSession({
          autoRetryDispatchPendingOnFailure: true,
          updatedAt: '2026-04-10T10:00:02.000Z',
        })
      ),
    });

    expect(store.getSessions(session.projectId)[0]?.autoRetryDispatchPendingOnFailure).toBe(true);
  });

  it('keeps the optimistic active call timeout toggle when an older summary arrives first', async () => {
    const store = useWebSessionStore();
    const session = makeSession({ activeCallTimeoutEnabled: false });
    listMock.mockResolvedValue([session]);

    await store.loadSessions(session.projectId, true);
    await store.openEventStream();
    await flushMicrotasks();

    const updatePromise = store.updateActiveCallTimeout(session.id, true);
    await flushMicrotasks();

    const eventSocket = findSocket('/api/v1/web-sessions/events');
    const commandSocket = findSocket('/api/v1/web-sessions/ws');
    if (!eventSocket || !commandSocket) {
      throw new Error('expected both event and command sockets to be connected');
    }

    const optimisticSession = store.getSessions(session.projectId)[0];
    expect(optimisticSession?.activeCallTimeoutEnabled).toBe(true);

    eventSocket.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'status',
      s: toWireSession(
        makeSession({
          activeCallTimeoutEnabled: false,
          updatedAt: '2026-04-10T09:59:59.000Z',
        })
      ),
    });

    const afterStaleFrame = store.getSessions(session.projectId)[0];
    expect(afterStaleFrame?.activeCallTimeoutEnabled).toBe(true);

    const requestId = String(commandSocket.sent[0]?.rid ?? '');
    commandSocket.dispatch({
      v: 1,
      k: 'ack',
      rid: requestId,
      sid: session.id,
      ts: Date.now(),
      op: 'set_act',
    });
    await updatePromise;

    eventSocket.dispatch({
      v: 1,
      k: 'evt',
      sid: session.id,
      ts: Date.now(),
      op: 'status',
      s: toWireSession(
        makeSession({
          activeCallTimeoutEnabled: true,
          updatedAt: '2026-04-10T10:00:02.000Z',
        })
      ),
    });

    const confirmedSession = store.getSessions(session.projectId)[0];
    expect(confirmedSession?.activeCallTimeoutEnabled).toBe(true);
  });
});
