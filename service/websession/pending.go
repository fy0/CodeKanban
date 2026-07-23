package websession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/utils"

	"go.uber.org/zap"
)

var (
	errInvalidPendingInputMode = errors.New("invalid pending input mode")
	errEmptyPendingInput       = errors.New("message is empty")
	errPendingInputNotFound    = errors.New("pending input not found")
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

func (m *Manager) validatePendingAttachmentIDs(attachmentIDs []string) error {
	sanitized := sanitizePendingAttachmentIDs(attachmentIDs)
	for _, attachmentID := range sanitized {
		if _, err := m.loadAttachment(attachmentID); err != nil {
			return fmt.Errorf("attachment %s not found", attachmentID)
		}
	}
	return nil
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
	sanitizedAttachmentIDs := sanitizePendingAttachmentIDs(attachmentIDs)
	item := PendingInput{
		ID:            normalizedPendingID,
		Mode:          normalizedMode,
		Text:          strings.TrimSpace(text),
		AttachmentIDs: sanitizedAttachmentIDs,
		CreatedAt:     time.Now(),
	}
	if item.Text == "" && len(item.AttachmentIDs) == 0 {
		return PendingInput{}, errEmptyPendingInput
	}
	if err := m.validatePendingAttachmentIDs(sanitizedAttachmentIDs); err != nil {
		return PendingInput{}, err
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
	if normalizedMode == PendingInputModeRedirect &&
		m.hasActiveRun(sessionID) &&
		normalizeAgent(Agent(record.Agent)) == AgentCodex &&
		effectiveSessionBackend(record) == SessionBackendCodexAppServer {
		handled, steerErr := m.steerActiveCodexTurn(ctx, record, text, attachmentIDs)
		if handled || steerErr != nil {
			return steerErr
		}
	}
	if normalizedMode != "" && (m.hasActiveRun(sessionID) || autoRetryDefersPending(record)) {
		_, err := m.queuePendingInput(sessionID, text, attachmentIDs, normalizedMode, pendingID)
		return err
	}

	err = m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, false)
	if normalizedMode != "" && err != nil && strings.Contains(strings.ToLower(err.Error()), "already running") {
		_, queueErr := m.queuePendingInput(sessionID, text, attachmentIDs, normalizedMode, pendingID)
		return queueErr
	}
	return err
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

func normalizePendingPartitionIndex(index int) int {
	if index < 0 {
		return 0
	}
	return index
}

func pendingPartitionItemCount(items []PendingInput, mode PendingInputMode) int {
	count := 0
	for _, item := range items {
		if item.Mode == mode {
			count++
		}
	}
	return count
}

func reorderPendingInput(queue []PendingInput, pendingID string, mode PendingInputMode, index int) ([]PendingInput, bool) {
	normalizedMode := normalizePendingInputMode(mode)
	normalizedPendingID := strings.TrimSpace(pendingID)
	if normalizedMode == "" || normalizedPendingID == "" || len(queue) == 0 {
		return queue, false
	}

	itemIndex := -1
	var item PendingInput
	remaining := make([]PendingInput, 0, len(queue)-1)
	for idx, queued := range queue {
		if itemIndex == -1 && queued.ID == normalizedPendingID {
			itemIndex = idx
			item = clonePendingInput(queued)
			continue
		}
		remaining = append(remaining, queued)
	}
	if itemIndex == -1 {
		return queue, false
	}

	item.Mode = normalizedMode
	targetIndex := normalizePendingPartitionIndex(index)
	partitionCount := pendingPartitionItemCount(remaining, normalizedMode)
	if targetIndex > partitionCount {
		targetIndex = partitionCount
	}

	result := make([]PendingInput, 0, len(queue))
	inserted := false
	seenInPartition := 0
	for _, queued := range remaining {
		if !inserted && queued.Mode == normalizedMode && seenInPartition == targetIndex {
			result = append(result, item)
			inserted = true
		}
		result = append(result, queued)
		if queued.Mode == normalizedMode {
			seenInPartition++
		}
	}
	if inserted {
		return result, true
	}

	if normalizedMode == PendingInputModeRedirect {
		insertAt := len(result)
		for idx, queued := range result {
			if queued.Mode != PendingInputModeRedirect {
				insertAt = idx
				break
			}
		}
		next := make([]PendingInput, 0, len(result)+1)
		next = append(next, result[:insertAt]...)
		next = append(next, item)
		next = append(next, result[insertAt:]...)
		return next, true
	}

	return append(result, item), true
}

func (m *Manager) updatePendingInput(sessionID, pendingID, text string) (PendingInput, error) {
	normalizedPendingID := strings.TrimSpace(pendingID)
	if normalizedPendingID == "" {
		return PendingInput{}, errPendingInputNotFound
	}
	normalizedText := strings.TrimSpace(text)
	if normalizedText == "" {
		return PendingInput{}, errEmptyPendingInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		return PendingInput{}, errPendingInputNotFound
	}
	for idx, item := range queue {
		if item.ID != normalizedPendingID {
			continue
		}
		item.Text = normalizedText
		queue[idx] = item
		m.pendingInputs[sessionID] = queue
		return clonePendingInput(item), nil
	}
	return PendingInput{}, errPendingInputNotFound
}

func (m *Manager) reorderPendingInput(sessionID, pendingID string, mode PendingInputMode, index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		return errPendingInputNotFound
	}
	next, ok := reorderPendingInput(queue, pendingID, mode, index)
	if !ok {
		return errPendingInputNotFound
	}
	m.pendingInputs[sessionID] = next
	return nil
}

func (m *Manager) clearPendingInputsForSession(sessionID string) bool {
	m.mu.Lock()
	if len(m.pendingInputs[sessionID]) == 0 {
		m.mu.Unlock()
		return false
	}
	delete(m.pendingInputs, sessionID)
	delete(m.pendingDirty, sessionID)
	m.mu.Unlock()
	m.broadcastPendingInputs(sessionID)
	m.triggerPendingProcessing(sessionID)
	return true
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
	_ = m.broadcastNextRevision(context.Background(), sessionID, func() (wireFrame, bool) {
		return newPendingFrame(sessionID, m.pendingInputsSnapshot(sessionID)), true
	})
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
		next, ok := m.peekPendingInput(sessionID)
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
			if next.Mode != PendingInputModeRedirect ||
				normalizeAgent(Agent(record.Agent)) != AgentCodex ||
				effectiveSessionBackend(record) != SessionBackendCodexAppServer {
				return
			}
			steerInput, ok := m.popPendingInput(sessionID)
			if !ok {
				return
			}
			m.broadcastPendingInputs(sessionID)
			handled, steerErr := m.steerActiveCodexTurn(
				ctx,
				record,
				steerInput.Text,
				steerInput.AttachmentIDs,
			)
			if steerErr != nil || !handled {
				m.prependPendingInput(sessionID, steerInput)
				m.broadcastPendingInputs(sessionID)
				if steerErr != nil && m.logger != nil {
					m.logger.Debug("failed to steer pending Codex redirect",
						zap.String("sessionId", sessionID),
						zap.String("pendingId", steerInput.ID),
						zap.Error(steerErr),
					)
				}
				return
			}
			continue
		}
		if autoRetryDefersPending(record) {
			return
		}

		next, ok = m.popPendingInput(sessionID)
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
	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	if run == nil || run.fromAutoRetry {
		return
	}

	item, ok := m.peekPendingInput(sessionID)
	if !ok || item.Mode != PendingInputModeRedirect {
		return
	}
	record, err := m.GetSession(context.Background(), sessionID)
	if err == nil &&
		normalizeAgent(Agent(record.Agent)) == AgentCodex &&
		effectiveSessionBackend(record) == SessionBackendCodexAppServer {
		m.triggerPendingProcessing(sessionID)
		return
	}
	go m.abortRunForRedirect(sessionID, item.ID, run)
}

func (m *Manager) abortRunForRedirect(sessionID, pendingID string, expectedRun *activeRun) {
	m.mu.RLock()
	current := m.runs[sessionID]
	isCurrent := current != nil && current == expectedRun && !current.fromAutoRetry
	m.mu.RUnlock()
	if !isCurrent {
		return
	}

	if expectedRun.cancel != nil {
		expectedRun.cancel()
	}
	killCmdTree(expectedRun.command())
	if m.logger != nil {
		m.logger.Debug("interrupted active session for redirect pending input",
			zap.String("sessionId", sessionID),
			zap.String("pendingId", pendingID),
		)
	}
}
