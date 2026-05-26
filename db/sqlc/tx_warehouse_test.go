//go:build integration

package db_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestSQLStore_CreateWarehouseTx(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	line2 := util.GetRandomStringPtr(t, 8)

	result, err := testStore.CreateWarehouseTx(t.Context(), db.CreateWarehouseTxParams{
		OrganizationID: organization.ID,
		Name:           util.GetRandomString(t, 8),
		Address: db.CreateAddressParams{
			Type:       string(util.AddressWarehouse),
			Label:      util.GetRandomString(t, 10),
			Line1:      util.GetRandomString(t, 8),
			Line2:      line2,
			PostalCode: util.GetRandomNumberString(t, 5),
			City:       util.GetRandomString(t, 8),
			State:      util.GetRandomString(t, 8),
			Country:    util.GetRandomString(t, 8),
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Warehouse.ID)
	require.NotEmpty(t, result.Address.ID)
	require.Equal(t, organization.ID, result.Warehouse.OrganizationID)
	require.Equal(t, organization.ID, result.Address.OrganizationID)
	require.Equal(t, result.Address.ID, result.Warehouse.AddressID)
	require.Equal(t, string(util.AddressWarehouse), result.Address.Type)
	require.Equal(t, *line2, *result.Address.Line2)
}

func TestSQLStore_UpdateWarehouseTx(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	created, err := testStore.CreateWarehouseTx(t.Context(), db.CreateWarehouseTxParams{
		OrganizationID: organization.ID,
		Name:           util.GetRandomString(t, 8),
		Address: db.CreateAddressParams{
			Type:       string(util.AddressWarehouse),
			Label:      util.GetRandomString(t, 10),
			Line1:      util.GetRandomString(t, 8),
			Line2:      nil,
			PostalCode: util.GetRandomNumberString(t, 5),
			City:       util.GetRandomString(t, 8),
			State:      util.GetRandomString(t, 8),
			Country:    util.GetRandomString(t, 8),
		},
	})
	require.NoError(t, err)

	newName := util.GetRandomString(t, 8)
	newLabel := util.GetRandomString(t, 10)
	updated, err := testStore.UpdateWarehouseTx(t.Context(), db.UpdateWarehouseTxParams{
		ID:             created.Warehouse.ID,
		OrganizationID: organization.ID,
		Name:           newName,
		IsActive:       true,
		Address: db.UpdateAddressByIDAndOrganizationParams{
			Label:      newLabel,
			Line1:      util.GetRandomString(t, 8),
			Line2:      util.GetRandomStringPtr(t, 8),
			PostalCode: util.GetRandomNumberString(t, 5),
			City:       util.GetRandomString(t, 8),
			State:      util.GetRandomString(t, 8),
			Country:    util.GetRandomString(t, 8),
		},
	})
	require.NoError(t, err)
	require.Equal(t, created.Warehouse.ID, updated.Warehouse.ID)
	require.Equal(t, created.Address.ID, updated.Address.ID)
	require.Equal(t, newName, updated.Warehouse.Name)
	require.True(t, updated.Warehouse.IsActive)
	require.Equal(t, newLabel, updated.Address.Label)
}
