package websession

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"

	"go.uber.org/zap"
)

var (
	errInvalidPendingInputMode   = errors.New("invalid pending input mode")
	errInvalidPendingInputUpdate = errors.New("invalid pending input update")
	errEmptyPendingInput         = errors.New("message is empty")
	errPendingInputNotFound      = errors.New("pending input not found")
)

const (
	defaultPendingSteerDelay = 5 * time.Second
	pendingSteerRetryDelay   = 500 * time.Millisecond
)

type pendingInputUpdate struct {
	Text   *string
	Paused *bool
}

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
	var readyAt *time.Time
	if item.ReadyAt != nil {
		value := *item.ReadyAt
		readyAt = &value
	}
	return PendingInput{
		ID:            strings.TrimSpace(item.ID),
		Mode:          normalizePendingInputMode(item.Mode),
		Text:          item.Text,
		AttachmentIDs: append([]string(nil), item.AttachmentIDs...),
		ReadyAt:       readyAt,
		Paused:        item.Paused,
		NativeQueued:  item.NativeQueued,
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

func isCodexSteerSession(record tables.WebSessionTable) bool {
	return normalizeAgent(Agent(record.Agent)) == AgentCodex &&
		effectiveSessionBackend(record) == SessionBackendCodexAppServer
}

func isPiNativePendingSession(record tables.WebSessionTable) bool {
	return normalizeAgent(Agent(record.Agent)) == AgentPi &&
		effectiveSessionBackend(record) == SessionBackendPiRPC
}

func (m *Manager) nextPendingSteerReadyAt() time.Time {
	delay := m.pendingSteerDelay
	if delay <= 0 {
		delay = defaultPendingSteerDelay
	}
	return time.Now().Add(delay)
}

func (m *Manager) sessionUsesCodexSteer(sessionID string) bool {
	record, err := m.GetSession(context.Background(), strings.TrimSpace(sessionID))
	return err == nil && record.ArchivedAt == nil && isCodexSteerSession(record)
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

func (m *Manager) pendingInputsDisplaySnapshot(sessionID string) []PendingInput {
	m.mu.RLock()
	defer m.mu.RUnlock()
	local := clonePendingInputs(m.pendingInputs[sessionID])
	native := clonePendingInputs(m.piNativeQueuedInputs[sessionID])
	return append(local, native...)
}

func (m *Manager) replacePiNativeQueuedInputs(sessionID string, steering, followUp []string) {
	now := time.Now()
	items := make([]PendingInput, 0, len(steering)+len(followUp))
	appendItems := func(mode PendingInputMode, values []string) {
		for index, value := range values {
			text := strings.TrimSpace(value)
			if text == "" {
				continue
			}
			digest := sha256.Sum256([]byte(text))
			items = append(items, PendingInput{
				ID:           fmt.Sprintf("pi-native:%s:%d:%x", mode, index, digest[:8]),
				Mode:         mode,
				Text:         text,
				NativeQueued: true,
				CreatedAt:    now,
			})
		}
	}
	appendItems(PendingInputModeRedirect, steering)
	appendItems(PendingInputModeQueue, followUp)

	m.mu.Lock()
	if len(items) == 0 {
		delete(m.piNativeQueuedInputs, sessionID)
	} else {
		m.piNativeQueuedInputs[sessionID] = items
	}
	m.mu.Unlock()
	m.broadcastPendingInputs(sessionID)
}

func (m *Manager) clearPiNativeQueuedInputs(sessionID string) {
	m.mu.Lock()
	hadItems := len(m.piNativeQueuedInputs[sessionID]) > 0
	delete(m.piNativeQueuedInputs, sessionID)
	m.mu.Unlock()
	if hadItems {
		m.broadcastPendingInputs(sessionID)
	}
}

func (m *Manager) queuePendingInput(
	sessionID string,
	text string,
	attachmentIDs []string,
	mode PendingInputMode,
	pendingID string,
) (PendingInput, error) {
	return m.queuePendingInputWithReadyAt(
		sessionID,
		text,
		attachmentIDs,
		mode,
		pendingID,
		nil,
	)
}

func (m *Manager) queuePendingInputWithReadyAt(
	sessionID string,
	text string,
	attachmentIDs []string,
	mode PendingInputMode,
	pendingID string,
	readyAt *time.Time,
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
	if readyAt != nil {
		value := *readyAt
		item.ReadyAt = &value
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

	m.cancelPendingInputTimer(sessionID)
	m.broadcastPendingInputs(sessionID)
	m.triggerPendingProcessing(sessionID)
	return clonePendingInput(item), nil
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
	if normalizedMode != "" && (m.hasActiveRun(sessionID) || autoRetryDefersPending(record)) {
		var readyAt *time.Time
		if normalizedMode == PendingInputModeRedirect && isCodexSteerSession(record) {
			value := m.nextPendingSteerReadyAt()
			readyAt = &value
		}
		_, err := m.queuePendingInputWithReadyAt(
			sessionID,
			text,
			attachmentIDs,
			normalizedMode,
			pendingID,
			readyAt,
		)
		return err
	}

	err = m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, sendMessageOptions{updateAutoTitle: true})
	if normalizedMode != "" && err != nil && strings.Contains(strings.ToLower(err.Error()), "already running") {
		var readyAt *time.Time
		if normalizedMode == PendingInputModeRedirect && isCodexSteerSession(record) {
			value := m.nextPendingSteerReadyAt()
			readyAt = &value
		}
		_, queueErr := m.queuePendingInputWithReadyAt(
			sessionID,
			text,
			attachmentIDs,
			normalizedMode,
			pendingID,
			readyAt,
		)
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
		m.cancelPendingInputTimer(sessionID)
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

func (m *Manager) updatePendingInput(
	sessionID string,
	pendingID string,
	update pendingInputUpdate,
) (PendingInput, error) {
	normalizedPendingID := strings.TrimSpace(pendingID)
	if normalizedPendingID == "" {
		return PendingInput{}, errPendingInputNotFound
	}
	if update.Text == nil && update.Paused == nil {
		return PendingInput{}, errInvalidPendingInputUpdate
	}
	var normalizedText string
	if update.Text != nil {
		normalizedText = strings.TrimSpace(*update.Text)
		if normalizedText == "" {
			return PendingInput{}, errEmptyPendingInput
		}
	}
	usesCodexSteer := false
	if update.Text != nil || (update.Paused != nil && !*update.Paused) {
		usesCodexSteer = m.sessionUsesCodexSteer(sessionID)
	}

	m.mu.Lock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		m.mu.Unlock()
		return PendingInput{}, errPendingInputNotFound
	}
	for idx, item := range queue {
		if item.ID != normalizedPendingID {
			continue
		}
		if update.Text != nil {
			item.Text = normalizedText
		}
		if update.Paused != nil && *update.Paused {
			item.Paused = true
			item.ReadyAt = nil
		} else if update.Text != nil || update.Paused != nil {
			item.Paused = false
			item.ReadyAt = nil
			if item.Mode == PendingInputModeRedirect && usesCodexSteer {
				value := m.nextPendingSteerReadyAt()
				item.ReadyAt = &value
			}
		}
		queue[idx] = item
		m.pendingInputs[sessionID] = queue
		updated := clonePendingInput(item)
		m.mu.Unlock()
		m.cancelPendingInputTimer(sessionID)
		return updated, nil
	}
	m.mu.Unlock()
	return PendingInput{}, errPendingInputNotFound
}

func (m *Manager) reorderPendingInput(sessionID, pendingID string, mode PendingInputMode, index int) error {
	normalizedMode := normalizePendingInputMode(mode)
	if normalizedMode == "" {
		return errInvalidPendingInputMode
	}
	normalizedPendingID := strings.TrimSpace(pendingID)
	if normalizedPendingID == "" {
		return errPendingInputNotFound
	}

	previousMode := PendingInputMode("")
	m.mu.RLock()
	for _, item := range m.pendingInputs[sessionID] {
		if item.ID == normalizedPendingID {
			previousMode = item.Mode
			break
		}
	}
	m.mu.RUnlock()
	if previousMode == "" {
		return errPendingInputNotFound
	}

	var readyAt *time.Time
	if previousMode != normalizedMode && normalizedMode == PendingInputModeRedirect &&
		m.sessionUsesCodexSteer(sessionID) {
		value := m.nextPendingSteerReadyAt()
		readyAt = &value
	}

	m.mu.Lock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		m.mu.Unlock()
		return errPendingInputNotFound
	}
	previousMode = ""
	for _, item := range queue {
		if item.ID == normalizedPendingID {
			previousMode = item.Mode
			break
		}
	}
	next, ok := reorderPendingInput(queue, normalizedPendingID, normalizedMode, index)
	if !ok {
		m.mu.Unlock()
		return errPendingInputNotFound
	}
	if previousMode != normalizedMode {
		for itemIndex := range next {
			if next[itemIndex].ID != normalizedPendingID {
				continue
			}
			next[itemIndex].Paused = false
			next[itemIndex].ReadyAt = nil
			if readyAt != nil {
				value := *readyAt
				next[itemIndex].ReadyAt = &value
			}
			break
		}
	}
	m.pendingInputs[sessionID] = next
	m.mu.Unlock()
	m.cancelPendingInputTimer(sessionID)
	return nil
}

func (m *Manager) clearPendingInputsForSession(sessionID string) bool {
	m.mu.Lock()
	if len(m.pendingInputs[sessionID]) == 0 {
		m.mu.Unlock()
		m.cancelPendingInputTimer(sessionID)
		return false
	}
	delete(m.pendingInputs, sessionID)
	delete(m.pendingDirty, sessionID)
	m.mu.Unlock()
	m.cancelPendingInputTimer(sessionID)
	m.broadcastPendingInputs(sessionID)
	m.triggerPendingProcessing(sessionID)
	return true
}

func (m *Manager) clearPendingInputs(sessionID string) {
	m.mu.Lock()
	delete(m.pendingInputs, sessionID)
	delete(m.pendingDirty, sessionID)
	m.mu.Unlock()
	m.cancelPendingInputTimer(sessionID)
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

func (m *Manager) claimPendingInput(
	sessionID string,
	pendingID string,
	mode PendingInputMode,
	now time.Time,
) (PendingInput, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		return PendingInput{}, false
	}
	if queue[0].ID != strings.TrimSpace(pendingID) || queue[0].Mode != mode || queue[0].Paused {
		return PendingInput{}, false
	}
	if queue[0].ReadyAt != nil && now.Before(*queue[0].ReadyAt) {
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

func (m *Manager) expireHeadPendingRedirectLocked(sessionID string, now time.Time) (bool, bool) {
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 || queue[0].Mode != PendingInputModeRedirect || queue[0].Paused {
		return false, false
	}

	changed := queue[0].ReadyAt == nil || queue[0].ReadyAt.After(now)
	if changed {
		readyAt := now
		queue[0].ReadyAt = &readyAt
		m.pendingInputs[sessionID] = queue
	}
	if timer := m.pendingInputTimers[sessionID]; timer != nil {
		timer.Stop()
	}
	delete(m.pendingInputTimers, sessionID)
	delete(m.pendingInputTimerDeadlines, sessionID)
	return true, changed
}

func (m *Manager) prependPendingInput(sessionID string, item PendingInput) {
	cloned := clonePendingInput(item)
	m.mu.Lock()
	queue := append([]PendingInput(nil), m.pendingInputs[sessionID]...)
	m.pendingInputs[sessionID] = append([]PendingInput{cloned}, queue...)
	m.mu.Unlock()
}

func (m *Manager) ensurePendingSteerReadyAt(
	sessionID string,
	pendingID string,
) (time.Time, bool, bool) {
	readyAt := m.nextPendingSteerReadyAt()
	m.mu.Lock()
	defer m.mu.Unlock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 || queue[0].ID != strings.TrimSpace(pendingID) ||
		queue[0].Mode != PendingInputModeRedirect || queue[0].Paused {
		return time.Time{}, false, false
	}
	if queue[0].ReadyAt != nil {
		return *queue[0].ReadyAt, true, false
	}
	value := readyAt
	queue[0].ReadyAt = &value
	m.pendingInputs[sessionID] = queue
	return readyAt, true, true
}

func (m *Manager) cancelPendingInputTimer(sessionID string) {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return
	}
	m.mu.Lock()
	if timer := m.pendingInputTimers[normalizedSessionID]; timer != nil {
		timer.Stop()
	}
	delete(m.pendingInputTimers, normalizedSessionID)
	delete(m.pendingInputTimerDeadlines, normalizedSessionID)
	m.mu.Unlock()
}

func (m *Manager) setPendingInputTimer(sessionID string, readyAt time.Time) {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return
	}
	delay := time.Until(readyAt)
	if delay <= 0 {
		m.triggerPendingProcessing(normalizedSessionID)
		return
	}

	m.mu.Lock()
	if current, ok := m.pendingInputTimerDeadlines[normalizedSessionID]; ok && current.Equal(readyAt) {
		m.mu.Unlock()
		return
	}
	if timer := m.pendingInputTimers[normalizedSessionID]; timer != nil {
		timer.Stop()
	}
	if m.pendingInputTimers == nil {
		m.pendingInputTimers = make(map[string]*time.Timer)
	}
	if m.pendingInputTimerDeadlines == nil {
		m.pendingInputTimerDeadlines = make(map[string]time.Time)
	}
	deadline := readyAt
	timer := time.AfterFunc(delay, func() {
		m.mu.Lock()
		current, ok := m.pendingInputTimerDeadlines[normalizedSessionID]
		if !ok || !current.Equal(deadline) {
			m.mu.Unlock()
			return
		}
		delete(m.pendingInputTimers, normalizedSessionID)
		delete(m.pendingInputTimerDeadlines, normalizedSessionID)
		m.mu.Unlock()
		m.triggerPendingProcessing(normalizedSessionID)
	})
	m.pendingInputTimers[normalizedSessionID] = timer
	m.pendingInputTimerDeadlines[normalizedSessionID] = deadline
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

func (m *Manager) hasPendingSessionWork(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pendingInputs[sessionID]) > 0 || m.pendingProcessing[sessionID]
}

func (m *Manager) broadcastPendingInputs(sessionID string) {
	record, err := m.GetSession(context.Background(), sessionID)
	if err != nil || record.ArchivedAt != nil {
		return
	}
	_ = m.broadcastTransientFrame(
		context.Background(),
		sessionID,
		newPendingFrame(sessionID, m.pendingInputsDisplaySnapshot(sessionID)),
	)
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
			m.cancelPendingInputTimer(sessionID)
			return
		}
		if next.Paused {
			m.cancelPendingInputTimer(sessionID)
			return
		}
		if next.ReadyAt != nil {
			if time.Now().Before(*next.ReadyAt) {
				m.setPendingInputTimer(sessionID, *next.ReadyAt)
				return
			}
			m.cancelPendingInputTimer(sessionID)
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
			m.mu.RLock()
			run := m.runs[sessionID]
			m.mu.RUnlock()
			if isPiNativePendingSession(record) {
				if run == nil || run.blocksPiPendingInput() {
					return
				}
				pending, claimed := m.claimPendingInput(sessionID, next.ID, next.Mode, time.Now())
				if !claimed {
					continue
				}
				m.cancelPendingInputTimer(sessionID)
				m.broadcastPendingInputs(sessionID)
				handled, sendErr := m.sendActivePiPendingInput(ctx, record, pending)
				if sendErr != nil || !handled {
					m.prependPendingInput(sessionID, pending)
					m.broadcastPendingInputs(sessionID)
					if sendErr != nil && m.logger != nil {
						m.logger.Debug("failed to send pending Pi input",
							zap.String("sessionId", sessionID),
							zap.String("pendingId", pending.ID),
							zap.Error(sendErr),
						)
					}
					return
				}
				continue
			}
			if next.Mode != PendingInputModeRedirect || !isCodexSteerSession(record) {
				return
			}
			if run != nil && run.blocksCodexSteerForUserInput() {
				return
			}
			if next.ReadyAt == nil {
				readyAt, found, changed := m.ensurePendingSteerReadyAt(sessionID, next.ID)
				if !found {
					continue
				}
				if changed {
					m.broadcastPendingInputs(sessionID)
				}
				m.setPendingInputTimer(sessionID, readyAt)
				return
			}
			steerInput, claimed := m.claimPendingInput(sessionID, next.ID, next.Mode, time.Now())
			if !claimed {
				continue
			}
			m.cancelPendingInputTimer(sessionID)
			m.broadcastPendingInputs(sessionID)
			handled, steerErr := m.steerActiveCodexTurn(ctx, record, steerInput.Text, steerInput.AttachmentIDs)
			if steerErr != nil || !handled {
				m.prependPendingInput(sessionID, steerInput)
				m.broadcastPendingInputs(sessionID)
				m.setPendingInputTimer(sessionID, time.Now().Add(pendingSteerRetryDelay))
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

		next, ok = m.claimPendingInput(sessionID, next.ID, next.Mode, time.Now())
		if !ok {
			continue
		}
		m.cancelPendingInputTimer(sessionID)
		m.broadcastPendingInputs(sessionID)

		if err := m.sendMessageInternal(ctx, sessionID, next.Text, next.AttachmentIDs, sendMessageOptions{updateAutoTitle: true}); err != nil {
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
	if !ok || item.Mode != PendingInputModeRedirect || item.Paused {
		return
	}
	record, err := m.GetSession(context.Background(), sessionID)
	if err == nil && (isCodexSteerSession(record) || isPiNativePendingSession(record)) {
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
