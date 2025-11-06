package sqlite_tools

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/glebarez/sqlite" // SQLite driver
)

// SqliteInit 初始化 SQLite 连接，供 sqlc 查询使用。
func SqliteInit(dsn string) (*sql.DB, error) {
	// 检查 DSN 格式，支持文件路径或 sqlite:// 格式
	if !strings.HasSuffix(dsn, ".db") && !strings.HasPrefix(dsn, "sqlite://") && !strings.HasPrefix(dsn, "file:") {
		// 如果不是明确的 SQLite 格式，假设是文件路径
		if !strings.Contains(dsn, ".db") {
			dsn = dsn + ".db"
		}
	}

	// 移除 sqlite:// 前缀（如果存在）
	if after, ok := strings.CutPrefix(dsn, "sqlite://"); ok {
		dsn = after
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 数据库失败: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(1) // SQLite 通常使用单连接
	db.SetMaxIdleConns(1)

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("SQLite 数据库不可用: %w", err)
	}

	// 启用 WAL 模式
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("启用 WAL 模式失败: %w", err)
	}

	return db, nil
}
