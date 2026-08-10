package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/x/xpty"
	"github.com/tuzig/vt10x"
	"go.uber.org/zap"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2"
	"code-kanban/utils/process"
)

// SessionStatus describes the lifecycle stage of a terminal session.
type SessionStatus string

const (
	SessionStatusStarting SessionStatus = "starting"
	SessionStatusRunning  SessionStatus = "running"
	SessionStatusClosed   SessionStatus = "closed"
	SessionStatusError    SessionStatus = "error"
)

// ErrInvalidEncoding indicates an unsupported encoding setting.
var ErrInvalidEncoding = errors.New("terminal: invalid encoding")

// SessionSnapshot captures immutable fields for API responses.
type SessionSnapshot struct {
	ID            string
	ProjectID     string
	WorktreeID    string
	WorkingDir    string
	Title         string
	OrderIndex    float64
	CreatedAt     time.Time
	LastActive    time.Time
	Status        SessionStatus
	Rows          int
	Cols          int
	Encoding      string
	TerminalModes *TerminalModesSnapshot `json:"terminalModes,omitempty"`
	// Process information
	ProcessPID         int32  `json:"processPid,omitempty"`
	ProcessStatus      string `json:"processStatus,omitempty"`
	ProcessHasChildren bool   `json:"processHasChildren,omitempty"`
	RunningCommand     string `json:"runningCommand,omitempty"`
	// AI Assistant information
	AIAssistant *ai_assistant2.AIAssistantInfo `json:"aiAssistant"`
	Traffic     *SessionTrafficStats           `json:"traffic,omitempty"`
}

type StreamEventType string

const (
	StreamEventData     StreamEventType = "data"
	StreamEventExit     StreamEventType = "exit"
	StreamEventMetadata StreamEventType = "metadata"
	StreamEventModes    StreamEventType = "modes"
)

type StreamEvent struct {
	Type     StreamEventType
	Data     []byte
	Err      error
	Metadata *SessionMetadata
	Modes    *TerminalModesSnapshot
}

type SessionMetadata struct {
	Title              string                         `json:"title,omitempty"`
	ProcessPID         int32                          `json:"processPid,omitempty"`
	ProcessStatus      string                         `json:"processStatus,omitempty"`
	ProcessHasChildren bool                           `json:"processHasChildren,omitempty"`
	RunningCommand     string                         `json:"runningCommand,omitempty"`
	AIAssistant        *ai_assistant2.AIAssistantInfo `json:"aiAssistant,omitempty"`
}

type TerminalStateCell struct {
	Char      string `json:"char,omitempty"`
	Mode      int16  `json:"mode"`
	FG        uint32 `json:"fg,omitempty"`
	BG        uint32 `json:"bg,omitempty"`
	FGDefault bool   `json:"fgDefault,omitempty"`
	BGDefault bool   `json:"bgDefault,omitempty"`
}

type TerminalStateSnapshot struct {
	Rows            int                   `json:"rows"`
	Cols            int                   `json:"cols"`
	Cells           [][]TerminalStateCell `json:"cells"`
	CursorX         int                   `json:"cursorX"`
	CursorY         int                   `json:"cursorY"`
	CursorVisible   bool                  `json:"cursorVisible"`
	CursorMode      int16                 `json:"cursorMode"`
	CursorFG        uint32                `json:"cursorFg,omitempty"`
	CursorBG        uint32                `json:"cursorBg,omitempty"`
	CursorFGDefault bool                  `json:"cursorFgDefault,omitempty"`
	CursorBGDefault bool                  `json:"cursorBgDefault,omitempty"`
	CapturedAt      time.Time             `json:"capturedAt,omitempty"`
}

type TerminalModesSnapshot struct {
	MouseTracking   string `json:"mouseTracking,omitempty"`
	MouseSGR        bool   `json:"mouseSgr,omitempty"`
	FocusReporting  bool   `json:"focusReporting,omitempty"`
	BracketedPaste  bool   `json:"bracketedPaste,omitempty"`
	AlternateScreen string `json:"alternateScreen,omitempty"`
}

type SessionStream struct {
	id     string
	events <-chan StreamEvent
	cancel context.CancelFunc
}

func (s *SessionStream) Events() <-chan StreamEvent {
	if s == nil {
		return nil
	}
	return s.events
}

func (s *SessionStream) Close() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
}

type sessionSubscriber struct {
	id     string
	ch     chan StreamEvent
	cancel context.CancelFunc
	once   sync.Once
}

const (
	subscriberBufferSize = 128

	// Metadata polling interval levels
	MetadataIntervalShort  = 2 * time.Second  // Active usage
	MetadataIntervalMedium = 10 * time.Second // Moderate inactivity
	MetadataIntervalLong   = 50 * time.Second // Extended inactivity

	// Number of ticks before moving to the next interval level
	intervalDowngradeThreshold = 5
)

// Session encapsulates a PTY-backed terminal command.
type Session struct {
	id                string
	projectID         string
	worktreeID        string
	workingDir        string
	initialWorkingDir string
	title             string
	command           []string
	env               []string
	rows              int
	cols              int
	orderIndex        float64

	createdAt  time.Time
	lastActive atomic.Int64
	status     atomic.Value

	cmd    *exec.Cmd
	pty    xpty.Pty
	cancel context.CancelFunc

	closeOnce sync.Once
	closed    chan struct{}
	err       atomic.Value

	logger   *zap.Logger
	encoding encoding.Encoding
	encName  string

	terminalStateEnabled         atomic.Bool
	terminalStateSuppressReplies atomic.Bool

	mu sync.RWMutex

	scrollMu             sync.RWMutex
	scrollback           [][]byte
	scrollbackTimestamps []time.Time
	scrollbackSize       int
	scrollbackLimit      int

	trafficMu            sync.Mutex
	trafficBuckets       []sessionTrafficBucket
	totalUpstreamBytes   uint64
	totalDownstreamBytes uint64

	terminalModesMu      sync.RWMutex
	terminalModes        terminalModesState
	terminalModesPartial string

	terminalStateMu         sync.Mutex
	terminalState           vt10x.Terminal
	terminalStateCapturedAt time.Time

	subMu       sync.RWMutex
	subscribers map[string]*sessionSubscriber
	exitOnce    sync.Once

	metaMu       sync.RWMutex
	lastMetadata *SessionMetadata

	shellEventCallback func(shellIntegrationEvent)
	shellIntegration   shellIntegrationState

	// Metadata polling interval tracking
	metaIntervalMu       sync.RWMutex
	metaIntervalLevel    int           // 0=short, 1=medium, 2=long
	metaIntervalTicks    int           // ticks since last user interaction
	metaIntervalNotifyCh chan struct{} // channel to notify interval change
}

// SessionParams collects the data required to bootstrap a session.
type SessionParams struct {
	ID                          string
	ProjectID                   string
	WorktreeID                  string
	WorkingDir                  string
	Title                       string
	Command                     []string
	Env                         []string
	Rows                        int
	Cols                        int
	Logger                      *zap.Logger
	Encoding                    string
	ScrollbackLimit             int
	EnableTerminalStateSnapshot bool
	OrderIndex                  float64
	ShellIntegration            shellIntegrationConfig
	StartupReplayCommand        string
	OnShellEvent                func(shellIntegrationEvent)
}

// sessionError provides a non-nil wrapper so atomic.Value never stores nil.
type sessionError struct {
	err error
}

// NewSession wires metadata without starting the PTY process.
func NewSession(params SessionParams) (*Session, error) {
	if len(params.Command) == 0 {
		return nil, errors.New("shell command is required")
	}

	if params.ID == "" {
		params.ID = utils.NewID()
	}

	scrollbackLimit := params.ScrollbackLimit
	if scrollbackLimit < 0 {
		scrollbackLimit = 0
	}

	enc, encName, err := resolveEncoding(params.Encoding)
	if err != nil {
		return nil, err
	}

	// Set default terminal size
	cols := params.Cols
	rows := params.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	session := &Session{
		id:                 params.ID,
		projectID:          params.ProjectID,
		worktreeID:         params.WorktreeID,
		workingDir:         params.WorkingDir,
		initialWorkingDir:  params.WorkingDir,
		title:              params.Title,
		command:            append([]string{}, params.Command...),
		env:                append([]string{}, params.Env...),
		rows:               rows,
		cols:               cols,
		orderIndex:         params.OrderIndex,
		createdAt:          time.Now(),
		closed:             make(chan struct{}),
		logger:             params.Logger,
		encoding:           enc,
		encName:            encName,
		scrollbackLimit:    scrollbackLimit,
		subscribers:        make(map[string]*sessionSubscriber),
		shellEventCallback: params.OnShellEvent,
	}
	session.terminalStateEnabled.Store(params.EnableTerminalStateSnapshot && runtime.GOOS != "windows")
	session.shellIntegration.family = params.ShellIntegration.Family
	session.shellIntegration.supported = params.ShellIntegration.Supported
	session.shellIntegration.cleanup = params.ShellIntegration.Cleanup
	session.shellIntegration.shellState = terminalRestoreShellStateIdle
	session.shellIntegration.startupReplayCommand = strings.TrimSpace(params.StartupReplayCommand)

	if session.title == "" {
		session.title = session.id
	}

	if session.logger == nil {
		session.logger = utils.Logger()
	}

	if session.terminalStateEnabled.Load() {
		session.initTerminalStateLocked(cols, rows)
	}

	session.status.Store(SessionStatusStarting)
	session.err.Store(sessionError{})
	session.Touch()

	return session, nil
}

// Start launches the PTY command.
func (s *Session) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	rows := s.rows
	if rows <= 0 {
		rows = 24
	}
	cols := s.cols
	if cols <= 0 {
		cols = 80
	}

	ptyDevice, err := xpty.NewPty(cols, rows)
	if err != nil {
		return err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(sessionCtx, s.command[0], s.command[1:]...)
	cmd.Dir = s.workingDir
	configurePTYCommand(cmd)

	env := append([]string{}, s.env...)
	env = append(env, "TERM=xterm-256color")
	env = append(env, "COLORTERM=truecolor")
	// Use GetFreshEnviron to pick up newly installed tools (e.g., updated PATH from registry on Windows)
	cmd.Env = append(utils.GetFreshEnviron(), env...)

	if err := ptyDevice.Start(cmd); err != nil {
		cancel()
		_ = ptyDevice.Close()
		s.setStatus(SessionStatusError)
		return err
	}

	s.mu.Lock()
	s.cmd = cmd
	s.pty = ptyDevice
	s.cancel = cancel
	s.rows = rows
	s.cols = cols
	s.mu.Unlock()

	s.setStatus(SessionStatusRunning)

	go s.wait(sessionCtx)
	go s.consumePTY(sessionCtx)
	go s.monitorMetadata(sessionCtx)

	// Detect the foreground process once after the shell has started.
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.checkAndBroadcastMetadata()
	}()

	return nil
}

func (s *Session) consumePTY(ctx context.Context) {
	reader := s.Reader()
	if reader == nil {
		return
	}

	buffer := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			s.Touch()
			normalized := s.NormalizeOutput(buffer[:n])
			if len(normalized) > 0 {
				normalized = s.stripShellIntegrationSequences(normalized)
			}
			if len(normalized) > 0 {
				modeSnapshot, modeChanged := s.updateTerminalModes(normalized)
				s.appendScrollback(normalized)
				s.appendTerminalState(normalized)
				s.broadcast(StreamEvent{Type: StreamEventData, Data: normalized})
				if modeChanged {
					s.broadcast(StreamEvent{Type: StreamEventModes, Modes: modeSnapshot})
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) monitorMetadata(ctx context.Context) {
	// Initialize interval notification channel
	s.metaIntervalMu.Lock()
	s.metaIntervalNotifyCh = make(chan struct{}, 1)
	s.metaIntervalMu.Unlock()

	ticker := time.NewTicker(MetadataIntervalShort)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.metaIntervalNotifyCh:
			// Interval level changed, reset ticker
			ticker.Stop()
			ticker = time.NewTicker(s.getCurrentMetadataInterval())
		case <-ticker.C:
			s.checkAndBroadcastMetadata()
			s.advanceIntervalTick()
		}
	}
}

// getCurrentMetadataInterval returns the current metadata polling interval based on level
func (s *Session) getCurrentMetadataInterval() time.Duration {
	s.metaIntervalMu.RLock()
	level := s.metaIntervalLevel
	s.metaIntervalMu.RUnlock()

	switch level {
	case 0:
		return MetadataIntervalShort
	case 1:
		return MetadataIntervalMedium
	default:
		return MetadataIntervalLong
	}
}

// advanceIntervalTick increments the tick counter and potentially downgrades interval level
func (s *Session) advanceIntervalTick() {
	s.metaIntervalMu.Lock()
	defer s.metaIntervalMu.Unlock()

	s.metaIntervalTicks++

	// Check if we should downgrade to a longer interval
	if s.metaIntervalTicks >= intervalDowngradeThreshold && s.metaIntervalLevel < 2 {
		s.metaIntervalLevel++
		s.metaIntervalTicks = 0

		// Notify the monitor loop to reset ticker
		select {
		case s.metaIntervalNotifyCh <- struct{}{}:
		default:
		}
	}
}

// resetMetadataInterval resets the polling interval to the shortest level (called on user interaction)
func (s *Session) resetMetadataInterval() {
	s.metaIntervalMu.Lock()

	if s.metaIntervalLevel == 0 && s.metaIntervalTicks == 0 {
		// Already at shortest level with no ticks, nothing to do
		s.metaIntervalMu.Unlock()
		return
	}

	s.metaIntervalLevel = 0
	s.metaIntervalTicks = 0
	notifyCh := s.metaIntervalNotifyCh
	s.metaIntervalMu.Unlock()

	// Notify the monitor loop to reset ticker
	if notifyCh != nil {
		select {
		case notifyCh <- struct{}{}:
		default:
		}
	}
}

func (s *Session) checkAndBroadcastMetadata() {
	pid := s.getPID()
	if pid <= 0 {
		return
	}

	metadata := &SessionMetadata{
		ProcessPID:         pid,
		ProcessStatus:      process.GetProcessStatus(pid),
		ProcessHasChildren: process.IsProcessBusy(pid),
		Title:              s.Title(),
	}

	if metadata.ProcessHasChildren {
		if cmd := process.GetForegroundCommand(pid); cmd != "" {
			metadata.RunningCommand = cmd

			metadata.AIAssistant = ai_assistant2.ToAIAssistantInfo(
				ai_assistant2.DetectFromCommand(cmd),
			)

		}
	}

	// Check if metadata changed
	s.metaMu.RLock()
	lastMeta := s.lastMetadata
	s.metaMu.RUnlock()

	if s.metadataChanged(lastMeta, metadata) {
		s.metaMu.Lock()
		s.lastMetadata = metadata
		s.metaMu.Unlock()

		// Broadcast metadata change
		s.broadcast(StreamEvent{
			Type:     StreamEventMetadata,
			Metadata: metadata,
		})
	}
}

func (s *Session) metadataChanged(old, new *SessionMetadata) bool {
	if old == nil {
		return true
	}
	if new == nil {
		return false
	}

	if old.Title != new.Title ||
		old.ProcessPID != new.ProcessPID ||
		old.ProcessStatus != new.ProcessStatus ||
		old.ProcessHasChildren != new.ProcessHasChildren ||
		old.RunningCommand != new.RunningCommand {
		return true
	}

	// Check AI assistant changes
	if (old.AIAssistant == nil) != (new.AIAssistant == nil) {
		return true
	}
	if old.AIAssistant != nil && new.AIAssistant != nil {
		if old.AIAssistant.Type != new.AIAssistant.Type ||
			old.AIAssistant.DisplayName != new.AIAssistant.DisplayName ||
			old.AIAssistant.Command != new.AIAssistant.Command {
			return true
		}
	}

	return false
}

// Reader exposes the PTY reader interface.
func (s *Session) Reader() io.Reader {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pty
}

// Writer exposes the PTY writer interface.
func (s *Session) Writer() io.Writer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pty
}

// Write writes bytes to the PTY, updating last activity timestamp.
func (s *Session) Write(p []byte) (int, error) {
	writer := s.Writer()
	if writer == nil {
		return 0, io.EOF
	}

	payload := s.prepareInput(p)
	s.Touch()
	s.resetMetadataInterval() // User input resets polling to short interval
	return writer.Write(payload)
}

// Resize updates the PTY window size.
func (s *Session) Resize(cols, rows int) error {
	s.mu.RLock()
	pty := s.pty
	currentCols := s.cols
	currentRows := s.rows
	s.mu.RUnlock()

	if pty == nil {
		return nil
	}

	if cols <= 0 || rows <= 0 {
		return nil
	}

	if err := resizePTYForTarget(pty, runtime.GOOS, currentCols, currentRows, cols, rows); err != nil {
		return err
	}

	s.mu.Lock()
	s.cols = cols
	s.rows = rows
	s.mu.Unlock()

	s.resizeTerminalState(cols, rows)

	s.Touch()
	s.resetMetadataInterval() // User interaction resets polling to short interval

	return nil
}

// ForceRedraw nudges the PTY to repaint the current visible screen without changing the final size.
func (s *Session) ForceRedraw() error {
	s.mu.RLock()
	pty := s.pty
	cols := s.cols
	rows := s.rows
	s.mu.RUnlock()

	if pty == nil || cols <= 0 || rows <= 0 {
		return nil
	}

	if err := resizePTYForTarget(pty, runtime.GOOS, cols, rows, cols, rows); err != nil {
		return err
	}

	s.Touch()
	s.resetMetadataInterval()
	return nil
}

// Subscribe registers a stream subscriber that receives PTY output events.
func (s *Session) Subscribe(ctx context.Context) (*SessionStream, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	subCtx, cancel := context.WithCancel(ctx)
	subscriber := &sessionSubscriber{
		id:     utils.NewID(),
		ch:     make(chan StreamEvent, subscriberBufferSize),
		cancel: cancel,
	}

	s.subMu.Lock()
	if s.subscribers == nil {
		s.subscribers = make(map[string]*sessionSubscriber)
	}
	s.subscribers[subscriber.id] = subscriber
	s.subMu.Unlock()

	// 立即发送当前 metadata 快照，确保新订阅者能获取到当前状态
	// 避免因订阅时序问题错过早期的状态变化事件
	s.metaMu.RLock()
	currentMeta := cloneSessionMetadata(s.lastMetadata)
	s.metaMu.RUnlock()
	if currentMeta != nil {
		select {
		case subscriber.ch <- StreamEvent{Type: StreamEventMetadata, Metadata: currentMeta}:
		default:
		}
	}

	go func() {
		<-subCtx.Done()
		s.removeSubscriber(subscriber.id)
	}()

	return &SessionStream{
		id:     subscriber.id,
		events: subscriber.ch,
		cancel: cancel,
	}, nil
}

// Scrollback returns a copy of the buffered PTY output.
func (s *Session) Scrollback() [][]byte {
	s.scrollMu.RLock()
	defer s.scrollMu.RUnlock()
	if len(s.scrollback) == 0 {
		return nil
	}
	result := make([][]byte, len(s.scrollback))
	for i, chunk := range s.scrollback {
		result[i] = cloneBytes(chunk)
	}
	return result
}

// ScrollbackSince returns buffered PTY output newer than the provided timestamp.
func (s *Session) ScrollbackSince(since time.Time) [][]byte {
	if since.IsZero() {
		return s.Scrollback()
	}

	s.scrollMu.RLock()
	defer s.scrollMu.RUnlock()
	if len(s.scrollback) == 0 {
		return nil
	}

	result := make([][]byte, 0, len(s.scrollback))
	for i, chunk := range s.scrollback {
		if i >= len(s.scrollbackTimestamps) {
			result = append(result, cloneBytes(chunk))
			continue
		}
		if !s.scrollbackTimestamps[i].After(since) {
			continue
		}
		result = append(result, cloneBytes(chunk))
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// Close terminates the session and underlying process.
func (s *Session) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.setStatus(SessionStatusClosed)

		s.cleanupShellIntegration()

		if s.cancel != nil {
			s.cancel()
		}
		var (
			pid int32
			pty xpty.Pty
		)
		s.mu.Lock()
		if s.cmd != nil && s.cmd.Process != nil {
			pid = int32(s.cmd.Process.Pid)
		}
		pty = s.pty
		s.pty = nil
		s.mu.Unlock()

		if pid > 0 {
			if err := process.KillProcessTree(pid); err != nil && s.logger != nil {
				s.logger.Warn("failed to kill terminal process tree",
					zap.Int32("pid", pid),
					zap.String("sessionId", s.id),
					zap.Error(err))
			}
		}

		if pty != nil {
			closeErr = pty.Close()
		}
		close(s.closed)
		s.notifyExit(s.Err())
	})
	return closeErr
}

// Closed channel closes once the session fully terminates.
func (s *Session) Closed() <-chan struct{} {
	return s.closed
}

// ID returns the stable identifier.
func (s *Session) ID() string {
	return s.id
}

// ProjectID returns the owning project.
func (s *Session) ProjectID() string {
	return s.projectID
}

// WorktreeID returns the associated worktree identifier.
func (s *Session) WorktreeID() string {
	return s.worktreeID
}

// WorkingDir exposes the shell working directory.
func (s *Session) WorkingDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workingDir
}

// InitialWorkingDir returns the shell cwd used when the terminal was first created.
func (s *Session) InitialWorkingDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialWorkingDir
}

func (s *Session) setWorkingDir(workingDir string) {
	normalized := strings.TrimSpace(workingDir)
	if normalized == "" {
		return
	}
	s.mu.Lock()
	s.workingDir = normalized
	s.mu.Unlock()
}

// Title returns the display name.
func (s *Session) Title() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.title
}

// OrderIndex returns the project-local tab ordering value.
func (s *Session) OrderIndex() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orderIndex
}

// SetOrderIndex updates the project-local tab ordering value.
func (s *Session) SetOrderIndex(orderIndex float64) {
	s.mu.Lock()
	s.orderIndex = orderIndex
	s.mu.Unlock()
}

// UpdateTitle mutates the tab label in a threadsafe manner.
func (s *Session) UpdateTitle(title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.title = title
	return nil
}

func (s *Session) restoreStateSnapshot() terminalRestoreStateSnapshot {
	s.mu.RLock()
	title := s.title
	projectID := s.projectID
	worktreeID := s.worktreeID
	workingDir := s.workingDir
	initialWorkingDir := s.initialWorkingDir
	orderIndex := s.orderIndex
	s.mu.RUnlock()

	s.shellIntegration.mu.Lock()
	pendingCommand := s.shellIntegration.pendingCommand
	replayEligible := s.shellIntegration.replayEligible
	commandStartedAt := s.shellIntegration.commandStartedAt
	shellFamily := s.shellIntegration.family
	shellSupported := s.shellIntegration.supported
	shellState := s.shellIntegration.shellState
	s.shellIntegration.mu.Unlock()

	var pendingCommandPtr *string
	if strings.TrimSpace(pendingCommand) != "" {
		value := pendingCommand
		pendingCommandPtr = &value
	}
	var startedAtPtr *time.Time
	if !commandStartedAt.IsZero() {
		value := commandStartedAt
		startedAtPtr = &value
	}

	return terminalRestoreStateSnapshot{
		SessionID:         s.id,
		ProjectID:         projectID,
		WorktreeID:        worktreeID,
		Title:             title,
		OrderIndex:        orderIndex,
		InitialWorkingDir: initialWorkingDir,
		LastCwd:           workingDir,
		ShellFamily:       shellFamily,
		ShellSupported:    shellSupported,
		ShellState:        shellState,
		PendingCommand:    pendingCommandPtr,
		ReplayEligible:    replayEligible,
		CommandStartedAt:  startedAtPtr,
	}
}

// CreatedAt returns the spawn timestamp.
func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

// LastActive returns the timestamp of the last interaction.
func (s *Session) LastActive() time.Time {
	return time.Unix(0, s.lastActive.Load())
}

// Status returns the current lifecycle status.
func (s *Session) Status() SessionStatus {
	if status, ok := s.status.Load().(SessionStatus); ok {
		return status
	}
	return SessionStatusStarting
}

// Touch updates the last activity timestamp.
func (s *Session) Touch() {
	s.lastActive.Store(time.Now().UnixNano())
}

// Snapshot copies current state for API responses.
func (s *Session) Snapshot() SessionSnapshot {
	s.mu.RLock()
	snapshot := SessionSnapshot{
		ID:            s.id,
		ProjectID:     s.projectID,
		WorktreeID:    s.worktreeID,
		WorkingDir:    s.workingDir,
		Title:         s.title,
		OrderIndex:    s.orderIndex,
		CreatedAt:     s.createdAt,
		LastActive:    s.LastActive(),
		Status:        s.Status(),
		Rows:          s.rows,
		Cols:          s.cols,
		Encoding:      s.encName,
		TerminalModes: s.TerminalModesSnapshot(),
	}
	pid := s.getPID()
	s.mu.RUnlock()

	// Get process information
	if pid > 0 {
		snapshot.ProcessPID = pid
		snapshot.ProcessStatus = process.GetProcessStatus(pid)
		snapshot.ProcessHasChildren = process.IsProcessBusy(pid)

		// Get foreground command if there are children
		if snapshot.ProcessHasChildren {
			if cmd := process.GetForegroundCommand(pid); cmd != "" {
				snapshot.RunningCommand = cmd
				snapshot.AIAssistant = ai_assistant2.ToAIAssistantInfo(
					ai_assistant2.DetectFromCommand(cmd),
				)
			}
		}
	}

	snapshot.Traffic = s.TrafficStatsSnapshot()

	return snapshot
}

// getPID returns the shell process PID, or 0 if not available.
func (s *Session) getPID() int32 {
	if s.cmd != nil && s.cmd.Process != nil {
		return int32(s.cmd.Process.Pid)
	}
	return 0
}

func (s *Session) setStatus(status SessionStatus) {
	s.status.Store(status)
}

// Err returns the last process error, if any.
func (s *Session) Err() error {
	if value, ok := s.err.Load().(sessionError); ok {
		return value.err
	}
	return nil
}

// NormalizeOutput converts PTY output to UTF-8 based on the configured encoding.
func (s *Session) NormalizeOutput(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	if s.encoding == nil || s.encName == "utf-8" {
		return cloneBytes(data)
	}
	decoded, _, err := transform.Bytes(s.encoding.NewDecoder(), data)
	if err != nil {
		return cloneBytes(data)
	}
	return decoded
}

func (s *Session) prepareInput(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	if s.encoding == nil || s.encName == "utf-8" {
		return cloneBytes(data)
	}
	encoded, _, err := transform.Bytes(s.encoding.NewEncoder(), data)
	if err != nil {
		return cloneBytes(data)
	}
	return encoded
}

func (s *Session) wait(ctx context.Context) {
	err := xpty.WaitProcess(ctx, s.cmd)
	if err != nil {
		s.err.Store(sessionError{err: err})
		s.setStatus(SessionStatusError)
		if s.logger != nil {
			s.logger.Debug("terminal session exited with error", zap.Error(err))
		}
	} else {
		s.err.Store(sessionError{})
		if s.logger != nil {
			s.logger.Debug("terminal session exited normally")
		}
	}
	_ = s.Close()
}

func (s *Session) appendScrollback(chunk []byte) {
	if len(chunk) == 0 || s.scrollbackLimit <= 0 {
		return
	}
	data := cloneBytes(chunk)
	timestamp := time.Now()

	s.scrollMu.Lock()
	s.scrollback = append(s.scrollback, data)
	s.scrollbackTimestamps = append(s.scrollbackTimestamps, timestamp)
	s.scrollbackSize += len(data)
	for s.scrollbackSize > s.scrollbackLimit && len(s.scrollback) > 0 {
		s.scrollbackSize -= len(s.scrollback[0])
		s.scrollback = s.scrollback[1:]
		s.scrollbackTimestamps = s.scrollbackTimestamps[1:]
	}
	s.scrollMu.Unlock()
}

// UpdateScrollbackLimit toggles scrollback buffering and trims existing data accordingly.
func (s *Session) UpdateScrollbackLimit(limit int) {
	if limit < 0 {
		limit = 0
	}

	s.scrollMu.Lock()
	s.scrollbackLimit = limit
	if limit == 0 {
		s.scrollback = nil
		s.scrollbackTimestamps = nil
		s.scrollbackSize = 0
		s.scrollMu.Unlock()
		return
	}

	for s.scrollbackSize > s.scrollbackLimit && len(s.scrollback) > 0 {
		s.scrollbackSize -= len(s.scrollback[0])
		s.scrollback = s.scrollback[1:]
		s.scrollbackTimestamps = s.scrollbackTimestamps[1:]
	}
	s.scrollMu.Unlock()
}

func (s *Session) broadcast(event StreamEvent) {
	listeners := s.snapshotSubscribers()
	for _, sub := range listeners {
		select {
		case sub.ch <- event:
		default:
			if s.logger != nil {
				s.logger.Debug("dropping terminal event for slow subscriber",
					zap.String("sessionId", s.id))
			}
		}
	}
}

func (s *Session) snapshotSubscribers() []*sessionSubscriber {
	s.subMu.RLock()
	defer s.subMu.RUnlock()
	if len(s.subscribers) == 0 {
		return nil
	}
	list := make([]*sessionSubscriber, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		list = append(list, sub)
	}
	return list
}

func (s *Session) notifyExit(err error) {
	s.exitOnce.Do(func() {
		event := StreamEvent{Type: StreamEventExit, Err: err}
		for _, sub := range s.snapshotSubscribers() {
			select {
			case sub.ch <- event:
			default:
			}
			if sub.cancel != nil {
				sub.cancel()
			}
		}
	})
}

func (s *Session) removeSubscriber(id string) {
	s.subMu.Lock()
	sub, ok := s.subscribers[id]
	if ok {
		delete(s.subscribers, id)
	}
	s.subMu.Unlock()
	if ok {
		sub.once.Do(func() {
			close(sub.ch)
		})
	}
}

// DebugInfo collects comprehensive debug information about the session.
type DebugInfo struct {
	SessionID                 string                         `json:"sessionId"`
	ProjectID                 string                         `json:"projectId"`
	WorktreeID                string                         `json:"worktreeId"`
	Status                    SessionStatus                  `json:"status"`
	Rows                      int                            `json:"rows"`
	Cols                      int                            `json:"cols"`
	ScrollbackChunks          []string                       `json:"scrollbackChunks"`
	ScrollbackChunksTimestamp []time.Time                    `json:"scrollbackChunksTimestamp"`
	ScrollbackSize            int                            `json:"scrollbackSize"`
	ScrollbackLimit           int                            `json:"scrollbackLimit"`
	AIAssistant               *ai_assistant2.AIAssistantInfo `json:"aiAssistant,omitempty"`
	Traffic                   *SessionTrafficStats           `json:"traffic,omitempty"`
}

// GetDebugInfo returns comprehensive debugging information about the session.
func (s *Session) GetDebugInfo() *DebugInfo {
	s.mu.RLock()
	rows := s.rows
	cols := s.cols
	s.mu.RUnlock()

	info := &DebugInfo{
		SessionID:       s.id,
		ProjectID:       s.projectID,
		WorktreeID:      s.worktreeID,
		Status:          s.Status(),
		Rows:            rows,
		Cols:            cols,
		ScrollbackLimit: s.scrollbackLimit,
	}

	// Get scrollback chunks and timestamps
	scrollback := s.Scrollback()
	s.scrollMu.RLock()
	timestamps := make([]time.Time, len(s.scrollbackTimestamps))
	copy(timestamps, s.scrollbackTimestamps)
	s.scrollMu.RUnlock()

	if len(scrollback) > 0 {
		chunks := make([]string, 0, len(scrollback))
		totalSize := 0
		for _, chunk := range scrollback {
			chunks = append(chunks, string(chunk))
			totalSize += len(chunk)
		}
		info.ScrollbackChunks = chunks
		info.ScrollbackChunksTimestamp = timestamps
		info.ScrollbackSize = totalSize
	}

	// Get AI Assistant info
	s.metaMu.RLock()
	if s.lastMetadata != nil && s.lastMetadata.AIAssistant != nil {
		aiCopy := *s.lastMetadata.AIAssistant
		info.AIAssistant = &aiCopy
	}
	s.metaMu.RUnlock()

	info.Traffic = s.TrafficStatsSnapshot()

	return info
}

// CapturedChunk represents a captured output chunk
type CapturedChunk struct {
	Data      []byte    `json:"-"`
	DataStr   string    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
	Size      int       `json:"size"`
}

// CaptureNextChunk triggers a resize and captures the next output chunk.
// timeout specifies how long to wait for the next chunk (default: 2 seconds).
func (s *Session) CaptureNextChunk(ctx context.Context, timeout time.Duration) (*CapturedChunk, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 1 * time.Second
	}

	// Subscribe to output stream
	stream, err := s.Subscribe(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to session: %w", err)
	}
	defer stream.Close()

	// Trigger a redraw to force the terminal to repaint the visible screen
	if err := s.ForceRedraw(); err != nil {
		return nil, fmt.Errorf("failed to trigger redraw: %w", err)
	}

	// Wait for the next data chunk
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("timeout waiting for output chunk")
		case event, ok := <-stream.Events():
			if !ok {
				return nil, fmt.Errorf("stream closed before receiving chunk")
			}
			if event.Type == StreamEventData && len(event.Data) > 0 {
				return &CapturedChunk{
					Data:      event.Data,
					DataStr:   string(event.Data),
					Timestamp: time.Now(),
					Size:      len(event.Data),
				}, nil
			}
			// Ignore other event types and continue waiting
		}
	}
}

func cloneSessionMetadata(meta *SessionMetadata) *SessionMetadata {
	if meta == nil {
		return nil
	}
	copyMeta := *meta
	if meta.AIAssistant != nil {
		infoCopy := *meta.AIAssistant
		copyMeta.AIAssistant = &infoCopy
	}
	return &copyMeta
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

func resolveEncoding(name string) (encoding.Encoding, string, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" || normalized == "utf-8" || normalized == "utf8" {
		return nil, "utf-8", nil
	}

	switch normalized {
	case "gbk":
		return simplifiedchinese.GBK, "gbk", nil
	case "gb18030", "gb-18030":
		return simplifiedchinese.GB18030, "gb18030", nil
	case "gb2312":
		return simplifiedchinese.HZGB2312, "gb2312", nil
	default:
		return nil, normalized, ErrInvalidEncoding
	}
}
