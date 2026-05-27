package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rlsDBTX struct {
	base DBTX
	pool *pgxpool.Pool
}

func newRLSDBTX(pool *pgxpool.Pool) DBTX {
	return &rlsDBTX{
		base: pool,
		pool: pool,
	}
}

func (dbtx *rlsDBTX) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (pgconn.CommandTag, error) {
	if _, ok := RLSContextFromContext(ctx); !ok {
		return dbtx.base.Exec(ctx, sql, args...)
	}

	tx, err := dbtx.beginRLS(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}

	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return tag, rollbackWithError(ctx, tx, err)
	}
	if err = tx.Commit(ctx); err != nil {
		return pgconn.CommandTag{}, err
	}

	return tag, nil
}

func (dbtx *rlsDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if _, ok := RLSContextFromContext(ctx); !ok {
		return dbtx.base.Query(ctx, sql, args...)
	}

	tx, err := dbtx.beginRLS(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, rollbackWithError(ctx, tx, err)
	}

	return &rlsRows{
		Rows:      rows,
		ctx:       ctx,
		tx:        tx,
		closed:    false,
		finishErr: nil,
	}, nil
}

func (dbtx *rlsDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if _, ok := RLSContextFromContext(ctx); !ok {
		return dbtx.base.QueryRow(ctx, sql, args...)
	}

	tx, err := dbtx.beginRLS(ctx)
	if err != nil {
		return errorRow{err: err}
	}

	return &rlsRow{
		row: tx.QueryRow(ctx, sql, args...),
		ctx: ctx,
		tx:  tx,
	}
}

func (dbtx *rlsDBTX) beginRLS(ctx context.Context) (pgx.Tx, error) {
	tx, err := dbtx.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}

	if err = applyRLS(ctx, tx); err != nil {
		return nil, rollbackWithError(ctx, tx, err)
	}

	return tx, nil
}

type rlsRows struct {
	pgx.Rows

	ctx       context.Context
	tx        pgx.Tx
	closed    bool
	finishErr error
}

func (r *rlsRows) Next() bool {
	if r.Rows.Next() {
		return true
	}
	r.Close()
	return false
}

func (r *rlsRows) Close() {
	if r.closed {
		return
	}
	r.closed = true

	r.Rows.Close()
	if err := r.Rows.Err(); err != nil {
		r.finishErr = rollbackWithError(r.ctx, r.tx, err)
		return
	}
	r.finishErr = r.tx.Commit(r.ctx)
}

func (r *rlsRows) Err() error {
	return errors.Join(r.Rows.Err(), r.finishErr)
}

type rlsRow struct {
	row pgx.Row
	ctx context.Context
	tx  pgx.Tx
}

func (r *rlsRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		if commitErr := r.tx.Commit(r.ctx); commitErr != nil {
			return commitErr
		}
		return err
	}

	return rollbackWithError(r.ctx, r.tx, err)
}

type errorRow struct {
	err error
}

func (r errorRow) Scan(...any) error {
	return r.err
}

func rollbackWithError(ctx context.Context, tx pgx.Tx, err error) error {
	if rbErr := tx.Rollback(ctx); rbErr != nil {
		return errors.Join(err, rbErr)
	}
	return err
}
