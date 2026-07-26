package websession

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils/model_base"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type HistoryCleanupScope string

const (
	HistoryCleanupScopeAll      HistoryCleanupScope = "all"
	HistoryCleanupScopeProjects HistoryCleanupScope = "projects"
	historyCleanupChunkSize                         = 400
)

var ErrInvalidHistoryCleanup = errors.New("invalid web session history cleanup request")

type HistoryCleanupParams struct {
	Scope            HistoryCleanupScope `json:"scope"`
	ProjectIDs       []string            `json:"projectIds,omitempty"`
	OlderThanDays    int                 `json:"olderThanDays"`
	RetainPerProject int                 `json:"retainPerProject"`
}

type HistoryCleanupStorageStats struct {
	PageSizeBytes int64 `json:"pageSizeBytes"`
	PageCount     int64 `json:"pageCount"`
	FreePageCount int64 `json:"freePageCount"`
	ReusableBytes int64 `json:"reusableBytes"`
}

type HistoryCleanupStats struct {
	ScopedProjectCount      int                        `json:"scopedProjectCount"`
	ScopedSessionCount      int                        `json:"scopedSessionCount"`
	HistorySessionCount     int                        `json:"historySessionCount"`
	SkippedBusySessionCount int                        `json:"skippedBusySessionCount"`
	NonSyncableSessionCount int                        `json:"nonSyncableSessionCount"`
	ItemRowCount            int64                      `json:"itemRowCount"`
	TurnRowCount            int64                      `json:"turnRowCount"`
	ObsoleteItemRowCount    int64                      `json:"obsoleteItemRowCount"`
	ObsoleteTurnRowCount    int64                      `json:"obsoleteTurnRowCount"`
	Storage                 HistoryCleanupStorageStats `json:"storage"`
}

type HistoryCleanupResult struct {
	HistoryCleanupStats
	ClearedSessionIDs       []string `json:"clearedSessionIds"`
	HistoryFileFailureCount int      `json:"historyFileFailureCount"`
}

type historyCleanupCounts struct {
	ActiveItems   int64
	ObsoleteItems int64
	ActiveTurns   int64
	ObsoleteTurns int64
}

type historyCleanupPlan struct {
	stats            HistoryCleanupStats
	scopedSessionIDs []string
	clearSessionIDs  []string
	resetSessionIDs  []string
}

type historyCleanupCountRow struct {
	WebSessionID string `gorm:"column:web_session_id"`
	ActiveCount  int64  `gorm:"column:active_count"`
	DeletedCount int64  `gorm:"column:deleted_count"`
}

func (m *Manager) PreviewHistoryCleanup(ctx context.Context, params HistoryCleanupParams) (HistoryCleanupStats, error) {
	plan, err := m.buildHistoryCleanupPlan(ctx, params, time.Now())
	if err != nil {
		return HistoryCleanupStats{}, err
	}
	return plan.stats, nil
}

func (m *Manager) RunHistoryCleanup(ctx context.Context, params HistoryCleanupParams) (HistoryCleanupResult, error) {
	m.historyCleanupMu.Lock()
	defer m.historyCleanupMu.Unlock()

	plan, err := m.buildHistoryCleanupPlan(ctx, params, time.Now())
	if err != nil {
		return HistoryCleanupResult{}, err
	}
	db := model.GetDB()
	if db == nil {
		return HistoryCleanupResult{}, model.ErrDBNotInitialized
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, ids := range chunkHistoryCleanupIDs(plan.scopedSessionIDs) {
			if err := tx.Unscoped().
				Where("web_session_id IN ? AND deleted_at IS NOT NULL", ids).
				Delete(&tables.WebSessionTurnTable{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().
				Where("web_session_id IN ? AND deleted_at IS NOT NULL", ids).
				Delete(&tables.WebSessionItemTable{}).Error; err != nil {
				return err
			}
		}
		for _, ids := range chunkHistoryCleanupIDs(plan.clearSessionIDs) {
			if err := tx.Unscoped().Where("web_session_id IN ?", ids).
				Delete(&tables.WebSessionTurnTable{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("web_session_id IN ?", ids).
				Delete(&tables.WebSessionItemTable{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("web_session_id IN ?", ids).
				Delete(&tables.WebSessionSubAgentTable{}).Error; err != nil {
				return err
			}
		}
		for _, ids := range chunkHistoryCleanupIDs(plan.resetSessionIDs) {
			if err := tx.Model(&tables.WebSessionTable{}).
				Where("id IN ?", ids).
				Updates(withSnapshotRevisionIncrement(map[string]any{
					"turn_count":     0,
					"item_count":     0,
					"last_event_seq": 0,
					"has_unread":     false,
					"sync_state":     SyncStateMissing,
					"sync_error":     nil,
					"last_sync_mode": "",
					"last_synced_at": nil,
				})).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return HistoryCleanupResult{}, err
	}

	fileFailureCount := 0
	for _, sessionID := range plan.clearSessionIDs {
		m.clearSessionEventState(sessionID)
		if err := m.store.deleteSessionHistory(sessionID); err != nil {
			fileFailureCount++
			m.logger.Warn("failed to delete web session history file",
				zap.String("sessionId", sessionID),
				zap.Error(err))
		}
	}
	model_base.FlushWAL(db)
	plan.stats.Storage = loadHistoryCleanupStorageStats(db)

	return HistoryCleanupResult{
		HistoryCleanupStats:     plan.stats,
		ClearedSessionIDs:       append([]string(nil), plan.resetSessionIDs...),
		HistoryFileFailureCount: fileFailureCount,
	}, nil
}

func (m *Manager) buildHistoryCleanupPlan(ctx context.Context, params HistoryCleanupParams, now time.Time) (historyCleanupPlan, error) {
	normalized, err := normalizeHistoryCleanupParams(params)
	if err != nil {
		return historyCleanupPlan{}, err
	}
	db := model.GetDB()
	if db == nil {
		return historyCleanupPlan{}, model.ErrDBNotInitialized
	}
	if normalized.Scope == HistoryCleanupScopeProjects {
		var count int64
		if err := db.WithContext(ctx).Model(&tables.ProjectTable{}).
			Where("id IN ?", normalized.ProjectIDs).Count(&count).Error; err != nil {
			return historyCleanupPlan{}, err
		}
		if count != int64(len(normalized.ProjectIDs)) {
			return historyCleanupPlan{}, ErrInvalidHistoryCleanup
		}
	}

	query := db.WithContext(ctx).Unscoped().Model(&tables.WebSessionTable{})
	if normalized.Scope == HistoryCleanupScopeProjects {
		query = query.Where("project_id IN ?", normalized.ProjectIDs)
	}
	var sessions []tables.WebSessionTable
	if err := query.Find(&sessions).Error; err != nil {
		return historyCleanupPlan{}, err
	}

	sessionIDs := make([]string, 0, len(sessions))
	projectSet := make(map[string]struct{})
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
		if projectID := strings.TrimSpace(session.ProjectID); projectID != "" {
			projectSet[projectID] = struct{}{}
		}
	}
	counts, err := loadHistoryCleanupCounts(ctx, db, sessionIDs)
	if err != nil {
		return historyCleanupPlan{}, err
	}
	busyIDs, err := m.historyCleanupBusySessionIDs(ctx, db, sessions)
	if err != nil {
		return historyCleanupPlan{}, err
	}
	historyFiles := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		historyFiles[session.ID] = m.store.hasSessionHistory(session.ID)
	}

	byProject := make(map[string][]tables.WebSessionTable)
	for _, session := range sessions {
		if session.DeletedAt.Valid || !historyCleanupSessionHasCurrentData(session, counts[session.ID], historyFiles[session.ID]) {
			continue
		}
		byProject[session.ProjectID] = append(byProject[session.ProjectID], session)
	}
	retained := make(map[string]struct{})
	for _, projectSessions := range byProject {
		sort.Slice(projectSessions, func(i, j int) bool {
			left := historyCleanupActivityAt(projectSessions[i])
			right := historyCleanupActivityAt(projectSessions[j])
			if left.Equal(right) {
				return projectSessions[i].ID > projectSessions[j].ID
			}
			return left.After(right)
		})
		limit := normalized.RetainPerProject
		if limit > len(projectSessions) {
			limit = len(projectSessions)
		}
		for index := 0; index < limit; index++ {
			retained[projectSessions[index].ID] = struct{}{}
		}
	}

	cutoff := now.Add(-time.Duration(normalized.OlderThanDays) * 24 * time.Hour)
	clearIDs := make([]string, 0)
	resetIDs := make([]string, 0)
	stats := HistoryCleanupStats{
		ScopedProjectCount: len(projectSet),
		ScopedSessionCount: len(sessions),
		Storage:            loadHistoryCleanupStorageStats(db),
	}
	for _, session := range sessions {
		rowCounts := counts[session.ID]
		stats.ObsoleteItemRowCount += rowCounts.ObsoleteItems
		stats.ObsoleteTurnRowCount += rowCounts.ObsoleteTurns
		if session.DeletedAt.Valid {
			if rowCounts.ActiveItems+rowCounts.ObsoleteItems+rowCounts.ActiveTurns+rowCounts.ObsoleteTurns > 0 || historyFiles[session.ID] {
				clearIDs = append(clearIDs, session.ID)
				stats.ItemRowCount += rowCounts.ActiveItems
				stats.TurnRowCount += rowCounts.ActiveTurns
			}
			continue
		}
		if !historyCleanupSessionHasCurrentData(session, rowCounts, historyFiles[session.ID]) {
			continue
		}
		if _, ok := retained[session.ID]; ok {
			continue
		}
		if normalized.OlderThanDays > 0 && !historyCleanupActivityAt(session).Before(cutoff) {
			continue
		}
		if _, ok := busyIDs[session.ID]; ok {
			stats.SkippedBusySessionCount++
			continue
		}
		clearIDs = append(clearIDs, session.ID)
		resetIDs = append(resetIDs, session.ID)
		stats.HistorySessionCount++
		stats.ItemRowCount += rowCounts.ActiveItems
		stats.TurnRowCount += rowCounts.ActiveTurns
		if !historyCleanupSessionSyncable(session) {
			stats.NonSyncableSessionCount++
		}
	}
	stats.ItemRowCount += stats.ObsoleteItemRowCount
	stats.TurnRowCount += stats.ObsoleteTurnRowCount

	return historyCleanupPlan{
		stats:            stats,
		scopedSessionIDs: sessionIDs,
		clearSessionIDs:  clearIDs,
		resetSessionIDs:  resetIDs,
	}, nil
}

func normalizeHistoryCleanupParams(params HistoryCleanupParams) (HistoryCleanupParams, error) {
	if params.OlderThanDays < 0 || params.OlderThanDays > 36500 || params.RetainPerProject < 0 || params.RetainPerProject > 10000 {
		return HistoryCleanupParams{}, ErrInvalidHistoryCleanup
	}
	params.Scope = HistoryCleanupScope(strings.ToLower(strings.TrimSpace(string(params.Scope))))
	if params.Scope != HistoryCleanupScopeAll && params.Scope != HistoryCleanupScopeProjects {
		return HistoryCleanupParams{}, ErrInvalidHistoryCleanup
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
		return HistoryCleanupParams{}, ErrInvalidHistoryCleanup
	}
	if params.Scope == HistoryCleanupScopeAll {
		projectIDs = nil
	}
	params.ProjectIDs = projectIDs
	return params, nil
}

func loadHistoryCleanupCounts(ctx context.Context, db *gorm.DB, sessionIDs []string) (map[string]historyCleanupCounts, error) {
	counts := make(map[string]historyCleanupCounts, len(sessionIDs))
	for _, ids := range chunkHistoryCleanupIDs(sessionIDs) {
		var itemRows []historyCleanupCountRow
		if err := db.WithContext(ctx).Unscoped().Model(&tables.WebSessionItemTable{}).
			Select("web_session_id, SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END) AS active_count, SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END) AS deleted_count").
			Where("web_session_id IN ?", ids).Group("web_session_id").Scan(&itemRows).Error; err != nil {
			return nil, err
		}
		for _, row := range itemRows {
			value := counts[row.WebSessionID]
			value.ActiveItems += row.ActiveCount
			value.ObsoleteItems += row.DeletedCount
			counts[row.WebSessionID] = value
		}
		var turnRows []historyCleanupCountRow
		if err := db.WithContext(ctx).Unscoped().Model(&tables.WebSessionTurnTable{}).
			Select("web_session_id, SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END) AS active_count, SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END) AS deleted_count").
			Where("web_session_id IN ?", ids).Group("web_session_id").Scan(&turnRows).Error; err != nil {
			return nil, err
		}
		for _, row := range turnRows {
			value := counts[row.WebSessionID]
			value.ActiveTurns += row.ActiveCount
			value.ObsoleteTurns += row.DeletedCount
			counts[row.WebSessionID] = value
		}
	}
	return counts, nil
}

func (m *Manager) historyCleanupBusySessionIDs(ctx context.Context, db *gorm.DB, sessions []tables.WebSessionTable) (map[string]struct{}, error) {
	busy := make(map[string]struct{})
	m.mu.RLock()
	for sessionID := range m.runs {
		busy[sessionID] = struct{}{}
	}
	for sessionID, inputs := range m.pendingInputs {
		if len(inputs) > 0 {
			busy[sessionID] = struct{}{}
		}
	}
	m.mu.RUnlock()

	sessionIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
		switch effectiveStatus(session, effectiveAssistantState(session)) {
		case StatusRunning, StatusWaitingApproval, StatusAborting:
			busy[session.ID] = struct{}{}
		}
	}
	for _, ids := range chunkHistoryCleanupIDs(sessionIDs) {
		var scheduledIDs []string
		if err := db.WithContext(ctx).Model(&tables.WebSessionScheduledInputTable{}).
			Distinct("web_session_id").
			Where("web_session_id IN ? AND status IN ?", ids, activeScheduledInputStatuses()).
			Pluck("web_session_id", &scheduledIDs).Error; err != nil {
			return nil, err
		}
		for _, sessionID := range scheduledIDs {
			busy[sessionID] = struct{}{}
		}
	}
	return busy, nil
}

func historyCleanupSessionHasCurrentData(session tables.WebSessionTable, counts historyCleanupCounts, hasHistoryFile bool) bool {
	return hasHistoryFile || session.ItemCount > 0 || session.TurnCount > 0 || counts.ActiveItems > 0 || counts.ActiveTurns > 0
}

func historyCleanupActivityAt(session tables.WebSessionTable) time.Time {
	if !session.ActivityAt.IsZero() {
		return session.ActivityAt
	}
	if session.LastMessageAt != nil && !session.LastMessageAt.IsZero() {
		return *session.LastMessageAt
	}
	if !session.UpdatedAt.IsZero() {
		return session.UpdatedAt
	}
	return session.CreatedAt
}

func historyCleanupSessionSyncable(session tables.WebSessionTable) bool {
	return (session.NativeSessionID != nil && strings.TrimSpace(*session.NativeSessionID) != "") ||
		(session.ThreadPath != nil && strings.TrimSpace(*session.ThreadPath) != "")
}

func loadHistoryCleanupStorageStats(db *gorm.DB) HistoryCleanupStorageStats {
	stats := HistoryCleanupStorageStats{}
	_ = db.Raw("PRAGMA page_size").Scan(&stats.PageSizeBytes).Error
	_ = db.Raw("PRAGMA page_count").Scan(&stats.PageCount).Error
	_ = db.Raw("PRAGMA freelist_count").Scan(&stats.FreePageCount).Error
	stats.ReusableBytes = stats.PageSizeBytes * stats.FreePageCount
	return stats
}

func chunkHistoryCleanupIDs(ids []string) [][]string {
	if len(ids) == 0 {
		return nil
	}
	chunks := make([][]string, 0, (len(ids)+historyCleanupChunkSize-1)/historyCleanupChunkSize)
	for start := 0; start < len(ids); start += historyCleanupChunkSize {
		end := start + historyCleanupChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[start:end])
	}
	return chunks
}
