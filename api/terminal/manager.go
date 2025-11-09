package terminal

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/shlex"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"go-template/utils"
)

// Config defines runtime constraints for terminal sessions.
type Config struct {
	Shell                 utils.TerminalShellConfig
	IdleTimeout           time.Duration
	MaxSessionsPerProject int
	Encoding              string
}

// CreateSessionParams describes API level inputs.
type CreateSessionParams struct {
	ID         string
	ProjectID  string
	WorktreeID string
	WorkingDir string
	Title      string
	Env        []string
	Rows       int
	Cols       int
	Encoding   string
}

// Manager orchestrates PTY sessions.
type Manager struct {
	cfg      Config
	mu       sync.RWMutex
	sessions map[string]*Session
	logger   *zap.Logger
	metrics  *metrics
	encoding string
}

// NewManager builds a manager instance.
func NewManager(cfg Config, logger *zap.Logger) *Manager {
	cfg.Encoding = strings.ToLower(strings.TrimSpace(cfg.Encoding))
	if logger == nil {
		logger = utils.Logger()
	}

	mgr := &Manager{
		cfg:      cfg,
		sessions: make(map[string]*Session),
		logger:   logger.Named("terminal-manager"),
		metrics:  newMetrics(),
		encoding: cfg.Encoding,
	}
	return mgr
}

// StartBackground kicks off cleanup goroutines.
func (m *Manager) StartBackground(ctx context.Context) {
	go m.reapIdleSessions(ctx)
}

// CreateSession spawns a PTY session respecting per-project limits.
func (m *Manager) CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error) {
	if params.ProjectID == "" || params.WorktreeID == "" {
		return nil, errors.New("projectId and worktreeId are required")
	}

	command, err := m.shellCommand()
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg.MaxSessionsPerProject > 0 && m.countByProjectLocked(params.ProjectID) >= m.cfg.MaxSessionsPerProject {
		return nil, ErrSessionLimitReached
	}

	if params.ID == "" {
		params.ID = utils.NewID()
	}

	session, err := NewSession(SessionParams{
		ID:         params.ID,
		ProjectID:  params.ProjectID,
		WorktreeID: params.WorktreeID,
		WorkingDir: params.WorkingDir,
		Title:      params.Title,
		Command:    command,
		Env:        params.Env,
		Rows:       params.Rows,
		Cols:       params.Cols,
		Logger:     m.logger,
		Encoding:   m.cfg.Encoding,
	})
	if err != nil {
		return nil, err
	}

	if err := session.Start(ctx); err != nil {
		return nil, err
	}

	m.sessions[session.ID()] = session
	m.metrics.activeSessionGauge.With(prometheus.Labels{"project_id": params.ProjectID}).Inc()

	go m.watchSession(session)

	return session, nil
}

// GetSession returns a session by identifier.
func (m *Manager) GetSession(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// CloseSession terminates and removes the session immediately.
func (m *Manager) CloseSession(id string) error {
	session, err := m.GetSession(id)
	if err != nil {
		return err
	}
	return session.Close()
}

// ListSessions enumerates sessions, optionally filtering by project.
func (m *Manager) ListSessions(projectID string) []SessionSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]SessionSnapshot, 0, len(m.sessions))
	for _, session := range m.sessions {
		if projectID != "" && session.ProjectID() != projectID {
			continue
		}
		results = append(results, session.Snapshot())
	}
	return results
}

func (m *Manager) shellCommand() ([]string, error) {
	var raw string
	switch runtime.GOOS {
	case "windows":
		raw = m.cfg.Shell.Windows
	case "darwin":
		raw = m.cfg.Shell.Darwin
	default:
		raw = m.cfg.Shell.Linux
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if runtime.GOOS == "windows" {
			raw = "powershell.exe -NoLogo"
		} else if runtime.GOOS == "darwin" {
			raw = "/bin/zsh"
		} else {
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

func (m *Manager) watchSession(session *Session) {
	<-session.Closed()
	m.mu.Lock()
	delete(m.sessions, session.ID())
	m.mu.Unlock()
	m.metrics.activeSessionGauge.With(prometheus.Labels{"project_id": session.ProjectID()}).Dec()
}

func (m *Manager) countByProjectLocked(projectID string) int {
	count := 0
	for _, session := range m.sessions {
		if session.ProjectID() == projectID {
			count++
		}
	}
	return count
}

func (m *Manager) reapIdleSessions(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.cleanupIdle()
		}
	}
}

func (m *Manager) cleanupIdle() {
	if m.cfg.IdleTimeout <= 0 {
		return
	}
	now := time.Now()

	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()

	for _, session := range sessions {
		if now.Sub(session.LastActive()) > m.cfg.IdleTimeout {
			m.logger.Info("closing idle terminal session",
				zap.String("sessionId", session.ID()),
				zap.String("projectId", session.ProjectID()),
				zap.Duration("idle", now.Sub(session.LastActive())),
			)
			_ = session.Close()
		}
	}
}

// ReportIO records bytes flowing through the PTY for Prometheus metrics.
func (m *Manager) ReportIO(projectID string, direction string, n int) {
	if n <= 0 {
		return
	}
	m.metrics.ioBytesCounter.With(prometheus.Labels{
		"project_id": projectID,
		"direction":  direction,
	}).Add(float64(n))
}
