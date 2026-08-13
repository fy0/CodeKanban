import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { TerminalSession } from '@/types/models';
import { useTerminalStore } from '@/stores/terminal';

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
  onclose: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
    queueMicrotask(() => {
      this.readyState = FakeWebSocket.OPEN;
      this.onopen?.({});
    });
  }

  addEventListener(type: string, listener: (...args: any[]) => void) {
    if (type === 'open') this.onopen = listener;
    if (type === 'message') this.onmessage = listener;
    if (type === 'error') this.onerror = listener;
    if (type === 'close') this.onclose = listener;
  }

  send(_payload: string) {}

  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.();
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
});
