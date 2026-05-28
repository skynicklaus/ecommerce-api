//go:build integration

package db_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

func TestSQLStore_AddCartItemTx(t *testing.T) {
	t.Run("success/creates_cart_group_and_item", func(t *testing.T) {
		buyerOrg := createBuyerOrganization(t)
		sellerOrg := createSellerOrganization(t)
		variant := createActiveVariantForCart(t, sellerOrg)

		result, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
			CustomerOrgID:    buyerOrg.ID,
			ProductVariantID: variant.ID,
			Quantity:         2,
		})
		require.NoError(t, err)
		cleanupCart(t, result.Cart.ID)
		require.Equal(t, buyerOrg.ID, result.Cart.CustomerOrgID)
		require.Equal(t, result.Cart.ID, result.ShopGroup.CartID)
		require.Equal(t, sellerOrg.ID, result.ShopGroup.MerchantOrgID)
		require.Equal(t, variant.ID, result.Item.ProductVariantID)
		require.Equal(t, int16(2), result.Item.Quantity)
		require.True(t, result.Item.UnitPrice.Equal(variant.Price))
		require.True(t, result.Item.IsSelected)
		require.True(t, result.UpdatedGroup.IsSelected)

		expectedSubtotal := variant.Price.Mul(decimal.NewFromInt(2))
		require.True(t, result.UpdatedGroup.Subtotal.Equal(expectedSubtotal))

		details, err := testStore.GetCartDetails(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		require.Len(t, details, 1)
		require.Equal(t, result.Item.ID, details[0].CartItemID)
		require.True(t, details[0].GroupIsSelected)
	})

	t.Run("success/increments_existing_item_and_recalculates_subtotal", func(t *testing.T) {
		buyerOrg := createBuyerOrganization(t)
		sellerOrg := createSellerOrganization(t)
		variant := createActiveVariantForCart(t, sellerOrg)

		first, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
			CustomerOrgID:    buyerOrg.ID,
			ProductVariantID: variant.ID,
			Quantity:         1,
		})
		require.NoError(t, err)
		cleanupCart(t, first.Cart.ID)

		second, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
			CustomerOrgID:    buyerOrg.ID,
			ProductVariantID: variant.ID,
			Quantity:         3,
		})
		require.NoError(t, err)
		require.Equal(t, first.Cart.ID, second.Cart.ID)
		require.Equal(t, first.ShopGroup.ID, second.ShopGroup.ID)
		require.Equal(t, first.Item.ID, second.Item.ID)
		require.Equal(t, int16(4), second.Item.Quantity)

		expectedSubtotal := variant.Price.Mul(decimal.NewFromInt(4))
		require.True(t, second.UpdatedGroup.Subtotal.Equal(expectedSubtotal))
		require.True(t, second.UpdatedGroup.IsSelected)
	})

	t.Run("success/keeps_group_unselected_when_existing_item_is_unselected", func(t *testing.T) {
		buyerOrg := createBuyerOrganization(t)
		sellerOrg := createSellerOrganization(t)
		variantA := createActiveVariantForCart(t, sellerOrg)
		variantB := createActiveVariantForCart(t, sellerOrg)

		first, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
			CustomerOrgID:    buyerOrg.ID,
			ProductVariantID: variantA.ID,
			Quantity:         1,
		})
		require.NoError(t, err)
		cleanupCart(t, first.Cart.ID)

		_, err = testStore.SetCartItemSelectedTx(t.Context(), db.SetCartItemSelectedTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    first.Item.ID,
			IsSelected:    false,
		})
		require.NoError(t, err)

		second, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
			CustomerOrgID:    buyerOrg.ID,
			ProductVariantID: variantB.ID,
			Quantity:         1,
		})
		require.NoError(t, err)
		require.Equal(t, first.ShopGroup.ID, second.ShopGroup.ID)
		require.False(t, second.UpdatedGroup.IsSelected)
	})

	t.Run("fail/rejects_seller_org_as_customer_org", func(t *testing.T) {
		sellerOrg := createSellerOrganization(t)
		variant := createActiveVariantForCart(t, sellerOrg)

		_, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
			CustomerOrgID:    sellerOrg.ID,
			ProductVariantID: variant.ID,
			Quantity:         1,
		})
		require.Error(t, err)
	})

	t.Run("fail/rejects_inactive_variant", func(t *testing.T) {
		buyerOrg := createBuyerOrganization(t)
		sellerOrg := createSellerOrganization(t)
		variant := createRandomProductVariantWithOrg(t, sellerOrg)

		_, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
			CustomerOrgID:    buyerOrg.ID,
			ProductVariantID: variant.ID,
			Quantity:         1,
		})
		require.ErrorIs(t, err, db.ErrNotFound)
	})

	t.Run("fail/rejects_invalid_quantity_and_rolls_back_group", func(t *testing.T) {
		buyerOrg := createBuyerOrganization(t)
		sellerOrg := createSellerOrganization(t)
		variant := createActiveVariantForCart(t, sellerOrg)

		_, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
			CustomerOrgID:    buyerOrg.ID,
			ProductVariantID: variant.ID,
			Quantity:         0,
		})
		require.Error(t, err)

		details, detailsErr := testStore.GetCartDetails(t.Context(), buyerOrg.ID)
		require.NoError(t, detailsErr)
		require.Empty(t, details)
	})
}

func TestSQLStore_UpdateCartItemQuantityTx(t *testing.T) {
	t.Run("success/updates_quantity_and_recalculates_subtotal", func(t *testing.T) {
		buyerOrg, _, _, group, variant := createCartFixture(t)
		item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         2,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		result, err := testStore.UpdateCartItemQuantityTx(t.Context(), db.UpdateCartItemQuantityTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    item.ID,
			Quantity:      5,
		})
		require.NoError(t, err)
		require.Equal(t, int16(5), result.Item.Quantity)
		require.True(t, result.UpdatedGroup.Subtotal.Equal(decimal.NewFromInt(50)))
	})

	t.Run("fail/wrong_customer_org_returns_not_found", func(t *testing.T) {
		_, _, _, group, variant := createCartFixture(t)
		otherBuyerOrg := createBuyerOrganization(t)
		item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         2,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		_, err = testStore.UpdateCartItemQuantityTx(t.Context(), db.UpdateCartItemQuantityTxParams{
			CustomerOrgID: otherBuyerOrg.ID,
			CartItemID:    item.ID,
			Quantity:      5,
		})
		require.ErrorIs(t, err, db.ErrNotFound)
	})

	t.Run("fail/invalid_quantity", func(t *testing.T) {
		buyerOrg, _, _, group, variant := createCartFixture(t)
		item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         2,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		_, err = testStore.UpdateCartItemQuantityTx(t.Context(), db.UpdateCartItemQuantityTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    item.ID,
			Quantity:      0,
		})
		require.Error(t, err)
	})
}

func TestSQLStore_SetCartItemSelectedTx(t *testing.T) {
	t.Run("success/recalculates_group_selection", func(t *testing.T) {
		buyerOrg, sellerOrg, _, group, variantA := createCartFixture(t)
		variantB := createActiveVariantForCart(t, sellerOrg)

		itemA, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variantA.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)
		_, err = testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variantB.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		selected, err := testStore.SetCartItemSelectedTx(t.Context(), db.SetCartItemSelectedTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    itemA.ID,
			IsSelected:    true,
		})
		require.NoError(t, err)
		require.True(t, selected.Item.IsSelected)
		require.True(t, selected.UpdatedGroup.IsSelected)

		unselected, err := testStore.SetCartItemSelectedTx(t.Context(), db.SetCartItemSelectedTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    itemA.ID,
			IsSelected:    false,
		})
		require.NoError(t, err)
		require.False(t, unselected.Item.IsSelected)
		require.False(t, unselected.UpdatedGroup.IsSelected)

		reselected, err := testStore.SetCartItemSelectedTx(t.Context(), db.SetCartItemSelectedTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    itemA.ID,
			IsSelected:    true,
		})
		require.NoError(t, err)
		require.True(t, reselected.Item.IsSelected)
		require.True(t, reselected.UpdatedGroup.IsSelected)
	})

	t.Run("fail/wrong_customer_org_returns_not_found", func(t *testing.T) {
		_, _, _, group, variant := createCartFixture(t)
		otherBuyerOrg := createBuyerOrganization(t)
		item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		_, err = testStore.SetCartItemSelectedTx(t.Context(), db.SetCartItemSelectedTxParams{
			CustomerOrgID: otherBuyerOrg.ID,
			CartItemID:    item.ID,
			IsSelected:    false,
		})
		require.ErrorIs(t, err, db.ErrNotFound)
	})
}

func TestSQLStore_RemoveCartItemTx(t *testing.T) {
	t.Run("success/removes_item_and_empty_group", func(t *testing.T) {
		buyerOrg, _, cart, group, variant := createCartFixture(t)
		item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		err = testStore.RemoveCartItemTx(t.Context(), db.RemoveCartItemTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    item.ID,
		})
		require.NoError(t, err)

		details, err := testStore.GetCartDetails(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		require.Empty(t, details)

		_, err = testStore.SetCartShopGroupSelectedForCustomerOrg(t.Context(), db.SetCartShopGroupSelectedForCustomerOrgParams{
			CartShopGroupID: group.ID,
			CustomerOrgID:   buyerOrg.ID,
			IsSelected:      true,
		})
		require.ErrorIs(t, err, db.ErrNotFound)

		cartAfter, err := testStore.GetCartByCustomerOrgID(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		require.Equal(t, cart.ID, cartAfter.ID)
	})

	t.Run("success/recalculates_group_when_other_items_remain", func(t *testing.T) {
		buyerOrg, _, _, group, variantA := createCartFixture(t)
		var sellerOrg db.Organization
		var err error
		sellerOrg, err = testStore.GetOrganizationByID(t.Context(), group.MerchantOrgID)
		require.NoError(t, err)
		variantB := createActiveVariantForCart(t, sellerOrg)

		itemA, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variantA.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)
		itemB, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variantB.ID,
			Quantity:         2,
			UnitPrice:        decimal.NewFromInt(5),
		})
		require.NoError(t, err)

		err = testStore.RemoveCartItemTx(t.Context(), db.RemoveCartItemTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    itemA.ID,
		})
		require.NoError(t, err)

		updatedGroup, err := testStore.RecalculateCartShopGroupSubtotal(t.Context(), group.ID)
		require.NoError(t, err)
		require.True(t, updatedGroup.Subtotal.Equal(itemB.UnitPrice.Mul(decimal.NewFromInt(int64(itemB.Quantity)))))
	})

	t.Run("success/removing_unselected_item_reselects_group_when_remaining_items_are_selected", func(t *testing.T) {
		buyerOrg, sellerOrg, _, group, variantA := createCartFixture(t)
		variantB := createActiveVariantForCart(t, sellerOrg)

		itemA, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variantA.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)
		_, err = testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variantB.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		result, err := testStore.SetCartItemSelectedTx(t.Context(), db.SetCartItemSelectedTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    itemA.ID,
			IsSelected:    false,
		})
		require.NoError(t, err)
		require.False(t, result.UpdatedGroup.IsSelected)

		err = testStore.RemoveCartItemTx(t.Context(), db.RemoveCartItemTxParams{
			CustomerOrgID: buyerOrg.ID,
			CartItemID:    itemA.ID,
		})
		require.NoError(t, err)

		details, err := testStore.GetCartDetails(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		require.Len(t, details, 1)
		require.True(t, details[0].GroupIsSelected)
		require.True(t, details[0].ItemIsSelected)
	})

	t.Run("fail/wrong_customer_org_returns_not_found", func(t *testing.T) {
		_, _, _, group, variant := createCartFixture(t)
		otherBuyerOrg := createBuyerOrganization(t)
		item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		err = testStore.RemoveCartItemTx(t.Context(), db.RemoveCartItemTxParams{
			CustomerOrgID: otherBuyerOrg.ID,
			CartItemID:    item.ID,
		})
		require.ErrorIs(t, err, db.ErrNotFound)
	})
}

func TestSQLStore_SetCartShopGroupSelectedTx(t *testing.T) {
	t.Run("success/updates_group_and_items", func(t *testing.T) {
		buyerOrg, _, _, group, variant := createCartFixture(t)
		item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
			CartShopGroupID:  group.ID,
			ProductVariantID: variant.ID,
			Quantity:         1,
			UnitPrice:        decimal.NewFromInt(10),
		})
		require.NoError(t, err)

		result, err := testStore.SetCartShopGroupSelectedTx(t.Context(), db.SetCartShopGroupSelectedTxParams{
			CustomerOrgID:   buyerOrg.ID,
			CartShopGroupID: group.ID,
			IsSelected:      false,
		})
		require.NoError(t, err)
		require.False(t, result.ShopGroup.IsSelected)

		updatedItem, err := testStore.GetCartItemForCustomerOrg(t.Context(), db.GetCartItemForCustomerOrgParams{
			CartItemID:    item.ID,
			CustomerOrgID: buyerOrg.ID,
		})
		require.NoError(t, err)
		require.False(t, updatedItem.IsSelected)

		result, err = testStore.SetCartShopGroupSelectedTx(t.Context(), db.SetCartShopGroupSelectedTxParams{
			CustomerOrgID:   buyerOrg.ID,
			CartShopGroupID: group.ID,
			IsSelected:      true,
		})
		require.NoError(t, err)
		require.True(t, result.ShopGroup.IsSelected)

		updatedItem, err = testStore.GetCartItemForCustomerOrg(t.Context(), db.GetCartItemForCustomerOrgParams{
			CartItemID:    item.ID,
			CustomerOrgID: buyerOrg.ID,
		})
		require.NoError(t, err)
		require.True(t, updatedItem.IsSelected)
	})

	t.Run("fail/wrong_customer_org_returns_not_found", func(t *testing.T) {
		_, _, _, group, _ := createCartFixture(t)
		otherBuyerOrg := createBuyerOrganization(t)

		_, err := testStore.SetCartShopGroupSelectedTx(t.Context(), db.SetCartShopGroupSelectedTxParams{
			CustomerOrgID:   otherBuyerOrg.ID,
			CartShopGroupID: group.ID,
			IsSelected:      false,
		})
		require.ErrorIs(t, err, db.ErrNotFound)
	})
}
