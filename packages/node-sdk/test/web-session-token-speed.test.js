import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeTokenSpeed, normalizeWebSessionFrame, normalizeWebSessionSummaryFromWire } from '../src/web-session-shared.js';

const speed = { epoch: 'process-1', version: 7, runId: 'run-1', sampleId: '2', tokensPerSecond: 42,
  estimated: false, generating: false, updatedAt: '2026-10-15T01:00:00Z' };

test('token speed is available in both summaries and ephemeral frames', () => {
  assert.deepEqual(normalizeWebSessionSummaryFromWire({ id: 's1', tsp: speed }).tokenSpeed, speed);
  const frame = normalizeWebSessionFrame({ v: 1, k: 'evt', sid: 's1', ts: 1, op: 'token_speed', p: speed });
  assert.equal(frame.type, 'tokenSpeed');
  assert.equal(frame.sessionId, 's1');
  assert.equal(frame.revision, null);
  assert.deepEqual(frame.tokenSpeed, speed);
});

test('older servers and invalid rates remain unavailable', () => {
  assert.equal(normalizeWebSessionSummaryFromWire({ id: 's1' }).tokenSpeed, null);
  assert.equal(normalizeTokenSpeed({ ...speed, tokensPerSecond: Infinity }).tokensPerSecond, null);
  assert.equal(normalizeTokenSpeed({ ...speed, version: -1 }), null);
});
