package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomAttributeValueWithOrg(
	t *testing.T,
	organization *db.Organization,
) db.AttributeValue {
	attribute := createRandomAttributeWithOrg(t, organization)

	arg := db.CreateAttributeValueParams{
		AttributeID:    attribute.ID,
		OrganizationID: attribute.OrganizationID,
		Label:          util.GetRandomString(t, 10),
		Value:          util.GetRandomString(t, 8),
		SortOrder:      util.GetRandomSortOrder(t),
	}

	attributeValue, err := testStore.CreateAttributeValue(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, attributeValue)

	require.Equal(t, arg.AttributeID, attributeValue.AttributeID)
	require.Equal(t, arg.Label, attributeValue.Label)
	require.Equal(t, arg.Value, attributeValue.Value)
	require.Equal(t, arg.SortOrder, attributeValue.SortOrder)

	if organization == nil {
		require.Empty(t, attributeValue.OrganizationID)
	} else {
		require.Equal(t, *arg.OrganizationID, *attributeValue.OrganizationID)
	}

	return attributeValue
}

func createRandomAttributeValue(t *testing.T) db.AttributeValue {
	n := util.CoinFlip(t)
	if n == 1 {
		org := createRandomOrganization(t)
		return createRandomAttributeValueWithOrg(t, &org)
	}

	return createRandomAttributeValueWithOrg(t, nil)
}

func TestCreateAttributeValue(t *testing.T) {
	createRandomAttributeValue(t)
}
