//go:build integration

package db_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomInventoryWithOrg(t *testing.T, organization db.Organization) db.Inventory {
	t.Helper()
	warehouse := createRandomWarehouseWithOrg(t, organization)
	productVariant := createRandomProductVariantWithOrg(t, organization)

	n := util.CoinFlip(t)
	var lowStockThreashold *int32
	if n == 1 {
		lowStockThreashold = util.GetRandomNumberPtr(t, 5)
	}

	arg := db.CreateInventoryParams{
		ProductVariantID:  productVariant.ID,
		WarehouseID:       warehouse.ID,
		QuantityOnHand:    util.GetRandomNumber(t, 200),
		LowStockThreshold: lowStockThreashold,
	}

	inventory, err := testStore.CreateInventory(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, inventory)

	require.Equal(t, arg.ProductVariantID, inventory.ProductVariantID)
	require.Equal(t, arg.WarehouseID, inventory.WarehouseID)
	require.Equal(t, arg.QuantityOnHand, inventory.QuantityOnHand)

	if n != 1 {
		require.Empty(t, inventory.LowStockThreshold)
	} else {
		require.Equal(t, *arg.LowStockThreshold, *inventory.LowStockThreshold)
	}

	return inventory
}

func createRandomInventory(t *testing.T) db.Inventory {
	t.Helper()
	organization := createRandomOrganization(t)
	return createRandomInventoryWithOrg(t, organization)
}

func TestCreateInventory(t *testing.T) {
	createRandomInventory(t)
}

func TestGetWarehouseVariantInventory(t *testing.T) {
	inventory1 := createRandomInventory(t)

	inventory2, err := testStore.GetWarehouseVariantInventory(
		t.Context(),
		db.GetWarehouseVariantInventoryParams{
			ProductVariantID: inventory1.ProductVariantID,
			WarehouseID:      inventory1.WarehouseID,
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, inventory2)

	require.Equal(t, inventory1.ProductVariantID, inventory2.ProductVariantID)
	require.Equal(t, inventory1.WarehouseID, inventory2.WarehouseID)
	require.Equal(t, inventory1.QuantityOnHand, inventory2.QuantityOnHand)
	require.Equal(t, inventory1.QuantityReserved, inventory2.QuantityReserved)
	require.Equal(t, inventory1.QuantityAvailable, inventory2.QuantityAvailable)
	require.Equal(t, inventory1.IsActive, inventory2.IsActive)
}
