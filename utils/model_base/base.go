package model_base

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go-template/utils"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type BaseModel struct {
	ID        uint64         `gorm:"primary_key;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
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
	case strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://"):
		dialector = postgres.Open(dsn)
	case strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp("):
		dialector = mysql.Open(strings.TrimPrefix(dsn, "mysql://"))
	case strings.HasSuffix(dsn, ".db") || strings.HasPrefix(dsn, "file:") || strings.HasPrefix(dsn, ":memory:"):
		dialector = sqliteOpen(dsn)
	default:
		return nil, fmt.Errorf("无法识别的数据库类型: %s", dsn)
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

	// SQLite 模式下开启 WAL 提升并发性能
	switch dialector.(type) {
	case *sqliteDialector:
		db.Exec("PRAGMA journal_mode=WAL")
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
