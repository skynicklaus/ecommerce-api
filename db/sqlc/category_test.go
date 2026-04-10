package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomCategory(t *testing.T) db.Category {
	n := util.CoinFlip(t)

	var organizationID *uuid.UUID = nil
	var description *string = nil
	if n == 1 {
		organization := createRandomOrganization(t)
		organizationID = &organization.ID
		description = util.GetRandomStringPtr(t, 20)
	}

	arg := db.CreateCategoryParams{
		OrganizationID: organizationID,
		ParentID:       nil,
		Name:           util.GetRandomString(t, 8),
		Slug:           util.GetRandomString(t, 10),
		Description:    description,
		SortOrder:      util.GetRandomSortOrder(t),
	}

	category, err := testStore.CreateCategory(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, category)

	require.Empty(t, category.ParentID)
	require.Equal(t, arg.Name, category.Name)
	require.Equal(t, arg.Slug, category.Slug)
	require.Equal(t, arg.SortOrder, category.SortOrder)
	require.True(t, category.IsActive)
	require.NotZero(t, category.CreatedAt)

	if n != 1 {
		require.Empty(t, category.OrganizationID)
		require.Empty(t, category.Description)
	} else {
		require.Equal(t, arg.OrganizationID, category.OrganizationID)
		require.Equal(t, *(arg.Description), *(category.Description))
	}

	return category
}

func TestCreateCategory(t *testing.T) {
	createRandomCategory(t)
}

func TestCreateChildCategory(t *testing.T) {
	parentCategory := createRandomCategory(t)

	n := util.CoinFlip(t)

	var organizationID *uuid.UUID
	var description *string
	if n == 1 {
		organization := createRandomOrganization(t)
		organizationID = &organization.ID
		description = util.GetRandomStringPtr(t, 20)
	}

	arg := db.CreateCategoryParams{
		OrganizationID: organizationID,
		ParentID:       &parentCategory.ID,
		Name:           util.GetRandomString(t, 8),
		Slug:           util.GetRandomString(t, 10),
		Description:    description,
		SortOrder:      util.GetRandomSortOrder(t),
	}

	childCategory, err := testStore.CreateCategory(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, childCategory)

	require.NotEmpty(t, childCategory.ParentID)
	require.Equal(t, parentCategory.ID, *(childCategory.ParentID))
	require.Equal(t, arg.Name, childCategory.Name)
	require.Equal(t, arg.Slug, childCategory.Slug)
	require.Equal(t, arg.SortOrder, childCategory.SortOrder)

	if n != 1 {
		require.Empty(t, childCategory.OrganizationID)
		require.Empty(t, childCategory.Description)
	} else {
		require.Equal(t, arg.OrganizationID, childCategory.OrganizationID)
		require.Equal(t, *arg.Description, *childCategory.Description)
	}
}

func TestGetCategoryByID(t *testing.T) {
	category1 := createRandomCategory(t)

	category2, err := testStore.GetCategoryByID(context.Background(), category1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, category2)

	require.Equal(t, category1.ID, category2.ID)
	require.Equal(t, category1.OrganizationID, category2.OrganizationID)
	require.Equal(t, category1.ParentID, category2.ParentID)
	require.Equal(t, category1.Name, category2.Name)
	require.Equal(t, category1.Slug, category2.Slug)
	require.Equal(t, category1.Description, category2.Description)
	require.Equal(t, category1.SortOrder, category2.SortOrder)
	require.Equal(t, category1.IsActive, category2.IsActive)
	require.WithinDuration(t, category1.CreatedAt, category2.CreatedAt, time.Second)
}
