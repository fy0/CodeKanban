package terminal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"code-kanban/utils"
)

const terminalSessionOrderStep = 1000.0

// Config defines runtime constraints for terminal sessions.
type Config struct {
	Shell                 utils.TerminalShellConfig
	IdleTimeout           time.Duration
	MaxSessionsPerProject int
	Encoding              string
	ScrollbackBytes       int
	ScrollbackEnabled     bool
	TerminalStateSnapshot bool
}

// CreateSessionParams describes API level inputs.
type CreateSessionParams struct {
	ID                   string
	ProjectID            string
	WorktreeID           string
	WorkingDir           string
	Title                string
	Env                  []string
	Rows                 int
	Cols                 int
	Encoding             string
	TaskID               string
	OrderIndex           float64
	InsertAfterSessionID string
	StartupReplayCommand string
}

// SessionListEvent broadcasts the current project terminal session list.
type SessionListEvent struct {
	Type      string            `json:"type"`
	ProjectID string            `json:"projectId"`
	Sessions  []SessionSnapshot `json:"sessions"`
}

// Manager orchestrates PTY sessions.
type Manager struct {
	cfg                     Config
	sessionMu               sync.Mutex
	sessions                utils.SyncMap[string, *Session]
	logger                  *zap.Logger
	encoding                string
	baseCtx                 context.Context
	baseCtxMu               sync.RWMutex
	sessionEventMu          sync.RWMutex
	sessionEventSubscribers map[chan SessionListEvent]struct{}
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
		cfg:                     cfg,
		logger:                  logger.Named("terminal-manager"),
		encoding:                cfg.Encoding,
		baseCtx:                 context.Background(),
		sessionEventSubscribers: make(map[chan SessionListEvent]struct{}),
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
	command, integrationEnv, shellIntegration, err := prepareShellIntegration(command)
	if err != nil {
		return nil, err
	}

	if params.ID == "" {
		params.ID = utils.NewID()
	}

	env := append([]string{}, params.Env...)
	env = append(env, integrationEnv...)

	var session *Session
	session, err = NewSession(SessionParams{
		ID:                          params.ID,
		ProjectID:                   params.ProjectID,
		WorktreeID:                  params.WorktreeID,
		WorkingDir:                  params.WorkingDir,
		Title:                       params.Title,
		Command:                     command,
		Env:                         env,
		Rows:                        params.Rows,
		Cols:                        params.Cols,
		Logger:                      m.logger,
		Encoding:                    m.cfg.Encoding,
		ScrollbackLimit:             m.scrollbackLimit(),
		EnableTerminalStateSnapshot: m.cfg.TerminalStateSnapshot,
		TaskID:                      params.TaskID,
		OrderIndex:                  params.OrderIndex,
		ShellIntegration:            shellIntegration,
		StartupReplayCommand:        params.StartupReplayCommand,
		OnShellEvent: func(event shellIntegrationEvent) {
			if session == nil {
				return
			}
			m.handleShellIntegrationEvent(session, event)
		},
	})
	if err != nil {
		if shellIntegration.Cleanup != nil {
			shellIntegration.Cleanup()
		}
		return nil, err
	}

	if err := m.addSession(session, params.InsertAfterSessionID); err != nil {
		_ = session.Close()
		return nil, err
	}
	if err := m.persistProjectRestoreSessions(context.Background(), params.ProjectID); err != nil {
		m.sessions.Delete(session.ID())
		_ = session.Close()
		_ = m.deleteRestoreSession(context.Background(), session.ID())
		return nil, err
	}

	startCtx := m.sessionContext()
	if err := startCtx.Err(); err != nil {
		m.sessions.Delete(session.ID())
		_ = session.Close()
		_ = m.deleteRestoreSession(context.Background(), session.ID())
		return nil, err
	}

	if err := session.Start(startCtx); err != nil {
		m.sessions.Delete(session.ID())
		_ = session.Close()
		_ = m.deleteRestoreSession(context.Background(), session.ID())
		return nil, err
	}

	go m.watchSession(session)
	m.broadcastProjectSessions(session.ProjectID())

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

	if err := session.UpdateTitle(normalized); err != nil {
		return nil, err
	}
	if err := m.persistRestoreSession(context.Background(), session); err != nil {
		return nil, err
	}
	m.broadcastProjectSessions(session.ProjectID())
	return session, nil
}

// MoveSession reorders a terminal session within its project.
func (m *Manager) MoveSession(projectID, sessionID, prevSessionID, nextSessionID string) (*Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, ErrSessionNotFound
	}

	m.sessionMu.Lock()
	session, ok := m.sessions.Load(sessionID)
	if !ok {
		m.sessionMu.Unlock()
		return nil, ErrSessionNotFound
	}
	resolvedProjectID := session.ProjectID()
	if projectID != "" && projectID != resolvedProjectID {
		m.sessionMu.Unlock()
		return nil, ErrSessionNotFound
	}
	if err := m.moveSessionLocked(resolvedProjectID, sessionID, prevSessionID, nextSessionID); err != nil {
		m.sessionMu.Unlock()
		return nil, err
	}
	m.sessionMu.Unlock()
	if err := m.persistProjectRestoreSessions(context.Background(), resolvedProjectID); err != nil {
		return nil, err
	}

	m.broadcastProjectSessions(resolvedProjectID)
	return session, nil
}

// CloseSession terminates and removes the session immediately.
func (m *Manager) CloseSession(id string) error {
	session, err := m.GetSession(id)
	if err != nil {
		return err
	}
	if closeErr := session.Close(); closeErr != nil && m.logger != nil {
		m.logger.Warn("terminal session closed with warning",
			zap.String("sessionId", id),
			zap.Error(closeErr))
	}
	return nil
}

// LinkTask associates a task with a terminal session.
func (m *Manager) LinkTask(sessionID, taskID string) (*Session, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	session.AssociateTask(taskID)
	if err := m.persistRestoreSession(context.Background(), session); err != nil {
		return nil, err
	}
	m.broadcastProjectSessions(session.ProjectID())
	return session, nil
}

// UnlinkTask removes the task association from a terminal session.
func (m *Manager) UnlinkTask(sessionID string) (*Session, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	session.ClearTaskAssociation()
	if err := m.persistRestoreSession(context.Background(), session); err != nil {
		return nil, err
	}
	m.broadcastProjectSessions(session.ProjectID())
	return session, nil
}

// ListSessions enumerates sessions, optionally filtering by project.
func (m *Manager) ListSessions(projectID string) []SessionSnapshot {
	m.sessionMu.Lock()
	ordered := m.orderedProjectSessionsLocked(projectID)
	m.sessionMu.Unlock()

	results := make([]SessionSnapshot, 0, len(ordered))
	for _, session := range ordered {
		results = append(results, session.Snapshot())
	}
	return results
}

// SubscribeSessionListEvents subscribes to terminal project list updates.
func (m *Manager) SubscribeSessionListEvents() (<-chan SessionListEvent, func()) {
	ch := make(chan SessionListEvent, 16)
	m.sessionEventMu.Lock()
	m.sessionEventSubscribers[ch] = struct{}{}
	m.sessionEventMu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			m.sessionEventMu.Lock()
			delete(m.sessionEventSubscribers, ch)
			close(ch)
			m.sessionEventMu.Unlock()
		})
	}
	return ch, cancel
}

func (m *Manager) broadcastProjectSessions(projectID string) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return
	}
	event := SessionListEvent{
		Type:      "sessions",
		ProjectID: projectID,
		Sessions:  m.ListSessions(projectID),
	}
	m.sessionEventMu.RLock()
	defer m.sessionEventMu.RUnlock()
	for ch := range m.sessionEventSubscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

// ListSessionsByTask enumerates sessions associated with a specific task.
func (m *Manager) ListSessionsByTask(taskID string) []SessionSnapshot {
	results := make([]SessionSnapshot, 0)
	if taskID == "" {
		return results
	}
	m.sessions.Range(func(_ string, session *Session) bool {
		if session.TaskID() == taskID {
			results = append(results, session.Snapshot())
		}
		return true
	})
	return results
}

// GetSessionDebugInfo returns comprehensive debug information for a session.
func (m *Manager) GetSessionDebugInfo(id string) (*DebugInfo, error) {
	session, err := m.GetSession(id)
	if err != nil {
		return nil, err
	}
	return session.GetDebugInfo(), nil
}

// CaptureChunk triggers a resize and captures the next output chunk from a session.
func (m *Manager) CaptureChunk(ctx context.Context, id string, timeout time.Duration) (*CapturedChunk, error) {
	session, err := m.GetSession(id)
	if err != nil {
		return nil, err
	}
	return session.CaptureNextChunk(ctx, timeout)
}

func (m *Manager) shellCommand() ([]string, error) {
	return utils.ResolveShellCommand("", m.cfg.Shell)
}

func (m *Manager) watchSession(session *Session) {
	<-session.Closed()
	projectID := session.ProjectID()
	m.sessionMu.Lock()
	m.sessions.Delete(session.ID())
	m.normalizeProjectOrderLocked(projectID)
	m.sessionMu.Unlock()
	if err := m.deleteRestoreSession(context.Background(), session.ID()); err != nil && m.logger != nil {
		m.logger.Warn("failed to delete terminal restore session",
			zap.String("sessionId", session.ID()),
			zap.Error(err))
	}
	if err := m.persistProjectRestoreSessions(context.Background(), projectID); err != nil && m.logger != nil {
		m.logger.Warn("failed to normalize terminal restore ordering after close",
			zap.String("projectId", projectID),
			zap.Error(err))
	}
	m.broadcastProjectSessions(projectID)
}

func (m *Manager) addSession(session *Session, insertAfterSessionID string) error {
	m.sessionMu.Lock()
	defer m.sessionMu.Unlock()

	if m.cfg.MaxSessionsPerProject > 0 && m.countByProject(session.ProjectID()) >= m.cfg.MaxSessionsPerProject {
		return ErrSessionLimitReached
	}

	if session.OrderIndex() <= 0 {
		session.SetOrderIndex(m.nextSessionOrderIndexLocked(session.ProjectID()))
	}
	m.sessions.Store(session.ID(), session)
	if strings.TrimSpace(insertAfterSessionID) != "" {
		if err := m.moveSessionLocked(session.ProjectID(), session.ID(), insertAfterSessionID, ""); err != nil && !errors.Is(err, ErrInvalidSessionMoveTarget) {
			m.sessions.Delete(session.ID())
			return err
		}
	}
	m.normalizeProjectOrderLocked(session.ProjectID())
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

func (m *Manager) orderedProjectSessionsLocked(projectID string) []*Session {
	items := make([]*Session, 0)
	m.sessions.Range(func(_ string, session *Session) bool {
		if projectID == "" || session.ProjectID() == projectID {
			items = append(items, session)
		}
		return true
	})
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		leftOrder := left.OrderIndex()
		rightOrder := right.OrderIndex()
		if leftOrder > 0 || rightOrder > 0 {
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
		}
		if !left.CreatedAt().Equal(right.CreatedAt()) {
			return left.CreatedAt().Before(right.CreatedAt())
		}
		return left.ID() < right.ID()
	})
	return items
}

func (m *Manager) nextSessionOrderIndexLocked(projectID string) float64 {
	maxOrder := 0.0
	m.sessions.Range(func(_ string, session *Session) bool {
		if session.ProjectID() == projectID && session.OrderIndex() > maxOrder {
			maxOrder = session.OrderIndex()
		}
		return true
	})
	return maxOrder + terminalSessionOrderStep
}

func (m *Manager) normalizeProjectOrderLocked(projectID string) {
	if strings.TrimSpace(projectID) == "" {
		return
	}
	ordered := m.orderedProjectSessionsLocked(projectID)
	for index, session := range ordered {
		session.SetOrderIndex(float64(index+1) * terminalSessionOrderStep)
	}
}

func resolveTerminalSessionInsertIndex(
	sessions []*Session,
	sessionID string,
	prevSessionID string,
	nextSessionID string,
) (int, error) {
	prevSessionID = strings.TrimSpace(prevSessionID)
	nextSessionID = strings.TrimSpace(nextSessionID)
	if prevSessionID != "" && prevSessionID == nextSessionID {
		return 0, ErrInvalidSessionMoveTarget
	}
	if prevSessionID == sessionID || nextSessionID == sessionID {
		return 0, ErrInvalidSessionMoveTarget
	}

	findIndex := func(targetID string) int {
		for index, item := range sessions {
			if item.ID() == targetID {
				return index
			}
		}
		return -1
	}

	if nextSessionID != "" {
		nextIndex := findIndex(nextSessionID)
		if nextIndex == -1 {
			return 0, ErrInvalidSessionMoveTarget
		}
		if prevSessionID != "" {
			prevIndex := findIndex(prevSessionID)
			if prevIndex == -1 || prevIndex >= nextIndex {
				return 0, ErrInvalidSessionMoveTarget
			}
		}
		return nextIndex, nil
	}

	if prevSessionID != "" {
		prevIndex := findIndex(prevSessionID)
		if prevIndex == -1 {
			return 0, ErrInvalidSessionMoveTarget
		}
		return prevIndex + 1, nil
	}

	return 0, nil
}

func (m *Manager) moveSessionLocked(projectID, sessionID, prevSessionID, nextSessionID string) error {
	ordered := m.orderedProjectSessionsLocked(projectID)
	if len(ordered) == 0 {
		return ErrSessionNotFound
	}

	filtered := make([]*Session, 0, len(ordered)-1)
	movingFound := false
	for _, session := range ordered {
		if session.ID() == sessionID {
			movingFound = true
			continue
		}
		filtered = append(filtered, session)
	}
	if !movingFound {
		return ErrSessionNotFound
	}

	insertIndex, err := resolveTerminalSessionInsertIndex(filtered, sessionID, prevSessionID, nextSessionID)
	if err != nil {
		return err
	}
	moving, _ := m.sessions.Load(sessionID)
	reordered := make([]*Session, 0, len(ordered))
	reordered = append(reordered, filtered[:insertIndex]...)
	reordered = append(reordered, moving)
	reordered = append(reordered, filtered[insertIndex:]...)
	for index, session := range reordered {
		session.SetOrderIndex(float64(index+1) * terminalSessionOrderStep)
	}
	return nil
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

func (m *Manager) scrollbackLimit() int {
	if !m.cfg.ScrollbackEnabled {
		return 0
	}
	if m.cfg.ScrollbackBytes <= 0 {
		return 0
	}
	return m.cfg.ScrollbackBytes
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

// UpdateScrollbackEnabled toggles scrollback buffering in real time for all sessions.
func (m *Manager) UpdateScrollbackEnabled(enabled bool) {
	m.sessionMu.Lock()
	m.cfg.ScrollbackEnabled = enabled
	limit := 0
	if enabled && m.cfg.ScrollbackBytes > 0 {
		limit = m.cfg.ScrollbackBytes
	}
	m.sessionMu.Unlock()

	m.sessions.Range(func(_ string, session *Session) bool {
		session.UpdateScrollbackLimit(limit)
		return true
	})
}

// UpdateTerminalStateSnapshotEnabled toggles server-side terminal state snapshots in real time.
func (m *Manager) UpdateTerminalStateSnapshotEnabled(enabled bool) {
	m.sessionMu.Lock()
	m.cfg.TerminalStateSnapshot = enabled
	m.sessionMu.Unlock()

	m.sessions.Range(func(_ string, session *Session) bool {
		session.SetTerminalStateSnapshotEnabled(enabled)
		return true
	})
}

// UpdateShellConfig updates the shell configuration for new terminal sessions.
func (m *Manager) UpdateShellConfig(shellConfig utils.TerminalShellConfig) {
	m.sessionMu.Lock()
	m.cfg.Shell = shellConfig
	m.sessionMu.Unlock()
}
