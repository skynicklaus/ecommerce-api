//go:build integration

package db_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestSQLStore_CreateProductTx(t *testing.T) {
	ctx := t.Context()

	org := createRandomOrganization(t)
	orgID := org.ID

	category, err := testStore.CreateCategory(ctx, db.CreateCategoryParams{
		OrganizationID: &orgID,
		ParentID:       nil,
		Name:           util.GetRandomString(t, 8),
		Slug:           util.GetRandomString(t, 12),
		Description:    nil,
		SortOrder:      1,
	})
	require.NoError(t, err)

	attrVal := createRandomAttributeValueWithOrg(t, &org)

	desc, err := json.Marshal(map[string]string{"text": "product description"})
	require.NoError(t, err)
	spec, err := json.Marshal(map[string]string{"weight": "1kg"})
	require.NoError(t, err)
	altText := "primary image"

	t.Run("success/with_variants_and_assets", func(t *testing.T) {
		arg := db.CreateProductTxParams{
			OrganizationID: org.ID,
			CategoryID:     category.ID,
			Name:           util.GetRandomString(t, 10),
			Slug:           "product-" + uuid.New().String()[:8],
			Description:    desc,
			Specification:  spec,
			IdempotencyKey: nil,
			Assets: []db.ProductAssetParams{
				{
					AssetKey:        "assets/" + org.ID.String() + "/primary.jpg",
					Type:            "image",
					MimeType:        "image/jpeg",
					AltText:         &altText,
					SortOrder:       1,
					IsPrimary:       true,
					DurationSeconds: nil,
				},
			},
			Variants: []db.ProductVariantParams{
				{
					Sku:               "SKU-" + uuid.New().String()[:8],
					Name:              "Default Variant",
					Price:             decimal.NewFromFloat(99.99),
					AttributeValueIDs: []int64{attrVal.ID},
					Asset:             nil,
				},
			},
		}

		got, txErr := testStore.CreateProductTx(ctx, arg)
		require.NoError(t, txErr)

		require.NotEmpty(t, got.Product.ID)
		require.Equal(t, arg.Name, got.Product.Name)
		require.Equal(t, arg.Slug, got.Product.Slug)
		require.Equal(t, arg.CategoryID, got.Product.CategoryID)
		require.Equal(t, arg.OrganizationID, got.Product.OrganizationID)

		require.Len(t, got.ProductVariants, 1)
		require.Equal(t, arg.Variants[0].Sku, got.ProductVariants[0].Sku)
		require.True(t, arg.Variants[0].Price.Equal(got.ProductVariants[0].Price))

		require.Len(t, got.ProductAssets, 1)
		require.Equal(t, arg.Assets[0].AssetKey, got.ProductAssets[0].AssetKey)
		require.True(t, got.ProductAssets[0].IsPrimary)

		// Attribute assignment is verified via the list query.
		variantAttrs, listErr := testStore.ListVariantAttributesByProduct(ctx, got.Product.ID)
		require.NoError(t, listErr)
		require.Len(t, variantAttrs, 1)
		require.Equal(t, attrVal.ID, variantAttrs[0].AttributeValueID)
	})

	t.Run("success/with_variant_asset", func(t *testing.T) {
		variantAltText := "variant image"
		variantAsset := &db.ProductAssetParams{
			AssetKey:        "assets/" + org.ID.String() + "/variant.jpg",
			Type:            "image",
			MimeType:        "image/jpeg",
			AltText:         &variantAltText,
			SortOrder:       1,
			IsPrimary:       false,
			DurationSeconds: nil,
		}

		arg := db.CreateProductTxParams{
			OrganizationID: org.ID,
			CategoryID:     category.ID,
			Name:           util.GetRandomString(t, 10),
			Slug:           "product-" + uuid.New().String()[:8],
			Description:    desc,
			Specification:  spec,
			IdempotencyKey: nil,
			Assets:         nil,
			Variants: []db.ProductVariantParams{
				{
					Sku:               "SKU-" + uuid.New().String()[:8],
					Name:              "Variant With Asset",
					Price:             decimal.NewFromFloat(49.99),
					AttributeValueIDs: nil,
					Asset:             variantAsset,
				},
			},
		}

		got, txErr := testStore.CreateProductTx(ctx, arg)
		require.NoError(t, txErr)

		require.Len(t, got.ProductVariants, 1)
		require.Len(t, got.ProductAssets, 1)

		// The variant asset must be linked to the variant, not the product only.
		require.Equal(t, got.ProductVariants[0].ID, *got.ProductAssets[0].ProductVariantID)
		require.Equal(t, variantAsset.AssetKey, got.ProductAssets[0].AssetKey)
	})

	t.Run("fail/invalid_category_rolls_back", func(t *testing.T) {
		slug := "rollback-" + uuid.New().String()[:8]

		arg := db.CreateProductTxParams{
			OrganizationID: org.ID,
			CategoryID:     uuid.New(), // non-existent — triggers FK violation
			Name:           util.GetRandomString(t, 10),
			Slug:           slug,
			Description:    desc,
			Specification:  spec,
			IdempotencyKey: nil,
			Assets:         nil,
			Variants: []db.ProductVariantParams{
				{
					Sku:               "SKU-" + uuid.New().String()[:8],
					Name:              "Should Not Exist",
					Price:             decimal.NewFromFloat(1.00),
					AttributeValueIDs: nil,
					Asset:             nil,
				},
			},
		}

		_, txErr := testStore.CreateProductTx(ctx, arg)
		require.Error(t, txErr)

		// Transaction must have rolled back — the product slug must not exist.
		_, getErr := testStore.GetProductBySlug(ctx, slug)
		require.Error(t, getErr, "product must not exist after a rolled-back transaction")
	})
}
