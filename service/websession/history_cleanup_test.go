package websession

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestRunHistoryCleanupHonorsProjectScopeAndRetention(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	projectA := seedProject(t)
	projectB := seedProject(t)
	now := time.Now()
	newestA := seedHistoryCleanupSession(t, projectA.ID, "Newest A", now.Add(-time.Hour), true)
	oldA := seedHistoryCleanupSession(t, projectA.ID, "Old A", now.Add(-60*24*time.Hour), true)
	oldB := seedHistoryCleanupSession(t, projectB.ID, "Old B", now.Add(-90*24*time.Hour), true)

	obsoleteItem := seedHistoryCleanupItem(t, newestA.ID, "obsolete")
	if err := model.GetDB().Delete(obsoleteItem).Error; err != nil {
		t.Fatalf("soft delete obsolete item: %v", err)
	}
	seedHistoryCleanupTurn(t, oldA.ID)
	seedHistoryCleanupTurn(t, oldB.ID)

	dataDir := t.TempDir()
	manager, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := manager.store.appendEvent(oldA.ID, Event{ID: "event-old-a", Seq: 1, Type: "note", Timestamp: now}); err != nil {
		t.Fatalf("append old A history: %v", err)
	}

	params := HistoryCleanupParams{
		Scope:            HistoryCleanupScopeProjects,
		ProjectIDs:       []string{projectA.ID},
		OlderThanDays:    30,
		RetainPerProject: 1,
	}
	preview, err := manager.PreviewHistoryCleanup(context.Background(), params)
	if err != nil {
		t.Fatalf("PreviewHistoryCleanup: %v", err)
	}
	if preview.HistorySessionCount != 1 {
		t.Fatalf("history session count = %d, want 1", preview.HistorySessionCount)
	}
	if preview.ObsoleteItemRowCount != 1 {
		t.Fatalf("obsolete item count = %d, want 1", preview.ObsoleteItemRowCount)
	}
	if preview.NonSyncableSessionCount != 0 {
		t.Fatalf("non-syncable count = %d, want 0", preview.NonSyncableSessionCount)
	}

	result, err := manager.RunHistoryCleanup(context.Background(), params)
	if err != nil {
		t.Fatalf("RunHistoryCleanup: %v", err)
	}
	if len(result.ClearedSessionIDs) != 1 || result.ClearedSessionIDs[0] != oldA.ID {
		t.Fatalf("cleared session IDs = %#v, want [%s]", result.ClearedSessionIDs, oldA.ID)
	}

	assertHistoryCleanupRowCount(t, &tables.WebSessionItemTable{}, oldA.ID, 0)
	assertHistoryCleanupRowCount(t, &tables.WebSessionTurnTable{}, oldA.ID, 0)
	assertHistoryCleanupRowCount(t, &tables.WebSessionItemTable{}, oldB.ID, 1)
	assertHistoryCleanupRowCount(t, &tables.WebSessionTurnTable{}, oldB.ID, 1)
	assertHistoryCleanupRowCount(t, &tables.WebSessionItemTable{}, newestA.ID, 1)

	var refreshed tables.WebSessionTable
	if err := model.GetDB().First(&refreshed, "id = ?", oldA.ID).Error; err != nil {
		t.Fatalf("load retained session metadata: %v", err)
	}
	if refreshed.ItemCount != 0 || refreshed.TurnCount != 0 || refreshed.LastEventSeq != 1 {
		t.Fatalf("history metadata was not reset: %+v", refreshed)
	}
	if normalizeSyncState(refreshed.SyncState) != SyncStateMissing {
		t.Fatalf("sync state = %q, want missing", refreshed.SyncState)
	}
	if refreshed.NativeSessionID == nil || *refreshed.NativeSessionID == "" {
		t.Fatalf("native session ID should be preserved")
	}
	if _, err := os.Stat(manager.store.historyPath(oldA.ID)); !os.IsNotExist(err) {
		t.Fatalf("history file should be removed, stat error = %v", err)
	}
	appended, err := manager.appendAndBroadcast(context.Background(), oldA.ID, refreshed, Event{
		ID:        "event-after-cleanup",
		Type:      "note",
		Timestamp: now.Add(time.Minute),
		Payload:   map[string]any{"txt": "after cleanup"},
	})
	if err != nil {
		t.Fatalf("append after cleanup: %v", err)
	}
	if appended.Seq != 2 {
		t.Fatalf("event sequence after cleanup = %d, want 2", appended.Seq)
	}
}

func TestRunHistoryCleanupWaitsForSessionDispatch(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedHistoryCleanupSession(t, project.ID, "Dispatch protected", time.Now().Add(-48*time.Hour), true)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	dispatchLock := &manager.sessionDispatchLocks[sessionRevisionLockIndex(session.ID)]
	dispatchLock.Lock()
	done := make(chan error, 1)
	go func() {
		_, runErr := manager.RunHistoryCleanup(context.Background(), HistoryCleanupParams{
			Scope:         HistoryCleanupScopeAll,
			OlderThanDays: 1,
		})
		done <- runErr
	}()
	select {
	case runErr := <-done:
		dispatchLock.Unlock()
		t.Fatalf("RunHistoryCleanup bypassed the session dispatch lock: %v", runErr)
	case <-time.After(50 * time.Millisecond):
	}
	dispatchLock.Unlock()
	if runErr := <-done; runErr != nil {
		t.Fatalf("RunHistoryCleanup returned error after dispatch release: %v", runErr)
	}
}

func TestHistoryCleanupConsumesRetainedEventLog(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedHistoryCleanupSession(t, project.ID, "Retained cleanup log", time.Now().Add(-48*time.Hour), true)
	dataDir := t.TempDir()
	manager, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	oldEvent := Event{ID: "evt-old", Seq: 1, Type: "note", Timestamp: time.Now().Add(-time.Hour), Payload: map[string]any{"txt": "old"}}
	if err := manager.store.appendEvent(session.ID, oldEvent); err != nil {
		t.Fatalf("append old event: %v", err)
	}
	if _, err := manager.RunHistoryCleanup(context.Background(), HistoryCleanupParams{
		Scope:         HistoryCleanupScopeAll,
		OlderThanDays: 1,
	}); err != nil {
		t.Fatalf("RunHistoryCleanup returned error: %v", err)
	}
	if err := manager.store.appendEvent(session.ID, oldEvent); err != nil {
		t.Fatalf("restore retained event log: %v", err)
	}

	restarted, err := NewManager(Config{DataDir: dataDir}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager after cleanup returned error: %v", err)
	}
	window, err := restarted.loadHistoryWindow(context.Background(), session.ID, 10, nil)
	if err != nil {
		t.Fatalf("loadHistoryWindow returned error: %v", err)
	}
	if len(window.Items) != 0 {
		t.Fatalf("cleanup log was reprojected: %#v", window.Items)
	}
	record, err := restarted.GetSession(context.Background(), session.ID)
	if err != nil || record.LastEventSeq != 1 {
		t.Fatalf("cleanup cursor = %d, %v; want 1, nil", record.LastEventSeq, err)
	}
}

func TestPreviewHistoryCleanupSkipsBusyAndScheduledSessions(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	now := time.Now()
	running := seedHistoryCleanupSession(t, project.ID, "Running", now.Add(-60*24*time.Hour), true)
	scheduled := seedHistoryCleanupSession(t, project.ID, "Scheduled", now.Add(-60*24*time.Hour), false)
	scheduledFor := now.Add(time.Hour)
	scheduledInput := &tables.WebSessionScheduledInputTable{
		WebSessionID:    scheduled.ID,
		Action:          string(ScheduledInputActionMessage),
		PayloadJSON:     "{}",
		Mode:            string(ScheduledInputModeSend),
		ScheduleKind:    string(ScheduledInputScheduleAtTime),
		ScheduledFor:    scheduledFor,
		BlockingReasons: "[]",
		Status:          string(ScheduledInputStatusScheduled),
	}
	scheduledInput.Init()
	if err := model.GetDB().Create(scheduledInput).Error; err != nil {
		t.Fatalf("create scheduled input: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	manager.mu.Lock()
	manager.runs[running.ID] = &activeRun{sessionID: running.ID}
	manager.mu.Unlock()
	preview, err := manager.PreviewHistoryCleanup(context.Background(), HistoryCleanupParams{
		Scope:            HistoryCleanupScopeAll,
		OlderThanDays:    0,
		RetainPerProject: 0,
	})
	if err != nil {
		t.Fatalf("PreviewHistoryCleanup: %v", err)
	}
	if preview.HistorySessionCount != 0 || preview.SkippedBusySessionCount != 2 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
}

func TestHistoryCleanupRejectsMissingProjectSelection(t *testing.T) {
	_, err := normalizeHistoryCleanupParams(HistoryCleanupParams{
		Scope:            HistoryCleanupScopeProjects,
		OlderThanDays:    30,
		RetainPerProject: 10,
	})
	if !errors.Is(err, ErrInvalidHistoryCleanup) {
		t.Fatalf("error = %v, want ErrInvalidHistoryCleanup", err)
	}
}

func TestBatchHistoryArchiveAndArchivedCacheCleanupKeepSessionShell(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	old := seedHistoryCleanupSession(t, project.ID, "Old archive candidate", time.Now().Add(-60*24*time.Hour), true)
	newer := seedHistoryCleanupSession(t, project.ID, "Recent archive candidate", time.Now().Add(-time.Hour), true)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	preview, err := manager.PreviewHistoryArchive(context.Background(), HistoryArchiveParams{
		Scope:         HistoryCleanupScopeAll,
		OlderThanDays: 30,
	})
	if err != nil {
		t.Fatalf("PreviewHistoryArchive: %v", err)
	}
	if preview.CandidateSessionCount != 1 {
		t.Fatalf("archive candidates = %d, want 1", preview.CandidateSessionCount)
	}
	archiveResult, err := manager.RunHistoryArchive(context.Background(), HistoryArchiveParams{
		Scope:         HistoryCleanupScopeAll,
		OlderThanDays: 30,
	})
	if err != nil {
		t.Fatalf("RunHistoryArchive: %v", err)
	}
	if len(archiveResult.ArchivedSessionIDs) != 1 || archiveResult.ArchivedSessionIDs[0] != old.ID {
		t.Fatalf("archived session IDs = %#v, want [%s]", archiveResult.ArchivedSessionIDs, old.ID)
	}

	var archived tables.WebSessionTable
	if err := model.GetDB().Unscoped().First(&archived, "id = ?", old.ID).Error; err != nil {
		t.Fatalf("load archived session: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archive timestamp was not set")
	}

	cachePreview, err := manager.PreviewHistoryCleanup(context.Background(), HistoryCleanupParams{
		Scope:                 HistoryCleanupScopeAll,
		ArchivedOnly:          true,
		ArchivedOlderThanDays: 0,
	})
	if err != nil {
		t.Fatalf("Preview archived cache cleanup: %v", err)
	}
	if cachePreview.HistorySessionCount != 1 || cachePreview.ItemRowCount == 0 {
		t.Fatalf("unexpected archived cache preview: %+v", cachePreview)
	}
	if _, err := manager.RunHistoryCleanup(context.Background(), HistoryCleanupParams{
		Scope:                 HistoryCleanupScopeAll,
		ArchivedOnly:          true,
		ArchivedOlderThanDays: 0,
	}); err != nil {
		t.Fatalf("Run archived cache cleanup: %v", err)
	}
	assertHistoryCleanupRowCount(t, &tables.WebSessionItemTable{}, old.ID, 0)
	assertHistoryCleanupRowCount(t, &tables.WebSessionItemTable{}, newer.ID, 1)
	if err := model.GetDB().Unscoped().First(&archived, "id = ?", old.ID).Error; err != nil {
		t.Fatalf("archived session shell was removed: %v", err)
	}
	if archived.ArchivedAt == nil || archived.ItemCount != 0 || archived.TurnCount != 0 {
		t.Fatalf("archived session shell metadata = %+v", archived)
	}
}

func TestHistoryStorageOverviewReportsLogicalCacheBytes(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "history-storage.db")
	if err := model.InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	defer model.DBClose()

	project := seedProject(t)
	session := seedHistoryCleanupSession(t, project.ID, "Storage overview", time.Now(), true)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	overview, err := manager.HistoryStorageOverview(context.Background())
	if err != nil {
		t.Fatalf("HistoryStorageOverview: %v", err)
	}
	if overview.ItemRowCount != 0 || overview.ItemBytes != 0 || overview.HistoryBytes != 0 {
		t.Fatalf("fast overview unexpectedly scanned payloads: %+v", overview)
	}
	details, err := manager.HistoryStorageDetails(context.Background())
	if err != nil {
		t.Fatalf("HistoryStorageDetails: %v", err)
	}
	if details.ItemRowCount == 0 || details.ItemBytes == 0 || details.HistoryBytes == 0 {
		t.Fatalf("details did not report item payload: %+v", details)
	}
	if details.ArchivedCacheBytes != 0 {
		t.Fatalf("unarchived session counted as archived cache: %+v", details)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).Where("id = ?", session.ID).
		Update("archived_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("archive storage overview session: %v", err)
	}
	details, err = manager.HistoryStorageDetails(context.Background())
	if err != nil {
		t.Fatalf("HistoryStorageDetails after archive: %v", err)
	}
	if details.ArchivedSessionCount != 1 || details.ArchivedCacheBytes == 0 {
		t.Fatalf("archived cache was not attributed: %+v", details)
	}

	mainSQLDB, err := model.GetDB().DB()
	if err != nil {
		t.Fatalf("main DB handle: %v", err)
	}
	databasePath, err := historyStorageDatabasePath(context.Background(), model.GetDB())
	if err != nil {
		t.Fatalf("historyStorageDatabasePath: %v", err)
	}
	reader, closeReader, err := openHistoryStorageReader(databasePath)
	if err != nil {
		t.Fatalf("openHistoryStorageReader: %v", err)
	}
	defer closeReader()
	readerSQLDB, err := reader.DB()
	if err != nil {
		t.Fatalf("reader DB handle: %v", err)
	}
	if readerSQLDB == mainSQLDB {
		t.Fatal("storage details reader reused the primary database pool")
	}
}

func TestHistoryCleanupRetriesHistoryFileOnlySessions(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "File Only", 1000)
	if err := model.GetDB().Model(&tables.WebSessionTable{}).Where("id = ?", session.ID).
		Update("activity_at", time.Now().Add(-60*24*time.Hour)).Error; err != nil {
		t.Fatalf("update activity: %v", err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := manager.store.appendEvent(session.ID, Event{ID: "file-only", Seq: 1, Type: "note", Timestamp: time.Now()}); err != nil {
		t.Fatalf("append history file: %v", err)
	}

	params := HistoryCleanupParams{Scope: HistoryCleanupScopeAll}
	preview, err := manager.PreviewHistoryCleanup(context.Background(), params)
	if err != nil {
		t.Fatalf("PreviewHistoryCleanup: %v", err)
	}
	if preview.HistorySessionCount != 1 || preview.ItemRowCount != 0 || preview.TurnRowCount != 0 {
		t.Fatalf("unexpected file-only preview: %+v", preview)
	}
	if _, err := manager.RunHistoryCleanup(context.Background(), params); err != nil {
		t.Fatalf("RunHistoryCleanup: %v", err)
	}
	if _, err := os.Stat(manager.store.historyPath(session.ID)); !os.IsNotExist(err) {
		t.Fatalf("history file should be removed, stat error = %v", err)
	}
}

func TestReplaceSessionHistoryCacheHardDeletesPreviousRows(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedHistoryCleanupSession(t, project.ID, "Replace", time.Now(), true)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	replacement := tables.WebSessionItemTable{
		WebSessionID: session.ID,
		OrderIndex:   2,
		ItemKind:     "assistant",
		ItemType:     "message",
		Text:         "replacement",
	}
	replacement.Init()
	if err := manager.replaceSessionHistoryCache(context.Background(), *session, nil, []tables.WebSessionItemTable{replacement}, nil); err != nil {
		t.Fatalf("replaceSessionHistoryCache: %v", err)
	}
	assertHistoryCleanupRowCount(t, &tables.WebSessionItemTable{}, session.ID, 1)
}

func seedHistoryCleanupSession(t *testing.T, projectID, title string, activityAt time.Time, syncable bool) *tables.WebSessionTable {
	t.Helper()
	session := seedWebSession(t, projectID, title, 1000)
	updates := map[string]any{
		"activity_at":    activityAt,
		"item_count":     1,
		"last_event_seq": 1,
		"sync_state":     string(SyncStateFresh),
	}
	if syncable {
		nativeID := "native-" + session.ID
		updates["native_session_id"] = nativeID
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).Where("id = ?", session.ID).Updates(updates).Error; err != nil {
		t.Fatalf("update cleanup session: %v", err)
	}
	if err := model.GetDB().First(session, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("reload cleanup session: %v", err)
	}
	seedHistoryCleanupItem(t, session.ID, title)
	return session
}

func seedHistoryCleanupItem(t *testing.T, sessionID, text string) *tables.WebSessionItemTable {
	t.Helper()
	item := &tables.WebSessionItemTable{
		WebSessionID: sessionID,
		OrderIndex:   time.Now().UnixNano(),
		ItemKind:     "user",
		ItemType:     "message",
		Text:         text,
	}
	item.Init()
	if err := model.GetDB().Create(item).Error; err != nil {
		t.Fatalf("seed history item: %v", err)
	}
	return item
}

func seedHistoryCleanupTurn(t *testing.T, sessionID string) *tables.WebSessionTurnTable {
	t.Helper()
	turn := &tables.WebSessionTurnTable{
		WebSessionID: sessionID,
		OrderIndex:   time.Now().UnixNano(),
		Status:       "completed",
	}
	turn.Init()
	if err := model.GetDB().Create(turn).Error; err != nil {
		t.Fatalf("seed history turn: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).Where("id = ?", sessionID).Update("turn_count", 1).Error; err != nil {
		t.Fatalf("update turn count: %v", err)
	}
	return turn
}

func assertHistoryCleanupRowCount(t *testing.T, table any, sessionID string, want int64) {
	t.Helper()
	var count int64
	if err := model.GetDB().Unscoped().Model(table).Where("web_session_id = ?", sessionID).Count(&count).Error; err != nil {
		t.Fatalf("count %T rows: %v", table, err)
	}
	if count != want {
		t.Fatalf("%T row count for %s = %d, want %d", table, sessionID, count, want)
	}
}
