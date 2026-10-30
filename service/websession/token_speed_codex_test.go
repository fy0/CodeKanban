package websession

import (
	"encoding/json"
	"testing"
	"time"
)

func codexSpeedEvent(t *testing.T, s *tokenSpeedState, start time.Time, seconds int, method, payload string) {
	t.Helper()
	now := start.Add(time.Duration(seconds) * time.Second)
	s.observeCodexTokenSpeed(method, decodeRawObject(json.RawMessage(payload)), now)
	s.updatedAt = now
}

func TestCodexTokenSpeedCommandTimeAndUsageOrdering(t *testing.T) {
	for _, order := range []string{"response-first", "thread-first"} {
		t.Run(order, func(t *testing.T) {
			start := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
			s := &tokenSpeedState{runID: "run"}
			send := func(seconds int, method, payload string) {
				codexSpeedEvent(t, s, start, seconds, method, payload)
			}
			send(0, "item/started", `{"item":{"type":"reasoning","id":"r1"}}`)
			send(1, "item/reasoning/textDelta", `{"itemId":"r1","delta":"thinking"}`)
			send(1, "item/started", `{"item":{"type":"plan","id":"p1"}}`)
			send(1, "item/plan/delta", `{"itemId":"p1","delta":"a plan"}`)
			send(2, "rawResponseItem/completed", `{"item":{"type":"function_call","call_id":"cmd1","arguments":"{}"}}`)
			send(2, "item/started", `{"item":{"type":"commandExecution","id":"cmd1"}}`)
			response := `{"responseId":"response-1","usage":{"outputTokens":80,"reasoningOutputTokens":40}}`
			thread := `{"tokenUsage":{"total":{"outputTokens":80},"last":{"outputTokens":80,"reasoningOutputTokens":40}}}`
			if order == "response-first" {
				send(32, "rawResponse/completed", response)
				send(33, "thread/tokenUsage/updated", thread)
			} else {
				send(32, "thread/tokenUsage/updated", thread)
				send(33, "rawResponse/completed", response)
			}
			got := s.snapshot("epoch")
			assertTokenSpeed(t, got, 40, false)
			assertTokenSpeedSummary(t, got.Stats.Session, 1, 80, 2000, false)
			if got.Generating || len(got.History) != 1 || !got.History[0].EndedAt.Equal(start.Add(2*time.Second)) {
				t.Fatalf("command execution extended the generation: %+v", got)
			}

			send(35, "item/reasoning/textDelta", `{"itemId":"r2","delta":""}`)
			send(40, "item/started", `{"item":{"type":"reasoning","id":"r2"}}`)
			send(41, "item/reasoning/textDelta", `{"itemId":"r2","delta":"more thinking"}`)
			send(42, "rawResponse/completed", `{"responseId":"response-2","usage":{"outputTokens":120}}`)
			send(43, "thread/tokenUsage/updated", `{"tokenUsage":{"total":{"outputTokens":200},"last":{"outputTokens":120}}}`)
			assertTokenSpeed(t, s.snapshot("epoch"), 60, false)
			assertTokenSpeedSummary(t, s.snapshot("epoch").Stats.Session, 2, 200, 4000, false)
		})
	}
}

func TestCodexTokenSpeedDelayedCompletionDoesNotStopNextGeneration(t *testing.T) {
	start := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	s := &tokenSpeedState{runID: "run"}
	send := func(seconds int, method, payload string) {
		codexSpeedEvent(t, s, start, seconds, method, payload)
	}
	send(0, "item/started", `{"item":{"type":"reasoning","id":"r1"}}`)
	send(1, "item/agentMessage/delta", `{"itemId":"a1","delta":"before commands"}`)
	send(2, "rawResponseItem/completed", `{"item":{"type":"function_call","call_id":"cmd1","arguments":"{}"}}`)
	send(2, "rawResponseItem/completed", `{"item":{"type":"function_call","call_id":"cmd2","arguments":"{}"}}`)
	send(2, "item/started", `{"item":{"type":"commandExecution","id":"cmd1"}}`)
	send(32, "item/started", `{"item":{"type":"reasoning","id":"r2"}}`)
	send(33, "item/reasoning/textDelta", `{"itemId":"r2","delta":"after commands"}`)
	send(33, "rawResponse/completed", `{"responseId":"response-1","usage":{"outputTokens":80}}`)
	send(33, "item/started", `{"item":{"type":"commandExecution","id":"cmd2"}}`)
	send(33, "item/completed", `{"item":{"type":"agentMessage","id":"a1","text":"before commands"}}`)
	send(33, "item/agentMessage/delta", `{"itemId":"a1","delta":"late duplicate"}`)
	send(33, "thread/tokenUsage/updated", `{"tokenUsage":{"total":{"outputTokens":80},"last":{"outputTokens":80}}}`)
	send(33, "rawResponse/completed", `{"responseId":"response-1","usage":{"outputTokens":80}}`)
	got := s.snapshot("epoch")
	if !got.Generating || got.SampleID != "2" || len(got.History) != 1 {
		t.Fatalf("delayed events changed the next generation: %+v", got)
	}
	assertTokenSpeedSummary(t, got.Stats.Recent5, 1, 80, 2000, false)
	send(34, "rawResponse/completed", `{"responseId":"response-2","usage":{"outputTokens":120}}`)
	assertTokenSpeed(t, s.snapshot("epoch"), 60, false)
	assertTokenSpeedSummary(t, s.snapshot("epoch").Stats.Session, 2, 200, 4000, false)
}

func TestCodexTokenSpeedPublishesCommandBoundary(t *testing.T) {
	m, session, run := tokenSpeedTestManager(t)
	m.observeCodexTokenSpeed(session, run, "item/started", json.RawMessage(`{"item":{"type":"reasoning","id":"r1"}}`))
	before := m.tokenSpeedSnapshot(session.ID)
	if !before.Generating {
		t.Fatal("generation did not start")
	}
	m.observeCodexTokenSpeed(session, run, "item/started", json.RawMessage(`{"item":{"type":"commandExecution","id":"cmd1"}}`))
	if got := m.tokenSpeedSnapshot(session.ID); got.Generating || got.Version <= before.Version {
		t.Fatalf("command boundary was throttled: %+v", got)
	}
}
