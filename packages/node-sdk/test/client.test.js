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

test('project Pi trust helpers use server-owned project context', async () => {
  const requests = [];
  const status = {
    projectId: 'p1',
    agent: 'pi',
    projectPath: 'D:/repo/demo',
    trusted: true,
  };
  const handlers = new Map([
    ['GET /api/v1/projects/p1/agent-trust/pi', ({ body }) => {
      requests.push({ method: 'GET', body });
      return createJsonResponse({ item: { ...status, trusted: false } });
    }],
    ['POST /api/v1/projects/p1/agent-trust/pi', ({ body }) => {
      requests.push({ method: 'POST', body });
      return createJsonResponse({ item: status });
    }],
    ['DELETE /api/v1/projects/p1/agent-trust/pi', ({ body }) => {
      requests.push({ method: 'DELETE', body });
      return createJsonResponse({ item: { ...status, trusted: false } });
    }],
  ]);
  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  assert.equal((await client.getProjectPiTrust({ projectId: 'p1' })).trusted, false);
  assert.equal((await client.trustProjectForPi({ projectId: 'p1' })).trusted, true);
  assert.equal((await client.revokeProjectPiTrust({ projectId: 'p1' })).trusted, false);
  assert.deepEqual(requests, [
    { method: 'GET', body: undefined },
    { method: 'POST', body: undefined },
    { method: 'DELETE', body: undefined },
  ]);
});

test('Pi Web Session import sends agent identity without accepting a native file path', async () => {
  const requests = [];
  const handlers = new Map([
    ['GET /api/v1/projects/p1/web-sessions/import-sources', ({ url }) => {
      requests.push({ method: 'GET', search: url.search });
      return createJsonResponse({ item: { items: [{ agent: 'pi', sessionId: 'pi-native' }], scanPhase: 'complete' } });
    }],
    ['POST /api/v1/projects/p1/web-sessions/import', ({ body }) => {
      requests.push({ method: 'POST', body });
      return createJsonResponse({ item: { created: true, session: { id: 'ws1', agent: 'pi' } } });
    }],
  ]);
  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const sources = await client.listWebSessionImportSources({ projectId: 'p1', refresh: true });
  const imported = await client.importWebSession({ projectId: 'p1', agent: 'pi', sessionId: 'pi-native' });
  assert.equal(sources.items[0].agent, 'pi');
  assert.equal(imported.session.agent, 'pi');
  assert.deepEqual(requests, [
    { method: 'GET', search: '?refresh=true' },
    { method: 'POST', body: { agent: 'pi', sessionId: 'pi-native' } },
  ]);
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
    ['GET /api/v1/projects/p1/ai-sessions', () => createJsonResponse({ item: { hasCodex: true, hasClaudeCode: false, hasPi: true, codexSessions: [{ id: 'a1' }], claudeSessions: [], piSessions: [{ id: 'p1' }] } })],
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
  assert.equal(result.aiSessions.piSessions.length, 1);
});


test('Pi web session tree REST helpers use revisioned contracts', async () => {
  const tree = {
    sessionId: 'ws1',
    leafId: 'a1',
    revision: 'rev-1',
    nodes: [
      { id: 'u1', parentId: null, type: 'message', role: 'user', preview: 'Start', active: true, children: ['a1'] },
      { id: 'a1', parentId: 'u1', type: 'message', role: 'assistant', preview: 'Answer', active: true, children: [] },
    ],
  };
  const handlers = new Map([
    ['GET /api/v1/projects/p1/web-sessions/ws1/tree', () => createWrappedJsonResponse({ item: tree })],
    ['POST /api/v1/projects/p1/web-sessions/ws1/tree/navigate', ({ body }) => {
      assert.deepEqual(body, { targetId: 'u1', revision: 'rev-1', summarize: true });
      return createWrappedJsonResponse({ item: { tree, editorText: 'Start' } });
    }],
    ['POST /api/v1/projects/p1/web-sessions/ws1/tree/fork', ({ body }) => {
      assert.deepEqual(body, { targetId: 'u1', revision: 'rev-1' });
      return createWrappedJsonResponse({ item: { session: { id: 'ws2', agent: 'pi' }, tree: { ...tree, sessionId: 'ws2' }, editorText: 'Start' } }, 201);
    }],
    ['POST /api/v1/projects/p1/web-sessions/ws1/tree/clone', ({ body }) => {
      assert.deepEqual(body, { revision: 'rev-1' });
      return createWrappedJsonResponse({ item: { session: { id: 'ws3', agent: 'pi' }, tree: { ...tree, sessionId: 'ws3' } } }, 201);
    }],
  ]);
  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const read = await client.getWebSessionTree({ projectId: 'p1', sessionId: 'ws1' });
  assert.equal(read.nodes.length, 2);
  const navigated = await client.navigateWebSessionTree({
    projectId: 'p1', sessionId: 'ws1', targetId: 'u1', revision: 'rev-1', summarize: true,
  });
  assert.equal(navigated.editorText, 'Start');
  const forked = await client.forkWebSessionTree({
    projectId: 'p1', sessionId: 'ws1', targetId: 'u1', revision: 'rev-1',
  });
  assert.equal(forked.session.id, 'ws2');
  assert.equal(forked.tree.sessionId, 'ws2');
  const cloned = await client.cloneWebSessionTree({
    projectId: 'p1', sessionId: 'ws1', revision: 'rev-1',
  });
  assert.equal(cloned.session.id, 'ws3');
  assert.equal(cloned.editorText, '');
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
