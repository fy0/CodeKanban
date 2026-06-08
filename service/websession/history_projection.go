package websession

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

const commandExecutionGroupPrefix = "cmdgrp_"

var ErrCommandExecutionGroupNotFound = errors.New("command execution group not found")

type toolSnapshot struct {
	toolID    string
	name      string
	kind      string
	input     any
	meta      map[string]any
	startedAt time.Time
	startSeq  int64
}

type commandExecutionProjectionGroup struct {
	groupID         string
	kind            string
	firstSeq        int64
	lastSeq         int64
	count           int
	toolIDs         map[string]struct{}
	latestToolID    string
	latestName      string
	latestKind      string
	latestInput     any
	latestOutput    string
	latestStatus    string
	latestTimestamp time.Time
	latestRunID     string
	latestParentID  string
	latestMeta      map[string]any
}

type commandExecutionDetailAccumulator struct {
	groupID    string
	kind       string
	title      string
	summary    string
	firstSeq   int64
	lastSeq    int64
	status     string
	latestTool string
	items      []CommandExecutionGroupItem
	itemIndex  map[string]int
}

func (m *Manager) projectedHistoryWindow(
	session tables.WebSessionTable,
	limit int,
	beforeSeq *int64,
) (HistoryWindow, error) {
	if limit <= 0 {
		limit = DefaultHistoryWindow
	}

	rawEvents, err := m.store.readEvents(session.ID)
	if err != nil {
		return HistoryWindow{}, err
	}

	projected := projectHistoryEvents(rawEvents, Agent(session.Agent))
	total := len(projected)
	filtered := projected
	if beforeSeq != nil {
		filtered = make([]Event, 0, len(projected))
		for _, event := range projected {
			if event.Seq < *beforeSeq {
				filtered = append(filtered, event)
			}
		}
	}

	if len(filtered) <= limit {
		return HistoryWindow{
			Events:       filtered,
			HasMore:      false,
			BeforeCursor: "",
			Total:        total,
		}, nil
	}

	start := len(filtered) - limit
	window := filtered[start:]
	hasMore := start > 0
	return HistoryWindow{
		Events:       window,
		HasMore:      hasMore,
		BeforeCursor: historyCursor(window, hasMore),
		Total:        total,
	}, nil
}

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

func projectHistoryEvents(events []Event, agent Agent) []Event {
	projected := make([]Event, 0, len(events))
	activeTools := make(map[string]toolSnapshot)
	var currentGroup *commandExecutionProjectionGroup
	var transparentSinceCompact bool

	flushGroup := func() {
		if currentGroup == nil {
			return
		}
		projected = append(projected, currentGroup.toEvent())
		currentGroup = nil
		transparentSinceCompact = false
	}

	for _, event := range events {
		toolID := eventToolID(event)
		if event.Type == "tool_st" && toolID != "" {
			activeTools[toolID] = snapshotToolEvent(event)
		}

		if isCompactToolEvent(event) {
			kind := compactToolKind(event)
			groupID := eventExplicitCommandGroupID(event)
			toolAlreadyInGroup := false
			if currentGroup != nil {
				_, toolAlreadyInGroup = currentGroup.toolIDs[toolID]
			}
			if currentGroup != nil && currentGroup.kind == kind && toolAlreadyInGroup {
				groupID = currentGroup.groupID
			} else if currentGroup != nil && currentGroup.kind == kind && transparentSinceCompact {
				groupID = currentGroup.groupID
			} else if groupID == "" {
				if currentGroup != nil && currentGroup.kind == kind {
					groupID = currentGroup.groupID
				} else {
					groupID = commandExecutionGroupID(toolID)
				}
			}
			if currentGroup != nil && (currentGroup.groupID != groupID || currentGroup.kind != kind) {
				flushGroup()
			}
			if currentGroup == nil {
				currentGroup = &commandExecutionProjectionGroup{
					groupID: groupID,
					kind:    kind,
					toolIDs: make(map[string]struct{}),
				}
			}
			currentGroup.applyEvent(event, activeTools[toolID])
			if event.Type == "tool_end" && toolID != "" {
				delete(activeTools, toolID)
			}
			transparentSinceCompact = false
			continue
		}

		if isReasoningToolEvent(event) {
			if reasoningEventHasDisplayContent(event) {
				if normalizeAgent(agent) != AgentCodex {
					flushGroup()
				}
				projected = append(projected, event)
			}
			if event.Type == "tool_end" && toolID != "" {
				delete(activeTools, toolID)
			}
			continue
		}

		if isCommandGroupTransparentEvent(event) {
			projected = append(projected, event)
			if currentGroup != nil {
				transparentSinceCompact = true
			}
			continue
		}

		flushGroup()
		projected = append(projected, event)
		if event.Type == "tool_end" && toolID != "" {
			delete(activeTools, toolID)
		}
	}

	flushGroup()
	return projected
}

func buildCommandExecutionGroupLookup(events []Event, agent Agent) map[string]CommandExecutionGroupDetail {
	groups := make(map[string]*commandExecutionDetailAccumulator)
	activeTools := make(map[string]toolSnapshot)
	var currentGroup *commandExecutionDetailAccumulator
	var transparentSinceCompact bool

	for _, event := range events {
		toolID := eventToolID(event)
		if event.Type == "tool_st" && toolID != "" {
			activeTools[toolID] = snapshotToolEvent(event)
		}

		if isCompactToolEvent(event) {
			kind := compactToolKind(event)
			groupID := eventExplicitCommandGroupID(event)
			toolAlreadyInGroup := false
			if currentGroup != nil {
				_, toolAlreadyInGroup = currentGroup.itemIndex[toolID]
			}
			if currentGroup != nil && currentGroup.kind == kind && toolAlreadyInGroup {
				groupID = currentGroup.groupID
			} else if currentGroup != nil && currentGroup.kind == kind && transparentSinceCompact {
				groupID = currentGroup.groupID
			} else if groupID == "" {
				if currentGroup != nil && currentGroup.kind == kind {
					groupID = currentGroup.groupID
				} else {
					groupID = commandExecutionGroupID(toolID)
				}
			}
			if currentGroup != nil && (currentGroup.groupID != groupID || currentGroup.kind != kind) {
				currentGroup = nil
				transparentSinceCompact = false
			}
			if currentGroup == nil {
				currentGroup = &commandExecutionDetailAccumulator{
					groupID:   groupID,
					kind:      kind,
					itemIndex: make(map[string]int),
				}
				groups[groupID] = currentGroup
			}
			currentGroup.applyEvent(event, activeTools[toolID])
			if event.Type == "tool_end" && toolID != "" {
				delete(activeTools, toolID)
			}
			transparentSinceCompact = false
			continue
		}

		if isReasoningToolEvent(event) {
			if reasoningEventHasDisplayContent(event) && normalizeAgent(agent) != AgentCodex {
				currentGroup = nil
			}
			if event.Type == "tool_end" && toolID != "" {
				delete(activeTools, toolID)
			}
			continue
		}

		if isCommandGroupTransparentEvent(event) {
			if currentGroup != nil {
				transparentSinceCompact = true
			}
			continue
		}

		currentGroup = nil
		transparentSinceCompact = false
		if event.Type == "tool_end" && toolID != "" {
			delete(activeTools, toolID)
		}
	}

	result := make(map[string]CommandExecutionGroupDetail, len(groups))
	for groupID, group := range groups {
		result[groupID] = group.toDetail()
	}
	return result
}

func (g *commandExecutionProjectionGroup) applyEvent(event Event, snapshot toolSnapshot) {
	toolID := eventToolID(event)
	if toolID == "" {
		toolID = event.ID
	}
	if g.kind == "" {
		g.kind = compactToolKind(event)
	}
	if g.toolIDs == nil {
		g.toolIDs = make(map[string]struct{})
	}
	if _, exists := g.toolIDs[toolID]; !exists {
		g.toolIDs[toolID] = struct{}{}
		g.count += 1
	}

	if g.firstSeq == 0 || event.Seq < g.firstSeq {
		g.firstSeq = event.Seq
	}
	if event.Seq > g.lastSeq {
		g.lastSeq = event.Seq
	}

	g.latestToolID = toolID
	g.latestTimestamp = event.Timestamp
	g.latestRunID = event.RunID
	g.latestParentID = event.ParentID

	name := firstNonEmpty(
		eventToolName(event),
		snapshot.name,
		g.latestName,
		compactToolTitle(g.kind),
	)
	kind := firstNonEmpty(eventToolKind(event), snapshot.kind, g.latestKind, g.kind)
	input := firstNonNil(eventToolInput(event), snapshot.input, g.latestInput)
	meta := firstMap(eventToolMeta(event), snapshot.meta, g.latestMeta)

	g.latestName = name
	g.latestKind = kind
	g.latestInput = input
	g.latestMeta = cloneMap(meta)
	if g.latestMeta == nil {
		g.latestMeta = make(map[string]any)
	}
	g.latestMeta["subtitle"] = compactToolSummary(g.kind, g.latestInput, g.latestMeta, eventToolOutput(event))

	if event.Type == "tool_end" {
		g.latestOutput = eventToolOutput(event)
		if eventToolSucceeded(event) {
			g.latestStatus = "done"
		} else {
			g.latestStatus = "error"
		}
		return
	}

	g.latestOutput = ""
	g.latestStatus = "running"
}

func (g *commandExecutionProjectionGroup) toEvent() Event {
	meta := cloneMap(g.latestMeta)
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["kind"] = g.kind
	meta["title"] = firstNonEmpty(stringValue(meta["title"]), g.latestName, compactToolTitle(g.kind))
	meta["subtitle"] = compactToolSummary(g.kind, g.latestInput, meta, g.latestOutput)
	meta["commandGroup"] = map[string]any{
		"id":           g.groupID,
		"count":        g.count,
		"firstSeq":     g.firstSeq,
		"lastSeq":      g.lastSeq,
		"latestToolId": g.latestToolID,
		"compacted":    true,
	}

	payload := map[string]any{
		"tid":  g.groupID,
		"name": firstNonEmpty(g.latestName, compactToolTitle(g.kind)),
		"kind": g.kind,
		"in":   g.latestInput,
		"meta": meta,
	}
	eventType := "tool_end"
	if g.latestStatus == "running" {
		eventType = "tool_st"
	} else {
		payload["out"] = g.latestOutput
		payload["ok"] = g.latestStatus != "error"
	}

	return Event{
		ID:        g.groupID,
		Seq:       g.lastSeq,
		Type:      eventType,
		RunID:     g.latestRunID,
		ParentID:  g.latestParentID,
		Timestamp: g.latestTimestamp,
		Payload:   payload,
	}
}

func (g *commandExecutionDetailAccumulator) applyEvent(event Event, snapshot toolSnapshot) {
	toolID := eventToolID(event)
	if toolID == "" {
		toolID = event.ID
	}
	if g.itemIndex == nil {
		g.itemIndex = make(map[string]int)
	}
	index, exists := g.itemIndex[toolID]
	if !exists {
		index = len(g.items)
		g.itemIndex[toolID] = index
		g.items = append(g.items, CommandExecutionGroupItem{
			ToolID: toolID,
			Kind:   g.kind,
			Title:  compactToolTitle(g.kind),
			Status: "running",
		})
	}

	if g.firstSeq == 0 || event.Seq < g.firstSeq {
		g.firstSeq = event.Seq
	}
	if event.Seq > g.lastSeq {
		g.lastSeq = event.Seq
	}
	g.latestTool = toolID

	item := g.items[index]
	meta := firstMap(eventToolMeta(event), snapshot.meta)
	item.Input = firstNonNil(item.Input, eventToolInput(event), snapshot.input)
	item.Title = compactToolTitle(g.kind)
	item.Kind = g.kind
	item.Summary = compactToolSummary(g.kind, item.Input, meta, eventToolOutput(event))
	item.Command = compactToolSummary(g.kind, item.Input, meta, eventToolOutput(event))
	if event.Type == "tool_st" {
		item.StartedAt = event.Timestamp
		item.Timestamp = event.Timestamp
		item.Status = "running"
	} else {
		item.Output = eventToolOutput(event)
		item.Timestamp = event.Timestamp
		item.CompletedAt = event.Timestamp
		if item.StartedAt.IsZero() {
			item.StartedAt = snapshot.startedAt
		}
		if eventToolSucceeded(event) {
			item.Status = "done"
		} else {
			item.Status = "error"
		}
	}

	g.status = item.Status
	g.title = item.Title
	g.summary = item.Summary
	g.items[index] = item
}

func (g *commandExecutionDetailAccumulator) toDetail() CommandExecutionGroupDetail {
	return CommandExecutionGroupDetail{
		GroupID:    g.groupID,
		Kind:       g.kind,
		Title:      firstNonEmpty(g.title, compactToolTitle(g.kind)),
		Summary:    g.summary,
		Count:      len(g.items),
		FirstSeq:   g.firstSeq,
		LastSeq:    g.lastSeq,
		Status:     g.status,
		LatestTool: g.latestTool,
		Items:      append([]CommandExecutionGroupItem(nil), g.items...),
	}
}

func snapshotToolEvent(event Event) toolSnapshot {
	return toolSnapshot{
		toolID:    eventToolID(event),
		name:      eventToolName(event),
		kind:      eventToolKind(event),
		input:     eventToolInput(event),
		meta:      cloneMap(eventToolMeta(event)),
		startedAt: event.Timestamp,
		startSeq:  event.Seq,
	}
}

func isCompactToolEvent(event Event) bool {
	if event.Type != "tool_st" && event.Type != "tool_end" {
		return false
	}
	return isCompactToolKind(eventToolKind(event))
}

func isCompactToolKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "command_execution", "file_change", "mcp_tool_call", "web_search":
		return true
	default:
		return false
	}
}

func compactToolKind(event Event) string {
	return eventToolKind(event)
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

func eventCommandGroupID(event Event) string {
	if id := eventExplicitCommandGroupID(event); id != "" {
		return id
	}
	return commandExecutionGroupID(eventToolID(event))
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
	default:
		return strings.TrimSpace(firstNonEmpty(stringValue(meta["subtitle"]), output))
	}
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

func firstMap(values ...map[string]any) map[string]any {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
