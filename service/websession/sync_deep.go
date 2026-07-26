package websession

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2/log_watcher"

	"go.uber.org/zap"
)

type codexLogSource struct {
	FilePath         string
	SessionStartedAt time.Time
	LastMessageAt    *time.Time
}

type codexTokenUsageSnapshot struct {
	InputTokens       int64
	CachedInputTokens int64
	OutputTokens      int64
	TotalTokens       int64
}

type codexDeepHistoryStats struct {
	HasTotalUsage                  bool
	TotalUsage                     codexTokenUsageSnapshot
	LatestTokenCount               codexTokenUsageSnapshot
	LatestTokenCountUpdatedAt      *time.Time
	SessionContextWindowTokens     int64
	SessionContextWindowObservedAt *time.Time
	LastContextCompactionAt        *time.Time
	HasContextBaseline             bool
	ContextBaseline                codexTokenUsageSnapshot
	CyberPolicyFlagged             bool
}

type codexDeepHistoryParseResult struct {
	Items []HistoryItem
	Stats codexDeepHistoryStats
}

type codexDeepMessageOrigin uint8

const (
	codexDeepMessageOriginEvent codexDeepMessageOrigin = 1 << iota
	codexDeepMessageOriginResponse
	codexDeepMessageDedupeWindow = 2 * time.Second
)

var codexEmbeddedImageEnvelopePattern = regexp.MustCompile(`(?s)[\r\n]*<image\b[^>]*\bpath="([^"]+)"[^>]*>\s*\[input_image:\]\s*</image>`)

func (m *Manager) syncSessionFromLogSource(
	ctx context.Context,
	session tables.WebSessionTable,
	force bool,
	clearExisting bool,
) (SessionSnapshot, error) {
	source, err := m.resolveCodexLogSource(ctx, session)
	if err != nil {
		return SessionSnapshot{}, err
	}

	parseResult, err := m.parseCodexDeepHistoryWithStats(source.FilePath)
	if err != nil {
		return SessionSnapshot{}, err
	}
	rootThreadID := ""
	if session.NativeSessionID != nil {
		rootThreadID = strings.TrimSpace(*session.NativeSessionID)
	}
	items := parseResult.Items
	for index := range items {
		if items[index].SourceThreadID == nil {
			items[index].SourceThreadID = nilIfEmptyHistory(rootThreadID)
		}
	}
	descendants := make([]codexThreadReadResult, 0)
	descendantsDiscovered := false
	if rootThreadID != "" {
		var descendantsErr error
		descendants, descendantsErr = m.readCodexDescendantThreads(ctx, session, rootThreadID)
		descendantsDiscovered = descendantsErr == nil
		if descendantsErr != nil && m.logger != nil {
			m.logger.Warn(
				"codex descendant discovery unavailable during deep sync; preserving cached sub agents",
				zap.String("sessionId", session.ID),
				zap.Error(descendantsErr),
			)
		}
	}
	for _, descendant := range descendants {
		path := strings.TrimSpace(descendant.Summary.Path)
		if path == "" {
			if m.logger != nil {
				m.logger.Warn(
					"codex descendant has no rollout path during deep sync",
					zap.String("sessionId", session.ID),
					zap.String("threadId", descendant.Summary.ID),
				)
			}
			continue
		}
		childResult, childErr := m.parseCodexDeepHistoryWithStats(path)
		if childErr != nil {
			return SessionSnapshot{}, fmt.Errorf("parse descendant %s: %w", descendant.Summary.ID, childErr)
		}
		for index := range childResult.Items {
			if childResult.Items[index].SourceThreadID == nil {
				childResult.Items[index].SourceThreadID = nilIfEmptyHistory(descendant.Summary.ID)
			}
		}
		items = append(items, childResult.Items...)
	}
	sortSyncedHistoryItems(items)
	items = compactSyncedHistoryItems(items)
	subAgents := subAgentsFromCodexThreads(descendants)
	updateSubAgentsFromHistory(subAgents, items)

	itemRows := make([]tables.WebSessionItemTable, 0, len(items))
	for _, item := range items {
		row := tables.WebSessionItemTable{}
		row.Init()
		row.WebSessionID = session.ID
		applyHistoryItemToRow(&row, session.ID, item)
		itemRows = append(itemRows, row)
	}

	var sourceCreatedAt *time.Time
	if !source.SessionStartedAt.IsZero() {
		value := source.SessionStartedAt
		sourceCreatedAt = &value
	} else if session.SourceCreatedAt != nil {
		value := *session.SourceCreatedAt
		sourceCreatedAt = &value
	}
	var sourceUpdatedAt *time.Time
	if info, statErr := os.Stat(source.FilePath); statErr == nil {
		value := info.ModTime()
		sourceUpdatedAt = &value
	} else if source.LastMessageAt != nil {
		value := *source.LastMessageAt
		sourceUpdatedAt = &value
	}

	updates := map[string]any{
		"source_kind":                            string(defaultSessionBackend(AgentCodex)),
		"source_created_at":                      sourceCreatedAt,
		"source_updated_at":                      sourceUpdatedAt,
		"last_synced_at":                         time.Now(),
		"sync_state":                             SyncStateFresh,
		"sync_error":                             nil,
		"last_sync_mode":                         string(SyncModeDeep),
		"turn_count":                             0,
		"item_count":                             len(itemRows),
		"last_event_seq":                         0,
		"updated_at":                             time.Now(),
		"latest_token_count_input_tokens":        0,
		"latest_token_count_cached_input_tokens": 0,
		"latest_token_count_output_tokens":       0,
		"latest_token_count_total_tokens":        0,
		"latest_token_count_updated_at":          nil,
		"session_context_window_tokens":          0,
		"session_context_window_observed_at":     nil,
	}
	applyCodexDeepHistoryStatsUpdates(updates, parseResult.Stats)
	if force {
		if latest := latestHistoryItemTimestamp(items); latest != nil {
			updates["activity_at"] = *latest
		} else if sourceUpdatedAt != nil {
			updates["activity_at"] = *sourceUpdatedAt
		}
	}

	if err := m.store.deleteSessionFiles(session.ID); err != nil {
		return SessionSnapshot{}, err
	}
	if err := m.replaceSessionHistoryCache(ctx, session, nil, itemRows, updates); err != nil {
		return SessionSnapshot{}, err
	}
	if descendantsDiscovered {
		if err := m.replaceSessionSubAgents(ctx, session.ID, subAgents, true); err != nil {
			return SessionSnapshot{}, err
		}
	}
	refreshed, err := m.GetSession(ctx, session.ID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	return m.loadSnapshotLocal(ctx, refreshed, DefaultHistoryWindow, false)
}

func (m *Manager) resolveCodexLogSource(
	ctx context.Context,
	session tables.WebSessionTable,
) (codexLogSource, error) {
	if session.ThreadPath != nil {
		path := strings.TrimSpace(*session.ThreadPath)
		if path != "" {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				source := codexLogSource{
					FilePath: path,
				}
				if session.SourceCreatedAt != nil {
					source.SessionStartedAt = *session.SourceCreatedAt
				}
				if session.LastMessageAt != nil {
					value := *session.LastMessageAt
					source.LastMessageAt = &value
				}
				return source, nil
			}
		}
	}
	if m.aiSessionSvc == nil {
		return codexLogSource{}, fmt.Errorf("ai session service is not configured")
	}
	record, err := m.aiSessionSvc.ResolveCodexSessionBySessionID(ctx, strings.TrimSpace(*session.NativeSessionID))
	if err != nil {
		return codexLogSource{}, err
	}
	return codexLogSource{
		FilePath:         record.FilePath,
		SessionStartedAt: record.SessionStartedAt,
		LastMessageAt:    record.LastMessageAt,
	}, nil
}

func (m *Manager) parseCodexDeepHistory(filePath string) ([]HistoryItem, error) {
	result, err := m.parseCodexDeepHistoryWithStats(filePath)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (m *Manager) parseCodexDeepHistoryWithStats(filePath string) (codexDeepHistoryParseResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return codexDeepHistoryParseResult{}, err
	}
	defer file.Close()

	items := make([]HistoryItem, 0, 256)
	pendingTools := make(map[string]int)
	pendingUserInputs := make(map[string]int)
	messageOrigins := make(map[string]codexDeepMessageOrigin)
	var orderIndex int64
	var stats codexDeepHistoryStats

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	appendItem := func(item HistoryItem) int {
		orderIndex++
		if strings.TrimSpace(item.ID) == "" {
			item.ID = fmt.Sprintf("deep_%d", orderIndex)
		}
		item.OrderIndex = orderIndex
		items = append(items, item)
		return len(items) - 1
	}

	appendMessage := func(item HistoryItem, origin codexDeepMessageOrigin) int {
		itemTime := codexDeepMessageTime(item)
		for index := len(items) - 1; index >= 0; index-- {
			candidate := items[index]
			candidateTime := codexDeepMessageTime(candidate)
			if itemTime != nil && candidateTime != nil && itemTime.Sub(*candidateTime) > codexDeepMessageDedupeWindow {
				break
			}
			candidateOrigin := messageOrigins[candidate.ID]
			if candidateOrigin == 0 || candidateOrigin&origin != 0 {
				continue
			}
			if !codexDeepMessagesMatch(candidate, item) {
				continue
			}
			mergeCodexDeepMessage(&items[index], item, candidateOrigin, origin)
			messageOrigins[candidate.ID] = candidateOrigin | origin
			return index
		}
		index := appendItem(item)
		messageOrigins[item.ID] = origin
		return index
	}

	appendIfNotDuplicate := func(item HistoryItem) {
		if item.ItemType == "context_compaction" && len(items) > 0 {
			last := items[len(items)-1]
			if shouldDedupeContextCompactionItems(last, item) {
				mergeContextCompactionHistoryItem(&items[len(items)-1], item)
				return
			}
		}
		if item.Tool == nil && len(items) > 0 {
			last := items[len(items)-1]
			if last.Kind == item.Kind && last.ItemType == item.ItemType && last.Text == item.Text {
				return
			}
		}
		appendItem(item)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry log_watcher.LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, entry.Timestamp)

		switch entry.Type {
		case "compacted":
			stats.observeContextCompaction(ts)
			appendIfNotDuplicate(codexContextCompactionHistoryItem(decodeRawObject(entry.Payload), ts))
		case "event_msg":
			payload, ok := entry.Payload.(map[string]any)
			if !ok {
				continue
			}
			stats.observeEventMessage(payload, ts)
			for _, item := range m.codexHistoryItemsFromEventMessage(payload, ts) {
				if isCodexDeepMessage(item) {
					appendMessage(item, codexDeepMessageOriginEvent)
					continue
				}
				appendIfNotDuplicate(item)
			}
		case "response_item":
			payload, ok := entry.Payload.(map[string]any)
			if !ok {
				continue
			}
			if normalizeCodexItemType(stringValue(payload["type"])) == "context_compaction" {
				stats.observeContextCompaction(ts)
			}
			m.applyCodexResponseItem(
				&items,
				payload,
				ts,
				appendItem,
				appendMessage,
				pendingTools,
				pendingUserInputs,
			)
		}
	}
	if err := scanner.Err(); err != nil {
		return codexDeepHistoryParseResult{}, err
	}

	items = dedupeCodexDeepPlanMessages(items)
	for index := range items {
		items[index].OrderIndex = int64(index + 1)
	}
	return codexDeepHistoryParseResult{
		Items: items,
		Stats: stats,
	}, nil
}

func isCodexDeepMessage(item HistoryItem) bool {
	return item.Tool == nil && (item.Kind == "user" || item.Kind == "assistant")
}

func normalizedCodexDeepMessageText(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
}

func codexDeepMessageTime(item HistoryItem) *time.Time {
	if item.Timestamp != nil {
		return item.Timestamp
	}
	return item.ObservedAt
}

func codexDeepMessageAttachmentsCompatible(left, right []HistoryAttachment) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	if len(left) != len(right) {
		return false
	}
	keys := make(map[string]struct{}, len(left))
	for _, attachment := range left {
		key := firstNonEmpty(strings.TrimSpace(attachment.ID), strings.TrimSpace(attachment.Path))
		keys[key] = struct{}{}
	}
	for _, attachment := range right {
		key := firstNonEmpty(strings.TrimSpace(attachment.ID), strings.TrimSpace(attachment.Path))
		if _, ok := keys[key]; !ok {
			return false
		}
	}
	return true
}

func codexDeepMessagesMatch(left, right HistoryItem) bool {
	if !isCodexDeepMessage(left) || !isCodexDeepMessage(right) || left.Kind != right.Kind {
		return false
	}
	if normalizedCodexDeepMessageText(left.Text) != normalizedCodexDeepMessageText(right.Text) {
		return false
	}
	if !codexDeepMessageAttachmentsCompatible(left.Attachments, right.Attachments) {
		return false
	}
	leftTime := codexDeepMessageTime(left)
	rightTime := codexDeepMessageTime(right)
	if leftTime == nil || rightTime == nil {
		return false
	}
	delta := leftTime.Sub(*rightTime)
	if delta < 0 {
		delta = -delta
	}
	return delta <= codexDeepMessageDedupeWindow
}

func mergeHistoryAttachments(left, right []HistoryAttachment) []HistoryAttachment {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	result := append([]HistoryAttachment{}, left...)
	seen := make(map[string]struct{}, len(result))
	for _, attachment := range result {
		key := firstNonEmpty(strings.TrimSpace(attachment.ID), strings.TrimSpace(attachment.Path))
		seen[key] = struct{}{}
	}
	for _, attachment := range right {
		key := firstNonEmpty(strings.TrimSpace(attachment.ID), strings.TrimSpace(attachment.Path))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, attachment)
	}
	return result
}

func mergeCodexDeepMessage(
	target *HistoryItem,
	source HistoryItem,
	targetOrigin codexDeepMessageOrigin,
	sourceOrigin codexDeepMessageOrigin,
) {
	if target == nil {
		return
	}
	if sourceOrigin == codexDeepMessageOriginEvent {
		target.Text = source.Text
		target.Attachments = mergeHistoryAttachments(source.Attachments, target.Attachments)
	} else {
		target.Attachments = mergeHistoryAttachments(target.Attachments, source.Attachments)
	}
	if sourceOrigin == codexDeepMessageOriginResponse || targetOrigin != codexDeepMessageOriginResponse {
		if source.SourceTurnID != nil {
			target.SourceTurnID = source.SourceTurnID
		}
		if source.SourceItemID != nil {
			target.SourceItemID = source.SourceItemID
		}
		if source.Payload != nil {
			target.Payload = source.Payload
		}
	}
	target.Done = target.Done || source.Done
	if target.Timestamp == nil || source.Timestamp != nil && source.Timestamp.Before(*target.Timestamp) {
		target.Timestamp = source.Timestamp
	}
	if target.ObservedAt == nil || source.ObservedAt != nil && source.ObservedAt.After(*target.ObservedAt) {
		target.ObservedAt = source.ObservedAt
	}
}

func codexProposedPlanText(text string) (string, bool) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	const openTag = "<proposed_plan>"
	const closeTag = "</proposed_plan>"
	if !strings.HasPrefix(trimmed, openTag) || !strings.HasSuffix(trimmed, closeTag) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, openTag), closeTag)), true
}

func codexDeepItemsShareTurnOrTime(left, right HistoryItem) bool {
	if left.SourceTurnID != nil && right.SourceTurnID != nil &&
		strings.TrimSpace(*left.SourceTurnID) != "" && *left.SourceTurnID == *right.SourceTurnID {
		return true
	}
	leftTime := codexDeepMessageTime(left)
	rightTime := codexDeepMessageTime(right)
	if leftTime == nil || rightTime == nil {
		return false
	}
	delta := leftTime.Sub(*rightTime)
	if delta < 0 {
		delta = -delta
	}
	return delta <= codexDeepMessageDedupeWindow
}

func dedupeCodexDeepPlanMessages(items []HistoryItem) []HistoryItem {
	if len(items) == 0 {
		return items
	}
	result := make([]HistoryItem, 0, len(items))
	for index, item := range items {
		planText, proposed := codexProposedPlanText(item.Text)
		if item.Kind == "assistant" && proposed {
			duplicate := false
			for candidateIndex, candidate := range items {
				if candidateIndex == index || candidate.Tool == nil || candidate.Tool.Kind != "plan" {
					continue
				}
				if normalizedCodexDeepMessageText(candidate.Tool.Output) == normalizedCodexDeepMessageText(planText) &&
					codexDeepItemsShareTurnOrTime(candidate, item) {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
		}
		result = append(result, item)
	}
	return result
}

func latestHistoryItemTimestamp(items []HistoryItem) *time.Time {
	var latest *time.Time
	for _, item := range items {
		candidate := item.ObservedAt
		if candidate == nil {
			candidate = item.Timestamp
		}
		if candidate == nil {
			continue
		}
		if latest == nil || candidate.After(*latest) {
			value := *candidate
			latest = &value
		}
	}
	return latest
}

func applyCodexDeepHistoryStatsUpdates(updates map[string]any, stats codexDeepHistoryStats) {
	if updates == nil {
		return
	}
	if stats.HasTotalUsage {
		updates["total_input_tokens"] = stats.TotalUsage.InputTokens
		updates["total_cached_input_tokens"] = stats.TotalUsage.CachedInputTokens
		updates["total_output_tokens"] = stats.TotalUsage.OutputTokens
	}
	if stats.LatestTokenCountUpdatedAt != nil {
		updates["latest_token_count_input_tokens"] = stats.LatestTokenCount.InputTokens
		updates["latest_token_count_cached_input_tokens"] = stats.LatestTokenCount.CachedInputTokens
		updates["latest_token_count_output_tokens"] = stats.LatestTokenCount.OutputTokens
		updates["latest_token_count_total_tokens"] = stats.LatestTokenCount.TotalTokens
		updates["latest_token_count_updated_at"] = *stats.LatestTokenCountUpdatedAt
	}
	if stats.SessionContextWindowTokens > 0 {
		updates["session_context_window_tokens"] = stats.SessionContextWindowTokens
		if stats.SessionContextWindowObservedAt != nil {
			updates["session_context_window_observed_at"] = *stats.SessionContextWindowObservedAt
		}
	}
	if stats.LastContextCompactionAt != nil {
		updates["last_context_compaction_at"] = *stats.LastContextCompactionAt
		if stats.HasContextBaseline {
			updates["context_baseline_input_tokens"] = stats.ContextBaseline.InputTokens
			updates["context_baseline_cached_input_tokens"] = stats.ContextBaseline.CachedInputTokens
			updates["context_baseline_output_tokens"] = stats.ContextBaseline.OutputTokens
		}
	}
	updates["cyber_policy_flagged"] = stats.CyberPolicyFlagged
}

func (stats *codexDeepHistoryStats) observeEventMessage(payload map[string]any, ts time.Time) {
	if stats == nil {
		return
	}
	stats.observeModelContextWindow(payload["model_context_window"], ts)
	switch strings.TrimSpace(stringValue(payload["type"])) {
	case "token_count":
		stats.observeTokenCount(payload, ts)
	case "context_compacted":
		stats.observeContextCompaction(ts)
	case "error":
		if isCodexCyberPolicyError(payload) {
			stats.CyberPolicyFlagged = true
		}
	}
}

func (stats *codexDeepHistoryStats) observeTokenCount(payload map[string]any, ts time.Time) {
	info := decodeRawObject(payload["info"])
	if len(info) == 0 {
		return
	}
	stats.observeModelContextWindow(info["model_context_window"], ts)
	if totalUsage, ok := parseCodexTokenUsageSnapshot(info["total_token_usage"]); ok {
		stats.HasTotalUsage = true
		stats.TotalUsage = totalUsage
	}
	if lastUsage, ok := parseCodexTokenUsageSnapshot(info["last_token_usage"]); ok {
		stats.LatestTokenCount = lastUsage
		stats.LatestTokenCountUpdatedAt = ptr(ts)
	}
}

func (stats *codexDeepHistoryStats) observeModelContextWindow(raw any, ts time.Time) {
	if stats == nil {
		return
	}
	value, ok := codexInt64Field(map[string]any{"value": raw}, "value")
	if !ok || value <= 0 {
		return
	}
	stats.SessionContextWindowTokens = value
	stats.SessionContextWindowObservedAt = ptr(ts)
}

func (stats *codexDeepHistoryStats) observeContextCompaction(ts time.Time) {
	if stats == nil || ts.IsZero() {
		return
	}
	if stats.LastContextCompactionAt != nil &&
		absDuration(ts.Sub(*stats.LastContextCompactionAt)) <= 5*time.Second {
		if ts.After(*stats.LastContextCompactionAt) {
			stats.LastContextCompactionAt = ptr(ts)
		}
		return
	}
	stats.LastContextCompactionAt = ptr(ts)
	if stats.HasTotalUsage {
		stats.ContextBaseline = stats.TotalUsage
		stats.HasContextBaseline = true
	}
}

func parseCodexTokenUsageSnapshot(raw any) (codexTokenUsageSnapshot, bool) {
	record := decodeRawObject(raw)
	if len(record) == 0 {
		return codexTokenUsageSnapshot{}, false
	}
	var snapshot codexTokenUsageSnapshot
	var seen bool
	if value, ok := codexInt64Field(record, "input_tokens", "inputTokens"); ok {
		snapshot.InputTokens = value
		seen = true
	}
	if value, ok := codexInt64Field(record, "cached_input_tokens", "cachedInputTokens"); ok {
		snapshot.CachedInputTokens = value
		seen = true
	}
	if value, ok := codexInt64Field(record, "output_tokens", "outputTokens"); ok {
		snapshot.OutputTokens = value
		seen = true
	}
	if value, ok := codexInt64Field(record, "total_tokens", "totalTokens"); ok {
		snapshot.TotalTokens = value
		seen = true
	}
	return snapshot, seen
}

func codexInt64Field(record map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		value, ok := record[key]
		if !ok {
			continue
		}
		return maxInt64(0, int64(numberValue(value))), true
	}
	return 0, false
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func shouldDedupeContextCompactionItems(previous HistoryItem, next HistoryItem) bool {
	if previous.ItemType != "context_compaction" || next.ItemType != "context_compaction" {
		return false
	}
	previousAt := historyItemObservedTimestamp(previous)
	nextAt := historyItemObservedTimestamp(next)
	if previousAt == nil || nextAt == nil {
		return true
	}
	return absDuration(nextAt.Sub(*previousAt)) <= 5*time.Second
}

func mergeContextCompactionHistoryItem(target *HistoryItem, source HistoryItem) {
	if target == nil {
		return
	}
	if strings.TrimSpace(target.Text) == "" {
		target.Text = source.Text
	}
	if target.Tool == nil {
		target.Tool = source.Tool
	} else if source.Tool != nil {
		if strings.TrimSpace(target.Tool.Output) == "" || target.Tool.Output == "Context compacted" {
			target.Tool.Output = source.Tool.Output
		}
		if len(target.Tool.Meta) == 0 {
			target.Tool.Meta = source.Tool.Meta
		}
	}
	if target.Payload == nil {
		target.Payload = source.Payload
	}
	if target.ObservedAt == nil || (source.ObservedAt != nil && source.ObservedAt.After(*target.ObservedAt)) {
		target.ObservedAt = source.ObservedAt
	}
}

func historyItemObservedTimestamp(item HistoryItem) *time.Time {
	if item.ObservedAt != nil {
		return item.ObservedAt
	}
	return item.Timestamp
}

func (m *Manager) codexHistoryItemsFromEventMessage(
	payload map[string]any,
	ts time.Time,
) []HistoryItem {
	itemType := strings.TrimSpace(stringValue(payload["type"]))
	switch itemType {
	case "user_message":
		text := strings.TrimSpace(stringValue(payload["message"]))
		attachments := m.codexHistoryAttachments(payload)
		if text == "" && len(attachments) == 0 {
			return nil
		}
		if isHiddenCodexPrompt(text) && len(attachments) == 0 {
			return nil
		}
		return []HistoryItem{{
			ID:          utils.NewID(),
			Kind:        "user",
			ItemType:    "user_message",
			Text:        text,
			Timestamp:   ptr(ts),
			ObservedAt:  ptr(ts),
			Attachments: attachments,
			Payload:     cloneMap(payload),
		}}
	case "agent_message":
		text := strings.TrimSpace(stringValue(payload["message"]))
		if text == "" {
			return nil
		}
		return []HistoryItem{{
			ID:         utils.NewID(),
			Kind:       "assistant",
			ItemType:   "agent_message",
			Text:       text,
			Timestamp:  ptr(ts),
			ObservedAt: ptr(ts),
			Done:       true,
			Payload:    cloneMap(payload),
		}}
	case "turn_aborted":
		return []HistoryItem{{
			ID:         utils.NewID(),
			Kind:       "system",
			ItemType:   "run_abort",
			Text:       firstNonEmpty(strings.TrimSpace(stringValue(payload["reason"])), "Run aborted"),
			Timestamp:  ptr(ts),
			ObservedAt: ptr(ts),
			Level:      "warn",
			Payload:    cloneMap(payload),
		}}
	case "error":
		if !isCodexCyberPolicyError(payload) {
			return nil
		}
		return []HistoryItem{{
			ID:         utils.NewID(),
			Kind:       "system",
			ItemType:   "cyber_policy",
			Text:       firstNonEmpty(codexErrorMessage(payload), codexCyberPolicyFallbackText),
			Timestamp:  ptr(ts),
			ObservedAt: ptr(ts),
			Level:      "error",
			Payload:    cloneMap(payload),
		}}
	case "context_compacted":
		return []HistoryItem{codexContextCompactionHistoryItem(payload, ts)}
	case "item_completed":
		item := decodeRawObject(payload["item"])
		plan := deepSyncPlanHistoryItem(item, stringValue(payload["turn_id"]), ts, cloneMap(payload))
		if plan == nil {
			return nil
		}
		return []HistoryItem{*plan}
	default:
		return nil
	}
}

func codexContextCompactionHistoryItem(payload map[string]any, ts time.Time) HistoryItem {
	toolID := strings.TrimSpace(stringValue(payload["id"]))
	if toolID == "" {
		toolID = utils.NewID()
	}
	output := contextCompactionHistoryOutput(payload)
	return HistoryItem{
		ID:           toolID,
		SourceItemID: ptr(toolID),
		Kind:         "tool",
		ItemType:     "context_compaction",
		Timestamp:    ptr(ts),
		ObservedAt:   ptr(ts),
		Payload:      cloneMap(payload),
		Tool: &HistoryTool{
			ID:     toolID,
			Name:   "Context Compaction",
			Kind:   "context_compaction",
			Output: output,
			Status: syncedToolStatus(firstNonEmpty(stringValue(payload["status"]), "completed")),
			Meta: map[string]any{
				"title":    "Context Compaction",
				"kind":     "context_compaction",
				"subtitle": firstNonEmpty(contextCompactionSubtitle(payload), output),
			},
		},
	}
}

func contextCompactionHistoryOutput(payload map[string]any) string {
	if strings.TrimSpace(stringValue(payload["type"])) == "context_compacted" {
		return "Context compacted"
	}
	return firstNonEmpty(extractContextCompactionText(payload), "Context compacted")
}

func deepSyncPlanHistoryItem(
	item map[string]any,
	turnID string,
	ts time.Time,
	payload map[string]any,
) *HistoryItem {
	if !strings.EqualFold(strings.TrimSpace(stringValue(item["type"])), "plan") {
		return nil
	}

	planID := strings.TrimSpace(stringValue(item["id"]))
	if planID == "" {
		planID = utils.NewID()
	}

	return &HistoryItem{
		ID:           planID,
		SourceTurnID: nilIfEmptyHistory(turnID),
		SourceItemID: nilIfEmptyHistory(planID),
		Kind:         "tool",
		ItemType:     "plan",
		Timestamp:    ptr(ts),
		ObservedAt:   ptr(ts),
		Payload:      payload,
		Tool: &HistoryTool{
			ID:     planID,
			Name:   "Plan",
			Kind:   "plan",
			Output: stringValue(item["text"]),
			Status: "done",
			Meta: map[string]any{
				"title": "Plan",
				"kind":  "plan",
			},
		},
	}
}

func (m *Manager) applyCodexResponseItem(
	items *[]HistoryItem,
	payload map[string]any,
	ts time.Time,
	appendItem func(item HistoryItem) int,
	appendMessage func(item HistoryItem, origin codexDeepMessageOrigin) int,
	pendingTools map[string]int,
	pendingUserInputs map[string]int,
) {
	responseType := strings.TrimSpace(stringValue(payload["type"]))
	switch responseType {
	case "message":
		role := strings.ToLower(strings.TrimSpace(stringValue(payload["role"])))
		if role == "developer" || role == "system" {
			return
		}
		content := decodeRawArray(payload["content"])
		textParts := make([]string, 0, len(content))
		for _, block := range content {
			blockType := strings.TrimSpace(stringValue(block["type"]))
			switch blockType {
			case "input_text", "output_text", "text":
				if text := strings.TrimSpace(stringValue(block["text"])); text != "" {
					textParts = append(textParts, text)
				}
			}
		}
		text, embeddedAttachments := m.normalizeCodexResponseMessage(strings.Join(textParts, "\n"))
		if text == "" {
			return
		}
		item := HistoryItem{
			ID:           utils.NewID(),
			SourceTurnID: nilIfEmptyHistory(codexResponseItemTurnID(payload)),
			SourceItemID: nilIfEmptyHistory(stringValue(payload["id"])),
			ItemType:     "message",
			Text:         text,
			Timestamp:    ptr(ts),
			ObservedAt:   ptr(ts),
			Attachments:  embeddedAttachments,
			Payload:      cloneMap(payload),
			Done:         true,
		}
		switch role {
		case "assistant":
			item.Kind = "assistant"
			item.ItemType = "agent_message"
		default:
			if isHiddenCodexPrompt(text) || isCodexInjectedHostContext(text) {
				return
			}
			item.Kind = "user"
			item.ItemType = "user_message"
		}
		appendMessage(item, codexDeepMessageOriginResponse)
	case "reasoning":
		text := codexReasoningSummary(payload)
		if text == "" {
			return
		}
		appendItem(HistoryItem{
			ID:         utils.NewID(),
			Kind:       "tool",
			ItemType:   "reasoning",
			Timestamp:  ptr(ts),
			ObservedAt: ptr(ts),
			Payload:    cloneMap(payload),
			Tool: &HistoryTool{
				ID:     utils.NewID(),
				Name:   "Reasoning",
				Kind:   "reasoning",
				Output: text,
				Status: "done",
				Meta: map[string]any{
					"title": "Reasoning",
					"kind":  "reasoning",
				},
			},
		})
	case "plan":
		plan := deepSyncPlanHistoryItem(payload, "", ts, cloneMap(payload))
		if plan == nil {
			return
		}
		appendItem(*plan)
	case "contextCompaction", "context_compaction":
		toolID := strings.TrimSpace(stringValue(payload["id"]))
		if toolID == "" {
			toolID = utils.NewID()
		}
		appendItem(HistoryItem{
			ID:           toolID,
			SourceItemID: ptr(toolID),
			Kind:         "tool",
			ItemType:     "context_compaction",
			Timestamp:    ptr(ts),
			ObservedAt:   ptr(ts),
			Payload:      cloneMap(payload),
			Tool: &HistoryTool{
				ID:     toolID,
				Name:   "Context Compaction",
				Kind:   "context_compaction",
				Output: extractContextCompactionText(payload),
				Status: syncedToolStatus(firstNonEmpty(stringValue(payload["status"]), "completed")),
				Meta: map[string]any{
					"title":    "Context Compaction",
					"kind":     "context_compaction",
					"subtitle": contextCompactionSubtitle(payload),
				},
			},
		})
	case "function_call":
		callID := strings.TrimSpace(stringValue(payload["call_id"]))
		if callID == "" {
			callID = utils.NewID()
		}
		toolName := strings.TrimSpace(stringValue(payload["name"]))
		input := decodeDeepSyncArguments(stringValue(payload["arguments"]))
		if toolName == "request_user_input" {
			questions := decodeToolQuestions(decodeRawObject(input)["questions"])
			prompt := summarizeToolQuestions(questions)
			index := appendItem(HistoryItem{
				ID:           callID,
				SourceItemID: ptr(callID),
				Kind:         "system",
				ItemType:     "user_input_request",
				Text:         prompt,
				Timestamp:    ptr(ts),
				ObservedAt:   ptr(ts),
				Level:        "warn",
				Detail: &HistoryDetail{
					Type:      "user_input_request",
					Prompt:    prompt,
					Questions: questions,
				},
				Payload: cloneMap(payload),
			})
			pendingUserInputs[callID] = index
			return
		}

		kind := deepSyncToolKind(toolName)
		normalizedInput := deepSyncToolInput(toolName, input)
		tool := &HistoryTool{
			ID:     callID,
			Name:   deepSyncToolDisplayName(toolName, kind),
			Kind:   kind,
			Input:  normalizedInput,
			Status: "running",
			Meta:   deepSyncToolMeta(toolName, kind, normalizedInput),
		}
		index := appendItem(HistoryItem{
			ID:           callID,
			SourceItemID: ptr(callID),
			Kind:         "tool",
			ItemType:     kind,
			Timestamp:    ptr(ts),
			ObservedAt:   ptr(ts),
			Payload:      cloneMap(payload),
			Tool:         tool,
		})
		pendingTools[callID] = index
	case "function_call_output":
		callID := strings.TrimSpace(stringValue(payload["call_id"]))
		output := strings.TrimSpace(stringValue(payload["output"]))
		if callID == "" {
			callID = utils.NewID()
		}

		if requestIndex, ok := pendingUserInputs[callID]; ok && requestIndex >= 0 && requestIndex < len(*items) {
			delete(pendingUserInputs, callID)
			response := HistoryItem{
				ID:           utils.NewID(),
				SourceItemID: ptr(callID),
				Kind:         "system",
				ItemType:     "user_input_response",
				Text:         "Submitted requested input",
				Timestamp:    ptr(ts),
				ObservedAt:   ptr(ts),
				Level:        "info",
				Payload:      cloneMap(payload),
			}
			var questions []toolRequestQuestion
			if detail := (*items)[requestIndex].Detail; detail != nil {
				questions = detail.Questions
			}
			if answers := decodeRequestUserInputAnswers(output, questions); len(answers) > 0 {
				response.Detail = &HistoryDetail{
					Type:    "user_input_response",
					Answers: answers,
				}
			} else {
				response.Text = firstNonEmpty(output, response.Text)
			}
			appendItem(response)
			return
		}

		if toolIndex, ok := pendingTools[callID]; ok && toolIndex >= 0 && toolIndex < len(*items) {
			item := (*items)[toolIndex]
			item.ObservedAt = ptr(ts)
			item.Done = true
			item.Payload = cloneMap(payload)
			if item.Tool == nil {
				item.Tool = &HistoryTool{
					ID:     callID,
					Name:   "ToolCall",
					Kind:   "dynamic_tool_call",
					Status: "done",
				}
			}
			item.Tool.Output = truncateToolOutput(item.Tool.Kind, output)
			item.Tool.Status = deepSyncToolStatus(output)
			(*items)[toolIndex] = item
			delete(pendingTools, callID)
			return
		}

		appendItem(HistoryItem{
			ID:         utils.NewID(),
			Kind:       "system",
			ItemType:   "tool_output",
			Text:       truncateToolOutput("dynamic_tool_call", output),
			Timestamp:  ptr(ts),
			ObservedAt: ptr(ts),
			Level:      "info",
			Payload:    cloneMap(payload),
		})
	}
}

func decodeDeepSyncArguments(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return decoded
	}
	return raw
}

func deepSyncToolKind(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "exec_command":
		return "command_execution"
	case "apply_patch":
		return "file_change"
	case "spawn_agent", "send_input", "wait_agent", "interrupt_agent", "close_agent", "resume_agent":
		return "sub_agent_tool_call"
	default:
		return "dynamic_tool_call"
	}
}

func deepSyncToolDisplayName(toolName string, kind string) string {
	switch kind {
	case "command_execution":
		return "CommandExecution"
	case "file_change":
		if strings.TrimSpace(toolName) == "apply_patch" {
			return "FileChange"
		}
	case "sub_agent_tool_call":
		return "Sub Agent"
	}
	return firstNonEmpty(strings.TrimSpace(toolName), "DynamicToolCall")
}

func deepSyncToolInput(toolName string, input any) any {
	record := decodeRawObject(input)
	switch strings.TrimSpace(toolName) {
	case "exec_command":
		return map[string]any{
			"command":             firstNonEmpty(stringValue(record["cmd"]), stringValue(record["command"])),
			"cwd":                 stringValue(record["workdir"]),
			"sandbox_permissions": record["sandbox_permissions"],
			"justification":       record["justification"],
		}
	case "apply_patch":
		if patchText, ok := input.(string); ok {
			return map[string]any{
				"patch": patchText,
			}
		}
		return input
	case "spawn_agent", "send_input", "wait_agent", "interrupt_agent", "close_agent", "resume_agent":
		result := cloneMap(record)
		if result == nil {
			result = map[string]any{}
		}
		result["tool"] = deepSyncCollabToolName(toolName)
		return result
	default:
		return input
	}
}

func deepSyncCollabToolName(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "spawn_agent":
		return "spawnAgent"
	case "send_input":
		return "sendInput"
	case "wait_agent":
		return "wait"
	case "interrupt_agent":
		return "interruptAgent"
	case "close_agent":
		return "closeAgent"
	case "resume_agent":
		return "resumeAgent"
	default:
		return strings.TrimSpace(toolName)
	}
}

func deepSyncToolMeta(toolName string, kind string, input any) map[string]any {
	record := decodeRawObject(input)
	subtitle := ""
	switch kind {
	case "command_execution":
		subtitle = firstNonEmpty(stringValue(record["cmd"]), stringValue(record["command"]), stringValue(record["workdir"]))
	case "file_change":
		subtitle = deepSyncFirstPatchPath(input)
	case "sub_agent_tool_call":
		subtitle = subAgentToolCallSummary(input)
	default:
		subtitle = firstNonEmpty(stringValue(record["header"]), stringValue(record["question"]))
	}
	return map[string]any{
		"title":    deepSyncToolDisplayName(toolName, kind),
		"kind":     kind,
		"subtitle": subtitle,
	}
}

func deepSyncFirstPatchPath(input any) string {
	record := decodeRawObject(input)
	patchText := strings.TrimSpace(stringValue(record["patch"]))
	if patchText == "" {
		return ""
	}
	for _, line := range strings.Split(patchText, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "*** Update File: "):
			return strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
		case strings.HasPrefix(line, "*** Add File: "):
			return strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
		case strings.HasPrefix(line, "*** Delete File: "):
			return strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
		}
	}
	return ""
}

func deepSyncToolStatus(output string) string {
	normalized := strings.ToLower(strings.TrimSpace(output))
	if strings.HasPrefix(normalized, "aborted by user") {
		return "error"
	}
	return "done"
}

func codexReasoningSummary(payload map[string]any) string {
	parts := []string{}
	if summary := strings.TrimSpace(strings.Join(stringArrayValues(payload["summary"]), "\n")); summary != "" {
		parts = append(parts, summary)
	}
	switch content := payload["content"].(type) {
	case string:
		if text := strings.TrimSpace(content); text != "" {
			parts = append(parts, text)
		}
	case []any:
		lines := make([]string, 0, len(content))
		for _, item := range content {
			record := decodeRawObject(item)
			if text := strings.TrimSpace(firstNonEmpty(stringValue(record["text"]), stringValue(item))); text != "" {
				lines = append(lines, text)
			}
		}
		if joined := strings.TrimSpace(strings.Join(lines, "\n")); joined != "" {
			parts = append(parts, joined)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func codexResponseItemTurnID(payload map[string]any) string {
	metadata := decodeRawObject(payload["internal_chat_message_metadata_passthrough"])
	return strings.TrimSpace(firstNonEmpty(
		stringValue(metadata["turn_id"]),
		stringValue(payload["turn_id"]),
	))
}

func isCodexInjectedHostContext(text string) bool {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "# AGENTS.md instructions for ") &&
		strings.Contains(trimmed, "<INSTRUCTIONS>") &&
		strings.Contains(trimmed, "</INSTRUCTIONS>") {
		return true
	}
	return strings.HasPrefix(trimmed, "<environment_context>") &&
		strings.HasSuffix(trimmed, "</environment_context>")
}

func (m *Manager) normalizeCodexResponseMessage(text string) (string, []HistoryAttachment) {
	attachments := make([]HistoryAttachment, 0)
	for _, match := range codexEmbeddedImageEnvelopePattern.FindAllStringSubmatch(text, -1) {
		if len(match) < 2 {
			continue
		}
		attachment, err := m.registerExternalAttachment(strings.TrimSpace(match[1]))
		if err != nil {
			continue
		}
		attachments = mergeHistoryAttachments(attachments, []HistoryAttachment{attachment})
	}
	text = codexEmbeddedImageEnvelopePattern.ReplaceAllString(text, "")
	return strings.TrimSpace(text), attachments
}

func (m *Manager) codexHistoryAttachments(payload map[string]any) []HistoryAttachment {
	sources := append([]string{}, extractCodexAttachmentSources(payload["local_images"])...)
	sources = append(sources, extractCodexAttachmentSources(payload["images"])...)
	if len(sources) == 0 {
		return nil
	}
	attachments := make([]HistoryAttachment, 0, len(sources))
	for _, source := range sources {
		attachment, err := m.registerExternalAttachment(source)
		if err != nil {
			continue
		}
		attachments = append(attachments, attachment)
	}
	if len(attachments) == 0 {
		return nil
	}
	return attachments
}

func extractCodexAttachmentSources(raw any) []string {
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

func decodeRequestUserInputAnswers(raw string, questions []toolRequestQuestion) []HistoryAnswerEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded struct {
		Answers map[string]struct {
			Answers []string `json:"answers"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	answers := make(map[string]any, len(decoded.Answers))
	for key, value := range decoded.Answers {
		if len(value.Answers) == 0 {
			continue
		}
		answers[key] = append([]string(nil), value.Answers...)
	}
	return historyAnswerEntries(answers, questions)
}
