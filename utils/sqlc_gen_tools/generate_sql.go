package sqlc_gen_tools

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SQLCollectorLogger 用于收集SQL语句的logger
type SQLCollectorLogger struct {
	CollectedSQL []string
}

func (l *SQLCollectorLogger) LogMode(level logger.LogLevel) logger.Interface {
	return l
}

func (l *SQLCollectorLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	// 不处理Info级别的日志
}

func (l *SQLCollectorLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	// 不处理Warn级别的日志
}

func (l *SQLCollectorLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	// 不处理Error级别的日志
}

func (l *SQLCollectorLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, _ := fc()
	l.CollectedSQL = append(l.CollectedSQL, sql)
}

// GenerateSQLForDialect 为指定方言生成SQL语句
func GenerateSQLForDialect(models []any, dialect string) ([]string, error) {
	// 创建sqlmock
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		// 忽略所有查询匹配，允许任何查询
		return nil
	})))
	if err != nil {
		return nil, fmt.Errorf("创建sqlmock失败: %v", err)
	}
	defer sqlDB.Close()

	// 设置期望以允许任何查询
	mock.ExpectQuery(".+").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("1.0.0"))

	// 创建SQL收集器logger
	collectorLogger := &SQLCollectorLogger{
		CollectedSQL: make([]string, 0),
	}

	// 根据方言创建不同的dialector
	var dialector gorm.Dialector
	switch dialect {
	case "sqlite":
		dialector = sqlite.New(sqlite.Config{Conn: sqlDB})
	case "postgres":
		dialector = postgres.New(postgres.Config{Conn: sqlDB})
	case "mysql":
		dialector = mysql.New(mysql.Config{Conn: sqlDB})
	default:
		return nil, fmt.Errorf("不支持的数据库方言: %s", dialect)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:                                   collectorLogger,
		DryRun:                                   true,
		SkipDefaultTransaction:                   true,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, fmt.Errorf("创建数据库连接失败: %v", err)
	}

	// 生成建表SQL
	for _, model := range models {
		err := db.Migrator().CreateTable(model)
		// 空一行，这样每个表之间有间隔
		collectorLogger.Trace(context.Background(), time.Now(), func() (sql string, rowsAffected int64) {
			return "", 0
		}, nil)
		if err != nil {
			log.Printf("警告: 为模型 %T 生成SQL时出错: %v", model, err)
		}
	}

	return collectorLogger.CollectedSQL, nil
}
