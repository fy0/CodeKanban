import test from "node:test";
import assert from "node:assert/strict";

import {
  analyzeWebSession,
  normalizeWebSessionFrame,
} from "../src/web-session-shared.js";

function sampleWireSession() {
  return {
    id: "ws1",
    pid: "p1",
    oi: 1000,
    ag: "codex",
    md: "gpt-5.6",
    re: "high",
    wm: "default",
    pl: "elevated",
    ttl: "Delegate work",
    cwd: "/repo/demo",
    st: "running",
    unr: false,
    act: 1785067200000,
    ca: 1785067200000,
    lu: 1785067201000,
    sk: "codex_app_server",
    ss: "fresh",
    usa: { in: 10, cin: 2, out: 4 },
    cost: 0.02,
    cws: "default",
  };
}

test("snapshot normalizes history sourceThreadId and the authoritative sub-agent registry", () => {
  const frame = normalizeWebSessionFrame({
    v: 1,
    k: "snap",
    sid: "ws1",
    ts: 1785067202000,
    s: sampleWireSession(),
    h: {
      its: [
        {
          id: "child-command",
          sthid: "thread-child",
          stid: "turn-child",
          oi: 1,
          kd: "tool",
          tp: "command_execution",
        },
      ],
      hm: false,
      tot: 1,
    },
    ags: [
      {
        tid: "thread-child",
        ptid: "thread-root",
        p: "review/atlas",
        nn: "Atlas",
        rl: "worker",
        st: "running",
        sm: "Inspecting the repository",
        ctid: "turn-child",
        liid: "child-command",
        loi: 1,
        sa: 1785067201000,
        la: 1785067202000,
      },
    ],
  });

  assert.equal(frame.type, "snapshot");
  assert.equal(frame.snapshot.history.items[0].sourceThreadId, "thread-child");
  assert.equal(frame.snapshot.subAgents.length, 1);
  assert.deepEqual(frame.snapshot.subAgents[0], {
    threadId: "thread-child",
    parentThreadId: "thread-root",
    path: "review/atlas",
    nickname: "Atlas",
    role: "worker",
    status: "running",
    summary: "Inspecting the repository",
    currentTurnId: "turn-child",
    latestItemId: "child-command",
    latestOrderIndex: 1,
    startedAt: "2026-07-26T12:00:01.000Z",
    lastActivityAt: "2026-07-26T12:00:02.000Z",
    endedAt: null,
  });
});

test("incremental sub_agent frames normalize to a dedicated SDK event", () => {
  const frame = normalizeWebSessionFrame({
    v: 1,
    k: "evt",
    sid: "ws1",
    ts: 1785067203000,
    op: "sub_agent",
    ag: {
      tid: "thread-child",
      st: "completed",
      sm: "Review complete",
      ea: 1785067203000,
    },
    s: sampleWireSession(),
  });

  assert.equal(frame.type, "subAgent");
  assert.equal(frame.sessionId, "ws1");
  assert.equal(frame.subAgent.threadId, "thread-child");
  assert.equal(frame.subAgent.status, "completed");
  assert.equal(frame.subAgent.summary, "Review complete");
  assert.equal(frame.subAgent.endedAt, "2026-07-26T12:00:03.000Z");
  assert.equal(frame.session.id, "ws1");
});

test("analysis excludes the native root and does not count reusable idle threads", () => {
  const state = analyzeWebSession({
    session: {
      id: "ws1",
      nativeSessionId: "thread-root",
      status: "running",
    },
    history: { items: [], hasMore: false, total: 0 },
    subAgents: [
      {
        threadId: "thread-root",
        status: "running",
        currentTurnId: "turn-root",
      },
      {
        threadId: "thread-working",
        status: "running",
        currentTurnId: "turn-working",
      },
      {
        threadId: "thread-reusable",
        status: "running",
      },
    ],
  });

  assert.deepEqual(
    state.subAgents.map((agent) => [agent.threadId, agent.status]),
    [
      ["thread-working", "running"],
      ["thread-reusable", "idle"],
    ],
  );
  assert.deepEqual(
    state.activeSubAgents.map((agent) => agent.threadId),
    ["thread-working"],
  );
});
