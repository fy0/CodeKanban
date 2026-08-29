package websession

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestMoveScopedTurnPlansToEnd(t *testing.T) {
	threadID := "root-thread"
	turnID := "turn-2"
	items := []HistoryItem{
		testScopedHistoryItem("plan", "tool", threadID, turnID, true),
		testScopedHistoryItem("redirect", "user", threadID, turnID, false),
		{ID: "subagent", Kind: "assistant"},
		testScopedHistoryItem("reasoning", "tool", threadID, turnID, false),
	}

	got := moveScopedTurnPlansToEnd(items)
	wantIDs := []string{"redirect", "reasoning", "subagent", "plan"}
	for index, wantID := range wantIDs {
		if got[index].ID != wantID {
			t.Fatalf("item %d: got %q want %q", index, got[index].ID, wantID)
		}
		if got[index].OrderIndex != int64(index+1) {
			t.Fatalf("item %d: got order %d", index, got[index].OrderIndex)
		}
	}
}

func TestWaitingPlanApprovalHistoryNeedsRepair(t *testing.T) {
	threadID := "root-thread"
	turnID := "turn-2"
	record := tables.WebSessionTable{
		Agent:           string(AgentCodex),
		NativeSessionID: &threadID,
		AssistantState:  string(AssistantStateWaitingPlanApproval),
	}
	plan := testScopedHistoryItem("plan", "tool", threadID, turnID, true)
	plan.OrderIndex = 1
	user := testScopedHistoryItem("redirect", "user", threadID, turnID, false)
	user.OrderIndex = 2

	if !waitingPlanApprovalHistoryNeedsRepair(record, []HistoryItem{plan, user}) {
		t.Fatal("expected same-turn user after plan to require repair")
	}
	plan.OrderIndex = 2
	user.OrderIndex = 1
	if waitingPlanApprovalHistoryNeedsRepair(record, []HistoryItem{user, plan}) {
		t.Fatal("expected complete trailing plan to be consistent")
	}
	record.AssistantState = string(AssistantStateWorking)
	if waitingPlanApprovalHistoryNeedsRepair(record, []HistoryItem{plan}) {
		t.Fatal("expected working state not to trigger plan repair")
	}
}

func TestReconcileSessionHistoryCacheUpdatesPlanAndPreservesRichData(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Reconcile plan", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	threadID := "root-thread"
	turnID := "turn-2"
	runID := "run-1"
	duration := int64(1234)
	observedAt := time.UnixMilli(1_000)
	plan := testScopedHistoryItem("plan-source", "tool", threadID, turnID, true)
	plan.OrderIndex = 1
	plan.Tool.Output = "old plan"
	plan.RunID = &runID
	plan.RunDurationMs = &duration
	plan.ObservedAt = &observedAt
	plan.Detail = &HistoryDetail{Type: "rich_detail", Prompt: "keep me"}
	persistedPlan, err := manager.appendHistoryItem(context.Background(), session.ID, plan)
	if err != nil {
		t.Fatalf("append plan: %v", err)
	}
	redirect := testScopedHistoryItem("redirect-source", "user", threadID, turnID, false)
	redirect.OrderIndex = 2
	if _, err := manager.appendHistoryItem(context.Background(), session.ID, redirect); err != nil {
		t.Fatalf("append redirect: %v", err)
	}
	filler := HistoryItem{ID: "filler-source", SourceItemID: ptr("filler-source"), Kind: "assistant", OrderIndex: 3}
	if _, err := manager.appendHistoryItem(context.Background(), session.ID, filler); err != nil {
		t.Fatalf("append filler: %v", err)
	}

	turnRow := tables.WebSessionTurnTable{}
	turnRow.Init()
	turnRow.WebSessionID = session.ID
	turnRow.SourceThreadID = &threadID
	turnRow.SourceTurnID = &turnID
	turnRow.Status = "completed"
	turnRow.SourceCreated = true

	incomingRedirect := historyItemTestRow(session.ID, redirect)
	incomingPlan := plan
	incomingPlan.Tool = &HistoryTool{ID: "plan-source", Kind: "plan", Output: "new complete plan", Status: "done"}
	incomingPlan.Detail = nil
	incomingPlan.RunID = nil
	incomingPlan.RunDurationMs = nil
	incomingPlan.ObservedAt = nil
	incomingPlan.Payload = map[string]any{"upstream": true}
	if err := manager.reconcileSessionHistoryCache(
		context.Background(),
		*session,
		[]tables.WebSessionTurnTable{turnRow},
		[]tables.WebSessionItemTable{incomingRedirect, historyItemTestRow(session.ID, incomingPlan)},
		map[string]any{"sync_state": SyncStateFresh, "last_sync_mode": string(SyncModeFast)},
		threadID,
	); err != nil {
		t.Fatalf("reconcileSessionHistoryCache: %v", err)
	}

	history, err := manager.loadHistoryWindow(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("loadHistoryWindow: %v", err)
	}
	if history.Total != 3 {
		t.Fatalf("expected reconciliation not to duplicate items, got %d", history.Total)
	}
	latest := history.Items[len(history.Items)-1]
	if latest.ID != persistedPlan.ID || latest.Tool == nil || latest.Tool.Output != "new complete plan" {
		t.Fatalf("expected updated persistent plan at timeline tail, got %#v", latest)
	}
	if latest.RunID == nil || *latest.RunID != runID || latest.RunDurationMs == nil || *latest.RunDurationMs != duration {
		t.Fatalf("expected run annotations to remain, got %#v", latest)
	}
	if latest.ObservedAt == nil || !latest.ObservedAt.Equal(observedAt) || latest.Detail == nil || latest.Detail.Prompt != "keep me" {
		t.Fatalf("expected timestamp and rich detail to remain, got %#v", latest)
	}

	var refreshed tables.WebSessionTable
	if err := model.GetDB().First(&refreshed, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if refreshed.SyncState != string(SyncStateFresh) || refreshed.ItemCount != 3 || refreshed.TurnCount != 1 {
		t.Fatalf("unexpected reconciled session metadata: %#v", refreshed)
	}
}

func TestSnapshotWithAutoSyncPreservesNonemptyHistoryWhenPlanRepairFails(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Plan repair fallback", 1000)
	threadID := "missing-native-thread"
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"native_session_id": threadID,
			"assistant_state":   string(AssistantStateWaitingPlanApproval),
			"item_count":        1,
			"sync_state":        string(SyncStateFresh),
		}).Error; err != nil {
		t.Fatalf("update session: %v", err)
	}
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: filepath.Join(t.TempDir(), "missing-codex"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if _, err := manager.appendHistoryItem(context.Background(), session.ID, HistoryItem{
		ID:           "cached-user",
		SourceItemID: ptr("cached-user"),
		OrderIndex:   1,
		Kind:         "user",
		ItemType:     "user_message",
		Text:         "keep cached history",
	}); err != nil {
		t.Fatalf("append cached history: %v", err)
	}

	snapshot, err := manager.SnapshotWithAutoSync(context.Background(), session.ID, 80)
	if err != nil {
		t.Fatalf("expected cached snapshot when repair fails, got %v", err)
	}
	if snapshot.History.Total != 1 || len(snapshot.History.Items) != 1 || snapshot.History.Items[0].Text != "keep cached history" {
		t.Fatalf("expected original nonempty history, got %#v", snapshot.History)
	}
}

func testScopedHistoryItem(id, kind, threadID, turnID string, plan bool) HistoryItem {
	item := HistoryItem{
		ID:             id,
		SourceThreadID: &threadID,
		SourceTurnID:   &turnID,
		SourceItemID:   &id,
		Kind:           kind,
		Done:           true,
	}
	if plan {
		item.ItemType = "plan"
		item.Tool = &HistoryTool{ID: id, Kind: "plan", Output: "complete plan", Status: "done"}
	}
	return item
}

func historyItemTestRow(sessionID string, item HistoryItem) tables.WebSessionItemTable {
	row := tables.WebSessionItemTable{}
	row.Init()
	applyHistoryItemToRow(&row, sessionID, item)
	return row
}
