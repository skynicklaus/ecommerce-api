//go:build integration

package db_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

func TestCartItemQueries(t *testing.T) {
	t.Run("upsert_inserts_then_increments_quantity_and_updates_price", func(t *testing.T) {
		_, _, _, group, variant := createCartFixture(t)

		firstPrice := decimal.NewFromInt(10)
		item1, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         2,
			UnitPrice:        firstPrice,
		})
		require.NoError(t, err)
		require.Equal(t, int16(2), item1.Quantity)
		require.True(t, item1.UnitPrice.Equal(firstPrice))
		require.True(t, item1.IsSelected)

		secondPrice := decimal.NewFromInt(12)
		item2, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         3,
			UnitPrice:        secondPrice,
		})
		require.NoError(t, err)
		require.Equal(t, item1.ID, item2.ID)
		require.Equal(t, int16(5), item2.Quantity)
		require.True(t, item2.UnitPrice.Equal(secondPrice))
		require.True(t, item2.IsSelected)
	})

	t.Run("rejects_invalid_quantity", func(t *testing.T) {
		_, _, _, group, variant := createCartFixture(t)

		_, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         0,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.Error(t, err)
	})

	t.Run("rejects_negative_price", func(t *testing.T) {
		_, _, _, group, variant := createCartFixture(t)

		_, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(-1),
		})
		require.Error(t, err)
	})
}

func TestCartItemScopedMutations(t *testing.T) {
	buyerOrg, _, _, group, variant := createCartFixture(t)
	otherBuyerOrg := createBuyerOrganization(t)

	item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
		CartShopGroupID:  group.ID,
		ProductVariantID: variant.ID,
		Quantity:         2,
		UnitPrice:        decimal.NewFromInt(15),
	})
	require.NoError(t, err)

	t.Run("update_quantity_scoped_to_buyer_org", func(t *testing.T) {
		updated, updateErr := testStore.UpdateCartItemQuantityForBuyerOrg(
			t.Context(),
			db.UpdateCartItemQuantityForBuyerOrgParams{
				CartItemID:    item.ID,
				BuyerOrgID: buyerOrg.ID,
				Quantity:      4,
			},
		)
		require.NoError(t, updateErr)
		require.Equal(t, int16(4), updated.Quantity)

		_, updateErr = testStore.UpdateCartItemQuantityForBuyerOrg(
			t.Context(),
			db.UpdateCartItemQuantityForBuyerOrgParams{
				CartItemID:    item.ID,
				BuyerOrgID: otherBuyerOrg.ID,
				Quantity:      1,
			},
		)
		require.ErrorIs(t, updateErr, db.ErrNotFound)
	})

	t.Run("set_item_selected_scoped_to_buyer_org", func(t *testing.T) {
		updated, updateErr := testStore.SetCartItemSelectedForBuyerOrg(
			t.Context(),
			db.SetCartItemSelectedForBuyerOrgParams{
				CartItemID:    item.ID,
				BuyerOrgID: buyerOrg.ID,
				IsSelected:    false,
			},
		)
		require.NoError(t, updateErr)
		require.False(t, updated.IsSelected)

		_, updateErr = testStore.SetCartItemSelectedForBuyerOrg(
			t.Context(),
			db.SetCartItemSelectedForBuyerOrgParams{
				CartItemID:    item.ID,
				BuyerOrgID: otherBuyerOrg.ID,
				IsSelected:    true,
			},
		)
		require.ErrorIs(t, updateErr, db.ErrNotFound)
	})

	t.Run("set_group_items_selected_scoped_to_buyer_org", func(t *testing.T) {
		updateErr := testStore.SetCartItemsSelectedByGroupForBuyerOrg(
			t.Context(),
			db.SetCartItemsSelectedByGroupForBuyerOrgParams{
				CartShopGroupID: group.ID,
				BuyerOrgID:   buyerOrg.ID,
				IsSelected:      false,
			},
		)
		require.NoError(t, updateErr)

		updated, updateErr := testStore.SetCartItemSelectedForBuyerOrg(
			t.Context(),
			db.SetCartItemSelectedForBuyerOrgParams{
				CartItemID:    item.ID,
				BuyerOrgID: buyerOrg.ID,
				IsSelected:    true,
			},
		)
		require.NoError(t, updateErr)
		require.True(t, updated.IsSelected)
	})

	t.Run("delete_item_scoped_to_buyer_org", func(t *testing.T) {
		deleteErr := testStore.DeleteCartItemForBuyerOrg(
			t.Context(),
			db.DeleteCartItemForBuyerOrgParams{
				CartItemID:    item.ID,
				BuyerOrgID: otherBuyerOrg.ID,
			},
		)
		require.NoError(t, deleteErr)

		details, detailsErr := testStore.GetCartDetails(t.Context(), buyerOrg.ID)
		require.NoError(t, detailsErr)
		require.Len(t, details, 1)

		deleteErr = testStore.DeleteCartItemForBuyerOrg(
			t.Context(),
			db.DeleteCartItemForBuyerOrgParams{
				CartItemID:    item.ID,
				BuyerOrgID: buyerOrg.ID,
			},
		)
		require.NoError(t, deleteErr)

		details, detailsErr = testStore.GetCartDetails(t.Context(), buyerOrg.ID)
		require.NoError(t, detailsErr)
		require.Empty(t, details)
	})
}
