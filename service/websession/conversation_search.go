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

	predicate, predicateArgs := sessionConversationSearchPredicate(
		query,
		includeUser,
		includeAssistant,
		includeTools,
		includeSystem,
	)
	baseScope := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where("web_session_id = ?", sessionID).
		Where(predicate, predicateArgs...)
	if sourceThreadID = strings.TrimSpace(sourceThreadID); sourceThreadID != "" {
		baseScope = baseScope.Where("source_thread_id = ?", sourceThreadID)
	}

	total := cursor.Total
	if cursor.ID == "" {
		var count int64
		if err := baseScope.Count(&count).Error; err != nil {
			return SessionConversationSearchResult{}, err
		}
		total = int(count)
	}

	pageScope := baseScope
	if cursor.ID != "" {
		pageScope = pageScope.Where(
			"(order_index < ? OR (order_index = ? AND id < ?))",
			cursor.OrderIndex,
			cursor.OrderIndex,
			cursor.ID,
		)
	}

	var rows []tables.WebSessionItemTable
	if err := pageScope.
		Select("id, source_thread_id, source_turn_id, source_item_id, order_index, item_kind, tool_json").
		Order("order_index DESC").
		Order("id DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return SessionConversationSearchResult{}, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]SessionConversationSearchMatch, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapSessionConversationSearchMatch(row, sessionID))
	}

	result := SessionConversationSearchResult{
		Items: items,
		Done:  !hasMore,
		Total: total,
	}
	if hasMore && len(result.Items) > 0 {
		result.NextCursor = encodeSessionConversationSearchCursor(result.Items[len(result.Items)-1], total)
	}
	return result, nil
}

func sessionConversationSearchPredicate(
	query string,
	includeUser bool,
	includeAssistant bool,
	includeTools bool,
	includeSystem bool,
) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 16)
	appendKind := func(kind string, columns ...string) {
		columnClauses := make([]string, 0, len(columns))
		args = append(args, kind)
		for _, column := range columns {
			columnClauses = append(
				columnClauses,
				"instr(lower(coalesce("+column+", '')), ?) > 0",
			)
			args = append(args, query)
		}
		clauses = append(
			clauses,
			"(lower(trim(item_kind)) = ? AND ("+strings.Join(columnClauses, " OR ")+"))",
		)
	}
	if includeUser {
		appendKind("user", "text")
	}
	if includeAssistant {
		appendKind("assistant", "text")
	}
	if includeTools {
		appendKind("tool", "text", "item_type", "tool_json", "payload_json")
	}
	if includeSystem {
		appendKind("system", "text", "item_type", "detail_json", "payload_json")
	}
	return strings.Join(clauses, " OR "), args
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
