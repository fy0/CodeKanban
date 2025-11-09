package terminal

import (
	"context"
	"errors"
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

	"go-template/utils"
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
}

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

	mu sync.RWMutex
}

// SessionParams collects the data required to bootstrap a session.
type SessionParams struct {
	ID         string
	ProjectID  string
	WorktreeID string
	WorkingDir string
	Title      string
	Command    []string
	Env        []string
	Rows       int
	Cols       int
	Logger     *zap.Logger
	Encoding   string
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

	enc, encName, err := resolveEncoding(params.Encoding)
	if err != nil {
		return nil, err
	}

	session := &Session{
		id:         params.ID,
		projectID:  params.ProjectID,
		worktreeID: params.WorktreeID,
		workingDir: params.WorkingDir,
		title:      params.Title,
		command:    append([]string{}, params.Command...),
		env:        append([]string{}, params.Env...),
		rows:       params.Rows,
		cols:       params.Cols,
		createdAt:  time.Now(),
		closed:     make(chan struct{}),
		logger:     params.Logger,
		encoding:   enc,
		encName:    encName,
	}

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

	return nil
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

	s.cols = cols
	s.rows = rows
	s.Touch()

	return nil
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
	return s.title
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
	return SessionSnapshot{
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
