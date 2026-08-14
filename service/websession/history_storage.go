package websession

import (
	"context"
	"os"
	"path/filepath"

	"code-kanban/model"
	"code-kanban/model/tables"

	"github.com/shirou/gopsutil/v4/disk"
	"gorm.io/gorm"
)

type historyDatabaseListRow struct {
	Name string `gorm:"column:name"`
	File string `gorm:"column:file"`
}

type historyStorageTableStats struct {
	RowCount int64 `gorm:"column:row_count"`
	Bytes    int64 `gorm:"column:bytes"`
}

// HistoryStorageOverview returns a read-only view of the SQLite file and the
// normalized conversation cache. The byte totals are logical payload sizes,
// so they explain where space is going without pretending to be page-level
// allocation accounting.
func (m *Manager) HistoryStorageOverview(ctx context.Context) (HistoryCleanupStorageStats, error) {
	db := model.GetDB()
	if db == nil {
		return HistoryCleanupStorageStats{}, model.ErrDBNotInitialized
	}
	return m.loadHistoryStorageStats(ctx, db, true), nil
}

func (m *Manager) loadHistoryCleanupStorageStats(ctx context.Context, db *gorm.DB) HistoryCleanupStorageStats {
	return m.loadHistoryStorageStats(ctx, db, false)
}

func (m *Manager) loadHistoryStorageStats(ctx context.Context, db *gorm.DB, includePayload bool) HistoryCleanupStorageStats {
	stats := HistoryCleanupStorageStats{}
	if ctx == nil {
		ctx = context.Background()
	}
	query := db.WithContext(ctx)
	_ = query.Raw("PRAGMA page_size").Scan(&stats.PageSizeBytes).Error
	_ = query.Raw("PRAGMA page_count").Scan(&stats.PageCount).Error
	_ = query.Raw("PRAGMA freelist_count").Scan(&stats.FreePageCount).Error
	stats.ReusableBytes = stats.PageSizeBytes * stats.FreePageCount

	var databases []historyDatabaseListRow
	if query.Raw("PRAGMA database_list").Scan(&databases).Error == nil {
		for _, database := range databases {
			if database.Name != "main" || database.File == "" {
				continue
			}
			databasePath := database.File
			if !filepath.IsAbs(databasePath) {
				if absolute, err := filepath.Abs(databasePath); err == nil {
					databasePath = absolute
				}
			}
			if info, err := os.Stat(databasePath); err == nil && !info.IsDir() {
				stats.DatabaseBytes = info.Size()
			}
			if info, err := os.Stat(databasePath + "-wal"); err == nil && !info.IsDir() {
				stats.WALBytes = info.Size()
			}
			if usage, err := disk.Usage(filepath.Dir(databasePath)); err == nil && usage != nil {
				stats.FreeDiskBytes = int64(usage.Free)
			}
			break
		}
	}
	if !includePayload {
		return stats
	}

	var itemStats historyStorageTableStats
	if query.Raw("SELECT COUNT(*) AS row_count, COALESCE(SUM("+historyCleanupItemBytesSQL()+"), 0) AS bytes FROM web_session_items").Scan(&itemStats).Error == nil {
		stats.ItemRowCount = itemStats.RowCount
		stats.ItemBytes = itemStats.Bytes
	}
	var turnStats historyStorageTableStats
	if query.Raw("SELECT COUNT(*) AS row_count, COALESCE(SUM("+historyCleanupTurnBytesSQL()+"), 0) AS bytes FROM web_session_turns").Scan(&turnStats).Error == nil {
		stats.TurnRowCount = turnStats.RowCount
		stats.TurnBytes = turnStats.Bytes
	}
	var subAgentStats historyStorageTableStats
	if query.Raw("SELECT COUNT(*) AS row_count, COALESCE(SUM("+historyCleanupSubAgentBytesSQL()+"), 0) AS bytes FROM web_session_sub_agents").Scan(&subAgentStats).Error == nil {
		stats.SubAgentRowCount = subAgentStats.RowCount
		stats.SubAgentBytes = subAgentStats.Bytes
	}

	var archivedSessionCount int64
	_ = query.Unscoped().Model(&tables.WebSessionTable{}).
		Where("deleted_at IS NULL AND archived_at IS NOT NULL").Count(&archivedSessionCount).Error
	stats.ArchivedSessionCount = archivedSessionCount
	var archivedItemBytes, archivedTurnBytes, archivedSubAgentBytes int64
	_ = query.Raw("SELECT COALESCE(SUM(" + historyCleanupItemBytesSQL() + "), 0) FROM web_session_items i JOIN web_sessions s ON s.id = i.web_session_id WHERE s.archived_at IS NOT NULL").Scan(&archivedItemBytes).Error
	_ = query.Raw("SELECT COALESCE(SUM(" + historyCleanupTurnBytesSQL() + "), 0) FROM web_session_turns t JOIN web_sessions s ON s.id = t.web_session_id WHERE s.archived_at IS NOT NULL").Scan(&archivedTurnBytes).Error
	_ = query.Raw("SELECT COALESCE(SUM(" + historyCleanupSubAgentBytesSQL() + "), 0) FROM web_session_sub_agents a JOIN web_sessions s ON s.id = a.web_session_id WHERE s.archived_at IS NOT NULL").Scan(&archivedSubAgentBytes).Error

	archivedIDs := make(map[string]bool)
	// Avoid depending on a time scanner here; only the NULL/non-NULL state is
	// needed to attribute history files to archived sessions.
	var sessionRows []struct {
		ID         string `gorm:"column:id"`
		ArchivedAt *int   `gorm:"column:archived_at"`
	}
	if query.Unscoped().Model(&tables.WebSessionTable{}).
		Select("id, CASE WHEN archived_at IS NULL THEN NULL ELSE 1 END AS archived_at").
		Find(&sessionRows).Error == nil {
		for _, session := range sessionRows {
			archivedIDs[session.ID] = session.ArchivedAt != nil
		}
	}

	if m.store != nil {
		if entries, err := os.ReadDir(m.store.rootDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() || entry.Name() == "_attachments" {
					continue
				}
				path := filepath.Join(m.store.rootDir, entry.Name(), "history.jsonl")
				info, statErr := os.Stat(path)
				if statErr != nil || info.IsDir() {
					continue
				}
				stats.HistoryFileBytes += info.Size()
				if archivedIDs[entry.Name()] {
					stats.ArchivedCacheBytes += info.Size()
				}
			}
		}
	}
	stats.HistoryBytes = stats.ItemBytes + stats.TurnBytes + stats.SubAgentBytes + stats.HistoryFileBytes
	stats.ArchivedCacheBytes += archivedItemBytes + archivedTurnBytes + archivedSubAgentBytes
	return stats
}
