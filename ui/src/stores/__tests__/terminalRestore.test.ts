import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick, watch } from 'vue';

import type { TerminalSession } from '@/types/models';
import { useTerminalStore } from '@/stores/terminal';
import { useSettingsStore } from '@/stores/settings';

const { listSendMock, listMock, postSendMock, postMock } = vi.hoisted(() => ({
  listSendMock: vi.fn(),
  listMock: vi.fn(),
  postSendMock: vi.fn(),
  postMock: vi.fn(),
}));

vi.mock('@/api', () => ({
  default: {
    terminalSession: {
      list: listMock,
      create: vi.fn(),
      rename: vi.fn(),
      close: vi.fn(),
      terminalCounts: vi.fn(),
    },
  },
  alovaInstance: {
    Post: postMock,
  },
  urlBase: 'http://localhost:5173',
}));

vi.mock('@/utils/ws', () => ({
  resolveWsUrl: (path: string) => path,
}));

class FakeWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  url: string;
  readyState = FakeWebSocket.CONNECTING;
  binaryType = 'blob';
  onopen: ((event: unknown) => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: ((event: unknown) => void) | null = null;
  onclose: ((event: unknown) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
    queueMicrotask(() => {
      this.readyState = FakeWebSocket.OPEN;
      this.onopen?.({});
    });
  }

  addEventListener(type: string, listener: (event: unknown) => void) {
    if (type === 'open') this.onopen = listener;
    if (type === 'message') this.onmessage = listener;
    if (type === 'error') this.onerror = listener;
    if (type === 'close') this.onclose = listener;
  }

  send = vi.fn();

  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.({});
  }
}

function findTerminalSocket(sessionId: string) {
  return (
    FakeWebSocket.instances.find(instance =>
      instance.url.includes(`/api/v1/terminal/ws?sessionId=${sessionId}`)
    ) ?? null
  );
}

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

function makeTerminalSession(overrides: Partial<TerminalSession> = {}): TerminalSession {
  const id = overrides.id ?? 'terminal-1';
  return {
    id,
    projectId: 'project-1',
    worktreeId: 'worktree-1',
    orderIndex: 1000,
    workingDir: '/tmp/project',
    title: 'Backend',
    createdAt: '2026-05-15T00:00:00.000Z',
    lastActive: '2026-05-15T00:01:00.000Z',
    status: 'running',
    wsPath: `ws://localhost:5173/api/v1/terminal/ws?sessionId=${id}`,
    wsUrl: `ws://localhost:5173/api/v1/terminal/ws?sessionId=${id}`,
    rows: 24,
    cols: 80,
    ...overrides,
  };
}

describe('terminal auto restore', () => {
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

    listSendMock.mockReset();
    listMock.mockReset();
    postSendMock.mockReset();
    postMock.mockReset();
    FakeWebSocket.instances = [];

    listMock.mockReturnValue({
      send: listSendMock,
    });
    postMock.mockReturnValue({
      send: postSendMock,
    });
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('restores sessions when the project has no live terminals', async () => {
    listSendMock.mockResolvedValue({ items: [] });
    postSendMock.mockResolvedValue({
      items: [makeTerminalSession({ id: 'restored-1', title: 'Restored' })],
    });

    const store = useTerminalStore();
    await store.loadSessions('project-1');

    expect(postMock).toHaveBeenCalledWith(
      '/api/v1/projects/project-1/terminals/restore',
      {},
      { cacheFor: 0 }
    );
    expect(store.getTabs('project-1').map(tab => tab.id)).toEqual(['restored-1']);
  });

  it('skips restore when the live session list is already populated', async () => {
    listSendMock.mockResolvedValue({
      items: [makeTerminalSession({ id: 'live-1', title: 'Live' })],
    });

    const store = useTerminalStore();
    await store.loadSessions('project-1');

    expect(postMock).not.toHaveBeenCalled();
    expect(store.getTabs('project-1').map(tab => tab.id)).toEqual(['live-1']);
  });

  it('reloads sessions and restores after an unexpected websocket close', async () => {
    vi.useFakeTimers();
    try {
      listSendMock
        .mockResolvedValueOnce({
          items: [makeTerminalSession({ id: 'restored-1', title: 'Live before restart' })],
        })
        .mockResolvedValueOnce({ items: [] });
      postSendMock.mockResolvedValue({
        items: [makeTerminalSession({ id: 'restored-1', title: 'Live after restart' })],
      });

      const store = useTerminalStore();
      await store.loadSessions('project-1');
      store.retainProjectConnections('project-1');

      await Promise.resolve();
      await Promise.resolve();

      const terminalSocket = findTerminalSocket('restored-1');
      expect(terminalSocket).toBeTruthy();
      terminalSocket?.close();

      await vi.advanceTimersByTimeAsync(1000);
      await Promise.resolve();
      await Promise.resolve();

      expect(postMock).toHaveBeenCalledWith(
        '/api/v1/projects/project-1/terminals/restore',
        {},
        { cacheFor: 0 }
      );
      expect(FakeWebSocket.instances.length).toBeGreaterThanOrEqual(2);
      expect(store.getTabs('project-1').map(tab => tab.id)).toEqual(['restored-1']);
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps the snapshot preference when the server falls back to live', async () => {
    listSendMock.mockResolvedValue({
      items: [makeTerminalSession({ id: 'fallback-1', title: 'Fallback' })],
    });

    const store = useTerminalStore();
    await store.loadSessions('project-1');
    store.retainProjectConnections('project-1');
    await Promise.resolve();
    await Promise.resolve();

    const terminalSocket = findTerminalSocket('fallback-1');
    expect(terminalSocket).toBeTruthy();

    store.setSessionRenderMode('project-1', 'fallback-1', 'snapshot');
    terminalSocket?.onmessage?.({
      data: JSON.stringify({
        type: 'render-mode',
        mode: 'live',
        snapshotIntervalMs: 50,
      }),
    });
    await Promise.resolve();
    await Promise.resolve();

    const tab = store.getTabs('project-1').find(item => item.id === 'fallback-1');
    expect(tab?.renderMode).toBe('snapshot');
    expect(tab?.connectionRenderMode).toBe('live');

    store.setSessionRenderMode('project-1', 'fallback-1', null);
    store.releaseProjectConnections('project-1');
  });

  it('reconciles 500 sessions without quadratic ID reads or invalidating list membership', async () => {
    const items = Array.from({ length: 500 }, (_, i) =>
      makeTerminalSession({ id: `scale-${i}`, orderIndex: i + 1 })
    );
    listSendMock.mockResolvedValue({ items });
    const store = useTerminalStore();
    await store.loadSessions('project-1');
    const tabs = store.getTabs('project-1');
    const first = tabs[0];
    let idReads = 0;
    for (const tab of tabs) {
      let id = tab.id;
      Object.defineProperty(tab, 'id', {
        configurable: true,
        enumerable: true,
        get() {
          idReads++;
          return id;
        },
        set(value: string) {
          id = value;
        },
      });
    }
    const structureChanged = vi.fn();
    const stop = watch(() => tabs.map(tab => tab.id), structureChanged);
    idReads = 0;
    listSendMock.mockResolvedValue({ items: items.map(item => ({ ...item, title: 'Updated' })) });
    await store.loadSessions('project-1');
    await nextTick();
    expect(idReads).toBeLessThan(items.length * 20);
    expect(store.getTabs('project-1')[0]).toBe(first);
    expect(first?.title).toBe('Updated');
    expect(structureChanged).not.toHaveBeenCalled();
    stop();
  });

  it('updates metadata locally and does not resend unchanged connection preferences during list refresh', async () => {
    const items = Array.from({ length: 20 }, (_, i) =>
      makeTerminalSession({ id: `metadata-${i}`, orderIndex: i + 1 })
    );
    listSendMock.mockResolvedValue({ items });
    const store = useTerminalStore();
    await store.loadSessions('project-1');
    store.retainProjectConnections('project-1');
    await Promise.resolve();
    const socket = findTerminalSocket('metadata-0')!;
    const tab = store.getSessionById('metadata-0')!;
    const titleChanged = vi.fn();
    const structureChanged = vi.fn();
    const stopTitle = watch(() => tab.title, titleChanged);
    const stopStructure = watch(
      () => store.getTabs('project-1').map(item => item.id),
      structureChanged
    );
    socket.send.mockClear();
    socket.onmessage?.({
      data: JSON.stringify({
        type: 'metadata',
        metadata: { title: 'Renamed', processStatus: 'busy' },
      }),
    });
    await nextTick();
    expect(titleChanged).toHaveBeenCalledOnce();
    expect(structureChanged).not.toHaveBeenCalled();
    await store.loadSessions('project-1');
    expect(socket.send).not.toHaveBeenCalled();
    expect(store.getSessionById(tab.id)).toBe(tab);
    stopTitle();
    stopStructure();
    store.releaseProjectConnections('project-1');
  });

  it('removes stale sessions and restores selection after an authoritative reorder', async () => {
    const first = makeTerminalSession({ id: 'remove-first', orderIndex: 1 });
    const second = makeTerminalSession({ id: 'keep-second', orderIndex: 2 });
    listSendMock.mockResolvedValue({ items: [first, second] });
    const store = useTerminalStore();
    await store.loadSessions('project-1');
    store.setActiveTab('project-1', first.id);
    listSendMock.mockResolvedValue({
      items: [second, makeTerminalSession({ id: 'new-first', orderIndex: 0.5 })],
    });
    await store.loadSessions('project-1');
    expect(store.getTabs('project-1').map(tab => tab.id)).toEqual(['new-first', second.id]);
    expect(store.getSessionById(first.id)).toBeUndefined();
    expect(store.getActiveTabId('project-1')).toBe('new-first');
    listSendMock.mockResolvedValue({ items: [] });
    postSendMock.mockResolvedValue({ items: [] });
    await store.loadSessions('project-1');
    expect(store.getActiveTabId('project-1')).toBe('');
    expect(store.getTabs('project-1')).toHaveLength(0);
  });

  it('reuses an unconsumed live replay but requests full history after renderer eviction', async () => {
    const id = 'live-replay';
    listSendMock.mockResolvedValue({ items: [makeTerminalSession({ id })] });
    const store = useTerminalStore();
    await store.loadSessions('project-1');
    store.retainProjectConnections('project-1');
    await Promise.resolve();
    const oldSocket = findTerminalSocket(id)!;
    const data = { type: 'data', data: 'aGlzdG9yeQ==' };
    oldSocket.onmessage?.({ data: JSON.stringify(data) });
    store.prepareViewport(id);
    expect(oldSocket.readyState).toBe(FakeWebSocket.OPEN);
    const receive = vi.fn();
    store.emitter.on(id, receive);
    expect(store.replayBufferedMessages(id).count).toBe(1);
    expect(receive).toHaveBeenCalledWith(data);
    store.emitter.off(id, receive);
    oldSocket.onmessage?.({ data: JSON.stringify({ type: 'data', data: 'c3RhbGU=' }) });
    store.prepareViewport(id);
    await Promise.resolve();
    const newSocket = FakeWebSocket.instances.at(-1)!;
    expect(newSocket).not.toBe(oldSocket);
    expect(store.replayBufferedMessages(id).count).toBe(0);
    // A late close/error from the evicted connection must not remove its replacement.
    oldSocket.onclose?.({});
    oldSocket.onerror?.({});
    expect(store.send(id, { type: 'input', data: 'x' })).toBe(true);
    expect(store.getSessionById(id)?.clientStatus).toBe('ready');
    store.releaseProjectConnections('project-1');
  });

  it('keeps mirror connections and only updates the two changed roles when selecting another tab', async () => {
    const settings = useSettingsStore();
    settings.updateTerminalConnectionPolicy('active-plus-mirror');
    settings.updateDefaultTerminalRenderMode('live');
    listSendMock.mockResolvedValue({
      items: Array.from({ length: 30 }, (_, i) =>
        makeTerminalSession({ id: `mirror-${i}`, orderIndex: i + 1 })
      ),
    });
    const store = useTerminalStore();
    await store.loadSessions('project-1');
    store.retainProjectConnections('project-1');
    await Promise.resolve();
    const sockets = FakeWebSocket.instances.filter(socket => socket.url.includes('/terminal/ws?'));
    expect(sockets).toHaveLength(30);
    sockets.forEach(socket => socket.send.mockClear());
    await store.loadSessions('project-1');
    expect(sockets.every(socket => socket.send.mock.calls.length === 0)).toBe(true);
    store.setActiveTab('project-1', 'mirror-29');
    expect(sockets.filter(socket => socket.send.mock.calls.length > 0)).toHaveLength(2);
    expect(store.getSessionById('mirror-0')?.connectionRole).toBe('mirror');
    expect(store.getSessionById('mirror-29')?.connectionRole).toBe('active');
    // Snapshot mirrors already have a complete frame, so remounting them needs no reconnect.
    const count = FakeWebSocket.instances.length;
    store.replayBufferedMessages('mirror-1');
    store.prepareViewport('mirror-1');
    expect(FakeWebSocket.instances).toHaveLength(count);
    store.releaseProjectConnections('project-1');
  });

  it('does not resurrect snapshot cache entries when a closed renderer unmounts', async () => {
    const id = 'snapshot-cleanup';
    listSendMock.mockResolvedValue({ items: [makeTerminalSession({ id })] });
    const store = useTerminalStore();
    await store.loadSessions('project-1');
    const snapshot = { content: 'screen', updatedAt: Date.now(), rows: 24, cols: 80 };
    store.saveSerializedSnapshot(id, snapshot);
    expect(store.getSerializedSnapshot(id)).toEqual(snapshot);
    store.disconnectTab(id);
    store.saveSerializedSnapshot(id, snapshot);
    expect(store.getSerializedSnapshot(id)).toBeUndefined();
  });

  it('requests a new live replay when an unmounted connection has passed through snapshot mode', async () => {
    const id = 'mirror-replay-gap';
    listSendMock.mockResolvedValue({ items: [makeTerminalSession({ id })] });
    const store = useTerminalStore();
    await store.loadSessions('project-1');
    store.retainProjectConnections('project-1');
    await Promise.resolve();
    const socket = findTerminalSocket(id)!;
    for (const mode of ['snapshot', 'live']) {
      socket.onmessage?.({ data: JSON.stringify({ type: 'render-mode', mode }) });
    }
    store.prepareViewport(id);
    expect(socket.readyState).toBe(FakeWebSocket.CLOSED);
    expect(FakeWebSocket.instances.at(-1)).not.toBe(socket);
    await Promise.resolve();
    store.releaseProjectConnections('project-1');
  });
});
