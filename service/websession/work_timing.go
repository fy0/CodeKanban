package websession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

const currentWorkTimingBackfillVersion = 1

const workTimingContinuationPayloadKey = "workTimingContinuation"

func isWorkTimingContinuationPayload(payload map[string]any) bool {
	return payload["autoRetry"] == true || payload[workTimingContinuationPayloadKey] == true
}

func normalizeWorkTimingBackfillState(value string) WorkTimingBackfillState {
	switch WorkTimingBackfillState(strings.TrimSpace(value)) {
	case WorkTimingBackfillComplete:
		return WorkTimingBackfillComplete
	case WorkTimingBackfillPartial:
		return WorkTimingBackfillPartial
	case WorkTimingBackfillUnavailable:
		return WorkTimingBackfillUnavailable
	case WorkTimingBackfillFailed:
		return WorkTimingBackfillFailed
	default:
		return WorkTimingBackfillPending
	}
}

func workTimingFromRecord(record tables.WebSessionTable) WorkTiming {
	timing := WorkTiming{
		CompletedDurationMs: maxInt64(0, record.WorkDurationMs),
		BackfillState:       normalizeWorkTimingBackfillState(record.WorkTimingBackfillState),
		BackfillVersion:     record.WorkTimingBackfillVersion,
	}
	if record.WorkCurrentRunID != nil && record.WorkCurrentRunStartedAt != nil {
		timing.CurrentRun = &WorkTimingCurrentRun{
			ID:               strings.TrimSpace(*record.WorkCurrentRunID),
			StartedAt:        *record.WorkCurrentRunStartedAt,
			PausedAt:         record.WorkCurrentRunPausedAt,
			PausedDurationMs: maxInt64(0, record.WorkCurrentRunPausedDurationMs),
		}
	} else if record.WorkRetryWaitStartedAt != nil {
		id := "retry"
		if record.WorkRetrySourceRunID != nil && strings.TrimSpace(*record.WorkRetrySourceRunID) != "" {
			id += ":" + strings.TrimSpace(*record.WorkRetrySourceRunID)
		}
		timing.CurrentRun = &WorkTimingCurrentRun{
			ID:        id,
			StartedAt: *record.WorkRetryWaitStartedAt,
		}
	}
	return timing
}

func (m *Manager) latestCompletedWorkTimingRunID(ctx context.Context, sessionID string) string {
	if m == nil {
		return ""
	}
	db := model.GetDB()
	if db == nil {
		return ""
	}
	var timing tables.WebSessionRunTimingTable
	if err := db.WithContext(ctx).
		Select("run_id").
		Where("web_session_id = ? AND ended_at IS NOT NULL", strings.TrimSpace(sessionID)).
		Order("ended_at DESC").
		First(&timing).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(timing.RunID)
}

func mergeRuntimeUpdates(target map[string]any, source map[string]any) map[string]any {
	if target == nil {
		target = make(map[string]any, len(source))
	}
	for key, value := range source {
		target[key] = value
	}
	return target
}

func isWorkTimingLifecycleEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "run_st", "approval_req", "approval_res", "user_input_req", "user_input_res", "run_done", "run_abort", "run_fail":
		return true
	default:
		return false
	}
}

func isWorkTimingTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "run_done", "run_abort", "run_fail":
		return true
	default:
		return false
	}
}

func workTimingOutcomeForEvent(event Event) WorkTimingOutcome {
	switch strings.TrimSpace(event.Type) {
	case "run_done":
		return WorkTimingOutcomeCompleted
	case "run_fail":
		if strings.Contains(strings.ToLower(strings.TrimSpace(stringValue(event.Payload["code"]))), "timeout") {
			return WorkTimingOutcomeTimeout
		}
		return WorkTimingOutcomeFailed
	case "run_abort":
		reason := strings.ToLower(strings.TrimSpace(stringValue(event.Payload["reason"])))
		if reason == recoveryReasonProcessRestart {
			return WorkTimingOutcomeInterrupted
		}
		if reason == activeCallTimeoutReason || strings.Contains(reason, "timeout") {
			return WorkTimingOutcomeTimeout
		}
		return WorkTimingOutcomeCanceled
	default:
		return ""
	}
}

func workTimingDurationMs(startedAt, endedAt time.Time, pausedDurationMs int64) int64 {
	if startedAt.IsZero() || endedAt.Before(startedAt) {
		return 0
	}
	return maxInt64(0, endedAt.Sub(startedAt).Milliseconds()-maxInt64(0, pausedDurationMs))
}

func (m *Manager) applyWorkTimingEventDB(
	ctx context.Context,
	tx *gorm.DB,
	sessionID string,
	event Event,
) (map[string]any, *HistoryItem, error) {
	if !isWorkTimingLifecycleEvent(event.Type) {
		return nil, nil, nil
	}

	var session tables.WebSessionTable
	if err := tx.WithContext(ctx).First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, nil, err
	}
	if event.Seq > 0 && session.LastEventSeq >= event.Seq {
		return nil, nil, nil
	}

	runID := strings.TrimSpace(event.RunID)
	if runID == "" && session.WorkCurrentRunID != nil {
		runID = strings.TrimSpace(*session.WorkCurrentRunID)
	}
	if runID == "" {
		return nil, nil, nil
	}

	switch strings.TrimSpace(event.Type) {
	case "run_st":
		startedAt := event.Timestamp
		var timingItem *HistoryItem
		completedDurationMs := session.WorkDurationMs
		if isWorkTimingContinuationPayload(event.Payload) && session.WorkRetryWaitStartedAt != nil {
			startedAt = *session.WorkRetryWaitStartedAt
		} else if session.WorkRetryWaitStartedAt != nil {
			addedDurationMs, updatedItem, err := settleRetryWaitIntoSourceRunDB(ctx, tx, session, event.Timestamp)
			if err != nil {
				return nil, nil, err
			}
			completedDurationMs += addedDurationMs
			timingItem = updatedItem
		}
		if err := upsertStartedRunTimingDB(ctx, tx, sessionID, runID, startedAt); err != nil {
			return nil, nil, err
		}
		updates := map[string]any{
			"work_current_run_id":                 runID,
			"work_current_run_started_at":         startedAt,
			"work_current_run_paused_at":          nil,
			"work_current_run_paused_duration_ms": 0,
			"work_current_run_pause_depth":        0,
			"work_retry_wait_started_at":          nil,
			"work_retry_source_run_id":            nil,
		}
		if completedDurationMs != session.WorkDurationMs {
			updates["work_duration_ms"] = completedDurationMs
		}
		return updates, timingItem, nil

	case "approval_req", "user_input_req":
		if session.WorkCurrentRunID == nil || strings.TrimSpace(*session.WorkCurrentRunID) != runID {
			return nil, nil, nil
		}
		updates := map[string]any{
			"work_current_run_pause_depth": session.WorkCurrentRunPauseDepth + 1,
		}
		if session.WorkCurrentRunPauseDepth == 0 || session.WorkCurrentRunPausedAt == nil {
			updates["work_current_run_paused_at"] = event.Timestamp
		}
		return updates, nil, nil

	case "approval_res", "user_input_res":
		if session.WorkCurrentRunID == nil || strings.TrimSpace(*session.WorkCurrentRunID) != runID || session.WorkCurrentRunPauseDepth <= 0 {
			return nil, nil, nil
		}
		depth := session.WorkCurrentRunPauseDepth - 1
		updates := map[string]any{"work_current_run_pause_depth": depth}
		if depth == 0 {
			pausedDurationMs := session.WorkCurrentRunPausedDurationMs
			if session.WorkCurrentRunPausedAt != nil && event.Timestamp.After(*session.WorkCurrentRunPausedAt) {
				pausedDurationMs += event.Timestamp.Sub(*session.WorkCurrentRunPausedAt).Milliseconds()
			}
			updates["work_current_run_paused_at"] = nil
			updates["work_current_run_paused_duration_ms"] = pausedDurationMs
			if err := tx.WithContext(ctx).Model(&tables.WebSessionRunTimingTable{}).
				Where("web_session_id = ? AND run_id = ? AND ended_at IS NULL", sessionID, runID).
				Update("paused_duration_ms", pausedDurationMs).Error; err != nil {
				return nil, nil, err
			}
		}
		return updates, nil, nil
	}

	if !isWorkTimingTerminalEvent(event.Type) {
		return nil, nil, nil
	}
	return m.finishWorkTimingRunDB(ctx, tx, session, runID, event)
}

func settleRetryWaitIntoSourceRunDB(
	ctx context.Context,
	tx *gorm.DB,
	session tables.WebSessionTable,
	endedAt time.Time,
) (int64, *HistoryItem, error) {
	if session.WorkRetryWaitStartedAt == nil || !endedAt.After(*session.WorkRetryWaitStartedAt) {
		return 0, nil, nil
	}
	addedDurationMs := endedAt.Sub(*session.WorkRetryWaitStartedAt).Milliseconds()
	if session.WorkRetrySourceRunID == nil || strings.TrimSpace(*session.WorkRetrySourceRunID) == "" {
		return addedDurationMs, nil, nil
	}
	runID := strings.TrimSpace(*session.WorkRetrySourceRunID)
	var timing tables.WebSessionRunTimingTable
	if err := tx.WithContext(ctx).
		Where("web_session_id = ? AND run_id = ?", session.ID, runID).
		First(&timing).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return addedDurationMs, nil, nil
		}
		return 0, nil, err
	}
	nextDurationMs := maxInt64(0, timing.DurationMs) + addedDurationMs
	if err := tx.WithContext(ctx).Model(&timing).Update("duration_ms", nextDurationMs).Error; err != nil {
		return 0, nil, err
	}
	if timing.AnchorItemID == nil || strings.TrimSpace(*timing.AnchorItemID) == "" {
		return addedDurationMs, nil, nil
	}
	if err := tx.WithContext(ctx).Model(&tables.WebSessionItemTable{}).
		Where("web_session_id = ? AND id = ?", session.ID, strings.TrimSpace(*timing.AnchorItemID)).
		Update("run_duration_ms", nextDurationMs).Error; err != nil {
		return 0, nil, err
	}
	var row tables.WebSessionItemTable
	if err := tx.WithContext(ctx).
		Where("web_session_id = ? AND id = ?", session.ID, strings.TrimSpace(*timing.AnchorItemID)).
		First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return addedDurationMs, nil, nil
		}
		return 0, nil, err
	}
	item := mapHistoryItemRowWithSession(row, session.ID)
	return addedDurationMs, &item, nil
}

func (m *Manager) settleRetryWaitAndUpdateSession(
	ctx context.Context,
	sessionID string,
	endedAt time.Time,
	updates map[string]any,
) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	merged := make(map[string]any, len(updates)+3)
	for key, value := range updates {
		merged[key] = value
	}
	merged["work_retry_wait_started_at"] = nil
	merged["work_retry_source_run_id"] = nil

	var timingItem *HistoryItem
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session tables.WebSessionTable
		if err := tx.WithContext(ctx).First(&session, "id = ?", strings.TrimSpace(sessionID)).Error; err != nil {
			return err
		}
		addedDurationMs, updatedItem, err := settleRetryWaitIntoSourceRunDB(ctx, tx, session, endedAt)
		if err != nil {
			return err
		}
		if addedDurationMs > 0 {
			merged["work_duration_ms"] = maxInt64(0, session.WorkDurationMs) + addedDurationMs
		}
		timingItem = updatedItem
		return m.updateRuntimeStateDB(ctx, tx, session.ID, merged)
	})
	if err != nil {
		return err
	}
	if timingItem != nil {
		m.broadcast(newHistoryItemFrame(sessionID, *timingItem, m.summaryForBroadcast(ctx, sessionID)))
	}
	m.broadcastSessionSummary(ctx, sessionID)
	return nil
}

func upsertStartedRunTimingDB(
	ctx context.Context,
	tx *gorm.DB,
	sessionID string,
	runID string,
	startedAt time.Time,
) error {
	var existing tables.WebSessionRunTimingTable
	err := tx.WithContext(ctx).
		Where("web_session_id = ? AND run_id = ?", sessionID, runID).
		First(&existing).Error
	if err == nil {
		if existing.StartedAt.IsZero() && existing.EndedAt == nil {
			return tx.WithContext(ctx).Model(&existing).Update("started_at", startedAt).Error
		}
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	record := tables.WebSessionRunTimingTable{
		WebSessionID: sessionID,
		RunID:        runID,
		StartedAt:    startedAt,
	}
	record.Init()
	return tx.WithContext(ctx).Create(&record).Error
}

func (m *Manager) finishWorkTimingRunDB(
	ctx context.Context,
	tx *gorm.DB,
	session tables.WebSessionTable,
	runID string,
	event Event,
) (map[string]any, *HistoryItem, error) {
	var timing tables.WebSessionRunTimingTable
	err := tx.WithContext(ctx).
		Where("web_session_id = ? AND run_id = ?", session.ID, runID).
		First(&timing).Error
	if err == gorm.ErrRecordNotFound {
		if session.WorkCurrentRunStartedAt == nil {
			return clearCurrentWorkTimingUpdates(session, runID), nil, nil
		}
		timing = tables.WebSessionRunTimingTable{
			WebSessionID: session.ID,
			RunID:        runID,
			StartedAt:    *session.WorkCurrentRunStartedAt,
		}
		timing.Init()
		if err := tx.WithContext(ctx).Create(&timing).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	if timing.EndedAt != nil {
		return clearCurrentWorkTimingUpdates(session, runID), nil, nil
	}

	pausedDurationMs := maxInt64(timing.PausedDurationMs, session.WorkCurrentRunPausedDurationMs)
	if session.WorkCurrentRunPausedAt != nil && event.Timestamp.After(*session.WorkCurrentRunPausedAt) {
		pausedDurationMs += event.Timestamp.Sub(*session.WorkCurrentRunPausedAt).Milliseconds()
	}
	durationMs := workTimingDurationMs(timing.StartedAt, event.Timestamp, pausedDurationMs)
	outcome := workTimingOutcomeForEvent(event)

	anchor, err := m.findWorkTimingAnchorDB(ctx, tx, session, runID, event)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, nil, err
	}
	updates := map[string]any{
		"ended_at":           event.Timestamp,
		"paused_duration_ms": pausedDurationMs,
		"duration_ms":        durationMs,
		"outcome":            string(outcome),
		"terminal_event_seq": event.Seq,
	}
	if anchor != nil {
		updates["anchor_item_id"] = anchor.ID
		updates["anchor_source_thread_id"] = anchor.SourceThreadID
		updates["anchor_source_turn_id"] = anchor.SourceTurnID
		updates["anchor_source_item_id"] = anchor.SourceItemID
		if err := tx.WithContext(ctx).Model(&tables.WebSessionItemTable{}).
			Where("id = ? AND web_session_id = ?", anchor.ID, session.ID).
			Updates(map[string]any{
				"run_id":          runID,
				"run_duration_ms": durationMs,
				"run_outcome":     string(outcome),
			}).Error; err != nil {
			return nil, nil, err
		}
		value := durationMs
		anchor.RunID = &runID
		anchor.RunDurationMs = &value
		anchor.RunOutcome = outcome
	}
	if err := tx.WithContext(ctx).Model(&timing).Updates(updates).Error; err != nil {
		return nil, nil, err
	}

	sessionUpdates := clearCurrentWorkTimingUpdates(session, runID)
	sessionUpdates["work_duration_ms"] = maxInt64(0, session.WorkDurationMs) + durationMs
	return sessionUpdates, anchor, nil
}

func clearCurrentWorkTimingUpdates(session tables.WebSessionTable, runID string) map[string]any {
	if session.WorkCurrentRunID == nil || strings.TrimSpace(*session.WorkCurrentRunID) != strings.TrimSpace(runID) {
		return nil
	}
	return map[string]any{
		"work_current_run_id":                 nil,
		"work_current_run_started_at":         nil,
		"work_current_run_paused_at":          nil,
		"work_current_run_paused_duration_ms": 0,
		"work_current_run_pause_depth":        0,
	}
}

func (m *Manager) beginWorkTimingContinuation(
	ctx context.Context,
	sessionID string,
	startedAt time.Time,
	sourceRunID string,
) error {
	if m == nil {
		return model.ErrDBNotInitialized
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	sourceRunID = strings.TrimSpace(sourceRunID)
	if sourceRunID == "" {
		sourceRunID = m.latestCompletedWorkTimingRunID(ctx, sessionID)
	}
	if err := m.updateRuntimeState(ctx, sessionID, map[string]any{
		"work_retry_wait_started_at": startedAt,
		"work_retry_source_run_id":   nilIfEmpty(sourceRunID),
		"updated_at":                 time.Now(),
	}); err != nil {
		return err
	}
	m.broadcastSessionSummary(ctx, sessionID)
	return nil
}

func (m *Manager) findWorkTimingAnchorDB(
	ctx context.Context,
	tx *gorm.DB,
	session tables.WebSessionTable,
	runID string,
	event Event,
) (*HistoryItem, error) {
	rootQuery := func(query *gorm.DB) *gorm.DB {
		if session.NativeSessionID == nil || strings.TrimSpace(*session.NativeSessionID) == "" {
			return query.Where("source_thread_id IS NULL OR source_thread_id = ''")
		}
		rootID := strings.TrimSpace(*session.NativeSessionID)
		return query.Where("source_thread_id IS NULL OR source_thread_id = '' OR source_thread_id = ?", rootID)
	}

	var row tables.WebSessionItemTable
	query := tx.WithContext(ctx).Where(
		"web_session_id = ? AND run_id = ? AND item_kind = ? AND done = ?",
		session.ID,
		runID,
		"assistant",
		true,
	)
	if err := rootQuery(query).Order("order_index DESC").First(&row).Error; err == nil {
		item := mapHistoryItemRowWithSession(row, session.ID)
		return &item, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	terminalType := strings.TrimSpace(event.Type)
	if terminalType == "run_abort" || terminalType == "run_fail" {
		if err := tx.WithContext(ctx).
			Where("web_session_id = ? AND item_type = ?", session.ID, terminalType).
			Order("order_index DESC").First(&row).Error; err == nil {
			item := mapHistoryItemRowWithSession(row, session.ID)
			return &item, nil
		} else if err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}

	query = tx.WithContext(ctx).Where("web_session_id = ? AND run_id = ? AND item_kind != ?", session.ID, runID, "tool")
	if err := rootQuery(query).Order("order_index DESC").First(&row).Error; err != nil {
		return nil, err
	}
	item := mapHistoryItemRowWithSession(row, session.ID)
	return &item, nil
}

func validateWorkTimingState(record tables.WebSessionRunTimingTable) error {
	if strings.TrimSpace(record.WebSessionID) == "" || strings.TrimSpace(record.RunID) == "" {
		return fmt.Errorf("work timing identity is required")
	}
	return nil
}

func (m *Manager) reapplyWorkTimingAnnotationsDB(
	ctx context.Context,
	tx *gorm.DB,
	sessionID string,
) error {
	var timings []tables.WebSessionRunTimingTable
	if err := tx.WithContext(ctx).
		Where("web_session_id = ? AND ended_at IS NOT NULL", sessionID).
		Order("ended_at ASC").
		Find(&timings).Error; err != nil {
		return err
	}
	if len(timings) == 0 {
		return nil
	}

	var rows []tables.WebSessionItemTable
	if err := tx.WithContext(ctx).
		Where("web_session_id = ?", sessionID).
		Order("order_index ASC").
		Find(&rows).Error; err != nil {
		return err
	}
	for index := range timings {
		timing := &timings[index]
		anchor := findReplacementWorkTimingAnchor(rows, *timing)
		if anchor == nil {
			continue
		}
		if err := tx.WithContext(ctx).Model(&tables.WebSessionItemTable{}).
			Where("id = ? AND web_session_id = ?", anchor.ID, sessionID).
			Updates(map[string]any{
				"run_id":          timing.RunID,
				"run_duration_ms": timing.DurationMs,
				"run_outcome":     timing.Outcome,
			}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(timing).Updates(map[string]any{
			"anchor_item_id":          anchor.ID,
			"anchor_source_thread_id": anchor.SourceThreadID,
			"anchor_source_turn_id":   anchor.SourceTurnID,
			"anchor_source_item_id":   anchor.SourceItemID,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func findReplacementWorkTimingAnchor(
	rows []tables.WebSessionItemTable,
	timing tables.WebSessionRunTimingTable,
) *tables.WebSessionItemTable {
	threadID := normalizedTimingIdentity(timing.AnchorSourceThreadID)
	turnID := normalizedTimingIdentity(timing.AnchorSourceTurnID)
	itemID := normalizedTimingIdentity(timing.AnchorSourceItemID)
	itemIDWithoutPrefix := strings.TrimPrefix(itemID, "assistant:")

	for index := len(rows) - 1; index >= 0; index-- {
		row := &rows[index]
		rowItemID := normalizedTimingIdentity(row.SourceItemID)
		if itemID != "" &&
			(rowItemID == itemID || strings.TrimPrefix(rowItemID, "assistant:") == itemIDWithoutPrefix) &&
			timingIdentityMatches(threadID, normalizedTimingIdentity(row.SourceThreadID)) {
			return row
		}
	}
	for index := len(rows) - 1; index >= 0; index-- {
		row := &rows[index]
		if turnID != "" && normalizedTimingIdentity(row.SourceTurnID) == turnID &&
			timingIdentityMatches(threadID, normalizedTimingIdentity(row.SourceThreadID)) &&
			row.ItemKind == "assistant" && row.Done {
			return row
		}
	}
	return nil
}

func normalizedTimingIdentity(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func timingIdentityMatches(expected, actual string) bool {
	return expected == "" || expected == actual
}
