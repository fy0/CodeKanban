package websession

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestSessionRevisionAdvancesAtomicallyAndControlsConditionalSnapshots(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	initial, err := manager.SnapshotIfChanged(
		context.Background(),
		created.ID,
		DefaultHistoryWindow,
		"",
	)
	if err != nil {
		t.Fatalf("initial snapshot returned error: %v", err)
	}
	if initial.Unchanged || initial.Session == nil || initial.Revision == "" ||
		initial.PendingEpoch == "" || initial.PendingInputs == nil {
		t.Fatalf("expected a full revisioned snapshot, got %#v", initial)
	}

	unchanged, err := manager.SnapshotIfChanged(
		context.Background(),
		created.ID,
		DefaultHistoryWindow,
		initial.Revision,
	)
	if err != nil {
		t.Fatalf("conditional snapshot returned error: %v", err)
	}
	if !unchanged.Unchanged || unchanged.Revision != initial.Revision || unchanged.Session != nil ||
		unchanged.PendingEpoch != initial.PendingEpoch ||
		unchanged.PendingVersion != initial.PendingVersion || unchanged.PendingInputs == nil ||
		len(unchanged.PendingInputs) != 0 {
		t.Fatalf("expected compact unchanged response, got %#v", unchanged)
	}

	const advances = 8
	revisions := make(chan int64, advances)
	errors := make(chan error, advances)
	var waitGroup sync.WaitGroup
	for index := 0; index < advances; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			revision, advanceErr := manager.advanceSessionRevision(context.Background(), created.ID)
			if advanceErr != nil {
				errors <- advanceErr
				return
			}
			revisions <- revision
		}()
	}
	waitGroup.Wait()
	close(revisions)
	close(errors)
	for advanceErr := range errors {
		t.Fatalf("advanceSessionRevision returned error: %v", advanceErr)
	}
	seen := make(map[int64]struct{}, advances)
	for revision := range revisions {
		seen[revision] = struct{}{}
	}
	if len(seen) != advances {
		t.Fatalf("expected %d unique revisions, got %#v", advances, seen)
	}

	changed, err := manager.SnapshotIfChanged(
		context.Background(),
		created.ID,
		DefaultHistoryWindow,
		initial.Revision,
	)
	if err != nil {
		t.Fatalf("changed snapshot returned error: %v", err)
	}
	if changed.Unchanged || changed.Session == nil {
		t.Fatalf("expected a full snapshot after revision advancement, got %#v", changed)
	}
	initialRevision, _ := strconv.ParseInt(initial.Revision, 10, 64)
	changedRevision, _ := strconv.ParseInt(changed.Revision, 10, 64)
	if changedRevision < initialRevision+advances {
		t.Fatalf("revision = %d, want at least %d", changedRevision, initialRevision+advances)
	}
}

func TestConditionalSnapshotLeavesStaleHistoryCountsForMaintenance(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	nativeSessionID := "native-with-cached-history"
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", created.ID).
		Update("native_session_id", nativeSessionID).Error; err != nil {
		t.Fatalf("set native session id: %v", err)
	}
	if _, err := manager.appendHistoryItem(context.Background(), created.ID, HistoryItem{
		ID:         "cached-item",
		OrderIndex: 1,
		Kind:       "assistant",
		ItemType:   "agent_message",
		Text:       "already cached",
		Timestamp:  ptr(time.Now()),
	}); err != nil {
		t.Fatalf("append cached history: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", created.ID).
		Updates(map[string]any{"item_count": 7, "turn_count": 9}).Error; err != nil {
		t.Fatalf("seed stale history counts: %v", err)
	}

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.ItemCount != 7 {
		t.Fatalf("expected seeded stale item count, got %d", record.ItemCount)
	}
	revision := formatSnapshotRevision(record.SnapshotRevision)
	response, err := manager.SnapshotIfChanged(
		context.Background(),
		created.ID,
		DefaultHistoryWindow,
		revision,
	)
	if err != nil {
		t.Fatalf("conditional snapshot returned error: %v", err)
	}
	if !response.Unchanged || response.Session != nil || response.Revision != revision {
		t.Fatalf("expected an unchanged response at revision %s, got %#v", revision, response)
	}

	repaired, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession after repair returned error: %v", err)
	}
	if repaired.ItemCount != 7 {
		t.Fatalf("snapshot changed stale item count, got %d", repaired.ItemCount)
	}
	if repaired.TurnCount != 9 {
		t.Fatalf("snapshot changed stale turn count, got %d", repaired.TurnCount)
	}
	if repaired.SnapshotRevision != record.SnapshotRevision {
		t.Fatalf("count repair advanced revision from %d to %d", record.SnapshotRevision, repaired.SnapshotRevision)
	}
}

func TestPersistCodexGoalStateDoesNotRewriteUnchangedGoal(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	budget := int64(50_000)
	goal := &SessionGoal{
		ThreadID:        "native-goal-session",
		Objective:       "Keep the goal stable",
		Status:          GoalStatusActive,
		TokenBudget:     &budget,
		TokensUsed:      120,
		TimeUsedSeconds: 45,
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now,
	}
	changed, err := manager.persistCodexGoalState(context.Background(), record, goal)
	if err != nil || !changed {
		t.Fatalf("initial goal persistence = (%v, %v), want changed", changed, err)
	}
	persisted, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession after goal persistence returned error: %v", err)
	}

	changed, err = manager.persistCodexGoalState(context.Background(), persisted, goal)
	if err != nil {
		t.Fatalf("persist unchanged goal returned error: %v", err)
	}
	if changed {
		t.Fatal("unchanged goal unexpectedly reported a write")
	}
	after, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession after unchanged goal returned error: %v", err)
	}
	if after.SnapshotRevision != persisted.SnapshotRevision {
		t.Fatalf("unchanged goal advanced revision from %d to %d", persisted.SnapshotRevision, after.SnapshotRevision)
	}
}
