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
	readerDB       *gorm.DB
	readerSQLDB    *sql.DB
	defaultQueries *Queries
	readerQueries  *Queries

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
	readerDB, err = model_base.DBInitReadOnly(dsn, logger.LogLevel(logLevel))
	if err != nil {
		model_base.DBClose(db)
		db = nil
		sqlDB = nil
		defaultQueries = nil
		readerQueries = nil
		return err
	}
	if readerDB == nil {
		readerDB = db
		readerSQLDB = sqlDB
	} else if readerSQLDB, err = readerDB.DB(); err != nil {
		model_base.DBClose(db)
		db = nil
		readerDB = nil
		sqlDB = nil
		defaultQueries = nil
		readerQueries = nil
		return err
	}
	if readerSQLDB == sqlDB {
		readerQueries = defaultQueries
	} else {
		readerQueries = New(readerSQLDB)
	}
	return nil
}

func DBClose() {
	if readerDB != nil && readerDB != db {
		model_base.DBClose(readerDB)
	}
	if db != nil {
		model_base.DBClose(db)
	}
	db = nil
	readerDB = nil
	sqlDB = nil
	readerSQLDB = nil
	defaultQueries = nil
	readerQueries = nil
}

func GetDB() *gorm.DB {
	return db
}

func GetReaderDB() *gorm.DB {
	return readerDB
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

func getDefaultQueries() (*Queries, error) {
	if err := ensureQueriesReady(); err != nil {
		return nil, err
	}
	return defaultQueries, nil
}

func getReaderQueries() (*Queries, error) {
	if err := ensureQueriesReady(); err != nil {
		return nil, err
	}
	if readerQueries == nil {
		return nil, ErrSQLCNotReady
	}
	return readerQueries, nil
}
