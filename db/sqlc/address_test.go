package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomAddressWithOrg(t *testing.T, organization db.Organization) db.Address {
	n := util.CoinFlip(t)
	var line2 *string = nil
	if n == 1 {
		line2 = util.GetRandomStringPtr(t, 8)
	}

	arg := db.CreateAddressParams{
		OrganizationID: organization.ID,
		Type:           util.GetRandomAddressType(t),
		Label:          util.GetRandomString(t, 10),
		Line1:          util.GetRandomString(t, 8),
		Line2:          line2,
		PostalCode:     util.GetRandomNumberString(t, 5),
		City:           util.GetRandomString(t, 8),
		State:          util.GetRandomString(t, 8),
		Country:        util.GetRandomString(t, 8),
	}

	address, err := testStore.CreateAddress(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, address)

	require.Equal(t, arg.OrganizationID, address.OrganizationID)
	require.Equal(t, arg.Type, address.Type)
	require.Equal(t, arg.Label, address.Label)
	require.Equal(t, arg.Line1, address.Line1)
	require.Equal(t, arg.PostalCode, address.PostalCode)
	require.Equal(t, arg.City, address.City)
	require.Equal(t, arg.State, address.State)
	require.Equal(t, arg.Country, address.Country)

	if line2 == nil {
		require.Empty(t, address.Line2)
	} else {
		require.Equal(t, *arg.Line2, *address.Line2)
	}

	return address
}

func createRandomAddress(t *testing.T) db.Address {
	organization := createRandomOrganization(t)
	return createRandomAddressWithOrg(t, organization)
}

func TestCreateAddress(t *testing.T) {
	createRandomAddress(t)
}

func TestGetAddressByID(t *testing.T) {
	address1 := createRandomAddress(t)

	address2, err := testStore.GetAddressByID(context.Background(), address1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, address2)

	require.Equal(t, address1.ID, address2.ID)
	require.Equal(t, address1.OrganizationID, address2.OrganizationID)
	require.Equal(t, address1.Type, address2.Type)
	require.Equal(t, address1.Line1, address2.Line1)
	require.Equal(t, address1.Line2, address2.Line2)
	require.Equal(t, address1.PostalCode, address2.PostalCode)
	require.Equal(t, address1.City, address2.City)
	require.Equal(t, address1.Country, address2.Country)
	require.Equal(t, address1.IsDefaultShipping, address2.IsDefaultShipping)
	require.Equal(t, address1.IsDefaultBilling, address2.IsDefaultBilling)
	require.WithinDuration(t, address1.CreatedAt, address2.CreatedAt, time.Second)
}
