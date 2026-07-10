package websession

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"

	"go.uber.org/zap"
)

type captureWSConn struct {
	frames []wireFrame
}

func (c *captureWSConn) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, io.EOF
}

func (c *captureWSConn) WriteJSON(v any) error {
	frame, ok := v.(wireFrame)
	if !ok {
		return fmt.Errorf("unexpected frame type %T", v)
	}
	c.frames = append(c.frames, frame)
	return nil
}

func (c *captureWSConn) Close() error {
	return nil
}

type heartbeatWSConn struct {
	mu      sync.Mutex
	frames  []wireFrame
	closed  chan struct{}
	closeMu sync.Once
}

func newHeartbeatWSConn() *heartbeatWSConn {
	return &heartbeatWSConn{
		closed: make(chan struct{}),
	}
}

func (c *heartbeatWSConn) ReadMessage() (messageType int, p []byte, err error) {
	<-c.closed
	return 0, nil, io.EOF
}

func (c *heartbeatWSConn) WriteJSON(v any) error {
	frame, ok := v.(wireFrame)
	if !ok {
		return fmt.Errorf("unexpected frame type %T", v)
	}
	c.mu.Lock()
	c.frames = append(c.frames, frame)
	c.mu.Unlock()
	return nil
}

func (c *heartbeatWSConn) Close() error {
	c.closeMu.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *heartbeatWSConn) snapshotFrames() []wireFrame {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]wireFrame(nil), c.frames...)
}

func attachmentExtensionFromMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".png"
	}
}

func TestManagerCreateSessionAppendsOrderIndex(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	seedWebSession(t, project.ID, "First", 1000)
	seedWebSession(t, project.ID, "Second", 2000)

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

	if created.OrderIndex != 3000 {
		t.Fatalf("expected orderIndex 3000, got %.2f", created.OrderIndex)
	}
	if created.WorkflowMode != WorkflowModeDefault {
		t.Fatalf("expected default workflow mode, got %q", created.WorkflowMode)
	}
	if created.PermissionLevel != PermissionLevelElevated {
		t.Fatalf("expected elevated permission level, got %q", created.PermissionLevel)
	}
}

func TestManagerCreateSessionDefaultsCodexToAppServerBackend(t *testing.T) {
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
	if effectiveSessionBackend(record) != SessionBackendCodexAppServer {
		t.Fatalf("expected codex sessions to default to %q, got %q", SessionBackendCodexAppServer, effectiveSessionBackend(record))
	}
}

func TestImportCodexSessionCreatesBoundSessionAndSyncsHistory(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		fmt.Sprintf(`{"timestamp":"2026-04-11T01:00:00Z","type":"session_meta","payload":{"id":"thread_import_1","timestamp":"2026-04-11T01:00:00Z","cwd":%q}}`, project.Path),
		`{"timestamp":"2026-04-11T01:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"import this history","images":[]}}`,
		`{"timestamp":"2026-04-11T01:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"history imported"}}`,
	})
	lastMessageAt := time.Date(2026, 4, 11, 1, 0, 2, 0, time.UTC)
	aiSession := seedCodexAISession(
		t,
		project.Path,
		"thread_import_1",
		filePath,
		"Imported Session",
		time.Date(2026, 4, 11, 1, 0, 0, 0, time.UTC),
		&lastMessageAt,
	)

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: filepath.Join(t.TempDir(), "missing-codex"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	result, err := manager.ImportCodexSession(context.Background(), project.ID, aiSession.ID, SyncModeFast)
	if err != nil {
		t.Fatalf("ImportCodexSession returned error: %v", err)
	}
	if !result.Created || result.Reused || !result.Synced {
		t.Fatalf("unexpected import result flags: %#v", result)
	}
	if result.Session.Title != "Imported Session" {
		t.Fatalf("expected imported title to be preserved, got %q", result.Session.Title)
	}
	if result.Session.NativeSessionID == nil || strings.TrimSpace(*result.Session.NativeSessionID) != "thread_import_1" {
		t.Fatalf("expected native session id thread_import_1, got %#v", result.Session.NativeSessionID)
	}
	if result.Session.ThreadPath == nil || strings.TrimSpace(*result.Session.ThreadPath) != filePath {
		t.Fatalf("expected thread path %q, got %#v", filePath, result.Session.ThreadPath)
	}
	if result.Session.LastSyncMode != SyncModeDeep {
		t.Fatalf("expected fast import to fall back to deep sync in test, got %q", result.Session.LastSyncMode)
	}
	if result.History.Total != 2 {
		t.Fatalf("expected 2 imported history items, got %d", result.History.Total)
	}
	if len(result.History.Items) != 2 || result.History.Items[0].Text != "import this history" {
		t.Fatalf("unexpected imported history items: %#v", result.History.Items)
	}

	record, err := manager.GetSession(context.Background(), result.Session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.WorktreeID != nil {
		t.Fatalf("expected imported session to stay unbound from worktrees, got %#v", record.WorktreeID)
	}
	if record.Cwd != project.Path {
		t.Fatalf("expected cwd %q, got %q", project.Path, record.Cwd)
	}
}

func TestImportCodexSessionReusesExistingSessionWithoutResync(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		fmt.Sprintf(`{"timestamp":"2026-04-11T02:00:00Z","type":"session_meta","payload":{"id":"thread_import_existing","timestamp":"2026-04-11T02:00:00Z","cwd":%q}}`, project.Path),
		`{"timestamp":"2026-04-11T02:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"reuse this history","images":[]}}`,
		`{"timestamp":"2026-04-11T02:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"existing imported history"}}`,
	})
	lastMessageAt := time.Date(2026, 4, 11, 2, 0, 2, 0, time.UTC)
	aiSession := seedCodexAISession(
		t,
		project.Path,
		"thread_import_existing",
		filePath,
		"Imported Source Title",
		time.Date(2026, 4, 11, 2, 0, 0, 0, time.UTC),
		&lastMessageAt,
	)

	archivedAt := time.Now().Add(-time.Hour)
	existingThreadPath := filepath.Join(t.TempDir(), "stale-thread.jsonl")
	existingPreview := "stale preview"
	existingNativeID := "thread_import_existing"
	existing := &tables.WebSessionTable{
		ProjectID:               project.ID,
		OrderIndex:              1000,
		Agent:                   string(AgentCodex),
		Backend:                 string(SessionBackendCodexAppServer),
		Title:                   "Pinned Title",
		TitleAuto:               false,
		Model:                   "gpt-5.4",
		ReasoningEffort:         string(ReasoningEffortMedium),
		WorkflowMode:            string(WorkflowModeDefault),
		PermissionLevel:         string(PermissionLevelElevated),
		AutoRetryEnabled:        false,
		AutoRetryScope:          string(AutoRetryScopeNetworkOnly),
		AutoRetryPreset:         string(AutoRetryPresetGentleStop),
		LegacyPermissionMode:    "default",
		Cwd:                     project.Path,
		NativeSessionID:         &existingNativeID,
		Status:                  string(StatusIdle),
		HasUnread:               true,
		ArchivedAt:              &archivedAt,
		ActivityAt:              time.Now().Add(-time.Minute),
		StatusUpdatedAt:         nil,
		AssistantStateUpdatedAt: nil,
		SourceKind:              defaultSourceKind(AgentCodex),
		SyncState:               string(SyncStateFresh),
		LastSyncMode:            string(SyncModeDeep),
		SourceCreatedAt:         nil,
		SourceUpdatedAt:         nil,
		LastSyncedAt:            nil,
		ThreadPath:              &existingThreadPath,
		ThreadPreview:           &existingPreview,
		TurnCount:               0,
		ItemCount:               1,
		LastMessageAt:           nil,
		LastEventSeq:            0,
	}
	existing.Init()
	if err := model.GetDB().Create(existing).Error; err != nil {
		t.Fatalf("seed existing web session failed: %v", err)
	}

	itemRow := tables.WebSessionItemTable{}
	itemRow.Init()
	itemRow.WebSessionID = existing.ID
	applyHistoryItemToRow(&itemRow, existing.ID, HistoryItem{
		ID:         "cached_history",
		OrderIndex: 1,
		Kind:       "assistant",
		ItemType:   "agent_message",
		Text:       "cached history",
		Timestamp:  ptr(time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)),
		ObservedAt: ptr(time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)),
		Done:       true,
	})
	if err := model.GetDB().Create(&itemRow).Error; err != nil {
		t.Fatalf("seed web session item failed: %v", err)
	}

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: filepath.Join(t.TempDir(), "missing-codex"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	result, err := manager.ImportCodexSession(context.Background(), project.ID, aiSession.ID, SyncModeFast)
	if err != nil {
		t.Fatalf("ImportCodexSession returned error: %v", err)
	}
	if result.Created || !result.Reused || result.Synced {
		t.Fatalf("unexpected reuse result flags: %#v", result)
	}
	if result.Session.ID != existing.ID {
		t.Fatalf("expected reused session id %q, got %q", existing.ID, result.Session.ID)
	}
	if result.Session.Title != "Pinned Title" {
		t.Fatalf("expected existing title to be preserved, got %q", result.Session.Title)
	}
	if result.Session.ArchivedAt != nil {
		t.Fatalf("expected reused session to be unarchived, got %#v", result.Session.ArchivedAt)
	}
	if result.History.Total != 1 || len(result.History.Items) != 1 || result.History.Items[0].Text != "cached history" {
		t.Fatalf("expected cached history to remain untouched, got %#v", result.History.Items)
	}

	record, err := manager.GetSession(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.ArchivedAt != nil {
		t.Fatalf("expected archived session to be restored, got %#v", record.ArchivedAt)
	}
	if record.ThreadPath == nil || strings.TrimSpace(*record.ThreadPath) != filePath {
		t.Fatalf("expected thread path to refresh to %q, got %#v", filePath, record.ThreadPath)
	}
	if record.Title != "Pinned Title" {
		t.Fatalf("expected existing title to remain, got %q", record.Title)
	}
}

func TestImportCodexSessionRejectsProjectPathMismatch(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	projectA := seedProject(t)
	projectB := seedProject(t)
	filePath := writeCodexDeepHistoryTempFile(t, []string{
		fmt.Sprintf(`{"timestamp":"2026-04-11T03:00:00Z","type":"session_meta","payload":{"id":"thread_import_mismatch","timestamp":"2026-04-11T03:00:00Z","cwd":%q}}`, projectA.Path),
		`{"timestamp":"2026-04-11T03:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"wrong project","images":[]}}`,
	})
	lastMessageAt := time.Date(2026, 4, 11, 3, 0, 1, 0, time.UTC)
	aiSession := seedCodexAISession(
		t,
		projectA.Path,
		"thread_import_mismatch",
		filePath,
		"Mismatch",
		time.Date(2026, 4, 11, 3, 0, 0, 0, time.UTC),
		&lastMessageAt,
	)

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	_, err = manager.ImportCodexSession(context.Background(), projectB.ID, aiSession.ID, SyncModeFast)
	if err == nil {
		t.Fatal("expected project path mismatch to fail")
	}
	if !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected project mismatch error, got %v", err)
	}
}

func TestListCodexImportSourcesUsesThreadListAndMarksDuplicates(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	nativeID := "thread_list"
	existing := &tables.WebSessionTable{
		ProjectID:            project.ID,
		OrderIndex:           1000,
		Agent:                string(AgentCodex),
		Backend:              string(SessionBackendCodexAppServer),
		Title:                "Imported Thread",
		Model:                "gpt-5.4",
		WorkflowMode:         string(WorkflowModeDefault),
		PermissionLevel:      string(PermissionLevelElevated),
		LegacyPermissionMode: "default",
		Cwd:                  project.Path,
		NativeSessionID:      &nativeID,
		Status:               string(StatusIdle),
		ActivityAt:           time.Now(),
	}
	existing.Init()
	if err := model.GetDB().Create(existing).Error; err != nil {
		t.Fatalf("seed existing web session failed: %v", err)
	}

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "list_threads"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	result, err := manager.ListCodexImportSources(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListCodexImportSources returned error: %v", err)
	}
	if result.ScanPhase != "complete" {
		t.Fatalf("expected scan phase complete, got %q", result.ScanPhase)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 import sources, got %d", len(result.Items))
	}

	var duplicate ImportSourceSummary
	foundDuplicate := false
	for _, item := range result.Items {
		if item.SessionID == "thread_list" {
			duplicate = item
			foundDuplicate = true
			break
		}
	}
	if !foundDuplicate {
		t.Fatalf("expected thread_list import source, got %#v", result.Items)
	}
	if !duplicate.Duplicate || duplicate.ExistingSession == nil {
		t.Fatalf("expected duplicate thread to be marked, got %#v", duplicate)
	}
	if duplicate.ExistingSession.ID != existing.ID {
		t.Fatalf("expected existing session id %q, got %#v", existing.ID, duplicate.ExistingSession)
	}
}

func TestManagerBroadcastOnlyTargetsEventClients(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	commandConn := &captureWSConn{}
	eventConn := &captureWSConn{}
	commandClient := manager.RegisterCommandClient(commandConn)
	eventClient := manager.RegisterEventClient(eventConn)
	defer manager.UnregisterClient(commandClient)
	defer manager.UnregisterClient(eventClient)

	manager.broadcast(newSessionFrame("session-1", SessionSummary{
		ID:        "session-1",
		ProjectID: "project-1",
		Title:     "Session 1",
		Agent:     AgentCodex,
		Status:    StatusRunning,
	}))

	if len(commandConn.frames) != 0 {
		t.Fatalf("expected command client to receive no broadcast frames, got %d", len(commandConn.frames))
	}
	if len(eventConn.frames) != 1 {
		t.Fatalf("expected event client to receive exactly one broadcast frame, got %d", len(eventConn.frames))
	}
	if eventConn.frames[0].Kind != "evt" || eventConn.frames[0].Operation != "session" {
		t.Fatalf("expected session event frame, got kind=%q op=%q", eventConn.frames[0].Kind, eventConn.frames[0].Operation)
	}
}

func TestManagerHandleHeartbeatPayloadRepliesToPing(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	conn := &captureWSConn{}
	client := manager.RegisterEventClient(conn)
	defer manager.UnregisterClient(client)

	handled, err := manager.HandleHeartbeatPayload(client, []byte(`{"v":1,"k":"hb","ts":1710000000000,"op":"ping"}`))
	if err != nil {
		t.Fatalf("HandleHeartbeatPayload returned error: %v", err)
	}
	if !handled {
		t.Fatal("expected heartbeat payload to be handled")
	}
	if len(conn.frames) != 1 {
		t.Fatalf("expected one heartbeat response frame, got %d", len(conn.frames))
	}
	if conn.frames[0].Kind != "hb" || conn.frames[0].Operation != "pong" {
		t.Fatalf("expected heartbeat pong response, got %#v", conn.frames[0])
	}
}

func TestHandleSendCommandRepliesWithAckAndSnapshot(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
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

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_send","sid":%q,"op":"send","p":{"txt":"first","atts":[]}}`, created.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 2 {
		t.Fatalf("expected ack and snapshot frames, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "send" {
		t.Fatalf("expected first frame to be send ack, got %#v", conn.frames[0])
	}
	if conn.frames[1].Kind != "snap" || conn.frames[1].SessionID != created.ID {
		t.Fatalf("expected second frame to be session snapshot, got %#v", conn.frames[1])
	}
	if conn.frames[1].History == nil || conn.frames[1].History.Total < 1 {
		t.Fatalf("expected snapshot history to contain the new message, got %#v", conn.frames[1].History)
	}
	if conn.frames[1].Session == nil || conn.frames[1].Session.Status == "" {
		t.Fatalf("expected snapshot to include session summary, got %#v", conn.frames[1].Session)
	}

	waitForSessionToSettle(t, manager, created.ID)
}

func TestHandleSendCommandRejectsMissingCodexBinary(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: filepath.Join(t.TempDir(), "missing-codex"),
	}, zap.NewNop())
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

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_send","sid":%q,"op":"send","p":{"txt":"first","atts":[]}}`, created.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 1 {
		t.Fatalf("expected a single error frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "err" {
		t.Fatalf("expected error frame, got %#v", conn.frames[0])
	}
	if conn.frames[0].Message != errCodexNotInstalled {
		t.Fatalf("expected message %q, got %q", errCodexNotInstalled, conn.frames[0].Message)
	}
}

func TestHandlePendingUpdateCommandRepliesWithAckAndSnapshot(t *testing.T) {
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

	item := PendingInput{
		ID:        "pending-1",
		Mode:      PendingInputModeQueue,
		Text:      "first draft",
		CreatedAt: time.Now(),
	}
	manager.mu.Lock()
	manager.pendingInputs[created.ID] = []PendingInput{item}
	manager.mu.Unlock()

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_pending_update","sid":%q,"op":"pending_update","p":{"id":%q,"txt":"updated draft"}}`, created.ID, item.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 2 {
		t.Fatalf("expected ack and snapshot frames, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "pending_update" {
		t.Fatalf("expected first frame to be pending_update ack, got %#v", conn.frames[0])
	}
	if conn.frames[1].Kind != "snap" || conn.frames[1].SessionID != created.ID {
		t.Fatalf("expected second frame to be session snapshot, got %#v", conn.frames[1])
	}
	if len(conn.frames[1].Pending) != 1 || conn.frames[1].Pending[0].Text != "updated draft" {
		t.Fatalf("expected snapshot pending input to be updated, got %#v", conn.frames[1].Pending)
	}
}

func TestHandlePendingReorderCommandMovesAcrossPartitions(t *testing.T) {
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

	manager.mu.Lock()
	manager.pendingInputs[created.ID] = []PendingInput{
		{ID: "redirect-1", Mode: PendingInputModeRedirect, Text: "redirect-1", CreatedAt: time.Now()},
		{ID: "queue-1", Mode: PendingInputModeQueue, Text: "queue-1", CreatedAt: time.Now()},
		{ID: "queue-2", Mode: PendingInputModeQueue, Text: "queue-2", CreatedAt: time.Now()},
	}
	manager.mu.Unlock()

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_pending_reorder","sid":%q,"op":"pending_reorder","p":{"id":"queue-2","mode":"redirect","idx":0}}`, created.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 2 {
		t.Fatalf("expected ack and snapshot frames, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "pending_reorder" {
		t.Fatalf("expected first frame to be pending_reorder ack, got %#v", conn.frames[0])
	}
	if got := conn.frames[1].Pending; len(got) != 3 || got[0].ID != "queue-2" || got[0].Mode != "redirect" || got[1].ID != "redirect-1" || got[2].ID != "queue-1" {
		t.Fatalf("expected reordered pending inputs in snapshot, got %#v", got)
	}
}

func TestHandlePendingClearCommandClearsSnapshot(t *testing.T) {
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

	manager.mu.Lock()
	manager.pendingInputs[created.ID] = []PendingInput{
		{ID: "queue-1", Mode: PendingInputModeQueue, Text: "queue-1", CreatedAt: time.Now()},
	}
	manager.mu.Unlock()

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_pending_clear","sid":%q,"op":"pending_clear","p":{}}`, created.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 2 {
		t.Fatalf("expected ack and snapshot frames, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "pending_clear" {
		t.Fatalf("expected first frame to be pending_clear ack, got %#v", conn.frames[0])
	}
	if len(conn.frames[1].Pending) != 0 {
		t.Fatalf("expected cleared pending inputs in snapshot, got %#v", conn.frames[1].Pending)
	}
}

func TestHandleGoalSetCommandRejectsOldCodexVersion(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.132.9"),
	}, zap.NewNop())
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

	threadID := "thread_test"
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", created.ID).
		Update("native_session_id", threadID).Error; err != nil {
		t.Fatalf("set native_session_id failed: %v", err)
	}

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_goal","sid":%q,"op":"goal_set","p":{"obj":"Finish migration","st":"active"}}`, created.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 1 {
		t.Fatalf("expected a single error frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "err" {
		t.Fatalf("expected error frame, got %#v", conn.frames[0])
	}
	expected := "Goal mode requires Codex >= 0.133.0. Current version: 0.132.9."
	if conn.frames[0].Message != expected {
		t.Fatalf("expected message %q, got %q", expected, conn.frames[0].Message)
	}
}

func TestHandleGoalBootstrapCommandStartsHiddenCodexRun(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
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

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_goal_bootstrap","sid":%q,"op":"goal_bootstrap","p":{"obj":"Generate a first draft immediately","st":"active"}}`, created.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	if len(conn.frames) < 2 {
		t.Fatalf("expected ack and snapshot frames, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" {
		t.Fatalf("expected first frame to be ack, got %#v", conn.frames[0])
	}
	last := conn.frames[len(conn.frames)-1]
	if last.Kind != "snap" || last.Session == nil {
		t.Fatalf("expected final frame to be snapshot, got %#v", last)
	}
	if last.Session.Goal == nil {
		t.Fatalf("expected snapshot to include goal, got %#v", last.Session)
	}
	if last.Session.Goal.Objective != "Generate a first draft immediately" {
		t.Fatalf("expected snapshot goal objective to be hydrated, got %#v", last.Session.Goal)
	}
	if last.Session.NativeSessionID == nil || strings.TrimSpace(*last.Session.NativeSessionID) == "" {
		t.Fatalf("expected snapshot to include native session id, got %#v", last.Session)
	}
}

func TestHandleScheduleSendCommandRejectsMissingCodexBinary(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: filepath.Join(t.TempDir(), "missing-codex"),
	}, zap.NewNop())
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

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_schedule","sid":%q,"op":"schedule_send","p":{"txt":"later","atts":[],"mode":"send","at":%d}}`, created.ID, time.Now().Add(time.Minute).UnixMilli())),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 1 {
		t.Fatalf("expected a single error frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "err" {
		t.Fatalf("expected error frame, got %#v", conn.frames[0])
	}
	if conn.frames[0].Message != errCodexNotInstalled {
		t.Fatalf("expected message %q, got %q", errCodexNotInstalled, conn.frames[0].Message)
	}
}

func TestManagerBroadcastFiltersHistoryFramesByFocusedSession(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	focusedConn := &captureWSConn{}
	otherConn := &captureWSConn{}
	focusedClient := manager.RegisterEventClient(focusedConn)
	otherClient := manager.RegisterEventClient(otherConn)
	defer manager.UnregisterClient(focusedClient)
	defer manager.UnregisterClient(otherClient)

	handled, err := manager.HandleHeartbeatPayload(
		focusedClient,
		[]byte(`{"v":1,"k":"hb","ts":1710000000000,"op":"focus","sid":"session-1"}`),
	)
	if err != nil {
		t.Fatalf("HandleHeartbeatPayload returned error: %v", err)
	}
	if !handled {
		t.Fatal("expected focus heartbeat payload to be handled")
	}

	manager.broadcast(newHistoryItemFrame("session-1", HistoryItem{
		ID:         "hist-1",
		OrderIndex: 1,
		Kind:       "assistant",
		ItemType:   "agent_message",
		Text:       "delta",
	}, nil))

	if len(focusedConn.frames) != 1 {
		t.Fatalf("expected focused client to receive one history frame, got %d", len(focusedConn.frames))
	}
	if len(otherConn.frames) != 0 {
		t.Fatalf("expected unfocused client to receive no history frame, got %d", len(otherConn.frames))
	}

	manager.broadcast(newSessionFrame("session-1", SessionSummary{
		ID:        "session-1",
		ProjectID: "project-1",
		Title:     "Session 1",
		Agent:     AgentCodex,
		Status:    StatusRunning,
	}))

	if len(focusedConn.frames) != 2 {
		t.Fatalf("expected focused client to receive session summary, got %d frames", len(focusedConn.frames))
	}
	if len(otherConn.frames) != 1 {
		t.Fatalf("expected unfocused client to still receive session summary, got %d", len(otherConn.frames))
	}
	if otherConn.frames[0].Operation != "session" {
		t.Fatalf("expected unfocused client frame to be session summary, got op=%q", otherConn.frames[0].Operation)
	}
}

func TestHandleApprovalCommandRepliesWithAckAndSnapshot(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "approval"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "make the edit", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestFileChangeApproval)
	if request == nil {
		t.Fatal("expected pending approval request")
	}

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_approve","sid":%q,"op":"approve","p":{}}`, created.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 2 {
		t.Fatalf("expected approve ack and snapshot frames, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "approve" {
		t.Fatalf("expected first frame to be approve ack, got %#v", conn.frames[0])
	}
	if conn.frames[1].Kind != "snap" || conn.frames[1].SessionID != created.ID {
		t.Fatalf("expected second frame to be approval snapshot, got %#v", conn.frames[1])
	}
	if conn.frames[1].Session == nil {
		t.Fatalf("expected snapshot summary after approval, got %#v", conn.frames[1])
	}

	waitForSessionToSettle(t, manager, created.ID)
}

func TestShouldMarkSessionUnreadForEvent(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  bool
	}{
		{name: "approval request", event: Event{Type: "approval_req"}, want: true},
		{name: "user input request", event: Event{Type: "user_input_req"}, want: true},
		{name: "run fail", event: Event{Type: "run_fail"}, want: true},
		{name: "run done", event: Event{Type: "run_done"}, want: true},
		{
			name:  "unexpected abort with reason",
			event: Event{Type: "run_abort", Payload: map[string]any{"reason": "process_restart"}},
			want:  true,
		},
		{name: "manual abort without payload", event: Event{Type: "run_abort"}, want: false},
		{name: "text delta", event: Event{Type: "txt_d"}, want: false},
		{name: "tool start", event: Event{Type: "tool_st"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldMarkSessionUnreadForEvent(tt.event); got != tt.want {
				t.Fatalf("shouldMarkSessionUnreadForEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManagerClientHeartbeatClosesIdleConnections(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	originalInterval := webSessionHeartbeatInterval
	originalTimeout := webSessionHeartbeatTimeout
	webSessionHeartbeatInterval = 10 * time.Millisecond
	webSessionHeartbeatTimeout = 25 * time.Millisecond
	defer func() {
		webSessionHeartbeatInterval = originalInterval
		webSessionHeartbeatTimeout = originalTimeout
	}()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	conn := newHeartbeatWSConn()
	client := manager.RegisterEventClient(conn)
	defer manager.UnregisterClient(client)

	select {
	case <-conn.closed:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected idle heartbeat client to be closed")
	}

	frames := conn.snapshotFrames()
	if len(frames) == 0 {
		t.Fatal("expected at least one heartbeat ping before close")
	}
	if frames[0].Kind != "hb" || frames[0].Operation != "ping" {
		t.Fatalf("expected first heartbeat frame to be ping, got %#v", frames[0])
	}
}

func TestManagerBroadcastSessionSummarySkipsArchivedSessions(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Archived Session", 1000)
	archivedAt := time.Now()
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Update("archived_at", &archivedAt).Error; err != nil {
		t.Fatalf("archive session failed: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	eventConn := &captureWSConn{}
	eventClient := manager.RegisterEventClient(eventConn)
	defer manager.UnregisterClient(eventClient)

	manager.broadcastSessionSummary(context.Background(), session.ID)

	if len(eventConn.frames) != 0 {
		t.Fatalf("expected archived session summary to produce no broadcast frames, got %d", len(eventConn.frames))
	}
}

func TestManagerBroadcastSnapshotSkipsArchivedSessions(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Archived Snapshot", 1000)
	archivedAt := time.Now()
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Update("archived_at", &archivedAt).Error; err != nil {
		t.Fatalf("archive session failed: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	eventConn := &captureWSConn{}
	eventClient := manager.RegisterEventClient(eventConn)
	defer manager.UnregisterClient(eventClient)

	if err := manager.broadcastSnapshot(context.Background(), session.ID); err != nil {
		t.Fatalf("broadcastSnapshot returned error: %v", err)
	}
	if len(eventConn.frames) != 0 {
		t.Fatalf("expected archived snapshot broadcast to produce no frames, got %d", len(eventConn.frames))
	}
}

func TestManagerAppendAndBroadcastPersistsArchivedHistoryWithoutRealtimeFrames(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Archived History", 1000)
	archivedAt := time.Now()
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Update("archived_at", &archivedAt).Error; err != nil {
		t.Fatalf("archive session failed: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	eventConn := &captureWSConn{}
	eventClient := manager.RegisterEventClient(eventConn)
	defer manager.UnregisterClient(eventClient)

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}

	eventTime := time.Now().UTC().Truncate(time.Millisecond)
	appended, err := manager.appendAndBroadcast(context.Background(), session.ID, record, Event{
		ID:        "evt_archived_note",
		Type:      "note",
		Timestamp: eventTime,
		Payload: map[string]any{
			"txt": "keep this history",
			"lvl": "info",
		},
	})
	if err != nil {
		t.Fatalf("appendAndBroadcast returned error: %v", err)
	}
	if appended.Seq != 1 {
		t.Fatalf("expected appended event seq 1, got %d", appended.Seq)
	}
	if len(eventConn.frames) != 0 {
		t.Fatalf("expected archived append to produce no realtime frames, got %d", len(eventConn.frames))
	}

	history, err := manager.History(context.Background(), session.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Items) != 1 {
		t.Fatalf("expected 1 archived history item, got %d", len(history.Items))
	}
	if history.Items[0].ItemType != "note" {
		t.Fatalf("expected archived history item type note, got %q", history.Items[0].ItemType)
	}
	if history.Items[0].Text != "keep this history" {
		t.Fatalf("expected archived history text to be preserved, got %q", history.Items[0].Text)
	}
}

func TestParseCodexContextWindowRootLevelOnly(t *testing.T) {
	raw := `
model_context_window = 1000000 # root setting

[model_providers.OpenAI]
model_context_window = 123
`

	got, ok := parseCodexContextWindow(raw)
	if !ok {
		t.Fatal("expected parseCodexContextWindow to succeed")
	}
	if got != 1000000 {
		t.Fatalf("expected 1000000, got %d", got)
	}
}

func TestManagerListSessionsIncludesConfiguredContextWindow(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	configDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir failed: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(configDir, "config.toml"),
		[]byte("model = \"gpt-5.5\"\nmodel_context_window = 1000000\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	project := seedProject(t)
	seedWebSession(t, project.ID, "Codex", 1000)

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	items, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 session, got %d", len(items))
	}
	if items[0].ContextWindowTokens == nil || *items[0].ContextWindowTokens != 1000000 {
		t.Fatalf("expected contextWindowTokens 1000000, got %#v", items[0].ContextWindowTokens)
	}
	if items[0].ContextWindowSource != ContextWindowSourceConfig {
		t.Fatalf("expected contextWindowSource %q, got %q", ContextWindowSourceConfig, items[0].ContextWindowSource)
	}
	config := manager.GetCodexRuntimeConfig()
	if config.Model != "gpt-5.5" {
		t.Fatalf("expected runtime model gpt-5.5, got %q", config.Model)
	}
	if config.ContextWindowTokens != 1000000 {
		t.Fatalf("expected runtime context window 1000000, got %d", config.ContextWindowTokens)
	}
	if config.CompactLimitTokens != 1000000 {
		t.Fatalf("expected runtime compact limit fallback 1000000, got %d", config.CompactLimitTokens)
	}
}

func TestManagerListSessionsDoesNotUseContextWindowFromDifferentConfiguredModel(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	configDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir failed: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(configDir, "config.toml"),
		[]byte("model = \"gpt-5.4\"\nmodel_context_window = 1000000\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	project := seedProject(t)
	seedWebSession(t, project.ID, "Codex", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	items, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 session, got %d", len(items))
	}
	if items[0].ContextWindowTokens != nil {
		t.Fatalf("expected no context window for a model override, got %#v", items[0].ContextWindowTokens)
	}
	if items[0].ContextWindowSource != ContextWindowSourceUnavailable {
		t.Fatalf("expected contextWindowSource %q, got %q", ContextWindowSourceUnavailable, items[0].ContextWindowSource)
	}
}

func TestUpdateModelClearsObservedContextWindow(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Codex", 1000)
	observedAt := time.Now()
	if err := model.GetDB().Model(session).Updates(map[string]any{
		"session_context_window_tokens":      int64(353400),
		"session_context_window_observed_at": observedAt,
	}).Error; err != nil {
		t.Fatalf("seed observed context window failed: %v", err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if _, err := manager.UpdateModel(context.Background(), session.ID, "gpt-5.6-luna"); err != nil {
		t.Fatalf("UpdateModel returned error: %v", err)
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.SessionContextWindowTokens != 0 {
		t.Fatalf("expected observed context window to be cleared, got %d", record.SessionContextWindowTokens)
	}
	if record.SessionContextWindowObservedAt != nil {
		t.Fatalf("expected observed context timestamp to be cleared, got %v", record.SessionContextWindowObservedAt)
	}
}

func TestManagerCountSessionsByProjectSkipsArchivedSessions(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	projectA := seedProject(t)
	projectB := seedProject(t)
	seedWebSession(t, projectA.ID, "A-1", 1000)
	archived := seedWebSession(t, projectA.ID, "A-2", 2000)
	seedWebSession(t, projectB.ID, "B-1", 1000)
	seedWebSession(t, projectB.ID, "B-2", 2000)

	archivedAt := time.Now()
	if err := model.GetDB().Model(archived).Update("archived_at", &archivedAt).Error; err != nil {
		t.Fatalf("archive session failed: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	counts, err := manager.CountSessionsByProject(context.Background())
	if err != nil {
		t.Fatalf("CountSessionsByProject returned error: %v", err)
	}

	if got := counts[projectA.ID]; got != 1 {
		t.Fatalf("expected project A count 1, got %d", got)
	}
	if got := counts[projectB.ID]; got != 2 {
		t.Fatalf("expected project B count 2, got %d", got)
	}
}

func TestManagerListSessionsMarksClaudeContextWindowUnavailable(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Claude", 1000)
	if err := model.GetDB().Model(session).Update("agent", string(AgentClaude)).Error; err != nil {
		t.Fatalf("update session agent failed: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	items, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 session, got %d", len(items))
	}
	if items[0].ContextWindowTokens != nil {
		t.Fatalf("expected nil contextWindowTokens, got %#v", items[0].ContextWindowTokens)
	}
	if items[0].ContextWindowSource != ContextWindowSourceUnavailable {
		t.Fatalf("expected contextWindowSource %q, got %q", ContextWindowSourceUnavailable, items[0].ContextWindowSource)
	}
}

func TestGetCodexRuntimeConfigIncludesBinaryCapabilities(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{
		DataDir:    t.TempDir(),
		CodexPath:  writeFakeCodexVersionCLI(t, "0.133.0"),
		ClaudePath: writeFakeClaudeStreamCLI(t),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	config := manager.GetCodexRuntimeConfig()
	if !config.HasCodex {
		t.Fatal("expected hasCodex true")
	}
	if !config.HasClaudeCode {
		t.Fatal("expected hasClaudeCode true")
	}
	if config.CodexVersion == nil || *config.CodexVersion != "0.133.0" {
		t.Fatalf("expected codexVersion 0.133.0, got %#v", config.CodexVersion)
	}
	if !config.SupportsGoalMode {
		t.Fatal("expected supportsGoalMode true")
	}
	if config.GoalModeMinVersion != "0.133.0" {
		t.Fatalf("expected goalModeMinVersion 0.133.0, got %q", config.GoalModeMinVersion)
	}
}

func TestGetCodexRuntimeConfigGoalModeDisabledForOldVersion(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.132.9"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	config := manager.GetCodexRuntimeConfig()
	if !config.HasCodex {
		t.Fatal("expected hasCodex true")
	}
	if config.CodexVersion == nil || *config.CodexVersion != "0.132.9" {
		t.Fatalf("expected codexVersion 0.132.9, got %#v", config.CodexVersion)
	}
	if config.SupportsGoalMode {
		t.Fatal("expected supportsGoalMode false")
	}
}

func TestGetCodexRuntimeConfigIncludesModelReasoningCatalog(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexModelCatalogCLI(t),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	config := manager.GetCodexRuntimeConfigWithModels()
	if len(config.Models) != 3 {
		t.Fatalf("expected 3 catalog models, got %#v", config.Models)
	}
	if got := config.Models[0].SupportedReasoningEfforts; !reflect.DeepEqual(got, []ReasoningEffort{
		ReasoningEffortLow,
		ReasoningEffortMedium,
		ReasoningEffortHigh,
		ReasoningEffortXHigh,
		ReasoningEffortMax,
		ReasoningEffortUltra,
	}) {
		t.Fatalf("unexpected Sol reasoning efforts: %#v", got)
	}
	if got := config.Models[2].SupportedReasoningEfforts; !reflect.DeepEqual(got, []ReasoningEffort{
		ReasoningEffortLow,
		ReasoningEffortMedium,
		ReasoningEffortHigh,
		ReasoningEffortXHigh,
		ReasoningEffortMax,
	}) {
		t.Fatalf("unexpected Luna reasoning efforts: %#v", got)
	}
}

func TestManagerListSessionsNormalizesLegacyStaleSyncState(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Legacy Stale", 1000)
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"sync_state": string(SyncStateStale),
			"updated_at": time.Now(),
		}).Error; err != nil {
		t.Fatalf("update session sync_state failed: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	items, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 session, got %d", len(items))
	}
	if items[0].SyncState != SyncStateFresh {
		t.Fatalf(
			"expected legacy stale sync_state to normalize to %q, got %q",
			SyncStateFresh,
			items[0].SyncState,
		)
	}
}

func TestDecodeToolQuestionsPreservesStructuredQuestions(t *testing.T) {
	questions := []toolRequestQuestion{
		{
			ID:       "scope",
			Header:   "范围",
			Question: "这次要验证哪种计划模式交互？",
			IsOther:  true,
			Options: []toolRequestOption{
				{Label: "仅草稿组内", Description: "保持现在的草稿分组。"},
				{Label: "整个标签系统统一", Description: "统一插入逻辑。"},
			},
		},
	}

	got := decodeToolQuestions(questions)
	if len(got) != 1 {
		t.Fatalf("expected 1 question, got %d", len(got))
	}
	if got[0].ID != questions[0].ID || got[0].Header != questions[0].Header || got[0].Question != questions[0].Question {
		t.Fatalf("expected question to be preserved, got %#v", got[0])
	}
	if len(got[0].Options) != len(questions[0].Options) {
		t.Fatalf("expected %d options, got %d", len(questions[0].Options), len(got[0].Options))
	}
}

func TestManagerMoveSessionRenormalizesProjectOrder(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	first := seedWebSession(t, project.ID, "First", 1000)
	second := seedWebSession(t, project.ID, "Second", 2000)
	third := seedWebSession(t, project.ID, "Third", 3000)

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	moved, err := manager.MoveSession(context.Background(), third.ID, "", first.ID)
	if err != nil {
		t.Fatalf("MoveSession returned error: %v", err)
	}
	if moved.OrderIndex != 1000 {
		t.Fatalf("expected moved session orderIndex 1000, got %.2f", moved.OrderIndex)
	}

	sessions, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	expectedIDs := []string{third.ID, first.ID, second.ID}
	for index, session := range sessions {
		if session.ID != expectedIDs[index] {
			t.Fatalf("expected session %s at index %d, got %s", expectedIDs[index], index, session.ID)
		}
		expectedOrder := float64(index+1) * sessionOrderStep
		if session.OrderIndex != expectedOrder {
			t.Fatalf("expected orderIndex %.2f at index %d, got %.2f", expectedOrder, index, session.OrderIndex)
		}
	}
}

func TestManagerArchiveSessionKeepsHistoryAndMovesSessionOutOfCurrentList(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	dataDir := t.TempDir()
	session := seedWebSession(t, project.ID, "Archive Me", 1000)

	manager, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if err := manager.store.appendEvent(session.ID, Event{
		ID:        "evt_history",
		Seq:       1,
		Type:      "note",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"txt": "keep this history",
		},
	}); err != nil {
		t.Fatalf("appendEvent returned error: %v", err)
	}

	archived, err := manager.ArchiveSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ArchiveSession returned error: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("expected archivedAt to be set")
	}

	current, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(current) != 0 {
		t.Fatalf("expected archived session to be removed from current list, got %d items", len(current))
	}

	archivedResult, err := manager.ListArchivedSessions(context.Background(), []string{project.ID}, 20, 0)
	if err != nil {
		t.Fatalf("ListArchivedSessions returned error: %v", err)
	}
	if archivedResult.Total != 1 || len(archivedResult.Items) != 1 {
		t.Fatalf("expected exactly one archived session, got total=%d items=%d", archivedResult.Total, len(archivedResult.Items))
	}
	if archivedResult.Items[0].ID != session.ID {
		t.Fatalf("expected archived session %s, got %s", session.ID, archivedResult.Items[0].ID)
	}
	if _, err := os.Stat(manager.store.historyPath(session.ID)); err != nil {
		t.Fatalf("expected archived history file to remain on disk: %v", err)
	}
}

func TestManagerUnarchiveSessionRestoresSessionToCurrentListEnd(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	first := seedWebSession(t, project.ID, "First", 1000)
	second := seedWebSession(t, project.ID, "Second", 2000)
	archivedAt := time.Now().Add(-time.Hour)
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", first.ID).
		Updates(map[string]any{
			"archived_at": archivedAt,
			"updated_at":  time.Now(),
		}).Error; err != nil {
		t.Fatalf("failed to archive seed session: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	restored, err := manager.UnarchiveSession(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("UnarchiveSession returned error: %v", err)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("expected archivedAt to be cleared")
	}
	if restored.OrderIndex <= second.OrderIndex {
		t.Fatalf("expected restored session to move to the end, got orderIndex %.2f", restored.OrderIndex)
	}

	current, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(current) != 2 {
		t.Fatalf("expected 2 current sessions, got %d", len(current))
	}
	if current[0].ID != second.ID || current[1].ID != first.ID {
		t.Fatalf("unexpected current session order after unarchive: %#v", current)
	}
}

func TestManagerListArchivedSessionsPaginatesByActivityDescending(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	now := time.Now()
	sessionA := seedWebSession(t, project.ID, "A", 1000)
	sessionB := seedWebSession(t, project.ID, "B", 2000)
	sessionC := seedWebSession(t, project.ID, "C", 3000)
	for id, activityAt := range map[string]time.Time{
		sessionA.ID: now.Add(-3 * time.Hour),
		sessionB.ID: now.Add(-1 * time.Hour),
		sessionC.ID: now.Add(-2 * time.Hour),
	} {
		if err := model.GetDB().Model(&tables.WebSessionTable{}).
			Where("id = ?", id).
			Updates(map[string]any{
				"archived_at": now,
				"activity_at": activityAt,
				"updated_at":  now,
			}).Error; err != nil {
			t.Fatalf("failed to update archived seed %s: %v", id, err)
		}
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	pageOne, err := manager.ListArchivedSessions(context.Background(), []string{project.ID}, 2, 0)
	if err != nil {
		t.Fatalf("ListArchivedSessions page one returned error: %v", err)
	}
	if !pageOne.HasMore || pageOne.NextOffset != 2 {
		t.Fatalf("expected first page to have more results, got %+v", pageOne)
	}
	if len(pageOne.Items) != 2 || pageOne.Items[0].ID != sessionB.ID || pageOne.Items[1].ID != sessionC.ID {
		t.Fatalf("unexpected first archived page order: %#v", pageOne.Items)
	}

	pageTwo, err := manager.ListArchivedSessions(context.Background(), []string{project.ID}, 2, pageOne.NextOffset)
	if err != nil {
		t.Fatalf("ListArchivedSessions page two returned error: %v", err)
	}
	if pageTwo.HasMore {
		t.Fatalf("expected second page to be final, got %+v", pageTwo)
	}
	if len(pageTwo.Items) != 1 || pageTwo.Items[0].ID != sessionA.ID {
		t.Fatalf("unexpected second archived page order: %#v", pageTwo.Items)
	}
}

func TestDetectApprovalPrompt(t *testing.T) {
	t.Run("codex confirm prompt", func(t *testing.T) {
		prompt, ok := detectApprovalPrompt([]string{
			"❯ 1. Approve",
			"› 2. Cancel",
			"  Press enter to confirm or esc to cancel",
		})
		if !ok {
			t.Fatalf("expected approval prompt to be detected")
		}
		if prompt == "" {
			t.Fatalf("expected non-empty approval prompt")
		}
	})

	t.Run("claude proceed prompt", func(t *testing.T) {
		prompt, ok := detectApprovalPrompt([]string{
			"Do you want to proceed?",
			"Esc to exit",
		})
		if !ok {
			t.Fatalf("expected approval prompt to be detected")
		}
		if prompt == "" {
			t.Fatalf("expected non-empty approval prompt")
		}
	})
}

func TestBuildExecCommandCodexClosesStdinWhenPromptArgProvided(t *testing.T) {
	manager := &Manager{cfg: Config{CodexPath: "codex"}}
	session := tables.WebSessionTable{
		Agent:           string(AgentCodex),
		Model:           "gpt-5.4",
		WorkflowMode:    string(WorkflowModeDefault),
		PermissionLevel: string(PermissionLevelDefault),
		Cwd:             "/tmp/project",
	}

	cmd, stdinBytes, closeStdinAfterWrite, err := manager.buildExecCommand(
		context.Background(),
		session,
		"say hi briefly",
		nil,
	)
	if err != nil {
		t.Fatalf("buildExecCommand returned error: %v", err)
	}
	if closeStdinAfterWrite != true {
		t.Fatalf("expected stdin to be closed after launch when prompt arg is provided")
	}
	if len(stdinBytes) != 0 {
		t.Fatalf("expected no stdin bytes when using prompt argument, got %q", string(stdinBytes))
	}
	joinedArgs := strings.Join(cmd.Args, " ")
	if strings.Contains(joinedArgs, " - ") || strings.HasSuffix(joinedArgs, " -") {
		t.Fatalf("expected prompt argument mode, got args %v", cmd.Args)
	}
	if !strings.Contains(joinedArgs, "say hi briefly") {
		t.Fatalf("expected prompt to be passed as an argument, got args %v", cmd.Args)
	}
	if !strings.Contains(joinedArgs, "-s workspace-write") {
		t.Fatalf("expected default codex permissions to use workspace-write sandbox, got args %v", cmd.Args)
	}
}

func TestBuildExecCommandCodexUsesModelReasoningEffortConfigKey(t *testing.T) {
	manager := &Manager{cfg: Config{CodexPath: "codex"}}
	session := tables.WebSessionTable{
		Agent:           string(AgentCodex),
		Model:           "gpt-5.6-sol",
		ReasoningEffort: string(ReasoningEffortUltra),
		WorkflowMode:    string(WorkflowModeDefault),
		PermissionLevel: string(PermissionLevelDefault),
		Cwd:             "/tmp/project",
	}

	cmd, _, _, err := manager.buildExecCommand(context.Background(), session, "say hi", nil)
	if err != nil {
		t.Fatalf("buildExecCommand returned error: %v", err)
	}
	found := false
	for i := 0; i+1 < len(cmd.Args); i++ {
		if cmd.Args[i] == "-c" && cmd.Args[i+1] == `model_reasoning_effort="ultra"` {
			found = true
		}
		if cmd.Args[i] == "-c" && strings.HasPrefix(cmd.Args[i+1], `reasoning_effort=`) {
			t.Fatalf("legacy reasoning_effort config key must not be used: %v", cmd.Args)
		}
	}
	if !found {
		t.Fatalf("expected model_reasoning_effort config override, got %v", cmd.Args)
	}
}

func TestNormalizeCodexReasoningEffortUsesModelCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		effort ReasoningEffort
		want   ReasoningEffort
	}{
		{name: "Sol Ultra", model: "gpt-5.6-sol", effort: ReasoningEffortUltra, want: ReasoningEffortUltra},
		{name: "Terra Max", model: "gpt-5.6-terra", effort: ReasoningEffortMax, want: ReasoningEffortMax},
		{name: "Luna Ultra", model: "gpt-5.6-luna", effort: ReasoningEffortUltra, want: ReasoningEffortDefault},
		{name: "Sol None", model: "gpt-5.6-sol", effort: ReasoningEffortNone, want: ReasoningEffortDefault},
		{name: "Legacy None", model: "gpt-5.5", effort: ReasoningEffortNone, want: ReasoningEffortNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeCodexReasoningEffort(tt.model, tt.effort); got != tt.want {
				t.Fatalf("normalizeCodexReasoningEffort(%q, %q) = %q, want %q", tt.model, tt.effort, got, tt.want)
			}
		})
	}
}

func TestBuildExecCommandCodexElevatedPlanAddsPreambleAndFullAccess(t *testing.T) {
	manager := &Manager{cfg: Config{CodexPath: "codex"}}
	session := tables.WebSessionTable{
		Agent:           string(AgentCodex),
		Model:           "gpt-5.4",
		WorkflowMode:    string(WorkflowModePlan),
		PermissionLevel: string(PermissionLevelElevated),
		Cwd:             "/tmp/project",
	}

	cmd, stdinBytes, closeStdinAfterWrite, err := manager.buildExecCommand(
		context.Background(),
		session,
		"inspect this repo",
		nil,
	)
	if err != nil {
		t.Fatalf("buildExecCommand returned error: %v", err)
	}
	if closeStdinAfterWrite != true {
		t.Fatalf("expected stdin to be closed after launch when prompt arg is provided")
	}
	if len(stdinBytes) != 0 {
		t.Fatalf("expected no stdin bytes for prompt argument mode, got %q", string(stdinBytes))
	}
	joinedArgs := strings.Join(cmd.Args, " ")
	if !strings.Contains(joinedArgs, "-s danger-full-access") {
		t.Fatalf("expected elevated codex permissions to use danger-full-access, got args %v", cmd.Args)
	}
	if !strings.Contains(joinedArgs, "You are operating in planning mode.") {
		t.Fatalf("expected plan preamble to be injected, got args %v", cmd.Args)
	}
}

func TestNewManagerMigratesLegacyPermissionMode(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	legacySession := &tables.WebSessionTable{
		ProjectID:            project.ID,
		OrderIndex:           1000,
		Agent:                string(AgentCodex),
		Title:                "Legacy",
		Model:                "gpt-5.4",
		WorkflowMode:         "",
		PermissionLevel:      "",
		LegacyPermissionMode: "plan",
		Cwd:                  t.TempDir(),
		Status:               string(StatusIdle),
	}
	legacySession.Init()
	if err := model.GetDB().Create(legacySession).Error; err != nil {
		t.Fatalf("seed legacy web session failed: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	record, err := manager.GetSession(context.Background(), legacySession.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if effectiveWorkflowMode(record) != WorkflowModePlan {
		t.Fatalf("expected migrated workflow mode plan, got %q", effectiveWorkflowMode(record))
	}
	if effectivePermissionLevel(record) != PermissionLevelElevated {
		t.Fatalf("expected migrated permission level elevated, got %q", effectivePermissionLevel(record))
	}
}

func TestNewManagerRecoversInterruptedSessionsOnStartup(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	dataDir := t.TempDir()
	eventStore, err := newStore(dataDir)
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}

	nativeSessionID := "thread_existing"
	session := &tables.WebSessionTable{
		ProjectID:            project.ID,
		OrderIndex:           1000,
		Agent:                string(AgentCodex),
		Backend:              string(SessionBackendCodexAppServer),
		Title:                "Recover Me",
		Model:                "gpt-5.4",
		WorkflowMode:         string(WorkflowModeDefault),
		PermissionLevel:      string(PermissionLevelElevated),
		Cwd:                  t.TempDir(),
		NativeSessionID:      &nativeSessionID,
		Status:               string(StatusRunning),
		LastEventSeq:         1,
		HasUnread:            true,
		LastMessageAt:        nil,
		LegacyPermissionMode: "default",
	}
	session.Init()
	if err := model.GetDB().Create(session).Error; err != nil {
		t.Fatalf("seed web session failed: %v", err)
	}

	if err := eventStore.appendEvent(session.ID, Event{
		ID:        "evt_approval",
		Seq:       1,
		Type:      "approval_req",
		Timestamp: time.Now().Add(-time.Minute),
		Payload: map[string]any{
			"prompt": "Need approval to continue",
		},
	}); err != nil {
		t.Fatalf("appendEvent returned error: %v", err)
	}

	manager, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusIdle) {
		t.Fatalf("expected recovered status %q, got %q", StatusIdle, record.Status)
	}
	if record.HasUnread {
		t.Fatalf("expected recovered session unread flag to be cleared")
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) != nativeSessionID {
		t.Fatalf("expected native session id %q to be preserved, got %v", nativeSessionID, record.NativeSessionID)
	}
	if record.LastEventSeq != 2 {
		t.Fatalf("expected recovered lastEventSeq 2, got %d", record.LastEventSeq)
	}

	history, err := manager.History(context.Background(), session.ID, 10, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("expected 2 events after recovery, got %d", len(history.Events))
	}
	lastEvent := history.Events[len(history.Events)-1]
	if lastEvent.Type != "run_abort" {
		t.Fatalf("expected recovery event run_abort, got %q", lastEvent.Type)
	}
	if got := fmt.Sprint(lastEvent.Payload["reason"]); got != recoveryReasonProcessRestart {
		t.Fatalf("expected recovery reason %q, got %q", recoveryReasonProcessRestart, got)
	}
	if got := fmt.Sprint(lastEvent.Payload["msg"]); !strings.Contains(got, "app restarted") {
		t.Fatalf("expected recovery message to mention app restart, got %q", got)
	}
}

func TestSendMessageAutoRenamesTitleFromFirstUserMessage(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
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

	messageText := "修复网页会话标题自动命名的问题，并补一个回归测试。"
	if err := manager.SendMessage(context.Background(), created.ID, messageText, nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Title != messageText {
		t.Fatalf("expected auto title %q, got %q", messageText, record.Title)
	}
	if record.TitleAuto {
		t.Fatalf("expected title auto flag to be cleared")
	}
}

func TestSendMessageDoesNotOverrideManualTitle(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
		Title:     "Manual Title",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "这条消息不应该覆盖手动标题。", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Title != "Manual Title" {
		t.Fatalf("expected manual title to be preserved, got %q", record.Title)
	}
	if record.TitleAuto {
		t.Fatalf("expected manual title to remain non-auto")
	}
}

func TestSendMessageCodexAppServerPersistsThreadID(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) != "thread_test" {
		t.Fatalf("expected native session id thread_test, got %v", record.NativeSessionID)
	}
	if effectiveSessionBackend(record) != SessionBackendCodexAppServer {
		t.Fatalf("expected app-server backend, got %q", effectiveSessionBackend(record))
	}
	history, err := manager.History(context.Background(), created.ID, 200, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if historyHasToolKind(history.Events, "reasoning") {
		t.Fatalf("expected empty reasoning items to be filtered from projected history, got %#v", history.Events)
	}
	snapshot, err := manager.Snapshot(context.Background(), created.ID, 200)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if historyItemsHaveToolKind(snapshot.History.Items, "reasoning") {
		t.Fatalf("expected empty reasoning items to be filtered from cached history items, got %#v", snapshot.History.Items)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if !historyHasToolKind(rawEvents, "reasoning") {
		t.Fatalf("expected raw history to retain reasoning items, got %#v", rawEvents)
	}
}

func TestSendMessageCodexAppServerAllowsNextTurnAfterTurnCompleted(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	codexPath := writeFakeCodexAppServerCLI(t, "turn_complete_linger")
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: codexPath,
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "first", nil); err != nil {
		t.Fatalf("first SendMessage returned error: %v", err)
	}
	waitForSessionStatus(t, manager, created.ID, StatusDone)

	if err := manager.SendMessage(context.Background(), created.ID, "second", nil); err != nil {
		t.Fatalf("second SendMessage returned error after turn completed: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "first|second" {
		t.Fatalf("expected two user messages, got %q", got)
	}
	waitForFakeCodexAppServerExitCount(t, codexPath, 2)
}

func TestCodexAppServerIgnoresSubAgentTerminalEventsUntilRootTurnCompletes(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	codexPath := writeFakeCodexAppServerCLI(t, "sub_agent_thread_isolation")
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: codexPath,
	}, zap.NewNop())
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
	if err := manager.SendMessage(context.Background(), created.ID, "delegate and wait", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	t.Cleanup(func() {
		if manager.hasActiveRun(created.ID) {
			_ = manager.AbortSession(created.ID)
		}
	})

	waitForFile(t, codexPath+".state.child-completed")
	waitForHistoryToolEvent(t, manager, created.ID, "child_plan", "tool_end")

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusRunning) {
		t.Fatalf("expected root session to remain running after child completion, got %q", record.Status)
	}
	if record.NativeSessionID == nil || *record.NativeSessionID != "thread_test" {
		t.Fatalf("expected child thread start to preserve root native session id, got %#v", record.NativeSessionID)
	}
	if !manager.hasActiveRun(created.ID) {
		t.Fatal("expected active run to remain registered after child completion")
	}

	manager.mu.RLock()
	run := manager.runs[created.ID]
	manager.mu.RUnlock()
	if run == nil {
		t.Fatal("expected active run while root turn is still running")
	}
	run.mu.Lock()
	_, tracksChildCommand := run.activeCalls["child_command"]
	rootAssistantMessageID := run.assistantMessageID
	completedPlanTool := run.completedPlanTool
	lastError := run.lastError
	run.mu.Unlock()
	if tracksChildCommand {
		t.Fatal("expected child command to be excluded from root active-call tracking")
	}
	if rootAssistantMessageID != "" {
		t.Fatalf("expected child message not to replace root assistant message id, got %q", rootAssistantMessageID)
	}
	if completedPlanTool {
		t.Fatal("expected child plan not to mark the root plan as completed")
	}
	if lastError != "" {
		t.Fatalf("expected child error not to fail the root run, got %q", lastError)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if historyHasEvent(rawEvents, "run_done") || historyHasEvent(rawEvents, "run_fail") {
		t.Fatalf("expected no terminal run event before root completion, got %#v", rawEvents)
	}
	for _, event := range rawEvents {
		if event.Type == "usage" && int64(numberValue(event.Payload["in"])) == 999 {
			t.Fatalf("expected child usage not to overwrite root context accounting, got %#v", event)
		}
	}
	if err := os.WriteFile(codexPath+".state.release-root", []byte("1"), 0o644); err != nil {
		t.Fatalf("release fake root turn: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)
	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error after root completion: %v", err)
	}
	terminalCount := 0
	sawChildResult := false
	sawRootAnswer := false
	for _, event := range rawEvents {
		if event.Type == "run_done" {
			terminalCount++
		}
		if event.Type == "tool_end" && strings.Contains(eventToolOutput(event), "child result preserved") {
			sawChildResult = true
		}
		if event.Type == "txt_d" && strings.Contains(stringValue(event.Payload["txt"]), "root-finished-after-child") {
			sawRootAnswer = true
		}
	}
	if terminalCount != 1 {
		t.Fatalf("expected exactly one root run_done event, got %d", terminalCount)
	}
	if !sawChildResult {
		t.Fatal("expected completed sub-agent wait result to be preserved")
	}
	if !sawRootAnswer {
		t.Fatal("expected root answer after sub-agent completion")
	}
}

func TestCodexAppServerTransportRetryPersistsAsNoteAndCompletes(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "reconnect_then_success"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected session status %q, got %q", StatusDone, record.Status)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if historyHasEvent(rawEvents, "run_fail") {
		t.Fatalf("expected retrying run to avoid run_fail, got %#v", rawEvents)
	}
	retryNoteFound := false
	for _, event := range rawEvents {
		if event.Type != "note" {
			continue
		}
		if stringValue(event.Payload["code"]) != codexTransportRetryingCode {
			continue
		}
		retryNoteFound = true
		if got := int(numberValue(event.Payload["attempt"])); got != 1 {
			t.Fatalf("expected retry attempt 1, got %d", got)
		}
		if got := int(numberValue(event.Payload["maxAttempts"])); got != 5 {
			t.Fatalf("expected max attempts 5, got %d", got)
		}
		if got := stringValue(event.Payload["remoteUrl"]); got != "https://proxy.example.test/v1" {
			t.Fatalf("expected sanitized retry remote URL, got %q", got)
		}
		if got := stringValue(event.Payload["txt"]); !strings.HasSuffix(got, "\nhttps://proxy.example.test/v1") {
			t.Fatalf("expected retry note text to include the remote URL, got %q", got)
		}
		break
	}
	if !retryNoteFound {
		t.Fatalf("expected transport retry note in raw events, got %#v", rawEvents)
	}

	history, err := manager.History(context.Background(), created.ID, 50, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	retryItemFound := false
	for _, item := range history.Items {
		if item.ItemType != "note" {
			continue
		}
		if stringValue(item.Payload["code"]) != codexTransportRetryingCode {
			continue
		}
		retryItemFound = true
		if got := stringValue(item.Payload["remoteUrl"]); got != "https://proxy.example.test/v1" {
			t.Fatalf("expected projected retry remote URL, got %q", got)
		}
		if !strings.HasSuffix(item.Text, "\nhttps://proxy.example.test/v1") {
			t.Fatalf("expected projected retry text to include the remote URL, got %q", item.Text)
		}
		break
	}
	if !retryItemFound {
		t.Fatalf("expected projected retry note in history items, got %#v", history.Items)
	}
}

func TestCodexAppServerTransportRetryContinuesWhenRemoteURLIsUnavailable(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "config_read_unsupported_reconnect"),
	}, zap.NewNop())
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
	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected session status %q, got %q", StatusDone, record.Status)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	for _, event := range rawEvents {
		if event.Type != "note" || stringValue(event.Payload["code"]) != codexTransportRetryingCode {
			continue
		}
		if got := stringValue(event.Payload["remoteUrl"]); got != "" {
			t.Fatalf("expected retry note without an unverified remote URL, got %q", got)
		}
		if got := stringValue(event.Payload["txt"]); strings.Contains(got, "proxy.example.test") {
			t.Fatalf("expected retry note text without a guessed remote URL, got %q", got)
		}
		return
	}
	t.Fatalf("expected transport retry note in raw events, got %#v", rawEvents)
}

func TestCodexAppServerTransportRetryExhaustionFailsRun(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "reconnect_then_fail"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusError) {
		t.Fatalf("expected session status %q, got %q", StatusError, record.Status)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	retryNoteCount := 0
	var finalFailure Event
	for _, event := range rawEvents {
		if event.Type == "note" && stringValue(event.Payload["code"]) == codexTransportRetryingCode {
			retryNoteCount++
		}
		if event.Type == "run_fail" {
			finalFailure = event
		}
	}
	if retryNoteCount < 2 {
		t.Fatalf("expected multiple retry notes before failure, got %#v", rawEvents)
	}
	if finalFailure.Type != "run_fail" {
		t.Fatalf("expected final run_fail event, got %#v", rawEvents)
	}
	if got := stringValue(finalFailure.Payload["code"]); got != codexTransportRetryExhaustedCode {
		t.Fatalf("expected final failure code %q, got %q", codexTransportRetryExhaustedCode, got)
	}

	history, err := manager.History(context.Background(), created.ID, 50, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	historyRetryCount := 0
	historyFailFound := false
	for _, item := range history.Items {
		if item.ItemType == "note" && stringValue(item.Payload["code"]) == codexTransportRetryingCode {
			historyRetryCount++
		}
		if item.ItemType == "run_fail" && stringValue(item.Payload["code"]) == codexTransportRetryExhaustedCode {
			historyFailFound = true
		}
	}
	if historyRetryCount < 2 {
		t.Fatalf("expected retry notes in projected history, got %#v", history.Items)
	}
	if !historyFailFound {
		t.Fatalf("expected projected final run_fail item, got %#v", history.Items)
	}
}

func TestAutoRetryEnabledSessionContinuesAfterFailure(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "auto_retry_then_success"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:        project.ID,
		Agent:            AgentCodex,
		AutoRetryEnabled: true,
		AutoRetryScope:   AutoRetryScopeNetworkOnly,
		AutoRetryPreset:  AutoRetryPresetAggressiveStop,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		record, getErr := manager.GetSession(context.Background(), created.ID)
		if getErr != nil {
			t.Fatalf("GetSession returned error: %v", getErr)
		}
		if record.Status == string(StatusDone) && !manager.hasActiveRun(created.ID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected session status %q after auto retry, got %q", StatusDone, record.Status)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	userMessages := make([]Event, 0, 2)
	for _, event := range rawEvents {
		if event.Type == "msg_u" {
			userMessages = append(userMessages, event)
		}
	}
	if len(userMessages) < 2 {
		t.Fatalf("expected auto retry to append a second user message, got %#v", rawEvents)
	}
	if got := stringValue(userMessages[len(userMessages)-1].Payload["txt"]); got != "continue" {
		t.Fatalf("expected automatic retry message %q, got %q", "continue", got)
	}
}

func TestAutoRetryEnabledMidRunAppliesToCurrentFailure(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "delayed_failure_then_success"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:        project.ID,
		Agent:            AgentCodex,
		AutoRetryEnabled: false,
		AutoRetryScope:   AutoRetryScopeNetworkOnly,
		AutoRetryPreset:  AutoRetryPresetAggressiveStop,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	runDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(runDeadline) {
		if manager.hasActiveRun(created.ID) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !manager.hasActiveRun(created.ID) {
		t.Fatalf("expected session %s to still be running before failure", created.ID)
	}

	if _, err := manager.UpdateAutoRetry(
		context.Background(),
		created.ID,
		true,
		AutoRetryScopeNetworkOnly,
		AutoRetryPresetAggressiveStop,
	); err != nil {
		t.Fatalf("UpdateAutoRetry returned error: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		record, getErr := manager.GetSession(context.Background(), created.ID)
		if getErr != nil {
			t.Fatalf("GetSession returned error: %v", getErr)
		}
		if record.Status == string(StatusDone) && !manager.hasActiveRun(created.ID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected session status %q after mid-run enable, got %q", StatusDone, record.Status)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	userMessages := make([]Event, 0, 2)
	for _, event := range rawEvents {
		if event.Type == "msg_u" {
			userMessages = append(userMessages, event)
		}
	}
	if len(userMessages) < 2 {
		t.Fatalf("expected auto retry to append a second user message, got %#v", rawEvents)
	}
	if got := stringValue(userMessages[len(userMessages)-1].Payload["txt"]); got != "continue" {
		t.Fatalf("expected automatic retry message %q, got %q", "continue", got)
	}
}

func TestUpdateAutoRetryOnErroredSessionSchedulesContinue(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "auto_retry_then_success"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:        project.ID,
		Agent:            AgentCodex,
		AutoRetryEnabled: false,
		AutoRetryScope:   AutoRetryScopeNetworkOnly,
		AutoRetryPreset:  AutoRetryPresetAggressiveStop,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	errorDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(errorDeadline) {
		record, getErr := manager.GetSession(context.Background(), created.ID)
		if getErr != nil {
			t.Fatalf("GetSession returned error: %v", getErr)
		}
		if record.Status == string(StatusError) && !manager.hasActiveRun(created.ID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusError) {
		t.Fatalf("expected session status %q before enabling auto retry, got %q", StatusError, record.Status)
	}

	if _, err := manager.UpdateAutoRetry(
		context.Background(),
		created.ID,
		true,
		AutoRetryScopeNetworkOnly,
		AutoRetryPresetAggressiveStop,
	); err != nil {
		t.Fatalf("UpdateAutoRetry returned error: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		record, getErr := manager.GetSession(context.Background(), created.ID)
		if getErr != nil {
			t.Fatalf("GetSession returned error: %v", getErr)
		}
		if record.Status == string(StatusDone) && !manager.hasActiveRun(created.ID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	record, err = manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected session status %q after enabling auto retry on error, got %q", StatusDone, record.Status)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	userMessages := make([]Event, 0, 2)
	for _, event := range rawEvents {
		if event.Type == "msg_u" {
			userMessages = append(userMessages, event)
		}
	}
	if len(userMessages) < 2 {
		t.Fatalf("expected auto retry to append a second user message, got %#v", rawEvents)
	}
	if got := stringValue(userMessages[len(userMessages)-1].Payload["txt"]); got != "continue" {
		t.Fatalf("expected automatic retry message %q, got %q", "continue", got)
	}
}

func TestActiveCallTimeoutInterruptsCommandExecutionAndContinues(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	timeoutConfig := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{
		EnabledMode:          utils.SettingModeOn,
		TimeoutMode:          utils.WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 10,
		PromptTemplate:       "The ${call} call timed out after ${duration}. Continue.",
		CallKinds: utils.WebSessionActiveCallTimeoutKindsConfig{
			UseDefault: false,
			Command:    true,
		},
	})
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "active_call_timeout_command_then_success"),
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return timeoutConfig
		},
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	call := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindCommand)
	setTrackedActiveCallStartedAt(t, manager, created.ID, call.ToolID, time.Now().Add(-12*time.Second))
	manager.RefreshDeveloperConfig()

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected session status %q after active call timeout recovery, got %q", StatusDone, record.Status)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if countEventsByType(rawEvents, "run_abort") == 0 {
		t.Fatalf("expected run_abort event after active call timeout, got %#v", rawEvents)
	}

	var timeoutAbort Event
	for _, event := range rawEvents {
		if event.Type == "run_abort" && stringValue(event.Payload["reason"]) == activeCallTimeoutReason {
			timeoutAbort = event
			break
		}
	}
	if timeoutAbort.Type != "run_abort" {
		t.Fatalf("expected active timeout run_abort payload, got %#v", rawEvents)
	}
	if got := stringValue(timeoutAbort.Payload["callKind"]); got != string(activeCallTimeoutKindCommand) {
		t.Fatalf("expected callKind %q, got %q", activeCallTimeoutKindCommand, got)
	}
	if got := stringValue(timeoutAbort.Payload["call"]); !strings.Contains(got, "pnpm dev --host 127.0.0.1 --port 4173") {
		t.Fatalf("expected timeout payload call label to include command text, got %q", got)
	}

	messages := userMessageTexts(rawEvents)
	if len(messages) < 2 {
		t.Fatalf("expected timeout recovery to append a follow-up user message, got %#v", rawEvents)
	}
	if got := messages[len(messages)-1]; !strings.Contains(got, "pnpm dev --host 127.0.0.1 --port 4173") || !strings.Contains(got, "timed out after") {
		t.Fatalf("expected rendered prompt with placeholders, got %q", got)
	}
}

func TestActiveCallTimeoutDefaultKindsSkipCommandExecution(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	timeoutConfig := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{
		EnabledMode:    utils.SettingModeOn,
		PromptTemplate: "The ${call} call timed out after ${duration}. Continue.",
	})
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "active_call_timeout_command_then_success"),
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return timeoutConfig
		},
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	call := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindCommand)
	setTrackedActiveCallStartedAt(t, manager, created.ID, call.ToolID, time.Now().Add(-12*time.Second))
	manager.RefreshDeveloperConfig()
	time.Sleep(150 * time.Millisecond)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	for _, event := range rawEvents {
		if event.Type == "run_abort" && stringValue(event.Payload["reason"]) == activeCallTimeoutReason {
			t.Fatalf("expected default monitored kinds to skip command execution, got %#v", rawEvents)
		}
	}

	if err := manager.AbortSession(created.ID); err != nil {
		t.Fatalf("AbortSession returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestActiveCallTimeoutSessionOverrideDisabledSkipsGlobalOn(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	timeoutConfig := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{
		EnabledMode:          utils.SettingModeOn,
		TimeoutMode:          utils.WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 10,
		PromptTemplate:       "The ${call} call timed out after ${duration}. Continue.",
		CallKinds: utils.WebSessionActiveCallTimeoutKindsConfig{
			UseDefault: false,
			Command:    true,
		},
	})
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "active_call_timeout_command_then_success"),
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return timeoutConfig
		},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:                project.ID,
		Agent:                    AgentCodex,
		ActiveCallTimeoutEnabled: ptr(false),
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if created.ActiveCallTimeoutEnabled {
		t.Fatalf("expected session active call timeout override to be disabled")
	}

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	call := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindCommand)
	setTrackedActiveCallStartedAt(t, manager, created.ID, call.ToolID, time.Now().Add(-12*time.Second))
	manager.RefreshDeveloperConfig()
	time.Sleep(150 * time.Millisecond)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	for _, event := range rawEvents {
		if event.Type == "run_abort" && stringValue(event.Payload["reason"]) == activeCallTimeoutReason {
			t.Fatalf("expected session override to skip active call timeout, got %#v", rawEvents)
		}
	}

	if err := manager.AbortSession(created.ID); err != nil {
		t.Fatalf("AbortSession returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestActiveCallTimeoutSessionOverrideEnabledOverridesGlobalOff(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	timeoutConfig := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{
		EnabledMode:          utils.SettingModeOff,
		TimeoutMode:          utils.WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 10,
		PromptTemplate:       "The ${call} call timed out after ${duration}. Continue.",
		CallKinds: utils.WebSessionActiveCallTimeoutKindsConfig{
			UseDefault: false,
			Command:    true,
		},
	})
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "active_call_timeout_command_then_success"),
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return timeoutConfig
		},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:                project.ID,
		Agent:                    AgentCodex,
		ActiveCallTimeoutEnabled: ptr(true),
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if !created.ActiveCallTimeoutEnabled {
		t.Fatalf("expected session active call timeout override to be enabled")
	}

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	call := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindCommand)
	setTrackedActiveCallStartedAt(t, manager, created.ID, call.ToolID, time.Now().Add(-12*time.Second))
	manager.RefreshDeveloperConfig()

	waitForSessionToSettle(t, manager, created.ID)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	for _, event := range rawEvents {
		if event.Type == "run_abort" && stringValue(event.Payload["reason"]) == activeCallTimeoutReason {
			return
		}
	}
	t.Fatalf("expected session override to trigger active call timeout, got %#v", rawEvents)
}

func TestActiveCallTimeoutSkipsSubAgentToolCalls(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	timeoutConfig := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{
		EnabledMode:          utils.SettingModeOn,
		TimeoutMode:          utils.WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 10,
		PromptTemplate:       "Continue after ${call}.",
		CallKinds: utils.WebSessionActiveCallTimeoutKindsConfig{
			UseDefault: false,
			Tool:       true,
		},
	})
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "active_call_timeout_sub_agent"),
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return timeoutConfig
		},
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "delegate research", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	call := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindSubAgent)
	setTrackedActiveCallStartedAt(t, manager, created.ID, call.ToolID, time.Now().Add(-12*time.Second))
	manager.RefreshDeveloperConfig()
	time.Sleep(150 * time.Millisecond)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	for _, event := range rawEvents {
		if event.Type == "run_abort" && stringValue(event.Payload["reason"]) == activeCallTimeoutReason {
			t.Fatalf("expected sub agent call to skip active call timeout, got %#v", rawEvents)
		}
	}
	messages := userMessageTexts(rawEvents)
	if len(messages) != 1 {
		t.Fatalf("expected no automatic continue message for sub agent timeout, got %#v", messages)
	}

	snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if !historyItemsHaveToolKind(snapshot.History.Items, "sub_agent_tool_call") {
		t.Fatalf("expected snapshot to include normalized sub agent tool call, got %#v", snapshot.History.Items)
	}
	var subAgentItem *HistoryItem
	for idx := range snapshot.History.Items {
		item := &snapshot.History.Items[idx]
		if item.Tool != nil && item.Tool.Kind == "sub_agent_tool_call" {
			subAgentItem = item
			break
		}
	}
	if subAgentItem == nil {
		t.Fatalf("expected snapshot to include one sub agent tool item, got %#v", snapshot.History.Items)
	}
	if got := stringValue(subAgentItem.Tool.Meta["subtitle"]); got != "Inspect current sub-agent support" {
		t.Fatalf("expected sub agent subtitle to preserve prompt summary, got %q", got)
	}

	if err := manager.AbortSession(created.ID); err != nil {
		t.Fatalf("AbortSession returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestActiveCallTimeoutUsesLatestTrackedCall(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	timeoutConfig := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{
		EnabledMode:          utils.SettingModeOn,
		TimeoutMode:          utils.WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 10,
		PromptTemplate:       "Continue after ${call}.",
	})
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "active_call_timeout_latest_then_success"),
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return timeoutConfig
		},
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForTrackedActiveCallCount(t, manager, created.ID, 2)
	commandCall := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindCommand)
	mcpCall := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindMCP)
	setTrackedActiveCallStartedAt(t, manager, created.ID, commandCall.ToolID, time.Now().Add(-20*time.Second))
	setTrackedActiveCallStartedAt(t, manager, created.ID, mcpCall.ToolID, time.Now().Add(-12*time.Second))
	manager.RefreshDeveloperConfig()

	waitForSessionToSettle(t, manager, created.ID)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	var timeoutAbort Event
	for _, event := range rawEvents {
		if event.Type == "run_abort" && stringValue(event.Payload["reason"]) == activeCallTimeoutReason {
			timeoutAbort = event
			break
		}
	}
	if timeoutAbort.Type != "run_abort" {
		t.Fatalf("expected active timeout run_abort payload, got %#v", rawEvents)
	}
	if got := stringValue(timeoutAbort.Payload["callKind"]); got != string(activeCallTimeoutKindMCP) {
		t.Fatalf("expected latest timed-out callKind %q, got %q", activeCallTimeoutKindMCP, got)
	}
	if got := stringValue(timeoutAbort.Payload["call"]); !strings.Contains(strings.ToLower(got), "settings") {
		t.Fatalf("expected timeout payload to mention MCP call, got %q", got)
	}
}

func TestActiveCallTimeoutPausesDuringApproval(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	timeoutConfig := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{
		EnabledMode:          utils.SettingModeOn,
		TimeoutMode:          utils.WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 10,
		PromptTemplate:       "Continue after ${call}.",
		CallKinds: utils.WebSessionActiveCallTimeoutKindsConfig{
			UseDefault: false,
			Tool:       true,
		},
	})
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "active_call_timeout_approval_then_success"),
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return timeoutConfig
		},
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "apply the patch", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestFileChangeApproval)
	if request == nil {
		t.Fatal("expected pending approval request")
	}
	call := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindTool)
	setTrackedActiveCallStartedAt(t, manager, created.ID, call.ToolID, time.Now().Add(-12*time.Second))
	manager.RefreshDeveloperConfig()
	time.Sleep(150 * time.Millisecond)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if countEventsByType(rawEvents, "run_abort") != 0 {
		t.Fatalf("expected no timeout abort while approval is pending, got %#v", rawEvents)
	}

	if err := manager.respondToApproval(created.ID, "approve"); err != nil {
		t.Fatalf("respondToApproval returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if countEventsByType(rawEvents, "run_abort") == 0 {
		t.Fatalf("expected timeout abort after approval resumed execution, got %#v", rawEvents)
	}
}

func TestActiveCallTimeoutPausesDuringUserInput(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	timeoutConfig := utils.NormalizeWebSessionActiveCallTimeoutConfig(utils.WebSessionActiveCallTimeoutConfig{
		EnabledMode:          utils.SettingModeOn,
		TimeoutMode:          utils.WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 10,
		PromptTemplate:       "Continue after ${call}.",
		CallKinds: utils.WebSessionActiveCallTimeoutKindsConfig{
			UseDefault: false,
			MCP:        true,
		},
	})
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "active_call_timeout_user_input_then_success"),
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return timeoutConfig
		},
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "inspect scope", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestUserInput)
	if request == nil {
		t.Fatal("expected pending user input request")
	}
	call := waitForTrackedActiveCall(t, manager, created.ID, activeCallTimeoutKindMCP)
	setTrackedActiveCallStartedAt(t, manager, created.ID, call.ToolID, time.Now().Add(-12*time.Second))
	manager.RefreshDeveloperConfig()
	time.Sleep(150 * time.Millisecond)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if countEventsByType(rawEvents, "run_abort") != 0 {
		t.Fatalf("expected no timeout abort while user input is pending, got %#v", rawEvents)
	}

	if err := manager.respondToUserInput(created.ID, request.ItemID, map[string][]string{
		"scope": []string{"Continue"},
	}); err != nil {
		t.Fatalf("respondToUserInput returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if countEventsByType(rawEvents, "run_abort") == 0 {
		t.Fatalf("expected timeout abort after user input resumed execution, got %#v", rawEvents)
	}
}

func TestRespondToUserInputCodexAppServer(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "user_input"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "plan this change", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestUserInput)
	if request == nil {
		t.Fatal("expected pending user input request")
	}
	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusRunning) {
		t.Fatalf("expected session status %q while waiting for input, got %q", StatusRunning, record.Status)
	}
	if record.AssistantState != string(AssistantStateWaitingInput) {
		t.Fatalf("expected assistant state %q, got %q", AssistantStateWaitingInput, record.AssistantState)
	}

	if err := manager.respondToUserInput(created.ID, request.ItemID, map[string][]string{
		"scope": {"full migration"},
	}); err != nil {
		t.Fatalf("respondToUserInput returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if !historyHasEvent(rawEvents, "user_input_req") {
		t.Fatalf("expected user_input_req event, got %#v", rawEvents)
	}
	if !historyHasEvent(rawEvents, "user_input_res") {
		t.Fatalf("expected user_input_res event, got %#v", rawEvents)
	}
}

func TestUserInputRequestProjectionPersistsSourceItemID(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Needs Input", 1000)

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	requestID := "req_input_123"
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_user_input",
		Seq:       1,
		Type:      "user_input_req",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"iid": requestID,
			"txt": "Please choose a scope",
			"qs": []map[string]any{
				{
					"id":       "scope",
					"header":   "Scope",
					"question": "Which scope should I use?",
					"options": []map[string]any{
						{
							"label":       "Full migration",
							"description": "Apply all changes",
						},
					},
				},
			},
		},
	})

	history, err := manager.History(context.Background(), session.ID, 10, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(history.Items))
	}
	if history.Items[0].SourceItemID == nil || *history.Items[0].SourceItemID != requestID {
		t.Fatalf("expected source item id %q, got %v", requestID, history.Items[0].SourceItemID)
	}

	snapshot, err := manager.Snapshot(context.Background(), session.ID, 10)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	frame := newSnapshotFrame(session.ID, snapshot)
	if frame.History == nil || len(frame.History.Items) != 1 {
		t.Fatalf("expected snapshot frame history item, got %#v", frame.History)
	}
	if frame.History.Items[0].SourceItemID == nil || *frame.History.Items[0].SourceItemID != requestID {
		t.Fatalf(
			"expected wire snapshot source item id %q, got %v",
			requestID,
			frame.History.Items[0].SourceItemID,
		)
	}

	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_user_input_response",
		Seq:       2,
		Type:      "user_input_res",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"iid": requestID,
			"ans": map[string]any{
				"scope": []any{"Full migration"},
			},
		},
	})

	history, err = manager.History(context.Background(), session.ID, 10, nil)
	if err != nil {
		t.Fatalf("History returned error after response: %v", err)
	}
	if len(history.Items) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(history.Items))
	}
	response := history.Items[1]
	if response.SourceItemID == nil || *response.SourceItemID != requestID {
		t.Fatalf("expected response source item id %q, got %v", requestID, response.SourceItemID)
	}
	if response.Detail == nil || len(response.Detail.Answers) != 1 {
		t.Fatalf("expected response answer detail, got %#v", response.Detail)
	}
	answer := response.Detail.Answers[0]
	if answer.Label != "Scope" {
		t.Fatalf("expected answer label Scope, got %q", answer.Label)
	}
	if len(answer.Values) != 1 || answer.Values[0] != "Full migration" {
		t.Fatalf("expected answer value Full migration, got %#v", answer.Values)
	}
}

func TestRespondToApprovalCodexAppServer(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "approval"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "make the edit", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestFileChangeApproval)
	if request == nil {
		t.Fatal("expected pending approval request")
	}
	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusRunning) {
		t.Fatalf("expected session status %q while waiting for approval, got %q", StatusRunning, record.Status)
	}

	if err := manager.respondToApproval(created.ID, "approve"); err != nil {
		t.Fatalf("respondToApproval returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if !historyHasEvent(rawEvents, "approval_req") {
		t.Fatalf("expected approval_req event, got %#v", rawEvents)
	}
	if !historyHasEvent(rawEvents, "approval_res") {
		t.Fatalf("expected approval_res event, got %#v", rawEvents)
	}
}

func TestCodexPlanCompletionSetsWaitingApprovalStatus(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "plan"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "inspect and plan", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusRunning) {
		t.Fatalf("expected session status %q, got %q", StatusRunning, record.Status)
	}
	if record.AssistantState != string(AssistantStateWaitingPlanApproval) {
		t.Fatalf("expected assistant state %q, got %q", AssistantStateWaitingPlanApproval, record.AssistantState)
	}

	history, err := manager.History(context.Background(), created.ID, 200, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if !historyHasToolKind(history.Events, "plan") {
		t.Fatalf("expected plan tool history, got %#v", history.Events)
	}
}

func TestCodexPlanCompletionUsesDoneStatusOutsidePlanWorkflow(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "plan"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModeDefault,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "plan and continue", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected session status %q, got %q", StatusDone, record.Status)
	}
	if record.AssistantState != "" {
		t.Fatalf("expected assistant state to be cleared, got %q", record.AssistantState)
	}
}

func TestSendMessageClearsWaitingApprovalStatus(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "plan"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "plan first", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusRunning) {
		t.Fatalf("expected first completion status %q, got %q", StatusRunning, record.Status)
	}
	if record.AssistantState != string(AssistantStateWaitingPlanApproval) {
		t.Fatalf("expected first assistant state %q, got %q", AssistantStateWaitingPlanApproval, record.AssistantState)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "implement now", nil); err != nil {
		t.Fatalf("second SendMessage returned error: %v", err)
	}

	record, err = manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error after second send: %v", err)
	}
	if record.Status != string(StatusRunning) {
		t.Fatalf("expected second send to move status to %q, got %q", StatusRunning, record.Status)
	}
	if record.AssistantState != string(AssistantStateWorking) {
		t.Fatalf("expected second send to move assistant state to %q, got %q", AssistantStateWorking, record.AssistantState)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err = manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error after second completion: %v", err)
	}
	if record.Status != string(StatusRunning) {
		t.Fatalf("expected second completion status %q, got %q", StatusRunning, record.Status)
	}
	if record.AssistantState != string(AssistantStateWaitingPlanApproval) {
		t.Fatalf("expected second assistant state %q, got %q", AssistantStateWaitingPlanApproval, record.AssistantState)
	}
}

func TestSendMessageWithModeQueuesUntilActiveRunStops(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "approval"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "first", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestFileChangeApproval)
	if request == nil {
		t.Fatal("expected pending approval request for the first run")
	}

	if err := manager.sendMessageWithMode(
		context.Background(),
		created.ID,
		"queued",
		nil,
		PendingInputModeQueue,
		"",
	); err != nil {
		t.Fatalf("sendMessageWithMode(queue) returned error: %v", err)
	}

	pending := manager.pendingInputsSnapshot(created.ID)
	if len(pending) != 1 || pending[0].Text != "queued" || pending[0].Mode != PendingInputModeQueue {
		t.Fatalf("expected one queued pending input, got %#v", pending)
	}

	snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.PendingInputs) != 1 || snapshot.PendingInputs[0].Text != "queued" {
		t.Fatalf("expected snapshot pending inputs to include queued item, got %#v", snapshot.PendingInputs)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := userMessageTexts(rawEvents); len(got) != 1 || got[0] != "first" {
		t.Fatalf("expected only the first user message before abort, got %#v", got)
	}

	if err := manager.AbortSession(created.ID); err != nil {
		t.Fatalf("AbortSession returned error: %v", err)
	}
	waitForUserMessageCount(t, manager, created.ID, 2)

	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error after flush: %v", err)
	}
	if got := userMessageTexts(rawEvents); strings.Join(got, "|") != "first|queued" {
		t.Fatalf("expected queued message to flush after abort, got %#v", got)
	}
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected pending inputs to be cleared after flush, got %#v", pending)
	}

	if err := manager.AbortSession(created.ID); err != nil {
		t.Fatalf("AbortSession returned error while cleaning up: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestSendMessageWithModeRedirectWaitsForCurrentStepBeforeInterrupting(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "step_redirect"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "first", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	waitForTrackedActiveCallID(t, manager, created.ID, "cmd_step_2")

	if err := manager.sendMessageWithMode(
		context.Background(),
		created.ID,
		"queued",
		nil,
		PendingInputModeQueue,
		"",
	); err != nil {
		t.Fatalf("sendMessageWithMode(queue) returned error: %v", err)
	}
	if err := manager.sendMessageWithMode(
		context.Background(),
		created.ID,
		"redirected",
		nil,
		PendingInputModeRedirect,
		"",
	); err != nil {
		t.Fatalf("sendMessageWithMode(redirect) returned error: %v", err)
	}

	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 2 || pending[0].Text != "redirected" {
		t.Fatalf("expected redirect item to be first in pending queue, got %#v", pending)
	}

	time.Sleep(30 * time.Millisecond)
	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error while checking immediate redirect behavior: %v", err)
	}
	if got := userMessageTexts(rawEvents); strings.Join(got, "|") != "first" {
		t.Fatalf("expected redirect not to interrupt the current step immediately, got %#v", got)
	}

	waitForUserMessageCount(t, manager, created.ID, 2)

	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := userMessageTexts(rawEvents); strings.Join(got, "|") != "first|redirected" {
		t.Fatalf("expected redirect message to run after the current step boundary and before queued message, got %#v", got)
	}
	pending := manager.pendingInputsSnapshot(created.ID)
	if len(pending) != 1 || pending[0].Text != "queued" || pending[0].Mode != PendingInputModeQueue {
		t.Fatalf("expected queued message to remain pending after redirect flush, got %#v", pending)
	}

	waitForUserMessageCount(t, manager, created.ID, 3)

	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error after second flush: %v", err)
	}
	if got := userMessageTexts(rawEvents); strings.Join(got, "|") != "first|redirected|queued" {
		t.Fatalf("expected queued message to flush after redirect run stopped, got %#v", got)
	}
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected pending inputs to be empty after second flush, got %#v", pending)
	}

	if err := manager.AbortSession(created.ID); err != nil {
		t.Fatalf("AbortSession returned error while cleaning up: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestSendMessageResumesRecoveredCodexSession(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	nativeSessionID := "thread_resume_only"
	session := &tables.WebSessionTable{
		ProjectID:            project.ID,
		OrderIndex:           1000,
		Agent:                string(AgentCodex),
		Backend:              string(SessionBackendCodexAppServer),
		Title:                "Resume Existing",
		Model:                "gpt-5.4",
		WorkflowMode:         string(WorkflowModeDefault),
		PermissionLevel:      string(PermissionLevelElevated),
		LegacyPermissionMode: "default",
		Cwd:                  t.TempDir(),
		NativeSessionID:      &nativeSessionID,
		Status:               string(StatusRunning),
	}
	session.Init()
	if err := model.GetDB().Create(session).Error; err != nil {
		t.Fatalf("seed web session failed: %v", err)
	}

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "resume_only"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), session.ID, "continue the existing thread", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, session.ID)

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected resumed session status %q, got %q", StatusDone, record.Status)
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) != nativeSessionID {
		t.Fatalf("expected resumed native session id %q, got %v", nativeSessionID, record.NativeSessionID)
	}

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if historyHasEvent(history.Events, "run_fail") {
		t.Fatalf("expected resume_only session to avoid run_fail, got %#v", history.Events)
	}
}

func TestHistoryAggregatesConsecutiveCommandExecutions(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Grouped Commands", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_st",
		Seq:       1,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(1_000),
		Payload: map[string]any{
			"tid":  "cmd1",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "ls"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "ls"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_end",
		Seq:       2,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_000),
		Payload: map[string]any{
			"tid": "cmd1",
			"out": "ls output",
			"ok":  true,
			"meta": map[string]any{
				"kind":  "command_execution",
				"title": "CommandExecution",
			},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_reasoning_empty_end",
		Seq:       3,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_500),
		Payload: map[string]any{
			"tid":  "rs1",
			"name": "Reasoning",
			"kind": "reasoning",
			"out":  "",
			"ok":   true,
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd2_st",
		Seq:       4,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(3_000),
		Payload: map[string]any{
			"tid":  "cmd2",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "pwd"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "pwd"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd2_end",
		Seq:       5,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(4_000),
		Payload: map[string]any{
			"tid": "cmd2",
			"out": "pwd output",
			"ok":  true,
			"meta": map[string]any{
				"kind":  "command_execution",
				"title": "CommandExecution",
			},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_note",
		Seq:       6,
		Type:      "note",
		Timestamp: time.UnixMilli(5_000),
		Payload: map[string]any{
			"txt": "done",
		},
	})

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("expected 2 projected events, got %d", len(history.Events))
	}

	grouped := history.Events[0]
	if grouped.Type != "tool_end" {
		t.Fatalf("expected grouped event type tool_end, got %q", grouped.Type)
	}
	if grouped.Seq != 5 {
		t.Fatalf("expected grouped event seq 5, got %d", grouped.Seq)
	}
	if got := fmt.Sprint(grouped.Payload["tid"]); got != commandExecutionGroupID("cmd1") {
		t.Fatalf("expected grouped tool id %q, got %q", commandExecutionGroupID("cmd1"), got)
	}
	groupMeta := decodeRawObject(decodeRawObject(grouped.Payload["meta"])["commandGroup"])
	if got := int(numberValue(groupMeta["count"])); got != 2 {
		t.Fatalf("expected grouped count 2, got %d", got)
	}
	input := decodeRawObject(grouped.Payload["in"])
	if got := stringValue(input["command"]); got != "pwd" {
		t.Fatalf("expected latest grouped command pwd, got %q", got)
	}
	if got := stringValue(grouped.Payload["out"]); got != "pwd output" {
		t.Fatalf("expected latest grouped output, got %q", got)
	}

	if history.Events[1].Type != "note" {
		t.Fatalf("expected second event note, got %q", history.Events[1].Type)
	}
}

func TestGetCommandExecutionGroupReturnsFullItems(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Grouped Commands", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_st",
		Seq:       1,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(1_000),
		Payload: map[string]any{
			"tid":  "cmd1",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "ls"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "ls"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_end",
		Seq:       2,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_000),
		Payload: map[string]any{
			"tid": "cmd1",
			"out": "ls output",
			"ok":  true,
			"meta": map[string]any{
				"kind":  "command_execution",
				"title": "CommandExecution",
			},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd2_st",
		Seq:       3,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(3_000),
		Payload: map[string]any{
			"tid":  "cmd2",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "pwd"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "pwd"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd2_end",
		Seq:       4,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(4_000),
		Payload: map[string]any{
			"tid": "cmd2",
			"out": "pwd output",
			"ok":  false,
			"meta": map[string]any{
				"kind":  "command_execution",
				"title": "CommandExecution",
			},
		},
	})

	group, err := manager.GetCommandExecutionGroup(
		context.Background(),
		session.ID,
		commandExecutionGroupID("cmd1"),
	)
	if err != nil {
		t.Fatalf("GetCommandExecutionGroup returned error: %v", err)
	}
	if group.Count != 2 {
		t.Fatalf("expected group count 2, got %d", group.Count)
	}
	if group.FirstSeq != 1 || group.LastSeq != 4 {
		t.Fatalf("expected seq range 1-4, got %d-%d", group.FirstSeq, group.LastSeq)
	}
	if group.Status != "error" {
		t.Fatalf("expected latest status error, got %q", group.Status)
	}
	if len(group.Items) != 2 {
		t.Fatalf("expected 2 group items, got %d", len(group.Items))
	}
	if group.Items[0].Command != "ls" || group.Items[0].Output != "ls output" {
		t.Fatalf("unexpected first group item: %#v", group.Items[0])
	}
	if group.Items[1].Command != "pwd" || group.Items[1].Status != "error" {
		t.Fatalf("unexpected second group item: %#v", group.Items[1])
	}
}

func TestProjectHistoryEventsSeparatesCompactToolsWhenExplicitGroupChanges(t *testing.T) {
	events := []Event{
		{
			ID:        "evt_cmd1_st",
			Seq:       1,
			Type:      "tool_st",
			Timestamp: time.UnixMilli(1_000),
			Payload: map[string]any{
				"tid":  "cmd1",
				"name": "CommandExecution",
				"kind": "command_execution",
				"in":   map[string]any{"command": "ls"},
				"meta": map[string]any{
					"kind":  "command_execution",
					"title": "CommandExecution",
					"commandGroup": map[string]any{
						"id":           commandExecutionGroupID("cmd1"),
						"count":        1,
						"latestToolId": "cmd1",
						"compacted":    true,
					},
				},
			},
		},
		{
			ID:        "evt_cmd1_end",
			Seq:       2,
			Type:      "tool_end",
			Timestamp: time.UnixMilli(2_000),
			Payload: map[string]any{
				"tid":  "cmd1",
				"kind": "command_execution",
				"out":  "ls output",
				"ok":   true,
				"meta": map[string]any{
					"kind":  "command_execution",
					"title": "CommandExecution",
					"commandGroup": map[string]any{
						"id":           commandExecutionGroupID("cmd1"),
						"count":        1,
						"latestToolId": "cmd1",
						"compacted":    true,
					},
				},
			},
		},
		{
			ID:        "evt_cmd2_st",
			Seq:       3,
			Type:      "tool_st",
			Timestamp: time.UnixMilli(3_000),
			Payload: map[string]any{
				"tid":  "cmd2",
				"name": "CommandExecution",
				"kind": "command_execution",
				"in":   map[string]any{"command": "pwd"},
				"meta": map[string]any{
					"kind":  "command_execution",
					"title": "CommandExecution",
					"commandGroup": map[string]any{
						"id":           commandExecutionGroupID("cmd2"),
						"count":        1,
						"latestToolId": "cmd2",
						"compacted":    true,
					},
				},
			},
		},
		{
			ID:        "evt_cmd2_end",
			Seq:       4,
			Type:      "tool_end",
			Timestamp: time.UnixMilli(4_000),
			Payload: map[string]any{
				"tid":  "cmd2",
				"kind": "command_execution",
				"out":  "pwd output",
				"ok":   true,
				"meta": map[string]any{
					"kind":  "command_execution",
					"title": "CommandExecution",
					"commandGroup": map[string]any{
						"id":           commandExecutionGroupID("cmd2"),
						"count":        1,
						"latestToolId": "cmd2",
						"compacted":    true,
					},
				},
			},
		},
	}

	projected := projectHistoryEvents(events, AgentCodex)
	if len(projected) != 2 {
		t.Fatalf("expected 2 projected events, got %d", len(projected))
	}
	if got := eventCommandGroupID(projected[0]); got != commandExecutionGroupID("cmd1") {
		t.Fatalf("expected first group id %q, got %q", commandExecutionGroupID("cmd1"), got)
	}
	if got := eventCommandGroupID(projected[1]); got != commandExecutionGroupID("cmd2") {
		t.Fatalf("expected second group id %q, got %q", commandExecutionGroupID("cmd2"), got)
	}

	groups := buildCommandExecutionGroupLookup(events, AgentCodex)
	if len(groups) != 2 {
		t.Fatalf("expected 2 command group details, got %d", len(groups))
	}
	if _, ok := groups[commandExecutionGroupID("cmd1")]; !ok {
		t.Fatalf("expected group %q in lookup", commandExecutionGroupID("cmd1"))
	}
	if _, ok := groups[commandExecutionGroupID("cmd2")]; !ok {
		t.Fatalf("expected group %q in lookup", commandExecutionGroupID("cmd2"))
	}
}

func TestHistoryReasoningWithContentDoesNotBreakCodexCommandExecutionGroup(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Grouped Commands", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_st",
		Seq:       1,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(1_000),
		Payload: map[string]any{
			"tid":  "cmd1",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "ls"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "ls"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_end",
		Seq:       2,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_000),
		Payload: map[string]any{
			"tid": "cmd1",
			"out": "ls output",
			"ok":  true,
			"meta": map[string]any{
				"kind":  "command_execution",
				"title": "CommandExecution",
			},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_reasoning_end",
		Seq:       3,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_500),
		Payload: map[string]any{
			"tid":  "rs1",
			"name": "Reasoning",
			"kind": "reasoning",
			"out":  "Need to try a different command.",
			"ok":   true,
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd2_st",
		Seq:       4,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(3_000),
		Payload: map[string]any{
			"tid":  "cmd2",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "pwd"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "pwd"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd2_end",
		Seq:       5,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(4_000),
		Payload: map[string]any{
			"tid": "cmd2",
			"out": "pwd output",
			"ok":  true,
			"meta": map[string]any{
				"kind":  "command_execution",
				"title": "CommandExecution",
			},
		},
	})

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("expected 2 projected events, got %d", len(history.Events))
	}
	if history.Events[0].Type != "tool_end" || eventToolKind(history.Events[0]) != "reasoning" {
		t.Fatalf("expected first event reasoning, got %#v", history.Events[0])
	}
	if history.Events[1].Type != "tool_end" || eventToolKind(history.Events[1]) != "command_execution" {
		t.Fatalf("expected second event grouped command execution, got %#v", history.Events[1])
	}
	groupMeta := decodeRawObject(decodeRawObject(history.Events[1].Payload["meta"])["commandGroup"])
	if got := int(numberValue(groupMeta["count"])); got != 2 {
		t.Fatalf("expected grouped count 2, got %d", got)
	}
}

func TestHistoryReasoningWithContentBreaksClaudeCommandExecutionGroup(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSessionWithAgent(t, project.ID, "Grouped Commands", 1000, AgentClaude)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_st",
		Seq:       1,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(1_000),
		Payload: map[string]any{
			"tid":  "cmd1",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "ls"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "ls"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_end",
		Seq:       2,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_000),
		Payload: map[string]any{
			"tid": "cmd1",
			"out": "ls output",
			"ok":  true,
			"meta": map[string]any{
				"kind":  "command_execution",
				"title": "CommandExecution",
			},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_reasoning_end",
		Seq:       3,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_500),
		Payload: map[string]any{
			"tid":  "rs1",
			"name": "Reasoning",
			"kind": "reasoning",
			"out":  "Need to try a different command.",
			"ok":   true,
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd2_st",
		Seq:       4,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(3_000),
		Payload: map[string]any{
			"tid":  "cmd2",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "pwd"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "pwd"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd2_end",
		Seq:       5,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(4_000),
		Payload: map[string]any{
			"tid": "cmd2",
			"out": "pwd output",
			"ok":  true,
			"meta": map[string]any{
				"kind":  "command_execution",
				"title": "CommandExecution",
			},
		},
	})

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Events) != 3 {
		t.Fatalf("expected 3 projected events, got %d", len(history.Events))
	}
	if history.Events[0].Type != "tool_end" || eventToolKind(history.Events[0]) != "command_execution" {
		t.Fatalf("expected first event grouped command execution, got %#v", history.Events[0])
	}
	if history.Events[1].Type != "tool_end" || eventToolKind(history.Events[1]) != "reasoning" {
		t.Fatalf("expected second event reasoning, got %#v", history.Events[1])
	}
	if history.Events[2].Type != "tool_end" || eventToolKind(history.Events[2]) != "command_execution" {
		t.Fatalf("expected third event grouped command execution, got %#v", history.Events[2])
	}
}

func TestHistoryAggregatesConsecutiveFileChanges(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Grouped File Changes", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_fc1_st",
		Seq:       1,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(1_000),
		Payload: map[string]any{
			"tid":  "fc1",
			"name": "FileChange",
			"kind": "file_change",
			"in": map[string]any{
				"path": "ui/src/App.vue",
				"changes": []any{
					map[string]any{"path": "ui/src/App.vue"},
				},
			},
			"meta": map[string]any{"kind": "file_change", "title": "FileChange", "subtitle": "ui/src/App.vue"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_fc1_end",
		Seq:       2,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_000),
		Payload: map[string]any{
			"tid": "fc1",
			"out": "patched",
			"ok":  true,
			"meta": map[string]any{
				"kind": "file_change", "title": "FileChange", "subtitle": "ui/src/App.vue",
			},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_fc2_st",
		Seq:       3,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(3_000),
		Payload: map[string]any{
			"tid":  "fc2",
			"name": "FileChange",
			"kind": "file_change",
			"in": map[string]any{
				"path": "ui/src/components/Panel.vue",
				"changes": []any{
					map[string]any{"path": "ui/src/components/Panel.vue"},
				},
			},
			"meta": map[string]any{"kind": "file_change", "title": "FileChange", "subtitle": "ui/src/components/Panel.vue"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_fc2_end",
		Seq:       4,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(4_000),
		Payload: map[string]any{
			"tid": "fc2",
			"out": "patched",
			"ok":  true,
			"meta": map[string]any{
				"kind": "file_change", "title": "FileChange", "subtitle": "ui/src/components/Panel.vue",
			},
		},
	})

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Events) != 1 {
		t.Fatalf("expected 1 projected file_change event, got %d", len(history.Events))
	}
	if got := eventToolKind(history.Events[0]); got != "file_change" {
		t.Fatalf("expected file_change kind, got %q", got)
	}
	groupMeta := decodeRawObject(decodeRawObject(history.Events[0].Payload["meta"])["commandGroup"])
	if got := int(numberValue(groupMeta["count"])); got != 2 {
		t.Fatalf("expected grouped count 2, got %d", got)
	}
	if got := stringValue(decodeRawObject(history.Events[0].Payload["meta"])["subtitle"]); got != "ui/src/components/Panel.vue" {
		t.Fatalf("expected latest file path summary, got %q", got)
	}
}

func TestHistoryUsageDoesNotResetLiveFileChangeGroup(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Usage Between File Changes", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	appendHistoryEvent(t, manager, session.ID, testFileChangeEvent("fc1", 1, "tool_st", "ui/src/App.vue", ""))
	appendHistoryEvent(t, manager, session.ID, testFileChangeEvent("fc1", 2, "tool_end", "ui/src/App.vue", ""))
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_usage",
		Seq:       3,
		Type:      "usage",
		Timestamp: time.UnixMilli(2_500),
		Payload: map[string]any{
			"in":  19585,
			"cin": 5504,
			"out": 648,
		},
	})
	appendHistoryEvent(t, manager, session.ID, testFileChangeEvent("fc2", 4, "tool_st", "ui/src/components/Panel.vue", ""))
	appendHistoryEvent(t, manager, session.ID, testFileChangeEvent("fc2", 5, "tool_end", "ui/src/components/Panel.vue", ""))

	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	fc2Start, ok := historyEventByID(events, "evt_fc2_tool_st")
	if !ok {
		t.Fatalf("expected fc2 start event in raw history")
	}
	if got := eventExplicitCommandGroupID(fc2Start); got != commandExecutionGroupID("fc1") {
		t.Fatalf("expected usage to preserve live group id %q, got %q", commandExecutionGroupID("fc1"), got)
	}

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("expected usage plus grouped file_change event, got %d", len(history.Events))
	}
	if history.Events[0].Type != "usage" {
		t.Fatalf("expected first projected event usage, got %#v", history.Events[0])
	}
	if got := eventToolKind(history.Events[1]); got != "file_change" {
		t.Fatalf("expected grouped file_change event, got %q", got)
	}
	groupMeta := decodeRawObject(decodeRawObject(history.Events[1].Payload["meta"])["commandGroup"])
	if got := int(numberValue(groupMeta["count"])); got != 2 {
		t.Fatalf("expected grouped count 2, got %d", got)
	}

	snapshot, err := manager.Snapshot(context.Background(), session.ID, 20)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.History.Items) != 1 {
		t.Fatalf("expected 1 grouped history item, got %d", len(snapshot.History.Items))
	}
	item := snapshot.History.Items[0]
	if item.Tool == nil || item.Tool.CommandGroup == nil {
		t.Fatalf("expected grouped file_change history item, got %#v", item)
	}
	if item.Tool.CommandGroup.ID != commandExecutionGroupID("fc1") || item.Tool.CommandGroup.Count != 2 {
		t.Fatalf("expected file_change group %q count 2, got %#v", commandExecutionGroupID("fc1"), item.Tool.CommandGroup)
	}
	if got := len(decodeHistoryGroupItems(item.Payload)); got != 2 {
		t.Fatalf("expected 2 file_change detail items, got %d", got)
	}
}

func TestProjectHistoryEventsTreatsUsageAsTransparentBetweenExplicitFileChangeGroups(t *testing.T) {
	events := []Event{
		testFileChangeEvent("fc1", 1, "tool_st", "ui/src/App.vue", commandExecutionGroupID("fc1")),
		testFileChangeEvent("fc1", 2, "tool_end", "ui/src/App.vue", commandExecutionGroupID("fc1")),
		{
			ID:        "evt_usage",
			Seq:       3,
			Type:      "usage",
			Timestamp: time.UnixMilli(2_500),
			Payload: map[string]any{
				"in":  19585,
				"cin": 5504,
				"out": 648,
			},
		},
		testFileChangeEvent("fc2", 4, "tool_st", "ui/src/components/Panel.vue", commandExecutionGroupID("fc2")),
		testFileChangeEvent("fc2", 5, "tool_end", "ui/src/components/Panel.vue", commandExecutionGroupID("fc2")),
	}

	projected := projectHistoryEvents(events, AgentCodex)
	if len(projected) != 2 {
		t.Fatalf("expected usage plus one grouped file_change event, got %d", len(projected))
	}
	if projected[0].Type != "usage" {
		t.Fatalf("expected usage event to be preserved, got %#v", projected[0])
	}
	if got := eventCommandGroupID(projected[1]); got != commandExecutionGroupID("fc1") {
		t.Fatalf("expected projected group id %q, got %q", commandExecutionGroupID("fc1"), got)
	}
	groupMeta := decodeRawObject(decodeRawObject(projected[1].Payload["meta"])["commandGroup"])
	if got := int(numberValue(groupMeta["count"])); got != 2 {
		t.Fatalf("expected grouped count 2, got %d", got)
	}

	groups := buildCommandExecutionGroupLookup(events, AgentCodex)
	if len(groups) != 1 {
		t.Fatalf("expected 1 command group detail, got %d", len(groups))
	}
	group, ok := groups[commandExecutionGroupID("fc1")]
	if !ok {
		t.Fatalf("expected group %q in lookup", commandExecutionGroupID("fc1"))
	}
	if group.Count != 2 || len(group.Items) != 2 {
		t.Fatalf("expected 2 file_change detail items, got count=%d items=%d", group.Count, len(group.Items))
	}
	if _, ok := groups[commandExecutionGroupID("fc2")]; ok {
		t.Fatalf("expected usage-separated stale group id %q to be folded into %q", commandExecutionGroupID("fc2"), commandExecutionGroupID("fc1"))
	}
}

func TestFileChangeSnapshotKeepsCurrentFileWhenToolEndOmitsInput(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Single File Change", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_fc_st",
		Seq:       1,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(1_000),
		Payload: map[string]any{
			"tid":  "fc1",
			"name": "FileChange",
			"kind": "file_change",
			"in": map[string]any{
				"changes": []any{
					map[string]any{"path": "/home/dev/CodeKanban/123.md"},
				},
			},
			"meta": map[string]any{"kind": "file_change", "title": "FileChange"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_fc_end",
		Seq:       2,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_000),
		Payload: map[string]any{
			"tid": "fc1",
			"out": "patched",
			"ok":  true,
			"meta": map[string]any{
				"kind":     "file_change",
				"title":    "FileChange",
				"subtitle": "",
			},
		},
	})

	snapshot, err := manager.Snapshot(context.Background(), session.ID, 10)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.History.Items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(snapshot.History.Items))
	}
	if snapshot.History.Items[0].Tool == nil {
		t.Fatalf("expected tool history item, got %#v", snapshot.History.Items[0])
	}

	meta := decodeRawObject(snapshot.History.Items[0].Tool.Meta)
	if got := stringValue(meta["subtitle"]); got != "/home/dev/CodeKanban/123.md" {
		t.Fatalf("expected snapshot subtitle to keep current file path, got %q", got)
	}

	input := decodeRawObject(snapshot.History.Items[0].Tool.Input)
	changes := decodeRawArray(input["changes"])
	if len(changes) != 1 || stringValue(changes[0]["path"]) != "/home/dev/CodeKanban/123.md" {
		t.Fatalf("expected snapshot tool input to keep file path, got %#v", snapshot.History.Items[0].Tool.Input)
	}
}

func TestFileChangeSummaryReturnsCurrentFilePath(t *testing.T) {
	t.Run("changes path", func(t *testing.T) {
		got := fileChangeSummary(map[string]any{
			"changes": []any{
				map[string]any{"path": "/home/dev/CodeKanban/123.md"},
				map[string]any{"path": "/home/dev/CodeKanban/other.md"},
			},
		})
		if got != "/home/dev/CodeKanban/123.md" {
			t.Fatalf("expected first changed path, got %q", got)
		}
	})

	t.Run("camel case path", func(t *testing.T) {
		got := fileChangeSummary(map[string]any{
			"changes": []any{
				map[string]any{"newPath": "/home/dev/CodeKanban/123.md"},
			},
		})
		if got != "/home/dev/CodeKanban/123.md" {
			t.Fatalf("expected camel-case path, got %q", got)
		}
	})
}

func TestHistorySeparatesDifferentCompactToolKinds(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Mixed Compact Tools", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_st",
		Seq:       1,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(1_000),
		Payload: map[string]any{
			"tid":  "cmd1",
			"name": "CommandExecution",
			"kind": "command_execution",
			"in":   map[string]any{"command": "pwd"},
			"meta": map[string]any{"kind": "command_execution", "title": "CommandExecution", "subtitle": "pwd"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_cmd1_end",
		Seq:       2,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(2_000),
		Payload: map[string]any{
			"tid": "cmd1",
			"out": "pwd output",
			"ok":  true,
			"meta": map[string]any{
				"kind": "command_execution", "title": "CommandExecution", "subtitle": "pwd",
			},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_mcp_st",
		Seq:       3,
		Type:      "tool_st",
		Timestamp: time.UnixMilli(3_000),
		Payload: map[string]any{
			"tid":  "mcp1",
			"name": "McpToolCall",
			"kind": "mcp_tool_call",
			"in": map[string]any{
				"tool_name": "fetch",
				"arguments": map[string]any{"url": "http://127.0.0.1:3007"},
			},
			"meta": map[string]any{"kind": "mcp_tool_call", "title": "McpToolCall", "subtitle": "fetch"},
		},
	})
	appendHistoryEvent(t, manager, session.ID, Event{
		ID:        "evt_mcp_end",
		Seq:       4,
		Type:      "tool_end",
		Timestamp: time.UnixMilli(4_000),
		Payload: map[string]any{
			"tid": "mcp1",
			"out": "ok",
			"ok":  true,
			"meta": map[string]any{
				"kind": "mcp_tool_call", "title": "McpToolCall", "subtitle": "fetch",
			},
		},
	})

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("expected 2 projected events, got %d", len(history.Events))
	}
	if got := eventToolKind(history.Events[0]); got != "command_execution" {
		t.Fatalf("expected first kind command_execution, got %q", got)
	}
	if got := eventToolKind(history.Events[1]); got != "mcp_tool_call" {
		t.Fatalf("expected second kind mcp_tool_call, got %q", got)
	}
}

func TestCodexToolResultUsesCamelCaseAggregatedOutput(t *testing.T) {
	got := codexToolResult(map[string]any{
		"type":             "commandExecution",
		"aggregatedOutput": "const styles = {}",
	})
	if got != "const styles = {}" {
		t.Fatalf("expected camelCase aggregatedOutput to be used, got %q", got)
	}
}

func TestTruncateToolOutputKeepsPlanText(t *testing.T) {
	planText := testLongPlanText()
	if got := truncateToolOutput("plan", planText); got != planText {
		t.Fatalf("expected full plan text to be preserved, got length %d want %d", len(got), len(planText))
	}
}

func TestTruncateToolOutputTruncatesNonPlanSafely(t *testing.T) {
	output := strings.Repeat("计划步骤保持完整", 600)

	got := truncateToolOutput("commandExecution", output)
	if got == output {
		t.Fatal("expected non-plan output to be truncated")
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated output suffix, got %q", got[len(got)-min(len(got), 12):])
	}
	if !utf8.ValidString(got) {
		t.Fatalf("expected truncated output to remain valid UTF-8, got %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("expected truncated output to avoid replacement rune, got %q", got)
	}
}

func TestHandleCodexAppServerItemCompletedPreservesFullPlanOutput(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Long Plan App Server", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	run := &activeRun{
		sessionID:          session.ID,
		runID:              "run_plan_app_server",
		assistantMessageID: "msg_plan_app_server",
		assistantDeltaSeen: make(map[string]bool),
	}
	planText := testLongPlanText()
	params := []byte(fmt.Sprintf(`{"item":{"type":"plan","id":"plan_test","text":%q}}`, planText))

	manager.handleCodexAppServerItemCompleted(*session, run, params, true)

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	event, ok := historyToolEventByKind(history.Events, "plan")
	if !ok {
		t.Fatalf("expected plan tool history, got %#v", history.Events)
	}
	if got := eventToolOutput(event); got != planText {
		t.Fatalf("expected app-server plan output to stay intact, got length %d want %d", len(got), len(planText))
	}
}

func TestHandleCodexEventPreservesFullPlanOutput(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Long Plan Legacy", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	run := &activeRun{
		sessionID:          session.ID,
		runID:              "run_plan_legacy",
		assistantMessageID: "msg_plan_legacy",
		assistantDeltaSeen: make(map[string]bool),
	}
	planText := testLongPlanText()

	manager.handleCodexEvent(*session, run, map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type": "plan",
			"id":   "plan_test",
			"text": planText,
		},
	})

	history, err := manager.History(context.Background(), session.ID, 20, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	event, ok := historyToolEventByKind(history.Events, "plan")
	if !ok {
		t.Fatalf("expected plan tool history, got %#v", history.Events)
	}
	if got := eventToolOutput(event); got != planText {
		t.Fatalf("expected legacy plan output to stay intact, got length %d want %d", len(got), len(planText))
	}
}

func TestHandleCodexAppServerUsageDefaultsContextEstimateToCumulativeTotal(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Cumulative Context Estimate", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	run := &activeRun{
		sessionID:          session.ID,
		runID:              "run_usage_only",
		assistantDeltaSeen: make(map[string]bool),
	}
	manager.handleCodexAppServerUsage(
		*session,
		run,
		[]byte(`{"tokenUsage":{"total":{"inputTokens":120,"cachedInputTokens":30,"outputTokens":10},"modelContextWindow":353400}}`),
	)

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	summary := manager.mapSessionSummary(record)
	if summary.ContextEstimateMode != ContextEstimateModeCumulativeTotal {
		t.Fatalf("expected context estimate mode %q, got %q", ContextEstimateModeCumulativeTotal, summary.ContextEstimateMode)
	}
	if summary.ContextEstimate.UsedTokens != 130 {
		t.Fatalf("expected usedTokens 130, got %d", summary.ContextEstimate.UsedTokens)
	}
	if summary.ContextEstimate.InputTokens != 120 || summary.ContextEstimate.CachedInputTokens != 30 || summary.ContextEstimate.OutputTokens != 10 {
		t.Fatalf("unexpected context estimate: %#v", summary.ContextEstimate)
	}
	if summary.ContextWindowTokens == nil || *summary.ContextWindowTokens != 353400 {
		t.Fatalf("expected model context window 353400, got %#v", summary.ContextWindowTokens)
	}
	if summary.ContextWindowSource != ContextWindowSourceSessionUsage {
		t.Fatalf("expected context window source %q, got %q", ContextWindowSourceSessionUsage, summary.ContextWindowSource)
	}
}

func TestBuildContextEstimateUsesLatestTokenCountSnapshot(t *testing.T) {
	now := time.Now()
	record := tables.WebSessionTable{
		TotalInputTokens:                 999,
		TotalCachedInputTokens:           111,
		TotalOutputTokens:                88,
		LatestTokenCountTotalTokens:      12656,
		LatestTokenCountUpdatedAt:        &now,
		LastContextCompactionAt:          &now,
		ContextBaselineInputTokens:       900,
		ContextBaselineCachedInputTokens: 100,
		ContextBaselineOutputTokens:      80,
	}

	estimate, mode := buildContextEstimate(record)
	if mode != ContextEstimateModeLatestTokenCount {
		t.Fatalf("expected context estimate mode %q, got %q", ContextEstimateModeLatestTokenCount, mode)
	}
	if estimate.UsedTokens != 12656 {
		t.Fatalf("expected usedTokens from latest token_count total, got %d", estimate.UsedTokens)
	}
	if estimate.InputTokens != 0 || estimate.CachedInputTokens != 0 || estimate.OutputTokens != 0 {
		t.Fatalf("expected zero token_count breakdown, got %#v", estimate)
	}
}

func TestFinalizeLatestTurnUsageUsesTurnDeltaEstimate(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Latest Turn Delta", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	run := &activeRun{
		sessionID:          session.ID,
		runID:              "run_latest_turn_delta",
		assistantDeltaSeen: make(map[string]bool),
	}

	manager.handleCodexAppServerUsage(
		*session,
		run,
		[]byte(`{"tokenUsage":{"total":{"inputTokens":120,"cachedInputTokens":30,"outputTokens":10}}}`),
	)
	if err := manager.finalizeLatestTurnUsage(context.Background(), session.ID); err != nil {
		t.Fatalf("finalizeLatestTurnUsage returned error: %v", err)
	}

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	summary := manager.mapSessionSummary(record)
	if summary.ContextEstimateMode != ContextEstimateModeLatestTurnDelta {
		t.Fatalf("expected context estimate mode %q, got %q", ContextEstimateModeLatestTurnDelta, summary.ContextEstimateMode)
	}
	if summary.LatestTurnUsage.InputTokens != 120 || summary.LatestTurnUsage.CachedInputTokens != 30 || summary.LatestTurnUsage.OutputTokens != 10 {
		t.Fatalf("unexpected latest turn usage after first turn: %#v", summary.LatestTurnUsage)
	}
	if summary.LatestTurnUsage.UsedTokens != 130 {
		t.Fatalf("expected latest turn usedTokens 130, got %d", summary.LatestTurnUsage.UsedTokens)
	}
	if summary.ContextEstimate != summary.LatestTurnUsage {
		t.Fatalf("expected context estimate to mirror latest turn usage, got %#v vs %#v", summary.ContextEstimate, summary.LatestTurnUsage)
	}

	if err := manager.updateRuntimeState(context.Background(), session.ID, map[string]any{
		"status":     string(StatusRunning),
		"updated_at": time.Now(),
	}); err != nil {
		t.Fatalf("updateRuntimeState returned error: %v", err)
	}
	manager.handleCodexAppServerUsage(
		*session,
		run,
		[]byte(`{"tokenUsage":{"total":{"inputTokens":150,"cachedInputTokens":35,"outputTokens":12}}}`),
	)

	record, err = manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	summary = manager.mapSessionSummary(record)
	if summary.ContextEstimateMode != ContextEstimateModeLatestTurnDelta {
		t.Fatalf("expected running context estimate mode %q, got %q", ContextEstimateModeLatestTurnDelta, summary.ContextEstimateMode)
	}
	if summary.ContextEstimate.InputTokens != 30 || summary.ContextEstimate.CachedInputTokens != 5 || summary.ContextEstimate.OutputTokens != 2 {
		t.Fatalf("unexpected running turn delta: %#v", summary.ContextEstimate)
	}
	if summary.ContextEstimate.UsedTokens != 32 {
		t.Fatalf("expected running usedTokens 32, got %d", summary.ContextEstimate.UsedTokens)
	}

	if err := manager.finalizeLatestTurnUsage(context.Background(), session.ID); err != nil {
		t.Fatalf("finalizeLatestTurnUsage returned error: %v", err)
	}
	if err := manager.updateRuntimeState(context.Background(), session.ID, map[string]any{
		"status":     string(StatusDone),
		"updated_at": time.Now(),
	}); err != nil {
		t.Fatalf("updateRuntimeState returned error: %v", err)
	}

	record, err = manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	summary = manager.mapSessionSummary(record)
	if summary.LatestTurnUsage.InputTokens != 30 || summary.LatestTurnUsage.CachedInputTokens != 5 || summary.LatestTurnUsage.OutputTokens != 2 {
		t.Fatalf("unexpected finalized latest turn usage: %#v", summary.LatestTurnUsage)
	}
	if summary.LatestTurnUsage.UsedTokens != 32 {
		t.Fatalf("expected finalized latest turn usedTokens 32, got %d", summary.LatestTurnUsage.UsedTokens)
	}
}

func TestHandleCodexAppServerContextCompactionResetsContextEstimateBaseline(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Context Compaction", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	run := &activeRun{
		sessionID:          session.ID,
		runID:              "run_context_compaction",
		assistantMessageID: "msg_context_compaction",
		assistantDeltaSeen: make(map[string]bool),
	}

	manager.handleCodexAppServerUsage(
		*session,
		run,
		[]byte(`{"tokenUsage":{"total":{"inputTokens":120,"cachedInputTokens":30,"outputTokens":10}}}`),
	)
	manager.handleCodexAppServerItemCompleted(
		*session,
		run,
		[]byte(`{"item":{"type":"contextCompaction","id":"compact_test","status":"completed","summary":["Compacted previous messages into a summary."]}}`),
		true,
	)
	manager.handleCodexAppServerUsage(
		*session,
		run,
		[]byte(`{"tokenUsage":{"total":{"inputTokens":150,"cachedInputTokens":35,"outputTokens":12}}}`),
	)

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	summary := manager.mapSessionSummary(record)
	if summary.ContextEstimateMode != ContextEstimateModeSinceCompaction {
		t.Fatalf("expected context estimate mode %q, got %q", ContextEstimateModeSinceCompaction, summary.ContextEstimateMode)
	}
	if summary.LastContextCompactionAt == nil {
		t.Fatal("expected lastContextCompactionAt to be recorded")
	}
	if summary.ContextEstimate.InputTokens != 30 || summary.ContextEstimate.CachedInputTokens != 5 || summary.ContextEstimate.OutputTokens != 2 {
		t.Fatalf("unexpected context estimate after compaction: %#v", summary.ContextEstimate)
	}
	if summary.ContextEstimate.UsedTokens != 32 {
		t.Fatalf("expected usedTokens 32 after compaction, got %d", summary.ContextEstimate.UsedTokens)
	}

	snapshot, err := manager.Snapshot(context.Background(), session.ID, 20)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.History.Items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(snapshot.History.Items))
	}
	item := snapshot.History.Items[0]
	if item.Tool == nil || item.Tool.Kind != "context_compaction" {
		t.Fatalf("expected context_compaction tool item, got %#v", item)
	}
	if !strings.Contains(item.Tool.Output, "Compacted previous messages") {
		t.Fatalf("expected compaction output to be preserved, got %q", item.Tool.Output)
	}
}

func initTestDB(t *testing.T) func() {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	if err := model.InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	return func() {
		model.DBClose()
	}
}

func seedProject(t *testing.T) *tables.ProjectTable {
	t.Helper()
	project := &tables.ProjectTable{
		Name: "Web Session Test",
		Path: t.TempDir(),
	}
	project.Init()
	if err := model.GetDB().Create(project).Error; err != nil {
		t.Fatalf("seed project failed: %v", err)
	}
	return project
}

func seedCodexAISession(
	t *testing.T,
	projectPath string,
	sessionID string,
	filePath string,
	title string,
	startedAt time.Time,
	lastMessageAt *time.Time,
) *tables.AISessionTable {
	t.Helper()

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat ai session file failed: %v", err)
	}

	session := &tables.AISessionTable{
		SessionID:             sessionID,
		Type:                  tables.AISessionTypeCodex,
		ProjectPath:           projectPath,
		FilePath:              filePath,
		Model:                 "gpt-5.4",
		Title:                 title,
		SessionStartedAt:      startedAt,
		LastMessageAt:         lastMessageAt,
		MessageCount:          1,
		AssistantMessageCount: 1,
		FileModTime:           info.ModTime(),
		FileSize:              info.Size(),
	}
	session.Init()
	if err := model.GetDB().Create(session).Error; err != nil {
		t.Fatalf("seed ai session failed: %v", err)
	}
	return session
}

func seedWebSession(t *testing.T, projectID, title string, orderIndex float64) *tables.WebSessionTable {
	return seedWebSessionWithAgent(t, projectID, title, orderIndex, AgentCodex)
}

func seedWebSessionWithAgent(
	t *testing.T,
	projectID, title string,
	orderIndex float64,
	agent Agent,
) *tables.WebSessionTable {
	t.Helper()
	session := &tables.WebSessionTable{
		ProjectID:            projectID,
		OrderIndex:           orderIndex,
		Agent:                string(normalizeAgent(agent)),
		Title:                title,
		Model:                defaultModel(normalizeAgent(agent), ""),
		WorkflowMode:         string(WorkflowModeDefault),
		PermissionLevel:      string(PermissionLevelElevated),
		LegacyPermissionMode: "default",
		Cwd:                  t.TempDir(),
		Status:               string(StatusIdle),
		ActivityAt:           time.Now(),
	}
	session.Init()
	if err := model.GetDB().Create(session).Error; err != nil {
		t.Fatalf("seed web session failed: %v", err)
	}
	return session
}

func writeFakeCodexCLI(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-codex.sh")
	script := `#!/bin/sh
printf '%s\n' '{"type":"thread.started","thread_id":"thread_test"}'
printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"done"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1}}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex cli failed: %v", err)
	}
	return path
}

func writeFakeCodexVersionCLI(t *testing.T, version string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-codex-version.sh")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'codex %s\n'
  exit 0
fi
if [ "$1" = "app-server" ]; then
  printf '%%s\n' '{"id":0,"result":{"userAgent":"fake-codex-app-server","codexHome":"/tmp/codex","platformFamily":"unix","platformOs":"linux"}}'
  exit 0
fi
exit 1
`, version)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex version cli failed: %v", err)
	}
	return path
}

func writeFakeCodexModelCatalogCLI(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-codex-model-catalog.js")
	script := `#!/usr/bin/env node
if (process.argv.includes('--version')) {
  process.stdout.write('codex 0.144.1\n');
  process.exit(0);
}
const readline = require('readline');
const input = readline.createInterface({ input: process.stdin });
const option = reasoningEffort => ({ reasoningEffort, description: reasoningEffort });
input.on('line', line => {
  const message = JSON.parse(line);
  if (message.method === 'initialize') {
    process.stdout.write(JSON.stringify({ id: message.id, result: { userAgent: 'fake-codex' } }) + '\n');
    return;
  }
  if (message.method === 'model/list') {
    const model = (name, efforts, defaultReasoningEffort) => ({
      model: name,
      displayName: name,
      defaultReasoningEffort,
      supportedReasoningEfforts: efforts.map(option),
    });
    process.stdout.write(JSON.stringify({
      id: message.id,
      result: {
        data: [
          model('gpt-5.6-sol', ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'], 'low'),
          model('gpt-5.6-terra', ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'], 'medium'),
          model('gpt-5.6-luna', ['low', 'medium', 'high', 'xhigh', 'max'], 'medium'),
        ],
        nextCursor: null,
      },
    }) + '\n');
  }
});
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake Codex model catalog CLI failed: %v", err)
	}
	return path
}

func writeFakeClaudeStreamCLI(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-claude.sh")
	script := `#!/bin/sh
read first_line
printf '%s\n' '{"type":"system","session_id":"claude-session-test"}'
printf '%s\n' '{"type":"assistant","uuid":"assistant_1","message":{"type":"message","role":"assistant","id":"assistant_msg_1","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn"}}'
cat >/dev/null
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude cli failed: %v", err)
	}
	return path
}

func writeFakeClaudeDeferredCLI(t *testing.T) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		dir := t.TempDir()
		stateFile := filepath.Join(dir, "claude-deferred-state.txt")
		psPath := filepath.Join(dir, "fake-claude-deferred.ps1")
		cmdPath := filepath.Join(dir, "fake-claude-deferred.cmd")
		psStateFile := strings.ReplaceAll(stateFile, "'", "''")
		script := `$stateFile = '` + psStateFile + `'
[Console]::In.ReadToEnd() | Out-Null
$count = 0
if (Test-Path -LiteralPath $stateFile) {
  $raw = Get-Content -LiteralPath $stateFile -Raw
  [int]::TryParse(($raw.Trim()), [ref]$count) | Out-Null
}
$count += 1
Set-Content -LiteralPath $stateFile -Value $count -NoNewline
if ($count -eq 1) {
  Write-Output '{"type":"system","subtype":"init","session_id":"claude-session-test"}'
  Write-Output '{"type":"assistant","uuid":"assistant_tool","message":{"type":"message","role":"assistant","id":"assistant_tool_msg","content":[{"type":"tool_use","id":"tool_ask_resume","name":"AskUserQuestion","input":{"questions":[{"header":"Direction","question":"What should happen next?","multiSelect":false,"options":[{"label":"Implement","description":"Start coding now."},{"label":"Plan","description":"Stay in planning mode."}]}]}}],"stop_reason":"tool_use"}}'
  Write-Output '{"type":"result","session_id":"claude-session-test","stop_reason":"tool_deferred","deferred_tool_use":{"id":"tool_ask_resume","name":"AskUserQuestion","input":{"questions":[{"header":"Direction","question":"What should happen next?","multiSelect":false,"options":[{"label":"Implement","description":"Start coding now."},{"label":"Plan","description":"Stay in planning mode."}]}]}}}'
  exit 0
}
Write-Output '{"type":"system","subtype":"init","session_id":"claude-session-test"}'
Write-Output '{"type":"assistant","uuid":"assistant_done","message":{"type":"message","role":"assistant","id":"assistant_done_msg","content":[{"type":"text","text":"continuing after the answer"}],"stop_reason":"end_turn"}}'
Write-Output '{"type":"result","session_id":"claude-session-test","stop_reason":"end_turn"}'
`
		if err := os.WriteFile(psPath, []byte(script), 0o644); err != nil {
			t.Fatalf("write fake claude deferred ps1 failed: %v", err)
		}
		cmd := "@echo off\r\npowershell -NoProfile -ExecutionPolicy Bypass -File \"%~dp0fake-claude-deferred.ps1\"\r\nexit /b %ERRORLEVEL%\r\n"
		if err := os.WriteFile(cmdPath, []byte(cmd), 0o755); err != nil {
			t.Fatalf("write fake claude deferred cmd failed: %v", err)
		}
		return cmdPath
	}

	path := filepath.Join(t.TempDir(), "fake-claude-deferred.sh")
	script := `#!/bin/sh
state_file="` + filepath.Join(t.TempDir(), "claude-deferred-state.txt") + `"
count=0
if [ -f "$state_file" ]; then
  count=$(cat "$state_file")
fi
count=$((count + 1))
printf '%s' "$count" >"$state_file"
if [ "$count" -eq 1 ]; then
  cat >/dev/null
  printf '%s\n' '{"type":"system","subtype":"init","session_id":"claude-session-test"}'
  printf '%s\n' '{"type":"assistant","uuid":"assistant_tool","message":{"type":"message","role":"assistant","id":"assistant_tool_msg","content":[{"type":"tool_use","id":"tool_ask_resume","name":"AskUserQuestion","input":{"questions":[{"header":"Direction","question":"What should happen next?","multiSelect":false,"options":[{"label":"Implement","description":"Start coding now."},{"label":"Plan","description":"Stay in planning mode."}]}]}}],"stop_reason":"tool_use"}}'
  printf '%s\n' '{"type":"result","session_id":"claude-session-test","stop_reason":"tool_deferred","deferred_tool_use":{"id":"tool_ask_resume","name":"AskUserQuestion","input":{"questions":[{"header":"Direction","question":"What should happen next?","multiSelect":false,"options":[{"label":"Implement","description":"Start coding now."},{"label":"Plan","description":"Stay in planning mode."}]}]}}}'
  exit 0
fi
cat >/dev/null
printf '%s\n' '{"type":"system","subtype":"init","session_id":"claude-session-test"}'
printf '%s\n' '{"type":"assistant","uuid":"assistant_done","message":{"type":"message","role":"assistant","id":"assistant_done_msg","content":[{"type":"text","text":"continuing after the answer"}],"stop_reason":"end_turn"}}'
printf '%s\n' '{"type":"result","session_id":"claude-session-test","stop_reason":"end_turn"}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude deferred cli failed: %v", err)
	}
	return path
}

func writeFakeCodexAppServerCLI(t *testing.T, mode string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-codex-app-server.js")
	script := fmt.Sprintf(`#!/usr/bin/env node
if (process.argv.includes('--version')) {
  process.stdout.write('codex 0.137.0\n');
  process.exit(0);
}
const readline = require('readline');
const fs = require('fs');

const mode = %q;
const threadId = 'thread_test';
const turnId = 'turn_test';
const childThreadId = 'thread_child';
const childTurnId = 'turn_child';
const stateFile = __filename + '.state';
const goalStateFile = stateFile + '.goal.json';
let activeThreadId = threadId;

function readGoalState() {
  try {
    return JSON.parse(fs.readFileSync(goalStateFile, 'utf8'));
  } catch (error) {
    return null;
  }
}

function writeGoalState(value) {
  if (!value) {
    try {
      fs.unlinkSync(goalStateFile);
    } catch (error) {
      // Ignore missing state files in tests.
    }
    return;
  }
  fs.writeFileSync(goalStateFile, JSON.stringify(value));
}

let goalState = readGoalState();

function send(message) {
  process.stdout.write(JSON.stringify(message) + '\n');
}

function respondThread(id, explicitThreadId) {
  send({
    id,
    result: {
      modelProvider: 'TestProvider',
      thread: { id: explicitThreadId || threadId },
    },
  });
}

function emitReasoning() {
  send({
    method: 'item/started',
    params: {
      item: { type: 'reasoning', id: 'rs_test', summary: [], content: [] },
      threadId: activeThreadId,
      turnId,
    },
  });
  send({
    method: 'item/completed',
    params: {
      item: { type: 'reasoning', id: 'rs_test', summary: [], content: [] },
      threadId: activeThreadId,
      turnId,
    },
  });
}

function emitPlan() {
  send({
    method: 'item/started',
    params: {
      item: { type: 'plan', id: 'plan_test', text: '## Plan\n- Review the repo\n- Make the change' },
      threadId: activeThreadId,
      turnId,
    },
  });
  send({
    method: 'item/completed',
    params: {
      item: { type: 'plan', id: 'plan_test', text: '## Plan\n- Review the repo\n- Make the change' },
      threadId: activeThreadId,
      turnId,
    },
  });
}

function emitCommandExecutionStart() {
  send({
    method: 'item/started',
    params: {
      item: {
        type: 'commandExecution',
        id: 'cmd_timeout',
        command: 'pnpm dev --host 127.0.0.1 --port 4173',
      },
      threadId,
      turnId,
    },
  });
}

function emitMcpToolCallStart() {
  send({
    method: 'item/started',
    params: {
      item: {
        type: 'mcpToolCall',
        id: 'mcp_timeout',
        tool_name: 'settings',
      },
      threadId,
      turnId,
    },
  });
}

function emitSubAgentToolCallStart() {
  send({
    method: 'item/started',
    params: {
      item: {
        type: 'collabAgentToolCall',
        id: 'sub_agent_timeout',
        title: 'Research agent',
        prompt: 'Inspect current sub-agent support',
      },
      threadId,
      turnId,
    },
  });
}

function emitFileChangeStart() {
  send({
    method: 'item/started',
    params: {
      item: {
        type: 'fileChange',
        id: 'file_change_timeout',
        path: 'README.md',
      },
      threadId,
      turnId,
    },
  });
}

function emitCommandExecutionCompleted(id, command) {
  send({
    method: 'item/completed',
    params: {
      item: {
        type: 'commandExecution',
        id,
        command,
        status: 'completed',
        output: command + ' done',
      },
      threadId,
      turnId,
    },
  });
}

function startTimedOutTurn(kind) {
  if (kind === 'command') {
    emitCommandExecutionStart();
    return;
  }
  if (kind === 'mcp') {
    emitMcpToolCallStart();
    return;
  }
  if (kind === 'approval') {
    emitFileChangeStart();
    awaiting = 'req_approval_timeout';
    send({
      id: awaiting,
      method: 'item/fileChange/requestApproval',
      params: {
        threadId,
        turnId,
        itemId: 'file_change_timeout',
        reason: 'Need approval before continuing.',
      },
    });
    return;
  }
  if (kind === 'user_input') {
    emitMcpToolCallStart();
    awaiting = 'req_user_timeout';
    send({
      id: awaiting,
      method: 'item/tool/requestUserInput',
      params: {
        threadId,
        turnId,
        itemId: 'mcp_timeout',
        questions: [
          {
            id: 'scope',
            header: 'Scope',
            question: 'Which scope should I use?',
            isOther: false,
            isSecret: false,
            options: [
              { label: 'Continue', description: 'Continue the turn.' },
              { label: 'Pause', description: 'Pause the turn.' },
            ],
          },
        ],
      },
    });
  }
}

function finishTurn(text) {
  emitReasoning();
  if (mode === 'plan') {
    emitPlan();
  }
  send({
    method: 'item/started',
    params: {
      item: { type: 'agentMessage', id: 'msg_test', text: '', phase: 'final_answer', memoryCitation: null },
      threadId: activeThreadId,
      turnId,
    },
  });
  send({
    method: 'item/agentMessage/delta',
    params: { threadId: activeThreadId, turnId, itemId: 'msg_test', delta: text },
  });
  send({
    method: 'item/completed',
    params: {
      item: { type: 'agentMessage', id: 'msg_test', text, phase: 'final_answer', memoryCitation: null },
      threadId: activeThreadId,
      turnId,
    },
  });
  send({
    method: 'thread/tokenUsage/updated',
    params: {
      threadId: activeThreadId,
      turnId,
      tokenUsage: {
        total: { inputTokens: 5, cachedInputTokens: 0, outputTokens: 3 },
      },
    },
  });
  send({
    method: 'turn/completed',
    params: {
      threadId: activeThreadId,
      turn: { id: turnId, items: [], status: 'completed', error: null },
    },
  });
}

function startSubAgentThreadIsolationTurn() {
  send({
    method: 'item/started',
    params: {
      item: {
        type: 'collabAgentToolCall',
        id: 'root_wait',
        agentsStates: {},
        receiverThreadIds: [childThreadId],
        senderThreadId: threadId,
        status: 'inProgress',
        tool: 'wait',
      },
      threadId,
      turnId,
    },
  });
  send({
    method: 'thread/started',
    params: { thread: { id: childThreadId } },
  });
  send({
    method: 'turn/started',
    params: {
      threadId: childThreadId,
      turn: { id: childTurnId, items: [], status: 'inProgress', error: null },
    },
  });
  send({
    method: 'item/started',
    params: {
      item: { type: 'agentMessage', id: 'child_message', text: '', phase: 'commentary' },
      threadId: childThreadId,
      turnId: childTurnId,
    },
  });
  send({
    method: 'item/agentMessage/delta',
    params: {
      threadId: childThreadId,
      turnId: childTurnId,
      itemId: 'child_message',
      delta: 'child still working',
    },
  });
  send({
    method: 'item/completed',
    params: {
      item: { type: 'agentMessage', id: 'child_message', text: 'child still working', phase: 'commentary' },
      threadId: childThreadId,
      turnId: childTurnId,
    },
  });
  send({
    method: 'item/started',
    params: {
      item: { type: 'plan', id: 'child_plan', text: 'child plan' },
      threadId: childThreadId,
      turnId: childTurnId,
    },
  });
  send({
    method: 'item/completed',
    params: {
      item: { type: 'plan', id: 'child_plan', text: 'child plan' },
      threadId: childThreadId,
      turnId: childTurnId,
    },
  });
  send({
    method: 'item/started',
    params: {
      item: { type: 'commandExecution', id: 'child_command', command: 'sleep 30' },
      threadId: childThreadId,
      turnId: childTurnId,
    },
  });
  send({
    method: 'thread/tokenUsage/updated',
    params: {
      threadId: childThreadId,
      turnId: childTurnId,
      tokenUsage: {
        total: { inputTokens: 999, cachedInputTokens: 111, outputTokens: 222 },
      },
    },
  });
  send({
    method: 'error',
    params: {
      threadId: childThreadId,
      turnId: childTurnId,
      error: { message: 'child failure should not fail root' },
      willRetry: false,
    },
  });
  send({
    method: 'turn/completed',
    params: {
      threadId: childThreadId,
      turn: {
        id: childTurnId,
        items: [],
        status: 'failed',
        error: { message: 'child failure should not fail root' },
      },
    },
  });
  fs.writeFileSync(stateFile + '.child-completed', '1');

  const releaseTimer = setInterval(() => {
    if (!fs.existsSync(stateFile + '.release-root')) {
      return;
    }
    clearInterval(releaseTimer);
    send({
      method: 'item/completed',
      params: {
        item: {
          type: 'commandExecution',
          id: 'child_command',
          command: 'sleep 30',
          status: 'failed',
          aggregatedOutput: 'child command stopped',
        },
        threadId: childThreadId,
        turnId: childTurnId,
      },
    });
    send({
      method: 'item/completed',
      params: {
        item: {
          type: 'collabAgentToolCall',
          id: 'root_wait',
          agentsStates: {
            [childThreadId]: { status: 'errored', message: 'child result preserved' },
          },
          receiverThreadIds: [childThreadId],
          senderThreadId: threadId,
          status: 'completed',
          tool: 'wait',
        },
        threadId,
        turnId,
      },
    });
    finishTurn('root-finished-after-child');
  }, 10);
}

function failTurn(message) {
  send({
    method: 'turn/completed',
    params: {
      threadId: activeThreadId,
      turn: {
        id: turnId,
        items: [],
        status: 'failed',
        error: { message },
      },
    },
  });
}

function delayFailTurn(message, delayMs) {
  setTimeout(() => failTurn(message), delayMs);
}

function readPersistentTurnCount() {
  try {
    return Number(fs.readFileSync(stateFile, 'utf8').trim()) || 0;
  } catch (error) {
    return 0;
  }
}

function writePersistentTurnCount(value) {
  fs.writeFileSync(stateFile, String(value));
}

let awaiting = null;
const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
let startedTurns = 0;
rl.on('line', line => {
  if (!line.trim()) {
    return;
  }

  const message = JSON.parse(line);
  if (message.method === 'initialize') {
    send({
      id: message.id,
      result: {
        userAgent: 'fake-codex-app-server',
        codexHome: '/tmp/codex',
        platformFamily: 'unix',
        platformOs: 'linux',
      },
    });
    return;
  }

  if (message.method === 'config/read') {
    if (mode === 'config_read_unsupported_reconnect') {
      send({
        id: message.id,
        error: { code: -32601, message: 'config/read is unavailable' },
      });
      return;
    }
    send({
      id: message.id,
      result: {
        config: {
          model_provider: 'TestProvider',
          model_providers: {
            TestProvider: {
              base_url:
                'https://user:password@proxy.example.test/v1?token=secret#fragment',
            },
          },
        },
        origins: {},
      },
    });
    return;
  }

  if (message.method === 'thread/list') {
    const archived = !!(message.params && message.params.archived);
    if (mode === 'list_threads') {
      send({
        id: message.id,
        result: {
          data: archived
            ? [
                {
                  id: 'thread_archived',
                  preview: 'Archived preview',
                  path: '/tmp/thread-archived.jsonl',
                  cwd: message.params && message.params.cwd,
                  status: 'archived',
                  createdAt: 1712793600,
                  updatedAt: 1712797200,
                },
              ]
            : [
                {
                  id: 'thread_list',
                  preview: 'Thread preview',
                  path: '/tmp/thread-list.jsonl',
                  cwd: message.params && message.params.cwd,
                  status: 'idle',
                  createdAt: 1712793600,
                  updatedAt: 1712797200,
                },
              ],
          nextCursor: '',
        },
      });
      return;
    }
    send({ id: message.id, result: { data: [], nextCursor: '' } });
    return;
  }

  if (message.method === 'thread/start' || message.method === 'thread/resume') {
    if (mode === 'resume_only' && message.method !== 'thread/resume') {
      send({
        id: message.id,
        error: { message: 'expected thread/resume for existing session' },
      });
      return;
    }
    const resumedThreadId = message.params && typeof message.params.threadId === 'string'
      ? message.params.threadId
      : threadId;
    activeThreadId = resumedThreadId;
    respondThread(message.id, resumedThreadId);
    return;
  }

  if (message.method === 'thread/goal/set') {
    const params = message.params || {};
    const nextObjective =
      typeof params.objective === 'string' && params.objective.trim()
        ? params.objective.trim()
        : goalState && typeof goalState.objective === 'string'
          ? goalState.objective
          : '';
    const nextStatus =
      typeof params.status === 'string' && params.status.trim()
        ? params.status.trim()
        : goalState && typeof goalState.status === 'string'
          ? goalState.status
          : 'active';
    if (!nextObjective) {
      send({
        id: message.id,
        error: { message: 'objective is required' },
      });
      return;
    }
    const timestamp = 1712797200;
    goalState = {
      threadId,
      objective: nextObjective,
      status: nextStatus,
      tokenBudget: null,
      tokensUsed: 0,
      timeUsedSeconds: 0,
      createdAt: goalState && goalState.createdAt ? goalState.createdAt : timestamp,
      updatedAt: timestamp,
    };
    writeGoalState(goalState);
    send({
      id: message.id,
      result: {
        goal: goalState,
      },
    });
    send({
      method: 'thread/goal/updated',
      params: {
        threadId,
        turnId: null,
        goal: goalState,
      },
    });
    return;
  }

  if (message.method === 'thread/goal/get') {
    goalState = readGoalState();
    send({
      id: message.id,
      result: {
        goal: goalState,
      },
    });
    return;
  }

  if (message.method === 'turn/start') {
    startedTurns += 1;
    send({
      id: message.id,
      result: {
        turn: { id: turnId, items: [], status: 'inProgress', error: null },
      },
    });

    if (mode === 'basic' || mode === 'resume_only' || mode === 'plan') {
      finishTurn('done');
      return;
    }

    if (mode === 'turn_complete_linger') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      finishTurn('done-' + persistedTurns);
      setTimeout(() => process.exit(0), 200);
      return;
    }

    if (mode === 'sub_agent_thread_isolation') {
      startSubAgentThreadIsolationTurn();
      return;
    }

    if (mode === 'reconnect_then_success' || mode === 'config_read_unsupported_reconnect') {
      send({
        method: 'error',
        params: {
          message: 'Reconnecting... 1/5 (unexpected status 502 Bad Gateway: Upstream service temporarily unavailable)',
        },
      });
      finishTurn('done');
      return;
    }

    if (mode === 'reconnect_then_fail') {
      send({
        method: 'error',
        params: {
          message: 'Reconnecting... 1/5 (unexpected status 502 Bad Gateway: Upstream service temporarily unavailable)',
        },
      });
      send({
        method: 'error',
        params: {
          message: 'Reconnecting... 2/5 (unexpected status 502 Bad Gateway: Upstream service temporarily unavailable)',
        },
      });
      failTurn('unexpected status 502 Bad Gateway: Upstream service temporarily unavailable');
      return;
    }

    if (mode === 'auto_retry_then_success') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        failTurn('unexpected status 502 Bad Gateway: Upstream service temporarily unavailable');
        return;
      }
      finishTurn('done');
      return;
    }

    if (mode === 'delayed_failure_then_success') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        delayFailTurn(
          'unexpected status 502 Bad Gateway: Upstream service temporarily unavailable',
          200
        );
        return;
      }
      finishTurn('done');
      return;
    }

    if (mode === 'active_call_timeout_command_then_success') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        startTimedOutTurn('command');
        return;
      }
      finishTurn('continued');
      return;
    }

    if (mode === 'active_call_timeout_mcp_then_success') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        startTimedOutTurn('mcp');
        return;
      }
      finishTurn('continued');
      return;
    }

    if (mode === 'active_call_timeout_latest_then_success') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        emitCommandExecutionStart();
        setTimeout(() => emitMcpToolCallStart(), 25);
        return;
      }
      finishTurn('continued');
      return;
    }

    if (mode === 'active_call_timeout_approval_then_success') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        startTimedOutTurn('approval');
        return;
      }
      finishTurn('continued');
      return;
    }

    if (mode === 'active_call_timeout_sub_agent') {
      emitSubAgentToolCallStart();
      return;
    }

    if (mode === 'active_call_timeout_user_input_then_success') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        startTimedOutTurn('user_input');
        return;
      }
      finishTurn('continued');
      return;
    }

    if (mode === 'user_input') {
      awaiting = 'req_user_1';
      send({
        id: awaiting,
        method: 'item/tool/requestUserInput',
        params: {
          threadId,
          turnId,
          itemId: 'ask_scope',
          questions: [
            {
              id: 'scope',
              header: 'Scope',
              question: 'Which migration scope should be implemented?',
              isOther: false,
              isSecret: false,
              options: [
                { label: 'full migration', description: 'Move all Codex web sessions to app-server.' },
                { label: 'plan only', description: 'Only switch plan mode to the real runtime mode.' },
              ],
            },
          ],
        },
      });
      return;
    }

    if (mode === 'approval') {
      awaiting = 'req_approval_1';
      send({
        id: awaiting,
        method: 'item/fileChange/requestApproval',
        params: {
          threadId,
          turnId,
          itemId: 'write_patch',
          reason: 'Need approval to apply the patch.',
        },
      });
      return;
    }

    if (mode === 'step_redirect') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        send({
          method: 'item/started',
          params: {
            item: {
              type: 'commandExecution',
              id: 'cmd_step_1',
              command: 'step-1',
            },
            threadId,
            turnId,
          },
        });
        emitCommandExecutionCompleted('cmd_step_1', 'step-1');
        send({
          method: 'item/started',
          params: {
            item: {
              type: 'commandExecution',
              id: 'cmd_step_2',
              command: 'step-2',
            },
            threadId,
            turnId,
          },
        });
        setTimeout(() => {
          emitCommandExecutionCompleted('cmd_step_2', 'step-2');
          setTimeout(() => {
            send({
              method: 'item/started',
              params: {
                item: {
                  type: 'commandExecution',
                  id: 'cmd_step_3',
                  command: 'step-3',
                },
                threadId,
                turnId,
              },
            });
          }, 80);
        }, 80);
        return;
      }
      finishTurn('continued');
      return;
    }
  }

  if (awaiting && message.id === awaiting) {
    if (mode === 'active_call_timeout_approval_then_success' || mode === 'active_call_timeout_user_input_then_success') {
      awaiting = null;
      return;
    }
    finishTurn(mode === 'user_input' ? 'answered' : 'approved');
    awaiting = null;
  }
});

rl.on('close', () => process.exit(0));
process.on('exit', () => {
  if (mode === 'turn_complete_linger') {
    fs.appendFileSync(stateFile + '.exits', '1\n');
  }
});
`, mode)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex app-server cli failed: %v", err)
	}
	return path
}

func waitForSessionToSettle(t *testing.T, manager *Manager, sessionID string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !manager.hasActiveRun(sessionID) {
			record, err := manager.GetSession(context.Background(), sessionID)
			if err == nil && (record.Status == string(StatusDone) ||
				record.Status == string(StatusError) ||
				record.Status == string(StatusIdle) ||
				(record.Status == string(StatusRunning) &&
					record.AssistantState == string(AssistantStateWaitingPlanApproval))) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	record, err := manager.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession returned error while waiting: %v", err)
	}
	t.Fatalf("session %s did not settle, status=%s", sessionID, record.Status)
}

func waitForSessionStatus(t *testing.T, manager *Manager, sessionID string, status Status) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		record, err := manager.GetSession(context.Background(), sessionID)
		if err == nil && record.Status == string(status) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	record, err := manager.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession returned error while waiting for status %s: %v", status, err)
	}
	t.Fatalf("session %s did not reach status %s, got %s", sessionID, status, record.Status)
}

func waitForFakeCodexAppServerExitCount(t *testing.T, codexPath string, count int) {
	t.Helper()

	exitPath := codexPath + ".state.exits"
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(exitPath)
		if err == nil && strings.Count(string(content), "\n") >= count {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	content, _ := os.ReadFile(exitPath)
	t.Fatalf("expected fake codex app-server exit count %d, got %d", count, strings.Count(string(content), "\n"))
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected file %s to be created", path)
}

func waitForHistoryToolEvent(
	t *testing.T,
	manager *Manager,
	sessionID string,
	toolID string,
	eventType string,
) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events, err := manager.store.readEvents(sessionID)
		if err == nil {
			for _, event := range events {
				if event.Type == eventType && stringValue(event.Payload["tid"]) == toolID {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %s event for tool %s", eventType, toolID)
}

func waitForPendingServerRequest(
	t *testing.T,
	manager *Manager,
	sessionID string,
	kind pendingServerRequestKind,
) *pendingServerRequest {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.RLock()
		run := manager.runs[sessionID]
		manager.mu.RUnlock()
		if run != nil {
			if request, ok := run.pendingApprovalRequest(); ok && request.Kind == kind {
				if waitForAssistantState(t, manager, sessionID, AssistantStateWaitingApproval, deadline) {
					return request
				}
			}
			if request, ok := run.pendingUserInputRequest(); ok && request.Kind == kind {
				if waitForAssistantState(t, manager, sessionID, AssistantStateWaitingInput, deadline) {
					return request
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

func waitForAssistantState(
	t *testing.T,
	manager *Manager,
	sessionID string,
	state AssistantState,
	deadline time.Time,
) bool {
	t.Helper()

	for time.Now().Before(deadline) {
		record, err := manager.GetSession(context.Background(), sessionID)
		if err == nil && record.AssistantState == string(state) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func waitForTrackedActiveCall(
	t *testing.T,
	manager *Manager,
	sessionID string,
	kind activeCallTimeoutKind,
) trackedActiveCall {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.RLock()
		run := manager.runs[sessionID]
		manager.mu.RUnlock()
		if run != nil {
			run.mu.Lock()
			for _, call := range run.activeCalls {
				if call.Kind == kind {
					run.mu.Unlock()
					return call
				}
			}
			run.mu.Unlock()
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for tracked active call kind %q", kind)
	return trackedActiveCall{}
}

func waitForTrackedActiveCallID(
	t *testing.T,
	manager *Manager,
	sessionID string,
	toolID string,
) trackedActiveCall {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.RLock()
		run := manager.runs[sessionID]
		manager.mu.RUnlock()
		if run != nil {
			run.mu.Lock()
			call, ok := run.activeCalls[toolID]
			run.mu.Unlock()
			if ok {
				return call
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for tracked active call id %q", toolID)
	return trackedActiveCall{}
}

func waitForTrackedActiveCallCount(t *testing.T, manager *Manager, sessionID string, count int) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.RLock()
		run := manager.runs[sessionID]
		manager.mu.RUnlock()
		if run != nil {
			run.mu.Lock()
			size := len(run.activeCalls)
			run.mu.Unlock()
			if size >= count {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d tracked active calls", count)
}

func setTrackedActiveCallStartedAt(
	t *testing.T,
	manager *Manager,
	sessionID string,
	toolID string,
	startedAt time.Time,
) {
	t.Helper()

	manager.mu.RLock()
	run := manager.runs[sessionID]
	manager.mu.RUnlock()
	if run == nil {
		t.Fatalf("expected active run for session %s", sessionID)
	}

	run.mu.Lock()
	call, ok := run.activeCalls[toolID]
	if !ok {
		run.mu.Unlock()
		t.Fatalf("tracked active call %s not found", toolID)
	}
	call.StartedAt = startedAt
	call.PauseTotal = 0
	run.activeCalls[toolID] = call
	run.mu.Unlock()
}

func countEventsByType(events []Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count += 1
		}
	}
	return count
}

func userMessageTexts(events []Event) []string {
	items := make([]string, 0, len(events))
	for _, event := range events {
		if event.Type == "msg_u" {
			items = append(items, stringValue(event.Payload["txt"]))
		}
	}
	return items
}

func waitForUserMessageCount(t *testing.T, manager *Manager, sessionID string, count int) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events, err := manager.store.readEvents(sessionID)
		if err == nil && len(userMessageTexts(events)) >= count {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	events, err := manager.store.readEvents(sessionID)
	if err != nil {
		t.Fatalf("readEvents returned error while waiting for user messages: %v", err)
	}
	t.Fatalf("expected at least %d user messages, got %#v", count, userMessageTexts(events))
}

func historyHasEvent(events []Event, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func historyHasToolKind(events []Event, kind string) bool {
	for _, event := range events {
		if event.Type != "tool_st" && event.Type != "tool_end" {
			continue
		}
		if value, ok := event.Payload["kind"].(string); ok && value == kind {
			return true
		}
	}
	return false
}

func historyItemsHaveToolKind(items []HistoryItem, kind string) bool {
	for _, item := range items {
		if item.Kind != "tool" || item.Tool == nil {
			continue
		}
		if item.Tool.Kind == kind {
			return true
		}
	}
	return false
}

func historyToolEventByKind(events []Event, kind string) (Event, bool) {
	for _, event := range events {
		if event.Type != "tool_st" && event.Type != "tool_end" {
			continue
		}
		if eventToolKind(event) == kind {
			return event, true
		}
	}
	return Event{}, false
}

func historyEventByID(events []Event, id string) (Event, bool) {
	for _, event := range events {
		if event.ID == id {
			return event, true
		}
	}
	return Event{}, false
}

func testFileChangeEvent(toolID string, seq int64, eventType string, path string, groupID string) Event {
	meta := map[string]any{
		"kind":     "file_change",
		"title":    "FileChange",
		"subtitle": path,
	}
	if groupID != "" {
		meta["commandGroup"] = map[string]any{
			"id":           groupID,
			"count":        1,
			"firstSeq":     seq,
			"lastSeq":      seq,
			"latestToolId": toolID,
			"compacted":    true,
		}
	}
	payload := map[string]any{
		"tid":  toolID,
		"name": "FileChange",
		"kind": "file_change",
		"meta": meta,
	}
	if eventType == "tool_st" {
		payload["in"] = map[string]any{
			"path": path,
			"changes": []any{
				map[string]any{"path": path},
			},
		}
	}
	if eventType == "tool_end" {
		payload["out"] = "patched"
		payload["ok"] = true
	}
	return Event{
		ID:        fmt.Sprintf("evt_%s_%s", toolID, eventType),
		Seq:       seq,
		Type:      eventType,
		Timestamp: time.UnixMilli(seq * 1_000),
		Payload:   payload,
	}
}

func testLongPlanText() string {
	return "## Plan\n" + strings.Repeat("- 计划步骤：保持中文内容完整，不要被截断。\n", 240)
}

func appendHistoryEvent(t *testing.T, manager *Manager, sessionID string, event Event) {
	t.Helper()
	manager.mu.Lock()
	if manager.runs[sessionID] == nil {
		manager.runs[sessionID] = &activeRun{
			sessionID:          sessionID,
			done:               make(chan struct{}),
			assistantDeltaSeen: make(map[string]bool),
		}
	}
	manager.mu.Unlock()
	manager.decorateProjectedEvent(sessionID, &event)
	if err := manager.store.appendEvent(sessionID, event); err != nil {
		t.Fatalf("appendEvent returned error: %v", err)
	}
	if _, err := manager.applyEventToHistoryCache(context.Background(), sessionID, event); err != nil {
		t.Fatalf("applyEventToHistoryCache returned error: %v", err)
	}
}
