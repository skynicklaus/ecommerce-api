package db_test

import (
	"context"
	"testing"

	"github.com/go-openapi/testify/v2/require"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomAttributeWithOrg(t *testing.T, organization *db.Organization) db.Attribute {
	var organizationID *uuid.UUID
	if organization != nil {
		organizationID = &organization.ID
	}

	arg := db.CreateAttributeParams{
		OrganizationID: organizationID,
		Name:           util.GetRandomString(t, 8),
		Slug:           util.GetRandomString(t, 10),
		Type:           util.GetRandomString(t, 8),
	}

	attribute, err := testStore.CreateAttribute(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, attribute)

	require.Equal(t, arg.Name, attribute.Name)
	require.Equal(t, arg.Slug, attribute.Slug)
	require.Equal(t, arg.Type, attribute.Type)

	if organization == nil {
		require.Empty(t, attribute.OrganizationID)
	} else {
		require.Equal(t, arg.OrganizationID, attribute.OrganizationID)
	}

	return attribute
}

func createRandomAttribute(t *testing.T) db.Attribute {
	n := util.CoinFlip(t)
	if n == 1 {
		org := createRandomOrganization(t)
		return createRandomAttributeWithOrg(t, &org)
	}
	return createRandomAttributeWithOrg(t, nil)
}

func TestCreateAttribute(t *testing.T) {
	createRandomAttribute(t)
}
