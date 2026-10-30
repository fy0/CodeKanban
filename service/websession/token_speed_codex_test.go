package websession

import (
	"encoding/json"
	"strings"
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
			send(42, "item/completed", `{"item":{"type":"reasoning","id":"r2"}}`)
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
	send(34, "item/completed", `{"item":{"type":"reasoning","id":"r2"}}`)
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

func TestCodexTokenSpeedIncludesHiddenReasoningAndExcludesToolFeedback(t *testing.T) {
	start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	s := &tokenSpeedState{runID: "run"}
	send := func(seconds int, method, payload string) {
		codexSpeedEvent(t, s, start, seconds, method, payload)
	}
	// A reasoning item can have no visible text while the model is thinking.
	send(0, "item/started", `{"item":{"type":"reasoning","id":"hidden-reasoning"}}`)
	send(20, "rawResponseItem/completed", `{"item":{"type":"function_call","call_id":"cmd","arguments":"{\"command\":\"test\"}"}}`)
	send(20, "item/started", `{"item":{"type":"commandExecution","id":"cmd"}}`)
	before := s.snapshot("epoch")
	if !before.Estimated || before.Generating || before.TokensPerSecond == nil {
		t.Fatalf("visible tool arguments should only provide an estimate before usage: %+v", before)
	}
	feedback := strings.Repeat("tool output ", 2000)
	outputDelta, _ := json.Marshal(map[string]any{"itemId": "cmd", "delta": feedback})
	send(30, "item/commandExecution/outputDelta", string(outputDelta))
	outputItem, _ := json.Marshal(map[string]any{"item": map[string]any{"type": "function_call_output", "call_id": "cmd", "output": feedback}})
	send(50, "rawResponseItem/completed", string(outputItem))
	send(50, "item/completed", `{"item":{"type":"commandExecution","id":"cmd","durationMs":30000}}`)
	after := s.snapshot("epoch")
	if after.SampleID != before.SampleID || after.Generating || after.TokensPerSecond == nil || *after.TokensPerSecond != *before.TokensPerSecond {
		t.Fatalf("tool feedback or execution time changed the generation estimate: %+v", after)
	}
	// These usage values follow real Codex records: 739 reasoning tokens are
	// already included in the 1492 output tokens, even without visible deltas.
	send(50, "rawResponse/completed", `{"responseId":"response-with-hidden-reasoning","usage":{"inputTokens":198531,"outputTokens":1492,"reasoningOutputTokens":739,"totalTokens":200023}}`)
	got := s.snapshot("epoch")
	assertTokenSpeed(t, got, 74.6, false)
	assertTokenSpeedSummary(t, got.Stats.Session, 1, 1492, 20000, false)
	if got.Generating || len(got.History) != 1 || !got.History[0].EndedAt.Equal(start.Add(20*time.Second)) {
		t.Fatalf("usage calibration included the 30-second command: %+v", got)
	}
}

func TestCodexTokenSpeedExcludesUsageAndCompletionDeliveryDelay(t *testing.T) {
	for _, report := range []string{"rawResponse/completed", "thread/tokenUsage/updated"} {
		t.Run(report, func(t *testing.T) {
			start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
			s := &tokenSpeedState{runID: "run"}
			send := func(seconds int, method, payload string) {
				codexSpeedEvent(t, s, start, seconds, method, payload)
			}
			send(0, "item/started", `{"item":{"type":"agentMessage","id":"answer"}}`)
			send(1, "item/agentMessage/delta", `{"itemId":"answer","delta":"first "}`)
			send(2, "item/agentMessage/delta", `{"itemId":"answer","delta":"last"}`)
			send(10, "item/agentMessage/delta", `{"itemId":"answer","delta":""}`)
			send(20, "item/completed", `{"item":{"type":"agentMessage","id":"answer","text":"first last"}}`)
			if report == "rawResponse/completed" {
				send(32, report, `{"responseId":"response","usage":{"outputTokens":80}}`)
			} else {
				send(32, report, `{"tokenUsage":{"total":{"outputTokens":80},"last":{"outputTokens":80}}}`)
			}
			got := s.snapshot("epoch")
			assertTokenSpeed(t, got, 40, false)
			assertTokenSpeedSummary(t, got.Stats.Session, 1, 80, 2000, false)
			if len(got.History) != 1 || !got.History[0].EndedAt.Equal(start.Add(2*time.Second)) {
				t.Fatalf("control notification delay extended the output interval: %+v", got)
			}
		})
	}
}

func TestCodexTokenSpeedTimesHiddenReasoningWithoutVisibleDeltas(t *testing.T) {
	start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	s := &tokenSpeedState{runID: "run"}
	codexSpeedEvent(t, s, start, 0, "item/started", `{"item":{"type":"reasoning","id":"hidden"}}`)
	codexSpeedEvent(t, s, start, 2, "item/completed", `{"item":{"type":"reasoning","id":"hidden","summary":[],"content":[]}}`)
	codexSpeedEvent(t, s, start, 32, "rawResponse/completed", `{"responseId":"hidden-response","usage":{"outputTokens":80,"reasoningOutputTokens":80}}`)
	got := s.snapshot("epoch")
	assertTokenSpeed(t, got, 40, false)
	assertTokenSpeedSummary(t, got.Stats.Session, 1, 80, 2000, false)
}

func TestCodexTokenSpeedDoesNotTimeInstantOutputByUsageDelay(t *testing.T) {
	for _, report := range []string{"rawResponse/completed", "thread/tokenUsage/updated"} {
		t.Run(report, func(t *testing.T) {
			start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
			s := &tokenSpeedState{runID: "run"}
			// With no start notification, a single delta provides no measurable
			// generation interval. Waiting for usage must not invent one.
			codexSpeedEvent(t, s, start, 0, "item/agentMessage/delta", `{"itemId":"answer","delta":"answer"}`)
			if report == "rawResponse/completed" {
				codexSpeedEvent(t, s, start, 30, report, `{"responseId":"response","usage":{"outputTokens":80}}`)
			} else {
				codexSpeedEvent(t, s, start, 30, report, `{"tokenUsage":{"total":{"outputTokens":80},"last":{"outputTokens":80}}}`)
			}
			got := s.snapshot("epoch")
			if got.Generating || got.TokensPerSecond != nil || len(got.History) != 0 || got.Stats.Session.SampleCount != 0 {
				t.Fatalf("usage delay created a rate for instantaneous output: %+v", got)
			}
		})
	}
}
