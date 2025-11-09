package ptytest

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

	"go-template/utils"
)

type sentinelError struct{}

func (sentinelError) Error() string { return "" }

var errNone error = sentinelError{}

type ptyError struct {
	err error
}

func (p ptyError) Error() string {
	return p.err.Error()
}

func (p ptyError) Unwrap() error {
	return p.err
}

// Session wraps a single PTY test process backed by xpty.
type Session struct {
	id         string
	workingDir string
	command    []string
	env        []string
	encoding   encoding.Encoding
	encName    string

	rows int
	cols int

	createdAt  time.Time
	lastActive atomic.Int64

	cmd    *exec.Cmd
	pty    xpty.Pty
	cancel context.CancelFunc

	closeOnce sync.Once
	closed    chan struct{}
	err       atomic.Pointer[error]

	logger *zap.Logger
}

// SessionParams configure a PTY test session.
type SessionParams struct {
	ID         string
	WorkingDir string
	Command    []string
	Env        []string
	Rows       int
	Cols       int
	Encoding   string
	Logger     *zap.Logger
}

// NewSession spawns a PTY test session backed by xpty.
func NewSession(ctx context.Context, params SessionParams) (*Session, error) {
	if len(params.Command) == 0 {
		return nil, errors.New("shell command is required for PTY test")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if params.ID == "" {
		params.ID = utils.NewID()
	}

	enc, normalizedName, err := resolveEncoding(params.Encoding)
	if err != nil {
		return nil, err
	}

	rows := params.Rows
	if rows <= 0 {
		rows = 24
	}
	cols := params.Cols
	if cols <= 0 {
		cols = 80
	}

	ptyInstance, err := xpty.NewPty(cols, rows)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(sessionCtx, params.Command[0], params.Command[1:]...)
	cmd.Dir = params.WorkingDir

	env := append([]string{}, params.Env...)
	env = append(env, "TERM=xterm-256color")
	cmd.Env = append(os.Environ(), env...)

	if err := ptyInstance.Start(cmd); err != nil {
		cancel()
		_ = ptyInstance.Close()
		return nil, err //nolint:wrapcheck
	}

	logger := params.Logger
	if logger == nil {
		logger = utils.Logger()
	}

	session := &Session{
		id:         params.ID,
		workingDir: params.WorkingDir,
		command:    append([]string{}, params.Command...),
		env:        append([]string{}, params.Env...),
		rows:       rows,
		cols:       cols,
		createdAt:  time.Now(),
		cmd:        cmd,
		pty:        ptyInstance,
		cancel:     cancel,
		closed:     make(chan struct{}),
		encoding:   enc,
		encName:    normalizedName,
		logger:     logger.Named("pty-test-session").With(zap.String("sessionId", params.ID)),
	}
	session.Touch()
	session.err.Store(&errNone)

	go session.wait(sessionCtx)

	return session, nil
}

// ID returns the session identifier.
func (s *Session) ID() string {
	return s.id
}

// WorkingDir returns the working directory.
func (s *Session) WorkingDir() string {
	return s.workingDir
}

// Shell returns the shell command used to start the session.
func (s *Session) Shell() []string {
	return append([]string{}, s.command...)
}

// Encoding returns the configured charset name.
func (s *Session) Encoding() string {
	return s.encName
}

// Rows returns the current row count.
func (s *Session) Rows() int {
	return s.rows
}

// Cols returns the current column count.
func (s *Session) Cols() int {
	return s.cols
}

// CreatedAt returns the creation timestamp.
func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

// LastActive reports the last interaction timestamp.
func (s *Session) LastActive() time.Time {
	ts := s.lastActive.Load()
	if ts == 0 {
		return s.createdAt
	}
	return time.Unix(0, ts)
}

// Reader exposes the PTY as an io.Reader.
func (s *Session) Reader() io.Reader {
	return s.pty
}

// Write sends bytes into the PTY.
func (s *Session) Write(b []byte) (int, error) {
	payload := s.prepareInput(b)
	if len(payload) == 0 {
		return 0, nil
	}
	n, err := s.pty.Write(payload)
	if n > 0 {
		s.Touch()
	}
	return n, err
}

// Resize updates the PTY window size.
func (s *Session) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	if err := s.pty.Resize(cols, rows); err != nil {
		return err //nolint:wrapcheck
	}
	s.cols = cols
	s.rows = rows
	s.Touch()
	return nil
}

// Close terminates the session and underlying PTY.
func (s *Session) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		closeErr = s.pty.Close()
		close(s.closed)
	})
	return closeErr
}

// Closed returns a channel that is closed when the session ends.
func (s *Session) Closed() <-chan struct{} {
	return s.closed
}

// Err returns the stored terminal error, if any.
func (s *Session) Err() error {
	if ptr := s.err.Load(); ptr != nil {
		if errors.Is(*ptr, errNone) {
			return nil
		}
		return *ptr
	}
	return nil
}

// Touch updates the last-active timestamp.
func (s *Session) Touch() {
	s.lastActive.Store(time.Now().UnixNano())
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
		wrapped := wrapPTYError(err)
		s.err.Store(&wrapped)
		if s.logger != nil {
			s.logger.Debug("pty test session exited with error", zap.Error(err))
		}
	} else {
		s.err.Store(&errNone)
		if s.logger != nil {
			s.logger.Debug("pty test session exited normally")
		}
	}
	_ = s.Close()
}

func wrapPTYError(err error) error {
	if err == nil {
		return errNone
	}
	if errors.Is(err, errNone) {
		return errNone
	}
	return ptyError{err: err}
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
		return nil, normalized, fmt.Errorf("%w: %s", ErrInvalidEncoding, name)
	}
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
