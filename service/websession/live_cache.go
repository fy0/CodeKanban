package websession

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func historyAttachmentsFromEventPayload(payload map[string]any) []HistoryAttachment {
	rawItems := decodeRawArray(payload["atts"])
	if len(rawItems) == 0 {
		return nil
	}
	items := make([]HistoryAttachment, 0, len(rawItems))
	for _, record := range rawItems {
		items = append(items, HistoryAttachment{
			ID:   stringValue(record["id"]),
			Name: stringValue(record["name"]),
			Mime: stringValue(record["mime"]),
			Size: int64(numberValue(record["sz"])),
			Path: stringValue(record["path"]),
		})
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func historyGroupDetailItemFromPayload(
	payload map[string]any,
	timestamp time.Time,
	status string,
) CommandExecutionGroupItem {
	tool := historyToolFromEventPayload(payload, status)
	detail := CommandExecutionGroupItem{
		ToolID:  firstNonEmpty(stringValue(payload["tid"]), tool.ID),
		Kind:    firstNonEmpty(tool.Kind, "tool"),
		Title:   firstNonEmpty(tool.Name, compactToolTitle(tool.Kind)),
		Status:  status,
		Input:   tool.Input,
		Output:  tool.Output,
		Summary: compactToolSummary(tool.Kind, tool.Input, tool.Meta, tool.Output),
		Command: compactToolSummary(tool.Kind, tool.Input, tool.Meta, tool.Output),
	}
	if commandInput := decodeRawObject(tool.Input); strings.TrimSpace(stringValue(commandInput["command"])) != "" {
		detail.Command = stringValue(commandInput["command"])
	}
	if !timestamp.IsZero() {
		detail.Timestamp = timestamp
	}
	if status == "running" {
		detail.StartedAt = timestamp
	} else {
		detail.CompletedAt = timestamp
	}
	return detail
}

func mergeHistoryToolPayload(existingPayload, nextPayload map[string]any) map[string]any {
	merged := cloneMap(existingPayload)
	if merged == nil {
		merged = make(map[string]any)
	}
	for key, value := range nextPayload {
		merged[key] = value
	}

	existingMeta := decodeRawObject(existingPayload["meta"])
	nextMeta := decodeRawObject(nextPayload["meta"])
	if len(existingMeta) > 0 || len(nextMeta) > 0 {
		meta := cloneMap(existingMeta)
		if meta == nil {
			meta = make(map[string]any)
		}
		for key, value := range nextMeta {
			if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
				if _, exists := meta[key]; exists {
					continue
				}
			}
			meta[key] = value
		}
		merged["meta"] = meta
	}

	if _, ok := nextPayload["in"]; !ok {
		if value, ok := existingPayload["in"]; ok {
			merged["in"] = value
		}
	}

	return merged
}

func mergeHistoryGroupItems(
	existing []CommandExecutionGroupItem,
	next CommandExecutionGroupItem,
) []CommandExecutionGroupItem {
	merged := make([]CommandExecutionGroupItem, 0, max(len(existing), 1))
	replaced := false
	for _, item := range existing {
		if item.ToolID != next.ToolID {
			merged = append(merged, item)
			continue
		}
		replaced = true
		combined := item
		combined.Kind = firstNonEmpty(next.Kind, item.Kind)
		combined.Title = firstNonEmpty(next.Title, item.Title)
		combined.Summary = firstNonEmpty(next.Summary, item.Summary)
		combined.Command = firstNonEmpty(next.Command, item.Command)
		if next.Input != nil {
			combined.Input = next.Input
		}
		if strings.TrimSpace(next.Output) != "" {
			combined.Output = next.Output
		}
		combined.Status = next.Status
		if !next.Timestamp.IsZero() {
			combined.Timestamp = next.Timestamp
		}
		if !next.StartedAt.IsZero() {
			combined.StartedAt = next.StartedAt
		}
		if !next.CompletedAt.IsZero() {
			combined.CompletedAt = next.CompletedAt
		}
		merged = append(merged, combined)
	}
	if !replaced {
		merged = append(merged, next)
	}
	return merged
}

func mergeHistoryGroupItemLists(
	existing []CommandExecutionGroupItem,
	next []CommandExecutionGroupItem,
) []CommandExecutionGroupItem {
	merged := append([]CommandExecutionGroupItem(nil), existing...)
	for _, item := range next {
		merged = mergeHistoryGroupItems(merged, item)
	}
	return merged
}

func decodeHistoryGroupItems(raw map[string]any) []CommandExecutionGroupItem {
	groupItems, ok := raw["groupItems"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(groupItems)
	if err != nil {
		return nil
	}
	var items []CommandExecutionGroupItem
	if err := json.Unmarshal(encoded, &items); err != nil {
		return nil
	}
	return items
}

func historyToolFromEventPayload(payload map[string]any, status string) *HistoryTool {
	if len(payload) == 0 {
		return nil
	}
	meta := cloneMap(decodeRawObject(payload["meta"]))
	group := parseHistoryToolCommandGroup(meta["commandGroup"])
	return &HistoryTool{
		ID:           stringValue(payload["tid"]),
		Name:         firstNonEmpty(stringValue(payload["name"]), stringValue(meta["title"]), "Tool"),
		Kind:         firstNonEmpty(stringValue(payload["kind"]), stringValue(meta["kind"])),
		Input:        payload["in"],
		Output:       stringValue(payload["out"]),
		Status:       status,
		Meta:         meta,
		CommandGroup: group,
	}
}

func parseHistoryToolCommandGroup(raw any) *HistoryToolCommandGroup {
	record := decodeRawObject(raw)
	if len(record) == 0 {
		return nil
	}
	id := strings.TrimSpace(stringValue(record["id"]))
	if id == "" {
		return nil
	}
	return &HistoryToolCommandGroup{
		ID:           id,
		Count:        max(1, int(numberValue(record["count"]))),
		FirstSeq:     int64(numberValue(record["firstSeq"])),
		LastSeq:      int64(numberValue(record["lastSeq"])),
		LatestToolID: stringValue(record["latestToolId"]),
		Compacted:    boolValue(record["compacted"]),
	}
}

func boolValue(raw any) bool {
	value, _ := raw.(bool)
	return value
}

func summarizeHistoryQuestions(questions []toolRequestQuestion) string {
	lines := make([]string, 0, len(questions))
	for _, question := range questions {
		text := strings.TrimSpace(firstNonEmpty(question.Question, question.Header))
		if text != "" {
			lines = append(lines, text)
		}
	}
	if len(lines) == 0 {
		return "Additional input is required."
	}
	return strings.Join(lines, "\n")
}

func historyAnswerEntries(raw map[string]any, questions []toolRequestQuestion) []HistoryAnswerEntry {
	result := make([]HistoryAnswerEntry, 0)
	questionMap := make(map[string]toolRequestQuestion, len(questions))
	for _, question := range questions {
		questionMap[strings.TrimSpace(question.ID)] = question
	}
	for questionID, value := range raw {
		valuesRaw, ok := value.([]string)
		if !ok {
			continue
		}
		values := make([]string, 0, len(valuesRaw))
		for _, item := range valuesRaw {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				values = append(values, trimmed)
			}
		}
		if len(values) == 0 {
			continue
		}
		question := questionMap[strings.TrimSpace(questionID)]
		result = append(result, HistoryAnswerEntry{
			ID:     questionID,
			Label:  firstNonEmpty(question.Header, question.Question, questionID, "Submitted answer"),
			Values: values,
			Masked: question.IsSecret,
		})
	}
	return result
}

func (m *Manager) userInputRequestQuestions(ctx context.Context, sessionID string, itemID string) []toolRequestQuestion {
	db := model.GetDB()
	if db == nil {
		return nil
	}
	return m.userInputRequestQuestionsDB(ctx, db, sessionID, itemID)
}

func (m *Manager) userInputRequestQuestionsDB(
	ctx context.Context,
	db *gorm.DB,
	sessionID string,
	itemID string,
) []toolRequestQuestion {
	normalizedItemID := strings.TrimSpace(itemID)
	if normalizedItemID == "" {
		return nil
	}

	var row tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where(
			"web_session_id = ? AND item_type = ? AND (source_item_id = ? OR id = ?)",
			sessionID,
			"user_input_request",
			normalizedItemID,
			normalizedItemID,
		).
		Order("order_index DESC").
		First(&row).Error; err != nil {
		return nil
	}
	item := mapHistoryItemRowWithSession(row, sessionID)
	if item.Detail == nil {
		return nil
	}
	return cloneToolRequestQuestions(item.Detail.Questions)
}

func resolveToolHistoryKey(payload map[string]any, fallback string) string {
	if group := parseHistoryToolCommandGroup(decodeRawObject(payload["meta"])["commandGroup"]); group != nil {
		return group.ID
	}
	return strings.TrimSpace(firstNonEmpty(stringValue(payload["tid"]), fallback))
}

func shouldSkipReasoningHistoryStart(agent Agent, event Event) bool {
	return normalizeAgent(agent) == AgentCodex && isReasoningToolEvent(event)
}

func shouldSkipReasoningHistoryEnd(agent Agent, event Event) bool {
	return normalizeAgent(agent) == AgentCodex &&
		isReasoningToolEvent(event) &&
		!reasoningEventHasDisplayContent(event)
}

func (m *Manager) applyEventToHistoryCache(
	ctx context.Context,
	sessionID string,
	event Event,
) (*HistoryItem, error) {
	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}
	return m.applyEventToHistoryCacheDB(ctx, db, sessionID, event)
}

func (m *Manager) applyEventToHistoryCacheDB(
	ctx context.Context,
	db *gorm.DB,
	sessionID string,
	event Event,
) (*HistoryItem, error) {
	payload := cloneMap(event.Payload)
	agent := m.sessionAgentDB(ctx, db, sessionID)
	sourceThreadID := nilIfEmptyHistory(event.ThreadID)
	sourceTurnID := nilIfEmptyHistory(event.TurnID)
	switch event.Type {
	case "sub_agent_state":
		return nil, nil
	case "sub_agent_activity":
		agentThreadID := strings.TrimSpace(firstNonEmpty(
			stringValue(payload["agentThreadId"]),
			stringValue(payload["threadId"]),
			event.ThreadID,
		))
		path := strings.TrimSpace(stringValue(payload["path"]))
		kind := strings.TrimSpace(stringValue(payload["kind"]))
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: nilIfEmptyHistory(agentThreadID),
			SourceTurnID:   sourceTurnID,
			SourceItemID:   nilIfEmptyHistory(stringValue(payload["itemId"])),
			Kind:           "system",
			ItemType:       "sub_agent_activity",
			Text:           subAgentActivityText(kind, path, agentThreadID),
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Level:          "info",
			Payload:        payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "msg_u":
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: sourceThreadID,
			SourceTurnID:   sourceTurnID,
			Kind:           "user",
			ItemType:       "user_message",
			Text:           stringValue(payload["txt"]),
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Attachments:    historyAttachmentsFromEventPayload(payload),
			Payload:        payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "msg_a_st":
		return nil, nil
	case "txt_d":
		messageID := strings.TrimSpace(firstNonEmpty(stringValue(payload["mid"]), event.ParentID, event.ID))
		item, err := m.upsertHistoryItemBySourceIdentityDB(ctx, db, sessionID, event.ThreadID, "assistant:"+messageID, func(next *HistoryItem) {
			next.SourceThreadID = sourceThreadID
			next.SourceTurnID = sourceTurnID
			next.Kind = "assistant"
			next.ItemType = "agent_message"
			next.Text = next.Text + stringValue(payload["txt"])
			next.ObservedAt = ptr(event.Timestamp)
			if next.Timestamp == nil {
				next.Timestamp = ptr(event.Timestamp)
			}
			next.Done = false
			next.Payload = payload
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "txt_end":
		messageID := strings.TrimSpace(firstNonEmpty(stringValue(payload["mid"]), event.ParentID, event.ID))
		item, err := m.upsertHistoryItemBySourceIdentityDB(ctx, db, sessionID, event.ThreadID, "assistant:"+messageID, func(next *HistoryItem) {
			next.SourceThreadID = sourceThreadID
			next.SourceTurnID = sourceTurnID
			next.Kind = "assistant"
			next.ItemType = "agent_message"
			next.ObservedAt = ptr(event.Timestamp)
			if next.Timestamp == nil {
				next.Timestamp = ptr(event.Timestamp)
			}
			next.Done = true
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "tool_st":
		if shouldSkipReasoningHistoryStart(agent, event) {
			return nil, nil
		}
		toolKey := resolveToolHistoryKey(payload, event.ID)
		sourceKey := historyToolSourceKey(toolKey)
		if group := parseHistoryToolCommandGroup(decodeRawObject(payload["meta"])["commandGroup"]); group != nil {
			if err := m.ensureCompactGroupHistorySourceKeyDB(ctx, db, sessionID, event.ThreadID, sourceKey, group.ID); err != nil {
				return nil, err
			}
		}
		item, err := m.upsertHistoryItemBySourceIdentityDB(ctx, db, sessionID, event.ThreadID, sourceKey, func(next *HistoryItem) {
			existingPayload := cloneMap(next.Payload)
			next.SourceThreadID = sourceThreadID
			next.SourceTurnID = sourceTurnID
			next.Kind = "tool"
			next.ItemType = firstNonEmpty(stringValue(payload["kind"]), "tool")
			next.Tool = historyToolFromEventPayload(payload, "running")
			next.SourceItemID = nilIfEmptyHistory(sourceKey)
			next.ObservedAt = ptr(event.Timestamp)
			if next.Timestamp == nil {
				next.Timestamp = ptr(event.Timestamp)
			}
			next.Payload = payload
			if next.Tool != nil && isCompactToolKind(next.Tool.Kind) {
				groupItems := mergeHistoryGroupItems(
					decodeHistoryGroupItems(existingPayload),
					historyGroupDetailItemFromPayload(payload, event.Timestamp, "running"),
				)
				next.Payload["groupItems"] = groupItems
			}
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "tool_end":
		if shouldSkipReasoningHistoryEnd(agent, event) {
			return nil, nil
		}
		toolKey := resolveToolHistoryKey(payload, event.ID)
		sourceKey := historyToolSourceKey(toolKey)
		if group := parseHistoryToolCommandGroup(decodeRawObject(payload["meta"])["commandGroup"]); group != nil {
			if err := m.ensureCompactGroupHistorySourceKeyDB(ctx, db, sessionID, event.ThreadID, sourceKey, group.ID); err != nil {
				return nil, err
			}
		}
		status := "done"
		if payload["ok"] == false {
			status = "error"
		}
		item, err := m.upsertHistoryItemBySourceIdentityDB(ctx, db, sessionID, event.ThreadID, sourceKey, func(next *HistoryItem) {
			existingPayload := cloneMap(next.Payload)
			next.SourceThreadID = sourceThreadID
			next.SourceTurnID = sourceTurnID
			mergedPayload := mergeHistoryToolPayload(existingPayload, payload)
			next.Kind = "tool"
			next.ItemType = firstNonEmpty(stringValue(mergedPayload["kind"]), next.ItemType, "tool")
			next.Tool = historyToolFromEventPayload(mergedPayload, status)
			next.SourceItemID = nilIfEmptyHistory(sourceKey)
			next.ObservedAt = ptr(event.Timestamp)
			if next.Timestamp == nil {
				next.Timestamp = ptr(event.Timestamp)
			}
			next.Done = true
			next.Payload = mergedPayload
			if next.Tool != nil && isCompactToolKind(next.Tool.Kind) {
				groupItems := mergeHistoryGroupItems(
					decodeHistoryGroupItems(existingPayload),
					historyGroupDetailItemFromPayload(mergedPayload, event.Timestamp, status),
				)
				next.Payload["groupItems"] = groupItems
			}
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "approval_req":
		itemID := stringValue(payload["iid"])
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: sourceThreadID,
			SourceTurnID:   sourceTurnID,
			SourceItemID:   nilIfEmptyHistory(itemID),
			Kind:           "system",
			ItemType:       "approval_request",
			Text:           firstNonEmpty(stringValue(payload["prompt"]), "Approval required"),
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Level:          "warn",
			Detail: &HistoryDetail{
				Type:         "approval_request",
				Prompt:       stringValue(payload["prompt"]),
				ApprovalKind: stringValue(payload["kind"]),
				Command:      stringValue(payload["command"]),
			},
			Payload: payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "approval_res":
		action := firstNonEmpty(stringValue(payload["act"]), "approve")
		text := "Approval granted"
		level := "info"
		if action == "reject" {
			text = "Approval rejected"
			level = "warn"
		}
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: sourceThreadID,
			SourceTurnID:   sourceTurnID,
			Kind:           "system",
			ItemType:       "approval_response",
			Text:           text,
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Level:          level,
			Detail: &HistoryDetail{
				Type:   "approval_response",
				Prompt: stringValue(payload["prompt"]),
				Action: action,
			},
			Payload: payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "user_input_req":
		questions := decodeToolQuestions(payload["qs"])
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: sourceThreadID,
			SourceTurnID:   sourceTurnID,
			SourceItemID:   nilIfEmptyHistory(stringValue(payload["iid"])),
			Kind:           "system",
			ItemType:       "user_input_request",
			Text:           firstNonEmpty(stringValue(payload["txt"]), summarizeHistoryQuestions(questions)),
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Level:          "warn",
			Detail: &HistoryDetail{
				Type:      "user_input_request",
				Prompt:    firstNonEmpty(stringValue(payload["txt"]), summarizeHistoryQuestions(questions)),
				Questions: questions,
			},
			Payload: payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "user_input_res":
		itemID := strings.TrimSpace(stringValue(payload["iid"]))
		answers := make(map[string][]string)
		for key, value := range decodeRawObject(payload["ans"]) {
			switch typed := value.(type) {
			case []string:
				answers[key] = typed
			case []any:
				next := make([]string, 0, len(typed))
				for _, entry := range typed {
					if text := strings.TrimSpace(stringValue(entry)); text != "" {
						next = append(next, text)
					}
				}
				answers[key] = next
			}
		}
		questions := m.userInputRequestQuestionsDB(ctx, db, sessionID, itemID)
		answerEntries := historyAnswerEntries(answersToRawMap(answers), questions)
		text := "Submitted requested input"
		level := "info"
		if errText := strings.TrimSpace(stringValue(payload["err"])); errText != "" {
			text = errText
			level = "warn"
		}
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: sourceThreadID,
			SourceTurnID:   sourceTurnID,
			SourceItemID:   nilIfEmptyHistory(itemID),
			Kind:           "system",
			ItemType:       "user_input_response",
			Text:           text,
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Level:          level,
			Detail: &HistoryDetail{
				Type:    "user_input_response",
				Answers: answerEntries,
			},
			Payload: payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "note":
		level := firstNonEmpty(stringValue(payload["lvl"]), "info")
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: sourceThreadID,
			SourceTurnID:   sourceTurnID,
			Kind:           "system",
			ItemType:       "note",
			Text:           stringValue(payload["txt"]),
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Level:          level,
			Payload:        payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "run_fail":
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: sourceThreadID,
			SourceTurnID:   sourceTurnID,
			Kind:           "system",
			ItemType:       "run_fail",
			Text:           firstNonEmpty(stringValue(payload["msg"]), "Run failed"),
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Level:          "error",
			Payload:        payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	case "run_abort":
		item, err := m.appendHistoryItemDB(ctx, db, sessionID, HistoryItem{
			SourceThreadID: sourceThreadID,
			SourceTurnID:   sourceTurnID,
			Kind:           "system",
			ItemType:       "run_abort",
			Text:           firstNonEmpty(stringValue(payload["msg"]), "Run aborted"),
			Timestamp:      ptr(event.Timestamp),
			ObservedAt:     ptr(event.Timestamp),
			Level:          "info",
			Payload:        payload,
		})
		if err != nil {
			return nil, err
		}
		return &item, nil
	default:
		return nil, nil
	}
}

func answersToRawMap(answers map[string][]string) map[string]any {
	result := make(map[string]any, len(answers))
	for key, value := range answers {
		result[key] = value
	}
	return result
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func (m *Manager) summaryForBroadcast(ctx context.Context, sessionID string) *SessionSummary {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return nil
	}
	// Archived sessions remain queryable via explicit snapshot/history APIs,
	// but they should not continue to emit live websocket updates.
	if record.ArchivedAt != nil {
		return nil
	}
	summaries := []SessionSummary{m.mapSessionSummary(record)}
	if err := m.decorateScheduledPlanExecutionState(ctx, summaries); err != nil {
		return nil
	}
	return &summaries[0]
}

func (m *Manager) maybeSyncSessionAfterRun(session tables.WebSessionTable) {
	agent := normalizeAgent(Agent(session.Agent))
	if agent != AgentCodex {
		return
	}
	if session.NativeSessionID == nil || strings.TrimSpace(*session.NativeSessionID) == "" {
		return
	}
	go func() {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := m.syncSessionFromSource(timeoutCtx, session.ID, m.defaultCodexSyncMode(), true, false); err != nil {
			if m.logger != nil {
				m.logger.Debug("failed to refresh session cache after run",
					zap.String("sessionId", session.ID),
					zap.Error(err),
				)
			}
			return
		}
		_ = m.broadcastSnapshot(context.Background(), session.ID)
	}()
}
