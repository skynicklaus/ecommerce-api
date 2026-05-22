//go:build integration

package db_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomCustomer(t *testing.T) db.Customer {
	t.Helper()
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
	customer, err := testStore.CreateCustomer(t.Context(), arg)
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

	customer2, err := testStore.GetCustomerByEmail(t.Context(), customer1.Email)
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

func TestGetCustomerWithCredential(t *testing.T) {
	identity := createRandomIdentity(t)
	name := util.GetRandomString(t, 8)
	email := util.GetRandomEmail(t, 10)

	customer, err := testStore.CreateCustomer(t.Context(), db.CreateCustomerParams{
		IdentityID: identity.ID,
		Name:       name,
		Email:      email,
	})
	require.NoError(t, err)

	hashedPass := "hashed_argon2id_password_123"
	_, err = testStore.CreateCustomerAccount(t.Context(), db.CreateCustomerAccountParams{
		CustomerID:     customer.ID,
		AccountID:      customer.ID.String(),
		ProviderID:     string(util.ProviderIDCredential),
		HashedPassword: &hashedPass,
	})
	require.NoError(t, err)

	row, err := testStore.GetCustomerWithCredential(t.Context(), email)
	require.NoError(t, err)
	require.Equal(t, customer.ID, row.ID)
	require.Equal(t, identity.ID, row.IdentityID)
	require.Equal(t, email, row.Email)
	require.Equal(t, hashedPass, *row.HashedPassword)

	identityOAuth := createRandomIdentity(t)
	emailOAuth := util.GetRandomEmail(t, 10)
	customerOAuth, err := testStore.CreateCustomer(t.Context(), db.CreateCustomerParams{
		IdentityID: identityOAuth.ID,
		Name:       "OAuth User",
		Email:      emailOAuth,
	})
	require.NoError(t, err)

	_, err = testStore.CreateCustomerAccount(t.Context(), db.CreateCustomerAccountParams{
		CustomerID: customerOAuth.ID,
		AccountID:  util.GetRandomString(t, 12),
		ProviderID: string(util.ProviderIDGoogle),
	})
	require.NoError(t, err)

	rowOAuth, err := testStore.GetCustomerWithCredential(t.Context(), emailOAuth)
	require.NoError(t, err)
	require.Equal(t, customerOAuth.ID, rowOAuth.ID)
	require.Equal(t, identityOAuth.ID, rowOAuth.IdentityID)
	require.Nil(t, rowOAuth.HashedPassword)

	_, err = testStore.GetCustomerWithCredential(t.Context(), "nonexistent@test.com")
	require.Error(t, err)
}

func TestGetCustomerByIdentityID(t *testing.T) {
	customer1 := createRandomCustomer(t)

	customer2, err := testStore.GetCustomerByIdentityID(t.Context(), customer1.IdentityID)
	require.NoError(t, err)
	require.NotEmpty(t, customer2)

	require.Equal(t, customer1.ID, customer2.ID)
	require.Equal(t, customer1.IdentityID, customer2.IdentityID)
	require.Equal(t, customer1.Name, customer2.Name)
	require.Equal(t, customer1.Email, customer2.Email)
}
