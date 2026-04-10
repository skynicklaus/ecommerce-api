package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomWarehouseWithOrg(t *testing.T, organization db.Organization) db.Warehouse {
	address := createRandomAddressWithOrg(t, organization)

	arg := db.CreateWarehouseParams{
		OrganizationID: address.OrganizationID,
		AddressID:      address.ID,
		Name:           util.GetRandomString(t, 8),
	}

	warehouse, err := testStore.CreateWarehouse(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, warehouse)

	require.Equal(t, arg.OrganizationID, warehouse.OrganizationID)
	require.Equal(t, arg.AddressID, warehouse.AddressID)
	require.Equal(t, arg.Name, warehouse.Name)

	return warehouse
}

func createRandomWarehouse(t *testing.T) db.Warehouse {
	organization := createRandomOrganization(t)
	return createRandomWarehouseWithOrg(t, organization)
}

func TestCreateWarehouse(t *testing.T) {
	createRandomWarehouse(t)
}
