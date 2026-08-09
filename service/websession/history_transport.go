package websession

import "strings"

const maxHistoryTransportSummaryRunes = 512

// HistoryWindowForTransport removes expandable command-group details from the
// history view while keeping the persisted data available to the detail API.
func HistoryWindowForTransport(window HistoryWindow) HistoryWindow {
	if len(window.Items) == 0 && len(window.Events) == 0 {
		return window
	}

	projected := window
	projected.Items = make([]HistoryItem, 0, len(window.Items))
	for _, item := range window.Items {
		projected.Items = append(projected.Items, historyItemForTransport(item))
	}
	if len(window.Events) > 0 {
		projected.Events = make([]Event, 0, len(window.Events))
		for _, event := range window.Events {
			projected.Events = append(projected.Events, historyEventForTransport(event))
		}
	}
	return projected
}

// SessionSnapshotResponseForTransport projects a snapshot response for HTTP.
func SessionSnapshotResponseForTransport(response SessionSnapshotResponse) SessionSnapshotResponse {
	projected := response
	if response.History != nil {
		history := HistoryWindowForTransport(*response.History)
		projected.History = &history
	}
	return projected
}

// SessionSnapshotForTransport projects a full snapshot for an HTTP response.
func SessionSnapshotForTransport(snapshot SessionSnapshot) SessionSnapshot {
	projected := snapshot
	projected.History = HistoryWindowForTransport(snapshot.History)
	return projected
}

// ImportResultForTransport projects an imported history for an HTTP response.
func ImportResultForTransport(result ImportResult) ImportResult {
	projected := result
	projected.History = HistoryWindowForTransport(result.History)
	return projected
}

func historyItemForTransport(item HistoryItem) HistoryItem {
	if !isExpandableCommandGroup(item) {
		return item
	}

	projected := item
	tool := *item.Tool
	tool.Input = nil
	tool.Output = ""
	tool.Meta = projectHistoryTransportMeta(
		item.Tool.Kind,
		item.Tool.Input,
		tool.Meta,
		item.Tool.Output,
	)
	if len(tool.Meta) == 0 {
		tool.Meta = nil
	}
	projected.Tool = &tool

	projected.Payload = cloneMap(item.Payload)
	delete(projected.Payload, "groupItems")
	delete(projected.Payload, "in")
	delete(projected.Payload, "out")
	if len(tool.Meta) > 0 {
		if projected.Payload == nil {
			projected.Payload = make(map[string]any)
		}
		projected.Payload["meta"] = tool.Meta
	}
	if len(projected.Payload) == 0 {
		projected.Payload = nil
	}
	return projected
}

func historyEventForTransport(event Event) Event {
	if !isExpandableCommandGroupEvent(event) {
		return event
	}

	projected := event
	projected.Payload = cloneMap(event.Payload)
	delete(projected.Payload, "groupItems")
	delete(projected.Payload, "in")
	delete(projected.Payload, "out")
	meta := projectHistoryTransportMeta(
		eventToolKind(event),
		eventToolInput(event),
		eventToolMeta(event),
		eventToolOutput(event),
	)
	if len(meta) > 0 {
		if projected.Payload == nil {
			projected.Payload = make(map[string]any)
		}
		projected.Payload["meta"] = meta
	}
	if len(projected.Payload) == 0 {
		projected.Payload = nil
	}
	return projected
}

func projectHistoryTransportMeta(kind string, input any, meta map[string]any, output string) map[string]any {
	projected := cloneMap(meta)
	summary := strings.TrimSpace(compactToolSummary(kind, input, projected, output))
	if summary == "" {
		summary = strings.TrimSpace(stringValue(projected["subtitle"]))
	}
	if summary != "" {
		if projected == nil {
			projected = make(map[string]any)
		}
		projected["subtitle"] = limitHistoryTransportSummary(summary)
	}
	return projected
}

func limitHistoryTransportSummary(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxHistoryTransportSummaryRunes {
		return string(runes)
	}
	return string(runes[:maxHistoryTransportSummaryRunes]) + "..."
}

func isExpandableCommandGroup(item HistoryItem) bool {
	if item.Kind != "tool" || item.Tool == nil || item.Tool.CommandGroup == nil {
		return false
	}
	return item.Tool.CommandGroup.Compacted || item.Tool.CommandGroup.Count > 1
}

func isExpandableCommandGroupEvent(event Event) bool {
	if event.Type != "tool_st" && event.Type != "tool_end" {
		return false
	}
	group := decodeRawObject(eventToolMeta(event)["commandGroup"])
	if len(group) == 0 {
		return false
	}
	compacted, _ := group["compacted"].(bool)
	count := int(numberValue(group["count"]))
	return compacted || count > 1
}
