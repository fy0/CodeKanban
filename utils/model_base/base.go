package model_base

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code-kanban/utils"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	sqliteMaxOpenConns = 1
	sqliteMaxIdleConns = 1
	sqliteBusyTimeout  = 5000
	sqliteReaderConns  = 4
)

type BaseModel struct {
	ID        uint64         `gorm:"primary_key;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
}

func DBInitReadOnly(dsn string, logLevel logger.LogLevel) (*gorm.DB, error) {
	readOnlyDSN, ok, err := sqliteReadOnlyDSN(dsn)
	if err != nil || !ok {
		return nil, err
	}
	db, err := gorm.Open(sqliteOpen(readOnlyDSN), &gorm.Config{
		TranslateError: true,
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{IgnoreRecordNotFoundError: true, LogLevel: logLevel},
		),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(sqliteReaderConns)
	sqlDB.SetMaxIdleConns(sqliteReaderConns)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(sqliteBusyTimeout)*time.Millisecond)
	defer cancel()
	connections := make([]*sql.Conn, 0, sqliteReaderConns)
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()
	for index := 0; index < sqliteReaderConns; index++ {
		connection, connErr := sqlDB.Conn(ctx)
		if connErr != nil {
			_ = sqlDB.Close()
			return nil, connErr
		}
		connections = append(connections, connection)
		if _, connErr = connection.ExecContext(ctx, "PRAGMA query_only = ON"); connErr != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("设置 SQLite 只读连接失败: %w", connErr)
		}
		if _, connErr = connection.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); connErr != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("设置 SQLite 只读 busy_timeout 失败: %w", connErr)
		}
	}
	return db, nil
}

func sqliteReadOnlyDSN(dsn string) (string, bool, error) {
	value := strings.TrimSpace(strings.TrimPrefix(dsn, "sqlite://"))
	lower := strings.ToLower(value)
	if value == "" || value == ":memory:" || strings.HasPrefix(lower, "file::memory:") || strings.Contains(lower, "mode=memory") {
		return "", false, nil
	}
	base, rawQuery, _ := strings.Cut(value, "?")
	if !strings.HasPrefix(strings.ToLower(base), "file:") {
		base = "file:" + filepath.ToSlash(base)
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", false, fmt.Errorf("解析 SQLite DSN 失败: %w", err)
	}
	query.Set("mode", "ro")
	return base + "?" + query.Encode(), true, nil
}

type StringPKBaseModel struct {
	ID        string         `gorm:"primary_key; not null" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
}

func (m *StringPKBaseModel) Init() {
	id := utils.NewID()
	m.ID = id
	// CreatedAt 和 UpdatedAt 会在数据库层自动维护，这里预先赋值便于在入库前取值
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
}

func (m *StringPKBaseModel) BeforeCreate(tx *gorm.DB) error {
	// 为避免忘记初始化，写入前兜底生成 ID
	if m.ID == "" {
		m.Init()
	}
	return nil
}

func DBInit(dsn string, logLevel logger.LogLevel) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch {
	case strings.HasSuffix(dsn, ".db") || strings.HasPrefix(dsn, "file:") || strings.HasPrefix(dsn, ":memory:"):
		dialector = sqliteOpen(dsn)
	default:
		return nil, fmt.Errorf("无法识别的数据库类型: %s (仅支持 SQLite)", dsn)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		TranslateError: true,
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				IgnoreRecordNotFoundError: true,
				LogLevel:                  logLevel,
			},
		),
	})
	if err != nil {
		return nil, err
	}

	// SQLite 只有一个写者。让 GORM、sqlc 和后台投影共享同一个连接，
	// 避免连接池内多个写事务互相竞争 RESERVED/EXCLUSIVE 锁。
	switch dialector.(type) {
	case *sqliteDialector:
		sqlDB, dbErr := db.DB()
		if dbErr != nil {
			return nil, dbErr
		}
		sqlDB.SetMaxOpenConns(sqliteMaxOpenConns)
		sqlDB.SetMaxIdleConns(sqliteMaxIdleConns)

		// 驱动默认 busy_timeout 通常只有约 1 秒；事件投影和历史同步
		// 的短暂写入高峰不应立即变成 SQLITE_BUSY。
		if result := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); result.Error != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("设置 SQLite busy_timeout 失败: %w", result.Error)
		}
		if result := db.Exec("PRAGMA journal_mode=WAL"); result.Error != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("启用 SQLite WAL 失败: %w", result.Error)
		}
	}

	return db, nil
}

func DBClose(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		panic("关闭数据库失败")
	}
	_ = sqlDB.Close()
}

func FlushWAL(db *gorm.DB) {
	switch db.Dialector.(type) {
	case *sqliteDialector:
		_ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		_ = db.Exec("PRAGMA shrink_memory")
	}
}
