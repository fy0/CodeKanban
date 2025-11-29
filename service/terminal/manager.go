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
	"code-kanban/utils/ai_assistant2"
	"code-kanban/utils/ai_assistant2/types"
)

// Config defines runtime constraints for terminal sessions.
type Config struct {
	Shell                  utils.TerminalShellConfig
	IdleTimeout            time.Duration
	MaxSessionsPerProject  int
	Encoding               string
	ScrollbackBytes        int
	AIAssistantStatus      utils.AIAssistantStatusConfig
	ScrollbackEnabled      bool
	RenameTitleEachCommand bool
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
	TaskID     string
}

// Manager orchestrates PTY sessions.
type Manager struct {
	cfg           Config
	sessionMu     sync.Mutex
	sessions      utils.SyncMap[string, *Session]
	logger        *zap.Logger
	encoding      string
	baseCtx       context.Context
	baseCtxMu     sync.RWMutex
	recordManager *RecordManager
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
		cfg:           cfg,
		logger:        logger.Named("terminal-manager"),
		encoding:      cfg.Encoding,
		baseCtx:       context.Background(),
		recordManager: NewRecordManager(),
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
		ScrollbackLimit: m.scrollbackLimit(),
		GetAIConfig: func() *utils.AIAssistantStatusConfig {
			m.sessionMu.Lock()
			defer m.sessionMu.Unlock()
			cfg := m.cfg.AIAssistantStatus
			return &cfg
		},
		TaskID:                 params.TaskID,
		RenameTitleEachCommand: m.cfg.RenameTitleEachCommand,
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

	if err := session.UpdateTitle(normalized); err != nil {
		return nil, err
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

// LinkTask associates a task with a terminal session.
func (m *Manager) LinkTask(sessionID, taskID string) (*Session, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	session.AssociateTask(taskID)
	return session, nil
}

// UnlinkTask removes the task association from a terminal session.
func (m *Manager) UnlinkTask(sessionID string) (*Session, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	session.ClearTaskAssociation()
	return session, nil
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
	go m.monitorAssistantRecords(session)
	<-session.Closed()
	m.recordManager.ClearSessionRecords(session.ID())
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

// UpdateAIAssistantStatusConfig updates the AI assistant status configuration for all sessions.
// This allows hot-reloading configuration without restarting the service.
func (m *Manager) UpdateAIAssistantStatusConfig(newConfig utils.AIAssistantStatusConfig) {
	m.sessionMu.Lock()
	m.cfg.AIAssistantStatus = newConfig
	m.sessionMu.Unlock()

	// Trigger metadata refresh for all active sessions
	// This will cause them to re-check their AI assistant status with the new config
	m.sessions.Range(func(_ string, session *Session) bool {
		// Just touching the session will trigger the next metadata update cycle
		// to re-evaluate the AI assistant with the new config
		session.Touch()
		return true
	})
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

// UpdateRenameTitleEachCommand toggles whether AI inputs rename terminal titles every time.
func (m *Manager) UpdateRenameTitleEachCommand(enabled bool) {
	m.sessionMu.Lock()
	m.cfg.RenameTitleEachCommand = enabled
	m.sessionMu.Unlock()

	m.sessions.Range(func(_ string, session *Session) bool {
		session.SetRenameTitleEachCommand(enabled)
		return true
	})
}

// GetRecordManager 返回记录管理器实例
func (m *Manager) GetRecordManager() *RecordManager {
	return m.recordManager
}

func (m *Manager) monitorAssistantRecords(session *Session) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := session.Subscribe(ctx)
	if err != nil {
		return
	}
	defer stream.Close()

	lastState := string(types.StateUnknown)

	for event := range stream.Events() {
		switch event.Type {
		case StreamEventMetadata:
			metadata := event.Metadata
			if metadata == nil || metadata.AIAssistant == nil {
				// AI 助手 detach 时，清除该 session 的所有记录
				if lastState != string(types.StateUnknown) {
					m.recordManager.ClearSessionRecords(session.ID())
					lastState = string(types.StateUnknown)
				}
				continue
			}
			state := metadata.AIAssistant.State
			if state == lastState && state != string(types.StateWaitingApproval) {
				continue
			}

			switch state {
			case string(types.StateWaitingInput):
				// 只有从 working 状态变为 waiting_input 才算完成任务
				// 避免在初始化时（unknown -> waiting_input）错误地创建完成记录
				if lastState == string(types.StateWorking) {
					m.handleSessionCompletionRecord(session, metadata.AIAssistant)
				}
			case string(types.StateWaitingApproval):
				if lastState != string(types.StateWaitingApproval) {
					m.handleSessionApprovalRecord(session, metadata.AIAssistant)
				}
			case string(types.StateWorking):
				// 确保有对应的通知，并标记为 working
				if !m.recordManager.UpdateCompletionStateBySession(session.ID(), "working") {
					m.handleSessionWorkingRecord(session, metadata.AIAssistant)
				}
				if lastState == string(types.StateWaitingApproval) {
					// 从审批状态恢复工作时也需要清理审批记录
					m.recordManager.ClearApprovalsBySession(session.ID())
				}
			default:
				if lastState == string(types.StateWaitingApproval) {
					m.recordManager.ClearApprovalsBySession(session.ID())
				}
			}

			lastState = state
		case StreamEventExit:
			return
		}
	}
}

func (m *Manager) handleSessionCompletionRecord(session *Session, info *ai_assistant2.AIAssistantInfo) {
	if session == nil || info == nil {
		return
	}

	record := &CompletionRecord{
		ID:          utils.NewID(),
		SessionID:   session.ID(),
		ProjectID:   session.ProjectID(),
		Title:       session.Title(),
		Assistant:   cloneAssistantInfo(info),
		CompletedAt: time.Now(),
		State:       "completed",
	}

	m.recordManager.ClearCompletionsBySession(session.ID())
	m.recordManager.AddCompletion(record)
}

func (m *Manager) handleSessionWorkingRecord(session *Session, info *ai_assistant2.AIAssistantInfo) {
	if session == nil {
		return
	}

	// 如果已经有记录则由调用方负责更新状态，这里仅在不存在时创建
	record := &CompletionRecord{
		ID:          utils.NewID(),
		SessionID:   session.ID(),
		ProjectID:   session.ProjectID(),
		Title:       session.Title(),
		Assistant:   cloneAssistantInfo(info),
		CompletedAt: time.Now(),
		State:       "working",
	}

	m.recordManager.ClearCompletionsBySession(session.ID())
	m.recordManager.AddCompletion(record)
}

func (m *Manager) handleSessionApprovalRecord(session *Session, info *ai_assistant2.AIAssistantInfo) {
	if session == nil || info == nil {
		return
	}

	record := &ApprovalRecord{
		ID:          utils.NewID(),
		SessionID:   session.ID(),
		ProjectID:   session.ProjectID(),
		Title:       session.Title(),
		Assistant:   cloneAssistantInfo(info),
		RequestedAt: time.Now(),
	}

	m.recordManager.ClearApprovalsBySession(session.ID())
	m.recordManager.AddApproval(record)
}

func cloneAssistantInfo(info *ai_assistant2.AIAssistantInfo) *ai_assistant2.AIAssistantInfo {
	if info == nil {
		return nil
	}
	copyInfo := *info
	return &copyInfo
}
