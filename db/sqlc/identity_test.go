package db_test

import (
	"context"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomIdentity(t *testing.T) db.Identity {
	identityType := util.GetRandomIdentityType(t)
	require.NotEmpty(t, identityType)

	identity, err := testStore.CreateIdentity(context.Background(), identityType)
	require.NoError(t, err)
	require.NotEmpty(t, identity)

	require.Equal(t, identityType, identity.Type)
	require.NotZero(t, identity.CreatedAt)

	return identity
}

func TestCreateIdentity(t *testing.T) {
	createRandomIdentity(t)
}
