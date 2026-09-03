package websession

import (
	"context"
	"encoding/json"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestHandlePiExtensionUIRequestIgnoresStatusAndKeepsNotify(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentPi,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	dispatch := &piRuntimeRun{
		run:     &activeRun{runID: "run-pi-extension-ui"},
		session: record,
	}

	statusRequest := json.RawMessage(`{"method":"setStatus","statusKey":"magic-context","statusText":"mc: 147.5K (72%) - idle"}`)
	if err := manager.handlePiExtensionUIRequest(dispatch, statusRequest); err != nil {
		t.Fatalf("handle setStatus request: %v", err)
	}
	window, err := manager.loadHistoryWindow(context.Background(), created.ID, 10, nil)
	if err != nil {
		t.Fatalf("load history after setStatus: %v", err)
	}
	if len(window.Items) != 0 {
		t.Fatalf("setStatus created timeline items: %#v", window.Items)
	}
	statusRecord, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession after setStatus: %v", err)
	}
	if statusRecord.LastEventSeq != 0 || statusRecord.ItemCount != 0 {
		t.Fatalf("setStatus changed history metadata: seq=%d items=%d", statusRecord.LastEventSeq, statusRecord.ItemCount)
	}

	notifyRequest := json.RawMessage(`{"id":"notify-1","method":"notify","message":"Extension warning","notifyType":"warning"}`)
	if err := manager.handlePiExtensionUIRequest(dispatch, notifyRequest); err != nil {
		t.Fatalf("handle notify request: %v", err)
	}
	window, err = manager.loadHistoryWindow(context.Background(), created.ID, 10, nil)
	if err != nil {
		t.Fatalf("load history after notify: %v", err)
	}
	if len(window.Items) != 1 {
		t.Fatalf("notify history item count = %d, want 1", len(window.Items))
	}
	item := window.Items[0]
	if item.ItemType != "note" || item.Text != "Extension warning" || item.Level != "warning" {
		t.Fatalf("unexpected notify history item: %#v", item)
	}
	if code := stringValue(item.Payload["code"]); code != "pi_extension_ui_notify" {
		t.Fatalf("notify code = %q, want pi_extension_ui_notify", code)
	}
}

func TestNewManagerCleansLegacyPiStatusNotes(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	piSession := seedWebSessionWithAgent(t, project.ID, "Pi legacy statuses", 1000, AgentPi)
	codexSession := seedWebSessionWithAgent(t, project.ID, "Codex control", 2000, AgentCodex)

	piStatus := seedLegacyStatusTestItem(t, piSession.ID, "pi-status", 1, "note", `{"code":"pi_extension_ui_setStatus"}`)
	seedLegacyStatusTestItem(t, piSession.ID, "pi-notify", 2, "note", `{"code":"pi_extension_ui_notify"}`)
	seedLegacyStatusTestItem(t, piSession.ID, "pi-invalid-json", 3, "note", `{invalid`)
	piNonNote := seedLegacyStatusTestItem(t, piSession.ID, "pi-non-note", 4, "tool", `{"code":"pi_extension_ui_setStatus"}`)
	codexStatus := seedLegacyStatusTestItem(t, codexSession.ID, "codex-status", 1, "note", `{"code":"pi_extension_ui_setStatus"}`)

	db := model.GetDB()
	if err := db.Model(&tables.WebSessionTable{}).
		Where("id = ?", piSession.ID).
		UpdateColumns(map[string]any{"item_count": 4, "history_epoch": 3, "snapshot_revision": 7}).Error; err != nil {
		t.Fatalf("seed Pi history metadata: %v", err)
	}
	if err := db.Model(&tables.WebSessionTable{}).
		Where("id = ?", codexSession.ID).
		UpdateColumns(map[string]any{"item_count": 1, "history_epoch": 5, "snapshot_revision": 9}).Error; err != nil {
		t.Fatalf("seed Codex history metadata: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	assertHistoryItemExists(t, piStatus.ID, false)
	assertHistoryItemExists(t, piNonNote.ID, true)
	assertHistoryItemExists(t, codexStatus.ID, true)

	refreshedPi, err := manager.GetSession(context.Background(), piSession.ID)
	if err != nil {
		t.Fatalf("GetSession Pi: %v", err)
	}
	if refreshedPi.ItemCount != 3 || refreshedPi.HistoryEpoch != 4 || refreshedPi.SnapshotRevision != 8 {
		t.Fatalf("unexpected Pi metadata after cleanup: items=%d epoch=%d revision=%d", refreshedPi.ItemCount, refreshedPi.HistoryEpoch, refreshedPi.SnapshotRevision)
	}
	refreshedCodex, err := manager.GetSession(context.Background(), codexSession.ID)
	if err != nil {
		t.Fatalf("GetSession Codex: %v", err)
	}
	if refreshedCodex.ItemCount != 1 || refreshedCodex.HistoryEpoch != 5 || refreshedCodex.SnapshotRevision != 9 {
		t.Fatalf("Codex metadata changed: items=%d epoch=%d revision=%d", refreshedCodex.ItemCount, refreshedCodex.HistoryEpoch, refreshedCodex.SnapshotRevision)
	}

	removed, err := manager.cleanupLegacyPiStatusNotes(context.Background())
	if err != nil {
		t.Fatalf("repeat cleanup: %v", err)
	}
	if removed != 0 {
		t.Fatalf("repeat cleanup removed %d rows, want 0", removed)
	}
	afterRepeat, err := manager.GetSession(context.Background(), piSession.ID)
	if err != nil {
		t.Fatalf("GetSession after repeat cleanup: %v", err)
	}
	if afterRepeat.HistoryEpoch != 4 || afterRepeat.SnapshotRevision != 8 {
		t.Fatalf("idempotent cleanup changed metadata: epoch=%d revision=%d", afterRepeat.HistoryEpoch, afterRepeat.SnapshotRevision)
	}
}

func seedLegacyStatusTestItem(
	t *testing.T,
	sessionID string,
	id string,
	orderIndex int64,
	itemType string,
	payloadJSON string,
) *tables.WebSessionItemTable {
	t.Helper()
	item := &tables.WebSessionItemTable{
		WebSessionID: sessionID,
		OrderIndex:   orderIndex,
		LastEventSeq: orderIndex,
		ItemKind:     "system",
		ItemType:     itemType,
		Level:        "info",
		Text:         "legacy status",
		PayloadJSON:  payloadJSON,
	}
	item.Init()
	item.ID = id
	if err := model.GetDB().Create(item).Error; err != nil {
		t.Fatalf("seed history item %q: %v", id, err)
	}
	return item
}

func assertHistoryItemExists(t *testing.T, itemID string, want bool) {
	t.Helper()
	var count int64
	if err := model.GetDB().Unscoped().Model(&tables.WebSessionItemTable{}).
		Where("id = ?", itemID).
		Count(&count).Error; err != nil {
		t.Fatalf("count history item %q: %v", itemID, err)
	}
	if got := count == 1; got != want {
		t.Fatalf("history item %q exists = %v, want %v", itemID, got, want)
	}
}
