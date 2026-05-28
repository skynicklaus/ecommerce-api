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
	CreateProductTx(
		context.Context,
		CreateProductTxParams,
	) (CreateProductTxResults, error)
	UpdateProductTx(
		context.Context,
		UpdateProductTxParams,
	) (CreateProductTxResults, error)
	CreateWarehouseTx(
		context.Context,
		CreateWarehouseTxParams,
	) (WarehouseTxResult, error)
	UpdateWarehouseTx(
		context.Context,
		UpdateWarehouseTxParams,
	) (WarehouseTxResult, error)
	AddCartItemTx(
		context.Context,
		AddCartItemTxParams,
	) (AddCartItemTxResult, error)
	UpdateCartItemQuantityTx(
		context.Context,
		UpdateCartItemQuantityTxParams,
	) (UpdateCartItemQuantityTxResult, error)
	SetCartItemSelectedTx(
		context.Context,
		SetCartItemSelectedTxParams,
	) (SetCartItemSelectedTxResult, error)
	RemoveCartItemTx(context.Context, RemoveCartItemTxParams) error
	SetCartShopGroupSelectedTx(
		context.Context,
		SetCartShopGroupSelectedTxParams,
	) (SetCartShopGroupSelectedTxResult, error)
}

type SQLStore struct {
	*Queries

	connPool *pgxpool.Pool
}

func NewStore(connPool *pgxpool.Pool) Store {
	return &SQLStore{
		Queries:  New(newRLSDBTX(connPool)),
		connPool: connPool,
	}
}
