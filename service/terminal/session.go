package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
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

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2"
	"code-kanban/utils/ai_assistant2/log_watcher"
	"code-kanban/utils/ai_assistant2/types"
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
	TaskID      string                         `json:"taskId,omitempty"`
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
	Title                  string                         `json:"title,omitempty"`
	ProcessPID             int32                          `json:"processPid,omitempty"`
	ProcessStatus          string                         `json:"processStatus,omitempty"`
	ProcessHasChildren     bool                           `json:"processHasChildren,omitempty"`
	RunningCommand         string                         `json:"runningCommand,omitempty"`
	AIAssistant            *ai_assistant2.AIAssistantInfo `json:"aiAssistant,omitempty"`
	TaskID                 string                         `json:"taskId,omitempty"`
	AIAssistantRecentInput string                         `json:"aiAssistantRecentInput,omitempty"`
	AISessionID            string                         `json:"aiSessionId,omitempty"`
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
	subscriberBufferSize  = 128
	maxSessionTitleLength = 64

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

	assistantTracker        *ai_assistant2.StatusTracker
	getAIConfig             func() *utils.AIAssistantStatusConfig
	assistantOutputNotifyCh chan struct{}
	assistantOutputMu       sync.Mutex
	assistantOutputQueue    [][]byte
	assistantOutputQueueMax int

	associatedTaskID             string
	lockedTitle                  string
	lastRecentInput              string
	renameTitleEachCommand       atomic.Bool
	autoTitleAssigned            atomic.Bool
	terminalStateEnabled         atomic.Bool
	terminalStateSuppressReplies atomic.Bool

	// LogWatcher for capturing user input from AI assistant session logs
	logWatcherMu          sync.RWMutex
	logWatcher            *log_watcher.LogWatcher
	lastLogWatcherMessage string
	aiProcessStartTime    time.Time // AI process creation time (from proc.CreateTime)
	aiProcessWorkingDir   string    // AI process working directory (from proc.Cwd)
	currentAISessionID    string    // Current AI session ID (found synchronously)
	currentAISessionFile  string    // Current AI session file path
	linkedAISessionID     string    // The session ID that has been linked to task (to avoid repeated linking)

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
	GetAIConfig                 func() *utils.AIAssistantStatusConfig
	EnableTerminalStateSnapshot bool
	TaskID                      string
	RenameTitleEachCommand      bool
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
		assistantTracker:   ai_assistant2.NewStatusTracker(),
		getAIConfig:        params.GetAIConfig,
		associatedTaskID:   params.TaskID,
		shellEventCallback: params.OnShellEvent,
	}
	session.renameTitleEachCommand.Store(params.RenameTitleEachCommand)
	session.terminalStateEnabled.Store(params.EnableTerminalStateSnapshot && runtime.GOOS != "windows")
	session.shellIntegration.family = params.ShellIntegration.Family
	session.shellIntegration.supported = params.ShellIntegration.Supported
	session.shellIntegration.cleanup = params.ShellIntegration.Cleanup
	session.shellIntegration.shellState = terminalRestoreShellStateIdle
	session.shellIntegration.startupReplayCommand = strings.TrimSpace(params.StartupReplayCommand)

	session.assistantTracker.SetCaptureFunc(session.captureTerminalLines)
	// Set state change callback for periodic checking
	session.assistantTracker.SetStateChangeCallback(session.handleStateChangeFromTracker)
	session.assistantTracker.SetTerminalSize(session.rows, session.cols)

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

	s.assistantOutputNotifyCh = make(chan struct{}, 1)
	if s.assistantTracker != nil {
		s.assistantTracker.SetTerminalSize(rows, cols)
	}

	go s.wait(sessionCtx)
	go s.consumePTY(sessionCtx)
	go s.monitorMetadata(sessionCtx)
	go s.processAssistantOutput(sessionCtx)

	// 立即执行一次 metadata 检测，确保 AI assistant tracker 尽早激活
	// 避免第一次状态变化时 tracker 还未激活的问题
	go func() {
		// 等待一小段时间让进程启动
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
				s.enqueueAssistantOutput(normalized)
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

func (s *Session) enqueueAssistantOutput(chunk []byte) {
	if len(chunk) == 0 {
		return
	}

	notifyCh := s.assistantOutputNotifyCh
	if notifyCh == nil {
		s.handleAssistantOutput(chunk)
		return
	}

	s.assistantOutputMu.Lock()
	s.assistantOutputQueue = append(s.assistantOutputQueue, chunk)
	if len(s.assistantOutputQueue) > s.assistantOutputQueueMax {
		s.assistantOutputQueueMax = len(s.assistantOutputQueue)
	}
	s.assistantOutputMu.Unlock()

	select {
	case notifyCh <- struct{}{}:
	default:
	}
}

func (s *Session) processAssistantOutput(ctx context.Context) {
	notifyCh := s.assistantOutputNotifyCh
	if notifyCh == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-notifyCh:
			for {
				chunk := s.dequeueAssistantOutput()
				if len(chunk) == 0 {
					break
				}
				s.handleAssistantOutput(chunk)
			}
		}
	}
}

func (s *Session) dequeueAssistantOutput() []byte {
	s.assistantOutputMu.Lock()
	defer s.assistantOutputMu.Unlock()

	if len(s.assistantOutputQueue) == 0 {
		return nil
	}

	chunk := s.assistantOutputQueue[0]
	s.assistantOutputQueue[0] = nil
	s.assistantOutputQueue = s.assistantOutputQueue[1:]

	if len(s.assistantOutputQueue) == 0 {
		s.assistantOutputQueue = nil
	}

	return chunk
}

func (s *Session) checkAndBroadcastMetadata() {
	pid := s.getPID()
	if pid <= 0 {
		return
	}

	metadata := &SessionMetadata{
		ProcessPID:             pid,
		ProcessStatus:          process.GetProcessStatus(pid),
		ProcessHasChildren:     process.IsProcessBusy(pid),
		TaskID:                 s.TaskID(),
		Title:                  s.Title(),
		AIAssistantRecentInput: s.LastRecentInput(),
	}

	tracker := s.assistantTracker
	if metadata.ProcessHasChildren {
		if cmd := process.GetForegroundCommand(pid); cmd != "" {
			metadata.RunningCommand = cmd

			// Detect AI Assistant
			aiInfo := ai_assistant2.DetectFromCommand(cmd)
			metadata.AIAssistant = s.enrichAssistantInfo(aiInfo)

			// Start LogWatcher for supported assistant types
			// Use FindAIAssistantProcess to get accurate working directory and process start time
			if aiInfo != nil {
				aiProcInfo := process.FindAIAssistantProcess(pid, func(cmdline string) bool {
					return ai_assistant2.DetectFromCommand(cmdline) != nil
				})
				if aiProcInfo != nil {
					// Synchronously find session file and get session ID
					sessionID, sessionFile := s.findAISessionSync(aiInfo.Type, aiProcInfo.Cwd, aiProcInfo.CreateTime)
					if sessionID != "" {
						metadata.AISessionID = sessionID
					}
					s.ensureLogWatcherStarted(aiInfo.Type, aiProcInfo.Cwd, aiProcInfo.CreateTime)

					// Auto-link to task if session found
					if sessionID != "" && sessionFile != "" {
						go s.autoLinkAISession(sessionID, sessionFile)
					}
				} else {
					// Fallback: start without working directory info
					s.ensureLogWatcherStarted(aiInfo.Type, "", time.Time{})
				}
			}
		} else if tracker != nil {
			tracker.Deactivate()
			s.stopLogWatcher()
		}
	} else if tracker != nil {
		tracker.Deactivate()
		s.stopLogWatcher()
	}

	// Populate cached AI session ID (LogWatcher found it asynchronously).
	s.logWatcherMu.RLock()
	if metadata.AISessionID == "" && s.currentAISessionID != "" {
		metadata.AISessionID = s.currentAISessionID
	}
	s.logWatcherMu.RUnlock()

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
		old.RunningCommand != new.RunningCommand ||
		old.TaskID != new.TaskID ||
		old.AISessionID != new.AISessionID ||
		old.AIAssistantRecentInput != new.AIAssistantRecentInput {
		return true
	}

	// Check AI assistant changes
	if (old.AIAssistant == nil) != (new.AIAssistant == nil) {
		return true
	}
	if old.AIAssistant != nil && new.AIAssistant != nil {
		if old.AIAssistant.Type != new.AIAssistant.Type ||
			old.AIAssistant.DisplayName != new.AIAssistant.DisplayName ||
			old.AIAssistant.Command != new.AIAssistant.Command ||
			old.AIAssistant.State != new.AIAssistant.State ||
			!old.AIAssistant.StateUpdatedAt.Equal(new.AIAssistant.StateUpdatedAt) {
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

	// Also resize terminal emulator
	if s.assistantTracker != nil {
		s.assistantTracker.SetTerminalSize(rows, cols)
	}
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

		// Stop LogWatcher first
		s.stopLogWatcher()
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

// TaskID returns the task associated with this session, if any.
func (s *Session) TaskID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.associatedTaskID
}

// AssociateTask links a task to the session.
func (s *Session) AssociateTask(taskID string) bool {
	s.mu.Lock()
	changed := s.associatedTaskID != taskID
	s.associatedTaskID = taskID
	s.mu.Unlock()

	if changed {
		s.broadcastMetadataSnapshot()
	}
	return changed
}

// ClearTaskAssociation removes the task link from the session.
func (s *Session) ClearTaskAssociation() bool {
	s.mu.Lock()
	if s.associatedTaskID == "" {
		s.mu.Unlock()
		return false
	}
	s.associatedTaskID = ""
	s.lockedTitle = ""
	s.mu.Unlock()

	s.broadcastMetadataSnapshot()
	return true
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
	if s.lockedTitle != "" && s.lockedTitle != title {
		return ErrSessionTitleLocked
	}
	s.title = title
	if s.associatedTaskID != "" && s.lockedTitle == "" {
		s.lockedTitle = title
	}
	return nil
}

func (s *Session) restoreStateSnapshot() terminalRestoreStateSnapshot {
	s.mu.RLock()
	taskID := s.associatedTaskID
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

	var taskIDPtr *string
	if strings.TrimSpace(taskID) != "" {
		value := taskID
		taskIDPtr = &value
	}
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
		TaskID:            taskIDPtr,
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

// SetRenameTitleEachCommand toggles whether AI input renames should run on each instruction.
func (s *Session) SetRenameTitleEachCommand(enabled bool) {
	s.renameTitleEachCommand.Store(enabled)
}

// LastRecentInput returns the last user input captured by the AI assistant.
func (s *Session) LastRecentInput() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastRecentInput
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
	rows := s.rows
	cols := s.cols
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
				snapshot.AIAssistant = s.enrichAssistantInfoWithSize(ai_assistant2.DetectFromCommand(cmd), rows, cols)
			}
		}
	}

	snapshot.TaskID = s.TaskID()
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

func (s *Session) handleAssistantOutput(chunk []byte) {
	tracker := s.assistantTracker
	if len(chunk) == 0 || tracker == nil {
		return
	}
	tracker.ProcessChunkInvoke(chunk)
}

func (s *Session) captureTerminalLines(rows, cols int) ([]string, error) {
	if rows <= 0 || cols <= 0 {
		s.mu.RLock()
		if rows <= 0 {
			rows = s.rows
		}
		if cols <= 0 {
			cols = s.cols
		}
		s.mu.RUnlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	chunk, err := s.CaptureNextChunk(ctx, time.Second)
	if err != nil {
		return nil, err
	}
	return ai_assistant2.RenderLinesFromBuffer(chunk.Data, rows, cols), nil
}

// handleStateChangeFromTracker is called by tracker when periodic check detects state change
func (s *Session) handleStateChangeFromTracker(event ai_assistant2.StateChangeEvent) {
	// 优先使用终端检测的用户输入，LogWatcher 作为备选
	// 这样可以避免第一次状态变化时 LogWatcher 还没准备好的问题
	if event.PreviousState == types.StateWaitingInput && event.State == types.StateWorking {
		if event.RecentInput == "" {
			s.logWatcherMu.RLock()
			if s.logWatcher != nil {
				if msg := s.logWatcher.LastUserMessage(); msg != nil && msg.Message != "" {
					event.RecentInput = msg.Message
				}
			}
			s.logWatcherMu.RUnlock()
		}
	}

	s.applyAssistantState(event)
	if event.RecentInput != "" {
		go s.handleRecentInput(event)
	}
}

func (s *Session) applyAssistantState(event ai_assistant2.StateChangeEvent) {
	s.metaMu.Lock()

	var metadata *SessionMetadata
	if s.lastMetadata == nil {
		// 第一次状态变化时 lastMetadata 可能为 nil，创建新的
		metadata = &SessionMetadata{
			Title:       s.Title(),
			TaskID:      s.TaskID(),
			AIAssistant: &ai_assistant2.AIAssistantInfo{},
		}
	} else if s.lastMetadata.AIAssistant == nil {
		// lastMetadata 存在但 AIAssistant 为 nil，克隆并创建 AIAssistant
		metadata = cloneSessionMetadata(s.lastMetadata)
		metadata.AIAssistant = &ai_assistant2.AIAssistantInfo{}
	} else {
		metadata = cloneSessionMetadata(s.lastMetadata)
	}

	ai_assistant2.SetState(metadata.AIAssistant, event.State, event.Timestamp)
	metadata.TaskID = s.TaskID()
	metadata.AIAssistantRecentInput = ""
	if event.PreviousState == types.StateWaitingInput &&
		event.State == types.StateWorking &&
		event.RecentInput != "" {
		metadata.AIAssistantRecentInput = event.RecentInput
	}
	// 从缓存获取 AI session ID
	s.logWatcherMu.RLock()
	if s.currentAISessionID != "" {
		metadata.AISessionID = s.currentAISessionID
	}
	s.logWatcherMu.RUnlock()
	s.lastMetadata = metadata
	s.metaMu.Unlock()

	s.broadcast(StreamEvent{Type: StreamEventMetadata, Metadata: metadata})
}

func (s *Session) handleRecentInput(event ai_assistant2.StateChangeEvent) {
	input := strings.TrimSpace(event.RecentInput)
	if input == "" {
		return
	}

	s.mu.Lock()
	if input == s.lastRecentInput {
		s.mu.Unlock()
		return
	}
	s.lastRecentInput = input
	s.mu.Unlock()

	if s.autoUpdateTitleFromInput(input) {
		s.notifyTitleChanged()
	}
}

func (s *Session) autoUpdateTitleFromInput(input string) bool {
	candidate := truncateString(sanitizeCapturedInput(input), maxSessionTitleLength)
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lockedTitle != "" {
		return false
	}

	if !s.renameTitleEachCommand.Load() && s.autoTitleAssigned.Load() {
		return false
	}

	if s.title == candidate {
		if !s.autoTitleAssigned.Load() {
			s.autoTitleAssigned.Store(true)
		}
		return false
	}

	s.title = candidate
	s.autoTitleAssigned.Store(true)
	return true
}

func (s *Session) notifyTitleChanged() {
	title := s.Title()
	s.metaMu.Lock()
	if s.lastMetadata == nil {
		s.lastMetadata = &SessionMetadata{}
	}
	s.lastMetadata.Title = title
	meta := cloneSessionMetadata(s.lastMetadata)
	s.metaMu.Unlock()
	if meta == nil {
		meta = &SessionMetadata{Title: title}
	}
	meta.TaskID = s.TaskID()
	s.broadcast(StreamEvent{Type: StreamEventMetadata, Metadata: meta})
}

func sanitizeCapturedInput(value string) string {
	fields := strings.Fields(value)
	return strings.Join(fields, " ")
}

func truncateString(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max == 1 {
		return string(runes[:1])
	}
	return string(runes[:max-1]) + "…"
}

func (s *Session) enrichAssistantInfo(info *types.AssistantInfo) *ai_assistant2.AIAssistantInfo {
	s.mu.RLock()
	cols, rows := s.cols, s.rows
	s.mu.RUnlock()
	return s.enrichAssistantInfoWithSize(info, rows, cols)
}

func (s *Session) enrichAssistantInfoWithSize(info *types.AssistantInfo, rows, cols int) *ai_assistant2.AIAssistantInfo {
	tracker := s.assistantTracker
	if info == nil {
		if tracker != nil {
			tracker.Deactivate()
		}
		return nil
	}

	// Check if this assistant type is enabled in config
	if s.getAIConfig != nil {
		config := s.getAIConfig()
		if config != nil && !config.IsEnabled(string(info.Type)) {
			// Config disabled this assistant type, deactivate tracker
			if tracker != nil {
				tracker.Deactivate()
			}
			// Return AIAssistantInfo with unknown state to indicate it's disabled
			aiInfo := ai_assistant2.ToAIAssistantInfo(info)
			ai_assistant2.SetState(aiInfo, types.StateUnknown, time.Now())
			return aiInfo
		}
	}

	// Convert to AIAssistantInfo
	aiInfo := ai_assistant2.ToAIAssistantInfo(info)

	// Activate tracker with terminal size
	if tracker != nil {
		tracker.Activate(info.Type, rows, cols)

		// 立即触发一次 PTY resize，让终端重新绘制完整内容
		// 这样虚拟终端就能同步到真实终端的当前状态
		_ = s.ForceRedraw()

		// Get current state
		if state, ts := tracker.State(); state != types.StateUnknown {
			ai_assistant2.SetState(aiInfo, state, ts)
		}
	}

	return aiInfo
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
	AIChunkCount              int64                          `json:"aiChunkCount,omitempty"`
	AssistantOutputQueueLen   int                            `json:"assistantOutputQueueLen,omitempty"`
	AssistantOutputQueueCap   int                            `json:"assistantOutputQueueCap,omitempty"`
	AssistantOutputQueueMax   int                            `json:"assistantOutputQueueMax,omitempty"`
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

	s.assistantOutputMu.Lock()
	info.AssistantOutputQueueLen = len(s.assistantOutputQueue)
	info.AssistantOutputQueueCap = cap(s.assistantOutputQueue)
	info.AssistantOutputQueueMax = s.assistantOutputQueueMax
	s.assistantOutputMu.Unlock()

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

	if s.assistantTracker != nil {
		info.AIChunkCount = s.assistantTracker.ChunkCount()
	}

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

func (s *Session) broadcastMetadataSnapshot() {
	s.metaMu.RLock()
	meta := cloneSessionMetadata(s.lastMetadata)
	s.metaMu.RUnlock()
	if meta == nil {
		return
	}
	meta.TaskID = s.TaskID()
	s.broadcast(StreamEvent{Type: StreamEventMetadata, Metadata: meta})
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

// ensureLogWatcherStarted starts a LogWatcher for the given assistant type if not already running.
// It uses the AI process's actual working directory and creation time for accurate session file lookup.
func (s *Session) ensureLogWatcherStarted(assistantType types.AssistantType, workingDir string, processStartTime time.Time) {
	s.logWatcherMu.Lock()
	defer s.logWatcherMu.Unlock()

	// Already have a watcher running
	if s.logWatcher != nil {
		return
	}

	// Store the AI process info
	s.aiProcessStartTime = processStartTime
	s.aiProcessWorkingDir = workingDir

	// Use current time as fallback if no process start time provided
	if s.aiProcessStartTime.IsZero() {
		s.aiProcessStartTime = time.Now()
	}

	// Create watcher with working directory for accurate session file lookup
	watcher, err := log_watcher.CreateWatcherForAssistantWithWorkingDir(
		assistantType,
		s.aiProcessStartTime,
		s.aiProcessWorkingDir,
		s.logger,
		s.handleLogWatcherEvent,
	)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to create log watcher",
				zap.String("assistantType", string(assistantType)),
				zap.String("workingDir", workingDir),
				zap.Error(err))
		}
		return
	}

	if watcher == nil {
		// Assistant type doesn't support log watching
		return
	}

	s.logWatcher = watcher

	// Start the watcher in background
	go func() {
		if err := watcher.Start(context.Background()); err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to start log watcher", zap.Error(err))
			}
		}
	}()

	if s.logger != nil {
		s.logger.Debug("log watcher started",
			zap.String("assistantType", string(assistantType)),
			zap.String("workingDir", s.aiProcessWorkingDir),
			zap.Time("processStartTime", s.aiProcessStartTime))
	}
}

// stopLogWatcher stops the current LogWatcher if running
func (s *Session) stopLogWatcher() {
	s.logWatcherMu.Lock()
	defer s.logWatcherMu.Unlock()

	if s.logWatcher == nil {
		return
	}

	s.logWatcher.Stop()
	s.logWatcher = nil
	s.aiProcessStartTime = time.Time{}
	s.aiProcessWorkingDir = ""
	s.currentAISessionID = ""
	s.currentAISessionFile = ""
	// Note: linkedAISessionID is NOT reset here, so if the same session is restarted,
	// it won't be linked again. It will only be linked again if a NEW session is created.

	if s.logger != nil {
		s.logger.Debug("log watcher stopped")
	}
}

// handleLogWatcherEvent handles events from the LogWatcher
func (s *Session) handleLogWatcherEvent(event log_watcher.WatcherEvent) {
	switch event.Type {
	case log_watcher.EventTypeNewMessage:
		if event.Message != nil && event.Message.Message != "" {
			s.handleLogWatcherMessage(event.Message)
		}

	case log_watcher.EventTypeSessionFound:
		s.logWatcherMu.RLock()
		watcher := s.logWatcher
		s.logWatcherMu.RUnlock()

		if watcher != nil {
			info := watcher.Info()
			sessionID := info.SessionID
			filePath := info.FilePath
			metaCwd := ""
			if info.SessionMeta != nil {
				metaCwd = info.SessionMeta.Cwd
			}

			if sessionID == "" && filePath != "" {
				base := filepath.Base(filePath)
				if strings.HasPrefix(base, log_watcher.CodexRolloutPrefix) && strings.HasSuffix(base, log_watcher.CodexRolloutSuffix) {
					sessionID = log_watcher.ExtractSessionIDFromFilename(base)
				} else if strings.HasSuffix(base, ".jsonl") {
					sessionID = strings.TrimSuffix(base, ".jsonl")
				} else {
					sessionID = strings.TrimSuffix(base, filepath.Ext(base))
				}
			}

			if sessionID != "" || filePath != "" || metaCwd != "" {
				s.logWatcherMu.Lock()
				if sessionID != "" {
					s.currentAISessionID = sessionID
				}
				if filePath != "" {
					s.currentAISessionFile = filePath
				}
				// Fill working dir from session_meta when process introspection is limited.
				if s.aiProcessWorkingDir == "" && metaCwd != "" {
					s.aiProcessWorkingDir = metaCwd
				}
				s.logWatcherMu.Unlock()
			}

			if s.logger != nil {
				s.logger.Info("log watcher found session",
					zap.String("sessionId", sessionID),
					zap.String("filePath", filePath),
					zap.String("metaCwd", metaCwd))
			}

			// Auto-link AI session to associated task
			go s.autoLinkAISession(sessionID, filePath)
			// Trigger a metadata refresh so UI can pick up `aiSessionId` even if status tracking is disabled.
			go s.checkAndBroadcastMetadata()
		}

	case log_watcher.EventTypeError:
		if s.logger != nil && event.Error != nil {
			s.logger.Warn("log watcher error", zap.Error(event.Error))
		}
	}
}

// handleLogWatcherMessage processes a user message from the LogWatcher
func (s *Session) handleLogWatcherMessage(msg *log_watcher.UserMessage) {
	if msg == nil || msg.Message == "" {
		return
	}

	s.logWatcherMu.Lock()
	// Check if this is a new message
	if msg.Message == s.lastLogWatcherMessage {
		s.logWatcherMu.Unlock()
		return
	}
	s.lastLogWatcherMessage = msg.Message
	s.logWatcherMu.Unlock()

	// Create a state change event to trigger the same flow as terminal-based detection
	event := ai_assistant2.StateChangeEvent{
		State:         types.StateWorking,
		PreviousState: types.StateWaitingInput,
		Timestamp:     msg.Timestamp,
		RecentInput:   msg.Message,
	}

	// Reuse the same recent-input path so terminal title updates stay consistent.
	go s.handleRecentInput(event)

	if s.logger != nil {
		s.logger.Debug("log watcher captured user message",
			zap.String("message", truncateString(msg.Message, 100)),
			zap.Time("timestamp", msg.Timestamp))
	}
}

// GetLogWatcherInfo returns the current LogWatcher status
func (s *Session) GetLogWatcherInfo() *log_watcher.WatcherInfo {
	s.logWatcherMu.RLock()
	defer s.logWatcherMu.RUnlock()

	if s.logWatcher == nil {
		return nil
	}

	info := s.logWatcher.Info()
	return &info
}

// autoLinkAISession automatically links the discovered AI session to the associated task.
// It creates a minimal AI session record if it doesn't exist, then creates the link.
func (s *Session) autoLinkAISession(sessionID, filePath string) {
	if sessionID == "" || filePath == "" {
		return
	}

	// Check if this specific session has already been linked
	s.logWatcherMu.RLock()
	alreadyLinked := s.linkedAISessionID == sessionID
	s.logWatcherMu.RUnlock()

	if alreadyLinked {
		return
	}

	// Get the associated task ID
	s.mu.RLock()
	taskID := s.associatedTaskID
	s.mu.RUnlock()

	if taskID == "" {
		if s.logger != nil {
			s.logger.Debug("no associated task for auto-linking AI session",
				zap.String("sessionId", sessionID))
		}
		return
	}

	// Get the working directory
	s.logWatcherMu.RLock()
	workingDir := s.aiProcessWorkingDir
	s.logWatcherMu.RUnlock()

	// Determine session type from the file path
	// Claude Code sessions are stored in ~/.claude/projects/
	// Codex sessions are stored in ~/.codex/sessions/YYYY/MM/DD/
	var sessionType tables.AISessionType
	if strings.Contains(filePath, ".codex") || strings.Contains(filePath, "codex-rollout") {
		sessionType = tables.AISessionTypeCodex
	} else {
		sessionType = tables.AISessionTypeClaudeCode
	}

	ctx := context.Background()

	// Ensure AI session exists and link to task
	taskAISessionService := &model.TaskAISessionService{}
	if err := taskAISessionService.EnsureAISessionAndLinkToTask(ctx, taskID, sessionID, filePath, workingDir, sessionType); err != nil {
		if s.logger != nil {
			s.logger.Debug("failed to auto-link AI session to task",
				zap.String("sessionId", sessionID),
				zap.String("taskId", taskID),
				zap.Error(err))
		}
		return
	}

	// Mark this session as linked to avoid repeated logging
	s.logWatcherMu.Lock()
	s.linkedAISessionID = sessionID
	s.logWatcherMu.Unlock()

	if s.logger != nil {
		s.logger.Info("auto-linked AI session to task",
			zap.String("sessionId", sessionID),
			zap.String("taskId", taskID),
			zap.String("filePath", filePath))
	}
}

// AISessionLinkInfo contains current AI session link information for recheck
type AISessionLinkInfo struct {
	SessionID       string
	TaskID          string
	FilePath        string
	LinkedSessionID string // The session ID that was previously linked
}

// GetAISessionLinkInfo returns the current AI session link information
func (s *Session) GetAISessionLinkInfo() AISessionLinkInfo {
	s.logWatcherMu.RLock()
	currentSessionID := s.currentAISessionID
	currentFilePath := s.currentAISessionFile
	linkedSessionID := s.linkedAISessionID
	s.logWatcherMu.RUnlock()

	s.mu.RLock()
	taskID := s.associatedTaskID
	s.mu.RUnlock()

	return AISessionLinkInfo{
		SessionID:       currentSessionID,
		TaskID:          taskID,
		FilePath:        currentFilePath,
		LinkedSessionID: linkedSessionID,
	}
}

// RecheckAISessionLink checks if the current AI session link is valid and triggers re-linking if needed.
// This can be called when anomalies are detected (e.g., session file deleted, task changed, etc.)
// Returns true if re-linking was triggered, false otherwise.
func (s *Session) RecheckAISessionLink(info AISessionLinkInfo) bool {
	// TODO: Implement recheck logic
	// Possible checks:
	// 1. Session file no longer exists
	// 2. Task association changed
	// 3. Session ID mismatch
	// 4. Database link record is missing/invalid

	needRelink := s.checkNeedRelink(info)
	if !needRelink {
		return false
	}

	// Reset linked session ID to trigger re-linking
	s.logWatcherMu.Lock()
	s.linkedAISessionID = ""
	s.logWatcherMu.Unlock()

	// Trigger re-link if we have current session info
	if info.SessionID != "" && info.FilePath != "" {
		go s.autoLinkAISession(info.SessionID, info.FilePath)
	}

	return true
}

// checkNeedRelink checks if re-linking is needed based on the provided info.
// Override this method to implement custom recheck logic.
func (s *Session) checkNeedRelink(info AISessionLinkInfo) bool {
	// TODO: Implement actual checks
	// For now, always return false (no recheck needed)
	return false
}

// findAISessionSync synchronously finds the AI session file and returns session ID and file path.
// This is called when AI assistant is detected to immediately get the session ID for metadata.
func (s *Session) findAISessionSync(assistantType types.AssistantType, workingDir string, processStartTime time.Time) (string, string) {
	if assistantType == types.AssistantTypeClaudeCode && workingDir == "" {
		return "", ""
	}

	s.logWatcherMu.Lock()
	// Check if we already have the session ID cached
	if s.currentAISessionID != "" && (assistantType == types.AssistantTypeCodex || s.aiProcessWorkingDir == workingDir) {
		sessionID := s.currentAISessionID
		sessionFile := s.currentAISessionFile
		s.logWatcherMu.Unlock()
		return sessionID, sessionFile
	}
	s.logWatcherMu.Unlock()

	var sessionFile string
	var sessionID string

	switch assistantType {
	case types.AssistantTypeClaudeCode:
		searcher, err := log_watcher.NewClaudeCodeFileSearcher(workingDir)
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("failed to create Claude Code file searcher", zap.Error(err))
			}
			return "", ""
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		filePath, err := searcher.FindSessionFile(ctx, processStartTime)
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("failed to find Claude Code session file", zap.Error(err))
			}
			return "", ""
		}

		if filePath == "" {
			return "", ""
		}

		sessionFile = filePath
		// Extract session ID from file name (e.g., "abc-123.jsonl" -> "abc-123")
		baseName := filepath.Base(filePath)
		sessionID = strings.TrimSuffix(baseName, ".jsonl")

	case types.AssistantTypeCodex:
		searcher, err := log_watcher.NewCodexFileSearcherWithWorkingDir(workingDir)
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("failed to create Codex file searcher", zap.Error(err))
			}
			return "", ""
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		filePath, err := searcher.FindSessionFile(ctx, processStartTime)
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("failed to find Codex session file", zap.Error(err))
			}
			return "", ""
		}

		if filePath == "" {
			return "", ""
		}

		sessionFile = filePath
		sessionID = log_watcher.ExtractSessionIDFromFilename(filepath.Base(filePath))

	default:
		return "", ""
	}

	if sessionID != "" {
		// Cache the result
		s.logWatcherMu.Lock()
		s.currentAISessionID = sessionID
		s.currentAISessionFile = sessionFile
		s.logWatcherMu.Unlock()

		if s.logger != nil {
			s.logger.Info("found AI session synchronously",
				zap.String("assistantType", string(assistantType)),
				zap.String("sessionId", sessionID),
				zap.String("filePath", sessionFile))
		}
	}

	return sessionID, sessionFile
}
