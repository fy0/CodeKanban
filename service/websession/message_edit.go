package websession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

var (
	ErrMessageEditUnsupported     = errors.New("message editing is only supported for Codex app-server sessions")
	ErrMessageEditSessionActive   = errors.New("stop the current run before editing a message")
	ErrMessageEditTargetNotFound  = errors.New("user message not found")
	ErrMessageEditHistoryConflict = errors.New("session history changed; refresh and try again")
	ErrMessageEditForkUnavailable = errors.New("the installed Codex version does not support message branching")
	ErrMessageEditEmpty           = errors.New("message is empty")
	ErrMessageEditSteeredMessage  = errors.New("messages added by steering an active turn cannot be edited independently")
)

var errMessageEditLocalBranchUnavailable = errors.New("local turn mapping is unavailable")

type editedMessageTurn struct {
	sourceTurnID string
	status       string
	errorJSON    string
}

type editedMessageBranchPoint struct {
	previousTurnID string
	prefixTurns    []editedMessageTurn
}

type codexEditableUserMessage struct {
	turnIndex   int
	turnID      string
	itemID      string
	text        string
	attachments int
}

// EditUserMessage creates a new Codex thread before the selected user turn and
// immediately starts a replacement turn. The source session is never mutated.
func (m *Manager) EditUserMessage(
	ctx context.Context,
	sessionID string,
	itemID string,
	text string,
) (SessionSnapshot, error) {
	sessionID = strings.TrimSpace(sessionID)
	itemID = strings.TrimSpace(itemID)
	if sessionID == "" || itemID == "" {
		return SessionSnapshot{}, ErrMessageEditTargetNotFound
	}

	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	dispatchLocked := true
	defer func() {
		if dispatchLocked {
			dispatchLock.Unlock()
		}
	}()

	source, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	if normalizeAgent(Agent(source.Agent)) != AgentCodex ||
		effectiveSessionBackend(source) != SessionBackendCodexAppServer {
		return SessionSnapshot{}, ErrMessageEditUnsupported
	}
	if source.NativeSessionID == nil || strings.TrimSpace(*source.NativeSessionID) == "" {
		return SessionSnapshot{}, ErrMessageEditUnsupported
	}
	if m.hasActiveRun(source.ID) {
		return SessionSnapshot{}, ErrMessageEditSessionActive
	}
	switch effectiveStatus(source, effectiveAssistantState(source)) {
	case StatusRunning, StatusWaitingApproval, StatusAborting:
		return SessionSnapshot{}, ErrMessageEditSessionActive
	}
	if err := m.ensureCodexMultiAgentV2Supported(); err != nil {
		return SessionSnapshot{}, err
	}

	target, err := m.findHistoryItemByID(ctx, source.ID, itemID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SessionSnapshot{}, ErrMessageEditTargetNotFound
		}
		return SessionSnapshot{}, err
	}
	if target.Kind != "user" {
		return SessionSnapshot{}, ErrMessageEditTargetNotFound
	}
	if err := m.ensureEditedMessageStartsTurn(ctx, source.ID, target); err != nil {
		return SessionSnapshot{}, err
	}

	text = strings.TrimSpace(text)
	if text == "" && len(target.Attachments) == 0 {
		return SessionSnapshot{}, ErrMessageEditEmpty
	}
	attachments, err := m.editMessageAttachments(target.Attachments)
	if err != nil {
		return SessionSnapshot{}, err
	}

	forkSummary, branchPoint, err := m.createEditedCodexBranch(ctx, source, target)
	if err != nil {
		return SessionSnapshot{}, err
	}
	if strings.TrimSpace(forkSummary.ID) == "" {
		return SessionSnapshot{}, fmt.Errorf("Codex did not return a forked thread id")
	}

	branch, err := m.createEditedWebSession(ctx, source, forkSummary, text)
	if err != nil {
		m.deleteCodexThreadBestEffort(forkSummary.ID, source.Cwd)
		return SessionSnapshot{}, err
	}
	cleanupBranch := func() {
		_ = m.DeleteSession(context.Background(), branch.ID)
		m.deleteCodexThreadBestEffort(forkSummary.ID, source.Cwd)
	}

	if err := m.copyEditedHistoryPrefix(ctx, source, branch, target.OrderIndex, branchPoint.prefixTurns); err != nil {
		cleanupBranch()
		return SessionSnapshot{}, err
	}

	attachmentIDs := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		attachmentIDs = append(attachmentIDs, attachment.ID)
	}

	// The replacement runs on the new branch, so it must not inherit the source
	// session's striped dispatch lock. The two IDs can hash to the same stripe.
	dispatchLock.Unlock()
	dispatchLocked = false
	if err := m.sendMessageInternal(ctx, branch.ID, text, attachmentIDs, sendMessageOptions{updateAutoTitle: true}); err != nil {
		cleanupBranch()
		return SessionSnapshot{}, err
	}

	snapshot, err := m.Snapshot(ctx, branch.ID, DefaultHistoryWindow)
	if err != nil {
		return SessionSnapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) editMessageAttachments(items []HistoryAttachment) ([]Attachment, error) {
	attachments := make([]Attachment, 0, len(items))
	for _, item := range items {
		attachment, err := m.loadAttachment(strings.TrimSpace(item.ID))
		if err != nil {
			return nil, fmt.Errorf("%w: attachment %s is unavailable", ErrMessageEditHistoryConflict, item.ID)
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func (m *Manager) ensureEditedMessageStartsTurn(
	ctx context.Context,
	sessionID string,
	target HistoryItem,
) error {
	if target.SourceTurnID == nil || strings.TrimSpace(*target.SourceTurnID) == "" {
		return nil
	}
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	var count int64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where(
			"web_session_id = ? AND source_turn_id = ? AND item_kind = ? AND order_index < ?",
			sessionID,
			strings.TrimSpace(*target.SourceTurnID),
			"user",
			target.OrderIndex,
		).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrMessageEditSteeredMessage
	}
	return nil
}

func (m *Manager) resolveEditedMessageTurn(
	ctx context.Context,
	sessionID string,
	target HistoryItem,
	turns []map[string]any,
) (int, error) {
	remoteMessages := m.codexEditableUserMessages(turns)
	if len(remoteMessages) == 0 {
		return -1, ErrMessageEditHistoryConflict
	}

	if target.SourceTurnID != nil {
		targetTurnID := strings.TrimSpace(*target.SourceTurnID)
		for index, candidate := range remoteMessages {
			if candidate.turnID == targetTurnID && editableMessagesMatch(target, candidate) {
				return validateRemoteEditedMessageStartsTurn(remoteMessages, index)
			}
		}
		return -1, ErrMessageEditHistoryConflict
	}
	if target.SourceItemID != nil {
		targetItemID := strings.TrimSpace(*target.SourceItemID)
		for index, candidate := range remoteMessages {
			if candidate.itemID == targetItemID && editableMessagesMatch(target, candidate) {
				return validateRemoteEditedMessageStartsTurn(remoteMessages, index)
			}
		}
	}

	db := model.GetDB()
	if db == nil {
		return -1, model.ErrDBNotInitialized
	}
	var rows []tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND item_kind = ?", sessionID, "user").
		Order("order_index ASC").
		Find(&rows).Error; err != nil {
		return -1, err
	}
	ordinal := -1
	for index, row := range rows {
		if row.ID == target.ID {
			ordinal = index
			break
		}
	}
	if ordinal < 0 || ordinal >= len(remoteMessages) || !editableMessagesMatch(target, remoteMessages[ordinal]) {
		return -1, ErrMessageEditHistoryConflict
	}
	return validateRemoteEditedMessageStartsTurn(remoteMessages, ordinal)
}

func validateRemoteEditedMessageStartsTurn(
	messages []codexEditableUserMessage,
	index int,
) (int, error) {
	if index < 0 || index >= len(messages) {
		return -1, ErrMessageEditHistoryConflict
	}
	turnIndex := messages[index].turnIndex
	for previous := 0; previous < index; previous++ {
		if messages[previous].turnIndex == turnIndex {
			return -1, ErrMessageEditSteeredMessage
		}
	}
	return turnIndex, nil
}

func (m *Manager) codexEditableUserMessages(turns []map[string]any) []codexEditableUserMessage {
	result := make([]codexEditableUserMessage, 0, len(turns))
	for turnIndex, turn := range turns {
		turnID := strings.TrimSpace(stringValue(turn["id"]))
		for _, rawItem := range decodeRawArray(turn["items"]) {
			item, err := m.mapThreadReadItem(rawItem, 0)
			if err != nil || item.Kind != "user" || item.ItemType != "userMessage" || isCodexInjectedHostContext(item.Text) {
				continue
			}
			result = append(result, codexEditableUserMessage{
				turnIndex:   turnIndex,
				turnID:      turnID,
				itemID:      strings.TrimSpace(stringValue(rawItem["id"])),
				text:        strings.TrimSpace(item.Text),
				attachments: len(item.Attachments),
			})
		}
	}
	return result
}

func editableMessagesMatch(target HistoryItem, candidate codexEditableUserMessage) bool {
	return strings.TrimSpace(target.Text) == candidate.text && len(target.Attachments) == candidate.attachments
}

func (m *Manager) createEditedCodexBranch(
	ctx context.Context,
	source tables.WebSessionTable,
	target HistoryItem,
) (codexThreadSummary, editedMessageBranchPoint, error) {
	branchPoint, err := m.resolveLocalEditedMessageBranchPoint(ctx, source.ID, target)
	if err == nil {
		summary, createErr := m.createEditedCodexThread(ctx, source, branchPoint.previousTurnID)
		return summary, branchPoint, createErr
	}
	if !errors.Is(err, errMessageEditLocalBranchUnavailable) {
		return codexThreadSummary{}, editedMessageBranchPoint{}, err
	}
	return m.createEditedCodexBranchFromRemote(ctx, source, target)
}

func (m *Manager) resolveLocalEditedMessageBranchPoint(
	ctx context.Context,
	sessionID string,
	target HistoryItem,
) (editedMessageBranchPoint, error) {
	targetTurnID := ""
	if target.SourceTurnID != nil {
		targetTurnID = strings.TrimSpace(*target.SourceTurnID)
	}
	if targetTurnID == "" {
		return editedMessageBranchPoint{}, errMessageEditLocalBranchUnavailable
	}

	db := model.GetDB()
	if db == nil {
		return editedMessageBranchPoint{}, model.ErrDBNotInitialized
	}
	var rows []tables.WebSessionTurnTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ?", sessionID).
		Order("order_index ASC").
		Find(&rows).Error; err != nil {
		return editedMessageBranchPoint{}, err
	}

	targetIndex := -1
	for index, row := range rows {
		if row.SourceTurnID != nil && strings.TrimSpace(*row.SourceTurnID) == targetTurnID {
			targetIndex = index
			break
		}
	}
	if targetIndex < 0 {
		return editedMessageBranchPoint{}, errMessageEditLocalBranchUnavailable
	}

	branchPoint := editedMessageBranchPoint{
		prefixTurns: make([]editedMessageTurn, 0, targetIndex),
	}
	for _, row := range rows[:targetIndex] {
		turnID := ""
		if row.SourceTurnID != nil {
			turnID = strings.TrimSpace(*row.SourceTurnID)
		}
		if turnID == "" {
			return editedMessageBranchPoint{}, errMessageEditLocalBranchUnavailable
		}
		branchPoint.prefixTurns = append(branchPoint.prefixTurns, editedMessageTurn{
			sourceTurnID: turnID,
			status:       row.Status,
			errorJSON:    row.ErrorJSON,
		})
	}
	if len(branchPoint.prefixTurns) > 0 {
		branchPoint.previousTurnID = branchPoint.prefixTurns[len(branchPoint.prefixTurns)-1].sourceTurnID
	}
	return branchPoint, nil
}

func (m *Manager) createEditedCodexBranchFromRemote(
	ctx context.Context,
	source tables.WebSessionTable,
	target HistoryItem,
) (codexThreadSummary, editedMessageBranchPoint, error) {
	var summary codexThreadSummary
	var branchPoint editedMessageBranchPoint
	err := m.withCodexQueryClient(ctx, source.Cwd, func(client *codexAppServerClient) error {
		response, err := client.request(ctx, "thread/read", map[string]any{
			"threadId":     strings.TrimSpace(*source.NativeSessionID),
			"includeTurns": true,
		})
		if err != nil {
			return err
		}
		payload := decodeRawObject(response.Result)
		thread := decodeRawObject(payload["thread"])
		turnValues := decodeRawArray(thread["turns"])
		turns := make([]map[string]any, 0, len(turnValues))
		for _, turn := range turnValues {
			turns = append(turns, decodeRawObject(turn))
		}

		targetTurnIndex, err := m.resolveEditedMessageTurn(ctx, source.ID, target, turns)
		if err != nil {
			return err
		}
		branchPoint.prefixTurns = make([]editedMessageTurn, 0, targetTurnIndex)
		for _, turn := range turns[:targetTurnIndex] {
			turnID := strings.TrimSpace(stringValue(turn["id"]))
			if turnID == "" {
				return ErrMessageEditHistoryConflict
			}
			branchPoint.prefixTurns = append(branchPoint.prefixTurns, editedMessageTurn{
				sourceTurnID: turnID,
				status:       firstNonEmpty(stringValue(turn["status"]), "completed"),
				errorJSON:    mustJSONText(turn["error"]),
			})
		}
		if len(branchPoint.prefixTurns) > 0 {
			branchPoint.previousTurnID = branchPoint.prefixTurns[len(branchPoint.prefixTurns)-1].sourceTurnID
		}

		summary, err = requestEditedCodexThread(ctx, client, source, branchPoint.previousTurnID)
		return err
	})
	return summary, branchPoint, err
}

func (m *Manager) createEditedCodexThread(
	ctx context.Context,
	source tables.WebSessionTable,
	previousTurnID string,
) (codexThreadSummary, error) {
	var summary codexThreadSummary
	err := m.withCodexQueryClient(ctx, source.Cwd, func(client *codexAppServerClient) error {
		var requestErr error
		summary, requestErr = requestEditedCodexThread(ctx, client, source, previousTurnID)
		return requestErr
	})
	return summary, err
}

func requestEditedCodexThread(
	ctx context.Context,
	client *codexAppServerClient,
	source tables.WebSessionTable,
	previousTurnID string,
) (codexThreadSummary, error) {
	method := "thread/start"
	params := codexThreadStartParams(source, true)
	if strings.TrimSpace(previousTurnID) != "" {
		method = "thread/fork"
		params = codexThreadForkParams(source, strings.TrimSpace(*source.NativeSessionID), previousTurnID)
	}
	response, err := client.request(ctx, method, params)
	if err != nil {
		if method == "thread/fork" && isCodexMethodUnavailable(err) {
			return codexThreadSummary{}, ErrMessageEditForkUnavailable
		}
		return codexThreadSummary{}, err
	}
	payload := decodeRawObject(response.Result)
	return parseCodexThreadSummary(payload["thread"]), nil
}

func codexThreadForkParams(session tables.WebSessionTable, threadID string, lastTurnID string) map[string]any {
	return map[string]any{
		"threadId":       strings.TrimSpace(threadID),
		"lastTurnId":     strings.TrimSpace(lastTurnID),
		"cwd":            session.Cwd,
		"model":          strings.TrimSpace(session.Model),
		"sandbox":        codexSandboxMode(effectivePermissionLevel(session)),
		"approvalPolicy": codexApprovalPolicy(effectivePermissionLevel(session)),
	}
}

func isCodexMethodUnavailable(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "method not found") ||
		strings.Contains(message, "unknown method") ||
		strings.Contains(message, "unsupported method")
}

func (m *Manager) createEditedWebSession(
	ctx context.Context,
	source tables.WebSessionTable,
	fork codexThreadSummary,
	text string,
) (tables.WebSessionTable, error) {
	title := deriveAutoTitleFromMessage(text)
	if title == "" {
		title = source.Title
	}
	worktreeID := ""
	if source.WorktreeID != nil {
		worktreeID = strings.TrimSpace(*source.WorktreeID)
	}
	summary, err := m.CreateSession(ctx, CreateParams{
		ProjectID:                         source.ProjectID,
		WorktreeID:                        worktreeID,
		Agent:                             AgentCodex,
		Backend:                           SessionBackendCodexAppServer,
		Model:                             source.Model,
		ReasoningEffort:                   ReasoningEffort(source.ReasoningEffort),
		WorkflowMode:                      effectiveWorkflowMode(source),
		PermissionLevel:                   effectivePermissionLevel(source),
		ActiveCallTimeoutEnabled:          source.ActiveCallTimeoutEnabled,
		ContextWindowSetting:              ptr(source.ContextWindowSetting),
		AutoRetryEnabled:                  source.AutoRetryEnabled,
		AutoRetryPolicyMode:               ptr(normalizeAutoRetryPolicyMode(AutoRetryPolicyMode(source.AutoRetryPolicyMode))),
		AutoRetryScope:                    ptr(normalizeAutoRetryScope(AutoRetryScope(source.AutoRetryScope))),
		AutoRetryPreset:                   ptr(normalizeAutoRetryPreset(AutoRetryPreset(source.AutoRetryPreset))),
		AutoRetryMaxAttempts:              ptr(normalizeAutoRetryMaxAttempts(source.AutoRetryMaxAttempts)),
		AutoRetryDispatchPendingOnFailure: ptr(source.AutoRetryDispatchPendingOnFailure),
		Title:                             title,
	})
	if err != nil {
		return tables.WebSessionTable{}, err
	}
	updates := map[string]any{
		"cwd":               source.Cwd,
		"native_session_id": fork.ID,
		"source_kind":       string(SessionBackendCodexAppServer),
		"sync_state":        SyncStateFresh,
		"last_sync_mode":    string(SyncModeFast),
		"source_created_at": fork.CreatedAt,
		"source_updated_at": fork.UpdatedAt,
		"last_synced_at":    time.Now(),
		"thread_path":       nilIfEmpty(fork.Path),
		"thread_preview":    nilIfEmpty(fork.Preview),
		"updated_at":        time.Now(),
	}
	if err := m.updateRuntimeState(ctx, summary.ID, updates); err != nil {
		_ = m.DeleteSession(context.Background(), summary.ID)
		return tables.WebSessionTable{}, err
	}
	return m.GetSession(ctx, summary.ID)
}

func (m *Manager) copyEditedHistoryPrefix(
	ctx context.Context,
	source tables.WebSessionTable,
	branch tables.WebSessionTable,
	targetOrder int64,
	turns []editedMessageTurn,
) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	var sourceItems []tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND order_index < ?", source.ID, targetOrder).
		Order("order_index ASC").
		Find(&sourceItems).Error; err != nil {
		return err
	}

	turnRows := make([]tables.WebSessionTurnTable, 0, len(turns))
	turnRowIDs := make(map[string]string, len(turns))
	for index, turn := range turns {
		turnID := strings.TrimSpace(turn.sourceTurnID)
		row := tables.WebSessionTurnTable{
			WebSessionID:  branch.ID,
			SourceTurnID:  nilIfEmptyHistory(turnID),
			OrderIndex:    int64(index + 1),
			Status:        firstNonEmpty(turn.status, "completed"),
			ErrorJSON:     turn.errorJSON,
			SourceCreated: true,
		}
		row.Init()
		turnRows = append(turnRows, row)
		if turnID != "" {
			turnRowIDs[turnID] = row.ID
		}
	}

	itemRows := make([]tables.WebSessionItemTable, 0, len(sourceItems))
	for _, sourceItem := range sourceItems {
		row := sourceItem
		row.ID = ""
		row.CreatedAt = time.Time{}
		row.UpdatedAt = time.Time{}
		row.DeletedAt = gorm.DeletedAt{}
		row.WebSessionID = branch.ID
		row.WebTurnID = nil
		row.Init()
		if row.SourceTurnID != nil {
			if turnRowID := turnRowIDs[strings.TrimSpace(*row.SourceTurnID)]; turnRowID != "" {
				row.WebTurnID = &turnRowID
			}
		}
		itemRows = append(itemRows, row)
	}

	return m.replaceSessionHistoryCache(ctx, branch, turnRows, itemRows, map[string]any{
		"turn_count":     len(turnRows),
		"item_count":     len(itemRows),
		"sync_state":     SyncStateFresh,
		"last_sync_mode": string(SyncModeFast),
		"last_event_seq": int64(0),
		"updated_at":     time.Now(),
	})
}

func (m *Manager) deleteCodexThreadBestEffort(threadID string, cwd string) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = m.withCodexQueryClient(ctx, cwd, func(client *codexAppServerClient) error {
		_, err := client.request(ctx, "thread/delete", map[string]any{"threadId": threadID})
		return err
	})
}
