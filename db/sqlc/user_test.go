//go:build integration

package db_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomUser(t *testing.T) db.User {
	t.Helper()
	identity := createRandomIdentity(t)

	name := util.GetRandomString(t, 8)
	require.NotEmpty(t, name)

	email := util.GetRandomEmail(t, 10)
	require.NotEmpty(t, email)

	arg := db.CreateUserParams{
		IdentityID: identity.ID,
		Name:       name,
		Email:      email,
	}
	user, err := testStore.CreateUser(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, user)

	require.Equal(t, arg.IdentityID, user.IdentityID)
	require.Equal(t, arg.Name, user.Name)
	require.Equal(t, arg.Email, user.Email)
	require.False(t, user.EmailVerified)
	require.Empty(t, user.Image)
	require.NotZero(t, user.CreatedAt)

	return user
}

func TestCreateUser(t *testing.T) {
	createRandomUser(t)
}

func TestGetUserByEmail(t *testing.T) {
	user1 := createRandomUser(t)

	user2, err := testStore.GetUserByEmail(t.Context(), user1.Email)
	require.NoError(t, err)
	require.NotEmpty(t, user2)

	require.Equal(t, user1.ID, user2.ID)
	require.Equal(t, user1.IdentityID, user2.IdentityID)
	require.Equal(t, user1.Name, user2.Name)
	require.Equal(t, user1.Email, user2.Email)
	require.Equal(t, user1.EmailVerified, user2.EmailVerified)
	require.Equal(t, user1.Image, user2.Image)
	require.Equal(t, user1.CreatedAt, user2.CreatedAt)
	require.Equal(t, user1.UpdatedAt, user2.UpdatedAt)
}

func TestGetUserWithCredential(t *testing.T) {
	identity := createRandomIdentity(t)
	name := util.GetRandomString(t, 8)
	email := util.GetRandomEmail(t, 10)

	user, err := testStore.CreateUser(t.Context(), db.CreateUserParams{
		IdentityID: identity.ID,
		Name:       name,
		Email:      email,
	})
	require.NoError(t, err)

	hashedPass := "hashed_argon2id_password_123"
	_, err = testStore.CreateUserAccount(t.Context(), db.CreateUserAccountParams{
		UserID:         user.ID,
		AccountID:      user.ID.String(),
		ProviderID:     string(util.ProviderIDCredential),
		HashedPassword: &hashedPass,
	})
	require.NoError(t, err)

	row, err := testStore.GetUserWithCredential(t.Context(), email)
	require.NoError(t, err)
	require.Equal(t, user.ID, row.ID)
	require.Equal(t, identity.ID, row.IdentityID)
	require.Equal(t, email, row.Email)
	require.Equal(t, hashedPass, *row.HashedPassword)

	identityOAuth := createRandomIdentity(t)
	emailOAuth := util.GetRandomEmail(t, 10)
	userOAuth, err := testStore.CreateUser(t.Context(), db.CreateUserParams{
		IdentityID: identityOAuth.ID,
		Name:       "OAuth User",
		Email:      emailOAuth,
	})
	require.NoError(t, err)

	_, err = testStore.CreateUserAccount(t.Context(), db.CreateUserAccountParams{
		UserID:     userOAuth.ID,
		AccountID:  util.GetRandomString(t, 12),
		ProviderID: string(util.ProviderIDGoogle),
	})
	require.NoError(t, err)

	rowOAuth, err := testStore.GetUserWithCredential(t.Context(), emailOAuth)
	require.NoError(t, err)
	require.Equal(t, userOAuth.ID, rowOAuth.ID)
	require.Equal(t, identityOAuth.ID, rowOAuth.IdentityID)
	require.Nil(t, rowOAuth.HashedPassword)

	_, err = testStore.GetUserWithCredential(t.Context(), "nonexistent@test.com")
	require.Error(t, err)
}

func TestGetUserByIdentityID(t *testing.T) {
	user1 := createRandomUser(t)

	user2, err := testStore.GetUserByIdentityID(t.Context(), user1.IdentityID)
	require.NoError(t, err)
	require.NotEmpty(t, user2)

	require.Equal(t, user1.ID, user2.ID)
	require.Equal(t, user1.IdentityID, user2.IdentityID)
	require.Equal(t, user1.Name, user2.Name)
	require.Equal(t, user1.Email, user2.Email)
}
