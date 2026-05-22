//go:build integration

package db_test

import (
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomCustomerAccount(t *testing.T) db.CreateCustomerAccountRow {
	t.Helper()
	customer := createRandomCustomer(t)

	n, err := rand.Int(rand.Reader, big.NewInt(2))
	require.NoError(t, err)
	require.NotEmpty(t, n)

	provider := util.GetRandomProvider(t)
	require.NotEmpty(t, provider)

	var accountID string
	var hashedPassword *string
	var accessToken *string
	var accessTokenExpiresAt *time.Time
	var refreshToken *string
	var refreshTokenExpiresAt *time.Time
	var scope *string
	var idToken *string
	if provider == string(util.ProviderIDCredential) {
		accountID = customer.ID.String()
		hashedPassword = util.GetRandomHashedPassword(t, 10)
	} else {
		accountID = util.GetRandomString(t, 8)
		accessToken = util.GetRandomStringPtr(t, 8)
		accessTokenExpiresAt = new(time.Now().Add(15 * time.Minute))
		refreshToken = util.GetRandomStringPtr(t, 8)
		refreshTokenExpiresAt = new(time.Now().Add(time.Hour))
		scope = util.GetRandomStringPtr(t, 8)
		idToken = util.GetRandomStringPtr(t, 8)
	}

	arg := db.CreateCustomerAccountParams{
		CustomerID:            customer.ID,
		AccountID:             accountID,
		ProviderID:            provider,
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		AccessTokenExpiresAt:  accessTokenExpiresAt,
		RefreshTokenExpiresAt: refreshTokenExpiresAt,
		Scope:                 scope,
		IDToken:               idToken,
		HashedPassword:        hashedPassword,
	}

	customerAccount, err := testStore.CreateCustomerAccount(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, customerAccount)

	customerHashedPassword, err := testStore.GetCustomerHashedPassword(
		t.Context(),
		db.GetCustomerHashedPasswordParams{
			CustomerID: customer.ID,
			ProviderID: provider,
		},
	)
	require.NoError(t, err)

	if provider == string(util.ProviderIDCredential) {
		require.NotEmpty(t, customerHashedPassword)
	}

	require.Equal(t, arg.CustomerID, customerAccount.CustomerID)
	require.Equal(t, arg.AccountID, customerAccount.AccountID)
	require.Equal(t, arg.ProviderID, customerAccount.ProviderID)
	require.NotZero(t, customerAccount.CreatedAt)

	if provider == string(util.ProviderIDCredential) {
		require.Equal(t, *arg.HashedPassword, *customerHashedPassword)
		require.Empty(t, customerAccount.AccessToken)
		require.Empty(t, customerAccount.RefreshToken)
		require.Empty(t, customerAccount.AccessTokenExpiresAt)
		require.Empty(t, customerAccount.RefreshTokenExpiresAt)
		require.Empty(t, customerAccount.Scope)
		require.Empty(t, customerAccount.IDToken)
	} else {
		require.Equal(t, *arg.AccessToken, *customerAccount.AccessToken)
		require.Equal(t, *arg.RefreshToken, *customerAccount.RefreshToken)
		require.WithinDuration(t, *arg.AccessTokenExpiresAt, *customerAccount.AccessTokenExpiresAt, 5*time.Second)
		require.WithinDuration(t, *arg.RefreshTokenExpiresAt, *customerAccount.RefreshTokenExpiresAt, 5*time.Second)
		require.Equal(t, *arg.Scope, *customerAccount.Scope)
		require.Equal(t, *arg.IDToken, *customerAccount.IDToken)
		require.Empty(t, customerHashedPassword)
	}

	return customerAccount
}

func TestCreateCustomerAccountAndGetHashedPassword(t *testing.T) {
	createRandomCustomerAccount(t)
}

func TestGetCustomerAccountByID(t *testing.T) {
	account1 := createRandomCustomerAccount(t)

	account2, err := testStore.GetCustomerAccountByID(t.Context(), account1.CustomerID)
	require.NoError(t, err)
	require.NotEmpty(t, account2)

	require.Equal(t, account1.ID, account2.ID)
	require.Equal(t, account1.CustomerID, account2.CustomerID)
	require.Equal(t, account1.AccountID, account2.AccountID)
	require.Equal(t, account1.ProviderID, account2.ProviderID)
}

func TestUpdateCustomerAccount(t *testing.T) {
	account1 := createRandomCustomerAccount(t)

	newAccessToken := "new-access-token-" + util.GetRandomString(t, 10)
	newRefreshToken := "new-refresh-token-" + util.GetRandomString(t, 10)

	row, err := testStore.UpdateCustomerAccount(t.Context(), db.UpdateCustomerAccountParams{
		ID:           account1.ID,
		AccessToken:  &newAccessToken,
		RefreshToken: &newRefreshToken,
	})
	require.NoError(t, err)
	require.NotEmpty(t, row)

	require.Equal(t, account1.ID, row.ID)
	require.Equal(t, newAccessToken, *row.AccessToken)
	require.Equal(t, newRefreshToken, *row.RefreshToken)
}
