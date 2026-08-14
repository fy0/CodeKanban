package model

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

const databaseHealthProbeTimeout = 2 * time.Second

// DatabaseProbe describes one short-lived SQLite probe.
type DatabaseProbe struct {
	OK         bool   `json:"ok"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

// DatabasePoolStats is the connection-pool state exported by database/sql.
type DatabasePoolStats struct {
	MaxOpenConnections int64 `json:"maxOpenConnections"`
	OpenConnections    int64 `json:"openConnections"`
	InUse              int64 `json:"inUse"`
	Idle               int64 `json:"idle"`
	WaitCount          int64 `json:"waitCount"`
	WaitDurationMs     int64 `json:"waitDurationMs"`
	MaxIdleClosed      int64 `json:"maxIdleClosed"`
	MaxLifetimeClosed  int64 `json:"maxLifetimeClosed"`
	MaxIdleTimeClosed  int64 `json:"maxIdleTimeClosed"`
}

// DatabaseHealth is a lightweight, non-destructive SQLite health snapshot.
// The write probe starts and immediately rolls back BEGIN IMMEDIATE, so it
// checks writer availability without changing application data.
type DatabaseHealth struct {
	Healthy       bool              `json:"healthy"`
	DatabaseBytes int64             `json:"databaseBytes"`
	WALBytes      int64             `json:"walBytes"`
	FreeDiskBytes int64             `json:"freeDiskBytes"`
	JournalMode   string            `json:"journalMode"`
	BusyTimeoutMs int64             `json:"busyTimeoutMs"`
	PageSizeBytes int64             `json:"pageSizeBytes"`
	PageCount     int64             `json:"pageCount"`
	FreePageCount int64             `json:"freePageCount"`
	ReusableBytes int64             `json:"reusableBytes"`
	Pool          DatabasePoolStats `json:"pool"`
	ReadProbe     DatabaseProbe     `json:"readProbe"`
	WriteProbe    DatabaseProbe     `json:"writeProbe"`
}

// InspectDatabase runs bounded read and writer probes against the configured
// database. Operational probe failures are returned in the snapshot so the
// caller can still inspect pool and storage state.
func InspectDatabase(ctx context.Context, dsn string) (DatabaseHealth, error) {
	health := DatabaseHealth{}
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil {
		return health, ErrDBNotInitialized
	}
	sqlDB, err := db.DB()
	if err != nil {
		return health, err
	}
	health.Pool = databasePoolStats(sqlDB.Stats())
	health.DatabaseBytes, health.WALBytes = databaseFileSizes(dsn)
	health.FreeDiskBytes = databaseFreeDiskBytes(dsn)

	probeCtx, cancel := context.WithTimeout(ctx, databaseHealthProbeTimeout)
	defer cancel()

	health.JournalMode, _ = queryStringPragma(probeCtx, sqlDB, "journal_mode")
	health.BusyTimeoutMs, _ = queryIntPragma(probeCtx, sqlDB, "busy_timeout")
	health.PageSizeBytes, _ = queryIntPragma(probeCtx, sqlDB, "page_size")
	health.PageCount, _ = queryIntPragma(probeCtx, sqlDB, "page_count")
	health.FreePageCount, _ = queryIntPragma(probeCtx, sqlDB, "freelist_count")
	health.ReusableBytes = health.PageSizeBytes * health.FreePageCount

	health.ReadProbe = probeDatabaseRead(probeCtx, sqlDB)
	health.WriteProbe = probeDatabaseWrite(probeCtx, sqlDB)
	health.Healthy = health.ReadProbe.OK && health.WriteProbe.OK
	return health, nil
}

func databasePoolStats(stats sql.DBStats) DatabasePoolStats {
	return DatabasePoolStats{
		MaxOpenConnections: int64(stats.MaxOpenConnections),
		OpenConnections:    int64(stats.OpenConnections),
		InUse:              int64(stats.InUse),
		Idle:               int64(stats.Idle),
		WaitCount:          stats.WaitCount,
		WaitDurationMs:     stats.WaitDuration.Milliseconds(),
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
		MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
	}
}

func queryStringPragma(ctx context.Context, sqlDB *sql.DB, name string) (string, error) {
	var value string
	err := sqlDB.QueryRowContext(ctx, "PRAGMA "+name).Scan(&value)
	return value, err
}

func queryIntPragma(ctx context.Context, sqlDB *sql.DB, name string) (int64, error) {
	var value int64
	err := sqlDB.QueryRowContext(ctx, "PRAGMA "+name).Scan(&value)
	return value, err
}

func probeDatabaseRead(ctx context.Context, sqlDB *sql.DB) DatabaseProbe {
	started := time.Now()
	var value int
	err := sqlDB.QueryRowContext(ctx, "SELECT 1").Scan(&value)
	probe := DatabaseProbe{
		OK:         err == nil && value == 1,
		DurationMs: time.Since(started).Milliseconds(),
	}
	if err != nil {
		probe.Error = err.Error()
	}
	return probe
}

func probeDatabaseWrite(ctx context.Context, sqlDB *sql.DB) DatabaseProbe {
	started := time.Now()
	probe := DatabaseProbe{}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		probe.DurationMs = time.Since(started).Milliseconds()
		probe.Error = err.Error()
		return probe
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		probe.DurationMs = time.Since(started).Milliseconds()
		probe.Error = err.Error()
		return probe
	}
	if _, err := conn.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		probe.DurationMs = time.Since(started).Milliseconds()
		probe.Error = err.Error()
		return probe
	}
	probe.OK = true
	probe.DurationMs = time.Since(started).Milliseconds()
	return probe
}

func databaseFileSizes(dsn string) (databaseBytes, walBytes int64) {
	databasePath := databasePathFromDSN(dsn)
	if databasePath == "" {
		return 0, 0
	}
	if info, err := os.Stat(databasePath); err == nil {
		databaseBytes = info.Size()
	}
	if info, err := os.Stat(databasePath + "-wal"); err == nil {
		walBytes = info.Size()
	}
	return databaseBytes, walBytes
}

func databaseFreeDiskBytes(dsn string) int64 {
	databasePath := databasePathFromDSN(dsn)
	if databasePath == "" {
		return 0
	}
	usage, err := disk.Usage(filepath.Dir(databasePath))
	if err != nil || usage == nil {
		return 0
	}
	return int64(usage.Free)
}

func databasePathFromDSN(dsn string) string {
	value := strings.TrimSpace(dsn)
	value = strings.TrimPrefix(value, "sqlite://")
	if strings.HasPrefix(value, "file:") {
		value = strings.TrimPrefix(value, "file:")
		if strings.Contains(value, "mode=memory") || value == ":memory:" {
			return ""
		}
	}
	if index := strings.IndexByte(value, '?'); index >= 0 {
		value = value[:index]
	}
	if value == "" || value == ":memory:" {
		return ""
	}
	if !filepath.IsAbs(value) {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return ""
		}
		value = absolute
	}
	return filepath.Clean(value)
}
