package websession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

var (
	errInvalidScheduledInputMode = errors.New("invalid scheduled input mode")
	errScheduledInputNotFound    = errors.New("scheduled input not found")
)

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
	default:
		return ""
	}
}

func activeScheduledInputStatuses() []string {
	return []string{
		string(ScheduledInputStatusScheduled),
		string(ScheduledInputStatusFailed),
	}
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

func mapScheduledInputRecord(record tables.WebSessionScheduledInputTable) ScheduledInput {
	return ScheduledInput{
		ID:            strings.TrimSpace(record.ID),
		Mode:          normalizeScheduledInputMode(ScheduledInputMode(record.Mode)),
		Text:          record.Text,
		AttachmentIDs: parseScheduledInputAttachmentIDs(record.AttachmentIDsJSON),
		ScheduledFor:  record.ScheduledFor,
		Status:        normalizeScheduledInputStatus(ScheduledInputStatus(record.Status)),
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
		SentAt:        record.SentAt,
		CanceledAt:    record.CanceledAt,
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

func (m *Manager) ScheduleInput(
	ctx context.Context,
	sessionID string,
	text string,
	attachmentIDs []string,
	mode ScheduledInputMode,
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

	if scheduledFor.IsZero() || !scheduledFor.After(time.Now()) {
		return ScheduledInput{}, fmt.Errorf("scheduled time must be in the future")
	}

	db := model.GetDB()
	if db == nil {
		return ScheduledInput{}, model.ErrDBNotInitialized
	}

	item := tables.WebSessionScheduledInputTable{
		WebSessionID:      record.ID,
		Mode:              string(normalizedMode),
		Text:              normalizedText,
		AttachmentIDsJSON: marshalScheduledInputAttachmentIDs(sanitizedAttachmentIDs),
		ScheduledFor:      scheduledFor,
		Status:            string(ScheduledInputStatusScheduled),
	}
	item.Init()
	if err := db.WithContext(ctx).Create(&item).Error; err != nil {
		return ScheduledInput{}, err
	}

	created := mapScheduledInputRecord(item)
	m.setScheduledInputTimer(created.ID, record.ID, created.ScheduledFor)
	m.broadcastScheduledInputs(record.ID)
	return created, nil
}

func (m *Manager) RemoveScheduledInput(ctx context.Context, sessionID, inputID string) error {
	normalizedSessionID := strings.TrimSpace(sessionID)
	normalizedInputID := strings.TrimSpace(inputID)
	if normalizedSessionID == "" || normalizedInputID == "" {
		return errScheduledInputNotFound
	}

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
	m.broadcastScheduledInputs(normalizedSessionID)
	return nil
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
		m.executeScheduledInput(inputID)
	})
	m.mu.Lock()
	m.scheduledInputTimers[inputID] = timer
	m.scheduledInputTimerSessions[inputID] = strings.TrimSpace(sessionID)
	m.mu.Unlock()
}

func (m *Manager) broadcastScheduledInputs(sessionID string) {
	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil || record.ArchivedAt != nil {
		return
	}
	items, err := m.scheduledInputsSnapshot(context.Background(), sessionID)
	if err != nil {
		return
	}
	m.broadcast(newScheduledFrame(sessionID, items))
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
	for _, record := range records {
		m.setScheduledInputTimer(record.ID, record.WebSessionID, record.ScheduledFor)
	}
	return nil
}

func (m *Manager) executeScheduledInput(inputID string) {
	ctx := context.Background()
	db := model.GetDB()
	if db == nil {
		return
	}

	var record tables.WebSessionScheduledInputTable
	if err := db.WithContext(ctx).First(&record, "id = ?", strings.TrimSpace(inputID)).Error; err != nil {
		return
	}
	if normalizeScheduledInputStatus(ScheduledInputStatus(record.Status)) != ScheduledInputStatusScheduled {
		return
	}

	session, err := m.GetSession(ctx, record.WebSessionID)
	if err != nil {
		_ = m.deleteScheduledInputByID(ctx, record.ID)
		return
	}
	if session.ArchivedAt != nil {
		_ = m.cancelScheduledInputByID(ctx, record.ID)
		return
	}

	attachmentIDs := parseScheduledInputAttachmentIDs(record.AttachmentIDsJSON)
	mode := normalizeScheduledInputMode(ScheduledInputMode(record.Mode))
	err = m.dispatchScheduledInput(ctx, record.WebSessionID, mode, record.Text, attachmentIDs)
	if err != nil {
		if shouldCancelScheduledInputDispatchError(err) {
			_ = m.cancelScheduledInputByID(ctx, record.ID)
		} else {
			_ = m.failScheduledInputByID(ctx, record.ID)
		}
		m.broadcastScheduledInputs(record.WebSessionID)
		return
	}

	_ = m.markScheduledInputDispatched(ctx, record.ID)
	m.broadcastScheduledInputs(record.WebSessionID)
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
		"sent_at":    now,
		"updated_at": now,
	})
}

func (m *Manager) failScheduledInputByID(ctx context.Context, inputID string) error {
	return m.updateScheduledInputStatus(ctx, inputID, map[string]any{
		"status":     string(ScheduledInputStatusFailed),
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
		Text        string   `json:"txt"`
		Attachments []string `json:"atts"`
		Mode        string   `json:"mode"`
		At          int64    `json:"at"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "bad_req", "invalid schedule payload", false))
	}
	created, err := m.ScheduleInput(
		ctx,
		frame.SessionID,
		payload.Text,
		payload.Attachments,
		ScheduledInputMode(payload.Mode),
		time.UnixMilli(payload.At),
	)
	if err != nil {
		return client.send(newErrorFrame(frame.RequestID, frame.SessionID, "invalid_state", err.Error(), false))
	}
	return client.send(newAckFrame(frame.RequestID, frame.Operation, frame.SessionID, mapWireScheduledInputs([]ScheduledInput{created})[0]))
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
	return client.send(newAckFrame(frame.RequestID, frame.Operation, frame.SessionID, map[string]any{
		"id": strings.TrimSpace(payload.ID),
	}))
}
