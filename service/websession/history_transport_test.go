package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHistoryWindowForTransportOmitsExpandableCommandDetails(t *testing.T) {
	groupItems := []CommandExecutionGroupItem{
		testCompactGroupDetail("tool-1", "command_execution", "pwd", time.UnixMilli(1_000)),
	}
	original := testGroupedCompactHistoryItem(
		"legacy-source",
		1,
		"tool-1",
		"group-1",
		"command_execution",
		"pwd",
		groupItems,
	)
	original.LastEventSeq = 7

	originalEvent := Event{
		Type: "tool_end",
		Payload: map[string]any{
			"in":  map[string]any{"command": "pwd"},
			"out": "large output",
			"meta": map[string]any{
				"commandGroup": map[string]any{
					"id":    "group-1",
					"count": 2,
				},
			},
		},
	}
	projected := HistoryWindowForTransport(HistoryWindow{
		Items:  []HistoryItem{original},
		Events: []Event{originalEvent},
	})
	if len(projected.Items) != 1 {
		t.Fatalf("expected one projected history item, got %d", len(projected.Items))
	}
	item := projected.Items[0]
	if item.Tool == nil || item.Tool.Input != nil || item.Tool.Output != "" {
		t.Fatalf("expected expandable tool input/output to be removed, got %#v", item.Tool)
	}
	if item.Tool.CommandGroup == nil || item.Tool.CommandGroup.ID != "group-1" {
		t.Fatalf("expected command group metadata to remain, got %#v", item.Tool.CommandGroup)
	}
	if item.Tool.Meta["subtitle"] != "pwd" {
		t.Fatalf("expected group summary to remain, got %#v", item.Tool.Meta)
	}
	if _, exists := item.Payload["groupItems"]; exists {
		t.Fatal("expected groupItems to be removed from the transport payload")
	}
	if _, exists := item.Payload["in"]; exists {
		t.Fatal("expected raw input to be removed from the transport payload")
	}
	if _, exists := item.Payload["out"]; exists {
		t.Fatal("expected raw output to be removed from the transport payload")
	}

	if len(decodeHistoryGroupItems(original.Payload)) != 1 {
		t.Fatal("expected the original history item to retain persisted group details")
	}
	if original.Tool == nil || original.Tool.Input == nil || original.Tool.Output == "" {
		t.Fatal("expected the original history item to remain unchanged")
	}
	if _, exists := projected.Events[0].Payload["in"]; exists {
		t.Fatal("expected raw event input to be removed from the transport payload")
	}
	if _, exists := projected.Events[0].Payload["out"]; exists {
		t.Fatal("expected raw event output to be removed from the transport payload")
	}
	if _, exists := originalEvent.Payload["out"]; !exists {
		t.Fatal("expected the original event to remain unchanged")
	}

	wireItem := mapWireHistoryItem(original)
	if item.LastEventSeq != 7 || wireItem.LastEventSeq != 7 {
		t.Fatalf("expected event sequence to survive transport projection, got item=%d wire=%d", item.LastEventSeq, wireItem.LastEventSeq)
	}
	if wireItem.Tool == nil || wireItem.Tool.Input != nil || wireItem.Tool.Output != "" {
		t.Fatalf("expected websocket tool input/output to be removed, got %#v", wireItem.Tool)
	}
	if _, exists := wireItem.Payload["groupItems"]; exists {
		t.Fatal("expected websocket groupItems to be removed")
	}
}

func TestHistoryWindowForTransportLimitsFallbackSummaries(t *testing.T) {
	group := &HistoryToolCommandGroup{
		ID:        "group-long-summary",
		Count:     2,
		Compacted: true,
	}
	item := HistoryItem{
		Kind: "tool",
		Tool: &HistoryTool{
			ID:           "tool-long-summary",
			Name:         "McpToolCall",
			Kind:         "mcp_tool_call",
			Output:       strings.Repeat("x", maxHistoryTransportSummaryRunes+100),
			Status:       "done",
			CommandGroup: group,
			Meta:         map[string]any{"commandGroup": group},
		},
		Payload: map[string]any{
			"out":  strings.Repeat("x", maxHistoryTransportSummaryRunes+100),
			"meta": map[string]any{"commandGroup": group},
		},
	}

	projected := HistoryWindowForTransport(HistoryWindow{Items: []HistoryItem{item}})
	summary := stringValue(projected.Items[0].Tool.Meta["subtitle"])
	if len([]rune(summary)) != maxHistoryTransportSummaryRunes+3 {
		t.Fatalf("expected bounded summary length, got %d", len([]rune(summary)))
	}
}

func TestHistoryWindowForTransportBoundsSingletonToolPayloads(t *testing.T) {
	largeValues := make([]any, 80)
	for index := range largeValues {
		largeValues[index] = strings.Repeat(fmt.Sprintf("value-%d-", index), 300)
	}
	item := HistoryItem{
		ID:         "singleton-command",
		OrderIndex: 1,
		Kind:       "tool",
		ItemType:   "command_execution",
		Tool: &HistoryTool{
			ID:     "tool-singleton",
			Name:   "CommandExecution",
			Kind:   "command_execution",
			Input:  map[string]any{"command": "go test ./...", "values": largeValues},
			Output: strings.Repeat("large output ", 20_000),
			Status: "done",
			Meta: map[string]any{
				"title":    "CommandExecution",
				"subtitle": "go test ./...",
				"details":  largeValues,
			},
		},
		Payload: map[string]any{
			"kind": "command_execution",
			"in":   map[string]any{"command": "go test ./...", "values": largeValues},
			"out":  strings.Repeat("large output ", 20_000),
			"meta": map[string]any{"subtitle": "go test ./...", "details": largeValues},
			"raw":  largeValues,
		},
	}

	projected := HistoryWindowForTransport(HistoryWindow{Items: []HistoryItem{item}}).Items[0]
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal projected item: %v", err)
	}
	if len(encoded) > 24*1024 {
		t.Fatalf("projected singleton tool is still unbounded: %d bytes", len(encoded))
	}
	if projected.Tool == nil {
		t.Fatal("expected projected tool")
	}
	input := decodeRawObject(projected.Tool.Input)
	if stringValue(input["command"]) != "go test ./..." {
		t.Fatalf("expected useful command summary to remain, got %#v", input)
	}
	if len([]rune(projected.Tool.Output)) > maxHistoryTransportToolOutputRunes+3 {
		t.Fatalf("tool output was not bounded: %d runes", len([]rune(projected.Tool.Output)))
	}
	for _, duplicate := range []string{"in", "out", "meta"} {
		if _, exists := projected.Payload[duplicate]; exists {
			t.Fatalf("expected duplicate payload field %q to be removed", duplicate)
		}
	}
	if item.Tool == nil || len(item.Tool.Output) <= len(projected.Tool.Output) {
		t.Fatal("expected persisted source item to remain unchanged")
	}
}

func TestBoundedHistoryTransportValueFallsBackForUnencodableValues(t *testing.T) {
	projected := boundedHistoryTransportValue(
		math.Inf(1),
		maxHistoryTransportPayloadBytes,
		map[string]any{"_truncated": true},
	)
	if _, err := json.Marshal(projected); err != nil {
		t.Fatalf("bounded transport value remains unencodable: %v", err)
	}
}

func TestGetCommandExecutionGroupFindsLegacyGroupBeyondRecentHistoryWindow(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Legacy command group", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	target := testGroupedCompactHistoryItem(
		"legacy-source",
		1,
		"legacy-tool",
		"legacy-group",
		"command_execution",
		"old command",
		[]CommandExecutionGroupItem{
			testCompactGroupDetail("legacy-tool", "command_execution", "old command", time.UnixMilli(1_000)),
		},
	)
	if _, err := manager.appendHistoryItem(context.Background(), session.ID, target); err != nil {
		t.Fatalf("append target history item returned error: %v", err)
	}

	for index := int64(2); index <= 1101; index++ {
		item := HistoryItem{
			SourceItemID: nilIfEmptyHistory(fmt.Sprintf("legacy-source-%d", index)),
			OrderIndex:   index,
			Kind:         "tool",
			ItemType:     "command_execution",
			Tool: &HistoryTool{
				ID:     fmt.Sprintf("tool-%d", index),
				Kind:   "command_execution",
				Status: "done",
			},
		}
		if _, err := manager.appendHistoryItem(context.Background(), session.ID, item); err != nil {
			t.Fatalf("append filler history item %d returned error: %v", index, err)
		}
	}

	detail, err := manager.GetCommandExecutionGroup(context.Background(), session.ID, "legacy-group")
	if err != nil {
		t.Fatalf("GetCommandExecutionGroup returned error: %v", err)
	}
	if detail.GroupID != "legacy-group" {
		t.Fatalf("expected group ID %q, got %q", "legacy-group", detail.GroupID)
	}
	if len(detail.Items) != 1 || detail.Items[0].Command != "old command" {
		t.Fatalf("expected the old group detail, got %#v", detail.Items)
	}
}
