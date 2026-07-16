import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

import { CodeKanbanClient } from '../src/client.js';
import { createFetchMock, createJsonResponse, FakeWebSocket } from './helpers.js';

function createWebSessionSnapshot({
  session = {},
  items = [],
  history = {},
  pendingUserInput = null,
} = {}) {
  return {
    session: {
      id: 'ws1',
      projectId: 'p1',
      worktreeId: 'w1',
      orderIndex: 1000,
      agent: 'codex',
      title: 'Session 1',
      model: 'gpt-5.5',
      reasoningEffort: 'high',
      workflowMode: 'plan',
      permissionLevel: 'elevated',
      cwd: '/repo/demo',
      nativeSessionId: 'native-1',
      status: 'idle',
      assistantState: null,
      hasUnread: false,
      archivedAt: null,
      activityAt: '2026-04-10T00:00:00Z',
      lastMessageAt: '2026-04-10T00:00:00Z',
      assistantStateUpdatedAt: null,
      sourceKind: 'codex_app_server',
      syncState: 'fresh',
      lastSyncMode: 'fast',
      sourceCreatedAt: null,
      sourceUpdatedAt: null,
      lastSyncedAt: null,
      threadPath: null,
      threadPreview: null,
      turnCount: 0,
      itemCount: items.length,
      syncError: null,
      createdAt: '2026-04-10T00:00:00Z',
      updatedAt: '2026-04-10T00:00:00Z',
      usage: {
        inputTokens: 0,
        cachedInputTokens: 0,
        outputTokens: 0,
        cost: 0,
      },
      contextWindowTokens: 1000000,
      contextWindowSource: 'config',
      ...session,
    },
    history: {
      items,
      hasMore: false,
      beforeCursor: null,
      total: items.length,
      ...history,
    },
    pendingUserInput,
  };
}

function withAckingWebSocket(assertions) {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
    const originalSend = socket.send.bind(socket);
    socket.send = payload => {
      originalSend(payload);
      const frame = JSON.parse(payload);
      assertions?.(frame, socket);
      queueMicrotask(() => {
        socket.emitJson({
          v: 1,
          k: 'ack',
          rid: frame.rid,
          sid: frame.sid,
          ts: 1710000001234,
          op: frame.op,
          ok: 1,
        });
      });
    };
  });
}

test('CodeKanbanClient web session HTTP methods call the expected endpoints', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/worktrees', () =>
      createJsonResponse({ items: [{ id: 'w-main', projectId: 'p1', isMain: true, path: '/repo/demo' }] })],
    ['GET /api/v1/projects/p1/web-sessions', () =>
      createJsonResponse({ items: [{ id: 'ws1', projectId: 'p1', title: 'Session 1' }] })],
    ['POST /api/v1/projects/p1/web-sessions', ({ body }) => {
      assert.deepEqual(body, {
        worktreeId: 'w-main',
        agent: 'codex',
        claudeRuntime: 'claude',
        model: 'gpt-5.5',
        reasoningEffort: 'xhigh',
        workflowMode: 'plan',
        permissionLevel: 'elevated',
        autoRetryEnabled: false,
        autoRetryScope: 'network_only',
        autoRetryPreset: 'gentle_stop',
        permissionMode: '',
        title: '',
      });
      return createJsonResponse({ item: { id: 'ws2', projectId: 'p1', title: 'Created' } }, 201);
    }],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', ({ url }) => {
      assert.equal(url.searchParams.get('limit'), '30');
      return createJsonResponse({
        item: {
          session: { id: 'ws1', projectId: 'p1' },
          history: { items: [], hasMore: false, total: 0 },
        },
      });
    }],
    ['GET /api/v1/projects/p1/web-sessions/ws1/history', ({ url }) => {
      assert.equal(url.searchParams.get('beforeCursor'), '99');
      assert.equal(url.searchParams.get('limit'), '20');
      return createJsonResponse({
        item: {
          items: [{ id: 'hist-1' }],
          hasMore: true,
          beforeCursor: '77',
          total: 10,
        },
      });
    }],
    ['POST /api/v1/projects/p1/web-sessions/ws1/sync', ({ body }) => {
      assert.deepEqual(body, { mode: 'deep', clearExisting: true });
      return createJsonResponse({
        item: {
          session: { id: 'ws1', projectId: 'p1' },
          history: { items: [], hasMore: false, total: 0 },
        },
      });
    }],
    ['POST /api/v1/projects/p1/web-sessions/ws1/archive', () =>
      createJsonResponse({ item: { id: 'ws1', projectId: 'p1', archivedAt: '2026-04-10T00:00:00Z' } })],
    ['POST /api/v1/projects/p1/web-sessions/ws1/unarchive', () =>
      createJsonResponse({ item: { id: 'ws1', projectId: 'p1', archivedAt: null } })],
    ['POST /api/v1/projects/p1/web-sessions/ws1/rename', ({ body }) => {
      assert.deepEqual(body, { title: 'Renamed' });
      return createJsonResponse({ item: { id: 'ws1', projectId: 'p1', title: 'Renamed' } });
    }],
    ['POST /api/v1/projects/p1/web-sessions/ws1/close', () => createJsonResponse({ body: { message: 'session aborted' } })],
    ['DELETE /api/v1/projects/p1/web-sessions/ws1', () => createJsonResponse({ body: { message: 'session deleted' } })],
    ['POST /api/v1/web-sessions/archived/query', ({ body }) => {
      assert.deepEqual(body, { projectIds: ['p1'], offset: 10, limit: 5 });
      return createJsonResponse({
        item: {
          items: [{ id: 'arch-1' }],
          total: 12,
          hasMore: true,
          nextOffset: 15,
        },
      });
    }],
    ['POST /api/v1/web-sessions/search', ({ body }) => {
      assert.deepEqual(body, {
        projectIds: ['p1'],
        query: 'needle',
        includeArchived: true,
        includeBody: true,
        cursor: 'cursor-1',
        scanLimit: 50,
      });
      return createJsonResponse({
        item: {
          items: [{ id: 'ws-search-1' }],
          nextCursor: 'cursor-2',
          done: false,
          scanned: 50,
          total: 120,
        },
      });
    }],
    ['GET /api/v1/projects/p1/web-sessions/ws1/command-groups/group-1', () =>
      createJsonResponse({ item: { groupId: 'group-1', count: 2 } })],
    ['GET /api/v1/web-sessions/runtime-config', () =>
      createJsonResponse({
        item: {
          contextWindowTokens: 200000,
          compactLimitTokens: 200000,
          source: 'default',
          hasCodex: true,
          hasClaudeCode: false,
          codexVersion: '0.133.0',
          supportsGoalMode: true,
          goalModeMinCodexVersion: '0.133.0',
        },
      })],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const sessions = await client.listWebSessions({ projectId: 'p1' });
  assert.equal(sessions.length, 1);

  const created = await client.createWebSession({
    projectId: 'p1',
    agent: 'codex',
    workflowMode: 'plan',
    permissionLevel: 'elevated',
  });
  assert.equal(created.id, 'ws2');

  const snapshot = await client.getWebSessionSnapshot({
    projectId: 'p1',
    sessionId: 'ws1',
    limit: 30,
  });
  assert.equal(snapshot.session.id, 'ws1');

  const history = await client.getWebSessionHistory({
    projectId: 'p1',
    sessionId: 'ws1',
    beforeCursor: '99',
    limit: 20,
  });
  assert.equal(history.beforeCursor, '77');

  const synced = await client.syncWebSession({
    projectId: 'p1',
    sessionId: 'ws1',
    mode: 'deep',
    clearExisting: true,
  });
  assert.equal(synced.session.id, 'ws1');

  const archived = await client.archiveWebSession({ projectId: 'p1', sessionId: 'ws1' });
  assert.equal(archived.id, 'ws1');

  const unarchived = await client.unarchiveWebSession({ projectId: 'p1', sessionId: 'ws1' });
  assert.equal(unarchived.archivedAt, null);

  const renamed = await client.renameWebSession({
    projectId: 'p1',
    sessionId: 'ws1',
    title: 'Renamed',
  });
  assert.equal(renamed.title, 'Renamed');

  const closed = await client.closeWebSession({ projectId: 'p1', sessionId: 'ws1' });
  assert.equal(closed.message, 'session aborted');

  const deleted = await client.deleteWebSession({ projectId: 'p1', sessionId: 'ws1' });
  assert.equal(deleted.message, 'session deleted');

  const archivedQuery = await client.queryArchivedWebSessions({
    projectIds: ['p1'],
    offset: 10,
    limit: 5,
  });
  assert.equal(archivedQuery.nextOffset, 15);

  const searchResult = await client.searchWebSessions({
    projectIds: ['p1'],
    query: 'needle',
    includeArchived: true,
    cursor: 'cursor-1',
  });
  assert.equal(searchResult.nextCursor, 'cursor-2');
  assert.equal(searchResult.items[0].id, 'ws-search-1');

  const commandGroup = await client.getWebSessionCommandGroup({
    projectId: 'p1',
    sessionId: 'ws1',
    groupId: 'group-1',
  });
  assert.equal(commandGroup.groupId, 'group-1');

  const runtimeConfig = await client.getWebSessionRuntimeConfig();
  assert.equal(runtimeConfig.contextWindowTokens, 200000);
  assert.equal(runtimeConfig.hasCodex, true);
  assert.equal(runtimeConfig.codexVersion, '0.133.0');
  assert.equal(runtimeConfig.supportsGoalMode, true);
});



test('CodeKanbanClient createWebSession resolves projectName for remote-friendly targeting', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects', () =>
      createJsonResponse({
        items: [{ id: 'p1', path: '/repo/demo', name: 'demo' }],
      })],
    ['GET /api/v1/projects/p1/worktrees', () =>
      createJsonResponse({
        items: [{ id: 'w-main', projectId: 'p1', isMain: true, path: '/repo/demo' }],
      })],
    ['POST /api/v1/projects/p1/web-sessions', () =>
      createJsonResponse({ item: { id: 'ws-created', projectId: 'p1', worktreeId: 'w-main' } }, 201)],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const created = await client.createWebSession({
    projectName: 'demo',
    agent: 'codex',
    workflowMode: 'plan',
    permissionLevel: 'elevated',
  });

  assert.equal(created.projectId, 'p1');
  assert.equal(created.worktreeId, 'w-main');
});

test('CodeKanbanClient createWebSession auto-selects main worktree and required defaults without fetching project metadata', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1/worktrees', () =>
      createJsonResponse({
        items: [
          { id: 'w-side', projectId: 'p1', isMain: false, path: '/repo/demo-side' },
          { id: 'w-main', projectId: 'p1', isMain: true, path: '/repo/demo' },
        ],
      })],
    ['POST /api/v1/projects/p1/web-sessions', ({ body }) => {
      assert.deepEqual(body, {
        worktreeId: 'w-main',
        agent: 'codex',
        claudeRuntime: 'claude',
        model: 'gpt-5.5',
        reasoningEffort: 'xhigh',
        workflowMode: 'default',
        permissionLevel: 'elevated',
        autoRetryEnabled: false,
        autoRetryScope: 'network_only',
        autoRetryPreset: 'gentle_stop',
        permissionMode: '',
        title: '',
      });
      return createJsonResponse({ item: { id: 'ws-created', projectId: 'p1', worktreeId: 'w-main' } }, 201);
    }],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const created = await client.createWebSession({
    projectId: 'p1',
    agent: 'codex',
  });

  assert.equal(created.id, 'ws-created');
  assert.equal(created.worktreeId, 'w-main');
});

test('CodeKanbanClient getWebSessionState uses projectId directly for polling reads', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () =>
      createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'done',
            assistantState: null,
          },
        }),
      })],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const state = await client.getWebSessionState({
    projectId: 'p1',
    sessionId: 'ws1',
  });

  assert.equal(state.phase, 'done');
});

test('CodeKanbanClient uploadWebSessionAttachment sends multipart image data with inferred mime type', async t => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), 'codekanban-sdk-'));
  t.after(async () => {
    await rm(tempDir, { recursive: true, force: true });
  });

  const filePath = path.join(tempDir, 'image.png');
  await writeFile(filePath, Buffer.from([0x89, 0x50, 0x4e, 0x47]));

  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['POST /api/v1/projects/p1/web-sessions/attachments', ({ body, headers }) => {
      assert.equal(headers.Accept, 'application/json');
      const file = body.get('file');
      assert.ok(file);
      assert.equal(file.name, 'image.png');
      assert.equal(file.type, 'image/png');
      return createJsonResponse({
        body: {
          item: {
            id: 'att-1',
            name: 'image.png',
            mime: 'image/png',
            size: 4,
            path: '/tmp/image.png',
            createdAt: '2026-04-10T00:00:00Z',
          },
        },
      }, 201);
    }],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const attachment = await client.uploadWebSessionAttachment({
    projectId: 'p1',
    filePath,
  });

  assert.equal(attachment.id, 'att-1');
  assert.equal(attachment.mime, 'image/png');
});

test('CodeKanbanClient opens web session websocket helpers with the expected URLs', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: async () => createJsonResponse({}),
    WebSocketImpl: FakeWebSocket,
  });

  const commandChannel = client.openWebSessionCommandChannel();
  await commandChannel.waitForOpen();
  const eventStream = client.openWebSessionEventStream({ sessionId: 'ws1' });
  await eventStream.waitForOpen();

  assert.equal(FakeWebSocket.instances[0].url, 'ws://127.0.0.1:3000/api/v1/web-sessions/ws');
  assert.equal(FakeWebSocket.instances[1].url, 'ws://127.0.0.1:3000/api/v1/web-sessions/events');

  commandChannel.close();
  eventStream.close();
});

test('CodeKanbanClient exposes goal helpers through the web session command channel', async () => {
  withAckingWebSocket((frame) => {
    assert.equal(frame.sid, 'ws-goal');
  });

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: async () => createJsonResponse({}),
    WebSocketImpl: FakeWebSocket,
  });

  await client.setWebSessionGoal({
    sessionId: 'ws-goal',
    objective: 'Ship the feature',
    status: 'active',
  });
  assert.equal(FakeWebSocket.instances[0].sent[0]?.op, 'goal_set');

  await client.pauseWebSessionGoal({ sessionId: 'ws-goal' });
  assert.equal(FakeWebSocket.instances[1].sent[0]?.op, 'goal_pause');

  await client.resumeWebSessionGoal({ sessionId: 'ws-goal' });
  assert.equal(FakeWebSocket.instances[2].sent[0]?.op, 'goal_resume');

  await client.clearWebSessionGoal({ sessionId: 'ws-goal' });
  assert.equal(FakeWebSocket.instances[3].sent[0]?.op, 'goal_clear');
});

test('CodeKanbanClient analyzeWebSession derives actionable polling state from snapshot', async () => {
  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: async () => createJsonResponse({}),
    WebSocketImpl: FakeWebSocket,
  });

  const state = client.analyzeWebSession(
    createWebSessionSnapshot({
      session: {
        status: 'running',
        assistantState: 'waiting_plan_approval',
        workflowMode: 'plan',
      },
      items: [
        {
          id: 'user-1',
          orderIndex: 1,
          kind: 'user',
          itemType: 'user_message',
          text: 'plan this',
          timestamp: '2026-04-10T00:00:01Z',
          observedAt: '2026-04-10T00:00:01Z',
          payload: {},
        },
        {
          id: 'plan-1',
          orderIndex: 2,
          kind: 'tool',
          itemType: 'plan',
          text: '',
          timestamp: '2026-04-10T00:00:02Z',
          observedAt: '2026-04-10T00:00:02Z',
          tool: {
            id: 'plan-tool-1',
            name: 'Plan',
            kind: 'plan',
            output: '# Plan',
            status: 'done',
            meta: { title: 'Plan' },
          },
          payload: {},
        },
      ],
    }),
  );

  assert.equal(state.phase, 'waiting_plan_approval');
  assert.equal(state.canSend, true);
  assert.equal(state.needsAction, true);
  assert.equal(state.nextAction.type, 'execute_plan');
  assert.equal(state.latestPlan.toolId, 'plan-tool-1');
});

test('CodeKanbanClient getWebSessionState identifies pending user input for polling clients', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () =>
      createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'running',
            assistantState: 'waiting_input',
          },
          items: [
            {
              id: 'req-1',
              sourceItemId: 'call_1',
              orderIndex: 1,
              kind: 'system',
              itemType: 'user_input_request',
              text: 'Pick one',
              timestamp: '2026-04-10T00:00:03Z',
              observedAt: '2026-04-10T00:00:03Z',
              detail: {
                type: 'user_input_request',
                prompt: 'Pick one',
                questions: [
                  {
                    id: 'scope',
                    header: 'Scope',
                    question: 'Pick one',
                    isOther: false,
                    isSecret: false,
                    options: [{ label: 'A', description: 'option A' }],
                  },
                ],
              },
              payload: { iid: 'call_1' },
            },
          ],
        }),
      })],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const state = await client.getWebSessionState({
    projectId: 'p1',
    sessionId: 'ws1',
  });

  assert.equal(state.phase, 'waiting_input');
  assert.equal(state.needsAction, true);
  assert.equal(state.nextAction.type, 'answer_user_input');
  assert.equal(state.pendingUserInput.itemId, 'call_1');
  assert.equal(state.canSend, false);
});

test('CodeKanbanClient getWebSessionState prefers snapshot.pendingUserInput when provided', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () =>
      createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'running',
            assistantState: 'waiting_input',
          },
          pendingUserInput: {
            itemId: 'call_snapshot',
            prompt: 'Pick one',
            questions: [
              {
                id: 'scope',
                header: 'Scope',
                question: 'Pick one',
                isOther: false,
                isSecret: false,
                options: [{ label: 'A', description: 'option A' }],
              },
            ],
            requestedAt: '2026-04-10T00:00:03Z',
          },
        }),
      })],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const state = await client.getWebSessionState({
    projectId: 'p1',
    sessionId: 'ws1',
  });

  assert.equal(state.pendingUserInput.itemId, 'call_snapshot');
  assert.equal(state.nextAction.type, 'answer_user_input');
});

test('CodeKanbanClient answerPendingUserInput uses the active itemId from snapshot analysis', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () =>
      createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'running',
            assistantState: 'waiting_input',
          },
          items: [
            {
              id: 'req-1',
              sourceItemId: 'call_42',
              orderIndex: 1,
              kind: 'system',
              itemType: 'user_input_request',
              text: 'Pick one',
              timestamp: '2026-04-10T00:00:03Z',
              observedAt: '2026-04-10T00:00:03Z',
              detail: {
                type: 'user_input_request',
                prompt: 'Pick one',
                questions: [],
              },
              payload: { iid: 'call_42' },
            },
          ],
        }),
      })],
  ]);

  withAckingWebSocket(frame => {
    assert.equal(frame.op, 'user_input');
    assert.equal(frame.sid, 'ws1');
    assert.deepEqual(frame.p, {
      iid: 'call_42',
      ans: { scope: ['full'] },
    });
  });

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.answerPendingUserInput({
    projectId: 'p1',
    sessionId: 'ws1',
    answers: { scope: ['full'] },
  });

  assert.equal(result.itemId, 'call_42');
  assert.equal(result.ack.operation, 'user_input');
});

test('CodeKanbanClient executeLatestPlan answers a plan-choice user input automatically', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () =>
      createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'running',
            assistantState: 'waiting_input',
            workflowMode: 'plan',
          },
          items: [
            {
              id: 'plan-1',
              orderIndex: 1,
              kind: 'tool',
              itemType: 'plan',
              text: '',
              timestamp: '2026-04-10T00:00:04Z',
              observedAt: '2026-04-10T00:00:04Z',
              tool: {
                id: 'plan-tool-1',
                name: 'Plan',
                kind: 'plan',
                output: '# Plan',
                status: 'done',
                meta: { title: 'Plan' },
              },
              payload: {},
            },
            {
              id: 'req-plan-choice',
              sourceItemId: 'call_plan_choice',
              orderIndex: 2,
              kind: 'system',
              itemType: 'user_input_request',
              text: 'What next?',
              timestamp: '2026-04-10T00:00:05Z',
              observedAt: '2026-04-10T00:00:05Z',
              detail: {
                type: 'user_input_request',
                prompt: 'What next?',
                questions: [
                  {
                    id: 'action',
                    header: 'Action',
                    question: 'What next?',
                    isOther: false,
                    isSecret: false,
                    options: [
                      { label: 'Execute plan', description: 'Start executing the plan now.' },
                      { label: 'Stay in plan', description: 'Keep planning for later.' },
                    ],
                  },
                ],
              },
              payload: { iid: 'call_plan_choice' },
            },
          ],
        }),
      })],
  ]);

  const seenOps = [];
  withAckingWebSocket(frame => {
    seenOps.push(frame.op);
    if (frame.op === 'set_wm') {
      assert.deepEqual(frame.p, { wm: 'default' });
    }
    if (frame.op === 'user_input') {
      assert.deepEqual(frame.p, {
        iid: 'call_plan_choice',
        ans: {
          action: ['Execute plan'],
        },
      });
    }
  });

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.executeLatestPlan({
    projectId: 'p1',
    sessionId: 'ws1',
  });

  assert.deepEqual(seenOps, ['set_wm', 'user_input']);
  assert.equal(result.mode, 'plan_choice');
});

test('CodeKanbanClient executeLatestPlan sends a follow-up implementation message when no plan-choice prompt exists', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () =>
      createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'running',
            assistantState: 'waiting_plan_approval',
            workflowMode: 'plan',
          },
          items: [
            {
              id: 'plan-1',
              orderIndex: 1,
              kind: 'tool',
              itemType: 'plan',
              text: '',
              timestamp: '2026-04-10T00:00:04Z',
              observedAt: '2026-04-10T00:00:04Z',
              tool: {
                id: 'plan-tool-1',
                name: 'Plan',
                kind: 'plan',
                output: '# Plan',
                status: 'done',
                meta: { title: 'Plan' },
              },
              payload: {},
            },
          ],
        }),
      })],
  ]);

  const seenOps = [];
  withAckingWebSocket(frame => {
    seenOps.push(frame.op);
    if (frame.op === 'send') {
      assert.deepEqual(frame.p, {
        txt: 'Implement the plan.',
        atts: [],
      });
    }
  });

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.executeLatestPlan({
    projectId: 'p1',
    sessionId: 'ws1',
  });

  assert.deepEqual(seenOps, ['set_wm', 'send']);
  assert.equal(result.mode, 'followup_message');
});

test('CodeKanbanClient waitForWebSessionState works with simple polling clients', async () => {
  let snapshotReads = 0;
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () => {
      snapshotReads += 1;
      return createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: snapshotReads >= 2 ? 'done' : 'running',
            assistantState: snapshotReads >= 2 ? null : 'working',
          },
        }),
      });
    }],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const state = await client.waitForWebSessionState({
    projectId: 'p1',
    sessionId: 'ws1',
    until: 'done',
    intervalMs: 1,
    timeoutMs: 1000,
  });

  assert.equal(state.phase, 'done');
  assert.equal(snapshotReads, 2);
});

test('CodeKanbanClient waitForWebSessionState tolerates transient fetch failures', async () => {
  let snapshotReads = 0;

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    requestRetry: { attempts: 1 },
    fetchImpl: async (input, init = {}) => {
      const url = input instanceof URL ? input : new URL(String(input));
      const method = (init.method || 'GET').toUpperCase();
      assert.equal(method, 'GET');
      assert.equal(url.pathname, '/api/v1/projects/p1/web-sessions/ws1/snapshot');
      snapshotReads += 1;
      if (snapshotReads <= 2) {
        throw new TypeError('fetch failed');
      }
      return createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: snapshotReads >= 4 ? 'done' : 'running',
            assistantState: snapshotReads >= 4 ? null : 'working',
          },
        }),
      });
    },
    WebSocketImpl: FakeWebSocket,
  });

  const state = await client.waitForWebSessionState({
    projectId: 'p1',
    sessionId: 'ws1',
    until: 'done',
    intervalMs: 1,
    timeoutMs: 1000,
  });

  assert.equal(state.phase, 'done');
  assert.equal(snapshotReads, 4);
});


test('CodeKanbanClient waitForWebSessionState settleMs requires a stable match window', async () => {
  let snapshotReads = 0;
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () => {
      snapshotReads += 1;
      return createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: snapshotReads === 1 || snapshotReads >= 3 ? 'done' : 'running',
            assistantState: snapshotReads === 2 ? 'working' : null,
          },
        }),
      });
    }],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const state = await client.waitForWebSessionState({
    projectId: 'p1',
    sessionId: 'ws1',
    until: 'done',
    intervalMs: 5,
    settleMs: 8,
    timeoutMs: 1000,
  });

  assert.equal(state.phase, 'done');
  assert.equal(snapshotReads, 5);
});

test('CodeKanbanClient waitForWebSessionPause stops on actionable states without assuming user input always exists', async () => {
  let snapshotReads = 0;
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () => {
      snapshotReads += 1;
      if (snapshotReads === 1) {
        return createJsonResponse({
          item: createWebSessionSnapshot({
            session: {
              status: 'done',
              assistantState: null,
            },
          }),
        });
      }
      if (snapshotReads === 2) {
        return createJsonResponse({
          item: createWebSessionSnapshot({
            session: {
              status: 'running',
              assistantState: 'working',
            },
          }),
        });
      }
      return createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'running',
            assistantState: 'waiting_input',
          },
          items: [
            {
              id: 'req-1',
              sourceItemId: 'call_99',
              orderIndex: 1,
              kind: 'system',
              itemType: 'user_input_request',
              text: 'Pick one',
              timestamp: '2026-04-10T00:00:03Z',
              observedAt: '2026-04-10T00:00:03Z',
              detail: {
                type: 'user_input_request',
                prompt: 'Pick one',
                questions: [
                  {
                    id: 'choice',
                    header: 'Choice',
                    question: 'Pick one',
                    isOther: false,
                    options: [{ label: 'A', description: 'first' }],
                  },
                ],
              },
              payload: { iid: 'call_99' },
            },
          ],
        }),
      });
    }],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const pause = await client.waitForWebSessionPause({
    projectId: 'p1',
    sessionId: 'ws1',
    intervalMs: 1,
    settleMs: 20,
    timeoutMs: 1000,
  });

  assert.equal(pause.reason, 'user_input');
  assert.equal(pause.state.pendingUserInput.itemId, 'call_99');
  assert.equal(snapshotReads, 3);
});

test('CodeKanbanClient runWebSessionUntilDone auto-answers user input and executes the latest plan', async () => {
  let snapshotReads = 0;
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () => {
      snapshotReads += 1;
      if (snapshotReads <= 2) {
        return createJsonResponse({
          item: createWebSessionSnapshot({
            session: {
              status: 'running',
              assistantState: 'waiting_input',
              workflowMode: 'plan',
            },
            items: [
              {
                id: 'req-1',
                sourceItemId: 'call_42',
                orderIndex: 1,
                kind: 'system',
                itemType: 'user_input_request',
                text: 'Pick one',
                timestamp: '2026-04-10T00:00:03Z',
                observedAt: '2026-04-10T00:00:03Z',
                detail: {
                  type: 'user_input_request',
                  prompt: 'Pick one',
                  questions: [
                    {
                      id: 'choice',
                      header: 'Choice',
                      question: 'Pick one',
                      isOther: false,
                      options: [
                        { label: 'A', description: 'first' },
                        { label: 'B', description: 'second' },
                      ],
                    },
                  ],
                },
                payload: { iid: 'call_42' },
              },
            ],
          }),
        });
      }
      if (snapshotReads <= 4) {
        return createJsonResponse({
          item: createWebSessionSnapshot({
            session: {
              status: 'running',
              assistantState: 'waiting_plan_approval',
              workflowMode: 'plan',
            },
            items: [
              {
                id: 'plan-1',
                orderIndex: 2,
                kind: 'tool',
                itemType: 'plan',
                text: '',
                timestamp: '2026-04-10T00:00:04Z',
                observedAt: '2026-04-10T00:00:04Z',
                tool: {
                  id: 'plan-tool-1',
                  name: 'Plan',
                  kind: 'plan',
                  output: '# Plan',
                  status: 'done',
                  meta: { title: 'Plan' },
                },
                payload: {},
              },
            ],
          }),
        });
      }
      return createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'done',
            assistantState: null,
            workflowMode: 'default',
          },
          items: [
            {
              id: 'assistant-1',
              orderIndex: 3,
              kind: 'assistant',
              itemType: 'message',
              text: 'Done.',
              timestamp: '2026-04-10T00:00:05Z',
              observedAt: '2026-04-10T00:00:05Z',
              payload: {},
            },
          ],
        }),
      });
    }],
  ]);

  const seenOps = [];
  withAckingWebSocket(frame => {
    seenOps.push(frame.op);
    if (frame.op === 'user_input') {
      assert.deepEqual(frame.p, {
        iid: 'call_42',
        ans: { choice: ['B'] },
      });
    }
    if (frame.op === 'set_wm') {
      assert.deepEqual(frame.p, { wm: 'default' });
    }
    if (frame.op === 'send') {
      assert.deepEqual(frame.p, {
        txt: 'Implement the plan.',
        atts: [],
      });
    }
  });

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.runWebSessionUntilDone({
    projectId: 'p1',
    sessionId: 'ws1',
    intervalMs: 1,
    timeoutMs: 1000,
    settleMs: 0,
  });

  assert.equal(result.stopReason, 'done');
  assert.equal(result.finalState.phase, 'done');
  assert.equal(result.actions.length, 2);
  assert.equal(result.actions[0].type, 'answer_user_input');
  assert.equal(result.actions[1].type, 'execute_plan');
  assert.equal(result.lastExecuteMode, 'followup_message');
  assert.deepEqual(seenOps, ['user_input', 'set_wm', 'send']);
});

test('CodeKanbanClient runWebSessionUntilDone stops on approvals for caller judgment', async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/web-sessions/ws1/snapshot', () =>
      createJsonResponse({
        item: createWebSessionSnapshot({
          session: {
            status: 'running',
            assistantState: 'waiting_approval',
          },
          items: [
            {
              id: 'approval-1',
              sourceItemId: 'approval-1',
              orderIndex: 1,
              kind: 'system',
              itemType: 'approval_request',
              text: 'Need approval',
              timestamp: '2026-04-10T00:00:03Z',
              observedAt: '2026-04-10T00:00:03Z',
              detail: {
                type: 'approval_request',
                prompt: 'Need approval',
                questions: [],
              },
              payload: {},
            },
          ],
        }),
      })],
  ]);

  const client = new CodeKanbanClient({
    baseURL: 'http://127.0.0.1:3000',
    fetchImpl: createFetchMock(handlers),
    WebSocketImpl: FakeWebSocket,
  });

  const result = await client.runWebSessionUntilDone({
    projectId: 'p1',
    sessionId: 'ws1',
    intervalMs: 1,
    timeoutMs: 1000,
    settleMs: 0,
  });

  assert.equal(result.stopReason, 'needs_approval');
  assert.equal(result.finalState.phase, 'waiting_approval');
});
