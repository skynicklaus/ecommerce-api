//go:build integration

package db_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomProductAsset(t *testing.T, productID uuid.UUID, isPrimary bool) db.ProductAsset {
	t.Helper()
	key := "products/" + productID.String() + "/" + util.GetRandomString(t, 8) + ".jpg"
	arg := db.CreateProductAssetParams{
		ProductID: productID,
		AssetKey:  key,
		Type:      string(util.ProductAssetImage),
		MimeType:  "image/jpeg",
		SortOrder: 1,
		IsPrimary: isPrimary,
	}

	asset, err := testStore.CreateProductAsset(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, asset)
	require.Equal(t, arg.ProductID, asset.ProductID)
	require.Equal(t, arg.AssetKey, asset.AssetKey)
	require.Equal(t, arg.Type, asset.Type)
	require.Equal(t, arg.MimeType, asset.MimeType)
	require.Equal(t, arg.SortOrder, asset.SortOrder)
	require.Equal(t, arg.IsPrimary, asset.IsPrimary)

	return asset
}

func TestProductAssetQueries(t *testing.T) {
	product1 := createRandomProduct(t)
	product2 := createRandomProduct(t)

	asset1 := createRandomProductAsset(t, product1.ID, true)
	asset2 := createRandomProductAsset(t, product1.ID, false)
	asset3 := createRandomProductAsset(t, product2.ID, true)

	assetsProd1, err := testStore.ListProductAssetsByProductID(t.Context(), product1.ID)
	require.NoError(t, err)
	require.Len(t, assetsProd1, 2)

	batchAssets, err := testStore.ListProductAssetsByProductIDs(t.Context(), []uuid.UUID{product1.ID, product2.ID})
	require.NoError(t, err)
	require.Len(t, batchAssets, 3)

	var found1, found2, found3 bool
	for _, a := range batchAssets {
		switch a.ID {
		case asset1.ID:
			found1 = true
		case asset2.ID:
			found2 = true
		case asset3.ID:
			found3 = true
		}
	}
	require.True(t, found1)
	require.True(t, found2)
	require.True(t, found3)
}
