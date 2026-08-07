import test from 'node:test';
import assert from 'node:assert/strict';

import { CodeKanbanClient } from '../src/client.js';
import { normalizeFsPath } from '../src/utils.js';

function createJsonResponse(payload, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    async text() {
      return JSON.stringify(payload);
    },
  };
}
function createWrappedJsonResponse(payload, status = 200) {
  return createJsonResponse({ body: payload }, status);
}

function createTextResponse(payload, status = 200, contentType = 'text/plain; charset=utf-8') {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get(name) {
        return String(name || '').toLowerCase() === 'content-type' ? contentType : null;
      },
    },
    async text() {
      return payload;
    },
  };
}


function createFetchMock(handlers) {
  return async (input, init = {}) => {
    const url = input instanceof URL ? input : new URL(String(input));
    const method = (init.method || 'GET').toUpperCase();
    const key = `${method} ${url.pathname}`;
    const handler = handlers.get(key);
    assert.ok(handler, `unexpected request: ${key}`);
    const parsedBody = init.body ? JSON.parse(init.body) : undefined;
    return handler({ url, method, body: parsedBody, headers: init.headers || {} });
  };
}

class FakeWebSocket {
  static instances = [];

  constructor(url, options) {
    this.url = url;
    this.options = options || null;
    this.listeners = new Map();
    this.sent = [];
    FakeWebSocket.instances.push(this);
    queueMicrotask(() => {
      this.emit('open', { type: 'open' });
      this.emit('message', { data: JSON.stringify({ type: 'ready', data: 'running' }) });
    });
  }

  addEventListener(type, handler) {
    const bucket = this.listeners.get(type) || new Set();
    bucket.add(handler);
    this.listeners.set(type, bucket);
  }

  removeEventListener(type, handler) {
    const bucket = this.listeners.get(type);
    if (!bucket) {
      return;
    }
    bucket.delete(handler);
  }

  emit(type, payload) {
    const bucket = this.listeners.get(type);
    if (!bucket) {
      return;
    }
    for (const handler of bucket) {
      handler(payload);
    }
  }

  send(payload) {
    this.sent.push(JSON.parse(payload));
  }

  close() {
    this.emit('close', { type: 'close' });
  }
}

test('resolveProject creates a project when path is not registered', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects', () => createJsonResponse({ items: [] })],
    ['POST /api/v1/projects/create', ({ body }) => createJsonResponse({ item: { id: 'p1', path: body.path, name: body.name } }, 201)],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.resolveProject({ path: 'D:/repo/demo' });
  assert.equal(result.project.id, 'p1');
  assert.equal(result.matchedBy, 'created');
});

test('startWorkflow creates a terminal and sends command plus prompt', async () => {
  FakeWebSocket.instances.length = 0;
  const handlers = new Map([
    ['GET /api/v1/projects', () => createJsonResponse({ items: [{ id: 'p1', path: 'D:/repo/demo', name: 'demo' }] })],
    ['GET /api/v1/projects/p1/worktrees', () => createJsonResponse({ items: [{ id: 'w1', path: 'D:/repo/demo', isMain: true }] })],
    ['POST /api/v1/projects/p1/worktrees/w1/terminals', () => createJsonResponse({ item: { id: 't1', wsPath: '/api/v1/terminal/ws?sessionId=t1', wsUrl: '/api/v1/terminal/ws?sessionId=t1', title: 'demo', projectId: 'p1', worktreeId: 'w1', workingDir: 'D:/repo/demo' } }, 201)],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.startWorkflow({
    path: 'D:/repo/demo',
    agent: 'codex',
    profile: 'plan',
    permissions: { addDirs: ['D:/shared'] },
    prompt: 'Inspect and plan first',
  });

  assert.equal(result.project.id, 'p1');
  assert.equal(result.worktree.id, 'w1');
  assert.equal(result.terminalSession.id, 't1');
  assert.equal(FakeWebSocket.instances.length, 1);
  assert.equal(FakeWebSocket.instances[0].url, 'ws://127.0.0.1:3000/api/v1/terminal/ws?sessionId=t1');
  assert.match(FakeWebSocket.instances[0].sent[0].data, /codex -s workspace-write -a on-request --add-dir D:\/shared/);
  assert.match(FakeWebSocket.instances[0].sent[1].data, /planning mode/i);
});

test('startWorkflow launches Claude through CCR when requested', async () => {
  FakeWebSocket.instances.length = 0;
  const handlers = new Map([
    ['GET /api/v1/projects', () => createJsonResponse({ items: [{ id: 'p1', path: 'D:/repo/demo', name: 'demo' }] })],
    ['GET /api/v1/projects/p1/worktrees', () => createJsonResponse({ items: [{ id: 'w1', path: 'D:/repo/demo', isMain: true }] })],
    ['POST /api/v1/projects/p1/worktrees/w1/terminals', () => createJsonResponse({ item: { id: 't1', wsPath: '/api/v1/terminal/ws?sessionId=t1', wsUrl: '/api/v1/terminal/ws?sessionId=t1', title: 'demo', projectId: 'p1', worktreeId: 'w1', workingDir: 'D:/repo/demo' } }, 201)],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.startWorkflow({
    path: 'D:/repo/demo',
    agent: 'claude',
    claudeRuntime: 'ccr',
    prompt: 'Inspect',
  });

  assert.equal(result.claudeRuntime, 'ccr');
  assert.equal(FakeWebSocket.instances.length, 1);
  assert.match(FakeWebSocket.instances[0].sent[0].data, /^ccr code\r$/);
  assert.match(FakeWebSocket.instances[0].sent[1].data, /^Inspect\r$/);
});

test('listSessions returns terminal and ai summaries', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: 'D:/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/terminals', () => createJsonResponse({ items: [{ id: 't1' }] })],
    ['GET /api/v1/projects/p1/ai-sessions', () => createJsonResponse({ item: { hasCodex: true, hasClaudeCode: false, codexSessions: [{ id: 'a1' }], claudeSessions: [] } })],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.listSessions({ projectId: 'p1' });
  assert.equal(result.project.id, 'p1');
  assert.equal(result.terminalSessions.length, 1);
  assert.equal(result.aiSessions.codexSessions.length, 1);
});


test('project file helpers call the file manager endpoints', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1/files/scopes', () => createWrappedJsonResponse({ items: [{ id: 'scope-main', rootPath: '/repo/demo' }] })],
    ['GET /api/v1/projects/p1/files/content', ({ url }) => {
      assert.equal(url.searchParams.get('scopeId'), 'scope-main');
      assert.equal(url.searchParams.get('path'), 'notes/123.md');
      return createTextResponse('# hello');
    }],
    ['POST /api/v1/projects/p1/files/delete', ({ body }) => {
      assert.deepEqual(body, { scopeId: 'scope-main', paths: ['notes/123.md'] });
      return createWrappedJsonResponse({ item: { succeeded: [{ path: 'notes/123.md', name: '123.md' }], failed: [] } });
    }],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const scopes = await client.listProjectFileScopes({ projectId: 'p1' });
  assert.equal(scopes[0].id, 'scope-main');

  const file = await client.readProjectFileText({
    projectId: 'p1',
    scopeId: 'scope-main',
    filePath: 'notes/123.md',
  });
  assert.equal(file.text, '# hello');

  const result = await client.deleteProjectFiles({
    projectId: 'p1',
    scopeId: 'scope-main',
    paths: ['notes/123.md'],
  });
  assert.equal(result.succeeded[0].name, '123.md');
});

test('websocket helpers receive configured websocket headers', async () => {
  FakeWebSocket.instances.length = 0;
  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(new Map()),
    WebSocketImpl: FakeWebSocket,
    webSocketOptions: {
      headers: {
        Authorization: 'Bearer token-123',
      },
    },
  });

  const channel = client.openWebSessionCommandChannel();
  await channel.waitForOpen();
  assert.equal(FakeWebSocket.instances[0].options.headers.Authorization, 'Bearer token-123');
  channel.close();
});


test('resolveProject supports projectName disambiguation with projectIndex', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects', () => createJsonResponse({
      items: [
        { id: 'p1', path: '/repo/alpha', name: 'demo' },
        { id: 'p2', path: '/repo/beta', name: 'demo' },
      ],
    })],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.resolveProject({
    projectName: 'demo',
    projectIndex: 2,
    ensureProject: false,
  });

  assert.equal(result.project.id, 'p2');
  assert.equal(result.matchedBy, 'projectName');
});

test('normalizeFsPath keeps absolute POSIX and Windows paths stable', () => {
  assert.equal(normalizeFsPath('/home/dev/test1'), '/home/dev/test1');
  assert.equal(normalizeFsPath('C:/Repo/../Demo'), ['c:', 'demo'].join('\\'));
});
