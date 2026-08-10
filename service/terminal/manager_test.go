package terminal

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
)

func seedTerminalManagerSession(manager *Manager, projectID, sessionID string, orderIndex float64) *Session {
	session := &Session{
		id:                sessionID,
		projectID:         projectID,
		worktreeID:        "worktree-1",
		workingDir:        "/tmp/current",
		initialWorkingDir: "/tmp/initial",
		title:             sessionID,
		createdAt:         time.Unix(0, int64(orderIndex)),
		orderIndex:        orderIndex,
		closed:            make(chan struct{}),
	}
	session.shellIntegration.shellState = terminalRestoreShellStateIdle
	session.status.Store(SessionStatusRunning)
	manager.sessions.Store(sessionID, session)
	return session
}

func initTerminalManagerTestDB(t *testing.T) func() {
	t.Helper()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	if err := model.InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN failed: %v", err)
	}
	return func() {
		model.DBClose()
	}
}

func TestManagerMoveSessionRenormalizesOrder(t *testing.T) {
	manager := NewManager(Config{}, nil)
	seedTerminalManagerSession(manager, "project-1", "session-a", 1000)
	seedTerminalManagerSession(manager, "project-1", "session-b", 2000)
	seedTerminalManagerSession(manager, "project-1", "session-c", 3000)

	moved, err := manager.MoveSession("project-1", "session-c", "", "session-a")
	if err != nil {
		t.Fatalf("MoveSession returned error: %v", err)
	}
	if moved.ID() != "session-c" {
		t.Fatalf("expected moved session-c, got %q", moved.ID())
	}

	sessions := manager.ListSessions("project-1")
	gotIDs := []string{sessions[0].ID, sessions[1].ID, sessions[2].ID}
	expectedIDs := []string{"session-c", "session-a", "session-b"}
	for index, expectedID := range expectedIDs {
		if gotIDs[index] != expectedID {
			t.Fatalf("expected order %v, got %v", expectedIDs, gotIDs)
		}
		expectedOrder := float64(index+1) * terminalSessionOrderStep
		if sessions[index].OrderIndex != expectedOrder {
			t.Fatalf("expected orderIndex %.0f at index %d, got %.0f", expectedOrder, index, sessions[index].OrderIndex)
		}
	}
}

func TestManagerAddSessionInsertsAfterAnchor(t *testing.T) {
	manager := NewManager(Config{}, nil)
	seedTerminalManagerSession(manager, "project-1", "session-a", 1000)
	seedTerminalManagerSession(manager, "project-1", "session-b", 2000)
	newSession := seedTerminalManagerSession(manager, "project-1", "session-c", 0)
	manager.sessions.Delete(newSession.ID())

	if err := manager.addSession(newSession, "session-a"); err != nil {
		t.Fatalf("addSession returned error: %v", err)
	}

	sessions := manager.ListSessions("project-1")
	gotIDs := []string{sessions[0].ID, sessions[1].ID, sessions[2].ID}
	expectedIDs := []string{"session-a", "session-c", "session-b"}
	for index, expectedID := range expectedIDs {
		if gotIDs[index] != expectedID {
			t.Fatalf("expected order %v, got %v", expectedIDs, gotIDs)
		}
	}
}

func TestManagerMoveSessionBroadcastsProjectList(t *testing.T) {
	manager := NewManager(Config{}, nil)
	seedTerminalManagerSession(manager, "project-1", "session-a", 1000)
	seedTerminalManagerSession(manager, "project-1", "session-b", 2000)

	events, unsubscribe := manager.SubscribeSessionListEvents()
	defer unsubscribe()

	if _, err := manager.MoveSession("project-1", "session-b", "", "session-a"); err != nil {
		t.Fatalf("MoveSession returned error: %v", err)
	}

	select {
	case event := <-events:
		if event.Type != "sessions" || event.ProjectID != "project-1" {
			t.Fatalf("unexpected event metadata: %#v", event)
		}
		if len(event.Sessions) != 2 || event.Sessions[0].ID != "session-b" || event.Sessions[1].ID != "session-a" {
			t.Fatalf("unexpected event sessions: %#v", event.Sessions)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session list event")
	}
}

func TestPersistRestoreSessionStoresCurrentState(t *testing.T) {
	cleanup := initTerminalManagerTestDB(t)
	defer cleanup()

	manager := NewManager(Config{}, nil)
	session := seedTerminalManagerSession(manager, "project-1", "session-a", 1000)
	session.title = "Backend"
	session.workingDir = t.TempDir()
	session.initialWorkingDir = t.TempDir()
	session.shellIntegration.family = shellIntegrationFamilyBash
	session.shellIntegration.supported = true
	session.shellIntegration.pendingCommand = "go run ."
	session.shellIntegration.replayEligible = true
	session.shellIntegration.commandStartedAt = time.Now().UTC().Round(time.Second)
	session.shellIntegration.shellState = terminalRestoreShellStateRunning

	if err := manager.persistRestoreSession(context.Background(), session); err != nil {
		t.Fatalf("persistRestoreSession returned error: %v", err)
	}

	var record tables.TerminalRestoreSessionTable
	if err := model.GetDB().First(&record, "id = ?", session.ID()).Error; err != nil {
		t.Fatalf("failed to load restore record: %v", err)
	}
	if record.ProjectID != session.ProjectID() {
		t.Fatalf("expected project %q, got %q", session.ProjectID(), record.ProjectID)
	}
	if record.Title != "Backend" {
		t.Fatalf("expected title Backend, got %q", record.Title)
	}
	if record.LastCwd != session.WorkingDir() {
		t.Fatalf("expected cwd %q, got %q", session.WorkingDir(), record.LastCwd)
	}
	if record.PendingCommand == nil || *record.PendingCommand != "go run ." {
		t.Fatalf("expected pending command to be persisted, got %#v", record.PendingCommand)
	}
	if !record.ReplayEligible {
		t.Fatal("expected replay_eligible to be true")
	}
}

func TestShellIntegrationSequenceUpdatesStateAndStripsOutput(t *testing.T) {
	session := &Session{
		workingDir:        "/tmp/original",
		initialWorkingDir: "/tmp/original",
		closed:            make(chan struct{}),
	}
	session.shellIntegration.supported = true
	session.shellIntegration.shellState = terminalRestoreShellStateIdle

	cwdPayload := base64.StdEncoding.EncodeToString([]byte("/tmp/updated"))
	cmdPayload := base64.StdEncoding.EncodeToString([]byte("go run ."))
	chunk := []byte(
		"before" +
			"\x1b]633;CodeKanban;cwd;" + cwdPayload + "\a" +
			"middle" +
			"\x1b]633;CodeKanban;cmd;" + cmdPayload + "\a" +
			"after",
	)

	stripped := session.stripShellIntegrationSequences(chunk)
	if string(stripped) != "beforemiddleafter" {
		t.Fatalf("expected OSC events to be removed, got %q", string(stripped))
	}
	if session.WorkingDir() != "/tmp/updated" {
		t.Fatalf("expected cwd to update, got %q", session.WorkingDir())
	}

	session.shellIntegration.mu.Lock()
	pendingCommand := session.shellIntegration.pendingCommand
	replayEligible := session.shellIntegration.replayEligible
	shellState := session.shellIntegration.shellState
	session.shellIntegration.mu.Unlock()

	if pendingCommand != "go run ." {
		t.Fatalf("expected pending command go run ., got %q", pendingCommand)
	}
	if !replayEligible {
		t.Fatal("expected command to be replay eligible")
	}
	if shellState != terminalRestoreShellStateRunning {
		t.Fatalf("expected shell state running, got %q", shellState)
	}
}
