package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const devinACPProtocolVersion = 1

type devinACPMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *devinACPError  `json:"error,omitempty"`
}

type devinACPError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *devinACPError) Error() string {
	if e == nil {
		return "Devin ACP error"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("Devin ACP error %d", e.Code)
}

type devinACPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	ctx    context.Context

	writeMu   sync.Mutex
	pending   map[string]chan devinACPMessage
	pendingMu sync.Mutex
	events    chan devinACPMessage
	closed    chan struct{}
	closeOnce sync.Once
	seq       atomic.Uint64

	terminalMu sync.Mutex
	terminals  map[string]*devinACPTerminal

	// shell is the resolved shell command (binary + startup args) used to run
	// terminal/create commands, mirroring how interactive terminals resolve it.
	shell []string

	// modeMu guards the attached session's advertised modes, its ACP session
	// id, and whether the mode we mapped from the session record was applied.
	modeMu       sync.Mutex
	acpSessionID string
	modes        *devinACPSessionModes
	modeApplied  bool
}

// devinACPSessionModes mirrors the `modes` object returned by session/new,
// session/load, and session/resume.
type devinACPSessionModes struct {
	CurrentModeID    string
	AvailableModeIDs map[string]bool
}

func parseDevinSessionModes(raw json.RawMessage) *devinACPSessionModes {
	var result struct {
		Modes *struct {
			CurrentModeID  string `json:"currentModeId"`
			AvailableModes []struct {
				ID string `json:"id"`
			} `json:"availableModes"`
		} `json:"modes"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &result) != nil || result.Modes == nil {
		return nil
	}
	modes := &devinACPSessionModes{
		CurrentModeID:    strings.TrimSpace(result.Modes.CurrentModeID),
		AvailableModeIDs: make(map[string]bool, len(result.Modes.AvailableModes)),
	}
	for _, mode := range result.Modes.AvailableModes {
		if id := strings.TrimSpace(mode.ID); id != "" {
			modes.AvailableModeIDs[id] = true
		}
	}
	return modes
}

func (c *devinACPClient) setSessionModes(sessionID string, modes *devinACPSessionModes, applied bool) {
	c.modeMu.Lock()
	defer c.modeMu.Unlock()
	c.acpSessionID = sessionID
	c.modes = modes
	c.modeApplied = applied
}

func (c *devinACPClient) modeAppliedSnapshot() bool {
	c.modeMu.Lock()
	defer c.modeMu.Unlock()
	return c.modeApplied
}

func (c *devinACPClient) sessionModesSnapshot() (*devinACPSessionModes, string) {
	c.modeMu.Lock()
	defer c.modeMu.Unlock()
	return c.modes, c.acpSessionID
}

type devinACPTerminal struct {
	id       string
	cmd      *exec.Cmd
	outputMu sync.Mutex
	output   bytes.Buffer
	done     chan struct{}
	exitCode *int
	exitErr  error
}

type devinACPTerminalWriter struct{ terminal *devinACPTerminal }

func (w devinACPTerminalWriter) Write(data []byte) (int, error) {
	w.terminal.outputMu.Lock()
	defer w.terminal.outputMu.Unlock()
	return w.terminal.output.Write(data)
}

func startDevinACP(ctx context.Context, path, cwd, model string) (*devinACPClient, error) {
	parts := splitCommandParts(path)
	if len(parts) == 0 {
		return nil, errors.New("Devin CLI path is empty")
	}
	args := append(parts[1:], "acp")
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", strings.TrimSpace(model))
	}
	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	client := &devinACPClient{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		ctx:       ctx,
		pending:   make(map[string]chan devinACPMessage),
		events:    make(chan devinACPMessage, 64),
		closed:    make(chan struct{}),
		terminals: make(map[string]*devinACPTerminal),
	}
	// Drain stderr independently so diagnostic output cannot block the ACP
	// process while the session is streaming events.
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	go client.readLoop()
	return client, nil
}

func (c *devinACPClient) readLoop() {
	defer close(c.events)
	defer close(c.closed)
	reader := bufio.NewReaderSize(c.stdout, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var message devinACPMessage
			if decodeErr := json.Unmarshal(line, &message); decodeErr != nil {
				continue
			}
			if key := devinACPIDKey(message.ID); key != "" && message.Method == "" {
				c.pendingMu.Lock()
				response := c.pending[key]
				delete(c.pending, key)
				c.pendingMu.Unlock()
				if response != nil {
					response <- message
					close(response)
				}
			} else {
				select {
				case c.events <- message:
				case <-c.ctx.Done():
					return
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func devinACPIDKey(raw json.RawMessage) string {
	return strings.TrimSpace(string(raw))
}

func (c *devinACPClient) send(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (c *devinACPClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.seq.Add(1)
	rawID := json.RawMessage(strconv.FormatUint(id, 10))
	response := make(chan devinACPMessage, 1)
	key := devinACPIDKey(rawID)
	c.pendingMu.Lock()
	c.pending[key] = response
	c.pendingMu.Unlock()
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, key)
		c.pendingMu.Unlock()
		return nil, err
	}
	select {
	case message := <-response:
		if message.Error != nil {
			return nil, message.Error
		}
		return message.Result, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, key)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, errors.New("Devin ACP process closed")
	}
}

func (c *devinACPClient) respond(id json.RawMessage, result any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func (c *devinACPClient) respondError(id json.RawMessage, code int, message string) error {
	return c.send(map[string]any{
		"jsonrpc": "2.0", "id": json.RawMessage(id),
		"error": map[string]any{"code": code, "message": message},
	})
}

func (c *devinACPClient) notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *devinACPClient) close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		for _, terminal := range c.terminalsSnapshot() {
			if terminal.cmd.Process != nil {
				_ = terminal.cmd.Process.Kill()
			}
		}
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	})
}

func (c *devinACPClient) terminalsSnapshot() []*devinACPTerminal {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	items := make([]*devinACPTerminal, 0, len(c.terminals))
	for _, terminal := range c.terminals {
		items = append(items, terminal)
	}
	return items
}

func (m *Manager) runDevinACPSession(ctx context.Context, run *activeRun, session tables.WebSessionTable, text string, attachments []Attachment) {
	var images []piRPCImage
	if len(attachments) > 0 {
		if known, supports := m.devinModelSupportsImages(session.Model); known && !supports {
			m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin model %s does not support image attachments", session.Model))
			return
		}
		var err error
		images, err = m.piPromptImages(attachments)
		if err != nil {
			m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP attachments: %w", err))
			return
		}
	}
	client, err := startDevinACP(ctx, m.cfg.DevinPath, session.Cwd, session.Model)
	if err != nil {
		m.handleRunFailure(session.ID, session, run, fmt.Errorf("failed to start Devin ACP: %w", err))
		return
	}
	client.shell, _ = utils.ResolveShellCommand("", m.terminalShellConfig())
	run.setDevinACP(client)
	run.setCommand(client.cmd)
	defer client.close()

	initialize, err := client.request(ctx, "initialize", m.devinACPInitializeParams())
	if err != nil {
		m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP initialize failed: %w", err))
		return
	}
	var initializeResult map[string]any
	_ = json.Unmarshal(initialize, &initializeResult)

	proj := newDevinRunProjection()
	nativeSessionID := pointerString(session.NativeSessionID)
	var sessionModes *devinACPSessionModes
	if nativeSessionID == "" {
		result, requestErr := client.request(ctx, "session/new", map[string]any{
			"cwd":        session.Cwd,
			"mcpServers": []any{},
		})
		if requestErr != nil {
			m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP session/new failed: %w", requestErr))
			return
		}
		var created struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(result, &created); err != nil || strings.TrimSpace(created.SessionID) == "" {
			m.handleRunFailure(session.ID, session, run, errors.New("Devin ACP session/new returned no sessionId"))
			return
		}
		nativeSessionID = created.SessionID
		session.NativeSessionID = &nativeSessionID
		sessionModes = parseDevinSessionModes(result)
		_ = m.updateRuntimeState(context.Background(), session.ID, map[string]any{
			"native_session_id": nativeSessionID,
			"updated_at":        time.Now(),
		})
	} else {
		agentCapabilities := initializeResult["agentCapabilities"]
		resumed := false
		var resumeErr error
		if sessionCapabilitiesContain(agentCapabilities, "resume") {
			// session/resume restores context without replaying history.
			if result, requestErr := client.request(ctx, "session/resume", map[string]any{
				"sessionId":  nativeSessionID,
				"cwd":        session.Cwd,
				"mcpServers": []any{},
			}); requestErr == nil {
				resumed = true
				sessionModes = parseDevinSessionModes(result)
			} else {
				resumeErr = requestErr
			}
		}
		switch {
		case resumed:
		case capabilitiesContain(agentCapabilities, "loadSession"):
			result, requestErr := m.loadDevinACPSession(ctx, client, session, run, proj, nativeSessionID)
			if requestErr != nil {
				m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP session/load failed: %w", requestErr))
				return
			}
			sessionModes = parseDevinSessionModes(result)
		case resumeErr != nil:
			m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP session/resume failed: %w", resumeErr))
			return
		}
	}

	m.applyDevinSessionMode(ctx, client, session, nativeSessionID, sessionModes)

	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		m.consumeDevinACPEvents(ctx, client, session, run, proj)
	}()

	prompt := make([]map[string]any, 0, 1+len(images))
	if strings.TrimSpace(text) != "" {
		prompt = append(prompt, map[string]any{"type": "text", "text": text})
	}
	for _, image := range images {
		prompt = append(prompt, map[string]any{"type": image.Type, "data": image.Data, "mimeType": image.MimeType})
	}
	promptResult, err := client.request(ctx, "session/prompt", map[string]any{
		"sessionId": nativeSessionID,
		"prompt":    prompt,
	})
	if err != nil {
		if ctx.Err() != nil || run.abortRequestedSnapshot() {
			_ = client.notify("session/cancel", map[string]any{"sessionId": nativeSessionID})
			m.finishAbortedRun(session.ID, session, run)
			return
		}
		m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP prompt failed: %w", err))
		return
	}
	client.close()
	<-eventsDone
	m.applyDevinPromptUsageFallback(session, run, proj, promptResult)
	m.finishDevinRun(session, run, proj)
}

// devinACPInitializeParams builds the ACP initialize request. clientInfo
// mirrors what Devin Desktop's ACP connector sends so the agent treats this
// like a first-party session. The declared clientCapabilities are the revert
// and sub-agent extension opt-ins under _meta — the agent only advertises
// subagent support back when both subagent keys are declared. elicitation/fs
// stay undeclared, and terminal is omitted like the desktop so the agent uses
// its native tools.
func (m *Manager) devinACPInitializeParams() map[string]any {
	return map[string]any{
		"protocolVersion": devinACPProtocolVersion,
		"clientCapabilities": map[string]any{
			"_meta": map[string]any{
				"cognition.ai/revert":          true,
				"cognition.ai/subagentSupport": true,
				"cognition.ai/subagentControl": true,
			},
		},
		"clientInfo": map[string]any{
			"name":    "windsurf",
			"version": m.devinACPClientVersion(),
		},
	}
}

// devinAgentMetaCapability reports whether agentCapabilities advertises the
// named private extension via _meta[key] === true.
func devinAgentMetaCapability(raw any, key string) bool {
	values, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	meta, ok := values["_meta"].(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := meta[key].(bool)
	return ok && enabled
}

// devinAgentSupportsRevert reports whether agentCapabilities advertises the
// private revert extension via _meta["cognition.ai/revert"] === true. The
// agent only enables listSteps/forkFromStep when the client opted in during
// initialize, which devinACPInitializeParams does.
func devinAgentSupportsRevert(raw any) bool {
	return devinAgentMetaCapability(raw, "cognition.ai/revert")
}

// devinAgentSupportsSubAgents reports whether agentCapabilities advertises
// the private sub-agent extension via _meta["cognition.ai/subagentControl"]
// === true. The agent only emits subagent_started/completed/context metadata
// when the client opted in during initialize, which
// devinACPInitializeParams does.
func devinAgentSupportsSubAgents(raw any) bool {
	return devinAgentMetaCapability(raw, "cognition.ai/subagentControl")
}

var devinClientVersion struct {
	once    sync.Once
	version string
}

// devinACPClientVersion reports the Devin CLI version, probed once per process
// and sent as clientInfo.version in the ACP initialize handshake.
func (m *Manager) devinACPClientVersion() string {
	devinClientVersion.once.Do(func() {
		if version := detectDevinVersion(m.cfg.DevinPath); version != nil {
			devinClientVersion.version = *version
		}
	})
	return devinClientVersion.version
}

func (m *Manager) terminalShellConfig() utils.TerminalShellConfig {
	if m != nil && m.cfg.TerminalShell != nil {
		return m.cfg.TerminalShell()
	}
	return utils.TerminalShellConfig{}
}

// devinModelSupportsImages reports whether the catalog explicitly flags the
// model's image support. It only returns known=true when the catalog provided
// the supports_images flag at all — models without the flag (older CLI output
// or unexpected layouts) are treated as unknown so the agent decides.
func (m *Manager) devinModelSupportsImages(model string) (known bool, supports bool) {
	model = strings.TrimSpace(model)
	for _, info := range m.getDevinModelCatalog(false) {
		if info.Model == model && info.supportsImagesSet {
			return true, info.SupportsImages
		}
	}
	return false, false
}

func capabilitiesContain(raw any, key string) bool {
	values, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	value, ok := values[key]
	return ok && value != nil
}

// sessionCapabilitiesContain reports whether agentCapabilities advertises the
// named sessionCapabilities entry (e.g. "resume").
func sessionCapabilitiesContain(raw any, key string) bool {
	values, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	scoped, ok := values["sessionCapabilities"].(map[string]any)
	if !ok {
		return false
	}
	value, ok := scoped[key]
	return ok && value != nil
}

// loadDevinACPSession performs session/load while draining client.events. Per
// the ACP spec the agent replays the entire conversation as session/update
// notifications before responding; those replays are discarded here so prior
// replies are not appended to the run again. Draining during the call also
// keeps the buffered events channel from filling up and deadlocking the load.
func (m *Manager) loadDevinACPSession(ctx context.Context, client *devinACPClient, session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, nativeSessionID string) (json.RawMessage, error) {
	type loadResult struct {
		raw json.RawMessage
		err error
	}
	done := make(chan loadResult, 1)
	go func() {
		raw, err := client.request(ctx, "session/load", map[string]any{
			"sessionId":  nativeSessionID,
			"cwd":        session.Cwd,
			"mcpServers": []any{},
		})
		done <- loadResult{raw: raw, err: err}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-done:
			// The response is ordered after every replayed notification on
			// the wire, so whatever is still queued is history — discard it.
			for {
				select {
				case message, ok := <-client.events:
					if !ok {
						return res.raw, res.err
					}
					m.dispatchDevinACPMessage(client, session, run, proj, message, devinACPDispatchReplay)
				default:
					return res.raw, res.err
				}
			}
		case message, ok := <-client.events:
			if !ok {
				return nil, errors.New("Devin ACP process closed")
			}
			m.dispatchDevinACPMessage(client, session, run, proj, message, devinACPDispatchReplay)
		}
	}
}

func (m *Manager) consumeDevinACPEvents(ctx context.Context, client *devinACPClient, session tables.WebSessionTable, run *activeRun, proj *devinRunProjection) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-client.events:
			if !ok {
				return
			}
			m.dispatchDevinACPMessage(client, session, run, proj, message, devinACPDispatchLive)
		}
	}
}

// devinACPDispatchMode controls how dispatchDevinACPMessage treats incoming
// agent→client traffic outside of an ordinary live run.
type devinACPDispatchMode int

const (
	// devinACPDispatchLive projects session/update into history and answers
	// terminal/permission requests normally.
	devinACPDispatchLive devinACPDispatchMode = iota
	// devinACPDispatchReplay drops session/update (session/load history
	// replay — codekanban already persisted those turns) but still answers
	// terminal/permission requests on the attached run.
	devinACPDispatchReplay
	// devinACPDispatchDetached drops session/update and refuses terminal and
	// permission requests: the ACP process is attached to a session only to
	// issue extension calls, so no operator exists to service requests.
	devinACPDispatchDetached
	// devinACPDispatchCapture projects session/update into history (used to
	// materialize a forked session's replay) while refusing terminal and
	// permission requests — replaying history must not execute commands or
	// block on prompts.
	devinACPDispatchCapture
)

// dispatchDevinACPMessage routes one agent→client message.
func (m *Manager) dispatchDevinACPMessage(client *devinACPClient, session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, message devinACPMessage, mode devinACPDispatchMode) {
	if message.Method == "session/update" {
		if mode == devinACPDispatchLive || mode == devinACPDispatchCapture {
			m.handleDevinACPUpdate(session, run, proj, message.Params)
		}
		return
	}
	if message.Method == "session/request_permission" {
		if mode == devinACPDispatchLive || mode == devinACPDispatchReplay {
			m.handleDevinPermissionRequest(client, session, run, proj, message)
		} else if message.ID != nil {
			_ = client.respond(message.ID, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
		}
		return
	}
	if strings.HasPrefix(message.Method, "terminal/") {
		if mode == devinACPDispatchLive || mode == devinACPDispatchReplay {
			m.handleDevinTerminalRequest(client, session, run, message)
		} else if message.ID != nil {
			_ = client.respondError(message.ID, -32601, "unsupported method: "+message.Method)
		}
		return
	}
	if isDevinCompactionNotification(message.Method) {
		if mode == devinACPDispatchLive {
			m.handleDevinCompactionNotification(session, run, proj, message.Params)
		}
		return
	}
	if isDevinQuotaStatsNotification(message.Method) {
		if mode == devinACPDispatchLive {
			m.handleDevinQuotaStatsNotification(session, run, proj, message.Params)
		}
		return
	}
	if message.ID != nil && strings.TrimSpace(message.Method) != "" {
		_ = client.respondError(message.ID, -32601, "unsupported method: "+message.Method)
	}
}

func (m *Manager) handleDevinTerminalRequest(client *devinACPClient, session tables.WebSessionTable, run *activeRun, message devinACPMessage) {
	var params map[string]any
	if err := json.Unmarshal(message.Params, &params); err != nil {
		_ = client.respondError(message.ID, -32602, err.Error())
		return
	}
	var result map[string]any
	var err error
	switch message.Method {
	case "terminal/create":
		result, err = client.terminalCreate(params, session.Cwd)
	case "terminal/output":
		result, err = client.terminalOutput(params, false)
	case "terminal/wait_for_exit":
		result, err = client.terminalOutput(params, true)
	case "terminal/kill":
		err = client.terminalKill(params)
		result = map[string]any{}
	case "terminal/release":
		err = client.terminalRelease(params)
		result = map[string]any{}
	default:
		_ = client.respondError(message.ID, -32601, "unsupported method: "+message.Method)
		return
	}
	if err != nil {
		_ = client.respondError(message.ID, -32603, err.Error())
		m.appendRunNote(session.ID, session, run, "warning",
			"Devin terminal request failed: "+err.Error(),
			map[string]any{"method": message.Method, "code": "devin_terminal_error"})
		return
	}
	_ = client.respond(message.ID, result)
}

// devinMessageState tracks one streaming context's open assistant message and
// thinking block. Contexts are keyed by the sub-agent id taken from
// _meta["cognition.ai/subagent_context"].parentAgentId ("" = the main agent)
// so a background sub-agent interleaving updates never shares message or
// thinking state with its parent.
type devinMessageState struct {
	messageID      string // currently open assistant message ("" = none)
	messageHasText bool
	thinkingID     string
	thinkingText   strings.Builder
	thinkingIndex  int
	lastThinkEmit  time.Time
}

// devinRunProjection tracks the Devin ACP run's per-context assistant message
// and thinking/tool state so the projection can emit Pi-shaped events: a
// single assistant message per turn that owns its thinking blocks and tool
// calls.
type devinRunProjection struct {
	mu       sync.Mutex
	messages map[string]*devinMessageState // by sub-agent context id ("" = main)
	tools    map[string]*devinToolState    // by ACP toolCallId

	// captureHistory marks the projection as materializing a session/load
	// replay (used when hydrating a forked session). In this mode
	// user_message_chunk updates are buffered into pendingUserText so the
	// caller can emit them as msg_u events at message boundaries.
	captureHistory  bool
	pendingUserText strings.Builder

	// usageSignatures dedupes usage_update notifications: the agent emits one
	// plain update plus a second copy tagged with subagent_context for the
	// same request, and only the first must be counted.
	usageSignatures map[string]bool
	// turnStatRequestIDs dedupes per-request quota reports carried by
	// cognition.ai/turn_stats and cognition.ai/agent_stopped — the same turn's
	// stats appear in both notifications. Kept separate from usageSignatures
	// because sawDevinUsageUpdate treats an empty map as "no usage_update
	// arrived" and falls back to the prompt response's usage block.
	turnStatRequestIDs map[string]bool
	// compactionToolID is the open context-compaction tool event, if any.
	compactionToolID string
}

type devinToolState struct {
	name     string
	kind     string
	input    any
	parentID string
	meta     map[string]any
}

func newDevinRunProjection() *devinRunProjection {
	return &devinRunProjection{
		messages:           make(map[string]*devinMessageState),
		tools:              make(map[string]*devinToolState),
		usageSignatures:    make(map[string]bool),
		turnStatRequestIDs: make(map[string]bool),
	}
}

// recordDevinTurnStatRequest reports whether requestID's quota stats were
// already folded in during this run; the first occurrence is recorded and
// returns false. An empty requestID is never deduped.
func (p *devinRunProjection) recordDevinTurnStatRequest(requestID string) bool {
	if requestID == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.turnStatRequestIDs[requestID] {
		return true
	}
	p.turnStatRequestIDs[requestID] = true
	return false
}

// recordDevinUsageSignature reports whether signature was already observed in
// this run; the first occurrence is recorded and returns false.
func (p *devinRunProjection) recordDevinUsageSignature(signature string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.usageSignatures[signature] {
		return true
	}
	p.usageSignatures[signature] = true
	return false
}

// sawDevinUsageUpdate reports whether any usage_update was recorded this run.
func (p *devinRunProjection) sawDevinUsageUpdate() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.usageSignatures) > 0
}

// openDevinCompaction returns the open compaction tool id, creating one when
// none is open. p.mu must not be held (it locks internally).
func (p *devinRunProjection) ensureDevinCompactionToolID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.compactionToolID == "" {
		p.compactionToolID = utils.NewID()
	}
	return p.compactionToolID
}

// takeDevinCompactionToolID returns and clears the open compaction tool id;
// empty when no compaction is in flight.
func (p *devinRunProjection) takeDevinCompactionToolID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.compactionToolID
	p.compactionToolID = ""
	return id
}

// devinToolSnapshot returns a copy of the tracked tool call, or nil when the
// projection has not seen it. session/request_permission payloads often carry
// only a toolCallId, so the approval card borrows the title/input captured
// from the earlier tool_call update.
func (p *devinRunProjection) devinToolSnapshot(toolCallID string) *devinToolState {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tool := p.tools[strings.TrimSpace(toolCallID)]
	if tool == nil {
		return nil
	}
	clone := *tool
	clone.meta = cloneMap(tool.meta)
	return &clone
}

// messageState returns the streaming state for a sub-agent context.
// p.mu must be held.
func (p *devinRunProjection) messageState(contextID string) *devinMessageState {
	state := p.messages[contextID]
	if state == nil {
		state = &devinMessageState{}
		p.messages[contextID] = state
	}
	return state
}

// takeDevinCapturedUserText drains user_message_chunk text buffered during
// history capture. Empty means no user message is pending emission.
func (p *devinRunProjection) takeDevinCapturedUserText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	text := p.pendingUserText.String()
	p.pendingUserText.Reset()
	return text
}

// devinChunkText extracts the text carried by a session/update chunk. The
// content block is authoritative whenever it holds a string — including
// whitespace-only values, which Devin streams as standalone chunks ("\n",
// " ") and which must be relayed verbatim or markdown loses its newlines.
func devinChunkText(update map[string]any) string {
	if content, ok := update["content"].(map[string]any); ok {
		if text, ok := content["text"].(string); ok {
			return text
		}
	}
	return stringValue(update["text"])
}

// devinACPSessionUpdateKind extracts update.sessionUpdate from a
// session/update notification payload without projecting it.
func devinACPSessionUpdateKind(raw json.RawMessage) string {
	var payload struct {
		Update struct {
			SessionUpdate string `json:"sessionUpdate"`
		} `json:"update"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	return payload.Update.SessionUpdate
}

// devinToolPlanPath returns the plan file path advertised by a switch_mode
// tool call's locations, used as the plan card subtitle.
func devinToolPlanPath(update map[string]any) string {
	if items, ok := update["locations"].([]any); ok {
		for _, item := range items {
			if path := strings.TrimSpace(stringValue(decodeRawObject(item)["path"])); path != "" {
				return path
			}
		}
	}
	return ""
}

// devinToolHistoryKind maps the raw ACP tool kind to a history kind the UI
// understands. Unknown kinds fall back to dynamic_tool_call like Pi.
func devinToolHistoryKind(acpKind string) string {
	switch strings.ToLower(strings.TrimSpace(acpKind)) {
	case "execute":
		return "command_execution"
	case "edit", "delete", "move":
		return "file_change"
	case "fetch":
		return "web_search"
	default:
		return "dynamic_tool_call"
	}
}

// devinToolOutputText extracts a displayable tool output from a tool_call_update
// payload. A string rawOutput is used directly; a non-nil non-string value is
// JSON-encoded; otherwise the content blocks (type=="content") are joined.
func devinToolOutputText(update map[string]any) string {
	kind := devinToolHistoryKind(stringValue(update["kind"]))
	switch typed := update["rawOutput"].(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			return truncateToolOutput(kind, typed)
		}
	case nil:
		// fall through to the content-block fallback below
	default:
		if encoded, err := json.Marshal(typed); err == nil && len(encoded) > 0 {
			return truncateToolOutput(kind, string(encoded))
		}
	}
	parts := make([]string, 0)
	if items, ok := update["content"].([]any); ok {
		for _, item := range items {
			entry := decodeRawObject(item)
			if !strings.EqualFold(stringValue(entry["type"]), "content") {
				continue
			}
			inner := decodeRawObject(entry["content"])
			if text := strings.TrimSpace(stringValue(inner["text"])); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return truncateToolOutput(kind, strings.Join(parts, "\n"))
}

// ensureDevinMessage opens an assistant message for the context if none is
// open and returns its id. It mirrors startPiAssistantMessage: msg_a_st +
// run.setAssistantMessageID (main context only — the run-level field tracks
// the primary agent's message).
func (m *Manager) ensureDevinMessage(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, contextID string) string {
	proj.mu.Lock()
	state := proj.messageState(contextID)
	if state.messageID != "" {
		id := state.messageID
		proj.mu.Unlock()
		return id
	}
	id := utils.NewID()
	state.messageID = id
	state.messageHasText = false
	proj.mu.Unlock()
	if contextID == "" {
		run.setAssistantMessageID(id)
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "msg_a_st", RunID: run.runID, ParentID: id, ThreadID: contextID,
		Timestamp: time.Now(), Payload: map[string]any{"mid": id},
	})
	return id
}

// finishDevinThinking emits the final reasoning tool_end for the context's
// open thinking block and resets the thinking state. Text or a tool call ends
// thinking.
func (m *Manager) finishDevinThinking(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, contextID string) {
	proj.mu.Lock()
	state := proj.messages[contextID]
	if state == nil || state.thinkingID == "" {
		proj.mu.Unlock()
		return
	}
	thinkingID := state.thinkingID
	text := state.thinkingText.String()
	messageID := state.messageID
	state.thinkingID = ""
	state.thinkingText.Reset()
	state.lastThinkEmit = time.Time{}
	proj.mu.Unlock()
	if messageID == "" {
		if contextID == "" {
			messageID = run.assistantMessageIDSnapshot()
		} else {
			messageID = m.ensureDevinMessage(session, run, proj, contextID)
		}
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "tool_end", RunID: run.runID, ParentID: messageID, ThreadID: contextID,
		Timestamp: time.Now(), Payload: map[string]any{
			"tid": thinkingID, "name": "Reasoning", "kind": "reasoning",
			"out": truncateToolOutput("reasoning", text), "ok": true,
		},
	})
}

// closeDevinMessage finishes any open thinking and emits txt_end when the
// context's open message produced text, then clears the message so the next
// turn opens a new bubble (mirrors Pi: text + tool calls belong to one
// message).
func (m *Manager) closeDevinMessage(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, contextID string) {
	m.finishDevinThinking(session, run, proj, contextID)
	proj.mu.Lock()
	state := proj.messages[contextID]
	if state == nil {
		proj.mu.Unlock()
		return
	}
	messageID := state.messageID
	hasText := state.messageHasText
	state.messageID = ""
	state.messageHasText = false
	proj.mu.Unlock()
	if messageID == "" || !hasText {
		return
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "txt_end", RunID: run.runID, ParentID: messageID, ThreadID: contextID,
		Timestamp: time.Now(), Payload: map[string]any{"mid": messageID},
	})
}

// closeAllDevinMessages closes every open message across all sub-agent
// contexts; used when the run ends.
func (m *Manager) closeAllDevinMessages(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection) {
	proj.mu.Lock()
	contextIDs := make([]string, 0, len(proj.messages))
	for contextID := range proj.messages {
		contextIDs = append(contextIDs, contextID)
	}
	proj.mu.Unlock()
	for _, contextID := range contextIDs {
		m.closeDevinMessage(session, run, proj, contextID)
	}
}

// Devin's private sub-agent extension rides on session/update _meta when the
// client opts in via cognition.ai/subagentSupport + subagentControl (both are
// declared in devinACPInitializeParams). The shapes mirror Devin Desktop's
// ACP connector: subagent_started/subagent_completed carry lifecycle
// transitions on the spawning tool call's updates, and subagent_context tags
// updates emitted by the child agent itself.
type devinSubAgentStartedMeta struct {
	AgentID      string
	Title        string
	Task         string
	Profile      string
	Depth        int
	IsBackground bool
	RunID        string
}

type devinSubAgentCompletedMeta struct {
	AgentID string
	Success *bool
	Summary string
	RunID   string
}

func parseDevinSubAgentStartedMeta(meta map[string]any) *devinSubAgentStartedMeta {
	raw := decodeRawObject(meta["cognition.ai/subagent_started"])
	if len(raw) == 0 {
		return nil
	}
	agentID := strings.TrimSpace(stringValue(raw["agentId"]))
	if agentID == "" {
		return nil
	}
	return &devinSubAgentStartedMeta{
		AgentID:      agentID,
		Title:        strings.TrimSpace(stringValue(raw["title"])),
		Task:         strings.TrimSpace(stringValue(raw["task"])),
		Profile:      strings.TrimSpace(stringValue(raw["profile"])),
		Depth:        int(numberValue(raw["depth"])),
		IsBackground: raw["isBackground"] == true,
		RunID:        strings.TrimSpace(stringValue(raw["runId"])),
	}
}

func parseDevinSubAgentCompletedMeta(meta map[string]any) *devinSubAgentCompletedMeta {
	raw := decodeRawObject(meta["cognition.ai/subagent_completed"])
	if len(raw) == 0 {
		return nil
	}
	agentID := strings.TrimSpace(stringValue(raw["agentId"]))
	if agentID == "" {
		return nil
	}
	completed := &devinSubAgentCompletedMeta{
		AgentID: agentID,
		Summary: strings.TrimSpace(stringValue(raw["summary"])),
		RunID:   strings.TrimSpace(stringValue(raw["runId"])),
	}
	if success, ok := raw["success"].(bool); ok {
		completed.Success = &success
	}
	return completed
}

// parseDevinSubAgentContextMeta returns the id of the sub-agent that owns the
// update ("" when the update belongs to the main agent). The agent tags the
// main context as "root", which maps back to the empty main context id.
func parseDevinSubAgentContextMeta(meta map[string]any) string {
	raw := decodeRawObject(meta["cognition.ai/subagent_context"])
	if id := strings.TrimSpace(stringValue(raw["parentAgentId"])); id != "root" {
		return id
	}
	return ""
}

// applyDevinSubAgentMeta projects sub-agent lifecycle metadata from an
// update's _meta into sub_agent_state/sub_agent_activity events and returns
// the owning sub-agent's context id for the update itself.
func (m *Manager) applyDevinSubAgentMeta(session tables.WebSessionTable, run *activeRun, meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	contextID := parseDevinSubAgentContextMeta(meta)
	if started := parseDevinSubAgentStartedMeta(meta); started != nil {
		m.appendDevinSubAgentStarted(session, run, *started, contextID)
	}
	if completed := parseDevinSubAgentCompletedMeta(meta); completed != nil {
		m.appendDevinSubAgentCompleted(session, run, *completed)
	}
	return contextID
}

// appendDevinSubAgentState emits a sub_agent_state event; the projection
// pipeline persists it into web_session_sub_agents and broadcasts the
// sub_agent wire frame.
func (m *Manager) appendDevinSubAgentState(session tables.WebSessionTable, run *activeRun, agentID string, payload map[string]any) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || run == nil {
		return
	}
	if session.NativeSessionID != nil && agentID == strings.TrimSpace(*session.NativeSessionID) {
		return
	}
	nextPayload := cloneMap(payload)
	if nextPayload == nil {
		nextPayload = map[string]any{}
	}
	nextPayload["threadId"] = agentID
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Type:      "sub_agent_state",
		RunID:     run.runID,
		ThreadID:  agentID,
		Timestamp: time.Now(),
		Payload:   nextPayload,
	})
}

func (m *Manager) appendDevinSubAgentStarted(session tables.WebSessionTable, run *activeRun, started devinSubAgentStartedMeta, parentAgentID string) {
	path := firstNonEmpty(started.Profile, started.Title)
	payload := map[string]any{
		"status": string(WebSessionSubAgentRunning),
		"active": true,
	}
	if path != "" {
		payload["path"] = path
	}
	if started.Title != "" {
		payload["nickname"] = started.Title
	}
	if started.Profile != "" {
		payload["role"] = started.Profile
	}
	if started.Task != "" {
		payload["summary"] = started.Task
	}
	if parentAgentID != "" {
		payload["parentThreadId"] = parentAgentID
	}
	m.appendDevinSubAgentState(session, run, started.AgentID, payload)
	// Mirror Codex's spawn marker so the timeline shows "Agent X started".
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Type:      "sub_agent_activity",
		RunID:     run.runID,
		ThreadID:  strings.TrimSpace(parentAgentID),
		Timestamp: time.Now(),
		Payload: map[string]any{
			"agentThreadId": started.AgentID,
			"path":          path,
			"kind":          "started",
		},
	})
}

func (m *Manager) appendDevinSubAgentCompleted(session tables.WebSessionTable, run *activeRun, completed devinSubAgentCompletedMeta) {
	status := WebSessionSubAgentCompleted
	if completed.Success != nil && !*completed.Success {
		status = WebSessionSubAgentErrored
	}
	payload := map[string]any{
		"status": string(status),
		"active": false,
	}
	if completed.Summary != "" {
		payload["summary"] = completed.Summary
	}
	m.appendDevinSubAgentState(session, run, completed.AgentID, payload)
}

// interruptActiveDevinSubAgents marks every still-active Devin sub-agent row
// as interrupted. The ACP process is per-run, so a sub-agent that is still
// running when the run ends (abort, failure, or a background agent whose
// completion never arrived) is torn down with it.
func (m *Manager) interruptActiveDevinSubAgents(session tables.WebSessionTable, run *activeRun) {
	if run == nil || normalizeAgent(Agent(session.Agent)) != AgentDevin {
		return
	}
	db := model.GetDB()
	if db == nil {
		return
	}
	var rows []tables.WebSessionSubAgentTable
	if err := db.Where("web_session_id = ? AND is_active = ?", session.ID, true).Find(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		m.appendDevinSubAgentState(session, run, row.ThreadID, map[string]any{
			"status": string(WebSessionSubAgentInterrupted),
			"active": false,
		})
	}
}

func (m *Manager) handleDevinACPUpdate(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, raw json.RawMessage) {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	update, _ := payload["update"].(map[string]any)
	kind := stringValue(update["sessionUpdate"])
	// The private sub-agent extension rides on update._meta: started/completed
	// feed the sub-agent registry, and context tags every event projected from
	// this update with the owning sub-agent's id.
	contextID := m.applyDevinSubAgentMeta(session, run, decodeRawObject(update["_meta"]))
	m.observeDevinTokenSpeed(session, run, proj, update, contextID)
	switch kind {
	case "user_message_chunk":
		// Only history capture consumes replayed user messages; live runs
		// already recorded the user's message when it was sent.
		if !proj.captureHistory {
			return
		}
		text := devinChunkText(update)
		proj.mu.Lock()
		proj.pendingUserText.WriteString(text)
		proj.mu.Unlock()
	case "agent_thought_chunk":
		text := devinChunkText(update)
		if text == "" {
			return
		}
		proj.mu.Lock()
		thinkingOpen := proj.messages[contextID] != nil && proj.messages[contextID].thinkingID != ""
		proj.mu.Unlock()
		if !thinkingOpen && strings.TrimSpace(text) == "" {
			// Whitespace chunks are content inside an open thinking block, but
			// a stray one must not materialize an empty block on its own.
			return
		}
		messageID := m.ensureDevinMessage(session, run, proj, contextID)
		proj.mu.Lock()
		state := proj.messageState(contextID)
		if state.thinkingID == "" {
			state.thinkingIndex++
			state.thinkingID = fmt.Sprintf("devin-thinking:%s:%s:%d", run.runID, messageID, state.thinkingIndex)
			state.thinkingText.Reset()
			state.lastThinkEmit = time.Time{}
		}
		state.thinkingText.WriteString(text)
		thinkingID := state.thinkingID
		snapshot := state.thinkingText.String()
		now := time.Now()
		emit := state.lastThinkEmit.IsZero() || now.Sub(state.lastThinkEmit) >= piToolProgressInterval
		if emit {
			state.lastThinkEmit = now
		}
		proj.mu.Unlock()
		if !emit {
			return
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "tool_st", RunID: run.runID, ParentID: messageID, ThreadID: contextID,
			Timestamp: time.Now(), Payload: map[string]any{
				"tid": thinkingID, "name": "Reasoning", "kind": "reasoning",
				"out": truncateToolOutput("reasoning", snapshot), "ok": true,
			},
		})
	case "agent_message_chunk":
		text := devinChunkText(update)
		if text == "" {
			return
		}
		proj.mu.Lock()
		messageHasText := proj.messages[contextID] != nil && proj.messages[contextID].messageHasText
		proj.mu.Unlock()
		if !messageHasText && strings.TrimSpace(text) == "" {
			// Whitespace chunks only matter once the message has real text; a
			// leading one must not materialize an empty assistant bubble.
			return
		}
		m.finishDevinThinking(session, run, proj, contextID)
		messageID := m.ensureDevinMessage(session, run, proj, contextID)
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "txt_d", RunID: run.runID, ParentID: messageID, ThreadID: contextID,
			Timestamp: time.Now(), Payload: map[string]any{"mid": messageID, "txt": text},
		})
		run.markAssistantDeltaSeen(messageID)
		proj.mu.Lock()
		proj.messageState(contextID).messageHasText = true
		proj.mu.Unlock()
	case "tool_call":
		m.finishDevinThinking(session, run, proj, contextID)
		messageID := m.ensureDevinMessage(session, run, proj, contextID)
		toolID := firstNonEmpty(stringValue(update["toolCallId"]), utils.NewID())
		title := firstNonEmpty(stringValue(update["title"]), "Tool")
		rawKind := stringValue(update["kind"])
		if devinToolCallIsPlanExit(update) {
			// Devin's "Exit plan mode" call carries the finished plan in
			// rawInput.plan. Project it as a Plan card like Claude's
			// ExitPlanMode so the implement action can render while the
			// companion session/request_permission is still pending.
			run.markCompletedPlanTool()
			rawInput := decodeRawObject(update["rawInput"])
			planText := strings.TrimSpace(stringValue(rawInput["plan"]))
			meta := map[string]any{"title": "Plan", "kind": "plan", "acpKind": rawKind}
			if planPath := devinToolPlanPath(update); planPath != "" {
				meta["path"] = planPath
				meta["subtitle"] = planPath
			}
			proj.mu.Lock()
			proj.tools[toolID] = &devinToolState{
				name:     "Plan",
				kind:     "plan",
				input:    update["rawInput"],
				parentID: messageID,
				meta:     meta,
			}
			proj.mu.Unlock()
			_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
				ID: utils.NewID(), Type: "tool_st", RunID: run.runID, ParentID: messageID, ThreadID: contextID,
				Timestamp: time.Now(), Payload: map[string]any{
					"tid": toolID, "name": "Plan", "kind": "plan", "meta": meta, "ok": true,
				},
			})
			_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
				ID: utils.NewID(), Type: "tool_end", RunID: run.runID, ParentID: messageID, ThreadID: contextID,
				Timestamp: time.Now(), Payload: map[string]any{
					"tid": toolID, "name": "Plan", "kind": "plan",
					"out": planText, "ok": true, "status": "completed", "meta": meta,
				},
			})
			m.closeDevinMessage(session, run, proj, contextID)
			return
		}
		mappedKind := devinToolHistoryKind(rawKind)
		tool := &devinToolState{
			name:     title,
			kind:     mappedKind,
			input:    update["rawInput"],
			parentID: messageID,
			meta:     map[string]any{"acpKind": rawKind, "title": title},
		}
		proj.mu.Lock()
		proj.tools[toolID] = tool
		meta := tool.meta
		proj.mu.Unlock()
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "tool_st", RunID: run.runID, ParentID: messageID, ThreadID: contextID,
			Timestamp: time.Now(), Payload: map[string]any{
				"tid": toolID, "name": title, "kind": mappedKind,
				"in": update["rawInput"], "meta": meta, "ok": true,
			},
		})
		m.closeDevinMessage(session, run, proj, contextID)
	case "tool_call_update":
		toolID := firstNonEmpty(stringValue(update["toolCallId"]), utils.NewID())
		status := strings.ToLower(strings.TrimSpace(stringValue(update["status"])))
		proj.mu.Lock()
		tool := proj.tools[toolID]
		if tool == nil {
			parentID := ""
			if state := proj.messages[contextID]; state != nil {
				parentID = state.messageID
			}
			if parentID == "" && contextID == "" {
				parentID = run.assistantMessageIDSnapshot()
			}
			tool = &devinToolState{parentID: parentID, meta: map[string]any{}}
			proj.tools[toolID] = tool
		}
		if title := strings.TrimSpace(stringValue(update["title"])); title != "" {
			tool.name = title
			tool.meta["title"] = title
		}
		if rawKind := strings.TrimSpace(stringValue(update["kind"])); rawKind != "" {
			tool.meta["acpKind"] = rawKind
			tool.kind = devinToolHistoryKind(rawKind)
		}
		if rawInput, ok := update["rawInput"]; ok && rawInput != nil {
			tool.input = rawInput
		}
		name := firstNonEmpty(tool.name, "Tool")
		mappedKind := firstNonEmpty(tool.kind, devinToolHistoryKind(stringValue(tool.meta["acpKind"])), "dynamic_tool_call")
		input := tool.input
		parentID := tool.parentID
		meta := cloneMap(tool.meta)
		proj.mu.Unlock()
		if status != "completed" && status != "failed" && status != "cancelled" {
			return
		}
		output := devinToolOutputText(update)
		if mappedKind == "plan" && strings.TrimSpace(output) == "" {
			// The plan card's body comes from rawInput.plan on the tool_call;
			// the follow-up tool_call_update carries no output, so keep the
			// captured plan text instead of overwriting it with "".
			output = truncateToolOutput("plan", strings.TrimSpace(stringValue(decodeRawObject(input)["plan"])))
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "tool_end", RunID: run.runID, ParentID: parentID, ThreadID: contextID,
			Timestamp: time.Now(), Payload: map[string]any{
				"tid": toolID, "name": name, "kind": mappedKind,
				"in": input, "out": output, "ok": status == "completed",
				"status": stringValue(update["status"]), "meta": meta,
			},
		})
	case "plan":
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "plan", RunID: run.runID, ParentID: run.assistantMessageIDSnapshot(), ThreadID: contextID,
			Timestamp: time.Now(), Payload: update,
		})
	case "usage_update":
		m.handleDevinUsageUpdate(session, run, proj, update)
	}
}

// devinUsageUpdateValues holds the token accounting extracted from a
// usage_update session update. Devin's inputTokens already includes cached
// reads and writes, matching the session model's input semantics.
type devinUsageUpdateValues struct {
	input       int64
	cachedInput int64
	output      int64
	used        int64
	size        int64
	cost        float64
	costUSD     bool
	hasTokens   bool
	acu         float64
	hasAcu      bool
	credit      float64
	hasCredit   bool
}

func devinUsageToken(meta map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		if value, ok := meta[key]; ok {
			return int64(numberValue(value)), true
		}
	}
	return 0, false
}

func devinUsageQuota(meta map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := meta[key]; ok {
			return numberValue(value), true
		}
	}
	return 0, false
}

func parseDevinUsageUpdate(update map[string]any) devinUsageUpdateValues {
	meta := decodeRawObject(update["_meta"])
	var values devinUsageUpdateValues
	in, hasIn := devinUsageToken(meta, "cognition.ai/inputTokens", "inputTokens")
	out, hasOut := devinUsageToken(meta, "cognition.ai/outputTokens", "outputTokens")
	cachedRead, hasCachedRead := devinUsageToken(meta, "cognition.ai/cachedReadTokens", "cachedReadTokens")
	cachedWrite, hasCachedWrite := devinUsageToken(meta, "cognition.ai/cachedWriteTokens", "cachedWriteTokens")
	values.input = in
	values.cachedInput = cachedRead + cachedWrite
	values.output = out
	values.hasTokens = hasIn || hasOut || hasCachedRead || hasCachedWrite
	values.acu, values.hasAcu = devinUsageQuota(meta, "cognition.ai/totalAcuCost", "totalAcuCost")
	values.credit, values.hasCredit = devinUsageQuota(meta, "cognition.ai/totalCreditCost", "totalCreditCost")
	values.used = int64(numberValue(update["used"]))
	values.size = int64(numberValue(update["size"]))
	if cost := decodeRawObject(update["cost"]); len(cost) > 0 {
		values.cost = numberValue(cost["amount"])
		currency := strings.TrimSpace(stringValue(cost["currency"]))
		values.costUSD = currency == "" || strings.EqualFold(currency, "USD")
	}
	return values
}

// devinUsageSignature fingerprints one usage_update report. Only accounting
// fields participate — context tags like subagent_context must not split the
// signature, because the agent emits a bare copy plus a context-tagged copy
// of the same report.
func devinUsageSignature(update map[string]any) string {
	meta := decodeRawObject(update["_meta"])
	parts := make([]string, 0, 10)
	for _, pair := range [][2]string{
		{"used", ""},
		{"size", ""},
		{"cognition.ai/inputTokens", "meta"},
		{"inputTokens", "meta"},
		{"cognition.ai/outputTokens", "meta"},
		{"outputTokens", "meta"},
		{"cognition.ai/cachedReadTokens", "meta"},
		{"cachedReadTokens", "meta"},
		{"cognition.ai/cachedWriteTokens", "meta"},
		{"cachedWriteTokens", "meta"},
	} {
		var value any
		var ok bool
		if pair[1] == "meta" {
			value, ok = meta[pair[0]]
		} else {
			value, ok = update[pair[0]]
		}
		if ok {
			parts = append(parts, pair[0]+"="+strconv.FormatFloat(numberValue(value), 'f', -1, 64))
		}
	}
	if cost := decodeRawObject(update["cost"]); len(cost) > 0 {
		parts = append(parts, "cost="+strconv.FormatFloat(numberValue(cost["amount"]), 'f', -1, 64)+stringValue(cost["currency"]))
	}
	return strings.Join(parts, "|")
}

// handleDevinUsageUpdate projects a usage_update session update into the
// session's token accounting, mirroring handleCodexAppServerUsage: per-request
// token counts accumulate into the totals while used/size feed the context
// estimate and window. The agent emits each report twice — once bare and once
// tagged with the owning sub-agent context — so identical payloads are
// deduped per run. Reports always fold into the session totals: the token
// spend belongs to one billable session regardless of which agent context
// issued the request, and the bare/context-tagged copies may arrive in either
// order.
func (m *Manager) handleDevinUsageUpdate(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, update map[string]any) {
	signature := devinUsageSignature(update)
	if signature == "" || proj.recordDevinUsageSignature(signature) {
		return
	}
	values := parseDevinUsageUpdate(update)
	now := time.Now()
	updates := map[string]any{"updated_at": now}
	if values.hasTokens {
		updates["total_input_tokens"] = gorm.Expr("total_input_tokens + ?", values.input)
		updates["total_cached_input_tokens"] = gorm.Expr("total_cached_input_tokens + ?", values.cachedInput)
		updates["total_output_tokens"] = gorm.Expr("total_output_tokens + ?", values.output)
	}
	if values.used > 0 {
		updates["latest_token_count_input_tokens"] = values.input
		updates["latest_token_count_cached_input_tokens"] = values.cachedInput
		updates["latest_token_count_output_tokens"] = values.output
		updates["latest_token_count_total_tokens"] = values.used
		updates["latest_token_count_updated_at"] = now
	}
	if values.size > 0 {
		updates["session_context_window_tokens"] = values.size
		updates["session_context_window_observed_at"] = now
	}
	costUSD := values.costUSD && values.cost > 0
	if costUSD {
		updates["total_cost"] = gorm.Expr("total_cost + ?", values.cost)
	}
	// The agent reports session-cumulative quota totals, so they replace the
	// stored values rather than accumulating per request.
	if values.hasAcu {
		updates["total_acu_cost"] = values.acu
	}
	if values.hasCredit {
		updates["total_credit_cost"] = values.credit
	}
	_ = m.updateRuntimeState(context.Background(), session.ID, updates)
	eventPayload := map[string]any{
		"in":  values.input,
		"cin": values.cachedInput,
		"out": values.output,
	}
	if values.size > 0 {
		eventPayload["cwt"] = values.size
	}
	if costUSD {
		eventPayload["cost"] = values.cost
	}
	if values.hasAcu {
		eventPayload["acu"] = values.acu
	}
	if values.hasCredit {
		eventPayload["crd"] = values.credit
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "usage", RunID: run.runID, Timestamp: now, Payload: eventPayload,
	})
	if values.size > 0 || values.hasAcu || values.hasCredit {
		m.broadcastSessionSummary(context.Background(), session.ID)
	}
}

// applyDevinPromptUsageFallback folds the session/prompt response's usage
// block into the session accounting when the agent never streamed a
// usage_update (older CLI versions). Both channels report per-request counts,
// so a streamed update takes precedence to avoid double counting.
func (m *Manager) applyDevinPromptUsageFallback(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, raw json.RawMessage) {
	if len(raw) == 0 || proj.sawDevinUsageUpdate() {
		return
	}
	usage := decodeRawObject(decodeRawObject(raw)["usage"])
	if output, ok := devinUsageToken(usage, "outputTokens"); ok {
		m.updateTokenSpeed(session.ID, run.runID, func(s *tokenSpeedState, now time.Time) {
			// A whole-prompt total can only be attributed when exactly one root
			// generation was observed and no subagent contributed output.
			if s.serial == 1 && !s.devinAmbiguous {
				s.finish("", &output, now, false)
			}
		}, true)
	}
	in := int64(numberValue(usage["inputTokens"]))
	out := int64(numberValue(usage["outputTokens"]))
	cin := int64(numberValue(usage["cachedReadTokens"])) + int64(numberValue(usage["cachedWriteTokens"]))
	used := int64(numberValue(usage["totalTokens"]))
	acu, hasAcu := devinUsageQuota(usage, "totalAcuCost", "acuCost")
	credit, hasCredit := devinUsageQuota(usage, "totalCreditCost", "creditCost")
	if in <= 0 && out <= 0 && cin <= 0 && used <= 0 && !hasAcu && !hasCredit {
		return
	}
	now := time.Now()
	updates := map[string]any{
		"total_input_tokens":        gorm.Expr("total_input_tokens + ?", in),
		"total_cached_input_tokens": gorm.Expr("total_cached_input_tokens + ?", cin),
		"total_output_tokens":       gorm.Expr("total_output_tokens + ?", out),
		"updated_at":                now,
	}
	if hasAcu {
		updates["total_acu_cost"] = acu
	}
	if hasCredit {
		updates["total_credit_cost"] = credit
	}
	if used > 0 {
		updates["latest_token_count_input_tokens"] = in
		updates["latest_token_count_cached_input_tokens"] = cin
		updates["latest_token_count_output_tokens"] = out
		updates["latest_token_count_total_tokens"] = used
		updates["latest_token_count_updated_at"] = now
	}
	_ = m.updateRuntimeState(context.Background(), session.ID, updates)
	eventPayload := map[string]any{"in": in, "cin": cin, "out": out}
	if hasAcu {
		eventPayload["acu"] = acu
	}
	if hasCredit {
		eventPayload["crd"] = credit
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "usage", RunID: run.runID, Timestamp: now,
		Payload: eventPayload,
	})
}

// isDevinCompactionNotification matches the compaction extension notification
// method; on the wire the agent prefixes private notifications with "_".
func isDevinCompactionNotification(method string) bool {
	return method == "_cognition.ai/compaction" || method == "cognition.ai/compaction"
}

// isDevinQuotaStatsNotification matches the private notifications that carry
// per-turn quota stats: cognition.ai/turn_stats fires once per request and
// cognition.ai/agent_stopped repeats the last turn's stats under params.stats.
func isDevinQuotaStatsNotification(method string) bool {
	switch method {
	case "_cognition.ai/turn_stats", "cognition.ai/turn_stats",
		"_cognition.ai/agent_stopped", "cognition.ai/agent_stopped":
		return true
	}
	return false
}

// handleDevinQuotaStatsNotification folds acuCost/creditCost from turn_stats
// and agent_stopped into the session totals. usage_update only reports token
// counts, so per-request quota costs ride on these notifications instead —
// acuCost/creditCost are per-request deltas here, while usage_update's
// totalAcuCost/totalCreditCost remain cumulative and overwrite the sums.
// agent_stopped repeats the last turn's stats under params.stats, so reports
// dedupe by turnRequestId/requestId.
func (m *Manager) handleDevinQuotaStatsNotification(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, raw json.RawMessage) {
	params := decodeRawObject(raw)
	stats := params
	if nested := decodeRawObject(params["stats"]); len(nested) > 0 {
		stats = nested
	}
	acu, hasAcu := devinUsageQuota(stats, "acuCost", "cognition.ai/acuCost")
	credit, hasCredit := devinUsageQuota(stats, "creditCost", "cognition.ai/creditCost")
	if (!hasAcu || acu <= 0) && (!hasCredit || credit <= 0) {
		return
	}
	requestID := stringValue(params["turnRequestId"])
	if requestID == "" {
		requestID = stringValue(stats["requestId"])
	}
	if requestID == "" {
		requestID = stringValue(params["requestId"])
	}
	if proj.recordDevinTurnStatRequest(requestID) {
		return
	}
	updates := map[string]any{"updated_at": time.Now()}
	if hasAcu && acu > 0 {
		updates["total_acu_cost"] = gorm.Expr("total_acu_cost + ?", acu)
	}
	if hasCredit && credit > 0 {
		updates["total_credit_cost"] = gorm.Expr("total_credit_cost + ?", credit)
	}
	_ = m.updateRuntimeState(context.Background(), session.ID, updates)
	eventPayload := map[string]any{}
	if hasAcu && acu > 0 {
		eventPayload["acu"] = acu
	}
	if hasCredit && credit > 0 {
		eventPayload["crd"] = credit
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "usage", RunID: run.runID, Timestamp: time.Now(),
		Payload: eventPayload,
	})
	m.broadcastSessionSummary(context.Background(), session.ID)
}

// handleDevinCompactionNotification projects cognition.ai/compaction
// extension notifications into a context_compaction tool card and resets the
// context estimate baseline once a compaction completes, matching how Codex
// surfaces its context_compaction item.
func (m *Manager) handleDevinCompactionNotification(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, raw json.RawMessage) {
	params := decodeRawObject(raw)
	status := strings.ToLower(strings.TrimSpace(stringValue(params["status"])))
	summary := strings.TrimSpace(stringValue(params["summary"]))
	now := time.Now()
	emitStart := func() {
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "tool_st", RunID: run.runID, ParentID: run.assistantMessageIDSnapshot(),
			Timestamp: now, Payload: map[string]any{
				"tid": proj.ensureDevinCompactionToolID(), "name": "ContextCompaction",
				"kind": "context_compaction", "ok": true,
			},
		})
	}
	switch status {
	case "started":
		emitStart()
	case "completed", "failed":
		// Devin also reports "completed" with an empty summary the moment the
		// history snapshot is dumped, minutes before the real summary arrives.
		// Surface that ping as progress so one logical compaction stays one card.
		if status == "completed" && summary == "" {
			emitStart()
			return
		}
		toolID := firstNonEmpty(proj.takeDevinCompactionToolID(), utils.NewID())
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "tool_end", RunID: run.runID, ParentID: run.assistantMessageIDSnapshot(),
			Timestamp: now, Payload: map[string]any{
				"tid": toolID, "name": "ContextCompaction", "kind": "context_compaction",
				"out": truncateToolOutput("context_compaction", summary),
				"ok":  status == "completed", "status": status,
			},
		})
		if status == "completed" {
			if record, err := m.GetSession(context.Background(), session.ID); err == nil {
				_ = m.updateRuntimeState(context.Background(), session.ID, contextEstimateBaselineResetUpdate(record, now))
				m.broadcastSessionSummary(context.Background(), session.ID)
			}
		}
	}
}

// devinSessionModeID maps the session's workflow mode and permission level
// onto the agent's ACP session modes. Plan mode overrides the permission
// level, matching how Claude's --permission-mode plan wins over its flags.
func devinSessionModeID(session tables.WebSessionTable) string {
	if normalizeWorkflowMode(effectiveWorkflowMode(session)) == WorkflowModePlan {
		return "plan"
	}
	switch normalizePermissionLevel(effectivePermissionLevel(session)) {
	case PermissionLevelYolo:
		return "bypass"
	case PermissionLevelDefault:
		return "accept-edits"
	default:
		return "smart"
	}
}

// applyDevinSessionMode pushes the mapped mode to the agent via
// session/set_mode. Unknown mode ids are silently reset to the agent default,
// so the request is only sent when the target is in availableModes.
func (m *Manager) applyDevinSessionMode(ctx context.Context, client *devinACPClient, session tables.WebSessionTable, nativeSessionID string, modes *devinACPSessionModes) {
	if client == nil {
		return
	}
	if modes == nil {
		client.setSessionModes(nativeSessionID, nil, false)
		return
	}
	target := devinSessionModeID(session)
	applied := modes.CurrentModeID == target
	if !applied && modes.AvailableModeIDs[target] {
		if _, err := client.request(ctx, "session/set_mode", map[string]any{
			"sessionId": nativeSessionID,
			"modeId":    target,
		}); err != nil {
			if m.logger != nil {
				m.logger.Warn("Devin ACP session/set_mode failed",
					zap.String("sessionId", session.ID),
					zap.String("modeId", target),
					zap.Error(err))
			}
		} else {
			applied = true
			modes.CurrentModeID = target
		}
	}
	client.setSessionModes(nativeSessionID, modes, applied)
}

// syncDevinSessionMode re-applies the mapped mode on a live ACP client after
// the workflow mode or permission level changed mid-run.
func (m *Manager) syncDevinSessionMode(ctx context.Context, sessionID string) {
	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	if run == nil || run.backend != SessionBackendDevinACP {
		return
	}
	if pending, ok := run.pendingServerRequest(); ok && pending.Kind == pendingServerRequestPlanApproval {
		// A pending "Exit plan mode" request owns the mode transition; pushing
		// session/set_mode now would race the agent's own switch on approval.
		return
	}
	client := run.devinACP()
	if client == nil {
		return
	}
	modes, nativeSessionID := client.sessionModesSnapshot()
	if modes == nil || strings.TrimSpace(nativeSessionID) == "" {
		return
	}
	record, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	m.applyDevinSessionMode(ctx, client, record, nativeSessionID, modes)
}

func (m *Manager) handleDevinPermissionRequest(client *devinACPClient, session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, message devinACPMessage) {
	// The event loop holds the session snapshot from the start of the run.
	// Honor permission changes made while a previous request was pending.
	current, err := m.GetSession(context.Background(), session.ID)
	if err != nil {
		_ = client.respond(message.ID, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
		return
	}
	session = current
	var params map[string]any
	_ = json.Unmarshal(message.Params, &params)
	options, _ := params["options"].([]any)
	planExit := devinPermissionIsPlanExit(params)
	// Yolo keeps the client-side auto-answer as a safety net (bypass should
	// not produce requests at all). Elevated falls back to it only when the
	// agent-side smart mode could not be applied — without it every request
	// would reach the user, the legacy behavior for agents without modes.
	// Exiting plan mode always asks the user, matching Claude/Codex.
	autoApprove := !planExit && (effectivePermissionLevel(session) == PermissionLevelYolo ||
		(effectivePermissionLevel(session) == PermissionLevelElevated && !client.modeAppliedSnapshot()))
	if !autoApprove {
		itemID := strings.TrimSpace(stringValue(decodeRawObject(params["toolCall"])["toolCallId"]))
		wantKind := pendingServerRequestCommandApproval
		if planExit {
			wantKind = pendingServerRequestPlanApproval
		}
		// session/load replays and re-attach re-requests resend the same
		// request_permission; keep the single pending request but retarget the
		// response at the latest JSON-RPC id instead of stacking duplicate
		// approval cards in the timeline.
		if existing, ok := run.pendingServerRequest(); ok && existing.Kind == wantKind && itemID != "" && existing.ItemID == itemID {
			existing.RawID = append(json.RawMessage(nil), message.ID...)
			existing.Permissions = params
			// The retried request may now resolve tool details that were
			// missing when the pending request was first recorded.
			tracked := proj.devinToolSnapshot(itemID)
			if wantKind != pendingServerRequestPlanApproval &&
				(existing.Prompt == "" || existing.Prompt == devinPermissionFallbackPrompt) {
				if prompt := devinPermissionPrompt(params, tracked); prompt != "" {
					existing.Prompt = prompt
				}
			}
			if existing.Command == "" {
				existing.Command = devinPermissionCommand(params, tracked)
			}
			run.setPendingServerRequest(existing)
			return
		}
		tracked := proj.devinToolSnapshot(itemID)
		now := time.Now()
		request := &pendingServerRequest{
			RawID:       append(json.RawMessage(nil), message.ID...),
			Kind:        wantKind,
			ItemID:      itemID,
			Prompt:      devinPermissionPrompt(params, tracked),
			Command:     devinPermissionCommand(params, tracked),
			RequestedAt: &now,
			Permissions: params,
		}
		assistantState := AssistantStateWaitingApproval
		if planExit {
			request.Prompt = "Exit plan mode"
			run.markCompletedPlanTool()
			assistantState = AssistantStateWaitingPlanApproval
		}
		run.setPendingServerRequest(request)
		m.pauseActiveCallTimeout(run)
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{ID: utils.NewID(), Type: "approval_req", RunID: run.runID, ParentID: run.assistantMessageID, Timestamp: now, Payload: map[string]any{"kind": string(request.Kind), "iid": request.ItemID, "prompt": request.Prompt, "command": request.Command}})
		_ = m.updateRuntimeState(context.Background(), session.ID, applyAssistantStateUpdates(map[string]any{"updated_at": now}, assistantState, now))
		m.broadcastSessionSummary(context.Background(), session.ID)
		return
	}
	selected := ""
	for _, raw := range options {
		option, _ := raw.(map[string]any)
		if strings.Contains(stringValue(option["kind"]), "allow") {
			selected = stringValue(option["optionId"])
			break
		}
	}
	outcome := map[string]any{"outcome": "cancelled"}
	if selected != "" {
		outcome = map[string]any{"outcome": "selected", "optionId": selected}
	}
	_ = client.respond(message.ID, map[string]any{"outcome": outcome})
}

// devinToolCallIsPlanExit reports whether a switch_mode tool call is Devin's
// "Exit plan mode" (any target other than plan). A switch INTO plan mode is
// not a completed plan.
func devinToolCallIsPlanExit(update map[string]any) bool {
	if !strings.EqualFold(strings.TrimSpace(stringValue(update["kind"])), "switch_mode") {
		return false
	}
	modeID := strings.TrimSpace(stringValue(decodeRawObject(update["rawInput"])["modeId"]))
	return modeID != "plan"
}

// devinPermissionIsPlanExit reports whether a session/request_permission
// call is Devin's "Exit plan mode" gate: its options use plan_ prefixed ids
// (plan_accept_edits, plan_bypass) and/or the tool call is a switch_mode
// targeting a non-plan mode.
func devinPermissionIsPlanExit(params map[string]any) bool {
	if options, ok := params["options"].([]any); ok {
		for _, raw := range options {
			if strings.HasPrefix(stringValue(decodeRawObject(raw)["optionId"]), "plan_") {
				return true
			}
		}
	}
	return devinToolCallIsPlanExit(decodeRawObject(params["toolCall"]))
}

const devinPermissionFallbackPrompt = "Devin is waiting for permission to continue."

func devinPermissionPrompt(params map[string]any, tracked *devinToolState) string {
	if reason := strings.TrimSpace(stringValue(params["reason"])); reason != "" {
		return reason
	}
	if title := strings.TrimSpace(stringValue(decodeRawObject(params["toolCall"])["title"])); title != "" {
		return title
	}
	if tracked != nil {
		if name := strings.TrimSpace(tracked.name); name != "" {
			return name
		}
	}
	if kind := strings.TrimSpace(stringValue(decodeRawObject(params["toolCall"])["kind"])); kind != "" {
		return fmt.Sprintf("Devin is waiting for approval to use %s.", kind)
	}
	return devinPermissionFallbackPrompt
}

func devinPermissionCommand(params map[string]any, tracked *devinToolState) string {
	if command := devinToolInputCommand(decodeRawObject(params["toolCall"])["rawInput"]); command != "" {
		return command
	}
	if tracked != nil {
		return devinToolInputCommand(tracked.input)
	}
	return ""
}

func devinToolInputCommand(rawInput any) string {
	input := decodeRawObject(rawInput)
	return strings.TrimSpace(firstNonEmpty(stringValue(input["command"]), stringValue(input["cmd"])))
}

func devinPermissionResponsePayload(action string, request *pendingServerRequest, session tables.WebSessionTable) any {
	if action == "reject" {
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	}
	optionID := ""
	if request != nil {
		options, _ := request.Permissions["options"].([]any)
		// Plan exit offers mode-specific options: bypass keeps yolo sessions
		// unrestricted, everything else exits into accept-edits.
		if request.Kind == pendingServerRequestPlanApproval {
			preferred := "plan_accept_edits"
			if effectivePermissionLevel(session) == PermissionLevelYolo {
				preferred = "plan_bypass"
			}
			for _, raw := range options {
				option, _ := raw.(map[string]any)
				if stringValue(option["optionId"]) == preferred {
					optionID = preferred
					break
				}
			}
		}
		if optionID == "" {
			for _, raw := range options {
				option, _ := raw.(map[string]any)
				if strings.Contains(stringValue(option["kind"]), "allow") {
					optionID = stringValue(option["optionId"])
					break
				}
			}
		}
	}
	if optionID == "" {
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	}
	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}
}

// devinPendingPlanApproval returns the run's pending "Exit plan mode"
// request, if the active run is a Devin ACP run blocked on one.
func (m *Manager) devinPendingPlanApproval(sessionID string) (*activeRun, *pendingServerRequest) {
	m.mu.RLock()
	run := m.runs[sessionID]
	m.mu.RUnlock()
	if run == nil || run.backend != SessionBackendDevinACP || run.devinACP() == nil {
		return nil, nil
	}
	pending, ok := run.pendingApprovalRequest()
	if !ok || pending.Kind != pendingServerRequestPlanApproval {
		return nil, nil
	}
	return run, pending
}

// resolveDevinPlanApprovalForSend handles a message sent while a Devin run is
// blocked on the "Exit plan mode" permission. The committed workflow mode
// carries the intent: the implement action switches the session to the
// default workflow before sending, so the request is approved in place and
// the agent continues the same run; any other message is feedback, so the
// exit is declined and the text is queued for the next turn.
// Returns handled=true when the send was consumed by this flow.
func (m *Manager) resolveDevinPlanApprovalForSend(
	ctx context.Context,
	sessionID string,
	record tables.WebSessionTable,
	text string,
	attachments []Attachment,
	attachmentIDs []string,
) (bool, error) {
	run, pending := m.devinPendingPlanApproval(sessionID)
	if run == nil || pending == nil {
		return false, nil
	}
	approve := effectiveWorkflowMode(record) == WorkflowModeDefault
	action := "reject"
	if approve {
		action = "approve"
	}
	if err := run.devinACP().respond(pending.RawID, devinPermissionResponsePayload(action, pending, record)); err != nil {
		return true, err
	}
	run.clearPendingServerRequest()
	run.clearCompletedPlanTool()
	m.resumeActiveCallTimeout(run)

	now := time.Now()
	_, _ = m.appendAndBroadcast(ctx, sessionID, record, Event{
		ID: utils.NewID(), Type: "approval_res", RunID: run.runID, ParentID: run.assistantMessageID, Timestamp: now,
		Payload: map[string]any{"act": action, "prompt": pending.Prompt, "command": pending.Command},
	})
	if !approve {
		// Stay in plan mode: the message is queued and dispatched as the next
		// prompt once this turn ends.
		_, err := m.queuePendingInput(sessionID, text, attachmentIDs, PendingInputModeQueue, "")
		m.broadcastSessionSummary(ctx, sessionID)
		return true, err
	}
	userMessageID := utils.NewID()
	_, _ = m.appendAndBroadcast(ctx, sessionID, record, Event{
		ID: utils.NewID(), Type: "msg_u", RunID: run.runID, ParentID: userMessageID, Timestamp: now,
		Payload: map[string]any{
			"mid":  userMessageID,
			"txt":  text,
			"atts": attachmentPayloads(attachments),
		},
	})
	_ = m.updateRuntimeState(ctx, sessionID, applyAssistantStateUpdates(map[string]any{"updated_at": now}, AssistantStateWorking, now))
	m.broadcastSessionSummary(ctx, sessionID)
	// syncDevinSessionMode was skipped while the plan approval was pending;
	// push the mapped mode now that the request is resolved.
	m.syncDevinSessionMode(ctx, sessionID)
	return true, nil
}

func (m *Manager) finishDevinRun(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection) {
	m.interruptActiveDevinSubAgents(session, run)
	m.closeAllDevinMessages(session, run, proj)
	_ = m.finalizeLatestTurnUsage(context.Background(), session.ID)
	finalStatus, finalAssistantState := m.completedRunState(context.Background(), session, run)
	now := time.Now()
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{ID: utils.NewID(), Type: "run_done", RunID: run.runID, Timestamp: now, Payload: map[string]any{"ok": true, "st": string(finalStatus)}})
	_ = m.updateRuntimeState(context.Background(), session.ID, applyAssistantStateUpdates(map[string]any{"status": string(finalStatus), "updated_at": now, "auto_retry_attempt": 0, "auto_retry_next_at": nil, "auto_retry_last_error_code": nil}, finalAssistantState, now))
	m.cancelAutoRetryTimer(session.ID)
	m.broadcastSessionSummary(context.Background(), session.ID)
	run.syncSourceAfterRun = true
}

func (c *devinACPClient) terminalCreate(params map[string]any, root string) (map[string]any, error) {
	command := strings.TrimSpace(stringValue(params["command"]))
	if command == "" {
		return nil, errors.New("terminal command is empty")
	}
	cwd := filepath.Clean(firstNonEmpty(stringValue(params["cwd"]), root))
	if !pathWithin(root, cwd) {
		return nil, errors.New("terminal cwd is outside the session workspace")
	}
	args := rawStringSlice(params["args"])
	cmd := devinShellCommand(c.ctx, c.shell, command, args)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	terminalID := utils.NewID()
	terminal := &devinACPTerminal{id: terminalID, cmd: cmd, done: make(chan struct{})}
	writer := devinACPTerminalWriter{terminal: terminal}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.terminalMu.Lock()
	c.terminals[terminalID] = terminal
	c.terminalMu.Unlock()
	go func() {
		err := cmd.Wait()
		terminal.outputMu.Lock()
		terminal.exitErr = err
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			terminal.exitCode = &code
		}
		terminal.outputMu.Unlock()
		close(terminal.done)
	}()
	return map[string]any{"terminalId": terminalID}, nil
}

func (c *devinACPClient) terminalOutput(params map[string]any, wait bool) (map[string]any, error) {
	id := strings.TrimSpace(stringValue(params["terminalId"]))
	c.terminalMu.Lock()
	terminal := c.terminals[id]
	c.terminalMu.Unlock()
	if terminal == nil {
		return nil, errors.New("terminal not found")
	}
	if wait {
		select {
		case <-terminal.done:
		case <-c.ctx.Done():
			return nil, c.ctx.Err()
		}
	}
	terminal.outputMu.Lock()
	defer terminal.outputMu.Unlock()
	result := map[string]any{"output": terminal.output.String(), "truncated": false}
	if terminal.exitCode != nil {
		result["exitStatus"] = map[string]any{"exitCode": *terminal.exitCode, "signal": nil}
	}
	return result, nil
}

func (c *devinACPClient) terminalKill(params map[string]any) error {
	id := strings.TrimSpace(stringValue(params["terminalId"]))
	c.terminalMu.Lock()
	terminal := c.terminals[id]
	c.terminalMu.Unlock()
	if terminal == nil {
		return errors.New("terminal not found")
	}
	if terminal.cmd.Process != nil {
		return terminal.cmd.Process.Kill()
	}
	return nil
}

func (c *devinACPClient) terminalRelease(params map[string]any) error {
	id := strings.TrimSpace(stringValue(params["terminalId"]))
	c.terminalMu.Lock()
	delete(c.terminals, id)
	c.terminalMu.Unlock()
	return nil
}

// devinShellCommand runs the ACP terminal command through the resolved shell so
// command lines that arrive as a single string (e.g. "git status && git log")
// work, as do plain (command, args) pairs. shell is the resolved shell command
// (binary + startup args) from utils.ResolveShellCommand; when empty it falls
// back to cmd /c on Windows and sh -c elsewhere.
func devinShellCommand(ctx context.Context, shell []string, command string, args []string) *exec.Cmd {
	line := strings.TrimSpace(command)
	for _, arg := range args {
		line += " " + devinShellQuoteArg(arg)
	}
	if len(shell) == 0 {
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "cmd", "/c", line)
		}
		return exec.CommandContext(ctx, "sh", "-c", line)
	}
	shellArgs := append([]string(nil), shell[1:]...)
	switch name := strings.ToLower(filepath.Base(shell[0])); {
	case strings.HasPrefix(name, "cmd"):
		shellArgs = append(shellArgs, "/c", line)
	case strings.Contains(name, "powershell") || strings.Contains(name, "pwsh"):
		shellArgs = append(shellArgs, "-NoProfile", "-Command", line)
	default:
		shellArgs = append(shellArgs, "-c", line)
	}
	return exec.CommandContext(ctx, shell[0], shellArgs...)
}

func devinShellQuoteArg(arg string) string {
	if runtime.GOOS == "windows" {
		if arg == "" {
			return `""`
		}
		if !strings.ContainsAny(arg, " \t&|<>\"^%") {
			return arg
		}
		return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
	}
	return `'` + strings.ReplaceAll(arg, `'`, `'\''`) + `'`
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) && !filepath.IsAbs(rel)
}

func rawStringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, stringValue(item))
	}
	return result
}
