package websession

import "testing"

func TestHistoryItemUpdatesSubAgentActivitySkipsTransportRetry(t *testing.T) {
	retry := HistoryItem{
		ItemType: "note",
		Text:     "Reconnecting... 1/5",
		Payload:  map[string]any{"code": "transport_retrying"},
	}
	if historyItemUpdatesSubAgentActivity(retry) {
		t.Fatal("transport retry must not replace the sub-agent's latest activity")
	}

	fileChange := HistoryItem{
		ItemType: "file_change",
		Tool:     &HistoryTool{Kind: "file_change"},
	}
	if !historyItemUpdatesSubAgentActivity(fileChange) {
		t.Fatal("file changes must update the sub-agent's latest activity")
	}
}

func TestCodexSubAgentErrorPayloadPreservesActivityWhileRetrying(t *testing.T) {
	payload := codexSubAgentErrorPayload("Reconnecting... 1/5 (unexpected status 502 Bad Gateway)")
	if got := stringValue(payload["status"]); got != string(WebSessionSubAgentRunning) {
		t.Fatalf("retry status = %q, want %q", got, WebSessionSubAgentRunning)
	}
	if _, exists := payload["summary"]; exists {
		t.Fatalf("retry payload must not replace the latest activity: %#v", payload)
	}

	payload = codexSubAgentErrorPayload("tool execution failed")
	if got := stringValue(payload["status"]); got != string(WebSessionSubAgentErrored) {
		t.Fatalf("error status = %q, want %q", got, WebSessionSubAgentErrored)
	}
	if got := stringValue(payload["summary"]); got != "tool execution failed" {
		t.Fatalf("error summary = %q", got)
	}
}

func TestCodexTurnCompletionDoesNotEndSubAgentLifecycle(t *testing.T) {
	if got := codexTurnSubAgentStatus("completed"); got != "" {
		t.Fatalf("completed turn must not produce a lifecycle status, got %q", got)
	}
	if got := codexTurnSubAgentStatus("inProgress"); got != WebSessionSubAgentRunning {
		t.Fatalf("in-progress turn status = %q, want %q", got, WebSessionSubAgentRunning)
	}
	if got := codexTurnSubAgentStatus("failed"); got != WebSessionSubAgentErrored {
		t.Fatalf("failed turn status = %q, want %q", got, WebSessionSubAgentErrored)
	}
}

func TestSubAgentThreadIdleAndCompletedTurnRemainActive(t *testing.T) {
	status := subAgentStatusFromThreadSummary(
		codexThreadSummary{Status: "idle"},
		[]map[string]any{{"id": "turn-child", "status": "completed"}},
	)
	if status != WebSessionSubAgentRunning {
		t.Fatalf("idle thread with completed turn = %q, want %q", status, WebSessionSubAgentRunning)
	}

	status = subAgentStatusFromThreadSummary(
		codexThreadSummary{},
		[]map[string]any{{"id": "turn-child", "status": "completed"}},
	)
	if status != WebSessionSubAgentRunning {
		t.Fatalf("completed turn without thread status = %q, want %q", status, WebSessionSubAgentRunning)
	}

	status = subAgentStatusFromThreadSummary(
		codexThreadSummary{Status: "idle"},
		[]map[string]any{{"id": "turn-child", "status": "failed"}},
	)
	if status != WebSessionSubAgentErrored {
		t.Fatalf("idle thread with failed turn = %q, want %q", status, WebSessionSubAgentErrored)
	}

	status = subAgentStatusFromThreadSummary(
		codexThreadSummary{Status: "systemError"},
		nil,
	)
	if status != WebSessionSubAgentErrored {
		t.Fatalf("system-error thread = %q, want %q", status, WebSessionSubAgentErrored)
	}

	status = subAgentStatusFromThreadSummary(
		codexThreadSummary{Status: "closed"},
		nil,
	)
	if status != WebSessionSubAgentShutdown {
		t.Fatalf("closed thread = %q, want %q", status, WebSessionSubAgentShutdown)
	}
}
