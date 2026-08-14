package websession

import (
	"context"
	"errors"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils/model_base"

	"gorm.io/gorm"
)

var ErrInvalidHistoryArchive = errors.New("invalid web session history archive request")

type HistoryArchiveParams struct {
	Scope         HistoryCleanupScope `json:"scope"`
	ProjectIDs    []string            `json:"projectIds,omitempty"`
	OlderThanDays int                 `json:"olderThanDays"`
}

type HistoryArchiveStats struct {
	ScopedProjectCount      int `json:"scopedProjectCount"`
	ScopedSessionCount      int `json:"scopedSessionCount"`
	CandidateSessionCount   int `json:"candidateSessionCount"`
	SkippedBusySessionCount int `json:"skippedBusySessionCount"`
}

type HistoryArchiveResult struct {
	HistoryArchiveStats
	ArchivedSessionIDs []string `json:"archivedSessionIds"`
}

type historyArchivePlan struct {
	stats      HistoryArchiveStats
	sessionIDs []string
}

func (m *Manager) PreviewHistoryArchive(ctx context.Context, params HistoryArchiveParams) (HistoryArchiveStats, error) {
	plan, err := m.buildHistoryArchivePlan(ctx, params, time.Now())
	if err != nil {
		return HistoryArchiveStats{}, err
	}
	return plan.stats, nil
}

func (m *Manager) RunHistoryArchive(ctx context.Context, params HistoryArchiveParams) (HistoryArchiveResult, error) {
	m.historyCleanupMu.Lock()
	defer m.historyCleanupMu.Unlock()
	releaseDispatchLocks := m.lockHistoryCleanupDispatchLocks()
	defer releaseDispatchLocks()

	plan, err := m.buildHistoryArchivePlan(ctx, params, time.Now())
	if err != nil {
		return HistoryArchiveResult{}, err
	}
	db := model.GetDB()
	if db == nil {
		return HistoryArchiveResult{}, model.ErrDBNotInitialized
	}
	now := time.Now()
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, ids := range chunkHistoryCleanupIDs(plan.sessionIDs) {
			if err := tx.Model(&tables.WebSessionTable{}).
				Where("id IN ? AND deleted_at IS NULL AND archived_at IS NULL", ids).
				Updates(withSnapshotRevisionIncrement(map[string]any{
					"archived_at":                now,
					"has_unread":                 false,
					"updated_at":                 now,
					"auto_retry_attempt":         0,
					"auto_retry_next_at":         nil,
					"auto_retry_last_error_code": nil,
				})).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return HistoryArchiveResult{}, err
	}
	model_base.FlushWAL(db)
	return HistoryArchiveResult{
		HistoryArchiveStats: plan.stats,
		ArchivedSessionIDs:  append([]string(nil), plan.sessionIDs...),
	}, nil
}

func (m *Manager) buildHistoryArchivePlan(ctx context.Context, params HistoryArchiveParams, now time.Time) (historyArchivePlan, error) {
	normalized, err := normalizeHistoryArchiveParams(params)
	if err != nil {
		return historyArchivePlan{}, err
	}
	db := model.GetDB()
	if db == nil {
		return historyArchivePlan{}, model.ErrDBNotInitialized
	}
	if normalized.Scope == HistoryCleanupScopeProjects {
		var count int64
		if err := db.WithContext(ctx).Model(&tables.ProjectTable{}).
			Where("id IN ?", normalized.ProjectIDs).Count(&count).Error; err != nil {
			return historyArchivePlan{}, err
		}
		if count != int64(len(normalized.ProjectIDs)) {
			return historyArchivePlan{}, ErrInvalidHistoryArchive
		}
	}
	query := db.WithContext(ctx).Unscoped().Model(&tables.WebSessionTable{}).
		Where("deleted_at IS NULL AND archived_at IS NULL")
	if normalized.Scope == HistoryCleanupScopeProjects {
		query = query.Where("project_id IN ?", normalized.ProjectIDs)
	}
	var sessions []tables.WebSessionTable
	if err := query.Find(&sessions).Error; err != nil {
		return historyArchivePlan{}, err
	}
	busyIDs, err := m.historyCleanupBusySessionIDs(ctx, db, sessions)
	if err != nil {
		return historyArchivePlan{}, err
	}
	cutoff := now.Add(-time.Duration(normalized.OlderThanDays) * 24 * time.Hour)
	projectSet := make(map[string]struct{})
	for _, session := range sessions {
		if projectID := strings.TrimSpace(session.ProjectID); projectID != "" {
			projectSet[projectID] = struct{}{}
		}
	}
	stats := HistoryArchiveStats{
		ScopedProjectCount: len(projectSet),
		ScopedSessionCount: len(sessions),
	}
	selected := make([]string, 0)
	for _, session := range sessions {
		if normalized.OlderThanDays > 0 && !historyCleanupActivityAt(session).Before(cutoff) {
			continue
		}
		if _, ok := busyIDs[session.ID]; ok {
			stats.SkippedBusySessionCount++
			continue
		}
		selected = append(selected, session.ID)
	}
	stats.CandidateSessionCount = len(selected)
	return historyArchivePlan{stats: stats, sessionIDs: selected}, nil
}

func normalizeHistoryArchiveParams(params HistoryArchiveParams) (HistoryArchiveParams, error) {
	if params.OlderThanDays < 0 || params.OlderThanDays > 36500 {
		return HistoryArchiveParams{}, ErrInvalidHistoryArchive
	}
	params.Scope = HistoryCleanupScope(strings.ToLower(strings.TrimSpace(string(params.Scope))))
	if params.Scope != HistoryCleanupScopeAll && params.Scope != HistoryCleanupScopeProjects {
		return HistoryArchiveParams{}, ErrInvalidHistoryArchive
	}
	seen := make(map[string]struct{})
	projectIDs := make([]string, 0, len(params.ProjectIDs))
	for _, projectID := range params.ProjectIDs {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			continue
		}
		if _, ok := seen[projectID]; ok {
			continue
		}
		seen[projectID] = struct{}{}
		projectIDs = append(projectIDs, projectID)
	}
	if params.Scope == HistoryCleanupScopeProjects && len(projectIDs) == 0 {
		return HistoryArchiveParams{}, ErrInvalidHistoryArchive
	}
	if params.Scope == HistoryCleanupScopeAll {
		projectIDs = nil
	}
	params.ProjectIDs = projectIDs
	return params, nil
}
