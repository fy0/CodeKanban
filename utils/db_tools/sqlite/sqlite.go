package sqlite_tools

import (
	"context"
	"database/sql"
)

type TransactionFactoryInfo[Q any] struct {
	Conn              *sql.DB
	InitFunc          func(dsn string)
	TransactionCreate func(ctx context.Context, fn func(q Q) error) error
	GetQ              func(q Q) Q
}

func TransactionCreateFactory[Q any](NewFunc func(tx any) Q) TransactionFactoryInfo[Q] {
	var _conn *sql.DB
	var _Q Q

	transaction := func(ctx context.Context, fn func(q Q) error) error {
		tx, err := _conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
				panic(r)
			}
			if err != nil {
				tx.Rollback()
			}
		}()

		q := NewFunc(tx)
		err = fn(q)
		if err != nil {
			return err
		}

		return tx.Commit()
	}

	sqlcInit := func(dsn string) {
		conn, err := SqliteInit(dsn)
		if err != nil {
			panic(err)
		}

		_conn = conn
		_Q = NewFunc(conn)
	}

	getQ := func(q Q) Q {
		var zero Q
		if any(q) == any(zero) {
			return _Q
		}
		return q
	}

	return TransactionFactoryInfo[Q]{
		Conn:              _conn,
		InitFunc:          sqlcInit,
		TransactionCreate: transaction,
		GetQ:              getQ,
	}
}
