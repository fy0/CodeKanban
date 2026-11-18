package websession

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	codexSampleMethod        = "thread/sample"
	codexSamplePrompt        = "Reply exactly OK. No tools, file access, changes, network, MCP, or questions."
	promptCacheProbeTimeout  = 3 * time.Second
	promptCacheFailureLimit  = 3
	promptCacheIdleRetention = 5 * time.Minute
)

type PromptCacheRenewalState struct {
	Availability string `json:"availability"`
	Remaining    int    `json:"remaining"`
	Infinite     bool   `json:"infinite"`
	LastError    string `json:"lastError,omitempty"`
}

type promptCacheRenewal struct {
	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{}
	client *codexAppServerClient
	parent tables.WebSessionTable
	scope  codexTurnScope
	config utils.WebSessionPromptCacheRenewalConfig

	mu            sync.Mutex
	ready         bool
	remaining     int
	infinite      bool
	failures      int
	generation    uint64
	nextAt        time.Time
	idleUntil     time.Time
	attemptCancel context.CancelFunc
}

type codexSampleParams struct {
	ThreadID       string `json:"threadId"`
	ExpectedTurnID string `json:"expectedTurnId"`
	Prompt         string `json:"prompt"`
}

func isCodexSampleRoot(raw any) bool {
	thread := decodeRawObject(raw)
	ephemeral, known := thread["ephemeral"].(bool)
	if !known || ephemeral ||
		stringValue(thread["forkedFromId"]) != "" || stringValue(thread["parentThreadId"]) != "" ||
		(thread["sideConversation"] != nil && thread["sideConversation"] != false) {
		return false
	}
	for _, key := range []string{"source", "threadSource"} {
		if thread[key] == nil {
			continue
		}
		switch strings.ToLower(stringValue(thread[key])) {
		case "cli", "vscode", "exec", "appserver", "unknown", "user":
		default:
			return false
		}
	}
	return true
}

func codexExperimentalAPIDisabled(raw json.RawMessage) bool {
	result := decodeRawObject(raw)
	for _, values := range []map[string]any{result, decodeRawObject(result["capabilities"])} {
		if enabled, known := values["experimentalApi"].(bool); known && !enabled {
			return true
		}
	}
	return false
}

func (m *Manager) preparePromptCacheRenewal(ctx context.Context, session tables.WebSessionTable, run *activeRun, client *codexAppServerClient, scope codexTurnScope) *promptCacheRenewal {
	if !run.promptCacheRoot || run.hiddenBootstrap ||
		scope.threadID == "" || scope.turnID == "" || session.ArchivedAt != nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.promptCacheUnsupported != "" {
		return nil
	}
	for _, current := range m.promptCacheRenewals {
		if current.scope.threadID == scope.threadID {
			return nil
		}
	}
	if m.promptCacheRenewals == nil {
		m.promptCacheRenewals = make(map[string]*promptCacheRenewal)
	}
	config := utils.NormalizeWebSessionPromptCacheRenewalConfig(m.cfg.PromptCacheRenewal)
	renewalCtx, cancel := context.WithCancel(ctx)
	now := time.Now()
	interval := time.Duration(config.IntervalSeconds) * time.Second
	renewal := &promptCacheRenewal{
		ctx: renewalCtx, cancel: cancel, client: client, parent: session, scope: scope, config: config,
		wake: make(chan struct{}, 1), nextAt: now.Add(interval), idleUntil: now.Add(min(interval+time.Minute, promptCacheIdleRetention)),
	}
	m.promptCacheRenewals[session.ID] = renewal
	return renewal
}

func (m *Manager) PromptCacheRenewal(ctx context.Context, session tables.WebSessionTable, action string) PromptCacheRenewalState {
	state := PromptCacheRenewalState{Availability: "disabled"}
	if effectiveSessionBackend(session) != SessionBackendCodexAppServer {
		return state
	}
	m.mu.RLock()
	unsupported := m.promptCacheUnsupported
	renewal := m.promptCacheRenewals[session.ID]
	state.LastError = m.promptCacheFailures[session.ID]
	m.mu.RUnlock()
	if unsupported != "" {
		if m.cfg.PromptCacheRenewal.ShowUnsupported {
			state.Availability = "unsupported"
			state.LastError = unsupported
		}
		return state
	}
	state.Availability = "unavailable"
	if renewal == nil || renewal.ctx.Err() != nil || session.ArchivedAt != nil {
		return state
	}
	if action == "cycle" && m.ensureSessionMessagingAuthorized(ctx, session) != nil {
		return state
	}
	renewal.mu.Lock()
	defer renewal.mu.Unlock()
	if !renewal.ready {
		state.Availability = "checking"
		return state
	}
	state.Availability = "ready"
	renewal.updateMode(action)
	state.Remaining = renewal.remaining
	state.Infinite = renewal.infinite
	return state
}

func (renewal *promptCacheRenewal) updateMode(action string) {
	switch {
	case action == "off" || (action == "cycle" && renewal.infinite):
		renewal.generation++
		renewal.remaining, renewal.infinite, renewal.failures = 0, false, 0
		renewal.idleUntil = time.Now().Add(time.Minute)
		if renewal.attemptCancel != nil {
			renewal.attemptCancel()
		}
	case action == "cycle" && renewal.remaining == 3:
		renewal.remaining, renewal.infinite = 0, true
	case action == "cycle":
		renewal.remaining++
	default:
		return
	}
	select {
	case renewal.wake <- struct{}{}:
	default:
	}
}

func (renewal *promptCacheRenewal) stop() {
	renewal.cancel()
}

func (renewal *promptCacheRenewal) preempt() {
	renewal.stop()
	renewal.mu.Lock()
	sampling := renewal.attemptCancel != nil
	renewal.mu.Unlock()
	if sampling && renewal.client.cmd != nil && renewal.client.cmd.Process != nil {
		go killCmdTree(renewal.client.cmd)
	}
}

func (m *Manager) stopPromptCacheRenewal(sessionID string) {
	m.mu.Lock()
	renewal := m.promptCacheRenewals[sessionID]
	if renewal != nil {
		renewal.stop()
	}
	delete(m.promptCacheFailures, sessionID)
	m.mu.Unlock()
}

func (m *Manager) StopAllPromptCacheRenewals() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, renewal := range m.promptCacheRenewals {
		renewal.stop()
	}
}

func (m *Manager) disablePromptCacheRenewal(category string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.promptCacheUnsupported != "" {
		return
	}
	m.promptCacheUnsupported = category
	for _, renewal := range m.promptCacheRenewals {
		renewal.stop()
	}
	m.logger.Info("Codex prompt cache renewal unavailable", zap.String("error_category", category))
}

func (m *Manager) runPromptCacheRenewal(renewal *promptCacheRenewal) {
	defer renewal.stop()
	if !m.probePromptCacheRenewal(renewal) {
		return
	}
	if category := readPromptCacheRoot(renewal.ctx, renewal, false); category != "" {
		m.recordPromptCacheFailure(renewal, category)
		return
	}
	renewal.mu.Lock()
	renewal.ready = true
	renewal.mu.Unlock()
	for renewal.ctx.Err() == nil {
		renewal.mu.Lock()
		queued := renewal.infinite || renewal.remaining > 0
		deadline := renewal.idleUntil
		if queued {
			deadline = renewal.nextAt
		}
		if time.Now().Before(deadline) {
			renewal.mu.Unlock()
			timer := time.NewTimer(time.Until(deadline))
			select {
			case <-renewal.ctx.Done():
			case <-renewal.wake:
			case <-timer.C:
			}
			timer.Stop()
			continue
		}
		if !queued {
			renewal.stop()
			renewal.mu.Unlock()
			return
		}
		attemptCtx, cancel := context.WithTimeout(renewal.ctx, time.Duration(renewal.config.SampleTimeoutSeconds)*time.Second)
		generation := renewal.generation
		renewal.attemptCancel = cancel
		renewal.mu.Unlock()

		category, stop := m.tryPromptCacheSample(attemptCtx, renewal)
		cancel()
		if !stop && category != "sample_cancelled" && renewal.ctx.Err() == nil {
			if parentError := m.promptCacheParentError(renewal.ctx, renewal); parentError != "" {
				category, stop = parentError, true
			}
		}
		m.recordPromptCacheFailure(renewal, category)
		renewal.mu.Lock()
		renewal.attemptCancel = nil
		renewal.finishAttempt(generation, category, time.Now())
		renewal.mu.Unlock()
		if stop {
			return
		}
	}
}

func readPromptCacheRoot(ctx context.Context, renewal *promptCacheRenewal, requireIdle bool) string {
	ctx, cancel := context.WithTimeout(ctx, promptCacheProbeTimeout)
	defer cancel()
	response, err := renewal.client.request(ctx, "thread/read", map[string]any{
		"threadId": renewal.scope.threadID, "includeTurns": false,
	})
	if err != nil {
		return codexSampleError(renewal.client, err)
	}
	thread := decodeRawObject(decodeRawObject(response.Result)["thread"])
	if !isCodexSampleRoot(thread) || stringValue(thread["path"]) == "" || stringValue(thread["id"]) != renewal.scope.threadID {
		return "parent_not_root"
	}
	if requireIdle {
		switch strings.ToLower(codexThreadStatusValue(thread["status"])) {
		case "idle":
		case "notloaded":
			return "parent_missing"
		default:
			return "parent_busy"
		}
	}
	return ""
}

func (renewal *promptCacheRenewal) finishAttempt(generation uint64, category string, now time.Time) {
	interval := renewal.config.IntervalSeconds
	if category != "" && category != "sample_cancelled" {
		interval = max(interval, renewal.config.FailureBackoffSeconds)
	}
	renewal.nextAt = now.Add(time.Duration(interval) * time.Second)
	if generation != renewal.generation {
		return
	}
	if renewal.remaining > 0 {
		renewal.remaining--
	}
	if category == "" {
		renewal.failures = 0
	} else {
		renewal.failures++
		if renewal.failures >= promptCacheFailureLimit {
			renewal.infinite = false
		}
	}
	if renewal.remaining == 0 && !renewal.infinite {
		renewal.idleUntil = now.Add(time.Minute)
	}
}

func (m *Manager) probePromptCacheRenewal(renewal *promptCacheRenewal) bool {
	ctx, cancel := context.WithTimeout(renewal.ctx, promptCacheProbeTimeout)
	defer cancel()
	_, err := renewal.client.request(ctx, codexSampleMethod, codexSampleParams{
		ThreadID: uuid.NewString(), ExpectedTurnID: uuid.NewString(), Prompt: codexSamplePrompt,
	})
	category := codexSampleError(renewal.client, err)
	if category == "parent_missing" {
		return true
	}
	if category == "" {
		category = "protocol_incompatible"
	}
	if isCodexSampleUnsupported(category) {
		m.disablePromptCacheRenewal(category)
	} else {
		m.recordPromptCacheFailure(renewal, category)
	}
	return false
}

func (m *Manager) promptCacheParentError(ctx context.Context, renewal *promptCacheRenewal) string {
	if m.hasActiveRun(renewal.parent.ID) || m.hasPendingSessionWork(renewal.parent.ID) {
		return "parent_busy"
	}
	current, err := m.GetSession(ctx, renewal.parent.ID)
	if err != nil || current.ArchivedAt != nil {
		return "parent_missing"
	}
	parent := renewal.parent
	if !stringPointerEqual(current.NativeSessionID, renewal.scope.threadID) || current.HistoryEpoch != parent.HistoryEpoch ||
		current.Cwd != parent.Cwd || current.Model != parent.Model || current.ReasoningEffort != parent.ReasoningEffort ||
		current.PermissionLevel != parent.PermissionLevel || current.WorkflowMode != parent.WorkflowMode ||
		current.ContextWindowSetting != parent.ContextWindowSetting || current.CodexFastMode != parent.CodexFastMode ||
		effectiveSessionBackend(current) != SessionBackendCodexAppServer {
		return "expected_turn_changed"
	}
	if m.ensureSessionMessagingAuthorized(ctx, current) != nil {
		return "parent_unavailable"
	}
	return readPromptCacheRoot(ctx, renewal, true)
}

func (m *Manager) tryPromptCacheSample(ctx context.Context, renewal *promptCacheRenewal) (string, bool) {
	category := m.promptCacheParentError(ctx, renewal)
	var usage codexSampleUsage
	var duration time.Duration
	stop := category != ""
	if ctx.Err() != nil {
		category = codexSampleError(renewal.client, ctx.Err())
		stop = false
	} else if category == "" {
		requestID := renewal.client.nextRequestID()
		startedAt := time.Now()
		response, err := renewal.client.requestWithID(ctx, requestID, codexSampleMethod, codexSampleParams{
			ThreadID: renewal.scope.threadID, ExpectedTurnID: renewal.scope.turnID, Prompt: codexSamplePrompt,
		})
		duration = time.Since(startedAt)
		category = codexSampleError(renewal.client, err)
		if err == nil {
			usage = decodeCodexSampleUsage(response.Result)
		} else if ctx.Err() != nil || isCodexAppServerRequestTimeout(err) {
			cancelCtx, cancel := context.WithTimeout(renewal.ctx, promptCacheProbeTimeout)
			_, cancelErr := renewal.client.request(cancelCtx, "thread/sample/cancel", map[string]string{"requestId": requestID})
			cancel()
			stop = cancelErr != nil
		}
	}
	m.logger.Info("Codex prompt cache sample",
		zap.String("parent_thread_id", renewal.scope.threadID), zap.String("expected_turn_id", renewal.scope.turnID),
		zap.Bool("success", category == ""), zap.Duration("rpc_duration", duration),
		zap.Int64p("input_tokens", usage.Input), zap.Int64p("cached_input_tokens", usage.Cached),
		zap.Int64p("output_tokens", usage.Output), zap.String("error_category", category),
	)
	if !stop && isCodexSampleUnsupported(category) {
		m.disablePromptCacheRenewal(category)
		return category, true
	}
	switch category {
	case "parent_busy", "parent_missing", "parent_not_root", "expected_turn_changed", "connection_closed":
		stop = true
	}
	return category, stop
}

func (m *Manager) recordPromptCacheFailure(renewal *promptCacheRenewal, category string) {
	if renewal.ctx.Err() != nil || category == "sample_cancelled" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.promptCacheRenewals[renewal.parent.ID] != renewal || renewal.ctx.Err() != nil {
		return
	}
	if category == "" {
		delete(m.promptCacheFailures, renewal.parent.ID)
		return
	}
	if m.promptCacheFailures == nil {
		m.promptCacheFailures = make(map[string]string)
	}
	m.promptCacheFailures[renewal.parent.ID] = category
}

func isCodexSampleUnsupported(category string) bool {
	return category == "method_not_found" || category == "experimental_api_disabled" || category == "protocol_incompatible"
}

func codexSampleError(client *codexAppServerClient, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "sample_cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "sample_timeout"
	}
	select {
	case <-client.closed:
		return "connection_closed"
	default:
	}
	var rpcErr *codexAppServerErr
	if !errors.As(err, &rpcErr) {
		return "rpc_error"
	}
	message := strings.ToLower(rpcErr.Message)
	switch {
	case strings.Contains(message, "experimental") && (strings.Contains(message, "disabled") || strings.Contains(message, "requires") || strings.Contains(message, "not enabled")):
		return "experimental_api_disabled"
	case rpcErr.Code == -32601 || strings.Contains(message, "method not found") || strings.Contains(message, "unknown method") || strings.Contains(message, "unknown variant `thread/sample`"):
		return "method_not_found"
	case strings.Contains(message, "must be idle") || strings.Contains(message, "thread is busy"):
		return "parent_busy"
	case strings.Contains(message, "persistent root") || strings.Contains(message, "ephemeral"):
		return "parent_not_root"
	case strings.Contains(message, "latest finished turn") || strings.Contains(message, "sample source changed"):
		return "expected_turn_changed"
	case strings.Contains(message, "requires a loaded source") || strings.Contains(message, "thread not loaded") || strings.Contains(message, "thread not found"):
		return "parent_missing"
	case strings.Contains(message, "sample timed out"):
		return "sample_timeout"
	case strings.Contains(message, "sample canceled"):
		return "sample_cancelled"
	case strings.Contains(message, "sample turn requires client interaction"):
		return "sample_interaction_requested"
	case rpcErr.Code == -32602 || strings.Contains(message, "invalid param") || strings.Contains(message, "unknown field") ||
		strings.Contains(message, "missing field") || strings.Contains(message, "invalid type") ||
		strings.Contains(message, "unsupported") || strings.Contains(message, "requires expectedturnid"):
		return "protocol_incompatible"
	default:
		return "rpc_error"
	}
}

type codexSampleUsage struct {
	Input  *int64
	Cached *int64
	Output *int64
}

func decodeCodexSampleUsage(raw json.RawMessage) codexSampleUsage {
	result := decodeRawObject(raw)
	usage := decodeRawObject(result["usage"])
	if len(usage) == 0 {
		usage = decodeRawObject(decodeRawObject(result["response"])["usage"])
	}
	if len(usage) == 0 {
		usage = result
	}
	input := codexSampleTokenCount(usage, "input_tokens", "inputTokens")
	output := codexSampleTokenCount(usage, "output_tokens", "outputTokens")
	cached := codexSampleTokenCount(usage, "cached_input_tokens", "cachedInputTokens")
	if cached == nil {
		cached = codexSampleTokenCount(decodeRawObject(usage["input_tokens_details"]), "cached_tokens")
	}
	return codexSampleUsage{Input: input, Cached: cached, Output: output}
}

func codexSampleTokenCount(values map[string]any, keys ...string) *int64 {
	for _, key := range keys {
		if number, ok := values[key].(float64); ok && number >= 0 && number < 1<<53 && number == float64(int64(number)) {
			return ptr(int64(number))
		}
	}
	return nil
}
