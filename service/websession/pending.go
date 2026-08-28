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
	errPendingInputDelivered     = errors.New("pending input was already delivered")
	errPendingInputIDConflict    = errors.New("pending input id was already used for different content")
)

const (
	pendingSteerRetryDelay     = 500 * time.Millisecond
	maxPendingSteerRetryDelay  = 5 * time.Second
	pendingSteerBlockedRecheck = 1 * time.Second
	maxDeliveredPendingInputs  = 256
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

func normalizePendingInputStatus(status PendingInputStatus) PendingInputStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(PendingInputStatusRetrying):
		return PendingInputStatusRetrying
	case string(PendingInputStatusPersisting):
		return PendingInputStatusPersisting
	case string(PendingInputStatusFailed):
		return PendingInputStatusFailed
	default:
		return ""
	}
}

func cloneCodexSteerReceipt(receipt *codexSteerReceipt) *codexSteerReceipt {
	if receipt == nil {
		return nil
	}
	cloned := *receipt
	cloned.event.Payload = cloneMap(receipt.event.Payload)
	return &cloned
}

func clonePendingInput(item PendingInput) PendingInput {
	var readyAt *time.Time
	if item.ReadyAt != nil {
		value := *item.ReadyAt
		readyAt = &value
	}
	return PendingInput{
		ID:                strings.TrimSpace(item.ID),
		Mode:              normalizePendingInputMode(item.Mode),
		Text:              item.Text,
		AttachmentIDs:     append([]string(nil), item.AttachmentIDs...),
		ReadyAt:           readyAt,
		Paused:            item.Paused,
		NativeQueued:      item.NativeQueued,
		Status:            normalizePendingInputStatus(item.Status),
		AttemptCount:      item.AttemptCount,
		LastError:         strings.TrimSpace(item.LastError),
		LastErrorCode:     strings.TrimSpace(item.LastErrorCode),
		CreatedAt:         item.CreatedAt,
		codexMessageID:    strings.TrimSpace(item.codexMessageID),
		codexSteerReceipt: cloneCodexSteerReceipt(item.codexSteerReceipt),
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

func resetPendingInputDelivery(item *PendingInput) {
	if item == nil {
		return
	}
	item.Status = ""
	item.AttemptCount = 0
	item.LastError = ""
	item.LastErrorCode = ""
	item.codexMessageID = ""
	item.codexSteerReceipt = nil
}

func pendingSteerRetryDelayForAttempt(attempt int) time.Duration {
	delay := pendingSteerRetryDelay
	for current := 1; current < attempt && delay < maxPendingSteerRetryDelay; current++ {
		delay *= 2
	}
	if delay > maxPendingSteerRetryDelay {
		return maxPendingSteerRetryDelay
	}
	return delay
}

func pendingInputRetry(
	item PendingInput,
	status PendingInputStatus,
	errorCode string,
	err error,
) PendingInput {
	item.Status = normalizePendingInputStatus(status)
	item.AttemptCount++
	item.LastErrorCode = strings.TrimSpace(errorCode)
	item.LastError = ""
	if err != nil {
		item.LastError = strings.TrimSpace(err.Error())
	}
	item.Paused = false
	readyAt := time.Now().Add(pendingSteerRetryDelayForAttempt(item.AttemptCount))
	item.ReadyAt = &readyAt
	return item
}

func failedPendingInput(item PendingInput, errorCode string, err error) PendingInput {
	item.Status = PendingInputStatusFailed
	item.AttemptCount++
	item.LastErrorCode = strings.TrimSpace(errorCode)
	item.LastError = ""
	if err != nil {
		item.LastError = strings.TrimSpace(err.Error())
	}
	item.Paused = true
	item.ReadyAt = nil
	return item
}

func pendingInputDeliveryLocked(item PendingInput) bool {
	return item.codexSteerReceipt != nil || item.Status == PendingInputStatusPersisting
}

func shouldLogPendingInputAttempt(attempt int) bool {
	return attempt <= 1 || attempt&(attempt-1) == 0
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

func (m *Manager) ensurePendingStateLocked() {
	if strings.TrimSpace(m.pendingEpoch) == "" {
		m.pendingEpoch = utils.NewID()
	}
	if m.pendingVersions == nil {
		m.pendingVersions = make(map[string]uint64)
	}
	if m.pendingDelivered == nil {
		m.pendingDelivered = make(map[string]map[string]PendingInput)
	}
	if m.pendingDeliveredOrder == nil {
		m.pendingDeliveredOrder = make(map[string][]string)
	}
}

func (m *Manager) bumpPendingVersionLocked(sessionID string) uint64 {
	m.ensurePendingStateLocked()
	m.pendingVersions[sessionID]++
	return m.pendingVersions[sessionID]
}

func (m *Manager) pendingStateSnapshot(sessionID string) (string, uint64, []PendingInput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensurePendingStateLocked()
	local := clonePendingInputs(m.pendingInputs[sessionID])
	native := clonePendingInputs(m.piNativeQueuedInputs[sessionID])
	items := make([]PendingInput, 0, len(local)+len(native))
	items = append(items, local...)
	items = append(items, native...)
	return m.pendingEpoch, m.pendingVersions[sessionID], items
}

func (m *Manager) pendingInputsDisplaySnapshot(sessionID string) []PendingInput {
	_, _, items := m.pendingStateSnapshot(sessionID)
	return items
}

func samePendingInputRequest(item PendingInput, text string, attachmentIDs []string, mode PendingInputMode) bool {
	if item.Mode != mode || item.Text != strings.TrimSpace(text) {
		return false
	}
	wanted := sanitizePendingAttachmentIDs(attachmentIDs)
	if len(item.AttachmentIDs) != len(wanted) {
		return false
	}
	for index := range wanted {
		if item.AttachmentIDs[index] != wanted[index] {
			return false
		}
	}
	return true
}

func (m *Manager) markPendingInputDelivered(sessionID string, item PendingInput) {
	pendingID := strings.TrimSpace(item.ID)
	if pendingID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensurePendingStateLocked()
	if m.pendingDelivered[sessionID] == nil {
		m.pendingDelivered[sessionID] = make(map[string]PendingInput)
	}
	if _, exists := m.pendingDelivered[sessionID][pendingID]; exists {
		m.pendingDelivered[sessionID][pendingID] = clonePendingInput(item)
		return
	}
	m.pendingDelivered[sessionID][pendingID] = clonePendingInput(item)
	order := append(m.pendingDeliveredOrder[sessionID], pendingID)
	if len(order) > maxDeliveredPendingInputs {
		oldest := order[0]
		order = order[1:]
		delete(m.pendingDelivered[sessionID], oldest)
	}
	m.pendingDeliveredOrder[sessionID] = order
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
	m.bumpPendingVersionLocked(sessionID)
	m.mu.Unlock()
	m.broadcastPendingInputs(sessionID)
}

func (m *Manager) clearPiNativeQueuedInputs(sessionID string) {
	m.mu.Lock()
	hadItems := len(m.piNativeQueuedInputs[sessionID]) > 0
	delete(m.piNativeQueuedInputs, sessionID)
	if hadItems {
		m.bumpPendingVersionLocked(sessionID)
	}
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
	m.ensurePendingStateLocked()
	for _, queued := range m.pendingInputs[sessionID] {
		if queued.ID != normalizedPendingID {
			continue
		}
		m.mu.Unlock()
		if !samePendingInputRequest(queued, item.Text, item.AttachmentIDs, item.Mode) {
			return PendingInput{}, errPendingInputIDConflict
		}
		return clonePendingInput(queued), nil
	}
	if delivered, ok := m.pendingDelivered[sessionID][normalizedPendingID]; ok {
		m.mu.Unlock()
		if !samePendingInputRequest(delivered, item.Text, item.AttachmentIDs, item.Mode) {
			return PendingInput{}, errPendingInputIDConflict
		}
		return clonePendingInput(delivered), nil
	}
	item.codexMessageID = normalizedPendingID
	m.pendingInputs[sessionID] = insertPendingInput(m.pendingInputs[sessionID], item)
	m.bumpPendingVersionLocked(sessionID)
	m.mu.Unlock()

	m.cancelPendingInputTimer(sessionID)
	m.broadcastPendingInputs(sessionID)
	m.triggerPendingProcessing(sessionID)
	return clonePendingInput(item), nil
}

type sendMessageModeResult struct {
	Pending bool
}

func (m *Manager) sendMessageWithModeResult(
	ctx context.Context,
	sessionID string,
	text string,
	attachmentIDs []string,
	mode PendingInputMode,
	pendingID string,
) (sendMessageModeResult, error) {
	normalizedMode := normalizePendingInputMode(mode)
	if normalizedMode != "" {
		m.mu.RLock()
		run := m.runs[sessionID]
		m.mu.RUnlock()
		if run != nil {
			_, err := m.queuePendingInput(
				sessionID,
				text,
				attachmentIDs,
				normalizedMode,
				pendingID,
			)
			return sendMessageModeResult{Pending: true}, err
		}
	}

	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return sendMessageModeResult{}, err
	}
	if record.ArchivedAt != nil {
		return sendMessageModeResult{}, errors.New("session is archived")
	}
	if err := m.ensureSessionMessagingAvailable(record); err != nil {
		return sendMessageModeResult{}, err
	}

	if normalizedMode != "" && (m.hasActiveRun(sessionID) || autoRetryDefersPending(record)) {
		_, err := m.queuePendingInput(
			sessionID,
			text,
			attachmentIDs,
			normalizedMode,
			pendingID,
		)
		return sendMessageModeResult{Pending: true}, err
	}

	err = m.sendMessageInternal(ctx, sessionID, text, attachmentIDs, sendMessageOptions{updateAutoTitle: true})
	if normalizedMode != "" && err != nil && strings.Contains(strings.ToLower(err.Error()), "already running") {
		_, queueErr := m.queuePendingInput(
			sessionID,
			text,
			attachmentIDs,
			normalizedMode,
			pendingID,
		)
		return sendMessageModeResult{Pending: true}, queueErr
	}
	return sendMessageModeResult{}, err
}

func (m *Manager) sendMessageWithMode(
	ctx context.Context,
	sessionID string,
	text string,
	attachmentIDs []string,
	mode PendingInputMode,
	pendingID string,
) error {
	_, err := m.sendMessageWithModeResult(ctx, sessionID, text, attachmentIDs, mode, pendingID)
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
			if pendingInputDeliveryLocked(item) {
				m.mu.Unlock()
				return false
			}
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
	if removed {
		m.bumpPendingVersionLocked(sessionID)
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
		if pendingInputDeliveryLocked(item) {
			m.mu.Unlock()
			return PendingInput{}, errPendingInputDelivered
		}
		if update.Text != nil {
			item.Text = normalizedText
			resetPendingInputDelivery(&item)
		}
		if update.Paused != nil && *update.Paused {
			item.Paused = true
			item.ReadyAt = nil
		} else if update.Text != nil || update.Paused != nil {
			if update.Text == nil {
				item.Status = ""
				item.AttemptCount = 0
				item.LastError = ""
				item.LastErrorCode = ""
			}
			item.Paused = false
			item.ReadyAt = nil
		}
		queue[idx] = item
		m.pendingInputs[sessionID] = queue
		m.bumpPendingVersionLocked(sessionID)
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
			if pendingInputDeliveryLocked(item) {
				m.mu.RUnlock()
				return errPendingInputDelivered
			}
			previousMode = item.Mode
			break
		}
	}
	m.mu.RUnlock()
	if previousMode == "" {
		return errPendingInputNotFound
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
			resetPendingInputDelivery(&next[itemIndex])
			next[itemIndex].Paused = false
			next[itemIndex].ReadyAt = nil
			break
		}
	}
	m.pendingInputs[sessionID] = next
	m.bumpPendingVersionLocked(sessionID)
	m.mu.Unlock()
	m.cancelPendingInputTimer(sessionID)
	return nil
}

func (m *Manager) clearPendingInputsForSession(sessionID string) bool {
	m.mu.Lock()
	queue := m.pendingInputs[sessionID]
	if len(queue) == 0 {
		m.mu.Unlock()
		m.cancelPendingInputTimer(sessionID)
		return false
	}
	retained := make([]PendingInput, 0, len(queue))
	for _, item := range queue {
		if pendingInputDeliveryLocked(item) {
			retained = append(retained, item)
		}
	}
	removed := len(retained) != len(queue)
	if len(retained) == 0 {
		delete(m.pendingInputs, sessionID)
	} else {
		m.pendingInputs[sessionID] = retained
	}
	if removed {
		m.bumpPendingVersionLocked(sessionID)
	}
	delete(m.pendingDirty, sessionID)
	m.mu.Unlock()
	if !removed {
		return false
	}
	m.cancelPendingInputTimer(sessionID)
	m.broadcastPendingInputs(sessionID)
	m.triggerPendingProcessing(sessionID)
	return true
}

func (m *Manager) clearPendingInputs(sessionID string) {
	m.mu.Lock()
	hadItems := len(m.pendingInputs[sessionID]) > 0 || len(m.piNativeQueuedInputs[sessionID]) > 0
	delete(m.pendingInputs, sessionID)
	delete(m.piNativeQueuedInputs, sessionID)
	delete(m.pendingDirty, sessionID)
	if hadItems {
		m.bumpPendingVersionLocked(sessionID)
	}
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
	m.bumpPendingVersionLocked(sessionID)
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
		m.bumpPendingVersionLocked(sessionID)
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
	m.bumpPendingVersionLocked(sessionID)
	m.mu.Unlock()
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
	epoch, version, items := m.pendingStateSnapshot(sessionID)
	m.broadcastFrame(newPendingFrame(sessionID, epoch, version, items))
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

		if next.codexSteerReceipt != nil {
			persistInput, claimed := m.claimPendingInput(sessionID, next.ID, next.Mode, time.Now())
			if !claimed {
				continue
			}
			m.cancelPendingInputTimer(sessionID)
			m.broadcastPendingInputs(sessionID)
			record, persistErr := m.GetSession(ctx, sessionID)
			if persistErr == nil {
				persistErr = m.persistCodexSteerReceipt(ctx, record, persistInput.codexSteerReceipt)
			}
			if persistErr == nil {
				continue
			}
			persistInput = pendingInputRetry(
				persistInput,
				PendingInputStatusPersisting,
				"local_persistence",
				persistErr,
			)
			m.prependPendingInput(sessionID, persistInput)
			m.broadcastPendingInputs(sessionID)
			m.setPendingInputTimer(sessionID, *persistInput.ReadyAt)
			if m.logger != nil && shouldLogPendingInputAttempt(persistInput.AttemptCount) {
				m.logger.Error("failed to persist accepted Codex steer message",
					zap.String("sessionId", sessionID),
					zap.String("pendingId", persistInput.ID),
					zap.String("turnId", persistInput.codexSteerReceipt.turnID),
					zap.Int("attempt", persistInput.AttemptCount),
					zap.Time("retryAt", *persistInput.ReadyAt),
					zap.Error(persistErr),
				)
			}
			return
		}

		m.mu.RLock()
		run := m.runs[sessionID]
		m.mu.RUnlock()
		if run != nil {
			if normalizeAgent(run.agent) == AgentPi && run.backend == SessionBackendPiRPC {
				if run.blocksPiPendingInput() {
					return
				}
				pending, claimed := m.claimPendingInput(sessionID, next.ID, next.Mode, time.Now())
				if !claimed {
					continue
				}
				m.cancelPendingInputTimer(sessionID)
				m.broadcastPendingInputs(sessionID)
				handled, sendErr := m.sendActivePiPendingInput(ctx, sessionID, pending)
				if sendErr != nil || !handled {
					if sendErr != nil {
						pending = failedPendingInput(pending, "pi_pending_send", sendErr)
					}
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
			if next.Mode != PendingInputModeRedirect || normalizeAgent(run.agent) != AgentCodex ||
				run.backend != SessionBackendCodexAppServer {
				return
			}
			if run.blocksCodexSteerForUserInput() {
				m.setPendingInputTimer(sessionID, time.Now().Add(pendingSteerBlockedRecheck))
				return
			}
			steerInput, claimed := m.claimPendingInput(sessionID, next.ID, next.Mode, time.Now())
			if !claimed {
				continue
			}
			m.cancelPendingInputTimer(sessionID)
			m.broadcastPendingInputs(sessionID)
			if strings.TrimSpace(steerInput.codexMessageID) == "" {
				steerInput.codexMessageID = utils.NewID()
			}
			handled, receipt, steerErr := m.steerActiveCodexTurn(
				ctx,
				sessionID,
				steerInput.Text,
				steerInput.AttachmentIDs,
				steerInput.codexMessageID,
			)
			if steerErr != nil || !handled || receipt == nil {
				if !handled && !m.hasActiveRun(sessionID) {
					resetPendingInputDelivery(&steerInput)
					steerInput.ReadyAt = nil
					m.prependPendingInput(sessionID, steerInput)
					m.broadcastPendingInputs(sessionID)
					continue
				}
				if steerErr == nil {
					steerErr = errors.New("active Codex turn is not currently steerable")
				}
				errorCode, retryable := codexSteerErrorMetadata(steerErr)
				if retryable || !handled {
					steerInput = pendingInputRetry(
						steerInput,
						PendingInputStatusRetrying,
						errorCode,
						steerErr,
					)
				} else {
					steerInput = failedPendingInput(steerInput, errorCode, steerErr)
				}
				m.prependPendingInput(sessionID, steerInput)
				m.broadcastPendingInputs(sessionID)
				if !steerInput.Paused && steerInput.ReadyAt != nil {
					m.setPendingInputTimer(sessionID, *steerInput.ReadyAt)
				}
				if m.logger != nil && (steerInput.Paused || shouldLogPendingInputAttempt(steerInput.AttemptCount)) {
					m.logger.Warn("failed to steer pending Codex redirect",
						zap.String("sessionId", sessionID),
						zap.String("pendingId", steerInput.ID),
						zap.String("errorCode", steerInput.LastErrorCode),
						zap.Int("attempt", steerInput.AttemptCount),
						zap.Bool("retrying", !steerInput.Paused),
						zap.Error(steerErr),
					)
				}
				return
			}
			m.markPendingInputDelivered(sessionID, steerInput)
			steerInput.codexSteerReceipt = receipt
			steerInput.Status = PendingInputStatusPersisting
			steerInput.AttemptCount = 0
			steerInput.LastError = ""
			steerInput.LastErrorCode = ""
			record, persistErr := m.GetSession(ctx, sessionID)
			if persistErr == nil {
				persistErr = m.persistCodexSteerReceipt(ctx, record, receipt)
			}
			if persistErr != nil {
				steerInput = pendingInputRetry(
					steerInput,
					PendingInputStatusPersisting,
					"local_persistence",
					persistErr,
				)
				m.prependPendingInput(sessionID, steerInput)
				m.broadcastPendingInputs(sessionID)
				m.setPendingInputTimer(sessionID, *steerInput.ReadyAt)
				if m.logger != nil && shouldLogPendingInputAttempt(steerInput.AttemptCount) {
					m.logger.Error("failed to persist accepted Codex steer message",
						zap.String("sessionId", sessionID),
						zap.String("pendingId", steerInput.ID),
						zap.String("turnId", receipt.turnID),
						zap.Int("attempt", steerInput.AttemptCount),
						zap.Time("retryAt", *steerInput.ReadyAt),
						zap.Error(persistErr),
					)
				}
				return
			}
			continue
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
		if autoRetryDefersPending(record) {
			return
		}

		next, ok = m.claimPendingInput(sessionID, next.ID, next.Mode, time.Now())
		if !ok {
			continue
		}
		m.cancelPendingInputTimer(sessionID)
		m.broadcastPendingInputs(sessionID)

		if err := m.sendMessageInternal(ctx, sessionID, next.Text, next.AttachmentIDs, sendMessageOptions{
			updateAutoTitle: true,
			userMessageID:   next.ID,
		}); err != nil {
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
		m.markPendingInputDelivered(sessionID, next)
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
