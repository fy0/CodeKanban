import test from 'node:test';
import assert from 'node:assert/strict';

import { WebSessionCommandChannel } from '../src/web-session-command-channel.js';
import { FakeWebSocket } from './helpers.js';

function sampleWireSession(overrides = {}) {
  return {
    id: 'ws1',
    pid: 'p1',
    oi: 1000,
    ag: 'codex',
    md: 'gpt-5',
    re: 'high',
    wm: 'plan',
    pl: 'elevated',
    ttl: 'Inspect repository',
    cwd: '/repo/demo',
    st: 'running',
    unr: false,
    act: 1710000000000,
    ca: 1710000000000,
    lu: 1710000001000,
    sk: 'codex_app_server',
    ss: 'fresh',
    usa: { in: 10, cin: 2, out: 4 },
    cost: 0.02,
    cws: 'default',
    ...overrides,
  };
}

test('WebSessionCommandChannel connect returns a normalized snapshot', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const channel = new WebSessionCommandChannel({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/ws',
    WebSocketImpl: FakeWebSocket,
  });

  await channel.waitForOpen();
  const promise = channel.connect('ws1');
  await new Promise(resolve => setTimeout(resolve, 0));

  const socket = FakeWebSocket.instances[0];
  assert.equal(socket.sent.length, 1);
  assert.equal(socket.sent[0].op, 'connect');
  assert.equal(socket.sent[0].sid, 'ws1');

  socket.emitJson({
    v: 1,
    k: 'ack',
    rid: socket.sent[0].rid,
    sid: 'ws1',
    ts: 1710000000001,
    op: 'connect',
    ok: 1,
  });
  socket.emitJson({
    v: 1,
    k: 'snap',
    sid: 'ws1',
    ts: 1710000000002,
    s: sampleWireSession(),
    h: {
      its: [
        {
          id: 'item-1',
          oi: 1,
          kd: 'assistant',
          tp: 'agent_message',
          txt: 'hello',
          ts2: 1710000000003,
        },
      ],
      hm: false,
      tot: 1,
    },
    pi: [
      {
        id: 'pending-1',
        m: 'redirect',
        txt: 'adjust course',
        ra: 1710000005000,
        ps: true,
        ca: 1710000000004,
      },
    ],
  });

  const snapshot = await promise;
  assert.equal(snapshot.session.id, 'ws1');
  assert.equal(snapshot.session.agent, 'codex');
  assert.equal(snapshot.history.items.length, 1);
  assert.equal(snapshot.history.items[0].text, 'hello');
  assert.deepEqual(snapshot.pendingInputs[0], {
    id: 'pending-1',
    mode: 'redirect',
    text: 'adjust course',
    attachmentIds: [],
    readyAt: '2024-03-09T16:00:05.000Z',
    paused: true,
    createdAt: '2024-03-09T16:00:00.004Z',
  });
  channel.close();
});

test('WebSessionCommandChannel updates pending text and pause state', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const channel = new WebSessionCommandChannel({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/ws',
    WebSocketImpl: FakeWebSocket,
  });

  await channel.waitForOpen();
  const promise = channel.updatePendingInput('ws1', {
    pendingId: 'pending-1',
    text: 'updated course',
    paused: false,
  });
  await new Promise(resolve => setTimeout(resolve, 0));

  const socket = FakeWebSocket.instances[0];
  assert.equal(socket.sent[0].op, 'pending_update');
  assert.deepEqual(socket.sent[0].p, {
    id: 'pending-1',
    txt: 'updated course',
    paused: false,
  });

  socket.emitJson({
    v: 1,
    k: 'ack',
    rid: socket.sent[0].rid,
    sid: 'ws1',
    ts: 1710000000030,
    op: 'pending_update',
    ok: 1,
  });

  await promise;
  channel.close();
});

test('WebSessionCommandChannel history sends the compact history payload and normalizes the page', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const channel = new WebSessionCommandChannel({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/ws',
    WebSocketImpl: FakeWebSocket,
  });

  await channel.waitForOpen();
  const promise = channel.history('ws1', {
    beforeCursor: '42',
    limit: 25,
  });
  await new Promise(resolve => setTimeout(resolve, 0));

  const socket = FakeWebSocket.instances[0];
  assert.equal(socket.sent[0].op, 'hist');
  assert.deepEqual(socket.sent[0].p, {
    lim: 25,
    bc: '42',
  });

  socket.emitJson({
    v: 1,
    k: 'ack',
    rid: socket.sent[0].rid,
    sid: 'ws1',
    ts: 1710000000010,
    op: 'hist',
    ok: 1,
  });
  socket.emitJson({
    v: 1,
    k: 'evt',
    sid: 'ws1',
    ts: 1710000000011,
    op: 'hist_page',
    h: {
      its: [],
      hm: true,
      bc: '17',
      tot: 100,
    },
  });

  const history = await promise;
  assert.equal(history.hasMore, true);
  assert.equal(history.beforeCursor, '17');
  assert.equal(history.total, 100);
  channel.close();
});

test('WebSessionCommandChannel rejects sendMessage when the server follows the ack with an error frame', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const channel = new WebSessionCommandChannel({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/ws',
    WebSocketImpl: FakeWebSocket,
  });

  await channel.waitForOpen();
  const promise = channel.sendMessage('ws1', {
    text: 'continue',
  });
  await new Promise(resolve => setTimeout(resolve, 0));

  const socket = FakeWebSocket.instances[0];
  const requestId = socket.sent[0].rid;
  socket.emitJson({
    v: 1,
    k: 'ack',
    rid: requestId,
    sid: 'ws1',
    ts: 1710000000020,
    op: 'send',
    ok: 1,
  });
  socket.emitJson({
    v: 1,
    k: 'err',
    rid: requestId,
    sid: 'ws1',
    ts: 1710000000021,
    code: 'invalid_state',
    msg: 'session is already running',
    retry: false,
  });

  await assert.rejects(promise, /session is already running/);
  channel.close();
});

test('WebSessionCommandChannel replies to heartbeat ping without disturbing pending commands', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const channel = new WebSessionCommandChannel({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/ws',
    WebSocketImpl: FakeWebSocket,
  });

  await channel.waitForOpen();
  const promise = channel.connect('ws1');
  await new Promise(resolve => setTimeout(resolve, 0));

  const socket = FakeWebSocket.instances[0];
  socket.emitJson({
    v: 1,
    k: 'hb',
    ts: 1710000000001,
    op: 'ping',
  });

  assert.equal(socket.sent.length, 2);
  assert.equal(socket.sent[1].k, 'hb');
  assert.equal(socket.sent[1].op, 'pong');

  socket.emitJson({
    v: 1,
    k: 'ack',
    rid: socket.sent[0].rid,
    sid: 'ws1',
    ts: 1710000000002,
    op: 'connect',
    ok: 1,
  });
  socket.emitJson({
    v: 1,
    k: 'snap',
    sid: 'ws1',
    ts: 1710000000003,
    s: sampleWireSession(),
    h: {
      its: [],
      hm: false,
      tot: 0,
    },
  });

  const snapshot = await promise;
  assert.equal(snapshot.session.id, 'ws1');
  channel.close();
});

test('WebSessionCommandChannel setGoal sends the expected payload', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const channel = new WebSessionCommandChannel({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/ws',
    WebSocketImpl: FakeWebSocket,
  });

  await channel.waitForOpen();
  const promise = channel.setGoal('ws1', {
    objective: 'Finish migration and keep tests green',
    status: 'active',
  });
  await new Promise(resolve => setTimeout(resolve, 0));

  const socket = FakeWebSocket.instances[0];
  assert.equal(socket.sent[0].op, 'goal_set');
  assert.deepEqual(socket.sent[0].p, {
    obj: 'Finish migration and keep tests green',
    st: 'active',
  });

  socket.emitJson({
    v: 1,
    k: 'ack',
    rid: socket.sent[0].rid,
    sid: 'ws1',
    ts: 1710000000030,
    op: 'goal_set',
    ok: 1,
  });

  await promise;
  channel.close();
});

test('WebSessionCommandChannel bootstrapGoal sends the expected payload', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const channel = new WebSessionCommandChannel({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/ws',
    WebSocketImpl: FakeWebSocket,
  });

  await channel.waitForOpen();
  const promise = channel.bootstrapGoal('ws1', {
    objective: 'Generate the first draft immediately',
    status: 'active',
  });
  await new Promise(resolve => setTimeout(resolve, 0));

  const socket = FakeWebSocket.instances[0];
  assert.equal(socket.sent[0].op, 'goal_bootstrap');
  assert.deepEqual(socket.sent[0].p, {
    obj: 'Generate the first draft immediately',
    st: 'active',
  });

  socket.emitJson({
    v: 1,
    k: 'ack',
    rid: socket.sent[0].rid,
    sid: 'ws1',
    ts: 1710000000030,
    op: 'goal_bootstrap',
    ok: 1,
  });

  await promise;
  channel.close();
});
