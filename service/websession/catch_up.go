package websession

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

const maxEventCursorOrder = int64(^uint64(0) >> 1)

type SessionCatchUpResponse struct {
	Revision          string               `json:"revision"`
	HistoryEpoch      string               `json:"historyEpoch"`
	NextEventCursor   string               `json:"nextEventCursor"`
	TargetEventCursor string               `json:"targetEventCursor"`
	HasMore           bool                 `json:"hasMore"`
	ResetRequired     bool                 `json:"resetRequired"`
	Session           SessionSummary       `json:"session"`
	Items             []HistoryItem        `json:"items"`
	Total             int                  `json:"total"`
	PendingEpoch      string               `json:"pendingEpoch"`
	PendingVersion    uint64               `json:"pendingVersion"`
	PendingInputs     []PendingInput       `json:"pendingInputs"`
	ScheduledInputs   []ScheduledInput     `json:"scheduledInputs"`
	PendingApproval   *PendingApproval     `json:"pendingApproval,omitempty"`
	PendingUserInput  *PendingUserInput    `json:"pendingUserInput,omitempty"`
	SubAgents         []WebSessionSubAgent `json:"subAgents"`
}

type sessionEventCursor struct {
	Seq   int64
	Order int64
}

func normalizeAttentionRevision(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeHistoryEpoch(value int64) int64 {
	if value < 1 {
		return 1
	}
	return value
}

func formatEventCursor(seq, order int64) string {
	if seq < 0 {
		seq = 0
	}
	if order < 0 {
		order = 0
	}
	return fmt.Sprintf("%d:%d", seq, order)
}

func parseEventCursor(value string) (sessionEventCursor, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return sessionEventCursor{}, nil
	}
	parts := strings.Split(normalized, ":")
	if len(parts) == 1 {
		seq, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || seq < 0 {
			return sessionEventCursor{}, fmt.Errorf("invalid event cursor")
		}
		return sessionEventCursor{Seq: seq, Order: maxEventCursorOrder}, nil
	}
	if len(parts) != 2 {
		return sessionEventCursor{}, fmt.Errorf("invalid event cursor")
	}
	seq, seqErr := strconv.ParseInt(parts[0], 10, 64)
	order, orderErr := strconv.ParseInt(parts[1], 10, 64)
	if seqErr != nil || orderErr != nil || seq < 0 || order < 0 {
		return sessionEventCursor{}, fmt.Errorf("invalid event cursor")
	}
	return sessionEventCursor{Seq: seq, Order: order}, nil
}

func compareEventCursors(left, right sessionEventCursor) int {
	if left.Seq < right.Seq || (left.Seq == right.Seq && left.Order < right.Order) {
		return -1
	}
	if left.Seq > right.Seq || (left.Seq == right.Seq && left.Order > right.Order) {
		return 1
	}
	return 0
}

func parseHistoryEpoch(value string) (int64, error) {
	epoch, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || epoch < 1 {
		return 0, fmt.Errorf("invalid history epoch")
	}
	return epoch, nil
}

func withHistoryEpochIncrement(updates map[string]any) map[string]any {
	if updates == nil {
		updates = make(map[string]any)
	}
	updates["history_epoch"] = gorm.Expr("history_epoch + 1")
	return updates
}

func (m *Manager) CatchUpSession(
	ctx context.Context,
	sessionID string,
	afterCursorValue string,
	targetCursorValue string,
	historyEpochValue string,
	limit int,
) (SessionCatchUpResponse, error) {
	after, err := parseEventCursor(afterCursorValue)
	if err != nil {
		return SessionCatchUpResponse{}, err
	}
	expectedEpoch, err := parseHistoryEpoch(historyEpochValue)
	if err != nil {
		return SessionCatchUpResponse{}, err
	}
	if limit <= 0 || limit > MaxHistoryWindow {
		limit = DefaultHistoryWindow
	}

	db := model.GetReaderDB()
	if db == nil {
		return SessionCatchUpResponse{}, model.ErrDBNotInitialized
	}

	var record tables.WebSessionTable
	var rows []tables.WebSessionItemTable
	var total int64
	target := sessionEventCursor{}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&record, "id = ?", sessionID).Error; err != nil {
			return err
		}
		if normalizeHistoryEpoch(record.HistoryEpoch) != expectedEpoch {
			return nil
		}

		target = sessionEventCursor{Seq: record.LastEventSeq, Order: maxEventCursorOrder}
		if strings.TrimSpace(targetCursorValue) != "" {
			requested, parseErr := parseEventCursor(targetCursorValue)
			if parseErr != nil {
				return parseErr
			}
			if compareEventCursors(requested, target) < 0 {
				target = requested
			}
		}
		if compareEventCursors(target, after) < 0 {
			return fmt.Errorf("target event cursor precedes after event cursor")
		}

		if err := tx.Model(&tables.WebSessionItemTable{}).
			Where("web_session_id = ?", sessionID).
			Count(&total).Error; err != nil {
			return err
		}
		return tx.
			Where(
				"web_session_id = ? AND last_event_seq > 0 AND (last_event_seq > ? OR (last_event_seq = ? AND order_index > ?)) AND (last_event_seq < ? OR (last_event_seq = ? AND order_index <= ?))",
				sessionID,
				after.Seq,
				after.Seq,
				after.Order,
				target.Seq,
				target.Seq,
				target.Order,
			).
			Order("last_event_seq ASC, order_index ASC").
			Limit(limit + 1).
			Find(&rows).Error
	})
	if err != nil {
		return SessionCatchUpResponse{}, err
	}

	summary := m.mapSessionSummary(record)
	response := SessionCatchUpResponse{
		Revision:          summary.Revision,
		HistoryEpoch:      summary.HistoryEpoch,
		NextEventCursor:   formatEventCursor(record.LastEventSeq, maxEventCursorOrder),
		TargetEventCursor: formatEventCursor(record.LastEventSeq, maxEventCursorOrder),
		ResetRequired:     normalizeHistoryEpoch(record.HistoryEpoch) != expectedEpoch,
		Session:           summary,
		Items:             []HistoryItem{},
		Total:             int(total),
		PendingInputs:     []PendingInput{},
		ScheduledInputs:   []ScheduledInput{},
		SubAgents:         []WebSessionSubAgent{},
	}
	if response.ResetRequired {
		return response, nil
	}

	response.TargetEventCursor = formatEventCursor(target.Seq, target.Order)
	response.HasMore = len(rows) > limit
	if response.HasMore {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		response.NextEventCursor = formatEventCursor(last.LastEventSeq, last.OrderIndex)
	} else {
		response.NextEventCursor = response.TargetEventCursor
	}
	response.Items = make([]HistoryItem, 0, len(rows))
	for _, row := range rows {
		response.Items = append(response.Items, mapHistoryItemRowWithSession(row, sessionID))
	}

	scheduledInputs, err := m.scheduledInputsSnapshot(ctx, sessionID)
	if err != nil {
		return SessionCatchUpResponse{}, err
	}
	response.ScheduledInputs = scheduledInputs
	response.Session.HasScheduledPlanExecution = scheduledInputsHavePendingPlanExecution(scheduledInputs)
	response.SubAgents, err = m.sessionSubAgents(ctx, sessionID)
	if err != nil {
		return SessionCatchUpResponse{}, err
	}
	response.PendingEpoch, response.PendingVersion, response.PendingInputs = m.pendingStateSnapshot(sessionID)
	response.PendingApproval = m.pendingApprovalSnapshot(record)
	latestHistory, historyErr := m.loadHistoryWindow(ctx, sessionID, DefaultHistoryWindow, nil)
	if historyErr == nil {
		response.PendingUserInput = pendingUserInputFromHistory(latestHistory.Items)
	}
	return response, nil
}

func SessionCatchUpResponseForTransport(response SessionCatchUpResponse) SessionCatchUpResponse {
	projected := HistoryWindowForTransport(HistoryWindow{Items: response.Items})
	response.Items = projected.Items
	return response
}

type SessionUnreadState struct {
	HasUnread         bool   `json:"hasUnread"`
	AttentionRevision string `json:"attentionRevision"`
}

func (m *Manager) MarkSessionRead(
	ctx context.Context,
	sessionID string,
	expectedRevisionValue string,
) (SessionUnreadState, error) {
	expected, err := strconv.ParseInt(strings.TrimSpace(expectedRevisionValue), 10, 64)
	if err != nil || expected < 0 {
		return SessionUnreadState{}, fmt.Errorf("invalid attention revision")
	}
	db := model.GetDB()
	if db == nil {
		return SessionUnreadState{}, model.ErrDBNotInitialized
	}
	result := db.WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("id = ? AND has_unread = ? AND attention_revision = ?", sessionID, true, expected).
		Updates(map[string]any{
			"has_unread":         false,
			"attention_revision": gorm.Expr("attention_revision + 1"),
		})
	if result.Error != nil {
		return SessionUnreadState{}, result.Error
	}
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionUnreadState{}, err
	}
	if result.RowsAffected > 0 {
		m.broadcastSessionSummary(ctx, sessionID)
	}
	return SessionUnreadState{
		HasUnread:         record.HasUnread,
		AttentionRevision: strconv.FormatInt(normalizeAttentionRevision(record.AttentionRevision), 10),
	}, nil
}
