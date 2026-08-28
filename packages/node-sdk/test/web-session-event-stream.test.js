import test from 'node:test';
import assert from 'node:assert/strict';

import { WebSessionEventStream } from '../src/web-session-event-stream.js';
import { FakeWebSocket } from './helpers.js';

function sampleSnapshotFrame(sessionId) {
  return {
    v: 1,
    k: 'snap',
    sid: sessionId,
    ts: 1710000100000,
    s: {
      id: sessionId,
      pid: 'p1',
      oi: 1000,
      ag: 'codex',
      md: 'gpt-5',
      re: 'high',
      wm: 'plan',
      pl: 'elevated',
      ttl: 'session',
      cwd: '/repo/demo',
      st: 'running',
      unr: false,
      act: 1710000100000,
      ca: 1710000100000,
      lu: 1710000100001,
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
  };
}

test('WebSessionEventStream filters by sessionId and does not duplicate iterator events', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const stream = new WebSessionEventStream({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/events',
    sessionId: 'ws1',
    WebSocketImpl: FakeWebSocket,
  });

  let frameEvents = 0;
  let snapshotEvents = 0;
  stream.on('frame', event => {
    frameEvents += 1;
    assert.equal(event.type, 'snapshot');
  });
  stream.on('snapshot', () => {
    snapshotEvents += 1;
  });

  await stream.waitForOpen();
  const iterator = stream[Symbol.asyncIterator]();
  const openEvent = await iterator.next();
  assert.equal(openEvent.value.type, 'open');

  const socket = FakeWebSocket.instances[0];
  assert.equal(socket.sent.length, 1);
  assert.equal(socket.sent[0].k, 'hb');
  assert.equal(socket.sent[0].op, 'focus');
  assert.equal(socket.sent[0].sid, 'ws1');
  socket.emitJson(sampleSnapshotFrame('ws2'));
  socket.emitJson(sampleSnapshotFrame('ws1'));

  const snapshotEvent = await iterator.next();
  assert.equal(snapshotEvent.value.type, 'snapshot');
  assert.equal(snapshotEvent.value.snapshot.session.id, 'ws1');
  assert.equal(frameEvents, 1);
  assert.equal(snapshotEvents, 1);

  stream.close();
  const closeEvent = await iterator.next();
  assert.equal(closeEvent.value.type, 'close');
  const doneEvent = await iterator.next();
  assert.equal(doneEvent.done, true);
});

test('WebSessionEventStream waitFor resolves a normalized history item event', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const stream = new WebSessionEventStream({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/events',
    WebSocketImpl: FakeWebSocket,
  });

  await stream.waitForOpen();
  const socket = FakeWebSocket.instances[0];
  const waitPromise = stream.waitFor(event => event.type === 'historyItem' && event.item.id === 'hist-1', {
    timeoutMs: 1000,
  });

  socket.emitJson({
    v: 1,
    k: 'evt',
    sid: 'ws9',
    ts: 1710000200000,
    op: 'hist_item',
    i: {
      id: 'hist-1',
      oi: 3,
      kd: 'tool',
      tp: 'exec_command',
      txt: '',
      ts2: 1710000200001,
      tl: {
        id: 'tool-1',
        name: 'exec_command',
        kind: 'command',
        in: { cmd: 'pwd' },
        out: '/repo/demo',
        st: 'done',
      },
    },
  });

  const event = await waitPromise;
  assert.equal(event.type, 'historyItem');
  assert.equal(event.sessionId, 'ws9');
  assert.equal(event.item.tool.name, 'exec_command');
  assert.equal(event.item.tool.output, '/repo/demo');
  stream.close();
});

test('WebSessionEventStream normalizes the pending clock and an empty snapshot', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const stream = new WebSessionEventStream({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/events',
    WebSocketImpl: FakeWebSocket,
  });
  await stream.waitForOpen();
  const iterator = stream[Symbol.asyncIterator]();
  await iterator.next();

  FakeWebSocket.instances[0].emitJson({
    v: 1,
    k: 'evt',
    sid: 'ws-pending',
    ts: 1710000250000,
    op: 'pending',
    pe: 'process-epoch-1',
    pv: 7,
    pi: [],
  });

  const event = (await iterator.next()).value;
  assert.equal(event.type, 'pending');
  assert.equal(event.revision, null);
  assert.equal(event.pendingEpoch, 'process-epoch-1');
  assert.equal(event.pendingVersion, 7);
  assert.deepEqual(event.pendingInputs, []);
  stream.close();
});

test('WebSessionEventStream replies to heartbeat ping without emitting a business event', async () => {
  FakeWebSocket.reset();
  FakeWebSocket.setFactory(socket => {
    queueMicrotask(() => socket.open());
  });

  const stream = new WebSessionEventStream({
    url: 'ws://127.0.0.1:3000/api/v1/web-sessions/events',
    WebSocketImpl: FakeWebSocket,
  });

  await stream.waitForOpen();
  const socket = FakeWebSocket.instances[0];
  const iterator = stream[Symbol.asyncIterator]();
  await iterator.next();

  socket.emitJson({
    v: 1,
    k: 'hb',
    ts: 1710000300000,
    op: 'ping',
  });

  assert.equal(socket.sent.length, 1);
  assert.equal(socket.sent[0].k, 'hb');
  assert.equal(socket.sent[0].op, 'pong');

  stream.close();
});
