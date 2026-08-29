package websession

import (
	"context"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestSnapshotDoesNotClearUnreadOrAdvanceRevision(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	session, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"has_unread": true, "attention_revision": 4}).Error; err != nil {
		t.Fatalf("seed unread state: %v", err)
	}
	before, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession before snapshot: %v", err)
	}

	snapshot, err := manager.SnapshotIfChanged(
		context.Background(),
		session.ID,
		DefaultHistoryWindow,
		formatSnapshotRevision(before.SnapshotRevision),
	)
	if err != nil {
		t.Fatalf("conditional snapshot returned error: %v", err)
	}
	if !snapshot.Unchanged {
		t.Fatalf("expected unchanged snapshot, got %#v", snapshot)
	}
	after, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession after snapshot: %v", err)
	}
	if !after.HasUnread || after.AttentionRevision != 4 {
		t.Fatalf("snapshot changed unread state: %#v", after)
	}
	if after.SnapshotRevision != before.SnapshotRevision {
		t.Fatalf("snapshot advanced revision from %d to %d", before.SnapshotRevision, after.SnapshotRevision)
	}
}

func TestCatchUpReturnsItemsUpdatedInPlace(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	session, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	row := tables.WebSessionItemTable{
		WebSessionID: session.ID,
		OrderIndex:   1,
		LastEventSeq: 3,
		ItemKind:     "assistant",
		ItemType:     "agent_message",
		Text:         "updated text",
	}
	row.Init()
	if err := model.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("create history item: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"last_event_seq": 3, "item_count": 1}).Error; err != nil {
		t.Fatalf("update session cursor: %v", err)
	}

	response, err := manager.CatchUpSession(
		context.Background(),
		session.ID,
		formatEventCursor(2, maxEventCursorOrder),
		"",
		"1",
		DefaultHistoryWindow,
	)
	if err != nil {
		t.Fatalf("CatchUpSession returned error: %v", err)
	}
	if response.ResetRequired || response.HasMore || len(response.Items) != 1 {
		t.Fatalf("unexpected catch-up response: %#v", response)
	}
	if response.Items[0].OrderIndex != 1 || response.Items[0].Text != "updated text" {
		t.Fatalf("updated item not returned: %#v", response.Items[0])
	}
	if response.NextEventCursor != formatEventCursor(3, maxEventCursorOrder) {
		t.Fatalf("next cursor = %q", response.NextEventCursor)
	}
}

func TestCatchUpRequiresResetWhenHistoryEpochChanges(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	session, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Update("history_epoch", 2).Error; err != nil {
		t.Fatalf("update history epoch: %v", err)
	}

	response, err := manager.CatchUpSession(
		context.Background(),
		session.ID,
		"0:0",
		"",
		"1",
		DefaultHistoryWindow,
	)
	if err != nil {
		t.Fatalf("CatchUpSession returned error: %v", err)
	}
	if !response.ResetRequired || response.HistoryEpoch != "2" {
		t.Fatalf("expected reset for epoch change, got %#v", response)
	}
}

func TestMarkSessionReadUsesAttentionRevisionCAS(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	session, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"has_unread": true, "attention_revision": 1}).Error; err != nil {
		t.Fatalf("seed unread state: %v", err)
	}
	before, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession before mark read: %v", err)
	}

	stale, err := manager.MarkSessionRead(context.Background(), session.ID, "0")
	if err != nil {
		t.Fatalf("stale MarkSessionRead returned error: %v", err)
	}
	if !stale.HasUnread || stale.AttentionRevision != "1" {
		t.Fatalf("stale mark read cleared newer unread: %#v", stale)
	}
	cleared, err := manager.MarkSessionRead(context.Background(), session.ID, "1")
	if err != nil {
		t.Fatalf("MarkSessionRead returned error: %v", err)
	}
	if cleared.HasUnread || cleared.AttentionRevision != "2" {
		t.Fatalf("mark read did not clear expected state: %#v", cleared)
	}
	after, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession after mark read: %v", err)
	}
	if after.SnapshotRevision != before.SnapshotRevision {
		t.Fatalf("mark read advanced content revision from %d to %d", before.SnapshotRevision, after.SnapshotRevision)
	}
}
