package websession

import (
	"context"
	"encoding/json"
	"errors"
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
	if created.Model != utils.DefaultWebSessionCodexModel {
		t.Fatalf("expected default model %q, got %q", utils.DefaultWebSessionCodexModel, created.Model)
	}
	if created.ReasoningEffort != ReasoningEffortXHigh {
		t.Fatalf("expected default reasoning effort %q, got %q", ReasoningEffortXHigh, created.ReasoningEffort)
	}
	if created.AutoRetryDispatchPendingOnFailure {
		t.Fatal("expected retry failure pending dispatch to default to disabled")
	}
}

func TestManagerCreateSessionUsesConfiguredCodexDefaultsAndExplicitOverrides(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	configuredModel := "gpt-5.6-luna"
	configuredEffort := ReasoningEffortHigh
	configuredPermission := utils.WebSessionCodexStandardPermission
	manager, err := NewManager(Config{
		DataDir: t.TempDir(),
		DefaultCodexModel: func() string {
			return configuredModel
		},
		DefaultCodexReasoningEffort: func() ReasoningEffort {
			return configuredEffort
		},
		DefaultCodexPermissionLevel: func() string {
			return configuredPermission
		},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	first, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if first.Model != configuredModel || first.ReasoningEffort != configuredEffort ||
		first.PermissionLevel != PermissionLevelDefault {
		t.Fatalf("expected configured defaults, got model=%q effort=%q permission=%q", first.Model, first.ReasoningEffort, first.PermissionLevel)
	}

	configuredModel = "gpt-5.6-terra"
	configuredEffort = ReasoningEffortMax
	configuredPermission = string(PermissionLevelYolo)
	second, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession after config update returned error: %v", err)
	}
	if second.Model != configuredModel || second.ReasoningEffort != configuredEffort ||
		second.PermissionLevel != PermissionLevelYolo {
		t.Fatalf("expected refreshed defaults, got model=%q effort=%q permission=%q", second.Model, second.ReasoningEffort, second.PermissionLevel)
	}
	if first.Model == second.Model || first.ReasoningEffort == second.ReasoningEffort ||
		first.PermissionLevel == second.PermissionLevel {
		t.Fatalf("existing session defaults changed unexpectedly: first=%#v second=%#v", first, second)
	}

	explicit, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:       project.ID,
		Agent:           AgentCodex,
		Model:           "gpt-5.5",
		ReasoningEffort: ReasoningEffortDefault,
		PermissionLevel: PermissionLevelElevated,
	})
	if err != nil {
		t.Fatalf("CreateSession with explicit values returned error: %v", err)
	}
	if explicit.Model != "gpt-5.5" || explicit.ReasoningEffort != ReasoningEffortDefault ||
		explicit.PermissionLevel != PermissionLevelElevated {
		t.Fatalf("expected explicit values to win, got model=%q effort=%q permission=%q", explicit.Model, explicit.ReasoningEffort, explicit.PermissionLevel)
	}
}

func TestManagerCreateSessionResolvesCodexDefaultSentinels(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	configuredModel := utils.WebSessionCodexDefaultSetting
	configuredEffort := ReasoningEffort(utils.WebSessionCodexDefaultSetting)
	configuredPermission := utils.WebSessionCodexDefaultSetting
	manager, err := NewManager(Config{
		DataDir: t.TempDir(),
		DefaultCodexModel: func() string {
			return configuredModel
		},
		DefaultCodexReasoningEffort: func() ReasoningEffort {
			return configuredEffort
		},
		DefaultCodexPermissionLevel: func() string {
			return configuredPermission
		},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	versionDefault, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if versionDefault.Model != utils.DefaultWebSessionCodexModel ||
		versionDefault.ReasoningEffort != ReasoningEffort(utils.DefaultWebSessionCodexReasoningEffort) ||
		versionDefault.PermissionLevel != PermissionLevel(utils.DefaultWebSessionCodexPermissionLevel) {
		t.Fatalf("expected effective version defaults, got %#v", versionDefault)
	}

	configuredModel = "custom-model"
	configuredEffort = ReasoningEffort(utils.WebSessionCodexModelDefaultEffort)
	configuredPermission = utils.WebSessionCodexStandardPermission
	modelDefaults, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession with explicit global settings returned error: %v", err)
	}
	if modelDefaults.Model != configuredModel ||
		modelDefaults.ReasoningEffort != ReasoningEffortDefault ||
		modelDefaults.PermissionLevel != PermissionLevelDefault {
		t.Fatalf("expected model-default reasoning and standard permission, got %#v", modelDefaults)
	}
}

func TestManagerDefaultCodexSyncModeResolvesSentinel(t *testing.T) {
	configured := SyncMode(utils.WebSessionCodexDefaultSetting)
	manager := &Manager{cfg: Config{
		DefaultCodexSyncMode: func() SyncMode {
			return configured
		},
	}}

	if got := manager.defaultCodexSyncMode(); got != SyncMode(utils.DefaultWebSessionCodexSyncMode) {
		t.Fatalf("expected effective sync default %q, got %q", utils.DefaultWebSessionCodexSyncMode, got)
	}
	configured = SyncModeDeep
	if got := manager.defaultCodexSyncMode(); got != SyncModeDeep {
		t.Fatalf("expected explicit deep sync mode, got %q", got)
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

func TestManagerCreateSessionYoloConfiguresFirstCodexThreadWithoutApprovals(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "verify_yolo"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:       project.ID,
		Agent:           AgentCodex,
		PermissionLevel: PermissionLevelYolo,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if created.PermissionLevel != PermissionLevelYolo || effectivePermissionLevel(record) != PermissionLevelYolo {
		t.Fatalf("expected persisted yolo permission, got summary=%q record=%q", created.PermissionLevel, record.PermissionLevel)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "verify first turn permissions", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
	settled, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession after first turn returned error: %v", err)
	}
	if settled.Status != string(StatusDone) {
		errorMessage := ""
		if settled.LastError != nil {
			errorMessage = *settled.LastError
		}
		t.Fatalf("expected first yolo turn to complete, got status=%q error=%q", settled.Status, errorMessage)
	}
}

func TestCodexCommandApprovalPersistsDetailsAndSnapshot(t *testing.T) {
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
	run := &activeRun{
		sessionID: created.ID,
		runID:     "run-command-approval",
		app:       &codexAppServerClient{},
	}
	manager.mu.Lock()
	manager.runs[created.ID] = run
	manager.mu.Unlock()

	message := codexAppServerIncoming{
		ID:     json.RawMessage(`"approval-command-1"`),
		Method: "item/commandExecution/requestApproval",
		Params: json.RawMessage(`{
			"threadId":"thread-1",
			"turnId":"turn-1",
			"itemId":"command-1",
			"command":"rm -r /tmp/example"
		}`),
	}
	if err := manager.handleCodexAppServerApprovalRequest(record, run, message); err != nil {
		t.Fatalf("handleCodexAppServerApprovalRequest returned error: %v", err)
	}

	refreshed, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession after approval returned error: %v", err)
	}
	snapshot, err := manager.loadSnapshotLocal(context.Background(), refreshed, DefaultHistoryWindow, false)
	if err != nil {
		t.Fatalf("loadSnapshotLocal returned error: %v", err)
	}
	if snapshot.PendingApproval == nil {
		t.Fatal("expected pending approval in snapshot")
	}
	if snapshot.PendingApproval.ItemID != "command-1" ||
		snapshot.PendingApproval.Kind != string(pendingServerRequestCommandApproval) ||
		snapshot.PendingApproval.Command != "rm -r /tmp/example" ||
		!snapshot.PendingApproval.Actionable ||
		strings.TrimSpace(snapshot.PendingApproval.Prompt) == "" {
		t.Fatalf("unexpected pending approval: %#v", snapshot.PendingApproval)
	}
	if len(snapshot.History.Items) != 1 || snapshot.History.Items[0].Detail == nil {
		t.Fatalf("expected persisted approval history item, got %#v", snapshot.History.Items)
	}
	detail := snapshot.History.Items[0].Detail
	if detail.ApprovalKind != string(pendingServerRequestCommandApproval) || detail.Command != "rm -r /tmp/example" {
		t.Fatalf("unexpected approval history detail: %#v", detail)
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
	if err := manager.replaceSessionSubAgents(context.Background(), existing.ID, []WebSessionSubAgent{
		{
			ThreadID: "thread_import_child",
			Nickname: "Imported Agent",
			Status:   WebSessionSubAgentCompleted,
			Summary:  "Cached child result",
		},
	}, true); err != nil {
		t.Fatalf("seed imported sub-agent registry: %v", err)
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
	if len(result.SubAgents) != 1 || result.SubAgents[0].ThreadID != "thread_import_child" {
		t.Fatalf("expected reused import to return its sub-agent registry, got %#v", result.SubAgents)
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

func TestHandleSendCommandRepliesWithRevisionAck(t *testing.T) {
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

	if len(conn.frames) != 1 {
		t.Fatalf("expected one ack frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "send" {
		t.Fatalf("expected first frame to be send ack, got %#v", conn.frames[0])
	}
	if conn.frames[0].Revision == "" {
		t.Fatalf("expected send ack to include revision, got %#v", conn.frames[0])
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

func TestHandleSetAutoRetryDispatchPendingOnFailureCommand(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:                         project.ID,
		Agent:                             AgentCodex,
		AutoRetryDispatchPendingOnFailure: true,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if !created.AutoRetryDispatchPendingOnFailure {
		t.Fatal("expected create parameter to enable retry failure pending dispatch")
	}
	if wire := mapWireSession(created); !wire.AutoRetryDispatchPendingOnFailure {
		t.Fatal("expected compact wire mapping to include enabled retry failure pending dispatch")
	}

	nextAt := time.Now().Add(time.Minute).Truncate(time.Millisecond)
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", created.ID).
		Updates(map[string]any{
			"auto_retry_attempt": 2,
			"auto_retry_next_at": nextAt,
		}).Error; err != nil {
		t.Fatalf("failed to seed auto retry progress: %v", err)
	}

	conn := &captureWSConn{}
	client := manager.RegisterCommandClient(conn)
	defer manager.UnregisterClient(client)

	if err := manager.HandleCommand(
		context.Background(),
		client,
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_set_ardpf","sid":%q,"op":"set_ardpf","p":{"ardpf":false}}`, created.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 1 {
		t.Fatalf("expected one ack frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "set_ardpf" || conn.frames[0].Revision == "" {
		t.Fatalf("expected set_ardpf ack with revision, got %#v", conn.frames[0])
	}

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.AutoRetryDispatchPendingOnFailure {
		t.Fatal("expected set_ardpf command to disable retry failure pending dispatch")
	}
	if record.AutoRetryAttempt != 2 || record.AutoRetryNextAt == nil || !record.AutoRetryNextAt.Equal(nextAt) {
		t.Fatalf("expected retry progress to remain unchanged, got attempt=%d nextAt=%v", record.AutoRetryAttempt, record.AutoRetryNextAt)
	}
	if summary := mapSessionRecord(record); summary.AutoRetryDispatchPendingOnFailure {
		t.Fatal("expected REST summary mapping to include disabled retry failure pending dispatch")
	} else if wire := mapWireSession(summary); wire.AutoRetryDispatchPendingOnFailure {
		t.Fatal("expected compact wire mapping to include disabled retry failure pending dispatch")
	}
}

func TestHandlePendingUpdateCommandRepliesWithRevisionAck(t *testing.T) {
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
		[]byte(fmt.Sprintf(`{"v":1,"k":"cmd","rid":"req_pending_update","sid":%q,"op":"pending_update","p":{"id":%q,"txt":"updated draft","paused":true}}`, created.ID, item.ID)),
	); err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}

	if len(conn.frames) != 1 {
		t.Fatalf("expected one ack frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "pending_update" {
		t.Fatalf("expected first frame to be pending_update ack, got %#v", conn.frames[0])
	}
	if conn.frames[0].Revision == "" {
		t.Fatalf("expected pending update ack revision, got %#v", conn.frames[0])
	}
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 1 ||
		pending[0].Text != "updated draft" || !pending[0].Paused || pending[0].ReadyAt != nil {
		t.Fatalf("expected pending input to be updated, got %#v", pending)
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

	if len(conn.frames) != 1 {
		t.Fatalf("expected one ack frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "pending_reorder" {
		t.Fatalf("expected first frame to be pending_reorder ack, got %#v", conn.frames[0])
	}
	if conn.frames[0].Revision == "" {
		t.Fatalf("expected pending reorder ack revision, got %#v", conn.frames[0])
	}
	if got := manager.pendingInputsSnapshot(created.ID); len(got) != 3 || got[0].ID != "queue-2" || got[0].Mode != "redirect" || got[1].ID != "redirect-1" || got[2].ID != "queue-1" {
		t.Fatalf("expected reordered pending inputs, got %#v", got)
	}
}

func TestHandlePendingClearCommandRepliesWithRevisionAck(t *testing.T) {
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

	if len(conn.frames) != 1 {
		t.Fatalf("expected one ack frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "pending_clear" {
		t.Fatalf("expected first frame to be pending_clear ack, got %#v", conn.frames[0])
	}
	if conn.frames[0].Revision == "" {
		t.Fatalf("expected pending clear ack revision, got %#v", conn.frames[0])
	}
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected cleared pending inputs, got %#v", pending)
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
	expected := "Codex web sessions require Codex >= 0.146.0. Current version: 0.132.9."
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

	if len(conn.frames) != 1 {
		t.Fatalf("expected one ack frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" {
		t.Fatalf("expected first frame to be ack, got %#v", conn.frames[0])
	}
	if conn.frames[0].Revision == "" {
		t.Fatalf("expected goal bootstrap ack revision, got %#v", conn.frames[0])
	}
	snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.Session.Goal == nil {
		t.Fatalf("expected session to include goal, got %#v", snapshot.Session)
	}
	if snapshot.Session.Goal.Objective != "Generate a first draft immediately" {
		t.Fatalf("expected goal objective to be hydrated, got %#v", snapshot.Session.Goal)
	}
	if snapshot.Session.NativeSessionID == nil || strings.TrimSpace(*snapshot.Session.NativeSessionID) == "" {
		t.Fatalf("expected session to include native session id, got %#v", snapshot.Session)
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

func TestHandleApprovalCommandRepliesWithRevisionAck(t *testing.T) {
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

	if len(conn.frames) != 1 {
		t.Fatalf("expected one approve ack frame, got %#v", conn.frames)
	}
	if conn.frames[0].Kind != "ack" || conn.frames[0].Operation != "approve" {
		t.Fatalf("expected first frame to be approve ack, got %#v", conn.frames[0])
	}
	if conn.frames[0].Revision == "" {
		t.Fatalf("expected approval ack revision, got %#v", conn.frames[0])
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

func TestManagerAppendAndBroadcastContinuesAfterDurableSeqAheadOfDatabase(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Durable event sequence", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if err := manager.store.appendEvent(session.ID, Event{
		ID:        "evt_durable_only",
		Seq:       7,
		Type:      "note",
		Timestamp: time.Now(),
		Payload:   map[string]any{"txt": "already durable"},
	}); err != nil {
		t.Fatalf("append durable-only event: %v", err)
	}

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	appended, err := manager.appendAndBroadcast(context.Background(), session.ID, record, Event{
		ID:        "evt_after_durable",
		Type:      "note",
		Timestamp: time.Now(),
		Payload:   map[string]any{"txt": "next event"},
	})
	if err != nil {
		t.Fatalf("appendAndBroadcast returned error: %v", err)
	}
	if appended.Seq != 8 {
		t.Fatalf("expected sequence 8 after durable sequence 7, got %d", appended.Seq)
	}
}

func TestManagerNextEventSeqRejectsCorruptDurableTail(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Unreadable durable sequence", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if err := manager.store.ensureSessionDir(session.ID); err != nil {
		t.Fatalf("ensure session directory: %v", err)
	}

	valid := `{"id":"evt-7","seq":7,"type":"note","timestamp":"2026-07-30T00:00:00Z"}` + "\n"
	tests := []struct {
		name string
		tail string
		want string
	}{
		{name: "incomplete trailing event", tail: `{"id":"evt-8","seq":8`, want: "incomplete trailing event"},
		{name: "malformed complete event", tail: "not-json\n", want: "decode web session history tail"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(manager.store.historyPath(session.ID), []byte(valid+test.tail), 0o600); err != nil {
				t.Fatalf("write corrupt durable tail: %v", err)
			}
			state := &sessionEventState{}
			if _, err := manager.nextEventSeq(context.Background(), session.ID, state); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected durable sequence read error containing %q, got %v", test.want, err)
			}
			if state.seqInitialized {
				t.Fatal("failed durable sequence read must not initialize event state")
			}
		})
	}
}

func TestStoreLatestEventSeqReadsLargeTrailingEvent(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	largeText := strings.Repeat("x", 8*1024*1024+1)
	if err := manager.store.appendEvent(session.ID, Event{
		ID:        "evt-large",
		Seq:       7,
		Type:      "note",
		Timestamp: time.Now(),
		Payload:   map[string]any{"txt": largeText},
	}); err != nil {
		t.Fatalf("append large durable event: %v", err)
	}
	seq, err := manager.store.latestEventSeq(session.ID)
	if err != nil || seq != 7 {
		t.Fatalf("latestEventSeq for large trailing event = %d, %v; want 7, nil", seq, err)
	}
}

func TestManagerRetriesPersistedEventProjectionWithoutReappend(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Persisted projection retry", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	state := manager.sessionEventState(session.ID)
	state.mu.Lock()
	state.seqInitialized = true
	state.lastSeq = 0
	state.mu.Unlock()

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	appended, err := manager.appendAndBroadcast(cancelledCtx, session.ID, *session, Event{
		ID:        "evt-projection-retry",
		Type:      "txt_d",
		RunID:     "run-1",
		ParentID:  "message-1",
		Timestamp: time.Now(),
		Payload:   map[string]any{"mid": "message-1", "txt": "project me once"},
	})
	if err == nil {
		t.Fatal("expected the cancelled projection to fail")
	}
	persisted, ok := persistedEventFromError(err, "evt-projection-retry")
	if !ok || persisted.Seq != 1 || appended.ID != "" {
		t.Fatalf("expected a classified persisted event, appended=%#v persisted=%#v ok=%v err=%v", appended, persisted, ok, err)
	}

	events, err := manager.store.readEvents(session.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one durable event before projection retry, events=%#v err=%v", events, err)
	}

	var window HistoryWindow
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		window, err = manager.loadHistoryWindow(context.Background(), session.ID, 10, nil)
		if err == nil && len(window.Items) == 1 && window.Items[0].Text == "project me once" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || len(window.Items) != 1 || window.Items[0].Text != "project me once" {
		t.Fatalf("background projection retry did not restore the cache: window=%#v err=%v", window, err)
	}

	events, err = manager.store.readEvents(session.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("projection retry must not reappend JSONL, events=%#v err=%v", events, err)
	}
	state.mu.Lock()
	queued := len(state.projectionRetries)
	timer := state.projectionTimer
	state.mu.Unlock()
	if queued != 0 || timer != nil {
		t.Fatalf("projection retry did not drain cleanly: queued=%d timer=%v", queued, timer)
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil || record.LastEventSeq != 1 {
		t.Fatalf("projection retry did not restore runtime sequence: seq=%d err=%v", record.LastEventSeq, err)
	}
}

func TestPersistedEventFromErrorRequiresMatchingEvent(t *testing.T) {
	persisted := Event{ID: "evt-1", Seq: 3}
	err := eventPersistedError{event: persisted, err: errors.New("cache unavailable")}
	if event, ok := persistedEventFromError(err, "evt-1"); !ok || event.Seq != 3 {
		t.Fatalf("expected matching persisted event, got %#v ok=%v", event, ok)
	}
	if _, ok := persistedEventFromError(err, "evt-2"); ok {
		t.Fatal("different event id must not be classified as persisted")
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
		[]byte(fmt.Sprintf("model = %q\nmodel_context_window = 1000000\n", utils.DefaultWebSessionCodexModel)),
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
	if config.Model != utils.DefaultWebSessionCodexModel {
		t.Fatalf("expected runtime model %q, got %q", utils.DefaultWebSessionCodexModel, config.Model)
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
		CodexPath:  writeFakeCodexVersionCLI(t, "0.146.0"),
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
	if config.CodexVersion == nil || *config.CodexVersion != "0.146.0" {
		t.Fatalf("expected codexVersion 0.146.0, got %#v", config.CodexVersion)
	}
	if !config.SupportsWebSession {
		t.Fatal("expected supportsWebSession true")
	}
	if config.WebSessionMinVersion != "0.146.0" {
		t.Fatalf("expected webSessionMinCodexVersion 0.146.0, got %q", config.WebSessionMinVersion)
	}
	if !config.SupportsGoalMode {
		t.Fatal("expected supportsGoalMode true")
	}
	if config.GoalModeMinVersion != "0.133.0" {
		t.Fatalf("expected goalModeMinVersion 0.133.0, got %q", config.GoalModeMinVersion)
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal runtime config: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal runtime config: %v", err)
	}
	if payload["supportsWebSession"] != true || payload["webSessionMinCodexVersion"] != "0.146.0" {
		t.Fatalf("unexpected web-session capability payload: %#v", payload)
	}
}

func TestGetCodexRuntimeConfigGoalModeDisabledForOldVersion(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.145.9"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	config := manager.GetCodexRuntimeConfig()
	if !config.HasCodex {
		t.Fatal("expected hasCodex true")
	}
	if config.CodexVersion == nil || *config.CodexVersion != "0.145.9" {
		t.Fatalf("expected codexVersion 0.145.9, got %#v", config.CodexVersion)
	}
	if config.SupportsWebSession {
		t.Fatal("expected supportsWebSession false")
	}
	if config.SupportsGoalMode {
		t.Fatal("expected supportsGoalMode false")
	}
}

func TestGetCodexRuntimeConfigWebSessionDisabledForUnknownVersion(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "unknown"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	config := manager.GetCodexRuntimeConfig()
	if !config.HasCodex {
		t.Fatal("expected hasCodex true")
	}
	if config.CodexVersion != nil {
		t.Fatalf("expected unknown codexVersion, got %#v", config.CodexVersion)
	}
	if config.SupportsWebSession {
		t.Fatal("expected supportsWebSession false")
	}
	if config.WebSessionMinVersion != "0.146.0" {
		t.Fatalf("expected webSessionMinCodexVersion 0.146.0, got %q", config.WebSessionMinVersion)
	}
}

func TestCodexWebSessionSupportErrorReportsVersionState(t *testing.T) {
	oldVersion := "0.145.9"
	tests := []struct {
		name     string
		config   CodexRuntimeConfig
		expected string
	}{
		{
			name: "detected old version",
			config: CodexRuntimeConfig{
				HasCodex:             true,
				CodexVersion:         &oldVersion,
				WebSessionMinVersion: "0.146.0",
			},
			expected: "Codex web sessions require Codex >= 0.146.0. Current version: 0.145.9.",
		},
		{
			name: "unknown version",
			config: CodexRuntimeConfig{
				HasCodex:             true,
				WebSessionMinVersion: "0.146.0",
			},
			expected: "Codex web sessions require Codex >= 0.146.0. The installed Codex version could not be determined.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := codexWebSessionSupportError(test.config)
			if err == nil || err.Error() != test.expected {
				t.Fatalf("expected error %q, got %v", test.expected, err)
			}
			if !errors.Is(err, ErrCodexWebSessionUnavailable) {
				t.Fatalf("expected runtime-unavailable classification, got %v", err)
			}
		})
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

	archivedResult, err := manager.ListArchivedSessions(context.Background(), []string{project.ID}, "", 20, 0)
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
	sessionB := seedWebSession(t, project.ID, "Needle title", 2000)
	sessionC := seedWebSession(t, project.ID, "C", 3000)
	seedWebSession(t, project.ID, "Needle current session", 4000)
	for id, activityAt := range map[string]time.Time{
		sessionA.ID: now.Add(-3 * time.Hour),
		sessionB.ID: now.Add(-1 * time.Hour),
		sessionC.ID: now.Add(-2 * time.Hour),
	} {
		updates := map[string]any{
			"archived_at": now,
			"activity_at": activityAt,
			"updated_at":  now,
		}
		if id == sessionC.ID {
			updates["thread_preview"] = "Needle in preview"
		}
		if err := model.GetDB().Model(&tables.WebSessionTable{}).
			Where("id = ?", id).
			Updates(updates).Error; err != nil {
			t.Fatalf("failed to update archived seed %s: %v", id, err)
		}
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	pageOne, err := manager.ListArchivedSessions(context.Background(), []string{project.ID}, "", 2, 0)
	if err != nil {
		t.Fatalf("ListArchivedSessions page one returned error: %v", err)
	}
	if !pageOne.HasMore || pageOne.NextOffset != 2 {
		t.Fatalf("expected first page to have more results, got %+v", pageOne)
	}
	if len(pageOne.Items) != 2 || pageOne.Items[0].ID != sessionB.ID || pageOne.Items[1].ID != sessionC.ID {
		t.Fatalf("unexpected first archived page order: %#v", pageOne.Items)
	}

	pageTwo, err := manager.ListArchivedSessions(context.Background(), []string{project.ID}, "", 2, pageOne.NextOffset)
	if err != nil {
		t.Fatalf("ListArchivedSessions page two returned error: %v", err)
	}
	if pageTwo.HasMore {
		t.Fatalf("expected second page to be final, got %+v", pageTwo)
	}
	if len(pageTwo.Items) != 1 || pageTwo.Items[0].ID != sessionA.ID {
		t.Fatalf("unexpected second archived page order: %#v", pageTwo.Items)
	}

	searchPageOne, err := manager.ListArchivedSessions(context.Background(), []string{project.ID}, "  NEEDLE ", 1, 0)
	if err != nil {
		t.Fatalf("ListArchivedSessions filtered page one returned error: %v", err)
	}
	if searchPageOne.Total != 2 || !searchPageOne.HasMore || searchPageOne.NextOffset != 1 {
		t.Fatalf("unexpected filtered first page metadata: %+v", searchPageOne)
	}
	if len(searchPageOne.Items) != 1 || searchPageOne.Items[0].ID != sessionB.ID {
		t.Fatalf("expected title match first, got %#v", searchPageOne.Items)
	}

	searchPageTwo, err := manager.ListArchivedSessions(
		context.Background(),
		[]string{project.ID},
		"needle",
		1,
		searchPageOne.NextOffset,
	)
	if err != nil {
		t.Fatalf("ListArchivedSessions filtered page two returned error: %v", err)
	}
	if searchPageTwo.Total != 2 || searchPageTwo.HasMore || searchPageTwo.NextOffset != 2 {
		t.Fatalf("unexpected filtered second page metadata: %+v", searchPageTwo)
	}
	if len(searchPageTwo.Items) != 1 || searchPageTwo.Items[0].ID != sessionC.ID {
		t.Fatalf("expected preview match second, got %#v", searchPageTwo.Items)
	}
}

func TestManagerSearchSessionsChunkFindsSyncedBodyProgressively(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	otherProject := seedProject(t)
	now := time.Now()
	titleMatch := seedWebSession(t, project.ID, "Needle in title", 1000)
	noMatch := seedWebSession(t, project.ID, "No match", 2000)
	bodyMatch := seedWebSession(t, project.ID, "Body match", 3000)
	archivedMatch := seedWebSession(t, project.ID, "Archived body match", 4000)
	otherMatch := seedWebSession(t, otherProject.ID, "Other project", 5000)

	for index, session := range []*tables.WebSessionTable{
		titleMatch,
		noMatch,
		bodyMatch,
		archivedMatch,
		otherMatch,
	} {
		updates := map[string]any{
			"activity_at": now.Add(-time.Duration(index) * time.Hour),
			"updated_at":  now,
		}
		if session.ID == archivedMatch.ID {
			updates["archived_at"] = now
		}
		if err := model.GetDB().Model(&tables.WebSessionTable{}).
			Where("id = ?", session.ID).
			Updates(updates).Error; err != nil {
			t.Fatalf("failed to update search seed %s: %v", session.ID, err)
		}
	}

	for order, session := range []*tables.WebSessionTable{
		titleMatch,
		bodyMatch,
		archivedMatch,
		otherMatch,
	} {
		row := &tables.WebSessionItemTable{
			WebSessionID: session.ID,
			OrderIndex:   int64(order + 1),
			ItemKind:     "message",
			ItemType:     "agent_message",
			Text:         "Needle stored in synchronized body text",
		}
		row.Init()
		if err := model.GetDB().Create(row).Error; err != nil {
			t.Fatalf("failed to seed search body: %v", err)
		}
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	pageOne, err := manager.SearchSessionsChunk(
		context.Background(),
		[]string{"", project.ID, project.ID},
		"  NEEDLE  ",
		false,
		true,
		"",
		2,
	)
	if err != nil {
		t.Fatalf("SearchSessionsChunk page one returned error: %v", err)
	}
	if pageOne.Done || pageOne.NextCursor == "" || pageOne.Scanned != 2 || pageOne.Total != 3 {
		t.Fatalf("unexpected first search page metadata: %+v", pageOne)
	}
	if len(pageOne.Items) != 1 || pageOne.Items[0].ID != titleMatch.ID {
		t.Fatalf("expected title match on first search page, got %#v", pageOne.Items)
	}
	if !reflect.DeepEqual(
		pageOne.Items[0].SearchMatchSources,
		[]SessionSearchMatchSource{SessionSearchMatchTitle, SessionSearchMatchBody},
	) {
		t.Fatalf("expected combined title and body sources, got %#v", pageOne.Items[0].SearchMatchSources)
	}

	pageTwo, err := manager.SearchSessionsChunk(
		context.Background(),
		[]string{project.ID},
		"needle",
		false,
		true,
		pageOne.NextCursor,
		2,
	)
	if err != nil {
		t.Fatalf("SearchSessionsChunk page two returned error: %v", err)
	}
	if !pageTwo.Done || pageTwo.NextCursor != "" || pageTwo.Scanned != 1 || pageTwo.Total != 3 {
		t.Fatalf("unexpected second search page metadata: %+v", pageTwo)
	}
	if len(pageTwo.Items) != 1 || pageTwo.Items[0].ID != bodyMatch.ID {
		t.Fatalf("expected synchronized body match on second search page, got %#v", pageTwo.Items)
	}
	if !reflect.DeepEqual(
		pageTwo.Items[0].SearchMatchSources,
		[]SessionSearchMatchSource{SessionSearchMatchBody},
	) {
		t.Fatalf("expected body-only source, got %#v", pageTwo.Items[0].SearchMatchSources)
	}

	withArchived, err := manager.SearchSessionsChunk(
		context.Background(),
		[]string{project.ID},
		"needle",
		true,
		true,
		"",
		10,
	)
	if err != nil {
		t.Fatalf("SearchSessionsChunk with archived returned error: %v", err)
	}
	if !withArchived.Done || withArchived.Total != 4 || len(withArchived.Items) != 3 {
		t.Fatalf("unexpected archived search result: %+v", withArchived)
	}
	if withArchived.Items[2].ID != archivedMatch.ID || withArchived.Items[2].ArchivedAt == nil {
		t.Fatalf("expected archived body match last, got %#v", withArchived.Items)
	}

	withoutBody, err := manager.SearchSessionsChunk(
		context.Background(),
		[]string{project.ID},
		"needle",
		true,
		false,
		"",
		10,
	)
	if err != nil {
		t.Fatalf("SearchSessionsChunk without body returned error: %v", err)
	}
	if len(withoutBody.Items) != 1 || withoutBody.Items[0].ID != titleMatch.ID {
		t.Fatalf("expected title-only match with body search disabled, got %#v", withoutBody.Items)
	}
	if !reflect.DeepEqual(
		withoutBody.Items[0].SearchMatchSources,
		[]SessionSearchMatchSource{SessionSearchMatchTitle},
	) {
		t.Fatalf("expected title-only source, got %#v", withoutBody.Items[0].SearchMatchSources)
	}

	if _, err := manager.SearchSessionsChunk(
		context.Background(),
		[]string{project.ID},
		"needle",
		false,
		true,
		"not-a-valid-cursor",
		2,
	); !errors.Is(err, ErrInvalidSessionSearchCursor) {
		t.Fatalf("expected invalid cursor error, got %v", err)
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

func TestCodexAppServerAutoContinuesIncompleteGPT56Turn(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		workflowMode WorkflowMode
	}{
		{name: "plan commentary only", mode: "incomplete_commentary_then_success", workflowMode: WorkflowModePlan},
		{name: "commentary phase inherited", mode: "incomplete_inherited_commentary_then_success", workflowMode: WorkflowModeDefault},
		{name: "empty unknown phase", mode: "incomplete_empty_unknown_then_success", workflowMode: WorkflowModeDefault},
		{name: "default tool only", mode: "incomplete_tool_then_success", workflowMode: WorkflowModeDefault},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleanup := initTestDB(t)
			defer cleanup()

			project := seedProject(t)
			manager, err := NewManager(Config{
				DataDir:   t.TempDir(),
				CodexPath: writeFakeCodexAppServerCLI(t, test.mode),
			}, zap.NewNop())
			if err != nil {
				t.Fatalf("NewManager returned error: %v", err)
			}

			created, err := manager.CreateSession(context.Background(), CreateParams{
				ProjectID:    project.ID,
				Agent:        AgentCodex,
				Model:        "gpt-5.6-sol",
				WorkflowMode: test.workflowMode,
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
				t.Fatalf("expected completed continuation, got status %q", record.Status)
			}

			events, err := manager.store.readEvents(created.ID)
			if err != nil {
				t.Fatalf("readEvents returned error: %v", err)
			}
			if countEventsByType(events, "run_st") != 1 || countEventsByType(events, "run_done") != 1 {
				t.Fatalf("expected one logical run, got %#v", events)
			}
			if countEventsByType(events, "run_fail") != 0 {
				t.Fatalf("expected successful continuation without run_fail, got %#v", events)
			}
			if got := userMessageTexts(events); len(got) != 1 || got[0] != "inspect" {
				t.Fatalf("expected internal continuation to stay hidden, got %#v", got)
			}

			sawContinuationNote := false
			sawFinalAnswer := false
			for _, event := range events {
				if event.Type == "note" && stringValue(event.Payload["code"]) == "incomplete_turn_auto_continue" {
					sawContinuationNote = true
				}
				if event.Type == "txt_d" && strings.Contains(stringValue(event.Payload["txt"]), "continued-final") {
					sawFinalAnswer = true
				}
			}
			if !sawContinuationNote {
				t.Fatal("expected automatic continuation note")
			}
			if !sawFinalAnswer {
				t.Fatal("expected final answer from the continued turn")
			}
		})
	}
}

func TestCodexAppServerAcceptsCompletedGPT56ResponseWithoutStablePhase(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "phase missing", mode: "completed_without_phase"},
		{name: "started final phase inherited", mode: "completed_with_inherited_final_phase"},
		{name: "unknown future phase", mode: "completed_with_unknown_phase"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleanup := initTestDB(t)
			defer cleanup()

			project := seedProject(t)
			manager, err := NewManager(Config{
				DataDir:   t.TempDir(),
				CodexPath: writeFakeCodexAppServerCLI(t, test.mode),
			}, zap.NewNop())
			if err != nil {
				t.Fatalf("NewManager returned error: %v", err)
			}

			created, err := manager.CreateSession(context.Background(), CreateParams{
				ProjectID: project.ID,
				Agent:     AgentCodex,
				Model:     "gpt-5.6-sol",
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
				t.Fatalf("expected compatible response to complete, got status %q", record.Status)
			}

			events, err := manager.store.readEvents(created.ID)
			if err != nil {
				t.Fatalf("readEvents returned error: %v", err)
			}
			if countEventsByType(events, "run_done") != 1 || countEventsByType(events, "run_fail") != 0 {
				t.Fatalf("expected one successful run, got %#v", events)
			}
			for _, event := range events {
				if event.Type == "note" && stringValue(event.Payload["code"]) == "incomplete_turn_auto_continue" {
					t.Fatalf("expected no automatic continuation, got %#v", event)
				}
			}
		})
	}
}

func TestCodexAppServerFailsAfterIncompleteGPT56Continuation(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "incomplete_twice"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
		Model:     "gpt-5.6-luna",
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
		t.Fatalf("expected incomplete turn to fail, got status %q", record.Status)
	}
	if record.AutoRetryLastErrorCode == nil || *record.AutoRetryLastErrorCode != codexIncompleteTurnErrorCode {
		t.Fatalf("expected incomplete_turn error code, got %#v", record.AutoRetryLastErrorCode)
	}

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if countEventsByType(events, "note") != 1 || countEventsByType(events, "run_fail") != 1 {
		t.Fatalf("expected one continuation and one terminal failure, got %#v", events)
	}
	if countEventsByType(events, "run_done") != 0 {
		t.Fatalf("expected no successful completion after repeated incomplete turns, got %#v", events)
	}
}

func TestCodexAppServerDoesNotGuardIncompleteNonGPT56Turn(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "incomplete_commentary_then_success"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
		Model:     "gpt-5.5",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "inspect", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if countEventsByType(events, "run_done") != 1 || countEventsByType(events, "note") != 0 {
		t.Fatalf("expected non-GPT-5.6 behavior to remain unchanged, got %#v", events)
	}
}

func TestCodexAppServerDoesNotContinueCompletedGPT56Plan(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "plan_only"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		Model:        "gpt-5.6-terra",
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "plan this", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusRunning) || record.AssistantState != string(AssistantStateWaitingPlanApproval) {
		t.Fatalf("expected plan approval state, got status=%q assistantState=%q", record.Status, record.AssistantState)
	}

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if countEventsByType(events, "note") != 0 {
		t.Fatalf("expected completed plan not to trigger automatic continuation, got %#v", events)
	}
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
	agents, err := manager.sessionSubAgents(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("sessionSubAgents returned error: %v", err)
	}
	if len(agents) != 1 || agents[0].ThreadID != "thread_child" {
		t.Fatalf("expected registry to exclude the root thread, got %#v", agents)
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
	if run.codexRolloutMonitorSnapshot() == nil {
		t.Fatal("expected a fresh Codex thread to monitor its rollout")
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

func TestCodexSubAgentRegistrySurvivesEmptyWaitTimeout(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Sub Agent registry", 1000)
	rootThreadID := "thread_root"
	session.NativeSessionID = ptr(rootThreadID)
	if err := model.GetDB().Save(session).Error; err != nil {
		t.Fatalf("save root thread id: %v", err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	run := &activeRun{runID: "run_sub_agent", agent: AgentCodex}
	rootScope := codexTurnScope{threadID: rootThreadID, turnID: "turn_root"}
	params := func(value map[string]any) json.RawMessage {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatalf("marshal app-server params: %v", marshalErr)
		}
		return encoded
	}

	_, err = manager.handleCodexAppServerMessage(*session, run, nil, rootScope, codexAppServerIncoming{
		Method: "item/completed",
		Params: params(map[string]any{
			"threadId": rootThreadID,
			"turnId":   "turn_root",
			"item": map[string]any{
				"type":              "collabAgentToolCall",
				"id":                "spawn_1",
				"tool":              "spawnAgent",
				"status":            "completed",
				"senderThreadId":    rootThreadID,
				"receiverThreadIds": []any{"thread_child"},
				"agentsStates": map[string]any{
					"thread_child": map[string]any{"status": "pendingInit"},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("handle spawn completion: %v", err)
	}

	agents, err := manager.sessionSubAgents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("sessionSubAgents after spawn: %v", err)
	}
	if len(agents) != 1 || agents[0].ThreadID != "thread_child" || !webSessionSubAgentIsActive(agents[0].Status) {
		t.Fatalf("expected one active child after spawn, got %#v", agents)
	}

	_, err = manager.handleCodexAppServerMessage(*session, run, nil, rootScope, codexAppServerIncoming{
		Method: "item/completed",
		Params: params(map[string]any{
			"threadId": rootThreadID,
			"turnId":   "turn_root",
			"item": map[string]any{
				"type":              "collabAgentToolCall",
				"id":                "wait_1",
				"tool":              "wait",
				"status":            "completed",
				"senderThreadId":    rootThreadID,
				"receiverThreadIds": []any{},
				"agentsStates":      map[string]any{},
			},
		}),
	})
	if err != nil {
		t.Fatalf("handle wait timeout completion: %v", err)
	}
	agents, err = manager.sessionSubAgents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("sessionSubAgents after wait timeout: %v", err)
	}
	if len(agents) != 1 || !webSessionSubAgentIsActive(agents[0].Status) {
		t.Fatalf("empty wait timeout must preserve active child, got %#v", agents)
	}

	_, err = manager.handleCodexAppServerMessage(*session, run, nil, rootScope, codexAppServerIncoming{
		Method: "item/started",
		Params: params(map[string]any{
			"threadId": "thread_child",
			"turnId":   "turn_child",
			"item": map[string]any{
				"type":    "commandExecution",
				"id":      "child_command",
				"command": "pwd",
			},
		}),
	})
	if err != nil {
		t.Fatalf("handle child command: %v", err)
	}
	snapshot, err := manager.Snapshot(context.Background(), session.ID, 20)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	var childCommand *HistoryItem
	for index := range snapshot.History.Items {
		item := &snapshot.History.Items[index]
		if item.Tool != nil && item.Tool.ID == "child_command" {
			childCommand = item
			break
		}
	}
	if childCommand == nil || childCommand.SourceThreadID == nil || *childCommand.SourceThreadID != "thread_child" {
		t.Fatalf("expected child command thread attribution, got %#v", childCommand)
	}

	_, err = manager.handleCodexAppServerMessage(*session, run, nil, rootScope, codexAppServerIncoming{
		Method: "turn/completed",
		Params: params(map[string]any{
			"threadId": "thread_child",
			"turn": map[string]any{
				"id":     "turn_child",
				"status": "completed",
			},
		}),
	})
	if err != nil {
		t.Fatalf("handle child completion: %v", err)
	}
	agents, err = manager.sessionSubAgents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("sessionSubAgents after child completion: %v", err)
	}
	if len(agents) != 1 || agents[0].Status != WebSessionSubAgentCompleted {
		t.Fatalf("expected completed child, got %#v", agents)
	}
}

func TestCodexV2SubAgentActivityUpdatesRegistryAndHistory(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "V2 Sub Agent activity", 1000)
	rootThreadID := "thread_root"
	session.NativeSessionID = ptr(rootThreadID)
	if err := model.GetDB().Save(session).Error; err != nil {
		t.Fatalf("save root thread id: %v", err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	run := &activeRun{runID: "run_v2_sub_agent", agent: AgentCodex}
	rootScope := codexTurnScope{threadID: rootThreadID, turnID: "turn_root"}
	params := func(kind string) json.RawMessage {
		encoded, marshalErr := json.Marshal(map[string]any{
			"threadId": rootThreadID,
			"turnId":   "turn_root",
			"item": map[string]any{
				"type":          "subAgentActivity",
				"id":            "activity_" + kind,
				"kind":          kind,
				"agentThreadId": "thread_child",
				"agentPath":     "review/atlas",
			},
		})
		if marshalErr != nil {
			t.Fatalf("marshal app-server params: %v", marshalErr)
		}
		return encoded
	}

	_, err = manager.handleCodexAppServerMessage(*session, run, nil, rootScope, codexAppServerIncoming{
		Method: "item/completed",
		Params: params("started"),
	})
	if err != nil {
		t.Fatalf("handle started activity: %v", err)
	}
	agents, err := manager.sessionSubAgents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("sessionSubAgents after started activity: %v", err)
	}
	if len(agents) != 1 || agents[0].ThreadID != "thread_child" ||
		agents[0].ParentThreadID == nil || *agents[0].ParentThreadID != rootThreadID ||
		agents[0].Path != "review/atlas" || agents[0].Status != WebSessionSubAgentRunning {
		t.Fatalf("unexpected registry after started activity: %#v", agents)
	}

	snapshot, err := manager.Snapshot(context.Background(), session.ID, 20)
	if err != nil {
		t.Fatalf("Snapshot after started activity: %v", err)
	}
	if len(snapshot.History.Items) != 1 {
		t.Fatalf("expected one semantic activity item, got %#v", snapshot.History.Items)
	}
	activity := snapshot.History.Items[0]
	if activity.ItemType != "sub_agent_activity" ||
		activity.SourceThreadID == nil || *activity.SourceThreadID != "thread_child" ||
		activity.SourceTurnID == nil || *activity.SourceTurnID != "turn_root" ||
		activity.SourceItemID == nil || *activity.SourceItemID != "activity_started" {
		t.Fatalf("unexpected semantic activity history item: %#v", activity)
	}

	_, err = manager.handleCodexAppServerMessage(*session, run, nil, rootScope, codexAppServerIncoming{
		Method: "item/completed",
		Params: params("interrupted"),
	})
	if err != nil {
		t.Fatalf("handle interrupted activity: %v", err)
	}
	agents, err = manager.sessionSubAgents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("sessionSubAgents after interrupted activity: %v", err)
	}
	if len(agents) != 1 || agents[0].Status != WebSessionSubAgentInterrupted || agents[0].EndedAt == nil {
		t.Fatalf("expected interrupted child with terminal timestamp, got %#v", agents)
	}
}

func TestRunFailureReclassifiesMessageOnlyCyberPolicyError(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:        project.ID,
		Agent:            AgentCodex,
		AutoRetryEnabled: true,
		AutoRetryScope:   AutoRetryScopeAllFailures,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	session, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	run := &activeRun{
		runID:         "run-cyber-policy",
		agent:         AgentCodex,
		lastErrorCode: codexRuntimeErrorCode,
	}
	manager.handleRunFailureWithCode(
		created.ID,
		session,
		run,
		codexRuntimeErrorCode,
		errors.New(
			"This content was flagged for possible cybersecurity risk. If this seems wrong, try rephrasing your request.",
		),
	)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusError) || !record.CyberPolicyFlagged {
		t.Fatalf("expected flagged error session, got status=%q flagged=%v", record.Status, record.CyberPolicyFlagged)
	}
	summary := manager.mapSessionSummary(record)
	if !summary.CyberPolicyFlagged || !mapWireSession(summary).CyberPolicyFlagged {
		t.Fatal("expected cyber policy flag in session summary and compact wire payload")
	}
	if record.AutoRetryAttempt != 0 || record.AutoRetryNextAt != nil {
		t.Fatalf("expected no cyber policy retry, got attempt=%d next=%v", record.AutoRetryAttempt, record.AutoRetryNextAt)
	}

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	for _, event := range events {
		if event.Type == "run_fail" && stringValue(event.Payload["code"]) == codexCyberPolicyErrorCode {
			return
		}
	}
	t.Fatalf("expected structured cyber policy run_fail event, got %#v", events)
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

func TestCodexSteerRunsBeforeAutoRetryContinuation(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	if runtime.GOOS == "windows" {
		stableCwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd returned error: %v", err)
		}
		project.Path = stableCwd
		if err := model.GetDB().Model(&tables.ProjectTable{}).
			Where("id = ?", project.ID).
			Update("path", project.Path).Error; err != nil {
			t.Fatalf("failed to set stable Windows process cwd: %v", err)
		}
	}
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "delayed_failure_then_success"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	manager.pendingSteerDelay = 25 * time.Millisecond

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
	if err := manager.sendMessageWithMode(
		context.Background(),
		created.ID,
		"next",
		nil,
		PendingInputModeRedirect,
		"next-1",
	); err != nil {
		t.Fatalf("sendMessageWithMode returned error: %v", err)
	}

	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		events, readErr := manager.store.readEvents(created.ID)
		if readErr == nil && len(userMessageTexts(events)) >= 3 && !manager.hasActiveRun(created.ID) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(events), "|"); got != "inspect|next|continue" {
		t.Fatalf("expected steer before retry continuation, got %q", got)
	}
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected pending redirect to flush after retry success, got %#v", pending)
	}
}

func TestTerminalAutoRetryFailureHoldsPendingUntilEnabled(t *testing.T) {
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
		ProjectID:        project.ID,
		Agent:            AgentCodex,
		AutoRetryEnabled: true,
		AutoRetryScope:   AutoRetryScopeNetworkOnly,
		AutoRetryPreset:  AutoRetryPresetGentleStop,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", created.ID).
		Updates(map[string]any{
			"status":                     string(StatusError),
			"last_error":                 "This request has been flagged for possible cybersecurity risk.",
			"auto_retry_last_error_code": codexCyberPolicyErrorCode,
			"auto_retry_next_at":         nil,
		}).Error; err != nil {
		t.Fatalf("failed to seed terminal retry failure: %v", err)
	}

	if _, err := manager.queuePendingInput(
		created.ID,
		"next",
		nil,
		PendingInputModeRedirect,
		"next-1",
	); err != nil {
		t.Fatalf("queuePendingInput returned error: %v", err)
	}
	time.Sleep(75 * time.Millisecond)
	if events, readErr := manager.store.readEvents(created.ID); readErr != nil {
		t.Fatalf("readEvents returned error: %v", readErr)
	} else if got := userMessageTexts(events); len(got) != 0 {
		t.Fatalf("expected terminal failure to hold pending input, got %#v", got)
	}
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 1 {
		t.Fatalf("expected pending input to remain queued, got %#v", pending)
	}

	if _, err := manager.UpdateAutoRetryDispatchPendingOnFailure(
		context.Background(),
		created.ID,
		true,
	); err != nil {
		t.Fatalf("UpdateAutoRetryDispatchPendingOnFailure returned error: %v", err)
	}
	waitForUserMessageCount(t, manager, created.ID, 1)
	if events, readErr := manager.store.readEvents(created.ID); readErr != nil {
		t.Fatalf("readEvents returned error after enabling pending dispatch: %v", readErr)
	} else if got := strings.Join(userMessageTexts(events), "|"); got != "next" {
		t.Fatalf("expected pending input to dispatch after enabling, got %q", got)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestAutoRetryEnabledSessionRetriesModelCapacityFailure(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "model_capacity_then_success"),
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

	deadline := time.Now().Add(7 * time.Second)
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
		t.Fatalf("expected session status %q after model capacity retry, got %q", StatusDone, record.Status)
	}

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := userMessageTexts(events); len(got) < 2 || got[len(got)-1] != "continue" {
		t.Fatalf("expected model capacity retry to append continue, got %#v", got)
	}
	for _, event := range events {
		if event.Type == "run_fail" && stringValue(event.Payload["code"]) == codexModelCapacityErrorCode {
			return
		}
	}
	t.Fatalf("expected structured model capacity run_fail event, got %#v", events)
}

func TestAutoRetryDisabledSessionDoesNotRetryModelCapacityFailure(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "model_capacity_then_success"),
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
	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusError) {
		t.Fatalf("expected disabled auto retry to leave status %q, got %q", StatusError, record.Status)
	}
	if record.AutoRetryAttempt != 0 || record.AutoRetryNextAt != nil {
		t.Fatalf("expected no scheduled retry, got attempt=%d next=%v", record.AutoRetryAttempt, record.AutoRetryNextAt)
	}

	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := userMessageTexts(events); len(got) != 1 || got[0] != "inspect" {
		t.Fatalf("expected no automatic continue, got %#v", got)
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

func TestSendMessageWithModeRedirectSteersActiveCodexTurn(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	codexPath := writeFakeCodexAppServerCLI(t, "step_redirect")
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: codexPath,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	manager.pendingSteerDelay = 150 * time.Millisecond

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

	steerStatePath := codexPath + ".state.steer.json"
	pending := manager.pendingInputsSnapshot(created.ID)
	if len(pending) != 2 || pending[0].Text != "redirected" ||
		pending[0].Mode != PendingInputModeRedirect || pending[0].ReadyAt == nil || pending[0].Paused ||
		pending[1].Text != "queued" || pending[1].Mode != PendingInputModeQueue {
		t.Fatalf("expected delayed redirect before queued input, got %#v", pending)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := os.Stat(steerStatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no steer before undo window, stat error=%v", err)
	}
	waitForFile(t, steerStatePath)
	steerState, err := os.ReadFile(steerStatePath)
	if err != nil {
		t.Fatalf("read steer state: %v", err)
	}
	var steerRequest map[string]any
	if err := json.Unmarshal(steerState, &steerRequest); err != nil {
		t.Fatalf("decode steer state: %v", err)
	}
	if stringValue(steerRequest["threadId"]) != "thread_test" ||
		stringValue(steerRequest["expectedTurnId"]) != "turn_test" {
		t.Fatalf("unexpected steer target: %#v", steerRequest)
	}
	inputs := decodeRawArray(steerRequest["input"])
	if len(inputs) != 1 || stringValue(inputs[0]["text"]) != "redirected" {
		t.Fatalf("unexpected steer input: %#v", steerRequest)
	}

	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 1 ||
		pending[0].Text != "queued" || pending[0].Mode != PendingInputModeQueue {
		t.Fatalf("expected only the queued message to remain pending, got %#v", pending)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error while checking steer behavior: %v", err)
	}
	if got := userMessageTexts(rawEvents); strings.Join(got, "|") != "first|redirected" {
		t.Fatalf("expected steer message in the active turn, got %#v", got)
	}
	if historyHasEvent(rawEvents, "run_abort") {
		t.Fatalf("expected steer to keep the active run alive, got %#v", rawEvents)
	}
	if !manager.hasActiveRun(created.ID) {
		t.Fatal("expected active Codex run to remain alive after steer")
	}

	if err := os.WriteFile(codexPath+".state.release-steer", []byte("1"), 0o644); err != nil {
		t.Fatalf("release steered turn: %v", err)
	}

	waitForUserMessageCount(t, manager, created.ID, 3)

	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := userMessageTexts(rawEvents); strings.Join(got, "|") != "first|redirected|queued" {
		t.Fatalf("expected queued message to flush after the steered turn, got %#v", got)
	}
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected pending inputs to be empty after queue flush, got %#v", pending)
	}

	if err := manager.AbortSession(created.ID); err != nil {
		t.Fatalf("AbortSession returned error while cleaning up: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestPendingCodexRedirectPauseAndEditRestartsUndoWindow(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	codexPath := writeFakeCodexAppServerCLI(t, "step_redirect")
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: codexPath,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	manager.pendingSteerDelay = 200 * time.Millisecond

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
	t.Cleanup(func() {
		if manager.hasActiveRun(created.ID) {
			_ = manager.AbortSession(created.ID)
		}
	})
	waitForTrackedActiveCallID(t, manager, created.ID, "cmd_step_2")

	if err := manager.sendMessageWithMode(
		context.Background(),
		created.ID,
		"original redirect",
		nil,
		PendingInputModeRedirect,
		"pending-edit",
	); err != nil {
		t.Fatalf("sendMessageWithMode returned error: %v", err)
	}

	paused := true
	updated, err := manager.updatePendingInput(created.ID, "pending-edit", pendingInputUpdate{
		Paused: &paused,
	})
	if err != nil {
		t.Fatalf("pause pending input: %v", err)
	}
	if !updated.Paused || updated.ReadyAt != nil {
		t.Fatalf("expected paused input without a deadline, got %#v", updated)
	}
	manager.triggerPendingProcessing(created.ID)

	steerStatePath := codexPath + ".state.steer.json"
	time.Sleep(250 * time.Millisecond)
	if _, err := os.Stat(steerStatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected paused input not to steer after its original deadline, stat error=%v", err)
	}

	updatedText := "edited redirect"
	paused = false
	resumedAt := time.Now()
	updated, err = manager.updatePendingInput(created.ID, "pending-edit", pendingInputUpdate{
		Text:   &updatedText,
		Paused: &paused,
	})
	if err != nil {
		t.Fatalf("save pending edit: %v", err)
	}
	if updated.Paused || updated.ReadyAt == nil || updated.ReadyAt.Before(resumedAt.Add(150*time.Millisecond)) {
		t.Fatalf("expected save to start a fresh undo window, got %#v", updated)
	}
	manager.triggerPendingProcessing(created.ID)
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(steerStatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected edited input not to steer before its fresh deadline, stat error=%v", err)
	}

	waitForFile(t, steerStatePath)
	steerState, err := os.ReadFile(steerStatePath)
	if err != nil {
		t.Fatalf("read steer state: %v", err)
	}
	var steerRequest map[string]any
	if err := json.Unmarshal(steerState, &steerRequest); err != nil {
		t.Fatalf("decode steer state: %v", err)
	}
	inputs := decodeRawArray(steerRequest["input"])
	if len(inputs) != 1 || stringValue(inputs[0]["text"]) != updatedText {
		t.Fatalf("expected edited text in steer request, got %#v", steerRequest)
	}

	if err := os.WriteFile(codexPath+".state.release-steer", []byte("1"), 0o644); err != nil {
		t.Fatalf("release steered turn: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestPendingCodexRedirectCanBeCanceledDuringUndoWindow(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	codexPath := writeFakeCodexAppServerCLI(t, "step_redirect")
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: codexPath,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	manager.pendingSteerDelay = 150 * time.Millisecond

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
	t.Cleanup(func() {
		if manager.hasActiveRun(created.ID) {
			_ = manager.AbortSession(created.ID)
		}
	})
	waitForTrackedActiveCallID(t, manager, created.ID, "cmd_step_2")

	if err := manager.sendMessageWithMode(
		context.Background(),
		created.ID,
		"cancel this redirect",
		nil,
		PendingInputModeRedirect,
		"pending-cancel",
	); err != nil {
		t.Fatalf("sendMessageWithMode returned error: %v", err)
	}
	if !manager.removePendingInput(created.ID, "pending-cancel") {
		t.Fatal("expected pending redirect to be removed")
	}

	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(codexPath + ".state.steer.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected canceled input never to steer, stat error=%v", err)
	}
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected canceled input to leave no pending state, got %#v", pending)
	}
}

func TestPendingCodexRedirectSteersWhenQueueModeChanges(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	codexPath := writeFakeCodexAppServerCLI(t, "step_redirect")
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: codexPath,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	manager.pendingSteerDelay = 120 * time.Millisecond

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
	t.Cleanup(func() {
		if manager.hasActiveRun(created.ID) {
			_ = manager.AbortSession(created.ID)
		}
	})
	waitForTrackedActiveCallID(t, manager, created.ID, "cmd_step_2")

	queued, err := manager.queuePendingInput(
		created.ID,
		"changed to redirect",
		nil,
		PendingInputModeQueue,
		"pending-change-mode",
	)
	if err != nil {
		t.Fatalf("queuePendingInput returned error: %v", err)
	}
	if err := manager.reorderPendingInput(
		created.ID,
		queued.ID,
		PendingInputModeRedirect,
		0,
	); err != nil {
		t.Fatalf("reorderPendingInput returned error: %v", err)
	}
	manager.triggerPendingProcessing(created.ID)

	steerStatePath := codexPath + ".state.steer.json"
	pending := manager.pendingInputsSnapshot(created.ID)
	if len(pending) != 1 || pending[0].ReadyAt == nil || pending[0].Paused {
		t.Fatalf("expected promoted redirect to receive an undo window, got %#v", pending)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := os.Stat(steerStatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected promoted redirect not to steer immediately, stat error=%v", err)
	}
	waitForFile(t, steerStatePath)
	waitForUserMessageCount(t, manager, created.ID, 2)
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected converted redirect to leave the pending queue, got %#v", pending)
	}
	events, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(events), "|"); got != "first|changed to redirect" {
		t.Fatalf("expected converted redirect to steer the active turn, got %q", got)
	}
	if historyHasEvent(events, "run_abort") {
		t.Fatalf("expected converted redirect not to abort the active run, got %#v", events)
	}

	if err := os.WriteFile(codexPath+".state.release-steer", []byte("1"), 0o644); err != nil {
		t.Fatalf("release steered turn: %v", err)
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

func TestHandleCodexAppServerUsageUsesLatestSnapshotForContextEstimate(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Latest Usage Snapshot", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	run := &activeRun{
		sessionID:          session.ID,
		runID:              "run_latest_snapshot",
		assistantDeltaSeen: make(map[string]bool),
	}
	manager.handleCodexAppServerUsage(
		*session,
		run,
		[]byte(`{"tokenUsage":{"total":{"inputTokens":4380698,"cachedInputTokens":4032512,"outputTokens":96629},"last":{"inputTokens":59571,"cachedInputTokens":59000,"outputTokens":199,"totalTokens":12656},"modelContextWindow":512000}}`),
	)

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	summary := manager.mapSessionSummary(record)
	if summary.ContextEstimateMode != ContextEstimateModeLatestTokenCount {
		t.Fatalf("expected context estimate mode %q, got %q", ContextEstimateModeLatestTokenCount, summary.ContextEstimateMode)
	}
	if summary.ContextEstimate.InputTokens != 59571 ||
		summary.ContextEstimate.CachedInputTokens != 59000 ||
		summary.ContextEstimate.OutputTokens != 199 ||
		summary.ContextEstimate.UsedTokens != 12656 {
		t.Fatalf("unexpected context estimate: %#v", summary.ContextEstimate)
	}
	if summary.Usage.InputTokens != 4380698 ||
		summary.Usage.CachedInputTokens != 4032512 ||
		summary.Usage.OutputTokens != 96629 {
		t.Fatalf("unexpected cumulative usage: %#v", summary.Usage)
	}
	if summary.ContextWindowTokens == nil || *summary.ContextWindowTokens != 512000 {
		t.Fatalf("expected session context window 512000, got %#v", summary.ContextWindowTokens)
	}
	if summary.ContextWindowSource != ContextWindowSourceSessionUsage {
		t.Fatalf("expected context window source %q, got %q", ContextWindowSourceSessionUsage, summary.ContextWindowSource)
	}
}

func TestHandleCodexAppServerUsageClearsLatestSnapshotWhenMissingLast(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Missing Last Snapshot", 1000)
	now := time.Now()
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"latest_token_count_input_tokens":        100,
			"latest_token_count_cached_input_tokens": 20,
			"latest_token_count_output_tokens":       5,
			"latest_token_count_total_tokens":        120,
			"latest_token_count_updated_at":          now,
		}).Error; err != nil {
		t.Fatalf("failed to seed latest token count: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	run := &activeRun{
		sessionID:          session.ID,
		runID:              "run_missing_last",
		assistantDeltaSeen: make(map[string]bool),
	}
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
	if record.LatestTokenCountUpdatedAt != nil ||
		record.LatestTokenCountInputTokens != 0 ||
		record.LatestTokenCountCachedInputTokens != 0 ||
		record.LatestTokenCountOutputTokens != 0 ||
		record.LatestTokenCountTotalTokens != 0 {
		t.Fatalf("expected latest token count fields to be cleared, got %#v", record)
	}
	if summary.ContextEstimateMode != ContextEstimateModeCumulativeTotal {
		t.Fatalf("expected fallback estimate mode %q, got %q", ContextEstimateModeCumulativeTotal, summary.ContextEstimateMode)
	}
	if summary.ContextEstimate.UsedTokens != 162 {
		t.Fatalf("expected cumulative usedTokens 162, got %d", summary.ContextEstimate.UsedTokens)
	}
}

func TestBuildContextEstimateUsesLatestTokenCountSnapshot(t *testing.T) {
	now := time.Now()
	record := tables.WebSessionTable{
		TotalInputTokens:                  999,
		TotalCachedInputTokens:            111,
		TotalOutputTokens:                 88,
		LatestTokenCountInputTokens:       200,
		LatestTokenCountCachedInputTokens: 190,
		LatestTokenCountOutputTokens:      50,
		LatestTokenCountTotalTokens:       12656,
		LatestTokenCountUpdatedAt:         &now,
		LastContextCompactionAt:           &now,
		ContextBaselineInputTokens:        900,
		ContextBaselineCachedInputTokens:  100,
		ContextBaselineOutputTokens:       80,
	}

	estimate, mode := buildContextEstimate(record)
	if mode != ContextEstimateModeLatestTokenCount {
		t.Fatalf("expected context estimate mode %q, got %q", ContextEstimateModeLatestTokenCount, mode)
	}
	if estimate.UsedTokens != 12656 {
		t.Fatalf("expected usedTokens from latest token_count total, got %d", estimate.UsedTokens)
	}
	if estimate.InputTokens != 200 || estimate.CachedInputTokens != 190 || estimate.OutputTokens != 50 {
		t.Fatalf("unexpected token_count breakdown, got %#v", estimate)
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

	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "fake-codex-version.cmd")
		script := fmt.Sprintf(`@echo off
if "%%~1"=="--version" (
  echo codex %s
  exit /b 0
)
exit /b 1
`, version)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake codex version cli failed: %v", err)
		}
		return path
	}

	path := filepath.Join(dir, "fake-codex-version.sh")
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

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-codex-model-catalog.js")
	script := `#!/usr/bin/env node
if (process.argv.includes('--version')) {
  process.stdout.write('codex 0.146.0\n');
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
	if runtime.GOOS == "windows" {
		cmdPath := filepath.Join(dir, "fake-codex-model-catalog.cmd")
		wrapper := "@echo off\r\nnode \"%~dp0fake-codex-model-catalog.js\" %*\r\nexit /b %ERRORLEVEL%\r\n"
		if err := os.WriteFile(cmdPath, []byte(wrapper), 0o755); err != nil {
			t.Fatalf("write fake Codex model catalog wrapper failed: %v", err)
		}
		return cmdPath
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

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-codex-app-server.js")
	rolloutPath := filepath.Join(dir, "fake-codex-rollout.jsonl")
	script := fmt.Sprintf(`#!/usr/bin/env node
if (process.argv.includes('--version')) {
  process.stdout.write('codex 0.146.0\n');
  process.exit(0);
}
const readline = require('readline');
const fs = require('fs');

const mode = %q;
const rolloutPath = %q;
const threadId = 'thread_test';
const turnId = 'turn_test';
const childThreadId = 'thread_child';
const childTurnId = 'turn_child';
const stateFile = (process.env.CODEKANBAN_FAKE_CODEX_PATH || __filename) + '.state';
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
	const responseThreadId = explicitThreadId || threadId;
	fs.writeFileSync(
		rolloutPath,
		JSON.stringify({
			timestamp: new Date().toISOString(),
			type: 'session_meta',
			payload: { id: responseThreadId },
		}) + '\n',
	);
  send({
    id,
    result: {
      modelProvider: 'TestProvider',
      thread: { id: responseThreadId, path: rolloutPath },
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

function finishTurn(text, startedPhase = 'final_answer', completedPhase = 'final_answer') {
  emitReasoning();
  if (mode === 'plan') {
    emitPlan();
  }
  send({
    method: 'item/started',
    params: {
      item: { type: 'agentMessage', id: 'msg_test', text: '', phase: startedPhase, memoryCitation: null },
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
      item: { type: 'agentMessage', id: 'msg_test', text, phase: completedPhase, memoryCitation: null },
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

function finishIncompleteCommentary(text) {
  send({
    method: 'item/started',
    params: {
      item: { type: 'agentMessage', id: 'msg_commentary_' + startedTurns, text: '', phase: 'commentary' },
      threadId: activeThreadId,
      turnId,
    },
  });
  send({
    method: 'item/agentMessage/delta',
    params: {
      threadId: activeThreadId,
      turnId,
      itemId: 'msg_commentary_' + startedTurns,
      delta: text,
    },
  });
  send({
    method: 'item/completed',
    params: {
      item: {
        type: 'agentMessage',
        id: 'msg_commentary_' + startedTurns,
        text,
        phase: 'commentary',
      },
      threadId: activeThreadId,
      turnId,
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

function finishIncompleteToolOnly() {
  send({
    method: 'item/started',
    params: {
      item: { type: 'commandExecution', id: 'cmd_incomplete', command: 'apply patch' },
      threadId: activeThreadId,
      turnId,
    },
  });
  emitCommandExecutionCompleted('cmd_incomplete', 'apply patch');
  send({
    method: 'turn/completed',
    params: {
      threadId: activeThreadId,
      turn: { id: turnId, items: [], status: 'completed', error: null },
    },
  });
}

function finishPlanOnly() {
  emitPlan();
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
let steered = false;
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
    if (
      mode === 'verify_yolo' &&
      message.method === 'thread/start' &&
      (!message.params ||
        message.params.approvalPolicy !== 'never' ||
        message.params.sandbox !== 'danger-full-access')
    ) {
      send({
        id: message.id,
        error: { message: 'expected yolo approvalPolicy=never and sandbox=danger-full-access' },
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

  if (message.method === 'turn/steer') {
    fs.writeFileSync(stateFile + '.steer.json', JSON.stringify(message.params || {}));
    send({ id: message.id, result: { turnId } });
    if (mode === 'step_redirect') {
      steered = true;
      emitCommandExecutionCompleted('cmd_step_2', 'step-2');
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
      const releasePath = stateFile + '.release-steer';
      const releaseTimer = setInterval(() => {
        if (!fs.existsSync(releasePath)) return;
        clearInterval(releaseTimer);
        emitCommandExecutionCompleted('cmd_step_3', 'step-3');
        finishTurn('steered');
      }, 10);
    }
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

    if (mode === 'incomplete_commentary_then_success') {
      if (startedTurns === 1) {
        finishIncompleteCommentary('I will inspect the repository next.');
        return;
      }
      finishTurn('continued-final');
      return;
    }

    if (mode === 'incomplete_inherited_commentary_then_success') {
      if (startedTurns === 1) {
        finishTurn('Still working.', 'commentary', null);
        return;
      }
      finishTurn('continued-final');
      return;
    }

    if (mode === 'incomplete_empty_unknown_then_success') {
      if (startedTurns === 1) {
        finishTurn('', null, null);
        return;
      }
      finishTurn('continued-final');
      return;
    }

    if (mode === 'incomplete_tool_then_success') {
      if (startedTurns === 1) {
        finishIncompleteToolOnly();
        return;
      }
      finishTurn('continued-final');
      return;
    }

    if (mode === 'incomplete_twice') {
      finishIncompleteCommentary('Still working.');
      return;
    }

    if (mode === 'completed_without_phase') {
      finishTurn('compatible-final', null, null);
      return;
    }

    if (mode === 'completed_with_inherited_final_phase') {
      finishTurn('compatible-final', 'final_answer', null);
      return;
    }

    if (mode === 'completed_with_unknown_phase') {
      finishTurn('compatible-final', 'future_phase', 'future_phase');
      return;
    }

    if (mode === 'plan_only') {
      finishPlanOnly();
      return;
    }

    if (mode === 'basic' || mode === 'resume_only' || mode === 'plan' || mode === 'verify_yolo') {
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

    if (mode === 'model_capacity_then_success') {
      const persistedTurns = readPersistentTurnCount() + 1;
      writePersistentTurnCount(persistedTurns);
      if (persistedTurns === 1) {
        failTurn('Selected model is at capacity. Please try a different model.');
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
          if (steered) return;
          emitCommandExecutionCompleted('cmd_step_2', 'step-2');
          setTimeout(() => {
            if (steered) return;
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
`, mode, filepath.ToSlash(rolloutPath))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex app-server cli failed: %v", err)
	}
	if runtime.GOOS == "windows" {
		cmdPath := filepath.Join(dir, "fake-codex-app-server.cmd")
		wrapper := "@echo off\r\nset \"CODEKANBAN_FAKE_CODEX_PATH=%~f0\"\r\nnode \"%~dp0fake-codex-app-server.js\" %*\r\nexit /b %ERRORLEVEL%\r\n"
		if err := os.WriteFile(cmdPath, []byte(wrapper), 0o755); err != nil {
			t.Fatalf("write fake codex app-server wrapper failed: %v", err)
		}
		return cmdPath
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
