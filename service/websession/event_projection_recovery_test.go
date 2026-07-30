package websession

import (
	"context"
	"errors"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestPersistedEventProjectionRollsBackHistoryWhenCursorUpdateFails(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Atomic projection", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	db := model.GetDB()
	callbackName := "test:fail_projection_cursor"
	cursorFailure := errors.New("cursor update failed")
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == (tables.WebSessionTable{}).TableName() {
			tx.AddError(cursorFailure)
		}
	}); err != nil {
		t.Fatalf("register update callback: %v", err)
	}
	callbackRegistered := true
	defer func() {
		if callbackRegistered {
			_ = db.Callback().Update().Remove(callbackName)
		}
	}()

	_, appendErr := manager.appendAndBroadcast(context.Background(), session.ID, *session, Event{
		ID:        "evt-atomic-note",
		Type:      "note",
		Timestamp: time.Now(),
		Payload:   map[string]any{"txt": "commit atomically"},
	})
	if appendErr == nil || !errors.Is(appendErr, cursorFailure) {
		t.Fatalf("appendAndBroadcast error = %v, want cursor failure", appendErr)
	}
	assertProjectionState := func(wantItems int64, wantSeq int64) {
		t.Helper()
		var itemCount int64
		if err := db.Model(&tables.WebSessionItemTable{}).Where("web_session_id = ?", session.ID).Count(&itemCount).Error; err != nil {
			t.Fatalf("count projected items: %v", err)
		}
		var record tables.WebSessionTable
		if err := db.First(&record, "id = ?", session.ID).Error; err != nil {
			t.Fatalf("load projection record: %v", err)
		}
		if itemCount != wantItems || record.LastEventSeq != wantSeq {
			t.Fatalf("projection state items=%d seq=%d, want items=%d seq=%d", itemCount, record.LastEventSeq, wantItems, wantSeq)
		}
	}
	assertProjectionState(0, 0)

	state := manager.sessionEventState(session.ID)
	state.mu.Lock()
	if err := db.Callback().Update().Remove(callbackName); err != nil {
		state.mu.Unlock()
		t.Fatalf("remove update callback: %v", err)
	}
	callbackRegistered = false
	if err := manager.flushEventProjectionRetriesLocked(context.Background(), session.ID, state); err != nil {
		state.mu.Unlock()
		t.Fatalf("flush projection retry: %v", err)
	}
	state.mu.Unlock()
	assertProjectionState(1, 1)
}

func TestNewManagerRecoversPersistedEventProjectionOnce(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Projection recovery", 1000)
	dataDir := t.TempDir()
	eventStore, err := newStore(dataDir)
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	observedAt := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	events := []Event{
		{ID: "evt-note", Seq: 1, Type: "note", Timestamp: observedAt, Payload: map[string]any{"txt": "persisted note"}},
		{ID: "evt-delta", Seq: 3, Type: "txt_d", ParentID: "message-1", Timestamp: observedAt.Add(time.Second), Payload: map[string]any{"mid": "message-1", "txt": "recovered text"}},
		{ID: "evt-agent", Seq: 4, Type: "sub_agent_state", ThreadID: "thread-child", Timestamp: observedAt.Add(2 * time.Second), Payload: map[string]any{"threadId": "thread-child", "status": "completed"}},
	}
	for _, event := range events {
		if err := eventStore.appendEvent(session.ID, event); err != nil {
			t.Fatalf("append event %d: %v", event.Seq, err)
		}
	}

	manager, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager recovery returned error: %v", err)
	}
	assertRecoveredProjection(t, manager, session.ID)

	restarted, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("second NewManager returned error: %v", err)
	}
	assertRecoveredProjection(t, restarted, session.ID)
}

func assertRecoveredProjection(t *testing.T, manager *Manager, sessionID string) {
	t.Helper()
	record, err := manager.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.LastEventSeq != 4 {
		t.Fatalf("last event sequence = %d, want 4", record.LastEventSeq)
	}
	window, err := manager.loadHistoryWindow(context.Background(), sessionID, 10, nil)
	if err != nil {
		t.Fatalf("loadHistoryWindow returned error: %v", err)
	}
	if len(window.Items) != 2 {
		t.Fatalf("history item count = %d, want 2", len(window.Items))
	}
	if window.Items[0].Text != "persisted note" || window.Items[1].Text != "recovered text" {
		t.Fatalf("recovered history = %#v", window.Items)
	}
	var agents []tables.WebSessionSubAgentTable
	if err := model.GetDB().Where("web_session_id = ?", sessionID).Find(&agents).Error; err != nil {
		t.Fatalf("load recovered sub agents: %v", err)
	}
	if len(agents) != 1 || agents[0].ThreadID != "thread-child" || agents[0].LastEventSeq != 4 {
		t.Fatalf("recovered sub agents = %#v", agents)
	}
}
