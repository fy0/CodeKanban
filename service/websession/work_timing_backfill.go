package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

const (
	defaultWorkTimingBackfillLimit = 50
	maxWorkTimingBackfillLimit     = 500
)

var (
	ErrInvalidWorkTimingBackfill = errors.New("invalid work timing backfill request")
	ErrWorkTimingBackfillBusy    = errors.New("work timing backfill is already running")
)

type WorkTimingCalculationStatus string

const (
	WorkTimingCalculationCalculated     WorkTimingCalculationStatus = "calculated"
	WorkTimingCalculationAlreadyCurrent WorkTimingCalculationStatus = "already_current"
	WorkTimingCalculationBusy           WorkTimingCalculationStatus = "busy"
	WorkTimingCalculationPartial        WorkTimingCalculationStatus = "partial"
	WorkTimingCalculationUnavailable    WorkTimingCalculationStatus = "unavailable"
	WorkTimingCalculationFailed         WorkTimingCalculationStatus = "failed"
)

type WorkTimingItemPatch struct {
	ItemID        string            `json:"itemId"`
	RunID         string            `json:"runId"`
	RunDurationMs int64             `json:"runDurationMs"`
	RunOutcome    WorkTimingOutcome `json:"runOutcome"`
}

type WorkTimingCalculationResult struct {
	Status  WorkTimingCalculationStatus `json:"status"`
	Session SessionSummary              `json:"session"`
	Items   []WorkTimingItemPatch       `json:"items"`
	Error   string                      `json:"error,omitempty"`
}

type WorkTimingBackfillParams struct {
	Limit int `json:"limit"`
}

type WorkTimingBackfillStatus struct {
	RemainingSessionCount   int64 `json:"remainingSessionCount"`
	BusySessionCount        int64 `json:"busySessionCount"`
	CompleteSessionCount    int64 `json:"completeSessionCount"`
	PartialSessionCount     int64 `json:"partialSessionCount"`
	UnavailableSessionCount int64 `json:"unavailableSessionCount"`
	FailedSessionCount      int64 `json:"failedSessionCount"`
}

type WorkTimingBackfillResult struct {
	WorkTimingBackfillStatus
	AttemptedSessionCount  int `json:"attemptedSessionCount"`
	CalculatedSessionCount int `json:"calculatedSessionCount"`
	PartialResultCount     int `json:"partialResultCount"`
	UnavailableResultCount int `json:"unavailableResultCount"`
	FailedResultCount      int `json:"failedResultCount"`
}

type scannedWorkTimingAnchor struct {
	SourceThreadID *string
	SourceTurnID   *string
	SourceItemID   *string
	TerminalType   string
	TerminalAt     time.Time
}

type scannedWorkTimingRun struct {
	RunID            string
	StartedAt        time.Time
	EndedAt          time.Time
	PausedDurationMs int64
	DurationMs       int64
	Outcome          WorkTimingOutcome
	TerminalEventSeq int64
	Anchor           scannedWorkTimingAnchor
}

type workTimingScanState struct {
	runID            string
	startedAt        time.Time
	pausedAt         *time.Time
	pausedDurationMs int64
	pauseDepth       int
	anchor           scannedWorkTimingAnchor
	ended            bool
}

type workTimingScanResult struct {
	runs           []scannedWorkTimingRun
	incomplete     bool
	missing        bool
	lastTerminalAt time.Time
}

func (m *Manager) CalculateSessionWorkTiming(
	ctx context.Context,
	sessionID string,
) (WorkTimingCalculationResult, error) {
	return m.calculateSessionWorkTiming(ctx, sessionID, true, false)
}

func (m *Manager) calculateSessionWorkTiming(
	ctx context.Context,
	sessionID string,
	broadcast bool,
	tryLock bool,
) (WorkTimingCalculationResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return WorkTimingCalculationResult{}, err
	}
	if workTimingSessionBusy(record) {
		return WorkTimingCalculationResult{
			Status:  WorkTimingCalculationBusy,
			Session: m.mapSessionSummary(record),
			Items:   []WorkTimingItemPatch{},
		}, nil
	}

	lock := &m.workTimingLocks[sessionRevisionLockIndex(sessionID)]
	if tryLock {
		if !lock.TryLock() {
			return WorkTimingCalculationResult{
				Status:  WorkTimingCalculationBusy,
				Session: m.mapSessionSummary(record),
				Items:   []WorkTimingItemPatch{},
			}, nil
		}
	} else {
		lock.Lock()
	}
	defer lock.Unlock()

	record, err = m.GetSession(ctx, sessionID)
	if err != nil {
		return WorkTimingCalculationResult{}, err
	}
	if workTimingSessionBusy(record) {
		return WorkTimingCalculationResult{
			Status:  WorkTimingCalculationBusy,
			Session: m.mapSessionSummary(record),
			Items:   []WorkTimingItemPatch{},
		}, nil
	}
	state := normalizeWorkTimingBackfillState(record.WorkTimingBackfillState)
	if record.WorkTimingBackfillVersion >= currentWorkTimingBackfillVersion &&
		state != WorkTimingBackfillPending && state != WorkTimingBackfillFailed {
		return WorkTimingCalculationResult{
			Status:  calculationStatusForBackfillState(state, true),
			Session: m.mapSessionSummary(record),
			Items:   []WorkTimingItemPatch{},
		}, nil
	}

	scan, scanErr := m.scanSessionWorkTimingEvents(record)
	if scanErr != nil {
		if err := m.markSessionWorkTimingBackfillFailed(ctx, sessionID); err != nil {
			return WorkTimingCalculationResult{}, err
		}
		refreshed, err := m.GetSession(ctx, sessionID)
		if err != nil {
			return WorkTimingCalculationResult{}, err
		}
		return WorkTimingCalculationResult{
			Status:  WorkTimingCalculationFailed,
			Session: m.mapSessionSummary(refreshed),
			Items:   []WorkTimingItemPatch{},
			Error:   scanErr.Error(),
		}, nil
	}

	backfillState, patches, err := m.persistScannedWorkTiming(ctx, record, scan)
	if err != nil {
		return WorkTimingCalculationResult{}, err
	}
	refreshed, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return WorkTimingCalculationResult{}, err
	}
	if broadcast {
		m.broadcastSessionSummary(ctx, sessionID)
	}
	return WorkTimingCalculationResult{
		Status:  calculationStatusForBackfillState(backfillState, false),
		Session: m.mapSessionSummary(refreshed),
		Items:   patches,
	}, nil
}

func calculationStatusForBackfillState(
	state WorkTimingBackfillState,
	alreadyCurrent bool,
) WorkTimingCalculationStatus {
	switch state {
	case WorkTimingBackfillPartial:
		return WorkTimingCalculationPartial
	case WorkTimingBackfillUnavailable:
		return WorkTimingCalculationUnavailable
	case WorkTimingBackfillFailed:
		return WorkTimingCalculationFailed
	default:
		if alreadyCurrent {
			return WorkTimingCalculationAlreadyCurrent
		}
		return WorkTimingCalculationCalculated
	}
}

func workTimingSessionBusy(record tables.WebSessionTable) bool {
	switch effectiveStatus(record, effectiveAssistantState(record)) {
	case StatusRunning, StatusWaitingApproval, StatusAborting:
		return true
	default:
		return false
	}
}

func (m *Manager) scanSessionWorkTimingEvents(
	session tables.WebSessionTable,
) (workTimingScanResult, error) {
	file, err := os.Open(m.store.historyPath(session.ID))
	if err != nil {
		if os.IsNotExist(err) {
			return workTimingScanResult{missing: true}, nil
		}
		return workTimingScanResult{}, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	states := make(map[string]*workTimingScanState)
	activeRunID := ""
	result := workTimingScanResult{runs: make([]scannedWorkTimingRun, 0, 16)}
	var previousSeq int64

	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			return workTimingScanResult{}, readErr
		}
		if readErr == io.EOF && len(line) > 0 {
			return workTimingScanResult{}, fmt.Errorf("work timing history has an incomplete trailing event")
		}
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var event Event
			if err := json.Unmarshal(line, &event); err != nil {
				return workTimingScanResult{}, fmt.Errorf("decode work timing event: %w", err)
			}
			if event.Seq <= 0 || event.Seq <= previousSeq {
				return workTimingScanResult{}, fmt.Errorf("work timing event sequence is invalid")
			}
			previousSeq = event.Seq
			applyScannedWorkTimingEvent(&result, states, &activeRunID, session, event)
		}
		if readErr == io.EOF {
			break
		}
	}
	for _, state := range states {
		if !state.ended {
			result.incomplete = true
		}
	}
	return result, nil
}

func applyScannedWorkTimingEvent(
	result *workTimingScanResult,
	states map[string]*workTimingScanState,
	activeRunID *string,
	session tables.WebSessionTable,
	event Event,
) {
	eventType := strings.TrimSpace(event.Type)
	runID := strings.TrimSpace(event.RunID)
	if runID == "" && eventType != "run_st" {
		runID = strings.TrimSpace(*activeRunID)
	}

	switch eventType {
	case "run_st":
		if runID == "" || event.Timestamp.IsZero() {
			result.incomplete = true
			return
		}
		if current := states[runID]; current != nil && !current.ended {
			result.incomplete = true
			return
		}
		startedAt := event.Timestamp
		if isWorkTimingContinuationPayload(event.Payload) &&
			!result.lastTerminalAt.IsZero() &&
			event.Timestamp.After(result.lastTerminalAt) {
			startedAt = result.lastTerminalAt
		}
		states[runID] = &workTimingScanState{runID: runID, startedAt: startedAt}
		*activeRunID = runID
		return
	case "txt_end":
		state := states[runID]
		if state == nil || state.ended || !isRootWorkTimingEvent(session, event) {
			return
		}
		messageID := strings.TrimSpace(firstNonEmpty(stringValue(event.Payload["mid"]), event.ParentID, event.ID))
		if messageID == "" {
			return
		}
		sourceItemID := "assistant:" + messageID
		state.anchor = scannedWorkTimingAnchor{
			SourceThreadID: nilIfEmptyHistory(event.ThreadID),
			SourceTurnID:   nilIfEmptyHistory(event.TurnID),
			SourceItemID:   &sourceItemID,
		}
		return
	case "approval_req", "user_input_req":
		state := states[runID]
		if state == nil || state.ended || event.Timestamp.IsZero() {
			result.incomplete = true
			return
		}
		state.pauseDepth++
		if state.pauseDepth == 1 {
			value := event.Timestamp
			state.pausedAt = &value
		}
		return
	case "approval_res", "user_input_res":
		state := states[runID]
		if state == nil || state.ended || state.pauseDepth <= 0 || event.Timestamp.IsZero() {
			result.incomplete = true
			return
		}
		state.pauseDepth--
		if state.pauseDepth == 0 && state.pausedAt != nil {
			if event.Timestamp.Before(*state.pausedAt) {
				result.incomplete = true
			} else {
				state.pausedDurationMs += event.Timestamp.Sub(*state.pausedAt).Milliseconds()
			}
			state.pausedAt = nil
		}
		return
	}

	if !isWorkTimingTerminalEvent(eventType) {
		return
	}
	state := states[runID]
	if state == nil || state.ended || event.Timestamp.IsZero() || event.Timestamp.Before(state.startedAt) {
		result.incomplete = true
		return
	}
	if state.pausedAt != nil {
		state.pausedDurationMs += event.Timestamp.Sub(*state.pausedAt).Milliseconds()
		state.pausedAt = nil
		state.pauseDepth = 0
	}
	state.ended = true
	result.lastTerminalAt = event.Timestamp
	if *activeRunID == runID {
		*activeRunID = ""
	}
	anchor := state.anchor
	if anchor.SourceItemID == nil && (eventType == "run_abort" || eventType == "run_fail") {
		anchor.TerminalType = eventType
		anchor.TerminalAt = event.Timestamp
	}
	result.runs = append(result.runs, scannedWorkTimingRun{
		RunID:            runID,
		StartedAt:        state.startedAt,
		EndedAt:          event.Timestamp,
		PausedDurationMs: state.pausedDurationMs,
		DurationMs:       workTimingDurationMs(state.startedAt, event.Timestamp, state.pausedDurationMs),
		Outcome:          workTimingOutcomeForEvent(event),
		TerminalEventSeq: event.Seq,
		Anchor:           anchor,
	})
}

func isRootWorkTimingEvent(session tables.WebSessionTable, event Event) bool {
	threadID := strings.TrimSpace(event.ThreadID)
	if threadID == "" {
		return true
	}
	return session.NativeSessionID != nil && strings.TrimSpace(*session.NativeSessionID) == threadID
}

func (m *Manager) persistScannedWorkTiming(
	ctx context.Context,
	session tables.WebSessionTable,
	scan workTimingScanResult,
) (WorkTimingBackfillState, []WorkTimingItemPatch, error) {
	db := model.GetDB()
	if db == nil {
		return "", nil, model.ErrDBNotInitialized
	}
	state := WorkTimingBackfillUnavailable
	patches := []WorkTimingItemPatch{}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, run := range scan.runs {
			if err := persistScannedWorkTimingRunDB(ctx, tx, session, run); err != nil {
				return err
			}
		}
		if err := m.reapplyWorkTimingAnnotationsDB(ctx, tx, session.ID); err != nil {
			return err
		}
		var timingCount int64
		if err := tx.WithContext(ctx).Model(&tables.WebSessionRunTimingTable{}).
			Where("web_session_id = ? AND ended_at IS NOT NULL", session.ID).
			Count(&timingCount).Error; err != nil {
			return err
		}
		if timingCount > 0 {
			if scan.missing || scan.incomplete {
				state = WorkTimingBackfillPartial
			} else {
				state = WorkTimingBackfillComplete
			}
		}
		var totalDurationMs int64
		if err := tx.WithContext(ctx).Model(&tables.WebSessionRunTimingTable{}).
			Where("web_session_id = ? AND ended_at IS NOT NULL", session.ID).
			Select("COALESCE(SUM(duration_ms), 0)").
			Scan(&totalDurationMs).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&tables.WebSessionTable{}).
			Where("id = ?", session.ID).
			Updates(withSnapshotRevisionIncrement(map[string]any{
				"work_duration_ms":             totalDurationMs,
				"work_timing_backfill_state":   string(state),
				"work_timing_backfill_version": currentWorkTimingBackfillVersion,
			})).Error; err != nil {
			return err
		}
		var rows []tables.WebSessionItemTable
		if err := tx.WithContext(ctx).
			Where("web_session_id = ? AND run_duration_ms IS NOT NULL", session.ID).
			Order("order_index ASC").
			Find(&rows).Error; err != nil {
			return err
		}
		patches = make([]WorkTimingItemPatch, 0, len(rows))
		for _, row := range rows {
			if row.RunID == nil || row.RunDurationMs == nil {
				continue
			}
			patches = append(patches, WorkTimingItemPatch{
				ItemID:        row.ID,
				RunID:         strings.TrimSpace(*row.RunID),
				RunDurationMs: *row.RunDurationMs,
				RunOutcome:    WorkTimingOutcome(row.RunOutcome),
			})
		}
		return nil
	})
	return state, patches, err
}

func persistScannedWorkTimingRunDB(
	ctx context.Context,
	tx *gorm.DB,
	session tables.WebSessionTable,
	run scannedWorkTimingRun,
) error {
	var timing tables.WebSessionRunTimingTable
	err := tx.WithContext(ctx).
		Where("web_session_id = ? AND run_id = ?", session.ID, run.RunID).
		First(&timing).Error
	if err == gorm.ErrRecordNotFound {
		timing = tables.WebSessionRunTimingTable{
			WebSessionID:     session.ID,
			RunID:            run.RunID,
			StartedAt:        run.StartedAt,
			EndedAt:          &run.EndedAt,
			PausedDurationMs: run.PausedDurationMs,
			DurationMs:       run.DurationMs,
			Outcome:          string(run.Outcome),
			TerminalEventSeq: run.TerminalEventSeq,
			Backfilled:       true,
		}
		timing.Init()
		if err := tx.WithContext(ctx).Create(&timing).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if timing.Backfilled || timing.EndedAt == nil {
		if err := tx.WithContext(ctx).Model(&timing).Updates(map[string]any{
			"started_at":         run.StartedAt,
			"ended_at":           run.EndedAt,
			"paused_duration_ms": run.PausedDurationMs,
			"duration_ms":        run.DurationMs,
			"outcome":            string(run.Outcome),
			"terminal_event_seq": run.TerminalEventSeq,
			"backfilled":         true,
		}).Error; err != nil {
			return err
		}
		timing.StartedAt = run.StartedAt
		timing.EndedAt = &run.EndedAt
		timing.DurationMs = run.DurationMs
		timing.Outcome = string(run.Outcome)
	}

	anchor, err := findScannedWorkTimingAnchorDB(ctx, tx, session.ID, run)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if err := tx.WithContext(ctx).Model(&tables.WebSessionItemTable{}).
		Where("id = ? AND web_session_id = ?", anchor.ID, session.ID).
		Updates(map[string]any{
			"run_id":          run.RunID,
			"run_duration_ms": timing.DurationMs,
			"run_outcome":     timing.Outcome,
		}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&timing).Updates(map[string]any{
		"anchor_item_id":          anchor.ID,
		"anchor_source_thread_id": anchor.SourceThreadID,
		"anchor_source_turn_id":   anchor.SourceTurnID,
		"anchor_source_item_id":   anchor.SourceItemID,
	}).Error
}

func findScannedWorkTimingAnchorDB(
	ctx context.Context,
	tx *gorm.DB,
	sessionID string,
	run scannedWorkTimingRun,
) (tables.WebSessionItemTable, error) {
	var row tables.WebSessionItemTable
	if run.Anchor.SourceItemID != nil && strings.TrimSpace(*run.Anchor.SourceItemID) != "" {
		itemID := strings.TrimSpace(*run.Anchor.SourceItemID)
		candidates := []string{itemID, strings.TrimPrefix(itemID, "assistant:")}
		query := tx.WithContext(ctx).
			Where("web_session_id = ? AND source_item_id IN ?", sessionID, candidates)
		if run.Anchor.SourceThreadID != nil && strings.TrimSpace(*run.Anchor.SourceThreadID) != "" {
			query = query.Where("source_thread_id = ?", strings.TrimSpace(*run.Anchor.SourceThreadID))
		}
		if err := query.Order("order_index DESC").First(&row).Error; err == nil {
			return row, nil
		} else if err != gorm.ErrRecordNotFound {
			return row, err
		}
	}
	if run.Anchor.SourceTurnID != nil && strings.TrimSpace(*run.Anchor.SourceTurnID) != "" {
		query := tx.WithContext(ctx).Where(
			"web_session_id = ? AND source_turn_id = ? AND item_kind = ? AND done = ?",
			sessionID,
			strings.TrimSpace(*run.Anchor.SourceTurnID),
			"assistant",
			true,
		)
		if err := query.Order("order_index DESC").First(&row).Error; err == nil {
			return row, nil
		} else if err != gorm.ErrRecordNotFound {
			return row, err
		}
	}
	if run.Anchor.TerminalType != "" && !run.Anchor.TerminalAt.IsZero() {
		if err := tx.WithContext(ctx).Where(
			"web_session_id = ? AND item_type = ? AND timestamp = ?",
			sessionID,
			run.Anchor.TerminalType,
			run.Anchor.TerminalAt,
		).Order("order_index DESC").First(&row).Error; err != nil {
			return row, err
		}
		return row, nil
	}
	return row, gorm.ErrRecordNotFound
}

func (m *Manager) markSessionWorkTimingBackfillFailed(ctx context.Context, sessionID string) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	return db.WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("id = ?", sessionID).
		Updates(withSnapshotRevisionIncrement(map[string]any{
			"work_timing_backfill_state":   string(WorkTimingBackfillFailed),
			"work_timing_backfill_version": currentWorkTimingBackfillVersion,
		})).Error
}

func (m *Manager) WorkTimingBackfillStatus(ctx context.Context) (WorkTimingBackfillStatus, error) {
	db := model.GetDB()
	if db == nil {
		return WorkTimingBackfillStatus{}, model.ErrDBNotInitialized
	}
	status := WorkTimingBackfillStatus{}
	pendingStates := []string{string(WorkTimingBackfillPending), string(WorkTimingBackfillFailed)}
	if err := db.WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("work_timing_backfill_version < ? OR work_timing_backfill_state IN ?",
			currentWorkTimingBackfillVersion, pendingStates).
		Count(&status.RemainingSessionCount).Error; err != nil {
		return status, err
	}
	if err := db.WithContext(ctx).Model(&tables.WebSessionTable{}).
		Where("work_timing_backfill_version < ? OR work_timing_backfill_state IN ?",
			currentWorkTimingBackfillVersion, pendingStates).
		Where("status IN ?", []string{
			string(StatusRunning),
			string(StatusWaitingApproval),
			string(StatusAborting),
		}).
		Count(&status.BusySessionCount).Error; err != nil {
		return status, err
	}
	counts := []struct {
		State string
		Count int64
	}{}
	if err := db.WithContext(ctx).Model(&tables.WebSessionTable{}).
		Select("work_timing_backfill_state AS state, COUNT(*) AS count").
		Where("work_timing_backfill_version >= ?", currentWorkTimingBackfillVersion).
		Group("work_timing_backfill_state").
		Scan(&counts).Error; err != nil {
		return status, err
	}
	for _, item := range counts {
		switch normalizeWorkTimingBackfillState(item.State) {
		case WorkTimingBackfillComplete:
			status.CompleteSessionCount = item.Count
		case WorkTimingBackfillPartial:
			status.PartialSessionCount = item.Count
		case WorkTimingBackfillUnavailable:
			status.UnavailableSessionCount = item.Count
		case WorkTimingBackfillFailed:
			status.FailedSessionCount = item.Count
		}
	}
	return status, nil
}

func (m *Manager) RunWorkTimingBackfill(
	ctx context.Context,
	params WorkTimingBackfillParams,
) (WorkTimingBackfillResult, error) {
	limit := params.Limit
	if limit == 0 {
		limit = defaultWorkTimingBackfillLimit
	}
	if limit < 1 || limit > maxWorkTimingBackfillLimit {
		return WorkTimingBackfillResult{}, ErrInvalidWorkTimingBackfill
	}
	if !m.workTimingBackfillMu.TryLock() {
		return WorkTimingBackfillResult{}, ErrWorkTimingBackfillBusy
	}
	defer m.workTimingBackfillMu.Unlock()

	db := model.GetDB()
	if db == nil {
		return WorkTimingBackfillResult{}, model.ErrDBNotInitialized
	}
	var sessions []tables.WebSessionTable
	if err := db.WithContext(ctx).
		Where("(work_timing_backfill_version < ? OR work_timing_backfill_state IN ?) AND status NOT IN ?",
			currentWorkTimingBackfillVersion,
			[]string{string(WorkTimingBackfillPending), string(WorkTimingBackfillFailed)},
			[]string{string(StatusRunning), string(StatusAborting), string(StatusWaitingApproval)}).
		Order("CASE WHEN work_timing_backfill_state = 'failed' THEN 1 ELSE 0 END ASC").
		Order("created_at ASC").
		Limit(limit).
		Find(&sessions).Error; err != nil {
		return WorkTimingBackfillResult{}, err
	}

	result := WorkTimingBackfillResult{}
	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		calculation, err := m.calculateSessionWorkTiming(ctx, session.ID, false, true)
		result.AttemptedSessionCount++
		if err != nil {
			result.FailedResultCount++
			continue
		}
		switch calculation.Status {
		case WorkTimingCalculationCalculated, WorkTimingCalculationAlreadyCurrent:
			result.CalculatedSessionCount++
		case WorkTimingCalculationPartial:
			result.PartialResultCount++
		case WorkTimingCalculationUnavailable:
			result.UnavailableResultCount++
		case WorkTimingCalculationFailed:
			result.FailedResultCount++
		case WorkTimingCalculationBusy:
			result.AttemptedSessionCount--
		}
	}
	status, err := m.WorkTimingBackfillStatus(ctx)
	if err != nil {
		return result, err
	}
	result.WorkTimingBackfillStatus = status
	return result, nil
}
