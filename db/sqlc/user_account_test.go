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

func createRandomUserAccount(t *testing.T) db.CreateUserAccountRow {
	user := createRandomUser(t)

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
	if provider == "credential" {
		accountID = user.ID.String()
		hashedPassword = util.GetRandomHashedPassword(t, 8)
	} else {
		accountID = util.GetRandomString(t, 8)
		accessToken = util.GetRandomStringPtr(t, 8)
		accessTokenExpiresAt = new(time.Now().Add(15 * time.Minute))
		refreshToken = util.GetRandomStringPtr(t, 8)
		refreshTokenExpiresAt = new(time.Now().Add(time.Hour))
		scope = util.GetRandomStringPtr(t, 8)
		idToken = util.GetRandomStringPtr(t, 8)
	}

	arg := db.CreateUserAccountParams{
		UserID:                user.ID,
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

	userAccount, err := testStore.CreateUserAccount(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, userAccount)

	userHashedPassword, err := testStore.GetUserHashedPassword(
		context.Background(),
		db.GetUserHashedPasswordParams{
			UserID:     user.ID,
			ProviderID: provider,
		},
	)
	require.NoError(t, err)

	if provider == "credential" {
		require.NotEmpty(t, hashedPassword)
	}

	require.Equal(t, arg.UserID, userAccount.UserID)
	require.Equal(t, arg.AccountID, userAccount.AccountID)
	require.Equal(t, arg.ProviderID, userAccount.ProviderID)
	require.NotZero(t, userAccount.CreatedAt)

	if provider == "credential" {
		require.Equal(t, *arg.HashedPassword, *userHashedPassword)
		require.Empty(t, userAccount.AccessToken)
		require.Empty(t, userAccount.RefreshToken)
		require.Empty(t, userAccount.AccessTokenExpiresAt)
		require.Empty(t, userAccount.RefreshTokenExpiresAt)
		require.Empty(t, userAccount.Scope)
		require.Empty(t, userAccount.IDToken)
	} else {
		require.Equal(t, *arg.AccessToken, *userAccount.AccessToken)
		require.Equal(t, *arg.RefreshToken, *userAccount.RefreshToken)
		require.WithinDuration(t, *arg.AccessTokenExpiresAt, *userAccount.AccessTokenExpiresAt, time.Second)
		require.WithinDuration(t, *arg.RefreshTokenExpiresAt, *userAccount.RefreshTokenExpiresAt, time.Second)
		require.Equal(t, *arg.Scope, *userAccount.Scope)
		require.Equal(t, *arg.IDToken, *userAccount.IDToken)
		require.Empty(t, userHashedPassword)
	}

	return userAccount
}

func TestCreateUserAccountAndGetHashedPassword(t *testing.T) {
	createRandomUserAccount(t)
}
