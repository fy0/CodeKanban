package websession

import (
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
