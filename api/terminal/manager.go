package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"code-kanban/utils"
)

// Config defines runtime constraints for terminal sessions.
type Config struct {
	Shell                 utils.TerminalShellConfig
	IdleTimeout           time.Duration
	MaxSessionsPerProject int
	Encoding              string
	ScrollbackBytes       int
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
	cfg       Config
	sessionMu sync.Mutex
	sessions  utils.SyncMap[string, *Session]
	logger    *zap.Logger
	encoding  string
	baseCtx   context.Context
	baseCtxMu sync.RWMutex
}

// NewManager builds a manager instance.
func NewManager(cfg Config, logger *zap.Logger) *Manager {
	cfg.Encoding = strings.ToLower(strings.TrimSpace(cfg.Encoding))
	if cfg.ScrollbackBytes <= 0 {
		cfg.ScrollbackBytes = 256 * 1024
	}
	if logger == nil {
		logger = utils.Logger()
	}

	mgr := &Manager{
		cfg:      cfg,
		logger:   logger.Named("terminal-manager"),
		encoding: cfg.Encoding,
		baseCtx:  context.Background(),
	}
	return mgr
}

// StartBackground kicks off cleanup goroutines.
func (m *Manager) StartBackground(ctx context.Context) {
	ctx = m.setBaseContext(ctx)
	go m.reapIdleSessions(ctx)
}

// CreateSession spawns a PTY session respecting per-project limits.
func (m *Manager) CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error) {
	if params.ProjectID == "" || params.WorktreeID == "" {
		return nil, errors.New("projectId and worktreeId are required")
	}

	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	command, err := m.shellCommand()
	if err != nil {
		return nil, err
	}

	if params.ID == "" {
		params.ID = utils.NewID()
	}

	session, err := NewSession(SessionParams{
		ID:              params.ID,
		ProjectID:       params.ProjectID,
		WorktreeID:      params.WorktreeID,
		WorkingDir:      params.WorkingDir,
		Title:           params.Title,
		Command:         command,
		Env:             params.Env,
		Rows:            params.Rows,
		Cols:            params.Cols,
		Logger:          m.logger,
		Encoding:        m.cfg.Encoding,
		ScrollbackLimit: m.cfg.ScrollbackBytes,
	})
	if err != nil {
		return nil, err
	}

	if err := m.addSession(session); err != nil {
		return nil, err
	}

	startCtx := m.sessionContext()
	if err := startCtx.Err(); err != nil {
		m.sessions.Delete(session.ID())
		_ = session.Close()
		return nil, err
	}

	if err := session.Start(startCtx); err != nil {
		m.sessions.Delete(session.ID())
		_ = session.Close()
		return nil, err
	}

	go m.watchSession(session)

	return session, nil
}

// GetSession returns a session by identifier.
func (m *Manager) GetSession(id string) (*Session, error) {
	session, ok := m.sessions.Load(id)
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// RenameSession updates the title of the targeted session.
func (m *Manager) RenameSession(projectID, sessionID, title string) (*Session, error) {
	normalized := strings.TrimSpace(title)
	if normalized == "" {
		return nil, ErrInvalidSessionTitle
	}
	if utf8.RuneCountInString(normalized) > 64 {
		return nil, fmt.Errorf("%w: title length must be <= 64 characters", ErrInvalidSessionTitle)
	}

	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if projectID != "" && session.ProjectID() != projectID {
		return nil, ErrSessionNotFound
	}

	session.UpdateTitle(normalized)
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
	results := make([]SessionSnapshot, 0)
	m.sessions.Range(func(_ string, session *Session) bool {
		if projectID != "" && session.ProjectID() != projectID {
			return true
		}
		results = append(results, session.Snapshot())
		return true
	})
	return results
}

func (m *Manager) shellCommand() ([]string, error) {
	return utils.ResolveShellCommand("", m.cfg.Shell)
}

func (m *Manager) watchSession(session *Session) {
	<-session.Closed()
	m.sessions.Delete(session.ID())
}

func (m *Manager) addSession(session *Session) error {
	if m.cfg.MaxSessionsPerProject <= 0 {
		m.sessions.Store(session.ID(), session)
		return nil
	}

	m.sessionMu.Lock()
	defer m.sessionMu.Unlock()

	if m.countByProject(session.ProjectID()) >= m.cfg.MaxSessionsPerProject {
		return ErrSessionLimitReached
	}

	m.sessions.Store(session.ID(), session)
	return nil
}

func (m *Manager) countByProject(projectID string) int {
	count := 0
	m.sessions.Range(func(_ string, session *Session) bool {
		if session.ProjectID() == projectID {
			count++
		}
		return true
	})
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

	sessions := make([]*Session, 0, m.sessions.Len())
	m.sessions.Range(func(_ string, session *Session) bool {
		sessions = append(sessions, session)
		return true
	})

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

func (m *Manager) setBaseContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	m.baseCtxMu.Lock()
	m.baseCtx = ctx
	m.baseCtxMu.Unlock()
	return ctx
}

func (m *Manager) sessionContext() context.Context {
	m.baseCtxMu.RLock()
	ctx := m.baseCtx
	m.baseCtxMu.RUnlock()
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
