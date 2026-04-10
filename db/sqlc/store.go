package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Querier

	RegistrationTx(ctx context.Context, arg RegistrationTxParams) (RegistrationTxResults, error)
}

type SQLStore struct {
	*Queries

	connPool *pgxpool.Pool
}

func NewStore(connPool *pgxpool.Pool) Store {
	return &SQLStore{
		Queries:  New(connPool),
		connPool: connPool,
	}
}
