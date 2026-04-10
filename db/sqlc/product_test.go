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

func createRandomProductWithOrg(t *testing.T, organization db.Organization) db.Product {
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

	product, err := testStore.CreateProduct(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, product)

	require.Equal(t, arg.OrganizationID, product.OrganizationID)
	require.Equal(t, arg.CategoryID, product.CategoryID)
	require.Equal(t, arg.Name, product.Name)
	require.Equal(t, arg.Slug, product.Slug)
	require.Equal(t, "draft", product.Status)
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
	organization := createRandomOrganization(t)
	return createRandomProductWithOrg(t, organization)
}

func TestCreateProduct(t *testing.T) {
	createRandomProduct(t)
}

func TestGetProductByID(t *testing.T) {
	product1 := createRandomProduct(t)

	product2, err := testStore.GetProductByID(context.Background(), product1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, product2)

	require.Equal(t, product1.ID, product2.ID)
	require.Equal(t, product1.OrganizationID, product2.OrganizationID)
	require.Equal(t, product1.CategoryID, product2.CategoryID)
	require.Equal(t, product1.Name, product2.Name)
	require.Equal(t, product1.Slug, product2.Slug)
	require.Equal(t, product1.Status, product2.Status)
	require.JSONEq(t, string(product1.Description), string(product2.Description))
	require.WithinDuration(t, product1.CreatedAt, product2.CreatedAt, time.Second)
	require.WithinDuration(t, product1.UpdatedAt, product2.UpdatedAt, time.Second)

	if len(product1.Specification) > 0 {
		require.JSONEq(t, string(product1.Specification), string(product2.Specification))
	} else {
		require.Equal(t, product1.Specification, product2.Specification)
	}
}
