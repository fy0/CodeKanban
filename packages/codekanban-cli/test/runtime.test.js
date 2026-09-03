import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

import { runCli } from '../src/runtime.js';
import { createFetchMock, createJsonResponse, FakeWebSocket } from './helpers.js';

function createAckingWebSocket(assertSent) {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
    const originalSend = socket.send.bind(socket);
    socket.send = payload => {
      originalSend(payload);
      const frame = JSON.parse(payload);
      assertSent?.(frame);
      queueMicrotask(() => {
        socket.emitJson({
          v: 1,
          k: 'ack',
          rid: frame.rid,
          sid: frame.sid,
          ts: 1710000300000,
          op: frame.op,
          ok: 1,
        });
      });
    };
  });
}

async function runCliCaptured(argv, options = {}) {
  const stdout = [];
  const stderr = [];
  const originalFetch = globalThis.fetch;
  const originalWebSocket = globalThis.WebSocket;

  globalThis.fetch = options.fetchImpl || (async () => createJsonResponse({}));
  globalThis.WebSocket = options.WebSocketImpl || FakeWebSocket;

  try {
    const exitCode = await runCli(argv, {
      stdout: {
        write(chunk) {
          stdout.push(String(chunk));
          return true;
        },
      },
      stderr: {
        write(chunk) {
          stderr.push(String(chunk));
          return true;
        },
      },
      clientFactory: options.clientFactory,
      clientOptions: options.clientOptions,
      defaultBaseURL: options.defaultBaseURL,
    });
    return {
      exitCode,
      stdout: stdout.join(''),
      stderr: stderr.join(''),
    };
  } finally {
    globalThis.fetch = originalFetch;
    globalThis.WebSocket = originalWebSocket;
    FakeWebSocket.reset();
  }
}



test('CLI --help prints usage text', { concurrency: false }, async () => {
  const result = await runCliCaptured(['--help']);
  assert.equal(result.exitCode, 0);
  assert.match(result.stdout, /CodeKanban command runtime/);
  assert.match(result.stdout, /--image <path>/);
  assert.equal(result.stderr, '');
});

test('CLI runtime rejects removed Kanban scopes before creating an SDK client', { concurrency: false }, async () => {
  let clientCreated = false;
  const result = await runCliCaptured(['tasks', 'list'], {
    clientFactory: () => {
      clientCreated = true;
      return {};
    },
  });

  assert.equal(result.exitCode, 1);
  assert.match(result.stderr, /removed with the Kanban task API/);
  assert.equal(clientCreated, false);
});

test('CLI workflow command supports Claude Code Router runtime', { concurrency: false }, async () => {
  const result = await runCliCaptured([
    'workflow',
    'command',
    '--agent',
    'claude',
    '--claude-runtime',
    'ccr',
    '--extra-arg',
    '--model',
    '--extra-arg',
    'sonnet',
    '--prompt',
    'Hello',
  ]);

  assert.equal(result.exitCode, 0);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.agent, 'claude');
  assert.equal(payload.claudeRuntime, 'ccr');
  assert.equal(payload.command, 'ccr code --model sonnet');
  assert.deepEqual(payload.argv, ['ccr', 'code', '--model', 'sonnet']);
});

test('CLI workflow start forwards Claude runtime to the SDK client', { concurrency: false }, async () => {
  const calls = [];
  const result = await runCliCaptured([
    'workflow',
    'start',
    '--base-url',
    'http://127.0.0.1:3000',
    '--project-id',
    'p1',
    '--agent',
    'claude',
    '--claude-runtime',
    'ccr',
    '--prompt',
    'Hello',
  ], {
    clientFactory: () => ({
      async startWorkflow(input) {
        calls.push(input);
        return { command: 'ccr code', claudeRuntime: input.claudeRuntime };
      },
    }),
  });

  assert.equal(result.exitCode, 0);
  assert.equal(calls[0].claudeRuntime, 'ccr');
  assert.equal(JSON.parse(result.stdout).claudeRuntime, 'ccr');
});


test('CLI web-session create prints the created session JSON', { concurrency: false }, async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['GET /api/v1/projects/p1/worktrees', () =>
      createJsonResponse({ items: [{ id: 'w-main', projectId: 'p1', isMain: true, path: '/repo/demo' }] })],
    ['POST /api/v1/projects/p1/web-sessions', ({ body }) => {
      assert.equal(body.agent, 'codex');
      assert.equal(body.workflowMode, 'plan');
      assert.equal(body.worktreeId, 'w-main');
      assert.equal(Object.hasOwn(body, 'model'), false);
      assert.equal(Object.hasOwn(body, 'reasoningEffort'), false);
      assert.equal(Object.hasOwn(body, 'permissionLevel'), false);
      assert.equal(body.autoRetryEnabled, false);
      assert.equal(body.autoRetryPolicyMode, 'custom');
      assert.equal(body.autoRetryScope, 'all_failures');
      assert.equal(body.autoRetryPreset, 'sustain_60s');
      assert.equal(body.autoRetryMaxAttempts, 9);
      return createJsonResponse({ item: { id: 'ws-created', projectId: 'p1', title: 'Created' } }, 201);
    }],
  ]);

  const result = await runCliCaptured(
    [
      'web-session',
      'create',
      '--base-url',
      'http://127.0.0.1:3000',
      '--project-id',
      'p1',
      '--agent',
      'codex',
      '--workflow-mode',
      'plan',
      '--auto-retry-policy-mode',
      'custom',
      '--auto-retry-scope',
      'all_failures',
      '--auto-retry-preset',
      'sustain_60s',
      '--auto-retry-max-attempts',
      '9',
    ],
    { fetchImpl: createFetchMock(handlers) },
  );

  assert.equal(result.exitCode, 0);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.id, 'ws-created');
});

test('CLI web-session send sends a websocket command and prints the ack', { concurrency: false }, async () => {
  createAckingWebSocket(frame => {
    assert.equal(frame.op, 'send');
    assert.equal(frame.sid, 'ws1');
    assert.deepEqual(frame.p, {
      txt: 'continue',
      atts: ['att-1'],
    });
  });

  const result = await runCliCaptured([
    'web-session',
    'send',
    '--base-url',
    'http://127.0.0.1:3000',
    '--session-id',
    'ws1',
    '--text',
    'continue',
    '--attachment-id',
    'att-1',
  ]);

  assert.equal(result.exitCode, 0);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.type, 'ack');
  assert.equal(payload.operation, 'send');
});

test('CLI web-session send uploads repeated local images and merges attachment ids', { concurrency: false }, async t => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), 'codekanban-cli-'));
  t.after(async () => {
    await rm(tempDir, { recursive: true, force: true });
  });

  const firstImage = path.join(tempDir, 'first.png');
  const secondImage = path.join(tempDir, 'second.jpg');
  await writeFile(firstImage, Buffer.from([0x89, 0x50, 0x4e, 0x47]));
  await writeFile(secondImage, Buffer.from([0xff, 0xd8, 0xff, 0xd9]));

  const uploadedNames = [];
  const handlers = new Map([
    ['POST /api/v1/projects/p1/web-sessions/attachments', ({ body }) => {
      const file = body.get('file');
      uploadedNames.push(file.name);
      const sequence = uploadedNames.length;
      return createJsonResponse({
        item: {
          id: `att-${sequence}`,
          name: file.name,
          mime: file.type,
          size: file.size,
          path: `/tmp/${file.name}`,
          createdAt: '2026-04-10T00:00:00Z',
        },
      }, 201);
    }],
  ]);

  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
    const originalSend = socket.send.bind(socket);
    socket.send = payload => {
      originalSend(payload);
      const frame = JSON.parse(payload);
      if (frame.op === 'connect') {
        queueMicrotask(() => {
          socket.emitJson({
            v: 1,
            k: 'ack',
            rid: frame.rid,
            sid: frame.sid,
            ts: 1710000300000,
            op: frame.op,
            ok: 1,
          });
          socket.emitJson({
            v: 1,
            k: 'snap',
            rid: frame.rid,
            sid: frame.sid,
            ts: 1710000300000,
            s: { id: 'ws1', pid: 'p1', ag: 'codex' },
            h: { items: [] },
            pi: [],
          });
        });
        return;
      }
      assert.equal(frame.op, 'send');
      assert.deepEqual(frame.p, {
        txt: 'compare the references',
        atts: ['att-existing', 'att-1', 'att-2'],
      });
      queueMicrotask(() => {
        socket.emitJson({
          v: 1,
          k: 'ack',
          rid: frame.rid,
          sid: frame.sid,
          ts: 1710000300000,
          op: frame.op,
          ok: 1,
        });
      });
    };
  });

  const result = await runCliCaptured([
    'web-session',
    'send',
    '--base-url',
    'http://127.0.0.1:3000',
    '--session-id',
    'ws1',
    '--text',
    'compare the references',
    '--attachment-id',
    'att-existing',
    '--image',
    firstImage,
    '--image',
    secondImage,
  ], {
    fetchImpl: createFetchMock(handlers),
  });

  assert.equal(result.exitCode, 0);
  assert.deepEqual(uploadedNames, ['first.png', 'second.jpg']);
  assert.equal(JSON.parse(result.stdout).operation, 'send');
});

test('CLI web-session send does not send when an image upload fails', { concurrency: false }, async () => {
  let sent = false;
  let closed = false;
  const result = await runCliCaptured([
    'web-session',
    'send',
    '--base-url',
    'http://127.0.0.1:3000',
    '--session-id',
    'ws1',
    '--text',
    'continue',
    '--image',
    './missing.png',
  ], {
    clientFactory: () => ({
      openWebSessionCommandChannel() {
        return {
          async waitForOpen() {},
          async connect() {
            return { session: { id: 'ws1', projectId: 'p1' } };
          },
          async sendMessage() {
            sent = true;
          },
          close() {
            closed = true;
          },
        };
      },
      async uploadWebSessionAttachment() {
        throw new Error('image upload failed');
      },
    }),
  });

  assert.equal(result.exitCode, 1);
  assert.equal(sent, false);
  assert.equal(closed, true);
  assert.match(JSON.parse(result.stderr).error.message, /image upload failed/);
});

test('CLI web-session user-input parses answers JSON and prints the ack', { concurrency: false }, async () => {
  createAckingWebSocket(frame => {
    assert.equal(frame.op, 'user_input');
    assert.deepEqual(frame.p, {
      iid: 'item-1',
      ans: { choice: ['A'] },
    });
  });

  const result = await runCliCaptured([
    'web-session',
    'user-input',
    '--base-url',
    'http://127.0.0.1:3000',
    '--session-id',
    'ws1',
    '--item-id',
    'item-1',
    '--answers-json',
    '{"choice":["A"]}',
  ]);

  assert.equal(result.exitCode, 0);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.operation, 'user_input');
});

test('CLI web-session set-workflow sends the workflow update command', { concurrency: false }, async () => {
  createAckingWebSocket(frame => {
    assert.equal(frame.op, 'set_wm');
    assert.deepEqual(frame.p, { wm: 'plan' });
  });

  const result = await runCliCaptured([
    'web-session',
    'set-workflow',
    '--base-url',
    'http://127.0.0.1:3000',
    '--session-id',
    'ws1',
    '--workflow-mode',
    'plan',
  ]);

  assert.equal(result.exitCode, 0);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.operation, 'set_wm');
});

test('CLI web-session sync calls the HTTP sync endpoint', { concurrency: false }, async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['POST /api/v1/projects/p1/web-sessions/ws1/sync', ({ body }) => {
      assert.deepEqual(body, { mode: 'deep', clearExisting: true });
      return createJsonResponse({
        item: {
          session: { id: 'ws1', projectId: 'p1' },
          history: { items: [], hasMore: false, total: 0 },
        },
      });
    }],
  ]);

  const result = await runCliCaptured(
    [
      'web-session',
      'sync',
      '--base-url',
      'http://127.0.0.1:3000',
      '--project-id',
      'p1',
      '--session-id',
      'ws1',
      '--mode',
      'deep',
      '--clear-existing',
    ],
    { fetchImpl: createFetchMock(handlers) },
  );

  assert.equal(result.exitCode, 0);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.session.id, 'ws1');
});

test('CLI web-session attach uploads a multipart image file', { concurrency: false }, async t => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), 'codekanban-cli-'));
  t.after(async () => {
    await rm(tempDir, { recursive: true, force: true });
  });

  const filePath = path.join(tempDir, 'upload.png');
  await writeFile(filePath, Buffer.from([0x89, 0x50, 0x4e, 0x47]));

  const handlers = new Map([
    ['GET /api/v1/projects/p1', () => createJsonResponse({ item: { id: 'p1', path: '/repo/demo', name: 'demo' } })],
    ['POST /api/v1/projects/p1/web-sessions/attachments', ({ body }) => {
      const file = body.get('file');
      assert.equal(file.name, 'upload.png');
      assert.equal(file.type, 'image/png');
      return createJsonResponse({
        item: {
          id: 'att-1',
          name: 'upload.png',
          mime: 'image/png',
          size: 4,
          path: '/tmp/upload.png',
          createdAt: '2026-04-10T00:00:00Z',
        },
      }, 201);
    }],
  ]);

  const result = await runCliCaptured(
    [
      'web-session',
      'attach',
      '--base-url',
      'http://127.0.0.1:3000',
      '--project-id',
      'p1',
      '--file',
      filePath,
    ],
    { fetchImpl: createFetchMock(handlers) },
  );

  assert.equal(result.exitCode, 0);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.id, 'att-1');
});

test('CLI web-session watch streams one NDJSON frame when max-events is set to 1', { concurrency: false }, async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => {
      socket.open();
      socket.emitJson({
        v: 1,
        k: 'snap',
        sid: 'ws1',
        ts: 1710000400000,
        s: {
          id: 'ws1',
          pid: 'p1',
          oi: 1000,
          ag: 'codex',
          md: 'gpt-5',
          re: 'high',
          wm: 'plan',
          pl: 'elevated',
          ttl: 'watch',
          cwd: '/repo/demo',
          st: 'running',
          unr: false,
          act: 1710000400000,
          ca: 1710000400000,
          lu: 1710000400001,
          sk: 'codex_app_server',
          ss: 'fresh',
          usa: { in: 1, cin: 0, out: 1 },
          cost: 0.01,
          cws: 'default',
        },
        h: {
          its: [],
          hm: false,
          tot: 0,
        },
      });
    });
  });

  const result = await runCliCaptured([
    'web-session',
    'watch',
    '--base-url',
    'http://127.0.0.1:3000',
    '--session-id',
    'ws1',
    '--max-events',
    '1',
    '--raw',
  ]);

  assert.equal(result.exitCode, 0);
  const lines = result.stdout.trim().split('\n');
  assert.equal(lines.length, 1);
  const payload = JSON.parse(lines[0]);
  assert.equal(payload.k, 'snap');
  assert.equal(payload.sid, 'ws1');
});


test('CLI web-session answer-pending can auto-answer with the second option', { concurrency: false }, async () => {
  const seen = [];
  const result = await runCliCaptured([
    'web-session',
    'answer-pending',
    '--base-url',
    'http://127.0.0.1:3000',
    '--project-id',
    'p1',
    '--session-id',
    'ws1',
    '--answer-strategy',
    'prefer-second-or-text',
  ], {
    clientFactory: () => ({
      async getWebSessionState() {
        return {
          pendingUserInput: {
            itemId: 'call-1',
            prompt: 'Pick one',
            questions: [
              {
                id: 'choice',
                options: [
                  { label: 'A', description: 'first' },
                  { label: 'B', description: 'second' },
                ],
              },
            ],
          },
        };
      },
      async answerPendingUserInput(input) {
        seen.push(input);
        return { itemId: 'call-1', ack: { operation: 'user_input' } };
      },
    }),
  });

  assert.equal(result.exitCode, 0);
  assert.deepEqual(seen[0].answers, { choice: ['B'] });
});

test('CLI web-session wait forwards settleMs to the SDK wait helper', { concurrency: false }, async () => {
  const seen = [];
  const result = await runCliCaptured([
    'web-session',
    'wait',
    '--base-url',
    'http://127.0.0.1:3000',
    '--project-id',
    'p1',
    '--session-id',
    'ws1',
    '--until',
    'done',
    '--interval-ms',
    '500',
    '--timeout-ms',
    '5000',
    '--settle-ms',
    '2000',
  ], {
    clientFactory: () => ({
      async waitForWebSessionState(input) {
        seen.push(input);
        return { phase: 'done' };
      },
    }),
  });

  assert.equal(result.exitCode, 0);
  assert.equal(seen[0].settleMs, 2000);
  assert.equal(seen[0].intervalMs, 500);
  assert.equal(seen[0].timeoutMs, 5000);
});

test('CLI web-session run reuses SDK orchestration and still reads optional files after completion', { concurrency: false }, async () => {
  const calls = [];
  const result = await runCliCaptured([
    'web-session',
    'run',
    '--base-url',
    'http://127.0.0.1:3000',
    '--project-id',
    'p1',
    '--agent',
    'codex',
    '--text',
    'Create notes/123.md with a short summary.',
    '--delete-file-before',
    'notes/123.md',
    '--read-file-after',
    'notes/123.md',
    '--strict-cwd',
    '--settle-ms',
    '2500',
  ], {
    clientFactory: () => ({
      async createWebSession(input) {
        calls.push({ type: 'create', input });
        return { id: 'ws-created', projectId: 'p1' };
      },
      async deleteProjectFiles(input) {
        calls.push({ type: 'delete-file', input });
        return { succeeded: [], failed: [] };
      },
      async sendWebSessionMessage(input) {
        calls.push({ type: 'send', input });
        return { operation: 'send' };
      },
      async runWebSessionUntilDone(input) {
        calls.push({ type: 'run-until-done', input });
        return {
          stopReason: 'done',
          actions: [
            { type: 'answer_user_input', itemId: 'call-1' },
            { type: 'execute_plan', mode: 'followup_message' },
          ],
          finalState: {
            phase: 'done',
            lastAssistantMessage: { text: 'Done.' },
          },
        };
      },
      async readProjectFileText(input) {
        calls.push({ type: 'read-file', input });
        return { path: input.filePath, text: '# summary' };
      },
    }),
  });

  assert.equal(result.exitCode, 0);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.session.id, 'ws-created');
  assert.equal(payload.actions.length, 2);
  assert.match(calls.find(call => call.type === 'send').input.text, /Stay strictly inside the current working directory/);
  const runInput = calls.find(call => call.type === 'run-until-done').input;
  assert.equal(runInput.settleMs, 2500);
  assert.equal(runInput.executePlanPrompt.includes('Stay strictly inside the current working directory'), true);
  assert.equal(payload.filesAfter[0].text, '# summary');
});

test('CLI web-session run accepts an image-only initial message', { concurrency: false }, async () => {
  const calls = [];
  const result = await runCliCaptured([
    'web-session',
    'run',
    '--base-url',
    'http://127.0.0.1:3000',
    '--project-id',
    'p1',
    '--image',
    './reference.png',
  ], {
    clientFactory: () => ({
      async uploadWebSessionAttachment(input) {
        calls.push({ type: 'upload', input });
        return { id: 'att-image', name: 'reference.png', mime: 'image/png' };
      },
      async createWebSession(input) {
        calls.push({ type: 'create', input });
        return { id: 'ws-created', projectId: 'p1' };
      },
      async sendWebSessionMessage(input) {
        calls.push({ type: 'send', input });
        return { operation: 'send' };
      },
      async runWebSessionUntilDone(input) {
        calls.push({ type: 'run-until-done', input });
        return {
          stopReason: 'done',
          actions: [],
          finalState: { phase: 'done' },
        };
      },
    }),
  });

  assert.equal(result.exitCode, 0);
  assert.deepEqual(calls.map(call => call.type), [
    'upload',
    'create',
    'send',
    'run-until-done',
  ]);
  assert.deepEqual(calls[0].input, {
    projectId: 'p1',
    path: undefined,
    filePath: './reference.png',
  });
  assert.equal(calls[2].input.text, '');
  assert.deepEqual(calls[2].input.attachmentIds, ['att-image']);
});

test('CLI web-session run does not create a session when an image upload fails', { concurrency: false }, async () => {
  let created = false;
  const result = await runCliCaptured([
    'web-session',
    'run',
    '--base-url',
    'http://127.0.0.1:3000',
    '--project-id',
    'p1',
    '--text',
    'inspect the image',
    '--image',
    './missing.png',
  ], {
    clientFactory: () => ({
      async uploadWebSessionAttachment() {
        throw new Error('image upload failed');
      },
      async createWebSession() {
        created = true;
      },
    }),
  });

  assert.equal(result.exitCode, 1);
  assert.equal(created, false);
  assert.match(JSON.parse(result.stderr).error.message, /image upload failed/);
});

test('CLI web-session list preserves array output', { concurrency: false }, async () => {
  const handlers = new Map([
    ['GET /api/v1/projects/p1/web-sessions', () => createJsonResponse({ items: [] })],
  ]);

  const result = await runCliCaptured([
    'web-session',
    'list',
    '--base-url',
    'http://127.0.0.1:3000',
    '--project-id',
    'p1',
  ], {
    fetchImpl: createFetchMock(handlers),
  });

  assert.equal(result.exitCode, 0);
  assert.equal(result.stdout.trim(), '[]');
});
