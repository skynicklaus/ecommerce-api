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

func createRandomProductWithOrg(t *testing.T, organization db.Organization) db.Product {
	t.Helper()
	category := createRandomCategory(t)

	n := util.CoinFlip(t)
	var specification []byte
	if n == 1 {
		specification = util.GetRandomDescriptionJSON(t, 25)
	}

	arg := db.CreateProductParams{
		OrganizationID: organization.ID,
		CategoryID:     category.ID,
		Name:           util.GetRandomString(t, 8),
		Slug:           fmt.Sprintf("%s.%s", organization.Slug, util.GetRandomString(t, 8)),
		Description:    util.GetRandomDescriptionJSON(t, 20),
		Specification:  specification,
	}

	product, err := testStore.CreateProduct(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, product)

	require.Equal(t, arg.OrganizationID, product.OrganizationID)
	require.Equal(t, arg.CategoryID, product.CategoryID)
	require.Equal(t, arg.Name, product.Name)
	require.Equal(t, arg.Slug, product.Slug)
	require.Equal(t, string(util.ProductStatusDraft), product.Status)
	require.JSONEq(t, string(arg.Description), string(product.Description))
	require.False(t, product.IsFeatured)
	require.NotZero(t, product.CreatedAt)
	require.NotZero(t, product.UpdatedAt)

	if len(specification) == 0 {
		require.Empty(t, product.Specification)
	} else {
		require.JSONEq(t, string(arg.Specification), string(product.Specification))
	}

	return product
}

func createRandomProduct(t *testing.T) db.Product {
	t.Helper()
	organization := createRandomOrganization(t)
	return createRandomProductWithOrg(t, organization)
}

func TestCreateProduct(t *testing.T) {
	createRandomProduct(t)
}

func TestGetProductByID(t *testing.T) {
	product1 := createRandomProduct(t)

	product2, err := testStore.GetProductByID(t.Context(), product1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, product2)

	require.Equal(t, product1.ID, product2.ID)
	require.Equal(t, product1.OrganizationID, product2.OrganizationID)
	require.Equal(t, product1.CategoryID, product2.CategoryID)
	require.Equal(t, product1.Name, product2.Name)
	require.Equal(t, product1.Slug, product2.Slug)
	require.Equal(t, product1.Status, product2.Status)
	require.JSONEq(t, string(product1.Description), string(product2.Description))
	require.WithinDuration(t, product1.CreatedAt, product2.CreatedAt, 5*time.Second)
	require.WithinDuration(t, product1.UpdatedAt, product2.UpdatedAt, 5*time.Second)

	if len(product1.Specification) > 0 {
		require.JSONEq(t, string(product1.Specification), string(product2.Specification))
	} else {
		require.Equal(t, product1.Specification, product2.Specification)
	}
}

func TestUpdateProductStatus(t *testing.T) {
	product := createRandomProduct(t)
	require.Equal(t, string(util.ProductStatusDraft), product.Status)

	row, err := testStore.UpdateProductStatus(t.Context(), db.UpdateProductStatusParams{
		ID:             product.ID,
		OrganizationID: product.OrganizationID,
		Status:         string(util.ProductStatusActive),
	})
	require.NoError(t, err)
	require.Equal(t, product.ID, row.ID)
	require.Equal(t, string(util.ProductStatusActive), row.Status)
	require.NotZero(t, row.UpdatedAt)

	fetched, err := testStore.GetProductByID(t.Context(), product.ID)
	require.NoError(t, err)
	require.Equal(t, string(util.ProductStatusActive), fetched.Status)
}

func TestDeleteProductArchivesProductAndDeactivatesVariants(t *testing.T) {
	product := createRandomProduct(t)
	variant, err := testStore.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      product.ID,
		OrganizationID: product.OrganizationID,
		Sku:            "archive-product-variant-" + uuid.NewString()[:8],
		Name:           "Archive Product Variant",
		Price:          util.GetRandomPrice(),
	})
	require.NoError(t, err)
	_, err = testPool.Exec(t.Context(), "UPDATE product_variants SET is_active = TRUE WHERE id = $1", variant.ID)
	require.NoError(t, err)

	err = testStore.DeleteProduct(t.Context(), db.DeleteProductParams{
		ID:             product.ID,
		OrganizationID: product.OrganizationID,
	})
	require.NoError(t, err)

	archived, err := testStore.GetProductByID(t.Context(), product.ID)
	require.NoError(t, err)
	require.Equal(t, string(util.ProductStatusArchived), archived.Status)

	variants, err := testStore.ListProductVariantsByProductID(t.Context(), product.ID)
	require.NoError(t, err)
	require.Len(t, variants, 1)
	require.False(t, variants[0].IsActive)
}

func TestGetActiveProductByIDAndSlug(t *testing.T) {
	product := createRandomProduct(t)

	// Should fail since it is in 'draft' status
	_, err := testStore.GetActiveProductByID(t.Context(), product.ID)
	require.Error(t, err)

	_, err = testStore.GetActiveProductBySlug(t.Context(), db.GetActiveProductBySlugParams{
		OrganizationID: product.OrganizationID,
		Slug:           product.Slug,
	})
	require.Error(t, err)

	// Activate product
	_, err = testStore.UpdateProductStatus(t.Context(), db.UpdateProductStatusParams{
		ID:             product.ID,
		OrganizationID: product.OrganizationID,
		Status:         string(util.ProductStatusActive),
	})
	require.NoError(t, err)

	// Should succeed now
	rowID, err := testStore.GetActiveProductByID(t.Context(), product.ID)
	require.NoError(t, err)
	require.Equal(t, product.ID, rowID.ID)
	require.Equal(t, string(util.ProductStatusActive), rowID.Status)

	rowSlug, err := testStore.GetActiveProductBySlug(t.Context(), db.GetActiveProductBySlugParams{
		OrganizationID: product.OrganizationID,
		Slug:           product.Slug,
	})
	require.NoError(t, err)
	require.Equal(t, product.ID, rowSlug.ID)
	require.Equal(t, product.Slug, rowSlug.Slug)
}

func TestGetProductBySlug(t *testing.T) {
	product := createRandomProduct(t)

	row, err := testStore.GetProductBySlug(t.Context(), db.GetProductBySlugParams{
		OrganizationID: product.OrganizationID,
		Slug:           product.Slug,
	})
	require.NoError(t, err)
	require.Equal(t, product.ID, row.ID)
	require.Equal(t, product.Slug, row.Slug)
}

func TestGetProductByIdempotencyKey(t *testing.T) {
	organization := createRandomOrganization(t)
	category := createRandomCategory(t)
	key := "idempotency-key-" + util.GetRandomString(t, 12)

	arg := db.CreateProductParams{
		OrganizationID: organization.ID,
		CategoryID:     category.ID,
		Name:           "Idempotent Product",
		Slug:           "idempotent-prod-" + util.GetRandomString(t, 8),
		Description:    []byte(`{}`),
		IdempotencyKey: &key,
	}

	product, err := testStore.CreateProduct(t.Context(), arg)
	require.NoError(t, err)

	row, err := testStore.GetProductByIdempotencyKey(t.Context(), db.GetProductByIdempotencyKeyParams{
		OrganizationID: organization.ID,
		IdempotencyKey: &key,
	})
	require.NoError(t, err)
	require.Equal(t, product.ID, row.ID)
	require.Equal(t, key, *row.IdempotencyKey)
}

func TestListProductsByOrganization(t *testing.T) {
	organization := createRandomOrganization(t)

	product1 := createRandomProductWithOrg(t, organization)
	product2 := createRandomProductWithOrg(t, organization)

	products, err := testStore.ListProductsByOrganization(t.Context(), organization.ID)
	require.NoError(t, err)
	require.Len(t, products, 2)

	// Ordered by created_at DESC
	require.Equal(t, product2.ID, products[0].ID)
	require.Equal(t, product1.ID, products[1].ID)
}

func TestListActiveProductsAfter(t *testing.T) {
	org := createRandomOrganization(t)

	// Create 2 active products and 1 draft product.
	p1 := createRandomProductWithOrg(t, org)
	_, err := testStore.UpdateProductStatus(t.Context(), db.UpdateProductStatusParams{
		ID:             p1.ID,
		OrganizationID: org.ID,
		Status:         string(util.ProductStatusActive),
	})
	require.NoError(t, err)

	p2 := createRandomProductWithOrg(t, org)
	_, err = testStore.UpdateProductStatus(t.Context(), db.UpdateProductStatusParams{
		ID:             p2.ID,
		OrganizationID: org.ID,
		Status:         string(util.ProductStatusActive),
	})
	require.NoError(t, err)

	draft := createRandomProductWithOrg(t, org) // intentionally left in draft status

	// Fetch active products.
	rows, err := testStore.ListActiveProductsAfter(t.Context(), db.ListActiveProductsAfterParams{
		AfterCreatedAt: time.Now().Add(time.Hour),
		AfterID:        uuid.Nil,
		PageLimit:      10,
	})
	require.NoError(t, err)

	// p1 and p2 must appear; the draft product must not.
	var foundP1, foundP2, foundDraft bool
	for _, r := range rows {
		switch r.ID {
		case p1.ID:
			foundP1 = true
		case p2.ID:
			foundP2 = true
		case draft.ID:
			foundDraft = true
		}
		require.Equal(t, string(util.ProductStatusActive), r.Status)
	}
	require.True(t, foundP1)
	require.True(t, foundP2)
	require.False(t, foundDraft, "draft product must not appear in active product listing")
}
