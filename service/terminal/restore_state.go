package terminal

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils/model_base"
)

const (
	terminalRestoreShellStateIdle    = "idle"
	terminalRestoreShellStateRunning = "running"
)

type terminalRestoreStateSnapshot struct {
	SessionID         string
	ProjectID         string
	WorktreeID        string
	Title             string
	OrderIndex        float64
	InitialWorkingDir string
	LastCwd           string
	ShellFamily       string
	ShellSupported    bool
	ShellState        string
	PendingCommand    *string
	ReplayEligible    bool
	CommandStartedAt  *time.Time
}

func (m *Manager) persistRestoreSession(ctx context.Context, session *Session) error {
	if session == nil {
		return nil
	}
	db := model.GetDB()
	if db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	snapshot := session.restoreStateSnapshot()
	record := tables.TerminalRestoreSessionTable{
		StringPKBaseModel: model_base.StringPKBaseModel{ID: snapshot.SessionID},
		ProjectID:         snapshot.ProjectID,
		WorktreeID:        snapshot.WorktreeID,
		Title:             snapshot.Title,
		OrderIndex:        snapshot.OrderIndex,
		InitialWorkingDir: snapshot.InitialWorkingDir,
		LastCwd:           snapshot.LastCwd,
		ShellFamily:       snapshot.ShellFamily,
		ShellSupported:    snapshot.ShellSupported,
		ShellState:        snapshot.ShellState,
		PendingCommand:    snapshot.PendingCommand,
		ReplayEligible:    snapshot.ReplayEligible,
		CommandStartedAt:  snapshot.CommandStartedAt,
	}
	if strings.TrimSpace(record.LastCwd) == "" {
		record.LastCwd = record.InitialWorkingDir
	}
	if strings.TrimSpace(record.ShellState) == "" {
		record.ShellState = terminalRestoreShellStateIdle
	}

	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"project_id",
			"worktree_id",
			"title",
			"order_index",
			"initial_working_dir",
			"last_cwd",
			"shell_family",
			"shell_supported",
			"shell_state",
			"pending_command",
			"replay_eligible",
			"command_started_at",
			"updated_at",
		}),
	}).Create(&record).Error
}

func (m *Manager) persistProjectRestoreSessions(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}

	m.sessionMu.Lock()
	ordered := append([]*Session(nil), m.orderedProjectSessionsLocked(projectID)...)
	m.sessionMu.Unlock()

	for _, session := range ordered {
		if err := m.persistRestoreSession(ctx, session); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) deleteRestoreSession(ctx context.Context, sessionID string) error {
	db := model.GetDB()
	if db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return db.WithContext(ctx).Delete(&tables.TerminalRestoreSessionTable{}, "id = ?", sessionID).Error
}

func (m *Manager) handleShellIntegrationEvent(session *Session, _ shellIntegrationEvent) {
	if session == nil {
		return
	}
	if err := m.persistRestoreSession(context.Background(), session); err != nil && m.logger != nil {
		m.logger.Warn("failed to persist terminal restore state",
			zap.String("sessionId", session.ID()),
			zap.Error(err))
	}
}

func (m *Manager) listRestoreSessions(ctx context.Context, projectID string) ([]tables.TerminalRestoreSessionTable, error) {
	db := model.GetDB()
	if db == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var records []tables.TerminalRestoreSessionTable
	if err := db.WithContext(ctx).
		Where("project_id = ?", strings.TrimSpace(projectID)).
		Order("order_index ASC").
		Order("created_at ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func restoreReplayCommand(record tables.TerminalRestoreSessionTable) string {
	if !record.ShellSupported || record.ShellState != terminalRestoreShellStateRunning {
		return ""
	}
	if !record.ReplayEligible || record.PendingCommand == nil {
		return ""
	}
	return strings.TrimSpace(*record.PendingCommand)
}

func (m *Manager) RestoreProjectSessions(ctx context.Context, projectID string) ([]SessionSnapshot, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, ErrSessionNotFound
	}

	liveSessions := m.ListSessions(projectID)
	if len(liveSessions) > 0 {
		return liveSessions, nil
	}

	records, err := m.listRestoreSessions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	createdSessions := make([]*Session, 0, len(records))
	for _, record := range records {
		workingDir := strings.TrimSpace(record.LastCwd)
		if workingDir == "" {
			workingDir = strings.TrimSpace(record.InitialWorkingDir)
		}
		session, createErr := m.CreateSession(ctx, CreateSessionParams{
			ID:                   record.ID,
			ProjectID:            record.ProjectID,
			WorktreeID:           record.WorktreeID,
			WorkingDir:           workingDir,
			Title:                record.Title,
			OrderIndex:           record.OrderIndex,
			StartupReplayCommand: restoreReplayCommand(record),
		})
		if createErr != nil {
			for _, created := range createdSessions {
				_ = created.Close()
			}
			return nil, createErr
		}
		createdSessions = append(createdSessions, session)
	}

	snapshots := make([]SessionSnapshot, 0, len(createdSessions))
	for _, session := range createdSessions {
		snapshots = append(snapshots, session.Snapshot())
	}
	return snapshots, nil
}
