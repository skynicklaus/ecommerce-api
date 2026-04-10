package db_test

import (
	"context"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomCustomer(t *testing.T) db.Customer {
	identity := createRandomIdentity(t)

	name := util.GetRandomString(t, 8)
	require.NotEmpty(t, name)

	email := util.GetRandomEmail(t, 10)
	require.NotEmpty(t, email)

	arg := db.CreateCustomerParams{
		IdentityID: identity.ID,
		Name:       name,
		Email:      email,
	}
	customer, err := testStore.CreateCustomer(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, customer)

	require.Equal(t, arg.IdentityID, customer.IdentityID)
	require.Equal(t, arg.Name, customer.Name)
	require.Equal(t, arg.Email, customer.Email)
	require.False(t, customer.EmailVerified)
	require.Empty(t, customer.Image)
	require.NotZero(t, customer.CreatedAt)

	return customer
}

func TestCreateCustomer(t *testing.T) {
	createRandomCustomer(t)
}

func TestGetCustomerByEmail(t *testing.T) {
	customer1 := createRandomCustomer(t)

	customer2, err := testStore.GetCustomerByEmail(context.Background(), customer1.Email)
	require.NoError(t, err)
	require.NotEmpty(t, customer2)

	require.Equal(t, customer1.ID, customer2.ID)
	require.Equal(t, customer1.IdentityID, customer2.IdentityID)
	require.Equal(t, customer1.Name, customer2.Name)
	require.Equal(t, customer1.Email, customer2.Email)
	require.Equal(t, customer1.EmailVerified, customer2.EmailVerified)
	require.Equal(t, customer1.Image, customer2.Image)
	require.Equal(t, customer1.CreatedAt, customer2.CreatedAt)
	require.Equal(t, customer1.UpdatedAt, customer2.UpdatedAt)
}
