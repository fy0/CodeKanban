package websession

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"code-kanban/model"
	"code-kanban/model/tables"
)

const (
	defaultSessionConversationSearchLimit = 100
	maxSessionConversationSearchLimit     = 200
)

var ErrInvalidSessionConversationSearchCursor = errors.New("invalid session conversation search cursor")

type sessionConversationSearchCursor struct {
	OrderIndex int64  `json:"orderIndex"`
	ID         string `json:"id"`
	Total      int    `json:"total"`
}

func encodeSessionConversationSearchCursor(match SessionConversationSearchMatch, total int) string {
	payload, err := json.Marshal(sessionConversationSearchCursor{
		OrderIndex: match.OrderIndex,
		ID:         match.ID,
		Total:      total,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeSessionConversationSearchCursor(value string) (sessionConversationSearchCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return sessionConversationSearchCursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return sessionConversationSearchCursor{}, ErrInvalidSessionConversationSearchCursor
	}
	var cursor sessionConversationSearchCursor
	if err := json.Unmarshal(payload, &cursor); err != nil ||
		strings.TrimSpace(cursor.ID) == "" || cursor.Total < 0 {
		return sessionConversationSearchCursor{}, ErrInvalidSessionConversationSearchCursor
	}
	return cursor, nil
}

func (m *Manager) SearchSessionConversation(
	ctx context.Context,
	sessionID string,
	query string,
	includeUser bool,
	includeAssistant bool,
	includeTools bool,
	includeSystem bool,
	sourceThreadID string,
	cursorValue string,
	limit int,
) (SessionConversationSearchResult, error) {
	db := model.GetDB()
	if db == nil {
		return SessionConversationSearchResult{}, model.ErrDBNotInitialized
	}

	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || !(includeUser || includeAssistant || includeTools || includeSystem) {
		return SessionConversationSearchResult{
			Items: []SessionConversationSearchMatch{},
			Done:  true,
		}, nil
	}
	if limit <= 0 {
		limit = defaultSessionConversationSearchLimit
	}
	if limit > maxSessionConversationSearchLimit {
		limit = maxSessionConversationSearchLimit
	}

	cursor, err := decodeSessionConversationSearchCursor(cursorValue)
	if err != nil {
		return SessionConversationSearchResult{}, err
	}

	scope := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where("web_session_id = ?", sessionID)
	if sourceThreadID = strings.TrimSpace(sourceThreadID); sourceThreadID != "" {
		scope = scope.Where("source_thread_id = ?", sourceThreadID)
	}
	scope = scope.Select(
		"id, source_thread_id, source_turn_id, source_item_id, order_index, item_kind, item_type, text, tool_json, detail_json, payload_json",
	)

	var rows []tables.WebSessionItemTable
	if err := scope.Order("order_index ASC").Order("id ASC").Find(&rows).Error; err != nil {
		return SessionConversationSearchResult{}, err
	}

	allMatches := make([]SessionConversationSearchMatch, 0)
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return SessionConversationSearchResult{}, err
		}
		if !matchesSessionConversationItem(
			row,
			query,
			includeUser,
			includeAssistant,
			includeTools,
			includeSystem,
		) {
			continue
		}
		match := mapSessionConversationSearchMatch(row, sessionID)
		if cursor.ID != "" && !afterSessionConversationSearchCursor(match, cursor) {
			continue
		}
		allMatches = append(allMatches, match)
	}

	total := len(allMatches)
	if cursor.ID != "" {
		total = cursor.Total
	}
	result := SessionConversationSearchResult{
		Items: allMatches,
		Done:  true,
		Total: total,
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.Done = false
		result.NextCursor = encodeSessionConversationSearchCursor(result.Items[len(result.Items)-1], total)
	}
	return result, nil
}

func afterSessionConversationSearchCursor(
	match SessionConversationSearchMatch,
	cursor sessionConversationSearchCursor,
) bool {
	return match.OrderIndex > cursor.OrderIndex ||
		(match.OrderIndex == cursor.OrderIndex && match.ID > cursor.ID)
}

func matchesSessionConversationItem(
	row tables.WebSessionItemTable,
	query string,
	includeUser bool,
	includeAssistant bool,
	includeTools bool,
	includeSystem bool,
) bool {
	kind := strings.ToLower(strings.TrimSpace(row.ItemKind))
	switch kind {
	case "user":
		return includeUser && strings.Contains(strings.ToLower(row.Text), query)
	case "assistant":
		return includeAssistant && strings.Contains(strings.ToLower(row.Text), query)
	case "tool":
		return includeTools && strings.Contains(sessionConversationToolSearchText(row), query)
	case "system":
		return includeSystem && strings.Contains(sessionConversationSystemSearchText(row), query)
	default:
		return false
	}
}

func sessionConversationToolSearchText(row tables.WebSessionItemTable) string {
	var tool HistoryTool
	decodeJSONText(row.ToolJSON, &tool)

	var builder strings.Builder
	appendSearchText := func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		builder.WriteString(value)
		builder.WriteByte('\n')
	}
	appendSearchText(row.Text)
	appendSearchText(row.ItemType)
	appendSearchText(tool.Name)
	appendSearchText(tool.Kind)
	appendSearchText(tool.Output)
	if tool.Input != nil {
		if encoded, err := json.Marshal(tool.Input); err == nil {
			appendSearchText(string(encoded))
		}
	}
	if tool.Meta != nil {
		if encoded, err := json.Marshal(tool.Meta); err == nil {
			appendSearchText(string(encoded))
		}
	}
	appendSearchText(row.ToolJSON)
	appendSearchText(row.PayloadJSON)
	return strings.ToLower(builder.String())
}

func sessionConversationSystemSearchText(row tables.WebSessionItemTable) string {
	var builder strings.Builder
	for _, value := range []string{row.Text, row.ItemType, row.DetailJSON, row.PayloadJSON} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		builder.WriteString(value)
		builder.WriteByte('\n')
	}
	return strings.ToLower(builder.String())
}

func mapSessionConversationSearchMatch(
	row tables.WebSessionItemTable,
	sessionID string,
) SessionConversationSearchMatch {
	id := strings.TrimSpace(row.ID)
	if id == "" {
		id = sessionID + ":" + formatOrderIndex(row.OrderIndex)
	}
	match := SessionConversationSearchMatch{
		ID:             id,
		SourceThreadID: row.SourceThreadID,
		SourceTurnID:   row.SourceTurnID,
		SourceItemID:   row.SourceItemID,
		OrderIndex:     row.OrderIndex,
		Kind:           strings.ToLower(strings.TrimSpace(row.ItemKind)),
	}

	var tool HistoryTool
	decodeJSONText(row.ToolJSON, &tool)
	if tool.ID != "" {
		match.ToolID = tool.ID
	}
	if tool.CommandGroup != nil {
		match.CommandGroupID = tool.CommandGroup.ID
	}
	return match
}

func formatOrderIndex(value int64) string {
	return strconv.FormatInt(value, 10)
}
