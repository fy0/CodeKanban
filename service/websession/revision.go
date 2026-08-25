package websession

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const initialSnapshotRevision int64 = 1

func normalizeSnapshotRevision(revision int64) int64 {
	if revision < initialSnapshotRevision {
		return initialSnapshotRevision
	}
	return revision
}

func formatSnapshotRevision(revision int64) string {
	return strconv.FormatInt(normalizeSnapshotRevision(revision), 10)
}

func parseSnapshotRevision(value string) (int64, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return 0, nil
	}
	revision, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil || revision < initialSnapshotRevision {
		return 0, fmt.Errorf("invalid snapshot revision")
	}
	return revision, nil
}

func withSnapshotRevisionIncrement(updates map[string]any) map[string]any {
	if updates == nil {
		updates = make(map[string]any, 1)
	}
	updates["snapshot_revision"] = gorm.Expr("snapshot_revision + 1")
	return updates
}

func (m *Manager) advanceSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	db := model.GetDB()
	if db == nil {
		return 0, model.ErrDBNotInitialized
	}
	var result struct {
		SnapshotRevision int64 `gorm:"column:snapshot_revision"`
	}
	query := db.WithContext(ctx).
		Raw(
			"UPDATE web_sessions SET snapshot_revision = snapshot_revision + 1 WHERE id = ? RETURNING snapshot_revision",
			strings.TrimSpace(sessionID),
		).
		Scan(&result)
	if query.Error != nil {
		return 0, query.Error
	}
	if result.SnapshotRevision < initialSnapshotRevision {
		return 0, gorm.ErrRecordNotFound
	}
	return normalizeSnapshotRevision(result.SnapshotRevision), nil
}

func sessionRevisionLockIndex(sessionID string) uint32 {
	hash := uint32(2166136261)
	for index := 0; index < len(sessionID); index++ {
		hash ^= uint32(sessionID[index])
		hash *= 16777619
	}
	return hash % 64
}

func (m *Manager) currentSessionRevision(ctx context.Context, sessionID string) string {
	db := model.GetReaderDB()
	if db == nil {
		return ""
	}
	var record tables.WebSessionTable
	if err := db.WithContext(ctx).
		Select("id", "snapshot_revision").
		First(&record, "id = ?", strings.TrimSpace(sessionID)).Error; err != nil {
		return ""
	}
	return formatSnapshotRevision(record.SnapshotRevision)
}

func (m *Manager) sessionHistoryCounts(ctx context.Context, sessionID string) (int, int, error) {
	db := model.GetReaderDB()
	if db == nil {
		return 0, 0, model.ErrDBNotInitialized
	}
	normalizedSessionID := strings.TrimSpace(sessionID)
	var counts struct {
		ItemCount int64 `gorm:"column:item_count"`
		TurnCount int64 `gorm:"column:turn_count"`
	}
	if err := db.WithContext(ctx).Raw(
		`SELECT
			(SELECT COUNT(*) FROM web_session_items WHERE web_session_id = ?) AS item_count,
			(SELECT COUNT(*) FROM web_session_turns WHERE web_session_id = ?) AS turn_count`,
		normalizedSessionID,
		normalizedSessionID,
	).Scan(&counts).Error; err != nil {
		return 0, 0, err
	}
	return int(counts.ItemCount), int(counts.TurnCount), nil
}

func (m *Manager) repairSessionHistoryCounts(
	ctx context.Context,
	record tables.WebSessionTable,
	itemCount int,
	turnCount int,
) {
	db := model.GetDB()
	if db == nil {
		return
	}
	result := db.WithContext(ctx).Exec(
		`UPDATE web_sessions
		 SET item_count = ?, turn_count = ?
		 WHERE id = ? AND item_count = ? AND turn_count = ?`,
		itemCount,
		turnCount,
		record.ID,
		record.ItemCount,
		record.TurnCount,
	)
	if result.Error != nil && m.logger != nil {
		m.logger.Debug("failed to repair web session history counts",
			zap.String("sessionId", record.ID),
			zap.Error(result.Error),
		)
	}
}
