//go:build integration

package db_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestGetActiveVariantForCart(t *testing.T) {
	t.Run("returns_active_variant_for_active_product", func(t *testing.T) {
		sellerOrg := createSellerOrganization(t)
		variant := createActiveVariantForCart(t, sellerOrg)

		row, err := testStore.GetActiveVariantForCart(t.Context(), variant.ID)
		require.NoError(t, err)
		require.Equal(t, variant.ID, row.ID)
		require.Equal(t, sellerOrg.ID, row.MerchantOrgID)
		require.True(t, row.Price.Equal(variant.Price))
		require.True(t, row.VariantIsActive)
		require.Equal(t, string(util.ProductStatusActive), row.ProductStatus)
	})

	t.Run("rejects_inactive_variant_or_draft_product", func(t *testing.T) {
		sellerOrg := createSellerOrganization(t)
		variant := createRandomProductVariantWithOrg(t, sellerOrg)

		_, err := testStore.GetActiveVariantForCart(t.Context(), variant.ID)
		require.ErrorIs(t, err, db.ErrNotFound)
	})
}
