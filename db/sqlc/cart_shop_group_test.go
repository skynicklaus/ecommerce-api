//go:build integration

package db_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

func TestCartShopGroupQueries(t *testing.T) {
	t.Run("creates_and_returns_existing_shop_group_for_seller_org", func(t *testing.T) {
		buyerOrg := createBuyerOrganization(t)
		sellerOrg := createSellerOrganization(t)
		cart, err := testStore.CreateCart(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		cleanupCart(t, cart.ID)

		group1, err := testStore.GetOrCreateCartShopGroup(t.Context(), db.GetOrCreateCartShopGroupParams{
			CartID:        cart.ID,
			MerchantOrgID: sellerOrg.ID,
		})
		require.NoError(t, err)
		require.Equal(t, cart.ID, group1.CartID)
		require.Equal(t, sellerOrg.ID, group1.MerchantOrgID)

		group2, err := testStore.GetOrCreateCartShopGroup(t.Context(), db.GetOrCreateCartShopGroupParams{
			CartID:        cart.ID,
			MerchantOrgID: sellerOrg.ID,
		})
		require.NoError(t, err)
		require.Equal(t, group1.ID, group2.ID)
	})

	t.Run("rejects_buyer_org_as_merchant_org", func(t *testing.T) {
		buyerOrg := createBuyerOrganization(t)
		cart, err := testStore.CreateCart(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		cleanupCart(t, cart.ID)

		_, err = testStore.GetOrCreateCartShopGroup(t.Context(), db.GetOrCreateCartShopGroupParams{
			CartID:        cart.ID,
			MerchantOrgID: buyerOrg.ID,
		})
		require.Error(t, err)
	})
}

func TestRecalculateCartShopGroupSubtotal(t *testing.T) {
	_, _, _, group, variant := createCartFixture(t)
	item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
		CartShopGroupID:  group.ID,
		ProductVariantID: variant.ID,
		Quantity:         3,
		UnitPrice:        decimal.NewFromInt(15),
	})
	require.NoError(t, err)

	recalculated, err := testStore.RecalculateCartShopGroupSubtotal(t.Context(), group.ID)
	require.NoError(t, err)
	expected := item.UnitPrice.Mul(decimal.NewFromInt(int64(item.Quantity)))
	require.True(t, recalculated.Subtotal.Equal(expected))
}

func TestDeleteEmptyCartShopGroups(t *testing.T) {
	_, _, cart, group, variant := createCartFixture(t)
	_, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
		CartShopGroupID:  group.ID,
		ProductVariantID: variant.ID,
		Quantity:         1,
		UnitPrice:        decimal.NewFromInt(10),
	})
	require.NoError(t, err)

	err = testStore.DeleteEmptyCartShopGroups(t.Context(), cart.ID)
	require.NoError(t, err)
	details, err := testStore.GetCartDetails(t.Context(), cart.CustomerOrgID)
	require.NoError(t, err)
	require.Len(t, details, 1)

	err = testStore.DeleteCartItemForCustomerOrg(t.Context(), db.DeleteCartItemForCustomerOrgParams{
		CartItemID:    details[0].CartItemID,
		CustomerOrgID: cart.CustomerOrgID,
	})
	require.NoError(t, err)

	err = testStore.DeleteEmptyCartShopGroups(t.Context(), cart.ID)
	require.NoError(t, err)
	details, err = testStore.GetCartDetails(t.Context(), cart.CustomerOrgID)
	require.NoError(t, err)
	require.Empty(t, details)
}
