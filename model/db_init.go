package model

import (
	"context"
	"database/sql"
	"errors"

	"code-kanban/utils/model_base"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db             *gorm.DB
	sqlDB          *sql.DB
	defaultQueries *Queries

	// ErrSQLCNotReady indicates sqlc helpers have not been initialized.
	ErrSQLCNotReady = errors.New("model: sql queries are not initialized")
)

func InitWithDSN(dsn string, logLevel int, autoMigrate bool) error {
	var err error
	db, err = model_base.DBInit(dsn, logger.LogLevel(logLevel))
	if err != nil {
		return err
	}

	if err := initQueries(); err != nil {
		return err
	}

	DBMigrate(autoMigrate)
	return nil
}

func DBClose() {
	if db != nil {
		model_base.DBClose(db)
	}
	sqlDB = nil
	defaultQueries = nil
}

func GetDB() *gorm.DB {
	return db
}

func initQueries() error {
	if db == nil {
		return ErrSQLCNotReady
	}

	sqlConn, err := db.DB()
	if err != nil {
		return err
	}

	sqlDB = sqlConn
	defaultQueries = New(sqlConn)
	return nil
}

func ensureQueriesReady() error {
	if sqlDB == nil || defaultQueries == nil {
		return ErrSQLCNotReady
	}
	return nil
}

func Transaction(ctx context.Context, fn func(q *Queries) error) error {
	if fn == nil {
		return nil
	}

	if err := ensureQueriesReady(); err != nil {
		return err
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	q := New(tx)
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
	}()

	if err := fn(q); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func GetQ(q *Queries) *Queries {
	if q != nil {
		return q
	}

	if defaultQueries == nil {
		panic("model: sql queries are not initialized")
	}

	return defaultQueries
}

func getDefaultQueries() (*Queries, error) {
	if err := ensureQueriesReady(); err != nil {
		return nil, err
	}
	return defaultQueries, nil
}
