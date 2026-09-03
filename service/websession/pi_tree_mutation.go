package websession

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

type PiTreeForkInput struct {
	TargetID string `json:"targetId"`
	Revision string `json:"revision"`
}

type PiTreeCloneInput struct {
	Revision string `json:"revision"`
}

type PiTreeCreateResult struct {
	Session    SessionSummary `json:"session"`
	Tree       PiTreeSnapshot `json:"tree"`
	EditorText string         `json:"editorText,omitempty"`
}

type piTreeMutationResult struct {
	Text      string `json:"text"`
	Cancelled bool   `json:"cancelled"`
}

func (m *Manager) ForkPiSessionTree(
	ctx context.Context,
	sessionID string,
	input PiTreeForkInput,
) (PiTreeCreateResult, error) {
	return m.createPiSessionFromTree(ctx, sessionID, "fork", strings.TrimSpace(input.TargetID), strings.TrimSpace(input.Revision))
}

func (m *Manager) ClonePiSessionTree(
	ctx context.Context,
	sessionID string,
	input PiTreeCloneInput,
) (PiTreeCreateResult, error) {
	return m.createPiSessionFromTree(ctx, sessionID, "clone", "", strings.TrimSpace(input.Revision))
}

func (m *Manager) createPiSessionFromTree(
	ctx context.Context,
	sessionID string,
	operation string,
	targetID string,
	expectedRevision string,
) (PiTreeCreateResult, error) {
	if m == nil {
		return PiTreeCreateResult{}, errors.New("web session manager is not configured")
	}
	if expectedRevision == "" {
		return PiTreeCreateResult{}, errors.New("Pi tree revision is required")
	}
	if operation == "fork" && targetID == "" {
		return PiTreeCreateResult{}, errors.New("Pi tree fork target id is required")
	}
	if operation != "fork" && operation != "clone" {
		return PiTreeCreateResult{}, errors.New("unsupported Pi tree mutation")
	}

	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	source, err := m.piTreeSession(ctx, sessionID)
	if err != nil {
		return PiTreeCreateResult{}, err
	}
	if m.hasActiveRun(source.ID) {
		return PiTreeCreateResult{}, errors.New("cannot mutate an active Pi web session")
	}
	if len(m.pendingInputsDisplaySnapshot(source.ID)) > 0 {
		return PiTreeCreateResult{}, errors.New("cannot mutate a Pi session while messages are pending")
	}

	runtime, err := m.getOrStartPiRuntime(ctx, source)
	if err != nil {
		return PiTreeCreateResult{}, err
	}
	mutationSent := false
	defer func() {
		if mutationSent {
			runtime.stop(errors.New("Pi tree mutation changed the native session"))
			return
		}
		runtime.scheduleIdle()
	}()

	current, rawCurrent, err := m.readPiTreeSnapshotRaw(ctx, runtime, source)
	if err != nil {
		return PiTreeCreateResult{}, err
	}
	if current.Revision != expectedRevision {
		return PiTreeCreateResult{}, ErrPiTreeRevisionConflict
	}
	if operation == "fork" {
		node, ok := rawCurrent[targetID]
		if !ok || isPiBridgeMarker(node.entry) || !piTreeForkableEntry(node.entry) {
			return PiTreeCreateResult{}, errors.New("Pi tree fork target is not a user message")
		}
	}

	operationCtx, cancel := context.WithTimeout(context.Background(), piRPCRequestTimeout)
	defer cancel()
	payload := map[string]any(nil)
	if operation == "fork" {
		payload = map[string]any{"entryId": targetID}
	}
	mutationSent = true
	var nativeResult piTreeMutationResult
	if err := runtime.client.Request(operationCtx, operation, payload, &nativeResult); err != nil {
		return PiTreeCreateResult{}, fmt.Errorf("Pi tree %s failed: %w", operation, err)
	}
	if nativeResult.Cancelled {
		return PiTreeCreateResult{}, fmt.Errorf("Pi tree %s was cancelled", operation)
	}

	var state piRPCState
	if err := runtime.client.Request(operationCtx, "get_state", nil, &state); err != nil {
		return PiTreeCreateResult{}, fmt.Errorf("read forked Pi session state: %w", err)
	}
	if err := validateNewPiTreeSessionIdentity(source, state, runtime.sessionRoot); err != nil {
		return PiTreeCreateResult{}, err
	}

	targetRecord, err := newPiTreeSessionRecord(source, operation, state)
	if err != nil {
		return PiTreeCreateResult{}, err
	}
	var entries piHistoryEntriesResponse
	if err := runtime.client.Request(operationCtx, "get_entries", nil, &entries); err != nil {
		return PiTreeCreateResult{}, fmt.Errorf("read forked Pi session entries: %w", err)
	}
	if operation == "clone" && strings.TrimSpace(pointerString(entries.LeafID)) == "" {
		return PiTreeCreateResult{}, errors.New("cloned Pi session has no active leaf")
	}
	targetRecord.NativeLeafID = nilIfEmpty(pointerString(entries.LeafID))

	var treeResponse struct {
		Tree   []piHistoryTreeNode `json:"tree"`
		LeafID *string             `json:"leafId"`
	}
	if err := runtime.client.Request(operationCtx, "get_tree", nil, &treeResponse); err != nil {
		return PiTreeCreateResult{}, fmt.Errorf("read forked Pi session tree: %w", err)
	}
	if pointerString(treeResponse.LeafID) != pointerString(entries.LeafID) {
		return PiTreeCreateResult{}, errors.New("forked Pi tree and entry leaf do not match")
	}
	revision := piSourceRevision(state.SessionFile, pointerString(treeResponse.LeafID))
	if revision == "" {
		return PiTreeCreateResult{}, errors.New("forked Pi tree revision is unavailable")
	}
	tree, _, err := projectPiTree(state.SessionID, revision, treeResponse.Tree, pointerString(treeResponse.LeafID))
	if err != nil {
		return PiTreeCreateResult{}, err
	}

	var stats piRPCSessionStats
	if err := runtime.client.Request(operationCtx, "get_session_stats", nil, &stats); err != nil {
		return PiTreeCreateResult{}, fmt.Errorf("read forked Pi session stats: %w", err)
	}
	if err := validatePiMutationStats(state, stats); err != nil {
		return PiTreeCreateResult{}, err
	}
	projection, err := buildPiHistoryProjection(targetRecord, entries)
	if err != nil {
		return PiTreeCreateResult{}, err
	}
	applyPiMutationStats(projection.updates, state, stats)
	if err := m.createProjectedPiTreeSession(operationCtx, &targetRecord, projection); err != nil {
		return PiTreeCreateResult{}, err
	}

	created, err := m.GetSession(operationCtx, targetRecord.ID)
	if err != nil {
		return PiTreeCreateResult{}, err
	}
	m.broadcastProjectSessionSummaries(context.Background(), source.ProjectID)
	return PiTreeCreateResult{
		Session: m.mapSessionSummary(created),
		Tree:    tree,
		EditorText: func() string {
			if operation == "fork" {
				return nativeResult.Text
			}
			return ""
		}(),
	}, nil
}

func piTreeForkableEntry(entry piHistoryEntry) bool {
	return strings.TrimSpace(entry.Type) == "message" && strings.EqualFold(strings.TrimSpace(entry.Message.Role), "user")
}

func validateNewPiTreeSessionIdentity(source tables.WebSessionTable, state piRPCState, sessionRoot string) error {
	if strings.TrimSpace(state.SessionID) == "" || strings.TrimSpace(state.SessionFile) == "" {
		return errors.New("Pi tree mutation returned an incomplete session identity")
	}
	if strings.TrimSpace(state.SessionID) == strings.TrimSpace(pointerString(source.NativeSessionID)) {
		return errors.New("Pi tree mutation did not create a new native session")
	}
	if samePiRuntimePath(state.SessionFile, pointerString(source.ThreadPath)) {
		return errors.New("Pi tree mutation reused the source session file")
	}
	candidate := source
	candidate.NativeSessionID = nilIfEmpty(state.SessionID)
	candidate.ThreadPath = nilIfEmpty(filepath.Clean(state.SessionFile))
	return validatePiRuntimeStartupStateWithinRoot(candidate, state, false, sessionRoot)
}

func newPiTreeSessionRecord(
	source tables.WebSessionTable,
	operation string,
	state piRPCState,
) (tables.WebSessionTable, error) {
	info, err := os.Stat(state.SessionFile)
	if err != nil {
		return tables.WebSessionTable{}, fmt.Errorf("stat forked Pi session: %w", err)
	}
	if !info.Mode().IsRegular() {
		return tables.WebSessionTable{}, errors.New("forked Pi session file is not regular")
	}
	now := time.Now()
	prefix := "Clone of "
	if operation == "fork" {
		prefix = "Fork of "
	}
	title := prefix + strings.TrimSpace(source.Title)
	if strings.TrimSpace(source.Title) == "" {
		title = prefix + "Pi session"
	}
	modelName := canonicalPiModel(state.Model)
	if modelName == "" {
		modelName = strings.TrimSpace(source.Model)
	}
	reasoning := piThinkingLevelToReasoning(state.ThinkingLevel)
	if reasoning == ReasoningEffortDefault {
		reasoning = normalizeReasoningEffort(ReasoningEffort(source.ReasoningEffort))
	}
	updatedAt := info.ModTime()
	record := tables.WebSessionTable{
		ProjectID: source.ProjectID, WorktreeID: source.WorktreeID,
		Agent: string(AgentPi), ClaudeRuntime: source.ClaudeRuntime,
		Backend: string(SessionBackendPiRPC), Title: title, TitleAuto: false,
		Model: modelName, ReasoningEffort: string(reasoning), WorkflowMode: source.WorkflowMode,
		PermissionLevel: source.PermissionLevel, ActiveCallTimeoutEnabled: source.ActiveCallTimeoutEnabled,
		AutoRetryEnabled: source.AutoRetryEnabled, AutoRetryScope: source.AutoRetryScope,
		AutoRetryPreset: source.AutoRetryPreset, AutoRetryMaxAttempts: source.AutoRetryMaxAttempts,
		AutoRetryDispatchPendingOnFailure: source.AutoRetryDispatchPendingOnFailure,
		LegacyPermissionMode:              source.LegacyPermissionMode, Cwd: source.Cwd,
		NativeSessionID: nilIfEmpty(state.SessionID), ThreadPath: nilIfEmpty(filepath.Clean(state.SessionFile)),
		Status: string(StatusIdle), AssistantState: "", HasUnread: false,
		ActivityAt: now, StatusUpdatedAt: &now, SourceKind: string(SessionBackendPiRPC),
		SyncState: string(SyncStateFresh), SourceUpdatedAt: &updatedAt,
		ThreadPreview: nilIfEmpty(title), AutoRetryAttempt: 0,
	}
	record.Init()
	return record, nil
}

func validatePiMutationStats(state piRPCState, stats piRPCSessionStats) error {
	if id := strings.TrimSpace(stats.SessionID); id != "" && id != strings.TrimSpace(state.SessionID) {
		return errors.New("forked Pi session stats id does not match state")
	}
	if path := strings.TrimSpace(stats.SessionFile); path != "" && !samePiRuntimePath(path, state.SessionFile) {
		return errors.New("forked Pi session stats file does not match state")
	}
	return nil
}

func applyPiMutationStats(updates map[string]any, state piRPCState, stats piRPCSessionStats) {
	usage := normalizePiSessionUsage(stats)
	now := time.Now()
	updates["native_session_id"] = strings.TrimSpace(state.SessionID)
	updates["thread_path"] = filepath.Clean(state.SessionFile)
	updates["total_input_tokens"] = usage.InputTokens
	updates["total_cached_input_tokens"] = usage.CachedInputTokens
	updates["total_output_tokens"] = usage.OutputTokens
	updates["total_cost"] = usage.Cost
	applyPiContextUsageUpdates(updates, stats.ContextUsage, true, now)
}

func (m *Manager) createProjectedPiTreeSession(
	ctx context.Context,
	record *tables.WebSessionTable,
	projection piHistoryProjection,
) error {
	if record == nil {
		return errors.New("forked Pi web session is missing")
	}
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var duplicate int64
		if err := tx.Unscoped().Model(&tables.WebSessionTable{}).
			Where("project_id = ? AND agent = ? AND native_session_id = ?", record.ProjectID, string(AgentPi), pointerString(record.NativeSessionID)).
			Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate > 0 {
			return errors.New("forked Pi session is already linked to a web session")
		}
		var maxOrder float64
		if err := tx.Model(&tables.WebSessionTable{}).
			Where("project_id = ? AND archived_at IS NULL", record.ProjectID).
			Select("COALESCE(MAX(order_index), 0)").Scan(&maxOrder).Error; err != nil {
			return err
		}
		record.OrderIndex = maxOrder + sessionOrderStep
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		if len(projection.turns) > 0 {
			if err := tx.Create(&projection.turns).Error; err != nil {
				return err
			}
		}
		if len(projection.items) > 0 {
			if err := tx.Create(&projection.items).Error; err != nil {
				return err
			}
		}
		if len(projection.updates) > 0 {
			if err := tx.Model(&tables.WebSessionTable{}).Where("id = ?", record.ID).
				Updates(withSnapshotRevisionIncrement(projection.updates)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
