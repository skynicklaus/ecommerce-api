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

func createRandomUserAccount(t *testing.T) db.CreateUserAccountRow {
	t.Helper()
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
	if provider == string(util.ProviderIDCredential) {
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

	userAccount, err := testStore.CreateUserAccount(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, userAccount)

	userHashedPassword, err := testStore.GetUserHashedPassword(
		t.Context(),
		db.GetUserHashedPasswordParams{
			UserID:     user.ID,
			ProviderID: provider,
		},
	)
	require.NoError(t, err)

	if provider == string(util.ProviderIDCredential) {
		require.NotEmpty(t, hashedPassword)
	}

	require.Equal(t, arg.UserID, userAccount.UserID)
	require.Equal(t, arg.AccountID, userAccount.AccountID)
	require.Equal(t, arg.ProviderID, userAccount.ProviderID)
	require.NotZero(t, userAccount.CreatedAt)

	if provider == string(util.ProviderIDCredential) {
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
		require.WithinDuration(t, *arg.AccessTokenExpiresAt, *userAccount.AccessTokenExpiresAt, 5*time.Second)
		require.WithinDuration(t, *arg.RefreshTokenExpiresAt, *userAccount.RefreshTokenExpiresAt, 5*time.Second)
		require.Equal(t, *arg.Scope, *userAccount.Scope)
		require.Equal(t, *arg.IDToken, *userAccount.IDToken)
		require.Empty(t, userHashedPassword)
	}

	return userAccount
}

func TestCreateUserAccountAndGetHashedPassword(t *testing.T) {
	createRandomUserAccount(t)
}

func TestGetUserAccountByID(t *testing.T) {
	account1 := createRandomUserAccount(t)

	account2, err := testStore.GetUserAccountByID(t.Context(), account1.UserID)
	require.NoError(t, err)
	require.NotEmpty(t, account2)

	require.Equal(t, account1.ID, account2.ID)
	require.Equal(t, account1.UserID, account2.UserID)
	require.Equal(t, account1.AccountID, account2.AccountID)
	require.Equal(t, account1.ProviderID, account2.ProviderID)
}

func TestUpdateUserAccount(t *testing.T) {
	account1 := createRandomUserAccount(t)

	newAccessToken := "new-access-token-" + util.GetRandomString(t, 10)
	newRefreshToken := "new-refresh-token-" + util.GetRandomString(t, 10)

	row, err := testStore.UpdateUserAccount(t.Context(), db.UpdateUserAccountParams{
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
