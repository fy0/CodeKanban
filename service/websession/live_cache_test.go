package websession

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHistoryAttachmentsFromEventPayloadSupportsMapSlices(t *testing.T) {
	payload := map[string]any{
		"atts": []map[string]any{
			{
				"id":   "att_live",
				"name": "image.png",
				"mime": "image/png",
				"sz":   int64(42),
			},
		},
	}

	got := historyAttachmentsFromEventPayload(payload)
	if len(got) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(got))
	}
	if got[0].ID != "att_live" {
		t.Fatalf("expected attachment id %q, got %q", "att_live", got[0].ID)
	}
	if got[0].Name != "image.png" {
		t.Fatalf("expected attachment name %q, got %q", "image.png", got[0].Name)
	}
	if got[0].Mime != "image/png" {
		t.Fatalf("expected attachment mime %q, got %q", "image/png", got[0].Mime)
	}
	if got[0].Size != 42 {
		t.Fatalf("expected attachment size %d, got %d", 42, got[0].Size)
	}
}

func TestApplyEventToHistoryCacheIncludesUserAttachmentsInRealtimeFrames(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Realtime Attachments", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	attachments := []Attachment{{
		ID:   "att_live",
		Name: "image.png",
		Mime: "image/png",
		Size: 42,
	}}

	item, err := manager.applyEventToHistoryCache(context.Background(), session.ID, Event{
		ID:        "evt_user_live",
		Type:      "msg_u",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"txt":  "hello [Image #1]",
			"atts": attachmentPayloads(attachments),
		},
	})
	if err != nil {
		t.Fatalf("applyEventToHistoryCache returned error: %v", err)
	}
	if item == nil {
		t.Fatal("expected history item, got nil")
	}
	if len(item.Attachments) != 1 {
		t.Fatalf("expected 1 attachment in history item, got %d", len(item.Attachments))
	}
	if item.Attachments[0].ID != attachments[0].ID {
		t.Fatalf("expected attachment id %q, got %q", attachments[0].ID, item.Attachments[0].ID)
	}

	frame := newHistoryItemFrame(session.ID, *item, nil)
	if frame.Item == nil {
		t.Fatal("expected history item wire frame payload")
	}
	if len(frame.Item.Attachments) != 1 {
		t.Fatalf("expected 1 attachment in wire frame, got %d", len(frame.Item.Attachments))
	}
	if frame.Item.Attachments[0].ID != attachments[0].ID {
		t.Fatalf("expected wire attachment id %q, got %q", attachments[0].ID, frame.Item.Attachments[0].ID)
	}
}

func TestCompactSyncedHistoryItemsUseGroupSourceKey(t *testing.T) {
	grouped := compactSyncedHistoryItems([]HistoryItem{
		testCompactHistoryItem("cmd1", "command_execution", "pwd", "pwd", 1),
		testCompactHistoryItem("cmd2", "command_execution", "ls", "ls", 2),
	})
	if len(grouped) != 1 {
		t.Fatalf("expected 1 compacted item, got %d", len(grouped))
	}

	groupID := commandExecutionGroupID("cmd1")
	if grouped[0].SourceItemID == nil || *grouped[0].SourceItemID != historyToolSourceKey(groupID) {
		t.Fatalf("expected compacted source item id %q, got %v", historyToolSourceKey(groupID), grouped[0].SourceItemID)
	}
	if grouped[0].Tool == nil || grouped[0].Tool.CommandGroup == nil || grouped[0].Tool.CommandGroup.ID != groupID {
		t.Fatalf("expected compacted command group %q, got %#v", groupID, grouped[0].Tool)
	}
}

func TestCompactSyncedHistoryItemsGroupsDynamicToolsByName(t *testing.T) {
	items := []HistoryItem{
		{
			ID:         "read-1",
			OrderIndex: 1,
			Kind:       "tool",
			ItemType:   "dynamic_tool_call",
			Tool: &HistoryTool{
				ID:     "read-1",
				Name:   "Read",
				Kind:   "dynamic_tool_call",
				Input:  map[string]any{"file_path": "src/App.vue"},
				Status: "done",
				Meta:   map[string]any{"title": "Read"},
			},
		},
		{
			ID:         "read-2",
			OrderIndex: 2,
			Kind:       "tool",
			ItemType:   "dynamic_tool_call",
			Payload:    map[string]any{},
			Tool: &HistoryTool{
				ID:     "read-2",
				Name:   "Read",
				Kind:   "dynamic_tool_call",
				Input:  map[string]any{"file_path": "src/main.ts"},
				Status: "done",
				Meta:   map[string]any{"title": "Read"},
			},
		},
		{
			ID:         "grep-1",
			OrderIndex: 3,
			Kind:       "tool",
			ItemType:   "dynamic_tool_call",
			Tool: &HistoryTool{
				ID:     "grep-1",
				Name:   "Grep",
				Kind:   "dynamic_tool_call",
				Input:  map[string]any{"pattern": "Goal", "path": "ui/src"},
				Status: "done",
				Meta:   map[string]any{"title": "Grep"},
			},
		},
	}

	grouped := compactSyncedHistoryItems(items)
	if len(grouped) != 2 {
		t.Fatalf("expected Read group plus Grep item, got %d: %#v", len(grouped), grouped)
	}
	if grouped[0].Tool == nil || grouped[0].Tool.Name != "Read" {
		t.Fatalf("expected grouped Read item, got %#v", grouped[0])
	}
	if grouped[0].Tool.CommandGroup == nil || grouped[0].Tool.CommandGroup.Count != 2 {
		t.Fatalf("expected Read command group count 2, got %#v", grouped[0].Tool.CommandGroup)
	}
	details, ok := grouped[0].Payload["groupItems"].([]CommandExecutionGroupItem)
	if !ok || len(details) != 2 {
		t.Fatalf("expected two Read detail items, got %#v", grouped[0].Payload["groupItems"])
	}
	if grouped[1].Tool == nil || grouped[1].Tool.Name != "Grep" || grouped[1].Tool.CommandGroup != nil {
		t.Fatalf("expected ungrouped Grep item, got %#v", grouped[1])
	}
}

func TestApplyEventToHistoryCacheCanonicalizesCommandExecutionGroupRows(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Command Group Canonicalization", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	groupID := commandExecutionGroupID("cmd1")
	first := time.UnixMilli(1_000)
	second := time.UnixMilli(2_000)

	if _, err := manager.appendHistoryItem(context.Background(), session.ID, testGroupedCompactHistoryItem(
		"tool:cmd2",
		1,
		"cmd2",
		groupID,
		"command_execution",
		"ls",
		[]CommandExecutionGroupItem{
			testCompactGroupDetail("cmd1", "command_execution", "pwd", first),
			testCompactGroupDetail("cmd2", "command_execution", "ls", second),
		},
	)); err != nil {
		t.Fatalf("appendHistoryItem returned error: %v", err)
	}
	if _, err := manager.appendHistoryItem(context.Background(), session.ID, testGroupedCompactHistoryItem(
		"tool:cmd3-stale",
		2,
		"cmd3",
		groupID,
		"command_execution",
		"git status",
		[]CommandExecutionGroupItem{
			testCompactGroupDetail("cmd3", "command_execution", "git status", time.UnixMilli(3_000)),
		},
	)); err != nil {
		t.Fatalf("appendHistoryItem returned error: %v", err)
	}

	item, err := manager.applyEventToHistoryCache(context.Background(), session.ID, Event{
		ID:        "evt_cmd3_end",
		Type:      "tool_end",
		Timestamp: time.UnixMilli(3_000),
		Payload: map[string]any{
			"tid":  "cmd3",
			"kind": "command_execution",
			"in":   map[string]any{"command": "git status"},
			"out":  "clean",
			"ok":   true,
			"meta": map[string]any{
				"kind":     "command_execution",
				"title":    "CommandExecution",
				"subtitle": "git status",
				"commandGroup": map[string]any{
					"id":           groupID,
					"count":        3,
					"firstSeq":     1,
					"lastSeq":      6,
					"latestToolId": "cmd3",
					"compacted":    true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("applyEventToHistoryCache returned error: %v", err)
	}
	if item == nil {
		t.Fatal("expected history item, got nil")
	}

	snapshot, err := manager.Snapshot(context.Background(), session.ID, 10)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.History.Items) != 1 {
		t.Fatalf("expected 1 grouped history item, got %d", len(snapshot.History.Items))
	}
	grouped := snapshot.History.Items[0]
	if grouped.SourceItemID == nil || *grouped.SourceItemID != historyToolSourceKey(groupID) {
		t.Fatalf("expected canonical source item id %q, got %v", historyToolSourceKey(groupID), grouped.SourceItemID)
	}
	if grouped.Tool == nil || grouped.Tool.CommandGroup == nil || grouped.Tool.CommandGroup.Count != 3 {
		t.Fatalf("expected grouped command count 3, got %#v", grouped.Tool)
	}
	if got := len(decodeHistoryGroupItems(grouped.Payload)); got != 3 {
		t.Fatalf("expected 3 command group detail items, got %d", got)
	}
}

func TestApplyEventToHistoryCacheCanonicalizesFileChangeGroupRows(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "File Change Group Canonicalization", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	groupID := commandExecutionGroupID("fc1")
	first := time.UnixMilli(1_000)
	second := time.UnixMilli(2_000)

	if _, err := manager.appendHistoryItem(context.Background(), session.ID, testGroupedCompactHistoryItem(
		"tool:fc2",
		1,
		"fc2",
		groupID,
		"file_change",
		"ui/src/components/web-session/WebSessionPanel.vue",
		[]CommandExecutionGroupItem{
			testCompactGroupDetail("fc1", "file_change", "ui/src/components/web-session/WebSessionPanel.vue", first),
			testCompactGroupDetail("fc2", "file_change", "ui/src/components/web-session/WebSessionPanel.vue", second),
		},
	)); err != nil {
		t.Fatalf("appendHistoryItem returned error: %v", err)
	}
	if _, err := manager.appendHistoryItem(context.Background(), session.ID, testGroupedCompactHistoryItem(
		"tool:fc3-stale",
		2,
		"fc3",
		groupID,
		"file_change",
		"ui/src/components/web-session/WebSessionPanel.vue",
		[]CommandExecutionGroupItem{
			testCompactGroupDetail("fc3", "file_change", "ui/src/components/web-session/WebSessionPanel.vue", time.UnixMilli(3_000)),
		},
	)); err != nil {
		t.Fatalf("appendHistoryItem returned error: %v", err)
	}

	item, err := manager.applyEventToHistoryCache(context.Background(), session.ID, Event{
		ID:        "evt_fc3_end",
		Type:      "tool_end",
		Timestamp: time.UnixMilli(3_000),
		Payload: map[string]any{
			"tid":  "fc3",
			"kind": "file_change",
			"in": map[string]any{
				"path": "ui/src/components/web-session/WebSessionPanel.vue",
				"changes": []any{
					map[string]any{"path": "ui/src/components/web-session/WebSessionPanel.vue"},
				},
			},
			"out": "patched",
			"ok":  true,
			"meta": map[string]any{
				"kind":     "file_change",
				"title":    "FileChange",
				"subtitle": "ui/src/components/web-session/WebSessionPanel.vue",
				"commandGroup": map[string]any{
					"id":           groupID,
					"count":        3,
					"firstSeq":     1,
					"lastSeq":      6,
					"latestToolId": "fc3",
					"compacted":    true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("applyEventToHistoryCache returned error: %v", err)
	}
	if item == nil {
		t.Fatal("expected history item, got nil")
	}

	snapshot, err := manager.Snapshot(context.Background(), session.ID, 10)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.History.Items) != 1 {
		t.Fatalf("expected 1 grouped history item, got %d", len(snapshot.History.Items))
	}
	grouped := snapshot.History.Items[0]
	if grouped.SourceItemID == nil || *grouped.SourceItemID != historyToolSourceKey(groupID) {
		t.Fatalf("expected canonical source item id %q, got %v", historyToolSourceKey(groupID), grouped.SourceItemID)
	}
	if grouped.Tool == nil || grouped.Tool.CommandGroup == nil || grouped.Tool.CommandGroup.Count != 3 {
		t.Fatalf("expected grouped file change count 3, got %#v", grouped.Tool)
	}
	if got := len(decodeHistoryGroupItems(grouped.Payload)); got != 3 {
		t.Fatalf("expected 3 file change detail items, got %d", got)
	}
}

func testCompactHistoryItem(toolID string, kind string, summary string, output string, orderIndex int64) HistoryItem {
	return HistoryItem{
		SourceItemID: nilIfEmptyHistory(toolID),
		OrderIndex:   orderIndex,
		Kind:         "tool",
		ItemType:     kind,
		Timestamp:    ptr(time.UnixMilli(orderIndex * 1_000)),
		ObservedAt:   ptr(time.UnixMilli(orderIndex * 1_000)),
		Tool: &HistoryTool{
			ID:     toolID,
			Name:   compactToolTitle(kind),
			Kind:   kind,
			Input:  map[string]any{"command": summary},
			Output: output,
			Status: "done",
			Meta: map[string]any{
				"title":    compactToolTitle(kind),
				"kind":     kind,
				"subtitle": summary,
			},
		},
		Payload: map[string]any{
			"tid":  toolID,
			"kind": kind,
			"in":   map[string]any{"command": summary},
			"out":  output,
			"meta": map[string]any{
				"title":    compactToolTitle(kind),
				"kind":     kind,
				"subtitle": summary,
			},
		},
	}
}

func testGroupedCompactHistoryItem(
	sourceKey string,
	orderIndex int64,
	toolID string,
	groupID string,
	kind string,
	summary string,
	groupItems []CommandExecutionGroupItem,
) HistoryItem {
	group := &HistoryToolCommandGroup{
		ID:           groupID,
		Count:        len(groupItems),
		LatestToolID: toolID,
		Compacted:    true,
	}
	meta := map[string]any{
		"title":        compactToolTitle(kind),
		"kind":         kind,
		"subtitle":     summary,
		"commandGroup": group,
	}
	return HistoryItem{
		SourceItemID: nilIfEmptyHistory(sourceKey),
		OrderIndex:   orderIndex,
		Kind:         "tool",
		ItemType:     kind,
		Timestamp:    ptr(time.UnixMilli(orderIndex * 1_000)),
		ObservedAt:   ptr(time.UnixMilli(orderIndex * 1_000)),
		Done:         true,
		Tool: &HistoryTool{
			ID:           toolID,
			Name:         compactToolTitle(kind),
			Kind:         kind,
			Input:        map[string]any{"command": summary},
			Output:       summary,
			Status:       "done",
			Meta:         meta,
			CommandGroup: group,
		},
		Payload: map[string]any{
			"tid":        toolID,
			"kind":       kind,
			"in":         map[string]any{"command": summary},
			"out":        summary,
			"meta":       meta,
			"groupItems": groupItems,
		},
	}
}

func testCompactGroupDetail(toolID string, kind string, summary string, ts time.Time) CommandExecutionGroupItem {
	return CommandExecutionGroupItem{
		ToolID:      toolID,
		Kind:        kind,
		Title:       compactToolTitle(kind),
		Summary:     summary,
		Command:     summary,
		Input:       map[string]any{"command": summary},
		Output:      summary,
		Status:      "done",
		Timestamp:   ts,
		StartedAt:   ts,
		CompletedAt: ts,
	}
}
