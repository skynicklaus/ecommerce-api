package db_test

import (
	"context"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomUser(t *testing.T) db.User {
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
	user, err := testStore.CreateUser(context.Background(), arg)
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

	user2, err := testStore.GetUserByEmail(context.Background(), user1.Email)
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
