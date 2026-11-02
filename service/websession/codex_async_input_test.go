package websession

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func asyncQuestionTestItem() map[string]any {
	return map[string]any{
		"type": "agentMessage", "id": "call-async", "text": "Which deployment?\n- Subdirectory\n- Copy folder",
		"phase": "final_answer", "delivery": "async",
		"questions": []map[string]any{
			{"title": "Which deployment?", "options": []string{"Subdirectory", "Copy folder"}},
			{"title": "What is the error?"},
		},
	}
}

func TestCodexAsyncInputProjectsWithoutBlockingOrCompletingRun(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Async questions", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	run := &activeRun{runID: "run-1", agent: AgentCodex}
	scope := codexTurnScope{threadID: "thread-1", turnID: "turn-1"}
	for _, method := range []string{"item/started", "item/completed"} {
		handleCodexTestNotification(t, manager, *session, run, scope, method, map[string]any{
			"threadId": "thread-1", "turnId": "turn-1", "item": asyncQuestionTestItem(),
		})
	}
	if run.hasPendingServerRequest() || run.completedReplySeen() {
		t.Fatal("async question must neither block input nor count as the final reply")
	}
	history, err := manager.loadHistoryWindow(context.Background(), session.ID, 80, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 1 || history.Items[0].Detail == nil {
		t.Fatalf("expected one structured question card: %#v", history.Items)
	}
	item := history.Items[0]
	if item.Detail.Type != codexAsyncInputDetailType || len(item.Detail.Questions) != 2 || !item.Detail.Questions[1].IsOther {
		t.Fatalf("lost async questions: %#v", item.Detail)
	}
	wire := mapWireHistoryDetail(item.Detail)
	if wire.Type != codexAsyncInputDetailType || len(wire.Questions) != 2 {
		t.Fatalf("lost async questions in transport: %#v", wire)
	}
	fast, err := manager.mapThreadReadItem(asyncQuestionTestItem(), 1)
	if err != nil || fast.Detail == nil || len(fast.Detail.Questions) != 2 {
		t.Fatalf("lost async questions in fast sync: %#v, %v", fast, err)
	}
	if pointerString(fast.SourceItemID) != pointerString(item.SourceItemID) {
		t.Fatal("fast sync must retain the live question identity")
	}
}

func TestCodexAsyncInputDeepHistoryDeduplicatesToolAndMessage(t *testing.T) {
	manager := &Manager{}
	item := asyncQuestionTestItem()
	// The native item includes content rather than the app-server text field.
	delete(item, "text")
	item["type"] = "AgentMessage"
	args := map[string]any{"questions": item["questions"]}
	rows := []map[string]any{
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "request_user_input_async", "call_id": "call-async", "arguments": mustJSONText(args),
		}},
		{"type": "event_msg", "payload": map[string]any{
			"type": "item_completed", "thread_id": "thread-1", "turn_id": "turn-1", "item": item,
		}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call_output", "call_id": "call-async", "output": `{"accepted":true}`,
		}},
	}
	var lines []string
	for _, row := range rows {
		row["timestamp"] = "2026-11-02T08:03:07Z"
		lines = append(lines, mustJSONText(row))
	}
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := manager.parseCodexDeepHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Detail == nil || len(items[0].Detail.Questions) != 2 {
		t.Fatalf("expected a single structured card, got %#v", items)
	}
	if strings.Contains(items[0].Text, "accepted") {
		t.Fatal("tool receipt is not a user answer")
	}
}

func TestCodexAsyncInputRepliesSteerRunningTurnAndPersist(t *testing.T) {
	cleanup := initTestDB(t)
	t.Cleanup(cleanup)
	project := seedProject(t)
	codexPath := writeFakeCodexAppServerCLI(t, "step_redirect")
	manager, err := NewManager(Config{DataDir: t.TempDir(), CodexPath: codexPath}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.CreateSession(context.Background(), CreateParams{ProjectID: project.ID, Agent: AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if manager.hasActiveRun(session.ID) {
			_ = manager.AbortSession(session.ID)
		}
		waitForSessionToSettle(t, manager, session.ID)
	})
	if err := manager.SendMessage(context.Background(), session.ID, "first", nil); err != nil {
		t.Fatal(err)
	}
	waitForHistoryToolEvent(t, manager, session.ID, "cmd_step_2", "tool_st")
	item, err := manager.appendHistoryItem(context.Background(), session.ID, codexAsyncInputHistoryItem(asyncQuestionTestItem(), time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	answers := map[string][]string{"0": {"Copy folder"}, "1": {"404 on assets"}}
	if err := manager.respondToCodexAsyncInput(context.Background(), session.ID, item.ID, map[string][]string{"0": {"Copy folder"}}); err == nil {
		t.Fatal("missing answers must leave the question open")
	}
	if err := manager.respondToCodexAsyncInput(context.Background(), session.ID, item.ID, answers); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, codexPath+".state.steer.json")
	if err := manager.respondToCodexAsyncInput(context.Background(), session.ID, item.ID, answers); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(codexPath + ".state.steer.json")
	if err != nil {
		t.Fatal(err)
	}
	var steer map[string]any
	if err := json.Unmarshal(raw, &steer); err != nil {
		t.Fatal(err)
	}
	if text := mustJSONText(steer["input"]); !strings.Contains(text, "Which deployment?") || !strings.Contains(text, "404 on assets") {
		t.Fatalf("reply must include both questions and answers: %s", text)
	}
	updated, err := manager.findHistoryItemByID(context.Background(), session.ID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Detail.Action != "submitted" || len(updated.Detail.Answers) != 2 {
		t.Fatalf("reply not persisted: %#v", updated.Detail)
	}
	if !manager.hasActiveRun(session.ID) {
		t.Fatal("reply must keep the active turn running")
	}
	// Deep synchronization must preserve the locally submitted answer status.
	merged := mergeReconciledHistoryItem(updated, item)
	if merged.Detail.Action != "submitted" {
		t.Fatal("deep sync reopened an answered question")
	}
	attempts, err := os.ReadFile(codexPath + ".state.steer-attempts")
	if err != nil || string(attempts) != "1" {
		t.Fatalf("duplicate reply was sent: %s, %v", attempts, err)
	}
}

func TestCodexAsyncInputReplyStartsIdleSession(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Finished questions", 1000)
	if err := model.GetDB().Model(session).Update("backend", string(SessionBackendCodexAppServer)).Error; err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir(), CodexPath: writeFakeCodexAppServerCLI(t, "basic")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	item, err := manager.appendHistoryItem(context.Background(), session.ID, codexAsyncInputHistoryItem(asyncQuestionTestItem(), time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.respondToCodexAsyncInput(context.Background(), session.ID, item.ID, map[string][]string{"0": {"Subdirectory"}, "1": {"404"}}); err != nil {
		t.Fatal(err)
	}
	waitForSessionToSettle(t, manager, session.ID)
	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	texts := userMessageTexts(events)
	if len(texts) != 1 || !strings.Contains(texts[0], "Subdirectory") {
		t.Fatalf("missing user reply: %#v", texts)
	}
	// The reply must not leak across sessions, including requests with a real row id.
	other := seedWebSession(t, project.ID, "Other session", 2000)
	if err := manager.respondToCodexAsyncInput(context.Background(), other.ID, item.ID, map[string][]string{"0": {"x"}}); err == nil {
		t.Fatal("accepted another session's question")
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).Where("id = ?", session.ID).Update("archived_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.respondToCodexAsyncInput(context.Background(), session.ID, item.ID, map[string][]string{"0": {"x"}}); err == nil {
		t.Fatal("accepted reply in an archived session")
	}
}
