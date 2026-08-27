package websession

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

func normalizeWebSessionSubAgentStatus(value string) WebSessionSubAgentStatus {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case "pendinginit", "pending":
		return WebSessionSubAgentPendingInit
	case "running", "inprogress", "active":
		return WebSessionSubAgentRunning
	case "idle", "notloaded":
		return WebSessionSubAgentIdle
	case "interrupted", "aborted":
		return WebSessionSubAgentInterrupted
	case "completed", "complete", "done":
		return WebSessionSubAgentCompleted
	case "errored", "error", "failed", "systemerror":
		return WebSessionSubAgentErrored
	case "shutdown", "closed":
		return WebSessionSubAgentShutdown
	case "notfound", "missing":
		return WebSessionSubAgentNotFound
	default:
		return ""
	}
}

func webSessionSubAgentIsActive(status WebSessionSubAgentStatus) bool {
	return status == WebSessionSubAgentPendingInit || status == WebSessionSubAgentRunning
}

func webSessionSubAgentIsTerminal(status WebSessionSubAgentStatus) bool {
	switch status {
	case WebSessionSubAgentInterrupted,
		WebSessionSubAgentCompleted,
		WebSessionSubAgentErrored,
		WebSessionSubAgentShutdown,
		WebSessionSubAgentNotFound:
		return true
	default:
		return false
	}
}

func codexSubAgentState(raw any) (WebSessionSubAgentStatus, string) {
	record := decodeRawObject(raw)
	statusText := stringValue(raw)
	message := ""
	if len(record) > 0 {
		statusText = firstNonEmpty(stringValue(record["status"]), stringValue(record["type"]))
		message = firstNonEmpty(
			stringValue(record["message"]),
			stringValue(record["error"]),
			stringValue(record["summary"]),
		)
	}
	return normalizeWebSessionSubAgentStatus(statusText), strings.TrimSpace(message)
}

func codexTurnSubAgentStatus(status string) WebSessionSubAgentStatus {
	// A completed turn leaves the child thread reusable, but it is no longer
	// doing work until another turn starts.
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "inprogress", "in_progress", "running", "active":
		return WebSessionSubAgentRunning
	case "completed", "complete", "done":
		return WebSessionSubAgentIdle
	case "interrupted", "cancelled", "canceled", "aborted":
		return WebSessionSubAgentInterrupted
	case "failed", "errored", "error", "systemerror", "system_error":
		return WebSessionSubAgentErrored
	default:
		return ""
	}
}

func mapWebSessionSubAgentRow(row tables.WebSessionSubAgentTable) WebSessionSubAgent {
	parentThreadID := row.ParentThreadID
	if parentThreadID != nil && strings.TrimSpace(*parentThreadID) == strings.TrimSpace(row.ThreadID) {
		parentThreadID = nil
	}
	return WebSessionSubAgent{
		ThreadID:         row.ThreadID,
		ParentThreadID:   parentThreadID,
		Path:             row.AgentPath,
		Nickname:         row.Nickname,
		Role:             row.Role,
		Status:           normalizeWebSessionSubAgentStatus(row.Status),
		Summary:          row.Summary,
		CurrentTurnID:    row.CurrentTurnID,
		LatestItemID:     row.LatestItemID,
		LatestOrderIndex: row.LatestOrderIndex,
		StartedAt:        row.StartedAt,
		LastActivityAt:   row.LastActivityAt,
		EndedAt:          row.EndedAt,
	}
}

func (m *Manager) sessionSubAgents(ctx context.Context, sessionID string) ([]WebSessionSubAgent, error) {
	db := model.GetReaderDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}
	var rows []tables.WebSessionSubAgentTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ?", sessionID).
		Order("started_at ASC, created_at ASC, thread_id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	rootThreadID := ""
	var session struct {
		NativeSessionID *string
	}
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionTable{}).
		Select("native_session_id").
		Where("id = ?", sessionID).
		Take(&session).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if session.NativeSessionID != nil {
		rootThreadID = strings.TrimSpace(*session.NativeSessionID)
	}
	items := make([]WebSessionSubAgent, 0, len(rows))
	for _, row := range rows {
		if rootThreadID != "" && strings.TrimSpace(row.ThreadID) == rootThreadID {
			continue
		}
		items = append(items, mapWebSessionSubAgentRow(row))
	}
	return items, nil
}

func applySubAgentPayload(row *tables.WebSessionSubAgentTable, payload map[string]any, event Event) {
	if row == nil {
		return
	}
	if value := strings.TrimSpace(stringValue(payload["parentThreadId"])); value != "" {
		row.ParentThreadID = ptr(value)
	}
	if value := strings.TrimSpace(stringValue(payload["path"])); value != "" {
		row.AgentPath = value
	}
	if value := strings.TrimSpace(stringValue(payload["nickname"])); value != "" {
		row.Nickname = value
	}
	if value := strings.TrimSpace(stringValue(payload["role"])); value != "" {
		row.Role = value
	}
	if value := strings.TrimSpace(stringValue(payload["summary"])); value != "" {
		row.Summary = value
	}
	if value := strings.TrimSpace(stringValue(payload["latestItemId"])); value != "" {
		row.LatestItemID = ptr(value)
	}
	if value := int64(numberValue(payload["latestOrderIndex"])); value > 0 {
		row.LatestOrderIndex = value
	}
	previousStatus := normalizeWebSessionSubAgentStatus(row.Status)
	status := normalizeWebSessionSubAgentStatus(stringValue(payload["status"]))
	if status == "" && payload["activeTurn"] == true && !webSessionSubAgentIsTerminal(previousStatus) {
		status = WebSessionSubAgentRunning
	}
	if status == "" && payload["turnCompleted"] == true && webSessionSubAgentIsActive(previousStatus) {
		status = WebSessionSubAgentIdle
	}
	if status != "" {
		row.Status = string(status)
		if webSessionSubAgentIsTerminal(status) {
			endedAt := event.Timestamp
			row.EndedAt = &endedAt
		} else {
			row.EndedAt = nil
		}
	} else {
		status = previousStatus
	}
	if payload["turnCompleted"] == true || !webSessionSubAgentIsActive(status) {
		row.CurrentTurnID = nil
	} else if value := strings.TrimSpace(firstNonEmpty(stringValue(payload["turnId"]), event.TurnID)); value != "" {
		row.CurrentTurnID = ptr(value)
	}
	activityAt := event.Timestamp
	if activityAt.IsZero() {
		activityAt = time.Now()
	}
	if row.StartedAt == nil {
		row.StartedAt = &activityAt
	}
	row.LastActivityAt = &activityAt
	if event.Seq > row.LastEventSeq {
		row.LastEventSeq = event.Seq
	}
}

func historyItemSubAgentSummary(item HistoryItem) string {
	if text := strings.TrimSpace(item.Text); text != "" {
		return truncateString(text, 240)
	}
	if item.Tool != nil {
		if summary := strings.TrimSpace(compactToolSummary(
			item.Tool.Kind,
			item.Tool.Input,
			item.Tool.Meta,
			item.Tool.Output,
		)); summary != "" {
			return truncateString(summary, 240)
		}
		if name := strings.TrimSpace(item.Tool.Name); name != "" {
			return name
		}
	}
	return ""
}

func historyItemUpdatesSubAgentActivity(item HistoryItem) bool {
	return !(item.ItemType == "note" && strings.TrimSpace(stringValue(item.Payload["code"])) == "transport_retrying")
}

func (m *Manager) applySubAgentHistoryItemDB(
	ctx context.Context,
	db *gorm.DB,
	session tables.WebSessionTable,
	event Event,
	item HistoryItem,
) (WebSessionSubAgent, bool, error) {
	if normalizeAgent(Agent(session.Agent)) != AgentCodex {
		return WebSessionSubAgent{}, false, nil
	}
	if !historyItemUpdatesSubAgentActivity(item) {
		return WebSessionSubAgent{}, false, nil
	}
	threadID := strings.TrimSpace(event.ThreadID)
	if threadID == "" {
		return WebSessionSubAgent{}, false, nil
	}
	if session.NativeSessionID != nil && threadID == strings.TrimSpace(*session.NativeSessionID) {
		return WebSessionSubAgent{}, false, nil
	}
	payload := map[string]any{
		"threadId":         threadID,
		"turnId":           event.TurnID,
		"activeTurn":       true,
		"latestItemId":     item.ID,
		"latestOrderIndex": item.OrderIndex,
	}
	if summary := historyItemSubAgentSummary(item); summary != "" {
		payload["summary"] = summary
	}
	event.Payload = payload
	return m.applySubAgentStateEventDB(ctx, db, session.ID, event)
}

func (m *Manager) applySubAgentStateEventDB(
	ctx context.Context,
	db *gorm.DB,
	sessionID string,
	event Event,
) (WebSessionSubAgent, bool, error) {
	threadID := strings.TrimSpace(firstNonEmpty(stringValue(event.Payload["threadId"]), event.ThreadID))
	if threadID == "" {
		return WebSessionSubAgent{}, false, nil
	}
	var result tables.WebSessionSubAgentTable
	changed := false
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row tables.WebSessionSubAgentTable
		err := tx.Where("web_session_id = ? AND thread_id = ?", sessionID, threadID).First(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row.Init()
			row.WebSessionID = sessionID
			row.ThreadID = threadID
			row.Status = string(WebSessionSubAgentPendingInit)
		}
		if event.Seq > 0 && row.LastEventSeq >= event.Seq {
			result = row
			return nil
		}
		applySubAgentPayload(&row, event.Payload, event)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if createErr := tx.Create(&row).Error; createErr != nil {
				return createErr
			}
		} else if saveErr := tx.Save(&row).Error; saveErr != nil {
			return saveErr
		}
		result = row
		changed = true
		return nil
	})
	if err != nil {
		return WebSessionSubAgent{}, false, err
	}
	return mapWebSessionSubAgentRow(result), changed, nil
}

func (m *Manager) replaceSessionSubAgents(
	ctx context.Context,
	sessionID string,
	agents []WebSessionSubAgent,
	prune bool,
) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		threadIDs := make([]string, 0, len(agents))
		for _, agent := range agents {
			threadID := strings.TrimSpace(agent.ThreadID)
			if threadID == "" {
				continue
			}
			threadIDs = append(threadIDs, threadID)
			var row tables.WebSessionSubAgentTable
			err := tx.Where("web_session_id = ? AND thread_id = ?", sessionID, threadID).First(&row).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				row.Init()
				row.WebSessionID = sessionID
				row.ThreadID = threadID
			}
			row.ParentThreadID = agent.ParentThreadID
			row.AgentPath = strings.TrimSpace(agent.Path)
			row.Nickname = strings.TrimSpace(agent.Nickname)
			row.Role = strings.TrimSpace(agent.Role)
			if status := normalizeWebSessionSubAgentStatus(string(agent.Status)); status != "" {
				row.Status = string(status)
			} else if row.Status == "" {
				row.Status = string(WebSessionSubAgentPendingInit)
			}
			row.Summary = strings.TrimSpace(agent.Summary)
			row.CurrentTurnID = agent.CurrentTurnID
			row.LatestItemID = agent.LatestItemID
			row.LatestOrderIndex = agent.LatestOrderIndex
			row.StartedAt = agent.StartedAt
			row.LastActivityAt = agent.LastActivityAt
			row.EndedAt = agent.EndedAt
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			} else if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		if !prune {
			return nil
		}
		query := tx.Unscoped().Where("web_session_id = ?", sessionID)
		if len(threadIDs) > 0 {
			query = query.Where("thread_id NOT IN ?", threadIDs)
		}
		return query.Delete(&tables.WebSessionSubAgentTable{}).Error
	})
}

func subAgentStatusFromThreadSummary(summary codexThreadSummary, turns []map[string]any) WebSessionSubAgentStatus {
	status := normalizeWebSessionSubAgentStatus(summary.Status)
	if webSessionSubAgentIsTerminal(status) {
		return status
	}
	for index := len(turns) - 1; index >= 0; index-- {
		turnStatusText := strings.ToLower(strings.TrimSpace(stringValue(turns[index]["status"])))
		if turnStatus := codexTurnSubAgentStatus(turnStatusText); turnStatus != "" {
			return turnStatus
		}
	}
	if status == WebSessionSubAgentRunning {
		return WebSessionSubAgentRunning
	}
	if status == WebSessionSubAgentIdle {
		return WebSessionSubAgentIdle
	}
	return WebSessionSubAgentPendingInit
}

func webSessionSubAgentFromThread(
	summary codexThreadSummary,
	turns []map[string]any,
) WebSessionSubAgent {
	status := subAgentStatusFromThreadSummary(summary, turns)
	agent := WebSessionSubAgent{
		ThreadID:       strings.TrimSpace(summary.ID),
		ParentThreadID: nilIfEmptyHistory(summary.ParentThreadID),
		Path:           strings.TrimSpace(summary.AgentPath),
		Nickname:       strings.TrimSpace(summary.Nickname),
		Role:           strings.TrimSpace(summary.Role),
		Status:         status,
		StartedAt:      summary.CreatedAt,
		LastActivityAt: summary.UpdatedAt,
	}
	if len(turns) > 0 {
		lastTurn := turns[len(turns)-1]
		turnID := strings.TrimSpace(stringValue(lastTurn["id"]))
		if webSessionSubAgentIsActive(status) &&
			codexTurnSubAgentStatus(stringValue(lastTurn["status"])) == WebSessionSubAgentRunning {
			agent.CurrentTurnID = nilIfEmptyHistory(turnID)
		} else if webSessionSubAgentIsTerminal(status) {
			agent.EndedAt = firstNonNilTime(
				parseHistoryTimestamp(lastTurn["completedAt"]),
				parseHistoryTimestamp(lastTurn["updatedAt"]),
				summary.UpdatedAt,
			)
		}
	}
	return agent
}

func firstNonNilTime(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func subAgentActivityText(kind string, path string, threadID string) string {
	display := strings.TrimSpace(path)
	if display == "" {
		display = strings.TrimSpace(threadID)
		if len(display) > 12 {
			display = display[:12]
		}
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "started":
		return "Agent " + display + " started"
	case "interacted":
		return "Agent " + display + " received input"
	case "interrupted":
		return "Agent " + display + " was interrupted"
	default:
		return "Agent " + display + " activity"
	}
}

func sortWebSessionSubAgents(items []WebSessionSubAgent) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i].StartedAt
		right := items[j].StartedAt
		if left != nil && right != nil && !left.Equal(*right) {
			return left.Before(*right)
		}
		if left != nil && right == nil {
			return true
		}
		if left == nil && right != nil {
			return false
		}
		return items[i].ThreadID < items[j].ThreadID
	})
}
