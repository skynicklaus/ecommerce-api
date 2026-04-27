package db

import (
	"context"
	"fmt"
)

func (store *SQLStore) execTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := store.connPool.Begin(ctx)
	if err != nil {
		return err
	}

	if err = applyRLS(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	q := New(tx)
	err = fn(q)
	if err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx error: %w, rb error: %w", err, rbErr)
		}

		return err
	}

	return tx.Commit(ctx)
}
