package websession

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	maxHistoryTransportSummaryRunes    = 512
	maxHistoryTransportStringRunes     = 1024
	maxHistoryTransportToolOutputRunes = 2048
	maxHistoryTransportToolInputBytes  = 8 * 1024
	maxHistoryTransportToolMetaBytes   = 4 * 1024
	maxHistoryTransportPayloadBytes    = 4 * 1024
	maxHistoryTransportMapKeys         = 32
	maxHistoryTransportSliceItems      = 24
	maxHistoryTransportDepth           = 4
)

// HistoryWindowForTransport removes duplicated tool payloads and bounds each
// tool record while keeping persisted command-group details available to the
// detail API.
func HistoryWindowForTransport(window HistoryWindow) HistoryWindow {
	if len(window.Items) == 0 {
		return window
	}

	projected := window
	projected.Items = make([]HistoryItem, 0, len(window.Items))
	for _, item := range window.Items {
		projected.Items = append(projected.Items, historyItemForTransport(item))
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

func historyItemForTransport(item HistoryItem) HistoryItem {
	if item.Kind != "tool" || item.Tool == nil {
		return item
	}

	projected := item
	tool := *item.Tool
	expandable := isExpandableCommandGroup(item)
	tool.Meta = projectHistoryTransportMeta(
		item.Tool.Kind,
		item.Tool.Input,
		item.Tool.Meta,
		item.Tool.Output,
	)
	if expandable {
		tool.Input = nil
		tool.Output = ""
	} else {
		summary := stringValue(tool.Meta["subtitle"])
		tool.Input = boundedHistoryTransportValue(
			item.Tool.Input,
			maxHistoryTransportToolInputBytes,
			historyTransportInputFallback(item.Tool.Kind, item.Tool.Input, summary),
		)
		tool.Output = historyTransportToolOutput(item.Tool.Kind, item.Tool.Output)
	}
	if len(tool.Meta) == 0 {
		tool.Meta = nil
	}
	projected.Tool = &tool

	projected.Payload = cloneMap(item.Payload)
	delete(projected.Payload, "groupItems")
	delete(projected.Payload, "in")
	delete(projected.Payload, "out")
	delete(projected.Payload, "meta")
	projected.Payload = boundedHistoryTransportMap(
		projected.Payload,
		maxHistoryTransportPayloadBytes,
		historyTransportMapFallback(projected.Payload, historyTransportPayloadKeys),
	)
	if len(projected.Payload) == 0 {
		projected.Payload = nil
	}
	return projected
}

func historyTransportToolOutput(kind, output string) string {
	if strings.EqualFold(strings.TrimSpace(kind), "plan") {
		return output
	}
	return limitHistoryTransportText(output, maxHistoryTransportToolOutputRunes)
}

func projectHistoryTransportMeta(kind string, input any, meta map[string]any, output string) map[string]any {
	projected := boundedHistoryTransportMap(
		meta,
		maxHistoryTransportToolMetaBytes,
		historyTransportMapFallback(meta, historyTransportMetaKeys),
	)
	summary := strings.TrimSpace(compactToolSummary(kind, input, meta, output))
	if summary == "" {
		summary = strings.TrimSpace(stringValue(meta["subtitle"]))
	}
	if summary != "" {
		if projected == nil {
			projected = make(map[string]any)
		}
		projected["subtitle"] = limitHistoryTransportSummary(summary)
	}
	return boundedHistoryTransportMap(
		projected,
		maxHistoryTransportToolMetaBytes,
		historyTransportMapFallback(projected, historyTransportMetaKeys),
	)
}

func limitHistoryTransportSummary(value string) string {
	return limitHistoryTransportText(strings.TrimSpace(value), maxHistoryTransportSummaryRunes)
}

var historyTransportInputKeys = []string{
	"command", "path", "file_path", "new_path", "old_path", "notebook_path",
	"pattern", "query", "url", "tool_name", "name", "server", "tool", "action",
	"receiver_ids", "agent_id", "thread_id",
}

var historyTransportMetaKeys = []string{
	"kind", "title", "name", "subtitle", "command", "commandGroup", "path", "threadId",
}

var historyTransportPayloadKeys = []string{
	"kind", "ok", "status", "code", "reason", "iid", "mid", "tid", "toolId", "threadId",
}

func historyTransportInputFallback(kind string, input any, summary string) map[string]any {
	fallback := historyTransportMapFallback(decodeRawObject(input), historyTransportInputKeys)
	if fallback == nil {
		fallback = make(map[string]any)
	}
	if summary = limitHistoryTransportSummary(summary); summary != "" {
		fallback["summary"] = summary
	}
	fallback["kind"] = strings.TrimSpace(kind)
	fallback["_truncated"] = true
	return fallback
}

func historyTransportMapFallback(source map[string]any, keys []string) map[string]any {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]any)
	for _, key := range keys {
		value, ok := source[key]
		if !ok {
			continue
		}
		result[key] = projectHistoryTransportValue(value, 2)
	}
	if len(result) == 0 {
		return nil
	}
	result["_truncated"] = true
	return result
}

func boundedHistoryTransportMap(source map[string]any, maxBytes int, fallback map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	projected := boundedHistoryTransportValue(source, maxBytes, fallback)
	result, _ := projected.(map[string]any)
	return result
}

func boundedHistoryTransportValue(value any, maxBytes int, fallback any) any {
	if value == nil {
		return nil
	}
	projected := projectHistoryTransportValue(value, maxHistoryTransportDepth)
	if historyTransportJSONFits(projected, maxBytes) {
		return projected
	}
	projectedFallback := projectHistoryTransportValue(fallback, 2)
	if projectedFallback != nil && historyTransportJSONFits(projectedFallback, maxBytes) {
		return projectedFallback
	}
	return map[string]any{"_truncated": true}
}

func projectHistoryTransportValue(value any, depth int) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return limitHistoryTransportText(typed, maxHistoryTransportStringRunes)
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(typed, &decoded) == nil {
			return projectHistoryTransportValue(decoded, depth)
		}
		return limitHistoryTransportText(string(typed), maxHistoryTransportStringRunes)
	case map[string]any:
		if depth <= 0 {
			return map[string]any{"_truncated": true}
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(left, right int) bool {
			leftPriority := historyTransportKeyPriority(keys[left])
			rightPriority := historyTransportKeyPriority(keys[right])
			if leftPriority != rightPriority {
				return leftPriority < rightPriority
			}
			return keys[left] < keys[right]
		})
		if len(keys) > maxHistoryTransportMapKeys {
			keys = keys[:maxHistoryTransportMapKeys]
		}
		result := make(map[string]any, len(keys)+1)
		for _, key := range keys {
			result[key] = projectHistoryTransportValue(typed[key], depth-1)
		}
		if len(typed) > len(keys) {
			result["_truncated"] = true
		}
		return result
	case []any:
		if depth <= 0 {
			return []any{}
		}
		limit := len(typed)
		if limit > maxHistoryTransportSliceItems {
			limit = maxHistoryTransportSliceItems
		}
		result := make([]any, 0, limit+1)
		for _, item := range typed[:limit] {
			result = append(result, projectHistoryTransportValue(item, depth-1))
		}
		if len(typed) > limit {
			result = append(result, map[string]any{"_truncated": true})
		}
		return result
	case bool, float32, float64, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number:
		return typed
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return limitHistoryTransportText(stringValue(typed), maxHistoryTransportStringRunes)
		}
		var decoded any
		if json.Unmarshal(encoded, &decoded) != nil {
			return nil
		}
		return projectHistoryTransportValue(decoded, depth)
	}
}

func historyTransportJSONFits(value any, maxBytes int) bool {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return len(encoded) <= maxBytes
}

func historyTransportKeyPriority(key string) int {
	for index, preferred := range historyTransportInputKeys {
		if key == preferred {
			return index
		}
	}
	for index, preferred := range historyTransportMetaKeys {
		if key == preferred {
			return len(historyTransportInputKeys) + index
		}
	}
	return len(historyTransportInputKeys) + len(historyTransportMetaKeys) + 1
}

func limitHistoryTransportText(value string, maxRunes int) string {
	runes := []rune(value)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func isExpandableCommandGroup(item HistoryItem) bool {
	if item.Kind != "tool" || item.Tool == nil || item.Tool.CommandGroup == nil {
		return false
	}
	return item.Tool.CommandGroup.Compacted || item.Tool.CommandGroup.Count > 1
}
