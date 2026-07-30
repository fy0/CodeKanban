package websession

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseCodexCollaborationCallSanitizesEncryptedMessage(t *testing.T) {
	item := map[string]any{
		"type":      "function_call",
		"namespace": "collaboration",
		"name":      "spawn_agent",
		"call_id":   "call-1",
		"arguments": `{"message":"encrypted-secret","task_name":"review","fork_turns":"all","model":"gpt-test"}`,
	}
	call, ok := parseCodexCollaborationCall("thread-1", "turn-1", item, time.Now())
	if !ok {
		t.Fatal("expected collaboration call")
	}
	encoded, err := json.Marshal(call.Input)
	if err != nil {
		t.Fatalf("marshal sanitized input: %v", err)
	}
	if strings.Contains(string(encoded), "encrypted-secret") || strings.Contains(string(encoded), "message") {
		t.Fatalf("encrypted message leaked into input: %s", encoded)
	}
	if call.Input["task_name"] != "review" || call.Input["tool"] != "spawnAgent" {
		t.Fatalf("unexpected sanitized input: %#v", call.Input)
	}
}

func TestParseCodexCollaborationCallAcceptsV2DirectToolFallback(t *testing.T) {
	call, ok := parseCodexCollaborationCall("thread-1", "turn-1", map[string]any{
		"type":      "function_call",
		"name":      "send_message",
		"call_id":   "call-direct",
		"arguments": `{"target":"/root","message":"encrypted-secret"}`,
	}, time.Now())
	if !ok {
		t.Fatal("expected namespace-free V2 direct tool to parse")
	}
	if call.Name != "send_message" || call.Input["target"] != "/root" {
		t.Fatalf("unexpected parsed call: %#v", call)
	}
	if encoded := mustJSONText(call.Input); strings.Contains(encoded, "encrypted-secret") || strings.Contains(encoded, "message") {
		t.Fatalf("encrypted message leaked into direct V2 input: %s", encoded)
	}
	if _, ok := parseCodexCollaborationCall("thread-1", "turn-1", map[string]any{
		"type": "function_call", "name": "unrelated_tool", "call_id": "call-other", "arguments": `{}`,
	}, time.Now()); ok {
		t.Fatal("unrelated namespace-free function must remain a generic tool")
	}
}

func TestCodexCollaborationTrackerEmitsOnlyUncoveredFailure(t *testing.T) {
	makeCall := func(callID string) codexCollaborationCall {
		return codexCollaborationCall{
			ThreadID:  "thread-1",
			TurnID:    "turn-1",
			CallID:    callID,
			Name:      "spawn_agent",
			Input:     map[string]any{"tool": "spawnAgent"},
			StartedAt: time.Now(),
		}
	}

	var tracker codexCollaborationTracker
	if !tracker.record(makeCall("call-failed")) {
		t.Fatal("expected failed call to be recorded")
	}
	failedCall, output, emit, handled := tracker.resolve(
		"thread-1",
		"turn-1",
		"call-failed",
		"failed to parse function arguments: missing field message",
	)
	if !handled || !emit || !strings.Contains(output, "missing field") {
		t.Fatalf("unexpected failure resolution: handled=%v emit=%v output=%q", handled, emit, output)
	}
	_, _, emit, handled = tracker.resolve(
		"thread-1",
		"turn-1",
		"call-failed",
		"failed to parse function arguments: missing field message",
	)
	if !handled || emit {
		t.Fatalf("in-flight failure must suppress duplicate projection: handled=%v emit=%v", handled, emit)
	}
	tracker.finishFailure(failedCall, false)
	failedCall, _, emit, handled = tracker.resolve(
		"thread-1",
		"turn-1",
		"call-failed",
		"failed to parse function arguments: missing field message",
	)
	if !handled || !emit {
		t.Fatalf("failed persistence must release the call for retry: handled=%v emit=%v", handled, emit)
	}
	tracker.finishFailure(failedCall, true)

	if !tracker.record(makeCall("call-covered")) {
		t.Fatal("expected covered call to be recorded")
	}
	tracker.markCovered("thread-1", "turn-1", "call-covered")
	_, _, emit, handled = tracker.resolve("thread-1", "turn-1", "call-covered", "unexpected output")
	if !handled || emit {
		t.Fatalf("covered call should not emit fallback: handled=%v emit=%v", handled, emit)
	}

	if !tracker.record(makeCall("call-success")) {
		t.Fatal("expected successful call to be recorded")
	}
	_, _, emit, handled = tracker.resolve(
		"thread-1",
		"turn-1",
		"call-success",
		`{"task_name":"/root/review"}`,
	)
	if !handled || emit {
		t.Fatalf("schema-valid success should not emit fallback: handled=%v emit=%v", handled, emit)
	}
}

func TestCodexCollaborationOutputTextOmitsEncryptedContent(t *testing.T) {
	output := []any{
		map[string]any{"type": "encrypted_content", "data": "secret"},
		map[string]any{"type": "input_text", "text": "visible error"},
		map[string]any{"type": "input_image", "image_url": "secret-image"},
	}
	if got := codexCollaborationOutputText(output); got != "visible error" {
		t.Fatalf("unexpected output text %q", got)
	}
}

func TestCodexCollaborationOutputSuccessSchemas(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "spawn_agent", output: `{"task_name":"/root/review"}`, want: true},
		{name: "spawn_agent", output: "missing field message", want: false},
		{name: "send_message", output: "", want: true},
		{name: "followup_task", output: "target not found", want: false},
		{name: "interrupt_agent", output: `{"previous_status":"running"}`, want: true},
		{name: "wait_agent", output: `{"message":"Wait timed out.","timed_out":true}`, want: true},
		{name: "wait_agent", output: "timeout_ms must be at least 10000", want: false},
	}
	for _, test := range tests {
		t.Run(test.name+test.output, func(t *testing.T) {
			if got := codexCollaborationOutputSucceeded(test.name, test.output); got != test.want {
				t.Fatalf("codexCollaborationOutputSucceeded(%q, %q)=%v want %v", test.name, test.output, got, test.want)
			}
		})
	}
}
