package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
	"code-kanban/utils/process"

	"go.uber.org/zap"
)

type codexAppServerIncoming struct {
	ID     json.RawMessage    `json:"id,omitempty"`
	Method string             `json:"method,omitempty"`
	Params json.RawMessage    `json:"params,omitempty"`
	Result json.RawMessage    `json:"result,omitempty"`
	Error  *codexAppServerErr `json:"error,omitempty"`
}

type codexAppServerErr struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *codexAppServerErr) Error() string {
	if e == nil {
		return "codex app-server error"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("codex app-server error %d", e.Code)
}

type codexAppServerOutgoing struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params any             `json:"params,omitempty"`
	Result any             `json:"result,omitempty"`
}

type codexAppServerClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	writeMu   sync.Mutex
	pending   map[string]chan codexAppServerIncoming
	pendingMu sync.Mutex
	incoming  chan codexAppServerIncoming
	closed    chan struct{}
	closeErr  error
	closeMu   sync.Mutex
	seq       uint64
}

type pendingServerRequestKind string

const (
	pendingServerRequestUserInput           pendingServerRequestKind = "user_input"
	pendingServerRequestCommandApproval     pendingServerRequestKind = "command_approval"
	pendingServerRequestFileChangeApproval  pendingServerRequestKind = "file_change_approval"
	pendingServerRequestPermissionsApproval pendingServerRequestKind = "permissions_approval"
	pendingServerRequestPlanApproval        pendingServerRequestKind = "plan_approval"
)

type toolRequestOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type toolRequestQuestion struct {
	ID          string              `json:"id"`
	Header      string              `json:"header"`
	Question    string              `json:"question"`
	MultiSelect bool                `json:"multiSelect,omitempty"`
	IsOther     bool                `json:"isOther"`
	IsSecret    bool                `json:"isSecret"`
	Options     []toolRequestOption `json:"options,omitempty"`
}

type pendingServerRequest struct {
	RawID       json.RawMessage
	Kind        pendingServerRequestKind
	ItemID      string
	Prompt      string
	Command     string
	RequestedAt *time.Time
	Questions   []toolRequestQuestion
	Permissions map[string]any
}

func (r *pendingServerRequest) clone() *pendingServerRequest {
	if r == nil {
		return nil
	}
	clone := &pendingServerRequest{
		RawID:       append(json.RawMessage(nil), r.RawID...),
		Kind:        r.Kind,
		ItemID:      r.ItemID,
		Prompt:      r.Prompt,
		Command:     r.Command,
		Permissions: nil,
	}
	if r.RequestedAt != nil {
		requestedAt := *r.RequestedAt
		clone.RequestedAt = &requestedAt
	}
	if len(r.Questions) > 0 {
		clone.Questions = make([]toolRequestQuestion, 0, len(r.Questions))
		for _, question := range r.Questions {
			nextQuestion := question
			if len(question.Options) > 0 {
				nextQuestion.Options = append([]toolRequestOption(nil), question.Options...)
			}
			clone.Questions = append(clone.Questions, nextQuestion)
		}
	}
	if len(r.Permissions) > 0 {
		clone.Permissions = make(map[string]any, len(r.Permissions))
		for key, value := range r.Permissions {
			clone.Permissions[key] = value
		}
	}
	return clone
}

func (r *pendingServerRequest) isApproval() bool {
	if r == nil {
		return false
	}
	switch r.Kind {
	case pendingServerRequestCommandApproval, pendingServerRequestFileChangeApproval, pendingServerRequestPermissionsApproval, pendingServerRequestPlanApproval:
		return true
	default:
		return false
	}
}

type codexTurnOutcome int

const (
	codexTurnOutcomeNone codexTurnOutcome = iota
	codexTurnOutcomeCompleted
	codexTurnOutcomeFailed
)

type codexTurnScope struct {
	threadID string
	turnID   string
}

func (s codexTurnScope) contains(params json.RawMessage) bool {
	threadID := codexNotificationThreadID(params)
	if threadID != "" && threadID != strings.TrimSpace(s.threadID) {
		return false
	}
	turnID := codexNotificationTurnID(params)
	if turnID != "" && strings.TrimSpace(s.turnID) != "" && turnID != strings.TrimSpace(s.turnID) {
		return false
	}
	return true
}

const (
	codexTransportRetryingCode       = "transport_retrying"
	codexTransportRetryExhaustedCode = "transport_retry_exhausted"
	codexRuntimeErrorCode            = "runtime_error"
	codexIncompleteTurnErrorCode     = "incomplete_turn"
	codexRetryNoteLevel              = "warn"
	codexIncompleteTurnMaxRetries    = 1
	codexSteerTargetWait             = 10 * time.Second
)

var codexReconnectProgressPattern = regexp.MustCompile(`(?i)reconnecting\.\.\.\s*(\d+)\s*/\s*(\d+)`)

type codexTransportRetryInfo struct {
	Message     string
	Attempt     int
	MaxAttempts int
	RemoteURL   string
}

func startCodexAppServer(ctx context.Context, codexPath, cwd string) (*codexAppServerClient, io.Reader, error) {
	cmd := exec.CommandContext(ctx, codexPath, "app-server", "--listen", "stdio://")
	cmd.Dir = cwd
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	client := &codexAppServerClient{
		cmd:      cmd,
		stdin:    stdin,
		pending:  make(map[string]chan codexAppServerIncoming),
		incoming: make(chan codexAppServerIncoming, 64),
		closed:   make(chan struct{}),
	}
	go client.readLoop(stdout)
	return client, stderr, nil
}

func (c *codexAppServerClient) readLoop(stdout io.Reader) {
	defer close(c.incoming)
	defer close(c.closed)

	scanner := bufio.NewScanner(stdout)
	const maxLine = 1024 * 1024 * 8
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, maxLine)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var message codexAppServerIncoming
		if err := json.Unmarshal(line, &message); err != nil {
			c.setCloseErr(fmt.Errorf("failed to decode codex app-server message: %w", err))
			return
		}

		if key := appServerIDKey(message.ID); key != "" && message.Method == "" {
			c.pendingMu.Lock()
			responseCh := c.pending[key]
			if responseCh != nil {
				delete(c.pending, key)
			}
			c.pendingMu.Unlock()
			if responseCh != nil {
				responseCh <- message
				close(responseCh)
				continue
			}
		}

		c.incoming <- message
	}

	if err := scanner.Err(); err != nil {
		c.setCloseErr(err)
	}
}

func (c *codexAppServerClient) request(ctx context.Context, method string, params any) (codexAppServerIncoming, error) {
	requestID := fmt.Sprintf("codekanban_%d", atomic.AddUint64(&c.seq, 1))
	rawID, _ := json.Marshal(requestID)
	responseCh := make(chan codexAppServerIncoming, 1)

	c.pendingMu.Lock()
	c.pending[requestID] = responseCh
	c.pendingMu.Unlock()

	if err := c.writeMessage(codexAppServerOutgoing{
		ID:     rawID,
		Method: method,
		Params: params,
	}); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, requestID)
		c.pendingMu.Unlock()
		return codexAppServerIncoming{}, err
	}

	select {
	case response, ok := <-responseCh:
		if !ok {
			return codexAppServerIncoming{}, fmt.Errorf("codex app-server closed while waiting for %s", method)
		}
		if response.Error != nil {
			return codexAppServerIncoming{}, response.Error
		}
		return response, nil
	case <-c.closed:
		return codexAppServerIncoming{}, c.readErr()
	case <-ctx.Done():
		return codexAppServerIncoming{}, ctx.Err()
	}
}

func (c *codexAppServerClient) respond(rawID json.RawMessage, result any) error {
	if len(rawID) == 0 {
		return fmt.Errorf("codex app-server request id is missing")
	}
	return c.writeMessage(codexAppServerOutgoing{
		ID:     append(json.RawMessage(nil), rawID...),
		Result: result,
	})
}

func (c *codexAppServerClient) writeMessage(message codexAppServerOutgoing) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.stdin == nil {
		return fmt.Errorf("codex app-server stdin is closed")
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = c.stdin.Write(encoded)
	return err
}

func (c *codexAppServerClient) closeStdin() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.stdin == nil {
		return nil
	}
	err := c.stdin.Close()
	c.stdin = nil
	return err
}

func (c *codexAppServerClient) readErr() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closeErr != nil {
		return c.closeErr
	}
	return fmt.Errorf("codex app-server closed unexpectedly")
}

func (c *codexAppServerClient) setCloseErr(err error) {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closeErr == nil && err != nil {
		c.closeErr = err
	}
}

func killCmdTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := int32(cmd.Process.Pid)
	if pid <= 0 {
		return
	}
	if err := process.KillProcessTree(pid); err != nil {
		_ = cmd.Process.Kill()
	}
}

func appServerIDKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err == nil {
		return fmt.Sprintf("%d", value)
	}
	return strings.TrimSpace(string(raw))
}

func (m *Manager) runCodexAppServerSession(
	ctx context.Context,
	run *activeRun,
	session tables.WebSessionTable,
	text string,
	attachments []Attachment,
) {
	client, stderr, err := startCodexAppServer(ctx, m.cfg.CodexPath, session.Cwd)
	if err != nil {
		run.resolveBootstrap(err)
		m.handleRunFailure(session.ID, session, run, err)
		return
	}
	run.setCodexAppServer(client)
	run.setCommand(client.cmd)

	stderrBuffer := bytes.NewBuffer(nil)
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(stderrBuffer, stderr)
	}()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- client.cmd.Wait()
	}()

	if _, err := client.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "codekanban-web-session",
			"version": "0.0.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}); err != nil {
		run.resolveBootstrap(err)
		m.waitAndFailCodexAppServer(session, run, client, waitCh, stderrDone, stderrBuffer, err)
		return
	}

	threadID, modelProvider, err := m.startOrResumeCodexThread(ctx, session, run, client)
	if err != nil {
		run.resolveBootstrap(err)
		m.waitAndFailCodexAppServer(session, run, client, waitCh, stderrDone, stderrBuffer, err)
		return
	}
	run.transportRemoteURL = readCodexRemoteURL(ctx, client, session.Cwd, modelProvider)

	turnResponse, err := client.request(ctx, "turn/start", codexTurnStartParams(session, threadID, text, attachments))
	if err != nil {
		run.resolveBootstrap(err)
		m.waitAndFailCodexAppServer(session, run, client, waitCh, stderrDone, stderrBuffer, err)
		return
	}
	turnID := parseCodexTurnID(turnResponse.Result)
	if turnID != "" {
		run.currentToolMessage = turnID
	}
	run.setCodexSteerTarget(threadID, turnID)
	rootScope := codexTurnScope{threadID: threadID, turnID: turnID}
	if strings.TrimSpace(run.bootstrapGoalObjective) != "" {
		if _, err := client.request(ctx, "thread/goal/set", map[string]any{
			"threadId":  threadID,
			"objective": strings.TrimSpace(run.bootstrapGoalObjective),
			"status":    string(run.bootstrapGoalState),
		}); err != nil {
			run.resolveBootstrap(err)
			m.waitAndFailCodexAppServer(session, run, client, waitCh, stderrDone, stderrBuffer, err)
			return
		}
		if err := m.syncCodexGoalState(ctx, session, client, threadID); err != nil {
			run.resolveBootstrap(err)
			m.waitAndFailCodexAppServer(session, run, client, waitCh, stderrDone, stderrBuffer, err)
			return
		}
		run.resolveBootstrap(nil)
	} else {
		run.resolveBootstrap(nil)
	}

	incoming := client.incoming
	var waitErr error
	processExited := false
	turnCompleted := false
	cancelled := false
	incompleteTurnRetries := 0

	for !processExited || incoming != nil {
		select {
		case <-ctx.Done():
			if !cancelled {
				cancelled = true
				_ = client.closeStdin()
				killCmdTree(client.cmd)
			}
		case message, ok := <-incoming:
			if !ok {
				incoming = nil
				continue
			}
			outcome, err := m.handleCodexAppServerMessage(session, run, client, rootScope, message)
			if err != nil {
				run.lastError = err.Error()
				if outcome == codexTurnOutcomeFailed {
					killCmdTree(client.cmd)
				}
				continue
			}
			if outcome == codexTurnOutcomeCompleted && !turnCompleted {
				if shouldContinueIncompleteCodexTurn(session, run) {
					if incompleteTurnRetries >= codexIncompleteTurnMaxRetries {
						run.lastErrorCode = codexIncompleteTurnErrorCode
						run.lastError = "Codex completed the turn without a final answer after one automatic continuation."
						killCmdTree(client.cmd)
						continue
					}

					incompleteTurnRetries++
					m.appendRunNote(
						session.ID,
						session,
						run,
						"warn",
						"Codex ended the turn without a final answer. Continuing automatically (1/1).",
						map[string]any{
							"code":        "incomplete_turn_auto_continue",
							"attempt":     incompleteTurnRetries,
							"maxAttempts": codexIncompleteTurnMaxRetries,
						},
					)
					run.resetCodexTurnCompletionEvidence()
					turnResponse, continueErr := client.request(
						ctx,
						"turn/start",
						codexTurnStartParams(session, threadID, incompleteTurnContinuationPrompt, nil),
					)
					if continueErr != nil {
						run.lastErrorCode = codexIncompleteTurnErrorCode
						run.lastError = fmt.Sprintf(
							"Codex ended the turn without a final answer and the automatic continuation could not start: %v",
							continueErr,
						)
						killCmdTree(client.cmd)
						continue
					}
					continuedTurnID := parseCodexTurnID(turnResponse.Result)
					if strings.TrimSpace(continuedTurnID) == "" {
						run.lastErrorCode = codexIncompleteTurnErrorCode
						run.lastError = "Codex ended the turn without a final answer and the automatic continuation did not return a turn id."
						killCmdTree(client.cmd)
						continue
					}
					run.currentToolMessage = continuedTurnID
					run.setCodexSteerTarget(threadID, continuedTurnID)
					rootScope = codexTurnScope{threadID: threadID, turnID: continuedTurnID}
					continue
				}

				turnCompleted = true
				finalStatus, finalAssistantState := m.completedRunState(context.Background(), session, run)
				now := time.Now()
				_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
					ID:        utils.NewID(),
					Seq:       0,
					Type:      "run_done",
					RunID:     run.runID,
					Timestamp: now,
					Payload: map[string]any{
						"ok": true,
						"st": string(finalStatus),
					},
				})
				_ = m.updateRuntimeState(
					context.Background(),
					session.ID,
					applyAssistantStateUpdates(map[string]any{
						"status":     string(finalStatus),
						"updated_at": now,
					}, finalAssistantState, now),
				)
				m.broadcastSessionSummary(context.Background(), session.ID)
				_ = client.closeStdin()
				m.maybeSyncSessionAfterRun(session)
				run.resetActiveCallTracking()
				m.releaseActiveRun(session.ID, run)
			}
		case waitErr = <-waitCh:
			processExited = true
			waitCh = nil
		}
	}

	<-stderrDone

	if ctx.Err() != nil {
		abortPayload := activeCallTimeoutAbortPayload(session, run.abortEventPayload())
		now := time.Now()
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "run_abort",
			RunID:     run.runID,
			Timestamp: now,
			Payload:   abortPayload,
		})
		_ = m.updateRuntimeState(
			context.Background(),
			session.ID,
			applyAssistantStateUpdates(map[string]any{
				"status":                     string(StatusIdle),
				"updated_at":                 now,
				"auto_retry_attempt":         0,
				"auto_retry_next_at":         nil,
				"auto_retry_last_error_code": nil,
			}, AssistantStateNone, now),
		)
		m.cancelAutoRetryTimer(session.ID)
		m.broadcastSessionSummary(context.Background(), session.ID)
		return
	}

	if turnCompleted {
		return
	}

	message := strings.TrimSpace(run.lastError)
	if message == "" {
		message = strings.TrimSpace(stderrBuffer.String())
	}
	if message == "" && waitErr != nil {
		message = waitErr.Error()
	}
	if message == "" {
		message = client.readErr().Error()
	}
	code := strings.TrimSpace(run.lastErrorCode)
	if code == "" && run.transportRetrySeen && isLikelyCodexTransportFailureMessage(message) {
		code = codexTransportRetryExhaustedCode
	}
	m.handleRunFailureWithCode(session.ID, session, run, code, fmt.Errorf("%s", message))
}

func (m *Manager) waitAndFailCodexAppServer(
	session tables.WebSessionTable,
	run *activeRun,
	client *codexAppServerClient,
	waitCh chan error,
	stderrDone chan struct{},
	stderrBuffer *bytes.Buffer,
	cause error,
) {
	_ = client.closeStdin()
	killCmdTree(client.cmd)
	<-waitCh
	<-stderrDone

	message := strings.TrimSpace(cause.Error())
	if message == "" {
		message = strings.TrimSpace(stderrBuffer.String())
	}
	if message == "" {
		message = "codex app-server setup failed"
	}
	m.handleRunFailureWithCode(session.ID, session, run, codexRuntimeErrorCode, fmt.Errorf("%s", message))
}

func (m *Manager) startOrResumeCodexThread(
	ctx context.Context,
	session tables.WebSessionTable,
	run *activeRun,
	client *codexAppServerClient,
) (string, string, error) {
	existingThreadID := ""
	if session.NativeSessionID != nil {
		existingThreadID = strings.TrimSpace(*session.NativeSessionID)
	}

	var (
		response codexAppServerIncoming
		err      error
	)
	if existingThreadID != "" {
		response, err = client.request(ctx, "thread/resume", codexThreadResumeParams(session, existingThreadID))
	} else {
		response, err = client.request(ctx, "thread/start", codexThreadStartParams(session))
	}
	if err != nil {
		return "", "", err
	}

	responsePayload := decodeRawObject(response.Result)
	threadID := parseCodexThreadID(response.Result)
	if threadID == "" {
		threadID = existingThreadID
	}
	if threadID == "" {
		return "", "", fmt.Errorf("codex app-server did not return a thread id")
	}
	if err := m.updateRuntimeState(context.Background(), session.ID, map[string]any{
		"native_session_id": threadID,
		"updated_at":        time.Now(),
	}); err != nil {
		return "", "", err
	}
	run.currentToolMessage = threadID
	return threadID, strings.TrimSpace(stringValue(responsePayload["modelProvider"])), nil
}

func readCodexRemoteURL(
	ctx context.Context,
	client *codexAppServerClient,
	cwd string,
	activeProvider string,
) string {
	if client == nil {
		return ""
	}
	response, err := client.request(ctx, "config/read", map[string]any{
		"cwd":           strings.TrimSpace(cwd),
		"includeLayers": false,
	})
	if err != nil {
		return ""
	}
	return parseCodexRemoteURL(response.Result, activeProvider)
}

func parseCodexRemoteURL(raw json.RawMessage, activeProvider string) string {
	result := decodeRawObject(raw)
	config := decodeRawObject(result["config"])
	providerName := firstNonEmpty(
		strings.TrimSpace(activeProvider),
		strings.TrimSpace(stringValue(config["model_provider"])),
	)
	if providerName == "" {
		return ""
	}

	providers := decodeRawObject(config["model_providers"])
	provider := decodeRawObject(providers[providerName])
	if len(provider) == 0 {
		for name, candidate := range providers {
			if strings.EqualFold(strings.TrimSpace(name), providerName) {
				provider = decodeRawObject(candidate)
				break
			}
		}
	}
	return sanitizeCodexRemoteURL(stringValue(provider["base_url"]))
}

func sanitizeCodexRemoteURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}

func (m *Manager) handleCodexAppServerMessage(
	session tables.WebSessionTable,
	run *activeRun,
	client *codexAppServerClient,
	rootScope codexTurnScope,
	message codexAppServerIncoming,
) (codexTurnOutcome, error) {
	isRootEvent := rootScope.contains(message.Params)
	switch strings.TrimSpace(message.Method) {
	case "":
		return codexTurnOutcomeNone, nil
	case "thread/started":
		if !isRootEvent {
			return codexTurnOutcomeNone, nil
		}
		if threadID := parseCodexThreadID(message.Params); threadID != "" {
			_ = m.updateRuntimeState(context.Background(), session.ID, map[string]any{
				"native_session_id": threadID,
				"updated_at":        time.Now(),
			})
		}
		return codexTurnOutcomeNone, nil
	case "turn/started":
		if !isRootEvent {
			return codexTurnOutcomeNone, nil
		}
		if turnID := parseCodexTurnID(message.Params); turnID != "" {
			run.currentToolMessage = turnID
			run.setCodexSteerTarget(codexNotificationThreadID(message.Params), turnID)
		}
		return codexTurnOutcomeNone, nil
	case "item/started":
		m.handleCodexAppServerItemStarted(session, run, message.Params, isRootEvent)
		return codexTurnOutcomeNone, nil
	case "item/agentMessage/delta":
		m.handleCodexAppServerAgentDelta(session, run, message.Params, isRootEvent)
		return codexTurnOutcomeNone, nil
	case "item/completed":
		m.handleCodexAppServerItemCompleted(session, run, message.Params, isRootEvent)
		return codexTurnOutcomeNone, nil
	case "thread/tokenUsage/updated":
		if isRootEvent {
			m.handleCodexAppServerUsage(session, run, message.Params)
		}
		return codexTurnOutcomeNone, nil
	case "thread/goal/updated":
		if !isRootEvent {
			return codexTurnOutcomeNone, nil
		}
		if err := m.handleCodexAppServerGoalUpdated(session, message.Params); err != nil {
			return codexTurnOutcomeFailed, err
		}
		return codexTurnOutcomeNone, nil
	case "thread/goal/cleared":
		if !isRootEvent {
			return codexTurnOutcomeNone, nil
		}
		if err := m.handleCodexAppServerGoalCleared(session); err != nil {
			return codexTurnOutcomeFailed, err
		}
		return codexTurnOutcomeNone, nil
	case "error":
		if !isRootEvent {
			return codexTurnOutcomeNone, nil
		}
		run.lastError, run.lastErrorCode = parseCodexTurnError(message.Params)
		if retryInfo, ok := classifyCodexTransportRetryMessage(run.lastError); ok {
			retryInfo = retryInfo.withRemoteURL(run.transportRemoteURL)
			run.transportRetrySeen = true
			m.appendRunNote(session.ID, session, run, codexRetryNoteLevel, retryInfo.Message, retryInfo.payload())
			run.lastError = ""
			run.lastErrorCode = ""
			return codexTurnOutcomeNone, nil
		}
		if run.transportRetrySeen {
			run.lastErrorCode = codexTransportRetryExhaustedCode
		} else if strings.TrimSpace(run.lastErrorCode) == "" {
			run.lastErrorCode = codexRuntimeErrorCode
		}
		return codexTurnOutcomeFailed, fmt.Errorf("%s", firstNonEmpty(run.lastError, "codex app-server turn failed"))
	case "turn/completed":
		if !isRootEvent {
			return codexTurnOutcomeNone, nil
		}
		status, errMessage, errCode := parseCodexTurnCompletion(message.Params)
		_ = m.finalizeLatestTurnUsage(context.Background(), session.ID)
		if status == "completed" {
			return codexTurnOutcomeCompleted, nil
		}
		if errMessage != "" {
			run.lastError = errMessage
		}
		if run.transportRetrySeen {
			run.lastErrorCode = codexTransportRetryExhaustedCode
		} else if errCode != "" {
			run.lastErrorCode = errCode
		} else {
			run.lastErrorCode = codexRuntimeErrorCode
		}
		return codexTurnOutcomeFailed, fmt.Errorf("%s", firstNonEmpty(run.lastError, "codex app-server turn failed"))
	case "item/tool/requestUserInput":
		if err := m.handleCodexAppServerUserInputRequest(session, run, message); err != nil {
			return codexTurnOutcomeFailed, err
		}
		return codexTurnOutcomeNone, nil
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval":
		if err := m.handleCodexAppServerApprovalRequest(session, run, message); err != nil {
			return codexTurnOutcomeFailed, err
		}
		return codexTurnOutcomeNone, nil
	case "configWarning", "account/rateLimits/updated", "serverRequest/resolved", "thread/status/changed",
		"item/plan/delta", "turn/plan/updated", "item/commandExecution/outputDelta",
		"item/fileChange/outputDelta", "item/reasoning/summaryTextDelta",
		"item/reasoning/summaryPartAdded", "item/reasoning/textDelta", "rawResponseItem/completed":
		return codexTurnOutcomeNone, nil
	default:
		return codexTurnOutcomeNone, nil
	}
}

func (m *Manager) steerActiveCodexTurn(
	ctx context.Context,
	session tables.WebSessionTable,
	text string,
	attachmentIDs []string,
) (bool, error) {
	m.mu.RLock()
	run := m.runs[session.ID]
	m.mu.RUnlock()
	if run == nil || normalizeAgent(run.agent) != AgentCodex || run.backend != SessionBackendCodexAppServer {
		return false, nil
	}

	attachments := make([]Attachment, 0, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		attachment, err := m.loadAttachment(strings.TrimSpace(attachmentID))
		if err != nil {
			return true, fmt.Errorf("attachment %s not found", attachmentID)
		}
		attachments = append(attachments, attachment)
	}
	text = strings.TrimSpace(text)
	if text == "" && len(attachments) == 0 {
		return true, fmt.Errorf("message is empty")
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(codexSteerTargetWait)
	defer timer.Stop()

	var client *codexAppServerClient
	var threadID string
	var turnID string
	for client == nil || threadID == "" || turnID == "" {
		client, threadID, turnID = run.codexSteerTarget()
		if client != nil && threadID != "" && turnID != "" {
			break
		}
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case <-run.done:
			return false, nil
		case <-timer.C:
			return true, fmt.Errorf("active Codex turn is not ready to steer")
		case <-ticker.C:
		}
	}

	messageID := utils.NewID()
	response, err := client.request(ctx, "turn/steer", map[string]any{
		"threadId":            threadID,
		"expectedTurnId":      turnID,
		"input":               codexUserInputs(text, attachments),
		"clientUserMessageId": messageID,
	})
	if err != nil {
		return true, err
	}
	responseTurnID := strings.TrimSpace(stringValue(decodeRawObject(response.Result)["turnId"]))
	if responseTurnID != "" && responseTurnID != turnID {
		return true, fmt.Errorf("Codex steered unexpected turn %s", responseTurnID)
	}

	if _, err := m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Type:      "msg_u",
		RunID:     run.runID,
		ParentID:  messageID,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"mid":  messageID,
			"txt":  text,
			"atts": attachmentPayloads(attachments),
		},
	}); err != nil && m.logger != nil {
		m.logger.Error("failed to persist Codex steer message",
			zap.String("sessionId", session.ID),
			zap.String("turnId", turnID),
			zap.Error(err),
		)
	}
	m.broadcastPendingInputs(session.ID)
	return true, nil
}

func classifyCodexTransportRetryMessage(message string) (codexTransportRetryInfo, bool) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return codexTransportRetryInfo{}, false
	}
	lower := strings.ToLower(trimmed)
	if !codexReconnectProgressPattern.MatchString(trimmed) &&
		!strings.Contains(lower, "retrying sampling request") &&
		!strings.Contains(lower, "falling back from websockets to https transport") {
		return codexTransportRetryInfo{}, false
	}
	info := codexTransportRetryInfo{Message: trimmed}
	matches := codexReconnectProgressPattern.FindStringSubmatch(trimmed)
	if len(matches) == 3 {
		if attempt, err := strconv.Atoi(matches[1]); err == nil && attempt > 0 {
			info.Attempt = attempt
		}
		if maxAttempts, err := strconv.Atoi(matches[2]); err == nil && maxAttempts > 0 {
			info.MaxAttempts = maxAttempts
		}
	}
	return info, true
}

func isLikelyCodexTransportFailureMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "unexpected status 502") ||
		strings.Contains(lower, "upstream service temporarily unavailable") ||
		strings.Contains(lower, "bad gateway") ||
		strings.Contains(lower, "retry limit") ||
		strings.Contains(lower, "transport error") ||
		strings.Contains(lower, "connection failed") ||
		strings.Contains(lower, "websocket")
}

func (i codexTransportRetryInfo) payload() map[string]any {
	payload := map[string]any{
		"code": codexTransportRetryingCode,
	}
	if i.Attempt > 0 {
		payload["attempt"] = i.Attempt
	}
	if i.MaxAttempts > 0 {
		payload["maxAttempts"] = i.MaxAttempts
	}
	if strings.TrimSpace(i.RemoteURL) != "" {
		payload["remoteUrl"] = strings.TrimSpace(i.RemoteURL)
	}
	return payload
}

func (i codexTransportRetryInfo) withRemoteURL(remoteURL string) codexTransportRetryInfo {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return i
	}
	i.RemoteURL = remoteURL
	if !strings.Contains(i.Message, remoteURL) {
		i.Message = strings.TrimSpace(i.Message) + "\n" + remoteURL
	}
	return i
}

func (m *Manager) handleCodexAppServerItemStarted(
	session tables.WebSessionTable,
	run *activeRun,
	params json.RawMessage,
	isRootEvent bool,
) {
	payload := decodeRawObject(params)
	item := decodeRawObject(payload["item"])
	itemType := normalizeCodexItemType(stringValue(item["type"]))
	switch itemType {
	case "user_message":
		return
	case "agent_message":
		messageID := firstNonEmpty(stringValue(item["id"]), utils.NewID())
		if isRootEvent {
			run.assistantMessageID = messageID
			run.recordAssistantMessageStarted(messageID, stringValue(item["phase"]))
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "msg_a_st",
			RunID:     run.runID,
			ParentID:  messageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"mid": messageID,
			},
		})
	default:
		toolName := codexToolName(item)
		toolInput := codexToolInput(item)
		toolMeta := codexToolMeta(item)
		toolID := firstNonEmpty(stringValue(item["id"]), utils.NewID())
		parentID := ""
		if isRootEvent {
			parentID = run.assistantMessageID
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "tool_st",
			RunID:     run.runID,
			ParentID:  parentID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"tid":  toolID,
				"name": toolName,
				"kind": itemType,
				"in":   toolInput,
				"meta": toolMeta,
			},
		})
		if isRootEvent {
			m.trackActiveCodexToolStart(run, toolID, itemType, toolName, toolInput, toolMeta)
		}
	}
}

func (m *Manager) handleCodexAppServerAgentDelta(
	session tables.WebSessionTable,
	run *activeRun,
	params json.RawMessage,
	isRootEvent bool,
) {
	payload := decodeRawObject(params)
	fallbackMessageID := ""
	if isRootEvent {
		fallbackMessageID = run.assistantMessageID
	}
	messageID := firstNonEmpty(stringValue(payload["itemId"]), fallbackMessageID, utils.NewID())
	if isRootEvent && run.assistantMessageID != messageID {
		run.assistantMessageID = messageID
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "msg_a_st",
			RunID:     run.runID,
			ParentID:  messageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"mid": messageID,
			},
		})
	}
	if isRootEvent {
		run.recordAssistantMessageDelta(messageID, stringValue(payload["delta"]))
	}

	run.markAssistantDeltaSeen(messageID)
	if err := m.enqueueTextDelta(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "txt_d",
		RunID:     run.runID,
		ParentID:  messageID,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"mid": messageID,
			"txt": stringValue(payload["delta"]),
		},
	}); err != nil && m.logger != nil {
		m.logger.Error(
			"failed to enqueue codex app-server text delta",
			zap.String("sessionId", session.ID),
			zap.String("runId", run.runID),
			zap.String("messageId", messageID),
			zap.Error(err),
		)
	}
}

func (m *Manager) handleCodexAppServerItemCompleted(
	session tables.WebSessionTable,
	run *activeRun,
	params json.RawMessage,
	isRootEvent bool,
) {
	payload := decodeRawObject(params)
	item := decodeRawObject(payload["item"])
	itemType := normalizeCodexItemType(stringValue(item["type"]))
	switch itemType {
	case "user_message":
		return
	case "agent_message":
		fallbackMessageID := ""
		if isRootEvent {
			fallbackMessageID = run.assistantMessageID
		}
		messageID := firstNonEmpty(stringValue(item["id"]), fallbackMessageID, utils.NewID())
		if isRootEvent {
			run.recordAssistantMessageCompleted(
				messageID,
				stringValue(item["phase"]),
				stringValue(item["text"]),
			)
		}
		if !run.assistantDeltaWasSeen(messageID) {
			text := stringValue(item["text"])
			if strings.TrimSpace(text) != "" {
				_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
					ID:        utils.NewID(),
					Seq:       0,
					Type:      "txt_d",
					RunID:     run.runID,
					ParentID:  messageID,
					Timestamp: time.Now(),
					Payload: map[string]any{
						"mid": messageID,
						"txt": text,
					},
				})
			}
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "txt_end",
			RunID:     run.runID,
			ParentID:  messageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"mid": messageID,
			},
		})
	default:
		toolSucceeded := codexToolSucceeded(item)
		if isRootEvent && toolSucceeded && codexToolIsPlan(item) {
			run.markCompletedPlanTool()
		}
		if isRootEvent && toolSucceeded && itemType == "context_compaction" {
			record, err := m.GetSession(context.Background(), session.ID)
			if err == nil {
				_ = m.updateRuntimeState(
					context.Background(),
					session.ID,
					contextEstimateBaselineResetUpdate(record, time.Now()),
				)
			}
		}
		toolID := firstNonEmpty(stringValue(item["id"]), utils.NewID())
		parentID := ""
		if isRootEvent {
			parentID = run.assistantMessageID
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "tool_end",
			RunID:     run.runID,
			ParentID:  parentID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"tid":  toolID,
				"kind": itemType,
				"out":  codexToolOutput(item),
				"ok":   toolSucceeded,
				"meta": codexToolMeta(item),
			},
		})
		if isRootEvent {
			m.trackActiveCodexToolComplete(run, toolID)
		}
	}
}

func shouldContinueIncompleteCodexTurn(session tables.WebSessionTable, run *activeRun) bool {
	if run == nil || !isGPT56CodexModel(session.Model) {
		return false
	}
	if run.completedReplySeen() || run.completedPlanToolSeen() || run.hasPendingServerRequest() {
		return false
	}
	return true
}

func isGPT56CodexModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra":
		return true
	default:
		return false
	}
}

func (m *Manager) handleCodexAppServerUsage(
	session tables.WebSessionTable,
	run *activeRun,
	params json.RawMessage,
) {
	payload := decodeRawObject(params)
	tokenUsage := decodeRawObject(payload["tokenUsage"])
	total, _ := parseCodexTokenUsageSnapshot(tokenUsage["total"])
	last, hasLast := parseCodexTokenUsageSnapshot(tokenUsage["last"])
	now := time.Now()
	updates := map[string]any{
		"total_input_tokens":                     total.InputTokens,
		"total_cached_input_tokens":              total.CachedInputTokens,
		"total_output_tokens":                    total.OutputTokens,
		"latest_token_count_input_tokens":        0,
		"latest_token_count_cached_input_tokens": 0,
		"latest_token_count_output_tokens":       0,
		"latest_token_count_total_tokens":        0,
		"latest_token_count_updated_at":          nil,
		"updated_at":                             now,
	}
	if hasLast {
		updates["latest_token_count_input_tokens"] = last.InputTokens
		updates["latest_token_count_cached_input_tokens"] = last.CachedInputTokens
		updates["latest_token_count_output_tokens"] = last.OutputTokens
		updates["latest_token_count_total_tokens"] = last.TotalTokens
		updates["latest_token_count_updated_at"] = now
	}
	contextWindow, hasContextWindow := codexInt64Field(
		tokenUsage,
		"modelContextWindow",
		"model_context_window",
	)
	if hasContextWindow && contextWindow > 0 {
		updates["session_context_window_tokens"] = contextWindow
		updates["session_context_window_observed_at"] = now
	}
	_ = m.updateRuntimeState(context.Background(), session.ID, updates)

	eventPayload := map[string]any{
		"in":  total.InputTokens,
		"cin": total.CachedInputTokens,
		"out": total.OutputTokens,
	}
	if contextWindow > 0 {
		eventPayload["cwt"] = contextWindow
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "usage",
		RunID:     run.runID,
		Timestamp: now,
		Payload:   eventPayload,
	})
	if contextWindow > 0 {
		m.broadcastSessionSummary(context.Background(), session.ID)
	}
}

func applySessionGoalUpdates(updates map[string]any, goal *SessionGoal) {
	if updates == nil {
		return
	}
	if goal == nil {
		updates["goal_objective"] = nil
		updates["goal_status"] = nil
		updates["goal_token_budget"] = nil
		updates["goal_tokens_used"] = int64(0)
		updates["goal_time_used_seconds"] = int64(0)
		updates["goal_created_at"] = nil
		updates["goal_updated_at"] = nil
		return
	}
	updates["goal_objective"] = goal.Objective
	updates["goal_status"] = string(goal.Status)
	updates["goal_token_budget"] = goal.TokenBudget
	updates["goal_tokens_used"] = goal.TokensUsed
	updates["goal_time_used_seconds"] = goal.TimeUsedSeconds
	updates["goal_created_at"] = goal.CreatedAt
	updates["goal_updated_at"] = goal.UpdatedAt
}

func (m *Manager) loadCodexGoal(
	ctx context.Context,
	session tables.WebSessionTable,
	client *codexAppServerClient,
	threadID string,
) (*SessionGoal, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, nil
	}
	if client != nil {
		response, err := client.request(ctx, "thread/goal/get", map[string]any{
			"threadId": threadID,
		})
		if err != nil {
			return nil, err
		}
		payload := decodeRawObject(response.Result)
		return parseCodexSessionGoal(payload["goal"], threadID), nil
	}

	var goal *SessionGoal
	err := m.withCodexQueryClient(ctx, session.Cwd, func(queryClient *codexAppServerClient) error {
		response, err := queryClient.request(ctx, "thread/goal/get", map[string]any{
			"threadId": threadID,
		})
		if err != nil {
			return err
		}
		payload := decodeRawObject(response.Result)
		goal = parseCodexSessionGoal(payload["goal"], threadID)
		return nil
	})
	return goal, err
}

func (m *Manager) syncCodexGoalState(
	ctx context.Context,
	session tables.WebSessionTable,
	client *codexAppServerClient,
	threadID string,
) error {
	goal, err := m.loadCodexGoal(ctx, session, client, threadID)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	applySessionGoalUpdates(updates, goal)
	return m.updateRuntimeState(ctx, session.ID, updates)
}

func (m *Manager) handleCodexAppServerGoalUpdated(
	session tables.WebSessionTable,
	params json.RawMessage,
) error {
	payload := decodeRawObject(params)
	threadID := strings.TrimSpace(firstNonEmpty(stringValue(payload["threadId"]), func() string {
		if session.NativeSessionID == nil {
			return ""
		}
		return *session.NativeSessionID
	}()))
	goal := parseCodexSessionGoal(payload["goal"], threadID)
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	applySessionGoalUpdates(updates, goal)
	if err := m.updateRuntimeState(context.Background(), session.ID, updates); err != nil {
		return err
	}
	m.broadcastSessionSummary(context.Background(), session.ID)
	return nil
}

func (m *Manager) handleCodexAppServerGoalCleared(session tables.WebSessionTable) error {
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	applySessionGoalUpdates(updates, nil)
	if err := m.updateRuntimeState(context.Background(), session.ID, updates); err != nil {
		return err
	}
	m.broadcastSessionSummary(context.Background(), session.ID)
	return nil
}

func (m *Manager) handleCodexAppServerUserInputRequest(
	session tables.WebSessionTable,
	run *activeRun,
	message codexAppServerIncoming,
) error {
	payload := decodeRawObject(message.Params)
	itemID := stringValue(payload["itemId"])
	questions := decodeToolQuestions(payload["questions"])
	request := &pendingServerRequest{
		RawID:     append(json.RawMessage(nil), message.ID...),
		Kind:      pendingServerRequestUserInput,
		ItemID:    itemID,
		Prompt:    summarizeToolQuestions(questions),
		Questions: questions,
	}
	run.setPendingServerRequest(request)
	m.pauseActiveCallTimeout(run)
	now := time.Now()
	_, err := m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "user_input_req",
		RunID:     run.runID,
		ParentID:  run.assistantMessageID,
		Timestamp: now,
		Payload: map[string]any{
			"iid": itemID,
			"txt": request.Prompt,
			"qs":  questions,
		},
	})
	if err == nil {
		_ = m.updateRuntimeState(
			context.Background(),
			session.ID,
			applyAssistantStateUpdates(map[string]any{
				"updated_at": now,
			}, AssistantStateWaitingInput, now),
		)
		m.broadcastSessionSummary(context.Background(), session.ID)
	}
	return err
}

func (m *Manager) handleCodexAppServerApprovalRequest(
	session tables.WebSessionTable,
	run *activeRun,
	message codexAppServerIncoming,
) error {
	request := decodePendingApprovalRequest(message)
	now := time.Now()
	request.RequestedAt = ptr(now)
	run.setPendingServerRequest(request)
	m.pauseActiveCallTimeout(run)
	_, err := m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "approval_req",
		RunID:     run.runID,
		ParentID:  run.assistantMessageID,
		Timestamp: now,
		Payload: map[string]any{
			"iid":     request.ItemID,
			"kind":    string(request.Kind),
			"prompt":  request.Prompt,
			"command": request.Command,
		},
	})
	if err == nil {
		_ = m.updateRuntimeState(
			context.Background(),
			session.ID,
			applyAssistantStateUpdates(map[string]any{
				"updated_at": now,
			}, AssistantStateWaitingApproval, now),
		)
		m.broadcastSessionSummary(context.Background(), session.ID)
	}
	return err
}

func decodePendingApprovalRequest(message codexAppServerIncoming) *pendingServerRequest {
	payload := decodeRawObject(message.Params)
	itemID := stringValue(payload["itemId"])

	request := &pendingServerRequest{
		RawID:   append(json.RawMessage(nil), message.ID...),
		ItemID:  itemID,
		Command: stringValue(payload["command"]),
	}

	switch message.Method {
	case "item/commandExecution/requestApproval":
		request.Kind = pendingServerRequestCommandApproval
	case "item/fileChange/requestApproval":
		request.Kind = pendingServerRequestFileChangeApproval
	case "item/permissions/requestApproval":
		request.Kind = pendingServerRequestPermissionsApproval
		request.Permissions = decodeRawObject(payload["permissions"])
	}
	request.Prompt = firstNonEmpty(
		stringValue(payload["reason"]),
		stringValue(payload["grantRoot"]),
		approvalPromptFallback(request.Kind),
	)
	return request
}

func approvalPromptFallback(kind pendingServerRequestKind) string {
	switch kind {
	case pendingServerRequestCommandApproval:
		return "Codex is waiting for approval to run this command."
	case pendingServerRequestFileChangeApproval:
		return "Codex is waiting for approval to apply file changes."
	case pendingServerRequestPermissionsApproval:
		return "Codex is waiting for approval to change permissions."
	default:
		return "Codex is waiting for approval before continuing."
	}
}

func approvalResponsePayload(request *pendingServerRequest, action string) any {
	if request == nil {
		return map[string]any{}
	}
	switch request.Kind {
	case pendingServerRequestCommandApproval:
		return map[string]any{
			"decision": approvalDecisionValue(action),
		}
	case pendingServerRequestFileChangeApproval:
		return map[string]any{
			"decision": approvalDecisionValue(action),
		}
	case pendingServerRequestPermissionsApproval:
		permissions := map[string]any{}
		if action != "reject" && len(request.Permissions) > 0 {
			permissions = request.Permissions
		}
		return map[string]any{
			"permissions": permissions,
			"scope":       "turn",
		}
	default:
		return map[string]any{}
	}
}

func approvalDecisionValue(action string) string {
	if action == "reject" {
		return "decline"
	}
	return "accept"
}

func userInputResponsePayload(answers map[string][]string) map[string]any {
	response := map[string]any{
		"answers": map[string]any{},
	}
	answerPayload := response["answers"].(map[string]any)
	for questionID, values := range answers {
		if strings.TrimSpace(questionID) == "" {
			continue
		}
		normalized := make([]string, 0, len(values))
		for _, value := range values {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				normalized = append(normalized, trimmed)
			}
		}
		answerPayload[questionID] = map[string]any{
			"answers": normalized,
		}
	}
	return response
}

func codexThreadStartParams(session tables.WebSessionTable) map[string]any {
	return map[string]any{
		"cwd":                    session.Cwd,
		"model":                  strings.TrimSpace(session.Model),
		"sandbox":                codexSandboxMode(effectivePermissionLevel(session)),
		"approvalPolicy":         codexApprovalPolicy(effectivePermissionLevel(session)),
		"persistExtendedHistory": true,
	}
}

func codexThreadResumeParams(session tables.WebSessionTable, threadID string) map[string]any {
	return map[string]any{
		"threadId":               strings.TrimSpace(threadID),
		"persistExtendedHistory": true,
		"cwd":                    session.Cwd,
		"model":                  strings.TrimSpace(session.Model),
		"sandbox":                codexSandboxMode(effectivePermissionLevel(session)),
		"approvalPolicy":         codexApprovalPolicy(effectivePermissionLevel(session)),
	}
}

func codexTurnStartParams(
	session tables.WebSessionTable,
	threadID string,
	text string,
	attachments []Attachment,
) map[string]any {
	return map[string]any{
		"threadId":          strings.TrimSpace(threadID),
		"input":             codexUserInputs(text, attachments),
		"collaborationMode": codexCollaborationMode(session),
	}
}

func codexApprovalPolicy(level PermissionLevel) string {
	if normalizePermissionLevel(level) == PermissionLevelYolo {
		return "never"
	}
	return "on-request"
}

func codexSandboxMode(level PermissionLevel) string {
	switch normalizePermissionLevel(level) {
	case PermissionLevelElevated, PermissionLevelYolo:
		return "danger-full-access"
	default:
		return "workspace-write"
	}
}

func codexCollaborationMode(session tables.WebSessionTable) map[string]any {
	settings := map[string]any{
		"model":                  strings.TrimSpace(session.Model),
		"developer_instructions": nil,
	}
	if effort := normalizeCodexReasoningEffort(session.Model, ReasoningEffort(session.ReasoningEffort)); effort != ReasoningEffortDefault {
		settings["reasoning_effort"] = string(effort)
	} else {
		settings["reasoning_effort"] = nil
	}
	return map[string]any{
		"mode":     string(normalizeWorkflowMode(effectiveWorkflowMode(session))),
		"settings": settings,
	}
}

func codexUserInputs(text string, attachments []Attachment) []map[string]any {
	inputs := make([]map[string]any, 0, len(attachments)+1)
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		inputs = append(inputs, map[string]any{
			"type":          "text",
			"text":          trimmed,
			"text_elements": []any{},
		})
	}
	for _, attachment := range attachments {
		inputs = append(inputs, map[string]any{
			"type": "localImage",
			"path": attachment.Path,
		})
	}
	return inputs
}

func parseCodexThreadID(raw json.RawMessage) string {
	payload := decodeRawObject(raw)
	thread := decodeRawObject(payload["thread"])
	return stringValue(thread["id"])
}

func parseCodexTurnID(raw json.RawMessage) string {
	payload := decodeRawObject(raw)
	turn := decodeRawObject(payload["turn"])
	return stringValue(turn["id"])
}

func codexNotificationThreadID(raw json.RawMessage) string {
	payload := decodeRawObject(raw)
	thread := decodeRawObject(payload["thread"])
	return strings.TrimSpace(firstNonEmpty(
		stringValue(payload["threadId"]),
		stringValue(thread["id"]),
	))
}

func codexNotificationTurnID(raw json.RawMessage) string {
	payload := decodeRawObject(raw)
	turn := decodeRawObject(payload["turn"])
	return strings.TrimSpace(firstNonEmpty(
		stringValue(payload["turnId"]),
		stringValue(turn["id"]),
	))
}

func parseCodexTurnError(raw json.RawMessage) (message string, code string) {
	payload := decodeRawObject(raw)
	errorMap := decodeRawObject(payload["error"])
	message = firstNonEmpty(codexErrorMessage(errorMap), codexErrorMessage(payload))
	code = firstNonEmpty(codexErrorInfo(errorMap), codexErrorInfo(payload))
	return message, code
}

func parseCodexTurnCompletion(raw json.RawMessage) (status string, errMessage string, errCode string) {
	payload := decodeRawObject(raw)
	turn := decodeRawObject(payload["turn"])
	status = firstNonEmpty(stringValue(turn["status"]), "completed")
	errorMap := decodeRawObject(turn["error"])
	errMessage = codexErrorMessage(errorMap)
	errCode = codexErrorInfo(errorMap)
	return status, errMessage, errCode
}

func decodeRawObject(raw any) map[string]any {
	switch typed := raw.(type) {
	case json.RawMessage:
		if len(typed) == 0 {
			return map[string]any{}
		}
		var value map[string]any
		if err := json.Unmarshal(typed, &value); err == nil && value != nil {
			return value
		}
	case map[string]any:
		return typed
	}
	return map[string]any{}
}

func decodeToolQuestions(raw any) []toolRequestQuestion {
	var items []map[string]any
	switch typed := raw.(type) {
	case json.RawMessage:
		_ = json.Unmarshal(typed, &items)
	case []toolRequestQuestion:
		return append([]toolRequestQuestion(nil), typed...)
	case []map[string]any:
		items = typed
	case []any:
		items = make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				items = append(items, object)
			}
		}
	}

	result := make([]toolRequestQuestion, 0, len(items))
	for _, item := range items {
		question := toolRequestQuestion{
			ID:          stringValue(item["id"]),
			Header:      stringValue(item["header"]),
			Question:    stringValue(item["question"]),
			MultiSelect: item["multiSelect"] == true,
			IsOther:     item["isOther"] == true,
			IsSecret:    item["isSecret"] == true,
		}
		if question.ID == "" {
			question.ID = stringValue(item["ID"])
		}
		if question.Header == "" {
			question.Header = stringValue(item["Header"])
		}
		if question.Question == "" {
			question.Question = stringValue(item["Question"])
		}
		if question.ID == "" {
			question.ID = firstNonEmpty(question.Question, question.Header)
		}
		if !question.MultiSelect {
			question.MultiSelect = item["MultiSelect"] == true
		}
		if !question.IsOther {
			question.IsOther = item["IsOther"] == true
		}
		if !question.IsSecret {
			question.IsSecret = item["IsSecret"] == true
		}
		if options, ok := item["options"].([]any); ok {
			question.Options = make([]toolRequestOption, 0, len(options))
			for _, optionRaw := range options {
				option, ok := optionRaw.(map[string]any)
				if !ok {
					continue
				}
				question.Options = append(question.Options, toolRequestOption{
					Label:       stringValue(option["label"]),
					Description: stringValue(option["description"]),
				})
			}
		} else if options, ok := item["Options"].([]toolRequestOption); ok {
			question.Options = append([]toolRequestOption(nil), options...)
		} else if options, ok := item["Options"].([]any); ok {
			question.Options = make([]toolRequestOption, 0, len(options))
			for _, optionRaw := range options {
				option, ok := optionRaw.(map[string]any)
				if !ok {
					continue
				}
				question.Options = append(question.Options, toolRequestOption{
					Label:       firstNonEmpty(stringValue(option["label"]), stringValue(option["Label"])),
					Description: firstNonEmpty(stringValue(option["description"]), stringValue(option["Description"])),
				})
			}
		}
		result = append(result, question)
	}
	return result
}

func summarizeToolQuestions(questions []toolRequestQuestion) string {
	if len(questions) == 0 {
		return "Codex needs more input before continuing."
	}
	lines := make([]string, 0, len(questions))
	for _, question := range questions {
		line := firstNonEmpty(question.Question, question.Header)
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "Codex needs more input before continuing."
	}
	return strings.Join(lines, "\n")
}

func codexToolSucceeded(item map[string]any) bool {
	status := strings.ToLower(strings.TrimSpace(stringValue(item["status"])))
	if status == "failed" || status == "error" || status == "cancelled" {
		return false
	}
	if exitCode := numberValue(item["exitCode"]); exitCode != 0 && status != "" {
		return false
	}
	return true
}
