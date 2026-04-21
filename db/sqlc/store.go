package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Querier

	CustomerRegistrationTx(
		context.Context,
		CustomerRegistrationTxParams,
	) (CustomerRegistrationTxResults, error)
	PlatformUserRegistrationTx(
		context.Context,
		PlatformUserRegistrationTxParams,
	) (PlatformUserRegistrationTxResults, error)
	UserRegistrationTx(
		context.Context,
		UserRegistrationTxParams,
	) (UserRegistrationTxResults, error)
	CreateOrganizationTx(
		context.Context,
		CreateOrganizationTxRequest,
	) (CreateOrganizationTxResponse, error)
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
