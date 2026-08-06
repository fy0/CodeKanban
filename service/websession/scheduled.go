package websession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	gitutil "code-kanban/utils/git"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	errInvalidScheduledInputAction = errors.New("invalid scheduled input action")
	errInvalidScheduledInputMode   = errors.New("invalid scheduled input mode")
	errScheduledInputNotFound      = errors.New("scheduled input not found")
	errScheduledInputNotEditable   = errors.New("scheduled input can no longer be changed")
	errScheduledInputNotExecutable = errors.New("scheduled input can no longer be executed")
	errScheduledPlanExpired        = errors.New("scheduled plan is no longer available")
	errScheduledPlanDuplicate      = errors.New("scheduled plan already exists")
)

const scheduledIdleRequiredChecks = 3

var (
	scheduledIdlePollInterval = 10 * time.Second
	// IdleSince is recorded by the first successful check. Two more intervals
	// provide three checks over roughly 30 seconds from scheduling.
	scheduledIdleStablePeriod     = time.Duration(scheduledIdleRequiredChecks-1) * scheduledIdlePollInterval
	scheduledIdleConditionTimeout = 30 * time.Second
)

type scheduledIdleGitCheck struct {
	dirty bool
	err   error
}

type scheduledIdleSessionCheck struct {
	blocked bool
	err     error
}

type scheduledIdleSweepCache struct {
	gitByPath         map[string]scheduledIdleGitCheck
	sessionsByProject map[string]scheduledIdleSessionCheck
}

type scheduledIdleConditionEvaluator func(
	ctx context.Context,
	target tables.WebSessionTable,
	cache *scheduledIdleSweepCache,
) ([]ScheduledInputBlockingReason, string, error)

func newScheduledIdleSweepCache() *scheduledIdleSweepCache {
	return &scheduledIdleSweepCache{
		gitByPath:         make(map[string]scheduledIdleGitCheck),
		sessionsByProject: make(map[string]scheduledIdleSessionCheck),
	}
}

type scheduledPlanExecutionPayload struct {
	PendingItemID      string `json:"pendingItemId,omitempty"`
	QuestionID         string `json:"questionId,omitempty"`
	ExecuteOptionLabel string `json:"executeOptionLabel,omitempty"`
}

type scheduledInputUpdate struct {
	Text         *string
	Mode         *ScheduledInputMode
	ScheduleKind *ScheduledInputScheduleKind
	ScheduledFor time.Time
}

func (m *Manager) withScheduledInputLock(inputID string, fn func() error) error {
	normalizedInputID := strings.TrimSpace(inputID)
	if normalizedInputID == "" {
		return errScheduledInputNotFound
	}
	hash := uint32(2166136261)
	for index := 0; index < len(normalizedInputID); index++ {
		hash ^= uint32(normalizedInputID[index])
		hash *= 16777619
	}
	lock := &m.scheduledInputLocks[hash%uint32(len(m.scheduledInputLocks))]
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

func loadScheduledInputRecord(
	ctx context.Context,
	sessionID string,
	inputID string,
) (tables.WebSessionScheduledInputTable, error) {
	db := model.GetDB()
	if db == nil {
		return tables.WebSessionScheduledInputTable{}, model.ErrDBNotInitialized
	}
	var record tables.WebSessionScheduledInputTable
	err := db.WithContext(ctx).
		Where("id = ? AND web_session_id = ?", strings.TrimSpace(inputID), strings.TrimSpace(sessionID)).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tables.WebSessionScheduledInputTable{}, errScheduledInputNotFound
	}
	return record, err
}

func normalizeScheduledInputAction(action ScheduledInputAction) ScheduledInputAction {
	switch strings.ToLower(strings.TrimSpace(string(action))) {
	case "", string(ScheduledInputActionMessage):
		return ScheduledInputActionMessage
	case string(ScheduledInputActionExecutePlan):
		return ScheduledInputActionExecutePlan
	default:
		return ""
	}
}

func normalizeScheduledInputMode(mode ScheduledInputMode) ScheduledInputMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case string(ScheduledInputModeSend):
		return ScheduledInputModeSend
	case string(ScheduledInputModeInterrupt), string(ScheduledInputModeRedirect):
		return ScheduledInputModeInterrupt
	case string(ScheduledInputModeQueue):
		return ScheduledInputModeQueue
	default:
		return ""
	}
}

func normalizeScheduledInputStatus(status ScheduledInputStatus) ScheduledInputStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(ScheduledInputStatusScheduled):
		return ScheduledInputStatusScheduled
	case string(ScheduledInputStatusDispatched):
		return ScheduledInputStatusDispatched
	case string(ScheduledInputStatusCanceled):
		return ScheduledInputStatusCanceled
	case string(ScheduledInputStatusFailed):
		return ScheduledInputStatusFailed
	case string(ScheduledInputStatusExpired):
		return ScheduledInputStatusExpired
	default:
		return ""
	}
}

func normalizeScheduledInputScheduleKind(kind ScheduledInputScheduleKind) ScheduledInputScheduleKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case "", string(ScheduledInputScheduleAtTime):
		return ScheduledInputScheduleAtTime
	case string(ScheduledInputScheduleWhenIdle):
		return ScheduledInputScheduleWhenIdle
	default:
		return ""
	}
}

func activeScheduledInputStatuses() []string {
	return []string{
		string(ScheduledInputStatusScheduled),
		string(ScheduledInputStatusFailed),
		string(ScheduledInputStatusExpired),
	}
}

func parseScheduledPlanExecutionPayload(raw string) (scheduledPlanExecutionPayload, error) {
	var payload scheduledPlanExecutionPayload
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return scheduledPlanExecutionPayload{}, err
		}
	}
	payload.PendingItemID = strings.TrimSpace(payload.PendingItemID)
	payload.QuestionID = strings.TrimSpace(payload.QuestionID)
	payload.ExecuteOptionLabel = strings.TrimSpace(payload.ExecuteOptionLabel)
	return payload, nil
}

func marshalScheduledPlanExecutionPayload(payload scheduledPlanExecutionPayload) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func parseScheduledInputAttachmentIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var attachmentIDs []string
	if err := json.Unmarshal([]byte(raw), &attachmentIDs); err != nil {
		return nil
	}
	return sanitizePendingAttachmentIDs(attachmentIDs)
}

func marshalScheduledInputAttachmentIDs(attachmentIDs []string) string {
	encoded, err := json.Marshal(sanitizePendingAttachmentIDs(attachmentIDs))
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func parseScheduledInputBlockingReasons(raw string) []ScheduledInputBlockingReason {
	if strings.TrimSpace(raw) == "" {
		return []ScheduledInputBlockingReason{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []ScheduledInputBlockingReason{}
	}
	reasons := make([]ScheduledInputBlockingReason, 0, len(values))
	seen := make(map[ScheduledInputBlockingReason]struct{}, len(values))
	for _, value := range values {
		reason := ScheduledInputBlockingReason(strings.TrimSpace(value))
		switch reason {
		case ScheduledInputBlockedGitDirty,
			ScheduledInputBlockedGitUnavailable,
			ScheduledInputBlockedNonPlanSessionActive:
			if _, ok := seen[reason]; ok {
				continue
			}
			seen[reason] = struct{}{}
			reasons = append(reasons, reason)
		}
	}
	return reasons
}

func marshalScheduledInputBlockingReasons(reasons []ScheduledInputBlockingReason) string {
	values := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		values = append(values, string(reason))
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func mapScheduledInputRecord(record tables.WebSessionScheduledInputTable) ScheduledInput {
	scheduleKind := normalizeScheduledInputScheduleKind(ScheduledInputScheduleKind(record.ScheduleKind))
	var scheduledFor *time.Time
	if scheduleKind == ScheduledInputScheduleAtTime {
		value := record.ScheduledFor
		scheduledFor = &value
	}
	return ScheduledInput{
		ID:              strings.TrimSpace(record.ID),
		Action:          normalizeScheduledInputAction(ScheduledInputAction(record.Action)),
		TargetID:        strings.TrimSpace(record.TargetID),
		Mode:            normalizeScheduledInputMode(ScheduledInputMode(record.Mode)),
		Text:            record.Text,
		AttachmentIDs:   parseScheduledInputAttachmentIDs(record.AttachmentIDsJSON),
		ScheduleKind:    scheduleKind,
		ScheduledFor:    scheduledFor,
		IdleSince:       record.IdleSince,
		BlockingReasons: parseScheduledInputBlockingReasons(record.BlockingReasons),
		ConditionError:  strings.TrimSpace(record.ConditionError),
		Status:          normalizeScheduledInputStatus(ScheduledInputStatus(record.Status)),
		LastError:       strings.TrimSpace(record.LastError),
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
		SentAt:          record.SentAt,
		CanceledAt:      record.CanceledAt,
	}
}

func (m *Manager) scheduledInputsSnapshot(ctx context.Context, sessionID string) ([]ScheduledInput, error) {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return []ScheduledInput{}, nil
	}
	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}
	var records []tables.WebSessionScheduledInputTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND status IN ?", normalizedSessionID, activeScheduledInputStatuses()).
		Order("scheduled_for ASC").
		Order("created_at ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []ScheduledInput{}, nil
	}
	items := make([]ScheduledInput, 0, len(records))
	for _, record := range records {
		items = append(items, mapScheduledInputRecord(record))
	}
	return items, nil
}

func scheduledInputsHavePendingPlanExecution(items []ScheduledInput) bool {
	for _, item := range items {
		if item.Action == ScheduledInputActionExecutePlan && item.Status == ScheduledInputStatusScheduled {
			return true
		}
	}
	return false
}

func (m *Manager) decorateScheduledPlanExecutionState(
	ctx context.Context,
	summaries []SessionSummary,
) error {
	if len(summaries) == 0 {
		return nil
	}
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	sessionIDs := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if sessionID := strings.TrimSpace(summary.ID); sessionID != "" {
			sessionIDs = append(sessionIDs, sessionID)
		}
	}
	if len(sessionIDs) == 0 {
		return nil
	}

	var scheduledSessionIDs []string
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionScheduledInputTable{}).
		Distinct("web_session_id").
		Where(
			"web_session_id IN ? AND action = ? AND status = ?",
			sessionIDs,
			string(ScheduledInputActionExecutePlan),
			string(ScheduledInputStatusScheduled),
		).
		Pluck("web_session_id", &scheduledSessionIDs).Error; err != nil {
		return err
	}

	pendingBySession := make(map[string]struct{}, len(scheduledSessionIDs))
	for _, sessionID := range scheduledSessionIDs {
		pendingBySession[strings.TrimSpace(sessionID)] = struct{}{}
	}
	for index := range summaries {
		_, summaries[index].HasScheduledPlanExecution = pendingBySession[summaries[index].ID]
	}
	return nil
}

func (m *Manager) ScheduleInput(
	ctx context.Context,
	sessionID string,
	text string,
	attachmentIDs []string,
	mode ScheduledInputMode,
	scheduledFor time.Time,
) (ScheduledInput, error) {
	return m.scheduleInput(
		ctx,
		sessionID,
		text,
		attachmentIDs,
		mode,
		ScheduledInputScheduleAtTime,
		scheduledFor,
	)
}

func (m *Manager) ScheduleInputWhenIdle(
	ctx context.Context,
	sessionID string,
	text string,
	attachmentIDs []string,
	mode ScheduledInputMode,
) (ScheduledInput, error) {
	return m.scheduleInput(
		ctx,
		sessionID,
		text,
		attachmentIDs,
		mode,
		ScheduledInputScheduleWhenIdle,
		time.Time{},
	)
}

func (m *Manager) scheduleInput(
	ctx context.Context,
	sessionID string,
	text string,
	attachmentIDs []string,
	mode ScheduledInputMode,
	scheduleKind ScheduledInputScheduleKind,
	scheduledFor time.Time,
) (ScheduledInput, error) {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return ScheduledInput{}, err
	}
	if record.ArchivedAt != nil {
		return ScheduledInput{}, fmt.Errorf("session is archived")
	}
	if err := m.ensureSessionMessagingAvailable(record); err != nil {
		return ScheduledInput{}, err
	}

	normalizedMode := normalizeScheduledInputMode(mode)
	if normalizedMode == "" {
		return ScheduledInput{}, errInvalidScheduledInputMode
	}

	normalizedText := strings.TrimSpace(text)
	sanitizedAttachmentIDs := sanitizePendingAttachmentIDs(attachmentIDs)
	if normalizedText == "" && len(sanitizedAttachmentIDs) == 0 {
		return ScheduledInput{}, errEmptyPendingInput
	}
	for _, attachmentID := range sanitizedAttachmentIDs {
		if _, err := m.loadAttachment(attachmentID); err != nil {
			return ScheduledInput{}, fmt.Errorf("attachment %s not found", attachmentID)
		}
	}

	normalizedScheduleKind := normalizeScheduledInputScheduleKind(scheduleKind)
	if normalizedScheduleKind == "" {
		return ScheduledInput{}, fmt.Errorf("invalid scheduled input kind")
	}
	if normalizedScheduleKind == ScheduledInputScheduleAtTime &&
		(scheduledFor.IsZero() || !scheduledFor.After(time.Now())) {
		return ScheduledInput{}, fmt.Errorf("scheduled time must be in the future")
	}

	db := model.GetDB()
	if db == nil {
		return ScheduledInput{}, model.ErrDBNotInitialized
	}

	scheduledForValue := scheduledFor
	if normalizedScheduleKind == ScheduledInputScheduleWhenIdle {
		scheduledForValue = time.Now()
	}
	item := tables.WebSessionScheduledInputTable{
		WebSessionID:      record.ID,
		Action:            string(ScheduledInputActionMessage),
		PayloadJSON:       "{}",
		Mode:              string(normalizedMode),
		Text:              normalizedText,
		AttachmentIDsJSON: marshalScheduledInputAttachmentIDs(sanitizedAttachmentIDs),
		ScheduleKind:      string(normalizedScheduleKind),
		ScheduledFor:      scheduledForValue,
		BlockingReasons:   "[]",
		Status:            string(ScheduledInputStatusScheduled),
	}
	item.Init()
	if err := db.WithContext(ctx).Create(&item).Error; err != nil {
		return ScheduledInput{}, err
	}

	created := mapScheduledInputRecord(item)
	if normalizedScheduleKind == ScheduledInputScheduleAtTime {
		m.setScheduledInputTimer(created.ID, record.ID, scheduledFor)
	} else {
		m.ensureScheduledIdleMonitor(scheduledIdlePollInterval)
	}
	m.broadcastScheduledInputs(record.ID)
	return created, nil
}

func normalizeScheduledPlanExecutionPayload(payload scheduledPlanExecutionPayload) (scheduledPlanExecutionPayload, error) {
	payload.PendingItemID = strings.TrimSpace(payload.PendingItemID)
	payload.QuestionID = strings.TrimSpace(payload.QuestionID)
	payload.ExecuteOptionLabel = strings.TrimSpace(payload.ExecuteOptionLabel)
	populated := 0
	for _, value := range []string{payload.PendingItemID, payload.QuestionID, payload.ExecuteOptionLabel} {
		if value != "" {
			populated++
		}
	}
	if populated != 0 && populated != 3 {
		return scheduledPlanExecutionPayload{}, fmt.Errorf("incomplete scheduled plan approval target")
	}
	return payload, nil
}

func (m *Manager) validateScheduledPlanHistory(ctx context.Context, sessionID, targetID string) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return fmt.Errorf("plan target is required")
	}

	var latestPlan tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND item_type = ?", sessionID, "plan").
		Order("order_index DESC").
		First(&latestPlan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errScheduledPlanExpired
		}
		return err
	}
	if strings.TrimSpace(latestPlan.ID) != targetID {
		return errScheduledPlanExpired
	}

	var userMessageCount int64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where("web_session_id = ? AND order_index > ? AND item_kind = ?", sessionID, latestPlan.OrderIndex, "user").
		Count(&userMessageCount).Error; err != nil {
		return err
	}
	if userMessageCount > 0 {
		return errScheduledPlanExpired
	}
	return nil
}

func (m *Manager) validateScheduledPlanApproval(
	ctx context.Context,
	session tables.WebSessionTable,
	payload scheduledPlanExecutionPayload,
) error {
	if payload.PendingItemID == "" {
		if m.hasActiveRun(session.ID) {
			return errScheduledPlanExpired
		}
		return nil
	}

	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	var row tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where(
			"web_session_id = ? AND item_type = ? AND (id = ? OR source_item_id = ?)",
			session.ID,
			"user_input_request",
			payload.PendingItemID,
			payload.PendingItemID,
		).
		Order("order_index DESC").
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errScheduledPlanExpired
		}
		return err
	}

	item := mapHistoryItemRowWithSession(row, session.ID)
	if item.Detail == nil {
		return errScheduledPlanExpired
	}
	matchedOption := false
	for _, question := range item.Detail.Questions {
		if strings.TrimSpace(question.ID) != payload.QuestionID {
			continue
		}
		for _, option := range question.Options {
			if strings.TrimSpace(option.Label) == payload.ExecuteOptionLabel {
				matchedOption = true
				break
			}
		}
	}
	if !matchedOption {
		return errScheduledPlanExpired
	}

	var responseCount int64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where(
			"web_session_id = ? AND order_index > ? AND item_type = ?",
			session.ID,
			row.OrderIndex,
			"user_input_response",
		).
		Count(&responseCount).Error; err != nil {
		return err
	}
	if responseCount > 0 || effectiveAssistantState(session) != AssistantStateWaitingInput {
		return errScheduledPlanExpired
	}

	if normalizeAgent(Agent(session.Agent)) == AgentCodex {
		m.mu.RLock()
		run := m.runs[session.ID]
		m.mu.RUnlock()
		if run == nil {
			return errScheduledPlanExpired
		}
		pending, ok := run.pendingUserInputRequest()
		if !ok || strings.TrimSpace(pending.ItemID) != payload.PendingItemID {
			return errScheduledPlanExpired
		}
	}
	return nil
}

func (m *Manager) validateScheduledPlanExecution(
	ctx context.Context,
	sessionID string,
	targetID string,
	payload scheduledPlanExecutionPayload,
) (tables.WebSessionTable, error) {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return tables.WebSessionTable{}, err
	}
	if session.ArchivedAt != nil {
		return tables.WebSessionTable{}, errScheduledPlanExpired
	}
	if err := m.ensureSessionMessagingAvailable(session); err != nil {
		return tables.WebSessionTable{}, err
	}
	if err := m.validateScheduledPlanHistory(ctx, session.ID, targetID); err != nil {
		return tables.WebSessionTable{}, err
	}
	if err := m.validateScheduledPlanApproval(ctx, session, payload); err != nil {
		return tables.WebSessionTable{}, err
	}
	return session, nil
}

func (m *Manager) SchedulePlanExecution(
	ctx context.Context,
	sessionID string,
	targetID string,
	payload scheduledPlanExecutionPayload,
	scheduledFor time.Time,
) (ScheduledInput, error) {
	return m.schedulePlanExecution(
		ctx,
		sessionID,
		targetID,
		payload,
		ScheduledInputScheduleAtTime,
		scheduledFor,
	)
}

func (m *Manager) SchedulePlanExecutionWhenIdle(
	ctx context.Context,
	sessionID string,
	targetID string,
	payload scheduledPlanExecutionPayload,
) (ScheduledInput, error) {
	return m.schedulePlanExecution(
		ctx,
		sessionID,
		targetID,
		payload,
		ScheduledInputScheduleWhenIdle,
		time.Time{},
	)
}

func (m *Manager) schedulePlanExecution(
	ctx context.Context,
	sessionID string,
	targetID string,
	payload scheduledPlanExecutionPayload,
	scheduleKind ScheduledInputScheduleKind,
	scheduledFor time.Time,
) (ScheduledInput, error) {
	payload, err := normalizeScheduledPlanExecutionPayload(payload)
	if err != nil {
		return ScheduledInput{}, err
	}
	normalizedScheduleKind := normalizeScheduledInputScheduleKind(scheduleKind)
	if normalizedScheduleKind == "" {
		return ScheduledInput{}, fmt.Errorf("invalid scheduled plan kind")
	}
	if normalizedScheduleKind == ScheduledInputScheduleAtTime &&
		(scheduledFor.IsZero() || !scheduledFor.After(time.Now())) {
		return ScheduledInput{}, fmt.Errorf("scheduled time must be in the future")
	}
	session, err := m.validateScheduledPlanExecution(ctx, sessionID, targetID, payload)
	if err != nil {
		return ScheduledInput{}, err
	}

	db := model.GetDB()
	if db == nil {
		return ScheduledInput{}, model.ErrDBNotInitialized
	}
	var duplicateCount int64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionScheduledInputTable{}).
		Where(
			"web_session_id = ? AND action = ? AND target_id = ? AND status = ?",
			session.ID,
			string(ScheduledInputActionExecutePlan),
			strings.TrimSpace(targetID),
			string(ScheduledInputStatusScheduled),
		).
		Count(&duplicateCount).Error; err != nil {
		return ScheduledInput{}, err
	}
	if duplicateCount > 0 {
		return ScheduledInput{}, errScheduledPlanDuplicate
	}

	scheduledForValue := scheduledFor
	if normalizedScheduleKind == ScheduledInputScheduleWhenIdle {
		scheduledForValue = time.Now()
	}
	item := tables.WebSessionScheduledInputTable{
		WebSessionID:      session.ID,
		Action:            string(ScheduledInputActionExecutePlan),
		TargetID:          strings.TrimSpace(targetID),
		PayloadJSON:       marshalScheduledPlanExecutionPayload(payload),
		Mode:              string(ScheduledInputModeSend),
		Text:              "Implement the plan.",
		AttachmentIDsJSON: "[]",
		ScheduleKind:      string(normalizedScheduleKind),
		ScheduledFor:      scheduledForValue,
		BlockingReasons:   "[]",
		Status:            string(ScheduledInputStatusScheduled),
	}
	item.Init()
	if err := db.WithContext(ctx).Create(&item).Error; err != nil {
		return ScheduledInput{}, err
	}

	created := mapScheduledInputRecord(item)
	if normalizedScheduleKind == ScheduledInputScheduleAtTime {
		m.setScheduledInputTimer(created.ID, session.ID, scheduledFor)
	} else {
		m.ensureScheduledIdleMonitor(scheduledIdlePollInterval)
	}
	m.broadcastScheduledInputs(session.ID)
	return created, nil
}

func (m *Manager) UpdateScheduledInput(
	ctx context.Context,
	sessionID string,
	inputID string,
	update scheduledInputUpdate,
) (ScheduledInput, error) {
	normalizedSessionID := strings.TrimSpace(sessionID)
	normalizedInputID := strings.TrimSpace(inputID)
	if normalizedSessionID == "" || normalizedInputID == "" {
		return ScheduledInput{}, errScheduledInputNotFound
	}

	var updated ScheduledInput
	shouldBroadcast := false
	err := m.withScheduledInputLock(normalizedInputID, func() error {
		record, err := loadScheduledInputRecord(ctx, normalizedSessionID, normalizedInputID)
		if err != nil {
			return err
		}
		status := normalizeScheduledInputStatus(ScheduledInputStatus(record.Status))
		if status != ScheduledInputStatusScheduled && status != ScheduledInputStatusFailed {
			return errScheduledInputNotEditable
		}

		session, err := m.GetSession(ctx, normalizedSessionID)
		if err != nil {
			return err
		}
		if session.ArchivedAt != nil {
			return fmt.Errorf("session is archived")
		}
		if err := m.ensureSessionMessagingAvailable(session); err != nil {
			return err
		}

		action := normalizeScheduledInputAction(ScheduledInputAction(record.Action))
		scheduleKind := normalizeScheduledInputScheduleKind(ScheduledInputScheduleKind(record.ScheduleKind))
		if update.ScheduleKind != nil {
			scheduleKind = normalizeScheduledInputScheduleKind(*update.ScheduleKind)
		} else if !update.ScheduledFor.IsZero() {
			// Legacy update payloads always carry a timestamp and therefore select at-time mode.
			scheduleKind = ScheduledInputScheduleAtTime
		}
		if scheduleKind == "" {
			return fmt.Errorf("invalid scheduled input kind")
		}
		if scheduleKind == ScheduledInputScheduleAtTime &&
			(update.ScheduledFor.IsZero() || !update.ScheduledFor.After(time.Now())) {
			return fmt.Errorf("scheduled time must be in the future")
		}

		scheduledForValue := time.Now()
		if scheduleKind == ScheduledInputScheduleAtTime {
			scheduledForValue = update.ScheduledFor
		}
		updates := map[string]any{
			"schedule_kind":         string(scheduleKind),
			"scheduled_for":         scheduledForValue,
			"idle_since":            nil,
			"blocking_reasons_json": "[]",
			"condition_error":       "",
			"status":                string(ScheduledInputStatusScheduled),
			"last_error":            "",
			"sent_at":               nil,
			"canceled_at":           nil,
			"updated_at":            time.Now(),
		}
		switch action {
		case ScheduledInputActionMessage:
			if update.Text == nil || update.Mode == nil {
				return fmt.Errorf("message text and mode are required")
			}
			normalizedText := strings.TrimSpace(*update.Text)
			attachmentIDs := parseScheduledInputAttachmentIDs(record.AttachmentIDsJSON)
			if normalizedText == "" && len(attachmentIDs) == 0 {
				return errEmptyPendingInput
			}
			normalizedMode := normalizeScheduledInputMode(*update.Mode)
			if normalizedMode == "" {
				return errInvalidScheduledInputMode
			}
			updates["text"] = normalizedText
			updates["mode"] = string(normalizedMode)
		case ScheduledInputActionExecutePlan:
			if update.Text != nil || update.Mode != nil {
				return fmt.Errorf("scheduled plan only supports changing its schedule")
			}
			parsedPayload, err := parseScheduledPlanExecutionPayload(record.PayloadJSON)
			if err != nil {
				err = errScheduledPlanExpired
			} else {
				parsedPayload, err = normalizeScheduledPlanExecutionPayload(parsedPayload)
				if err == nil {
					_, err = m.validateScheduledPlanExecution(ctx, normalizedSessionID, record.TargetID, parsedPayload)
				}
			}
			if err != nil {
				if shouldExpireScheduledPlanExecution(err) {
					_ = m.expireScheduledInputByID(ctx, record.ID, err.Error())
					m.cancelScheduledInputTimer(record.ID)
					shouldBroadcast = true
				}
				return err
			}
			db := model.GetDB()
			if db == nil {
				return model.ErrDBNotInitialized
			}
			var duplicateCount int64
			if err := db.WithContext(ctx).
				Model(&tables.WebSessionScheduledInputTable{}).
				Where(
					"web_session_id = ? AND action = ? AND target_id = ? AND status = ? AND id <> ?",
					normalizedSessionID,
					string(ScheduledInputActionExecutePlan),
					record.TargetID,
					string(ScheduledInputStatusScheduled),
					record.ID,
				).
				Count(&duplicateCount).Error; err != nil {
				return err
			}
			if duplicateCount > 0 {
				return errScheduledPlanDuplicate
			}
		default:
			return errInvalidScheduledInputAction
		}

		db := model.GetDB()
		if db == nil {
			return model.ErrDBNotInitialized
		}
		result := db.WithContext(ctx).
			Model(&tables.WebSessionScheduledInputTable{}).
			Where(
				"id = ? AND web_session_id = ? AND status IN ?",
				normalizedInputID,
				normalizedSessionID,
				[]string{string(ScheduledInputStatusScheduled), string(ScheduledInputStatusFailed)},
			).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errScheduledInputNotEditable
		}
		var refreshed tables.WebSessionScheduledInputTable
		if err := db.WithContext(ctx).First(&refreshed, "id = ?", normalizedInputID).Error; err != nil {
			return err
		}
		record = refreshed
		updated = mapScheduledInputRecord(record)
		m.cancelScheduledInputTimer(updated.ID)
		if updated.ScheduleKind == ScheduledInputScheduleAtTime && updated.ScheduledFor != nil {
			m.setScheduledInputTimer(updated.ID, normalizedSessionID, *updated.ScheduledFor)
		} else if updated.ScheduleKind == ScheduledInputScheduleWhenIdle {
			m.ensureScheduledIdleMonitor(scheduledIdlePollInterval)
		}
		shouldBroadcast = true
		return nil
	})
	if shouldBroadcast {
		m.broadcastScheduledInputs(normalizedSessionID)
	}
	return updated, err
}

func (m *Manager) DispatchScheduledInputNow(ctx context.Context, sessionID, inputID string) error {
	normalizedSessionID := strings.TrimSpace(sessionID)
	normalizedInputID := strings.TrimSpace(inputID)
	if normalizedSessionID == "" || normalizedInputID == "" {
		return errScheduledInputNotFound
	}
	shouldBroadcast := false
	err := m.withScheduledInputLock(normalizedInputID, func() error {
		record, err := loadScheduledInputRecord(ctx, normalizedSessionID, normalizedInputID)
		if err != nil {
			return err
		}
		status := normalizeScheduledInputStatus(ScheduledInputStatus(record.Status))
		if status != ScheduledInputStatusScheduled && status != ScheduledInputStatusFailed {
			return errScheduledInputNotExecutable
		}
		m.cancelScheduledInputTimer(normalizedInputID)
		err = m.dispatchScheduledInputRecord(ctx, record)
		shouldBroadcast = true
		return err
	})
	if shouldBroadcast {
		m.broadcastScheduledInputs(normalizedSessionID)
	}
	return err
}

func (m *Manager) RemoveScheduledInput(ctx context.Context, sessionID, inputID string) error {
	normalizedSessionID := strings.TrimSpace(sessionID)
	normalizedInputID := strings.TrimSpace(inputID)
	if normalizedSessionID == "" || normalizedInputID == "" {
		return errScheduledInputNotFound
	}

	err := m.withScheduledInputLock(normalizedInputID, func() error {
		db := model.GetDB()
		if db == nil {
			return model.ErrDBNotInitialized
		}

		now := time.Now()
		result := db.WithContext(ctx).
			Model(&tables.WebSessionScheduledInputTable{}).
			Where("id = ? AND web_session_id = ? AND status IN ?", normalizedInputID, normalizedSessionID, activeScheduledInputStatuses()).
			Updates(map[string]any{
				"status":      string(ScheduledInputStatusCanceled),
				"canceled_at": now,
				"updated_at":  now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errScheduledInputNotFound
		}

		m.cancelScheduledInputTimer(normalizedInputID)
		return nil
	})
	if err == nil {
		m.broadcastScheduledInputs(normalizedSessionID)
	}
	return err
}

func (m *Manager) cancelActiveScheduledInputs(ctx context.Context, sessionID string) error {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return nil
	}
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	now := time.Now()
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionScheduledInputTable{}).
		Where("web_session_id = ? AND status IN ?", normalizedSessionID, activeScheduledInputStatuses()).
		Updates(map[string]any{
			"status":      string(ScheduledInputStatusCanceled),
			"canceled_at": now,
			"updated_at":  now,
		}).Error; err != nil {
		return err
	}
	m.cancelScheduledInputTimersForSession(normalizedSessionID)
	return nil
}

func (m *Manager) deleteScheduledInputsForSession(ctx context.Context, sessionID string) error {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return nil
	}
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	if err := db.WithContext(ctx).
		Where("web_session_id = ?", normalizedSessionID).
		Delete(&tables.WebSessionScheduledInputTable{}).Error; err != nil {
		return err
	}
	m.cancelScheduledInputTimersForSession(normalizedSessionID)
	return nil
}

func (m *Manager) cancelScheduledInputTimer(inputID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	timer := m.scheduledInputTimers[inputID]
	if timer != nil {
		timer.Stop()
		delete(m.scheduledInputTimers, inputID)
	}
	delete(m.scheduledInputTimerSessions, inputID)
}

func (m *Manager) cancelScheduledInputTimersForSession(sessionID string) {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for inputID, timerSessionID := range m.scheduledInputTimerSessions {
		if timerSessionID != normalizedSessionID {
			continue
		}
		if timer := m.scheduledInputTimers[inputID]; timer != nil {
			timer.Stop()
		}
		delete(m.scheduledInputTimers, inputID)
		delete(m.scheduledInputTimerSessions, inputID)
	}
}

func (m *Manager) setScheduledInputTimer(inputID, sessionID string, scheduledFor time.Time) {
	m.cancelScheduledInputTimer(inputID)
	delay := time.Until(scheduledFor)
	if delay < 0 {
		delay = 0
	}
	timer := time.AfterFunc(delay, func() {
		m.cancelScheduledInputTimer(inputID)
		m.executeScheduledInput(inputID, scheduledFor)
	})
	m.mu.Lock()
	m.scheduledInputTimers[inputID] = timer
	m.scheduledInputTimerSessions[inputID] = strings.TrimSpace(sessionID)
	m.mu.Unlock()
}

func (m *Manager) broadcastScheduledInputs(sessionID string) {
	lock := &m.scheduledBroadcastLocks[sessionRevisionLockIndex(sessionID)]
	lock.Lock()
	defer lock.Unlock()

	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil || record.ArchivedAt != nil {
		return
	}
	items, err := m.scheduledInputsSnapshot(context.Background(), sessionID)
	if err != nil {
		return
	}
	_ = m.broadcastNextRevision(context.Background(), sessionID, func() (wireFrame, bool) {
		return newScheduledFrame(sessionID, items), true
	})
}

func (m *Manager) recoverPendingScheduledInputs(ctx context.Context) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}

	var records []tables.WebSessionScheduledInputTable
	if err := db.WithContext(ctx).
		Table("web_session_scheduled_inputs").
		Select("web_session_scheduled_inputs.*").
		Joins("JOIN web_sessions ON web_sessions.id = web_session_scheduled_inputs.web_session_id").
		Where("web_session_scheduled_inputs.status = ? AND web_sessions.archived_at IS NULL", string(ScheduledInputStatusScheduled)).
		Order("web_session_scheduled_inputs.scheduled_for ASC").
		Find(&records).Error; err != nil {
		return err
	}
	hasIdleInputs := false
	for _, record := range records {
		switch normalizeScheduledInputScheduleKind(ScheduledInputScheduleKind(record.ScheduleKind)) {
		case ScheduledInputScheduleWhenIdle:
			hasIdleInputs = true
			if err := db.WithContext(ctx).
				Model(&tables.WebSessionScheduledInputTable{}).
				Where("id = ?", record.ID).
				Updates(map[string]any{
					"idle_since":            nil,
					"blocking_reasons_json": "[]",
					"condition_error":       "",
				}).Error; err != nil {
				return err
			}
		case ScheduledInputScheduleAtTime:
			m.setScheduledInputTimer(record.ID, record.WebSessionID, record.ScheduledFor)
		}
	}
	if hasIdleInputs {
		m.ensureScheduledIdleMonitor(scheduledIdlePollInterval)
	}
	return nil
}

func (m *Manager) ensureScheduledIdleMonitor(delay time.Duration) {
	if delay < 0 {
		delay = 0
	}
	m.mu.Lock()
	if m.scheduledIdleTimer != nil {
		m.mu.Unlock()
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		m.mu.Lock()
		if m.scheduledIdleTimer == timer {
			m.scheduledIdleTimer = nil
		}
		m.mu.Unlock()
		m.runScheduledIdleSweep()
	})
	m.scheduledIdleTimer = timer
	m.mu.Unlock()
}

func (m *Manager) cancelScheduledIdleMonitor() {
	m.mu.Lock()
	if m.scheduledIdleTimer != nil {
		m.scheduledIdleTimer.Stop()
		m.scheduledIdleTimer = nil
	}
	m.mu.Unlock()
}

func (m *Manager) runScheduledIdleSweep() {
	m.scheduledIdleSweepMu.Lock()
	defer m.scheduledIdleSweepMu.Unlock()
	hasPending, err := m.processScheduledIdleInputs(context.Background())
	if err != nil && m.logger != nil {
		m.logger.Warn("scheduled idle sweep failed", zap.Error(err))
	}
	if hasPending || err != nil {
		m.ensureScheduledIdleMonitor(scheduledIdlePollInterval)
	}
}

func (m *Manager) processScheduledIdleInputs(ctx context.Context) (bool, error) {
	db := model.GetDB()
	if db == nil {
		return false, model.ErrDBNotInitialized
	}
	var records []tables.WebSessionScheduledInputTable
	if err := db.WithContext(ctx).
		Where(
			"status = ? AND schedule_kind = ?",
			string(ScheduledInputStatusScheduled),
			string(ScheduledInputScheduleWhenIdle),
		).
		Order("created_at ASC").
		Find(&records).Error; err != nil {
		return false, err
	}
	cache := newScheduledIdleSweepCache()
	for _, record := range records {
		if err := m.processScheduledIdleInput(ctx, record.ID, cache); err != nil && m.logger != nil {
			m.logger.Warn(
				"scheduled idle input check failed",
				zap.String("scheduledInputId", record.ID),
				zap.Error(err),
			)
		}
	}
	return len(records) > 0, nil
}

func (m *Manager) processScheduledIdleInput(
	ctx context.Context,
	inputID string,
	cache *scheduledIdleSweepCache,
) error {
	return m.processScheduledIdleInputWithEvaluator(
		ctx,
		inputID,
		cache,
		m.evaluateScheduledIdleConditionsCached,
	)
}

func (m *Manager) processScheduledIdleInputWithEvaluator(
	ctx context.Context,
	inputID string,
	cache *scheduledIdleSweepCache,
	evaluate scheduledIdleConditionEvaluator,
) error {
	if evaluate == nil {
		evaluate = m.evaluateScheduledIdleConditionsCached
	}
	record, active, err := loadActiveScheduledIdleInput(ctx, inputID)
	if err != nil || !active {
		return err
	}
	now := time.Now()
	if record.IdleSince == nil && !record.UpdatedAt.IsZero() &&
		now.Sub(record.UpdatedAt) < scheduledIdlePollInterval {
		return nil
	}
	applyValidationError := func(
		expected tables.WebSessionScheduledInputTable,
		validationErr error,
	) error {
		changed, applyErr := m.mutateCurrentScheduledIdleInput(ctx, expected, func(current tables.WebSessionScheduledInputTable) (bool, error) {
			return m.handleScheduledIdleValidationError(ctx, current, validationErr)
		})
		if changed {
			m.broadcastScheduledInputs(expected.WebSessionID)
		}
		return applyErr
	}

	session, validationErr := m.validateScheduledIdleInput(ctx, record)
	if validationErr != nil {
		return applyValidationError(record, validationErr)
	}

	readyToDispatch := record.IdleSince != nil && now.Sub(*record.IdleSince) >= scheduledIdleStablePeriod

	evaluationCache := cache
	if readyToDispatch {
		evaluationCache = nil
	}
	reasons, conditionError, evaluateErr := evaluate(ctx, session, evaluationCache)
	if evaluateErr != nil {
		conditionError = evaluateErr.Error()
	}
	if len(reasons) > 0 || conditionError != "" {
		changed, applyErr := m.mutateCurrentScheduledIdleInput(ctx, record, func(current tables.WebSessionScheduledInputTable) (bool, error) {
			return m.updateScheduledIdleCondition(ctx, current, nil, reasons, conditionError)
		})
		if changed {
			m.broadcastScheduledInputs(record.WebSessionID)
		}
		return applyErr
	}

	if !readyToDispatch {
		if record.IdleSince != nil {
			return nil
		}
		idleSince := time.Now()
		changed, applyErr := m.mutateCurrentScheduledIdleInput(ctx, record, func(current tables.WebSessionScheduledInputTable) (bool, error) {
			return m.updateScheduledIdleCondition(ctx, current, &idleSince, nil, "")
		})
		if changed {
			m.broadcastScheduledInputs(record.WebSessionID)
		}
		return applyErr
	}

	changed, dispatchErr := m.mutateCurrentScheduledIdleInput(ctx, record, func(current tables.WebSessionScheduledInputTable) (bool, error) {
		projectLock := m.scheduledProjectLock(session.ProjectID)
		projectLock.Lock()
		defer projectLock.Unlock()
		if current.IdleSince == nil || time.Since(*current.IdleSince) < scheduledIdleStablePeriod {
			return false, nil
		}
		return true, m.dispatchScheduledInputRecord(ctx, current)
	})
	if changed {
		m.broadcastScheduledInputs(record.WebSessionID)
	}
	return dispatchErr
}

func loadActiveScheduledIdleInput(
	ctx context.Context,
	inputID string,
) (tables.WebSessionScheduledInputTable, bool, error) {
	db := model.GetDB()
	if db == nil {
		return tables.WebSessionScheduledInputTable{}, false, model.ErrDBNotInitialized
	}
	var record tables.WebSessionScheduledInputTable
	if err := db.WithContext(ctx).First(&record, "id = ?", strings.TrimSpace(inputID)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tables.WebSessionScheduledInputTable{}, false, nil
		}
		return tables.WebSessionScheduledInputTable{}, false, err
	}
	active := normalizeScheduledInputStatus(ScheduledInputStatus(record.Status)) == ScheduledInputStatusScheduled &&
		normalizeScheduledInputScheduleKind(ScheduledInputScheduleKind(record.ScheduleKind)) == ScheduledInputScheduleWhenIdle
	return record, active, nil
}

func (m *Manager) mutateCurrentScheduledIdleInput(
	ctx context.Context,
	expected tables.WebSessionScheduledInputTable,
	mutate func(tables.WebSessionScheduledInputTable) (bool, error),
) (bool, error) {
	changed := false
	err := m.withScheduledInputLock(expected.ID, func() error {
		current, active, err := loadActiveScheduledIdleInput(ctx, expected.ID)
		if err != nil || !active {
			return err
		}
		if !scheduledIdleRecordVersionMatches(expected, current) {
			return nil
		}
		changed, err = mutate(current)
		return err
	})
	return changed, err
}

func scheduledIdleRecordVersionMatches(
	expected tables.WebSessionScheduledInputTable,
	current tables.WebSessionScheduledInputTable,
) bool {
	if expected.ID != current.ID ||
		expected.WebSessionID != current.WebSessionID ||
		expected.Status != current.Status ||
		expected.ScheduleKind != current.ScheduleKind ||
		!expected.ScheduledFor.Equal(current.ScheduledFor) ||
		expected.BlockingReasons != current.BlockingReasons ||
		expected.ConditionError != current.ConditionError ||
		!expected.UpdatedAt.Equal(current.UpdatedAt) {
		return false
	}
	return scheduledIdleOptionalTimeEqual(expected.IdleSince, current.IdleSince)
}

func scheduledIdleOptionalTimeEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func (m *Manager) validateScheduledIdleInput(
	ctx context.Context,
	record tables.WebSessionScheduledInputTable,
) (tables.WebSessionTable, error) {
	switch normalizeScheduledInputAction(ScheduledInputAction(record.Action)) {
	case ScheduledInputActionExecutePlan:
		payload, err := parseScheduledPlanExecutionPayload(record.PayloadJSON)
		if err == nil {
			payload, err = normalizeScheduledPlanExecutionPayload(payload)
		}
		if err != nil {
			return tables.WebSessionTable{}, errScheduledPlanExpired
		}
		return m.validateScheduledPlanExecution(ctx, record.WebSessionID, record.TargetID, payload)
	case ScheduledInputActionMessage:
		session, err := m.GetSession(ctx, record.WebSessionID)
		if err != nil {
			return tables.WebSessionTable{}, err
		}
		if session.ArchivedAt != nil {
			return tables.WebSessionTable{}, fmt.Errorf("session is archived")
		}
		return session, nil
	default:
		return tables.WebSessionTable{}, errInvalidScheduledInputAction
	}
}

func (m *Manager) handleScheduledIdleValidationError(
	ctx context.Context,
	record tables.WebSessionScheduledInputTable,
	err error,
) (bool, error) {
	action := normalizeScheduledInputAction(ScheduledInputAction(record.Action))
	switch {
	case action == ScheduledInputActionExecutePlan && shouldExpireScheduledPlanExecution(err):
		_ = m.expireScheduledInputByID(ctx, record.ID, err.Error())
		return true, err
	case action == ScheduledInputActionMessage && shouldCancelScheduledInputDispatchError(err):
		_ = m.cancelScheduledInputByID(ctx, record.ID)
		return true, err
	case action == "":
		_ = m.failScheduledInputByID(ctx, record.ID, err.Error())
		return true, err
	}

	return m.updateScheduledIdleCondition(ctx, record, nil, nil, err.Error())
}

func (m *Manager) evaluateScheduledIdleConditions(
	ctx context.Context,
	target tables.WebSessionTable,
) ([]ScheduledInputBlockingReason, string, error) {
	return m.evaluateScheduledIdleConditionsCached(ctx, target, nil)
}

func (m *Manager) evaluateScheduledIdleConditionsCached(
	ctx context.Context,
	target tables.WebSessionTable,
	cache *scheduledIdleSweepCache,
) ([]ScheduledInputBlockingReason, string, error) {
	reasons := make([]ScheduledInputBlockingReason, 0, 2)
	worktreePath := strings.TrimSpace(target.Cwd)
	gitCheck, ok := scheduledIdleGitCheck{}, false
	gitCacheKey := model.NormalizePathCase(worktreePath)
	if cache != nil {
		gitCheck, ok = cache.gitByPath[gitCacheKey]
	}
	if !ok {
		gitContext := ctx
		cancel := func() {}
		if scheduledIdleConditionTimeout > 0 {
			gitContext, cancel = context.WithTimeout(ctx, scheduledIdleConditionTimeout)
		}
		gitCheck.dirty, gitCheck.err = gitutil.HasTrackedWorktreeChangesContext(gitContext, worktreePath)
		cancel()
		if cache != nil {
			cache.gitByPath[gitCacheKey] = gitCheck
		}
	}
	if gitCheck.err != nil {
		reasons = append(reasons, ScheduledInputBlockedGitUnavailable)
	} else if gitCheck.dirty {
		reasons = append(reasons, ScheduledInputBlockedGitDirty)
	}

	projectID := strings.TrimSpace(target.ProjectID)
	sessionCheck, ok := scheduledIdleSessionCheck{}, false
	if cache != nil {
		sessionCheck, ok = cache.sessionsByProject[projectID]
	}
	if !ok {
		sessionCheck.blocked, sessionCheck.err = m.hasBlockingNonPlanSession(ctx, projectID)
		if cache != nil {
			cache.sessionsByProject[projectID] = sessionCheck
		}
	}
	if sessionCheck.err != nil {
		return reasons, "", sessionCheck.err
	}
	if sessionCheck.blocked {
		reasons = append(reasons, ScheduledInputBlockedNonPlanSessionActive)
	}

	conditionError := ""
	if gitCheck.err != nil {
		conditionError = gitCheck.err.Error()
	}
	return reasons, conditionError, nil
}

func (m *Manager) hasBlockingNonPlanSession(ctx context.Context, projectID string) (bool, error) {
	db := model.GetDB()
	if db == nil {
		return false, model.ErrDBNotInitialized
	}
	var sessions []tables.WebSessionTable
	if err := db.WithContext(ctx).
		Where("project_id = ? AND archived_at IS NULL", projectID).
		Find(&sessions).Error; err != nil {
		return false, err
	}
	for _, session := range sessions {
		if effectiveWorkflowMode(session) == WorkflowModePlan {
			continue
		}
		assistantState := effectiveAssistantState(session)
		status := effectiveStatus(session, assistantState)
		if m.hasActiveRun(session.ID) ||
			assistantState != AssistantStateNone ||
			status == StatusRunning ||
			status == StatusWaitingApproval ||
			status == StatusAborting ||
			session.AutoRetryNextAt != nil {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) updateScheduledIdleCondition(
	ctx context.Context,
	record tables.WebSessionScheduledInputTable,
	idleSince *time.Time,
	reasons []ScheduledInputBlockingReason,
	conditionError string,
) (bool, error) {
	encodedReasons := marshalScheduledInputBlockingReasons(reasons)
	normalizedError := strings.TrimSpace(conditionError)
	sameIdleSince := record.IdleSince == nil && idleSince == nil
	if record.IdleSince != nil && idleSince != nil {
		sameIdleSince = record.IdleSince.UnixMilli() == idleSince.UnixMilli()
	}
	if sameIdleSince && record.BlockingReasons == encodedReasons &&
		strings.TrimSpace(record.ConditionError) == normalizedError {
		return false, nil
	}
	db := model.GetDB()
	if db == nil {
		return false, model.ErrDBNotInitialized
	}
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionScheduledInputTable{}).
		Where("id = ? AND status = ?", record.ID, string(ScheduledInputStatusScheduled)).
		Updates(map[string]any{
			"idle_since":            idleSince,
			"blocking_reasons_json": encodedReasons,
			"condition_error":       normalizedError,
			"updated_at":            time.Now(),
		}).Error; err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) scheduledProjectLock(projectID string) *sync.Mutex {
	normalizedProjectID := strings.TrimSpace(projectID)
	hash := uint32(2166136261)
	for index := 0; index < len(normalizedProjectID); index++ {
		hash ^= uint32(normalizedProjectID[index])
		hash *= 16777619
	}
	return &m.scheduledProjectLocks[hash%uint32(len(m.scheduledProjectLocks))]
}

func (m *Manager) executeScheduledInput(inputID string, expectedScheduledFor time.Time) {
	sessionID := ""
	shouldBroadcast := false
	_ = m.withScheduledInputLock(inputID, func() error {
		ctx := context.Background()
		db := model.GetDB()
		if db == nil {
			return model.ErrDBNotInitialized
		}
		var record tables.WebSessionScheduledInputTable
		if err := db.WithContext(ctx).First(&record, "id = ?", strings.TrimSpace(inputID)).Error; err != nil {
			return err
		}
		sessionID = record.WebSessionID
		if normalizeScheduledInputStatus(ScheduledInputStatus(record.Status)) != ScheduledInputStatusScheduled {
			return nil
		}
		if normalizeScheduledInputScheduleKind(ScheduledInputScheduleKind(record.ScheduleKind)) != ScheduledInputScheduleAtTime ||
			record.ScheduledFor.UnixMilli() != expectedScheduledFor.UnixMilli() {
			return nil
		}
		shouldBroadcast = true
		return m.dispatchScheduledInputRecord(ctx, record)
	})
	if shouldBroadcast {
		m.broadcastScheduledInputs(sessionID)
	}
}

func (m *Manager) dispatchScheduledInputRecord(
	ctx context.Context,
	record tables.WebSessionScheduledInputTable,
) error {
	session, err := m.GetSession(ctx, record.WebSessionID)
	if err != nil {
		_ = m.deleteScheduledInputByID(ctx, record.ID)
		return err
	}
	action := normalizeScheduledInputAction(ScheduledInputAction(record.Action))
	if session.ArchivedAt != nil {
		err = fmt.Errorf("session is archived")
		if action == ScheduledInputActionExecutePlan {
			_ = m.expireScheduledInputByID(ctx, record.ID, err.Error())
		} else {
			_ = m.cancelScheduledInputByID(ctx, record.ID)
		}
		return err
	}
	if err := m.ensureSessionMessagingAvailable(session); err != nil {
		_ = m.failScheduledInputByID(ctx, record.ID, err.Error())
		return err
	}

	switch action {
	case ScheduledInputActionExecutePlan:
		err = m.dispatchScheduledPlanExecution(ctx, record)
	case ScheduledInputActionMessage:
		attachmentIDs := parseScheduledInputAttachmentIDs(record.AttachmentIDsJSON)
		mode := normalizeScheduledInputMode(ScheduledInputMode(record.Mode))
		err = m.dispatchScheduledInput(ctx, record.WebSessionID, mode, record.Text, attachmentIDs)
	default:
		err = errInvalidScheduledInputAction
	}
	if err != nil {
		if action == ScheduledInputActionExecutePlan && shouldExpireScheduledPlanExecution(err) {
			_ = m.expireScheduledInputByID(ctx, record.ID, err.Error())
		} else if shouldCancelScheduledInputDispatchError(err) {
			_ = m.cancelScheduledInputByID(ctx, record.ID)
		} else {
			_ = m.failScheduledInputByID(ctx, record.ID, err.Error())
		}
		return err
	}

	if err := m.markScheduledInputDispatched(ctx, record.ID); err != nil {
		return err
	}
	return nil
}

func shouldExpireScheduledPlanExecution(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errScheduledPlanExpired) || errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, fragment := range []string{
		"session is archived",
		"session is not running",
		"session is already running",
		"no pending user input request",
		"does not match the active prompt",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func (m *Manager) dispatchScheduledPlanExecution(
	ctx context.Context,
	record tables.WebSessionScheduledInputTable,
) error {
	parsedPayload, err := parseScheduledPlanExecutionPayload(record.PayloadJSON)
	if err != nil {
		return errScheduledPlanExpired
	}
	payload, err := normalizeScheduledPlanExecutionPayload(parsedPayload)
	if err != nil {
		return errScheduledPlanExpired
	}
	session, err := m.validateScheduledPlanExecution(ctx, record.WebSessionID, record.TargetID, payload)
	if err != nil {
		return err
	}
	if effectiveWorkflowMode(session) == WorkflowModePlan {
		if _, err := m.UpdateWorkflowMode(ctx, session.ID, WorkflowModeDefault); err != nil {
			return err
		}
		m.broadcastSessionSummary(ctx, session.ID)
	}
	if payload.PendingItemID != "" {
		return m.respondToUserInput(session.ID, payload.PendingItemID, map[string][]string{
			payload.QuestionID: {payload.ExecuteOptionLabel},
		})
	}
	return m.sendMessageInternal(ctx, session.ID, "Implement the plan.", nil, false)
}

func shouldCancelScheduledInputDispatchError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "session is archived")
}

func (m *Manager) dispatchScheduledInput(
	ctx context.Context,
	sessionID string,
	mode ScheduledInputMode,
	text string,
	attachmentIDs []string,
) error {
	switch normalizeScheduledInputMode(mode) {
	case ScheduledInputModeSend:
		if m.hasActiveRun(sessionID) {
			return m.sendMessageWithMode(ctx, sessionID, text, attachmentIDs, PendingInputModeRedirect, "")
		}
		err := m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, false)
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "already running") {
			return m.sendMessageWithMode(ctx, sessionID, text, attachmentIDs, PendingInputModeRedirect, "")
		}
		return err
	case ScheduledInputModeInterrupt:
		return m.sendMessageAfterInterrupt(ctx, sessionID, text, attachmentIDs)
	case ScheduledInputModeQueue:
		return m.sendMessageWithMode(ctx, sessionID, text, attachmentIDs, PendingInputModeQueue, "")
	default:
		return errInvalidScheduledInputMode
	}
}

func (m *Manager) sendMessageAfterInterrupt(
	ctx context.Context,
	sessionID string,
	text string,
	attachmentIDs []string,
) error {
	if err := m.stopRunIfActive(sessionID, 5*time.Second); err != nil {
		return err
	}
	err := m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, false)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "already running") {
		return err
	}
	if err := m.stopRunIfActive(sessionID, 5*time.Second); err != nil {
		return err
	}
	return m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, false)
}

func (m *Manager) markScheduledInputDispatched(ctx context.Context, inputID string) error {
	now := time.Now()
	return m.updateScheduledInputStatus(ctx, inputID, map[string]any{
		"status":     string(ScheduledInputStatusDispatched),
		"last_error": "",
		"sent_at":    now,
		"updated_at": now,
	})
}

func (m *Manager) failScheduledInputByID(ctx context.Context, inputID, reason string) error {
	return m.updateScheduledInputStatus(ctx, inputID, map[string]any{
		"status":     string(ScheduledInputStatusFailed),
		"last_error": strings.TrimSpace(reason),
		"updated_at": time.Now(),
	})
}

func (m *Manager) expireScheduledInputByID(ctx context.Context, inputID, reason string) error {
	return m.updateScheduledInputStatus(ctx, inputID, map[string]any{
		"status":     string(ScheduledInputStatusExpired),
		"last_error": strings.TrimSpace(reason),
		"updated_at": time.Now(),
	})
}

func (m *Manager) cancelScheduledInputByID(ctx context.Context, inputID string) error {
	now := time.Now()
	return m.updateScheduledInputStatus(ctx, inputID, map[string]any{
		"status":      string(ScheduledInputStatusCanceled),
		"canceled_at": now,
		"updated_at":  now,
	})
}

func (m *Manager) updateScheduledInputStatus(ctx context.Context, inputID string, updates map[string]any) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionScheduledInputTable{}).
		Where("id = ?", strings.TrimSpace(inputID)).
		Updates(updates).Error; err != nil {
		return err
	}
	return nil
}

func (m *Manager) deleteScheduledInputByID(ctx context.Context, inputID string) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	m.cancelScheduledInputTimer(strings.TrimSpace(inputID))
	return db.WithContext(ctx).
		Delete(&tables.WebSessionScheduledInputTable{}, "id = ?", strings.TrimSpace(inputID)).Error
}

func (m *Manager) handleScheduleSendCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		Text         string   `json:"txt"`
		Attachments  []string `json:"atts"`
		Mode         string   `json:"mode"`
		ScheduleKind string   `json:"sk"`
		At           *int64   `json:"at"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid schedule payload", false))
	}
	scheduleKind := normalizeScheduledInputScheduleKind(ScheduledInputScheduleKind(payload.ScheduleKind))
	var created ScheduledInput
	var err error
	switch scheduleKind {
	case ScheduledInputScheduleWhenIdle:
		created, err = m.ScheduleInputWhenIdle(
			ctx,
			frame.SessionID,
			payload.Text,
			payload.Attachments,
			ScheduledInputMode(payload.Mode),
		)
	case ScheduledInputScheduleAtTime:
		if payload.At == nil {
			return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "scheduled time is required", false))
		}
		created, err = m.ScheduleInput(
			ctx,
			frame.SessionID,
			payload.Text,
			payload.Attachments,
			ScheduledInputMode(payload.Mode),
			time.UnixMilli(*payload.At),
		)
	default:
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid scheduled input kind", false))
	}
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(
		ctx,
		client,
		frame,
		mapWireScheduledInputs([]ScheduledInput{created})[0],
	)
}

func (m *Manager) handleSchedulePlanCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		PlanItemID         string `json:"pid"`
		PendingItemID      string `json:"iid"`
		QuestionID         string `json:"qid"`
		ExecuteOptionLabel string `json:"opt"`
		ScheduleKind       string `json:"sk"`
		At                 *int64 `json:"at"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid scheduled plan payload", false))
	}
	executionPayload := scheduledPlanExecutionPayload{
		PendingItemID:      payload.PendingItemID,
		QuestionID:         payload.QuestionID,
		ExecuteOptionLabel: payload.ExecuteOptionLabel,
	}
	scheduleKind := normalizeScheduledInputScheduleKind(ScheduledInputScheduleKind(payload.ScheduleKind))
	var created ScheduledInput
	var err error
	switch scheduleKind {
	case ScheduledInputScheduleWhenIdle:
		created, err = m.SchedulePlanExecutionWhenIdle(
			ctx,
			frame.SessionID,
			payload.PlanItemID,
			executionPayload,
		)
	case ScheduledInputScheduleAtTime:
		if payload.At == nil {
			return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "scheduled time is required", false))
		}
		created, err = m.SchedulePlanExecution(
			ctx,
			frame.SessionID,
			payload.PlanItemID,
			executionPayload,
			time.UnixMilli(*payload.At),
		)
	default:
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid scheduled plan kind", false))
	}
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(
		ctx,
		client,
		frame,
		mapWireScheduledInputs([]ScheduledInput{created})[0],
	)
}

func (m *Manager) handleScheduledDeleteCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid scheduled delete payload", false))
	}
	if err := m.RemoveScheduledInput(ctx, frame.SessionID, payload.ID); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, map[string]any{
		"id": strings.TrimSpace(payload.ID),
	})
}

func (m *Manager) handleScheduledUpdateCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		ID           string  `json:"id"`
		Text         *string `json:"txt"`
		Mode         *string `json:"mode"`
		ScheduleKind *string `json:"sk"`
		At           *int64  `json:"at"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil || (payload.At == nil && payload.ScheduleKind == nil) {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid scheduled update payload", false))
	}
	var mode *ScheduledInputMode
	if payload.Mode != nil {
		value := ScheduledInputMode(*payload.Mode)
		mode = &value
	}
	var scheduleKind *ScheduledInputScheduleKind
	if payload.ScheduleKind != nil {
		value := ScheduledInputScheduleKind(*payload.ScheduleKind)
		scheduleKind = &value
	}
	scheduledFor := time.Time{}
	if payload.At != nil {
		scheduledFor = time.UnixMilli(*payload.At)
	}
	updated, err := m.UpdateScheduledInput(ctx, frame.SessionID, payload.ID, scheduledInputUpdate{
		Text:         payload.Text,
		Mode:         mode,
		ScheduleKind: scheduleKind,
		ScheduledFor: scheduledFor,
	})
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(
		ctx,
		client,
		frame,
		mapWireScheduledInputs([]ScheduledInput{updated})[0],
	)
}

func (m *Manager) handleScheduledNowCommand(ctx context.Context, client *client, frame wireCommandFrame) error {
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid scheduled execution payload", false))
	}
	if err := m.DispatchScheduledInputNow(ctx, frame.SessionID, payload.ID); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return m.sendMutationAck(ctx, client, frame, map[string]any{
		"id": strings.TrimSpace(payload.ID),
	})
}
