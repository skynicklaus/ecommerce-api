package db

import (
	"context"

	"github.com/google/uuid"
)

type CreateProductTxParams struct {
	OrganizationID uuid.UUID
	CategoryID     uuid.UUID
	Name           string
	Slug           string
	Description    string
	Assets         []ProductAssetParams
	Variants       []ProductVariantParams
}

type ProductVariantParams struct {
	AssetsParam ProductAssetParams
}

type ProductAssetParams struct{}

type CreateProductTxResults struct{}

func (store *SQLStore) CreateProductTx(
	ctx context.Context,
	arg CreateProductTxParams,
) (CreateProductTxResults, error) {
	var results CreateProductTxResults

	err := store.execTx(ctx, func(q *Queries) error {
		return nil
	})

	return results, err
}
