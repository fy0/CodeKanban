package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		Permissions: nil,
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

const (
	codexTransportRetryingCode       = "transport_retrying"
	codexTransportRetryExhaustedCode = "transport_retry_exhausted"
	codexRuntimeErrorCode            = "runtime_error"
	codexRetryNoteLevel              = "warn"
)

var codexReconnectProgressPattern = regexp.MustCompile(`(?i)reconnecting\.\.\.\s*(\d+)\s*/\s*(\d+)`)

type codexTransportRetryInfo struct {
	Message     string
	Attempt     int
	MaxAttempts int
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
	run.app = client
	run.cmd = client.cmd

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

	threadID, err := m.startOrResumeCodexThread(ctx, session, run, client)
	if err != nil {
		run.resolveBootstrap(err)
		m.waitAndFailCodexAppServer(session, run, client, waitCh, stderrDone, stderrBuffer, err)
		return
	}

	turnResponse, err := client.request(ctx, "turn/start", codexTurnStartParams(session, threadID, text, attachments))
	if err != nil {
		run.resolveBootstrap(err)
		m.waitAndFailCodexAppServer(session, run, client, waitCh, stderrDone, stderrBuffer, err)
		return
	}
	if turnID := parseCodexTurnID(turnResponse.Result); turnID != "" {
		run.currentToolMessage = turnID
	}
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
			outcome, err := m.handleCodexAppServerMessage(session, run, client, message)
			if err != nil {
				run.lastError = err.Error()
				if outcome == codexTurnOutcomeFailed {
					killCmdTree(client.cmd)
				}
				continue
			}
			if outcome == codexTurnOutcomeCompleted && !turnCompleted {
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
) (string, error) {
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
		return "", err
	}

	threadID := parseCodexThreadID(response.Result)
	if threadID == "" {
		threadID = existingThreadID
	}
	if threadID == "" {
		return "", fmt.Errorf("codex app-server did not return a thread id")
	}
	if err := m.updateRuntimeState(context.Background(), session.ID, map[string]any{
		"native_session_id": threadID,
		"updated_at":        time.Now(),
	}); err != nil {
		return "", err
	}
	run.currentToolMessage = threadID
	return threadID, nil
}

func (m *Manager) handleCodexAppServerMessage(
	session tables.WebSessionTable,
	run *activeRun,
	client *codexAppServerClient,
	message codexAppServerIncoming,
) (codexTurnOutcome, error) {
	switch strings.TrimSpace(message.Method) {
	case "":
		return codexTurnOutcomeNone, nil
	case "thread/started":
		if threadID := parseCodexThreadID(message.Params); threadID != "" {
			_ = m.updateRuntimeState(context.Background(), session.ID, map[string]any{
				"native_session_id": threadID,
				"updated_at":        time.Now(),
			})
		}
		return codexTurnOutcomeNone, nil
	case "turn/started":
		if turnID := parseCodexTurnID(message.Params); turnID != "" {
			run.currentToolMessage = turnID
		}
		return codexTurnOutcomeNone, nil
	case "item/started":
		m.handleCodexAppServerItemStarted(session, run, message.Params)
		return codexTurnOutcomeNone, nil
	case "item/agentMessage/delta":
		m.handleCodexAppServerAgentDelta(session, run, message.Params)
		return codexTurnOutcomeNone, nil
	case "item/completed":
		m.handleCodexAppServerItemCompleted(session, run, message.Params)
		return codexTurnOutcomeNone, nil
	case "thread/tokenUsage/updated":
		m.handleCodexAppServerUsage(session, run, message.Params)
		return codexTurnOutcomeNone, nil
	case "thread/goal/updated":
		if err := m.handleCodexAppServerGoalUpdated(session, message.Params); err != nil {
			return codexTurnOutcomeFailed, err
		}
		return codexTurnOutcomeNone, nil
	case "thread/goal/cleared":
		if err := m.handleCodexAppServerGoalCleared(session); err != nil {
			return codexTurnOutcomeFailed, err
		}
		return codexTurnOutcomeNone, nil
	case "error":
		run.lastError = parseCodexTurnError(message.Params)
		if retryInfo, ok := classifyCodexTransportRetryMessage(run.lastError); ok {
			run.transportRetrySeen = true
			m.appendRunNote(session.ID, session, run, codexRetryNoteLevel, retryInfo.Message, retryInfo.payload())
			run.lastError = ""
			run.lastErrorCode = ""
			return codexTurnOutcomeNone, nil
		}
		run.lastErrorCode = codexRuntimeErrorCode
		if run.transportRetrySeen {
			run.lastErrorCode = codexTransportRetryExhaustedCode
		}
		return codexTurnOutcomeFailed, fmt.Errorf("%s", firstNonEmpty(run.lastError, "codex app-server turn failed"))
	case "turn/completed":
		status, errMessage := parseCodexTurnCompletion(message.Params)
		_ = m.finalizeLatestTurnUsage(context.Background(), session.ID)
		if status == "completed" {
			return codexTurnOutcomeCompleted, nil
		}
		if errMessage != "" {
			run.lastError = errMessage
		}
		if run.transportRetrySeen {
			run.lastErrorCode = codexTransportRetryExhaustedCode
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
	return payload
}

func (m *Manager) handleCodexAppServerItemStarted(
	session tables.WebSessionTable,
	run *activeRun,
	params json.RawMessage,
) {
	payload := decodeRawObject(params)
	item := decodeRawObject(payload["item"])
	itemType := normalizeCodexItemType(stringValue(item["type"]))
	switch itemType {
	case "user_message":
		return
	case "agent_message":
		messageID := firstNonEmpty(stringValue(item["id"]), utils.NewID())
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
	default:
		toolName := codexToolName(item)
		toolInput := codexToolInput(item)
		toolMeta := codexToolMeta(item)
		toolID := firstNonEmpty(stringValue(item["id"]), utils.NewID())
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "tool_st",
			RunID:     run.runID,
			ParentID:  run.assistantMessageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"tid":  toolID,
				"name": toolName,
				"kind": itemType,
				"in":   toolInput,
				"meta": toolMeta,
			},
		})
		m.trackActiveCodexToolStart(run, toolID, itemType, toolName, toolInput, toolMeta)
	}
}

func (m *Manager) handleCodexAppServerAgentDelta(
	session tables.WebSessionTable,
	run *activeRun,
	params json.RawMessage,
) {
	payload := decodeRawObject(params)
	messageID := firstNonEmpty(stringValue(payload["itemId"]), run.assistantMessageID, utils.NewID())
	if run.assistantMessageID != messageID {
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

	run.markAssistantDeltaSeen(messageID)
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
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
	})
}

func (m *Manager) handleCodexAppServerItemCompleted(
	session tables.WebSessionTable,
	run *activeRun,
	params json.RawMessage,
) {
	payload := decodeRawObject(params)
	item := decodeRawObject(payload["item"])
	itemType := normalizeCodexItemType(stringValue(item["type"]))
	switch itemType {
	case "user_message":
		return
	case "agent_message":
		messageID := firstNonEmpty(stringValue(item["id"]), run.assistantMessageID, utils.NewID())
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
		if toolSucceeded && codexToolIsPlan(item) {
			run.markCompletedPlanTool()
		}
		if toolSucceeded && itemType == "context_compaction" {
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
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID:        utils.NewID(),
			Seq:       0,
			Type:      "tool_end",
			RunID:     run.runID,
			ParentID:  run.assistantMessageID,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"tid":  toolID,
				"kind": itemType,
				"out":  codexToolOutput(item),
				"ok":   toolSucceeded,
				"meta": codexToolMeta(item),
			},
		})
		m.trackActiveCodexToolComplete(run, toolID)
	}
}

func (m *Manager) handleCodexAppServerUsage(
	session tables.WebSessionTable,
	run *activeRun,
	params json.RawMessage,
) {
	payload := decodeRawObject(params)
	tokenUsage := decodeRawObject(payload["tokenUsage"])
	total := decodeRawObject(tokenUsage["total"])
	in := int64(numberValue(total["inputTokens"]))
	cin := int64(numberValue(total["cachedInputTokens"]))
	out := int64(numberValue(total["outputTokens"]))
	_ = m.updateRuntimeState(context.Background(), session.ID, contextEstimateTotalsUpdate(in, cin, out))

	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "usage",
		RunID:     run.runID,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"in":  in,
			"cin": cin,
			"out": out,
		},
	})
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
	run.setPendingServerRequest(request)
	m.pauseActiveCallTimeout(run)
	now := time.Now()
	_, err := m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Seq:       0,
		Type:      "approval_req",
		RunID:     run.runID,
		ParentID:  run.assistantMessageID,
		Timestamp: now,
		Payload: map[string]any{
			"prompt": request.Prompt,
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
		RawID:  append(json.RawMessage(nil), message.ID...),
		ItemID: itemID,
		Prompt: firstNonEmpty(
			stringValue(payload["reason"]),
			stringValue(payload["command"]),
			stringValue(payload["grantRoot"]),
			"Codex is waiting for approval before continuing.",
		),
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
	return request
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
	if effort := normalizeReasoningEffort(ReasoningEffort(session.ReasoningEffort)); effort != ReasoningEffortDefault {
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

func parseCodexTurnError(raw json.RawMessage) string {
	payload := decodeRawObject(raw)
	errorMap := decodeRawObject(payload["error"])
	return firstNonEmpty(
		stringValue(errorMap["message"]),
		stringValue(errorMap["additionalDetails"]),
		stringValue(payload["message"]),
	)
}

func parseCodexTurnCompletion(raw json.RawMessage) (status string, errMessage string) {
	payload := decodeRawObject(raw)
	turn := decodeRawObject(payload["turn"])
	status = firstNonEmpty(stringValue(turn["status"]), "completed")
	errorMap := decodeRawObject(turn["error"])
	errMessage = firstNonEmpty(
		stringValue(errorMap["message"]),
		stringValue(errorMap["additionalDetails"]),
	)
	return status, errMessage
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
