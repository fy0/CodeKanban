package websession

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPiProjectionBoundsToolOutputBeforePersistence(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	dispatch := &piRuntimeRun{
		run:                &activeRun{runID: "pi-tool-run"},
		session:            *session,
		assistantMessageID: "pi-tool-message",
		contents:           make(map[int]*piRuntimeContentState),
		tools:              make(map[string]*piRuntimeToolState),
	}
	payload, err := json.Marshal(piRPCToolExecutionEvent{
		Type: "tool_execution_end", ToolCallID: "tool-1", ToolName: "custom_tool",
		Result: strings.Repeat("x", defaultToolOutputLimit+1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.handlePiToolExecution(dispatch, piRPCEvent{Type: "tool_execution_end", Raw: payload}); err != nil {
		t.Fatalf("handle tool execution: %v", err)
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	for _, event := range events {
		if event.Type != "tool_end" {
			continue
		}
		if got := stringValue(event.Payload["out"]); len(got) != defaultToolOutputLimit+3 || !strings.HasSuffix(got, "...") {
			t.Fatalf("unexpected persisted tool output: length=%d value=%q", len(got), got)
		}
		return
	}
	t.Fatal("tool_end event was not persisted")
}

func TestPiProjectionCoalescesStreamingTextAndReasoning(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	run := &activeRun{runID: "pi-run"}
	dispatch := &piRuntimeRun{
		run:                  run,
		session:              *session,
		assistantMessageID:   "pi-message",
		assistantMessageOpen: true,
		contents:             make(map[int]*piRuntimeContentState),
		tools:                make(map[string]*piRuntimeToolState),
	}

	var text strings.Builder
	for index := 0; index < 100; index++ {
		chunk := "text-chunk-"
		text.WriteString(chunk)
		if err := manager.handlePiAssistantMessageEvent(dispatch, piAssistantMessageEvent{
			Type: "text_delta", ContentIndex: 0, Delta: chunk,
		}); err != nil {
			t.Fatalf("handle text delta %d: %v", index, err)
		}
	}

	if err := manager.handlePiAssistantMessageEvent(dispatch, piAssistantMessageEvent{
		Type: "thinking_start", ContentIndex: 1,
	}); err != nil {
		t.Fatalf("handle thinking start: %v", err)
	}
	var reasoning strings.Builder
	for index := 0; index < 100; index++ {
		chunk := strings.Repeat("r", 80)
		reasoning.WriteString(chunk)
		if err := manager.handlePiAssistantMessageEvent(dispatch, piAssistantMessageEvent{
			Type: "thinking_delta", ContentIndex: 1, Delta: chunk,
		}); err != nil {
			t.Fatalf("handle thinking delta %d: %v", index, err)
		}
	}
	finalReasoning := reasoning.String()
	if err := manager.handlePiAssistantMessageEvent(dispatch, piAssistantMessageEvent{
		Type: "thinking_end", ContentIndex: 1, Content: finalReasoning,
	}); err != nil {
		t.Fatalf("handle thinking end: %v", err)
	}
	if err := manager.finishPiAssistantMessage(dispatch, piRPCMessage{
		Role: "assistant",
		Content: []struct {
			Type      string         `json:"type"`
			Text      string         `json:"text"`
			Thinking  string         `json:"thinking"`
			ID        string         `json:"id"`
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}{
			{Type: "text", Text: text.String()},
			{Type: "thinking", Thinking: finalReasoning},
		},
	}); err != nil {
		t.Fatalf("finish assistant message: %v", err)
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	if count := countEventsByType(events, "txt_d"); count != 1 {
		t.Fatalf("persisted text delta count = %d, want 1", count)
	}
	if got := joinedTextDeltaPayload(events); got != text.String() {
		t.Fatalf("merged Pi text length = %d, want %d", len(got), text.Len())
	}

	reasoningUpdates := 0
	finalReasoningSeen := false
	wantFinal := strings.Repeat("r", defaultToolOutputLimit) + "..."
	for _, event := range events {
		if event.Type != "tool_st" && event.Type != "tool_end" || stringValue(event.Payload["kind"]) != "reasoning" {
			continue
		}
		reasoningUpdates++
		output := stringValue(event.Payload["out"])
		if len(output) > defaultToolOutputLimit+3 {
			t.Fatalf("reasoning output was not bounded: %d bytes", len(output))
		}
		if event.Type == "tool_end" && output == wantFinal {
			finalReasoningSeen = true
		}
	}
	if reasoningUpdates >= 100 {
		t.Fatalf("reasoning updates were not throttled: %d events", reasoningUpdates)
	}
	if !finalReasoningSeen {
		t.Fatal("final bounded reasoning snapshot was not persisted")
	}
}

func TestPiProjectionStreamsUsageAndNativeStateChanges(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	dispatch := &piRuntimeRun{
		run:      &activeRun{runID: "pi-usage-run"},
		session:  *session,
		contents: make(map[int]*piRuntimeContentState),
		tools:    make(map[string]*piRuntimeToolState),
	}

	usageEvent := piRPCEvent{Type: "message_update", Raw: json.RawMessage(`{
		"type":"message_update",
		"usage":{"input":100,"output":20,"cacheRead":15,"cacheWrite":5,"totalTokens":140,"cost":{"total":0.02}},
		"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"hello"}
	}`)}
	if err := manager.handlePiRuntimeEvent(dispatch, usageEvent); err != nil {
		t.Fatalf("handle usage update: %v", err)
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.LatestTokenCountInputTokens != 100 || record.LatestTokenCountCachedInputTokens != 20 ||
		record.LatestTokenCountOutputTokens != 20 || record.LatestTokenCountTotalTokens != 140 ||
		record.LatestTokenCountUpdatedAt == nil {
		t.Fatalf("unexpected live Pi usage projection: %#v", record)
	}
	events := readTextDeltaTestEvents(t, manager, session.ID)
	usageCount := 0
	for _, event := range events {
		if event.Type == "usage" {
			usageCount++
			if int64(numberValue(event.Payload["in"])) != 100 || int64(numberValue(event.Payload["cin"])) != 20 || int64(numberValue(event.Payload["out"])) != 20 {
				t.Fatalf("unexpected usage event payload: %#v", event.Payload)
			}
		}
	}
	if usageCount != 1 {
		t.Fatalf("usage event count = %d, want 1", usageCount)
	}

	if err := manager.handlePiRuntimeEvent(dispatch, piRPCEvent{
		Type: "session_info_changed", Raw: json.RawMessage(`{"type":"session_info_changed","name":"Native name"}`),
	}); err != nil {
		t.Fatalf("handle session info change: %v", err)
	}
	if err := manager.handlePiRuntimeEvent(dispatch, piRPCEvent{
		Type: "thinking_level_changed", Raw: json.RawMessage(`{"type":"thinking_level_changed","level":"high"}`),
	}); err != nil {
		t.Fatalf("handle thinking level change: %v", err)
	}
	record, err = manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Title != "Native name" || record.TitleAuto || record.ReasoningEffort != string(ReasoningEffortHigh) {
		t.Fatalf("native state changes were not projected: title=%q auto=%v reasoning=%q", record.Title, record.TitleAuto, record.ReasoningEffort)
	}

	// The same cumulative usage snapshot is common across adjacent Pi events;
	// it should not create duplicate persisted usage events.
	if err := manager.handlePiRuntimeEvent(dispatch, usageEvent); err != nil {
		t.Fatalf("deduplicated usage update: %v", err)
	}
	if countEventsByType(readTextDeltaTestEvents(t, manager, session.ID), "usage") != 1 {
		t.Fatal("duplicate Pi usage snapshot was persisted")
	}
}
