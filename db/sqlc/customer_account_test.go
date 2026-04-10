package db_test

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomCustomerAccount(t *testing.T) db.CreateCustomerAccountRow {
	customer := createRandomCustomer(t)

	n, err := rand.Int(rand.Reader, big.NewInt(2))
	require.NoError(t, err)
	require.NotEmpty(t, n)

	provider := util.GetRandomProvider(t)
	require.NotEmpty(t, provider)

	var accountID string
	var hashedPassword *string = nil
	var accessToken *string = nil
	var accessTokenExpiresAt *time.Time = nil
	var refreshToken *string = nil
	var refreshTokenExpiresAt *time.Time = nil
	var scope *string = nil
	var idToken *string = nil
	if provider == "credential" {
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

	customerAccount, err := testStore.CreateCustomerAccount(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, customerAccount)

	customerHashedPassword, err := testStore.GetCustomerHashedPassword(
		context.Background(),
		db.GetCustomerHashedPasswordParams{
			CustomerID: customer.ID,
			ProviderID: provider,
		},
	)
	require.NoError(t, err)

	if provider == "credential" {
		require.NotEmpty(t, customerHashedPassword)
	}

	require.Equal(t, arg.CustomerID, customerAccount.CustomerID)
	require.Equal(t, arg.AccountID, customerAccount.AccountID)
	require.Equal(t, arg.ProviderID, customerAccount.ProviderID)
	require.NotZero(t, customerAccount.CreatedAt)

	if provider == "credential" {
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
		require.WithinDuration(t, *arg.AccessTokenExpiresAt, *customerAccount.AccessTokenExpiresAt, time.Second)
		require.WithinDuration(t, *arg.RefreshTokenExpiresAt, *customerAccount.RefreshTokenExpiresAt, time.Second)
		require.Equal(t, *arg.Scope, *customerAccount.Scope)
		require.Equal(t, *arg.IDToken, *customerAccount.IDToken)
		require.Empty(t, customerHashedPassword)
	}

	return customerAccount
}

func TestCreateCustomerAccountAndGetHashedPassword(t *testing.T) {
	createRandomCustomerAccount(t)
}
