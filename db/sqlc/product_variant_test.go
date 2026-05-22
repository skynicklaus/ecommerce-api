//go:build integration

package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomProductVariantWithOrg(
	t *testing.T,
	organization db.Organization,
) db.ProductVariant {
	t.Helper()
	product := createRandomProductWithOrg(t, organization)

	arg := db.CreateProductVariantParams{
		ProductID:      product.ID,
		OrganizationID: product.OrganizationID,
		Name:           util.GetRandomString(t, 8),
		Sku:            fmt.Sprintf("%s.%s", product.Slug, util.GetRandomString(t, 8)),
		Price:          util.GetRandomPrice(),
	}

	productVariant, err := testStore.CreateProductVariant(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, productVariant)

	require.Equal(t, arg.ProductID, productVariant.ProductID)
	require.Equal(t, arg.OrganizationID, productVariant.OrganizationID)
	require.Equal(t, arg.Name, productVariant.Name)
	require.Equal(t, arg.Sku, productVariant.Sku)
	require.True(t, arg.Price.Equal(productVariant.Price))
	require.True(t, productVariant.TrackInventory)
	require.False(t, productVariant.IsActive)
	require.NotZero(t, productVariant.CreatedAt)
	require.NotZero(t, productVariant.UpdatedAt)

	return productVariant
}

func createRandomProductVariant(t *testing.T) db.ProductVariant {
	t.Helper()
	organization := createRandomOrganization(t)
	return createRandomProductVariantWithOrg(t, organization)
}

func TestCreateProductVariant(t *testing.T) {
	createRandomProductVariant(t)
}

func TestGetProductVariantByID(t *testing.T) {
	productVariant1 := createRandomProductVariant(t)

	productVariant2, err := testStore.GetProductVariantByID(
		t.Context(),
		productVariant1.ID,
	)
	require.NoError(t, err)
	require.NotEmpty(t, productVariant2)

	require.Equal(t, productVariant1.ID, productVariant2.ID)
	require.Equal(t, productVariant1.OrganizationID, productVariant2.OrganizationID)
	require.Equal(t, productVariant1.Name, productVariant2.Name)
	require.Equal(t, productVariant1.Sku, productVariant2.Sku)
	require.Equal(t, productVariant1.Price, productVariant2.Price)
	require.Equal(t, productVariant1.TrackInventory, productVariant2.TrackInventory)
	require.Equal(t, productVariant1.IsActive, productVariant2.IsActive)
	require.WithinDuration(t, productVariant1.CreatedAt, productVariant2.CreatedAt, 5*time.Second)
	require.WithinDuration(t, productVariant1.UpdatedAt, productVariant2.UpdatedAt, 5*time.Second)
}

func TestProductVariantListQueries(t *testing.T) {
	org := createRandomOrganization(t)

	p1 := createRandomProductWithOrg(t, org)
	p2 := createRandomProductWithOrg(t, org)

	v1, err := testStore.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      p1.ID,
		OrganizationID: org.ID,
		Name:           "Variant 1",
		Sku:            "sku-v1-" + util.GetRandomString(t, 6),
		Price:          util.GetRandomPrice(),
	})
	require.NoError(t, err)

	v2, err := testStore.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      p1.ID,
		OrganizationID: org.ID,
		Name:           "Variant 2",
		Sku:            "sku-v2-" + util.GetRandomString(t, 6),
		Price:          util.GetRandomPrice(),
	})
	require.NoError(t, err)

	v3, err := testStore.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      p2.ID,
		OrganizationID: org.ID,
		Name:           "Variant 3",
		Sku:            "sku-v3-" + util.GetRandomString(t, 6),
		Price:          util.GetRandomPrice(),
	})
	require.NoError(t, err)

	variants, err := testStore.ListProductVariantsByProductID(t.Context(), p1.ID)
	require.NoError(t, err)
	require.Len(t, variants, 2)
	require.Contains(t, []uuid.UUID{v1.ID, v2.ID}, variants[0].ID)
	require.Contains(t, []uuid.UUID{v1.ID, v2.ID}, variants[1].ID)

	batchVariants, err := testStore.ListProductVariantsByProductIDs(t.Context(), []uuid.UUID{p1.ID, p2.ID})
	require.NoError(t, err)
	require.Len(t, batchVariants, 3)

	var found1, found2, found3 bool
	for _, v := range batchVariants {
		switch v.ID {
		case v1.ID:
			found1 = true
		case v2.ID:
			found2 = true
		case v3.ID:
			found3 = true
		}
	}
	require.True(t, found1)
	require.True(t, found2)
	require.True(t, found3)
}

func TestListVariantAttributes(t *testing.T) {
	org := createRandomOrganization(t)
	p := createRandomProductWithOrg(t, org)

	v, err := testStore.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      p.ID,
		OrganizationID: org.ID,
		Name:           "Red Large Variant",
		Sku:            "sku-attr-" + util.GetRandomString(t, 6),
		Price:          util.GetRandomPrice(),
	})
	require.NoError(t, err)

	val := createRandomAttributeValueWithOrg(t, &org)

	err = testStore.AssignAttributeValueToProductVariant(t.Context(), db.AssignAttributeValueToProductVariantParams{
		ProductVariantID: v.ID,
		AttributeValueID: val.ID,
	})
	require.NoError(t, err)

	rows, err := testStore.ListVariantAttributesByProduct(t.Context(), p.ID)
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	var found bool
	for _, r := range rows {
		if r.ProductVariantID == v.ID && r.AttributeValueID == val.ID {
			found = true
			break
		}
	}
	require.True(t, found)

	batchRows, err := testStore.ListVariantAttributesByProductIDs(t.Context(), []uuid.UUID{p.ID})
	require.NoError(t, err)
	require.NotEmpty(t, batchRows)

	var foundBatch bool
	for _, r := range batchRows {
		if r.ProductVariantID == v.ID && r.AttributeValueID == val.ID {
			foundBatch = true
			break
		}
	}
	require.True(t, foundBatch)
}
