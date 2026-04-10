package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func randomAssignAttributeValueToProductVariant(t *testing.T, organization db.Organization) {
	n := util.CoinFlip(t)
	var org *db.Organization
	if n == 1 {
		org = &organization
	}

	productVariant := createRandomProductVariantWithOrg(t, organization)
	attributeValue := createRandomAttributeValueWithOrg(t, org)

	arg := db.AssignAttributeValueToProductVariantParams{
		ProductVariantID: productVariant.ID,
		AttributeValueID: attributeValue.ID,
	}

	err := testStore.AssignAttributeValueToProductVariant(context.Background(), arg)
	require.NoError(t, err)
}

func TestAssignAttributeValueToProductVariant(t *testing.T) {
	organization := createRandomOrganization(t)
	randomAssignAttributeValueToProductVariant(t, organization)
}
