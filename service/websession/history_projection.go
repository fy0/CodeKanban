package websession

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"code-kanban/utils"
)

const commandExecutionGroupPrefix = "cmdgrp_"

var ErrCommandExecutionGroupNotFound = errors.New("command execution group not found")

func (m *Manager) GetCommandExecutionGroup(
	ctx context.Context,
	sessionID string,
	groupID string,
) (CommandExecutionGroupDetail, error) {
	cachedItem, err := m.findHistoryItemByToolKey(ctx, sessionID, strings.TrimSpace(groupID))
	if err != nil {
		return CommandExecutionGroupDetail{}, ErrCommandExecutionGroupNotFound
	}
	if cachedItem.Tool == nil {
		return CommandExecutionGroupDetail{}, ErrCommandExecutionGroupNotFound
	}

	items := []CommandExecutionGroupItem{}
	if rawGroupItems, ok := cachedItem.Payload["groupItems"]; ok {
		decodeRawObject := mustJSONCompatibleGroupItems(rawGroupItems)
		if len(decodeRawObject) > 0 {
			items = decodeRawObject
		}
	}
	if len(items) == 0 {
		items = append(items, historyGroupDetailItem(cachedItem))
	}

	var firstSeq int64
	var lastSeq int64
	latestToolID := cachedItem.Tool.ID
	if cachedItem.Tool.CommandGroup != nil {
		firstSeq = cachedItem.Tool.CommandGroup.FirstSeq
		lastSeq = cachedItem.Tool.CommandGroup.LastSeq
		latestToolID = firstNonEmpty(cachedItem.Tool.CommandGroup.LatestToolID, latestToolID)
	}

	return CommandExecutionGroupDetail{
		GroupID:    firstNonEmpty(groupID, cachedItem.Tool.ID),
		Kind:       cachedItem.Tool.Kind,
		Title:      firstNonEmpty(stringValue(cachedItem.Tool.Meta["title"]), cachedItem.Tool.Name),
		Summary:    compactToolSummary(cachedItem.Tool.Kind, cachedItem.Tool.Input, cachedItem.Tool.Meta, cachedItem.Tool.Output),
		Count:      len(items),
		FirstSeq:   firstSeq,
		LastSeq:    lastSeq,
		Status:     cachedItem.Tool.Status,
		LatestTool: latestToolID,
		Items:      items,
	}, nil
}

func mustJSONCompatibleGroupItems(raw any) []CommandExecutionGroupItem {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var items []CommandExecutionGroupItem
	if err := json.Unmarshal(encoded, &items); err != nil {
		return nil
	}
	return items
}

func isCompactToolEvent(event Event) bool {
	if event.Type != "tool_st" && event.Type != "tool_end" {
		return false
	}
	if !isCompactToolKind(eventToolKind(event)) {
		return false
	}
	return !isInteractiveDynamicToolName(eventToolName(event))
}

func isCompactToolKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "command_execution", "file_change", "mcp_tool_call", "web_search", "dynamic_tool_call":
		return true
	default:
		return false
	}
}

func compactToolKind(event Event) string {
	return eventToolKind(event)
}

func compactToolGroupKey(event Event) string {
	kind := compactToolKind(event)
	if kind != "dynamic_tool_call" {
		return kind
	}
	name := strings.ToLower(strings.TrimSpace(eventToolName(event)))
	if name == "" {
		return kind
	}
	return kind + "\x00" + name
}

func historyCompactToolGroupKey(item HistoryItem) string {
	if item.Tool == nil || item.Tool.Kind != "dynamic_tool_call" {
		if item.Tool == nil {
			return ""
		}
		return strings.TrimSpace(item.Tool.Kind)
	}
	name := strings.ToLower(strings.TrimSpace(item.Tool.Name))
	if name == "" {
		return item.Tool.Kind
	}
	return item.Tool.Kind + "\x00" + name
}

func isInteractiveDynamicToolName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "askuserquestion":
		return true
	default:
		return false
	}
}

func isReasoningToolEvent(event Event) bool {
	if event.Type != "tool_st" && event.Type != "tool_end" {
		return false
	}
	return eventToolKind(event) == "reasoning"
}

func reasoningEventHasDisplayContent(event Event) bool {
	if !isReasoningToolEvent(event) {
		return false
	}
	return strings.TrimSpace(eventToolOutput(event)) != ""
}

func isCommandGroupTransparentEvent(event Event) bool {
	switch strings.TrimSpace(event.Type) {
	case "usage":
		return true
	default:
		return false
	}
}

func compactToolTitle(kind string) string {
	switch strings.TrimSpace(kind) {
	case "command_execution":
		return "CommandExecution"
	case "file_change":
		return "FileChange"
	case "mcp_tool_call":
		return "McpToolCall"
	case "web_search":
		return "WebSearch"
	default:
		return "Tool"
	}
}

func eventToolID(event Event) string {
	return strings.TrimSpace(firstNonEmpty(stringValue(event.Payload["tid"]), event.ID))
}

func eventToolName(event Event) string {
	return strings.TrimSpace(firstNonEmpty(stringValue(event.Payload["name"]), stringValue(eventToolMeta(event)["title"])))
}

func eventToolKind(event Event) string {
	return normalizeCodexItemType(firstNonEmpty(
		stringValue(event.Payload["kind"]),
		stringValue(eventToolMeta(event)["kind"]),
	))
}

func eventToolInput(event Event) any {
	if event.Payload == nil {
		return nil
	}
	if value, ok := event.Payload["in"]; ok {
		return value
	}
	return nil
}

func eventToolOutput(event Event) string {
	return stringValue(event.Payload["out"])
}

func eventToolSucceeded(event Event) bool {
	if event.Type != "tool_end" {
		return true
	}
	if event.Payload == nil {
		return true
	}
	if value, ok := event.Payload["ok"].(bool); ok {
		return value
	}
	return true
}

func eventToolMeta(event Event) map[string]any {
	return decodeRawObject(event.Payload["meta"])
}

func eventExplicitCommandGroupID(event Event) string {
	meta := eventToolMeta(event)
	group := decodeRawObject(meta["commandGroup"])
	return strings.TrimSpace(stringValue(group["id"]))
}

func commandExecutionGroupID(toolID string) string {
	normalized := strings.TrimSpace(toolID)
	if normalized == "" {
		return commandExecutionGroupPrefix + utils.NewID()
	}
	return commandExecutionGroupPrefix + normalized
}

func commandFromInput(input any) string {
	record := decodeRawObject(input)
	if command := strings.TrimSpace(stringValue(record["command"])); command != "" {
		return command
	}
	return ""
}

func commandFromMeta(meta map[string]any) string {
	return strings.TrimSpace(firstNonEmpty(stringValue(meta["subtitle"]), stringValue(meta["command"])))
}

func compactToolSummary(kind string, input any, meta map[string]any, output string) string {
	switch strings.TrimSpace(kind) {
	case "command_execution":
		return strings.TrimSpace(firstNonEmpty(commandFromInput(input), commandFromMeta(meta)))
	case "file_change":
		if summary := fileChangeSummary(input); summary != "" {
			return summary
		}
		return strings.TrimSpace(firstNonEmpty(stringValue(meta["subtitle"]), summarizeChanges(input)))
	case "mcp_tool_call":
		if summary := mcpToolCallSummary(input); summary != "" {
			return summary
		}
		return strings.TrimSpace(firstNonEmpty(stringValue(meta["subtitle"]), output))
	case "sub_agent_tool_call":
		if summary := subAgentToolCallSummary(input); summary != "" {
			return summary
		}
		return strings.TrimSpace(firstNonEmpty(stringValue(meta["subtitle"]), output))
	case "web_search":
		if summary := webSearchSummary(input); summary != "" {
			return summary
		}
		return strings.TrimSpace(firstNonEmpty(stringValue(meta["subtitle"]), output))
	case "dynamic_tool_call":
		return dynamicToolSummary(input, meta, output)
	default:
		return strings.TrimSpace(firstNonEmpty(stringValue(meta["subtitle"]), output))
	}
}

func dynamicToolSummary(input any, meta map[string]any, output string) string {
	record := decodeRawObject(input)
	toolName := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		stringValue(meta["title"]),
		stringValue(meta["name"]),
	)))
	path := strings.TrimSpace(firstNonEmpty(
		stringValue(record["file_path"]),
		stringValue(record["path"]),
		stringValue(record["notebook_path"]),
	))
	pattern := strings.TrimSpace(stringValue(record["pattern"]))
	query := strings.TrimSpace(firstNonEmpty(stringValue(record["query"]), stringValue(record["url"])))

	switch toolName {
	case "read", "notebookread", "ls":
		if path != "" {
			return path
		}
	case "grep":
		if pattern != "" && path != "" {
			return pattern + " · " + path
		}
		if pattern != "" {
			return pattern
		}
		if path != "" {
			return path
		}
	case "glob":
		if pattern != "" && path != "" {
			return pattern + " · " + path
		}
		if pattern != "" {
			return pattern
		}
		if path != "" {
			return path
		}
	default:
		if path != "" {
			return path
		}
		if query != "" {
			return query
		}
		if pattern != "" {
			return pattern
		}
	}

	return strings.TrimSpace(firstNonEmpty(stringValue(meta["subtitle"]), output))
}

func webSearchSummary(input any) string {
	record := decodeRawObject(input)
	query := strings.TrimSpace(stringValue(record["query"]))
	if query != "" {
		return query
	}
	action := decodeRawObject(record["action"])
	queries := decodeStringArray(action["queries"])
	if len(queries) > 0 {
		return queries[0]
	}
	return ""
}

func decodeStringArray(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func fileChangeSummary(input any) string {
	record := decodeRawObject(input)
	if path := fileChangePath(record); path != "" {
		return path
	}

	if changes := decodeRawArray(record["changes"]); len(changes) > 0 {
		for _, change := range changes {
			if path := fileChangePath(change); path != "" {
				return path
			}
		}
	}
	return ""
}

func fileChangePath(record map[string]any) string {
	return strings.TrimSpace(firstNonEmpty(
		stringValue(record["path"]),
		stringValue(record["file_path"]),
		stringValue(record["new_path"]),
		stringValue(record["old_path"]),
		stringValue(record["newPath"]),
		stringValue(record["oldPath"]),
		stringValue(record["move_path"]),
		stringValue(record["movePath"]),
	))
}

func summarizeChanges(input any) string {
	record := decodeRawObject(input)
	changes := decodeRawArray(record["changes"])
	if len(changes) == 1 {
		return "1 change"
	}
	if len(changes) > 1 {
		return strconv.Itoa(len(changes)) + " changes"
	}
	return ""
}

func mcpToolCallSummary(input any) string {
	record := decodeRawObject(input)
	toolName := strings.TrimSpace(firstNonEmpty(
		stringValue(record["tool_name"]),
		stringValue(record["name"]),
	))
	target := strings.TrimSpace(firstNonEmpty(
		extractMcpArgumentHint(record["arguments"]),
		stringValue(record["server"]),
		stringValue(record["path"]),
	))
	if toolName != "" && target != "" && toolName != target {
		return toolName + " · " + target
	}
	return firstNonEmpty(toolName, target)
}

func extractMcpArgumentHint(value any) string {
	record := decodeRawObject(value)
	return strings.TrimSpace(firstNonEmpty(
		stringValue(record["url"]),
		stringValue(record["query"]),
		stringValue(record["path"]),
		stringValue(record["file"]),
		stringValue(record["name"]),
		stringValue(record["id"]),
	))
}

func subAgentToolCallSummary(input any) string {
	record := decodeRawObject(input)
	if len(record) == 0 {
		return ""
	}
	for _, key := range []string{
		"task",
		"prompt",
		"description",
		"instruction",
		"instructions",
		"objective",
		"title",
		"name",
	} {
		if value := strings.TrimSpace(stringValue(record[key])); value != "" {
			return value
		}
	}
	if args := decodeRawObject(record["arguments"]); len(args) > 0 {
		for _, key := range []string{"task", "prompt", "description", "instruction", "objective", "title"} {
			if value := strings.TrimSpace(stringValue(args[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func decodeRawArray(raw any) []map[string]any {
	var items []any
	switch typed := raw.(type) {
	case []any:
		items = typed
	case []map[string]any:
		items = make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
	default:
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		record := decodeRawObject(item)
		if len(record) == 0 {
			continue
		}
		result = append(result, record)
	}
	return result
}

func cloneMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
