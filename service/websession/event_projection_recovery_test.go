package websession

import (
	"context"
	"errors"
	"os"
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

	appended, appendErr := manager.appendAndBroadcast(context.Background(), session.ID, *session, Event{
		ID:        "evt-atomic-note",
		Type:      "note",
		Timestamp: time.Now(),
		Payload:   map[string]any{"txt": "commit atomically"},
	})
	if appendErr != nil || appended.ID != "evt-atomic-note" {
		t.Fatalf("durable append must not surface projection failure: event=%#v error=%v", appended, appendErr)
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

func TestNewManagerDoesNotReplayPersistedEventProjection(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Projection replay skipped", 1000)
	dataDir := t.TempDir()
	eventStore, err := newStore(dataDir)
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	observedAt := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	if err := eventStore.appendEvent(session.ID, Event{
		ID: "evt-note", Seq: 1, Type: "note", Timestamp: observedAt,
		Payload: map[string]any{"txt": "persisted but not projected"},
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	corruptSession := seedWebSession(t, project.ID, "Unreadable history is ignored", 2000)
	if err := eventStore.ensureSessionDir(corruptSession.ID); err != nil {
		t.Fatalf("ensure corrupt session directory: %v", err)
	}
	if err := os.WriteFile(eventStore.historyPath(corruptSession.ID), []byte(`{"id":"incomplete"}`), 0o600); err != nil {
		t.Fatalf("write incomplete history tail: %v", err)
	}

	manager, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.LastEventSeq != 0 {
		t.Fatalf("last event sequence = %d, want 0 without startup replay", record.LastEventSeq)
	}
	window, err := manager.loadHistoryWindow(context.Background(), session.ID, 10, nil)
	if err != nil {
		t.Fatalf("loadHistoryWindow returned error: %v", err)
	}
	if len(window.Items) != 0 {
		t.Fatalf("startup replayed persisted history: %#v", window.Items)
	}
}

func TestNewManagerRecoversInterruptedSessionPastUnprojectedTail(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Interrupted projection tail", 1000)
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Update("status", string(StatusRunning)).Error; err != nil {
		t.Fatalf("mark session running: %v", err)
	}
	dataDir := t.TempDir()
	eventStore, err := newStore(dataDir)
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	if err := eventStore.appendEvent(session.ID, Event{
		ID: "evt-unprojected", Seq: 1, Type: "note", Timestamp: time.Now().Add(-time.Minute),
		Payload: map[string]any{"txt": "unprojected tail"},
	}); err != nil {
		t.Fatalf("append unprojected event: %v", err)
	}

	manager, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusIdle) || record.LastEventSeq != 2 {
		t.Fatalf("recovered session status=%q seq=%d, want idle seq=2", record.Status, record.LastEventSeq)
	}
	events, err := manager.History(context.Background(), session.ID, 10, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(events.Events) != 2 || events.Events[0].ID != "evt-unprojected" || events.Events[1].Type != "run_abort" {
		t.Fatalf("durable history after interrupted recovery = %#v", events.Events)
	}
	window, err := manager.loadHistoryWindow(context.Background(), session.ID, 10, nil)
	if err != nil {
		t.Fatalf("loadHistoryWindow returned error: %v", err)
	}
	for _, item := range window.Items {
		if item.Text == "unprojected tail" {
			t.Fatalf("startup replayed the unprojected tail: %#v", window.Items)
		}
	}
}
