package websession

import (
	"context"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
)

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

func TestCodexTurnCompletionMarksReusableSubAgentIdle(t *testing.T) {
	if got := codexTurnSubAgentStatus("completed"); got != WebSessionSubAgentIdle {
		t.Fatalf("completed turn status = %q, want %q", got, WebSessionSubAgentIdle)
	}
	if got := codexTurnSubAgentStatus("inProgress"); got != WebSessionSubAgentRunning {
		t.Fatalf("in-progress turn status = %q, want %q", got, WebSessionSubAgentRunning)
	}
	if got := codexTurnSubAgentStatus("failed"); got != WebSessionSubAgentErrored {
		t.Fatalf("failed turn status = %q, want %q", got, WebSessionSubAgentErrored)
	}
}

func TestSubAgentThreadIdleAndCompletedTurnAreNotActive(t *testing.T) {
	status := subAgentStatusFromThreadSummary(
		codexThreadSummary{Status: "idle"},
		[]map[string]any{{"id": "turn-child", "status": "completed"}},
	)
	if status != WebSessionSubAgentIdle || webSessionSubAgentIsActive(status) {
		t.Fatalf("idle thread with completed turn = %q, want inactive %q", status, WebSessionSubAgentIdle)
	}

	status = subAgentStatusFromThreadSummary(
		codexThreadSummary{},
		[]map[string]any{{"id": "turn-child", "status": "completed"}},
	)
	if status != WebSessionSubAgentIdle || webSessionSubAgentIsActive(status) {
		t.Fatalf("completed turn without thread status = %q, want inactive %q", status, WebSessionSubAgentIdle)
	}

	status = subAgentStatusFromThreadSummary(
		codexThreadSummary{Status: "active"},
		[]map[string]any{
			{"id": "turn-older", "status": "interrupted"},
			{"id": "turn-latest", "status": "completed"},
		},
	)
	if status != WebSessionSubAgentIdle {
		t.Fatalf("latest completed turn must win over older interruption, got %q", status)
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

func TestApplySubAgentPayloadClearsCompletedTurnWithoutEndingThread(t *testing.T) {
	currentTurnID := "turn-child"
	row := tables.WebSessionSubAgentTable{
		ThreadID:      "thread-child",
		Status:        string(WebSessionSubAgentRunning),
		CurrentTurnID: &currentTurnID,
	}
	eventTime := time.Date(2026, 8, 26, 4, 0, 0, 0, time.UTC)
	applySubAgentPayload(&row, map[string]any{
		"turnCompleted": true,
		"status":        string(WebSessionSubAgentIdle),
	}, Event{Timestamp: eventTime})
	if row.Status != string(WebSessionSubAgentIdle) || row.CurrentTurnID != nil || row.EndedAt != nil {
		t.Fatalf("completed turn must leave an idle reusable thread, got %#v", row)
	}

	endedAt := eventTime.Add(time.Minute)
	row.Status = string(WebSessionSubAgentInterrupted)
	row.EndedAt = &endedAt
	applySubAgentPayload(&row, map[string]any{
		"activeTurn": true,
		"turnId":     "late-turn-item",
	}, Event{Timestamp: endedAt.Add(time.Minute)})
	if row.Status != string(WebSessionSubAgentInterrupted) || row.CurrentTurnID != nil || row.EndedAt != &endedAt {
		t.Fatalf("late activity must not revive a terminal thread, got %#v", row)
	}
}

func TestSessionSubAgentsFiltersNativeRootAndSelfParent(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Sub Agent filtering", 1000)
	rootThreadID := "thread-root"
	session.NativeSessionID = &rootThreadID
	if err := model.GetDB().Save(session).Error; err != nil {
		t.Fatalf("save native root: %v", err)
	}
	selfParentID := "thread-child"
	manager := &Manager{}
	if err := manager.replaceSessionSubAgents(context.Background(), session.ID, []WebSessionSubAgent{
		{ThreadID: rootThreadID, Status: WebSessionSubAgentRunning},
		{ThreadID: "thread-child", ParentThreadID: &selfParentID, Status: WebSessionSubAgentIdle},
	}, true); err != nil {
		t.Fatalf("seed sub-agent registry: %v", err)
	}

	agents, err := manager.sessionSubAgents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("read sub-agent registry: %v", err)
	}
	if len(agents) != 1 || agents[0].ThreadID != "thread-child" || agents[0].ParentThreadID != nil {
		t.Fatalf("expected only sanitized child entry, got %#v", agents)
	}
}
