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
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	inventory1 := createRandomInventoryWithOrg(t, organization)

	inventory2, err := testStore.GetWarehouseVariantInventory(
		t.Context(),
		db.GetWarehouseVariantInventoryParams{
			OrganizationID:   organization.ID,
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

func TestUpsertInventory(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	warehouse := createRandomWarehouseWithOrg(t, organization)
	productVariant := createRandomProductVariantWithOrg(t, organization)
	threshold := int32(7)

	arg := db.UpsertInventoryParams{
		OrganizationID:    organization.ID,
		ProductVariantID:  productVariant.ID,
		WarehouseID:       warehouse.ID,
		QuantityOnHand:    20,
		LowStockThreshold: &threshold,
		IsActive:          true,
	}
	inventory, err := testStore.UpsertInventory(t.Context(), arg)
	require.NoError(t, err)
	require.Equal(t, arg.ProductVariantID, inventory.ProductVariantID)
	require.Equal(t, arg.WarehouseID, inventory.WarehouseID)
	require.Equal(t, arg.QuantityOnHand, inventory.QuantityOnHand)
	require.Equal(t, threshold, *inventory.LowStockThreshold)
	require.True(t, inventory.IsActive)

	arg.QuantityOnHand = 35
	arg.LowStockThreshold = nil
	arg.IsActive = false
	updated, err := testStore.UpsertInventory(t.Context(), arg)
	require.NoError(t, err)
	require.Equal(t, int32(35), updated.QuantityOnHand)
	require.Nil(t, updated.LowStockThreshold)
	require.False(t, updated.IsActive)
}

func TestUpsertInventoryRejectsCrossOrgReferences(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	otherOrganization := createRandomOrganization(t)
	cleanupOrganization(t, otherOrganization.ID.String())

	warehouse := createRandomWarehouseWithOrg(t, organization)
	otherProductVariant := createRandomProductVariantWithOrg(t, otherOrganization)

	_, err := testStore.UpsertInventory(t.Context(), db.UpsertInventoryParams{
		OrganizationID:   organization.ID,
		ProductVariantID: otherProductVariant.ID,
		WarehouseID:      warehouse.ID,
		QuantityOnHand:   20,
		IsActive:         true,
	})
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestInventoryScopedQueries(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	warehouse := createRandomWarehouseWithOrg(t, organization)
	productVariant := createRandomProductVariantWithOrg(t, organization)
	_, err := testStore.UpsertInventory(t.Context(), db.UpsertInventoryParams{
		OrganizationID:   organization.ID,
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		QuantityOnHand:   12,
		IsActive:         true,
	})
	require.NoError(t, err)

	organizationRows, err := testStore.ListInventoryByOrganization(
		t.Context(),
		db.ListInventoryByOrganizationParams{
			OrganizationID: organization.ID,
			PageLimit:      20,
		},
	)
	require.NoError(t, err)
	require.Len(t, organizationRows, 1)

	productRows, err := testStore.ListInventoryByProduct(
		t.Context(),
		db.ListInventoryByProductParams{
			OrganizationID: organization.ID,
			ProductID:      productVariant.ProductID,
		},
	)
	require.NoError(t, err)
	require.Len(t, productRows, 1)
	require.Equal(t, productVariant.ID, productRows[0].ProductVariantID)

	variantRows, err := testStore.ListInventoryByVariant(
		t.Context(),
		db.ListInventoryByVariantParams{
			OrganizationID:   organization.ID,
			ProductVariantID: productVariant.ID,
		},
	)
	require.NoError(t, err)
	require.Len(t, variantRows, 1)
	require.Equal(t, warehouse.ID, variantRows[0].WarehouseID)

	detail, err := testStore.GetInventoryByVariantAndWarehouseForOrganization(
		t.Context(),
		db.GetInventoryByVariantAndWarehouseForOrganizationParams{
			OrganizationID:   organization.ID,
			ProductVariantID: productVariant.ID,
			WarehouseID:      warehouse.ID,
		},
	)
	require.NoError(t, err)
	require.Equal(t, productVariant.ProductID, detail.ProductID)
	require.Equal(t, productVariant.Sku, detail.ProductVariantSku)
}
