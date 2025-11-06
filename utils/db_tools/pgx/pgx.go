package pgx_tools

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionFactoryInfo[Q any] struct {
	Conn              *pgxpool.Pool
	InitFunc          func(dsn string)
	TransactionCreate func(ctx context.Context, fn func(q Q) error) error
	GetQ              func(q Q) Q
}

func TransactionCreateFactory[Q any](NewFunc func(tx any) Q) TransactionFactoryInfo[Q] {
	var _conn *pgxpool.Pool
	var _Q Q

	transaction := func(ctx context.Context, fn func(q Q) error) error {
		tx, err := _conn.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback(ctx)
				panic(r)
			}
			if err != nil {
				tx.Rollback(ctx)
			}
		}()

		q := NewFunc(tx)
		err = fn(q)
		if err != nil {
			return err
		}

		return tx.Commit(ctx)
	}

	sqlcInit := func(dsn string) {
		conn, err := PgxInit(dsn)
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
