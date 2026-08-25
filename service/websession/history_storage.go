package websession

import (
	"context"
	"net/url"
	"os"
	"path/filepath"

	"code-kanban/model"
	"code-kanban/model/tables"

	"github.com/glebarez/sqlite"
	"github.com/shirou/gopsutil/v4/disk"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type historyDatabaseListRow struct {
	Name string `gorm:"column:name"`
	File string `gorm:"column:file"`
}

type historyStorageTableStats struct {
	RowCount      int64 `gorm:"column:row_count"`
	Bytes         int64 `gorm:"column:bytes"`
	ArchivedBytes int64 `gorm:"column:archived_bytes"`
}

// HistoryStorageOverview returns file-level stats without scanning chat payloads.
func (m *Manager) HistoryStorageOverview(ctx context.Context) (HistoryCleanupStorageStats, error) {
	db := model.GetDB()
	if db == nil {
		return HistoryCleanupStorageStats{}, model.ErrDBNotInitialized
	}
	return m.loadHistoryStorageStats(ctx, db, false), nil
}

// HistoryStorageDetails scans normalized chat payloads on an isolated read-only
// connection. The primary SQLite pool intentionally has one connection, so a
// large analytical scan must not occupy it and stall normal application reads.
func (m *Manager) HistoryStorageDetails(ctx context.Context) (HistoryCleanupStorageStats, error) {
	db := model.GetDB()
	if db == nil {
		return HistoryCleanupStorageStats{}, model.ErrDBNotInitialized
	}
	databasePath, err := historyStorageDatabasePath(ctx, db)
	if err != nil {
		return HistoryCleanupStorageStats{}, err
	}
	if databasePath == "" {
		// Shared in-memory databases used by tests cannot be reopened by path.
		return m.loadHistoryStorageStats(ctx, db, true), nil
	}
	reader, closeReader, err := openHistoryStorageReader(databasePath)
	if err != nil {
		return HistoryCleanupStorageStats{}, err
	}
	defer closeReader()
	return m.loadHistoryStorageStats(ctx, reader, true), nil
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

	databasePath, _ := historyStorageDatabasePath(ctx, db)
	if databasePath != "" {
		if info, err := os.Stat(databasePath); err == nil && !info.IsDir() {
			stats.DatabaseBytes = info.Size()
		}
		if info, err := os.Stat(databasePath + "-wal"); err == nil && !info.IsDir() {
			stats.WALBytes = info.Size()
		}
		if usage, err := disk.Usage(filepath.Dir(databasePath)); err == nil && usage != nil {
			stats.FreeDiskBytes = int64(usage.Free)
		}
	}
	if !includePayload {
		return stats
	}

	var itemStats historyStorageTableStats
	if query.Raw(historyStorageTableStatsSQL("web_session_items", historyCleanupItemBytesSQL())).Scan(&itemStats).Error == nil {
		stats.ItemRowCount = itemStats.RowCount
		stats.ItemBytes = itemStats.Bytes
	}
	var turnStats historyStorageTableStats
	if query.Raw(historyStorageTableStatsSQL("web_session_turns", historyCleanupTurnBytesSQL())).Scan(&turnStats).Error == nil {
		stats.TurnRowCount = turnStats.RowCount
		stats.TurnBytes = turnStats.Bytes
	}
	var subAgentStats historyStorageTableStats
	if query.Raw(historyStorageTableStatsSQL("web_session_sub_agents", historyCleanupSubAgentBytesSQL())).Scan(&subAgentStats).Error == nil {
		stats.SubAgentRowCount = subAgentStats.RowCount
		stats.SubAgentBytes = subAgentStats.Bytes
	}

	var archivedSessionCount int64
	_ = query.Unscoped().Model(&tables.WebSessionTable{}).
		Where("deleted_at IS NULL AND archived_at IS NOT NULL").Count(&archivedSessionCount).Error
	stats.ArchivedSessionCount = archivedSessionCount

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
	stats.ArchivedCacheBytes += itemStats.ArchivedBytes + turnStats.ArchivedBytes + subAgentStats.ArchivedBytes
	return stats
}

func historyStorageDatabasePath(ctx context.Context, db *gorm.DB) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var databases []historyDatabaseListRow
	if err := db.WithContext(ctx).Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
		return "", err
	}
	for _, database := range databases {
		if database.Name != "main" || database.File == "" {
			continue
		}
		databasePath := database.File
		if !filepath.IsAbs(databasePath) {
			absolute, err := filepath.Abs(databasePath)
			if err != nil {
				return "", err
			}
			databasePath = absolute
		}
		return databasePath, nil
	}
	return "", nil
}

func openHistoryStorageReader(databasePath string) (*gorm.DB, func(), error) {
	uriPath := filepath.ToSlash(databasePath)
	if volume := filepath.VolumeName(databasePath); volume != "" && uriPath[0] != '/' {
		uriPath = "/" + uriPath
	}
	dsn := (&url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro"}).String()
	reader, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := reader.DB()
	if err != nil {
		return nil, nil, err
	}
	closeReader := func() { _ = sqlDB.Close() }
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if result := reader.Exec("PRAGMA query_only = ON"); result.Error != nil {
		closeReader()
		return nil, nil, result.Error
	}
	return reader, closeReader, nil
}

func historyStorageTableStatsSQL(tableName, bytesSQL string) string {
	return "SELECT COUNT(*) AS row_count, COALESCE(SUM(" + bytesSQL + "), 0) AS bytes, " +
		"COALESCE(SUM(CASE WHEN web_sessions.archived_at IS NOT NULL THEN " + bytesSQL + " ELSE 0 END), 0) AS archived_bytes " +
		"FROM " + tableName + " LEFT JOIN web_sessions ON web_sessions.id = " + tableName + ".web_session_id"
}
