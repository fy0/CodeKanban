package ptytest

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"

	"github.com/google/shlex"
	"go.uber.org/zap"

	"go-template/utils"
)

// Config defines runtime settings for the PTY test manager.
type Config struct {
	Shell utils.TerminalShellConfig
}

// CreateSessionParams defines inputs when creating a PTY test session.
type CreateSessionParams struct {
	ID         string
	WorkingDir string
	Shell      string
	Rows       int
	Cols       int
	Env        []string
	Encoding   string
}

// Manager maintains PTY test sessions in memory.
type Manager struct {
	cfg      Config
	mu       sync.RWMutex
	sessions map[string]*Session
	logger   *zap.Logger
}

// NewManager builds a PTY test manager.
func NewManager(cfg Config, logger *zap.Logger) *Manager {
	if logger == nil {
		logger = utils.Logger()
	}
	return &Manager{
		cfg:      cfg,
		sessions: make(map[string]*Session),
		logger:   logger.Named("pty-test-manager"),
	}
}

// CreateSession starts a new PTY test session.
func (m *Manager) CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error) {
	command, err := m.shellCommand(params.Shell)
	if err != nil {
		return nil, err
	}

	session, err := NewSession(ctx, SessionParams{
		ID:         params.ID,
		WorkingDir: params.WorkingDir,
		Command:    command,
		Env:        params.Env,
		Rows:       params.Rows,
		Cols:       params.Cols,
		Encoding:   params.Encoding,
		Logger:     m.logger,
	})
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[session.ID()] = session
	m.mu.Unlock()

	go m.watchSession(session)

	return session, nil
}

// GetSession returns a PTY test session by ID.
func (m *Manager) GetSession(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session, ok := m.sessions[id]; ok {
		return session, nil
	}
	return nil, ErrSessionNotFound
}

// CloseSession terminates and removes a PTY test session.
func (m *Manager) CloseSession(id string) error {
	session, err := m.GetSession(id)
	if err != nil {
		return err
	}
	return session.Close()
}

func (m *Manager) watchSession(session *Session) {
	<-session.Closed()
	m.mu.Lock()
	delete(m.sessions, session.ID())
	m.mu.Unlock()
}

func (m *Manager) shellCommand(override string) ([]string, error) {
	raw := strings.TrimSpace(override)
	if raw == "" {
		switch runtime.GOOS {
		case "windows":
			raw = m.cfg.Shell.Windows
		case "darwin":
			raw = m.cfg.Shell.Darwin
		default:
			raw = m.cfg.Shell.Linux
		}
		raw = strings.TrimSpace(raw)
	}

	if raw == "" {
		switch runtime.GOOS {
		case "windows":
			raw = "powershell.exe -NoLogo"
		case "darwin":
			raw = "/bin/zsh"
		default:
			raw = "/bin/bash"
		}
	}

	parts, err := shlex.Split(raw)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, errors.New("invalid shell configuration")
	}
	return parts, nil
}
