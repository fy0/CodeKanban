package websession

import (
	"context"
	"errors"
	"strings"
	"time"

	"code-kanban/utils"

	"go.uber.org/zap"
)

var (
	errInvalidPendingInputMode = errors.New("invalid pending input mode")
	errEmptyPendingInput       = errors.New("message is empty")
)

func normalizePendingInputMode(mode PendingInputMode) PendingInputMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case string(PendingInputModeRedirect):
		return PendingInputModeRedirect
	case string(PendingInputModeQueue):
		return PendingInputModeQueue
	default:
		return ""
	}
}

func clonePendingInput(item PendingInput) PendingInput {
	return PendingInput{
		ID:            strings.TrimSpace(item.ID),
		Mode:          normalizePendingInputMode(item.Mode),
		Text:          item.Text,
		AttachmentIDs: append([]string(nil), item.AttachmentIDs...),
		CreatedAt:     item.CreatedAt,
	}
}

func clonePendingInputs(items []PendingInput) []PendingInput {
	if len(items) == 0 {
		return []PendingInput{}
	}
	cloned := make([]PendingInput, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, clonePendingInput(item))
	}
	return cloned
}

func sanitizePendingAttachmentIDs(attachmentIDs []string) []string {
	if len(attachmentIDs) == 0 {
		return nil
	}
	sanitized := make([]string, 0, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		trimmed := strings.TrimSpace(attachmentID)
		if trimmed == "" {
			continue
		}
		sanitized = append(sanitized, trimmed)
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func insertPendingInput(queue []PendingInput, item PendingInput) []PendingInput {
	if item.Mode != PendingInputModeRedirect {
		return append(queue, item)
	}
	insertAt := len(queue)
	for idx, queued := range queue {
		if queued.Mode != PendingInputModeRedirect {
			insertAt = idx
			break
		}
	}
	next := make([]PendingInput, 0, len(queue)+1)
	next = append(next, queue[:insertAt]...)
	next = append(next, item)
	next = append(next, queue[insertAt:]...)
	return next
}

func (m *Manager) pendingInputsSnapshot(sessionID string) []PendingInput {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return clonePendingInputs(m.pendingInputs[sessionID])
}

func (m *Manager) queuePendingInput(
	sessionID string,
	text string,
	attachmentIDs []string,
	mode PendingInputMode,
	pendingID string,
) (PendingInput, error) {
	normalizedMode := normalizePendingInputMode(mode)
	if normalizedMode == "" {
		return PendingInput{}, errInvalidPendingInputMode
	}
	normalizedPendingID := strings.TrimSpace(pendingID)
	if normalizedPendingID == "" {
		normalizedPendingID = utils.NewID()
	}
	item := PendingInput{
		ID:            normalizedPendingID,
		Mode:          normalizedMode,
		Text:          strings.TrimSpace(text),
		AttachmentIDs: sanitizePendingAttachmentIDs(attachmentIDs),
		CreatedAt:     time.Now(),
	}
	if item.Text == "" && len(item.AttachmentIDs) == 0 {
		return PendingInput{}, errEmptyPendingInput
	}

	m.mu.Lock()
	m.pendingInputs[sessionID] = insertPendingInput(m.pendingInputs[sessionID], item)
	m.mu.Unlock()

	m.broadcastPendingInputs(sessionID)
	m.triggerPendingProcessing(sessionID)
	return item, nil
}

func (m *Manager) sendMessageWithMode(
	ctx context.Context,
	sessionID string,
	text string,
	attachmentIDs []string,
	mode PendingInputMode,
	pendingID string,
) error {
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if record.ArchivedAt != nil {
		return errors.New("session is archived")
	}
	if err := m.ensureSessionMessagingAvailable(record); err != nil {
		return err
	}

	normalizedMode := normalizePendingInputMode(mode)
	if normalizedMode != "" && m.hasActiveRun(sessionID) {
		_, err := m.queuePendingInput(sessionID, text, attachmentIDs, normalizedMode, pendingID)
		return err
	}

	return m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, false)
}

func (m *Manager) removePendingInput(sessionID, pendingID string) bool {
	normalizedPendingID := strings.TrimSpace(pendingID)
	if normalizedPendingID == "" {
		return false
	}

	m.mu.Lock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		m.mu.Unlock()
		return false
	}
	next := make([]PendingInput, 0, len(queue))
	removed := false
	for _, item := range queue {
		if !removed && item.ID == normalizedPendingID {
			removed = true
			continue
		}
		next = append(next, item)
	}
	if len(next) == 0 {
		delete(m.pendingInputs, sessionID)
	} else {
		m.pendingInputs[sessionID] = next
	}
	m.mu.Unlock()

	if removed {
		m.broadcastPendingInputs(sessionID)
		m.triggerPendingProcessing(sessionID)
	}
	return removed
}

func (m *Manager) clearPendingInputs(sessionID string) {
	m.mu.Lock()
	delete(m.pendingInputs, sessionID)
	delete(m.pendingDirty, sessionID)
	m.mu.Unlock()
}

func (m *Manager) peekPendingInput(sessionID string) (PendingInput, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		return PendingInput{}, false
	}
	return clonePendingInput(queue[0]), true
}

func (m *Manager) popPendingInput(sessionID string) (PendingInput, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		return PendingInput{}, false
	}
	item := clonePendingInput(queue[0])
	if len(queue) == 1 {
		delete(m.pendingInputs, sessionID)
	} else {
		m.pendingInputs[sessionID] = append([]PendingInput(nil), queue[1:]...)
	}
	return item, true
}

func (m *Manager) prependPendingInput(sessionID string, item PendingInput) {
	cloned := clonePendingInput(item)
	m.mu.Lock()
	queue := append([]PendingInput(nil), m.pendingInputs[sessionID]...)
	m.pendingInputs[sessionID] = append([]PendingInput{cloned}, queue...)
	m.mu.Unlock()
}

func (m *Manager) triggerPendingProcessing(sessionID string) {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return
	}

	start := false
	m.mu.Lock()
	if m.pendingProcessing[normalizedSessionID] {
		m.pendingDirty[normalizedSessionID] = true
	} else {
		m.pendingProcessing[normalizedSessionID] = true
		delete(m.pendingDirty, normalizedSessionID)
		start = true
	}
	m.mu.Unlock()

	if !start {
		return
	}
	go m.runPendingProcessor(normalizedSessionID)
}

func (m *Manager) broadcastPendingInputs(sessionID string) {
	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil || record.ArchivedAt != nil {
		return
	}
	m.broadcast(newPendingFrame(sessionID, m.pendingInputsSnapshot(sessionID)))
}

func (m *Manager) finishPendingProcessing(sessionID string) {
	restart := false
	m.mu.Lock()
	delete(m.pendingProcessing, sessionID)
	if m.pendingDirty[sessionID] {
		delete(m.pendingDirty, sessionID)
		restart = true
	}
	m.mu.Unlock()

	if restart {
		m.triggerPendingProcessing(sessionID)
	}
}

func (m *Manager) runPendingProcessor(sessionID string) {
	defer m.finishPendingProcessing(sessionID)

	ctx := context.Background()
	for {
		_, ok := m.peekPendingInput(sessionID)
		if !ok {
			return
		}

		record, err := m.GetSession(ctx, sessionID)
		if err != nil {
			m.clearPendingInputs(sessionID)
			return
		}
		if record.ArchivedAt != nil {
			m.clearPendingInputs(sessionID)
			return
		}

		if m.hasActiveRun(sessionID) {
			return
		}

		next, ok := m.popPendingInput(sessionID)
		if !ok {
			return
		}
		m.broadcastPendingInputs(sessionID)

		if err := m.sendMessageInternal(ctx, sessionID, next.Text, next.AttachmentIDs, false); err != nil {
			m.prependPendingInput(sessionID, next)
			m.broadcastPendingInputs(sessionID)
			if m.logger != nil {
				m.logger.Debug("failed to flush pending input",
					zap.String("sessionId", sessionID),
					zap.String("pendingId", next.ID),
					zap.Error(err),
				)
			}
			if strings.Contains(strings.ToLower(err.Error()), "already running") {
				continue
			}
			return
		}
	}
}

func (m *Manager) maybeInterruptForRedirect(sessionID string) {
	item, ok := m.peekPendingInput(sessionID)
	if !ok || item.Mode != PendingInputModeRedirect || !m.hasActiveRun(sessionID) {
		return
	}
	go func(pendingID string) {
		if err := m.AbortSession(sessionID); err != nil && m.logger != nil {
			m.logger.Debug("failed to interrupt active session for redirect pending input",
				zap.String("sessionId", sessionID),
				zap.String("pendingId", pendingID),
				zap.Error(err),
			)
		}
	}(item.ID)
}
