package websession

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2/log_watcher"

	"go.uber.org/zap"
)

const piSessionHeaderLimit = 1024 * 1024

type piRuntimeContentState struct {
	kind   string
	text   string
	toolID string
}

type piRuntimeToolState struct {
	id        string
	name      string
	args      any
	output    string
	parentID  string
	lastEmit  time.Time
	completed bool
}

type piRuntimeDialog struct {
	id        string
	method    string
	itemID    string
	title     string
	requested time.Time
	expiresAt *time.Time
}

type piRuntimeRun struct {
	runtime *piSessionRuntime
	run     *activeRun
	session tables.WebSessionTable
	settled chan error
	once    sync.Once

	mu                   sync.Mutex
	assistantMessageID   string
	assistantMessageOpen bool
	contents             map[int]*piRuntimeContentState
	tools                map[string]*piRuntimeToolState
	dialog               *piRuntimeDialog
	compactionToolID     string
	compactionStarted    time.Time
	compactionCompleted  time.Time
	lastAttemptError     string
}

func (r *piRuntimeRun) settle(err error) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		r.settled <- err
		close(r.settled)
	})
}

type piSessionRuntime struct {
	manager   *Manager
	client    *piRPCClient
	sessionID string
	projectID string
	cwd       string

	mu        sync.Mutex
	active    *piRuntimeRun
	idleTimer *time.Timer
	stopped   bool
	stopOnce  sync.Once
}

func (m *Manager) getOrStartPiRuntime(
	ctx context.Context,
	session tables.WebSessionTable,
) (*piSessionRuntime, error) {
	if normalizeAgent(Agent(session.Agent)) != AgentPi || effectiveSessionBackend(session) != SessionBackendPiRPC {
		return nil, errors.New("Pi RPC runtime requires a Pi web session")
	}
	if err := m.EnsureProjectPiTrust(ctx, session.ProjectID, session.Cwd); err != nil {
		return nil, err
	}

	m.piRuntimeMu.Lock()
	existing := m.piRuntimes[session.ID]
	m.piRuntimeMu.Unlock()
	if existing != nil && existing.acquire() {
		return existing, nil
	}

	created, err := m.startPiRuntime(ctx, session)
	if err != nil {
		return nil, err
	}
	m.piRuntimeMu.Lock()
	if existing = m.piRuntimes[session.ID]; existing == nil {
		m.piRuntimes[session.ID] = created
		m.piRuntimeTerminators[session.ID] = piRuntimeTerminator{
			projectID: session.ProjectID,
			terminate: func() { created.stop(errors.New("Pi RPC runtime terminated")) },
		}
		m.piRuntimeMu.Unlock()
		go created.consumeEvents()
		return created, nil
	}
	m.piRuntimeMu.Unlock()
	created.stop(errors.New("duplicate Pi RPC runtime"))
	if existing.acquire() {
		return existing, nil
	}
	return nil, errors.New("Pi RPC runtime stopped during startup")
}

func (m *Manager) startPiRuntime(
	ctx context.Context,
	session tables.WebSessionTable,
) (*piSessionRuntime, error) {
	if !m.GetWebSessionRuntimeConfig().SupportsPiWebSession {
		return nil, errors.New(errPiWebSessionUnavailable)
	}
	bridgePath, err := m.materializePiBridge()
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, 4)
	threadPath := pointerString(session.ThreadPath)
	if threadPath != "" {
		args = append(args, "--session", threadPath)
	} else {
		args = append(args, "--name", session.Title)
	}
	// The reusable RPC process must outlive the active run that triggered its start.
	cmd, err := m.buildTrustedPiRPCCommand(context.Background(), session.ProjectID, session.Cwd, args...)
	if err != nil {
		return nil, err
	}
	client, err := startPiRPCClient(cmd)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = client.Close()
		}
	}()

	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var state piRPCState
	if err := client.Request(requestCtx, "get_state", nil, &state); err != nil {
		return nil, err
	}
	if err := validatePiRuntimeStartupState(session, state, threadPath == ""); err != nil {
		return nil, err
	}
	var commands struct {
		Commands []piRPCSlashCommand `json:"commands"`
	}
	if err := client.Request(requestCtx, "get_commands", nil, &commands); err != nil {
		return nil, err
	}
	if err := validatePiBridgeCommands(commands.Commands, bridgePath); err != nil {
		return nil, err
	}

	var entries struct {
		LeafID *string `json:"leafId"`
	}
	if err := client.Request(requestCtx, "get_entries", nil, &entries); err != nil {
		return nil, err
	}
	modelName := canonicalPiModel(state.Model)
	updates := map[string]any{
		"source_kind": string(SessionBackendPiRPC),
		"sync_error":  nil,
		"updated_at":  time.Now(),
	}
	if _, statErr := os.Stat(state.SessionFile); statErr == nil {
		updates["native_session_id"] = strings.TrimSpace(state.SessionID)
		updates["thread_path"] = filepath.Clean(state.SessionFile)
		updates["native_leaf_id"] = nilIfEmpty(pointerString(entries.LeafID))
		updates["source_revision"] = nilIfEmpty(piSourceRevision(state.SessionFile, pointerString(entries.LeafID)))
		updates["sync_state"] = string(SyncStateFresh)
	}
	if modelName != "" {
		updates["model"] = modelName
	}
	if normalized := piThinkingLevelToReasoning(state.ThinkingLevel); normalized != ReasoningEffortDefault {
		updates["reasoning_effort"] = string(normalized)
	}
	if err := m.updateRuntimeState(ctx, session.ID, updates); err != nil {
		return nil, err
	}

	failed = false
	return &piSessionRuntime{
		manager:   m,
		client:    client,
		sessionID: session.ID,
		projectID: session.ProjectID,
		cwd:       session.Cwd,
	}, nil
}

func validatePiRuntimeState(session tables.WebSessionTable, state piRPCState) error {
	return validatePiRuntimeStartupState(session, state, false)
}

func validatePiRuntimeStartupState(session tables.WebSessionTable, state piRPCState, allowMissingNewFile bool) error {
	sessionID := strings.TrimSpace(state.SessionID)
	sessionFile := strings.TrimSpace(state.SessionFile)
	if sessionID == "" || sessionFile == "" || !filepath.IsAbs(sessionFile) {
		return errors.New("Pi RPC get_state returned an invalid session identity")
	}
	if expected := strings.TrimSpace(pointerString(session.NativeSessionID)); expected != "" && expected != sessionID {
		return fmt.Errorf("Pi session id mismatch: got %q", sessionID)
	}
	if expected := strings.TrimSpace(pointerString(session.ThreadPath)); expected != "" && !samePiRuntimePath(expected, sessionFile) {
		return errors.New("Pi session file mismatch")
	}
	if err := validatePiSessionRoot(sessionFile); err != nil {
		return err
	}
	header, err := readPiRuntimeSessionHeader(sessionFile)
	if err != nil {
		if allowMissingNewFile && os.IsNotExist(err) && strings.TrimSpace(pointerString(session.NativeSessionID)) == "" && strings.TrimSpace(pointerString(session.ThreadPath)) == "" {
			return nil
		}
		return fmt.Errorf("verify Pi session header: %w", err)
	}
	if header.ID != sessionID {
		return errors.New("Pi session header id does not match get_state")
	}
	if !samePiRuntimePath(header.Cwd, session.Cwd) {
		return errors.New("Pi session cwd does not match the web session")
	}
	return nil
}

type piRuntimeSessionHeader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Cwd  string `json:"cwd"`
}

func readPiRuntimeSessionHeader(path string) (piRuntimeSessionHeader, error) {
	file, err := os.Open(path)
	if err != nil {
		return piRuntimeSessionHeader{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), piSessionHeaderLimit)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return piRuntimeSessionHeader{}, err
		}
		return piRuntimeSessionHeader{}, errors.New("empty Pi session file")
	}
	var header piRuntimeSessionHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return piRuntimeSessionHeader{}, err
	}
	header.Type = strings.TrimSpace(header.Type)
	header.ID = strings.TrimSpace(header.ID)
	header.Cwd = strings.TrimSpace(header.Cwd)
	if header.Type != "session" || header.ID == "" || header.Cwd == "" {
		return piRuntimeSessionHeader{}, errors.New("invalid Pi session header")
	}
	return header, nil
}

func canonicalPiRuntimePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	value = filepath.Clean(absolute)
	unresolved := make([]string, 0, 2)
	current := value
	for {
		if _, statErr := os.Lstat(current); statErr == nil {
			if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
				current = filepath.Clean(resolved)
			}
			break
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		unresolved = append(unresolved, filepath.Base(current))
		current = parent
	}
	for index := len(unresolved) - 1; index >= 0; index-- {
		current = filepath.Join(current, unresolved[index])
	}
	value = filepath.Clean(current)
	if runtime.GOOS == "windows" {
		value = strings.ToLower(value)
	}
	return value, nil
}

func samePiRuntimePath(left, right string) bool {
	canonicalLeft, leftErr := canonicalPiRuntimePath(left)
	canonicalRight, rightErr := canonicalPiRuntimePath(right)
	return leftErr == nil && rightErr == nil && canonicalLeft == canonicalRight
}

func validatePiSessionRoot(sessionFile string) error {
	root, err := log_watcher.ResolvePiSessionDir()
	if err != nil {
		return fmt.Errorf("resolve Pi session root: %w", err)
	}
	canonicalRoot, err := canonicalPiRuntimePath(root)
	if err != nil {
		return fmt.Errorf("resolve Pi session root: %w", err)
	}
	canonicalFile, err := canonicalPiRuntimePath(sessionFile)
	if err != nil {
		return fmt.Errorf("resolve Pi session file: %w", err)
	}
	relative, err := filepath.Rel(canonicalRoot, canonicalFile)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("Pi session file is outside the configured session root")
	}
	return nil
}

func piSourceRevision(path, leafID string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d:%s", info.ModTime().UnixNano(), info.Size(), strings.TrimSpace(leafID))
}

func canonicalPiModel(model *struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name"`
}) string {
	if model == nil {
		return ""
	}
	provider := strings.Trim(strings.TrimSpace(model.Provider), "/")
	id := strings.Trim(strings.TrimSpace(model.ID), "/")
	if provider == "" || id == "" {
		return ""
	}
	return provider + "/" + id
}

func splitPiModel(value string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", errors.New("Pi model must use provider/modelId")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func validatePiReasoningEffort(effort ReasoningEffort) error {
	normalized := normalizeReasoningEffort(effort)
	if normalized != ReasoningEffortDefault && piReasoningToThinkingLevel(normalized) == "" {
		return fmt.Errorf("Pi does not support reasoning effort %q", normalized)
	}
	return nil
}

func piReasoningToThinkingLevel(effort ReasoningEffort) string {
	switch normalizeReasoningEffort(effort) {
	case ReasoningEffortNone:
		return "off"
	case ReasoningEffortMinimal:
		return "minimal"
	case ReasoningEffortLow:
		return "low"
	case ReasoningEffortMedium:
		return "medium"
	case ReasoningEffortHigh:
		return "high"
	case ReasoningEffortXHigh:
		return "xhigh"
	case ReasoningEffortMax:
		return "max"
	default:
		return ""
	}
}

func piThinkingLevelToReasoning(level string) ReasoningEffort {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "off":
		return ReasoningEffortNone
	case "minimal":
		return ReasoningEffortMinimal
	case "low":
		return ReasoningEffortLow
	case "medium":
		return ReasoningEffortMedium
	case "high":
		return ReasoningEffortHigh
	case "xhigh":
		return ReasoningEffortXHigh
	case "max":
		return ReasoningEffortMax
	default:
		return ReasoningEffortDefault
	}
}

func (m *Manager) piPromptImages(attachments []Attachment) ([]piRPCImage, error) {
	images := make([]piRPCImage, 0, len(attachments))
	attachmentRoot, err := canonicalPiRuntimePath(m.store.attachmentsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve attachment root: %w", err)
	}
	for _, attachment := range attachments {
		declaredMime := strings.ToLower(strings.TrimSpace(attachment.Mime))
		if !strings.HasPrefix(declaredMime, "image/") {
			return nil, fmt.Errorf("Pi only supports image attachments; %s is %s", attachment.Name, attachment.Mime)
		}
		attachmentPath, err := canonicalPiRuntimePath(attachment.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve attachment %s: %w", attachment.Name, err)
		}
		relative, err := filepath.Rel(attachmentRoot, attachmentPath)
		if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("attachment %s is outside the attachment root", attachment.Name)
		}
		info, err := os.Stat(attachmentPath)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > m.cfg.AttachmentSizeLimit {
			return nil, fmt.Errorf("attachment %s has an invalid size or type", attachment.Name)
		}
		data, err := os.ReadFile(attachmentPath)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != info.Size() || !strings.HasPrefix(strings.ToLower(http.DetectContentType(data)), "image/") {
			return nil, fmt.Errorf("attachment %s is not a valid image", attachment.Name)
		}
		images = append(images, piRPCImage{
			Type:     "image",
			Data:     base64.StdEncoding.EncodeToString(data),
			MimeType: declaredMime,
		})
	}
	return images, nil
}

func (r *piSessionRuntime) acquire() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return false
	}
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	return true
}

func (r *piSessionRuntime) activate(run *activeRun, session tables.WebSessionTable) (*piRuntimeRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil, errPiRPCClosed
	}
	if r.active != nil {
		return nil, errors.New("Pi RPC runtime is already processing a prompt")
	}
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	dispatch := &piRuntimeRun{
		runtime:  r,
		run:      run,
		session:  session,
		settled:  make(chan error, 1),
		contents: make(map[int]*piRuntimeContentState),
		tools:    make(map[string]*piRuntimeToolState),
	}
	r.active = dispatch
	return dispatch, nil
}

func (r *piSessionRuntime) deactivate(dispatch *piRuntimeRun) {
	r.mu.Lock()
	if r.active == dispatch {
		r.active = nil
	}
	r.scheduleIdleLocked()
	r.mu.Unlock()
}

func (r *piSessionRuntime) scheduleIdle() {
	r.mu.Lock()
	r.scheduleIdleLocked()
	r.mu.Unlock()
}

func (r *piSessionRuntime) scheduleIdleLocked() {
	if r.stopped || r.active != nil {
		return
	}
	if r.idleTimer != nil {
		r.idleTimer.Stop()
	}
	ttl := r.manager.cfg.PiRuntimeIdleTTL
	r.idleTimer = time.AfterFunc(ttl, func() {
		r.stop(errors.New("Pi RPC idle timeout"))
	})
}

func (r *piSessionRuntime) consumeEvents() {
	for event := range r.client.Events() {
		r.mu.Lock()
		dispatch := r.active
		r.mu.Unlock()
		if dispatch == nil {
			continue
		}
		if event.Type == "codekanban_barrier" {
			if event.BarrierDone != nil {
				close(event.BarrierDone)
			}
			if !event.WakePending || event.BarrierRunID != dispatch.run.runID ||
				!dispatch.run.finishPiResponseBarrier(event.BarrierGeneration) {
				continue
			}
			r.manager.triggerPendingProcessing(dispatch.session.ID)
			continue
		}
		if err := r.manager.handlePiRuntimeEvent(dispatch, event); err != nil {
			dispatch.settle(err)
			continue
		}
		if event.Type == "agent_settled" {
			dispatch.settle(nil)
		}
	}
	err := r.client.processError()
	if err == nil {
		err = errPiRPCClosed
	}
	r.mu.Lock()
	dispatch := r.active
	r.mu.Unlock()
	if dispatch != nil {
		dispatch.settle(err)
	}
	r.manager.clearPiNativeQueuedInputs(r.sessionID)
	r.removeFromManager()
}

func (r *piSessionRuntime) stop(reason error) {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		r.mu.Lock()
		r.stopped = true
		if r.idleTimer != nil {
			r.idleTimer.Stop()
			r.idleTimer = nil
		}
		dispatch := r.active
		r.mu.Unlock()
		if dispatch != nil {
			abortCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = r.client.Request(abortCtx, "abort", nil, nil)
			cancel()
			dispatch.settle(reason)
		}
		_ = r.client.Close()
		r.manager.clearPiNativeQueuedInputs(r.sessionID)
		r.removeFromManager()
	})
}

func (r *piSessionRuntime) removeFromManager() {
	m := r.manager
	if m == nil {
		return
	}
	m.piRuntimeMu.Lock()
	if m.piRuntimes[r.sessionID] == r {
		delete(m.piRuntimes, r.sessionID)
		delete(m.piRuntimeTerminators, r.sessionID)
	}
	m.piRuntimeMu.Unlock()
}

func (m *Manager) runPiRPCSession(
	ctx context.Context,
	run *activeRun,
	session tables.WebSessionTable,
	text string,
	attachments []Attachment,
) {
	runtime, err := m.getOrStartPiRuntime(ctx, session)
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	dispatch, err := runtime.activate(run, session)
	if err != nil {
		runtime.scheduleIdle()
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	defer runtime.deactivate(dispatch)

	if model := strings.TrimSpace(session.Model); model != "" {
		provider, modelID, err := splitPiModel(model)
		if err != nil {
			m.handleRunFailure(session.ID, session, run, err)
			return
		}
		var selected piRPCSetModelResult
		if err := requestPiRuntimeControl(runtime.client, "set_model", map[string]any{
			"provider": provider,
			"modelId":  modelID,
		}, &selected); err != nil {
			m.handleRunFailure(session.ID, session, run, err)
			return
		}
		if !strings.EqualFold(strings.TrimSpace(selected.Provider), provider) ||
			!strings.EqualFold(strings.TrimSpace(selected.ID), modelID) {
			m.handleRunFailure(session.ID, session, run, errors.New("Pi did not select the requested model"))
			return
		}
	}
	effort := normalizeReasoningEffort(ReasoningEffort(session.ReasoningEffort))
	level := piReasoningToThinkingLevel(effort)
	if effort != ReasoningEffortDefault && level == "" {
		m.handleRunFailure(session.ID, session, run, fmt.Errorf("Pi does not support reasoning effort %q", effort))
		return
	}
	if ctx.Err() != nil {
		m.finishPiAbortedRun(session, run)
		return
	}
	if level != "" {
		if err := requestPiRuntimeControl(runtime.client, "set_thinking_level", map[string]any{"level": level}, nil); err != nil {
			m.handleRunFailure(session.ID, session, run, err)
			return
		}
	}
	if ctx.Err() != nil {
		m.finishPiAbortedRun(session, run)
		return
	}
	images, err := m.piPromptImages(attachments)
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	payload := map[string]any{"message": preparePromptText(text, effectiveWorkflowMode(session))}
	if len(images) > 0 {
		payload["images"] = images
	}
	if ctx.Err() != nil {
		m.finishPiAbortedRun(session, run)
		return
	}
	if err := requestPiRuntimeControl(runtime.client, "prompt", payload, nil); err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}

	select {
	case err := <-dispatch.settled:
		if ctx.Err() != nil {
			m.finishPiAbortedRun(session, run)
			return
		}
		if err != nil {
			m.handleRunFailure(session.ID, session, run, err)
			return
		}
	case <-ctx.Done():
		abortCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = runtime.client.Request(abortCtx, "abort", nil, nil)
		cancel()
		select {
		case <-dispatch.settled:
		case <-time.After(3 * time.Second):
			runtime.stop(errors.New("Pi RPC abort timed out"))
		}
		m.finishPiAbortedRun(session, run)
		return
	}

	if err := m.finishSuccessfulPiRun(runtime, dispatch, session, run); err != nil {
		m.handleRunFailure(session.ID, session, run, err)
	}
}

func (m *Manager) runPiRPCCompaction(ctx context.Context, run *activeRun, session tables.WebSessionTable) {
	runtime, err := m.getOrStartPiRuntime(ctx, session)
	if err != nil {
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	dispatch, err := runtime.activate(run, session)
	if err != nil {
		runtime.scheduleIdle()
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	defer runtime.deactivate(dispatch)

	requestDone := make(chan error, 1)
	go func() {
		requestCtx, cancel := context.WithTimeout(context.Background(), piRPCRequestTimeout)
		defer cancel()
		requestDone <- runtime.client.Request(requestCtx, "compact", nil, nil)
	}()
	select {
	case err := <-requestDone:
		if ctx.Err() != nil {
			m.finishPiAbortedRun(session, run)
			return
		}
		if err != nil {
			m.handleRunFailure(session.ID, session, run, err)
			return
		}
	case <-ctx.Done():
		abortCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = runtime.client.Request(abortCtx, "abort", nil, nil)
		cancel()
		select {
		case <-requestDone:
		case <-time.After(3 * time.Second):
			runtime.stop(errors.New("Pi RPC compaction abort timed out"))
		}
		m.finishPiAbortedRun(session, run)
		return
	}

	barrierCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = runtime.client.BarrierAndWait(barrierCtx, run.runID)
	cancel()
	if err != nil {
		runtime.stop(errors.New("Pi RPC compaction event barrier failed"))
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	if err := m.finishSuccessfulPiRun(runtime, dispatch, session, run); err != nil {
		m.handleRunFailure(session.ID, session, run, err)
	}
}

func (m *Manager) finishSuccessfulPiRun(runtime *piSessionRuntime, dispatch *piRuntimeRun, session tables.WebSessionTable, run *activeRun) error {
	var compactionAt *time.Time
	if dispatch != nil {
		dispatch.mu.Lock()
		if !dispatch.compactionCompleted.IsZero() {
			value := dispatch.compactionCompleted
			compactionAt = &value
		}
		dispatch.mu.Unlock()
	}
	if err := m.syncPiRuntimeSnapshot(context.Background(), runtime, session, compactionAt); err != nil {
		return err
	}
	if err := m.broadcastResyncRequired(context.Background(), session.ID, resyncReasonRuntimeReconciled); err != nil {
		return err
	}
	now := time.Now()
	if _, err := m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "run_done", RunID: run.runID, Timestamp: now,
		Payload: map[string]any{"ok": true, "st": string(StatusDone)},
	}); err != nil {
		return err
	}
	if err := m.updateRuntimeState(context.Background(), session.ID, applyAssistantStateUpdates(map[string]any{
		"status": string(StatusDone), "updated_at": now, "last_error": nil,
		"auto_retry_attempt": 0, "auto_retry_next_at": nil, "auto_retry_last_error_code": nil,
	}, AssistantStateNone, now)); err != nil {
		return err
	}
	m.cancelAutoRetryTimer(session.ID)
	m.broadcastSessionSummary(context.Background(), session.ID)
	run.syncSourceAfterRun = true
	return nil
}

func requestPiRuntimeControl(client *piRPCClient, command string, payload map[string]any, target any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.Request(ctx, command, payload, target)
}

func (m *Manager) sendActivePiPendingInput(
	ctx context.Context,
	sessionID string,
	pending PendingInput,
) (bool, error) {
	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	if run == nil || normalizeAgent(run.agent) != AgentPi || run.backend != SessionBackendPiRPC {
		return false, nil
	}
	if run.blocksPiPendingInput() {
		return false, nil
	}
	m.piRuntimeMu.Lock()
	runtime := m.piRuntimes[sessionID]
	m.piRuntimeMu.Unlock()
	if runtime == nil {
		return false, nil
	}
	runtime.mu.Lock()
	dispatch := runtime.active
	valid := !runtime.stopped && dispatch != nil && dispatch.run == run
	var session tables.WebSessionTable
	if valid {
		session = dispatch.session
	}
	runtime.mu.Unlock()
	if !valid {
		return false, nil
	}

	attachments := make([]Attachment, 0, len(pending.AttachmentIDs))
	for _, attachmentID := range pending.AttachmentIDs {
		attachment, err := m.loadAttachment(strings.TrimSpace(attachmentID))
		if err != nil {
			return true, fmt.Errorf("attachment %s not found", attachmentID)
		}
		attachments = append(attachments, attachment)
	}
	text := strings.TrimSpace(pending.Text)
	if text == "" && len(attachments) == 0 {
		return true, errors.New("message is empty")
	}
	images, err := m.piPromptImages(attachments)
	if err != nil {
		return true, err
	}
	command := "follow_up"
	if pending.Mode == PendingInputModeRedirect {
		command = "steer"
	}
	payload := map[string]any{"message": text}
	if len(images) > 0 {
		payload["images"] = images
	}
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = runtime.client.Request(requestCtx, command, payload, nil)
	cancel()
	if err != nil {
		return true, err
	}

	messageID := strings.TrimSpace(pending.ID)
	if messageID == "" {
		messageID = utils.NewID()
	}
	m.markPendingInputDelivered(sessionID, pending)
	if _, err := m.appendAndBroadcast(context.Background(), sessionID, session, Event{
		ID: utils.NewID(), Type: "msg_u", RunID: run.runID, ParentID: messageID, Timestamp: time.Now(),
		Payload: map[string]any{
			"mid": messageID, "txt": text, "atts": attachmentPayloads(attachments), "piQueueMode": string(pending.Mode),
		},
	}); err != nil && m.logger != nil {
		m.logger.Error("failed to persist Pi queued message", zap.String("sessionId", sessionID), zap.Error(err))
	}
	return true, nil
}

func (m *Manager) syncPiRuntimeSnapshot(ctx context.Context, runtime *piSessionRuntime, session tables.WebSessionTable, compactionAt ...*time.Time) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	var state piRPCState
	if err := runtime.client.Request(ctx, "get_state", nil, &state); err != nil {
		return err
	}
	if err := validatePiRuntimeState(session, state); err != nil {
		return err
	}
	var entries piHistoryEntriesResponse
	if err := runtime.client.Request(ctx, "get_entries", nil, &entries); err != nil {
		return err
	}
	var stats piRPCSessionStats
	if err := runtime.client.Request(ctx, "get_session_stats", nil, &stats); err != nil {
		return err
	}
	updates := map[string]any{
		"native_session_id":         state.SessionID,
		"thread_path":               filepath.Clean(state.SessionFile),
		"native_leaf_id":            nilIfEmpty(pointerString(entries.LeafID)),
		"source_revision":           nilIfEmpty(piSourceRevision(state.SessionFile, pointerString(entries.LeafID))),
		"total_input_tokens":        stats.Tokens.Input,
		"total_cached_input_tokens": stats.Tokens.CacheRead,
		"total_output_tokens":       stats.Tokens.Output,
		"total_cost":                stats.Cost,
		"last_synced_at":            time.Now(),
		"sync_state":                string(SyncStateFresh),
		"sync_error":                nil,
		"updated_at":                time.Now(),
	}
	if len(compactionAt) > 0 && compactionAt[0] != nil && !compactionAt[0].IsZero() {
		updates["last_context_compaction_at"] = *compactionAt[0]
		updates["context_baseline_input_tokens"] = stats.Tokens.Input
		updates["context_baseline_cached_input_tokens"] = stats.Tokens.CacheRead
		updates["context_baseline_output_tokens"] = stats.Tokens.Output
		updates["latest_token_count_input_tokens"] = 0
		updates["latest_token_count_cached_input_tokens"] = 0
		updates["latest_token_count_output_tokens"] = 0
		updates["latest_token_count_total_tokens"] = 0
		updates["latest_token_count_updated_at"] = nil
		updates["latest_turn_input_tokens"] = 0
		updates["latest_turn_cached_input_tokens"] = 0
		updates["latest_turn_output_tokens"] = 0
		updates["latest_turn_usage_updated_at"] = nil
	}
	if stats.ContextUsage != nil {
		updates["session_context_window_tokens"] = stats.ContextUsage.ContextWindow
		updates["session_context_window_observed_at"] = time.Now()
		if len(compactionAt) == 0 || compactionAt[0] == nil || compactionAt[0].IsZero() {
			updates["latest_token_count_total_tokens"] = stats.ContextUsage.Tokens
			updates["latest_token_count_updated_at"] = time.Now()
		}
	}
	if model := canonicalPiModel(state.Model); model != "" {
		updates["model"] = model
	}
	if effort := piThinkingLevelToReasoning(state.ThinkingLevel); effort != ReasoningEffortDefault {
		updates["reasoning_effort"] = string(effort)
	}
	return m.reconcileLivePiHistory(ctx, session, state.SessionID, entries, updates)
}

func (m *Manager) finishPiAbortedRun(session tables.WebSessionTable, run *activeRun) {
	_ = m.closePendingPiDialog(session, run, "Pi extension input was canceled because the run was aborted")
	now := time.Now()
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "run_abort", RunID: run.runID, Timestamp: now,
	})
	_ = m.updateRuntimeState(context.Background(), session.ID, applyAssistantStateUpdates(map[string]any{
		"status": string(StatusIdle), "updated_at": now,
		"auto_retry_attempt": 0, "auto_retry_next_at": nil, "auto_retry_last_error_code": nil,
	}, AssistantStateNone, now))
	m.cancelAutoRetryTimer(session.ID)
	m.broadcastSessionSummary(context.Background(), session.ID)
}
