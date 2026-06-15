package websession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"
)

const cacheSyncReadTimeout = 10 * time.Second

func syncedToolStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "completed", "success", "succeeded", "done":
		return "done"
	case "failed", "error":
		return "error"
	default:
		return "running"
	}
}

func joinedNonEmpty(parts ...string) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}
	return strings.Join(items, "\n")
}

func parseHistoryTimestamp(raw any) *time.Time {
	switch value := raw.(type) {
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return nil
		}
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return &parsed
		}
		if parsed, err := time.Parse(time.RFC3339, text); err == nil {
			return &parsed
		}
	case float64:
		if value <= 0 {
			return nil
		}
		seconds := int64(value)
		if value >= 1e12 {
			parsed := time.UnixMilli(seconds)
			return &parsed
		}
		parsed := time.Unix(seconds, 0)
		return &parsed
	case int64:
		if value <= 0 {
			return nil
		}
		parsed := time.UnixMilli(value)
		return &parsed
	case int:
		if value <= 0 {
			return nil
		}
		parsed := time.UnixMilli(int64(value))
		return &parsed
	}
	return nil
}

func threadReadItemTimestamp(item map[string]any) *time.Time {
	for _, key := range []string{"timestamp", "createdAt", "startedAt", "completedAt", "updatedAt"} {
		if parsed := parseHistoryTimestamp(item[key]); parsed != nil {
			return parsed
		}
	}
	return nil
}

func (m *Manager) mapThreadReadItem(
	item map[string]any,
	orderIndex int64,
) (HistoryItem, error) {
	itemType := strings.TrimSpace(stringValue(item["type"]))
	sourceItemID := strings.TrimSpace(stringValue(item["id"]))
	itemTimestamp := threadReadItemTimestamp(item)
	result := HistoryItem{
		ID:           sourceItemID,
		SourceItemID: nilIfEmptyHistory(sourceItemID),
		OrderIndex:   orderIndex,
		ItemType:     itemType,
		Timestamp:    itemTimestamp,
		ObservedAt:   itemTimestamp,
		Payload:      cloneMap(item),
	}

	switch itemType {
	case "userMessage":
		content := decodeRawArray(item["content"])
		texts := make([]string, 0, len(content))
		attachments := make([]HistoryAttachment, 0)
		for _, block := range content {
			switch strings.TrimSpace(stringValue(block["type"])) {
			case "text":
				texts = append(texts, stringValue(block["text"]))
			case "localImage":
				attachment, err := m.registerExternalAttachment(stringValue(block["path"]))
				if err == nil {
					attachments = append(attachments, attachment)
				}
			}
		}
		result.Kind = "user"
		result.Text = strings.TrimSpace(strings.Join(texts, "\n"))
		if isGoalBootstrapPrompt(result.Text) && len(attachments) == 0 {
			return HistoryItem{}, nil
		}
		result.Attachments = attachments
		return result, nil
	case "agentMessage":
		result.Kind = "assistant"
		result.Text = stringValue(item["text"])
		result.Done = true
		return result, nil
	case "plan":
		result.Kind = "tool"
		result.Text = ""
		result.Tool = &HistoryTool{
			ID:     firstNonEmpty(sourceItemID, fmt.Sprintf("plan_%d", orderIndex)),
			Name:   "Plan",
			Kind:   "plan",
			Output: stringValue(item["text"]),
			Status: "done",
			Meta: map[string]any{
				"title": "Plan",
				"kind":  "plan",
			},
		}
		return result, nil
	case "reasoning":
		summaryParts := stringArrayValues(item["summary"])
		contentParts := stringArrayValues(item["content"])
		result.Kind = "tool"
		result.Tool = &HistoryTool{
			ID:     firstNonEmpty(sourceItemID, fmt.Sprintf("reasoning_%d", orderIndex)),
			Name:   "Reasoning",
			Kind:   "reasoning",
			Output: joinedNonEmpty(strings.Join(summaryParts, "\n"), strings.Join(contentParts, "\n")),
			Status: "done",
			Meta: map[string]any{
				"title": "Reasoning",
				"kind":  "reasoning",
			},
		}
		return result, nil
	case "contextCompaction", "context_compaction":
		result.Kind = "tool"
		result.Tool = &HistoryTool{
			ID:     firstNonEmpty(sourceItemID, fmt.Sprintf("context_compaction_%d", orderIndex)),
			Name:   "Context Compaction",
			Kind:   "context_compaction",
			Output: extractContextCompactionText(item),
			Status: syncedToolStatus(firstNonEmpty(stringValue(item["status"]), "completed")),
			Meta: map[string]any{
				"title":    "Context Compaction",
				"kind":     "context_compaction",
				"subtitle": contextCompactionSubtitle(item),
			},
		}
		return result, nil
	case "commandExecution":
		command := stringValue(item["command"])
		cwd := stringValue(item["cwd"])
		status := syncedToolStatus(stringValue(item["status"]))
		result.Kind = "tool"
		result.Tool = &HistoryTool{
			ID:   firstNonEmpty(sourceItemID, fmt.Sprintf("command_%d", orderIndex)),
			Name: "CommandExecution",
			Kind: "command_execution",
			Input: map[string]any{
				"command":        command,
				"cwd":            cwd,
				"commandActions": item["commandActions"],
			},
			Output: stringValue(item["aggregatedOutput"]),
			Status: status,
			Meta: map[string]any{
				"title":    "CommandExecution",
				"kind":     "command_execution",
				"subtitle": firstNonEmpty(command, cwd),
				"duration": item["durationMs"],
				"exitCode": item["exitCode"],
			},
		}
		return result, nil
	case "fileChange":
		status := syncedToolStatus(stringValue(item["status"]))
		changes := decodeRawArray(item["changes"])
		subtitle := ""
		if len(changes) > 0 {
			change := changes[0]
			subtitle = firstNonEmpty(
				stringValue(change["path"]),
				stringValue(change["newPath"]),
				stringValue(change["oldPath"]),
			)
		}
		result.Kind = "tool"
		result.Tool = &HistoryTool{
			ID:   firstNonEmpty(sourceItemID, fmt.Sprintf("file_change_%d", orderIndex)),
			Name: "FileChange",
			Kind: "file_change",
			Input: map[string]any{
				"changes": changes,
			},
			Status: status,
			Meta: map[string]any{
				"title":    "FileChange",
				"kind":     "file_change",
				"subtitle": subtitle,
			},
		}
		return result, nil
	case "mcpToolCall":
		status := syncedToolStatus(stringValue(item["status"]))
		result.Kind = "tool"
		result.Tool = &HistoryTool{
			ID:   firstNonEmpty(sourceItemID, fmt.Sprintf("mcp_%d", orderIndex)),
			Name: "McpToolCall",
			Kind: "mcp_tool_call",
			Input: map[string]any{
				"server":    item["server"],
				"tool_name": item["tool"],
				"arguments": item["arguments"],
			},
			Output: mustJSONText(item["result"]),
			Status: status,
			Meta: map[string]any{
				"title":    "McpToolCall",
				"kind":     "mcp_tool_call",
				"subtitle": firstNonEmpty(stringValue(item["tool"]), stringValue(item["server"])),
			},
		}
		return result, nil
	case "dynamicToolCall":
		status := syncedToolStatus(stringValue(item["status"]))
		result.Kind = "tool"
		result.Tool = &HistoryTool{
			ID:     firstNonEmpty(sourceItemID, fmt.Sprintf("dynamic_%d", orderIndex)),
			Name:   firstNonEmpty(stringValue(item["tool"]), "DynamicToolCall"),
			Kind:   "dynamic_tool_call",
			Input:  item["arguments"],
			Output: mustJSONText(item["contentItems"]),
			Status: status,
			Meta: map[string]any{
				"title": firstNonEmpty(stringValue(item["tool"]), "DynamicToolCall"),
				"kind":  "dynamic_tool_call",
			},
		}
		return result, nil
	case "webSearch":
		result.Kind = "tool"
		result.Tool = &HistoryTool{
			ID:   firstNonEmpty(sourceItemID, fmt.Sprintf("web_search_%d", orderIndex)),
			Name: "WebSearch",
			Kind: "web_search",
			Input: map[string]any{
				"query":  item["query"],
				"action": item["action"],
			},
			Output: mustJSONText(item["action"]),
			Status: "done",
			Meta: map[string]any{
				"title":    "WebSearch",
				"kind":     "web_search",
				"subtitle": firstNonEmpty(stringValue(item["query"]), webSearchSummary(map[string]any{"action": item["action"]})),
			},
		}
		return result, nil
	default:
		result.Kind = "system"
		result.Level = "info"
		result.Text = firstNonEmpty(
			stringValue(item["text"]),
			stringValue(item["review"]),
			fmt.Sprintf("[%s]", itemType),
		)
		return result, nil
	}
}

func nilIfEmptyHistory(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringArrayValues(raw any) []string {
	values := []string{}
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				values = append(values, text)
			}
		}
	case []string:
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				values = append(values, text)
			}
		}
	}
	return values
}

func isCompactHistoryTool(item HistoryItem) bool {
	if item.Kind != "tool" || item.Tool == nil {
		return false
	}
	switch strings.TrimSpace(item.Tool.Kind) {
	case "command_execution", "file_change", "mcp_tool_call", "web_search":
		return true
	default:
		return false
	}
}

func historyGroupDetailItem(item HistoryItem) CommandExecutionGroupItem {
	detail := CommandExecutionGroupItem{
		ToolID: item.Tool.ID,
		Kind:   item.Tool.Kind,
		Title:  firstNonEmpty(item.Tool.Name, compactToolTitle(item.Tool.Kind)),
		Status: item.Tool.Status,
		Input:  item.Tool.Input,
		Output: item.Tool.Output,
	}
	if item.Timestamp != nil {
		detail.Timestamp = *item.Timestamp
	}
	if subtitle := compactToolSummary(item.Tool.Kind, item.Tool.Input, item.Tool.Meta, item.Tool.Output); subtitle != "" {
		detail.Summary = subtitle
		detail.Command = subtitle
	}
	if input := decodeRawObject(item.Tool.Input); strings.TrimSpace(stringValue(input["command"])) != "" {
		detail.Command = stringValue(input["command"])
	}
	return detail
}

func compactSyncedHistoryItems(items []HistoryItem) []HistoryItem {
	if len(items) == 0 {
		return items
	}
	result := make([]HistoryItem, 0, len(items))
	index := 0
	for index < len(items) {
		current := items[index]
		if !isCompactHistoryTool(current) {
			result = append(result, current)
			index++
			continue
		}

		kind := current.Tool.Kind
		group := []HistoryItem{current}
		nextIndex := index + 1
		for nextIndex < len(items) {
			next := items[nextIndex]
			if !isCompactHistoryTool(next) || next.Tool.Kind != kind {
				break
			}
			group = append(group, next)
			nextIndex++
		}
		if len(group) == 1 {
			result = append(result, current)
			index = nextIndex
			continue
		}

		latest := group[len(group)-1]
		groupID := commandExecutionGroupID(firstNonEmpty(current.Tool.ID, current.ID))
		latest.SourceItemID = nilIfEmptyHistory(historyToolSourceKey(groupID))
		latest.Tool.CommandGroup = &HistoryToolCommandGroup{
			ID:           groupID,
			Count:        len(group),
			LatestToolID: latest.Tool.ID,
			Compacted:    true,
		}
		if latest.Tool.Meta == nil {
			latest.Tool.Meta = map[string]any{}
		}
		latest.Tool.Meta["commandGroup"] = latest.Tool.CommandGroup
		latest.Payload = cloneMap(latest.Payload)
		latest.Payload["groupItems"] = func() []CommandExecutionGroupItem {
			details := make([]CommandExecutionGroupItem, 0, len(group))
			for _, item := range group {
				details = append(details, historyGroupDetailItem(item))
			}
			return details
		}()
		result = append(result, latest)
		index = nextIndex
	}

	for index := range result {
		result[index].OrderIndex = int64(index + 1)
	}
	return result
}

func (m *Manager) defaultCodexSyncMode() SyncMode {
	if m.cfg.DefaultCodexSyncMode != nil {
		return normalizeSyncMode(string(m.cfg.DefaultCodexSyncMode()))
	}
	return SyncModeFast
}

func (m *Manager) shouldPreserveExistingHistoryOnFastSync(session tables.WebSessionTable) bool {
	if normalizeSyncMode(session.LastSyncMode) == SyncModeDeep {
		return true
	}
	return session.LastEventSeq > 0
}

func (m *Manager) syncSessionFromSource(
	ctx context.Context,
	sessionID string,
	mode SyncMode,
	force bool,
	clearExisting bool,
) (SessionSnapshot, error) {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	agent := normalizeAgent(Agent(session.Agent))
	if agent == AgentClaude {
		return m.syncClaudeSessionFromSource(ctx, session, force, clearExisting)
	}
	if agent != AgentCodex {
		return SessionSnapshot{}, fmt.Errorf("sync is only supported for codex or claude sessions")
	}
	if session.NativeSessionID == nil || strings.TrimSpace(*session.NativeSessionID) == "" {
		return SessionSnapshot{}, fmt.Errorf("session has no native thread id")
	}
	mode = normalizeSyncMode(string(mode))

	now := time.Now()
	_ = m.updateRuntimeState(ctx, sessionID, map[string]any{
		"sync_state": SyncStateSyncing,
		"sync_error": nil,
		"updated_at": now,
	})

	var snapshot SessionSnapshot
	switch mode {
	case SyncModeDeep:
		snapshot, err = m.syncSessionFromLogSource(ctx, session, force, clearExisting)
	default:
		snapshot, err = m.syncSessionFromThreadSource(ctx, session, force, clearExisting)
		if err != nil || snapshot.History.Total == 0 {
			fallbackSnapshot, fallbackErr := m.syncSessionFromLogSource(ctx, session, force, clearExisting)
			if fallbackErr == nil {
				snapshot = fallbackSnapshot
				err = nil
			} else if err == nil {
				err = fallbackErr
			}
		}
	}
	if err != nil {
		_ = m.updateRuntimeState(ctx, sessionID, map[string]any{
			"sync_state": SyncStateError,
			"sync_error": err.Error(),
			"updated_at": time.Now(),
		})
		return SessionSnapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) syncSessionFromThreadSource(
	ctx context.Context,
	session tables.WebSessionTable,
	force bool,
	clearExisting bool,
) (SessionSnapshot, error) {
	remote, err := m.readCodexThread(ctx, session, strings.TrimSpace(*session.NativeSessionID))
	if err != nil {
		return SessionSnapshot{}, err
	}

	metadataUpdates := map[string]any{
		"source_kind":       string(defaultSessionBackend(AgentCodex)),
		"native_session_id": nilIfEmpty(remote.Summary.ID),
		"source_created_at": remote.Summary.CreatedAt,
		"source_updated_at": remote.Summary.UpdatedAt,
		"last_synced_at":    time.Now(),
		"sync_state":        SyncStateFresh,
		"sync_error":        nil,
		"thread_path":       nilIfEmpty(remote.Summary.Path),
		"thread_preview":    nilIfEmpty(remote.Summary.Preview),
		"updated_at":        time.Now(),
	}
	applySessionGoalUpdates(metadataUpdates, remote.Goal)
	if force && remote.Summary.UpdatedAt != nil {
		metadataUpdates["activity_at"] = *remote.Summary.UpdatedAt
	}

	if !clearExisting && m.shouldPreserveExistingHistoryOnFastSync(session) {
		if err := m.updateRuntimeState(ctx, session.ID, metadataUpdates); err != nil {
			return SessionSnapshot{}, err
		}
		refreshed, err := m.GetSession(ctx, session.ID)
		if err != nil {
			return SessionSnapshot{}, err
		}
		return m.loadSnapshotLocal(ctx, refreshed, DefaultHistoryWindow, false)
	}

	turnRows := make([]tables.WebSessionTurnTable, 0, len(remote.Turns))
	historyItems := make([]HistoryItem, 0)

	for turnIndex, turn := range remote.Turns {
		turnRow := tables.WebSessionTurnTable{}
		turnRow.Init()
		turnRow.WebSessionID = session.ID
		turnID := strings.TrimSpace(stringValue(turn["id"]))
		turnRow.SourceTurnID = nilIfEmptyHistory(turnID)
		turnRow.OrderIndex = int64(turnIndex + 1)
		turnRow.Status = firstNonEmpty(stringValue(turn["status"]), "completed")
		turnRow.ErrorJSON = mustJSONText(turn["error"])
		turnRow.SourceCreated = true
		turnRows = append(turnRows, turnRow)

		items := decodeRawArray(turn["items"])
		for _, rawItem := range items {
			historyItem, itemErr := m.mapThreadReadItem(rawItem, 0)
			if itemErr != nil {
				return SessionSnapshot{}, itemErr
			}
			if strings.TrimSpace(historyItem.ID) == "" &&
				strings.TrimSpace(historyItem.Kind) == "" &&
				strings.TrimSpace(historyItem.ItemType) == "" &&
				strings.TrimSpace(historyItem.Text) == "" &&
				historyItem.Tool == nil &&
				len(historyItem.Attachments) == 0 {
				continue
			}
			historyItem.SourceTurnID = nilIfEmptyHistory(turnID)
			historyItems = append(historyItems, historyItem)
		}
	}

	historyItems = compactSyncedHistoryItems(historyItems)
	itemRows := make([]tables.WebSessionItemTable, 0, len(historyItems))
	turnIDToRowID := make(map[string]string, len(turnRows))
	for _, turnRow := range turnRows {
		if turnRow.SourceTurnID != nil {
			turnIDToRowID[*turnRow.SourceTurnID] = turnRow.ID
		}
	}
	for _, historyItem := range historyItems {
		row := tables.WebSessionItemTable{}
		row.Init()
		row.WebSessionID = session.ID
		if historyItem.SourceTurnID != nil {
			if rowID, ok := turnIDToRowID[*historyItem.SourceTurnID]; ok {
				row.WebTurnID = &rowID
			}
		}
		applyHistoryItemToRow(&row, session.ID, historyItem)
		itemRows = append(itemRows, row)
	}
	if len(itemRows) == 0 {
		return m.syncSessionFromLogSource(ctx, session, force, clearExisting)
	}

	updates := cloneMap(metadataUpdates)
	updates["last_sync_mode"] = string(SyncModeFast)
	updates["turn_count"] = len(turnRows)
	updates["item_count"] = len(itemRows)
	updates["last_event_seq"] = 0
	if err := m.store.deleteSessionFiles(session.ID); err != nil {
		return SessionSnapshot{}, err
	}
	if err := m.replaceSessionHistoryCache(ctx, session, turnRows, itemRows, updates); err != nil {
		return SessionSnapshot{}, err
	}
	refreshed, err := m.GetSession(ctx, session.ID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	return m.loadSnapshotLocal(ctx, refreshed, DefaultHistoryWindow, false)
}

func (m *Manager) SyncSession(ctx context.Context, sessionID string) (SessionSnapshot, error) {
	return m.syncSessionFromSource(ctx, sessionID, m.defaultCodexSyncMode(), true, false)
}

func (m *Manager) SyncSessionWithMode(
	ctx context.Context,
	sessionID string,
	mode SyncMode,
	clearExisting bool,
) (SessionSnapshot, error) {
	return m.syncSessionFromSource(ctx, sessionID, mode, true, clearExisting)
}

func (m *Manager) refreshSessionSourceStates(
	_ context.Context,
	records []tables.WebSessionTable,
) []tables.WebSessionTable {
	// Passive "source is newer than cache" polling was removed because it mostly
	// added noisy stale markers while sessions were actively running.
	return records
}

func (m *Manager) syncSessionIfCacheMissing(
	ctx context.Context,
	session tables.WebSessionTable,
) error {
	if session.NativeSessionID == nil || strings.TrimSpace(*session.NativeSessionID) == "" {
		return nil
	}
	if normalizeAgent(Agent(session.Agent)) != AgentCodex {
		return nil
	}
	if session.ItemCount > 0 {
		return nil
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, cacheSyncReadTimeout)
	defer cancel()
	_, err := m.syncSessionFromSource(timeoutCtx, session.ID, m.defaultCodexSyncMode(), false, false)
	return err
}
