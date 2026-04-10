package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomProductVariantWithOrg(
	t *testing.T,
	organization db.Organization,
) db.ProductVariant {
	product := createRandomProductWithOrg(t, organization)

	arg := db.CreateProductVariantParams{
		ProductID:      product.ID,
		OrganizationID: product.OrganizationID,
		Name:           util.GetRandomString(t, 8),
		Sku:            fmt.Sprintf("%s.%s", product.Slug, util.GetRandomString(t, 8)),
		Price:          util.GetRandomPrice(),
	}

	productVariant, err := testStore.CreateProductVariant(context.Background(), arg)
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
	organization := createRandomOrganization(t)
	return createRandomProductVariantWithOrg(t, organization)
}

func TestCreateProductVariant(t *testing.T) {
	createRandomProductVariant(t)
}

func TestGetProductVariantByID(t *testing.T) {
	productVariant1 := createRandomProductVariant(t)

	productVariant2, err := testStore.GetProductVariantByID(
		context.Background(),
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
	require.WithinDuration(t, productVariant1.CreatedAt, productVariant2.CreatedAt, time.Second)
	require.WithinDuration(t, productVariant1.UpdatedAt, productVariant2.UpdatedAt, time.Second)
}
