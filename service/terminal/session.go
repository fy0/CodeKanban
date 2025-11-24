package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/x/xpty"
	"go.uber.org/zap"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2"
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
	ID         string
	ProjectID  string
	WorktreeID string
	WorkingDir string
	Title      string
	CreatedAt  time.Time
	LastActive time.Time
	Status     SessionStatus
	Rows       int
	Cols       int
	Encoding   string
	// Process information
	ProcessPID         int32  `json:"processPid,omitempty"`
	ProcessStatus      string `json:"processStatus,omitempty"`
	ProcessHasChildren bool   `json:"processHasChildren,omitempty"`
	RunningCommand     string `json:"runningCommand,omitempty"`
	// AI Assistant information
	AIAssistant *ai_assistant2.AIAssistantInfo `json:"aiAssistant"`
}

type StreamEventType string

const (
	StreamEventData     StreamEventType = "data"
	StreamEventExit     StreamEventType = "exit"
	StreamEventMetadata StreamEventType = "metadata"
)

type StreamEvent struct {
	Type     StreamEventType
	Data     []byte
	Err      error
	Metadata *SessionMetadata
}

type SessionMetadata struct {
	ProcessPID         int32                          `json:"processPid,omitempty"`
	ProcessStatus      string                         `json:"processStatus,omitempty"`
	ProcessHasChildren bool                           `json:"processHasChildren,omitempty"`
	RunningCommand     string                         `json:"runningCommand,omitempty"`
	AIAssistant        *ai_assistant2.AIAssistantInfo `json:"aiAssistant,omitempty"`
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

const subscriberBufferSize = 128

// Session encapsulates a PTY-backed terminal command.
type Session struct {
	id         string
	projectID  string
	worktreeID string
	workingDir string
	title      string
	command    []string
	env        []string
	rows       int
	cols       int

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

	assistantTracker *ai_assistant2.StatusTracker
	getAIConfig      func() *utils.AIAssistantStatusConfig

	mu sync.RWMutex

	scrollMu             sync.RWMutex
	scrollback           [][]byte
	scrollbackTimestamps []time.Time
	scrollbackSize       int
	scrollbackLimit      int

	subMu       sync.RWMutex
	subscribers map[string]*sessionSubscriber
	exitOnce    sync.Once

	metaMu       sync.RWMutex
	lastMetadata *SessionMetadata
}

// SessionParams collects the data required to bootstrap a session.
type SessionParams struct {
	ID              string
	ProjectID       string
	WorktreeID      string
	WorkingDir      string
	Title           string
	Command         []string
	Env             []string
	Rows            int
	Cols            int
	Logger          *zap.Logger
	Encoding        string
	ScrollbackLimit int
	GetAIConfig     func() *utils.AIAssistantStatusConfig
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
		id:               params.ID,
		projectID:        params.ProjectID,
		worktreeID:       params.WorktreeID,
		workingDir:       params.WorkingDir,
		title:            params.Title,
		command:          append([]string{}, params.Command...),
		env:              append([]string{}, params.Env...),
		rows:             rows,
		cols:             cols,
		createdAt:        time.Now(),
		closed:           make(chan struct{}),
		logger:           params.Logger,
		encoding:         enc,
		encName:          encName,
		scrollbackLimit:  scrollbackLimit,
		subscribers:      make(map[string]*sessionSubscriber),
		assistantTracker: ai_assistant2.NewStatusTracker(),
		getAIConfig:      params.GetAIConfig,
	}

	// Set state change callback for periodic checking
	session.assistantTracker.SetStateChangeCallback(session.handleStateChangeFromTracker)

	if session.title == "" {
		session.title = session.id
	}

	if session.logger == nil {
		session.logger = utils.Logger()
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

	env := append([]string{}, s.env...)
	env = append(env, "TERM=xterm-256color")
	cmd.Env = append(os.Environ(), env...)

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
				s.appendScrollback(normalized)
				s.broadcast(StreamEvent{Type: StreamEventData, Data: normalized})
				s.handleAssistantOutput(normalized)
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) monitorMetadata(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAndBroadcastMetadata()
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
	}

	tracker := s.assistantTracker
	if metadata.ProcessHasChildren {
		if cmd := process.GetForegroundCommand(pid); cmd != "" {
			metadata.RunningCommand = cmd

			// Detect AI Assistant
			aiInfo := ai_assistant2.DetectFromCommand(cmd)
			metadata.AIAssistant = s.enrichAssistantInfo(aiInfo)
		} else if tracker != nil {
			tracker.Deactivate()
		}
	} else if tracker != nil {
		tracker.Deactivate()
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

	if old.ProcessPID != new.ProcessPID ||
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
	return writer.Write(payload)
}

// Resize updates the PTY window size.
func (s *Session) Resize(cols, rows int) error {
	s.mu.RLock()
	pty := s.pty
	s.mu.RUnlock()

	if pty == nil {
		return nil
	}

	if cols <= 0 || rows <= 0 {
		return nil
	}

	if err := pty.Resize(cols, rows); err != nil {
		return err
	}

	s.mu.Lock()
	s.cols = cols
	s.rows = rows
	s.mu.Unlock()

	// Also resize terminal emulator
	// Resize emulator in tracker if active
	if s.assistantTracker != nil {
		s.assistantTracker.Activate(s.assistantTracker.AssistantType(), rows, cols)
	}

	s.Touch()

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

// Close terminates the session and underlying process.
func (s *Session) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.setStatus(SessionStatusClosed)

		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Lock()
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		if s.pty != nil {
			closeErr = s.pty.Close()
			s.pty = nil
		}
		s.mu.Unlock()
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
	return s.workingDir
}

// Title returns the display name.
func (s *Session) Title() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.title
}

// UpdateTitle mutates the tab label in a threadsafe manner.
func (s *Session) UpdateTitle(title string) {
	s.mu.Lock()
	s.title = title
	s.mu.Unlock()
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
		ID:         s.id,
		ProjectID:  s.projectID,
		WorktreeID: s.worktreeID,
		WorkingDir: s.workingDir,
		Title:      s.title,
		CreatedAt:  s.createdAt,
		LastActive: s.LastActive(),
		Status:     s.Status(),
		Rows:       s.rows,
		Cols:       s.cols,
		Encoding:   s.encName,
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
	if len(chunk) == 0 || s.assistantTracker == nil {
		return
	}

	// Feed chunk to tracker's virtual terminal and detect state changes
	state, ts, changed := s.assistantTracker.ProcessChunk(chunk)

	if !changed || state == types.StateUnknown {
		return
	}

	s.metaMu.Lock()
	if s.lastMetadata == nil || s.lastMetadata.AIAssistant == nil {
		s.metaMu.Unlock()
		return
	}
	metadata := cloneSessionMetadata(s.lastMetadata)
	ai_assistant2.SetState(metadata.AIAssistant, state, ts)
	s.lastMetadata = metadata
	s.metaMu.Unlock()

	s.broadcast(StreamEvent{Type: StreamEventMetadata, Metadata: metadata})
}

// handleStateChangeFromTracker is called by tracker when periodic check detects state change
func (s *Session) handleStateChangeFromTracker(state types.State, ts time.Time) {
	s.metaMu.Lock()
	if s.lastMetadata == nil || s.lastMetadata.AIAssistant == nil {
		s.metaMu.Unlock()
		return
	}
	metadata := cloneSessionMetadata(s.lastMetadata)
	ai_assistant2.SetState(metadata.AIAssistant, state, ts)
	s.lastMetadata = metadata
	s.metaMu.Unlock()

	s.broadcast(StreamEvent{Type: StreamEventMetadata, Metadata: metadata})
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
		timeout = 2 * time.Second
	}

	// Subscribe to output stream
	stream, err := s.Subscribe(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to session: %w", err)
	}
	defer stream.Close()

	// Trigger a resize to force terminal redraw
	s.mu.RLock()
	rows, cols := s.rows, s.cols
	s.mu.RUnlock()

	if err := s.Resize(cols, rows); err != nil {
		return nil, fmt.Errorf("failed to trigger resize: %w", err)
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
