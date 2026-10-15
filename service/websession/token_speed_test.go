package websession

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"code-kanban/model/tables"
)

func TestTokenSpeedSamplingAndCalibration(t *testing.T) {
	start := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	s := &tokenSpeedState{runID: "run"}
	s.begin("request-1", start, true)
	for _, text := range []string{"abcd", "中文", "🙂", "efgh"} {
		s.delta("request-1", "text", text, start.Add(time.Second))
	}
	s.updatedAt = start.Add(2 * time.Second)
	assertTokenSpeed(t, s.snapshot("epoch"), 2.5, true)
	output := int64(80)
	s.finish("request-1", &output, start.Add(2*time.Second), true)
	s.updatedAt = start.Add(time.Minute)
	assertTokenSpeed(t, s.snapshot("epoch"), 40, false)
	// An approval or terminal cleanup after completion cannot lengthen the
	// generation interval or invalidate a previously calibrated sample.
	s.finish("", nil, start.Add(time.Minute), false)
	assertTokenSpeed(t, s.snapshot("epoch"), 40, false)
	s.begin("request-2", start.Add(time.Minute), true)
	s.delta("request-2", "text", "abcd", start.Add(time.Minute))
	s.delta("request-1", "text", "late duplicate", start.Add(time.Minute))
	s.finish("request-1", &output, start.Add(time.Minute), true)
	if s.current().key != "request-2" {
		t.Fatal("late data replaced the current sample")
	}
	s.updatedAt = start.Add(time.Minute + time.Second)
	assertTokenSpeed(t, s.snapshot("epoch"), 1, true)
}

func TestTokenSpeedShortMissingAndInterruptedSamples(t *testing.T) {
	start := time.Now()
	for _, duration := range []time.Duration{0, -time.Second, 499 * time.Millisecond} {
		s := &tokenSpeedState{updatedAt: start.Add(duration)}
		s.begin("request", start, true)
		s.delta("request", "text", "abcd", start)
		if got := s.snapshot("e"); got.TokensPerSecond != nil {
			t.Fatalf("short sample: %#v", got)
		}
	}
	s := &tokenSpeedState{updatedAt: start.Add(time.Second)}
	s.delta("", "text", "abcd", start)
	output := int64(50)
	s.finish("", &output, start.Add(time.Second), true)
	assertTokenSpeed(t, s.snapshot("e"), 50, true) // no observed start boundary
	s = &tokenSpeedState{updatedAt: start.Add(time.Second)}
	s.begin("request", start, true)
	s.delta("request", "text", "abcd", start)
	s.finish("", nil, start.Add(time.Second), false)
	s.stopped = true
	s.delta("request", "text", "ignored", start.Add(time.Hour))
	assertTokenSpeed(t, s.snapshot("e"), 1, true)
}

func assertTokenSpeed(t *testing.T, got TokenSpeed, want float64, estimated bool) {
	t.Helper()
	if got.TokensPerSecond == nil || math.Abs(*got.TokensPerSecond-want) > .01 || got.Estimated != estimated {
		t.Fatalf("speed = %#v (%v), want %v estimated=%v", got, got.TokensPerSecond, want, estimated)
	}
}

func tokenSpeedTestManager(t *testing.T) (*Manager, tables.WebSessionTable, *activeRun) {
	t.Helper()
	m := &Manager{}
	var session tables.WebSessionTable
	session.ID = "speed-session"
	run := &activeRun{runID: "speed-run"}
	m.observeTokenSpeedLifecycle(session, Event{Type: "run_st", RunID: run.runID})
	t.Cleanup(func() { m.clearTokenSpeed(session.ID) })
	return m, session, run
}

func ageTokenSpeedSample(m *Manager, sessionID string) {
	m.tokenSpeedMu.Lock()
	m.tokenSpeeds[sessionID].current().started = time.Now().Add(-2 * time.Second)
	m.tokenSpeedMu.Unlock()
}

func TestCodexTokenSpeedUsageAndRequestBoundaries(t *testing.T) {
	m, session, run := tokenSpeedTestManager(t)
	send := func(method, data string) { m.observeCodexTokenSpeed(session, run, method, json.RawMessage(data)) }
	send("item/started", `{"item":{"type":"reasoning","id":"r1"}}`)
	send("item/reasoning/summaryTextDelta", `{"itemId":"r1","delta":"summary"}`)
	send("item/reasoning/textDelta", `{"itemId":"r1","delta":"real thinking"}`)
	send("item/agentMessage/delta", `{"itemId":"a1","delta":"answer"}`)
	ageTokenSpeedSample(m, session.ID)
	send("rawResponse/completed", `{}`)
	send("thread/tokenUsage/updated", `{"tokenUsage":{"total":{"outputTokens":80},"last":{"outputTokens":80}}}`)
	assertTokenSpeed(t, *m.tokenSpeedSnapshot(session.ID), 40, false)
	send("item/started", `{"item":{"type":"agentMessage","id":"a2"}}`)
	send("thread/tokenUsage/updated", `{"tokenUsage":{"total":{"outputTokens":80},"last":{"outputTokens":80}}}`)
	if snapshot := m.tokenSpeedSnapshot(session.ID); !snapshot.Generating || snapshot.TokensPerSecond != nil {
		t.Fatalf("duplicate usage affected next request: %#v", snapshot)
	}
	// A missing usage report must not cause a later batch to calibrate the wrong request.
	send("rawResponse/completed", `{}`)
	send("item/started", `{"item":{"type":"agentMessage","id":"a3"}}`)
	send("thread/tokenUsage/updated", `{"tokenUsage":{"total":{"outputTokens":150},"last":{"outputTokens":70}}}`)
	if !m.tokenSpeedSnapshot(session.ID).Estimated {
		t.Fatal("ambiguous request was calibrated")
	}
}

func TestClaudeTokenSpeedPartialEventsAndDuplicateFinalMessage(t *testing.T) {
	m, session, run := tokenSpeedTestManager(t)
	send := func(raw string) {
		var event map[string]any
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatal(err)
		}
		m.observeClaudeTokenSpeed(session, run, event)
	}
	send(`{"type":"stream_event","event":{"type":"message_start","message":{"id":"m1"}}}`)
	send(`{"type":"stream_event","event":{"type":"message_delta","usage":{"output_tokens":1}}}`)
	m.tokenSpeedMu.Lock()
	endedEarly := !m.tokenSpeeds[session.ID].current().ended.IsZero()
	m.tokenSpeedMu.Unlock()
	if endedEarly {
		t.Fatal("intermediate usage ended a streaming message")
	}
	send(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"text":"abcd中文"}}}`)
	ageTokenSpeedSample(m, session.ID)
	send(`{"type":"stream_event","event":{"type":"message_delta","usage":{"output_tokens":80}}}`)
	send(`{"type":"stream_event","event":{"type":"message_stop"}}`)
	send(`{"type":"assistant","message":{"id":"m1","content":[{"text":"abcd中文"}],"usage":{"output_tokens":80}}}`)
	assertTokenSpeed(t, *m.tokenSpeedSnapshot(session.ID), 40, false)
	send(`{"type":"stream_event","parent_tool_use_id":"child","event":{"type":"message_start","message":{"id":"child"}}}`)
	if m.tokenSpeedSnapshot(session.ID).SampleID != "1" {
		t.Fatal("child message entered root telemetry")
	}
}

func TestCodexTokenSpeedFreshThreadResetsUsageBaseline(t *testing.T) {
	m, session, run := tokenSpeedTestManager(t)
	session.TotalOutputTokens = 1000
	m.observeTokenSpeedLifecycle(session, Event{Type: "run_st", RunID: run.runID})
	m.observeCodexTokenSpeed(session, run, "item/started", json.RawMessage(`{"threadId":"fresh-thread","item":{"type":"agentMessage","id":"message"}}`))
	ageTokenSpeedSample(m, session.ID)
	m.observeCodexTokenSpeed(session, run, "thread/tokenUsage/updated", json.RawMessage(`{"threadId":"fresh-thread","tokenUsage":{"total":{"outputTokens":40},"last":{"outputTokens":40}}}`))
	assertTokenSpeed(t, *m.tokenSpeedSnapshot(session.ID), 20, false)
}

func TestDevinTokenSpeedRootUsageDeduplication(t *testing.T) {
	m, session, run := tokenSpeedTestManager(t)
	proj := &devinRunProjection{}
	chunk := map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": strings.Repeat("x", 80)}}
	m.observeDevinTokenSpeed(session, run, proj, chunk, "")
	ageTokenSpeedSample(m, session.ID)
	usage := map[string]any{"sessionUpdate": "usage_update", "_meta": map[string]any{"cognition.ai/outputTokens": 80, "cognition.ai/subagent_context": map[string]any{"parentAgentId": "root"}}}
	m.observeDevinTokenSpeed(session, run, proj, usage, "")
	assertTokenSpeed(t, *m.tokenSpeedSnapshot(session.ID), 40, true)
	m.observeDevinTokenSpeed(session, run, proj, chunk, "")
	m.observeDevinTokenSpeed(session, run, proj, usage, "")
	if !m.tokenSpeedSnapshot(session.ID).Generating {
		t.Fatal("duplicate usage ended the next sample")
	}
	m.observeDevinTokenSpeed(session, run, proj, chunk, "child")
	if m.tokenSpeedSnapshot(session.ID).SampleID != "2" {
		t.Fatal("child affected root sample")
	}
}

func TestPiTokenSpeedMessageUsage(t *testing.T) {
	m, session := newTextDeltaTestManager(t)
	run := &activeRun{runID: "pi-speed"}
	t.Cleanup(func() { m.clearTokenSpeed(session.ID) })
	dispatch := &piRuntimeRun{run: run, session: *session, tools: make(map[string]*piRuntimeToolState)}
	if err := m.startPiAssistantMessage(dispatch); err != nil {
		t.Fatal(err)
	}
	if err := m.handlePiAssistantMessageEvent(dispatch, piAssistantMessageEvent{Type: "text_delta", Delta: "abcd"}); err != nil {
		t.Fatal(err)
	}
	ageTokenSpeedSample(m, session.ID)
	if err := m.finishPiAssistantMessage(dispatch, piRPCMessage{Role: "assistant", StopReason: "stop", Usage: &piRPCTokenUsage{Output: 80}}); err != nil {
		t.Fatal(err)
	}
	got := m.tokenSpeedSnapshot(session.ID)
	if got.TokensPerSecond == nil || math.Abs(*got.TokensPerSecond-40) > 2 || got.Estimated || got.Generating {
		t.Fatalf("Pi speed: %#v", got)
	}
}

func TestTokenSpeedSnapshotDoesNotAdvanceHistory(t *testing.T) {
	m, session := newTextDeltaTestManager(t)
	t.Cleanup(func() { m.clearTokenSpeed(session.ID) })
	before, err := m.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	m.updateTokenSpeed(session.ID, "run", func(s *tokenSpeedState, now time.Time) {
		s.begin("request", now.Add(-time.Second), true)
		s.delta("request", "text", "abcd", now)
		s.finish("request", nil, now, true)
	}, true)
	response, err := m.SnapshotIfChanged(context.Background(), session.ID, 10, formatSnapshotRevision(normalizeSnapshotRevision(before.SnapshotRevision)))
	if err != nil {
		t.Fatal(err)
	}
	if !response.Unchanged || response.TokenSpeed == nil || response.TokenSpeed.TokensPerSecond == nil {
		t.Fatalf("conditional snapshot lost telemetry: %#v", response)
	}
	after, err := m.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.SnapshotRevision != after.SnapshotRevision || !before.ActivityAt.Equal(after.ActivityAt) || before.HasUnread != after.HasUnread {
		t.Fatal("telemetry changed durable session state")
	}
	m.clearTokenSpeed(session.ID)
	if m.tokenSpeedSnapshot(session.ID).TokensPerSecond != nil {
		t.Fatal("clear retained a sample")
	}
}

func TestTokenSpeedThrottlesSummariesAndRejectsPreviousRun(t *testing.T) {
	m, session, run := tokenSpeedTestManager(t)
	m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
		s.begin("one", now.Add(-time.Second), true)
		s.delta("one", "text", "abcd", now)
	}, true)
	first := m.tokenSpeedSnapshot(session.ID)
	m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
		s.delta("one", "text", "efgh", now)
	}, false)
	if second := m.tokenSpeedSnapshot(session.ID); second.Version != first.Version {
		t.Fatal("summary bypassed the publication throttle")
	}
	m.observeTokenSpeedLifecycle(session, Event{Type: "run_st", RunID: "new-run"})
	m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
		s.begin("late", now, true)
	}, true)
	if got := m.tokenSpeedSnapshot(session.ID); got.RunID != "new-run" || got.TokensPerSecond != nil || got.Generating {
		t.Fatalf("stale run affected telemetry: %#v", got)
	}
}

func TestTokenSpeedNotificationsOnlyReachFocusedClients(t *testing.T) {
	m, session, run := tokenSpeedTestManager(t)
	focusedConn, otherConn := &captureWSConn{}, &captureWSConn{}
	focused := &client{conn: focusedConn, kind: clientKindEvent, focusedSID: session.ID}
	other := &client{conn: otherConn, kind: clientKindEvent, focusedSID: "different-session"}
	m.clients = map[*client]struct{}{focused: {}, other: {}}
	m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
		s.begin("one", now.Add(-time.Second), true)
		s.delta("one", "text", "abcd", now)
		s.finish("one", nil, now, true)
	}, true)
	if len(focusedConn.frames) != 1 || len(otherConn.frames) != 0 {
		t.Fatal("speed notification escaped focus filtering")
	}
	if frame := focusedConn.frames[0]; frame.Revision != "" || frame.Operation != wireOpTokenSpeed {
		t.Fatalf("unexpected speed frame: %#v", frame)
	}
}
