package websession

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestParseCodexDeepHistoryCapturesToolsAndTimestamps(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T01:00:00Z","type":"session_meta","payload":{"id":"session-1","timestamp":"2026-04-09T01:00:00Z","cwd":"/tmp/test"}}`,
		`{"timestamp":"2026-04-09T01:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"inspect the repo","images":[]}}`,
		`{"timestamp":"2026-04-09T01:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\",\"workdir\":\"/tmp/test\"}","call_id":"call_1"}}`,
		`{"timestamp":"2026-04-09T01:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"Command: /bin/bash -lc pwd\nProcess exited with code 0\nOutput:\n/tmp/test\n"}}`,
		`{"timestamp":"2026-04-09T01:00:04Z","type":"response_item","payload":{"type":"reasoning","summary":["Need to inspect sync paths"],"content":null}}`,
		`{"timestamp":"2026-04-09T01:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"I found the sync entrypoints."}}`,
	})

	items, err := manager.parseCodexDeepHistory(filePath)
	if err != nil {
		t.Fatalf("parseCodexDeepHistory returned error: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 history items, got %d", len(items))
	}
	if items[0].Kind != "user" || items[0].Text != "inspect the repo" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].Tool == nil {
		t.Fatalf("expected tool item at index 1, got %#v", items[1])
	}
	if items[1].Tool.Kind != "command_execution" {
		t.Fatalf("expected command_execution kind, got %q", items[1].Tool.Kind)
	}
	if items[1].Tool.Status != "done" {
		t.Fatalf("expected tool status done, got %q", items[1].Tool.Status)
	}
	if items[1].Timestamp == nil || items[1].Timestamp.Format(time.RFC3339) != "2026-04-09T01:00:02Z" {
		t.Fatalf("expected tool timestamp from function_call, got %#v", items[1].Timestamp)
	}
	if items[1].ObservedAt == nil || items[1].ObservedAt.Format(time.RFC3339) != "2026-04-09T01:00:03Z" {
		t.Fatalf("expected tool observedAt from function_call_output, got %#v", items[1].ObservedAt)
	}
	if items[2].Tool == nil || items[2].Tool.Kind != "reasoning" {
		t.Fatalf("expected reasoning tool at index 2, got %#v", items[2])
	}
	if items[3].Kind != "assistant" || items[3].Text != "I found the sync entrypoints." {
		t.Fatalf("unexpected last item: %#v", items[3])
	}
}

func TestParseCodexDeepHistoryCapturesMessageOnlyCyberPolicyError(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	message := "This content was flagged for possible cybersecurity risk. If this seems wrong, try rephrasing your request."
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-05-03T12:33:18Z","type":"event_msg","payload":{"type":"user_message","message":"inspect this","images":[]}}`,
		`{"timestamp":"2026-05-03T12:33:19Z","type":"event_msg","payload":{"type":"error","message":` + strconv.Quote(message) + `}}`,
	})

	result, err := manager.parseCodexDeepHistoryWithStats(filePath)
	if err != nil {
		t.Fatalf("parseCodexDeepHistoryWithStats returned error: %v", err)
	}
	if !result.Stats.CyberPolicyFlagged {
		t.Fatal("expected cyber policy flag in deep history stats")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected user message and cyber policy error, got %#v", result.Items)
	}
	item := result.Items[1]
	if item.Kind != "system" || item.ItemType != "cyber_policy" || item.Level != "error" {
		t.Fatalf("unexpected cyber policy history item: %#v", item)
	}
	if item.Text != message || codexErrorInfo(item.Payload) != "" {
		t.Fatalf("unexpected cyber policy history payload: %#v", item)
	}

	updates := map[string]any{}
	applyCodexDeepHistoryStatsUpdates(updates, result.Stats)
	if flagged, ok := updates["cyber_policy_flagged"].(bool); !ok || !flagged {
		t.Fatalf("expected deep sync update to persist cyber policy flag, got %#v", updates)
	}
}

func TestParseCodexDeepHistoryFiltersIncompleteTurnContinuationPrompt(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T01:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"inspect the repo","images":[]}}`,
		`{"timestamp":"2026-04-09T01:00:01Z","type":"event_msg","payload":{"type":"user_message","message":` + strconv.Quote(incompleteTurnContinuationPrompt) + `,"images":[]}}`,
		`{"timestamp":"2026-04-09T01:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":` + strconv.Quote(incompleteTurnContinuationPrompt) + `}]}}`,
		`{"timestamp":"2026-04-09T01:00:03Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}`,
	})

	items, err := manager.parseCodexDeepHistory(filePath)
	if err != nil {
		t.Fatalf("parseCodexDeepHistory returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected only the real user and assistant messages, got %#v", items)
	}
	if items[0].Kind != "user" || items[0].Text != "inspect the repo" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].Kind != "assistant" || items[1].Text != "done" {
		t.Fatalf("unexpected final item: %#v", items[1])
	}
}

func TestMapThreadReadItemFiltersIncompleteTurnContinuationPrompt(t *testing.T) {
	manager := &Manager{}

	item, err := manager.mapThreadReadItem(map[string]any{
		"id":   "internal_continue",
		"type": "userMessage",
		"content": []any{
			map[string]any{"type": "text", "text": incompleteTurnContinuationPrompt},
		},
	}, 1)
	if err != nil {
		t.Fatalf("mapThreadReadItem returned error: %v", err)
	}
	if item.Kind != "" || item.Text != "" {
		t.Fatalf("expected internal continuation prompt to be filtered, got %#v", item)
	}
}

func TestParseCodexDeepHistoryCapturesPlanFromCompletedEvent(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	planText := testLongPlanText()
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T01:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"plan this change","images":[]}}`,
		`{"timestamp":"2026-04-09T01:00:01Z","type":"event_msg","payload":{"type":"item_completed","thread_id":"thread-1","turn_id":"turn-1","item":{"type":"Plan","id":"plan_test","text":` + strconv.Quote(planText) + `}}}`,
	})

	items, err := manager.parseCodexDeepHistory(filePath)
	if err != nil {
		t.Fatalf("parseCodexDeepHistory returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(items))
	}
	if items[1].Tool == nil {
		t.Fatalf("expected plan tool item, got %#v", items[1])
	}
	if items[1].Tool.Kind != "plan" {
		t.Fatalf("expected plan kind, got %q", items[1].Tool.Kind)
	}
	if items[1].Tool.Output != planText {
		t.Fatalf("expected full plan text to be preserved, got length %d want %d", len(items[1].Tool.Output), len(planText))
	}
	if items[1].SourceTurnID == nil || *items[1].SourceTurnID != "turn-1" {
		t.Fatalf("expected source turn id turn-1, got %#v", items[1].SourceTurnID)
	}
	if items[1].SourceItemID == nil || *items[1].SourceItemID != "plan_test" {
		t.Fatalf("expected source item id plan_test, got %#v", items[1].SourceItemID)
	}
}

func TestMapThreadReadItemMapsContextCompactionToToolHistory(t *testing.T) {
	manager := &Manager{}

	item, err := manager.mapThreadReadItem(map[string]any{
		"id":        "compact_1",
		"type":      "contextCompaction",
		"status":    "completed",
		"summary":   []any{"Compacted prior messages."},
		"createdAt": "2026-04-09T01:00:00Z",
	}, 1)
	if err != nil {
		t.Fatalf("mapThreadReadItem returned error: %v", err)
	}
	if item.Kind != "tool" || item.Tool == nil {
		t.Fatalf("expected tool history item, got %#v", item)
	}
	if item.Tool.Kind != "context_compaction" {
		t.Fatalf("expected context_compaction kind, got %q", item.Tool.Kind)
	}
	if item.Tool.Name != "Context Compaction" {
		t.Fatalf("expected context compaction display name, got %q", item.Tool.Name)
	}
	if !strings.Contains(item.Tool.Output, "Compacted prior messages") {
		t.Fatalf("expected compaction output, got %q", item.Tool.Output)
	}
}

func TestParseCodexDeepHistoryMapsContextCompaction(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T01:00:00Z","type":"response_item","payload":{"type":"contextCompaction","id":"compact_1","status":"completed","summary":["Compacted earlier turns into a shorter summary."]}}`,
	})

	items, err := manager.parseCodexDeepHistory(filePath)
	if err != nil {
		t.Fatalf("parseCodexDeepHistory returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}
	if items[0].Tool == nil || items[0].Tool.Kind != "context_compaction" {
		t.Fatalf("expected context_compaction tool item, got %#v", items[0])
	}
	if !strings.Contains(items[0].Tool.Output, "Compacted earlier turns") {
		t.Fatalf("expected compaction summary text, got %q", items[0].Tool.Output)
	}
}

func TestParseCodexDeepHistoryFiltersDeveloperMessagesAndDedupesBothOrders(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T01:00:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"developer prompt"}]}}`,
		`{"timestamp":"2026-04-09T01:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
		`{"timestamp":"2026-04-09T01:00:01.020Z","type":"event_msg","payload":{"type":"user_message","message":"hello","images":[]}}`,
		`{"timestamp":"2026-04-09T01:00:03Z","type":"event_msg","payload":{"type":"agent_message","message":"world"}}`,
		`{"timestamp":"2026-04-09T01:00:03.020Z","type":"response_item","payload":{"type":"message","role":"assistant","id":"msg_world","content":[{"type":"output_text","text":"world"}]}}`,
	})

	items, err := manager.parseCodexDeepHistory(filePath)
	if err != nil {
		t.Fatalf("parseCodexDeepHistory returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 visible history items, got %d", len(items))
	}
	if items[0].Kind != "user" || items[0].Text != "hello" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].Kind != "assistant" || items[1].Text != "world" {
		t.Fatalf("unexpected second item: %#v", items[1])
	}
	if items[1].SourceItemID == nil || *items[1].SourceItemID != "msg_world" {
		t.Fatalf("expected response item id to be preserved, got %#v", items[1].SourceItemID)
	}
}

func TestParseCodexDeepHistoryProjectsVisibleConversation(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	dataDir := t.TempDir()
	manager, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	imagePath := filepath.Join(dataDir, "reference.png")
	if err := os.WriteFile(imagePath, []byte("image"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	userText := "inspect this\n\n[Image #1]"
	responseUserText := userText + "\n<image name=[Image #1] path=\"" + imagePath + "\">\n[input_image:]\n</image>"
	planText := "# Fix history\n\n- Filter protocol messages.\n- Merge duplicate messages."
	proposedPlan := "<proposed_plan>\n" + planText + "\n</proposed_plan>"
	hostContext := "# AGENTS.md instructions for D:\\\\repo\n\n<INSTRUCTIONS>\ninternal\n</INSTRUCTIONS>\n<environment_context>internal</environment_context>"
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T01:00:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<collaboration_mode># Plan Mode</collaboration_mode>"}]}}`,
		`{"timestamp":"2026-04-09T01:00:00.005Z","type":"response_item","payload":{"type":"message","role":"system","content":[{"type":"input_text","text":"internal system prompt"}]}}`,
		`{"timestamp":"2026-04-09T01:00:00.010Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":` + strconv.Quote(hostContext) + `}]}}`,
		`{"timestamp":"2026-04-09T01:00:00.020Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>internal</environment_context>"}]}}`,
		`{"timestamp":"2026-04-09T01:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":` + strconv.Quote(responseUserText) + `}]}}`,
		`{"timestamp":"2026-04-09T01:00:01.020Z","type":"event_msg","payload":{"type":"user_message","message":` + strconv.Quote(userText) + `,"images":[` + strconv.Quote(imagePath) + `]}}`,
		`{"timestamp":"2026-04-09T01:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"working","phase":"commentary"}}`,
		`{"timestamp":"2026-04-09T01:00:02.020Z","type":"response_item","payload":{"type":"message","role":"assistant","id":"msg_working","phase":"commentary","content":[{"type":"output_text","text":"working"}]}}`,
		`{"timestamp":"2026-04-09T01:00:03Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn_plan","item":{"type":"Plan","id":"plan_1","text":` + strconv.Quote(planText) + `}}}`,
		`{"timestamp":"2026-04-09T01:00:03.020Z","type":"response_item","payload":{"type":"message","role":"assistant","id":"msg_plan","content":[{"type":"output_text","text":` + strconv.Quote(proposedPlan) + `}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn_plan"}}}`,
		`{"timestamp":"2026-04-09T01:00:04Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<collaboration_mode># Collaboration Mode: Default</collaboration_mode>"}]}}`,
		`{"timestamp":"2026-04-09T01:00:04.010Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Implement the plan."}]}}`,
		`{"timestamp":"2026-04-09T01:00:04.020Z","type":"event_msg","payload":{"type":"user_message","message":"Implement the plan.","images":[]}}`,
		`{"timestamp":"2026-04-09T01:00:10Z","type":"event_msg","payload":{"type":"user_message","message":"repeat","images":[]}}`,
		`{"timestamp":"2026-04-09T01:00:10.100Z","type":"event_msg","payload":{"type":"user_message","message":"repeat","images":[]}}`,
	})

	items, err := manager.parseCodexDeepHistory(filePath)
	if err != nil {
		t.Fatalf("parseCodexDeepHistory returned error: %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("expected 6 visible history items, got %d: %#v", len(items), items)
	}
	if items[0].Kind != "user" || items[0].Text != userText || len(items[0].Attachments) != 1 {
		t.Fatalf("unexpected image user item: %#v", items[0])
	}
	if items[0].Attachments[0].Path != imagePath {
		t.Fatalf("expected image attachment path %q, got %#v", imagePath, items[0].Attachments)
	}
	if items[1].Kind != "assistant" || items[1].Text != "working" {
		t.Fatalf("unexpected assistant item: %#v", items[1])
	}
	if items[2].Tool == nil || items[2].Tool.Kind != "plan" || items[2].Tool.Output != planText {
		t.Fatalf("unexpected plan item: %#v", items[2])
	}
	if items[3].Kind != "user" || items[3].Text != "Implement the plan." {
		t.Fatalf("unexpected implementation message: %#v", items[3])
	}
	if items[4].Text != "repeat" || items[5].Text != "repeat" {
		t.Fatalf("same-source repeated messages must be preserved: %#v", items[4:])
	}
	for _, item := range items {
		if strings.Contains(item.Text, "collaboration_mode") || strings.Contains(item.Text, "AGENTS.md instructions") {
			t.Fatalf("internal protocol message leaked into history: %#v", item)
		}
	}
}

func TestSyncSessionFromLogSourceReplacesCacheAndMarksDeepSync(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Deep Sync Session", 1000)

	nativeSessionID := "session-deep-sync"
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"native_session_id": nativeSessionID,
			"cwd":               project.Path,
		}).Error; err != nil {
		t.Fatalf("failed to update web session: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T02:00:00Z","type":"session_meta","payload":{"id":"session-deep-sync","timestamp":"2026-04-09T02:00:00Z","cwd":"` + filepath.ToSlash(project.Path) + `"}}`,
		`{"timestamp":"2026-04-09T02:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"run deep sync","images":[]}}`,
		`{"timestamp":"2026-04-09T02:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\",\"workdir\":\"` + filepath.ToSlash(project.Path) + `\"}","call_id":"call_sync"}}`,
		`{"timestamp":"2026-04-09T02:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_sync","output":"Command: /bin/bash -lc pwd\nOutput:\n` + filepath.ToSlash(project.Path) + `\n"}}`,
		`{"timestamp":"2026-04-09T02:00:04Z","type":"event_msg","payload":{"type":"agent_message","message":"deep sync finished"}}`,
	})
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}

	now := time.Now()
	aiRecord := tables.AISessionTable{
		SessionID:             nativeSessionID,
		Type:                  tables.AISessionTypeCodex,
		ProjectPath:           project.Path,
		FilePath:              filePath,
		Model:                 "gpt-5.4",
		Title:                 "run deep sync",
		SessionStartedAt:      time.Date(2026, 4, 9, 2, 0, 0, 0, time.UTC),
		LastMessageAt:         ptr(time.Date(2026, 4, 9, 2, 0, 4, 0, time.UTC)),
		MessageCount:          1,
		AssistantMessageCount: 1,
		FileModTime:           info.ModTime(),
		FileSize:              info.Size(),
	}
	aiRecord.Init()
	aiRecord.CreatedAt = now
	aiRecord.UpdatedAt = now
	if err := model.GetDB().Create(&aiRecord).Error; err != nil {
		t.Fatalf("failed to seed ai session record: %v", err)
	}

	refreshedSession, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}

	snapshot, err := manager.syncSessionFromLogSource(context.Background(), refreshedSession, true, false)
	if err != nil {
		t.Fatalf("syncSessionFromLogSource returned error: %v", err)
	}
	if snapshot.Session.LastSyncMode != SyncModeDeep {
		t.Fatalf("expected last sync mode deep, got %q", snapshot.Session.LastSyncMode)
	}
	if snapshot.Session.ItemCount != 3 {
		t.Fatalf("expected 3 cached items, got %d", snapshot.Session.ItemCount)
	}
	if len(snapshot.History.Items) != 3 {
		t.Fatalf("expected 3 history items, got %d", len(snapshot.History.Items))
	}
	if snapshot.History.Items[1].Tool == nil || snapshot.History.Items[1].Tool.Kind != "command_execution" {
		t.Fatalf("expected cached command_execution tool, got %#v", snapshot.History.Items[1])
	}
}

func TestSyncSessionFromLogSourceUsesHiddenTokenCountUsage(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Hidden Usage Session", 1000)
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T04:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-1","model_context_window":258400}}`,
		`{"timestamp":"2026-04-09T04:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"inspect usage","images":[]}}`,
		`{"timestamp":"2026-04-09T04:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":14537402,"cached_input_tokens":13527296,"output_tokens":66916,"total_tokens":14604318},"last_token_usage":{"input_tokens":207171,"cached_input_tokens":4352,"output_tokens":1345,"total_tokens":208516},"model_context_window":258400}}}`,
		`{"timestamp":"2026-04-09T04:00:03Z","type":"compacted","payload":{"message":"Compacted earlier turns into a summary."}}`,
		`{"timestamp":"2026-04-09T04:00:03Z","type":"event_msg","payload":{"type":"context_compacted"}}`,
		`{"timestamp":"2026-04-09T04:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":14537402,"cached_input_tokens":13527296,"output_tokens":66916,"total_tokens":14604318},"last_token_usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"total_tokens":12656},"model_context_window":258400}}}`,
		`{"timestamp":"2026-04-09T04:00:05Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":14600000,"cached_input_tokens":13550000,"output_tokens":67000,"total_tokens":14667000},"last_token_usage":{"input_tokens":59571,"cached_input_tokens":59000,"output_tokens":199,"total_tokens":59770},"model_context_window":258400}}}`,
	})
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"thread_path": filePath,
			"cwd":         project.Path,
		}).Error; err != nil {
		t.Fatalf("failed to update web session: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	refreshedSession, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}

	snapshot, err := manager.syncSessionFromLogSource(context.Background(), refreshedSession, true, false)
	if err != nil {
		t.Fatalf("syncSessionFromLogSource returned error: %v", err)
	}

	compactionCount := 0
	for _, item := range snapshot.History.Items {
		if item.Tool != nil && item.Tool.Kind == "context_compaction" {
			compactionCount++
		}
	}
	if compactionCount != 1 {
		t.Fatalf("expected one context compaction history item, got %d", compactionCount)
	}
	if snapshot.Session.LastContextCompactionAt == nil {
		t.Fatal("expected lastContextCompactionAt to be recorded")
	}
	if snapshot.Session.ContextEstimateMode != ContextEstimateModeLatestTokenCount {
		t.Fatalf("expected estimate mode %q, got %q", ContextEstimateModeLatestTokenCount, snapshot.Session.ContextEstimateMode)
	}
	if snapshot.Session.ContextEstimate.InputTokens != 59571 ||
		snapshot.Session.ContextEstimate.CachedInputTokens != 59000 ||
		snapshot.Session.ContextEstimate.OutputTokens != 199 ||
		snapshot.Session.ContextEstimate.UsedTokens != 59770 {
		t.Fatalf("unexpected context estimate: %#v", snapshot.Session.ContextEstimate)
	}
	if snapshot.Session.Usage.InputTokens != 14600000 ||
		snapshot.Session.Usage.CachedInputTokens != 13550000 ||
		snapshot.Session.Usage.OutputTokens != 67000 {
		t.Fatalf("unexpected cumulative usage: %#v", snapshot.Session.Usage)
	}
	if snapshot.Session.ContextWindowTokens == nil || *snapshot.Session.ContextWindowTokens != 258400 {
		t.Fatalf("expected session context window 258400, got %#v", snapshot.Session.ContextWindowTokens)
	}
	if snapshot.Session.ContextWindowSource != ContextWindowSourceSessionUsage {
		t.Fatalf("expected context window source %q, got %q", ContextWindowSourceSessionUsage, snapshot.Session.ContextWindowSource)
	}
}

func TestSyncSessionFromLogSourceCachesPlanToolFromCompletedEvent(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Deep Sync Plan Session", 1000)

	nativeSessionID := "session-deep-sync-plan"
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"native_session_id": nativeSessionID,
			"cwd":               project.Path,
		}).Error; err != nil {
		t.Fatalf("failed to update web session: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	planText := testLongPlanText()
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T02:00:00Z","type":"session_meta","payload":{"id":"session-deep-sync-plan","timestamp":"2026-04-09T02:00:00Z","cwd":"` + filepath.ToSlash(project.Path) + `"}}`,
		`{"timestamp":"2026-04-09T02:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"run deep sync","images":[]}}`,
		`{"timestamp":"2026-04-09T02:00:02Z","type":"event_msg","payload":{"type":"item_completed","thread_id":"session-deep-sync-plan","turn_id":"turn-plan","item":{"type":"Plan","id":"plan_test","text":` + strconv.Quote(planText) + `}}}`,
	})
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}

	now := time.Now()
	aiRecord := tables.AISessionTable{
		SessionID:             nativeSessionID,
		Type:                  tables.AISessionTypeCodex,
		ProjectPath:           project.Path,
		FilePath:              filePath,
		Model:                 "gpt-5.4",
		Title:                 "run deep sync",
		SessionStartedAt:      time.Date(2026, 4, 9, 2, 0, 0, 0, time.UTC),
		LastMessageAt:         ptr(time.Date(2026, 4, 9, 2, 0, 2, 0, time.UTC)),
		MessageCount:          1,
		AssistantMessageCount: 0,
		FileModTime:           info.ModTime(),
		FileSize:              info.Size(),
	}
	aiRecord.Init()
	aiRecord.CreatedAt = now
	aiRecord.UpdatedAt = now
	if err := model.GetDB().Create(&aiRecord).Error; err != nil {
		t.Fatalf("failed to seed ai session record: %v", err)
	}

	refreshedSession, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}

	snapshot, err := manager.syncSessionFromLogSource(context.Background(), refreshedSession, true, false)
	if err != nil {
		t.Fatalf("syncSessionFromLogSource returned error: %v", err)
	}
	if len(snapshot.History.Items) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(snapshot.History.Items))
	}
	if snapshot.History.Items[1].Tool == nil || snapshot.History.Items[1].Tool.Kind != "plan" {
		t.Fatalf("expected cached plan tool, got %#v", snapshot.History.Items[1])
	}
	if snapshot.History.Items[1].Tool.Output != planText {
		t.Fatalf("expected full plan text to be preserved, got length %d want %d", len(snapshot.History.Items[1].Tool.Output), len(planText))
	}
}

func TestSnapshotWithAutoSyncFallsBackToLogSourceWhenFastSyncCannotReadThread(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Fallback Session", 1000)

	nativeSessionID := "session-fast-fallback"
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		`{"timestamp":"2026-04-09T03:00:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"developer prompt"}]}}`,
		`{"timestamp":"2026-04-09T03:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello fallback"}]}}`,
		`{"timestamp":"2026-04-09T03:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"fallback works"}]}}`,
	})
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	now := time.Now()
	aiRecord := tables.AISessionTable{
		SessionID:             nativeSessionID,
		Type:                  tables.AISessionTypeCodex,
		ProjectPath:           project.Path,
		FilePath:              filePath,
		Model:                 "gpt-5.4",
		Title:                 "hello fallback",
		SessionStartedAt:      time.Date(2026, 4, 9, 3, 0, 0, 0, time.UTC),
		LastMessageAt:         ptr(time.Date(2026, 4, 9, 3, 0, 2, 0, time.UTC)),
		MessageCount:          2,
		AssistantMessageCount: 1,
		FileModTime:           info.ModTime(),
		FileSize:              info.Size(),
	}
	aiRecord.Init()
	aiRecord.CreatedAt = now
	aiRecord.UpdatedAt = now
	if err := model.GetDB().Create(&aiRecord).Error; err != nil {
		t.Fatalf("failed to seed ai session record: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"native_session_id": nativeSessionID,
			"cwd":               project.Path,
		}).Error; err != nil {
		t.Fatalf("failed to update web session: %v", err)
	}

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: filepath.Join(t.TempDir(), "missing-codex"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	snapshot, err := manager.SnapshotWithAutoSync(context.Background(), session.ID, 80)
	if err != nil {
		t.Fatalf("SnapshotWithAutoSync returned error: %v", err)
	}
	if snapshot.Session.LastSyncMode != SyncModeDeep {
		t.Fatalf("expected fallback sync mode deep, got %q", snapshot.Session.LastSyncMode)
	}
	if snapshot.History.Total != 2 {
		t.Fatalf("expected 2 visible history items after fallback sync, got %d", snapshot.History.Total)
	}
	if len(snapshot.History.Items) != 2 {
		t.Fatalf("expected 2 loaded history items, got %d", len(snapshot.History.Items))
	}
	if snapshot.History.Items[1].Kind != "assistant" || snapshot.History.Items[1].Text != "fallback works" {
		t.Fatalf("unexpected assistant item: %#v", snapshot.History.Items[1])
	}
}

func TestShouldPreserveExistingHistoryOnFastSync(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if !manager.shouldPreserveExistingHistoryOnFastSync(tables.WebSessionTable{LastSyncMode: "deep"}) {
		t.Fatal("expected deep-synced cache to be preserved on fast sync")
	}
	if !manager.shouldPreserveExistingHistoryOnFastSync(tables.WebSessionTable{LastEventSeq: 3}) {
		t.Fatal("expected live event cache to be preserved on fast sync")
	}
	if manager.shouldPreserveExistingHistoryOnFastSync(tables.WebSessionTable{}) {
		t.Fatal("expected empty cache to be replaceable on fast sync")
	}
}

func writeCodexDeepHistoryTempFile(t *testing.T, lines []string) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "codex-deep-history.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp history: %v", err)
	}
	return filePath
}
