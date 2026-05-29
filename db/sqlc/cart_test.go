//go:build integration

package db_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createCartTestOrganization(
	t *testing.T,
	organizationType util.OrganizationType,
	capability util.OrganizationCapability,
) db.Organization {
	t.Helper()

	org, err := testStore.CreateOrganization(t.Context(), db.CreateOrganizationParams{
		Name:       fmt.Sprintf("%s-%s", organizationType, uuid.NewString()),
		Slug:       fmt.Sprintf("%s-%s", organizationType, uuid.NewString()),
		Status:     string(util.OrganizationStatusActive),
		Type:       string(organizationType),
		Capability: string(capability),
		Metadata:   []byte("{}"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	return org
}

func createBuyerOrganization(t *testing.T) db.Organization {
	t.Helper()
	return createCartTestOrganization(t, util.OrganizationTypeIndividual, util.OrganizationCapabilityBuyer)
}

func cleanupCart(t *testing.T, cartID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM carts WHERE id = $1", cartID)
	})
}

func createSellerOrganization(t *testing.T) db.Organization {
	t.Helper()
	return createCartTestOrganization(t, util.OrganizationTypeMerchant, util.OrganizationCapabilitySeller)
}

func createActiveVariantForCart(t *testing.T, sellerOrg db.Organization) db.ProductVariant {
	t.Helper()

	variant := createRandomProductVariantWithOrg(t, sellerOrg)
	_, err := testStore.UpdateProductStatus(t.Context(), db.UpdateProductStatusParams{
		ID:             variant.ProductID,
		OrganizationID: sellerOrg.ID,
		Status:         string(util.ProductStatusActive),
	})
	require.NoError(t, err)

	_, err = testPool.Exec(t.Context(), "UPDATE product_variants SET is_active = TRUE WHERE id = $1", variant.ID)
	require.NoError(t, err)
	variant.IsActive = true

	return variant
}

func createCartFixture(t *testing.T) (db.Organization, db.Organization, db.Cart, db.CartShopGroup, db.ProductVariant) {
	t.Helper()

	buyerOrg := createBuyerOrganization(t)
	sellerOrg := createSellerOrganization(t)
	variant := createActiveVariantForCart(t, sellerOrg)

	cart, err := testStore.CreateCart(t.Context(), buyerOrg.ID)
	require.NoError(t, err)
	cleanupCart(t, cart.ID)

	group, err := testStore.GetOrCreateCartShopGroup(t.Context(), db.GetOrCreateCartShopGroupParams{
		CartID:        cart.ID,
		MerchantOrgID: sellerOrg.ID,
	})
	require.NoError(t, err)

	return buyerOrg, sellerOrg, cart, group, variant
}

func TestCreateCartQueries(t *testing.T) {
	t.Run("creates_and_returns_existing_cart_for_buyer_org", func(t *testing.T) {
		buyerOrg := createBuyerOrganization(t)

		cart1, err := testStore.CreateCart(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		cleanupCart(t, cart1.ID)
		require.Equal(t, buyerOrg.ID, cart1.BuyerOrgID)
		require.NotZero(t, cart1.ID)

		cart2, err := testStore.CreateCart(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		require.Equal(t, cart1.ID, cart2.ID)

		fetched, err := testStore.GetCartByBuyerOrgID(t.Context(), buyerOrg.ID)
		require.NoError(t, err)
		require.Equal(t, cart1.ID, fetched.ID)
	})

	t.Run("rejects_seller_org_as_buyer_org", func(t *testing.T) {
		sellerOrg := createSellerOrganization(t)

		_, err := testStore.CreateCart(t.Context(), sellerOrg.ID)
		require.Error(t, err)
	})
}

func TestGetCartDetails(t *testing.T) {
	buyerOrg, sellerOrg, _, group, variant := createCartFixture(t)
	productAssetKey := "assets/cart/product-" + uuid.NewString() + ".webp"
	_, err := testStore.CreateProductAsset(t.Context(), db.CreateProductAssetParams{
		ProductID:        variant.ProductID,
		ProductVariantID: nil,
		AssetKey:         productAssetKey,
		Type:             string(util.ProductAssetImage),
		MimeType:         "image/webp",
		SortOrder:        1,
		IsPrimary:        true,
	})
	require.NoError(t, err)

	item, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
		CartShopGroupID:  group.ID,
		ProductVariantID: variant.ID,
		Quantity:         2,
		UnitPrice:        decimal.NewFromInt(22),
	})
	require.NoError(t, err)

	details, err := testStore.GetCartDetails(t.Context(), buyerOrg.ID)
	require.NoError(t, err)
	require.Len(t, details, 1)

	row := details[0]
	require.Equal(t, buyerOrg.ID, row.BuyerOrgID)
	require.Equal(t, group.ID, row.CartShopGroupID)
	require.Equal(t, sellerOrg.ID, row.MerchantOrgID)
	require.Equal(t, sellerOrg.Name, row.MerchantName)
	require.Equal(t, item.ID, row.CartItemID)
	require.Equal(t, variant.ID, row.ProductVariantID)
	require.Equal(t, item.Quantity, row.Quantity)
	require.True(t, item.UnitPrice.Equal(row.UnitPrice))
	require.Equal(t, variant.ProductID, row.ProductID)
	require.Equal(t, variant.Name, row.VariantName)
	require.Equal(t, variant.Sku, row.Sku)
	require.True(t, variant.Price.Equal(row.CurrentPrice))
	require.Equal(t, string(util.ProductStatusActive), row.ProductStatus)
	require.Equal(t, productAssetKey, row.ThumbnailAssetKey)
	require.Equal(t, "product", row.ThumbnailSource)

	variantID := variant.ID
	variantAssetKey := "assets/cart/variant-" + uuid.NewString() + ".webp"
	_, err = testStore.CreateProductAsset(t.Context(), db.CreateProductAssetParams{
		ProductID:        variant.ProductID,
		ProductVariantID: &variantID,
		AssetKey:         variantAssetKey,
		Type:             string(util.ProductAssetImage),
		MimeType:         "image/webp",
		SortOrder:        2,
		IsPrimary:        false,
	})
	require.NoError(t, err)

	details, err = testStore.GetCartDetails(t.Context(), buyerOrg.ID)
	require.NoError(t, err)
	require.Len(t, details, 1)
	require.Equal(t, variantAssetKey, details[0].ThumbnailAssetKey)
	require.Equal(t, "variant", details[0].ThumbnailSource)
}

func TestCheckoutCartQueries(t *testing.T) {
	buyerOrg, sellerOrg, _, group, variantA := createCartFixture(t)
	variantB := createActiveVariantForCart(t, sellerOrg)

	itemA, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
		CartShopGroupID:  group.ID,
		ProductVariantID: variantA.ID,
		Quantity:         1,
		UnitPrice:        decimal.NewFromInt(11),
	})
	require.NoError(t, err)
	itemB, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
		CartShopGroupID:  group.ID,
		ProductVariantID: variantB.ID,
		Quantity:         1,
		UnitPrice:        decimal.NewFromInt(12),
	})
	require.NoError(t, err)

	selectedRows, err := testStore.ListSelectedCartItemsForCheckout(t.Context(), buyerOrg.ID)
	require.NoError(t, err)
	require.Len(t, selectedRows, 2)

	deletedRows, err := testStore.DeleteSelectedCartItemsForCheckout(
		t.Context(),
		db.DeleteSelectedCartItemsForCheckoutParams{
			BuyerOrgID:  buyerOrg.ID,
			CartItemIds: []uuid.UUID{itemA.ID},
		},
	)
	require.NoError(t, err)
	require.Len(t, deletedRows, 1)
	require.Equal(t, itemA.ID, deletedRows[0].ID)

	remainingRows, err := testStore.GetCartDetails(t.Context(), buyerOrg.ID)
	require.NoError(t, err)
	require.Len(t, remainingRows, 1)
	require.Equal(t, itemB.ID, remainingRows[0].CartItemID)

	otherBuyerOrg := createBuyerOrganization(t)
	otherCart, err := testStore.CreateCart(t.Context(), otherBuyerOrg.ID)
	require.NoError(t, err)
	cleanupCart(t, otherCart.ID)
	otherGroup, err := testStore.GetOrCreateCartShopGroup(t.Context(), db.GetOrCreateCartShopGroupParams{
		CartID:        otherCart.ID,
		MerchantOrgID: sellerOrg.ID,
	})
	require.NoError(t, err)
	otherItem, err := testStore.UpsertCartItem(t.Context(), db.UpsertCartItemParams{
		CartShopGroupID:  otherGroup.ID,
		ProductVariantID: variantA.ID,
		Quantity:         1,
		UnitPrice:        decimal.NewFromInt(13),
	})
	require.NoError(t, err)

	deletedRows, err = testStore.DeleteSelectedCartItemsForCheckout(
		t.Context(),
		db.DeleteSelectedCartItemsForCheckoutParams{
			BuyerOrgID:  buyerOrg.ID,
			CartItemIds: []uuid.UUID{otherItem.ID},
		},
	)
	require.NoError(t, err)
	require.Empty(t, deletedRows)

	otherRows, err := testStore.GetCartDetails(t.Context(), otherBuyerOrg.ID)
	require.NoError(t, err)
	require.Len(t, otherRows, 1)
	require.Equal(t, otherItem.ID, otherRows[0].CartItemID)
}
